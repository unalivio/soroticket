# Architecture Decision Records — Sorodeal

Short ADRs capturing the "why." Newest decisions append at the bottom.

---

## ADR-001 — Sorodeal is a standalone protocol, not a BotCore feature
**Context:** A working coupon system was prototyped inside BotCore (a WhatsApp sales-bot platform). The valuable part is the on-chain redemption primitive, not the WhatsApp/Twilio/SaaS scaffolding around it.
**Decision:** Extract it into its own repo as an open protocol. BotCore is the *donor* of code only.
**Consequences:** Drop multi-tenant SaaS, white-label, and WhatsApp concerns. The home of the design is this repo's `docs/`, not BotCore.

## ADR-002 — Permissionless / ownership-based, no global admin
**Context:** The donor contract uses a single global `ADMIN` (`require_admin`) that must sign every campaign, mint, and redeem. That is a single-operator model and cannot back an open standard.
**Decision:** Campaigns are owned by their creator's `Address`. Authorization is by campaign owner (and explicit delegates), enforced with `require_auth`. Anyone can create campaigns from their own keypair.
**Consequences:** A real contract redesign, not cosmetic. Enables the "each account independent, no central supervision" model the protocol is meant to have.

## ADR-003 — Two redemption profiles: Burn (sync) and Tally (async)
**Context:** Unique tickets and shared promo codes have fundamentally different volume/value/real-time needs. Forcing both through synchronous on-chain burns is wrong for shared codes (cost/latency at scale; no double-spend to prevent).
**Decision:** Standardize two profiles. **Burn** = unique tokens, 1 tx/redeem, synchronous. **Tally** = shared codes, off-chain hot path + periodic on-chain commitment of counts + attribution (Merkle-anchored).
**Consequences:** The spec must define both. The pilot may start synchronous-only, but Tally must exist in the spec for the standard to be credible at delivery/UGC scale.

## ADR-004 — "Why Stellar" is attribution + settlement, not double-spend
**Context:** The donor pitch leans on tamper-proof / anti-double-spend. That is strong for tickets but weak for shared codes (which are meant to be reused). It is also the argument SCF reviewers most easily attack.
**Decision:** Position the flagship value as **trustless attribution + automatic USDC payout per conversion**, viable because Stellar fees are sub-cent and USDC is native.
**Consequences:** Marketing, SEP, and SCF framing all lead with attribution/settlement. Double-spend is mentioned only for the Burn/ticket profile.

## ADR-005 — Hybrid on-chain / off-chain boundary
**Context:** Privacy (PII), latency (checkout/POS UX), and cost forbid putting everything on-chain.
**Decision:** On-chain = campaign terms, supply commitments, redemption *commitments* (hashed/Merkle), attribution counts, settlement events — the source of truth for *proof and counts*. Off-chain = the fast path, plaintext PII, rich metadata, periodically anchored.
**Consequences:** Redeemer identity on-chain is always a commitment/hash, never plaintext PII. The donor contract's plaintext `burned_by` is a bug to fix.

## ADR-006 — Name: Sorodeal
**Context:** Needed a name for the protocol/repo. Candidates floated: Tessera, Canje, Sello, Stub.
**Decision:** **Sorodeal** (Soroban + deal).
**Consequences:** Repo, package, and SEP naming follow. Still pursue a candidate SEP for "standard" legitimacy regardless of brand name.

## ADR-007 — Authorization granularity: owner-only issuance, owner-or-delegate redemption
**Context:** ADR-002 makes campaigns owner-authorized "with explicit delegates" but does not say which operations a delegate may perform. Issuing codes creates value and changes the supply cap; redemption is the routine act done "at the door" (POS staff, many devices).
**Decision:** In the reference contract, `create_campaign`, `issue_unique`, and delegate management (`add_delegate`/`remove_delegate`) require the campaign **owner**. `redeem_unique` accepts the **owner or any registered delegate** (per-campaign). The donor's global `ADMIN`/`initialize`/`require_admin` are removed entirely — there is no privileged singleton. Redemptions carry only an opaque `redeemer_ref_hash` commitment (ADR-005/010); the on-chain anchor is the ledger sequence, not a global counter.
**Consequences:** Operators can be granted and revoked without sharing the owner key, while supply creation stays owner-gated. This realizes ADR-002/005 for the Burn profile. The async Tally profile (ADR-003) and USDC settlement (ADR-004) are not yet in the contract — they are the next milestone.

## ADR-008 — On-chain owner index (enumerable campaigns), not client-side storage
**Context:** Soroban storage is not iterable, so there was no way to answer "which campaigns does this account own?" The reference web client first cached created campaigns in `localStorage` — fragile, per-device, and not authoritative.
**Decision:** The contract maintains an `owner → Vec<campaign_id>` index, updated on `create_campaign` and exposed by a public `campaigns_of(owner) -> Vec<u64>`. Clients enumerate an owner's campaigns directly from the chain (then `get_campaign` for details); `localStorage` is removed.
**Consequences:** The chain is the single source of truth for ownership — a returning client rebuilds its view from `campaigns_of`. Cost: a growing `Vec` per owner (fine for the reference implementation; a production variant may page/shard at very high campaign counts). Contract `create_campaign` events remain a complementary index but are not relied on (testnet retains events only ~7 days).

## ADR-009 — Per-campaign code scope, term validation, TTL maintenance (audit hardening)
**Context:** A security audit flagged three things in the Burn contract: a *global* code index lets anyone squat human-readable codes ("WELCOME", "10OFF") across the whole contract; `create_campaign`/`issue_unique` lacked structural validation; and a fixed ~155-day storage TTL can expire before a long campaign's `valid_until`.
**Decision:** (a) **Scope codes per campaign** — `CodeIndex(campaign_id, code)`; `verify`/`is_valid`/`redeem_unique` take `campaign_id` (a QR/link carries it). The same string may exist in different campaigns. (b) **Validate structural terms** on-chain: `total_supply > 0`, `valid_until` in the future, name 1..=96 bytes, discount type 1..=32 bytes, code ≤ 64 bytes, batch 1..=100 (no empty batch), and no issuing after expiry — but reward semantics (`discount_type`/`value`) stay **opaque** (a standard must not constrain them). (c) Add **`bump_campaign(campaign_id)`** (metadata + owner index) and **`bump_codes(campaign_id, codes)`** (specific coupons' Token + code-index entries) so anyone can re-extend storage TTL for long-running promotions — Soroban storage is not iterable, so the caller supplies which coupons to bump; entries beyond the network max TTL inherently need periodic rent.
**Consequences:** No cross-campaign squatting, and Burn codes can be human-readable once namespaced. Verify/redeem now need the campaign id — acceptable, it is public and travels with the code. Idle tokens past the TTL window remain restorable (`bump_campaign` / RestoreFootprint). New errors: `InvalidTerms`, `BatchTooLarge`, `CodeTooLong`.

## ADR-010 — Redeemer privacy: opaque commitment, not a public-salt hash
**Context:** The reference web client committed the redeemer reference as `SHA-256("sorodeal:v1:" + contractId_prefix ∥ ref)` — a **public, constant salt**. For low-entropy identifiers (email, phone, order#) that is brute-forceable, and being deterministic it is linkable; `verify` returns the hash publicly. The "PII-free" claim did not hold.
**Decision:** On-chain we store an **opaque, non-reversible commitment**: `SHA-256(random per-redemption nonce ∥ ref)` (nonce kept off-chain), or `HMAC(merchant pepper, ref)` in production. The client uses a random nonce; UI/docs describe it accurately (a commitment, not "private by salt").
**Consequences:** On-chain data is public by definition — privacy now comes from the commitment being non-reversible and unlinkable, not from a salt. Proving a specific identity later needs the off-chain nonce/pepper. The contract is unchanged (it stores 32 opaque bytes); this is a client/spec correctness fix.

## ADR-011 — Tally profile implemented: shared codes, attribution commitments, settlement
**Context:** The contract implemented only the Burn profile (unique single-use codes). The flagship use cases — shared promo codes and creator/UGC codes with trustless attribution and automatic payout (ADR-003/004) — need the async Tally profile.
**Decision:** Add Tally to the same contract, **additively** (a campaign may use either profile; shared codes live in a namespace separate from Burn's unique codes). Surface: `register_shared(owner, campaign_id, code, attributed_to: Option<Address>)`; `commit_tally(owner, campaign_id, code, period, count, merkle_root, per_attribution: Map<Address,u32>)` — append-only per (code, period), with per-attribution ≤ count; `get_shared`/`get_tally` (public reads); `compute_payouts(campaign_id, code, period, rate)` (read-only preview); `settle(owner, campaign_id, code, period, token, rate)` — pays each attributed address `count*rate` of `token` from the owner's balance via the SAC token client, once per period.
**Consequences:** On-chain holds only periodic commitments (count + Merkle root of the epoch's off-chain signed receipts + per-attribution counts), so anyone can audit a claimed count and creators can verify their own number without trusting the operator (ADR-004). Settlement is a **real on-chain token transfer** (USDC SAC on mainnet; any SAC/test token on testnet) — the owner must hold the token and authorize. The off-chain receipt format, epoch cadence, and who pays the commit fee remain integration choices (SPEC §6). New errors: `SharedNotFound`, `AlreadyRegistered`, `PeriodCommitted`, `TallyNotFound`, `AlreadySettled`, `InvalidTally`.

> **Updated by ADR-012/013:** `compute_payouts`/`settle` dropped their `token`/`rate` arguments (read from the shared code now), and extra Tally invariants were added. The signatures in this ADR are superseded — see ADR-012/013 and `docs/SPEC.md` §5.

## ADR-012 — Tally settlement hardening (audit): binding attribution, immutable token/rate
**Context:** A review found the first Tally cut was exploitable: `commit_tally`/`settle` took an arbitrary `per_attribution` map and an arbitrary `token`/`rate` at settle time. So an owner could register a code for creator A yet pay B; settle with `rate = 0` or a wrong token, permanently **locking** the period as settled; and `count * rate` could overflow.
**Decision:** (a) **Binding attribution** — if a shared code has a single registered `attributed_to`, `commit_tally` may only credit that address (`AttributionMismatch` otherwise). (b) **Immutable settlement config** — `payout_token` + `payout_rate` are fixed at `register_shared` (rate must be > 0 when a token is set, else `InvalidSettlement`); `settle`/`compute_payouts` read them on-chain and take no token/rate argument. (c) **Safe math** — `checked_mul` for amounts (`InvalidSettlement` on overflow). `settle` emits token+rate in its event.
**Consequences:** The on-chain attribution target and payout terms are now trustworthy — a creator can rely on the registered config, and a period can't be locked by a wrong settlement. New errors: `InvalidSettlement`, `AttributionMismatch`. The pinned toolchain also adds clippy/rustfmt; the web adds `.nvmrc` (Node 22).

## ADR-013 — Tally edge-case hardening (audit): config invariants, expiry, TTL
**Context:** A further review found four Tally edges: (1) a count-only code (no payout token) could still set a positive rate, producing payouts `settle` could never pay; (2) an unattributed code (`attributed_to = None`) could still commit per-attribution to arbitrary addresses; (3) `register_shared`/`commit_tally` ignored the campaign's `valid_until`; (4) no TTL bump existed for Tally storage.
**Decision:** (1) Settlement config is consistent — `payout_token` set ⇔ `payout_rate > 0`; no token ⇒ rate 0 (`InvalidSettlement`). (2) An unattributed code may not set a payout token and its tallies must carry no per-attribution (`AttributionMismatch`); an attributed code still credits only its registered address. (3) `register_shared` rejects an expired campaign (`CampaignExpired`); `commit_tally` is intentionally still allowed post-expiry, since tallies are committed retrospectively after an epoch closes. (4) Added `bump_tally(campaign_id, code, periods)` to re-extend a shared code + its periods' tally/settlement entries.
**Consequences:** Tally state is internally consistent and auditable long-term; the two modes are clean — *attributed + payable* or *count-only / unattributed*. Reuses existing errors (`InvalidSettlement`, `AttributionMismatch`, `CampaignExpired`). 30 unit tests; `clippy -D warnings` and `fmt --check` clean.
**Trust-model note:** allowing `commit_tally` post-expiry means the chain does not (and cannot) attest that the underlying redemptions occurred before `valid_until` — that is a *receipt-level* guarantee (signed, Merkle-anchored, off-chain), not a contract-enforced one. This boundary is spelled out in SPEC §4 and must be stated in any pitch/SEP rather than implied by "trustless."

---

### TODO / unresolved
- Verify the deployed prototype's mainnet contract address — the donor pitch cited two inconsistent addresses; do not assert either as fact until checked on-chain.
- Choose a license (Apache-2.0 vs MIT).
- Decide SEP scope: core primitive as the SEP, settlement as a companion proposal?
