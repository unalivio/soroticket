# CLAUDE.md — Soroticket

Orientation for any AI/dev session in this repo. **Read this before acting.**

## What Soroticket is

An open protocol **and hosted preview suite** for tokenized coupons, vouchers,
tickets and loyalty on Stellar Soroban. The repository contains the contract,
Go/TypeScript SDKs, developer playground, Cloud API and Cloud console.

The protocol can remain a public good while Cloud is an optional hosted product.
Do not describe either preview environment as production or mainnet.

## Critical context (do not lose this)

- **This repo is the home of the protocol.** It is NOT a feature of BotCore. BotCore was only the prototype where the code originated.
- Everything under `reference/botcore-donor/` is archived donor/prototype code;
  it is not part of the runtime or the current security boundary.
- `contracts/coupon-ledger/` is the current candidate v0.2 source. The immutable
  v0.1 testnet deployment is deprecated and remains the SDK default only until
  an authorized v0.2 deployment exists.
- The design target and its trust boundaries live in `docs/SPEC.md` and
  `docs/DECISIONS.md`. The deployed address never overrides those warnings.

## Goals

1. Publish reusable contract interfaces and SDKs as a candidate open standard.
2. Offer a hosted integration path without making Cloud a protocol dependency.
3. Reach the production gates in `docs/ROADMAP.md` before handling real value.

## The design in one screen

- **One primitive, current knobs.** The implemented axes are unique/shared
  cardinality and optional fixed attribution. Per-user limits, geofence,
  `valid_from`, transfers and refunds are product ideas, not guarantees.
- **Spec nouns:** Campaign → Code → Redemption → Settlement.
- **Two redemption profiles:**
  - **Burn (synchronous)** — unique tokens, 1 tx/redeem. Tickets, high-value vouchers. The donor contract already implements this.
  - **Tally (asynchronous)** — shared codes, off-chain hot path + periodic
    on-chain commitment (counts + attribution, Merkle-anchored). Implemented in
    v0.2, which Cloud preview runs against; Cloud publishes signed receipt proofs.
- **Why Stellar:** Burn prevents repeated use on-chain. Tally makes a published
  aggregate immutable and v0.2 enforces exact configured attribution at
  settlement. It does not prove that an off-chain sale occurred; the receipt
  signer remains trusted for event truth.
- **Settlement:** v0.2 supports permissionless triggering after the owner grants
  the contract a token allowance. It is not a running automatic scheduler.
- **Permissionless / ownership-based.** Each merchant/creator operates from their own Stellar account. NO global admin (the donor contract's `require_admin` single-ADMIN model is the #1 thing to redesign).

## Security and release status

- Read `docs/SECURITY_AUDIT_2026-07-11.md` before changing release claims.
- v0.2.0 is **deployed to testnet** (2026-07-12,
  `CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF`); the record with
  real transaction hashes is `deployments/testnet-v0.2.0.json`. Never invent a
  contract ID, transaction hash, receipt, metric, balance or payment destination.
- Cloud TEST and internal `live`/public METERED both use Stellar testnet and the
  v0.2 contract. METERED changes preview credits, not the network. The v0.1
  contract is deprecated and kept only for backward compatibility.
- Signed receipts prove signer attestation and Merkle inclusion; they do not
  independently prove a purchase.
- Main production blockers include external audit, KMS/HSM, MFA/recovery,
  chain/DB reconciliation, multi-instance coordination, schedulers and billing.

## Stack

- **Contract:** Rust + Soroban SDK (`contracts/coupon-ledger/`). Build/test with the `stellar`/`soroban` CLI and `cargo test`.
- **SDKs:** `sdk/ts` and `sdk/go`; their checked-in defaults are explicitly
  legacy testnet until v0.2 is deployed.
- **E2E apps:** historical consumer scenarios under `tests/e2e`; a compile or
  prior run is not evidence that the current candidate was tested live.
- **Settlement:** SAC payout is implemented locally in v0.2 and requires an
  owner allowance. Do not claim a current network E2E without a recorded run.

## Docs

- `docs/SPEC.md` — the protocol spec (the design target).
- `docs/DECISIONS.md` — architecture decision records (the "why").

## Working agreements

- Keep this standalone — do not re-entangle with BotCore, WhatsApp, Twilio, or multi-tenant SaaS concerns. Those were prototype scaffolding.
- Treat `reference/` as historical context only. Update `docs/SPEC.md` when
  protocol decisions change, and add an ADR to `docs/DECISIONS.md`.
- Label product/UI capabilities `implemented`, `preview/testnet` or `planned`.
