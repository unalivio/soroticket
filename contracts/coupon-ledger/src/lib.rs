#![no_std]

//! Sorodeal coupon-ledger — Burn profile (permissionless).
//!
//! Implements the synchronous **Burn** redemption profile from `docs/SPEC.md`
//! §3.1: unique single-use codes, one on-chain tx per redemption, with
//! protocol-enforced single-use and supply integrity.
//!
//! Permissionless / ownership-based (ADR-002): there is NO global admin.
//! Each campaign is owned by its creator's `Address`; authorization is by the
//! campaign owner and its explicit delegates, enforced with `require_auth`.
//! Anyone can create a campaign from their own keypair.
//!
//! Codes are scoped per campaign — `(campaign_id, code)` (ADR-009): there is no
//! global code namespace to squat, so two campaigns may reuse the same string.
//! `verify`/`redeem` therefore take the `campaign_id` (a QR/link carries it).
//!
//! Privacy (ADR-005/010): the redeemer's identity is only ever an opaque,
//! non-reversible commitment (`redeemer_ref_hash: BytesN<32>`) computed
//! off-chain (random nonce or merchant HMAC). No plaintext PII is stored.
//!
//! The async **Tally** profile (shared codes, Merkle-anchored counts +
//! attribution + USDC settlement; ADR-003/004/011) is also implemented:
//! `register_shared` → `commit_tally` (count + merkle_root + per-attribution
//! counts) → `get_tally`/`compute_payouts` → `settle` (token payout to the
//! attributed addresses). Shared codes live in a namespace separate from the
//! Burn unique codes, so a campaign may use either profile.

use soroban_sdk::{
    contract, contracterror, contractimpl, contracttype, log, symbol_short, Address, BytesN, Env,
    Map, String, Symbol, Vec,
};

// ═══════════════════════════════════════════════════════════════════
// CONSTANTS
// ═══════════════════════════════════════════════════════════════════

const LEDGER_BUMP: u32 = 535_680; // ~31 days  (TTL bump threshold)
const EXTEND_TO: u32 = 2_678_400; // ~155 days (kept under network max_entry_ttl;
                                  //  longer campaigns are kept alive via bump_campaign — ADR-009)

const MAX_BATCH: u32 = 100; // max codes per issue_unique call (resource/cost bound)
const MAX_CODE_LEN: u32 = 64; // max coupon code length (bytes)
const MAX_NAME_LEN: u32 = 96; // max campaign name length (bytes)
const MAX_DISCOUNT_TYPE_LEN: u32 = 32; // max reward metadata label length (bytes)

// ═══════════════════════════════════════════════════════════════════
// TYPES — On-chain data structures
// ═══════════════════════════════════════════════════════════════════

/// Campaign — a collection of unique coupon codes with shared terms.
/// Owned by `owner`: the authority for all privileged operations (ADR-002).
#[contracttype]
#[derive(Clone, Debug)]
pub struct Campaign {
    pub id: u64,
    pub owner: Address, // creator; the authority for this campaign (ADR-002)
    pub name: String,
    pub discount_type: String, // opaque reward metadata (e.g. "percentage", "fixed_amount", "free_item")
    pub discount_value: u64,   // opaque reward value (interpreted off-chain)
    pub total_supply: u32,     // max coupons that can be issued
    pub minted: u32,           // total issued so far
    pub burned: u32,           // total redeemed (burned) so far
    pub valid_until: u64,      // expiration timestamp (unix seconds)
}

/// CouponToken — an individual unique single-use coupon (Burn profile).
#[contracttype]
#[derive(Clone, Debug)]
pub struct CouponToken {
    pub token_id: u64,
    pub campaign_id: u64,
    pub code: String, // unique *within its campaign* (ADR-009)
    pub is_burned: bool,
    pub minted_at: u64,
    /// Opaque, non-reversible commitment to the redeemer's identity, supplied
    /// off-chain at redemption. All-zero until burned. NEVER plaintext PII (ADR-005/010).
    pub redeemer_ref: BytesN<32>,
    pub burned_at: u64, // 0 until burned
}

/// RedemptionReceipt — returned after a successful burn/redeem.
#[contracttype]
#[derive(Clone, Debug, PartialEq)]
pub struct RedemptionReceipt {
    pub token_id: u64,
    pub code: String,
    pub campaign_id: u64,
    pub campaign_name: String,
    pub discount_type: String,
    pub discount_value: u64,
    pub redeemer_ref: BytesN<32>,
    pub burned_at: u64,
    pub ledger_seq: u32, // ledger sequence at redemption — the on-chain anchor
}

/// Campaign stats for dashboard queries.
#[contracttype]
#[derive(Clone, Debug)]
pub struct CampaignStats {
    pub total_supply: u32,
    pub minted: u32,
    pub burned: u32,
    pub available: u32,
    pub is_expired: bool,
}

// ─── Tally profile (shared codes) — ADR-003/004/011 ──────────────────

/// A shared, multi-use code registered under a campaign. Unlike Burn's unique
/// tokens, a shared code is redeemed off-chain many times; the chain holds only
/// periodic commitments (see TallyCommitment). May credit an attribution target
/// (creator/referrer) for payout.
#[contracttype]
#[derive(Clone, Debug)]
pub struct SharedCode {
    pub campaign_id: u64,
    pub code: String,
    pub attributed_to: Option<Address>, // creator/referrer credited for redemptions
    pub payout_token: Option<Address>,  // settlement token (SAC); None = count-only, no payout
    pub payout_rate: i128,              // token base-units per attributed redemption (> 0 if payout_token set)
    pub registered_at: u64,
}

/// A per-epoch on-chain commitment of off-chain redemptions for a shared code:
/// a total `count`, a `merkle_root` anchoring the underlying signed receipts (so
/// anyone can audit a claimed count), and `per_attribution` counts used for
/// settlement. This is what makes attribution trustless (ADR-004).
#[contracttype]
#[derive(Clone, Debug, PartialEq)]
pub struct TallyCommitment {
    pub period: u64,
    pub count: u32,
    pub merkle_root: BytesN<32>,
    pub per_attribution: Map<Address, u32>,
}

/// A payout to an attributed address (computed or settled).
#[contracttype]
#[derive(Clone, Debug, PartialEq)]
pub struct Payout {
    pub to: Address,
    pub amount: i128,
}

// ═══════════════════════════════════════════════════════════════════
// ERRORS
// ═══════════════════════════════════════════════════════════════════

#[contracterror]
#[derive(Copy, Clone, Debug, Eq, PartialEq)]
#[repr(u32)]
pub enum Error {
    CampaignNotFound = 1,
    CouponNotFound = 2,
    AlreadyRedeemed = 3,
    CampaignExpired = 4,
    SupplyExhausted = 5,
    Unauthorized = 6, // caller is not the campaign owner / an authorized delegate (ADR-002)
    InvalidCode = 7,  // empty coupon code
    DuplicateCode = 8, // code already issued *within this campaign* (ADR-009)
    InvalidTerms = 9, // invalid supply/time/name/reward metadata (ADR-009)
    BatchTooLarge = 10, // more than MAX_BATCH codes in one issue_unique
    CodeTooLong = 11, // coupon code exceeds MAX_CODE_LEN
    SharedNotFound = 12, // shared code not registered (Tally)
    AlreadyRegistered = 13, // shared code already registered (Tally)
    PeriodCommitted = 14, // a tally for this (code, period) was already committed
    TallyNotFound = 15, // no tally committed for this (code, period)
    AlreadySettled = 16, // this tally period was already settled
    InvalidTally = 17, // per-attribution counts exceed the committed total
    InvalidSettlement = 18, // payout rate not > 0, settlement not configured, or amount overflow
    AttributionMismatch = 19, // tally credits an address other than the code's registered attributed_to
}

// ═══════════════════════════════════════════════════════════════════
// STORAGE KEYS
// ═══════════════════════════════════════════════════════════════════

const CAMP_CTR: Symbol = symbol_short!("camp_ctr");
const TOKEN_CTR: Symbol = symbol_short!("tok_ctr");

/// Dynamic storage keys.
#[contracttype]
#[derive(Clone)]
pub enum DataKey {
    Campaign(u64),          // Campaign by ID
    Token(u64),             // CouponToken by token ID
    CodeIndex(u64, String), // (campaign_id, code) → token ID — scoped per campaign (ADR-009)
    Delegate(u64, Address), // (campaign_id, delegate) present ⇒ authorized to redeem
    OwnerIndex(Address),    // owner → Vec<u64> of the campaign IDs they created
    Shared(u64, String),       // (campaign_id, code) → SharedCode (Tally)
    Tally(u64, String, u64),   // (campaign_id, code, period) → TallyCommitment
    Settled(u64, String, u64), // (campaign_id, code, period) present ⇒ already settled
}

// ═══════════════════════════════════════════════════════════════════
// CONTRACT
// ═══════════════════════════════════════════════════════════════════

#[contract]
pub struct CouponLedger;

#[contractimpl]
impl CouponLedger {
    // ─── CAMPAIGNS ───────────────────────────────────────────────

    /// Create a new coupon campaign owned by `owner`. Permissionless (ADR-002).
    /// Validates structural terms (ADR-009); reward semantics stay opaque.
    pub fn create_campaign(
        env: Env,
        owner: Address,
        name: String,
        discount_type: String,
        discount_value: u64,
        total_supply: u32,
        valid_until: u64,
    ) -> Result<u64, Error> {
        owner.require_auth();

        // Structural validation (reward type/value are left opaque on purpose).
        if total_supply == 0
            || name.is_empty()
            || name.len() > MAX_NAME_LEN
            || discount_type.is_empty()
            || discount_type.len() > MAX_DISCOUNT_TYPE_LEN
        {
            return Err(Error::InvalidTerms);
        }
        if valid_until <= env.ledger().timestamp() {
            return Err(Error::InvalidTerms);
        }

        let id: u64 = env.storage().instance().get(&CAMP_CTR).unwrap_or(0) + 1;
        env.storage().instance().set(&CAMP_CTR, &id);
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        let campaign = Campaign {
            id,
            owner: owner.clone(),
            name: name.clone(),
            discount_type,
            discount_value,
            total_supply,
            minted: 0,
            burned: 0,
            valid_until,
        };

        let key = DataKey::Campaign(id);
        env.storage().persistent().set(&key, &campaign);
        env.storage()
            .persistent()
            .extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        // Owner → [campaign_id] index so an owner can enumerate their campaigns
        // on-chain (Soroban storage is not iterable). Chain is the source of truth.
        let idx_key = DataKey::OwnerIndex(owner.clone());
        let mut owned: Vec<u64> = env
            .storage()
            .persistent()
            .get(&idx_key)
            .unwrap_or_else(|| Vec::new(&env));
        owned.push_back(id);
        env.storage().persistent().set(&idx_key, &owned);
        env.storage()
            .persistent()
            .extend_ttl(&idx_key, LEDGER_BUMP, EXTEND_TO);

        env.events().publish(
            (symbol_short!("campaign"), symbol_short!("create")),
            (id, owner, name, total_supply),
        );

        log!(&env, "Campaign created: id={}, supply={}", id, total_supply);
        Ok(id)
    }

    /// Get campaign details. Public, no auth.
    pub fn get_campaign(env: Env, campaign_id: u64) -> Result<Campaign, Error> {
        env.storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)
    }

    /// List the campaign IDs created by `owner`. Public, no auth (ADR-008).
    pub fn campaigns_of(env: Env, owner: Address) -> Vec<u64> {
        env.storage()
            .persistent()
            .get(&DataKey::OwnerIndex(owner))
            .unwrap_or_else(|| Vec::new(&env))
    }

    /// Get campaign statistics. Public, no auth.
    pub fn campaign_stats(env: Env, campaign_id: u64) -> Result<CampaignStats, Error> {
        let camp: Campaign = env
            .storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        Ok(CampaignStats {
            total_supply: camp.total_supply,
            minted: camp.minted,
            burned: camp.burned,
            available: camp.minted.saturating_sub(camp.burned),
            is_expired: env.ledger().timestamp() > camp.valid_until,
        })
    }

    /// Re-extend the storage TTL of a campaign (and its owner index). Public —
    /// anyone may pay to keep a long-running campaign's metadata alive past the
    /// default window (ADR-009). Tokens are extended on access.
    pub fn bump_campaign(env: Env, campaign_id: u64) -> Result<(), Error> {
        let camp: Campaign = env
            .storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        let ckey = DataKey::Campaign(campaign_id);
        env.storage()
            .persistent()
            .extend_ttl(&ckey, LEDGER_BUMP, EXTEND_TO);

        let okey = DataKey::OwnerIndex(camp.owner);
        if env.storage().persistent().has(&okey) {
            env.storage()
                .persistent()
                .extend_ttl(&okey, LEDGER_BUMP, EXTEND_TO);
        }
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);
        Ok(())
    }

    /// Re-extend the storage TTL of specific coupons (their Token + code-index
    /// entries) under a campaign. Public — anyone may pay rent to keep coupons
    /// of a long-running campaign alive (ADR-009). Bounded to MAX_BATCH per call;
    /// unknown codes are skipped. (Soroban storage is not iterable, so the caller
    /// supplies the codes to bump — there is no per-campaign token list to walk.)
    pub fn bump_codes(env: Env, campaign_id: u64, codes: Vec<String>) -> Result<(), Error> {
        if codes.is_empty() {
            return Err(Error::InvalidCode);
        }
        if codes.len() > MAX_BATCH {
            return Err(Error::BatchTooLarge);
        }
        for i in 0..codes.len() {
            let code = codes.get(i).unwrap();
            if code.is_empty() {
                return Err(Error::InvalidCode);
            }
            if code.len() > MAX_CODE_LEN {
                return Err(Error::CodeTooLong);
            }

            let code_key = DataKey::CodeIndex(campaign_id, code);
            if let Some(token_id) = env.storage().persistent().get::<_, u64>(&code_key) {
                env.storage()
                    .persistent()
                    .extend_ttl(&code_key, LEDGER_BUMP, EXTEND_TO);
                let token_key = DataKey::Token(token_id);
                if env.storage().persistent().has(&token_key) {
                    env.storage()
                        .persistent()
                        .extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);
                }
            }
        }
        Ok(())
    }

    // ─── DELEGATES (ADR-002: "explicit delegates") ───────────────

    /// Authorize `delegate` to redeem coupons of `campaign_id`. Owner only.
    pub fn add_delegate(
        env: Env,
        owner: Address,
        campaign_id: u64,
        delegate: Address,
    ) -> Result<(), Error> {
        owner.require_auth();
        Self::require_owner(&env, campaign_id, &owner)?;

        let key = DataKey::Delegate(campaign_id, delegate.clone());
        env.storage().persistent().set(&key, &true);
        env.storage()
            .persistent()
            .extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        env.events().publish(
            (symbol_short!("delegate"), symbol_short!("add")),
            (campaign_id, delegate),
        );
        Ok(())
    }

    /// Revoke a delegate's redemption authority for `campaign_id`. Owner only.
    pub fn remove_delegate(
        env: Env,
        owner: Address,
        campaign_id: u64,
        delegate: Address,
    ) -> Result<(), Error> {
        owner.require_auth();
        Self::require_owner(&env, campaign_id, &owner)?;

        let key = DataKey::Delegate(campaign_id, delegate.clone());
        env.storage().persistent().remove(&key);

        env.events().publish(
            (symbol_short!("delegate"), symbol_short!("remove")),
            (campaign_id, delegate),
        );
        Ok(())
    }

    /// Whether `who` is an authorized delegate of `campaign_id`. Public read.
    pub fn is_delegate(env: Env, campaign_id: u64, who: Address) -> bool {
        Self::is_delegate_internal(&env, campaign_id, &who)
    }

    // ─── ISSUANCE (unique codes) ─────────────────────────────────

    /// Issue a batch of unique coupon codes under a campaign. Owner only.
    /// Codes are unique *within the campaign* (ADR-009).
    pub fn issue_unique(
        env: Env,
        owner: Address,
        campaign_id: u64,
        codes: Vec<String>,
    ) -> Result<Vec<u64>, Error> {
        owner.require_auth();

        let mut camp: Campaign = env
            .storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        if owner != camp.owner {
            return Err(Error::Unauthorized);
        }
        // No issuing into an expired campaign.
        if env.ledger().timestamp() > camp.valid_until {
            return Err(Error::CampaignExpired);
        }

        let count = codes.len();
        if count == 0 {
            return Err(Error::InvalidCode); // an empty batch is a signed no-op
        }
        if count > MAX_BATCH {
            return Err(Error::BatchTooLarge);
        }
        let new_minted = camp
            .minted
            .checked_add(count)
            .ok_or(Error::SupplyExhausted)?;
        if new_minted > camp.total_supply {
            return Err(Error::SupplyExhausted);
        }

        let mut token_ids = Vec::new(&env);
        let now = env.ledger().timestamp();
        let zero_ref = BytesN::from_array(&env, &[0u8; 32]);

        for i in 0..count {
            let code = codes.get(i).unwrap();
            if code.is_empty() {
                return Err(Error::InvalidCode);
            }
            if code.len() > MAX_CODE_LEN {
                return Err(Error::CodeTooLong);
            }

            // Per-campaign uniqueness (ADR-009): same string may exist in another campaign.
            let code_key = DataKey::CodeIndex(campaign_id, code.clone());
            if env.storage().persistent().has(&code_key) {
                return Err(Error::DuplicateCode);
            }

            let token_id: u64 = env.storage().instance().get(&TOKEN_CTR).unwrap_or(0) + 1;
            env.storage().instance().set(&TOKEN_CTR, &token_id);

            let token = CouponToken {
                token_id,
                campaign_id,
                code: code.clone(),
                is_burned: false,
                minted_at: now,
                redeemer_ref: zero_ref.clone(),
                burned_at: 0,
            };

            let token_key = DataKey::Token(token_id);
            env.storage().persistent().set(&token_key, &token);
            env.storage()
                .persistent()
                .extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);

            env.storage().persistent().set(&code_key, &token_id);
            env.storage()
                .persistent()
                .extend_ttl(&code_key, LEDGER_BUMP, EXTEND_TO);

            token_ids.push_back(token_id);

            // Code is NOT published in events (prevents harvesting).
            env.events().publish(
                (symbol_short!("coupon"), symbol_short!("issue")),
                (token_id, campaign_id),
            );
        }

        camp.minted = new_minted;
        let camp_key = DataKey::Campaign(campaign_id);
        env.storage().persistent().set(&camp_key, &camp);
        env.storage()
            .persistent()
            .extend_ttl(&camp_key, LEDGER_BUMP, EXTEND_TO);
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        log!(
            &env,
            "Issued {} coupons for campaign {}",
            count,
            campaign_id
        );
        Ok(token_ids)
    }

    // ─── REDEMPTION (burn) ───────────────────────────────────────

    /// Redeem (burn) a unique coupon by `(campaign_id, code)` — the Burn path.
    /// Irreversible single-use; authorized by the campaign owner or a delegate
    /// (ADR-002). `redeemer_ref_hash` is an opaque off-chain commitment (ADR-005/010).
    pub fn redeem_unique(
        env: Env,
        authorizer: Address,
        campaign_id: u64,
        code: String,
        redeemer_ref_hash: BytesN<32>,
    ) -> Result<RedemptionReceipt, Error> {
        authorizer.require_auth();

        let token_id: u64 = env
            .storage()
            .persistent()
            .get(&DataKey::CodeIndex(campaign_id, code.clone()))
            .ok_or(Error::CouponNotFound)?;

        let mut token: CouponToken = env
            .storage()
            .persistent()
            .get(&DataKey::Token(token_id))
            .ok_or(Error::CouponNotFound)?;

        if token.is_burned {
            return Err(Error::AlreadyRedeemed);
        }

        let mut camp: Campaign = env
            .storage()
            .persistent()
            .get(&DataKey::Campaign(token.campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        if authorizer != camp.owner
            && !Self::is_delegate_internal(&env, token.campaign_id, &authorizer)
        {
            return Err(Error::Unauthorized);
        }

        if env.ledger().timestamp() > camp.valid_until {
            return Err(Error::CampaignExpired);
        }

        let now = env.ledger().timestamp();
        let seq = env.ledger().sequence();
        token.is_burned = true;
        token.redeemer_ref = redeemer_ref_hash.clone();
        token.burned_at = now;
        let token_key = DataKey::Token(token_id);
        env.storage().persistent().set(&token_key, &token);
        env.storage()
            .persistent()
            .extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);

        camp.burned += 1;
        let camp_key = DataKey::Campaign(token.campaign_id);
        env.storage().persistent().set(&camp_key, &camp);
        env.storage()
            .persistent()
            .extend_ttl(&camp_key, LEDGER_BUMP, EXTEND_TO);
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        let receipt = RedemptionReceipt {
            token_id,
            code,
            campaign_id: camp.id,
            campaign_name: camp.name,
            discount_type: camp.discount_type,
            discount_value: camp.discount_value,
            redeemer_ref: redeemer_ref_hash,
            burned_at: now,
            ledger_seq: seq,
        };

        env.events().publish(
            (symbol_short!("coupon"), symbol_short!("burn")),
            (token_id, seq),
        );

        log!(&env, "Coupon burned: token={}, ledger={}", token_id, seq);
        Ok(receipt)
    }

    // ─── VERIFICATION (public) ───────────────────────────────────

    /// Verify a coupon by `(campaign_id, code)`. Public, no auth.
    pub fn verify(env: Env, campaign_id: u64, code: String) -> Result<CouponToken, Error> {
        let token_id: u64 = env
            .storage()
            .persistent()
            .get(&DataKey::CodeIndex(campaign_id, code.clone()))
            .ok_or(Error::CouponNotFound)?;

        env.storage()
            .persistent()
            .get(&DataKey::Token(token_id))
            .ok_or(Error::CouponNotFound)
    }

    /// Whether `(campaign_id, code)` is valid and available for redemption. Public.
    pub fn is_valid(env: Env, campaign_id: u64, code: String) -> bool {
        let token_id: Option<u64> = env
            .storage()
            .persistent()
            .get(&DataKey::CodeIndex(campaign_id, code.clone()));
        match token_id {
            None => false,
            Some(id) => {
                let token: Option<CouponToken> =
                    env.storage().persistent().get(&DataKey::Token(id));
                match token {
                    None => false,
                    Some(t) => {
                        if t.is_burned {
                            return false;
                        }
                        let camp: Option<Campaign> = env
                            .storage()
                            .persistent()
                            .get(&DataKey::Campaign(t.campaign_id));
                        match camp {
                            None => false,
                            Some(c) => env.ledger().timestamp() <= c.valid_until,
                        }
                    }
                }
            }
        }
    }

    // ─── GLOBAL STATS (read-only) ────────────────────────────────

    /// Total campaigns created across all owners.
    pub fn total_campaigns(env: Env) -> u64 {
        env.storage().instance().get(&CAMP_CTR).unwrap_or(0)
    }

    /// Total coupons issued across all campaigns.
    pub fn total_minted(env: Env) -> u64 {
        env.storage().instance().get(&TOKEN_CTR).unwrap_or(0)
    }

    // ─── TALLY PROFILE (shared codes) — ADR-003/004/011 ──────────

    /// Register a shared, multi-use code under a campaign, optionally crediting
    /// an attribution target (creator/referrer). Owner only. Shared codes live
    /// in a namespace separate from Burn's unique codes (ADR-011).
    pub fn register_shared(
        env: Env,
        owner: Address,
        campaign_id: u64,
        code: String,
        attributed_to: Option<Address>,
        payout_token: Option<Address>,
        payout_rate: i128,
    ) -> Result<(), Error> {
        owner.require_auth();
        Self::require_owner(&env, campaign_id, &owner)?;
        if code.is_empty() {
            return Err(Error::InvalidCode);
        }
        if code.len() > MAX_CODE_LEN {
            return Err(Error::CodeTooLong);
        }
        // Settlement config is fixed at registration (immutable): if a payout
        // token is set, the rate must be positive. None ⇒ count-only (no payout).
        if payout_token.is_some() && payout_rate <= 0 {
            return Err(Error::InvalidSettlement);
        }
        let key = DataKey::Shared(campaign_id, code.clone());
        if env.storage().persistent().has(&key) {
            return Err(Error::AlreadyRegistered);
        }
        let shared = SharedCode {
            campaign_id,
            code,
            attributed_to: attributed_to.clone(),
            payout_token,
            payout_rate,
            registered_at: env.ledger().timestamp(),
        };
        env.storage().persistent().set(&key, &shared);
        env.storage().persistent().extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        env.events().publish(
            (symbol_short!("shared"), symbol_short!("reg")),
            (campaign_id, attributed_to),
        );
        Ok(())
    }

    /// Get a registered shared code. Public, no auth.
    pub fn get_shared(env: Env, campaign_id: u64, code: String) -> Result<SharedCode, Error> {
        env.storage()
            .persistent()
            .get(&DataKey::Shared(campaign_id, code))
            .ok_or(Error::SharedNotFound)
    }

    /// Commit an epoch's off-chain tally for a shared code: a total `count`, a
    /// `merkle_root` anchoring the signed receipts, and `per_attribution` counts.
    /// Owner only; one commitment per (code, period) — append-only so history is
    /// auditable. Per-attribution counts must not exceed `count`. (ADR-003/004)
    pub fn commit_tally(
        env: Env,
        owner: Address,
        campaign_id: u64,
        code: String,
        period: u64,
        count: u32,
        merkle_root: BytesN<32>,
        per_attribution: Map<Address, u32>,
    ) -> Result<(), Error> {
        owner.require_auth();
        Self::require_owner(&env, campaign_id, &owner)?;

        let shared: SharedCode = env
            .storage()
            .persistent()
            .get(&DataKey::Shared(campaign_id, code.clone()))
            .ok_or(Error::SharedNotFound)?;
        let key = DataKey::Tally(campaign_id, code.clone(), period);
        if env.storage().persistent().has(&key) {
            return Err(Error::PeriodCommitted); // append-only
        }
        // Attribution counts cannot exceed the total, and — if the code has a
        // single registered creator/referrer — may ONLY credit that address
        // (so an owner can't register for A then pay B).
        let mut attributed: u32 = 0;
        for (addr, v) in per_attribution.iter() {
            if let Some(target) = shared.attributed_to.clone() {
                if addr != target {
                    return Err(Error::AttributionMismatch);
                }
            }
            attributed = attributed.checked_add(v).ok_or(Error::InvalidTally)?;
        }
        if attributed > count {
            return Err(Error::InvalidTally);
        }

        let commitment = TallyCommitment {
            period,
            count,
            merkle_root,
            per_attribution,
        };
        env.storage().persistent().set(&key, &commitment);
        env.storage().persistent().extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        env.events().publish(
            (symbol_short!("tally"), symbol_short!("commit")),
            (campaign_id, period, count),
        );
        Ok(())
    }

    /// Get a committed tally for a (shared code, period). Public, no auth —
    /// anyone can audit the count against the merkle_root. (ADR-004)
    pub fn get_tally(
        env: Env,
        campaign_id: u64,
        code: String,
        period: u64,
    ) -> Result<TallyCommitment, Error> {
        env.storage()
            .persistent()
            .get(&DataKey::Tally(campaign_id, code, period))
            .ok_or(Error::TallyNotFound)
    }

    /// Preview the payouts a settlement would produce for a committed tally at
    /// `rate` (token base-units per redemption). Public, read-only — no transfer,
    /// no auth. (ADR-004)
    pub fn compute_payouts(
        env: Env,
        campaign_id: u64,
        code: String,
        period: u64,
    ) -> Result<Vec<Payout>, Error> {
        let shared: SharedCode = env
            .storage()
            .persistent()
            .get(&DataKey::Shared(campaign_id, code.clone()))
            .ok_or(Error::SharedNotFound)?;
        let tally: TallyCommitment = env
            .storage()
            .persistent()
            .get(&DataKey::Tally(campaign_id, code, period))
            .ok_or(Error::TallyNotFound)?;
        let mut out = Vec::new(&env);
        for (addr, n) in tally.per_attribution.iter() {
            let amount = (n as i128)
                .checked_mul(shared.payout_rate)
                .ok_or(Error::InvalidSettlement)?;
            out.push_back(Payout { to: addr, amount });
        }
        Ok(out)
    }

    /// Settle a committed tally: pay each attributed address `count * rate` of
    /// `token`, from the campaign `owner`'s balance (SDP-style automatic payout —
    /// ADR-004). Owner only; a period settles once. `rate` is token base-units
    /// per redemption. Returns the payouts made.
    pub fn settle(
        env: Env,
        owner: Address,
        campaign_id: u64,
        code: String,
        period: u64,
    ) -> Result<Vec<Payout>, Error> {
        owner.require_auth();
        Self::require_owner(&env, campaign_id, &owner)?;

        // Token + rate are fixed at registration (immutable) — settle can't use
        // an arbitrary/zero rate or wrong token to lock a period with a bad payout.
        let shared: SharedCode = env
            .storage()
            .persistent()
            .get(&DataKey::Shared(campaign_id, code.clone()))
            .ok_or(Error::SharedNotFound)?;
        let token = shared.payout_token.clone().ok_or(Error::InvalidSettlement)?;
        if shared.payout_rate <= 0 {
            return Err(Error::InvalidSettlement);
        }

        let tally: TallyCommitment = env
            .storage()
            .persistent()
            .get(&DataKey::Tally(campaign_id, code.clone(), period))
            .ok_or(Error::TallyNotFound)?;

        let settled_key = DataKey::Settled(campaign_id, code, period);
        if env.storage().persistent().has(&settled_key) {
            return Err(Error::AlreadySettled);
        }

        let client = soroban_sdk::token::Client::new(&env, &token);
        let mut out = Vec::new(&env);
        for (addr, n) in tally.per_attribution.iter() {
            let amount = (n as i128)
                .checked_mul(shared.payout_rate)
                .ok_or(Error::InvalidSettlement)?;
            if amount > 0 {
                client.transfer(&owner, &addr, &amount);
                out.push_back(Payout { to: addr, amount });
            }
        }

        env.storage().persistent().set(&settled_key, &true);
        env.storage()
            .persistent()
            .extend_ttl(&settled_key, LEDGER_BUMP, EXTEND_TO);

        env.events().publish(
            (symbol_short!("tally"), symbol_short!("settle")),
            (campaign_id, period, token, shared.payout_rate),
        );
        Ok(out)
    }

    // ─── INTERNAL HELPERS ────────────────────────────────────────

    fn require_owner(env: &Env, campaign_id: u64, caller: &Address) -> Result<(), Error> {
        let camp: Campaign = env
            .storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;
        if *caller != camp.owner {
            return Err(Error::Unauthorized);
        }
        Ok(())
    }

    fn is_delegate_internal(env: &Env, campaign_id: u64, who: &Address) -> bool {
        env.storage()
            .persistent()
            .get(&DataKey::Delegate(campaign_id, who.clone()))
            .unwrap_or(false)
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::{
        testutils::{Address as _, Ledger as _},
        Env, String, Vec,
    };

    fn setup() -> (Env, CouponLedgerClient<'static>) {
        let env = Env::default();
        env.mock_all_auths();
        let contract_id = env.register(CouponLedger, ());
        let client = CouponLedgerClient::new(&env, &contract_id);
        (env, client)
    }

    fn redeemer_hash(env: &Env, b: u8) -> BytesN<32> {
        BytesN::from_array(env, &[b; 32])
    }

    fn codes2(env: &Env) -> Vec<String> {
        let mut codes = Vec::new(env);
        codes.push_back(String::from_str(env, "PK486CZQ"));
        codes.push_back(String::from_str(env, "MLSJXKVM"));
        codes
    }

    /// End-to-end Burn lifecycle in the permissionless model.
    #[test]
    fn test_permissionless_lifecycle() {
        let (env, client) = setup();
        let owner = Address::generate(&env);

        let campaign_id = client.create_campaign(
            &owner,
            &String::from_str(&env, "Cafe Gratis"),
            &String::from_str(&env, "percentage"),
            &10000,
            &100,
            &9999999999,
        );
        assert_eq!(campaign_id, 1);
        assert_eq!(client.total_campaigns(), 1);
        assert_eq!(client.get_campaign(&campaign_id).owner, owner);

        let token_ids = client.issue_unique(&owner, &campaign_id, &codes2(&env));
        assert_eq!(token_ids.len(), 2);
        assert_eq!(client.total_minted(), 2);
        assert!(client.is_valid(&campaign_id, &String::from_str(&env, "PK486CZQ")));

        // Redeem with an opaque commitment — no plaintext PII on-chain (ADR-005/010).
        let href = redeemer_hash(&env, 7);
        let receipt = client.redeem_unique(
            &owner,
            &campaign_id,
            &String::from_str(&env, "PK486CZQ"),
            &href,
        );
        assert_eq!(receipt.discount_value, 10000);
        assert_eq!(receipt.campaign_id, campaign_id);
        assert_eq!(receipt.redeemer_ref, href);

        let token = client.verify(&campaign_id, &String::from_str(&env, "PK486CZQ"));
        assert!(token.is_burned);
        assert_eq!(token.redeemer_ref, href);
        assert!(!client.is_valid(&campaign_id, &String::from_str(&env, "PK486CZQ")));

        assert!(client.is_valid(&campaign_id, &String::from_str(&env, "MLSJXKVM")));

        let stats = client.campaign_stats(&campaign_id);
        assert_eq!(stats.minted, 2);
        assert_eq!(stats.burned, 1);
        assert_eq!(stats.available, 1);
    }

    /// Two independent owners coexist; neither can touch the other's campaign.
    #[test]
    fn test_independent_owners_isolated() {
        let (env, client) = setup();
        let owner_a = Address::generate(&env);
        let owner_b = Address::generate(&env);

        let camp_a = client.create_campaign(
            &owner_a,
            &String::from_str(&env, "A"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        let camp_b = client.create_campaign(
            &owner_b,
            &String::from_str(&env, "B"),
            &String::from_str(&env, "fixed_amount"),
            &500,
            &10,
            &9999999999,
        );
        assert_eq!(camp_a, 1);
        assert_eq!(camp_b, 2);

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "HIJACK01"));
        assert_eq!(
            client.try_issue_unique(&owner_b, &camp_a, &codes),
            Err(Ok(Error::Unauthorized))
        );
    }

    /// Codes are scoped per campaign: the same string lives independently in two
    /// campaigns, with no DuplicateCode and no cross-effect on redemption (ADR-009).
    #[test]
    fn test_same_code_across_campaigns() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let c1 = client.create_campaign(
            &owner,
            &String::from_str(&env, "C1"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        let c2 = client.create_campaign(
            &owner,
            &String::from_str(&env, "C2"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "SAVE10"));
        client.issue_unique(&owner, &c1, &codes);
        client.issue_unique(&owner, &c2, &codes); // same string, different campaign → OK

        let code = String::from_str(&env, "SAVE10");
        assert!(client.is_valid(&c1, &code));
        assert!(client.is_valid(&c2, &code));

        client.redeem_unique(&owner, &c1, &code, &redeemer_hash(&env, 1));
        assert!(!client.is_valid(&c1, &code)); // burned in c1
        assert!(client.is_valid(&c2, &code)); // untouched in c2
    }

    /// A delegate may redeem; a stranger may not (ADR-002).
    #[test]
    fn test_delegate_can_redeem_stranger_cannot() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let delegate = Address::generate(&env);
        let stranger = Address::generate(&env);

        let cid = client.create_campaign(
            &owner,
            &String::from_str(&env, "Promo"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "DELEG001"));
        codes.push_back(String::from_str(&env, "DELEG002"));
        client.issue_unique(&owner, &cid, &codes);

        assert_eq!(
            client.try_redeem_unique(
                &stranger,
                &cid,
                &String::from_str(&env, "DELEG001"),
                &redeemer_hash(&env, 1)
            ),
            Err(Ok(Error::Unauthorized))
        );

        client.add_delegate(&owner, &cid, &delegate);
        assert!(client.is_delegate(&cid, &delegate));
        client.redeem_unique(
            &delegate,
            &cid,
            &String::from_str(&env, "DELEG001"),
            &redeemer_hash(&env, 2),
        );

        client.remove_delegate(&owner, &cid, &delegate);
        assert!(!client.is_delegate(&cid, &delegate));
        assert_eq!(
            client.try_redeem_unique(
                &delegate,
                &cid,
                &String::from_str(&env, "DELEG002"),
                &redeemer_hash(&env, 3)
            ),
            Err(Ok(Error::Unauthorized))
        );
    }

    /// The on-chain owner index lets each account enumerate its own campaigns (ADR-008).
    #[test]
    fn test_campaigns_of_indexes_by_owner() {
        let (env, client) = setup();
        let a = Address::generate(&env);
        let b = Address::generate(&env);

        let a1 = client.create_campaign(
            &a,
            &String::from_str(&env, "A1"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        let a2 = client.create_campaign(
            &a,
            &String::from_str(&env, "A2"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        let b1 = client.create_campaign(
            &b,
            &String::from_str(&env, "B1"),
            &String::from_str(&env, "fixed_amount"),
            &500,
            &10,
            &9999999999,
        );

        let a_ids = client.campaigns_of(&a);
        assert_eq!(a_ids.len(), 2);
        assert_eq!(a_ids.get(0).unwrap(), a1);
        assert_eq!(a_ids.get(1).unwrap(), a2);

        assert_eq!(client.campaigns_of(&b).get(0).unwrap(), b1);
        assert_eq!(client.campaigns_of(&Address::generate(&env)).len(), 0);
    }

    #[test]
    fn test_bump_campaign() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let cid = client.create_campaign(
            &owner,
            &String::from_str(&env, "Long"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        client.bump_campaign(&cid); // ok
        assert_eq!(
            client.try_bump_campaign(&999u64),
            Err(Ok(Error::CampaignNotFound))
        );
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #3)")]
    fn test_double_burn_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Promo"),
            &String::from_str(&env, "fixed_amount"),
            &500,
            &10,
            &9999999999,
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "TESTCODE"));
        client.issue_unique(&owner, &1, &codes);

        let code = String::from_str(&env, "TESTCODE");
        client.redeem_unique(&owner, &1, &code, &redeemer_hash(&env, 1));
        client.redeem_unique(&owner, &1, &code, &redeemer_hash(&env, 2)); // #3 AlreadyRedeemed
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #5)")]
    fn test_issue_exceeds_supply() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Limited"),
            &String::from_str(&env, "percentage"),
            &1000,
            &2,
            &9999999999, // supply = 2
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "AAA"));
        codes.push_back(String::from_str(&env, "BBB"));
        codes.push_back(String::from_str(&env, "CCC"));
        client.issue_unique(&owner, &1, &codes); // 3 > 2 → #5
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #8)")]
    fn test_duplicate_code_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Dupes"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );

        let mut codes1 = Vec::new(&env);
        codes1.push_back(String::from_str(&env, "UNIQ0001"));
        client.issue_unique(&owner, &1, &codes1);

        let mut codes2 = Vec::new(&env);
        codes2.push_back(String::from_str(&env, "UNIQ0001"));
        client.issue_unique(&owner, &1, &codes2); // same code, same campaign → #8
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #9)")]
    fn test_invalid_terms_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        // total_supply == 0 → InvalidTerms (#9)
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Bad"),
            &String::from_str(&env, "percentage"),
            &1000,
            &0,
            &9999999999,
        );
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #9)")]
    fn test_invalid_discount_type_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Bad Reward"),
            &String::from_str(&env, ""),
            &1000,
            &10,
            &9999999999,
        );
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #4)")]
    fn test_issue_after_expiry_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Expiring"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &500, // valid_until = 500 (> now=0, so create is allowed)
        );
        env.ledger().set_timestamp(1000); // past expiry
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "LATE0001"));
        client.issue_unique(&owner, &1, &codes); // #4 CampaignExpired
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #11)")]
    fn test_code_too_long_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Lengthy"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        // 70-char code exceeds MAX_CODE_LEN (64) → #11
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(
            &env,
            "AAAAAAAAAABBBBBBBBBBCCCCCCCCCCDDDDDDDDDDEEEEEEEEEEFFFFFFFFFFGGGGGGGGGG",
        ));
        client.issue_unique(&owner, &1, &codes);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #7)")]
    fn test_empty_batch_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Empty"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        client.issue_unique(&owner, &1, &Vec::new(&env)); // empty batch → #7
    }

    #[test]
    fn test_bump_codes() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let cid = client.create_campaign(
            &owner,
            &String::from_str(&env, "Bump"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "BUMP0001"));
        client.issue_unique(&owner, &cid, &codes);
        client.bump_codes(&cid, &codes); // existing code → ok

        let mut unknown = Vec::new(&env);
        unknown.push_back(String::from_str(&env, "NOPE9999"));
        client.bump_codes(&cid, &unknown); // unknown code → skipped, no error
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #7)")]
    fn test_bump_codes_empty_rejected() {
        let (env, client) = setup();
        client.bump_codes(&1, &Vec::new(&env));
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #11)")]
    fn test_bump_codes_long_code_rejected() {
        let (env, client) = setup();
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(
            &env,
            "AAAAAAAAAABBBBBBBBBBCCCCCCCCCCDDDDDDDDDDEEEEEEEEEEFFFFFFFFFFGGGGGGGGGG",
        ));
        client.bump_codes(&1, &codes);
    }

    // ─── Tally profile (shared codes) ────────────────────────────

    #[test]
    fn test_tally_and_settle() {
        use soroban_sdk::token;
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let creator = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let code = String::from_str(&env, "ROBERTOX");

        // settlement token + rate are fixed at registration (immutable)
        let sac = env.register_stellar_asset_contract_v2(Address::generate(&env));
        let token_addr = sac.address();
        client.register_shared(&owner, &cid, &code, &Some(creator.clone()), &Some(token_addr.clone()), &5i128);
        assert_eq!(client.get_shared(&cid, &code).attributed_to, Some(creator.clone()));

        let mut attr = Map::new(&env);
        attr.set(creator.clone(), 30u32);
        client.commit_tally(&owner, &cid, &code, &1u64, &40u32, &BytesN::from_array(&env, &[9u8; 32]), &attr);
        assert_eq!(client.get_tally(&cid, &code, &1u64).count, 40);

        // preview uses the code's fixed rate (no rate arg) → 30 * 5 = 150
        assert_eq!(client.compute_payouts(&cid, &code, &1u64).get(0).unwrap().amount, 150);

        token::StellarAssetClient::new(&env, &token_addr).mint(&owner, &1000i128);
        let payouts = client.settle(&owner, &cid, &code, &1u64);
        assert_eq!(payouts.len(), 1);
        assert_eq!(token::Client::new(&env, &token_addr).balance(&creator), 150i128);
        assert_eq!(token::Client::new(&env, &token_addr).balance(&owner), 850i128);
    }

    /// A code registered to creator A cannot credit B — attribution is binding.
    #[test]
    #[should_panic(expected = "Error(Contract, #19)")]
    fn test_attribution_must_match_registered() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let a = Address::generate(&env);
        let b = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let code = String::from_str(&env, "ROBERTOX");
        client.register_shared(&owner, &cid, &code, &Some(a.clone()), &None::<Address>, &0i128);
        let mut attr = Map::new(&env);
        attr.set(b.clone(), 5u32); // credit B, not registered A → #19
        client.commit_tally(&owner, &cid, &code, &1u64, &10u32, &BytesN::from_array(&env, &[0u8; 32]), &attr);
    }

    /// rate must be > 0 when a payout token is configured.
    #[test]
    #[should_panic(expected = "Error(Contract, #18)")]
    fn test_register_rate_must_be_positive() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let sac = env.register_stellar_asset_contract_v2(Address::generate(&env));
        client.register_shared(&owner, &cid, &String::from_str(&env, "C"), &None::<Address>, &Some(sac.address()), &0i128); // rate 0 → #18
    }

    /// settle on a code with no payout token configured is rejected (not locked).
    #[test]
    #[should_panic(expected = "Error(Contract, #18)")]
    fn test_settle_unconfigured_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let creator = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let code = String::from_str(&env, "PROMO");
        client.register_shared(&owner, &cid, &code, &Some(creator.clone()), &None::<Address>, &0i128); // no token
        let mut attr = Map::new(&env);
        attr.set(creator.clone(), 5u32);
        client.commit_tally(&owner, &cid, &code, &1u64, &10u32, &BytesN::from_array(&env, &[0u8; 32]), &attr);
        client.settle(&owner, &cid, &code, &1u64); // not configured → #18
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #16)")]
    fn test_double_settle_rejected() {
        use soroban_sdk::token;
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let creator = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let code = String::from_str(&env, "ROBERTOX");
        let sac = env.register_stellar_asset_contract_v2(Address::generate(&env));
        client.register_shared(&owner, &cid, &code, &Some(creator.clone()), &Some(sac.address()), &5i128);
        let mut attr = Map::new(&env);
        attr.set(creator.clone(), 10u32);
        client.commit_tally(&owner, &cid, &code, &1u64, &10u32, &BytesN::from_array(&env, &[1u8; 32]), &attr);
        token::StellarAssetClient::new(&env, &sac.address()).mint(&owner, &1000i128);
        client.settle(&owner, &cid, &code, &1u64);
        client.settle(&owner, &cid, &code, &1u64); // #16
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #14)")]
    fn test_double_commit_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let code = String::from_str(&env, "PROMO");
        client.register_shared(&owner, &cid, &code, &None::<Address>, &None::<Address>, &0i128);
        let attr = Map::new(&env);
        client.commit_tally(&owner, &cid, &code, &1u64, &5u32, &BytesN::from_array(&env, &[0u8; 32]), &attr);
        client.commit_tally(&owner, &cid, &code, &1u64, &7u32, &BytesN::from_array(&env, &[0u8; 32]), &attr); // #14
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #17)")]
    fn test_attribution_exceeds_count_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let creator = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        let code = String::from_str(&env, "PROMO");
        client.register_shared(&owner, &cid, &code, &Some(creator.clone()), &None::<Address>, &0i128);
        let mut attr = Map::new(&env);
        attr.set(creator.clone(), 50u32); // 50 > count 10 → #17
        client.commit_tally(&owner, &cid, &code, &1u64, &10u32, &BytesN::from_array(&env, &[0u8; 32]), &attr);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #6)")]
    fn test_register_shared_requires_owner() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let stranger = Address::generate(&env);
        let cid = client.create_campaign(&owner, &String::from_str(&env, "UGC"), &String::from_str(&env, "percentage"), &1000, &1000, &9999999999);
        client.register_shared(&stranger, &cid, &String::from_str(&env, "X"), &None::<Address>, &None::<Address>, &0i128); // #6
    }
}
