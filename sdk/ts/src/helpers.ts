/**
 * @soroticket/sdk — ergonomic layer over the generated, typed contract client.
 * Network presets, signer helpers (Freighter for browsers, keypair for servers),
 * and the off-chain redeemer commitment. The typed `Client` (all contract
 * methods + types + Errors) is re-exported from `./contract`.
 */
import {
  Address,
  BASE_FEE,
  Contract,
  Keypair,
  nativeToScVal,
  rpc,
  TransactionBuilder,
} from "@stellar/stellar-sdk";
import { basicNodeSigner, type ClientOptions } from "@stellar/stellar-sdk/contract";
import { Buffer } from "buffer";
import { Client } from "./contract.js";

/** Deprecated immutable v0.1 testnet deployment. Never use for real value. */
export const LEGACY_TESTNET = {
  contractId: "CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX",
  networkPassphrase: "Test SDF Network ; September 2015",
  rpcUrl: "https://soroban-testnet.stellar.org",
} as const;

/**
 * Current reviewed v0.2.0 testnet deployment (2026-07-12; see
 * deployments/testnet-v0.2.0.json). Testnet preview only — never real value.
 */
export const TESTNET = {
  contractId: "CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF",
  networkPassphrase: "Test SDF Network ; September 2015",
  rpcUrl: "https://soroban-testnet.stellar.org",
} as const;

export type SoroticketOptions = Partial<ClientOptions>;

/**
 * Build a typed Soroticket client (defaults to testnet). Read-only calls need no
 * signer; writes need `publicKey` + `signTransaction` (see keypairSigner /
 * freighterSigner). Override `contractId`/`rpcUrl`/`networkPassphrase` for other
 * deployments.
 */
export function soroticket(opts: SoroticketOptions = {}): Client {
  return new Client({
    contractId: TESTNET.contractId,
    networkPassphrase: TESTNET.networkPassphrase,
    rpcUrl: TESTNET.rpcUrl,
    ...opts,
  } as ClientOptions);
}

/** Server/back-end signer from a secret seed (`S...`). */
export function keypairSigner(secret: string, networkPassphrase: string = TESTNET.networkPassphrase) {
  const kp = Keypair.fromSecret(secret);
  return { publicKey: kp.publicKey(), ...basicNodeSigner(kp, networkPassphrase) };
}

/** Browser signer via the Freighter extension (lazy-imported). */
export async function freighterSigner(networkPassphrase: string = TESTNET.networkPassphrase) {
  const fr: any = await import("@stellar/freighter-api");
  const access = await fr.requestAccess();
  if (access?.error) throw new Error(String(access.error));
  const publicKey: string = access?.address ?? (await fr.getAddress())?.address;
  return {
    publicKey,
    signTransaction: (xdr: string, opts?: Record<string, unknown>) =>
      fr.signTransaction(xdr, { networkPassphrase, address: publicKey, ...opts }),
  };
}

/**
 * Grant the Soroticket contract an exact settlement allowance on a SAC payout
 * token (contract v0.2+). The owner signs `approve(owner, soroticket, amount,
 * live_until_ledger)` on the token; any keeper can then trigger `settle` for
 * the committed period, which consumes the allowance via `transfer_from`.
 * Approve only the exact period total right before settling — no standing
 * spend authority should be left behind (`live_until_ledger` defaults to
 * ~latest + 200).
 */
export async function approveSettlement(opts: {
  ownerSecret: string;
  payoutToken: string;
  amount: bigint;
  liveUntilLedger?: number;
  contractId?: string;
  rpcUrl?: string;
  networkPassphrase?: string;
}): Promise<void> {
  const {
    ownerSecret,
    payoutToken,
    amount,
    contractId = TESTNET.contractId,
    rpcUrl = TESTNET.rpcUrl,
    networkPassphrase = TESTNET.networkPassphrase,
  } = opts;
  const kp = Keypair.fromSecret(ownerSecret);
  const server = new rpc.Server(rpcUrl);
  const live =
    opts.liveUntilLedger ?? (await server.getLatestLedger()).sequence + 200;
  const source = await server.getAccount(kp.publicKey());
  const tx = new TransactionBuilder(source, { fee: BASE_FEE, networkPassphrase })
    .addOperation(
      new Contract(payoutToken).call(
        "approve",
        new Address(kp.publicKey()).toScVal(),
        new Address(contractId).toScVal(),
        nativeToScVal(amount, { type: "i128" }),
        nativeToScVal(live, { type: "u32" }),
      ),
    )
    .setTimeout(60)
    .build();
  // The owner is the transaction source, so simulation resolves the token's
  // require_auth to source-account credentials — no extra auth signing needed.
  const prepared = await server.prepareTransaction(tx);
  prepared.sign(kp);
  const sent = await server.sendTransaction(prepared);
  if (sent.status === "ERROR") {
    throw new Error(`approve submission failed: ${sent.status}`);
  }
  for (let i = 0; i < 30; i++) {
    const res = await server.getTransaction(sent.hash);
    if (res.status === "SUCCESS") return;
    if (res.status === "FAILED") throw new Error("approve transaction failed on-chain");
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error("approve transaction was not confirmed in time");
}

/**
 * Opaque, non-reversible redeemer commitment for `redeem_unique`:
 * `SHA-256(nonce ∥ "|" ∥ ref)` with a random per-redemption nonce. No plaintext
 * PII goes on-chain; keep `nonce` off-chain if you need to prove the ref later
 * (ADR-005/010). Returns the 32-byte hash to pass as `redeemer_ref_hash`.
 */
export async function redeemerCommitment(
  ref: string,
  nonceHex?: string,
): Promise<{ nonce: string; hash: Buffer }> {
  if (nonceHex !== undefined && !/^[0-9a-fA-F]{32}$/.test(nonceHex)) {
    throw new RangeError("nonceHex must contain exactly 16 bytes (32 hex characters)");
  }
  const nonce = nonceHex?.toLowerCase() ?? randomNonceHex();
  const data = new TextEncoder().encode(nonce + "|" + ref);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return { nonce, hash: Buffer.from(new Uint8Array(digest)) };
}

function randomNonceHex(): string {
  const a = new Uint8Array(16);
  crypto.getRandomValues(a);
  return Array.from(a, (b) => b.toString(16).padStart(2, "0")).join("");
}
