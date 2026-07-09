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
	maxBatch   = 100 // contract MAX_BATCH
	maxCodeLen = 64
)

var kindLabels = map[string]bool{"coupon": true, "creator": true, "voucher": true, "ticket": true, "loyalty": true}

type campaignOut struct {
	ID            int64  `json:"id"`
	ChainID       uint64 `json:"chain_id"`
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
	Events30d     int64  `json:"events_30d,omitempty"`
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
		writeProblem(w, 400, "kind must be one of coupon|creator|voucher|ticket|loyalty")
		return
	}
	if strings.TrimSpace(in.Name) == "" || in.DiscountType == "" {
		writeProblem(w, 400, "name and discount_type are required")
		return
	}
	if in.ValidUntil == 0 {
		in.ValidUntil = time.Now().AddDate(1, 0, 0).Unix()
	}
	if in.TotalSupply <= 0 {
		in.TotalSupply = 10_000 // shared/loyalty campaigns don't mint up-front; give headroom for vouchers
	}
	isShared := in.Kind == "coupon" || in.Kind == "creator"
	if isShared && (in.Shared == nil || strings.TrimSpace(in.Shared.Code) == "") {
		writeProblem(w, 400, "shared.code is required for coupon/creator campaigns")
		return
	}

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
	now := time.Now().Unix()
	res, err := s.db.Exec(`INSERT INTO campaigns
	  (org_id, env, chain_id, kind, name, discount_type, discount_value, total_supply, valid_until, created_at)
	  VALUES (?,?,?,?,?,?,?,?,?,?)`,
		a.OrgID, a.Env, chainID, in.Kind, in.Name, in.DiscountType, in.DiscountValue, in.TotalSupply, in.ValidUntil, now)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()

	if !s.charge(w, a, "create_campaign", in.Name, mcrCreateCampaign, "") {
		return
	}
	s.logActivity(a, "campaign", "", "Campaign created · "+in.Name, "", &id, 0)

	// shared kinds register their code in the same flow (wizard step 2)
	if isShared {
		code := strings.ToUpper(strings.TrimSpace(in.Shared.Code))
		var attr, tok *string
		rate := big.NewInt(0)
		if in.Kind == "creator" {
			at := strings.TrimSpace(in.Shared.AttributedTo)
			if at == "" {
				writeProblem(w, 400, "attributed_to (creator address) is required for creator codes")
				return
			}
			attr = &at
			if in.Shared.PayoutRate != "" && in.Shared.PayoutRate != "0" {
				var ok bool
				rate, ok = new(big.Int).SetString(in.Shared.PayoutRate, 10)
				if !ok || rate.Sign() < 0 {
					writeProblem(w, 400, "payout_rate must be a non-negative integer (token base-units)")
					return
				}
				t := payoutToken
				tok = &t
			}
		}
		if err := c.RegisterShared(r.Context(), chainID, code, attr, tok, rate); err != nil {
			writeErr(w, err)
			return
		}
		_, _ = s.db.Exec(`INSERT INTO shared_codes (campaign_id, code, attributed_to, payout_token, payout_rate, created_at)
		  VALUES (?,?,?,?,?,?)`, id, code, attr, tok, rate.String(), now)
		if !s.charge(w, a, "register_shared", code, mcrRegisterShared, "") {
			return
		}
		s.logActivity(a, "campaign", code, "Shared code registered · "+code, "", &id, 0)
	}

	out, _ := s.campaignByID(a, id)
	writeJSON(w, 201, out)
}

func (s *server) campaignByID(a *authCtx, id int64) (*campaignOut, error) {
	row := s.db.QueryRow(`SELECT id, chain_id, kind, name, discount_type, discount_value, total_supply,
	  valid_until, minted, burned, archived, COALESCE(tx_hash,''), created_at
	  FROM campaigns WHERE id = ? AND org_id = ? AND env = ?`, id, a.OrgID, a.Env)
	var o campaignOut
	var archived int
	if err := row.Scan(&o.ID, &o.ChainID, &o.Kind, &o.Name, &o.DiscountType, &o.DiscountValue,
		&o.TotalSupply, &o.ValidUntil, &o.Minted, &o.Burned, &archived, &o.TxHash, &o.CreatedAt); err != nil {
		return nil, err
	}
	o.Archived = archived == 1
	var code string
	var attr sql.NullString
	var rate string
	if err := s.db.QueryRow(`SELECT code, attributed_to, payout_rate FROM shared_codes WHERE campaign_id = ? ORDER BY id LIMIT 1`,
		id).Scan(&code, &attr, &rate); err == nil {
		o.SharedCode = code
		if attr.Valid {
			o.AttributedTo = attr.String
		}
		o.PayoutRate = rate
		_ = s.db.QueryRow(`SELECT COALESCE(SUM(e.count),0) FROM shared_events e
		  JOIN shared_codes sc ON sc.id = e.shared_code_id WHERE sc.campaign_id = ?`, id).Scan(&o.Events30d)
	}
	return &o, nil
}

func (s *server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	rows, err := s.db.Query(`SELECT id FROM campaigns WHERE org_id = ? AND env = ? ORDER BY id DESC`, a.OrgID, a.Env)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	out := []*campaignOut{}
	for _, id := range ids {
		if c, err := s.campaignByID(a, id); err == nil {
			out = append(out, c)
		}
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
	rows, _ := s.db.Query(`SELECT code, status, COALESCE(token_id,0), COALESCE(tx_hash,''), created_at, COALESCE(redeemed_at,0)
	  FROM codes WHERE campaign_id = ? ORDER BY id DESC LIMIT 500`, id)
	for rows.Next() {
		var code, status, tx string
		var tokenID, created, redeemed int64
		_ = rows.Scan(&code, &status, &tokenID, &tx, &created, &redeemed)
		codes = append(codes, map[string]any{
			"code": code, "status": status, "token_id": tokenID, "tx_hash": tx,
			"created_at": created, "redeemed_at": redeemed,
		})
	}
	rows.Close()
	writeJSON(w, 200, map[string]any{"campaign": c, "codes": codes})
}

func (s *server) handleArchiveCampaign(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id := r.PathValue("id")
	_, err := s.db.Exec(`UPDATE campaigns SET archived = 1 WHERE id = ? AND org_id = ? AND env = ?`, id, a.OrgID, a.Env)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ── codes (Burn) ───────────────────────────────────────────────────

const codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789" // no 0/O/1/I/L

func genCode(prefix string, n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	out := make([]byte, n)
	for i := range b {
		out[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	if prefix != "" {
		return prefix + "-" + string(out)
	}
	return string(out)
}

func (s *server) handleIssueCodes(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	c, err := s.campaignByID(a, id)
	if err != nil {
		writeProblem(w, 404, "campaign not found")
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
		for i := 0; i < in.Generate.Count; i++ {
			codes = append(codes, genCode(strings.ToUpper(strings.TrimSpace(in.Generate.Prefix)), 8))
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

	cl, release, err := s.clientFor(a.OrgID, a.Env)
	if err != nil {
		writeErrFunding(w, err)
		return
	}
	defer release()

	now := time.Now().Unix()
	issued := []map[string]any{}
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
				writeJSON(w, 207, map[string]any{"issued": issued, "error": problemFromErr(err)})
				return
			}
			writeErr(w, err)
			return
		}
		for i, cd := range chunk {
			var tid any
			if i < len(ids) {
				tid = ids[i]
			}
			_, _ = s.db.Exec(`INSERT INTO codes (campaign_id, code, token_id, created_at) VALUES (?,?,?,?)`,
				id, cd, tid, now)
			issued = append(issued, map[string]any{"code": cd, "token_id": tid})
		}
	}
	_, _ = s.db.Exec(`UPDATE campaigns SET minted = minted + ? WHERE id = ?`, len(issued), id)
	if !s.charge(w, a, "issue_codes", fmt.Sprintf("%d codes · %s", len(issued), c.Name), int64(len(issued))*mcrIssuePerCode, "") {
		return
	}
	s.logActivity(a, "issue", "", fmt.Sprintf("%d codes issued · %s", len(issued), c.Name), "", &id, 0)
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
	c, err := s.campaignByID(a, in.CampaignID)
	if err != nil {
		writeProblem(w, 404, "campaign not found")
		return
	}

	// opaque redeemer commitment: SHA-256(random nonce ∥ "|" ∥ ref) — no PII
	// on-chain, and the platform stores only the hash (ADR-005/010). The nonce
	// is returned once so the integrator can prove the ref later if they must.
	nonce := randHex(16)
	var refHash [32]byte
	if in.RedeemerRef != "" {
		refHash = sha256.Sum256([]byte(nonce + "|" + in.RedeemerRef))
	} else {
		_, _ = rand.Read(refHash[:])
	}

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
	_, _ = s.db.Exec(`INSERT INTO redemptions
	  (org_id, env, campaign_id, code, ok, redeemer_ref, token_id, ledger_seq, tx_hash, created_at)
	  VALUES (?,?,?,?,1,?,?,?,?,?)`,
		a.OrgID, a.Env, in.CampaignID, in.Code, hex.EncodeToString(rec.RedeemerRef), rec.TokenID, rec.LedgerSeq, "", now)
	_, _ = s.db.Exec(`UPDATE codes SET status = 'redeemed', redeemed_at = ? WHERE campaign_id = ? AND code = ?`,
		now, in.CampaignID, in.Code)
	_, _ = s.db.Exec(`UPDATE campaigns SET burned = burned + 1 WHERE id = ?`, in.CampaignID)
	if !s.charge(w, a, "redeem", in.Code+" · "+c.Name, mcrRedeem, "") {
		return
	}
	s.logActivity(a, "redemption", in.Code, "redeemed · "+c.Name, "", &in.CampaignID, 0)

	// loyalty reward vouchers flip their status when redeemed
	_, _ = s.db.Exec(`UPDATE rewards SET redeemed = 1 WHERE code = ? AND program_id IN
	  (SELECT id FROM loyalty_programs WHERE campaign_id = ?)`, in.Code, in.CampaignID)

	writeJSON(w, 201, map[string]any{
		"receipt": map[string]any{
			"token_id": rec.TokenID, "code": rec.Code, "campaign_id": in.CampaignID,
			"campaign_name": rec.CampaignName, "discount_type": rec.DiscountType,
			"discount_value": rec.DiscountValue, "burned_at": rec.BurnedAt,
			"ledger_seq": rec.LedgerSeq, "redeemer_ref": hex.EncodeToString(rec.RedeemerRef),
		},
		"redeemer_nonce": nonce,
	})
}

func (s *server) handleListRedemptions(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	q := `SELECT r.id, r.campaign_id, c.name, r.code, r.ok, COALESCE(r.error_code,0), COALESCE(r.error_name,''),
	  COALESCE(r.redeemer_ref,''), COALESCE(r.token_id,0), COALESCE(r.ledger_seq,0), r.created_at
	  FROM redemptions r JOIN campaigns c ON c.id = r.campaign_id
	  WHERE r.org_id = ? AND r.env = ? ORDER BY r.id DESC LIMIT 200`
	rows, err := s.db.Query(q, a.OrgID, a.Env)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, cid, ecode, tokenID, seq, created int64
		var name, code, ename, ref string
		var ok int
		_ = rows.Scan(&id, &cid, &name, &code, &ok, &ecode, &ename, &ref, &tokenID, &seq, &created)
		out = append(out, map[string]any{
			"id": id, "campaign_id": cid, "campaign_name": name, "code": code, "ok": ok == 1,
			"error_code": ecode, "error_name": ename, "redeemer_ref": ref,
			"token_id": tokenID, "ledger_seq": seq, "created_at": created,
		})
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
