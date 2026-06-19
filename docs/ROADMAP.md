# Sorodeal — Roadmap / pending work

Status snapshot and the outstanding work, so nothing is lost across history
rewrites. Current state: contract (Burn + Tally + settlement) audited and
deployed to testnet, web playground, TS SDK, docs/SEP/license — all committed.

Live testnet contract: `CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX`
(wasm `e88e1cda…`, ABI frozen as `contracts/coupon-ledger/abi-v0.1.0.txt`).

## A · Compacting (in progress)
- [ ] **Unify the web on `@sorodeal/sdk`** — `web/src/store.jsx` calls the typed
      client; retire the hand-written `web/src/lib/soroban.js`; wire the local
      package into Vite. (Build-validated here; browser smoke test = owner.)
- [ ] **Squash history** (~16 commits → a few clean ones).
- [ ] **Tag `v0.1.0`** on the final state.

## B · Next build
- [ ] **Go SDK** — hand-written over the Go Stellar SDK: in-process signing,
      idempotency keys, retries, sequence management (CLAUDE.md gap #2; replaces
      the donor `reference/botcore-donor/stellar-adapter/client.go`). Off-chain
      idempotency wrapper (gap #4) lives here.

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
