package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	maxBatch        = 100 // contract MAX_BATCH
	maxCodeLen      = 64
	maxReferenceLen = 512
)

var kindLabels = map[string]bool{"coupon": true, "creator": true, "gift": true, "voucher": true, "ticket": true}

type campaignOut struct {
	ID            int64  `json:"id"`
	ChainID       uint64 `json:"chain_id"`
	ContractID    string `json:"contract_id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	DiscountType  string `json:"discount_type"`
	DiscountValue int64  `json:"discount_value"`
	TotalSupply   int64  `json:"total_supply"`
	ValidUntil    int64  `json:"valid_until"`
	Minted        int64  `json:"minted"`
	Burned        int64  `json:"burned"`
	Archived      bool   `json:"archived"`
	TxHash        string `json:"tx_hash"`
	CreatedAt     int64  `json:"created_at"`
	SharedCode    string `json:"shared_code,omitempty"`
	AttributedTo  string `json:"attributed_to,omitempty"`
	PayoutRate    string `json:"payout_rate,omitempty"`
	EventsTotal   int64  `json:"events_total,omitempty"`
	PendingEvents int64  `json:"pending_events,omitempty"`
	LegacyEvents  int64  `json:"legacy_unsigned_events,omitempty"`
}

func (s *server) handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var in struct {
		Kind          string `json:"kind"`
		Name          string `json:"name"`
		DiscountType  string `json:"discount_type"`
		DiscountValue int64  `json:"discount_value"`
		TotalSupply   int64  `json:"total_supply"`
		ValidUntil    int64  `json:"valid_until"`
		Shared        *struct {
			Code         string `json:"code"`
			AttributedTo string `json:"attributed_to"`
			PayoutRate   string `json:"payout_rate"` // token base-units per conversion
		} `json:"shared"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	if !kindLabels[in.Kind] {
		writeProblem(w, 400, "kind must be one of coupon|creator|gift|voucher|ticket; create loyalty through /v1/loyalty/programs")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.DiscountType = strings.TrimSpace(in.DiscountType)
	if in.Name == "" || len(in.Name) > 96 || in.DiscountType == "" || len(in.DiscountType) > 32 {
		writeProblem(w, 400, "name and discount_type are required")
		return
	}
	if in.DiscountValue < 0 {
		writeProblem(w, 400, "discount_value cannot be negative")
		return
	}
	if in.ValidUntil == 0 {
		in.ValidUntil = time.Now().AddDate(1, 0, 0).Unix()
	}
	if in.ValidUntil <= time.Now().Unix() {
		writeProblem(w, 400, "valid_until must be in the future")
		return
	}
	isShared := in.Kind == "coupon" || in.Kind == "creator" || in.Kind == "gift"
	if in.TotalSupply == 0 {
		if !isShared {
			writeProblem(w, 400, "total_supply is required for voucher/ticket campaigns")
			return
		}
		in.TotalSupply = 10_000 // shared campaigns do not mint unique codes up-front
	}
	if in.TotalSupply < 0 || in.TotalSupply > int64(^uint32(0)) {
		writeProblem(w, 400, "total_supply must fit an unsigned 32-bit integer")
		return
	}
	if isShared && (in.Shared == nil || strings.TrimSpace(in.Shared.Code) == "") {
		writeProblem(w, 400, "shared.code is required for coupon/creator/gift campaigns")
		return
	}

	var sharedCode string
	var sharedAttr, sharedToken *string
	sharedRate := big.NewInt(0)
	if isShared {
		sharedCode = strings.ToUpper(strings.TrimSpace(in.Shared.Code))
		if len(sharedCode) > maxCodeLen {
			writeProblem(w, 400, "shared.code exceeds 64 UTF-8 bytes")
			return
		}
		// creator binds attribution to the promoter (required); gift — the
		// delivery/usage-proof profile — MAY attribute a delivery point (e.g.
		// the venue that received the gifted product) and MAY pay it per
		// verified event. Payout always requires an attribution target.
		attributionRequired := in.Kind == "creator"
		if attributionRequired || in.Kind == "gift" {
			at := strings.TrimSpace(in.Shared.AttributedTo)
			if at == "" && !attributionRequired {
				if in.Shared.PayoutRate != "" && in.Shared.PayoutRate != "0" {
					writeProblem(w, 400, "payout_rate requires attributed_to")
					return
				}
			} else {
				if !validStellarAddress(at) {
					writeProblem(w, 400, "attributed_to must be a valid Stellar account or contract address")
					return
				}
				sharedAttr = &at
				if in.Shared.PayoutRate != "" && in.Shared.PayoutRate != "0" {
					var ok bool
					sharedRate, ok = new(big.Int).SetString(in.Shared.PayoutRate, 10)
					if !ok || sharedRate.Sign() <= 0 || sharedRate.BitLen() > 127 {
						writeProblem(w, 400, "payout_rate must be a positive i128 integer (token base-units)")
						return
					}
					t := payoutToken
					sharedToken = &t
				}
			}
		}
	}

	totalCharge := int64(mcrCreateCampaign)
	if isShared {
		totalCharge += mcrRegisterShared
	}
	reservation, ok := s.reserveCharge(w, a, "create_campaign", in.Name, totalCharge)
	if !ok {
		return
	}
	usedCharge := int64(0)
	defer func() { reservation.CommitUsed(usedCharge) }()

	c, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()

	chainID, err := c.CreateCampaign(r.Context(), in.Name, in.DiscountType,
		uint64(in.DiscountValue), uint32(in.TotalSupply), uint64(in.ValidUntil))
	if err != nil {
		writeErr(w, err)
		return
	}
	campaignTx := c.LastTransactionHash()
	usedCharge += mcrCreateCampaign
	now := time.Now().Unix()
	res, err := s.db.Exec(`INSERT INTO campaigns
	  (org_id, env, chain_id, contract_id, kind, name, discount_type, discount_value, total_supply, valid_until, tx_hash, created_at)
	  VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.OrgID, a.Env, chainID, currentContractID, in.Kind, in.Name, in.DiscountType, in.DiscountValue, in.TotalSupply, in.ValidUntil, campaignTx, now)
	if err != nil {
		writeInternal(w, err, "index created campaign")
		return
	}
	id, _ := res.LastInsertId()
	s.logActivity(a, "campaign", "", "Campaign created · "+in.Name, campaignTx, &id, 0)

	// shared kinds register their code in the same flow (wizard step 2)
	if isShared {
		if err := c.RegisterShared(r.Context(), chainID, sharedCode, sharedAttr, sharedToken, sharedRate); err != nil {
			writeErr(w, err)
			return
		}
		sharedTx := c.LastTransactionHash()
		usedCharge += mcrRegisterShared
		if _, err := s.db.Exec(`INSERT INTO shared_codes (campaign_id, code, attributed_to, payout_token, payout_rate, tx_hash, created_at)
		  VALUES (?,?,?,?,?,?,?)`, id, sharedCode, sharedAttr, sharedToken, sharedRate.String(), sharedTx, now); err != nil {
			writeProblem(w, 500, "shared code was registered on-chain but could not be indexed locally")
			return
		}
		s.logActivity(a, "campaign", sharedCode, "Shared code registered · "+sharedCode, sharedTx, &id, 0)
	}

	out, _ := s.campaignByID(a, id)
	writeJSON(w, 201, out)
}

func (s *server) campaignByID(a *authCtx, id int64) (*campaignOut, error) {
	row := s.db.QueryRow(`SELECT id, chain_id, contract_id, kind, name, discount_type, discount_value, total_supply,
	  valid_until, minted, burned, archived, COALESCE(tx_hash,''), created_at
	  FROM campaigns WHERE id = ? AND org_id = ? AND env = ?`, id, a.OrgID, a.Env)
	var o campaignOut
	var archived int
	if err := row.Scan(&o.ID, &o.ChainID, &o.ContractID, &o.Kind, &o.Name, &o.DiscountType, &o.DiscountValue,
		&o.TotalSupply, &o.ValidUntil, &o.Minted, &o.Burned, &archived, &o.TxHash, &o.CreatedAt); err != nil {
		return nil, err
	}
	o.Archived = archived == 1
	var code string
	var attr sql.NullString
	var rate string
	err := s.db.QueryRow(`SELECT code, attributed_to, payout_rate FROM shared_codes WHERE campaign_id = ? ORDER BY id LIMIT 1`,
		id).Scan(&code, &attr, &rate)
	if err == nil {
		o.SharedCode = code
		if attr.Valid {
			o.AttributedTo = attr.String
		}
		o.PayoutRate = rate
		if err := s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0),
		  COALESCE(SUM(CASE WHEN e.committed_period IS NULL THEN e.count ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN e.committed_period = -1 THEN e.count ELSE 0 END),0)
		  FROM shared_events e JOIN shared_codes sc ON sc.id = e.shared_code_id
		  WHERE sc.campaign_id = ?`, id).Scan(&o.EventsTotal, &o.PendingEvents, &o.LegacyEvents); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return &o, nil
}

func (s *server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	rows, err := s.db.Query(`SELECT id FROM campaigns WHERE org_id = ? AND env = ? ORDER BY id DESC`, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "list campaigns")
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			writeInternal(w, err, "decode campaign id")
			return
		}
		ids = append(ids, id)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeInternal(w, rowsErr, "read campaigns")
		return
	}
	out := []*campaignOut{}
	for _, id := range ids {
		c, err := s.campaignByID(a, id)
		if err != nil {
			writeInternal(w, err, "load campaign")
			return
		}
		out = append(out, c)
	}
	writeJSON(w, 200, map[string]any{"campaigns": out})
}

func (s *server) handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	c, err := s.campaignByID(a, id)
	if err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}
	// codes list (Burn side)
	codes := []map[string]any{}
	rows, err := s.db.Query(`SELECT code, status, COALESCE(token_id,0), COALESCE(tx_hash,''), created_at, COALESCE(redeemed_at,0)
	  FROM codes WHERE campaign_id = ? ORDER BY id DESC LIMIT 500`, id)
	if err != nil {
		writeInternal(w, err, "load campaign codes")
		return
	}
	for rows.Next() {
		var code, status, tx string
		var tokenID, created, redeemed int64
		if err := rows.Scan(&code, &status, &tokenID, &tx, &created, &redeemed); err != nil {
			rows.Close()
			writeInternal(w, err, "decode campaign code")
			return
		}
		codes = append(codes, map[string]any{
			"code": code, "status": status, "token_id": tokenID, "tx_hash": tx,
			"created_at": created, "redeemed_at": redeemed,
		})
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeInternal(w, rowsErr, "read campaign codes")
		return
	}
	writeJSON(w, 200, map[string]any{"campaign": c, "codes": codes})
}

func (s *server) handleArchiveCampaign(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeProblem(w, 404, "campaign not found")
		return
	}
	result, err := s.db.Exec(`UPDATE campaigns SET archived = 1 WHERE id = ? AND org_id = ? AND env = ?`, id, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "archive campaign")
		return
	}
	updated, err := result.RowsAffected()
	if err != nil {
		writeInternal(w, err, "confirm archived campaign")
		return
	}
	if updated == 0 {
		writeProblem(w, 404, "campaign not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ── codes (Burn) ───────────────────────────────────────────────────

const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // no 0/O/1/I/L

func genCode(prefix string, n int) (string, error) {
	out := make([]byte, n)
	bound := big.NewInt(int64(len(codeAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, bound)
		if err != nil {
			return "", err
		}
		out[i] = codeAlphabet[v.Int64()]
	}
	if prefix != "" {
		return prefix + "-" + string(out), nil
	}
	return string(out), nil
}

func (s *server) handleIssueCodes(w http.ResponseWriter, r *http.Request) {
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
	if !s.requireCurrentContract(w, c.ContractID) {
		return
	}
	var in struct {
		Codes    []string `json:"codes"`
		Generate *struct {
			Count  int    `json:"count"`
			Prefix string `json:"prefix"`
		} `json:"generate"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	codes := []string{}
	for _, cd := range in.Codes {
		cd = strings.ToUpper(strings.TrimSpace(cd))
		if cd != "" {
			codes = append(codes, cd)
		}
	}
	if in.Generate != nil && in.Generate.Count > 0 {
		if in.Generate.Count > 1000 || len(codes) > 1000-in.Generate.Count {
			writeProblem(w, 400, "max 1000 codes per request")
			return
		}
		prefix := strings.ToUpper(strings.TrimSpace(in.Generate.Prefix))
		if len(prefix)+1+12 > maxCodeLen {
			writeProblem(w, 400, "generated-code prefix is too long")
			return
		}
		for i := 0; i < in.Generate.Count; i++ {
			generated, err := genCode(prefix, 12)
			if err != nil {
				writeProblem(w, 500, "secure randomness unavailable")
				return
			}
			codes = append(codes, generated)
		}
	}
	if len(codes) == 0 {
		writeProblem(w, 400, "provide codes[] or generate.count")
		return
	}
	if len(codes) > 1000 {
		writeProblem(w, 400, "max 1000 codes per request")
		return
	}
	for _, cd := range codes {
		if len(cd) > maxCodeLen {
			writeProblem(w, 400, fmt.Sprintf("code %q exceeds %d chars", cd, maxCodeLen))
			return
		}
	}
	seen := make(map[string]struct{}, len(codes))
	for _, cd := range codes {
		if _, exists := seen[cd]; exists {
			writeProblem(w, 400, fmt.Sprintf("duplicate code %q in request", cd))
			return
		}
		seen[cd] = struct{}{}
	}

	reservation, ok := s.reserveCharge(w, a, "issue_codes", fmt.Sprintf("%d codes · %s", len(codes), c.Name), int64(len(codes))*mcrIssuePerCode)
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

	now := time.Now().Unix()
	issued := []map[string]any{}
	lastTxHash := ""
	// chunk to the contract batch bound
	for start := 0; start < len(codes); start += maxBatch {
		end := start + maxBatch
		if end > len(codes) {
			end = len(codes)
		}
		chunk := codes[start:end]
		ids, err := cl.IssueUnique(r.Context(), c.ChainID, chunk)
		if err != nil {
			// partial success is reported: earlier chunks were applied
			if len(issued) > 0 {
				reservation.CommitUsed(int64(len(issued)) * mcrIssuePerCode)
				writeJSON(w, 207, map[string]any{"issued": issued, "error": problemFromErr(err)})
				return
			}
			writeErr(w, err)
			return
		}
		lastTxHash = cl.LastTransactionHash()
		for i, cd := range chunk {
			var tid any
			if i < len(ids) {
				tid = ids[i]
			}
			_, _ = s.db.Exec(`INSERT INTO codes (campaign_id, code, token_id, tx_hash, created_at) VALUES (?,?,?,?,?)`,
				id, cd, tid, lastTxHash, now)
			issued = append(issued, map[string]any{"code": cd, "token_id": tid, "tx_hash": lastTxHash})
		}
	}
	_, _ = s.db.Exec(`UPDATE campaigns SET minted = minted + ? WHERE id = ?`, len(issued), id)
	reservation.Commit()
	s.logActivity(a, "issue", "", fmt.Sprintf("%d codes issued · %s", len(issued), c.Name), lastTxHash, &id, 0)
	writeJSON(w, 201, map[string]any{"issued": issued})
}

func problemFromErr(err error) problem {
	var p problem
	p.Status = 409
	p.Message = err.Error()
	return p
}

// ── verify + redemptions ───────────────────────────────────────────

func (s *server) handleVerify(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	cid, _ := strconv.ParseInt(r.URL.Query().Get("campaign_id"), 10, 64)
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	c, err := s.campaignByID(a, cid)
	if err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}
	if !s.requireCurrentContract(w, c.ContractID) {
		return
	}
	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()
	tok, err := cl.Verify(r.Context(), c.ChainID, code)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"token_id": tok.TokenID, "code": tok.Code, "is_burned": tok.IsBurned,
		"minted_at": tok.MintedAt, "burned_at": tok.BurnedAt,
		"redeemer_ref": hex.EncodeToString(tok.RedeemerRef),
	})
}

func (s *server) handleRedeem(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var in struct {
		CampaignID  int64  `json:"campaign_id"`
		Code        string `json:"code"`
		RedeemerRef string `json:"redeemer_ref"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if len(in.RedeemerRef) > maxReferenceLen {
		writeProblem(w, 400, "redeemer_ref exceeds 512 bytes")
		return
	}
	c, err := s.campaignByID(a, in.CampaignID)
	if err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}
	if c.Archived {
		writeProblem(w, http.StatusConflict, "campaign is archived")
		return
	}
	if !s.requireCurrentContract(w, c.ContractID) {
		return
	}

	// opaque redeemer commitment: SHA-256(random nonce ∥ "|" ∥ ref) — no PII
	// on-chain, and the platform stores only the hash (ADR-005/010). The nonce
	// is returned once so the integrator can prove the ref later if they must.
	nonce, err := randHex(16)
	if err != nil {
		writeProblem(w, 500, "secure randomness unavailable")
		return
	}
	var refHash [32]byte
	if in.RedeemerRef != "" {
		refHash = sha256.Sum256([]byte(nonce + "|" + in.RedeemerRef))
	} else {
		if _, err := rand.Read(refHash[:]); err != nil {
			writeProblem(w, 500, "secure randomness unavailable")
			return
		}
	}
	reservation, ok := s.reserveCharge(w, a, "redeem", in.Code+" · "+c.Name, mcrRedeem)
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

	rec, err := cl.RedeemUnique(r.Context(), c.ChainID, in.Code, refHash)
	now := time.Now().Unix()
	if err != nil {
		// rejected redemptions are first-class events (activity + row)
		p := problem{Status: 409, Message: err.Error()}
		if code, ok := contractCodeOf(err); ok {
			p.Code = code
			p.Name = codeName(code)
			if m := friendlyErrByNum(code); m != "" {
				p.Message = m
			}
		}
		_, _ = s.db.Exec(`INSERT INTO redemptions (org_id, env, campaign_id, code, ok, error_code, error_name, created_at)
		  VALUES (?,?,?,?,0,?,?,?)`, a.OrgID, a.Env, in.CampaignID, in.Code, p.Code, p.Name, now)
		s.logActivity(a, "rejected", in.Code, "rejected — "+strings.TrimSuffix(p.Message, "."), "", &in.CampaignID, p.Code)
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(p.Status)
		writeRaw(w, p)
		return
	}
	txHash := cl.LastTransactionHash()
	reservation.Commit()
	_, _ = s.db.Exec(`INSERT INTO redemptions
	  (org_id, env, campaign_id, code, ok, redeemer_ref, token_id, ledger_seq, tx_hash, created_at)
	  VALUES (?,?,?,?,1,?,?,?,?,?)`,
		a.OrgID, a.Env, in.CampaignID, in.Code, hex.EncodeToString(rec.RedeemerRef), rec.TokenID, rec.LedgerSeq, txHash, now)
	_, _ = s.db.Exec(`UPDATE codes SET status = 'redeemed', redeemed_at = ? WHERE campaign_id = ? AND code = ?`,
		now, in.CampaignID, in.Code)
	_, _ = s.db.Exec(`UPDATE campaigns SET burned = burned + 1 WHERE id = ?`, in.CampaignID)
	s.logActivity(a, "redemption", in.Code, "redeemed · "+c.Name, txHash, &in.CampaignID, 0)

	// loyalty reward vouchers flip their status when redeemed
	_, _ = s.db.Exec(`UPDATE rewards SET redeemed = 1 WHERE code = ? AND program_id IN
	  (SELECT id FROM loyalty_programs WHERE campaign_id = ?)`, in.Code, in.CampaignID)

	writeJSON(w, 201, map[string]any{
		"receipt": map[string]any{
			"token_id": rec.TokenID, "code": rec.Code, "campaign_id": in.CampaignID,
			"campaign_name": rec.CampaignName, "discount_type": rec.DiscountType,
			"discount_value": rec.DiscountValue, "burned_at": rec.BurnedAt,
			"ledger_seq": rec.LedgerSeq, "tx_hash": txHash, "redeemer_ref": hex.EncodeToString(rec.RedeemerRef),
		},
		"redeemer_nonce": nonce,
	})
}

func (s *server) handleListRedemptions(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	q := `SELECT r.id, r.campaign_id, c.name, r.code, r.ok, COALESCE(r.error_code,0), COALESCE(r.error_name,''),
	  COALESCE(r.redeemer_ref,''), COALESCE(r.token_id,0), COALESCE(r.ledger_seq,0), COALESCE(r.tx_hash,''), r.created_at
	  FROM redemptions r JOIN campaigns c ON c.id = r.campaign_id
	  WHERE r.org_id = ? AND r.env = ? ORDER BY r.id DESC LIMIT 200`
	rows, err := s.db.Query(q, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "list redemptions")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, cid, ecode, tokenID, seq, created int64
		var name, code, ename, ref, txHash string
		var ok int
		if err := rows.Scan(&id, &cid, &name, &code, &ok, &ecode, &ename, &ref, &tokenID, &seq, &txHash, &created); err != nil {
			writeInternal(w, err, "decode redemption")
			return
		}
		out = append(out, map[string]any{
			"id": id, "campaign_id": cid, "campaign_name": name, "code": code, "ok": ok == 1,
			"error_code": ecode, "error_name": ename, "redeemer_ref": ref,
			"token_id": tokenID, "ledger_seq": seq, "tx_hash": txHash, "created_at": created,
		})
	}
	if err := rows.Err(); err != nil {
		writeInternal(w, err, "read redemptions")
		return
	}
	writeJSON(w, 200, map[string]any{"redemptions": out})
}

// helpers bridging sd error codes without importing in handlers everywhere

func writeErrFunding(w http.ResponseWriter, err error) {
	if errors.Is(err, errAccountFunding) {
		writeProblem(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeErr(w, err)
}

func writeRaw(w http.ResponseWriter, v any) { writeJSONBody(w, v) }
