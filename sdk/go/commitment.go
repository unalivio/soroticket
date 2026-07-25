package soroticket

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// RedeemerCommitment creates an unlinkable SHA-256 commitment for a redeemer
// reference. Keep the returned nonce off-chain if the reference must later be
// proven; only Hash is passed to RedeemUnique.
func RedeemerCommitment(ref string) (hash [32]byte, nonce [16]byte, err error) {
	if ref == "" {
		return hash, nonce, errors.New("redeemer reference cannot be empty")
	}
	if _, err = rand.Read(nonce[:]); err != nil {
		return hash, nonce, err
	}
	hash, err = RedeemerCommitmentWithNonce(ref, nonce)
	return hash, nonce, err
}

// RedeemerCommitmentWithNonce reproduces a commitment from its retained nonce.
// The encoding matches the TypeScript SDK and Cloud:
// SHA-256(lowercase_hex(nonce) || "|" || ref).
func RedeemerCommitmentWithNonce(ref string, nonce [16]byte) ([32]byte, error) {
	var hash [32]byte
	if ref == "" {
		return hash, errors.New("redeemer reference cannot be empty")
	}
	hash = sha256.Sum256([]byte(hex.EncodeToString(nonce[:]) + "|" + ref))
	return hash, nil
}

// HMACRedeemerCommitment creates a deterministic merchant-scoped commitment.
// Use a random secret pepper of at least 32 bytes and never publish it. This is
// useful when the merchant needs stable lookup semantics without exposing PII.
func HMACRedeemerCommitment(ref string, pepper []byte) ([32]byte, error) {
	var out [32]byte
	if ref == "" {
		return out, errors.New("redeemer reference cannot be empty")
	}
	if len(pepper) < 32 {
		return out, errors.New("merchant pepper must contain at least 32 bytes")
	}
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte("soroticket/redeemer/v1"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(ref))
	copy(out[:], mac.Sum(nil))
	return out, nil
}
