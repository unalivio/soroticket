# Soroticket — roadmap and release gates

Status snapshot: 2026-07-12. This file distinguishes working code from design
targets so the console and documentation do not imply mock features are live.

## Current artifacts

- **v0.2.0 testnet:** deployed 2026-07-12 at
  `CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF`
  (`deployments/testnet-v0.2.0.json`): Burn + Tally, exact attribution,
  allowance-based permissionless settlement, checks-effects-interactions,
  public settlement state, paged campaign ownership and TTL helpers. Testnet
  preview — never real value.
- **Legacy v0.1 testnet:** deployed at
  `CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX`; deprecated,
  superseded by v0.2.0 and unsafe for new or real-value integrations. It
  accepts partial attributed tallies and requires an owner signature for each
  settlement.
- **SDKs/playground:** default to the v0.2.0 deployment; the legacy
  compatibility alias is retired (`LEGACY_TESTNET`/`LegacyTestnetContractID`
  remain only as explicitly named v0.1 constants).
- **Cloud:** real Go API and React console; TEST and METERED are testnet
  previews over v0.2.0. Campaign rows are stamped with their contract
  deployment; rows created on v0.1 fail closed (409) for chain operations.
  Mainnet and real billing are disabled.

## Completed in this security pass

- [x] Smart-contract attribution and settlement hardening.
- [x] O(1) owner campaign index plus bounded pagination.
- [x] Typed events, generated v0.2 TypeScript binding and frozen ABI.
- [x] Atomic credit reservation/refund and request-body-bound idempotency.
- [x] Loyalty concurrency, reward issuance and secure-code fixes.
- [x] Signed Ed25519 event receipts, Merkle proofs and public audit endpoint.
- [x] Bounded/paginated audit proofs, receipt batching and public IP rate limit.
- [x] HMAC business-reference deduplication plus one-shot legacy privacy
  migration/quarantine (no retroactive fake receipts).
- [x] Separate reference HMAC key and receipt signer; fail-closed key loading.
- [x] Session/API security, rate limits, strict JSON and HTTP hardening.
- [x] Real webhook management/delivery, HMAC signatures, retries and SSRF
  defenses.
- [x] Empty playground state; no generated fake campaigns, codes or roots.
- [x] Recharge endpoint fails with `501 Not Implemented` instead of returning a
  pretend payment destination.

## Hardening after the 2026-07-16 external review

- [x] Signed receipt v2: network + contract deployment identity plus optional
  integrator evidence metadata (`evidence_type`/`context_hash`/
  `policy_version`); v1 receipts remain verifiable as issued.
- [x] Public audit route scoped by contract deployment — `chain_id` ambiguity
  across v0.1/v0.2 removed; responses carry `network` + `contract_id`.
- [x] Settlement reconciliation for permissionless keepers: `is_settled`
  pre-check plus post-race reconciliation, never fabricating a tx hash.
- [x] Console: sign-out menu, derived loyalty status, display-only org chip,
  batch-issue partial failures surfaced, onboarding checklist completes.
- [ ] Operation journal/outbox + PostgreSQL migration (ADR-017) — the next
  major Cloud work before charging for the API.
- [ ] v0.3 settlement isolation design (ADR-018).

## v0.2 testnet release gates (completed 2026-07-12)

- [x] Security report reviewed; deployment explicitly authorized by the
  operator (2026-07-12).
- [x] WASM deployed with the reproducible hash; the real contract ID and
  transaction hashes are recorded in `deployments/testnet-v0.2.0.json`. No
  fabricated seed data — on-chain state comes from live E2E and Cloud fixtures.
- [x] SDK, playground and Cloud defaults point to the v0.2.0 deployment; the
  legacy compatibility alias is retired.
- [x] Live E2E re-run against v0.2: Go 53/53 and TypeScript 53/53 scenarios,
  including exact-attribution commits, settle-without-allowance rejection
  (#18), the owner's exact allowance approval and a third-party keeper
  settlement that consumes the allowance to zero.
- [ ] Publish a signed receipt verifier package/CLI, not only the Cloud
  endpoint.

## Production blockers

- [ ] KMS/HSM-backed custodial and receipt keys, rotation and recovery runbooks.
- [ ] Email verification, MFA/passkeys, password reset and account recovery.
- [ ] Durable outbox/reconciliation for the on-chain-write/local-index boundary.
- [ ] Multi-instance distributed sequence locks, idempotency and rate limiting.
- [ ] Automated TTL maintenance with dry-run, rent estimates and alerts.
- [ ] Automated tally scheduling with a published epoch/late-event policy.
- [ ] Stripe and/or verified USDC deposit confirmation; recharges remain disabled.
- [ ] Mainnet contract/configuration, asset allowlist, limits, monitoring and an
  independent external audit.
- [ ] Data retention/deletion policy, backups and incident-response exercises.

## Console polish (quick wins — completed 2026-07-16)

- [x] Sign-out lives in the user-avatar menu (`POST /auth/logout`).
- [x] Loyalty status pill derives Archived/Expired/Active from the backing
  campaign (API exposes `archived` + `valid_until`).
- [x] Org chip is display-only until orgs/teams land; user avatar opens a menu.
- [x] Batch-issue 207 surfaces the partial failure and its error message.
- [x] First-run checklist completes step 4 from the live API-key count.
- [ ] Campaign detail shows only the first shared code; surface all venue
  codes of a gift campaign (Settlements already lists them all).

## Product work after the release gates

- [ ] Console QR deep-link generator for shared codes (gift/delivery-proof
  campaigns print one label batch per venue; see `docs/USE_CASES.md`).
- [ ] Per-customer policy caps on shared codes (Cloud-level knob over customer
  commitments, e.g. one serving per phone per day) ahead of protocol-level
  per-user limits.
- [ ] Public verification API/widget with publishable keys and bounded
  pagination.
- [ ] Ticket transfer/ownership extension and seat inventory.
- [ ] Shared-code global caps and per-redeemer policy enforcement.
- [ ] Gift-card/stored-value primitive with refund and liability accounting.
- [ ] Coalition loyalty and cross-merchant settlement.
- [ ] Configure the real repository/discussion URLs and complete SEP metadata
  before publication; no remote is configured today.
