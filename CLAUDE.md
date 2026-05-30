# CLAUDE.md — Sorodeal

Orientation for any AI/dev session in this repo. **Read this before acting.**

## What Sorodeal is

An **open, permissionless protocol/standard** for tokenized coupons, vouchers, and redeemable codes on **Stellar Soroban**, with verifiable attribution and optional automatic settlement (USDC payouts per conversion).

It is a **public good / standard**, not a SaaS product. Think "like Stellar Disbursement Platform (SDP)" — a reference implementation others reuse. The intended path is a candidate **SEP (Stellar Ecosystem Proposal)** plus a milestone-funded **SCF** grant.

## Critical context (do not lose this)

- **This repo is the home of the protocol.** It is NOT a feature of BotCore. BotCore was only the prototype where the code originated.
- The `contracts/coupon-ledger/` contract and everything under `reference/botcore-donor/` are **donor/prototype code**, brought here to reuse the progress. They reflect the OLD design and must not be treated as the final spec.
- The **real design target** lives in `docs/SPEC.md` and `docs/DECISIONS.md`. When in doubt, the spec wins over the donor code.

## Goals

1. Publish Sorodeal as an open standard (candidate SEP) — reusable by anyone, not just the author.
2. **Design partner:** the author's geolocated social network is the first real integration — dogfood + visibility. It uses the protocol; it does not own it.
3. **Funding:** submit to the Stellar Community Fund (SCF) as a tranche/milestone-based grant. This repo lives under `hackathons/`, so a hackathon may be the launch/visibility vehicle.

## The design in one screen

- **One primitive, policy knobs.** The "three kinds of coupons" are one primitive varying on three axes: cardinality (code→redemptions: unique vs shared), attribution (is a redemption credited to a creator/referrer?), and per-user limits (once-per-user, geofence, time window).
- **Spec nouns:** Campaign → Code → Redemption → Settlement.
- **Two redemption profiles:**
  - **Burn (synchronous)** — unique tokens, 1 tx/redeem. Tickets, high-value vouchers. The donor contract already implements this.
  - **Tally (asynchronous)** — shared codes, off-chain hot path + periodic on-chain commitment (counts + attribution, Merkle-anchored). Delivery promos, creator/referral codes. This is the new work.
- **"Why Stellar" = attribution + settlement, NOT double-spend.** For shared codes there is no double-spend to prevent (they are meant to be reused many times). The real value is: trustless per-creator/referrer redemption counts + automatic USDC payout per conversion, viable only because Stellar fees are sub-cent and USDC is native.
- **Permissionless / ownership-based.** Each merchant/creator operates from their own Stellar account. NO global admin (the donor contract's `require_admin` single-ADMIN model is the #1 thing to redesign).

## Production bar / known gaps to fix (carried from the prototype)

1. **Permissionless redesign** of the contract: campaigns owned by their creator's `Address`; auth by campaign owner, not a global admin.
2. **Replace the `stellar` CLI shell-out** (`reference/botcore-donor/stellar-adapter/client.go`) with in-process signing via the Go Stellar SDK + Soroban RPC: idempotency keys, retries, sequence management. The CLI was fine for a prototype, not for a standard.
3. **PII on-chain:** the donor contract stores the redeemer's name in plaintext (`burned_by`). Hash with salt or omit.
4. **Idempotency** on the off-chain redemption wrapper (network retries must not double-count or double-burn).
5. **Verify the deployed prototype address.** The donor pitch (`reference`/BotCore `PITCH_SOROBAN.md`) cited two inconsistent mainnet contract addresses — do not assert one as fact until verified.

## Stack

- **Contract:** Rust + Soroban SDK (`contracts/coupon-ledger/`). Build/test with the `stellar`/`soroban` CLI and `cargo test`.
- **SDK (planned):** Go first (reuse donor patterns), TypeScript later for web/mobile integrators.
- **Settlement (planned):** USDC on Stellar, SDP-style disbursement keyed off committed tallies.

## Docs

- `docs/SPEC.md` — the protocol spec (the design target).
- `docs/SCF.md` — the SCF tranche/milestone plan (M0–M3).
- `docs/DECISIONS.md` — architecture decision records (the "why").

## Working agreements

- Keep this standalone — do not re-entangle with BotCore, WhatsApp, Twilio, or multi-tenant SaaS concerns. Those were prototype scaffolding.
- Treat the donor contract as a starting point to refactor, not gospel. Update `docs/SPEC.md` when decisions change, and add an ADR to `docs/DECISIONS.md`.
