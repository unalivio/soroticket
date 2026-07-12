package main

import (
	"encoding/json"
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
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 96 || in.Threshold < 1 || in.Threshold > 1_000_000 {
		writeProblem(w, 400, "name and a threshold ≥ 1 are required")
		return
	}
	if in.RewardDiscountValue < 0 {
		writeProblem(w, 400, "reward_discount_value cannot be negative")
		return
	}
	if in.RewardDiscountType == "" {
		in.RewardDiscountType = "free_item"
	}
	in.RewardDiscountType = strings.TrimSpace(in.RewardDiscountType)
	if len(in.RewardDiscountType) > 32 {
		writeProblem(w, 400, "reward_discount_type exceeds 32 UTF-8 bytes")
		return
	}
	if in.ValidYears <= 0 {
		in.ValidYears = 2
	}
	if in.ValidYears > 10 {
		writeProblem(w, 400, "valid_years cannot exceed 10")
		return
	}

	reservation, ok := s.reserveCharge(w, a, "create_campaign", in.Name+" (loyalty)", mcrCreateCampaign+mcrRegisterShared)
	if !ok {
		return
	}
	usedCharge := int64(0)
	defer func() { reservation.CommitUsed(usedCharge) }()

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
	campaignTx := cl.LastTransactionHash()
	usedCharge += mcrCreateCampaign

	// Unique/shared namespaces are per campaign, so a fixed earn code is both
	// simpler and collision-free.
	earnCode := "PUNCH"
	if err := cl.RegisterShared(r.Context(), chainID, earnCode, nil, nil, big.NewInt(0)); err != nil {
		writeErr(w, err)
		return
	}
	sharedTx := cl.LastTransactionHash()
	usedCharge += mcrRegisterShared

	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO campaigns
	  (org_id, env, chain_id, kind, name, discount_type, discount_value, total_supply, valid_until, tx_hash, created_at)
	  VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		a.OrgID, a.Env, chainID, "loyalty", in.Name, in.RewardDiscountType, in.RewardDiscountValue, 10_000, validUntil, campaignTx, now)
	if err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}
	campID, err := res.LastInsertId()
	if err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}

	res2, err := tx.Exec(`INSERT INTO loyalty_programs
	  (org_id, env, name, threshold, campaign_id, earn_code, reward_discount_type, reward_discount_value, created_at)
	  VALUES (?,?,?,?,?,?,?,?,?)`,
		a.OrgID, a.Env, in.Name, in.Threshold, campID, earnCode, in.RewardDiscountType, in.RewardDiscountValue, now)
	if err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}
	progID, err := res2.LastInsertId()
	if err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}
	if _, err = tx.Exec(`INSERT INTO shared_codes (campaign_id, code, payout_rate, tx_hash, created_at) VALUES (?,?, '0', ?, ?)`,
		campID, earnCode, sharedTx, now); err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}
	if err = tx.Commit(); err != nil {
		writeProblem(w, 500, "program exists on-chain but local indexing failed")
		return
	}
	s.logActivity(a, "program", "", "Loyalty program created · "+in.Name, sharedTx, &campID, 0)
	writeJSON(w, 201, map[string]any{"program": map[string]any{
		"id": progID, "name": in.Name, "threshold": in.Threshold, "campaign_id": campID, "earn_code": earnCode,
		"campaign_tx_hash": campaignTx, "shared_tx_hash": sharedTx,
	}})
}

func (s *server) handleListPrograms(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	// collect first, then enrich: with a single SQLite connection, issuing
	// queries while iterating rows deadlocks (rows holds the only conn)
	type progRow struct {
		id, threshold, campID, rval, created int64
		name, earn, rtype                    string
	}
	rows, err := s.db.Query(`SELECT id, name, threshold, campaign_id, earn_code, reward_discount_type,
	  reward_discount_value, created_at FROM loyalty_programs WHERE org_id = ? AND env = ? ORDER BY id DESC`,
		a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "list loyalty programs")
		return
	}
	progs := []progRow{}
	for rows.Next() {
		var p progRow
		if err := rows.Scan(&p.id, &p.name, &p.threshold, &p.campID, &p.earn, &p.rtype, &p.rval, &p.created); err != nil {
			rows.Close()
			writeInternal(w, err, "decode loyalty program")
			return
		}
		progs = append(progs, p)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeInternal(w, rowsErr, "read loyalty programs")
		return
	}
	out := []map[string]any{}
	for _, p := range progs {
		var punches, customers, rewards, redeemed int64
		if err := s.db.QueryRow(`SELECT COALESCE(SUM(count),0), COUNT(DISTINCT customer_ref) FROM punches WHERE program_id = ?`, p.id).
			Scan(&punches, &customers); err != nil {
			writeInternal(w, err, "summarize loyalty program punches")
			return
		}
		if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(redeemed),0) FROM rewards WHERE program_id = ?`, p.id).
			Scan(&rewards, &redeemed); err != nil {
			writeInternal(w, err, "summarize loyalty program rewards")
			return
		}
		out = append(out, map[string]any{
			"id": p.id, "name": p.name, "threshold": p.threshold, "campaign_id": p.campID, "earn_code": p.earn,
			"reward_discount_type": p.rtype, "reward_discount_value": p.rval, "created_at": p.created,
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
	if threshold <= 0 {
		writeProblem(w, 500, "loyalty program has an invalid threshold")
		return
	}
	// customers with punch totals + reward counts (collect first — see
	// handleListPrograms for the single-connection deadlock this avoids)
	type custRow struct {
		ref           string
		punches, last int64
	}
	rows, err := s.db.Query(`SELECT customer_ref, SUM(count) as punches, MAX(created_at)
	  FROM punches WHERE program_id = ? GROUP BY customer_ref ORDER BY punches DESC LIMIT 500`, id)
	if err != nil {
		writeInternal(w, err, "load loyalty customers")
		return
	}
	custRows := []custRow{}
	for rows.Next() {
		var cr custRow
		if err := rows.Scan(&cr.ref, &cr.punches, &cr.last); err != nil {
			rows.Close()
			writeInternal(w, err, "decode loyalty customer")
			return
		}
		custRows = append(custRows, cr)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeInternal(w, rowsErr, "read loyalty customers")
		return
	}
	customers := []map[string]any{}
	for _, cr := range custRows {
		var rw int64
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM rewards WHERE program_id = ? AND customer_ref = ?`, id, cr.ref).Scan(&rw); err != nil {
			writeInternal(w, err, "count loyalty customer rewards")
			return
		}
		customers = append(customers, map[string]any{
			"customer_ref": cr.ref, "punches": cr.punches, "progress": cr.punches % threshold,
			"rewards_earned": rw, "last_punch_at": cr.last,
		})
	}
	rewards := []map[string]any{}
	rrows, err := s.db.Query(`SELECT customer_ref, code, issued_at, redeemed FROM rewards WHERE program_id = ? ORDER BY id DESC LIMIT 200`, id)
	if err != nil {
		writeInternal(w, err, "load loyalty rewards")
		return
	}
	for rrows.Next() {
		var ref, code string
		var issued, redeemed int64
		if err := rrows.Scan(&ref, &code, &issued, &redeemed); err != nil {
			rrows.Close()
			writeInternal(w, err, "decode loyalty reward")
			return
		}
		rewards = append(rewards, map[string]any{"customer_ref": ref, "code": code, "issued_at": issued, "redeemed": redeemed == 1})
	}
	rrowsErr := rrows.Err()
	rrows.Close()
	if rrowsErr != nil {
		writeInternal(w, rrowsErr, "read loyalty rewards")
		return
	}
	var punches, custCount, rwCount, rwRedeemed int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(count),0), COUNT(DISTINCT customer_ref) FROM punches WHERE program_id = ?`, id).
		Scan(&punches, &custCount); err != nil {
		writeInternal(w, err, "summarize loyalty punches")
		return
	}
	if err := s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(redeemed),0) FROM rewards WHERE program_id = ?`, id).
		Scan(&rwCount, &rwRedeemed); err != nil {
		writeInternal(w, err, "summarize loyalty rewards")
		return
	}
	var pendingEvents int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e JOIN shared_codes sc ON sc.id = e.shared_code_id
	  WHERE sc.campaign_id = ? AND e.committed_period IS NULL`, campID).Scan(&pendingEvents); err != nil {
		writeInternal(w, err, "summarize pending loyalty receipts")
		return
	}
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
	var threshold, campID, sharedID, validUntil int64
	var archived int
	var chainID uint64
	err := s.db.QueryRow(`SELECT lp.name, lp.threshold, lp.campaign_id, lp.earn_code, c.chain_id, sc.id, c.valid_until, c.archived
	  FROM loyalty_programs lp JOIN campaigns c ON c.id = lp.campaign_id
	  JOIN shared_codes sc ON sc.campaign_id = c.id AND sc.code = lp.earn_code
	  WHERE lp.id = ? AND lp.org_id = ? AND lp.env = ?`, id, a.OrgID, a.Env).
		Scan(&name, &threshold, &campID, &earn, &chainID, &sharedID, &validUntil, &archived)
	if err != nil {
		writeProblem(w, 404, "program not found")
		return
	}
	if threshold <= 0 {
		writeProblem(w, 500, "loyalty program has an invalid threshold")
		return
	}
	if archived == 1 {
		writeProblem(w, http.StatusConflict, "loyalty campaign is archived")
		return
	}
	if time.Now().Unix() > validUntil {
		writeProblem(w, http.StatusConflict, "loyalty campaign has expired")
		return
	}
	var in struct {
		CustomerRef string `json:"customer_ref"`
		EventRef    string `json:"event_ref"`
		Count       *int64 `json:"count"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(in.CustomerRef) == "" {
		writeProblem(w, 400, "customer_ref is required")
		return
	}
	if len(in.CustomerRef) > maxReferenceLen || len(in.EventRef) > maxReferenceLen {
		writeProblem(w, 400, "customer_ref and event_ref must not exceed 512 bytes")
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
	if count > 1000 {
		writeProblem(w, 400, "count too large")
		return
	}
	ref := s.opaqueRef(fmt.Sprintf("org:%d|env:%s|loyalty:%d|customer", a.OrgID, a.Env, id), strings.TrimSpace(in.CustomerRef))
	legacyRef := legacyCustomerReference(id, strings.TrimSpace(in.CustomerRef))
	eventRef := s.opaqueRef(fmt.Sprintf("org:%d|env:%s|loyalty:%d|event", a.OrgID, a.Env, id), strings.TrimSpace(in.EventRef))
	mu := s.lockFor(a.OrgID, fmt.Sprintf("loyalty/%s/%d", a.Env, id))
	mu.Lock()
	defer mu.Unlock()
	if err := s.migrateLegacyLoyaltyCustomer(id, sharedID, legacyRef, ref); err != nil {
		writeInternal(w, err, "migrate legacy loyalty customer")
		return
	}
	if eventRef != "" {
		var exists int
		if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM operation_dedup
		  WHERE org_id=? AND env=? AND scope=? AND reference=?)`,
			a.OrgID, a.Env, fmt.Sprintf("loyalty:%d", id), eventRef).Scan(&exists); err != nil {
			writeInternal(w, err, "check loyalty event reference")
			return
		}
		if exists == 1 {
			writeProblem(w, http.StatusConflict, "event_ref was already recorded for this loyalty program")
			return
		}
	}

	now := time.Now().Unix()
	var before int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(count),0) FROM punches WHERE program_id = ? AND customer_ref = ?`, id, ref).Scan(&before); err != nil {
		writeProblem(w, 500, "could not read loyalty balance")
		return
	}
	if before > int64(^uint64(0)>>1)-count {
		writeProblem(w, 400, "loyalty punch total is too large")
		return
	}
	after := before + count
	var existingRewards int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM rewards WHERE program_id = ? AND customer_ref = ?`, id, ref).Scan(&existingRewards); err != nil {
		writeProblem(w, 500, "could not read loyalty rewards")
		return
	}
	newRewards := after/threshold - existingRewards
	if newRewards < 0 {
		newRewards = 0
	}
	if newRewards > maxBatch {
		writeProblem(w, 400, "this punch would issue more than 100 rewards; split it into smaller requests")
		return
	}
	receipt, err := s.signReceipt(a.OrgID, a.Env, chainID, earn, count, ref, eventRef, now)
	if err != nil {
		writeProblem(w, 500, "could not sign loyalty receipt")
		return
	}
	totalCharge := count*mcrPunch + newRewards*mcrIssuePerCode
	reservation, ok := s.reserveCharge(w, a, "punch", name, totalCharge)
	if !ok {
		return
	}
	usedCharge := int64(0)
	defer func() { reservation.CommitUsed(usedCharge) }()

	issued := []string{}
	issuedIDs := []uint64{}
	issueTxHash := ""
	if newRewards > 0 {
		cl, release, err := s.clientFor(a.OrgID, a.Env)
		if err != nil {
			writeErrFunding(w, err)
			return
		}
		defer release()
		codes := make([]string, 0, newRewards)
		for i := int64(0); i < newRewards; i++ {
			generated, err := genCode("RW", 12)
			if err != nil {
				writeProblem(w, 500, "secure randomness unavailable")
				return
			}
			codes = append(codes, generated)
		}
		issuedIDs, err = cl.IssueUnique(r.Context(), chainID, codes)
		if err != nil {
			writeErr(w, err)
			return
		}
		issueTxHash = cl.LastTransactionHash()
		if len(issuedIDs) != len(codes) {
			usedCharge = int64(len(codes)) * mcrIssuePerCode
			writeProblem(w, 500, "reward vouchers were issued on-chain but token ids were not decoded")
			return
		}
		issued = codes
		usedCharge = int64(len(codes)) * mcrIssuePerCode
	}

	tx, err := s.db.Begin()
	if err != nil {
		writeProblem(w, 500, "could not persist loyalty punch")
		return
	}
	defer tx.Rollback()
	if eventRef != "" {
		dedup, err := tx.Exec(`INSERT OR IGNORE INTO operation_dedup
		  (org_id,env,scope,reference,created_at) VALUES (?,?,?,?,?)`,
			a.OrgID, a.Env, fmt.Sprintf("loyalty:%d", id), eventRef, now)
		if err != nil {
			writeProblem(w, 500, "could not reserve loyalty event reference")
			return
		}
		inserted, err := dedup.RowsAffected()
		if err != nil || inserted != 1 {
			writeProblem(w, 500, "loyalty event reference changed during processing; reconcile before retrying")
			return
		}
	}
	if _, err = tx.Exec(`INSERT INTO punches (program_id, customer_ref, count, created_at) VALUES (?,?,?,?)`,
		id, ref, count, now); err != nil {
		writeProblem(w, 500, "could not persist loyalty punch")
		return
	}
	res, err := tx.Exec(`INSERT INTO shared_events (shared_code_id, count, customer_ref, created_at) VALUES (?,?,?,?)`,
		sharedID, count, ref, now)
	if err != nil {
		writeProblem(w, 500, "could not persist loyalty receipt")
		return
	}
	eventID, _ := res.LastInsertId()
	if _, err = tx.Exec(`INSERT INTO event_receipts (event_id, payload, leaf_hash, signature, signer) VALUES (?,?,?,?,?)`,
		eventID, receipt.Payload, hexRoot(receipt.Leaf), receipt.Signature, receipt.Signer); err != nil {
		writeProblem(w, 500, "could not persist loyalty receipt")
		return
	}
	for i, cd := range issued {
		if _, err = tx.Exec(`INSERT INTO codes (campaign_id, code, token_id, tx_hash, created_at) VALUES (?,?,?,?,?)`, campID, cd, issuedIDs[i], issueTxHash, now); err != nil {
			writeProblem(w, 500, "reward issued on-chain but local indexing failed")
			return
		}
		if _, err = tx.Exec(`INSERT INTO rewards (program_id, customer_ref, code, issued_at) VALUES (?,?,?,?)`,
			id, ref, cd, now); err != nil {
			writeProblem(w, 500, "reward issued on-chain but local indexing failed")
			return
		}
	}
	if len(issued) > 0 {
		if _, err = tx.Exec(`UPDATE campaigns SET minted = minted + ? WHERE id = ?`, len(issued), campID); err != nil {
			writeProblem(w, 500, "reward issued on-chain but local indexing failed")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		writeProblem(w, 500, "could not commit loyalty punch")
		return
	}
	usedCharge = totalCharge
	if len(issued) > 0 {
		s.logActivity(a, "reward", issued[0], fmt.Sprintf("%d reward voucher(s) issued · %s", len(issued), name), issueTxHash, &campID, 0)
	}
	writeJSON(w, 201, map[string]any{
		"customer_ref": ref, "punches": after, "progress": after % threshold,
		"threshold": threshold, "rewards_issued": issued, "reward_tx_hash": issueTxHash,
		"receipt": map[string]any{"payload": json.RawMessage(receipt.Payload), "leaf_hash": hexRoot(receipt.Leaf), "signature": receipt.Signature, "signer": receipt.Signer},
	})
}

func (s *server) migrateLegacyLoyaltyCustomer(programID, sharedID int64, legacyRef, currentRef string) error {
	if legacyRef == "" || currentRef == "" || legacyRef == currentRef {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE punches SET customer_ref=? WHERE program_id=? AND customer_ref=?`,
		currentRef, programID, legacyRef); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE rewards SET customer_ref=? WHERE program_id=? AND customer_ref=?`,
		currentRef, programID, legacyRef); err != nil {
		return err
	}
	// Only unsigned legacy rows may be rewritten. A signed payload's commitment
	// is immutable and must continue matching its stored leaf hash.
	if _, err = tx.Exec(`UPDATE shared_events SET customer_ref=?
	  WHERE shared_code_id=? AND customer_ref=? AND NOT EXISTS
	  (SELECT 1 FROM event_receipts er WHERE er.event_id=shared_events.id)`,
		currentRef, sharedID, legacyRef); err != nil {
		return err
	}
	return tx.Commit()
}
