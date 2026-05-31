# Sorodeal Playground (web)

Interactive developer playground for the Sorodeal coupon protocol — **live on Stellar testnet**, signing with **Freighter**. Create campaigns, issue codes, redeem, verify, manage delegates, and copy ready-to-run CLI / TypeScript / Go snippets for every call.

## Run

```bash
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

- **Real contract.** Every write builds a Soroban transaction, simulates it, is signed by Freighter, and is submitted to the live testnet contract. Reads run as RPC simulations (no signature). Contract: `CA5AFYJEX2DIFMIH3IGBDBEBTIPSRO2W7FJPK6HOP6CLK2FUTQQS44QP` (see `../deployments/testnet.json`).
- **No PII on-chain.** The redeemer reference is committed in-browser as an opaque, non-reversible 32-byte `redeemer_ref_hash` = SHA-256(random nonce ∥ reference). A public/constant salt would be brute-forceable for low-entropy refs; the random nonce makes it non-reversible and unlinkable (production may HMAC with a merchant pepper). ADR-005/010.
- **Chain is the source of truth.** On connect, the app loads your campaigns from the chain via `campaigns_of(owner)` — no localStorage. The seeded *Demo Cafe* (campaign `1`, codes `DEMO0001`/`DEMO0002`) backs the no-wallet Verify demo. Codes are scoped per campaign (ADR-009), so verify/redeem take a campaign id.

### Try it
1. **Verify** `DEMO0001` — works immediately, no wallet (public read against the live contract).
2. **Connect Freighter** (Testnet) → **Create Campaign** (owned by your wallet) → **Issue Codes** under it → **Redeem** one → **Verify** it flipped to *Burned*.
   *(Issuing/redeeming the seeded `DEMO0001` will return `Unauthorized` — that campaign is owned by the deployer, not you. That's the permissionless model working: create your own campaign.)*

## Architecture

A design export (React, authored for Babel-in-browser with global symbols) runs under Vite via a small globals bridge (`src/globals.js`) so the design files are preserved verbatim. The only rewritten file is `src/store.jsx`, which calls the real Soroban layer in `src/lib/soroban.js` (`@stellar/stellar-sdk` + `@stellar/freighter-api`). Network constants live in `src/data.js` (`SD.NET`).

| File | Role |
|---|---|
| `src/lib/soroban.js` | Real Soroban RPC + Freighter signing → `window.SDK` |
| `src/store.jsx` | App state + contract calls (context API) → `window.useApp` |
| `src/data.js` | Testnet constants, error map, CLI/TS/Go snippet generators |
| `src/*.jsx`, `src/*.css` | Design export (UI), unchanged |
