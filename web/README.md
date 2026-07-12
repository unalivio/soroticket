# Sorodeal Playground (web)

Interactive developer playground for the Sorodeal coupon protocol, signing with
Freighter on Stellar testnet.

> It calls the real **v0.2.0 testnet** deployment (2026-07-12). Testnet
> preview only — do not treat this playground as a production or real-value
> environment.

## Run

The playground consumes the local `@sorodeal/sdk` package (wired via
`file:../sdk/ts`), so build the SDK first:

```bash
(cd ../sdk/ts && npm install)   # builds dist/ via the prepare script
npm install
npm run dev      # http://localhost:5173
```

Build a static bundle (deploy to any static host — Netlify, Vercel, GitHub Pages):

```bash
npm run build    # → dist/
npm run preview
```

## Prerequisites

- Node 18+ (built on Node 22).
- The [Freighter](https://www.freighter.app/) browser extension, set to the **Testnet** network, with a funded testnet account (fund at <https://friendbot.stellar.org>). Required only for write calls (create / issue / redeem / delegates). **Verify / Inspect works with no wallet.**

## How it works

- **Real contract.** Every write builds, simulates, signs and submits a
  testnet transaction. Contract v0.2.0:
  `CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF`; deployment
  record in `../deployments/testnet-v0.2.0.json`. (The deprecated v0.1
  contract and its known issues remain documented in
  `../deployments/testnet.json`.)
- **No PII on-chain.** The redeemer reference is committed in-browser as an opaque, non-reversible 32-byte `redeemer_ref_hash` = SHA-256(random nonce ∥ reference). A public/constant salt would be brute-forceable for low-entropy refs; the random nonce makes it non-reversible and unlinkable (production may HMAC with a merchant pepper). ADR-005/010.
- **No fake UI state.** On connect, the app loads campaigns from the chain via
  `campaigns_of(owner)`; there are no local/demo campaigns or generated Merkle
  roots in memory. The fresh v0.2 deployment starts empty — create your own
  campaigns. (The old v0.1 chain still holds the historical `ROBERTOX` tally,
  40 total / 30 attributed: evidence of the v0.1 partial-attribution flaw that
  v0.2 now rejects.)

### Try it
1. **Verify** `DEMO0001` — works immediately, no wallet (public read against the live contract).
2. **Connect Freighter** (Testnet) → **Create Campaign** (owned by your wallet) → **Issue Codes** under it → **Redeem** one → **Verify** it flipped to *Burned*.
   *(Issuing/redeeming the seeded `DEMO0001` will return `Unauthorized` — that campaign is owned by the deployer, not you. That's the permissionless model working: create your own campaign.)*

## Architecture

A design export (React, authored for Babel-in-browser with global symbols) runs under Vite via a small globals bridge (`src/globals.js`) so the design files are preserved verbatim. The only rewritten file is `src/store.jsx`, which calls the real Soroban layer. That layer lives **once** in `@sorodeal/sdk` (`freighterClient`); `src/lib/soroban.js` is a thin bridge that binds it to `SD.NET` and exposes `window.SDK`. Network constants live in `src/data.js` (`SD.NET`).

| File | Role |
|---|---|
| `src/lib/soroban.js` | Thin bridge: binds `@sorodeal/sdk`'s `freighterClient` to `SD.NET` → `window.SDK` |
| `src/store.jsx` | App state + contract calls (context API) → `window.useApp` |
| `src/data.js` | Testnet constants, error map, CLI/TS/Go snippet generators |
| `src/*.jsx`, `src/*.css` | Design export (UI), unchanged |
