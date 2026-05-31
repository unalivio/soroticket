# SEP-XXXX — Tokenized Coupons, Vouchers & Redeemable Codes (Sorodeal)

> **Candidate / Draft.** This is a pre-submission draft of a Stellar Ecosystem
> Proposal. It synthesizes the reference implementation in this repository
> (`contracts/coupon-ledger/`, live on testnet) into a standard interface.
> Section numbers and the SEP number are placeholders until assigned.

## Preamble

```
SEP: XXXX (unassigned)
Title: Tokenized Coupons, Vouchers, and Redeemable Codes
Authors: Fabian Fariñas <fill-in contact>
Track: Standard
Status: Draft
Created: 2026-05-31
Version: 0.1.0
Discussion: <fill-in: GitHub discussion / Stellar Dev Discord thread>
Reference implementation: https://github.com/<org>/sorodeal (testnet contract
  CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX)
```

## Summary

This SEP standardizes how to **issue**, **redeem**, **attribute**, and
**settle** redeemable codes ("deals") on Stellar via a Soroban contract
interface. It covers one-time unique tokens (tickets, vouchers) and shared
multi-use codes (promo, creator/UGC, referral) as a single primitive with
policy knobs, and defines an optional automatic USDC settlement keyed to
trustless, on-chain attribution counts.

## Motivation

Redeemable codes today are either paper (forgeable, unauditable) or locked
inside closed SaaS platforms. There is no open, interoperable standard where:

- a merchant can issue a deal that **anyone** can verify on-chain;
- a creator/referrer can **prove** how many redemptions they drove without
  trusting the brand's dashboard; and
- they get **paid per conversion automatically** in a native asset (USDC).

Stellar's sub-cent fees + native USDC + Soroban make per-conversion payouts of
fractions of a cent economically viable for the first time. The flagship value
is **trustless attribution + automatic settlement** — not anti-double-spend
(which is only meaningful for the unique-token case). A shared standard lets
wallets, point-of-sale tools, indexers, and disbursement services interoperate
across issuers instead of re-implementing a closed system each time.

## Abstract

A **Campaign** (owned by a creator `Address`, permissionless — no global admin)
issues **Codes**. Codes are described by three policy axes — **cardinality**
(unique vs shared), **attribution** (none, or a credited `Address`), and
**per-redeemer limits**. Two redemption **profiles** follow from cardinality:

- **Burn (synchronous)** — unique single-use tokens; one on-chain transaction
  per redemption; the contract enforces single-use and supply integrity.
- **Tally (asynchronous)** — shared multi-use codes; redemptions happen
  off-chain on a fast path, and the owner periodically commits an on-chain
  **TallyCommitment** (count + Merkle root of signed receipts + per-attribution
  counts). An optional **settlement** pays attributed addresses a configured
  token at a configured rate.

## Specification

> Types and signatures below mirror the reference Soroban contract. A code is
> identified by `(campaign_id, code)`; codes are namespaced per campaign (no
> global namespace to squat).

### 1. Terminology

- **Campaign** — terms of a promotion: owner, reward metadata, supply/cap,
  validity window, per its codes' profile.
- **Code** — a redeemable string under a campaign; unique (Burn) or shared
  (Tally); may carry an attribution target.
- **Redemption** — an event consuming a code; on-chain (Burn) or off-chain with
  periodic commitment (Tally).
- **Settlement** — optional value transfer to attributed addresses.

### 2. Roles & authorization

- **Permissionless.** Any account creates a campaign for itself; `owner`
  authorizes via `require_auth`. There is no privileged operator.
- **Owner** — created the campaign; authorizes issuance, shared-code
  registration, tally commitment, delegate management, and settlement.
- **Delegate** — an explicit per-campaign `Address` the owner authorizes to
  **redeem** unique codes (e.g. POS staff); delegates may not issue or manage.
- **Public** — `verify`, `is_valid`, `get_*`, `compute_payouts`, and the
  `bump_*` rent operations require no ownership.

### 3. Data structures

```
Campaign { id: u64, owner: Address, name: String, discount_type: String,
           discount_value: u64, total_supply: u32, minted: u32, burned: u32,
           valid_until: u64 }

CouponToken { token_id: u64, campaign_id: u64, code: String, is_burned: bool,
              minted_at: u64, redeemer_ref: BytesN<32>, burned_at: u64 }

RedemptionReceipt { token_id, code, campaign_id, campaign_name, discount_type,
                    discount_value, redeemer_ref: BytesN<32>, burned_at,
                    ledger_seq: u32 }

SharedCode { campaign_id: u64, code: String, attributed_to: Option<Address>,
             payout_token: Option<Address>, payout_rate: i128, registered_at: u64 }

TallyCommitment { period: u64, count: u32, merkle_root: BytesN<32>,
                  per_attribution: Map<Address, u32> }

Payout { to: Address, amount: i128 }
```

`discount_type`/`discount_value` are **opaque reward metadata** — the standard
does not constrain their meaning (percentage, fixed, free-item, etc. are an
integrator convention).

### 4. Burn profile (unique codes)

```
create_campaign(owner, name, discount_type, discount_value, total_supply, valid_until) -> u64
                                                              // require_auth(owner); validates terms
get_campaign(campaign_id) -> Campaign                         // public
campaign_stats(campaign_id) -> CampaignStats                  // public
campaigns_of(owner) -> Vec<u64>                               // public; enumerate an owner's campaigns
issue_unique(owner, campaign_id, codes: Vec<String>) -> Vec<u64>   // require_auth(owner); owner only
redeem_unique(authorizer, campaign_id, code, redeemer_ref_hash: BytesN<32>) -> RedemptionReceipt
                                                              // require_auth; owner or delegate; single-use
verify(campaign_id, code) -> CouponToken                      // public
is_valid(campaign_id, code) -> bool                           // public
add_delegate(owner, campaign_id, delegate)                    // require_auth(owner)
remove_delegate(owner, campaign_id, delegate)                 // require_auth(owner)
is_delegate(campaign_id, who) -> bool                         // public
bump_campaign(campaign_id)                                    // public; extend campaign storage TTL
bump_codes(campaign_id, codes: Vec<String>)                   // public; extend specific coupons' TTL
```

Burn guarantees, enforced on-chain: **single-use** (`AlreadyRedeemed`),
**supply integrity** (cannot oversell), validity-window and per-campaign
uniqueness. These are genuine real-time guarantees at the point of redemption.

### 5. Tally profile (shared codes)

```
register_shared(owner, campaign_id, code, attributed_to: Option<Address>,
                payout_token: Option<Address>, payout_rate: i128)
                  // require_auth(owner); rejects expired campaign; settlement config fixed & immutable
get_shared(campaign_id, code) -> SharedCode                   // public
commit_tally(owner, campaign_id, code, period, count, merkle_root, per_attribution: Map<Address,u32>)
                  // require_auth(owner); append-only per (code,period); attribution binding
get_tally(campaign_id, code, period) -> TallyCommitment       // public
bump_tally(campaign_id, code, periods: Vec<u64>)              // public; extend shared/tally/settled TTL
```

Invariants (rejected otherwise):

- **Immutable settlement config:** `payout_token` set ⇔ `payout_rate > 0`;
  no token ⇒ rate 0. Token + rate are fixed at registration; they are not
  caller-supplied at settle time.
- **Binding attribution:** a code with a registered `attributed_to` may credit
  only that address; an unattributed code (`attributed_to = None`) may not set
  a payout token and its tallies must carry no per-attribution.
- **Append-only:** one commitment per `(code, period)`; per-attribution counts
  must not exceed `count`.

### 6. Settlement

```
compute_payouts(campaign_id, code, period) -> Vec<Payout>     // public, read-only preview
settle(owner, campaign_id, code, period) -> Vec<Payout>       // require_auth(owner); pays once per period
```

`settle` transfers, from the owner's balance via the token's SAC client,
`count × payout_rate` of `payout_token` to each attributed address, then marks
the `(code, period)` settled (idempotent). `compute_payouts` previews the same
amounts read-only. Amounts use `i128`; integrators MUST treat token amounts as
`i128`/decimal strings off-chain (not IEEE-754) to avoid precision loss.

### 7. Errors

```
1 CampaignNotFound   6 Unauthorized      11 CodeTooLong        16 AlreadySettled
2 CouponNotFound     7 InvalidCode       12 SharedNotFound     17 InvalidTally
3 AlreadyRedeemed    8 DuplicateCode     13 AlreadyRegistered  18 InvalidSettlement
4 CampaignExpired    9 InvalidTerms      14 PeriodCommitted    19 AttributionMismatch
5 SupplyExhausted   10 BatchTooLarge     15 TallyNotFound
```

### 8. Events

- `(campaign, create) → (id, owner, name, total_supply)`
- `(coupon, issue) → (token_id, campaign_id)` · `(coupon, burn) → (token_id, ledger_seq)`
- `(delegate, add|remove) → (campaign_id, delegate)`
- `(shared, reg) → (campaign_id, attributed_to)`
- `(tally, commit) → (campaign_id, period, count)` · `(tally, settle) → (campaign_id, period, token, rate)`

Raw codes and redeemer references are never published in events.

### 9. Privacy

A redeemer's identity is only ever an **opaque, non-reversible commitment**
on-chain: `redeemer_ref_hash = H(nonce ∥ ref)` with a random per-redemption
nonce, or `HMAC(merchant_pepper, ref)` in production. A public/constant salt is
NOT acceptable, as low-entropy identifiers (email, phone, order#) would be
brute-forceable. No plaintext PII is written on-chain.

### 10. Trust model & on-chain/off-chain boundary

The chain stores **commitments**, not proofs of off-chain facts.

- *Enforced on-chain:* permissionless ownership and delegate authorization,
  single-use + supply for Burn, per-campaign code uniqueness, binding
  attribution, immutable settlement config, append-only tally periods, and
  no shared-code registration under an expired campaign.
- *NOT attested by the contract:* that off-chain redemptions are genuine or
  occurred before `valid_until`. `commit_tally` is intentionally allowed after
  expiry (epochs are tallied retrospectively), so temporal validity and
  authenticity are **receipt-level** guarantees: signed receipts Merkle-anchored
  by `commit_tally`, auditable by anyone via inclusion proofs against
  `merkle_root`. Integrators MUST publish their receipt format and signing key
  and SHOULD timestamp receipts.

## Design Rationale

Captured as ADRs in `docs/DECISIONS.md`: permissionless/no-admin (ADR-002),
two profiles (ADR-003), "why Stellar = attribution+settlement" (ADR-004),
hybrid on/off-chain + PII-as-commitment (ADR-005/010), delegate granularity
(ADR-007), on-chain owner index (ADR-008), per-campaign code scope + term
validation + TTL ops (ADR-009), Tally + settlement (ADR-011), settlement
hardening — binding attribution & immutable config (ADR-012), and Tally
edge invariants (ADR-013).

## Security Concerns

- On-chain data is public; privacy comes from non-reversible commitments, not
  from hiding (§9).
- Settlement transfers real value; `payout_token`/`payout_rate` are immutable
  from registration and `settle` is idempotent per period to prevent a wrong or
  repeated payout (§5–6).
- Soroban state archival: entries beyond the network max TTL require periodic
  `bump_*`; long-lived campaigns must budget rent.
- The off-chain receipt layer is the basis of attribution trust; a compromised
  signing key undermines audits (§10) — key management is the integrator's
  responsibility.

## Reference Implementation

`contracts/coupon-ledger/` (Rust + Soroban SDK), deployed to Stellar testnet at
`CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX`; an interactive
developer playground in `web/`; reproducible builds via a pinned toolchain.

## Changelog

- 0.1.0 (2026-05-31): initial draft from the reference implementation.
