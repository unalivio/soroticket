package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
)

// merkleRoot computes a SHA-256 Merkle root over the given leaves (an odd node
// promotes). Empty input yields the zero root. Leaves here are the recorded
// off-chain events, so anyone holding the event set can recompute and audit
// the committed root (docs/SPEC.md §10 trust model).
func merkleRoot(leaves [][32]byte) [32]byte {
	levels := merkleLevels(leaves)
	if len(levels) == 0 {
		return [32]byte{}
	}
	return levels[len(levels)-1][0]
}

// merkleLevels builds each tree level once. Reusing these levels for a page of
// proofs avoids rebuilding the full tree for every receipt (quadratic work).
func merkleLevels(leaves [][32]byte) [][][32]byte {
	if len(leaves) == 0 {
		return nil
	}
	level := append([][32]byte(nil), leaves...)
	levels := [][][32]byte{level}
	for len(level) > 1 {
		next := make([][32]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				continue
			}
			h := merkleParent(level[i], level[i+1])
			next = append(next, h)
		}
		level = next
		levels = append(levels, level)
	}
	return levels
}

func merkleParent(left, right [32]byte) [32]byte {
	var pair [64]byte
	copy(pair[:32], left[:])
	copy(pair[32:], right[:])
	return sha256.Sum256(pair[:])
}

func hexRoot(r [32]byte) string { return hex.EncodeToString(r[:]) }

// receiptPayload is the canonical signed receipt. Version 2 adds the network
// and contract deployment (so a receipt is unambiguous across deployments)
// plus optional integrator-declared evidence metadata. Version 1 receipts
// remain stored and verifiable exactly as issued.
type receiptPayload struct {
	Version            int    `json:"version"`
	Network            string `json:"network,omitempty"`     // v2
	ContractID         string `json:"contract_id,omitempty"` // v2
	CampaignID         uint64 `json:"campaign_id"`
	Code               string `json:"code"`
	Count              int64  `json:"count"`
	CustomerCommitment string `json:"customer_commitment,omitempty"`
	OrderCommitment    string `json:"order_commitment,omitempty"`
	EvidenceType       string `json:"evidence_type,omitempty"`  // v2, integrator-declared
	ContextHash        string `json:"context_hash,omitempty"`   // v2, opaque integrator hash
	PolicyVersion      string `json:"policy_version,omitempty"` // v2, integrator-declared
	Timestamp          int64  `json:"timestamp"`
	Nonce              string `json:"nonce"`
	Signer             string `json:"signer"`
}

// receiptEvidence is optional integrator-declared metadata embedded in the
// signed payload. Cloud does not interpret it: it binds the integrator's own
// evidence policy (e.g. "whatsapp_scan" under policy v3 with a context hash)
// to the attestation, per docs/USE_CASES.md.
type receiptEvidence struct {
	Type          string
	ContextHash   string
	PolicyVersion string
}

type signedReceipt struct {
	Payload   []byte
	Leaf      [32]byte
	Signature string
	Signer    string
}

func (s *server) opaqueRef(domain, ref string) string {
	return opaqueReference(s.refKey, domain, ref)
}

func opaqueReference(key []byte, domain, ref string) string {
	if ref == "" {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(ref))
	return hex.EncodeToString(mac.Sum(nil))
}

// legacyCustomerReference reproduces the pre-audit 64-bit public hash only so
// an existing loyalty balance can be moved to its new HMAC identity when that
// customer next appears. It must never be used for new storage. The pre-audit
// loyalty domain was program-scoped: sha256("sorodeal-loyalty|{programID}|{ref}")
// (the "sorodeal-cust|" domain belonged to tally events, not loyalty).
func legacyCustomerReference(programID int64, ref string) string {
	if ref == "" {
		return ""
	}
	h := sha256.Sum256(fmt.Appendf(nil, "sorodeal-loyalty|%d|%s", programID, ref))
	return hex.EncodeToString(h[:8])
}

func (s *server) signReceipt(orgID int64, env string, campaignID uint64, code string, count int64, customerCommitment, orderCommitment string, timestamp int64, evidence receiptEvidence) (signedReceipt, error) {
	signer, err := s.receiptSigner(orgID, env)
	if err != nil {
		return signedReceipt{}, err
	}
	nonce, err := randHex(16)
	if err != nil {
		return signedReceipt{}, err
	}
	p := receiptPayload{
		Version:            2,
		Network:            cloudNetwork,
		ContractID:         currentContractID,
		CampaignID:         campaignID,
		Code:               code,
		Count:              count,
		CustomerCommitment: customerCommitment,
		OrderCommitment:    orderCommitment,
		EvidenceType:       evidence.Type,
		ContextHash:        evidence.ContextHash,
		PolicyVersion:      evidence.PolicyVersion,
		Timestamp:          timestamp,
		Nonce:              nonce,
		Signer:             signer.Address(),
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return signedReceipt{}, err
	}
	sig, err := signer.Sign(payload)
	if err != nil {
		return signedReceipt{}, err
	}
	return signedReceipt{
		Payload: payload, Leaf: sha256.Sum256(payload),
		Signature: base64.StdEncoding.EncodeToString(sig), Signer: signer.Address(),
	}, nil
}

type merkleStep struct {
	Position string `json:"position"` // left|right
	Hash     string `json:"hash"`
}

func merkleProof(leaves [][32]byte, index int) []merkleStep {
	return merkleProofFromLevels(merkleLevels(leaves), index)
}

func merkleProofFromLevels(levels [][][32]byte, index int) []merkleStep {
	if len(levels) == 0 || index < 0 || index >= len(levels[0]) {
		return nil
	}
	proof := []merkleStep{}
	for levelIndex := 0; levelIndex < len(levels)-1; levelIndex++ {
		level := levels[levelIndex]
		if index%2 == 1 {
			proof = append(proof, merkleStep{Position: "left", Hash: hexRoot(level[index-1])})
		} else if index+1 < len(level) {
			proof = append(proof, merkleStep{Position: "right", Hash: hexRoot(level[index+1])})
		}
		index /= 2
	}
	return proof
}

func decodeHash(value string) ([32]byte, error) {
	var out [32]byte
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != 32 {
		if err == nil {
			err = &hashLengthError{got: len(b)}
		}
		return out, err
	}
	copy(out[:], b)
	return out, nil
}

type hashLengthError struct{ got int }

func (e *hashLengthError) Error() string { return "hash must be 32 bytes, got " + strconv.Itoa(e.got) }

// isLowerHex reports whether s contains only lowercase hex characters.
func isLowerHex(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
