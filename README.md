# Sorodeal

**An open protocol for tokenized coupons, vouchers, and redeemable codes on Stellar Soroban — with verifiable attribution and automatic settlement.**

Sorodeal is a public-good **standard** (not a product) for issuing and redeeming "deals" on-chain: classic promo codes, creator/affiliate codes, referral codes, and unique event tickets — all as one primitive, with on-chain proof of every redemption and optional USDC payout per conversion.

> **Status: early.** The reference Soroban contract was donated from a working WhatsApp prototype (BotCore) and is being reshaped from a single-admin design into a **permissionless standard**. Targeting the Stellar Community Fund (SCF) as a milestone-based grant, with a geolocated social network as the first design partner.

## Why this exists

Coupons today are either paper (forgeable, unauditable) or locked inside closed SaaS platforms. There is no open standard where:

- a merchant can issue a deal that **anyone** can verify,
- a creator/referrer can **prove** how many redemptions they drove without trusting the brand's dashboard,
- and they get **paid per conversion automatically** in USDC.

Stellar's sub-cent fees + native USDC + Soroban make per-conversion payouts economically viable for the first time. **That is the "why Stellar."** (Not "tamper-proof double-spend prevention" — that argument is weak for shared codes; see `docs/DECISIONS.md`.)

## One primitive, not three

The "different kinds of coupons" are a single primitive with policy knobs — cardinality (code→redemptions) × attribution × per-user limits:

| Use case | Code → redemptions | Attribution | Redemption profile |
|---|---|---|---|
| Delivery promo (`SUPERBOWL10`) | 1 → many | no | Tally (async) |
| Creator / UGC code (`ROBERTOX`) | 1 → many | yes (creator) | Tally + payout |
| Referral (P2P) | 1 per user → many | yes (referrer) | Tally + payout |
| Event ticket / unique voucher | many × (1 → 1) | optional | Burn (sync) |
| Geo-drop / proof-of-presence | unique or capped | optional | Burn (sync) + geofence |

Spec nouns: **Campaign → Code → Redemption → Settlement.**

## Two redemption profiles

- **Burn (synchronous):** unique single-use tokens. One on-chain tx per redemption. Best for tickets and high-value vouchers — real-time double-use prevention "at the door." *(This is what the donor contract already does.)*
- **Tally (asynchronous):** shared multi-use codes. Hot path off-chain; periodic on-chain commitment of redemption counts + attribution via Merkle-anchored receipts. Cheap at scale and the basis for trustless creator payouts. *(This is the new work.)*

## Repo layout

```
sorodeal/
├── README.md
├── CLAUDE.md                 # orientation for AI sessions — read this first
├── contracts/
│   └── coupon-ledger/        # donor Soroban contract (single-admin PROTOTYPE — to be redesigned permissionless)
│       ├── src/lib.rs
│       ├── Cargo.toml
│       └── test_snapshots/
├── docs/
│   ├── SPEC.md               # the protocol spec (the real design target)
│   ├── SCF.md                # Stellar Community Fund tranche/milestone plan
│   └── DECISIONS.md          # architecture decision records
└── reference/
    └── botcore-donor/        # reference-only Go from the prototype (does NOT build standalone)
        ├── stellar-adapter/  # how the contract was invoked (CLI shell-out — to be rewritten with the Go SDK)
        ├── domain/           # data model: code generation, QR payload, reference numbers
        └── port/             # interface sketches
```

## License

TBD — intended to be permissive (Apache-2.0 or MIT) to maximize ecosystem reuse as a public good.
