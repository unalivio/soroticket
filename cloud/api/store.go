package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// openStore opens (and migrates) the SQLite database. One writer at a time —
// SQLite in WAL mode is plenty for the v1 console.
func openStore(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // modernc sqlite: serialize writes
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  pass_hash TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS orgs (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS org_members (
  org_id INTEGER NOT NULL REFERENCES orgs(id),
  user_id INTEGER NOT NULL REFERENCES users(id),
  PRIMARY KEY (org_id, user_id)
);

-- one custodial Stellar account per org per environment (test/live)
CREATE TABLE IF NOT EXISTS org_accounts (
  org_id INTEGER NOT NULL REFERENCES orgs(id),
  env TEXT NOT NULL CHECK (env IN ('test','live')),
  public_key TEXT NOT NULL,
  secret_enc BLOB NOT NULL,
  funded INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, env)
);

CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS api_keys (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES orgs(id),
  env TEXT NOT NULL CHECK (env IN ('test','live')),
  label TEXT NOT NULL,
  prefix TEXT NOT NULL,          -- display: sk_test_9F…
  hash TEXT NOT NULL UNIQUE,     -- sha256 of the full key
  created_at INTEGER NOT NULL,
  last_used_at INTEGER,
  revoked_at INTEGER
);

CREATE TABLE IF NOT EXISTS campaigns (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES orgs(id),
  env TEXT NOT NULL,
  chain_id INTEGER NOT NULL,     -- on-chain campaign id (u64)
  kind TEXT NOT NULL,            -- coupon|creator|voucher|ticket|loyalty
  name TEXT NOT NULL,
  discount_type TEXT NOT NULL,
  discount_value INTEGER NOT NULL,
  total_supply INTEGER NOT NULL,
  valid_until INTEGER NOT NULL,
  minted INTEGER NOT NULL DEFAULT 0,
  burned INTEGER NOT NULL DEFAULT 0,
  archived INTEGER NOT NULL DEFAULT 0,
  tx_hash TEXT,
  created_at INTEGER NOT NULL
);

-- unique (Burn) codes issued under a campaign
CREATE TABLE IF NOT EXISTS codes (
  id INTEGER PRIMARY KEY,
  campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
  code TEXT NOT NULL,
  token_id INTEGER,
  status TEXT NOT NULL DEFAULT 'issued', -- issued|redeemed
  tx_hash TEXT,
  created_at INTEGER NOT NULL,
  redeemed_at INTEGER,
  UNIQUE (campaign_id, code)
);

CREATE TABLE IF NOT EXISTS redemptions (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
  code TEXT NOT NULL,
  ok INTEGER NOT NULL,           -- 1 success, 0 rejected
  error_code INTEGER,            -- contract error # when rejected
  error_name TEXT,
  redeemer_ref TEXT,             -- opaque commitment hex (never PII)
  token_id INTEGER,
  ledger_seq INTEGER,
  tx_hash TEXT,
  created_at INTEGER NOT NULL
);

-- shared (Tally) codes registered under a campaign
CREATE TABLE IF NOT EXISTS shared_codes (
  id INTEGER PRIMARY KEY,
  campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
  code TEXT NOT NULL,
  attributed_to TEXT,            -- Stellar address or NULL
  payout_token TEXT,
  payout_rate TEXT NOT NULL DEFAULT '0', -- i128 as string
  tx_hash TEXT,
  created_at INTEGER NOT NULL,
  UNIQUE (campaign_id, code)
);

-- off-chain redemption events (the hot path)
CREATE TABLE IF NOT EXISTS shared_events (
  id INTEGER PRIMARY KEY,
  shared_code_id INTEGER NOT NULL REFERENCES shared_codes(id),
  count INTEGER NOT NULL DEFAULT 1,
  customer_ref TEXT,             -- opaque hash or NULL
  order_ref TEXT,
  committed_period INTEGER,      -- NULL until anchored
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS tallies (
  id INTEGER PRIMARY KEY,
  shared_code_id INTEGER NOT NULL REFERENCES shared_codes(id),
  period INTEGER NOT NULL,
  count INTEGER NOT NULL,
  attributed_count INTEGER NOT NULL DEFAULT 0,
  merkle_root TEXT NOT NULL,
  tx_hash TEXT,
  committed_at INTEGER NOT NULL,
  settled INTEGER NOT NULL DEFAULT 0,
  settle_tx TEXT,
  settled_at INTEGER,
  payout_amount TEXT,            -- i128 string, set on settle
  UNIQUE (shared_code_id, period)
);

CREATE TABLE IF NOT EXISTS loyalty_programs (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  name TEXT NOT NULL,
  threshold INTEGER NOT NULL,
  campaign_id INTEGER NOT NULL REFERENCES campaigns(id), -- dual-profile campaign
  earn_code TEXT NOT NULL,       -- shared code anchoring punches
  reward_discount_type TEXT NOT NULL,
  reward_discount_value INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS punches (
  id INTEGER PRIMARY KEY,
  program_id INTEGER NOT NULL REFERENCES loyalty_programs(id),
  customer_ref TEXT NOT NULL,    -- opaque commitment (hashed server-side)
  count INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rewards (
  id INTEGER PRIMARY KEY,
  program_id INTEGER NOT NULL REFERENCES loyalty_programs(id),
  customer_ref TEXT NOT NULL,
  code TEXT NOT NULL,
  issued_at INTEGER NOT NULL,
  redeemed INTEGER NOT NULL DEFAULT 0
);

-- credits ledger in MILLIcredits (1 cr = 1000 mcr) so 0.2 cr prices stay exact
CREATE TABLE IF NOT EXISTS credits (
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  balance_mcr INTEGER NOT NULL DEFAULT 0,
  grant_month TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (org_id, env)
);

CREATE TABLE IF NOT EXISTS credit_ledger (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  ts INTEGER NOT NULL,
  operation TEXT NOT NULL,
  detail TEXT,
  delta_mcr INTEGER NOT NULL,
  balance_mcr INTEGER NOT NULL,
  tx_hash TEXT
);

CREATE TABLE IF NOT EXISTS activity (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  ts INTEGER NOT NULL,
  kind TEXT NOT NULL,            -- redemption|rejected|issue|campaign|tally|settle|reward|event|key|program
  code TEXT,
  message TEXT NOT NULL,
  tx_hash TEXT,
  error_code INTEGER,
  campaign_id INTEGER
);
CREATE INDEX IF NOT EXISTS idx_activity_org ON activity(org_id, env, ts DESC);

CREATE TABLE IF NOT EXISTS idempotency (
  key TEXT NOT NULL,
  org_id INTEGER NOT NULL,
  endpoint TEXT NOT NULL,
  status INTEGER NOT NULL,
  body BLOB NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (key, org_id, endpoint)
);
`
