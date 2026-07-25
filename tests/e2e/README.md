# Soroticket — end-to-end tests

Consumer tester apps that exercise the Soroticket coupon-ledger contract against
the **live Stellar testnet** deployment through the published SDKs — one per
language, each importing its SDK exactly as an external integrator would.

Each runner generates and funds 4 ephemeral testnet accounts (owner, delegate,
stranger, creator) via friendbot, then walks the **same 50-scenario matrix** and
exits non-zero if any scenario fails.

| Runner | Consumes | Run |
|---|---|---|
| `go/` | `github.com/soroticket/soroticket-go` (local `replace`) | `cd go && go run .` |
| `ts/` | `@soroticket/sdk` (local `file:`) | `cd ts && npm install && npm run e2e` |

Both target the contract in `deployments/testnet.json`. A full run takes
~3–5 minutes (most steps wait for a testnet ledger to close; the expiry check
waits for a short-lived campaign to lapse). No secrets or pre-funded accounts
are required — friendbot funds the ephemeral keypairs.

## Scenario matrix (both runners, in order)

**Burn profile** — create (percentage); field round-trip; owner index;
issue batch; `is_valid`; redeem + receipt; verify-burned; stats; and the
errors: double-redeem `#3`, unknown `#2`, stranger `#6`.

**Discount variants** — `fixed_amount` and `free_item` campaigns create + redeem
(the opaque reward metadata round-trips in the receipt).

**Validation** — `create_campaign` `#9` (supply 0 / empty type / past expiry);
`issue_unique` `#8` duplicate, `#7` empty, `#11` too-long, `#6` stranger,
`#5` supply exhausted.

**Scoping (ADR-009)** — the same code string in two campaigns is independent;
burning it in one leaves the other valid.

**Delegates (ADR-002)** — add → delegate redeems → remove → delegate denied `#6`.

**TTL** — `bump_campaign` (public) ok / unknown `#1`; `bump_codes` existing +
unknown skipped / empty `#7`.

**Tally profile** — `register_shared` attributed+settlement, attributed
count-only, unattributed count-only; settlement-config errors `#18` (×3),
duplicate `#13`, stranger `#6`, bad code `#7`/`#11`, `get_shared` `#12`.

**Commit** — `commit_tally` ok + `get_tally` round-trip; re-commit `#14`;
attribution mismatch `#19` (×2); over-count `#17`; stranger `#6`; missing `#15`.

**Settlement** — `compute_payouts` preview (`count*rate`); `settle` with a
**real native-XLM transfer** to the attributed creator; re-settle `#16`;
count-only settle `#18`; `bump_tally` ok / unknown `#12`.

**Expiry** — a short-lived campaign (created first) lapses mid-run; afterwards
`issue_unique` / `redeem_unique` / `register_shared` all reject with `#4`.

## Notes

- The TS runner uses the generated typed `Client` + `keypairSigner`, and
  recovers contract error codes via `contractErrorCode(tx.simulation.error)`
  (the typed bindings drop the code on a simulation trap).
- The Go runner uses the hand-written in-process client; contract traps surface
  as `*soroticket.ContractError` (`soroticket.CodeOf`).
- Settlement uses the testnet native-XLM SAC as the payout token; the owner pays
  the creator `count * rate` base-units (stroops) from its own balance.
