# Sorodeal Protocol Spec (draft)

> Status: **draft / design target.** The reference contract in `contracts/coupon-ledger/` now implements **both** the permissionless **Burn** and async **Tally** profiles of this spec (ADR-002/005/011), live on Stellar testnet — including per-attribution tally commitments and token settlement.

## 1. Overview

Sorodeal defines how to **issue**, **redeem**, **attribute**, and **settle** redeemable codes ("deals") on Stellar Soroban. A deal can be a one-time unique token (a ticket) or a shared multi-use code (a promo). Redemptions can be credited to a creator/referrer and trigger automatic payout.

The protocol is **permissionless**: any account can create a campaign and issue codes from its own keypair. There is no privileged operator.

## 2. Core primitive

Four nouns:

```
Campaign ──< Code ──< Redemption ──> Settlement
```

- **Campaign** — the terms of a promotion: owner, reward, validity window, supply/cap, redemption profile.
- **Code** — a redeemable string issued under a campaign, with an issuance shape and a redemption policy. May carry an attribution target (a creator/referrer Address).
- **Redemption** — an event consuming a code, producing a receipt.
- **Settlement** — optional value transfer triggered by redemptions (e.g., pay the attributed creator in USDC).

### The three policy axes

Every code is described by:

1. **Cardinality** — `unique` (1 code = 1 redemption) or `shared` (1 code = N redemptions, up to a cap).
2. **Attribution** — none, or an `Address` credited for each redemption (creator, affiliate, referrer).
3. **Per-redeemer limits** — once-per-user, geofence, time window, optional KYC.

### Use-case mapping

| Use case | Cardinality | Attribution | Per-user | Profile |
|---|---|---|---|---|
| Delivery promo `SUPERBOWL10` | shared | none | once/user | Tally |
| Creator code `ROBERTOX` | shared | creator | once/user | Tally + payout |
| Referral (P2P) | shared (1/user) | referrer | anti-abuse | Tally + payout |
| Event ticket | unique | optional | transferable? | Burn |
| Geo-drop | unique/capped | optional | geofence + window | Burn |

## 3. Redemption profiles

The cardinality determines the on-chain interaction pattern.

### 3.1 Burn (synchronous)

For **unique** codes. Each redemption is an on-chain transaction that marks the token burned. The contract enforces single-use (`AlreadyRedeemed`) and supply caps at the protocol level. Real-time, low-volume, high-value-per-item.

- **Why on-chain, honestly:** double-use prevention at the point of redemption + supply integrity (cannot oversell seats). These are genuine, real-time guarantees.
- The reference contract (`contracts/coupon-ledger/`) implements this path, permissionless and PII-free: `create_campaign` → `issue_unique` → `redeem_unique(campaign_id, code, redeemer_ref_hash)` → `verify(campaign_id, code)`. Each campaign is owned by its creator; the owner or an explicit delegate authorizes redemptions (ADR-007).

### 3.2 Tally (asynchronous)

For **shared** codes at scale (delivery, UGC). Putting every redemption on-chain synchronously is wrong here — too slow/expensive, and there is no double-spend to prevent (shared codes are meant to be reused).

Instead:

1. Redemptions happen off-chain on the integrator's hot path (fast, private). Each produces a **signed receipt** `{code, redeemer_ref_hash, attributed_to, ts, nonce}`.
2. Periodically (per epoch), the campaign owner commits on-chain: `commit_tally(campaign_id, code, period, count, merkle_root, per_attribution_counts)`.
3. Anyone can audit a claimed count against the committed `merkle_root` by checking inclusion of the underlying receipts. **The tally is verifiable without trusting the operator** — this is what makes attribution trustless.

- **Why on-chain, honestly:** trustless redemption counts + attribution, and a settlement trigger. NOT double-spend.
- **Implemented** (ADR-011): `register_shared` → `commit_tally(count, merkle_root, per_attribution)` → `get_tally`/`compute_payouts` → `settle` (token payout to attributed addresses).

## 4. Attribution & settlement

- A code may set `attributed_to: Address`. Tally commitments include per-attribution counts.
- **Settlement** (optional module): given committed per-attribution counts and a payout rate, disburse USDC to attributed creators/referrers — SDP-style. Economical only because Stellar fees are sub-cent, so paying fractions of a cent per conversion is viable.
- **Trust model & on-chain/off-chain boundary (Tally).** The chain stores a *commitment* — counts, a Merkle root, per-attribution, and immutable token/rate — not a proof that each underlying redemption is genuine or occurred within the campaign window.
  - *What the contract enforces:* a single registered creator binds attribution; token + rate are fixed at registration; commitments are append-only per period; settlement pays only the registered address in the registered token; and a code cannot be registered under an expired campaign.
  - *What it cannot attest by itself:* that the off-chain redemptions are real and happened before `valid_until`. That comes from the **signed receipts** Merkle-anchored by `commit_tally` — anyone can audit a claimed count by checking receipt inclusion against the root, and a creator can independently verify their own number; disputes resolve against the committed leaves.
  - Because `commit_tally` is intentionally allowed *after* expiry (epochs are tallied retrospectively — ADR-013), the **temporal validity** of redemptions is a *receipt-level* guarantee asserted by the integrator's signing key, not by the contract. Integrators therefore MUST publish their receipt format and signing key and SHOULD timestamp receipts, so the audit is meaningful. State this boundary explicitly in any pitch.

## 5. Proposed contract interface (permissionless)

> Sketch — replaces the donor's global-admin model. `owner` authenticates via `require_auth`; campaigns are owned by their creator, not a global admin.

```
// Campaigns — anyone can create from their own account
create_campaign(owner: Address, terms: CampaignTerms) -> u64           // require_auth(owner)
get_campaign(campaign_id: u64) -> Campaign
campaign_stats(campaign_id: u64) -> CampaignStats
campaigns_of(owner: Address) -> Vec<u64>                               // public — enumerate an owner's campaigns on-chain (ADR-008)

// Burn profile (unique) — codes are scoped per campaign (ADR-009)
issue_unique(owner, campaign_id, codes: Vec<String>) -> Vec<u64>       // require_auth(owner) — owner only; validates terms/batch
redeem_unique(authorizer, campaign_id, code, redeemer_ref_hash) -> Receipt  // require_auth; owner or delegate; single-use
verify(campaign_id, code) -> Token                                     // public, no auth
bump_campaign(campaign_id)                                             // public — extend a campaign's storage TTL (ADR-009)
bump_codes(campaign_id, codes: Vec<String>)                            // public — extend specific coupons' storage TTL (ADR-009)

// Delegation (Burn) — owner grants/revokes operator redemption rights (ADR-007)
add_delegate(owner, campaign_id, delegate: Address)                    // require_auth(owner)
remove_delegate(owner, campaign_id, delegate: Address)                 // require_auth(owner)
is_delegate(campaign_id, who: Address) -> bool                         // public, no auth

// Tally profile (shared) — implemented (ADR-011)
register_shared(owner, campaign_id, code, attributed_to: Option<Address>,
                payout_token: Option<Address>, payout_rate: i128)      // require_auth(owner); token+rate fixed & immutable here
get_shared(campaign_id, code) -> SharedCode                            // public, no auth
commit_tally(owner, campaign_id, code, period, count, merkle_root,
             per_attribution: Map<Address,u32>)                        // require_auth(owner); append-only; attribution must match the registered creator
get_tally(campaign_id, code, period) -> TallyCommitment                // public, no auth

// Settlement (implemented) — token + rate come from the shared code (immutable; not caller-supplied)
compute_payouts(campaign_id, code, period) -> Vec<Payout>              // public, read-only preview
settle(owner, campaign_id, code, period) -> Vec<Payout>                // require_auth(owner); pays attributed_to in payout_token, once per period
bump_tally(campaign_id, code, periods: Vec<u64>)                       // public — extend a shared code + its periods' storage TTL (ADR-013)
```

### Data structures (target)

- `CampaignTerms { name, reward_type, reward_value, profile, supply_or_cap, valid_from, valid_until }`
- `Code { campaign_id, code, cardinality, policy, attributed_to }`
- `Receipt { code, redeemer_ref_hash, attributed_to, ts, tx_or_nonce }`
- `TallyCommitment { period, count, merkle_root, per_attribution: Map<Address,u32> }`

**Privacy:** redeemer identity is only ever an **opaque, non-reversible commitment** on-chain — `SHA-256(random nonce ∥ ref)` or `HMAC(merchant pepper, ref)`, never a public-salt hash of a low-entropy identifier (which is brute-forceable). No plaintext PII (ADR-005/010).

## 6. Open questions

- Geofence enforcement is inherently off-chain/oracle — the *policy* is declared on-chain, but GPS cannot be trusted on-chain. How much to standardize?
- Transferable tickets (bearer) vs bound-to-redeemer — separate extension?
- Loyalty / stamp-cards (accumulation, not redemption) — out of core, roadmap extension.
- Epoch length and who pays the `commit_tally` fee in the permissionless model.
- Exact SEP scope: core (Campaign/Code/Redemption) as the SEP, settlement as a companion?
