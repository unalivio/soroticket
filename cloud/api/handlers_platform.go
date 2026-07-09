package main

import (
	"net/http"
	"strconv"
	"time"
)

// handleOverview aggregates the dashboard KPIs, 30-day series and the latest
// activity in one call.
func (s *server) handleOverview(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	now := time.Now()
	from30 := now.AddDate(0, 0, -30).Unix()

	var redemptions30 int64
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM redemptions WHERE org_id = ? AND env = ? AND ok = 1 AND created_at >= ?`,
		a.OrgID, a.Env, from30).Scan(&redemptions30)
	var events30 int64
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e
	  JOIN shared_codes sc ON sc.id = e.shared_code_id JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.org_id = ? AND c.env = ? AND e.created_at >= ?`, a.OrgID, a.Env, from30).Scan(&events30)
	var activeCampaigns int64
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM campaigns WHERE org_id = ? AND env = ? AND archived = 0 AND valid_until > ?`,
		a.OrgID, a.Env, now.Unix()).Scan(&activeCampaigns)

	// settled total (token base-units) across settled tallies
	var settledTotal float64
	rows, _ := s.db.Query(`SELECT COALESCE(t.payout_amount,'0') FROM tallies t
	  JOIN shared_codes sc ON sc.id = t.shared_code_id JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.org_id = ? AND c.env = ? AND t.settled = 1`, a.OrgID, a.Env)
	for rows.Next() {
		var amt string
		_ = rows.Scan(&amt)
		f, _ := strconv.ParseFloat(amt, 64)
		settledTotal += f / 1e7
	}
	rows.Close()

	// per-day series (successful redemptions + shared events), last 30 days
	series := make([]map[string]any, 0, 30)
	for i := 29; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).Unix()
		dayEnd := dayStart + 86400
		var n int64
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM redemptions WHERE org_id = ? AND env = ? AND ok = 1 AND created_at >= ? AND created_at < ?`,
			a.OrgID, a.Env, dayStart, dayEnd).Scan(&n)
		var ev int64
		_ = s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e
		  JOIN shared_codes sc ON sc.id = e.shared_code_id JOIN campaigns c ON c.id = sc.campaign_id
		  WHERE c.org_id = ? AND c.env = ? AND e.created_at >= ? AND e.created_at < ?`,
			a.OrgID, a.Env, dayStart, dayEnd).Scan(&ev)
		series = append(series, map[string]any{"date": day.Format("Jan 2"), "count": n + ev})
	}

	out := map[string]any{
		"redemptions_30d":  redemptions30 + events30,
		"active_campaigns": activeCampaigns,
		"settled_total":    settledTotal,
		"settled_unit":     payoutUnit,
		"series":           series,
		"activity":         s.recentActivity(a, 8),
	}
	if a.Env == "live" {
		s.ensureMonthlyGrant(a.OrgID)
		var bal int64
		_ = s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id = ? AND env = 'live'`, a.OrgID).Scan(&bal)
		out["credits_mcr"] = bal
	}
	// funding status so the console can show "setting up your account"
	var funded int
	_ = s.db.QueryRow(`SELECT funded FROM org_accounts WHERE org_id = ? AND env = ?`, a.OrgID, a.Env).Scan(&funded)
	out["account_funded"] = funded == 1
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
		_ = rows.Scan(&ts, &kind, &code, &msg, &tx, &ecode, &cid)
		out = append(out, map[string]any{
			"ts": ts, "kind": kind, "code": code, "message": msg,
			"tx_hash": tx, "error_code": ecode, "campaign_id": cid,
		})
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
		s.ensureMonthlyGrant(a.OrgID)
		var bal int64
		var gm string
		_ = s.db.QueryRow(`SELECT balance_mcr, grant_month FROM credits WHERE org_id = ? AND env = 'live'`, a.OrgID).
			Scan(&bal, &gm)
		out["balance_mcr"] = bal
		out["grant_month"] = gm
	}
	rows, _ := s.db.Query(`SELECT ts, operation, COALESCE(detail,''), delta_mcr, balance_mcr
	  FROM credit_ledger WHERE org_id = ? AND env = ? ORDER BY id DESC LIMIT 100`, a.OrgID, a.Env)
	ledger := []map[string]any{}
	for rows.Next() {
		var ts, delta, bal int64
		var op, detail string
		_ = rows.Scan(&ts, &op, &detail, &delta, &bal)
		ledger = append(ledger, map[string]any{
			"ts": ts, "operation": op, "detail": detail, "delta_mcr": delta, "balance_mcr": bal,
		})
	}
	rows.Close()
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
		writeProblem(w, 500, err.Error())
		return
	}
	defer rows.Close()
	byOp := []map[string]any{}
	for rows.Next() {
		var op string
		var count, spent int64
		_ = rows.Scan(&op, &count, &spent)
		byOp = append(byOp, map[string]any{"operation": op, "count": count, "spent_mcr": spent})
	}
	// operation counts also exist in test (unmetered) via activity
	var ops30 int64
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM activity WHERE org_id = ? AND env = ? AND ts >= ?`, a.OrgID, a.Env, from).
		Scan(&ops30)
	writeJSON(w, 200, map[string]any{"by_operation": byOp, "operations_30d": ops30})
}

// handleRecharge is the v1 stub: it returns USDC-on-Stellar payment
// instructions. Card (Stripe) checkout and automatic crediting land with the
// billing integration; during the free period the monthly grant covers usage.
func (s *server) handleRecharge(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var in struct {
		AmountUSD float64 `json:"amount_usd"`
		Method    string  `json:"method"`
	}
	_ = readBody(r, &in)
	if in.AmountUSD <= 0 {
		in.AmountUSD = 25
	}
	var pk string
	_ = s.db.QueryRow(`SELECT public_key FROM org_accounts WHERE org_id = ? AND env = 'live'`, a.OrgID).Scan(&pk)
	writeJSON(w, 200, map[string]any{
		"status":  "manual",
		"message": "During the free period your monthly grant covers usage. To recharge, send USDC on Stellar to the address below with the memo — credits are applied after 1 confirmation.",
		"usdc": map[string]any{
			"address": pk, "memo": "SDCLOUD-" + strconv.FormatInt(a.OrgID, 10),
			"amount_usd": in.AmountUSD, "credits": int64(in.AmountUSD * 1000),
		},
	})
}
