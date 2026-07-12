import { Buffer } from "buffer";
import { Address } from "@stellar/stellar-sdk";
import {
  AssembledTransaction,
  Client as ContractClient,
  ClientOptions as ContractClientOptions,
  MethodOptions,
  Result,
  Spec as ContractSpec,
} from "@stellar/stellar-sdk/contract";
import type {
  u32,
  i32,
  u64,
  i64,
  u128,
  i128,
  u256,
  i256,
  Option,
  Timepoint,
  Duration,
} from "@stellar/stellar-sdk/contract";
export * from "@stellar/stellar-sdk";
export * as contract from "@stellar/stellar-sdk/contract";
export * as rpc from "@stellar/stellar-sdk/rpc";

if (typeof window !== "undefined") {
  //@ts-ignore Buffer exists
  window.Buffer = window.Buffer || Buffer;
}




export const Errors = {
  1: {message:"CampaignNotFound"},
  2: {message:"CouponNotFound"},
  3: {message:"AlreadyRedeemed"},
  4: {message:"CampaignExpired"},
  5: {message:"SupplyExhausted"},
  6: {message:"Unauthorized"},
  7: {message:"InvalidCode"},
  8: {message:"DuplicateCode"},
  9: {message:"InvalidTerms"},
  10: {message:"BatchTooLarge"},
  11: {message:"CodeTooLong"},
  12: {message:"SharedNotFound"},
  13: {message:"AlreadyRegistered"},
  14: {message:"PeriodCommitted"},
  15: {message:"TallyNotFound"},
  16: {message:"AlreadySettled"},
  17: {message:"InvalidTally"},
  18: {message:"InvalidSettlement"},
  19: {message:"AttributionMismatch"}
}


/**
 * A payout to an attributed address (computed or settled).
 */
export interface Payout {
  amount: i128;
  to: string;
}

/**
 * Dynamic storage keys.
 */
export type DataKey = {tag: "Campaign", values: readonly [u64]} | {tag: "Token", values: readonly [u64]} | {tag: "CodeIndex", values: readonly [u64, string]} | {tag: "Delegate", values: readonly [u64, string]} | {tag: "OwnerCount", values: readonly [string]} | {tag: "OwnerCampaign", values: readonly [string, u64]} | {tag: "CampaignOwnerSlot", values: readonly [u64]} | {tag: "Shared", values: readonly [u64, string]} | {tag: "Tally", values: readonly [u64, string, u64]} | {tag: "Settled", values: readonly [u64, string, u64]};


/**
 * Campaign — a collection of unique coupon codes with shared terms.
 * Owned by `owner`: the authority for all privileged operations (ADR-002).
 */
export interface Campaign {
  burned: u32;
  discount_type: string;
  discount_value: u64;
  id: u64;
  minted: u32;
  name: string;
  owner: string;
  total_supply: u32;
  valid_until: u64;
}


/**
 * A shared, multi-use code registered under a campaign. Unlike Burn's unique
 * tokens, a shared code is redeemed off-chain many times; the chain holds only
 * periodic commitments (see TallyCommitment). May credit an attribution target
 * (creator/referrer) for payout.
 */
export interface SharedCode {
  attributed_to: Option<string>;
  campaign_id: u64;
  code: string;
  payout_rate: i128;
  payout_token: Option<string>;
  registered_at: u64;
}


/**
 * CouponToken — an individual unique single-use coupon (Burn profile).
 */
export interface CouponToken {
  burned_at: u64;
  campaign_id: u64;
  code: string;
  is_burned: boolean;
  minted_at: u64;
  /**
 * Opaque, non-reversible commitment to the redeemer's identity, supplied
 * off-chain at redemption. All-zero until burned. NEVER plaintext PII (ADR-005/010).
 */
redeemer_ref: Buffer;
  token_id: u64;
}





/**
 * Campaign stats for dashboard queries.
 */
export interface CampaignStats {
  available: u32;
  burned: u32;
  is_expired: boolean;
  minted: u32;
  total_supply: u32;
}




/**
 * A per-epoch on-chain commitment of off-chain redemptions for a shared code:
 * a total `count`, a `merkle_root` anchoring the underlying signed receipts,
 * and `per_attribution` counts used for settlement. Anyone with the receipts
 * can detect changes against the root; the receipt signer remains the trust
 * anchor for whether an off-chain redemption was genuine (ADR-004/014).
 */
export interface TallyCommitment {
  count: u32;
  merkle_root: Buffer;
  per_attribution: Map<string, u32>;
  period: u64;
}





/**
 * RedemptionReceipt — returned after a successful burn/redeem.
 */
export interface RedemptionReceipt {
  burned_at: u64;
  campaign_id: u64;
  campaign_name: string;
  code: string;
  discount_type: string;
  discount_value: u64;
  ledger_seq: u32;
  redeemer_ref: Buffer;
  token_id: u64;
}

export interface Client {
  /**
   * Construct and simulate a settle transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Settle a committed tally: pay each attributed address `count * rate` of
   * `token`, from the campaign `owner`'s balance (keeper-triggerable payout —
   * ADR-004/014). Permissionless execution: the owner pre-approves this
   * contract as a token spender, then any fee payer may trigger settlement.
   * A period settles once. Returns the payouts made.
   */
  settle: ({owner, campaign_id, code, period}: {owner: string, campaign_id: u64, code: string, period: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<Array<Payout>>>>

  /**
   * Construct and simulate a verify transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Verify a coupon by `(campaign_id, code)`. Public, no auth.
   */
  verify: ({campaign_id, code}: {campaign_id: u64, code: string}, options?: MethodOptions) => Promise<AssembledTransaction<Result<CouponToken>>>

  /**
   * Construct and simulate a is_valid transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Whether `(campaign_id, code)` is valid and available for redemption. Public.
   */
  is_valid: ({campaign_id, code}: {campaign_id: u64, code: string}, options?: MethodOptions) => Promise<AssembledTransaction<boolean>>

  /**
   * Construct and simulate a get_tally transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Get a committed tally for a (shared code, period). Public, no auth —
   * anyone with the signed receipts can verify inclusion against merkle_root.
   */
  get_tally: ({campaign_id, code, period}: {campaign_id: u64, code: string, period: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<TallyCommitment>>>

  /**
   * Construct and simulate a bump_codes transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Re-extend the storage TTL of specific coupons (their Token + code-index
   * entries) under a campaign. Public — anyone may pay rent to keep coupons
   * of a long-running campaign alive (ADR-009). Bounded to MAX_BATCH per call;
   * unknown codes are skipped. (Soroban storage is not iterable, so the caller
   * supplies the codes to bump — there is no per-campaign token list to walk.)
   */
  bump_codes: ({campaign_id, codes}: {campaign_id: u64, codes: Array<string>}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a bump_tally transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Re-extend the storage TTL of a shared code and the given periods' tally +
   * settlement entries, for long-lived auditability. Public — anyone may pay
   * rent. Bounded to MAX_BATCH periods per call. (ADR-013)
   */
  bump_tally: ({campaign_id, code, periods}: {campaign_id: u64, code: string, periods: Array<u64>}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a get_shared transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Get a registered shared code. Public, no auth.
   */
  get_shared: ({campaign_id, code}: {campaign_id: u64, code: string}, options?: MethodOptions) => Promise<AssembledTransaction<Result<SharedCode>>>

  /**
   * Construct and simulate a is_settled transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Whether a committed tally period has already been paid. Public, no auth.
   * This lets keepers, SDKs and auditors avoid attempting a duplicate payout.
   */
  is_settled: ({campaign_id, code, period}: {campaign_id: u64, code: string, period: u64}, options?: MethodOptions) => Promise<AssembledTransaction<boolean>>

  /**
   * Construct and simulate a is_delegate transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Whether `who` is an authorized delegate of `campaign_id`. Public read.
   */
  is_delegate: ({campaign_id, who}: {campaign_id: u64, who: string}, options?: MethodOptions) => Promise<AssembledTransaction<boolean>>

  /**
   * Construct and simulate a add_delegate transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Authorize `delegate` to redeem coupons of `campaign_id`. Owner only.
   */
  add_delegate: ({owner, campaign_id, delegate}: {owner: string, campaign_id: u64, delegate: string}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a campaigns_of transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * List every campaign ID created by `owner`. Public compatibility helper;
   * integrations with potentially large accounts should use `campaigns_page`.
   */
  campaigns_of: ({owner}: {owner: string}, options?: MethodOptions) => Promise<AssembledTransaction<Array<u64>>>

  /**
   * Construct and simulate a commit_tally transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Commit an epoch's off-chain tally for a shared code: a total `count`, a
   * `merkle_root` anchoring the signed receipts, and `per_attribution` counts.
   * Owner only; one commitment per (code, period) — append-only so history is
   * auditable. For an attributed code, every redemption belongs to its one
   * registered target, so the attributed count must equal `count`; allowing
   * a smaller number would let an owner commit conversions while
   * underpaying the creator. (ADR-003/004/014)
   */
  commit_tally: ({owner, campaign_id, code, period, count, merkle_root, per_attribution}: {owner: string, campaign_id: u64, code: string, period: u64, count: u32, merkle_root: Buffer, per_attribution: Map<string, u32>}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a get_campaign transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Get campaign details. Public, no auth.
   */
  get_campaign: ({campaign_id}: {campaign_id: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<Campaign>>>

  /**
   * Construct and simulate a issue_unique transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Issue a batch of unique coupon codes under a campaign. Owner only.
   * Codes are unique *within the campaign* (ADR-009).
   */
  issue_unique: ({owner, campaign_id, codes}: {owner: string, campaign_id: u64, codes: Array<string>}, options?: MethodOptions) => Promise<AssembledTransaction<Result<Array<u64>>>>

  /**
   * Construct and simulate a total_minted transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Total coupons issued across all campaigns.
   */
  total_minted: (options?: MethodOptions) => Promise<AssembledTransaction<u64>>

  /**
   * Construct and simulate a bump_campaign transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Re-extend the storage TTL of a campaign (and its owner-index entries). Public —
   * anyone may pay to keep a long-running campaign's metadata alive past the
   * default window (ADR-009). Coupon and delegate entries must be supplied to
   * `bump_codes` / `bump_delegates` because persistent storage is not iterable.
   */
  bump_campaign: ({campaign_id}: {campaign_id: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a redeem_unique transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Redeem (burn) a unique coupon by `(campaign_id, code)` — the Burn path.
   * Irreversible single-use; authorized by the campaign owner or a delegate
   * (ADR-002). `redeemer_ref_hash` is an opaque off-chain commitment (ADR-005/010).
   */
  redeem_unique: ({authorizer, campaign_id, code, redeemer_ref_hash}: {authorizer: string, campaign_id: u64, code: string, redeemer_ref_hash: Buffer}, options?: MethodOptions) => Promise<AssembledTransaction<Result<RedemptionReceipt>>>

  /**
   * Construct and simulate a bump_delegates transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Re-extend specific delegate authorizations for a long-running campaign.
   * Public — anyone may pay rent; unknown delegates are skipped. The caller
   * supplies addresses because Soroban persistent storage is not iterable.
   */
  bump_delegates: ({campaign_id, delegates}: {campaign_id: u64, delegates: Array<string>}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a campaign_stats transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Get campaign statistics. Public, no auth.
   */
  campaign_stats: ({campaign_id}: {campaign_id: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<CampaignStats>>>

  /**
   * Construct and simulate a campaigns_page transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Bounded owner-campaign pagination. `cursor` is a zero-based slot and
   * `limit` must be 1..=MAX_BATCH. Public, no auth.
   */
  campaigns_page: ({owner, cursor, limit}: {owner: string, cursor: u64, limit: u32}, options?: MethodOptions) => Promise<AssembledTransaction<Result<Array<u64>>>>

  /**
   * Construct and simulate a compute_payouts transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Preview the payouts a settlement would produce for a committed tally at
   * the shared code's immutable rate. Public, read-only — no transfer or auth.
   */
  compute_payouts: ({campaign_id, code, period}: {campaign_id: u64, code: string, period: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<Array<Payout>>>>

  /**
   * Construct and simulate a create_campaign transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Create a new coupon campaign owned by `owner`. Permissionless (ADR-002).
   * Validates structural terms (ADR-009); reward semantics stay opaque.
   */
  create_campaign: ({owner, name, discount_type, discount_value, total_supply, valid_until}: {owner: string, name: string, discount_type: string, discount_value: u64, total_supply: u32, valid_until: u64}, options?: MethodOptions) => Promise<AssembledTransaction<Result<u64>>>

  /**
   * Construct and simulate a register_shared transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Register a shared, multi-use code under a campaign, optionally crediting
   * an attribution target (creator/referrer). Owner only. Shared codes live
   * in a namespace separate from Burn's unique codes (ADR-011).
   */
  register_shared: ({owner, campaign_id, code, attributed_to, payout_token, payout_rate}: {owner: string, campaign_id: u64, code: string, attributed_to: Option<string>, payout_token: Option<string>, payout_rate: i128}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a remove_delegate transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Revoke a delegate's redemption authority for `campaign_id`. Owner only.
   */
  remove_delegate: ({owner, campaign_id, delegate}: {owner: string, campaign_id: u64, delegate: string}, options?: MethodOptions) => Promise<AssembledTransaction<Result<void>>>

  /**
   * Construct and simulate a total_campaigns transaction. Returns an `AssembledTransaction` object which will have a `result` field containing the result of the simulation. If this transaction changes contract state, you will need to call `signAndSend()` on the returned object.
   * Total campaigns created across all owners.
   */
  total_campaigns: (options?: MethodOptions) => Promise<AssembledTransaction<u64>>

}
export class Client extends ContractClient {
  static async deploy<T = Client>(
    /** Options for initializing a Client as well as for calling a method, with extras specific to deploying. */
    options: MethodOptions &
      Omit<ContractClientOptions, "contractId"> & {
        /** The hash of the Wasm blob, which must already be installed on-chain. */
        wasmHash: Buffer | string;
        /** Salt used to generate the contract's ID. Passed through to {@link Operation.createCustomContract}. Default: random. */
        salt?: Buffer | Uint8Array;
        /** The format used to decode `wasmHash`, if it's provided as a string. */
        format?: "hex" | "base64";
      }
  ): Promise<AssembledTransaction<T>> {
    return ContractClient.deploy(null, options)
  }
  constructor(public readonly options: ContractClientOptions) {
    super(
      new ContractSpec([ "AAAABAAAAAAAAAAAAAAABUVycm9yAAAAAAAAEwAAAAAAAAAQQ2FtcGFpZ25Ob3RGb3VuZAAAAAEAAAAAAAAADkNvdXBvbk5vdEZvdW5kAAAAAAACAAAAAAAAAA9BbHJlYWR5UmVkZWVtZWQAAAAAAwAAAAAAAAAPQ2FtcGFpZ25FeHBpcmVkAAAAAAQAAAAAAAAAD1N1cHBseUV4aGF1c3RlZAAAAAAFAAAAAAAAAAxVbmF1dGhvcml6ZWQAAAAGAAAAAAAAAAtJbnZhbGlkQ29kZQAAAAAHAAAAAAAAAA1EdXBsaWNhdGVDb2RlAAAAAAAACAAAAAAAAAAMSW52YWxpZFRlcm1zAAAACQAAAAAAAAANQmF0Y2hUb29MYXJnZQAAAAAAAAoAAAAAAAAAC0NvZGVUb29Mb25nAAAAAAsAAAAAAAAADlNoYXJlZE5vdEZvdW5kAAAAAAAMAAAAAAAAABFBbHJlYWR5UmVnaXN0ZXJlZAAAAAAAAA0AAAAAAAAAD1BlcmlvZENvbW1pdHRlZAAAAAAOAAAAAAAAAA1UYWxseU5vdEZvdW5kAAAAAAAADwAAAAAAAAAOQWxyZWFkeVNldHRsZWQAAAAAABAAAAAAAAAADEludmFsaWRUYWxseQAAABEAAAAAAAAAEUludmFsaWRTZXR0bGVtZW50AAAAAAAAEgAAAAAAAAATQXR0cmlidXRpb25NaXNtYXRjaAAAAAAT",
        "AAAAAQAAADhBIHBheW91dCB0byBhbiBhdHRyaWJ1dGVkIGFkZHJlc3MgKGNvbXB1dGVkIG9yIHNldHRsZWQpLgAAAAAAAAAGUGF5b3V0AAAAAAACAAAAAAAAAAZhbW91bnQAAAAAAAsAAAAAAAAAAnRvAAAAAAAT",
        "AAAAAgAAABVEeW5hbWljIHN0b3JhZ2Uga2V5cy4AAAAAAAAAAAAAB0RhdGFLZXkAAAAACgAAAAEAAAAAAAAACENhbXBhaWduAAAAAQAAAAYAAAABAAAAAAAAAAVUb2tlbgAAAAAAAAEAAAAGAAAAAQAAAAAAAAAJQ29kZUluZGV4AAAAAAAAAgAAAAYAAAAQAAAAAQAAAAAAAAAIRGVsZWdhdGUAAAACAAAABgAAABMAAAABAAAAAAAAAApPd25lckNvdW50AAAAAAABAAAAEwAAAAEAAAAAAAAADU93bmVyQ2FtcGFpZ24AAAAAAAACAAAAEwAAAAYAAAABAAAAAAAAABFDYW1wYWlnbk93bmVyU2xvdAAAAAAAAAEAAAAGAAAAAQAAAAAAAAAGU2hhcmVkAAAAAAACAAAABgAAABAAAAABAAAAAAAAAAVUYWxseQAAAAAAAAMAAAAGAAAAEAAAAAYAAAABAAAAAAAAAAdTZXR0bGVkAAAAAAMAAAAGAAAAEAAAAAY=",
        "AAAAAQAAAIxDYW1wYWlnbiDigJQgYSBjb2xsZWN0aW9uIG9mIHVuaXF1ZSBjb3Vwb24gY29kZXMgd2l0aCBzaGFyZWQgdGVybXMuCk93bmVkIGJ5IGBvd25lcmA6IHRoZSBhdXRob3JpdHkgZm9yIGFsbCBwcml2aWxlZ2VkIG9wZXJhdGlvbnMgKEFEUi0wMDIpLgAAAAAAAAAIQ2FtcGFpZ24AAAAJAAAAAAAAAAZidXJuZWQAAAAAAAQAAAAAAAAADWRpc2NvdW50X3R5cGUAAAAAAAAQAAAAAAAAAA5kaXNjb3VudF92YWx1ZQAAAAAABgAAAAAAAAACaWQAAAAAAAYAAAAAAAAABm1pbnRlZAAAAAAABAAAAAAAAAAEbmFtZQAAABAAAAAAAAAABW93bmVyAAAAAAAAEwAAAAAAAAAMdG90YWxfc3VwcGx5AAAABAAAAAAAAAALdmFsaWRfdW50aWwAAAAABg==",
        "AAAAAQAAAQNBIHNoYXJlZCwgbXVsdGktdXNlIGNvZGUgcmVnaXN0ZXJlZCB1bmRlciBhIGNhbXBhaWduLiBVbmxpa2UgQnVybidzIHVuaXF1ZQp0b2tlbnMsIGEgc2hhcmVkIGNvZGUgaXMgcmVkZWVtZWQgb2ZmLWNoYWluIG1hbnkgdGltZXM7IHRoZSBjaGFpbiBob2xkcyBvbmx5CnBlcmlvZGljIGNvbW1pdG1lbnRzIChzZWUgVGFsbHlDb21taXRtZW50KS4gTWF5IGNyZWRpdCBhbiBhdHRyaWJ1dGlvbiB0YXJnZXQKKGNyZWF0b3IvcmVmZXJyZXIpIGZvciBwYXlvdXQuAAAAAAAAAAAKU2hhcmVkQ29kZQAAAAAABgAAAAAAAAANYXR0cmlidXRlZF90bwAAAAAAA+gAAAATAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAARjb2RlAAAAEAAAAAAAAAALcGF5b3V0X3JhdGUAAAAACwAAAAAAAAAMcGF5b3V0X3Rva2VuAAAD6AAAABMAAAAAAAAADXJlZ2lzdGVyZWRfYXQAAAAAAAAG",
        "AAAAAQAAAEZDb3Vwb25Ub2tlbiDigJQgYW4gaW5kaXZpZHVhbCB1bmlxdWUgc2luZ2xlLXVzZSBjb3Vwb24gKEJ1cm4gcHJvZmlsZSkuAAAAAAAAAAAAC0NvdXBvblRva2VuAAAAAAcAAAAAAAAACWJ1cm5lZF9hdAAAAAAAAAYAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAABGNvZGUAAAAQAAAAAAAAAAlpc19idXJuZWQAAAAAAAABAAAAAAAAAAltaW50ZWRfYXQAAAAAAAAGAAAAmU9wYXF1ZSwgbm9uLXJldmVyc2libGUgY29tbWl0bWVudCB0byB0aGUgcmVkZWVtZXIncyBpZGVudGl0eSwgc3VwcGxpZWQKb2ZmLWNoYWluIGF0IHJlZGVtcHRpb24uIEFsbC16ZXJvIHVudGlsIGJ1cm5lZC4gTkVWRVIgcGxhaW50ZXh0IFBJSSAoQURSLTAwNS8wMTApLgAAAAAAAAxyZWRlZW1lcl9yZWYAAAPuAAAAIAAAAAAAAAAIdG9rZW5faWQAAAAG",
        "AAAAAAAAAVBTZXR0bGUgYSBjb21taXR0ZWQgdGFsbHk6IHBheSBlYWNoIGF0dHJpYnV0ZWQgYWRkcmVzcyBgY291bnQgKiByYXRlYCBvZgpgdG9rZW5gLCBmcm9tIHRoZSBjYW1wYWlnbiBgb3duZXJgJ3MgYmFsYW5jZSAoa2VlcGVyLXRyaWdnZXJhYmxlIHBheW91dCDigJQKQURSLTAwNC8wMTQpLiBQZXJtaXNzaW9ubGVzcyBleGVjdXRpb246IHRoZSBvd25lciBwcmUtYXBwcm92ZXMgdGhpcwpjb250cmFjdCBhcyBhIHRva2VuIHNwZW5kZXIsIHRoZW4gYW55IGZlZSBwYXllciBtYXkgdHJpZ2dlciBzZXR0bGVtZW50LgpBIHBlcmlvZCBzZXR0bGVzIG9uY2UuIFJldHVybnMgdGhlIHBheW91dHMgbWFkZS4AAAAGc2V0dGxlAAAAAAAEAAAAAAAAAAVvd25lcgAAAAAAABMAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAABGNvZGUAAAAQAAAAAAAAAAZwZXJpb2QAAAAAAAYAAAABAAAD6QAAA+oAAAfQAAAABlBheW91dAAAAAAAAw==",
        "AAAAAAAAADpWZXJpZnkgYSBjb3Vwb24gYnkgYChjYW1wYWlnbl9pZCwgY29kZSlgLiBQdWJsaWMsIG5vIGF1dGguAAAAAAAGdmVyaWZ5AAAAAAACAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAARjb2RlAAAAEAAAAAEAAAPpAAAH0AAAAAtDb3Vwb25Ub2tlbgAAAAAD",
        "AAAABQAAAAAAAAAAAAAADENvdXBvbkJ1cm5lZAAAAAIAAAAGY291cG9uAAAAAAAEYnVybgAAAAIAAAAAAAAACHRva2VuX2lkAAAABgAAAAAAAAAAAAAACmxlZGdlcl9zZXEAAAAAAAQAAAAAAAAAAQ==",
        "AAAABQAAAAAAAAAAAAAADENvdXBvbklzc3VlZAAAAAIAAAAGY291cG9uAAAAAAAFaXNzdWUAAAAAAAACAAAAAAAAAAh0b2tlbl9pZAAAAAYAAAAAAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAAE=",
        "AAAABQAAAAAAAAAAAAAADFRhbGx5U2V0dGxlZAAAAAIAAAAFdGFsbHkAAAAAAAAGc2V0dGxlAAAAAAAEAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAAAAAAAGcGVyaW9kAAAAAAAGAAAAAAAAAAAAAAAFdG9rZW4AAAAAAAATAAAAAAAAAAAAAAALcGF5b3V0X3JhdGUAAAAACwAAAAAAAAAB",
        "AAAAAQAAACVDYW1wYWlnbiBzdGF0cyBmb3IgZGFzaGJvYXJkIHF1ZXJpZXMuAAAAAAAAAAAAAA1DYW1wYWlnblN0YXRzAAAAAAAABQAAAAAAAAAJYXZhaWxhYmxlAAAAAAAABAAAAAAAAAAGYnVybmVkAAAAAAAEAAAAAAAAAAppc19leHBpcmVkAAAAAAABAAAAAAAAAAZtaW50ZWQAAAAAAAQAAAAAAAAADHRvdGFsX3N1cHBseQAAAAQ=",
        "AAAAAAAAAExXaGV0aGVyIGAoY2FtcGFpZ25faWQsIGNvZGUpYCBpcyB2YWxpZCBhbmQgYXZhaWxhYmxlIGZvciByZWRlbXB0aW9uLiBQdWJsaWMuAAAACGlzX3ZhbGlkAAAAAgAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAEY29kZQAAABAAAAABAAAAAQ==",
        "AAAABQAAAAAAAAAAAAAADURlbGVnYXRlQWRkZWQAAAAAAAACAAAACGRlbGVnYXRlAAAAA2FkZAAAAAACAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAAAAAAAIZGVsZWdhdGUAAAATAAAAAAAAAAE=",
        "AAAAAAAAAJBHZXQgYSBjb21taXR0ZWQgdGFsbHkgZm9yIGEgKHNoYXJlZCBjb2RlLCBwZXJpb2QpLiBQdWJsaWMsIG5vIGF1dGgg4oCUCmFueW9uZSB3aXRoIHRoZSBzaWduZWQgcmVjZWlwdHMgY2FuIHZlcmlmeSBpbmNsdXNpb24gYWdhaW5zdCBtZXJrbGVfcm9vdC4AAAAJZ2V0X3RhbGx5AAAAAAAAAwAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAEY29kZQAAABAAAAAAAAAABnBlcmlvZAAAAAAABgAAAAEAAAPpAAAH0AAAAA9UYWxseUNvbW1pdG1lbnQAAAAAAw==",
        "AAAABQAAAAAAAAAAAAAADlRhbGx5Q29tbWl0dGVkAAAAAAACAAAABXRhbGx5AAAAAAAABmNvbW1pdAAAAAAAAwAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAAAAAABnBlcmlvZAAAAAAABgAAAAAAAAAAAAAABWNvdW50AAAAAAAABAAAAAAAAAAB",
        "AAAAAQAAAXFBIHBlci1lcG9jaCBvbi1jaGFpbiBjb21taXRtZW50IG9mIG9mZi1jaGFpbiByZWRlbXB0aW9ucyBmb3IgYSBzaGFyZWQgY29kZToKYSB0b3RhbCBgY291bnRgLCBhIGBtZXJrbGVfcm9vdGAgYW5jaG9yaW5nIHRoZSB1bmRlcmx5aW5nIHNpZ25lZCByZWNlaXB0cywKYW5kIGBwZXJfYXR0cmlidXRpb25gIGNvdW50cyB1c2VkIGZvciBzZXR0bGVtZW50LiBBbnlvbmUgd2l0aCB0aGUgcmVjZWlwdHMKY2FuIGRldGVjdCBjaGFuZ2VzIGFnYWluc3QgdGhlIHJvb3Q7IHRoZSByZWNlaXB0IHNpZ25lciByZW1haW5zIHRoZSB0cnVzdAphbmNob3IgZm9yIHdoZXRoZXIgYW4gb2ZmLWNoYWluIHJlZGVtcHRpb24gd2FzIGdlbnVpbmUgKEFEUi0wMDQvMDE0KS4AAAAAAAAAAAAAD1RhbGx5Q29tbWl0bWVudAAAAAAEAAAAAAAAAAVjb3VudAAAAAAAAAQAAAAAAAAAC21lcmtsZV9yb290AAAAA+4AAAAgAAAAAAAAAA9wZXJfYXR0cmlidXRpb24AAAAD7AAAABMAAAAEAAAAAAAAAAZwZXJpb2QAAAAAAAY=",
        "AAAAAAAAAXRSZS1leHRlbmQgdGhlIHN0b3JhZ2UgVFRMIG9mIHNwZWNpZmljIGNvdXBvbnMgKHRoZWlyIFRva2VuICsgY29kZS1pbmRleAplbnRyaWVzKSB1bmRlciBhIGNhbXBhaWduLiBQdWJsaWMg4oCUIGFueW9uZSBtYXkgcGF5IHJlbnQgdG8ga2VlcCBjb3Vwb25zCm9mIGEgbG9uZy1ydW5uaW5nIGNhbXBhaWduIGFsaXZlIChBRFItMDA5KS4gQm91bmRlZCB0byBNQVhfQkFUQ0ggcGVyIGNhbGw7CnVua25vd24gY29kZXMgYXJlIHNraXBwZWQuIChTb3JvYmFuIHN0b3JhZ2UgaXMgbm90IGl0ZXJhYmxlLCBzbyB0aGUgY2FsbGVyCnN1cHBsaWVzIHRoZSBjb2RlcyB0byBidW1wIOKAlCB0aGVyZSBpcyBubyBwZXItY2FtcGFpZ24gdG9rZW4gbGlzdCB0byB3YWxrLikAAAAKYnVtcF9jb2RlcwAAAAAAAgAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAFY29kZXMAAAAAAAPqAAAAEAAAAAEAAAPpAAAAAgAAAAM=",
        "AAAAAAAAAMtSZS1leHRlbmQgdGhlIHN0b3JhZ2UgVFRMIG9mIGEgc2hhcmVkIGNvZGUgYW5kIHRoZSBnaXZlbiBwZXJpb2RzJyB0YWxseSArCnNldHRsZW1lbnQgZW50cmllcywgZm9yIGxvbmctbGl2ZWQgYXVkaXRhYmlsaXR5LiBQdWJsaWMg4oCUIGFueW9uZSBtYXkgcGF5CnJlbnQuIEJvdW5kZWQgdG8gTUFYX0JBVENIIHBlcmlvZHMgcGVyIGNhbGwuIChBRFItMDEzKQAAAAAKYnVtcF90YWxseQAAAAAAAwAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAEY29kZQAAABAAAAAAAAAAB3BlcmlvZHMAAAAD6gAAAAYAAAABAAAD6QAAAAIAAAAD",
        "AAAAAAAAAC5HZXQgYSByZWdpc3RlcmVkIHNoYXJlZCBjb2RlLiBQdWJsaWMsIG5vIGF1dGguAAAAAAAKZ2V0X3NoYXJlZAAAAAAAAgAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAEY29kZQAAABAAAAABAAAD6QAAB9AAAAAKU2hhcmVkQ29kZQAAAAAAAw==",
        "AAAAAAAAAJJXaGV0aGVyIGEgY29tbWl0dGVkIHRhbGx5IHBlcmlvZCBoYXMgYWxyZWFkeSBiZWVuIHBhaWQuIFB1YmxpYywgbm8gYXV0aC4KVGhpcyBsZXRzIGtlZXBlcnMsIFNES3MgYW5kIGF1ZGl0b3JzIGF2b2lkIGF0dGVtcHRpbmcgYSBkdXBsaWNhdGUgcGF5b3V0LgAAAAAACmlzX3NldHRsZWQAAAAAAAMAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAABGNvZGUAAAAQAAAAAAAAAAZwZXJpb2QAAAAAAAYAAAABAAAAAQ==",
        "AAAABQAAAAAAAAAAAAAAD0NhbXBhaWduQ3JlYXRlZAAAAAACAAAACGNhbXBhaWduAAAABmNyZWF0ZQAAAAAABAAAAAAAAAACaWQAAAAAAAYAAAAAAAAAAAAAAAVvd25lcgAAAAAAABMAAAAAAAAAAAAAAARuYW1lAAAAEAAAAAAAAAAAAAAADHRvdGFsX3N1cHBseQAAAAQAAAAAAAAAAQ==",
        "AAAABQAAAAAAAAAAAAAAD0RlbGVnYXRlUmVtb3ZlZAAAAAACAAAACGRlbGVnYXRlAAAABnJlbW92ZQAAAAAAAgAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAAAAAACGRlbGVnYXRlAAAAEwAAAAAAAAAB",
        "AAAAAAAAAEZXaGV0aGVyIGB3aG9gIGlzIGFuIGF1dGhvcml6ZWQgZGVsZWdhdGUgb2YgYGNhbXBhaWduX2lkYC4gUHVibGljIHJlYWQuAAAAAAALaXNfZGVsZWdhdGUAAAAAAgAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAADd2hvAAAAABMAAAABAAAAAQ==",
        "AAAABQAAAAAAAAAAAAAAEFNoYXJlZFJlZ2lzdGVyZWQAAAACAAAABnNoYXJlZAAAAAAAA3JlZwAAAAACAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAAAAAAANYXR0cmlidXRlZF90bwAAAAAAA+gAAAATAAAAAAAAAAE=",
        "AAAAAQAAAD5SZWRlbXB0aW9uUmVjZWlwdCDigJQgcmV0dXJuZWQgYWZ0ZXIgYSBzdWNjZXNzZnVsIGJ1cm4vcmVkZWVtLgAAAAAAAAAAABFSZWRlbXB0aW9uUmVjZWlwdAAAAAAAAAkAAAAAAAAACWJ1cm5lZF9hdAAAAAAAAAYAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAADWNhbXBhaWduX25hbWUAAAAAAAAQAAAAAAAAAARjb2RlAAAAEAAAAAAAAAANZGlzY291bnRfdHlwZQAAAAAAABAAAAAAAAAADmRpc2NvdW50X3ZhbHVlAAAAAAAGAAAAAAAAAApsZWRnZXJfc2VxAAAAAAAEAAAAAAAAAAxyZWRlZW1lcl9yZWYAAAPuAAAAIAAAAAAAAAAIdG9rZW5faWQAAAAG",
        "AAAAAAAAAERBdXRob3JpemUgYGRlbGVnYXRlYCB0byByZWRlZW0gY291cG9ucyBvZiBgY2FtcGFpZ25faWRgLiBPd25lciBvbmx5LgAAAAxhZGRfZGVsZWdhdGUAAAADAAAAAAAAAAVvd25lcgAAAAAAABMAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAACGRlbGVnYXRlAAAAEwAAAAEAAAPpAAAAAgAAAAM=",
        "AAAAAAAAAJFMaXN0IGV2ZXJ5IGNhbXBhaWduIElEIGNyZWF0ZWQgYnkgYG93bmVyYC4gUHVibGljIGNvbXBhdGliaWxpdHkgaGVscGVyOwppbnRlZ3JhdGlvbnMgd2l0aCBwb3RlbnRpYWxseSBsYXJnZSBhY2NvdW50cyBzaG91bGQgdXNlIGBjYW1wYWlnbnNfcGFnZWAuAAAAAAAADGNhbXBhaWduc19vZgAAAAEAAAAAAAAABW93bmVyAAAAAAAAEwAAAAEAAAPqAAAABg==",
        "AAAAAAAAAdVDb21taXQgYW4gZXBvY2gncyBvZmYtY2hhaW4gdGFsbHkgZm9yIGEgc2hhcmVkIGNvZGU6IGEgdG90YWwgYGNvdW50YCwgYQpgbWVya2xlX3Jvb3RgIGFuY2hvcmluZyB0aGUgc2lnbmVkIHJlY2VpcHRzLCBhbmQgYHBlcl9hdHRyaWJ1dGlvbmAgY291bnRzLgpPd25lciBvbmx5OyBvbmUgY29tbWl0bWVudCBwZXIgKGNvZGUsIHBlcmlvZCkg4oCUIGFwcGVuZC1vbmx5IHNvIGhpc3RvcnkgaXMKYXVkaXRhYmxlLiBGb3IgYW4gYXR0cmlidXRlZCBjb2RlLCBldmVyeSByZWRlbXB0aW9uIGJlbG9uZ3MgdG8gaXRzIG9uZQpyZWdpc3RlcmVkIHRhcmdldCwgc28gdGhlIGF0dHJpYnV0ZWQgY291bnQgbXVzdCBlcXVhbCBgY291bnRgOyBhbGxvd2luZwphIHNtYWxsZXIgbnVtYmVyIHdvdWxkIGxldCBhbiBvd25lciBjb21taXQgY29udmVyc2lvbnMgd2hpbGUKdW5kZXJwYXlpbmcgdGhlIGNyZWF0b3IuIChBRFItMDAzLzAwNC8wMTQpAAAAAAAADGNvbW1pdF90YWxseQAAAAcAAAAAAAAABW93bmVyAAAAAAAAEwAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAEY29kZQAAABAAAAAAAAAABnBlcmlvZAAAAAAABgAAAAAAAAAFY291bnQAAAAAAAAEAAAAAAAAAAttZXJrbGVfcm9vdAAAAAPuAAAAIAAAAAAAAAAPcGVyX2F0dHJpYnV0aW9uAAAAA+wAAAATAAAABAAAAAEAAAPpAAAAAgAAAAM=",
        "AAAAAAAAACZHZXQgY2FtcGFpZ24gZGV0YWlscy4gUHVibGljLCBubyBhdXRoLgAAAAAADGdldF9jYW1wYWlnbgAAAAEAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAABAAAD6QAAB9AAAAAIQ2FtcGFpZ24AAAAD",
        "AAAAAAAAAHRJc3N1ZSBhIGJhdGNoIG9mIHVuaXF1ZSBjb3Vwb24gY29kZXMgdW5kZXIgYSBjYW1wYWlnbi4gT3duZXIgb25seS4KQ29kZXMgYXJlIHVuaXF1ZSAqd2l0aGluIHRoZSBjYW1wYWlnbiogKEFEUi0wMDkpLgAAAAxpc3N1ZV91bmlxdWUAAAADAAAAAAAAAAVvd25lcgAAAAAAABMAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAABWNvZGVzAAAAAAAD6gAAABAAAAABAAAD6QAAA+oAAAAGAAAAAw==",
        "AAAAAAAAACpUb3RhbCBjb3Vwb25zIGlzc3VlZCBhY3Jvc3MgYWxsIGNhbXBhaWducy4AAAAAAAx0b3RhbF9taW50ZWQAAAAAAAAAAQAAAAY=",
        "AAAAAAAAATBSZS1leHRlbmQgdGhlIHN0b3JhZ2UgVFRMIG9mIGEgY2FtcGFpZ24gKGFuZCBpdHMgb3duZXItaW5kZXggZW50cmllcykuIFB1YmxpYyDigJQKYW55b25lIG1heSBwYXkgdG8ga2VlcCBhIGxvbmctcnVubmluZyBjYW1wYWlnbidzIG1ldGFkYXRhIGFsaXZlIHBhc3QgdGhlCmRlZmF1bHQgd2luZG93IChBRFItMDA5KS4gQ291cG9uIGFuZCBkZWxlZ2F0ZSBlbnRyaWVzIG11c3QgYmUgc3VwcGxpZWQgdG8KYGJ1bXBfY29kZXNgIC8gYGJ1bXBfZGVsZWdhdGVzYCBiZWNhdXNlIHBlcnNpc3RlbnQgc3RvcmFnZSBpcyBub3QgaXRlcmFibGUuAAAADWJ1bXBfY2FtcGFpZ24AAAAAAAABAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAQAAA+kAAAACAAAAAw==",
        "AAAAAAAAAOFSZWRlZW0gKGJ1cm4pIGEgdW5pcXVlIGNvdXBvbiBieSBgKGNhbXBhaWduX2lkLCBjb2RlKWAg4oCUIHRoZSBCdXJuIHBhdGguCklycmV2ZXJzaWJsZSBzaW5nbGUtdXNlOyBhdXRob3JpemVkIGJ5IHRoZSBjYW1wYWlnbiBvd25lciBvciBhIGRlbGVnYXRlCihBRFItMDAyKS4gYHJlZGVlbWVyX3JlZl9oYXNoYCBpcyBhbiBvcGFxdWUgb2ZmLWNoYWluIGNvbW1pdG1lbnQgKEFEUi0wMDUvMDEwKS4AAAAAAAANcmVkZWVtX3VuaXF1ZQAAAAAAAAQAAAAAAAAACmF1dGhvcml6ZXIAAAAAABMAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAABGNvZGUAAAAQAAAAAAAAABFyZWRlZW1lcl9yZWZfaGFzaAAAAAAAA+4AAAAgAAAAAQAAA+kAAAfQAAAAEVJlZGVtcHRpb25SZWNlaXB0AAAAAAAAAw==",
        "AAAAAAAAANhSZS1leHRlbmQgc3BlY2lmaWMgZGVsZWdhdGUgYXV0aG9yaXphdGlvbnMgZm9yIGEgbG9uZy1ydW5uaW5nIGNhbXBhaWduLgpQdWJsaWMg4oCUIGFueW9uZSBtYXkgcGF5IHJlbnQ7IHVua25vd24gZGVsZWdhdGVzIGFyZSBza2lwcGVkLiBUaGUgY2FsbGVyCnN1cHBsaWVzIGFkZHJlc3NlcyBiZWNhdXNlIFNvcm9iYW4gcGVyc2lzdGVudCBzdG9yYWdlIGlzIG5vdCBpdGVyYWJsZS4AAAAOYnVtcF9kZWxlZ2F0ZXMAAAAAAAIAAAAAAAAAC2NhbXBhaWduX2lkAAAAAAYAAAAAAAAACWRlbGVnYXRlcwAAAAAAA+oAAAATAAAAAQAAA+kAAAACAAAAAw==",
        "AAAAAAAAAClHZXQgY2FtcGFpZ24gc3RhdGlzdGljcy4gUHVibGljLCBubyBhdXRoLgAAAAAAAA5jYW1wYWlnbl9zdGF0cwAAAAAAAQAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAEAAAPpAAAH0AAAAA1DYW1wYWlnblN0YXRzAAAAAAAAAw==",
        "AAAAAAAAAHRCb3VuZGVkIG93bmVyLWNhbXBhaWduIHBhZ2luYXRpb24uIGBjdXJzb3JgIGlzIGEgemVyby1iYXNlZCBzbG90IGFuZApgbGltaXRgIG11c3QgYmUgMS4uPU1BWF9CQVRDSC4gUHVibGljLCBubyBhdXRoLgAAAA5jYW1wYWlnbnNfcGFnZQAAAAAAAwAAAAAAAAAFb3duZXIAAAAAAAATAAAAAAAAAAZjdXJzb3IAAAAAAAYAAAAAAAAABWxpbWl0AAAAAAAABAAAAAEAAAPpAAAD6gAAAAYAAAAD",
        "AAAAAAAAAJRQcmV2aWV3IHRoZSBwYXlvdXRzIGEgc2V0dGxlbWVudCB3b3VsZCBwcm9kdWNlIGZvciBhIGNvbW1pdHRlZCB0YWxseSBhdAp0aGUgc2hhcmVkIGNvZGUncyBpbW11dGFibGUgcmF0ZS4gUHVibGljLCByZWFkLW9ubHkg4oCUIG5vIHRyYW5zZmVyIG9yIGF1dGguAAAAD2NvbXB1dGVfcGF5b3V0cwAAAAADAAAAAAAAAAtjYW1wYWlnbl9pZAAAAAAGAAAAAAAAAARjb2RlAAAAEAAAAAAAAAAGcGVyaW9kAAAAAAAGAAAAAQAAA+kAAAPqAAAH0AAAAAZQYXlvdXQAAAAAAAM=",
        "AAAAAAAAAIxDcmVhdGUgYSBuZXcgY291cG9uIGNhbXBhaWduIG93bmVkIGJ5IGBvd25lcmAuIFBlcm1pc3Npb25sZXNzIChBRFItMDAyKS4KVmFsaWRhdGVzIHN0cnVjdHVyYWwgdGVybXMgKEFEUi0wMDkpOyByZXdhcmQgc2VtYW50aWNzIHN0YXkgb3BhcXVlLgAAAA9jcmVhdGVfY2FtcGFpZ24AAAAABgAAAAAAAAAFb3duZXIAAAAAAAATAAAAAAAAAARuYW1lAAAAEAAAAAAAAAANZGlzY291bnRfdHlwZQAAAAAAABAAAAAAAAAADmRpc2NvdW50X3ZhbHVlAAAAAAAGAAAAAAAAAAx0b3RhbF9zdXBwbHkAAAAEAAAAAAAAAAt2YWxpZF91bnRpbAAAAAAGAAAAAQAAA+kAAAAGAAAAAw==",
        "AAAAAAAAAMxSZWdpc3RlciBhIHNoYXJlZCwgbXVsdGktdXNlIGNvZGUgdW5kZXIgYSBjYW1wYWlnbiwgb3B0aW9uYWxseSBjcmVkaXRpbmcKYW4gYXR0cmlidXRpb24gdGFyZ2V0IChjcmVhdG9yL3JlZmVycmVyKS4gT3duZXIgb25seS4gU2hhcmVkIGNvZGVzIGxpdmUKaW4gYSBuYW1lc3BhY2Ugc2VwYXJhdGUgZnJvbSBCdXJuJ3MgdW5pcXVlIGNvZGVzIChBRFItMDExKS4AAAAPcmVnaXN0ZXJfc2hhcmVkAAAAAAYAAAAAAAAABW93bmVyAAAAAAAAEwAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAEY29kZQAAABAAAAAAAAAADWF0dHJpYnV0ZWRfdG8AAAAAAAPoAAAAEwAAAAAAAAAMcGF5b3V0X3Rva2VuAAAD6AAAABMAAAAAAAAAC3BheW91dF9yYXRlAAAAAAsAAAABAAAD6QAAAAIAAAAD",
        "AAAAAAAAAEdSZXZva2UgYSBkZWxlZ2F0ZSdzIHJlZGVtcHRpb24gYXV0aG9yaXR5IGZvciBgY2FtcGFpZ25faWRgLiBPd25lciBvbmx5LgAAAAAPcmVtb3ZlX2RlbGVnYXRlAAAAAAMAAAAAAAAABW93bmVyAAAAAAAAEwAAAAAAAAALY2FtcGFpZ25faWQAAAAABgAAAAAAAAAIZGVsZWdhdGUAAAATAAAAAQAAA+kAAAACAAAAAw==",
        "AAAAAAAAACpUb3RhbCBjYW1wYWlnbnMgY3JlYXRlZCBhY3Jvc3MgYWxsIG93bmVycy4AAAAAAA90b3RhbF9jYW1wYWlnbnMAAAAAAAAAAAEAAAAG" ]),
      options
    )
  }
  public readonly fromJSON = {
    settle: this.txFromJSON<Result<Array<Payout>>>,
        verify: this.txFromJSON<Result<CouponToken>>,
        is_valid: this.txFromJSON<boolean>,
        get_tally: this.txFromJSON<Result<TallyCommitment>>,
        bump_codes: this.txFromJSON<Result<void>>,
        bump_tally: this.txFromJSON<Result<void>>,
        get_shared: this.txFromJSON<Result<SharedCode>>,
        is_settled: this.txFromJSON<boolean>,
        is_delegate: this.txFromJSON<boolean>,
        add_delegate: this.txFromJSON<Result<void>>,
        campaigns_of: this.txFromJSON<Array<u64>>,
        commit_tally: this.txFromJSON<Result<void>>,
        get_campaign: this.txFromJSON<Result<Campaign>>,
        issue_unique: this.txFromJSON<Result<Array<u64>>>,
        total_minted: this.txFromJSON<u64>,
        bump_campaign: this.txFromJSON<Result<void>>,
        redeem_unique: this.txFromJSON<Result<RedemptionReceipt>>,
        bump_delegates: this.txFromJSON<Result<void>>,
        campaign_stats: this.txFromJSON<Result<CampaignStats>>,
        campaigns_page: this.txFromJSON<Result<Array<u64>>>,
        compute_payouts: this.txFromJSON<Result<Array<Payout>>>,
        create_campaign: this.txFromJSON<Result<u64>>,
        register_shared: this.txFromJSON<Result<void>>,
        remove_delegate: this.txFromJSON<Result<void>>,
        total_campaigns: this.txFromJSON<u64>
  }
}