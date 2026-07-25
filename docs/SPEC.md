# Soroticket Protocol Spec (v0.2)

> v0.2.0 implements Burn and Tally and is deployed to **testnet** at
> `CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF`
> (2026-07-12; see `deployments/testnet-v0.2.0.json`). Testnet preview only —
> never point real value at it. The v0.1 testnet contract is deprecated. The
> frozen ABI is `contracts/coupon-ledger/abi-v0.2.0.txt`.

## 1. Scope and trust model

Soroticket standardizes a permissionless Soroban primitive for unique redeemable
codes and shared promotion codes:

```text
Campaign -> Code -> Redemption -> optional Settlement
```

- **Burn:** unique code, one on-chain redemption, real-time single-use and
  supply enforcement.
- **Tally:** shared code, off-chain signed events, periodic on-chain count/root,
  exact binding to one optional attribution target and optional token payout.

There is no global administrator. A campaign owner controls issuance,
registration, tally commitment and delegates. Public callers can read state,
pay rent and, after the owner has granted the contract a token allowance,
trigger a configured settlement.

The contract verifies state transitions, not real-world facts. A Merkle root
makes a published receipt set tamper-evident after commitment; it does not prove
that an off-chain purchase, location or identity claim was genuine. That fact
depends on the receipt signer/integrator.

## 2. Implemented data model

```text
Campaign {
  id: u64, owner: Address, name: String,
  discount_type: String, discount_value: u64,
  total_supply: u32, minted: u32, burned: u32,
  valid_until: u64
}

CouponToken {
  token_id: u64, campaign_id: u64, code: String,
  is_burned: bool, minted_at: u64,
  redeemer_ref: BytesN<32>, burned_at: u64
}

RedemptionReceipt {
  token_id, code, campaign_id, campaign_name,
  discount_type, discount_value, redeemer_ref,
  burned_at, ledger_seq
}

SharedCode {
  campaign_id: u64, code: String,
  attributed_to: Option<Address>,
  payout_token: Option<Address>, payout_rate: i128,
  registered_at: u64
}

TallyCommitment {
  period: u64, count: u32, merkle_root: BytesN<32>,
  per_attribution: Map<Address,u32>
}

Payout { to: Address, amount: i128 }
```

`discount_type` and `discount_value` are opaque reward metadata interpreted by
the integrator. There is no on-chain percentage/currency semantics.

## 3. v0.2 interface

### Campaigns and Burn

```text
create_campaign(owner, name, discount_type, discount_value,
                total_supply, valid_until) -> u64       // owner auth
get_campaign(campaign_id) -> Campaign                  // public
campaign_stats(campaign_id) -> CampaignStats           // public
campaigns_of(owner) -> Vec<u64>                        // compatibility; unbounded
campaigns_page(owner, cursor, limit) -> Vec<u64>        // public; limit 1..100
total_campaigns() -> u64                               // public
total_minted() -> u64                                  // public

issue_unique(owner, campaign_id, codes) -> Vec<u64>    // owner auth; 1..100
redeem_unique(authorizer, campaign_id, code,
              redeemer_ref_hash) -> RedemptionReceipt  // owner/delegate auth
verify(campaign_id, code) -> CouponToken               // public
is_valid(campaign_id, code) -> bool                    // public

add_delegate(owner, campaign_id, delegate)             // owner auth
remove_delegate(owner, campaign_id, delegate)          // owner auth
is_delegate(campaign_id, who) -> bool                  // public

bump_campaign(campaign_id)                             // public fee payer
bump_codes(campaign_id, codes)                         // public; max 100
bump_delegates(campaign_id, delegates)                 // public; max 100
```

Burn enforces:

- campaign ownership and owner/delegate authorization;
- future expiry at creation and no issue/redeem after expiry;
- nonzero supply, bounded metadata/codes and batches;
- code uniqueness inside a campaign;
- total issuance cap and exactly-once redemption.

The code string is public through `verify`; it must be treated as a bearer
secret until redemption if possession authorizes use.

### Shared codes and Tally

```text
register_shared(owner, campaign_id, code, attributed_to,
                payout_token, payout_rate)              // owner auth
get_shared(campaign_id, code) -> SharedCode             // public

commit_tally(owner, campaign_id, code, period, count,
             merkle_root, per_attribution)              // owner auth
get_tally(campaign_id, code, period) -> TallyCommitment // public
is_settled(campaign_id, code, period) -> bool           // public
compute_payouts(campaign_id, code, period) -> Vec<Payout>
settle(owner, campaign_id, code, period) -> Vec<Payout> // permissionless trigger
bump_tally(campaign_id, code, periods)                  // public; max 100
```

Tally invariants:

- registration rejects an expired campaign;
- `payout_token` is set if and only if `payout_rate > 0`;
- an unattributed code cannot configure payout or carry attribution entries;
- an attributed code can contain only its registered address and that
  address's count must **exactly equal** the total count;
- a `(campaign_id, code, period)` commitment is append-only;
- payout math is checked `i128` arithmetic.

`commit_tally` remains allowed after campaign expiry because an epoch may close
later. Therefore event time validity is a signed-receipt policy, not a contract
guarantee.

## 4. Settlement authorization and ordering

At registration, token and rate become immutable. Before settlement, the owner
uses the payout token contract's standard `approve` function to authorize the
Soroticket contract address as spender for a bounded amount and expiration
ledger. This approval is outside the Soroticket ABI.

Anyone may then call `settle(owner, campaign_id, code, period)` and pay the
transaction fee. The contract:

1. confirms `owner` owns the campaign;
2. computes all payouts and total with checked arithmetic;
3. checks the owner's token balance and contract allowance;
4. records the period as settled;
5. calls `transfer_from` for each payout.

Soroban atomicity rolls step 4 back if a token call fails. Re-entry cannot
settle the same period twice. Owners should approve only the expected amount
and a short expiration, and revoke unused allowance.

## 5. Receipt profile used by Soroticket Cloud

The core contract accepts any 32-byte Merkle root; receipt serialization is an
integration profile. Cloud v1 uses canonical `encoding/json` output with fields
in this order:

```text
version, campaign_id, code, count,
customer_commitment?, order_commitment?,
timestamp, nonce, signer
```

- `version = 1`, `count > 0`, nonce is 16 random bytes encoded as hex.
- Customer/order values are HMAC-SHA256 domain-separated opaque commitments.
- `signer` is the organization/environment Ed25519 public key (`G...`).
- Signature: Ed25519 over the exact UTF-8 JSON bytes, standard Base64.
- Leaf: `SHA-256(payload)`.
- Parent: `SHA-256(left || right)`; an odd final node is promoted.

Cloud exposes the receipt set and inclusion proofs at
`GET /v1/audit/tallies/{chain_id}/{code}/{period}` with cursor pagination (at
most 100 receipts/page). A Cloud tally is bounded to 10,000 receipts. Its first
weekly batch uses period `YYYYWW`; extra batches use `YYYYWW01..YYYYWW99`.
These encodings are a Cloud convention—the contract treats `period` as an
opaque `u64`. Auditors must additionally read the contract tally and compare
its root/count. A valid signature says the named signer issued the receipt, not
that the underlying sale was honest.

## 6. Privacy

Burn stores only a 32-byte opaque redeemer commitment. Recommended constructions:

- unlinkable disclosure: `SHA-256(random_nonce || "|" || reference)`, retaining
  the nonce off-chain if later proof is required;
- stable server-side identity: `HMAC-SHA256(strong_secret_pepper,
  domain || reference)`.

A public or constant salt does not protect low-entropy email/phone/order values.
No plaintext PII should enter transaction arguments, events, logs or receipts.

## 7. Errors

```text
1 CampaignNotFound   6 Unauthorized      11 CodeTooLong        16 AlreadySettled
2 CouponNotFound     7 InvalidCode       12 SharedNotFound     17 InvalidTally
3 AlreadyRedeemed    8 DuplicateCode     13 AlreadyRegistered  18 InvalidSettlement
4 CampaignExpired    9 InvalidTerms      14 PeriodCommitted    19 AttributionMismatch
5 SupplyExhausted   10 BatchTooLarge     15 TallyNotFound
```

## 8. Storage lifetime

Persistent entries are extended to a bounded TTL, not forever. Long-lived
integrations must inventory campaign IDs, codes, delegates and tally periods,
call the appropriate `bump_*` methods before archival and budget network rent.
The contract cannot iterate all codes/delegates for an owner.

## 9. Deliberately outside v0.2

The following are product ideas, not implemented contract guarantees:

- shared-code global/cumulative caps;
- once-per-redeemer rules, geofence, KYC or proof-of-presence;
- `valid_from` scheduling;
- ticket ownership or transferability;
- refunds, stored-value balances or subscription renewal;
- multiple attribution recipients/revenue splits;
- autonomous tally scheduling or TTL workers.

These must not appear in product copy as enforced until an extension and tests
exist.

## 10. Version status

- **v0.1 testnet (2026-05-31):** immutable legacy deployment; deprecated.
- **v0.2 candidate (2026-07-11):** local ABI/build only; exact attribution,
  allowance-based permissionless settlement, CEI, public settlement state,
  delegate TTL maintenance, bounded owner pagination and typed events.
