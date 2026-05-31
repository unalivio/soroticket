/**
 * @sorodeal/sdk — ergonomic layer over the generated, typed contract client.
 * Network presets, signer helpers (Freighter for browsers, keypair for servers),
 * and the off-chain redeemer commitment. The typed `Client` (all contract
 * methods + types + Errors) is re-exported from `./contract`.
 */
import { Keypair } from "@stellar/stellar-sdk";
import { basicNodeSigner, type ClientOptions } from "@stellar/stellar-sdk/contract";
import { Buffer } from "buffer";
import { Client } from "./contract.js";

/** Live testnet deployment (see deployments/testnet.json). */
export const TESTNET = {
  contractId: "CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX",
  networkPassphrase: "Test SDF Network ; September 2015",
  rpcUrl: "https://soroban-testnet.stellar.org",
} as const;

export type SorodealOptions = Partial<ClientOptions>;

/**
 * Build a typed Sorodeal client (defaults to testnet). Read-only calls need no
 * signer; writes need `publicKey` + `signTransaction` (see keypairSigner /
 * freighterSigner). Override `contractId`/`rpcUrl`/`networkPassphrase` for other
 * deployments.
 */
export function sorodeal(opts: SorodealOptions = {}): Client {
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
 * Opaque, non-reversible redeemer commitment for `redeem_unique`:
 * `SHA-256(nonce ∥ "|" ∥ ref)` with a random per-redemption nonce. No plaintext
 * PII goes on-chain; keep `nonce` off-chain if you need to prove the ref later
 * (ADR-005/010). Returns the 32-byte hash to pass as `redeemer_ref_hash`.
 */
export async function redeemerCommitment(
  ref: string,
  nonceHex?: string,
): Promise<{ nonce: string; hash: Buffer }> {
  const nonce = nonceHex ?? randomNonceHex();
  const data = new TextEncoder().encode(nonce + "|" + ref);
  const digest = await crypto.subtle.digest("SHA-256", data);
  return { nonce, hash: Buffer.from(new Uint8Array(digest)) };
}

function randomNonceHex(): string {
  const a = new Uint8Array(16);
  crypto.getRandomValues(a);
  return Array.from(a, (b) => b.toString(16).padStart(2, "0")).join("");
}
