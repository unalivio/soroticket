# Sorodeal — roadmap and release gates

Status snapshot: 2026-07-11. This file distinguishes working code from design
targets so the console and documentation do not imply mock features are live.

## Current artifacts

- **Legacy v0.1 testnet:** deployed at
  `CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX`; deprecated and
  unsafe for new or real-value integrations. It accepts partial attributed
  tallies and requires an owner signature for each settlement.
- **Candidate v0.2:** Burn + Tally, exact attribution, allowance-based
  permissionless settlement, checks-effects-interactions, public settlement
  state, paged campaign ownership and TTL helpers. Built/tested locally; **not
  deployed**. See `deployments/candidate-v0.2.0.json`.
- **SDKs/playground:** updated for the v0.2 ABI, but their compatibility default
  still names legacy v0.1 until an approved v0.2 deployment exists.
- **Cloud:** real Go API and React console, but TEST and METERED are both
  testnet previews over legacy v0.1. Mainnet and real billing are disabled.

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

## Required before a v0.2 testnet release

- [ ] Review the security report and authorize deployment explicitly.
- [ ] Deploy the candidate WASM, record the real contract ID/hash and seed only
  valid examples.
- [ ] Point SDK, playground and Cloud defaults to that deployment; retire the
  legacy compatibility alias.
- [ ] Re-run live E2E tests against the v0.2 contract, including token allowance
  approval and a third-party keeper settlement.
- [ ] Publish a signed receipt verifier package/CLI, not only the Cloud endpoint.

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

## Console polish (quick wins, non-blocking)

- [ ] Wire a sign-out control to `POST /auth/logout` (endpoint exists; no UI yet).
- [ ] Loyalty status pill is hardcoded `Active`; derive it (archived/expired
  programs 409 punches with no UI explanation).
- [ ] Org switcher / user-avatar buttons are inert; give them a menu or make
  them non-interactive.
- [ ] Batch-issue partial success returns HTTP 207 but the toast reports it as a
  clean success and drops the `error` member.
- [ ] First-run checklist step 4 ("Get your API key") never marks done.

## Product work after the release gates

- [ ] Public verification API/widget with publishable keys and bounded
  pagination.
- [ ] Ticket transfer/ownership extension and seat inventory.
- [ ] Shared-code global caps and per-redeemer policy enforcement.
- [ ] Gift-card/stored-value primitive with refund and liability accounting.
- [ ] Coalition loyalty and cross-merchant settlement.
- [ ] Configure the real repository/discussion URLs and complete SEP metadata
  before publication; no remote is configured today.
