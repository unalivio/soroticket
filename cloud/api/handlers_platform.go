package main

import (
	"math/big"
	"net/http"
	"time"
)

// handleOverview aggregates the dashboard KPIs, 30-day series and the latest
// activity in one call.
func (s *server) handleOverview(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	now := time.Now()
	from30 := now.AddDate(0, 0, -30).Unix()

	var redemptions30 int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM redemptions WHERE org_id = ? AND env = ? AND ok = 1 AND created_at >= ?`,
		a.OrgID, a.Env, from30).Scan(&redemptions30); err != nil {
		writeInternal(w, err, "count overview redemptions")
		return
	}
	var events30 int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e
	  JOIN shared_codes sc ON sc.id = e.shared_code_id JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.org_id = ? AND c.env = ? AND e.created_at >= ?`, a.OrgID, a.Env, from30).Scan(&events30); err != nil {
		writeInternal(w, err, "count overview shared events")
		return
	}
	var activeCampaigns int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE org_id = ? AND env = ? AND archived = 0 AND valid_until > ?`,
		a.OrgID, a.Env, now.Unix()).Scan(&activeCampaigns); err != nil {
		writeInternal(w, err, "count active campaigns")
		return
	}

	// settled total (token base-units) across settled tallies
	settledTotal := big.NewInt(0)
	rows, err := s.db.Query(`SELECT COALESCE(t.payout_amount,'0') FROM tallies t
	  JOIN shared_codes sc ON sc.id = t.shared_code_id JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.org_id = ? AND c.env = ? AND t.settled = 1`, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "summarize settled payouts")
		return
	}
	for rows.Next() {
		var amt string
		if err := rows.Scan(&amt); err != nil {
			rows.Close()
			writeInternal(w, err, "decode settled payout")
			return
		}
		if parsed, ok := new(big.Int).SetString(amt, 10); ok && parsed.Sign() >= 0 {
			settledTotal.Add(settledTotal, parsed)
		}
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeInternal(w, rowsErr, "read settled payouts")
		return
	}

	// per-day series (successful redemptions + shared events), last 30 days
	series := make([]map[string]any, 0, 30)
	for i := 29; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dayStartTime := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		dayStart := dayStartTime.Unix()
		dayEnd := dayStartTime.AddDate(0, 0, 1).Unix()
		var n int64
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM redemptions WHERE org_id = ? AND env = ? AND ok = 1 AND created_at >= ? AND created_at < ?`,
			a.OrgID, a.Env, dayStart, dayEnd).Scan(&n); err != nil {
			writeInternal(w, err, "build daily redemption series")
			return
		}
		var ev int64
		if err := s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e
		  JOIN shared_codes sc ON sc.id = e.shared_code_id JOIN campaigns c ON c.id = sc.campaign_id
		  WHERE c.org_id = ? AND c.env = ? AND e.created_at >= ? AND e.created_at < ?`,
			a.OrgID, a.Env, dayStart, dayEnd).Scan(&ev); err != nil {
			writeInternal(w, err, "build daily shared-event series")
			return
		}
		series = append(series, map[string]any{"date": day.Format("Jan 2"), "count": n + ev})
	}

	out := map[string]any{
		"redemptions_30d":          redemptions30 + events30,
		"active_campaigns":         activeCampaigns,
		"settled_total_base_units": settledTotal.String(),
		"settled_unit":             payoutUnit,
		"series":                   series,
		"activity":                 s.recentActivity(a, 8),
	}
	if a.Env == "live" {
		if err := s.ensureMonthlyGrant(a.OrgID); err != nil {
			writeInternal(w, err, "refresh overview credits")
			return
		}
		var bal int64
		if err := s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id = ? AND env = 'live'`, a.OrgID).Scan(&bal); err != nil {
			writeInternal(w, err, "load overview credits")
			return
		}
		out["credits_mcr"] = bal
	}
	// funding status so the console can show "setting up your account"
	var funded int
	if err := s.db.QueryRow(`SELECT funded FROM org_accounts WHERE org_id = ? AND env = ?`, a.OrgID, a.Env).Scan(&funded); err != nil {
		writeInternal(w, err, "load account funding status")
		return
	}
	out["account_funded"] = funded == 1
	// active key count lets the onboarding checklist complete its last step
	var apiKeys int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE org_id = ? AND env = ? AND revoked_at IS NULL`,
		a.OrgID, a.Env).Scan(&apiKeys); err != nil {
		writeInternal(w, err, "count overview api keys")
		return
	}
	out["api_keys"] = apiKeys
	writeJSON(w, 200, out)
}

func (s *server) recentActivity(a *authCtx, limit int) []map[string]any {
	rows, err := s.db.Query(`SELECT ts, kind, COALESCE(code,''), message, COALESCE(tx_hash,''),
	  COALESCE(error_code,0), COALESCE(campaign_id,0)
	  FROM activity WHERE org_id = ? AND env = ? ORDER BY id DESC LIMIT ?`, a.OrgID, a.Env, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var ts, ecode, cid int64
		var kind, code, msg, tx string
		if err := rows.Scan(&ts, &kind, &code, &msg, &tx, &ecode, &cid); err != nil {
			return []map[string]any{}
		}
		out = append(out, map[string]any{
			"ts": ts, "kind": kind, "code": code, "message": msg,
			"tx_hash": tx, "error_code": ecode, "campaign_id": cid,
		})
	}
	if rows.Err() != nil {
		return []map[string]any{}
	}
	return out
}

func (s *server) handleActivity(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	writeJSON(w, 200, map[string]any{"activity": s.recentActivity(a, 100)})
}

// ── usage & credits ─────────────────────────────────────────────────

func (s *server) handleCredits(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	out := map[string]any{
		"env":               a.Env,
		"mode":              externalMode(a.Env),
		"network":           "testnet",
		"production":        false,
		"recharges_enabled": false,
		"monthly_grant_mcr": monthlyGrantMcr,
		"metered":           a.Env == "live",
		"price_table": []map[string]any{
			{"operation": "Reads (verify, stats, lists)", "mcr": 0},
			{"operation": "Create campaign", "mcr": mcrCreateCampaign},
			{"operation": "Issue unique code", "mcr": mcrIssuePerCode, "per": "code"},
			{"operation": "Redeem (burn)", "mcr": mcrRedeem},
			{"operation": "Register shared code", "mcr": mcrRegisterShared},
			{"operation": "Record shared-code event", "mcr": mcrSharedEvent},
			{"operation": "Commit tally", "mcr": mcrCommitTally},
			{"operation": "Settle period", "mcr": mcrSettle},
			{"operation": "Loyalty punch", "mcr": mcrPunch},
		},
	}
	if a.Env == "live" {
		if err := s.ensureMonthlyGrant(a.OrgID); err != nil {
			writeInternal(w, err, "refresh credits")
			return
		}
		var bal int64
		var gm string
		if err := s.db.QueryRow(`SELECT balance_mcr, grant_month FROM credits WHERE org_id = ? AND env = 'live'`, a.OrgID).
			Scan(&bal, &gm); err != nil {
			writeInternal(w, err, "load credits")
			return
		}
		out["balance_mcr"] = bal
		out["grant_month"] = gm
	}
	rows, err := s.db.Query(`SELECT ts, operation, COALESCE(detail,''), delta_mcr, balance_mcr
	  FROM credit_ledger WHERE org_id = ? AND env = ? ORDER BY id DESC LIMIT 100`, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "load credit ledger")
		return
	}
	ledger := []map[string]any{}
	for rows.Next() {
		var ts, delta, bal int64
		var op, detail string
		if err := rows.Scan(&ts, &op, &detail, &delta, &bal); err != nil {
			rows.Close()
			writeInternal(w, err, "decode credit ledger")
			return
		}
		ledger = append(ledger, map[string]any{
			"ts": ts, "operation": op, "detail": detail, "delta_mcr": delta, "balance_mcr": bal,
		})
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeInternal(w, rowsErr, "read credit ledger")
		return
	}
	out["ledger"] = ledger
	writeJSON(w, 200, out)
}

// handleUsage returns per-day, per-operation counts for the usage chart.
func (s *server) handleUsage(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	from := time.Now().AddDate(0, 0, -30).Unix()
	rows, err := s.db.Query(`SELECT operation, COUNT(*), COALESCE(SUM(-delta_mcr),0)
	  FROM credit_ledger WHERE org_id = ? AND env = ? AND ts >= ? AND delta_mcr < 0
	  GROUP BY operation ORDER BY 3 DESC`, a.OrgID, a.Env, from)
	if err != nil {
		writeInternal(w, err, "read usage")
		return
	}
	defer rows.Close()
	byOp := []map[string]any{}
	for rows.Next() {
		var op string
		var count, spent int64
		if err := rows.Scan(&op, &count, &spent); err != nil {
			writeInternal(w, err, "decode usage row")
			return
		}
		byOp = append(byOp, map[string]any{"operation": op, "count": count, "spent_mcr": spent})
	}
	if err := rows.Err(); err != nil {
		writeInternal(w, err, "read usage rows")
		return
	}
	// operation counts also exist in test (unmetered) via activity
	var ops30 int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM activity WHERE org_id = ? AND env = ? AND ts >= ?`, a.OrgID, a.Env, from).
		Scan(&ops30); err != nil {
		writeInternal(w, err, "count usage operations")
		return
	}
	writeJSON(w, 200, map[string]any{"by_operation": byOp, "operations_30d": ops30})
}

// handleRecharge fails closed until a payment processor or a verified on-chain
// deposit watcher is configured. Returning an org's own custodial address as a
// pretend payment destination would create an untrackable, unsafe billing flow.
func (s *server) handleRecharge(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, http.StatusNotImplemented,
		"credit recharges are disabled until payment confirmation is implemented; no payment address has been issued")
}
