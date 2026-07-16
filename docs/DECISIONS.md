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
**Context:** The donor pitch leans on tamper-proof / anti-double-spend. That is strong for tickets but weak for shared codes (which are meant to be reused). It is also the argument technical reviewers most easily attack.
**Decision:** Position the flagship value as **tamper-evident attribution commitments + token settlement**, while stating that the receipt signer remains the trust anchor for genuine off-chain events.
**Consequences:** Marketing and SEP framing must distinguish immutable commitments from proof of a real-world sale. Double-spend is mentioned only for the Burn/ticket profile.

## ADR-005 — Hybrid on-chain / off-chain boundary
**Context:** Privacy (PII), latency (checkout/POS UX), and cost forbid putting everything on-chain.
**Decision:** On-chain = campaign terms, supply commitments, redemption *commitments* (hashed/Merkle), attribution counts, settlement events — the source of truth for what was committed. Off-chain = the fast path, plaintext PII, rich metadata and signed receipts, periodically anchored.
**Consequences:** Redeemer identity on-chain is always a commitment/hash, never plaintext PII. The donor contract's plaintext `burned_by` is a bug to fix.

## ADR-006 — Name: Sorodeal
**Context:** Needed a name for the protocol/repo. Candidates floated: Tessera, Canje, Sello, Stub.
**Decision:** **Sorodeal** (Soroban + deal).
**Consequences:** Repo, package, and SEP naming follow. Still pursue a candidate SEP for "standard" legitimacy regardless of brand name.

## ADR-007 — Authorization granularity: owner-only issuance, owner-or-delegate redemption
**Context:** ADR-002 makes campaigns owner-authorized "with explicit delegates" but does not say which operations a delegate may perform. Issuing codes creates value and changes the supply cap; redemption is the routine act done "at the door" (POS staff, many devices).
**Decision:** In the reference contract, `create_campaign`, `issue_unique`, and delegate management (`add_delegate`/`remove_delegate`) require the campaign **owner**. `redeem_unique` accepts the **owner or any registered delegate** (per-campaign). The donor's global `ADMIN`/`initialize`/`require_admin` are removed entirely — there is no privileged singleton. Redemptions carry only an opaque `redeemer_ref_hash` commitment (ADR-005/010); the on-chain anchor is the ledger sequence, not a global counter.
**Consequences:** Operators can be granted and revoked without sharing the owner key, while supply creation stays owner-gated. This realizes ADR-002/005 for the Burn profile. At the time of this decision Tally was the next milestone; ADR-011 records its later implementation.

## ADR-008 — On-chain owner index (enumerable campaigns), not client-side storage
**Context:** Soroban storage is not iterable, so there was no way to answer "which campaigns does this account own?" The reference web client first cached created campaigns in `localStorage` — fragile, per-device, and not authoritative.
**Decision:** The contract maintains an `owner → Vec<campaign_id>` index, updated on `create_campaign` and exposed by a public `campaigns_of(owner) -> Vec<u64>`. Clients enumerate an owner's campaigns directly from the chain (then `get_campaign` for details); `localStorage` is removed.
**Consequences:** The chain is the single source of truth for ownership — a returning client rebuilds its view from `campaigns_of`. Cost: a growing `Vec` per owner (fine for the reference implementation; a production variant may page/shard at very high campaign counts). Contract `create_campaign` events remain a complementary index but are not relied on (testnet retains events only ~7 days).

> **Superseded by ADR-015:** candidate v0.2 replaces the growing owner `Vec`
> with O(1) indexed slots and adds bounded `campaigns_page`; `campaigns_of`
> remains only as a compatibility read.

## ADR-009 — Per-campaign code scope, term validation, TTL maintenance (audit hardening)
**Context:** A security audit flagged three things in the Burn contract: a *global* code index lets anyone squat human-readable codes ("WELCOME", "10OFF") across the whole contract; `create_campaign`/`issue_unique` lacked structural validation; and a fixed ~155-day storage TTL can expire before a long campaign's `valid_until`.
**Decision:** (a) **Scope codes per campaign** — `CodeIndex(campaign_id, code)`; `verify`/`is_valid`/`redeem_unique` take `campaign_id` (a QR/link carries it). The same string may exist in different campaigns. (b) **Validate structural terms** on-chain: `total_supply > 0`, `valid_until` in the future, name 1..=96 bytes, discount type 1..=32 bytes, code ≤ 64 bytes, batch 1..=100 (no empty batch), and no issuing after expiry — but reward semantics (`discount_type`/`value`) stay **opaque** (a standard must not constrain them). (c) Add **`bump_campaign(campaign_id)`** (metadata + owner index) and **`bump_codes(campaign_id, codes)`** (specific coupons' Token + code-index entries) so anyone can re-extend storage TTL for long-running promotions — Soroban storage is not iterable, so the caller supplies which coupons to bump; entries beyond the network max TTL inherently need periodic rent.
**Consequences:** No cross-campaign squatting, and Burn codes can be human-readable once namespaced. Verify/redeem now need the campaign id — acceptable, it is public and travels with the code. Idle tokens past the TTL window remain restorable (`bump_campaign` / RestoreFootprint). New errors: `InvalidTerms`, `BatchTooLarge`, `CodeTooLong`.

## ADR-010 — Redeemer privacy: opaque commitment, not a public-salt hash
**Context:** The reference web client committed the redeemer reference as `SHA-256("sorodeal:v1:" + contractId_prefix ∥ ref)` — a **public, constant salt**. For low-entropy identifiers (email, phone, order#) that is brute-forceable, and being deterministic it is linkable; `verify` returns the hash publicly. The "PII-free" claim did not hold.
**Decision:** On-chain we store an **opaque, non-reversible commitment**: `SHA-256(random per-redemption nonce ∥ ref)` (nonce kept off-chain), or `HMAC(merchant pepper, ref)` in production. The client uses a random nonce; UI/docs describe it accurately (a commitment, not "private by salt").
**Consequences:** On-chain data is public by definition — privacy now comes from the commitment being non-reversible and unlinkable, not from a salt. Proving a specific identity later needs the off-chain nonce/pepper. The contract is unchanged (it stores 32 opaque bytes); this is a client/spec correctness fix.

## ADR-011 — Tally profile implemented: shared codes, attribution commitments, settlement
**Context:** The contract implemented only the Burn profile (unique single-use codes). Shared promo and creator/referral codes need an async Tally profile with published audit receipts and token settlement (ADR-003/004).
**Decision:** Add Tally to the same contract, **additively** (a campaign may use either profile; shared codes live in a namespace separate from Burn's unique codes). Surface: `register_shared(owner, campaign_id, code, attributed_to: Option<Address>)`; `commit_tally(owner, campaign_id, code, period, count, merkle_root, per_attribution: Map<Address,u32>)` — append-only per (code, period), with per-attribution ≤ count; `get_shared`/`get_tally` (public reads); `compute_payouts(campaign_id, code, period, rate)` (read-only preview); `settle(owner, campaign_id, code, period, token, rate)` — pays each attributed address `count*rate` of `token` from the owner's balance via the SAC token client, once per period.
**Consequences:** On-chain holds only periodic commitments (count + Merkle root of signed receipts + per-attribution counts). Anyone with the published receipts/proofs can detect disagreement with the committed root; the operator-controlled signer still attests whether events were genuine. Settlement is a real on-chain token transfer. Receipt format, epoch cadence and fee payer remain integration choices. New errors: `SharedNotFound`, `AlreadyRegistered`, `PeriodCommitted`, `TallyNotFound`, `AlreadySettled`, `InvalidTally`.

> **Updated by ADR-012/013/014:** `compute_payouts`/`settle` read immutable
> token/rate, attributed count must equal total, and settlement now consumes an
> owner-approved token allowance without requiring an owner signature per call.

## ADR-012 — Tally settlement hardening (audit): binding attribution, immutable token/rate
**Context:** A review found the first Tally cut was exploitable: `commit_tally`/`settle` took an arbitrary `per_attribution` map and an arbitrary `token`/`rate` at settle time. So an owner could register a code for creator A yet pay B; settle with `rate = 0` or a wrong token, permanently **locking** the period as settled; and `count * rate` could overflow.
**Decision:** (a) **Binding attribution** — if a shared code has a single registered `attributed_to`, `commit_tally` may only credit that address (`AttributionMismatch` otherwise). (b) **Immutable settlement config** — `payout_token` + `payout_rate` are fixed at `register_shared` (rate must be > 0 when a token is set, else `InvalidSettlement`); `settle`/`compute_payouts` read them on-chain and take no token/rate argument. (c) **Safe math** — `checked_mul` for amounts (`InvalidSettlement` on overflow). `settle` emits token+rate in its event.
**Consequences:** The on-chain attribution target and payout terms are now trustworthy — a creator can rely on the registered config, and a period can't be locked by a wrong settlement. New errors: `InvalidSettlement`, `AttributionMismatch`. The pinned toolchain also adds clippy/rustfmt; the web adds `.nvmrc` (Node 22).

## ADR-013 — Tally edge-case hardening (audit): config invariants, expiry, TTL
**Context:** A further review found four Tally edges: (1) a count-only code (no payout token) could still set a positive rate, producing payouts `settle` could never pay; (2) an unattributed code (`attributed_to = None`) could still commit per-attribution to arbitrary addresses; (3) `register_shared`/`commit_tally` ignored the campaign's `valid_until`; (4) no TTL bump existed for Tally storage.
**Decision:** (1) Settlement config is consistent — `payout_token` set ⇔ `payout_rate > 0`; no token ⇒ rate 0 (`InvalidSettlement`). (2) An unattributed code may not set a payout token and its tallies must carry no per-attribution (`AttributionMismatch`); an attributed code still credits only its registered address. (3) `register_shared` rejects an expired campaign (`CampaignExpired`); `commit_tally` is intentionally still allowed post-expiry, since tallies are committed retrospectively after an epoch closes. (4) Added `bump_tally(campaign_id, code, periods)` to re-extend a shared code + its periods' tally/settlement entries.
**Consequences:** Tally state is internally consistent and auditable long-term; the two modes are clean — *attributed + payable* or *count-only / unattributed*. Reuses existing errors (`InvalidSettlement`, `AttributionMismatch`, `CampaignExpired`). Candidate v0.2 has 34 contract unit tests after the 2026-07-11 review, including explicit recording-mode assertions for privileged authorizers.
**Trust-model note:** allowing `commit_tally` post-expiry means the chain does not (and cannot) attest that the underlying redemptions occurred before `valid_until` — that is a *receipt-level* guarantee (signed, Merkle-anchored, off-chain), not a contract-enforced one. This boundary is spelled out in SPEC §4 and must be stated in any pitch/SEP rather than implied by "trustless."

## ADR-014 — Candidate v0.2: exact attribution and keeper settlement

**Context:** The v0.1 contract accepted an attributed count below the committed
total, so an owner could acknowledge 40 conversions but pay a creator for 30.
Its `settle` also required the owner to authenticate every invocation, despite
product copy calling settlement automatic. The external token call occurred
before the settled guard was persisted.

**Decision:** For an attributed shared code, the only attribution entry must
equal the total `count` exactly. The owner pre-approves the Sorodeal contract as
a standard token spender for a bounded amount/expiration; any fee payer may
then call `settle`. Settlement checks balance/allowance, records its guard before
`transfer_from`, relies on transaction rollback on failure, and exposes public
`is_settled`.

**Consequences:** Underpayment through partial attribution is rejected and a
keeper can execute a payout without custody of the owner key. Allowances become
an explicit operational responsibility and should be minimal/short-lived. The
immutable v0.1 testnet deployment cannot be patched and is deprecated; v0.2 is
a locally built candidate until explicitly deployed.

## ADR-015 — Bounded owner indexing and complete TTL maintenance

**Context:** Rewriting one ever-growing `Vec<campaign_id>` on each campaign
creation creates an eventual cost/resource denial of service. Delegate entries
also lacked a public TTL maintenance operation.

**Decision:** Store `OwnerCount(owner)`, `OwnerCampaign(owner, slot)` and the
reverse campaign slot as separate entries, so creation is O(1). Add
`campaigns_page(owner, cursor, limit)` with a 100-item bound while retaining
`campaigns_of` for compatibility. Add `bump_delegates`, and export typed event
schemas for current SDK/indexer tooling.

**Consequences:** New integrations must page rather than call the compatibility
read for prolific owners. TTL maintenance still needs an off-chain inventory
and scheduler; the contract cannot discover every code/delegate itself.

---

### TODO / unresolved
- The donor prototype's alleged mainnet addresses are obsolete and must never be
  presented as Sorodeal deployments. Mainnet is not enabled.
- ~~Choose a license~~ → **Apache-2.0** (see `LICENSE`).
- SEP: drafted in `docs/SEP.md` (core + Tally + settlement as one standard). Open: confirm whether settlement should be split into a companion proposal before submission.

---

## ADR-016 — Cloud is optional and deployment capabilities are explicit

**Context:** The repository now includes a hosted API/console in addition to the
open contract and SDKs. Its TEST and METERED environments currently share the
deprecated v0.1 testnet contract. Treating Cloud as either nonexistent or as a
production/mainnet service makes both architecture and product copy misleading.

**Decision:** Cloud is an optional integration layer, never a protocol
dependency. It pins its contract ID and labels network/capabilities explicitly.
The legacy owner-signed settlement must not create an unused token allowance;
allowance-based keeper settlement is enabled only with a deliberate v0.2
deployment migration.

**Consequences:** A future contract upgrade requires a reviewed configuration
change, migration/reconciliation plan and capability tests. METERED refers only
to preview credits and does not imply mainnet, production billing or live value.

## ADR-017 — Chain–DB atomicity: operation journal (outbox) before engine choice

**Context:** Cloud writes to two systems that cannot commit atomically: Stellar
transactions and the local database (index, credits, receipts). The failure
window is narrowed today case by case — loyalty issues rewards on-chain first
and commits local rows in one SQL transaction, idempotency keys are reserved
before execution, business references deduplicate events — but a crash between
a confirmed chain write and its local indexing still requires manual
reconciliation. Separately, SQLite (single process, one writer) is the preview
store; a paid API needs PostgreSQL for concurrency, constraints, observability
and multi-instance deployments.

**Decision:** Introduce a durable operation journal (outbox) as the single
choke point for every chain-writing operation, before and independently of the
engine migration:

1. Record intent locally first, under a deterministic business key
   (org, env, operation, resource, period/idempotency identity).
2. Drive each entry through `pending → submitted → confirmed → indexed` (or
   `failed`), persisting the tx hash as soon as it exists.
3. Retries always resume the SAME journal entry and consult chain state before
   re-submitting — the entry, not the HTTP request, is the unit of work, so
   nothing can be double-issued.
4. A periodic reconciler sweeps non-terminal entries against chain truth
   (`is_settled`, `verify`, transaction lookup), and unique constraints cover
   events, rewards, periods and settlements.

PostgreSQL is adopted with (not before) this journal: the schema is written
engine-portable and the migration carries users, orgs, sealed key blobs, index
tables and the journal itself.

**Consequences:** The chain/DB boundary becomes crash-recoverable and
observable — every stuck operation is a visible journal row — at the cost of
one extra local write per chain operation. Multi-instance deployment still
additionally requires distributed locks/idempotency (ROADMAP production
blockers). Until the journal lands, Cloud remains a single-instance preview
and the documented failure windows stand.

## ADR-018 — Settlement isolation for multi-keeper safety (Proposed, v0.3)

**Context:** v0.2 settlement is permissionless by design: the owner grants the
contract a token allowance and any keeper triggers `settle`. Allowances are per
owner+token — not per obligation — so when one owner has several committed
periods, a keeper can consume the allowance the owner intended for a different
period. Cloud narrows this operationally (exact-amount approvals immediately
before settling, `is_settled` pre-checks, reconciliation of externally settled
periods, short allowance expiries), but the ambiguity is structural.

**Decision (proposed):** For contract v0.3, evaluate binding settlement funds
to their obligation instead of to their owner: a per-campaign (or per-period)
escrow/vault that the owner funds explicitly and from which `settle` pays out.
The alternative — keeping allowances but adding on-chain per-period
reservation — stays on the table if escrow UX proves too heavy. Either way,
v0.3 must keep settlement permissionless and funds owner-recoverable (no
admin, per ADR-001).

**Consequences:** Not a v0.2 change: the deployed contract is immutable and
its behavior is documented honestly (docs/CLOUD.md). Cloud reconciliation
remains necessary regardless — external keepers are a feature, not a fault.
Any v0.3 settlement redesign is gated on a written escrow-vs-reserved-allowance
comparison covering rent costs and recovery paths.
