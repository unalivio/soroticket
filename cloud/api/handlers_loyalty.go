package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Loyalty programs compose the two existing profiles on ONE dual-profile
// campaign (unique and shared codes live in separate namespaces — ADR-011):
//   earn side  → a count-only shared code anchors punch totals on-chain
//   reward side → crossing the threshold auto-issues a unique Burn voucher
// Per-customer balances stay off-chain (customers aren't Stellar addresses);
// period commits make the totals publicly auditable.

func custHash(programID int64, ref string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("sorodeal-loyalty|%d|%s", programID, ref)))
	return hex.EncodeToString(h[:8])
}

func (s *server) handleCreateProgram(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var in struct {
		Name                string `json:"name"`
		Threshold           int64  `json:"threshold"`
		RewardDiscountType  string `json:"reward_discount_type"`
		RewardDiscountValue int64  `json:"reward_discount_value"`
		ValidYears          int    `json:"valid_years"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" || in.Threshold < 1 {
		writeProblem(w, 400, "name and a threshold ≥ 1 are required")
		return
	}
	if in.RewardDiscountType == "" {
		in.RewardDiscountType = "free_item"
	}
	if in.ValidYears <= 0 {
		in.ValidYears = 2
	}

	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()

	validUntil := time.Now().AddDate(in.ValidYears, 0, 0).Unix()
	chainID, err := cl.CreateCampaign(r.Context(), in.Name, in.RewardDiscountType,
		uint64(in.RewardDiscountValue), 10_000, uint64(validUntil))
	if err != nil {
		writeErr(w, err)
		return
	}
	now := time.Now().Unix()
	res, _ := s.db.Exec(`INSERT INTO campaigns
	  (org_id, env, chain_id, kind, name, discount_type, discount_value, total_supply, valid_until, created_at)
	  VALUES (?,?,?,?,?,?,?,?,?,?)`,
		a.OrgID, a.Env, chainID, "loyalty", in.Name, in.RewardDiscountType, in.RewardDiscountValue, 10_000, validUntil, now)
	campID, _ := res.LastInsertId()

	res2, _ := s.db.Exec(`INSERT INTO loyalty_programs
	  (org_id, env, name, threshold, campaign_id, earn_code, reward_discount_type, reward_discount_value, created_at)
	  VALUES (?,?,?,?,?,?,?,?,?)`,
		a.OrgID, a.Env, in.Name, in.Threshold, campID, "", in.RewardDiscountType, in.RewardDiscountValue, now)
	progID, _ := res2.LastInsertId()

	// earn anchor: count-only shared code on the same campaign
	earnCode := fmt.Sprintf("PUNCH-%d", progID)
	if err := cl.RegisterShared(r.Context(), chainID, earnCode, nil, nil, big.NewInt(0)); err != nil {
		writeErr(w, err)
		return
	}
	_, _ = s.db.Exec(`UPDATE loyalty_programs SET earn_code = ? WHERE id = ?`, earnCode, progID)
	_, _ = s.db.Exec(`INSERT INTO shared_codes (campaign_id, code, payout_rate, created_at) VALUES (?,?, '0', ?)`,
		campID, earnCode, now)

	if !s.charge(w, a, "create_campaign", in.Name+" (loyalty)", mcrCreateCampaign+mcrRegisterShared, "") {
		return
	}
	s.logActivity(a, "program", "", "Loyalty program created · "+in.Name, "", &campID, 0)
	writeJSON(w, 201, map[string]any{"program": map[string]any{
		"id": progID, "name": in.Name, "threshold": in.Threshold, "campaign_id": campID, "earn_code": earnCode,
	}})
}

func (s *server) handleListPrograms(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	rows, err := s.db.Query(`SELECT id, name, threshold, campaign_id, earn_code, reward_discount_type,
	  reward_discount_value, created_at FROM loyalty_programs WHERE org_id = ? AND env = ? ORDER BY id DESC`,
		a.OrgID, a.Env)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, threshold, campID, rval, created int64
		var name, earn, rtype string
		_ = rows.Scan(&id, &name, &threshold, &campID, &earn, &rtype, &rval, &created)
		var punches, customers, rewards, redeemed int64
		_ = s.db.QueryRow(`SELECT COALESCE(SUM(count),0), COUNT(DISTINCT customer_ref) FROM punches WHERE program_id = ?`, id).
			Scan(&punches, &customers)
		_ = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(redeemed),0) FROM rewards WHERE program_id = ?`, id).
			Scan(&rewards, &redeemed)
		out = append(out, map[string]any{
			"id": id, "name": name, "threshold": threshold, "campaign_id": campID, "earn_code": earn,
			"reward_discount_type": rtype, "reward_discount_value": rval, "created_at": created,
			"punches": punches, "customers": customers, "rewards_issued": rewards, "rewards_redeemed": redeemed,
		})
	}
	writeJSON(w, 200, map[string]any{"programs": out})
}

func (s *server) handleGetProgram(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var name, earn, rtype string
	var threshold, campID, rval, created int64
	err := s.db.QueryRow(`SELECT name, threshold, campaign_id, earn_code, reward_discount_type,
	  reward_discount_value, created_at FROM loyalty_programs WHERE id = ? AND org_id = ? AND env = ?`,
		id, a.OrgID, a.Env).Scan(&name, &threshold, &campID, &earn, &rtype, &rval, &created)
	if err != nil {
		writeProblem(w, 404, "program not found")
		return
	}
	// customers with punch totals + reward counts
	rows, _ := s.db.Query(`SELECT customer_ref, SUM(count) as punches, MAX(created_at)
	  FROM punches WHERE program_id = ? GROUP BY customer_ref ORDER BY punches DESC LIMIT 500`, id)
	customers := []map[string]any{}
	for rows.Next() {
		var ref string
		var punches, last int64
		_ = rows.Scan(&ref, &punches, &last)
		var rw int64
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM rewards WHERE program_id = ? AND customer_ref = ?`, id, ref).Scan(&rw)
		customers = append(customers, map[string]any{
			"customer_ref": ref, "punches": punches, "progress": punches % threshold,
			"rewards_earned": rw, "last_punch_at": last,
		})
	}
	rows.Close()
	rewards := []map[string]any{}
	rrows, _ := s.db.Query(`SELECT customer_ref, code, issued_at, redeemed FROM rewards WHERE program_id = ? ORDER BY id DESC LIMIT 200`, id)
	for rrows.Next() {
		var ref, code string
		var issued, redeemed int64
		_ = rrows.Scan(&ref, &code, &issued, &redeemed)
		rewards = append(rewards, map[string]any{"customer_ref": ref, "code": code, "issued_at": issued, "redeemed": redeemed == 1})
	}
	rrows.Close()
	var punches, custCount, rwCount, rwRedeemed int64
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(count),0), COUNT(DISTINCT customer_ref) FROM punches WHERE program_id = ?`, id).
		Scan(&punches, &custCount)
	_ = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(redeemed),0) FROM rewards WHERE program_id = ?`, id).Scan(&rwCount, &rwRedeemed)
	var pendingEvents int64
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e JOIN shared_codes sc ON sc.id = e.shared_code_id
	  WHERE sc.campaign_id = ? AND e.committed_period IS NULL`, campID).Scan(&pendingEvents)
	writeJSON(w, 200, map[string]any{"program": map[string]any{
		"id": id, "name": name, "threshold": threshold, "campaign_id": campID, "earn_code": earn,
		"reward_discount_type": rtype, "reward_discount_value": rval, "created_at": created,
		"punches": punches, "customers": custCount, "rewards_issued": rwCount, "rewards_redeemed": rwRedeemed,
		"pending_anchor_events": pendingEvents,
	}, "customers": customers, "rewards": rewards})
}

func (s *server) handlePunch(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var name, earn string
	var threshold, campID int64
	err := s.db.QueryRow(`SELECT name, threshold, campaign_id, earn_code FROM loyalty_programs
	  WHERE id = ? AND org_id = ? AND env = ?`, id, a.OrgID, a.Env).Scan(&name, &threshold, &campID, &earn)
	if err != nil {
		writeProblem(w, 404, "program not found")
		return
	}
	var in struct {
		CustomerRef string `json:"customer_ref"`
		Count       int64  `json:"count"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.CustomerRef) == "" {
		writeProblem(w, 400, "customer_ref is required")
		return
	}
	if in.Count <= 0 {
		in.Count = 1
	}
	if in.Count > 1000 {
		writeProblem(w, 400, "count too large")
		return
	}
	ref := custHash(id, strings.TrimSpace(in.CustomerRef))
	if !s.charge(w, a, "punch", name, in.Count*mcrPunch, "") {
		return
	}
	now := time.Now().Unix()
	var before int64
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(count),0) FROM punches WHERE program_id = ? AND customer_ref = ?`, id, ref).Scan(&before)
	_, _ = s.db.Exec(`INSERT INTO punches (program_id, customer_ref, count, created_at) VALUES (?,?,?,?)`,
		id, ref, in.Count, now)
	after := before + in.Count

	// mirror punches into the earn anchor's event stream (per-period commits)
	var sharedID int64
	if err := s.db.QueryRow(`SELECT id FROM shared_codes WHERE campaign_id = ? AND code = ?`, campID, earn).Scan(&sharedID); err == nil {
		_, _ = s.db.Exec(`INSERT INTO shared_events (shared_code_id, count, customer_ref, created_at) VALUES (?,?,?,?)`,
			sharedID, in.Count, ref, now)
	}

	// every threshold crossing earns a voucher
	newRewards := after/threshold - before/threshold
	issued := []string{}
	if newRewards > 0 {
		cl, release, err := s.clientFor(a.OrgID, a.Env)
		if err != nil {
			writeErrFunding(w, err)
			return
		}
		defer release()
		var chainID uint64
		_ = s.db.QueryRow(`SELECT chain_id FROM campaigns WHERE id = ?`, campID).Scan(&chainID)
		codes := make([]string, 0, newRewards)
		for i := int64(0); i < newRewards; i++ {
			codes = append(codes, genCode("RW", 6))
		}
		if _, err := cl.IssueUnique(r.Context(), chainID, codes); err != nil {
			writeErr(w, err)
			return
		}
		for _, cd := range codes {
			_, _ = s.db.Exec(`INSERT INTO codes (campaign_id, code, created_at) VALUES (?,?,?)`, campID, cd, now)
			_, _ = s.db.Exec(`INSERT INTO rewards (program_id, customer_ref, code, issued_at) VALUES (?,?,?,?)`,
				id, ref, cd, now)
			issued = append(issued, cd)
		}
		_, _ = s.db.Exec(`UPDATE campaigns SET minted = minted + ? WHERE id = ?`, len(codes), campID)
		if !s.charge(w, a, "issue_codes", fmt.Sprintf("%d reward vouchers · %s", len(codes), name),
			int64(len(codes))*mcrIssuePerCode, "") {
			return
		}
		s.logActivity(a, "reward", issued[0], "Reward voucher issued · "+name, "", &campID, 0)
	}
	writeJSON(w, 201, map[string]any{
		"customer_ref": ref, "punches": after, "progress": after % threshold,
		"threshold": threshold, "rewards_issued": issued,
	})
}
