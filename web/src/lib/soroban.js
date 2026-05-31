/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — real Soroban integration → window.SDK
   Reads/writes the live testnet contract and signs with Freighter.
   The simulated store.jsx is replaced by calls into this module.
   ═══════════════════════════════════════════════════════════════════ */
import * as Stellar from "@stellar/stellar-sdk";
import * as Freighter from "@stellar/freighter-api";

const {
  Contract, TransactionBuilder, BASE_FEE, Account,
  nativeToScVal, scValToNative, xdr,
} = Stellar;

// rpc namespace moved around across SDK majors — support both.
const rpc = Stellar.rpc || Stellar.SorobanRpc;

/* ── config (single source of truth = window.SD.NET, read lazily) ── */
const NET = () => (window.SD && window.SD.NET) || {};
const server = () => new rpc.Server(NET().rpc);
const PASSPHRASE = () => NET().passphrase;
const CID = () => NET().contractId;
// Any existing testnet account works as the "source" for read-only
// simulations (no signature, sequence is irrelevant). We use the deployer.
const READ_SOURCE = () => NET().ownerWallet;

/* ── ScVal encoders (match contracts/coupon-ledger/src/lib.rs types) ── */
const scAddr = (a) => nativeToScVal(a, { type: "address" });
const scStr = (s) => nativeToScVal(String(s), { type: "string" });
const scU64 = (n) => nativeToScVal(BigInt(n), { type: "u64" });
const scU32 = (n) => nativeToScVal(Number(n), { type: "u32" });
const scVecStr = (arr) => xdr.ScVal.scvVec((arr || []).map((c) => scStr(c)));
const scBytes32 = (hex) => nativeToScVal(hexToBytes(hex), { type: "bytes" }); // BytesN<32>

/* ── hex helpers ──────────────────────────────────────────────── */
function hexToBytes(hex) {
  const s = String(hex || "").replace(/^0x/, "");
  if (!/^[0-9a-fA-F]{64}$/.test(s)) {
    throw new Error("Expected a 32-byte hex value.");
  }
  const out = new Uint8Array(Math.floor(s.length / 2));
  for (let i = 0; i < out.length; i++) out[i] = parseInt(s.substr(i * 2, 2), 16);
  return out;
}
function bytesToHex(b) {
  if (b == null) return "";
  if (typeof b === "string") return b;
  const arr = b instanceof Uint8Array ? b : (b.data ? Uint8Array.from(b.data) : Uint8Array.from(b));
  return [...arr].map((x) => x.toString(16).padStart(2, "0")).join("");
}

/* ── error surfacing ──────────────────────────────────────────────
   Contract errors (Unauthorized, AlreadyRedeemed, …) surface at
   SIMULATION as a host-error string containing "Error(Contract, #N)".
   We tag the thrown Error with .contractCode; store.jsx maps it to the
   friendly SD.ERRORS entry. */
function contractErr(raw) {
  const s = typeof raw === "string" ? raw : (raw && raw.message) || JSON.stringify(raw);
  const m = /Error\(Contract,\s*#(\d+)\)/.exec(s);
  const e = new Error(s);
  if (m) e.contractCode = Number(m[1]);
  return e;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function waitForTx(srv, hash) {
  let res = await srv.getTransaction(hash);
  let tries = 0;
  while (res.status === "NOT_FOUND" && tries < 20) {
    await sleep(900);
    res = await srv.getTransaction(hash);
    tries++;
  }
  return res;
}

/* ── read-only call (simulate, no signature) ──────────────────── */
async function read(method, args) {
  const srv = server();
  const source = new Account(READ_SOURCE(), "0");
  const contract = new Contract(CID());
  const tx = new TransactionBuilder(source, { fee: BASE_FEE, networkPassphrase: PASSPHRASE() })
    .addOperation(contract.call(method, ...args))
    .setTimeout(60)
    .build();
  const sim = await srv.simulateTransaction(tx);
  if (rpc.Api.isSimulationError(sim)) throw contractErr(sim.error);
  return sim.result ? sim.result.retval : null;
}

/* ── write call (simulate → assemble → Freighter sign → send → poll) ── */
async function write(from, method, args) {
  const srv = server();
  const source = await srv.getAccount(from);
  const contract = new Contract(CID());
  const built = new TransactionBuilder(source, { fee: BASE_FEE, networkPassphrase: PASSPHRASE() })
    .addOperation(contract.call(method, ...args))
    .setTimeout(60)
    .build();

  const sim = await srv.simulateTransaction(built);
  if (rpc.Api.isSimulationError(sim)) throw contractErr(sim.error);
  const prepared = rpc.assembleTransaction(built, sim).build();

  const signed = await Freighter.signTransaction(prepared.toXDR(), {
    networkPassphrase: PASSPHRASE(),
    address: from,
  });
  if (signed && signed.error) {
    const e = new Error(String(signed.error.message || signed.error));
    e.sd = { title: "Signing cancelled", msg: "The transaction was not signed in Freighter.", code: "—" };
    throw e;
  }
  const signedXdr = typeof signed === "string" ? signed : signed.signedTxXdr;
  const signedTx = TransactionBuilder.fromXDR(signedXdr, PASSPHRASE());

  const sent = await srv.sendTransaction(signedTx);
  if (sent.status === "ERROR") throw contractErr(JSON.stringify(sent.errorResult || sent));

  const got = await waitForTx(srv, sent.hash);
  if (got.status !== "SUCCESS") {
    let detail = "Transaction failed on-chain.";
    try { detail = got.resultXdr ? got.resultXdr.toXDR("base64") : detail; } catch (_) {}
    throw contractErr(detail);
  }
  return { retval: got.returnValue, tx: sent.hash, seq: got.ledger };
}

/* ── wallet (Freighter) ───────────────────────────────────────── */
async function connectWallet() {
  let conn;
  try { conn = await Freighter.isConnected(); } catch (_) { conn = false; }
  const installed = conn === true || (conn && conn.isConnected);
  if (!installed) {
    const e = new Error("freighter-missing");
    e.sd = { title: "Freighter not found", msg: "Install the Freighter browser extension to sign transactions. You can still use public read calls like verify().", code: "—" };
    throw e;
  }
  const access = await Freighter.requestAccess();
  if (access && access.error) {
    const e = new Error(String(access.error));
    e.sd = { title: "Connection rejected", msg: "Freighter access was not granted.", code: "—" };
    throw e;
  }
  const address = (access && access.address) || ((await Freighter.getAddress()) || {}).address;
  if (!address) {
    const e = new Error("no-address");
    e.sd = { title: "No account", msg: "Could not read an address from Freighter.", code: "—" };
    throw e;
  }

  // network guard — must be Testnet
  try {
    const details = Freighter.getNetworkDetails ? await Freighter.getNetworkDetails() : await Freighter.getNetwork();
    const pp = (details && (details.networkPassphrase || details.network)) || "";
    if (pp && pp !== PASSPHRASE()) {
      const e = new Error("wrong-network");
      e.sd = { title: "Wrong network", msg: "Switch Freighter to the Testnet network, then reconnect.", code: "—" };
      throw e;
    }
  } catch (e) {
    if (e.sd) throw e; // only rethrow our explicit network error
  }

  const balance = await getBalance(address).catch(() => "—");
  return { address, balance };
}

async function getBalance(addr) {
  const r = await fetch("https://horizon-testnet.stellar.org/accounts/" + addr);
  if (!r.ok) return "—";
  const j = await r.json();
  const n = (j.balances || []).find((b) => b.asset_type === "native");
  return n ? Number(n.balance).toLocaleString(undefined, { maximumFractionDigits: 2 }) : "—";
}

const toNative = (scv) => (scv == null ? null : scValToNative(scv));

/* ── enumerate an owner's campaigns from the chain (the source of truth) ── */
async function campaignsOf(owner) {
  const ids = toNative(await read("campaigns_of", [scAddr(owner)]));
  return (ids || []).map(Number);
}
async function getCampaign(id) {
  const c = toNative(await read("get_campaign", [scU64(id)]));
  return {
    id: Number(c.id), owner: c.owner, name: c.name,
    discount_type: c.discount_type, discount_value: Number(c.discount_value),
    total_supply: Number(c.total_supply), minted: Number(c.minted),
    burned: Number(c.burned), valid_until: Number(c.valid_until),
  };
}

/* ── Tally profile (shared codes) — ADR-003/004/011 ── */
const scI128 = (n) => nativeToScVal(BigInt(n), { type: "i128" });
const scOptAddr = (a) => (a ? scAddr(a) : xdr.ScVal.scvVoid()); // Option<Address>: Some|None
const scMapAddrU32 = (pairs) =>
  xdr.ScVal.scvMap(
    (pairs || []).map(([addr, n]) => new xdr.ScMapEntry({ key: scAddr(addr), val: scU32(n) }))
  );

async function registerShared(from, campaignId, code, attributedTo) {
  return await write(from, "register_shared", [
    scAddr(from), scU64(campaignId), scStr(code), scOptAddr(attributedTo),
  ]);
}
async function getShared(campaignId, code) {
  return toNative(await read("get_shared", [scU64(campaignId), scStr(code)]));
}
async function commitTally(from, campaignId, code, period, count, merkleHex, perAttribution) {
  // perAttribution: array of [address, count]
  return await write(from, "commit_tally", [
    scAddr(from), scU64(campaignId), scStr(code), scU64(period), scU32(count),
    scBytes32(merkleHex), scMapAddrU32(perAttribution),
  ]);
}
async function getTally(campaignId, code, period) {
  return toNative(await read("get_tally", [scU64(campaignId), scStr(code), scU64(period)]));
}
async function computePayouts(campaignId, code, period, rate) {
  return toNative(await read("compute_payouts", [scU64(campaignId), scStr(code), scU64(period), scI128(rate)]));
}
async function settle(from, campaignId, code, period, token, rate) {
  return await write(from, "settle", [
    scAddr(from), scU64(campaignId), scStr(code), scU64(period), scAddr(token), scI128(rate),
  ]);
}

window.SDK = {
  read, write, connectWallet, getBalance, toNative, bytesToHex,
  campaignsOf, getCampaign,
  registerShared, getShared, commitTally, getTally, computePayouts, settle,
  scAddr, scStr, scU64, scU32, scVecStr, scBytes32, scI128,
};
