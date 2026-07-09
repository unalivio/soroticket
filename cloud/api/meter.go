package main

import (
	"net/http"
	"time"
)

// Credits are metered in MILLIcredits (1 cr = 1,000 mcr) so sub-credit prices
// stay exact integers. 1,000 cr = $1 (PLACEHOLDER pricing, per docs/CLOUD.md).
const (
	mcrCreateCampaign = 20_000 // 20 cr
	mcrIssuePerCode   = 2_000  // 2 cr / code
	mcrRedeem         = 5_000  // 5 cr
	mcrRegisterShared = 10_000 // 10 cr
	mcrSharedEvent    = 200    // 0.2 cr
	mcrCommitTally    = 15_000 // 15 cr
	mcrSettle         = 25_000 // 25 cr
	mcrPunch          = 200    // 0.2 cr

	monthlyGrantMcr = 25_000_000 // 25,000 cr free each month (non-accumulating)
)

// ensureMonthlyGrant tops the live balance up to the monthly grant once per
// calendar month (non-accumulating: it raises the balance to the grant floor,
// never stacks).
func (s *server) ensureMonthlyGrant(orgID int64) {
	month := time.Now().UTC().Format("2006-01")
	var bal int64
	var gm string
	err := s.db.QueryRow(`SELECT balance_mcr, grant_month FROM credits WHERE org_id = ? AND env = 'live'`, orgID).
		Scan(&bal, &gm)
	if err != nil {
		_, _ = s.db.Exec(`INSERT INTO credits (org_id, env, balance_mcr, grant_month) VALUES (?, 'live', ?, ?)`,
			orgID, monthlyGrantMcr, month)
		s.ledger(orgID, "live", "monthly_grant", "opening grant", monthlyGrantMcr, "")
		return
	}
	if gm != month && bal < monthlyGrantMcr {
		delta := monthlyGrantMcr - bal
		_, _ = s.db.Exec(`UPDATE credits SET balance_mcr = ?, grant_month = ? WHERE org_id = ? AND env = 'live'`,
			monthlyGrantMcr, month, orgID)
		s.ledger(orgID, "live", "monthly_grant", "monthly top-up", delta, "")
	} else if gm != month {
		_, _ = s.db.Exec(`UPDATE credits SET grant_month = ? WHERE org_id = ? AND env = 'live'`, month, orgID)
	}
}

// charge debits the live balance (test is never metered). It returns false —
// and writes a 402 — when the balance can't cover the operation.
func (s *server) charge(w http.ResponseWriter, a *authCtx, operation, detail string, mcr int64, txHash string) bool {
	if a.Env != "live" || mcr == 0 {
		return true
	}
	s.ensureMonthlyGrant(a.OrgID)
	var bal int64
	if err := s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id = ? AND env = 'live'`, a.OrgID).Scan(&bal); err != nil {
		writeProblem(w, 500, "credits account missing")
		return false
	}
	if bal < mcr {
		writeProblem(w, http.StatusPaymentRequired, "insufficient credits — recharge to continue")
		return false
	}
	_, _ = s.db.Exec(`UPDATE credits SET balance_mcr = balance_mcr - ? WHERE org_id = ? AND env = 'live'`, mcr, a.OrgID)
	s.ledger(a.OrgID, "live", operation, detail, -mcr, txHash)
	return true
}

func (s *server) ledger(orgID int64, env, operation, detail string, deltaMcr int64, txHash string) {
	var bal int64
	_ = s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id = ? AND env = ?`, orgID, env).Scan(&bal)
	_, _ = s.db.Exec(`INSERT INTO credit_ledger (org_id, env, ts, operation, detail, delta_mcr, balance_mcr, tx_hash)
	  VALUES (?,?,?,?,?,?,?,?)`, orgID, env, time.Now().Unix(), operation, detail, deltaMcr, bal, txHash)
}

func (s *server) logActivity(a *authCtx, kind, code, message, txHash string, campaignID *int64, errorCode int) {
	var cid any
	if campaignID != nil {
		cid = *campaignID
	}
	var ec any
	if errorCode != 0 {
		ec = errorCode
	}
	_, _ = s.db.Exec(`INSERT INTO activity (org_id, env, ts, kind, code, message, tx_hash, error_code, campaign_id)
	  VALUES (?,?,?,?,?,?,?,?,?)`, a.OrgID, a.Env, time.Now().Unix(), kind, code, message, txHash, ec, cid)
}
