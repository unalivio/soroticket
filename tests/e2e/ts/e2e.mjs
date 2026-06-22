/**
 * Consumer E2E tester for @sorodeal/sdk against the LIVE Stellar testnet
 * deployment. It uses the package exactly as a server integrator would — the
 * generated typed `Client` (via `sorodeal()`), `keypairSigner` for in-process
 * signing, and `contractErrorCode` to recover the contract error code the typed
 * bindings drop on simulation failures.
 *
 * Mirrors tests/e2e/go: both coupon profiles (Burn + Tally), every discount
 * variant, owner/delegate/stranger auth, the settlement transfer, and every
 * documented error (#1–#19). Exit 0 ⇒ all scenarios passed.
 *
 *   npm install && npm run e2e
 */
import { sorodeal, keypairSigner, contractErrorCode } from "@sorodeal/sdk";
import { Keypair } from "@stellar/stellar-sdk";
import { createHash } from "node:crypto";

const NATIVE_SAC = "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC";

// ── contract error codes (mirror the contract Error enum) ──────────────
const E = {
  CampaignNotFound: 1, CouponNotFound: 2, AlreadyRedeemed: 3, CampaignExpired: 4,
  SupplyExhausted: 5, Unauthorized: 6, InvalidCode: 7, DuplicateCode: 8,
  InvalidTerms: 9, BatchTooLarge: 10, CodeTooLong: 11, SharedNotFound: 12,
  AlreadyRegistered: 13, PeriodCommitted: 14, TallyNotFound: 15, AlreadySettled: 16,
  InvalidTally: 17, InvalidSettlement: 18, AttributionMismatch: 19,
};
const NAME = Object.fromEntries(Object.entries(E).map(([k, v]) => [v, k]));

// ═══════════════════════════════════════════════════════════════════
// harness
// ═══════════════════════════════════════════════════════════════════
let pass = 0, fail = 0;
async function step(name, fn) {
  const t0 = Date.now();
  try {
    await fn();
    pass++;
    console.log(`ok    ${name.padEnd(58)} (${Date.now() - t0}ms)`);
  } catch (e) {
    fail++;
    console.log(`FAIL  ${name.padEnd(58)} (${Date.now() - t0}ms)\n        ↳ ${e?.message ?? e}`);
  }
}
function assert(cond, msg) { if (!cond) throw new Error(`assertion failed: ${msg}`); }

// unwrap a binding result (Result<T> → T, or pass through a plain value)
function unwrap(r) { return r && typeof r.unwrap === "function" ? r.unwrap() : r; }

// run a write: build → (reject if sim error) → sign+send → unwrap.
// force:true is required for the TTL-bump methods (bump_campaign/codes/tally):
// they only call extend_ttl, so the simulation's readWrite footprint is empty
// and the SDK's isReadCall heuristic would otherwise refuse to submit them —
// even though they DO change on-chain state. It's a no-op for ordinary writes.
async function okWrite(thunk) {
  const tx = await thunk();
  if (tx?.simulation?.error) throw new Error(`simulation error: ${tx.simulation.error}`);
  const res = await tx.signAndSend({ force: true });
  return unwrap(res.result);
}
// run a read: build → (reject if sim error) → unwrap
async function okRead(thunk) {
  const tx = await thunk();
  if (tx?.simulation?.error) throw new Error(`simulation error: ${tx.simulation.error}`);
  return unwrap(tx.result);
}
// recover the contract error code from a trapping call (no send)
async function codeOf(thunk) {
  try {
    const tx = await thunk();
    const e = tx?.simulation?.error;
    return e ? contractErrorCode(e) : undefined;
  } catch (err) {
    return (
      contractErrorCode(err?.simulation?.error) ??
      contractErrorCode(err?.message) ??
      contractErrorCode(String(err))
    );
  }
}
async function wantCode(thunk, want) {
  const got = await codeOf(thunk);
  if (got === undefined) throw new Error(`expected #${want} (${NAME[want]}), got no contract error`);
  if (got !== want) throw new Error(`expected #${want} (${NAME[want]}), got #${got} (${NAME[got] ?? "?"})`);
}

// the bindings may decode a soroban Map<Address,u32> as a JS Map, a plain
// object, or an array of [k,v] entries — read it uniformly.
function mapGet(m, key) {
  if (m == null) return undefined;
  if (typeof m.get === "function") return m.get(key);
  if (Array.isArray(m)) return m.find(([k]) => k === key)?.[1];
  return m[key];
}

// ── helpers ────────────────────────────────────────────────────────────
const ref = (s) => createHash("sha256").update(s).digest(); // 32-byte Buffer
const farFuture = () => BigInt(Math.floor(Date.now() / 1000) + 10 * 365 * 24 * 3600);
const longCode = (n) => "A".repeat(n);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function fund(addr) {
  await fetch("https://friendbot.stellar.org/?addr=" + encodeURIComponent(addr)).catch(() => {});
  for (let i = 0; i < 40; i++) {
    const r = await fetch("https://horizon-testnet.stellar.org/accounts/" + addr).catch(() => null);
    if (r && r.status === 200) return;
    await sleep(1000);
  }
  throw new Error(`account ${addr} not visible after funding`);
}

function client(secret) {
  const s = keypairSigner(secret);
  return sorodeal({ publicKey: s.publicKey, signTransaction: s.signTransaction });
}

// ═══════════════════════════════════════════════════════════════════
async function main() {
  console.log("Sorodeal TS SDK — end-to-end against testnet");
  console.log("contract: CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX\n");

  const owner = Keypair.random(), delegate = Keypair.random(), stranger = Keypair.random(), creator = Keypair.random();
  console.log("funding 4 ephemeral accounts via friendbot…");
  await Promise.all([owner, delegate, stranger, creator].map((k) => fund(k.publicKey())));
  console.log("  owner   ", owner.publicKey());
  console.log("  creator ", creator.publicKey(), "\n");

  const O = client(owner.secret());
  const D = client(delegate.secret());
  const S = client(stranger.secret());
  const C = client(creator.secret());
  const pub = sorodeal(); // read-only
  const ownerAddr = owner.publicKey(), creatorAddr = creator.publicKey();

  // ── EXPIRY: kick off early, assert late ────────────────────────────
  const expValidUntil = BigInt(Math.floor(Date.now() / 1000) + 30);
  let expID;
  await step("setup: short-lived campaign for expiry checks", async () => {
    expID = await okWrite(() => O.create_campaign({
      owner: ownerAddr, name: "Expiring", discount_type: "percentage",
      discount_value: 1000n, total_supply: 5, valid_until: expValidUntil,
    }));
    await okWrite(() => O.issue_unique({ owner: ownerAddr, campaign_id: expID, codes: ["EXP00001"] }));
  });

  // ════════════════════════════════════════════════════════════════
  // BURN PROFILE
  // ════════════════════════════════════════════════════════════════
  let campPct;
  await step("create_campaign (percentage)", async () => {
    campPct = await okWrite(() => O.create_campaign({
      owner: ownerAddr, name: "Cafe Promo", discount_type: "percentage",
      discount_value: 1500n, total_supply: 100, valid_until: farFuture(),
    }));
    assert(campPct > 0n, "campaign id should be > 0");
  });

  await step("get_campaign round-trips fields", async () => {
    const c = await okRead(() => pub.get_campaign({ campaign_id: campPct }));
    assert(c.owner === ownerAddr, "owner");
    assert(c.name === "Cafe Promo", "name");
    assert(c.discount_type === "percentage", "discount_type");
    assert(c.discount_value === 1500n, "discount_value");
    assert(c.total_supply === 100, "total_supply");
  });

  await step("campaigns_of(owner) includes new campaign", async () => {
    const ids = await okRead(() => pub.campaigns_of({ owner: ownerAddr }));
    assert(ids.includes(campPct) && ids.includes(expID), "owner index");
  });

  await step("issue_unique batch", async () => {
    const ids = await okWrite(() => O.issue_unique({ owner: ownerAddr, campaign_id: campPct, codes: ["BURN0001", "BURN0002", "BURN0003"] }));
    assert(ids.length === 3, "expected 3 token ids");
  });

  await step("is_valid true for issued code", async () => {
    assert(await okRead(() => pub.is_valid({ campaign_id: campPct, code: "BURN0001" })), "BURN0001 valid");
  });

  await step("redeem_unique (owner) success + receipt", async () => {
    const r = await okWrite(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: campPct, code: "BURN0001", redeemer_ref_hash: ref("order-1") }));
    assert(r.campaign_id === campPct, "receipt campaign id");
    assert(r.discount_value === 1500n, "receipt discount value");
    assert(r.code === "BURN0001", "receipt code");
    assert(Number(r.ledger_seq) > 0, "receipt ledger seq");
  });

  await step("verify shows burned; is_valid now false", async () => {
    const tok = await okRead(() => pub.verify({ campaign_id: campPct, code: "BURN0001" }));
    const valid = await okRead(() => pub.is_valid({ campaign_id: campPct, code: "BURN0001" }));
    assert(tok.is_burned, "should be burned");
    assert(!valid, "should be invalid after burn");
  });

  await step("campaign_stats reflects 1 burn", async () => {
    const s = await okRead(() => pub.campaign_stats({ campaign_id: campPct }));
    assert(s.minted === 3, "minted");
    assert(s.burned === 1, "burned");
    assert(!s.is_expired, "not expired");
  });

  await step("redeem_unique double → #3 AlreadyRedeemed", () =>
    wantCode(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: campPct, code: "BURN0001", redeemer_ref_hash: ref("x") }), E.AlreadyRedeemed));

  await step("redeem_unique unknown code → #2 CouponNotFound", () =>
    wantCode(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: campPct, code: "NOSUCH", redeemer_ref_hash: ref("x") }), E.CouponNotFound));

  await step("redeem_unique by stranger → #6 Unauthorized", () =>
    wantCode(() => S.redeem_unique({ authorizer: stranger.publicKey(), campaign_id: campPct, code: "BURN0002", redeemer_ref_hash: ref("x") }), E.Unauthorized));

  // ── discount variants ────────────────────────────────────────────
  await step("variant: fixed_amount campaign create+redeem", async () => {
    const id = await okWrite(() => O.create_campaign({ owner: ownerAddr, name: "Fixed 5 off", discount_type: "fixed_amount", discount_value: 500n, total_supply: 10, valid_until: farFuture() }));
    await okWrite(() => O.issue_unique({ owner: ownerAddr, campaign_id: id, codes: ["FIX00001"] }));
    const r = await okWrite(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: id, code: "FIX00001", redeemer_ref_hash: ref("fix") }));
    assert(r.discount_type === "fixed_amount" && r.discount_value === 500n, "fixed_amount receipt");
  });

  await step("variant: free_item campaign create+redeem", async () => {
    const id = await okWrite(() => O.create_campaign({ owner: ownerAddr, name: "Free coffee", discount_type: "free_item", discount_value: 0n, total_supply: 10, valid_until: farFuture() }));
    await okWrite(() => O.issue_unique({ owner: ownerAddr, campaign_id: id, codes: ["FREE0001"] }));
    const r = await okWrite(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: id, code: "FREE0001", redeemer_ref_hash: ref("free") }));
    assert(r.discount_type === "free_item", "free_item receipt");
  });

  // ── create_campaign validation (#9) ──────────────────────────────
  await step("create_campaign supply=0 → #9 InvalidTerms", () =>
    wantCode(() => O.create_campaign({ owner: ownerAddr, name: "Bad", discount_type: "percentage", discount_value: 1000n, total_supply: 0, valid_until: farFuture() }), E.InvalidTerms));
  await step("create_campaign empty discount_type → #9 InvalidTerms", () =>
    wantCode(() => O.create_campaign({ owner: ownerAddr, name: "Bad", discount_type: "", discount_value: 1000n, total_supply: 10, valid_until: farFuture() }), E.InvalidTerms));
  await step("create_campaign valid_until in past → #9 InvalidTerms", () =>
    wantCode(() => O.create_campaign({ owner: ownerAddr, name: "Bad", discount_type: "percentage", discount_value: 1000n, total_supply: 10, valid_until: 1n }), E.InvalidTerms));

  // ── issue_unique validation ──────────────────────────────────────
  await step("issue_unique duplicate code → #8 DuplicateCode", () =>
    wantCode(() => O.issue_unique({ owner: ownerAddr, campaign_id: campPct, codes: ["BURN0002"] }), E.DuplicateCode));
  await step("issue_unique empty batch → #7 InvalidCode", () =>
    wantCode(() => O.issue_unique({ owner: ownerAddr, campaign_id: campPct, codes: [] }), E.InvalidCode));
  await step("issue_unique code too long → #11 CodeTooLong", () =>
    wantCode(() => O.issue_unique({ owner: ownerAddr, campaign_id: campPct, codes: [longCode(70)] }), E.CodeTooLong));
  await step("issue_unique by stranger → #6 Unauthorized", () =>
    wantCode(() => S.issue_unique({ owner: stranger.publicKey(), campaign_id: campPct, codes: ["HIJACK01"] }), E.Unauthorized));
  await step("issue_unique exceeding supply → #5 SupplyExhausted", async () => {
    const id = await okWrite(() => O.create_campaign({ owner: ownerAddr, name: "Tiny", discount_type: "percentage", discount_value: 1000n, total_supply: 2, valid_until: farFuture() }));
    await wantCode(() => O.issue_unique({ owner: ownerAddr, campaign_id: id, codes: ["A1", "A2", "A3"] }), E.SupplyExhausted);
  });

  // ── cross-campaign scoping (ADR-009) ─────────────────────────────
  await step("same code in two campaigns is independent (ADR-009)", async () => {
    const c1 = await okWrite(() => O.create_campaign({ owner: ownerAddr, name: "X1", discount_type: "percentage", discount_value: 1000n, total_supply: 10, valid_until: farFuture() }));
    const c2 = await okWrite(() => O.create_campaign({ owner: ownerAddr, name: "X2", discount_type: "percentage", discount_value: 1000n, total_supply: 10, valid_until: farFuture() }));
    await okWrite(() => O.issue_unique({ owner: ownerAddr, campaign_id: c1, codes: ["SHARED"] }));
    await okWrite(() => O.issue_unique({ owner: ownerAddr, campaign_id: c2, codes: ["SHARED"] }));
    await okWrite(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: c1, code: "SHARED", redeemer_ref_hash: ref("c1") }));
    assert(!(await okRead(() => pub.is_valid({ campaign_id: c1, code: "SHARED" }))), "burned in c1");
    assert(await okRead(() => pub.is_valid({ campaign_id: c2, code: "SHARED" })), "still valid in c2");
  });

  // ── delegates ────────────────────────────────────────────────────
  await step("delegate: add → redeem → remove → denied", async () => {
    await okWrite(() => O.add_delegate({ owner: ownerAddr, campaign_id: campPct, delegate: delegate.publicKey() }));
    assert(await okRead(() => pub.is_delegate({ campaign_id: campPct, who: delegate.publicKey() })), "delegate registered");
    await okWrite(() => D.redeem_unique({ authorizer: delegate.publicKey(), campaign_id: campPct, code: "BURN0002", redeemer_ref_hash: ref("by-delegate") }));
    await okWrite(() => O.remove_delegate({ owner: ownerAddr, campaign_id: campPct, delegate: delegate.publicKey() }));
    assert(!(await okRead(() => pub.is_delegate({ campaign_id: campPct, who: delegate.publicKey() }))), "delegate revoked");
    await wantCode(() => D.redeem_unique({ authorizer: delegate.publicKey(), campaign_id: campPct, code: "BURN0003", redeemer_ref_hash: ref("after-revoke") }), E.Unauthorized);
  });

  // ── TTL bumps ────────────────────────────────────────────────────
  await step("bump_campaign ok; unknown → #1 CampaignNotFound", async () => {
    await okWrite(() => C.bump_campaign({ campaign_id: campPct }));
    await wantCode(() => O.bump_campaign({ campaign_id: 9_000_000n }), E.CampaignNotFound);
  });
  await step("bump_codes existing+unknown ok; empty → #7 InvalidCode", async () => {
    await okWrite(() => C.bump_codes({ campaign_id: campPct, codes: ["BURN0001", "UNKNOWN9"] }));
    await wantCode(() => C.bump_codes({ campaign_id: campPct, codes: [] }), E.InvalidCode);
  });

  // ════════════════════════════════════════════════════════════════
  // TALLY PROFILE + settlement
  // ════════════════════════════════════════════════════════════════
  let ugc;
  await step("setup: campaign for Tally", async () => {
    ugc = await okWrite(() => O.create_campaign({ owner: ownerAddr, name: "Creator program", discount_type: "percentage", discount_value: 1000n, total_supply: 1000, valid_until: farFuture() }));
  });

  await step("register_shared attributed + settlement", async () => {
    await okWrite(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", attributed_to: creatorAddr, payout_token: NATIVE_SAC, payout_rate: 1000n }));
    const s = await okRead(() => pub.get_shared({ campaign_id: ugc, code: "ROBERTOX" }));
    assert(s.attributed_to === creatorAddr, "attributed_to");
    assert(s.payout_token === NATIVE_SAC, "payout_token");
    assert(s.payout_rate === 1000n, "payout_rate");
  });
  await step("register_shared attributed, count-only", () =>
    okWrite(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "COUNTONLY", attributed_to: creatorAddr, payout_token: undefined, payout_rate: 0n })));
  await step("register_shared unattributed, count-only", () =>
    okWrite(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "NOATTRIB", attributed_to: undefined, payout_token: undefined, payout_rate: 0n })));

  await step("register_shared unattributed + token → #18", () =>
    wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "BAD1", attributed_to: undefined, payout_token: NATIVE_SAC, payout_rate: 1000n }), E.InvalidSettlement));
  await step("register_shared token + rate 0 → #18", () =>
    wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "BAD2", attributed_to: creatorAddr, payout_token: NATIVE_SAC, payout_rate: 0n }), E.InvalidSettlement));
  await step("register_shared no token + rate != 0 → #18", () =>
    wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "BAD3", attributed_to: creatorAddr, payout_token: undefined, payout_rate: 5n }), E.InvalidSettlement));
  await step("register_shared duplicate → #13 AlreadyRegistered", () =>
    wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", attributed_to: creatorAddr, payout_token: undefined, payout_rate: 0n }), E.AlreadyRegistered));
  await step("register_shared by stranger → #6 Unauthorized", () =>
    wantCode(() => S.register_shared({ owner: stranger.publicKey(), campaign_id: ugc, code: "SNEAK", attributed_to: undefined, payout_token: undefined, payout_rate: 0n }), E.Unauthorized));
  await step("register_shared empty code → #7; long → #11", async () => {
    await wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: "", attributed_to: undefined, payout_token: undefined, payout_rate: 0n }), E.InvalidCode);
    await wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: ugc, code: longCode(70), attributed_to: undefined, payout_token: undefined, payout_rate: 0n }), E.CodeTooLong);
  });
  await step("get_shared not found → #12 SharedNotFound", () =>
    wantCode(() => pub.get_shared({ campaign_id: ugc, code: "GHOST" }), E.SharedNotFound));

  // ── commit_tally ─────────────────────────────────────────────────
  await step("commit_tally (attributed) → get_tally matches", async () => {
    const attr = new Map([[creatorAddr, 3]]);
    await okWrite(() => O.commit_tally({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", period: 1n, count: 5, merkle_root: ref("merkle-1"), per_attribution: attr }));
    const t = await okRead(() => pub.get_tally({ campaign_id: ugc, code: "ROBERTOX", period: 1n }));
    assert(t.count === 5, "count");
    assert(Number(mapGet(t.per_attribution, creatorAddr)) === 3, "per_attribution");
  });
  await step("commit_tally same period again → #14 PeriodCommitted", () =>
    wantCode(() => O.commit_tally({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", period: 1n, count: 2, merkle_root: ref("m"), per_attribution: new Map([[creatorAddr, 1]]) }), E.PeriodCommitted));
  await step("commit_tally crediting wrong addr → #19 AttributionMismatch", () =>
    wantCode(() => O.commit_tally({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", period: 2n, count: 5, merkle_root: ref("m"), per_attribution: new Map([[stranger.publicKey(), 1]]) }), E.AttributionMismatch));
  await step("commit_tally on unattributed code w/ attribution → #19", () =>
    wantCode(() => O.commit_tally({ owner: ownerAddr, campaign_id: ugc, code: "NOATTRIB", period: 1n, count: 5, merkle_root: ref("m"), per_attribution: new Map([[creatorAddr, 1]]) }), E.AttributionMismatch));
  await step("commit_tally sum > count → #17 InvalidTally", () =>
    wantCode(() => O.commit_tally({ owner: ownerAddr, campaign_id: ugc, code: "COUNTONLY", period: 1n, count: 5, merkle_root: ref("m"), per_attribution: new Map([[creatorAddr, 9]]) }), E.InvalidTally));
  await step("commit_tally by stranger → #6 Unauthorized", () =>
    wantCode(() => S.commit_tally({ owner: stranger.publicKey(), campaign_id: ugc, code: "ROBERTOX", period: 3n, count: 5, merkle_root: ref("m"), per_attribution: new Map([[creatorAddr, 1]]) }), E.Unauthorized));
  await step("get_tally missing period → #15 TallyNotFound", () =>
    wantCode(() => pub.get_tally({ campaign_id: ugc, code: "ROBERTOX", period: 99n }), E.TallyNotFound));

  // ── payouts + settlement ─────────────────────────────────────────
  await step("compute_payouts previews count*rate", async () => {
    const ps = await okRead(() => pub.compute_payouts({ campaign_id: ugc, code: "ROBERTOX", period: 1n }));
    assert(ps.length === 1, "one payout");
    assert(ps[0].to === creatorAddr, "recipient");
    assert(ps[0].amount === 3000n, "3 * 1000 = 3000 stroops");
  });
  await step("settle pays the attributed creator (real transfer)", async () => {
    const ps = await okWrite(() => O.settle({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", period: 1n }));
    assert(ps.length === 1, "one payout made");
    assert(ps[0].to === creatorAddr, "paid creator");
    assert(ps[0].amount === 3000n, "paid 3000 stroops");
  });
  await step("settle again → #16 AlreadySettled", () =>
    wantCode(() => O.settle({ owner: ownerAddr, campaign_id: ugc, code: "ROBERTOX", period: 1n }), E.AlreadySettled));
  await step("settle count-only code → #18 InvalidSettlement", async () => {
    await okWrite(() => O.commit_tally({ owner: ownerAddr, campaign_id: ugc, code: "COUNTONLY", period: 2n, count: 5, merkle_root: ref("m2"), per_attribution: new Map([[creatorAddr, 1]]) }));
    await wantCode(() => O.settle({ owner: ownerAddr, campaign_id: ugc, code: "COUNTONLY", period: 2n }), E.InvalidSettlement);
  });
  await step("bump_tally ok; unknown shared → #12 SharedNotFound", async () => {
    await okWrite(() => C.bump_tally({ campaign_id: ugc, code: "ROBERTOX", periods: [1n] }));
    await wantCode(() => C.bump_tally({ campaign_id: ugc, code: "GHOST", periods: [1n] }), E.SharedNotFound);
  });

  // ════════════════════════════════════════════════════════════════
  // EXPIRY assertions (campaign created at start has now lapsed)
  // ════════════════════════════════════════════════════════════════
  await step("expired campaign rejects writes (#4 CampaignExpired)", async () => {
    while (BigInt(Math.floor(Date.now() / 1000)) <= expValidUntil + 3n) await sleep(1000);
    await wantCode(() => O.issue_unique({ owner: ownerAddr, campaign_id: expID, codes: ["LATE0001"] }), E.CampaignExpired);
    await wantCode(() => O.redeem_unique({ authorizer: ownerAddr, campaign_id: expID, code: "EXP00001", redeemer_ref_hash: ref("late") }), E.CampaignExpired);
    await wantCode(() => O.register_shared({ owner: ownerAddr, campaign_id: expID, code: "LATESHARE", attributed_to: undefined, payout_token: undefined, payout_rate: 0n }), E.CampaignExpired);
  });

  console.log(`\n${pass} passed, ${fail} failed (of ${pass + fail} scenarios)`);
  if (fail > 0) process.exit(1);
  console.log("ALL E2E SCENARIOS PASSED ✔");
}

main().catch((e) => { console.error("FATAL:", e); process.exit(1); });
