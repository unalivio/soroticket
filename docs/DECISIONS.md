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
**Consequences:** Redeemer identity on-chain is always a salted hash. The donor contract's plaintext `burned_by` is a bug to fix.

## ADR-006 — Name: Sorodeal
**Context:** Needed a name for the protocol/repo. Candidates floated: Tessera, Canje, Sello, Stub.
**Decision:** **Sorodeal** (Soroban + deal).
**Consequences:** Repo, package, and SEP naming follow. Still pursue a candidate SEP for "standard" legitimacy regardless of brand name.

---

### TODO / unresolved
- Verify the deployed prototype's mainnet contract address — the donor pitch cited two inconsistent addresses; do not assert either as fact until checked on-chain.
- Choose a license (Apache-2.0 vs MIT).
- Decide SEP scope: core primitive as the SEP, settlement as a companion proposal?
