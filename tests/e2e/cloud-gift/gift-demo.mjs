/**
 * Proof-of-gift / delivery demo — "the wine-bottle problem" over Sorodeal
 * Cloud (docs/USE_CASES.md §1), exercised exactly like a real integrator:
 * sign up, mint an API key, then run everything through the REST API.
 *
 *   A winery gifts bottles to three venues. Diners scan a per-venue code
 *   once per serving. The winery gets tamper-evident per-venue counts,
 *   catches abuse in its own integration layer (one phone scanning
 *   everything), anchors the tallies on-chain and pays each venue per
 *   verified serving through the v0.2 exact-allowance settlement.
 *
 * TEST environment only (free, unmetered). Requires the local Cloud API:
 *
 *   npm install && npm run demo          # API on http://localhost:8787
 *
 * Exit 0 ⇒ every assertion passed.
 */
import { Keypair } from "@stellar/stellar-sdk";

const API = process.env.SORODEAL_API ?? "http://localhost:8787";
const RATE = 1000n; // stroops per verified serving (0.0001 XLM)

// ── tiny harness ─────────────────────────────────────────────────────
let pass = 0, fail = 0;
async function step(name, fn) {
  const t0 = Date.now();
  try {
    const out = await fn();
    pass++;
    console.log(`ok    ${name.padEnd(62)} (${Date.now() - t0}ms)`);
    return out;
  } catch (e) {
    fail++;
    console.log(`FAIL  ${name.padEnd(62)} (${Date.now() - t0}ms)\n        ↳ ${e?.message ?? e}`);
    throw e;
  }
}
const assert = (cond, msg) => { if (!cond) throw new Error(`assertion failed: ${msg}`); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ── HTTP client: session cookie for onboarding, API key for the rest ──
let cookie = "";
let apiKey = "";
async function req(method, path, body, { auth = "key", expect } = {}) {
  const headers = { "X-Env": "test" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (auth === "session" && cookie) headers.Cookie = cookie;
  if (auth === "key" && apiKey) headers.Authorization = `Bearer ${apiKey}`;
  const res = await fetch(API + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const setCookie = res.headers.get("set-cookie");
  if (setCookie) cookie = setCookie.split(";")[0];
  const text = await res.text();
  const json = text ? JSON.parse(text) : null;
  if (expect !== undefined) {
    if (res.status !== expect) throw new Error(`${method} ${path} → ${res.status} (want ${expect}): ${text.slice(0, 200)}`);
    return json;
  }
  if (!res.ok) throw new Error(`${method} ${path} → ${res.status}: ${text.slice(0, 200)}`);
  return json;
}

async function fund(addr) {
  await fetch("https://friendbot.stellar.org/?addr=" + encodeURIComponent(addr)).catch(() => {});
  for (let i = 0; i < 40; i++) {
    const r = await fetch("https://horizon-testnet.stellar.org/accounts/" + addr).catch(() => null);
    if (r && r.status === 200) return;
    await sleep(1000);
  }
  throw new Error(`venue account ${addr} not visible after funding`);
}

// campaign creation retries while the custodial org account is being funded
async function createWithFundingRetry(path, body) {
  for (let i = 0; i < 30; i++) {
    const res = await fetch(API + path, {
      method: "POST",
      headers: { "X-Env": "test", "Content-Type": "application/json", Authorization: `Bearer ${apiKey}` },
      body: JSON.stringify(body),
    });
    const json = JSON.parse(await res.text());
    if (res.ok) return json;
    if (String(json?.message ?? "").includes("still being set up")) { await sleep(2000); continue; }
    throw new Error(`POST ${path} → ${res.status}: ${JSON.stringify(json).slice(0, 200)}`);
  }
  throw new Error("custodial account was never funded");
}

// ═════════════════════════════════════════════════════════════════════
async function main() {
  console.log("Sorodeal Cloud — proof-of-gift/delivery demo (wine bottles)");
  console.log(`api: ${API} · env: test\n`);

  // ── onboarding: the developer path ─────────────────────────────────
  const email = `bodega-demo-${Date.now()}@example.test`;
  await step("winery signs up (email/password)", () =>
    req("POST", "/auth/signup", { email, password: "gift-demo-password-123" }, { auth: "session" }));
  await step("winery creates its organization", () =>
    req("POST", "/orgs", { name: "Bodega Río Claro (demo)" }, { auth: "session" }));
  const key = await step("winery mints an API key (sk_test_…)", async () => {
    const k = await req("POST", "/v1/keys", { label: "gift-demo" }, { auth: "session" });
    assert(k.key?.startsWith("sk_test_"), "test-mode key");
    return k;
  });
  apiKey = key.key;

  // ── venues: three restaurants with their own Stellar addresses ─────
  const venues = [
    { name: "Café Rosales", code: "COPA-ROSALES", kp: Keypair.random() },
    { name: "Don Mario", code: "COPA-DONMARIO", kp: Keypair.random() },
    { name: "La Esquina", code: "COPA-LAESQUINA", kp: Keypair.random() },
  ];
  await step("fund 3 venue accounts via friendbot (payout targets)", () =>
    Promise.all(venues.map((v) => fund(v.kp.publicKey()))));

  // ── the gift campaign: one code per venue, paid per verified serving ─
  const campaign = await step("create gift campaign (kind=gift, venue 1 attributed)", async () => {
    const c = await createWithFundingRetry("/v1/campaigns", {
      kind: "gift", name: "Copas de cortesía — Río Claro",
      discount_type: "free_item", discount_value: 1, total_supply: 0,
      valid_until: Math.floor(Date.now() / 1000) + 90 * 86400,
      shared: { code: venues[0].code, attributed_to: venues[0].kp.publicKey(), payout_rate: RATE.toString() },
    });
    assert(c.kind === "gift", "kind persisted");
    assert(c.contract_id?.startsWith("C"), "stamped with the current contract");
    return c;
  });
  await step("register venue 2 and 3 codes on the same campaign", async () => {
    for (const v of venues.slice(1)) {
      await req("POST", `/v1/campaigns/${campaign.id}/shared-codes`,
        { code: v.code, attributed_to: v.kp.publicKey(), payout_rate: RATE.toString() });
    }
  });

  // ── scans: diners' phones — the integrator layer keeps the mapping ──
  // ledger: phone → { scans, venue, commitments[], leaves[] }
  const ledger = new Map();
  async function scan(venue, phone, bottle, glass) {
    const r = await req("POST", `/v1/shared-codes/${campaign.id}/${venue.code}/events`,
      { customer_ref: phone, order_ref: `LOTE7-${bottle}#${glass}` });
    // the API inlines the canonical receipt JSON; tolerate a string form too
    const payload = typeof r.receipt.payload === "string" ? JSON.parse(r.receipt.payload) : r.receipt.payload;
    const entry = ledger.get(phone) ?? { scans: 0, venue: venue.name, commitments: new Set(), leaves: [] };
    entry.scans++;
    if (payload.customer_commitment) entry.commitments.add(payload.customer_commitment);
    entry.leaves.push(r.receipt.leaf_hash);
    ledger.set(phone, entry);
    return r;
  }

  await step("venue 1: 3 bottles × 4 servings, distinct diners (12 scans)", async () => {
    let phone = 5841200001;
    for (let b = 1; b <= 3; b++) for (let g = 1; g <= 4; g++) await scan(venues[0], `+${phone++}`, `B${b}`, `C${g}`);
  });
  await step("venue 2: 2 bottles × 3 servings, distinct diners (6 scans)", async () => {
    let phone = 5841300001;
    for (let b = 1; b <= 2; b++) for (let g = 1; g <= 3; g++) await scan(venues[1], `+${phone++}`, `B${b}`, `C${g}`);
  });
  await step("venue 3: ONE phone scans 15 servings + 5 legit diners (20 scans)", async () => {
    const cheater = "+584129999999";
    let n = 0;
    for (let b = 1; b <= 3; b++) for (let g = 1; g <= 5; g++) { n++; await scan(venues[2], cheater, `B${b}`, `C${g}`); }
    let phone = 5841400001;
    for (let g = 1; g <= 5; g++) await scan(venues[2], `+${phone++}`, "B4", `C${g}`);
    assert(n === 15, "cheater volume");
  });
  await step("same serving cannot count twice (order_ref dedup → 409)", async () => {
    await req("POST", `/v1/shared-codes/${campaign.id}/${venues[0].code}/events`,
      { customer_ref: "+584125555555", order_ref: "LOTE7-B1#C1" }, { expect: 409 });
  });

  // ── integrator-layer abuse detection (docs/USE_CASES.md: layer 3) ──
  const cheaters = await step("integrator detects abuse from its own ledger", async () => {
    const flagged = [...ledger.entries()].filter(([, e]) => e.scans > 6);
    assert(flagged.length === 1, "exactly one phone above threshold");
    const [phone, e] = flagged[0];
    assert(e.scans === 15 && e.venue === "La Esquina", "the cheater is venue 3's phone");
    return flagged.map(([p, x]) => ({ phone: p, scans: x.scans, venue: x.venue, commitment: [...x.commitments][0]?.slice(0, 16) }));
  });

  // ── anchor: one on-chain commit per venue code ──────────────────────
  const commits = {};
  await step("commit all three tallies on-chain (v0.2, exact attribution)", async () => {
    for (const v of venues) {
      commits[v.code] = await req("POST", `/v1/shared-codes/${campaign.id}/${v.code}/commits`, {});
      assert(commits[v.code].tx_hash?.length === 64, `commit tx for ${v.code}`);
    }
    assert(commits[venues[0].code].count === 12, "venue 1 count");
    assert(commits[venues[1].code].count === 6, "venue 2 count");
    assert(commits[venues[2].code].count === 20, "venue 3 count (abuse included — the record is honest)");
  });

  // ── public audit: anyone can re-verify, no auth ─────────────────────
  await step("public audit endpoint re-proves receipts + inclusion", async () => {
    for (const v of venues) {
      const c = commits[v.code];
      const a = await fetch(`${API}/v1/audit/tallies/${campaign.chain_id}/${v.code}/${c.period}`).then((r) => r.json());
      assert(a.merkle_root === c.merkle_root, `root matches for ${v.code}`);
      assert(a.count === c.count, `count matches for ${v.code}`);
      assert(Array.isArray(a.receipts) && a.receipt_total === c.receipt_count, `receipt set for ${v.code}`);
    }
    // one leaf we captured at write time is present in the published set
    const anyLeaf = ledger.values().next().value.leaves[0];
    const a0 = await fetch(`${API}/v1/audit/tallies/${campaign.chain_id}/${venues[0].code}/${commits[venues[0].code].period}`).then((r) => r.json());
    assert(a0.receipts.some((r) => r.leaf_hash === anyLeaf), "write-time leaf is in the audited set");
  });

  // ── settlement: pay venues per verified serving (exact allowance) ──
  await step("settle all venues — approve exact total, keeper-style payout", async () => {
    for (const v of venues) {
      const expected = BigInt(commits[v.code].count) * RATE;
      const s = await req("POST", "/v1/settlements", { campaign_id: campaign.id, code: v.code, period: commits[v.code].period });
      assert(BigInt(s.total) === expected, `payout for ${v.code} == count × rate`);
      assert(s.payouts[0].to === v.kp.publicKey(), `paid to ${v.name}`);
      v.settleTx = s.tx_hash;
      v.paid = s.total;
    }
  });

  // ── the business decision, informed by all three layers ────────────
  console.log("\n── winery report ────────────────────────────────────────────");
  for (const v of venues) {
    const c = commits[v.code];
    const suspended = cheaters.some((x) => x.venue === v.name);
    console.log(`  ${v.name.padEnd(14)} ${String(c.count).padStart(3)} servings · paid ${v.paid} stroops · ${suspended ? "⚠ SUSPEND DELIVERIES (abuse detected)" : "OK"}`);
    console.log(`    commit ${c.tx_hash.slice(0, 16)}… · settle ${v.settleTx.slice(0, 16)}… · root ${c.merkle_root.slice(0, 16)}…`);
  }
  for (const ch of cheaters) {
    console.log(`  abuse evidence: ${ch.phone} scanned ${ch.scans}× at ${ch.venue} (commitment ${ch.commitment}…)`);
    console.log(`  bounded fraud: cheater extracted ${15n * RATE} stroops of incentive — capped by design; deliveries stop.`);
  }
  console.log(`\n${pass} passed, ${fail} failed`);
  if (fail > 0) process.exit(1);
  console.log("PROOF-OF-GIFT DEMO PASSED ✔");
}

main().catch((e) => { console.error("\nFATAL:", e?.message ?? e); process.exit(1); });
