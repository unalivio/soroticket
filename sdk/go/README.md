# sorodeal-go

Go SDK for the **Sorodeal** coupon protocol on Stellar Soroban — the **Burn**
profile (unique single-use codes) and the **Tally** profile (shared codes +
settlement).

Unlike the prototype's `stellar` CLI shell-out, this client **signs in-process**
with the Go Stellar SDK and talks to Soroban RPC directly: it simulates,
assembles (footprint + resource fee + auth), signs, submits with retries, and
re-submits the *same* signed envelope on transient failures — so a network retry
can never double-burn (CLAUDE.md production gaps #2 and #4).

```bash
go get github.com/sorodeal/sorodeal-go
```

## Read (no signer)

```go
c, _ := sorodeal.New(sorodeal.Config{}) // testnet defaults
defer c.Close()

camp, _ := c.GetCampaign(ctx, 1)
fmt.Println(camp.Name, camp.Minted, camp.Burned)

ok, _ := c.IsValid(ctx, 1, "DEMO0001")
```

## Write (keypair)

```go
kp := keypair.MustParseFull("S...")          // or keypair.Random()
c, _ := sorodeal.Testnet(kp)

// create a campaign owned by the signer
id, _ := c.CreateCampaign(ctx, "Cafe", "percentage", 1500, 100, validUntil)
c.IssueUnique(ctx, id, []string{"SAVE0001", "SAVE0002"})

// redeem with an opaque off-chain commitment (no PII on-chain — ADR-005/010)
ref := sha256.Sum256([]byte("order-8842"))
receipt, _ := c.RedeemUnique(ctx, id, "SAVE0001", ref)
```

Each `Client` is bound to one signing identity; build several `Client`s for
several actors (owner, delegate, …). Reads need no signer.

## Tally (shared codes + settlement)

```go
rate := big.NewInt(1000) // token base-units per attributed redemption
creator := "G..."; token := sorodeal.TestnetNativeSAC // any SAC
c.RegisterShared(ctx, id, "FALL25", &creator, &token, rate)        // immutable token+rate
c.CommitTally(ctx, id, "FALL25", 1, 40, merkleRoot, map[string]uint32{creator: 30})
payouts, _ := c.ComputePayouts(ctx, id, "FALL25", 1)               // preview (no transfer)
payouts, _ = c.Settle(ctx, id, "FALL25", 1)                        // pays the creator
```

The off-chain signed-receipt / Merkle layer is the integrator's responsibility —
see `docs/SPEC.md` §10 (trust model).

## Errors

Contract traps surface as `*ContractError` carrying the contract's numeric code:

```go
_, err := c.RedeemUnique(ctx, id, "USED", ref)
if code, ok := sorodeal.CodeOf(err); ok && code == sorodeal.ErrAlreadyRedeemed {
    // code.String() == "AlreadyRedeemed"
}
```

Codes `1`–`19` are exported (`ErrCampaignNotFound … ErrAttributionMismatch`) and
match the contract `Error` enum / `abi-v0.1.0.txt`. Most are raised at simulation
(before any submission), so an expected-failure call costs no fee.

## Notes

- Token amounts and `u64`/`i128` fields are `uint64` / `*big.Int` — never lossy.
- Defaults target testnet (`TestnetContractID`/`TestnetRPC`/`TestnetPassphrase`);
  set `Config.ContractID`/`RPCURL`/`NetworkPassphrase` for another deployment.
- A `Client` is for sequential use; sequence numbers are loaded per call.

See `tests/e2e/go` for a consumer app exercising every method and error against
the live testnet contract.
