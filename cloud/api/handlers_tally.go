package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// isoWeekPeriod encodes the current ISO week as YYYYWW (e.g. 202628) — the
// default commit period for shared codes.
func isoWeekPeriod(t time.Time) uint64 {
	y, w := t.ISOWeek()
	return uint64(y*100 + w)
}

func periodLabel(p uint64) string {
	return fmt.Sprintf("%d-W%02d", p/100, p%100)
}

func (s *server) sharedByCampaignCode(a *authCtx, campaignID int64, code string) (sharedID int64, chainID uint64, attributedTo string, payoutRate string, campName string, err error) {
	var attr sql.NullString
	err = s.db.QueryRow(`SELECT sc.id, c.chain_id, sc.attributed_to, sc.payout_rate, c.name
	  FROM shared_codes sc JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE sc.campaign_id = ? AND sc.code = ? AND c.org_id = ? AND c.env = ?`,
		campaignID, code, a.OrgID, a.Env).Scan(&sharedID, &chainID, &attr, &payoutRate, &campName)
	if attr.Valid {
		attributedTo = attr.String
	}
	return
}

// ── shared-code registration (post-wizard additions) ────────────────

func (s *server) handleRegisterShared(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	c, err := s.campaignByID(a, id)
	if err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}
	var in struct {
		Code         string `json:"code"`
		AttributedTo string `json:"attributed_to"`
		PayoutRate   string `json:"payout_rate"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		writeProblem(w, 400, "code is required")
		return
	}
	var attr, tok *string
	rate := big.NewInt(0)
	if at := strings.TrimSpace(in.AttributedTo); at != "" {
		attr = &at
		if in.PayoutRate != "" && in.PayoutRate != "0" {
			var ok bool
			rate, ok = new(big.Int).SetString(in.PayoutRate, 10)
			if !ok || rate.Sign() < 0 {
				writeProblem(w, 400, "payout_rate must be a non-negative integer")
				return
			}
			t := payoutToken
			tok = &t
		}
	}
	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()
	if err := cl.RegisterShared(r.Context(), c.ChainID, code, attr, tok, rate); err != nil {
		writeErr(w, err)
		return
	}
	_, _ = s.db.Exec(`INSERT INTO shared_codes (campaign_id, code, attributed_to, payout_token, payout_rate, created_at)
	  VALUES (?,?,?,?,?,?)`, id, code, attr, tok, rate.String(), time.Now().Unix())
	if !s.charge(w, a, "register_shared", code, mcrRegisterShared, "") {
		return
	}
	s.logActivity(a, "campaign", code, "Shared code registered · "+code, "", &id, 0)
	writeJSON(w, 201, map[string]any{"ok": true, "code": code})
}

// ── off-chain events (the hot path) ─────────────────────────────────

func (s *server) handleRecordEvents(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	cid, _ := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	code := strings.ToUpper(r.PathValue("code"))
	sharedID, _, _, _, campName, err := s.sharedByCampaignCode(a, cid, code)
	if err != nil {
		writeProblem(w, 404, "shared code not found")
		return
	}
	var in struct {
		Count       int64  `json:"count"`
		CustomerRef string `json:"customer_ref"`
		OrderRef    string `json:"order_ref"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if in.Count <= 0 {
		in.Count = 1
	}
	if in.Count > 10_000 {
		writeProblem(w, 400, "count too large")
		return
	}
	var custRef string
	if in.CustomerRef != "" {
		h := sha256.Sum256([]byte("sorodeal-cust|" + in.CustomerRef))
		custRef = hex.EncodeToString(h[:8]) // opaque, truncated — display only
	}
	if !s.charge(w, a, "shared_event", code, in.Count*mcrSharedEvent, "") {
		return
	}
	_, _ = s.db.Exec(`INSERT INTO shared_events (shared_code_id, count, customer_ref, order_ref, created_at)
	  VALUES (?,?,?,?,?)`, sharedID, in.Count, custRef, strings.TrimSpace(in.OrderRef), time.Now().Unix())
	s.logActivity(a, "event", code, fmt.Sprintf("+%d events recorded · %s", in.Count, campName), "", &cid, 0)
	var pending int64
	_ = s.db.QueryRow(`SELECT COALESCE(SUM(count),0) FROM shared_events WHERE shared_code_id = ? AND committed_period IS NULL`,
		sharedID).Scan(&pending)
	writeJSON(w, 201, map[string]any{"ok": true, "pending_events": pending})
}

// ── commits (anchor a period on-chain) ──────────────────────────────

func (s *server) handleCommitTally(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	cid, _ := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	code := strings.ToUpper(r.PathValue("code"))
	sharedID, chainID, attributedTo, _, _, err := s.sharedByCampaignCode(a, cid, code)
	if err != nil {
		writeProblem(w, 404, "shared code not found")
		return
	}
	var in struct {
		Period uint64 `json:"period"`
	}
	_ = readBody(r, &in) // empty body is fine
	if in.Period == 0 {
		in.Period = isoWeekPeriod(time.Now().UTC())
	}

	// gather uncommitted events
	rows, err := s.db.Query(`SELECT id, count, created_at FROM shared_events
	  WHERE shared_code_id = ? AND committed_period IS NULL ORDER BY id`, sharedID)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	var leaves [][32]byte
	var total int64
	var eventIDs []int64
	for rows.Next() {
		var id, count, ts int64
		_ = rows.Scan(&id, &count, &ts)
		leaves = append(leaves, eventLeaf(id, code, count, ts))
		total += count
		eventIDs = append(eventIDs, id)
	}
	rows.Close()
	if total == 0 {
		writeProblem(w, 400, "no uncommitted events to anchor")
		return
	}
	if total > 4_000_000_000 {
		writeProblem(w, 400, "count overflows u32")
		return
	}

	root := merkleRoot(leaves)
	attribution := map[string]uint32{}
	attributedCount := int64(0)
	if attributedTo != "" {
		// creator codes credit every counted conversion to the registered creator
		attribution[attributedTo] = uint32(total)
		attributedCount = total
	}

	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()
	if err := cl.CommitTally(r.Context(), chainID, code, in.Period, uint32(total), root, attribution); err != nil {
		writeErr(w, err)
		return
	}
	now := time.Now().Unix()
	_, _ = s.db.Exec(`INSERT INTO tallies (shared_code_id, period, count, attributed_count, merkle_root, committed_at)
	  VALUES (?,?,?,?,?,?)`, sharedID, in.Period, total, attributedCount, hexRoot(root), now)
	for _, id := range eventIDs {
		_, _ = s.db.Exec(`UPDATE shared_events SET committed_period = ? WHERE id = ?`, in.Period, id)
	}
	if !s.charge(w, a, "commit_tally", fmt.Sprintf("%s · %s", code, periodLabel(in.Period)), mcrCommitTally, "") {
		return
	}
	s.logActivity(a, "tally", code, fmt.Sprintf("Tally committed %s · %s", periodLabel(in.Period), code), "", &cid, 0)
	writeJSON(w, 201, map[string]any{
		"period": in.Period, "period_label": periodLabel(in.Period),
		"count": total, "merkle_root": hexRoot(root),
	})
}

// ── settlements ─────────────────────────────────────────────────────

// handleListSettlements returns every committed period across the org's shared
// codes, with settle state and payout preview — the Settlements screen.
func (s *server) handleListSettlements(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	q := `SELECT t.id, c.id, c.name, sc.code, COALESCE(sc.attributed_to,''), sc.payout_rate,
	  t.period, t.count, t.attributed_count, t.merkle_root, t.settled, COALESCE(t.settle_tx,''),
	  COALESCE(t.payout_amount,''), t.committed_at
	  FROM tallies t
	  JOIN shared_codes sc ON sc.id = t.shared_code_id
	  JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.org_id = ? AND c.env = ? ORDER BY t.id DESC`
	rows, err := s.db.Query(q, a.OrgID, a.Env)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var tid, cid, period, count, attributed, committedAt int64
		var settled int
		var name, code, attr, rate, root, settleTx, amount string
		_ = rows.Scan(&tid, &cid, &name, &code, &attr, &rate, &period, &count, &attributed,
			&root, &settled, &settleTx, &amount, &committedAt)
		preview := "0"
		if attr != "" {
			r, _ := new(big.Int).SetString(rate, 10)
			if r != nil {
				preview = new(big.Int).Mul(r, big.NewInt(attributed)).String()
			}
		}
		out = append(out, map[string]any{
			"tally_id": tid, "campaign_id": cid, "campaign_name": name, "code": code,
			"attributed_to": attr, "payout_rate": rate, "payout_unit": payoutUnit,
			"period": period, "period_label": periodLabel(uint64(period)),
			"count": count, "attributed_count": attributed, "merkle_root": root,
			"settled": settled == 1, "settle_tx": settleTx,
			"payout_amount": amount, "payout_preview": preview, "committed_at": committedAt,
		})
	}
	writeJSON(w, 200, map[string]any{"settlements": out})
}

func (s *server) handleSettle(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var in struct {
		CampaignID int64  `json:"campaign_id"`
		Code       string `json:"code"`
		Period     uint64 `json:"period"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	sharedID, chainID, _, _, campName, err := s.sharedByCampaignCode(a, in.CampaignID, code)
	if err != nil {
		writeProblem(w, 404, "shared code not found")
		return
	}
	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()
	payouts, err := cl.Settle(r.Context(), chainID, code, in.Period)
	if err != nil {
		writeErr(w, err)
		return
	}
	total := big.NewInt(0)
	outs := []map[string]any{}
	for _, p := range payouts {
		total.Add(total, p.Amount)
		outs = append(outs, map[string]any{"to": p.To, "amount": p.Amount.String()})
	}
	now := time.Now().Unix()
	_, _ = s.db.Exec(`UPDATE tallies SET settled = 1, settled_at = ?, payout_amount = ? WHERE shared_code_id = ? AND period = ?`,
		now, total.String(), sharedID, in.Period)
	if !s.charge(w, a, "settle", fmt.Sprintf("%s · %s", code, periodLabel(in.Period)), mcrSettle, "") {
		return
	}
	s.logActivity(a, "settle", code,
		fmt.Sprintf("Settled %s · %s → %s %s", periodLabel(in.Period), campName, formatUnits(total), payoutUnit), "", &in.CampaignID, 0)
	writeJSON(w, 201, map[string]any{"payouts": outs, "total": total.String(), "unit": payoutUnit})
}

// formatUnits renders token base-units (stroops, 1e7/unit) as a decimal amount.
func formatUnits(base *big.Int) string {
	f := new(big.Float).Quo(new(big.Float).SetInt(base), big.NewFloat(1e7))
	return f.Text('f', 2)
}
