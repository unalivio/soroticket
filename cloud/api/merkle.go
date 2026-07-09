package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// merkleRoot computes a SHA-256 Merkle root over the given leaves (an odd node
// promotes). Empty input yields the zero root. Leaves here are the recorded
// off-chain events, so anyone holding the event set can recompute and audit
// the committed root (docs/SPEC.md §10 trust model).
func merkleRoot(leaves [][32]byte) [32]byte {
	if len(leaves) == 0 {
		return [32]byte{}
	}
	level := leaves
	for len(level) > 1 {
		next := make([][32]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				continue
			}
			h := sha256.Sum256(append(level[i][:], level[i+1][:]...))
			next = append(next, h)
		}
		level = next
	}
	return level[0]
}

func eventLeaf(id int64, code string, count int64, ts int64) [32]byte {
	return sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%d", id, code, count, ts)))
}

func hexRoot(r [32]byte) string { return hex.EncodeToString(r[:]) }
