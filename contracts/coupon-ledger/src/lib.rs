#![no_std]

use soroban_sdk::{
    contract, contractimpl, contracttype, contracterror,
    symbol_short, Address, Env, String, Symbol, Vec, log,
};

// ═══════════════════════════════════════════════════════════════════
// TTL CONSTANTS (VULN-05)
// ═══════════════════════════════════════════════════════════════════

const LEDGER_BUMP: u32 = 535_680;   // ~31 days
const EXTEND_TO: u32 = 2_678_400;   // ~155 days

// ═══════════════════════════════════════════════════════════════════
// TYPES — On-chain data structures
// ═══════════════════════════════════════════════════════════════════

/// Campaign — a collection of coupon tokens with shared terms.
#[contracttype]
#[derive(Clone, Debug)]
pub struct Campaign {
    pub id: u64,
    pub name: String,
    pub discount_type: String,   // "percentage", "fixed_amount", "free_item"
    pub discount_value: u64,     // value in cents (e.g., 2000 = 20.00 or 20%)
    pub total_supply: u32,       // max coupons that can be minted
    pub minted: u32,             // total minted so far
    pub burned: u32,             // total redeemed (burned) so far
    pub valid_until: u64,        // expiration timestamp
    pub owner: Address,          // campaign creator
}

/// CouponToken — individual NFT-like coupon.
/// Each coupon is unique, has its own code, and can only be burned once.
#[contracttype]
#[derive(Clone, Debug)]
pub struct CouponToken {
    pub token_id: u64,
    pub campaign_id: u64,
    pub code: String,            // unique 8-char code (e.g., "PK486CZQ")
    pub is_burned: bool,
    pub minted_at: u64,
    pub burned_by: String,       // redeemer name (empty if not burned)
    pub burned_at: u64,          // 0 if not burned
    pub reference: String,       // reference number (empty if not burned)
}

/// RedemptionReceipt — returned after a successful burn/redeem.
#[contracttype]
#[derive(Clone, Debug)]
pub struct RedemptionReceipt {
    pub token_id: u64,
    pub code: String,
    pub campaign_name: String,
    pub discount_type: String,
    pub discount_value: u64,
    pub reference: String,
    pub burned_by: String,
    pub burned_at: u64,
    pub tx_sequence: u64,        // on-chain sequence number
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
    Unauthorized = 6,
    InvalidCode = 7,
    DuplicateCode = 8, // VULN-13: duplicate coupon code in mint_batch
}

// ═══════════════════════════════════════════════════════════════════
// STORAGE KEYS
// ═══════════════════════════════════════════════════════════════════

const ADMIN: Symbol = symbol_short!("admin");
const CAMP_CTR: Symbol = symbol_short!("camp_ctr");
const TOKEN_CTR: Symbol = symbol_short!("tok_ctr");
const BURN_CTR: Symbol = symbol_short!("burn_ctr");

/// Dynamic storage keys for campaigns, tokens, and code lookups.
#[contracttype]
#[derive(Clone)]
pub enum DataKey {
    Campaign(u64),       // Campaign by ID
    Token(u64),          // CouponToken by token ID
    CodeIndex(String),   // Maps coupon code → token ID
}

// ═══════════════════════════════════════════════════════════════════
// CONTRACT
// ═══════════════════════════════════════════════════════════════════

#[contract]
pub struct CouponLedger;

#[contractimpl]
impl CouponLedger {
    // ─── INITIALIZATION ──────────────────────────────────────────

    /// Initialize the contract with an admin address.
    /// The admin can create campaigns and mint coupons.
    /// VULN-06: Returns Result instead of panicking on re-init.
    pub fn initialize(env: Env, admin: Address) -> Result<(), Error> {
        if env.storage().instance().has(&ADMIN) {
            return Err(Error::Unauthorized);
        }
        env.storage().instance().set(&ADMIN, &admin);
        env.storage().instance().set(&CAMP_CTR, &0u64);
        env.storage().instance().set(&TOKEN_CTR, &0u64);
        env.storage().instance().set(&BURN_CTR, &0u64);

        // VULN-05: extend instance TTL after writes
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        // Event: contract initialized
        env.events().publish(
            (symbol_short!("init"),),
            admin,
        );

        Ok(())
    }

    // ─── CAMPAIGNS ───────────────────────────────────────────────

    /// Create a new coupon campaign. Only the admin can call this.
    /// Returns the campaign ID.
    pub fn create_campaign(
        env: Env,
        caller: Address,
        name: String,
        discount_type: String,
        discount_value: u64,
        total_supply: u32,
        valid_until: u64,
    ) -> Result<u64, Error> {
        caller.require_auth();
        Self::require_admin(&env, &caller)?;

        let id: u64 = env.storage().instance().get(&CAMP_CTR).unwrap_or(0) + 1;
        env.storage().instance().set(&CAMP_CTR, &id);

        // VULN-05: extend instance TTL after write
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        let campaign = Campaign {
            id,
            name: name.clone(),
            discount_type: discount_type.clone(),
            discount_value,
            total_supply,
            minted: 0,
            burned: 0,
            valid_until,
            owner: caller,
        };

        let key = DataKey::Campaign(id);
        env.storage().persistent().set(&key, &campaign);
        // VULN-05: extend persistent TTL after write
        env.storage().persistent().extend_ttl(&key, LEDGER_BUMP, EXTEND_TO);

        // Event: campaign created
        env.events().publish(
            (symbol_short!("campaign"), symbol_short!("create")),
            (id, name, total_supply),
        );

        log!(&env, "Campaign created: id={}, supply={}", id, total_supply);
        Ok(id)
    }

    /// Get campaign details.
    pub fn get_campaign(env: Env, campaign_id: u64) -> Result<Campaign, Error> {
        env.storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)
    }

    /// Get campaign statistics.
    pub fn campaign_stats(env: Env, campaign_id: u64) -> Result<CampaignStats, Error> {
        let camp: Campaign = env.storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        Ok(CampaignStats {
            total_supply: camp.total_supply,
            minted: camp.minted,
            burned: camp.burned,
            // VULN-15: saturating_sub prevents underflow
            available: camp.minted.saturating_sub(camp.burned),
            is_expired: env.ledger().timestamp() > camp.valid_until,
        })
    }

    // ─── MINTING (Token Creation) ────────────────────────────────

    /// Mint a batch of coupon tokens for a campaign.
    /// Each code becomes a unique on-chain NFT-like token.
    /// Returns the list of token IDs.
    pub fn mint_batch(
        env: Env,
        caller: Address,
        campaign_id: u64,
        codes: Vec<String>,
    ) -> Result<Vec<u64>, Error> {
        caller.require_auth();
        Self::require_admin(&env, &caller)?;

        let mut camp: Campaign = env.storage()
            .persistent()
            .get(&DataKey::Campaign(campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        let count = codes.len() as u32;
        if camp.minted + count > camp.total_supply {
            return Err(Error::SupplyExhausted);
        }

        let mut token_ids = Vec::new(&env);
        let now = env.ledger().timestamp();

        for i in 0..codes.len() {
            let code = codes.get(i).unwrap();

            // VULN-13: reject duplicate coupon codes
            let code_key = DataKey::CodeIndex(code.clone());
            if env.storage().persistent().has(&code_key) {
                return Err(Error::DuplicateCode);
            }

            // Global token counter
            let token_id: u64 = env.storage().instance().get(&TOKEN_CTR).unwrap_or(0) + 1;
            env.storage().instance().set(&TOKEN_CTR, &token_id);

            let token = CouponToken {
                token_id,
                campaign_id,
                code: code.clone(),
                is_burned: false,
                minted_at: now,
                burned_by: String::from_str(&env, ""),
                burned_at: 0,
                reference: String::from_str(&env, ""),
            };

            // Store token by ID and by code (for lookup)
            let token_key = DataKey::Token(token_id);
            env.storage().persistent().set(&token_key, &token);
            // VULN-05: extend persistent TTL after write
            env.storage().persistent().extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);

            env.storage().persistent().set(&code_key, &token_id);
            // VULN-05: extend persistent TTL after write
            env.storage().persistent().extend_ttl(&code_key, LEDGER_BUMP, EXTEND_TO);

            token_ids.push_back(token_id);

            // Event: coupon minted (VULN-14: code NOT published to prevent harvesting)
            env.events().publish(
                (symbol_short!("coupon"), symbol_short!("mint")),
                (token_id, campaign_id),
            );
        }

        // Update campaign mint count
        camp.minted += count;
        let camp_key = DataKey::Campaign(campaign_id);
        env.storage().persistent().set(&camp_key, &camp);
        // VULN-05: extend persistent TTL after write
        env.storage().persistent().extend_ttl(&camp_key, LEDGER_BUMP, EXTEND_TO);

        // VULN-05: extend instance TTL after writes
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        log!(&env, "Minted {} coupons for campaign {}", count, campaign_id);
        Ok(token_ids)
    }

    // ─── REDEMPTION (Token Burn) ─────────────────────────────────

    /// Redeem (burn) a coupon token by its code.
    /// This is the core operation — irreversible destruction of the token.
    /// Returns a RedemptionReceipt as cryptographic proof.
    pub fn redeem(
        env: Env,
        caller: Address,
        code: String,
        redeemer_name: String,
        reference: String,
    ) -> Result<RedemptionReceipt, Error> {
        caller.require_auth();
        Self::require_admin(&env, &caller)?;

        // Lookup token by code
        let token_id: u64 = env.storage()
            .persistent()
            .get(&DataKey::CodeIndex(code.clone()))
            .ok_or(Error::CouponNotFound)?;

        let mut token: CouponToken = env.storage()
            .persistent()
            .get(&DataKey::Token(token_id))
            .ok_or(Error::CouponNotFound)?;

        // Enforce single-use — the core reason for using a smart contract
        if token.is_burned {
            return Err(Error::AlreadyRedeemed);
        }

        // Check campaign expiration
        let camp: Campaign = env.storage()
            .persistent()
            .get(&DataKey::Campaign(token.campaign_id))
            .ok_or(Error::CampaignNotFound)?;

        if env.ledger().timestamp() > camp.valid_until {
            return Err(Error::CampaignExpired);
        }

        // BURN the token — irreversible
        let now = env.ledger().timestamp();
        token.is_burned = true;
        token.burned_by = redeemer_name.clone();
        token.burned_at = now;
        token.reference = reference.clone();
        let token_key = DataKey::Token(token_id);
        env.storage().persistent().set(&token_key, &token);
        // VULN-05: extend persistent TTL after write
        env.storage().persistent().extend_ttl(&token_key, LEDGER_BUMP, EXTEND_TO);

        // Update campaign burn count
        let mut camp_updated = camp.clone();
        camp_updated.burned += 1;
        let camp_key = DataKey::Campaign(token.campaign_id);
        env.storage().persistent().set(&camp_key, &camp_updated);
        // VULN-05: extend persistent TTL after write
        env.storage().persistent().extend_ttl(&camp_key, LEDGER_BUMP, EXTEND_TO);

        // Global burn counter
        let burns: u64 = env.storage().instance().get(&BURN_CTR).unwrap_or(0) + 1;
        env.storage().instance().set(&BURN_CTR, &burns);

        // VULN-05: extend instance TTL after writes
        env.storage().instance().extend_ttl(LEDGER_BUMP, EXTEND_TO);

        // Build receipt
        let receipt = RedemptionReceipt {
            token_id,
            code: code.clone(),
            campaign_name: camp.name,
            discount_type: camp.discount_type,
            discount_value: camp.discount_value,
            reference: reference.clone(),
            burned_by: redeemer_name.clone(),
            burned_at: now,
            tx_sequence: burns,
        };

        // VULN-14: hash coupon codes in events instead of publishing raw
        // Event: coupon burned (VULN-14: code NOT published, only token_id + reference)
        env.events().publish(
            (symbol_short!("coupon"), symbol_short!("burn")),
            (token_id, reference.clone()),
        );

        log!(&env, "Coupon burned: token={}, code=<hashed>", token_id);
        Ok(receipt)
    }

    // ─── VERIFICATION (Public) ───────────────────────────────────

    /// Verify a coupon by its code. Anyone can call this.
    /// No auth required — this is the public verification interface.
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

    /// Check if a coupon code is valid and available for redemption.
    pub fn is_valid(env: Env, code: String) -> bool {
        let token_id: Option<u64> = env.storage().persistent().get(&DataKey::CodeIndex(code.clone()));
        match token_id {
            None => false,
            Some(id) => {
                let token: Option<CouponToken> = env.storage().persistent().get(&DataKey::Token(id));
                match token {
                    None => false,
                    Some(t) => {
                        if t.is_burned { return false; }
                        let camp: Option<Campaign> = env.storage().persistent().get(&DataKey::Campaign(t.campaign_id));
                        match camp {
                            None => false,
                            Some(c) => env.ledger().timestamp() <= c.valid_until,
                        }
                    }
                }
            }
        }
    }

    // ─── GLOBAL STATS ────────────────────────────────────────────

    /// Total coupons minted across all campaigns.
    pub fn total_minted(env: Env) -> u64 {
        env.storage().instance().get(&TOKEN_CTR).unwrap_or(0)
    }

    /// Total coupons burned (redeemed) across all campaigns.
    pub fn total_burned(env: Env) -> u64 {
        env.storage().instance().get(&BURN_CTR).unwrap_or(0)
    }

    /// Total campaigns created.
    pub fn total_campaigns(env: Env) -> u64 {
        env.storage().instance().get(&CAMP_CTR).unwrap_or(0)
    }

    // ─── INTERNAL HELPERS ────────────────────────────────────────

    fn require_admin(env: &Env, caller: &Address) -> Result<(), Error> {
        let admin: Address = env.storage()
            .instance()
            .get(&ADMIN)
            .ok_or(Error::Unauthorized)?;
        if *caller != admin {
            return Err(Error::Unauthorized);
        }
        Ok(())
    }
}

#[cfg(test)]
mod test {
    use super::*;
    use soroban_sdk::{testutils::Address as _, Env};

    #[test]
    fn test_full_lifecycle() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register(CouponLedger, ());
        let client = CouponLedgerClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        // Initialize (VULN-06: now returns Result)
        client.initialize(&admin);

        // Create campaign
        let campaign_id = client.create_campaign(
            &admin,
            &String::from_str(&env, "Cafe Gratis"),
            &String::from_str(&env, "percentage"),
            &10000, // 100%
            &100,
            &9999999999,
        );
        assert_eq!(campaign_id, 1);
        assert_eq!(client.total_campaigns(), 1);

        // Mint coupons
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "PK486CZQ"));
        codes.push_back(String::from_str(&env, "MLSJXKVM"));

        let token_ids = client.mint_batch(&admin, &campaign_id, &codes);
        assert_eq!(token_ids.len(), 2);
        assert_eq!(client.total_minted(), 2);

        // Verify coupon is valid
        assert!(client.is_valid(&String::from_str(&env, "PK486CZQ")));

        // Redeem first coupon
        let receipt = client.redeem(
            &admin,
            &String::from_str(&env, "PK486CZQ"),
            &String::from_str(&env, "Fabian"),
            &String::from_str(&env, "REF-20260330-8118"),
        );
        assert_eq!(receipt.tx_sequence, 1);
        assert_eq!(receipt.discount_value, 10000);

        // Verify it's burned
        let token = client.verify(&String::from_str(&env, "PK486CZQ"));
        assert!(token.is_burned);
        assert!(!client.is_valid(&String::from_str(&env, "PK486CZQ")));

        // Second coupon still valid
        assert!(client.is_valid(&String::from_str(&env, "MLSJXKVM")));

        // Stats
        let stats = client.campaign_stats(&campaign_id);
        assert_eq!(stats.minted, 2);
        assert_eq!(stats.burned, 1);
        assert_eq!(stats.available, 1);

        assert_eq!(client.total_burned(), 1);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #6)")]
    fn test_double_initialize_rejected() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register(CouponLedger, ());
        let client = CouponLedgerClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        // First init — OK
        client.initialize(&admin);
        // Second init — must fail (Error #6 = Unauthorized)
        client.initialize(&admin);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #3)")]
    fn test_double_burn_rejected() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register(CouponLedger, ());
        let client = CouponLedgerClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        client.initialize(&admin);
        client.create_campaign(
            &admin,
            &String::from_str(&env, "Promo"),
            &String::from_str(&env, "fixed_amount"),
            &500,
            &10,
            &9999999999,
        );

        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "TESTCODE"));
        client.mint_batch(&admin, &1, &codes);

        let code = String::from_str(&env, "TESTCODE");
        let name = String::from_str(&env, "Employee");
        let ref1 = String::from_str(&env, "REF-001");
        let ref2 = String::from_str(&env, "REF-002");

        // First redeem — OK
        client.redeem(&admin, &code, &name, &ref1);
        // Second redeem — must fail (Error #3 = AlreadyRedeemed)
        client.redeem(&admin, &code, &name, &ref2);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #5)")]
    fn test_mint_exceeds_supply() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register(CouponLedger, ());
        let client = CouponLedgerClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        client.initialize(&admin);
        client.create_campaign(
            &admin,
            &String::from_str(&env, "Limited"),
            &String::from_str(&env, "percentage"),
            &1000,
            &2, // only 2 allowed
            &9999999999,
        );

        // Try to mint 3 — must fail (Error #5 = SupplyExhausted)
        let mut codes = Vec::new(&env);
        codes.push_back(String::from_str(&env, "AAA"));
        codes.push_back(String::from_str(&env, "BBB"));
        codes.push_back(String::from_str(&env, "CCC"));
        client.mint_batch(&admin, &1, &codes);
    }

    #[test]
    #[should_panic(expected = "Error(Contract, #8)")]
    fn test_duplicate_code_rejected() {
        let env = Env::default();
        env.mock_all_auths();

        let contract_id = env.register(CouponLedger, ());
        let client = CouponLedgerClient::new(&env, &contract_id);
        let admin = Address::generate(&env);

        client.initialize(&admin);
        client.create_campaign(
            &admin,
            &String::from_str(&env, "Dupes"),
            &String::from_str(&env, "percentage"),
            &1000,
            &10,
            &9999999999,
        );

        // Mint first batch
        let mut codes1 = Vec::new(&env);
        codes1.push_back(String::from_str(&env, "UNIQ0001"));
        client.mint_batch(&admin, &1, &codes1);

        // Mint second batch with duplicate code — must fail (Error #8 = DuplicateCode)
        let mut codes2 = Vec::new(&env);
        codes2.push_back(String::from_str(&env, "UNIQ0001"));
        client.mint_batch(&admin, &1, &codes2);
    }
}
