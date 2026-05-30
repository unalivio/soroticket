# Sorodeal — Stellar Community Fund plan

> The SCF funds by **demonstrable milestones (tranches)**, escalating ambition and de-risking each step. A **standard / public good with a real design partner** is one of the strongest SCF narratives — it is reusable by the whole ecosystem (multiplier effect), like SDP.

## The "why Stellar" one-liner

> Stellar's sub-cent fees + native USDC + Soroban make it **economically viable for the first time to pay creators/referrers per conversion** — an open coupon/redemption standard with verifiable attribution and automatic settlement. No other chain can pay out fractions of a cent per redemption profitably.

Lead the pitch with **attribution + settlement**, never with "tamper-proof / anti double-spend" (that argument is weak for shared codes and reviewers will probe it).

## Milestones

### M0 — Positioning (at/before application)
Open repo + **SEP draft** + the protocol spec (`docs/SPEC.md`). This labels the project as a *standard*, not "another coupon dApp." Cheap, high reputational impact.
**Deliverable:** public repo, spec doc, SEP draft submitted to Stellar dev discussions.

### M1 — Spec + permissionless contract (testnet)
Ownership-based Soroban contract (no global admin), full test suite, reusing the prototype's security hardening (the `VULN-xx` work already in the donor contract). Public demo: create → issue → redeem across both profiles (burn + basic tally).
**Deliverable:** contract on testnet, tests, demo. *De-risks the core.*

### M2 — SDK + reference integration on mainnet (design partner)
Go SDK (and TS if frontends need it). The geolocated social network integrates it **live with real redemptions on mainnet**. First primitive: **referrals (P2P)** or **geo-drop**.
**Deliverable:** published SDK, live mainnet integration, case study / demo video. *This is the milestone reviewers most want — real adoption, not vaporware.*

### M3 — Trustless attribution + USDC payout (flagship)
Tally profile with Merkle-anchored receipts, per-creator/referrer attribution **auditable by anyone**, and **automatic USDC disbursement per conversion** (SDP-style).
**Deliverable:** trustless attribution + automated payout, docs, ideally a second integrator or a public dashboard. *The "only-on-Stellar" capability; ambitious, so it goes last.*

## How the open questions map to milestones

- Payout is **in scope** — it is M3, the flagship, not optional.
- First pilot primitive (referrals vs geo-drop) is an **M2** decision.
- SEP scope (core vs settlement companion) should be settled during M0/M1.

## Reality-check (what SCF will scrutinize)

- "Why on-chain?" → answer with attribution/payout, not double-spend.
- "Can others reuse it?" → yes: SEP + SDK + permissive license. Position as a public good, not a solo app.
- Solo founder is fine **if** you show pull — the design partner is that proof.
- Keep M1 tight; do **not** promise payout in M1.
- Verify current SCF round mechanics/amounts before applying (they change) — worth a fresh check at submission time.
