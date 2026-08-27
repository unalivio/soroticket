# Soroticket

**An open protocol and hosted suite for coupons, vouchers, tickets and loyalty on Stellar Soroban — with signed audit receipts and token settlement.**

Soroticket combines a permissionless contract, Go/TypeScript SDKs, a developer playground and a hosted Cloud API. Burn redemptions are recorded individually on-chain; high-volume shared-code events stay off-chain as signed receipts and are periodically anchored by a Merkle root. Attributed tallies may pay a configured Stellar asset.

> **Status:** contract **v0.2.0 is deployed to testnet** (2026-07-12) at
> `CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF` — the SDKs, Cloud
> and playground all point at it; see `deployments/testnet-v0.2.0.json` for the
> upload/deploy transaction hashes. The earlier v0.1 testnet deployment is
> **deprecated** (it permitted creator underpayment and required an owner
> signature to settle) and is immutable, so it stays readable but unused.
>
> This is a **testnet preview under active development**, not production and not
> mainnet: Cloud TEST is free and METERED exercises the credit ledger, but both
> run on testnet and neither moves real value. The 2026-07-11 review was internal
> and assisted — an independent audit is still an open release gate. Findings,
> residual risks and the release criteria are in
> `docs/SECURITY_AUDIT_2026-07-11.md`; the remaining gates are in `docs/ROADMAP.md`.

> **Formerly Sorodeal.** The project started under that name and was renamed on
> 2026-07-25, once tickets and proof-of-delivery — not discount "deals" — turned
> out to be the driving use cases (ADR-006 in `docs/DECISIONS.md`). A few
> identifiers deliberately keep the old string: the `sorodeal-loyalty` hash
> domain in `cloud/api/merkle.go` reproduces commitments already written to disk
> and must never change, and `docs/DECISIONS.md` / `docs/SECURITY_AUDIT_2026-07-11.md`
> quote it where they record history. A local checkout may also still sit in a
> folder named `sorodeal`; nothing in the code depends on the directory name.

## Why this exists

Coupons today are either paper (forgeable, unauditable) or locked inside closed SaaS platforms. There is no open standard where:

- a merchant can issue a deal that **anyone** can verify,
- a creator/referrer can verify that published signed receipts are included in an immutable tally commitment,
- and an approved token allowance can be settled without another owner signature on v0.2.

Stellar's low fees, asset model and Soroban make small-value settlement practical. For shared codes, the contract prevents commitment changes and attribution underpayment; it does **not** prove that an off-chain purchase was genuine. The receipt signer remains that trust anchor. See `docs/SPEC.md`.

## One primitive, not three

The product family shares a core primitive. This table distinguishes what is
implemented from policy extensions that still require design and code:

| Use case | Code → redemptions | Profile | Status |
|---|---|---|---|
| Delivery promo (`SUPERBOWL10`) | 1 → many | Tally | v0.2 testnet + Cloud preview |
| Creator / UGC code (`ROBERTOX`) | 1 → many, fixed attribution | Tally + payout | v0.2 testnet + Cloud preview |
| Gift / delivery proof (gifted bottles → scanned servings) | 1 per venue → many scans | Tally + optional payout | v0.2 testnet + Cloud preview; see `docs/USE_CASES.md` |
| Event ticket / unique voucher | many × (1 → 1) | Burn | v0.2 testnet + Cloud preview |
| Loyalty threshold → reward | shared punches + unique rewards | Tally + Burn | Cloud preview; customer ledger is off-chain |
| Referral with once-per-user rules | 1 per user → many | Tally + payout | Planned; anti-fraud/reversals absent |
| Geo-drop / proof-of-presence | unique or capped | Burn + geofence | Planned; geofence absent |

Spec nouns: **Campaign → Code → Redemption → Settlement.**

## Two redemption profiles

- **Burn (synchronous):** unique single-use tokens. One on-chain tx per redemption. Best for tickets and high-value vouchers — real-time double-use prevention "at the door." *(Implemented.)*
- **Tally (asynchronous):** shared multi-use codes. Hot path off-chain; periodic on-chain commitment of signed receipt hashes and exact attributed counts. Cheap at scale and tamper-evident after commitment. *(Implemented in v0.2 on testnet; Cloud publishes receipt proofs.)*

## Repo layout

A monorepo by layout, not by tooling: every unit below builds on its own (four Go
modules, six npm packages, one Rust crate) and there is no workspace file, no
Makefile and no CI. Nothing here is published as a package yet.

```
soroticket/
├── contracts/coupon-ledger/   # THE PROTOCOL — reference Soroban contract, Rust
│                              # Burn + Tally profiles (ADR-002/005/011). v0.2.0 on testnet.
├── sdk/
│   ├── go/                    # github.com/soroticket/soroticket-go — in-process signing over Soroban RPC
│   └── ts/                    # @soroticket/sdk — generated typed client + ergonomic + browser wrapper
│
├── cloud/                     # THE HOSTED PRODUCT — optional, never a protocol dependency (ADR-016)
│   ├── api/                   # Go REST API: custodial org accounts, credits, receipts, settlement
│   ├── console/               # merchant portal (React) — campaigns, scans, redemptions, settlements
│   └── scanner/               # WhatsApp webhook: QR scan → redemption, with location evidence
│
├── web/                       # developer playground (Vite/React + Freighter) — talks to the
│                              # contract directly via @soroticket/sdk, no Cloud involved
├── soroticket-index.html      # public landing; `npm run build` → dist/ → Cloudflare (wrangler.jsonc)
├── scripts/                   # landing dev server + build
├── assets/soroticket/         # brand assets and fonts for the landing
│
├── docs/                      # SPEC (design target), DECISIONS (ADRs), SECURITY_AUDIT, ROADMAP, SEP,
│                              # CLOUD, USE_CASES, PRODUCT_OPPORTUNITIES, LANDING_COPY_ES
├── deployments/               # on-chain deployment records with real transaction hashes
├── tests/e2e/                 # consumer tester apps (Go, TS, and the gift/proof-of-delivery demo)
└── reference/botcore-donor/   # archived prototype Go — reference only, does NOT build standalone
```

**Three separate web surfaces**, often confused for one:

| Surface | What it is | Who it serves | Talks to |
|---|---|---|---|
| `soroticket-index.html` | static multilingual landing | the public | nothing |
| `web/` | developer playground | integrators evaluating the protocol | the contract, directly |
| `cloud/console/` | merchant portal | customers running campaigns | `cloud/api` |

## License

[Apache-2.0](LICENSE) — a permissive license, to maximize ecosystem reuse as a public good. A candidate SEP draft lives in [`docs/SEP.md`](docs/SEP.md).
