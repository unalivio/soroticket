package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Credits are metered in MILLIcredits (1 cr = 1,000 mcr) so sub-credit prices
// stay exact integers. 1,000 cr = $1 in preview pricing (docs/CLOUD.md); no
// payment processor or real-money billing is enabled.
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
func (s *server) ensureMonthlyGrant(orgID int64) error {
	month := time.Now().UTC().Format("2006-01")
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var bal int64
	var gm string
	err = tx.QueryRow(`SELECT balance_mcr, grant_month FROM credits WHERE org_id = ? AND env = 'live'`, orgID).
		Scan(&bal, &gm)
	if err == sql.ErrNoRows {
		if _, err = tx.Exec(
			`INSERT INTO credits (org_id, env, balance_mcr, grant_month) VALUES (?, 'live', ?, ?)`,
			orgID, monthlyGrantMcr, month,
		); err != nil {
			return err
		}
		if err = insertLedgerTx(tx, orgID, "live", "monthly_grant", "opening grant", monthlyGrantMcr, monthlyGrantMcr, ""); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if gm != month && bal < monthlyGrantMcr {
		delta := monthlyGrantMcr - bal
		if _, err = tx.Exec(`UPDATE credits SET balance_mcr = ?, grant_month = ? WHERE org_id = ? AND env = 'live'`,
			monthlyGrantMcr, month, orgID); err != nil {
			return err
		}
		if err = insertLedgerTx(tx, orgID, "live", "monthly_grant", "monthly top-up", delta, monthlyGrantMcr, ""); err != nil {
			return err
		}
	} else if gm != month {
		if _, err = tx.Exec(`UPDATE credits SET grant_month = ? WHERE org_id = ? AND env = 'live'`, month, orgID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// charge debits the live balance (test is never metered). It returns false —
// and writes a 402 — when the balance can't cover the operation.
func (s *server) charge(w http.ResponseWriter, a *authCtx, operation, detail string, mcr int64, txHash string) bool {
	if a.Env != "live" || mcr == 0 {
		return true
	}
	if mcr < 0 {
		writeProblem(w, http.StatusInternalServerError, "invalid negative charge")
		return false
	}
	if err := s.ensureMonthlyGrant(a.OrgID); err != nil {
		writeProblem(w, 500, "could not refresh credits")
		return false
	}
	tx, err := s.db.Begin()
	if err != nil {
		writeProblem(w, 500, "could not reserve credits")
		return false
	}
	defer tx.Rollback()
	var balance int64
	err = tx.QueryRow(`UPDATE credits SET balance_mcr = balance_mcr - ?
	  WHERE org_id = ? AND env = 'live' AND balance_mcr >= ? RETURNING balance_mcr`,
		mcr, a.OrgID, mcr).Scan(&balance)
	if err == sql.ErrNoRows {
		writeProblem(w, http.StatusPaymentRequired, "insufficient credits — recharge to continue")
		return false
	}
	if err != nil {
		writeProblem(w, 500, "could not reserve credits")
		return false
	}
	if err = insertLedgerTx(tx, a.OrgID, "live", operation, detail, -mcr, balance, txHash); err != nil {
		writeProblem(w, 500, "could not record credit charge")
		return false
	}
	if err = tx.Commit(); err != nil {
		writeProblem(w, 500, "could not commit credit charge")
		return false
	}
	return true
}

func (s *server) maybeNotifyLowCredits(a *authCtx) {
	if a.Env != "live" {
		return
	}
	var balance int64
	if err := s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id=? AND env='live'`, a.OrgID).Scan(&balance); err != nil {
		log.Printf("read low-credit balance org=%d: %v", a.OrgID, err)
		return
	}
	if balance < monthlyGrantMcr/10 {
		s.notifyLowCredits(a, balance)
	}
}

func (s *server) notifyLowCredits(a *authCtx, balance int64) {
	month := time.Now().UTC().Format("2006-01")
	result, err := s.db.Exec(`INSERT OR IGNORE INTO credit_alerts
	  (org_id, env, month, alert_type, created_at) VALUES (?,?,?,'low',?)`,
		a.OrgID, a.Env, month, time.Now().Unix())
	if err != nil {
		log.Printf("record low-credit alert org=%d: %v", a.OrgID, err)
		return
	}
	inserted, _ := result.RowsAffected()
	if inserted == 1 {
		s.logActivity(a, "credits_low", fmt.Sprintf("%d", balance),
			fmt.Sprintf("Credit balance is low · %d mcr remaining", balance), "", nil, 0)
	}
}

type chargeReservation struct {
	s         *server
	a         *authCtx
	operation string
	detail    string
	mcr       int64
	active    bool
}

func (s *server) reserveCharge(w http.ResponseWriter, a *authCtx, operation, detail string, mcr int64) (*chargeReservation, bool) {
	if !s.charge(w, a, operation, detail, mcr, "") {
		return nil, false
	}
	return &chargeReservation{s: s, a: a, operation: operation, detail: detail, mcr: mcr, active: a.Env == "live" && mcr > 0}, true
}

func (r *chargeReservation) Commit() {
	if r != nil && r.active {
		r.active = false
		r.s.maybeNotifyLowCredits(r.a)
	}
}

// CommitUsed keeps only the portion actually consumed by a partially
// successful batch and refunds the remainder.
func (r *chargeReservation) CommitUsed(usedMcr int64) {
	if r == nil || !r.active {
		return
	}
	if usedMcr < 0 {
		usedMcr = 0
	}
	if usedMcr > r.mcr {
		usedMcr = r.mcr
	}
	r.active = false
	if refund := r.mcr - usedMcr; refund > 0 {
		if err := r.s.refundCharge(r.a, r.operation, r.detail, refund); err != nil {
			log.Printf("partial credit refund failed for org=%d operation=%s: %v", r.a.OrgID, r.operation, err)
		}
	}
	if usedMcr > 0 {
		r.s.maybeNotifyLowCredits(r.a)
	}
}

func (r *chargeReservation) Refund() {
	if r == nil || !r.active {
		return
	}
	r.active = false
	if err := r.s.refundCharge(r.a, r.operation, r.detail, r.mcr); err != nil {
		log.Printf("credit refund failed for org=%d operation=%s: %v", r.a.OrgID, r.operation, err)
	}
}

func (s *server) refundCharge(a *authCtx, operation, detail string, mcr int64) error {
	if a.Env != "live" || mcr <= 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var balance int64
	if err = tx.QueryRow(`UPDATE credits SET balance_mcr = balance_mcr + ?
	  WHERE org_id = ? AND env = 'live' RETURNING balance_mcr`, mcr, a.OrgID).Scan(&balance); err != nil {
		return err
	}
	if err = insertLedgerTx(tx, a.OrgID, "live", operation+"_refund", "refund · "+detail, mcr, balance, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func insertLedgerTx(tx *sql.Tx, orgID int64, env, operation, detail string, deltaMcr, balance int64, txHash string) error {
	_, err := tx.Exec(`INSERT INTO credit_ledger (org_id, env, ts, operation, detail, delta_mcr, balance_mcr, tx_hash)
	  VALUES (?,?,?,?,?,?,?,?)`, orgID, env, time.Now().Unix(), operation, detail, deltaMcr, balance, txHash)
	return err
}

func (s *server) ledger(orgID int64, env, operation, detail string, deltaMcr int64, txHash string) {
	var bal int64
	if err := s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id = ? AND env = ?`, orgID, env).Scan(&bal); err != nil {
		log.Printf("read credit balance for ledger: %v", err)
		return
	}
	if _, err := s.db.Exec(
		`INSERT INTO credit_ledger (org_id, env, ts, operation, detail, delta_mcr, balance_mcr, tx_hash)
		 VALUES (?,?,?,?,?,?,?,?)`,
		orgID, env, time.Now().Unix(), operation, detail, deltaMcr, bal, txHash,
	); err != nil {
		log.Printf("write credit ledger: %v", err)
	}
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
	now := time.Now().Unix()
	result, err := s.db.Exec(`INSERT INTO activity (org_id, env, ts, kind, code, message, tx_hash, error_code, campaign_id)
	  VALUES (?,?,?,?,?,?,?,?,?)`, a.OrgID, a.Env, now, kind, code, message, txHash, ec, cid)
	if err != nil {
		log.Printf("write activity org=%d env=%s: %v", a.OrgID, a.Env, err)
		return
	}
	activityID, err := result.LastInsertId()
	if err != nil {
		log.Printf("read activity id org=%d env=%s: %v", a.OrgID, a.Env, err)
		return
	}
	s.enqueueActivityWebhook(a, activityID, kind, code, message, txHash, campaignID, errorCode, now)
}
