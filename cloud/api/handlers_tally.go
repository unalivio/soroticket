package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// isoWeekPeriod encodes an ISO week as YYYYWW (e.g. 202628). If one week needs
// multiple bounded receipt batches, Cloud appends a two-digit suffix:
// YYYYWW01..YYYYWW99.
func isoWeekPeriod(t time.Time) uint64 {
	y, w := t.ISOWeek()
	return uint64(y*100 + w)
}

func periodLabel(p uint64) string {
	base, batch := p, uint64(0)
	if p >= 10_000_000 {
		base, batch = p/100, p%100
	}
	label := fmt.Sprintf("%d-W%02d", base/100, base%100)
	if batch > 0 {
		label += fmt.Sprintf(".%02d", batch)
	}
	return label
}

func validCloudPeriod(p uint64) bool {
	base, batch := p, uint64(0)
	if p >= 10_000_000 {
		base, batch = p/100, p%100
		if batch == 0 {
			return false
		}
	}
	year, week := base/100, base%100
	if year < 1970 || year > 9999 || week < 1 || week > 53 {
		return false
	}
	_, lastWeek := time.Date(int(year), time.December, 28, 0, 0, 0, 0, time.UTC).ISOWeek()
	return week <= uint64(lastWeek)
}

func (s *server) nextCommitPeriod(sharedID int64, now time.Time) (uint64, error) {
	base := isoWeekPeriod(now)
	var baseExists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tallies WHERE shared_code_id=? AND period=?`,
		sharedID, base).Scan(&baseExists); err != nil {
		return 0, err
	}
	var maxBatch sql.NullInt64
	low, high := base*100+1, base*100+99
	if err := s.db.QueryRow(`SELECT MAX(period % 100) FROM tallies
	  WHERE shared_code_id=? AND period BETWEEN ? AND ?`, sharedID, low, high).Scan(&maxBatch); err != nil {
		return 0, err
	}
	if maxBatch.Valid {
		if maxBatch.Int64 >= 99 {
			return 0, errors.New("this shared code already has 100 tally batches for the current ISO week")
		}
		return base*100 + uint64(maxBatch.Int64+1), nil
	}
	if baseExists == 0 {
		return base, nil
	}
	return base*100 + 1, nil
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
	if c.Archived {
		writeProblem(w, http.StatusConflict, "campaign is archived")
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
	if len(code) > maxCodeLen {
		writeProblem(w, 400, "code exceeds 64 UTF-8 bytes")
		return
	}
	var attr, tok *string
	rate := big.NewInt(0)
	if at := strings.TrimSpace(in.AttributedTo); at != "" {
		if !validStellarAddress(at) {
			writeProblem(w, 400, "attributed_to must be a valid Stellar account or contract address")
			return
		}
		attr = &at
		if in.PayoutRate != "" && in.PayoutRate != "0" {
			var ok bool
			rate, ok = new(big.Int).SetString(in.PayoutRate, 10)
			if !ok || rate.Sign() <= 0 || rate.BitLen() > 127 {
				writeProblem(w, 400, "payout_rate must be a positive i128 integer")
				return
			}
			t := payoutToken
			tok = &t
		}
	} else if in.PayoutRate != "" && in.PayoutRate != "0" {
		writeProblem(w, 400, "payout_rate requires attributed_to")
		return
	}
	reservation, ok := s.reserveCharge(w, a, "register_shared", code, mcrRegisterShared)
	if !ok {
		return
	}
	defer reservation.Refund()
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
	txHash := cl.LastTransactionHash()
	reservation.Commit()
	if _, err := s.db.Exec(`INSERT INTO shared_codes (campaign_id, code, attributed_to, payout_token, payout_rate, tx_hash, created_at)
	  VALUES (?,?,?,?,?,?,?)`, id, code, attr, tok, rate.String(), txHash, time.Now().Unix()); err != nil {
		writeProblem(w, 500, "shared code was registered on-chain but could not be indexed locally")
		return
	}
	s.logActivity(a, "campaign", code, "Shared code registered · "+code, txHash, &id, 0)
	writeJSON(w, 201, map[string]any{"ok": true, "code": code, "tx_hash": txHash})
}

// ── off-chain events (the hot path) ─────────────────────────────────

func (s *server) handleRecordEvents(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	cid, _ := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	code := strings.ToUpper(r.PathValue("code"))
	sharedID, chainID, _, _, campName, err := s.sharedByCampaignCode(a, cid, code)
	if err != nil {
		writeProblem(w, 404, "shared code not found")
		return
	}
	var validUntil int64
	var archived int
	if err := s.db.QueryRow(`SELECT valid_until, archived FROM campaigns WHERE id=? AND org_id=? AND env=?`,
		cid, a.OrgID, a.Env).Scan(&validUntil, &archived); err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}
	if archived == 1 {
		writeProblem(w, http.StatusConflict, "campaign is archived")
		return
	}
	if time.Now().Unix() > validUntil {
		writeProblem(w, http.StatusConflict, "campaign has expired; new events are not accepted")
		return
	}
	var in struct {
		Count       *int64 `json:"count"`
		CustomerRef string `json:"customer_ref"`
		OrderRef    string `json:"order_ref"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	count := int64(1)
	if in.Count != nil {
		count = *in.Count
		if count <= 0 {
			writeProblem(w, 400, "count must be positive when provided")
			return
		}
	}
	if count > 10_000 {
		writeProblem(w, 400, "count too large")
		return
	}
	if len(in.CustomerRef) > maxReferenceLen || len(in.OrderRef) > maxReferenceLen {
		writeProblem(w, 400, "customer_ref and order_ref must not exceed 512 bytes")
		return
	}
	domain := fmt.Sprintf("org:%d|env:%s|campaign:%d|code:%s", a.OrgID, a.Env, chainID, code)
	custRef := s.opaqueRef(domain+"|customer", strings.TrimSpace(in.CustomerRef))
	orderRef := s.opaqueRef(domain+"|order", strings.TrimSpace(in.OrderRef))
	now := time.Now().Unix()
	receipt, err := s.signReceipt(a.OrgID, a.Env, chainID, code, count, custRef, orderRef, now)
	if err != nil {
		writeProblem(w, 500, "could not sign redemption receipt")
		return
	}
	reservation, ok := s.reserveCharge(w, a, "shared_event", code, count*mcrSharedEvent)
	if !ok {
		return
	}
	defer reservation.Refund()
	tx, err := s.db.Begin()
	if err != nil {
		writeProblem(w, 500, "could not record event")
		return
	}
	defer tx.Rollback()
	if orderRef != "" {
		dedup, err := tx.Exec(`INSERT OR IGNORE INTO operation_dedup
		  (org_id,env,scope,reference,created_at) VALUES (?,?,?,?,?)`,
			a.OrgID, a.Env, fmt.Sprintf("shared:%d", sharedID), orderRef, now)
		if err != nil {
			writeProblem(w, 500, "could not reserve order reference")
			return
		}
		inserted, err := dedup.RowsAffected()
		if err != nil {
			writeProblem(w, 500, "could not confirm order reference")
			return
		}
		if inserted == 0 {
			writeProblem(w, http.StatusConflict, "order_ref was already recorded for this shared code")
			return
		}
	}
	res, err := tx.Exec(`INSERT INTO shared_events (shared_code_id, count, customer_ref, order_ref, created_at)
	  VALUES (?,?,?,?,?)`, sharedID, count, custRef, orderRef, now)
	if err != nil {
		writeProblem(w, 500, "could not record event")
		return
	}
	eventID, err := res.LastInsertId()
	if err != nil {
		writeProblem(w, 500, "could not identify event")
		return
	}
	if _, err = tx.Exec(`INSERT INTO event_receipts (event_id, payload, leaf_hash, signature, signer)
	  VALUES (?,?,?,?,?)`, eventID, receipt.Payload, hexRoot(receipt.Leaf), receipt.Signature, receipt.Signer); err != nil {
		writeProblem(w, 500, "could not store signed receipt")
		return
	}
	if err = tx.Commit(); err != nil {
		writeProblem(w, 500, "could not commit event")
		return
	}
	reservation.Commit()
	s.logActivity(a, "event", code, fmt.Sprintf("+%d events recorded · %s", count, campName), "", &cid, 0)
	response := map[string]any{
		"ok": true,
		"receipt": map[string]any{
			"payload": json.RawMessage(receipt.Payload), "leaf_hash": hexRoot(receipt.Leaf),
			"signature": receipt.Signature, "signer": receipt.Signer,
		},
	}
	var pending int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(count),0) FROM shared_events WHERE shared_code_id = ? AND committed_period IS NULL`,
		sharedID).Scan(&pending); err != nil {
		log.Printf("summarize pending events campaign=%d code=%s: %v", cid, code, err)
	} else {
		response["pending_events"] = pending
	}
	writeJSON(w, 201, response)
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
	if err := readOptionalBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if in.Period == 0 {
		in.Period, err = s.nextCommitPeriod(sharedID, time.Now().UTC())
		if err != nil {
			writeProblem(w, http.StatusConflict, err.Error())
			return
		}
	}
	if !validCloudPeriod(in.Period) {
		writeProblem(w, 400, "period must be YYYYWW or YYYYWW01..YYYYWW99 for a valid ISO week")
		return
	}
	var unsignedReceipts int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM shared_events e
	  LEFT JOIN event_receipts er ON er.event_id=e.id
	  WHERE e.shared_code_id=? AND e.committed_period IS NULL AND er.event_id IS NULL`,
		sharedID).Scan(&unsignedReceipts); err != nil {
		writeInternal(w, err, "check signed tally receipts")
		return
	}
	if unsignedReceipts > 0 {
		writeProblem(w, http.StatusConflict,
			"legacy unsigned events cannot be anchored as signed receipts; export and reconcile them explicitly")
		return
	}

	// Acquire the account lock before selecting events. Otherwise concurrent
	// commits for different periods can anchor the same uncommitted rows twice.
	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()

	// gather uncommitted events
	rows, err := s.db.Query(`SELECT e.id, e.count, er.leaf_hash FROM shared_events e
	  JOIN event_receipts er ON er.event_id = e.id
	  WHERE e.shared_code_id = ? AND e.committed_period IS NULL ORDER BY e.id
	  LIMIT ?`, sharedID, maxReceiptsPerTally)
	if err != nil {
		writeInternal(w, err, "load uncommitted events")
		return
	}
	var leaves [][32]byte
	var total int64
	var eventIDs []int64
	for rows.Next() {
		var id, count int64
		var leafHex string
		if err := rows.Scan(&id, &count, &leafHex); err != nil {
			rows.Close()
			writeProblem(w, 500, "could not decode signed event")
			return
		}
		leaf, err := decodeHash(leafHex)
		if err != nil {
			rows.Close()
			writeProblem(w, 500, "signed event has invalid leaf hash")
			return
		}
		leaves = append(leaves, leaf)
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

	reservation, ok := s.reserveCharge(w, a, "commit_tally", fmt.Sprintf("%s · %s", code, periodLabel(in.Period)), mcrCommitTally)
	if !ok {
		return
	}
	defer reservation.Refund()
	if err := cl.CommitTally(r.Context(), chainID, code, in.Period, uint32(total), root, attribution); err != nil {
		writeErr(w, err)
		return
	}
	txHash := cl.LastTransactionHash()
	reservation.Commit()
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		writeProblem(w, 500, "tally committed on-chain but local indexing failed")
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO tallies (shared_code_id, period, count, attributed_count, merkle_root, tx_hash, committed_at)
	  VALUES (?,?,?,?,?,?,?)`, sharedID, in.Period, total, attributedCount, hexRoot(root), txHash, now); err != nil {
		writeProblem(w, 500, "tally committed on-chain but local indexing failed")
		return
	}
	lastEventID := eventIDs[len(eventIDs)-1]
	result, err := tx.Exec(`UPDATE shared_events SET committed_period = ?
	  WHERE shared_code_id = ? AND committed_period IS NULL AND id <= ?`, in.Period, sharedID, lastEventID)
	if err != nil {
		writeProblem(w, 500, "tally committed on-chain but event indexing failed")
		return
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != int64(len(eventIDs)) {
		writeProblem(w, 500, "tally committed on-chain but the receipt batch changed during indexing")
		return
	}
	if err = tx.Commit(); err != nil {
		writeProblem(w, 500, "tally committed on-chain but local indexing failed")
		return
	}
	s.logActivity(a, "tally", code, fmt.Sprintf("Tally committed %s · %s", periodLabel(in.Period), code), txHash, &cid, 0)
	response := map[string]any{
		"period": in.Period, "period_label": periodLabel(in.Period),
		"count": total, "receipt_count": len(eventIDs),
		"merkle_root": hexRoot(root), "tx_hash": txHash,
	}
	var remainingReceipts int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM shared_events
	  WHERE shared_code_id = ? AND committed_period IS NULL`, sharedID).Scan(&remainingReceipts); err != nil {
		log.Printf("summarize remaining tally receipts shared=%d: %v", sharedID, err)
	} else {
		response["remaining_receipts"] = remainingReceipts
	}
	writeJSON(w, 201, response)
}

// ── settlements ─────────────────────────────────────────────────────

// handleListSettlements returns every committed period across the org's shared
// codes, with settle state and payout preview — the Settlements screen.
func (s *server) handleListSettlements(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	q := `SELECT t.id, c.id, c.name, sc.code, COALESCE(sc.attributed_to,''), sc.payout_rate,
	  t.period, t.count, t.attributed_count, t.merkle_root, COALESCE(t.tx_hash,''), t.settled, COALESCE(t.settle_tx,''),
	  COALESCE(t.payout_amount,''), t.committed_at
	  FROM tallies t
	  JOIN shared_codes sc ON sc.id = t.shared_code_id
	  JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.org_id = ? AND c.env = ? ORDER BY t.id DESC`
	rows, err := s.db.Query(q, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "list settlements")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var tid, cid, period, count, attributed, committedAt int64
		var settled int
		var name, code, attr, rate, root, commitTx, settleTx, amount string
		if err := rows.Scan(&tid, &cid, &name, &code, &attr, &rate, &period, &count, &attributed,
			&root, &commitTx, &settled, &settleTx, &amount, &committedAt); err != nil {
			writeInternal(w, err, "decode settlement")
			return
		}
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
			"count": count, "attributed_count": attributed, "merkle_root": root, "commit_tx": commitTx,
			"settled": settled == 1, "settle_tx": settleTx,
			"payout_amount": amount, "payout_preview": preview, "committed_at": committedAt,
		})
	}
	if err := rows.Err(); err != nil {
		writeInternal(w, err, "read settlements")
		return
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
	reservation, ok := s.reserveCharge(w, a, "settle", fmt.Sprintf("%s · %s", code, periodLabel(in.Period)), mcrSettle)
	if !ok {
		return
	}
	defer reservation.Refund()
	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()
	shared, err := cl.GetShared(r.Context(), chainID, code)
	if err != nil {
		writeErr(w, err)
		return
	}
	if shared.PayoutToken == nil {
		writeProblem(w, http.StatusConflict, "settlement is not configured for this code")
		return
	}
	// Cloud currently targets the immutable v0.1 deployment. That contract
	// requires the owner signature supplied by this custodial client and calls
	// token.transfer directly; it does not consume an allowance. Creating one
	// here would leave unnecessary spend authority behind. A future v0.2 Cloud
	// deployment must be explicitly version-gated and approve only the exact
	// period amount immediately before its allowance-based settlement.
	payouts, err := cl.Settle(r.Context(), chainID, code, in.Period)
	if err != nil {
		writeErr(w, err)
		return
	}
	txHash := cl.LastTransactionHash()
	reservation.Commit()
	total := big.NewInt(0)
	outs := []map[string]any{}
	for _, p := range payouts {
		total.Add(total, p.Amount)
		outs = append(outs, map[string]any{"to": p.To, "amount": p.Amount.String()})
	}
	now := time.Now().Unix()
	_, _ = s.db.Exec(`UPDATE tallies SET settled = 1, settle_tx = ?, settled_at = ?, payout_amount = ? WHERE shared_code_id = ? AND period = ?`,
		txHash, now, total.String(), sharedID, in.Period)
	s.logActivity(a, "settle", code,
		fmt.Sprintf("Settled %s · %s → %s %s", periodLabel(in.Period), campName, formatUnits(total), payoutUnit), txHash, &in.CampaignID, 0)
	writeJSON(w, 201, map[string]any{"payouts": outs, "total": total.String(), "unit": payoutUnit, "tx_hash": txHash})
}

// formatUnits renders token base-units (stroops, 1e7/unit) as a decimal amount.
func formatUnits(base *big.Int) string {
	if base == nil || base.Sign() == 0 {
		return "0"
	}
	n := new(big.Int).Set(base)
	negative := n.Sign() < 0
	n.Abs(n)
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(n, big.NewInt(10_000_000), remainder)
	result := whole.String()
	if remainder.Sign() != 0 {
		fraction := remainder.String()
		fraction = strings.Repeat("0", 7-len(fraction)) + fraction
		fraction = strings.TrimRight(fraction, "0")
		result += "." + fraction
	}
	if negative {
		result = "-" + result
	}
	return result
}
