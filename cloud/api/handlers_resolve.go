package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

// handleResolveCode routes a bare scanned code (no campaign context — exactly
// what a WhatsApp scan delivers) to its campaign inside the caller's org and
// environment. Shared codes resolve first (customer-scan path), then unique
// codes (employee-validation path). The same code string may exist in several
// campaigns (ADR-009): active campaigns win, then the newest.
func (s *server) handleResolveCode(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if code == "" || len(code) > maxCodeLen {
		writeProblem(w, 400, "code is required (1-64 bytes)")
		return
	}
	now := time.Now().Unix()

	// Camino A — shared code: the scan IS the redemption event.
	var out struct {
		campaignID, discountValue, validUntil int64
		chainID                               uint64
		archived                              int
		kind, name, discountType, contract    string
		attributedTo                          sql.NullString
	}
	err := s.db.QueryRow(`SELECT c.id, c.chain_id, c.kind, c.name, c.discount_type, c.discount_value,
	  c.valid_until, c.archived, c.contract_id, sc.attributed_to
	  FROM shared_codes sc JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE sc.code = ? AND c.org_id = ? AND c.env = ?
	  ORDER BY (c.archived = 0 AND c.valid_until > ?) DESC, c.id DESC LIMIT 1`,
		code, a.OrgID, a.Env, now).
		Scan(&out.campaignID, &out.chainID, &out.kind, &out.name, &out.discountType,
			&out.discountValue, &out.validUntil, &out.archived, &out.contract, &out.attributedTo)
	if err == nil {
		writeJSON(w, 200, map[string]any{
			"type": "shared", "code": code, "campaign_id": out.campaignID,
			"chain_id": out.chainID, "kind": out.kind, "campaign_name": out.name,
			"discount_type": out.discountType, "discount_value": out.discountValue,
			"valid_until": out.validUntil, "archived": out.archived == 1,
			"expired":     out.validUntil < now,
			"attributed":  out.attributedTo.Valid && out.attributedTo.String != "",
			"contract_id": out.contract,
		})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		writeInternal(w, err, "resolve shared code")
		return
	}

	// Camino B — unique code: the scan IS the validation (burn).
	var status string
	err = s.db.QueryRow(`SELECT c.id, c.chain_id, c.kind, c.name, c.discount_type, c.discount_value,
	  c.valid_until, c.archived, c.contract_id, co.status
	  FROM codes co JOIN campaigns c ON c.id = co.campaign_id
	  WHERE co.code = ? AND c.org_id = ? AND c.env = ?
	  ORDER BY (c.archived = 0 AND c.valid_until > ?) DESC, c.id DESC LIMIT 1`,
		code, a.OrgID, a.Env, now).
		Scan(&out.campaignID, &out.chainID, &out.kind, &out.name, &out.discountType,
			&out.discountValue, &out.validUntil, &out.archived, &out.contract, &status)
	if errors.Is(err, sql.ErrNoRows) {
		writeProblem(w, 404, "code not found in this organization")
		return
	}
	if err != nil {
		writeInternal(w, err, "resolve unique code")
		return
	}
	writeJSON(w, 200, map[string]any{
		"type": "unique", "code": code, "campaign_id": out.campaignID,
		"chain_id": out.chainID, "kind": out.kind, "campaign_name": out.name,
		"discount_type": out.discountType, "discount_value": out.discountValue,
		"valid_until": out.validUntil, "archived": out.archived == 1,
		"expired": out.validUntil < now, "status": status,
		"contract_id": out.contract,
	})
}
