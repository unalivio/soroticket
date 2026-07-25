# soroticket-go

Go SDK for the **Soroticket** coupon protocol on Stellar Soroban — the **Burn**
profile (unique single-use codes) and the **Tally** profile (shared codes +
settlement).

> `TestnetContractID` defaults to the reviewed v0.2.0 testnet deployment
> (2026-07-12; see `deployments/testnet-v0.2.0.json`). `LegacyTestnetContractID`
> is the deprecated v0.1 contract — its partial-attribution settlement flaw
> cannot be patched. Testnet preview: never point real value at either.

Unlike the prototype's `stellar` CLI shell-out, this client **signs in-process**
with the Go Stellar SDK and talks to Soroban RPC directly: it simulates,
assembles (footprint + resource fee + auth), signs, submits with retries, and
re-submits the *same* signed envelope on transient failures. This protects the
SDK's internal submission retry; an application retry must still reuse its own
business/idempotency key, especially for campaign creation.

```bash
go get github.com/soroticket/soroticket-go
```

## Read (no signer)

```go
c, _ := soroticket.New(soroticket.Config{}) // legacy v0.1 testnet compatibility
defer c.Close()

camp, _ := c.GetCampaign(ctx, 1)
fmt.Println(camp.Name, camp.Minted, camp.Burned)

ok, _ := c.IsValid(ctx, 1, "DEMO0001")
```

## Write (keypair)

```go
kp := keypair.MustParseFull("S...")          // or keypair.Random()
c, _ := soroticket.Testnet(kp)

// create a campaign owned by the signer
id, _ := c.CreateCampaign(ctx, "Cafe", "percentage", 1500, 100, validUntil)
c.IssueUnique(ctx, id, []string{"SAVE0001", "SAVE0002"})

// randomized commitment; store nonce only if later proof is required
ref, nonce, _ := soroticket.RedeemerCommitment("order-8842")
_ = nonce
receipt, _ := c.RedeemUnique(ctx, id, "SAVE0001", ref)
```

Each `Client` is bound to one signing identity; build several `Client`s for
several actors (owner, delegate, …). Reads need no signer.

## Tally (shared codes + settlement)

```go
// v0.2 example only: reviewedV2ID must name a real deployed candidate.
owner, _ := soroticket.New(soroticket.Config{ContractID: reviewedV2ID, Signer: kp})
id, _ := owner.CreateCampaign(ctx, "Creator promo", "percentage", 1000, 100, validUntil)

rate := big.NewInt(1000) // token base-units per attributed redemption
creator := "G..."; token := soroticket.TestnetNativeSAC // any SAC
owner.RegisterShared(ctx, id, "FALL25", &creator, &token, rate)        // immutable token+rate
owner.CommitTally(ctx, id, "FALL25", 1, 40, merkleRoot, map[string]uint32{creator: 40})
payouts, _ := owner.ComputePayouts(ctx, id, "FALL25", 1)               // preview (no transfer)

latest, _ := owner.LatestLedger(ctx)
owner.ApproveSettlement(ctx, token, big.NewInt(40_000), latest+10_000)  // exact allowance
payouts, _ = owner.Settle(ctx, id, "FALL25", 1)                        // owner can trigger

// A different signed client may now pay the fee and trigger the same flow:
// payouts, _ = keeper.SettleFor(ctx, owner.Address(), id, "FALL25", 1)
```

The off-chain signed-receipt / Merkle layer is the integrator's responsibility —
see `docs/SPEC.md` §5 (receipt profile and trust model).

## Errors

Contract traps surface as `*ContractError` carrying the contract's numeric code:

```go
_, err := c.RedeemUnique(ctx, id, "USED", ref)
if code, ok := soroticket.CodeOf(err); ok && code == soroticket.ErrAlreadyRedeemed {
    // code.String() == "AlreadyRedeemed"
}
```

Codes `1`–`19` are exported (`ErrCampaignNotFound … ErrAttributionMismatch`) and
match the contract `Error` enum / `abi-v0.2.0.txt`. Most are raised at simulation
(before any submission), so an expected-failure call costs no fee.

## Notes

- Token amounts and `u64`/`i128` fields are `uint64` / `*big.Int` — never lossy.
- `TestnetContractID` is the reviewed v0.2.0 testnet deployment (2026-07-12;
  see `deployments/testnet-v0.2.0.json`). `LegacyTestnetContractID` is the
  deprecated v0.1 deployment — do not use it for new integrations.
- A `Client` is for sequential use; sequence numbers are loaded per call.

The E2E apps under `tests/e2e/go` follow the SDK's `TestnetContractID` default,
so they exercise the v0.2.0 testnet deployment.
