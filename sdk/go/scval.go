package soroticket

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// ═══════════════════════════════════════════════════════════════════
// ScVal encoders — build the typed arguments the contract expects.
// (Mirrors contracts/coupon-ledger/src/lib.rs parameter types.)
// ═══════════════════════════════════════════════════════════════════

func scU64(v uint64) xdr.ScVal {
	u := xdr.Uint64(v)
	return xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &u}
}

func scU32(v uint32) xdr.ScVal {
	u := xdr.Uint32(v)
	return xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &u}
}

func scBool(b bool) xdr.ScVal {
	return xdr.ScVal{Type: xdr.ScValTypeScvBool, B: &b}
}

func scString(s string) xdr.ScVal {
	v := xdr.ScString(s)
	return xdr.ScVal{Type: xdr.ScValTypeScvString, Str: &v}
}

func scBytes(b []byte) xdr.ScVal {
	v := xdr.ScBytes(b)
	return xdr.ScVal{Type: xdr.ScValTypeScvBytes, Bytes: &v}
}

func scVoid() xdr.ScVal {
	return xdr.ScVal{Type: xdr.ScValTypeScvVoid}
}

// ScVal.Vec / ScVal.Map are double-pointers (**ScVec / **ScMap).
func scVec(items []xdr.ScVal) xdr.ScVal {
	v := xdr.ScVec(items)
	vp := &v
	return xdr.ScVal{Type: xdr.ScValTypeScvVec, Vec: &vp}
}

func scMap(entries []xdr.ScMapEntry) xdr.ScVal {
	m := xdr.ScMap(entries)
	mp := &m
	return xdr.ScVal{Type: xdr.ScValTypeScvMap, Map: &mp}
}

var (
	two128  = new(big.Int).Lsh(big.NewInt(1), 128)
	minI128 = new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), 127))
	maxI128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 127), big.NewInt(1))
	maskU64 = new(big.Int).SetUint64(^uint64(0))
)

// scI128 encodes a (possibly negative) big.Int as a 128-bit two's-complement ScVal.
// Values outside the ABI range are rejected instead of silently wrapping.
func scI128(v *big.Int) (xdr.ScVal, error) {
	if v == nil || v.Cmp(minI128) < 0 || v.Cmp(maxI128) > 0 {
		return xdr.ScVal{}, fmt.Errorf("i128 value out of range")
	}
	n := new(big.Int).Mod(v, two128) // Euclidean mod ⇒ two's complement for negatives
	lo := new(big.Int).And(n, maskU64).Uint64()
	hi := new(big.Int).Rsh(n, 64).Uint64()
	p := xdr.Int128Parts{Hi: xdr.Int64(int64(hi)), Lo: xdr.Uint64(lo)}
	return xdr.ScVal{Type: xdr.ScValTypeScvI128, I128: &p}, nil
}

// scAddress encodes a Stellar account ("G…") or contract ("C…") address.
func scAddress(addr string) (xdr.ScVal, error) {
	sa, err := parseScAddress(addr)
	if err != nil {
		return xdr.ScVal{}, err
	}
	return xdr.ScVal{Type: xdr.ScValTypeScvAddress, Address: &sa}, nil
}

// scOptAddress encodes Option<Address>: Some(addr) ⇒ the address, None ⇒ Void.
func scOptAddress(addr *string) (xdr.ScVal, error) {
	if addr == nil {
		return scVoid(), nil
	}
	return scAddress(*addr)
}

func parseScAddress(addr string) (xdr.ScAddress, error) {
	if len(addr) > 0 && addr[0] == 'C' {
		decoded, err := strkey.Decode(strkey.VersionByteContract, addr)
		if err != nil {
			return xdr.ScAddress{}, err
		}
		var cid xdr.ContractId
		copy(cid[:], decoded)
		return xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &cid}, nil
	}
	aid, err := xdr.AddressToAccountId(addr)
	if err != nil {
		return xdr.ScAddress{}, err
	}
	return xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeAccount, AccountId: &aid}, nil
}

// scAttributionMap encodes Map<Address, u32>, sorted by the key's XDR encoding
// (Soroban requires ScMap keys in ascending canonical order).
func scAttributionMap(perAttribution map[string]uint32) (xdr.ScVal, error) {
	type kv struct {
		key xdr.ScVal
		enc string
		val uint32
	}
	pairs := make([]kv, 0, len(perAttribution))
	for addr, n := range perAttribution {
		k, err := scAddress(addr)
		if err != nil {
			return xdr.ScVal{}, err
		}
		enc, err := xdr.MarshalBase64(k)
		if err != nil {
			return xdr.ScVal{}, err
		}
		pairs = append(pairs, kv{key: k, enc: enc, val: n})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].enc < pairs[j].enc })
	entries := make([]xdr.ScMapEntry, 0, len(pairs))
	for _, p := range pairs {
		entries = append(entries, xdr.ScMapEntry{Key: p.key, Val: scU32(p.val)})
	}
	return scMap(entries), nil
}

// ═══════════════════════════════════════════════════════════════════
// ScVal decoders — turn return values into native Go values.
// ═══════════════════════════════════════════════════════════════════

func u128ToBig(p xdr.UInt128Parts) *big.Int {
	res := new(big.Int).SetUint64(uint64(p.Hi))
	res.Lsh(res, 64)
	return res.Add(res, new(big.Int).SetUint64(uint64(p.Lo)))
}

func i128ToBig(p xdr.Int128Parts) *big.Int {
	res := big.NewInt(int64(p.Hi))
	res.Lsh(res, 64)
	return res.Add(res, new(big.Int).SetUint64(uint64(p.Lo)))
}

func addrToString(a xdr.ScAddress) (string, error) {
	switch a.Type {
	case xdr.ScAddressTypeScAddressTypeAccount:
		return a.AccountId.Address(), nil
	case xdr.ScAddressTypeScAddressTypeContract:
		return strkey.Encode(strkey.VersionByteContract, a.ContractId[:])
	}
	return "", fmt.Errorf("unsupported address type %v", a.Type)
}

// native converts an ScVal into a native Go value:
//
//	void→nil, bool→bool, u32→uint32, i32→int32, u64/timepoint/duration→uint64,
//	i64→int64, u128/i128→*big.Int, string/symbol→string, bytes→[]byte,
//	address→string, vec→[]interface{}, map→map[string]interface{}.
func native(v xdr.ScVal) (interface{}, error) {
	switch v.Type {
	case xdr.ScValTypeScvVoid:
		return nil, nil
	case xdr.ScValTypeScvBool:
		return *v.B, nil
	case xdr.ScValTypeScvU32:
		return uint32(*v.U32), nil
	case xdr.ScValTypeScvI32:
		return int32(*v.I32), nil
	case xdr.ScValTypeScvU64:
		return uint64(*v.U64), nil
	case xdr.ScValTypeScvI64:
		return int64(*v.I64), nil
	case xdr.ScValTypeScvTimepoint:
		return uint64(*v.Timepoint), nil
	case xdr.ScValTypeScvDuration:
		return uint64(*v.Duration), nil
	case xdr.ScValTypeScvU128:
		return u128ToBig(*v.U128), nil
	case xdr.ScValTypeScvI128:
		return i128ToBig(*v.I128), nil
	case xdr.ScValTypeScvString:
		return string(*v.Str), nil
	case xdr.ScValTypeScvSymbol:
		return string(*v.Sym), nil
	case xdr.ScValTypeScvBytes:
		return []byte(*v.Bytes), nil
	case xdr.ScValTypeScvAddress:
		return addrToString(*v.Address)
	case xdr.ScValTypeScvVec:
		out := []interface{}{}
		if v.Vec != nil && *v.Vec != nil {
			for _, e := range **v.Vec {
				n, err := native(e)
				if err != nil {
					return nil, err
				}
				out = append(out, n)
			}
		}
		return out, nil
	case xdr.ScValTypeScvMap:
		out := map[string]interface{}{}
		if v.Map != nil && *v.Map != nil {
			for _, e := range **v.Map {
				k, err := native(e.Key)
				if err != nil {
					return nil, err
				}
				val, err := native(e.Val)
				if err != nil {
					return nil, err
				}
				out[fmt.Sprint(k)] = val
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported scval type %v", v.Type)
}

// nativeMap decodes a struct-shaped ScVal (ScvMap with symbol field keys).
func nativeMap(v xdr.ScVal) (map[string]interface{}, error) {
	n, err := native(v)
	if err != nil {
		return nil, err
	}
	m, ok := n.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected a struct/map scval, got %T", n)
	}
	return m, nil
}

// ─── small typed field accessors over the decoded map ────────────────

func asU64(m map[string]interface{}, k string) uint64 {
	switch x := m[k].(type) {
	case uint64:
		return x
	case uint32:
		return uint64(x)
	}
	return 0
}

func asU32(m map[string]interface{}, k string) uint32 {
	switch x := m[k].(type) {
	case uint32:
		return x
	case uint64:
		return uint32(x)
	}
	return 0
}

func asString(m map[string]interface{}, k string) string {
	if s, ok := m[k].(string); ok {
		return s
	}
	return ""
}

func asBool(m map[string]interface{}, k string) bool {
	if b, ok := m[k].(bool); ok {
		return b
	}
	return false
}

func asBytes(m map[string]interface{}, k string) []byte {
	if b, ok := m[k].([]byte); ok {
		return b
	}
	return nil
}

func asBig(m map[string]interface{}, k string) *big.Int {
	if b, ok := m[k].(*big.Int); ok {
		return b
	}
	return big.NewInt(0)
}

// asOptString returns nil for a None (decoded as nil), else the address string.
func asOptString(m map[string]interface{}, k string) *string {
	if s, ok := m[k].(string); ok {
		return &s
	}
	return nil
}
