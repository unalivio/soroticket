/**
 * @sorodeal/sdk — low-level browser client (Freighter signing).
 *
 * This is a hand-written client over raw ScVal encoding + Soroban RPC. Unlike
 * the generated typed `Client` (see ./contract), it surfaces the *contract
 * error code* at simulation time: view calls that panic (e.g. `verify` on an
 * unknown code) reject with an `Error` whose `.contractCode` is set, so a UI
 * can map it to a friendly message. The typed bindings swallow that code for
 * simulation errors — hence this client exists for the playground / browser
 * dapps that need precise per-error UX.
 *
 * For server-side or strongly-typed use, prefer `sorodeal()` (the typed
 * `Client`) from ./helpers.
 */
import * as StellarTyped from "@stellar/stellar-sdk";

// The XDR builder chains and the `rpc`/`SorobanRpc` namespace shuffle across
// SDK majors are deliberately consumed dynamically — typing them buys nothing.
const Stellar: any = StellarTyped;

// Freighter is lazy-imported (it touches browser globals): this keeps the SDK
// barrel safe to import from Node, where only reads / the typed Client run.
const loadFreighter = async (): Promise<any> => await import("@stellar/freighter-api");

const {
  Contract, TransactionBuilder, BASE_FEE, Account,
  nativeToScVal, scValToNative, xdr,
} = Stellar;

// rpc namespace moved around across SDK majors — support both.
const rpc = Stellar.rpc || Stellar.SorobanRpc;

/** Network + addressing config for the low-level browser client. */
export interface FreighterClientConfig {
  /** Deployed coupon-ledger contract id (`C...`). */
  contractId: string;
  /** Soroban RPC endpoint. */
  rpcUrl: string;
  /** Network passphrase (must match the connected Freighter network). */
  networkPassphrase: string;
  /**
   * Any existing account on this network. Used purely as the source for
   * read-only simulations — no signature, sequence is irrelevant.
   */
  readSource: string;
  /** Horizon endpoint for balance lookups. Defaults to testnet Horizon. */
  horizonUrl?: string;
}

/**
 * Extract a contract error code from a host-error string/message. Contract
 * errors surface at simulation as a string containing `Error(Contract, #N)`.
 * Returns the numeric code, or `undefined` if none is present. Exported so
 * typed-`Client` users can recover the code the bindings drop.
 */
export function contractErrorCode(raw: unknown): number | undefined {
  const s = typeof raw === "string" ? raw : ((raw as any)?.message ?? JSON.stringify(raw));
  const m = /Error\(Contract,\s*#(\d+)\)/.exec(s);
  return m ? Number(m[1]) : undefined;
}

/**
 * Build a low-level browser client bound to one deployment. Returns the
 * read/write primitives, Freighter wallet helpers, ScVal encoders, and the
 * Tally/Burn convenience methods. Errors thrown by reads/writes carry
 * `.contractCode` when the contract panicked.
 */
export function freighterClient(cfg: FreighterClientConfig) {
  const horizonUrl = cfg.horizonUrl ?? "https://horizon-testnet.stellar.org";
  const server = () => new rpc.Server(cfg.rpcUrl);

  /* ── ScVal encoders (match contracts/coupon-ledger/src/lib.rs types) ── */
  const scAddr = (a: string) => nativeToScVal(a, { type: "address" });
  const scStr = (s: unknown) => nativeToScVal(String(s), { type: "string" });
  const scU64 = (n: number | bigint | string) => nativeToScVal(BigInt(n as any), { type: "u64" });
  const scU32 = (n: number | string) => nativeToScVal(Number(n), { type: "u32" });
  const scVecStr = (arr: string[]) => xdr.ScVal.scvVec((arr || []).map((c) => scStr(c)));
  const scBytes32 = (hex: string) => nativeToScVal(hexToBytes(hex), { type: "bytes" }); // BytesN<32>
  // i128 from a decimal string/BigInt — exact, never via Number (avoids precision loss)
  const scI128 = (n: unknown) =>
    nativeToScVal(BigInt(String(n ?? "0").trim().split(".")[0] || "0"), { type: "i128" });
  const scVecU64 = (arr: (number | bigint | string)[]) =>
    xdr.ScVal.scvVec((arr || []).map((n) => scU64(n)));
  const scOptAddr = (a?: string | null) => (a ? scAddr(a) : xdr.ScVal.scvVoid()); // Option<Address>
  const scMapAddrU32 = (pairs: [string, number | string][]) =>
    xdr.ScVal.scvMap(
      (pairs || []).map(([addr, n]) => new xdr.ScMapEntry({ key: scAddr(addr), val: scU32(n) })),
    );

  /* ── error surfacing ────────────────────────────────────────────
     Tag the thrown Error with .contractCode; callers map it to a
     friendly message. */
  function contractErr(raw: unknown): Error & { contractCode?: number } {
    const s = typeof raw === "string" ? raw : ((raw as any)?.message ?? JSON.stringify(raw));
    const e: any = new Error(s);
    const code = contractErrorCode(s);
    if (code !== undefined) e.contractCode = code;
    return e;
  }

  const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

  async function waitForTx(srv: any, hash: string) {
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
  async function read(method: string, args: any[]) {
    const srv = server();
    const source = new Account(cfg.readSource, "0");
    const contract = new Contract(cfg.contractId);
    const tx = new TransactionBuilder(source, { fee: BASE_FEE, networkPassphrase: cfg.networkPassphrase })
      .addOperation(contract.call(method, ...args))
      .setTimeout(60)
      .build();
    const sim = await srv.simulateTransaction(tx);
    if (rpc.Api.isSimulationError(sim)) throw contractErr(sim.error);
    return sim.result ? sim.result.retval : null;
  }

  /* ── write call (simulate → assemble → Freighter sign → send → poll) ── */
  async function write(from: string, method: string, args: any[]) {
    const Freighter = await loadFreighter();
    const srv = server();
    const source = await srv.getAccount(from);
    const contract = new Contract(cfg.contractId);
    const built = new TransactionBuilder(source, { fee: BASE_FEE, networkPassphrase: cfg.networkPassphrase })
      .addOperation(contract.call(method, ...args))
      .setTimeout(60)
      .build();

    const sim = await srv.simulateTransaction(built);
    if (rpc.Api.isSimulationError(sim)) throw contractErr(sim.error);
    const prepared = rpc.assembleTransaction(built, sim).build();

    const signed = await Freighter.signTransaction(prepared.toXDR(), {
      networkPassphrase: cfg.networkPassphrase,
      address: from,
    });
    if (signed && signed.error) {
      const e: any = new Error(String(signed.error.message || signed.error));
      e.sd = { title: "Signing cancelled", msg: "The transaction was not signed in Freighter.", code: "—" };
      throw e;
    }
    const signedXdr = typeof signed === "string" ? signed : signed.signedTxXdr;
    const signedTx = TransactionBuilder.fromXDR(signedXdr, cfg.networkPassphrase);

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
    const Freighter = await loadFreighter();
    let conn;
    try { conn = await Freighter.isConnected(); } catch (_) { conn = false; }
    const installed = conn === true || (conn && conn.isConnected);
    if (!installed) {
      const e: any = new Error("freighter-missing");
      e.sd = { title: "Freighter not found", msg: "Install the Freighter browser extension to sign transactions. You can still use public read calls like verify().", code: "—" };
      throw e;
    }
    const access = await Freighter.requestAccess();
    if (access && access.error) {
      const e: any = new Error(String(access.error));
      e.sd = { title: "Connection rejected", msg: "Freighter access was not granted.", code: "—" };
      throw e;
    }
    const address = (access && access.address) || ((await Freighter.getAddress()) || {}).address;
    if (!address) {
      const e: any = new Error("no-address");
      e.sd = { title: "No account", msg: "Could not read an address from Freighter.", code: "—" };
      throw e;
    }

    // network guard — must match the configured passphrase
    try {
      const details = Freighter.getNetworkDetails ? await Freighter.getNetworkDetails() : await Freighter.getNetwork();
      const pp = (details && (details.networkPassphrase || details.network)) || "";
      if (pp && pp !== cfg.networkPassphrase) {
        const e: any = new Error("wrong-network");
        e.sd = { title: "Wrong network", msg: "Switch Freighter to the Testnet network, then reconnect.", code: "—" };
        throw e;
      }
    } catch (e: any) {
      if (e.sd) throw e; // only rethrow our explicit network error
    }

    const balance = await getBalance(address).catch(() => "—");
    return { address, balance };
  }

  async function getBalance(addr: string) {
    const r = await fetch(horizonUrl + "/accounts/" + addr);
    if (!r.ok) return "—";
    const j = await r.json();
    const n = (j.balances || []).find((b: any) => b.asset_type === "native");
    return n ? Number(n.balance).toLocaleString(undefined, { maximumFractionDigits: 2 }) : "—";
  }

  const toNative = (scv: any) => (scv == null ? null : scValToNative(scv));

  /* ── enumerate an owner's campaigns from the chain (the source of truth) ── */
  async function campaignsOf(owner: string) {
    const ids = toNative(await read("campaigns_of", [scAddr(owner)]));
    return (ids || []).map(Number);
  }
  async function getCampaign(id: number | bigint) {
    const c = toNative(await read("get_campaign", [scU64(id)]));
    return {
      id: Number(c.id), owner: c.owner, name: c.name,
      discount_type: c.discount_type, discount_value: Number(c.discount_value),
      total_supply: Number(c.total_supply), minted: Number(c.minted),
      burned: Number(c.burned), valid_until: Number(c.valid_until),
    };
  }

  /* ── Tally profile (shared codes) — ADR-003/004/011 ── */
  async function registerShared(
    from: string, campaignId: number, code: string,
    attributedTo?: string | null, payoutToken?: string | null, payoutRate?: unknown,
  ) {
    // payout token + rate are fixed at registration (immutable) — settle uses them
    return await write(from, "register_shared", [
      scAddr(from), scU64(campaignId), scStr(code),
      scOptAddr(attributedTo), scOptAddr(payoutToken), scI128(payoutRate),
    ]);
  }
  async function getShared(campaignId: number, code: string) {
    return toNative(await read("get_shared", [scU64(campaignId), scStr(code)]));
  }
  async function commitTally(
    from: string, campaignId: number, code: string, period: number,
    count: number, merkleHex: string, perAttribution: [string, number | string][],
  ) {
    // perAttribution: array of [address, count]
    return await write(from, "commit_tally", [
      scAddr(from), scU64(campaignId), scStr(code), scU64(period), scU32(count),
      scBytes32(merkleHex), scMapAddrU32(perAttribution),
    ]);
  }
  async function getTally(campaignId: number, code: string, period: number) {
    return toNative(await read("get_tally", [scU64(campaignId), scStr(code), scU64(period)]));
  }
  async function computePayouts(campaignId: number, code: string, period: number) {
    // rate comes from the registered shared code (no arbitrary rate)
    return toNative(await read("compute_payouts", [scU64(campaignId), scStr(code), scU64(period)]));
  }
  async function settle(from: string, campaignId: number, code: string, period: number) {
    // token + rate are read from the shared code on-chain (immutable)
    return await write(from, "settle", [scAddr(from), scU64(campaignId), scStr(code), scU64(period)]);
  }
  async function bumpTally(from: string, campaignId: number, code: string, periods: number[]) {
    // public maintenance: re-extend a shared code + its periods' storage TTL (from = fee payer)
    return await write(from, "bump_tally", [scU64(campaignId), scStr(code), scVecU64(periods)]);
  }

  return {
    read, write, connectWallet, getBalance, toNative, bytesToHex,
    campaignsOf, getCampaign,
    registerShared, getShared, commitTally, getTally, computePayouts, settle, bumpTally,
    scAddr, scStr, scU64, scU32, scVecStr, scBytes32, scI128,
  };
}

export type FreighterClient = ReturnType<typeof freighterClient>;

/* ── hex helpers (pure; exported for callers) ─────────────────────── */
export function hexToBytes(hex: string): Uint8Array {
  const s = String(hex || "").replace(/^0x/, "");
  if (!/^[0-9a-fA-F]{64}$/.test(s)) {
    throw new Error("Expected a 32-byte hex value.");
  }
  const out = new Uint8Array(Math.floor(s.length / 2));
  for (let i = 0; i < out.length; i++) out[i] = parseInt(s.substr(i * 2, 2), 16);
  return out;
}
export function bytesToHex(b: unknown): string {
  if (b == null) return "";
  if (typeof b === "string") return b;
  const arr = b instanceof Uint8Array ? b : ((b as any).data ? Uint8Array.from((b as any).data) : Uint8Array.from(b as any));
  return [...arr].map((x) => x.toString(16).padStart(2, "0")).join("");
}
