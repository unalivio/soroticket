# Claude Design prompt — Sorodeal Console

> Archived design input. The implemented console is the source of truth; this
> prompt contains aspirational flows and must not be used as a feature-status
> document.

> Paste everything below the line into Claude Design, giving it this whole
> repository as context.

---

You have the full Sorodeal repository. Ground the design in these files —
read them before designing:

- `docs/CLOUD.md` — the hosted-platform/API spec. **Source of truth** for
  screens, entities, credits model, and flows; where this prompt and that spec
  differ, the spec wins.
- `docs/SPEC.md` — the protocol (Campaign → Code → Redemption → Settlement).
- `contracts/coupon-ledger/abi-v0.2.0.txt` — candidate data shapes and the 19
  error names (`src/lib.rs` has the full semantics if needed).
- `web/src/styles.css` — the playground's design tokens: the brand DNA to
  carry over. `web/src/data.js` shows the tone of sample data and copy.
- Ignore `reference/botcore-donor/` (legacy prototype) and `tests/`.

Design **Sorodeal Console** — the self-service web app for Sorodeal Cloud, the
hosted platform of the Sorodeal protocol (open standard for coupons, vouchers,
event tickets and loyalty programs on Stellar Soroban, with signed audit
receipts and allowance-based token settlement).

The audience is a **merchant or growth/ops person, not a crypto user**. They
sign up with email, get an API key, and create coupons — the blockchain is an
implementation detail they can inspect (every action links to an on-chain
transaction) but never need to understand. No wallets, no seed phrases, no gas:
the platform custodies a Stellar account per organization and meters usage
against a prepaid **credits** balance.

## Product mental model (design around this)

One primitive — a **Campaign** that contains **Codes** that produce
**Redemptions** and optionally **Settlements** — wearing five faces:

| Face | Example code | Behavior |
|---|---|---|
| General coupon | `10OFF` | One shared code, redeemed by many; on-chain count. |
| Creator / referral coupon | `PEDROPROMO10` | Shared code attributed to a creator; every redemption credits them on-chain and settles in USDC per conversion. |
| Unique voucher | `A7K9-QX2M` | Issued in batches; each code redeems exactly once, then is burned. |
| Event ticket | `TICKET-0042` | Unique code burned at the door; unforgeable admission. |
| Loyalty program | punch card | Earn punches per purchase (anchored on-chain per period); crossing the threshold auto-issues a reward voucher. |

Two redemption engines surface in the UI as plain language: unique codes
("each code works once") vs shared codes ("one code, many uses, counted").
Never say "Burn profile" or "Tally profile" in UI copy — say "unique codes"
and "shared codes".

## Brand — continuity with the existing Sorodeal playground (attached CSS)

- **Editorial, paper-like, warm.** Background `#FBFBF8`, surfaces `#FFFFFF` /
  `#F2F2EE`, ink `#0C0D0E` (secondary `#5B5C5E`, tertiary `#8C8D8F`),
  hairlines `#E7E6E0`.
- **Accent: yellow** `#FBD024` (hover `#F7C600`, washes `#FCE588`/`#FBF0BD`)
  with **black text on accent**. Semantic: valid green `#16863E`/`#E6F4EA`,
  burned/error red `#C8372A`/`#FBE9E6`, pending amber `#B57708`.
- **Type:** Newsreader (serif — page titles, big numbers, editorial moments),
  Hanken Grotesk (UI), JetBrains Mono (codes, addresses, tx hashes, credits).
- Truncate Stellar values `G…SV6` / `CBST…QDBX` style, mono, with copy-on-click.
- Feel: a well-set ledger book, not a crypto dashboard. Generous whitespace,
  hairline tables, no glassmorphism, no gradients.

## Global chrome

- Left sidebar nav: Overview · Campaigns · Redemptions · Settlements ·
  Loyalty · API Keys · Usage & Credits · Webhooks · Settings. Docs link at
  bottom.
- Top bar: org switcher (name + custodial address truncated), **environment
  toggle TEST / METERED** (both testnet previews; METERED exercises credits),
  credits
  balance chip (mono, e.g. `18,540 cr`), user menu.
- Every on-chain object shows a small `↗ stellar.expert` link.
- All state-changing errors surface the contract's error name in a friendly
  toast: e.g. #3 → "Already redeemed — this code was used on Jun 12, 14:02."
  (19 named errors exist in the attached contract; map the common ones:
  AlreadyRedeemed, CampaignExpired, SupplyExhausted, Unauthorized,
  DuplicateCode, AlreadySettled.)

## Screens (design all, desktop-first 1280px, key mobile variants for 1, 3, 6)

**1 · Auth + onboarding.** Email/password sign-in and sign-up; create
organization (name only). First-run checklist on empty Overview: ① Create your
first campaign ② Issue codes ③ Make a test redemption ④ Get your API key.
No wallet steps anywhere.

**2 · Overview (dashboard).** KPI row (Newsreader numerals): Redemptions
(30d), Active campaigns, Credits balance, Settled to creators (USDC). A
redemptions-over-time area chart (30d). Activity feed (mono timestamps):
"`TICKET-0042` redeemed · Cafe Promo · tx `a1b2…` ↗". Quick actions: New
campaign, Issue codes, Create API key.

**3 · Campaigns.** Index: filterable table (kind badge, name, status
active/expired/archived, codes issued/redeemed progress bar, created, env).
**New Campaign wizard** — step 1 is a five-card picker matching the five faces
above (card = icon + name + one-line description + example code in mono);
step 2 adapts fields per face (shared: the code itself + optional creator
address + optional payout token/rate with an "immutable after creation" note;
unique/ticket: supply + code generation pattern; loyalty: punches to reward +
reward discount + validity); step 3 review → Create. Detail page: stats header
(minted/redeemed/available, expiry countdown), tabs: Codes · Redemptions ·
Settlement (shared only) · Activity. Codes tab: batch issue modal (paste list
or generate N, chunk note), QR per code, CSV export. Archive action. Automated
TTL keep-alive and billing are explicitly not implemented.

**4 · Redemptions.** Live table across campaigns: code (mono), campaign,
result badge (Redeemed green / rejected reason red), redeemer ref (opaque hash,
tooltip: "Stored as a privacy-preserving commitment — no personal data
on-chain"), ledger seq, tx link. Row expands to the full receipt. A "Redeem a
code" manual action (campaign + code + optional reference) for counter use.

**5 · Settlements (shared/creator codes).** Per shared code: registered
creator address, payout rate, committed periods table (period, count, per-
creator count, Merkle root truncated, status: committed / settled / payable).
"Preview payout" (count × rate, USDC) → "Settle period" with confirm dialog
(shows exact USDC amount and destination). History with tx links.

**6 · Loyalty.** Programs index (name, punches→reward rule, active customers,
rewards issued). Program detail: big progress ring of rewards issued vs
outstanding; customers table (opaque customer ref, punches `7/10` with mini
progress, rewards earned); "Record punches" manual action; reward settings
(read-only after creation). Empty state that sells the feature: "Buy 10, get
1 free — on-chain."

**7 · API Keys.** Table: label, key prefix `sk_live_9F…` (reveal-once on
create), created, last used, revoke. Create modal → one-time full-key display
with copy + "you won't see this again". Below: quick-start snippet block with
tabs `curl` / `TypeScript` / `Go` calling `POST /v1/redemptions` with the real
base URL and this org's key prefix — mono, copyable.

**8 · Usage & Credits.** Balance header: big mono balance, monthly free grant
line ("25,000 cr renews Aug 1"), **Recharge** button → modal with two methods:
Card (amount presets) and **USDC on Stellar** (shows payment address + memo,
mono, QR, "credited after 1 confirmation"). Usage: stacked bar chart by
operation type (30d), table of the credit ledger (ts, operation, campaign,
Δ credits, balance, tx) — reads like a bank statement. Price table accordion
("What things cost") pulling the values from the spec, flagged
"introductory — free tier covers typical pilots".

**9 · Webhooks.** Endpoints table (url, events subscribed, status, last
delivery), add-endpoint modal (event checklist: redemption.created,
tally.committed, settlement.paid, loyalty.reward_issued, credits.low), signing
secret reveal, recent deliveries log with replay.

**10 · Settings.** Org name; custodial account card (address mono + QR +
stellar.expert link + explainer "Sorodeal holds this key for you — export
coming later"); danger zone (delete org).

## States to design explicitly

- TEST vs LIVE variants of Overview and Campaigns (test = amber hint + free).
- Empty states for every index (illustrated with the editorial style, one
  clear CTA), loading skeletons for tables, and the error toast anatomy.
- Low-credits warning banner (balance < est. 7 days) with Recharge CTA.
- A rejected redemption receipt (AlreadyRedeemed) side-by-side with a
  successful one.

## Sample data (use this, not lorem ipsum)

Org "Cafe Rosales" · address `GCIJ…DSV6` · balance `18,540 cr` · campaigns:
"Cafe Promo" (unique, 100 supply, 37 redeemed), "10OFF Delivery" (shared,
1,204 redemptions), "PEDROPROMO10" (creator `GAOQ…SRHQ`, rate 0.25 USDC, period
2026-W26 count 89 → payable 22.25 USDC), "Noche de Jazz" (tickets, 150 issued,
141 burned), "Cafetero Frecuente" (loyalty 10→1, 63 customers, 12 rewards).
Codes like `BURN-7Q2M`, `TICKET-0042`; tx hashes like `a1b2c3…9f`; dates
relative to July 2026.

## Deliverable

A cohesive multi-screen desktop web app design (React-ready), consistent
component system (tables, badges, modals, wizard, charts, toasts, sidebar),
light theme only for v1. Prioritize: 3 (Campaigns + wizard) → 2 → 8 → 6 → 7 →
4 → 5 → 9 → 10 → 1.
