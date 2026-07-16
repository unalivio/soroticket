package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/stellar/go-stellar-sdk/keypair"
)

const (
	maxAuditPageSize    = 100
	maxReceiptsPerTally = 10_000
)

// handleAuditTally publishes the signed receipt set and one Merkle inclusion
// proof per receipt. It intentionally needs no Sorodeal account/API key: the
// value of a commitment is that creators and auditors can verify it without
// trusting the merchant dashboard.
func (s *server) handleAuditTally(w http.ResponseWriter, r *http.Request) {
	chainID, err := strconv.ParseUint(r.PathValue("chain_id"), 10, 64)
	if err != nil || chainID == 0 {
		writeProblem(w, 400, "invalid chain campaign id")
		return
	}
	period, err := strconv.ParseUint(r.PathValue("period"), 10, 64)
	if err != nil || period == 0 {
		writeProblem(w, 400, "invalid period")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	if code == "" || len(code) > maxCodeLen {
		writeProblem(w, 400, "invalid shared code")
		return
	}
	// chain_id+code+period alone is ambiguous across contract deployments
	// (numbering restarts). Default to the current deployment; the deprecated
	// one stays reachable explicitly for historical audits.
	auditContract := currentContractID
	if raw := strings.TrimSpace(r.URL.Query().Get("contract")); raw != "" {
		if raw != currentContractID && raw != legacyContractID {
			writeProblem(w, 404, "unknown contract deployment")
			return
		}
		auditContract = raw
	}
	cursor := int64(0)
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cursor < 0 {
			writeProblem(w, 400, "cursor must be a non-negative integer")
			return
		}
	}
	limit := int64(maxAuditPageSize)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || limit < 1 || limit > maxAuditPageSize {
			writeProblem(w, 400, "limit must be between 1 and 100")
			return
		}
	}
	var sharedID int64
	var count int64
	var root string
	var committedAt int64
	err = s.db.QueryRow(`SELECT sc.id, t.count, t.merkle_root, t.committed_at
	  FROM tallies t
	  JOIN shared_codes sc ON sc.id = t.shared_code_id
	  JOIN campaigns c ON c.id = sc.campaign_id
	  WHERE c.chain_id = ? AND sc.code = ? AND t.period = ? AND c.contract_id = ?`,
		chainID, code, period, auditContract).
		Scan(&sharedID, &count, &root, &committedAt)
	if err != nil {
		writeProblem(w, 404, "committed tally not found")
		return
	}

	var receiptTotal, receiptCount int64
	if err = s.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(e.count),0)
	  FROM shared_events e WHERE e.shared_code_id = ? AND e.committed_period = ?`,
		sharedID, period).Scan(&receiptTotal, &receiptCount); err != nil {
		writeProblem(w, 500, "could not summarize audit receipts")
		return
	}
	if receiptTotal == 0 {
		writeProblem(w, http.StatusGone, "tally has no published signed receipt set")
		return
	}
	if receiptTotal > maxReceiptsPerTally {
		writeProblem(w, 500, "committed tally has an invalid receipt-set size")
		return
	}
	if receiptCount != count {
		writeProblem(w, 500, "receipt counts do not equal the committed tally")
		return
	}
	var signedTotal int64
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM shared_events e
	  JOIN event_receipts er ON er.event_id=e.id
	  WHERE e.shared_code_id=? AND e.committed_period=?`, sharedID, period).Scan(&signedTotal); err != nil {
		writeProblem(w, 500, "could not summarize signed audit receipts")
		return
	}
	if signedTotal == 0 {
		writeProblem(w, http.StatusGone, "legacy tally predates signed receipts and has no verifiable receipt set")
		return
	}
	if signedTotal != receiptTotal {
		writeProblem(w, 500, "committed tally mixes signed and unsigned receipts")
		return
	}
	if cursor > receiptTotal {
		writeProblem(w, 400, "cursor exceeds the receipt set")
		return
	}

	// Recompute the committed root once from the small fixed-size leaf hashes.
	// Payload/signature verification is then performed for the requested page;
	// this keeps unauthenticated requests bounded while every returned receipt
	// remains independently verifiable.
	rows, err := s.db.Query(`SELECT er.leaf_hash
	  FROM shared_events e JOIN event_receipts er ON er.event_id = e.id
	  WHERE e.shared_code_id = ? AND e.committed_period = ? ORDER BY e.id`, sharedID, period)
	if err != nil {
		writeProblem(w, 500, "could not load audit receipt hashes")
		return
	}
	leaves := make([][32]byte, 0, receiptTotal)
	for rows.Next() {
		var leafHex string
		if err := rows.Scan(&leafHex); err != nil {
			rows.Close()
			writeProblem(w, 500, "could not decode audit receipt hash")
			return
		}
		leaf, err := decodeHash(leafHex)
		if err != nil {
			rows.Close()
			writeProblem(w, 500, "stored receipt hash is invalid")
			return
		}
		leaves = append(leaves, leaf)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil || int64(len(leaves)) != receiptTotal {
		writeProblem(w, 500, "could not read the complete receipt hash set")
		return
	}
	levels := merkleLevels(leaves)
	if len(levels) == 0 || hexRoot(levels[len(levels)-1][0]) != root {
		writeProblem(w, 500, "receipt set does not match the committed Merkle root")
		return
	}

	type storedReceipt struct {
		count     int64
		payload   []byte
		leaf      string
		signature string
		signer    string
	}
	rows, err = s.db.Query(`SELECT e.count, er.payload, er.leaf_hash, er.signature, er.signer
	  FROM shared_events e JOIN event_receipts er ON er.event_id = e.id
	  WHERE e.shared_code_id = ? AND e.committed_period = ? ORDER BY e.id
	  LIMIT ? OFFSET ?`, sharedID, period, limit, cursor)
	if err != nil {
		writeProblem(w, 500, "could not load audit receipts")
		return
	}
	stored := []storedReceipt{}
	for rows.Next() {
		var sr storedReceipt
		if err := rows.Scan(&sr.count, &sr.payload, &sr.leaf, &sr.signature, &sr.signer); err != nil {
			rows.Close()
			writeProblem(w, 500, "could not decode audit receipt")
			return
		}
		leaf, err := decodeHash(sr.leaf)
		if err != nil {
			rows.Close()
			writeProblem(w, 500, "stored receipt hash is invalid")
			return
		}
		if sha256.Sum256(sr.payload) != leaf {
			rows.Close()
			writeProblem(w, 500, "stored receipt payload does not match its leaf hash")
			return
		}
		var payload receiptPayload
		if err := json.Unmarshal(sr.payload, &payload); err != nil ||
			(payload.Version != 1 && payload.Version != 2) ||
			payload.CampaignID != chainID || payload.Code != code || payload.Count <= 0 ||
			payload.Count != sr.count || payload.Signer != sr.signer ||
			(payload.Version == 2 && (payload.Network != cloudNetwork || payload.ContractID != auditContract)) {
			rows.Close()
			writeProblem(w, 500, "stored receipt payload is inconsistent")
			return
		}
		publicKey, err := keypair.ParseAddress(sr.signer)
		if err != nil {
			rows.Close()
			writeProblem(w, 500, "stored receipt signer is invalid")
			return
		}
		signature, err := base64.StdEncoding.DecodeString(sr.signature)
		if err != nil || publicKey.Verify(sr.payload, signature) != nil {
			rows.Close()
			writeProblem(w, 500, "stored receipt signature is invalid")
			return
		}
		stored = append(stored, sr)
	}
	rowsErr = rows.Err()
	rows.Close()
	if rowsErr != nil {
		writeProblem(w, 500, "could not read audit receipts")
		return
	}

	receipts := make([]map[string]any, 0, len(stored))
	for i, sr := range stored {
		leafIndex := int(cursor) + i
		receipts = append(receipts, map[string]any{
			"payload": json.RawMessage(sr.payload), "leaf_hash": sr.leaf,
			"signature": sr.signature, "signer": sr.signer,
			"proof": merkleProofFromLevels(levels, leafIndex),
		})
	}
	pageEnd := cursor + int64(len(receipts))
	var nextCursor any
	if pageEnd < receiptTotal {
		nextCursor = pageEnd
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, 200, map[string]any{
		"network": cloudNetwork, "contract_id": auditContract,
		"campaign_id": chainID, "code": code, "period": period,
		"count": count, "merkle_root": root, "committed_at": committedAt,
		"receipt_total": receiptTotal, "cursor": cursor, "limit": limit,
		"next_cursor": nextCursor, "receipts": receipts,
	})
}
