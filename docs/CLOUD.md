# Sorodeal Cloud — hosted platform spec (v1 draft)

The self-service layer on top of the Sorodeal protocol: a **REST API + web
console** for teams that don't want to run the SDKs or touch a wallet. The
platform custodies a Stellar account per organization, pays the network fees
and storage rent on their behalf, and meters usage against a **prepaid credits
balance** (Apify-style). The protocol stays permissionless and free — this is
the convenience layer.

> Status: design spec. This document is the source of truth for the console
> design and the backend build. Final decisions become ADRs in DECISIONS.md.

---

## 1. Architecture

```
┌────────────┐     ┌───────────────────────────────┐     ┌──────────────────┐
│  Console   │────▶│  Cloud API (Go)               │────▶│ Soroban testnet/  │
│ (web SPA)  │     │  · auth: sessions + API keys  │     │ mainnet contract  │
└────────────┘     │  · tenancy + custodial keys   │     └──────────────────┘
┌────────────┐     │  · credits ledger + metering  │
│ Integrator │────▶│  · TTL keep-alive cron        │
│ (REST)     │     │  · webhooks + idempotency     │
└────────────┘     │  · uses sdk/go (sorodeal-go)  │
                   └───────────────────────────────┘
```

- **Backend: Go**, consuming `sorodeal-go` (dogfooding the SDK).
- **One custodial Stellar keypair per organization**, encrypted at rest (KMS).
  All the org's campaigns are owned on-chain by that address, so everything the
  platform does for a tenant is publicly auditable, and a future **key export /
  non-custodial migration** is possible (the org takes its address with it).
- **Two environments, Stripe-style:** every org gets *test mode* (testnet,
  free forever, `sk_test_…` keys) and *live mode* (mainnet, metered,
  `sk_live_…` keys). One toggle in the console switches everything.
- The platform pays all network fees + storage rent; that cost is what the
  credit metering recovers.

## 2. Tenancy & auth

| Concept | Shape |
|---|---|
| **Organization** | The tenant. Has: name, custodial Stellar account (per env), credits balance, plan. |
| **User** | Email + password (magic-link later). Belongs to one org v1; teams later. |
| **API key** | `sk_test_…` / `sk_live_…`, hashed at rest, label, last-used, revocable. Multiple per org. |
| **Publishable key** | `pk_…` — allowed only on public reads (`verify`) for client-side widgets. Later. |

API auth: `Authorization: Bearer sk_live_…`. Console auth: session cookie.
All POSTs accept an **`Idempotency-Key` header** (stored 24h; same key ⇒ same
response — no double-issues, no double-redeems on retries).

## 3. Credits, metering, tiers

**Credits are a prepaid balance.** 1,000 credits = $1 (PLACEHOLDER — calibrate
against real testnet/mainnet cost measurements before launch). Every metered
operation debits the ledger atomically with the operation itself.

**Price table (PLACEHOLDER values — the shape is what matters):**

| Operation | Credits | Why |
|---|---|---|
| Reads (`verify`, stats, lists) | 0 | Simulations are ~free for us; keep the API friction-less. |
| Create campaign | 20 | On-chain write + storage. |
| Issue unique codes | 2 / code | Batched writes; per-code storage. |
| Redeem (burn) | 5 | On-chain write. |
| Register shared code | 10 | On-chain write. |
| Record shared-code event (off-chain) | 0.2 | DB + later anchoring amortized. |
| Commit tally / settle | 15 / 25 | On-chain write; settle moves tokens. |
| Loyalty punch | 0.2 | Off-chain, anchored periodically. |
| **Campaign keep-alive** | 30 / campaign / month | TTL rent (`bump_*`) — the real recurring on-chain cost. |

**Tiers:**

| Tier | v1 | Future |
|---|---|---|
| **Free** | Monthly grant (e.g. 25,000 credits ≈ $25 PLACEHOLDER), non-accumulating. Test mode unlimited. Generous enough for a real integrator pilot. | Stays; grant tuned by real costs. |
| **Pay-as-you-go** | Exists in the model from day one (recharge works), even if we top everyone up manually during the free period. | Card (Stripe) + **USDC on Stellar** recharges (on-brand: pay the platform through the same rail it settles on). |
| **Pro / Scale** | — | Included-credit bundles, volume discounts, SLA, teams, custom limits. |

**Design principle: build the meter now, price later.** The ledger, price
table, and recharge flow all exist in v1; the free period is just a monthly
grant on top of the same machinery.

Ledger entry: `{ts, api_key, operation, campaign_id?, credits_delta, balance_after, tx_hash?}` — the
console renders this as both a usage dashboard and an auditable statement.

## 4. REST API v1

Base: `https://api.sorodeal.org/v1` (PLACEHOLDER domain). JSON. Errors are
`application/problem+json` and **pass through the contract error verbatim**:
`{status, code: 3, name: "AlreadyRedeemed", message, tx_hash?}` — same 19 codes
as the protocol.

### Campaigns
| Method | Path | Notes |
|---|---|---|
| POST | `/campaigns` | `{kind: coupon\|voucher\|ticket\|loyalty, name, discount_type, discount_value, total_supply, valid_until}`. `kind` is presentation-level (the contract sees one primitive). |
| GET | `/campaigns` · `/campaigns/{id}` | Includes on-chain stats + `stellar_expert_url`. |
| GET | `/campaigns/{id}/stats` | minted / burned / available / expired. |

### Unique codes (Burn — coupons, vouchers, tickets)
| POST | `/campaigns/{id}/codes` | `{codes: […]}` or `{generate: {count, prefix?}}`. Auto-chunks (contract max 100/batch). Returns codes + QR payloads. |
| GET | `/campaigns/{id}/codes` | With status filter. |
| GET | `/verify?campaign_id&code` | **Public read.** Token status; never leaks unissued codes. |
| POST | `/redemptions` | `{campaign_id, code, redeemer_ref?}`. The platform computes the opaque commitment (random nonce ∥ ref → SHA-256; ADR-005/010) — **no PII on-chain, ever**. Returns the receipt: `{token_id, discount, burned_at, ledger_seq, tx_hash}`. |
| GET | `/redemptions?campaign_id` | Timeline. |

### Shared codes (Tally — general + creator/attributed)
| POST | `/campaigns/{id}/shared-codes` | `{code, attributed_to?, payout?: {token, rate}}` — payout config immutable (ADR-012). |
| POST | `/shared-codes/{cid}/{code}/events` | Off-chain redemption event(s): `{count?, customer_ref?, order_ref?}`. This is the hot path (0-ish cost). |
| POST | `/shared-codes/{cid}/{code}/commits` | `{period}` — platform computes count + Merkle root from recorded events and commits on-chain. Auto-commit schedule configurable (e.g. weekly). |
| GET | `/shared-codes/{cid}/{code}/tallies` | Committed periods, auditable. |
| POST | `/settlements` | `{campaign_id, code, period}` → pays the attributed address `count × rate` from the org's custodial account. |

### Loyalty (built on the same primitive)
A **program** = earn side (Tally-anchored punches) + reward side (Burn voucher).

| POST | `/loyalty/programs` | `{name, threshold, reward: {discount_type, discount_value, validity_days}}`. Creates the earn anchor (shared code) + the reward campaign (Burn). |
| POST | `/loyalty/programs/{id}/punches` | `{customer_ref, count?}`. `customer_ref` is merchant-side and stored only as an opaque hash (same PII rule). |
| GET | `/loyalty/programs/{id}/customers` | Punch counts, progress, rewards earned. |
| GET | `/loyalty/programs/{id}` | Program stats: punches, rewards issued/redeemed. |

When a customer crosses `threshold`, the platform **auto-issues a unique
voucher** on the reward campaign and fires `loyalty.reward_issued`. The voucher
then lives the normal Burn life (verify / redeem / receipt). Punch history is
anchored on-chain per period (count + Merkle root) so a program's totals are
auditable; per-customer balances stay off-chain (customers aren't Stellar
addresses). A fully-trustless per-customer profile is a candidate **contract
v0.2 / SEP extension**.

### Platform
| GET | `/usage?group_by=day\|operation\|key` | Metering rollups for charts. |
| GET | `/credits` | `{balance, monthly_grant, ledger: […]}`. |
| POST | `/credits/recharges` | `{amount_usd, method: card\|usdc}` → card = Stripe checkout; usdc = payment address + memo, credited on confirmation. |
| POST/GET | `/webhooks` | Endpoints + secret; events HMAC-signed. Events: `redemption.created`, `tally.committed`, `settlement.paid`, `loyalty.reward_issued`, `credits.low`. |

Rate limits per key (headers `X-RateLimit-*`). Cursor pagination everywhere.

## 5. Ops invariants (carried from the protocol work)

- **Idempotent submission** end-to-end: API `Idempotency-Key` at the edge +
  same-envelope re-submission at the Soroban layer (sdk/go) — a retry can never
  double-burn or double-charge credits.
- **TTL keep-alive cron**: bumps every live campaign/code/tally inside its
  window; the keep-alive meter line recovers this cost. A campaign the org
  archives stops being bumped (and stops charging).
- **Custody**: org keypairs in KMS, never in app memory longer than a signing
  call; distribution/fee float account separate from tenant accounts; all
  tenant actions auditable on-chain under the org's address.
- Mainnet deploys use a dedicated deployer identity — never a personal one.

## 6. Build order

1. **Spec freeze** (this doc) → console design (Claude Design) in parallel.
2. API skeleton: orgs/auth/keys + campaigns/codes/redemptions on **testnet**.
3. Metering ledger + usage endpoints (free tier = monthly grant).
4. Tally + settlement endpoints; webhooks.
5. Loyalty programs.
6. Console build against the real API.
7. Mainnet enablement + recharges (Stripe + USDC).
