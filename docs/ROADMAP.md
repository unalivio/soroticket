# Sorodeal — Roadmap / pending work

Status snapshot and the outstanding work, so nothing is lost across history
rewrites. Current state: contract (Burn + Tally + settlement) audited and
deployed to testnet, web playground, TS SDK, docs/SEP/license — all committed.

Live testnet contract: `CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX`
(wasm `e88e1cda…`, ABI frozen as `contracts/coupon-ledger/abi-v0.1.0.txt`).

## A · Compacting (in progress)
- [x] **Unify the web on `@sorodeal/sdk`** — the low-level client now lives once
      in the SDK (`freighterClient`, `sdk/ts/src/browser.ts`); `web/src/lib/
      soroban.js` is a thin bridge binding it to `SD.NET`. Build-validated;
      browser smoke test = owner.
- [ ] **Squash history** (now ~17 commits + the Go-SDK/E2E work → a few clean ones).
- [ ] **Tag `v0.1.0`** on the final state.

## B · Go SDK + E2E (done)
- [x] **Go SDK** (`sdk/go`, `github.com/sorodeal/sorodeal-go`) — in-process
      signing over `github.com/stellar/go-stellar-sdk` (Soroban RPC): simulate →
      assemble (footprint + resource fee + auth) → sign → submit with retries +
      idempotent re-submission (same envelope) so a retry can't double-burn
      (CLAUDE.md gaps #2/#4). All 22 methods, typed structs, `*ContractError`
      codes 1–19. Handles Protocol-23 TransactionMeta **V4** return values.
- [x] **E2E tester apps** (`tests/e2e/{go,ts}`) — consumer apps importing each
      SDK; fund ephemeral accounts via friendbot; **50 scenarios** green against
      live testnet (both profiles, all variants, owner/delegate/stranger,
      real settlement transfer, every error #1–#19). See `tests/e2e/README.md`.

## C2 · Sorodeal Cloud (new workstream — spec'd, not built)
- [x] **Platform spec** (`docs/CLOUD.md`): REST API v1, per-org custodial keys,
      credits ledger + metering (build the meter now, price later), free tier
      as monthly grant, recharge (card + USDC), TEST/LIVE envs, webhooks,
      idempotency, TTL keep-alive cron, **loyalty programs** (Tally-anchored
      punches → auto-issued Burn reward voucher).
- [x] **Console design prompt** (`docs/design/CONSOLE_DESIGN_PROMPT.md`) — for
      Claude Design, attach CLOUD.md + SPEC.md + lib.rs + playground styles.
- [x] Console design (Claude Design) → imported (`docs/design/export/`).
- [x] **Cloud API built** (`cloud/api`, Go over `sorodeal-go`) — testnet, both
      envs; run: `cd cloud/api && go run .` (:8787, data in `cloud/api/data/`).
- [x] **Console built** (`cloud/console`, Vite/React from the design) — run:
      `npm run dev` (:5180, proxies /api → :8787). Browser-verified E2E.
- [ ] Webhook delivery (events already fire internally).
- [ ] Stripe checkout + automatic USDC recharge crediting.
- [ ] Mainnet enablement for LIVE (today LIVE = testnet + real metering).

## C · Publish / SEP (non-code)
- [ ] Publish the repo on GitHub (Apache-2.0 allows it).
- [ ] Fill SEP placeholders (`docs/SEP.md`): assigned number, author contact,
      discussion URL, repo URL.
- [ ] Fix the web footer `#` links (`web/src/console.jsx`: GitHub / Spec / SEP).
- [ ] Decide: settlement inside the SEP, or a companion proposal?
- [ ] Eventually: deploy to mainnet + freeze a mainnet version.

## D · Polish (nice-to-have)
- [ ] Code-split the web bundle (the only `npm run build` warning).
- [ ] Tally UI commit card supports one creator; the full `Map` is in the
      snippets. Extend if needed.
- [ ] "Real" settle demo: the seeded `ROBERTOX` pays deployer→deployer (trivial);
      a moving-funds demo needs a code attributed to a distinct address + a
      funded token.
- [ ] Refresh `docs/SCF.md` milestone plan to reflect what's already built.

## E · Obsolete (close)
- [ ] DECISIONS TODO "verify the donor prototype's mainnet address" — about the
      old BotCore prototype, not the current contract. Close as obsolete.
