# Sorodeal Cloud — implementation status and API v1

Sorodeal Cloud is the hosted convenience layer over the open contract: a Go
REST API, SQLite index and React console for teams that do not want to operate a
wallet or SDK directly.

> Status (2026-07-11): working **testnet preview**, not production. TEST is free
> and METERED exercises the credit ledger, but both use the deprecated v0.1
> testnet contract. Mainnet, production billing, automatic TTL maintenance and
> automatic tally scheduling are disabled.

## 1. Implemented architecture

```text
Console / integrator
        |
        v
Cloud API (Go) ---- SQLite/WAL index, receipts, credits, idempotency
        |
        v
Stellar testnet ---- legacy contract v0.1
```

- One encrypted custodial Stellar seed per organization and environment.
- A separate encrypted Ed25519 receipt-signing seed per organization and
  environment.
- A dedicated HMAC key creates stable opaque customer/order commitments; it is
  not coupled to encryption-key rotation.
- Local v1 encryption uses AES-GCM with 0600 key files. Invalid/missing key
  material fails closed. **This is not a production KMS.**
- Cloud serializes writes per org/environment inside one process. It is not yet
  safe for a horizontally scaled deployment.

Upgrade note: migration v1 forces a one-time logout because legacy raw session
tokens cannot be distinguished safely from new digests. It HMACs historical
plaintext `order_ref` values. Pre-audit events are preserved with
`committed_period = -1` but quarantined rather than retroactively signed;
legacy committed tallies without receipts return `410 Gone` from the audit
route. When an old loyalty customer next appears, the known legacy hash is
moved to the new HMAC identity, but the unsigned historical event remains
quarantined. Pre-upgrade idempotency fingerprints also fail closed with `409`
instead of being replayed under the new keyed format. Operators should
export/reconcile these rows explicitly.

## 2. Authentication and request rules

- Console: email/password plus an 8-hour `HttpOnly`, `SameSite=Lax` session;
  only a SHA-256 digest of the bearer is stored. Maximum five active sessions.
- **Identity roadmap (planned, not implemented):** the email/password form is a
  placeholder identity until a real provider lands. Planned, in order:
  transactional email provider → email verification + password recovery →
  magic-link sign-in → OAuth/SSO (Google, GitHub) and social login → MFA
  (TOTP) → org roles/RBAC and team invites. Nothing in the current schema
  blocks this: `users` is keyed by email and sessions are provider-agnostic,
  so adding `auth_identities` (provider, subject, user_id) is additive.
- API: `Authorization: Bearer sk_test_…` or `sk_metered_…`; only a digest is stored
  and keys are revocable.
- Session mutations enforce same-origin/fetch-metadata checks. Login and signup
  have independent abuse limits.
- Authenticated limits are 300 writes/minute and 1,200 reads/minute per
  principal. Responses include `X-RateLimit-*` and `Retry-After` when blocked.
- JSON mutations require `Content-Type: application/json`, reject unknown
  fields/multiple JSON values and cap bodies at 1 MiB.
- `Idempotency-Key` is optional but strongly recommended for supported
  authenticated v1 mutations.
  It is scoped by org/environment/endpoint, bound to method/path/body, reserved
  before execution and retained for 24 hours. A response is acknowledged only
  after its replay result is stored. Reusing a key with different input returns
  `409`.
- Shared events may carry `order_ref`; loyalty punches may carry `event_ref`.
  Cloud HMACs these values and enforces uniqueness inside their code/program,
  so changing the HTTP idempotency key cannot count the same referenced event
  twice. Omitting the business reference opts out of this second layer.

## 3. Implemented endpoints

All routes below require session/API-key authentication unless marked public.

| Area | Methods and routes |
|---|---|
| Auth | `POST /auth/signup`, `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`, `POST /orgs` |
| Overview | `GET /v1/overview`, `GET /v1/activity` |
| Campaigns | `POST/GET /v1/campaigns`, `GET /v1/campaigns/{id}`, `POST /v1/campaigns/{id}/archive` |
| Burn | `POST /v1/campaigns/{id}/codes`, `GET /v1/verify`, `POST/GET /v1/redemptions` |
| Tally | `POST /v1/campaigns/{id}/shared-codes`, `POST /v1/shared-codes/{cid}/{code}/events`, `POST /v1/shared-codes/{cid}/{code}/commits`, `GET/POST /v1/settlements` |
| Loyalty | `POST/GET /v1/loyalty/programs`, `GET /v1/loyalty/programs/{id}`, `POST /v1/loyalty/programs/{id}/punches` |
| Platform | `GET/POST /v1/keys`, `POST /v1/keys/{id}/revoke`, `GET /v1/credits`, `GET /v1/usage` |
| Webhooks | `GET/POST /v1/webhooks`, `POST /v1/webhooks/{id}/disable`, `POST /v1/webhooks/{id}/test` |
| Public audit | `GET /v1/audit/tallies/{chain_id}/{code}/{period}` |

`POST /v1/credits/recharges` deliberately returns `501 Not Implemented`. It
does not issue a payment address or credit a balance.

Lists currently use fixed server limits rather than cursor pagination. The
Cloud `verify` route is authenticated; public verification remains available
directly through the contract/playground. Do not advertise either as a
publishable-key API yet.

Archiving is an off-chain Cloud control: it blocks new issues, redemptions,
shared-code registrations, shared events and loyalty punches through Cloud,
while allowing retrospective tally commits and settlements. Expiry also blocks
new shared events/punches. Archive does not modify or preserve existing Soroban
state.

Cloud pins the legacy contract ID explicitly. Its current settlement is signed
by the custodial campaign owner and uses v0.1's direct token transfer; Cloud
does not create an unused token allowance. A future v0.2 migration must be
explicitly capability-gated and approve only the exact period amount before
calling the allowance-based settlement.

## 4. Credits and metering

Credits use integer millicredits (`1 cr = 1,000 mcr`). METERED reserves credits
before an operation and records refunds if the action fails; a conditional SQL
update prevents concurrent overdrafts. A refund that cannot reach SQLite is
currently logged but has no durable retry, another reason this is preview-only.
TEST is unmetered.

The current table and the monthly 25,000-credit grant are **preview values**, not
real-money pricing. No payment processor exists. The console and API expose:

| Operation | Preview price |
|---|---:|
| Create campaign | 20 cr |
| Issue unique code | 2 cr/code |
| Redeem unique code | 5 cr |
| Register shared code | 10 cr |
| Record shared event | 0.2 cr/event |
| Commit tally | 15 cr |
| Settle | 25 cr |
| Loyalty punch | 0.2 cr/punch |

No TTL keep-alive charge exists because the maintenance worker is not
implemented.

## 5. Signed receipts and Merkle audit

Recording a shared event or loyalty punch produces canonical JSON:

```json
{
  "version": 1,
  "campaign_id": 42,
  "code": "SAVE10",
  "count": 1,
  "customer_commitment": "optional HMAC hex",
  "order_commitment": "optional HMAC hex",
  "timestamp": 1783790000,
  "nonce": "32 hex chars",
  "signer": "G..."
}
```

The signer signs the exact UTF-8 JSON bytes with Ed25519. `leaf_hash` is
`SHA-256(payload)` and the signature is standard Base64. Parent nodes are
`SHA-256(left || right)`; an unpaired odd node is promoted unchanged.

The public audit endpoint checks the global receipt count, recomputes the root
from every stored leaf hash, and re-hashes/verifies the payload, signature,
metadata and inclusion proof for each receipt in the requested page. Pages use
`cursor` plus `limit` (default/max 100) and the public route is limited to 30
requests/minute per source IP. One tally contains at most 10,000 signed
receipts; if more are pending, a commit anchors the first batch and returns
`remaining_receipts`. The first batch uses `YYYYWW`; additional batches in the
same ISO week use `YYYYWW01` through `YYYYWW99`, so they can be committed
immediately without colliding with the append-only period key.

This proves that the published receipt set matches the on-chain commitment. It
does **not** prove an off-chain purchase was genuine; the
organization-controlled signer remains the trust anchor for that fact.

## 6. Loyalty behavior

A loyalty program creates a shared earn code and a Burn reward campaign.
Punches are serialized per program. When cumulative punches cross one or more
thresholds, Cloud issues exactly the number of rewards owed, on-chain first,
then commits the local punch/reward rows in one transaction. Reward codes use
12 cryptographically random characters.

`event_ref` is optional but recommended for POS integrations. A duplicate
reference returns `409` before a new punch; in the single-process preview the
program lock and database uniqueness protect the check. Multi-instance safety
still requires distributed coordination/outbox work.

Per-customer balances are off-chain HMAC commitments. Only aggregate signed
receipts are anchored. This is not a fully trustless customer balance.

## 7. Webhooks

Supported events are `redemption.created`, `tally.committed`,
`settlement.paid`, `loyalty.reward_issued` and `credits.low`.

- At most 20 active endpoints per environment.
- Public HTTPS port 443 only; credentials, fragments, redirects, proxies,
  private/link-local/shared/benchmark IPs and DNS rebinding to blocked IPs are
  rejected.
- Payloads are signed as
  `HMAC-SHA256(secret, timestamp + "." + raw_body)` in
  `X-Sorodeal-Signature: v1=<hex>`.
- Delivery ID, event type and timestamp are sent in `X-Sorodeal-*` headers.
- Envelopes always declare `network: "testnet"`, `production: false`,
  `livemode: false` and public mode `test`/`metered`; internal DB mode `live`
  remains only for backward compatibility.
- Non-2xx responses retry with backoff up to eight attempts.
- The signing secret is returned once; only an encrypted copy and display
  prefix remain.

Consumers must reject stale timestamps and deduplicate delivery IDs.

## 8. Explicitly not implemented / production blockers

- Mainnet and a reviewed v0.2 deployed contract.
- Stripe checkout or a verified USDC deposit watcher.
- KMS/HSM custody, key export, rotation workflow and recovery.
- MFA/passkeys, email verification and password reset.
- Automatic TTL bumping and automatic tally commit schedules.
- Durable outbox/reconciliation when a chain write succeeds but the local
  SQLite update fails.
- Multi-instance locks, distributed rate limiting and distributed idempotency.
- Cursor pagination and a publishable-key/public verification surface.
- Retention/deletion controls, backup validation and production observability.
