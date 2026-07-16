# Sorodeal — worked use cases

End-to-end integration profiles over the deployed primitives. Everything here
is labeled **implemented** (works today on the testnet preview) or **planned**
(product idea that still requires design and code). These are integration
profiles, not new contract features — the protocol stays neutral.

## 1. Gift / proof of delivery — "the wine-bottle problem"

**Scenario.** A winery gifts bottles to restaurants under one promise: pour
each bottle as complimentary glasses for diners. The winery wants evidence
that the bottles reach real customers — and are not quietly resold — without
taking the venue's word for it.

**Mechanics (implemented).** This is the Tally profile plus Cloud's opaque
commitments; no new primitive was needed:

- One `gift` campaign per program (e.g. *Copas de cortesía Q3*).
- One shared code per venue (`COPA-ROSALES`, `COPA-DONMARIO`, …), optionally
  attributed to the venue's Stellar address with a payout rate per verified
  serving. Additional venue codes are registered on the campaign after
  creation.
- Each bottle label encodes a deep link (`wa.me/<bot>?text=COPA-ROSALES`). A
  diner scans it; the integrator's bot records a shared event with
  `customer_ref` = the diner's phone (stored only as an HMAC commitment) and
  `order_ref` = `bottle#glass` (deduplicated server-side — the same glass can
  never be counted twice, even across retries).
- Every event produces an Ed25519-signed receipt; a periodic commit anchors
  the batch on-chain (count + Merkle root + **exact** attribution, contract
  v0.2).
- The public audit endpoint re-proves signatures, roots and inclusion proofs
  for anyone — including the venue itself.
- Settlement (v0.2) inverts policing into incentive: the campaign owner
  approves the exact period total and any keeper triggers the payout, so the
  venue can be **paid per verified serving**. Fraud is bounded by
  construction: a dishonest venue can extract at most
  `rate × servings per bottle` of product it already received as a gift.

### Three layers of resolution

| Layer | Sees | Cannot see |
|---|---|---|
| Chain | counts, Merkle roots, attribution totals, settlement | any identity |
| Cloud | opaque HMAC commitments, dedup, signed receipts | raw phone numbers |
| Integrator (bot/POS) | raw identities and business context | — |

The integrator must capture the mapping `(identity ↔ commitment ↔ leaf hash)`
**at write time** — commitments are computed with Cloud's HMAC key and cannot
be recomputed from the identity later. This is deliberate: two organizations
cannot cross-link the same customer through the published hashes.

### The granularity dial

| QR per… | Printing cost | What it buys |
|---|---|---|
| campaign (1 global) | one label design | totals only; venue dimension recoverable only through the integrator's own side-channel |
| **venue (recommended)** | one label batch per venue | per-venue dashboard, on-chain attribution and optional automatic payout |
| delivery batch | one label per box | temporal granularity per shipment |
| bottle (Burn profile) | unique labels | per-unit on-chain evidence, for premium cases |

On-chain attribution and settlement follow the **code**, so per-venue payouts
require per-venue codes. Analytics can always be reconstructed by the
integrator regardless of granularity.

### Honest trust limits

- The chain proves that the scan set is immutable and anchored. It does
  **not** prove a glass was actually poured; the receipt signer remains the
  trust anchor for event truth (`docs/SPEC.md` §1).
- Distinct customer commitments and time-of-day patterns raise the fraud bar;
  a determined venue with many SIM cards can still simulate scans — which is
  exactly why the bounded-payout framing matters.
- A photographed QR leaks. `order_ref` dedup bounds the damage; placing the
  QR under the capsule (scannable only after opening) both defeats copying
  and destroys resale value.
- Abuse detection is an integrator-layer responsibility by design: Sorodeal
  supplies the opaque commitments and the tamper-evident record, the
  integrator applies the policy (e.g. suspend deliveries).
- Communicate it accordingly: "validated scan", "registered delivery",
  "immutable proof of the recorded event" — an attestation under the
  integrator's declared policy — never "universal proof of physical
  consumption".

### Status

- **implemented** — `gift` campaign kind (Cloud API + console wizard),
  per-venue shared codes with optional attribution/payout, customer/order
  commitments and dedup, signed receipts with integrator evidence metadata
  (`evidence_type`/`context_hash`/`policy_version`, e.g. `whatsapp_scan`
  under your policy version), on-chain commits, the contract-scoped public
  audit route and exact-allowance keeper settlement. Runnable demo:
  `tests/e2e/cloud-gift/`.
- **planned** — console QR deep-link generator; per-customer policy caps
  (e.g. one serving per phone per day) as a Cloud-level knob; per-code global
  caps and geofence at the protocol level (`docs/SPEC.md` §9,
  `docs/ROADMAP.md`).
