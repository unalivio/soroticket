package sorodeal

import (
	"math/big"
	"testing"
)

func TestScI128RejectsWrapping(t *testing.T) {
	tests := []struct {
		name string
		v    *big.Int
		ok   bool
	}{
		{"zero", big.NewInt(0), true},
		{"max", new(big.Int).Set(maxI128), true},
		{"min", new(big.Int).Set(minI128), true},
		{"above max", new(big.Int).Add(new(big.Int).Set(maxI128), big.NewInt(1)), false},
		{"below min", new(big.Int).Sub(new(big.Int).Set(minI128), big.NewInt(1)), false},
		{"nil", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := scI128(tt.v)
			if (err == nil) != tt.ok {
				t.Fatalf("scI128(%v) error = %v, want ok=%v", tt.v, err, tt.ok)
			}
			if !tt.ok {
				return
			}
			decoded, err := native(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.(*big.Int).Cmp(tt.v) != 0 {
				t.Fatalf("round-trip = %v, want %v", decoded, tt.v)
			}
		})
	}
}
