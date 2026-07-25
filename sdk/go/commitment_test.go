package soroticket

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestRedeemerCommitmentsAreUnlinkable(t *testing.T) {
	first, firstNonce, err := RedeemerCommitment("order-8842")
	if err != nil {
		t.Fatal(err)
	}
	second, secondNonce, err := RedeemerCommitment("order-8842")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || bytes.Equal(firstNonce[:], secondNonce[:]) {
		t.Fatal("randomized commitments must not be linkable")
	}
}

func TestRedeemerCommitmentMatchesTypeScriptEncoding(t *testing.T) {
	var nonce [16]byte
	for i := range nonce {
		nonce[i] = byte(i)
	}
	got, err := RedeemerCommitmentWithNonce("order-8842", nonce)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256("000102030405060708090a0b0c0d0e0f|order-8842")
	const want = "b0861d099c557b835475af928af2a826fb340e831fa48a94685dcb29dcffd481"
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("commitment=%x, want %s", got, want)
	}
}

func TestHMACRedeemerCommitmentRequiresStrongPepper(t *testing.T) {
	if _, err := HMACRedeemerCommitment("customer", []byte("short")); err == nil {
		t.Fatal("expected short pepper to be rejected")
	}
	pepper := bytes.Repeat([]byte{7}, 32)
	a, err := HMACRedeemerCommitment("customer", pepper)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := HMACRedeemerCommitment("customer", pepper)
	if a != b {
		t.Fatal("HMAC commitments must be stable for the same merchant pepper")
	}
}
