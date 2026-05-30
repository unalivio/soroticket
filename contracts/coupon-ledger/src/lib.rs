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
//! Privacy (ADR-005): the redeemer's identity is only ever a salted hash
//! (`redeemer_ref_hash: BytesN<32>`) computed off-chain. No plaintext PII is
//! stored on-chain.
//!
//! The async **Tally** profile (shared codes, Merkle-anchored counts +
//! attribution + USDC settlement; ADR-003/004) is intentionally NOT in this
//! module — it is the next milestone.

use soroban_sdk::{
    contract, contractimpl, contracttype, contracterror,
    symbol_short, Address, BytesN, Env, String, Symbol, Vec, log,
};

// ═══════════════════════════════════════════════════════════════════
// TTL CONSTANTS — keep storage alive across redemption windows.
// ═══════════════════════════════════════════════════════════════════

const LEDGER_BUMP: u32 = 535_680;   // ~31 days
const EXTEND_TO: u32 = 2_678_400;   // ~155 days

// ═══════════════════════════════════════════════════════════════════
// TYPES — On-chain data structures
// ═══════════════════════════════════════════════════════════════════

/// Campaign — a collection of unique coupon codes with shared terms.
///
/// Owned by `owner`: the account that created it and the authority for all
/// privileged operations on it (ADR-002). There is no global admin.
#[contracttype]
#[derive(Clone, Debug)]
pub struct Campaign {
    pub id: u64,
    pub owner: Address,          // creator; the authority for this campaign (ADR-002)
    pub name: String,
    pub discount_type: String,   // "percentage", "fixed_amount", "free_item"
    pub discount_value: u64,     // value in cents (e.g., 2000 = 20.00 or 20%)
    pub total_supply: u32,       // max coupons that can be issued
    pub minted: u32,             // total issued so far
    pub burned: u32,             // total redeemed (burned) so far
    pub valid_until: u64,        // expiration timestamp (unix seconds)
}

/// CouponToken — an individual unique single-use coupon (Burn profile).
/// Each token has its own code and can only be burned once.
#[contracttype]
#[derive(Clone, Debug)]
pub struct CouponToken {
    pub token_id: u64,
    pub campaign_id: u64,
    pub code: String,            // unique code (e.g., "PK486CZQ")
    pub is_burned: bool,
    pub minted_at: u64,
    /// Salted hash of the redeemer's identity, supplied off-chain at redemption.
    /// All-zero until burned. NEVER plaintext PII (ADR-005).
    pub redeemer_ref: BytesN<32>,
    pub burned_at: u64,          // 0 until burned
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
    pub redeemer_ref: BytesN<32>,  // the salted hash that was committed (ADR-005)
    pub burned_at: u64,
    pub ledger_seq: u32,           // ledger sequence at redemption — the on-chain anchor
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
    Unauthorized = 6,   // caller is not the campaign owner / an authorized delegate (ADR-002)
    InvalidCode = 7,    // empty or malformed coupon code
    DuplicateCode = 8,  // coupon code already issued
}

// ═══════════════════════════════════════════════════════════════════
// STORAGE KEYS
// ═══════════════════════════════════════════════════════════════════

// Global, monotonic ID counters (instance storage). These allocate IDs only;
// they carry no authority — there is no admin singleton anymore (ADR-002).
const CAMP_CTR: Symbol = symbol_short!("camp_ctr");
const TOKEN_CTR: Symbol = symbol_short!("tok_ctr");

/// Dynamic storage keys.
#[contracttype]
#[derive(Clone)]
pub enum DataKey {
    Campaign(u64),            // Campaign by ID
    Token(u64),               // CouponToken by token ID
    CodeIndex(String),        // coupon code → token ID
    Delegate(u64, Address),   // (campaign_id, delegate) present ⇒ authorized to redeem
}

// ═══════════════════════════════════════════════════════════════════
// CONTRACT
// ═══════════════════════════════════════════════════════════════════

#[contract]
pub struct CouponLedger;

#[contractimpl]
impl CouponLedger {
    // ─── CAMPAIGNS ───────────────────────────────────────────────

    /// Create a new coupon campaign owned by `owner`.
    ///
    /// Permissionless (ADR-002): any account can call this for itself. The
    /// only authorization is `owner.require_auth()` — there is no admin.
    /// Returns the new campaign ID.
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
        env.storage().persistent().extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        // Include owner so indexers can attribute campaigns in the permissionless model.
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

    /// Get campaign statistics. Public, no auth.
    pub fn campaign_stats(env: Env, campaign_id: u64) -> Result<CampaignStats, Error> {
        let camp: Campaign = env.storage()
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

    // ─── DELEGATES (ADR-002: "explicit delegates") ───────────────

    /// Authorize `delegate` to redeem coupons of `campaign_id`.
    /// Only the campaign owner may manage delegates.
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
        env.storage().persistent().extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        env.events().publish(
            (symbol_short!("delegate"), symbol_short!("add")),
            (campaign_id, delegate),
        );
        Ok(())
    }

    /// Revoke a delegate's redemption authority for `campaign_id`.
    /// Only the campaign owner may manage delegates.
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

    /// Issue a batch of unique coupon codes under a campaign.
    /// Each code becomes a unique single-use on-chain token.
    ///
    /// Authority: the campaign **owner** only. Issuing supply is an
    /// ownership-level action and is not delegated (delegates may redeem,
    /// not mint). Returns the list of token IDs.
    pub fn issue_unique(
        env: Env,
        owner: Address,
        campaign_id: u64,
        codes: Vec<String>,
    ) -> Result<Vec<u64>, Error> {
        owner.require_auth();

        let mut camp: Campaign = env.storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        if owner != camp.owner {
            return Err(Error::Unauthorized);
        }

        let count = codes.len();
        // Checked supply integrity — cannot oversell, and cannot overflow.
        let new_minted = camp.minted.checked_add(count).ok_or(Error::SupplyExhausted)?;
        if new_minted > camp.total_supply {
            return Err(Error::SupplyExhausted);
        }

        let mut token_ids = Vec::new(&env);
        let now = env.ledger().timestamp();
        let zero_ref = BytesN::from_array(&env, &[0u8; 32]);

        for i in 0..count {
            let code = codes.get(i).unwrap();
            if code.len() == 0 {
                return Err(Error::InvalidCode);
            }

            // Reject duplicate coupon codes (across all campaigns).
            let code_key = DataKey::CodeIndex(code.clone());
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
            env.storage().persistent().extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);

            env.storage().persistent().set(&code_key, &token_id);
            env.storage().persistent().extend_ttl(&code_key, LEDGER_BUMP, EXTEND_TO);

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
        env.storage().persistent().extend_ttl(&camp_key, LEDGER_BUMP, EXTEND_TO);
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        log!(&env, "Issued {} coupons for campaign {}", count, campaign_id);
        Ok(token_ids)
    }

    // ─── REDEMPTION (burn) ───────────────────────────────────────

    /// Redeem (burn) a unique coupon by its code — the synchronous Burn path.
    /// Irreversible single-use: a second redemption fails with `AlreadyRedeemed`.
    ///
    /// Authority: the campaign **owner or an authorized delegate** (ADR-002).
    /// `redeemer_ref_hash` is a salted hash of the redeemer's identity computed
    /// off-chain — never plaintext PII (ADR-005). Returns a receipt.
    pub fn redeem_unique(
        env: Env,
        authorizer: Address,
        code: String,
        redeemer_ref_hash: BytesN<32>,
    ) -> Result<RedemptionReceipt, Error> {
        authorizer.require_auth();

        let token_id: u64 = env.storage()
            .persistent()
            .get(&DataKey::CodeIndex(code.clone()))
            .ok_or(Error::CouponNotFound)?;

        let mut token: CouponToken = env.storage()
            .persistent()
            .get(&DataKey::Token(token_id))
            .ok_or(Error::CouponNotFound)?;

        // Enforce single-use — the genuine on-chain guarantee of the Burn profile.
        if token.is_burned {
            return Err(Error::AlreadyRedeemed);
        }

        let mut camp: Campaign = env.storage()
            .persistent()
            .get(&DataKey::Campaign(token.campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        // Owner or delegate may authorize a redemption (ADR-002).
        if authorizer != camp.owner
            && !Self::is_delegate_internal(&env, token.campaign_id, &authorizer)
        {
            return Err(Error::Unauthorized);
        }

        if env.ledger().timestamp() > camp.valid_until {
            return Err(Error::CampaignExpired);
        }

        // BURN — irreversible. Store only the salted hash, never plaintext (ADR-005).
        let now = env.ledger().timestamp();
        let seq = env.ledger().sequence();
        token.is_burned = true;
        token.redeemer_ref = redeemer_ref_hash.clone();
        token.burned_at = now;
        let token_key = DataKey::Token(token_id);
        env.storage().persistent().set(&token_key, &token);
        env.storage().persistent().extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);

        camp.burned += 1;
        let camp_key = DataKey::Campaign(token.campaign_id);
        env.storage().persistent().set(&camp_key, &camp);
        env.storage().persistent().extend_ttl(&camp_key, LEDGER_BUMP, EXTEND_TO);
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

        // Neither the raw code nor the redeemer hash is published in events.
        env.events().publish(
            (symbol_short!("coupon"), symbol_short!("burn")),
            (token_id, seq),
        );

        log!(&env, "Coupon burned: token={}, ledger={}", token_id, seq);
        Ok(receipt)
    }

    // ─── VERIFICATION (public) ───────────────────────────────────

    /// Verify a coupon by its code. Public, no auth.
    pub fn verify(env: Env, code: String) -> Result<CouponToken, Error> {
        let token_id: u64 = env.storage()
            .persistent()
            .get(&DataKey::CodeIndex(code.clone()))
            .ok_or(Error::CouponNotFound)?;

        env.storage()
            .persistent()
            .get(&DataKey::Token(token_id))
            .ok_or(Error::CouponNotFound)
    }

    /// Check if a coupon code is valid and available for redemption. Public.
    pub fn is_valid(env: Env, code: String) -> bool {
        let token_id: Option<u64> =
            env.storage().persistent().get(&DataKey::CodeIndex(code.clone()));
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
                        let camp: Option<Campaign> =
                            env.storage().persistent().get(&DataKey::Campaign(t.campaign_id));
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

    // ─── INTERNAL HELPERS ────────────────────────────────────────

    /// Require that `caller` is the owner of `campaign_id`.
    fn require_owner(env: &Env, campaign_id: u64, caller: &Address) -> Result<(), Error> {
        let camp: Campaign = env.storage()
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

    /// End-to-end Burn lifecycle in the permissionless model: no initialize,
    /// no admin — an owner creates and operates its own campaign.
    #[test]
    fn test_permissionless_lifecycle() {
        let (env, client) = setup();
        let owner = Address::generate(&env);

        // No initialize() needed — anyone creates a campaign from their own account.
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
        assert!(client.is_valid(&String::from_str(&env, "PK486CZQ")));

        // Redeem with a salted hash — no plaintext PII on-chain (ADR-005).
        let href = redeemer_hash(&env, 7);
        let receipt = client.redeem_unique(
            &owner,
            &String::from_str(&env, "PK486CZQ"),
            &href,
        );
        assert_eq!(receipt.discount_value, 10000);
        assert_eq!(receipt.campaign_id, campaign_id);
        assert_eq!(receipt.redeemer_ref, href);

        // The burned token stores the hash, not a name.
        let token = client.verify(&String::from_str(&env, "PK486CZQ"));
        assert!(token.is_burned);
        assert_eq!(token.redeemer_ref, href);
        assert!(!client.is_valid(&String::from_str(&env, "PK486CZQ")));

        // Second coupon untouched.
        assert!(client.is_valid(&String::from_str(&env, "MLSJXKVM")));

        let stats = client.campaign_stats(&campaign_id);
        assert_eq!(stats.minted, 2);
        assert_eq!(stats.burned, 1);
        assert_eq!(stats.available, 1);
    }

    /// Two independent owners coexist with no central authority, and neither
    /// can touch the other's campaign (ADR-002).
    #[test]
    fn test_independent_owners_isolated() {
        let (env, client) = setup();
        let owner_a = Address::generate(&env);
        let owner_b = Address::generate(&env);

        let camp_a = client.create_campaign(
            &owner_a,
            &String::from_str(&env, "A"),
            &String::from_str(&env, "percentage"),
            &1000, &10, &9999999999,
        );
        let camp_b = client.create_campaign(
            &owner_b,
            &String::from_str(&env, "B"),
            &String::from_str(&env, "fixed_amount"),
            &500, &10, &9999999999,
        );
        assert_eq!(camp_a, 1);
        assert_eq!(camp_b, 2);

        // Owner B cannot issue into Owner A's campaign.
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "HIJACK01"));
        assert_eq!(
            client.try_issue_unique(&owner_b, &camp_a, &codes),
            Err(Ok(Error::Unauthorized))
        );
    }

    /// A delegate may redeem; a stranger may not (ADR-002).
    #[test]
    fn test_delegate_can_redeem_stranger_cannot() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        let delegate = Address::generate(&env);
        let stranger = Address::generate(&env);

        let campaign_id = client.create_campaign(
            &owner,
            &String::from_str(&env, "Promo"),
            &String::from_str(&env, "percentage"),
            &1000, &10, &9999999999,
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "DELEG001"));
        codes.push_back(String::from_str(&env, "DELEG002"));
        client.issue_unique(&owner, &campaign_id, &codes);

        // Stranger cannot redeem.
        assert_eq!(
            client.try_redeem_unique(
                &stranger,
                &String::from_str(&env, "DELEG001"),
                &redeemer_hash(&env, 1),
            ),
            Err(Ok(Error::Unauthorized))
        );

        // Owner authorizes the delegate, who can now redeem.
        client.add_delegate(&owner, &campaign_id, &delegate);
        assert!(client.is_delegate(&campaign_id, &delegate));
        client.redeem_unique(
            &delegate,
            &String::from_str(&env, "DELEG001"),
            &redeemer_hash(&env, 2),
        );

        // After revocation the delegate loses authority.
        client.remove_delegate(&owner, &campaign_id, &delegate);
        assert!(!client.is_delegate(&campaign_id, &delegate));
        assert_eq!(
            client.try_redeem_unique(
                &delegate,
                &String::from_str(&env, "DELEG002"),
                &redeemer_hash(&env, 3),
            ),
            Err(Ok(Error::Unauthorized))
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
            &500, &10, &9999999999,
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "TESTCODE"));
        client.issue_unique(&owner, &1, &codes);

        let code = String::from_str(&env, "TESTCODE");
        client.redeem_unique(&owner, &code, &redeemer_hash(&env, 1));
        // Second redeem must fail (Error #3 = AlreadyRedeemed).
        client.redeem_unique(&owner, &code, &redeemer_hash(&env, 2));
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
            &1000, &2, &9999999999, // supply = 2
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "AAA"));
        codes.push_back(String::from_str(&env, "BBB"));
        codes.push_back(String::from_str(&env, "CCC"));
        // 3 > 2 → SupplyExhausted (#5)
        client.issue_unique(&owner, &1, &codes);
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
            &1000, &10, &9999999999,
        );

        let mut codes1 = Vec::new(&env);
        codes1.push_back(String::from_str(&env, "UNIQ0001"));
        client.issue_unique(&owner, &1, &codes1);

        let mut codes2 = Vec::new(&env);
        codes2.push_back(String::from_str(&env, "UNIQ0001"));
        // Duplicate code → DuplicateCode (#8)
        client.issue_unique(&owner, &1, &codes2);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #4)")]
    fn test_expired_campaign_rejected() {
        let (env, client) = setup();
        let owner = Address::generate(&env);
        client.create_campaign(
            &owner,
            &String::from_str(&env, "Expiring"),
            &String::from_str(&env, "percentage"),
            &1000, &10, &500, // valid_until = 500
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "EXPIRED1"));
        client.issue_unique(&owner, &1, &codes);

        // Advance ledger time past expiry.
        env.ledger().set_timestamp(1000);
        client.redeem_unique(
            &owner,
            &String::from_str(&env, "EXPIRED1"),
            &redeemer_hash(&env, 1),
        );
    }
}
