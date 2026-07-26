package main

import (
	"database/sql"
	"fmt"
	"time"

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
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);

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

-- Dedicated off-chain receipt signer. It is intentionally separate from the
-- custodial account that holds funds, so receipt traffic never exposes the
-- settlement key to the hot path.
CREATE TABLE IF NOT EXISTS org_receipt_keys (
  org_id INTEGER NOT NULL REFERENCES orgs(id),
  env TEXT NOT NULL CHECK (env IN ('test','live')),
  public_key TEXT NOT NULL,
  secret_enc BLOB NOT NULL,
  PRIMARY KEY (org_id, env)
);

CREATE TABLE IF NOT EXISTS sessions (
  token TEXT PRIMARY KEY,       -- SHA-256 of cookie token; raw bearer never stored
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
  contract_id TEXT NOT NULL DEFAULT '', -- deployment stamp; ''/unknown fails closed on chain ops
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
-- Operational columns (customer_tail, coordinates, evidence_type) exist so the
-- merchant portal can answer "who scanned and from where" — the whole point of
-- proof-of-delivery. They are deliberately NOT in the published signed receipt,
-- which keeps carrying commitments only: the merchant sees its own scans, the
-- public audit trail never exposes a customer's phone or position.
CREATE TABLE IF NOT EXISTS shared_events (
  id INTEGER PRIMARY KEY,
  shared_code_id INTEGER NOT NULL REFERENCES shared_codes(id),
  count INTEGER NOT NULL DEFAULT 1,
  customer_ref TEXT,             -- opaque hash or NULL
  order_ref TEXT,
  committed_period INTEGER,      -- NULL until anchored
  created_at INTEGER NOT NULL,
  customer_tail TEXT,            -- last 4 digits only; the full number is never stored
  lat REAL,                      -- where the scan happened, when shared
  lon REAL,
  accuracy_m REAL,               -- GPS accuracy the device reported, when known
  evidence_type TEXT             -- integrator-declared (e.g. whatsapp_scan_geo)
);

CREATE TABLE IF NOT EXISTS event_receipts (
  event_id INTEGER PRIMARY KEY REFERENCES shared_events(id),
  payload BLOB NOT NULL,
  leaf_hash TEXT NOT NULL,
  signature TEXT NOT NULL,
  signer TEXT NOT NULL
);

-- Optional external event/order references are HMACed before reaching this
-- table. They prevent a caller from counting the same business event again
-- under a different HTTP idempotency key.
CREATE TABLE IF NOT EXISTS operation_dedup (
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  scope TEXT NOT NULL,
  reference TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (org_id, env, scope, reference)
);
INSERT OR IGNORE INTO operation_dedup (org_id, env, scope, reference, created_at)
SELECT c.org_id, c.env, 'shared:' || e.shared_code_id, e.order_ref, MIN(e.created_at)
FROM shared_events e
JOIN shared_codes sc ON sc.id = e.shared_code_id
JOIN campaigns c ON c.id = sc.campaign_id
WHERE e.order_ref IS NOT NULL AND e.order_ref <> ''
GROUP BY c.org_id, c.env, e.shared_code_id, e.order_ref;

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

CREATE TABLE IF NOT EXISTS credit_alerts (
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  month TEXT NOT NULL,
  alert_type TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (org_id, env, month, alert_type)
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

-- v2 reserves a key before executing, preventing concurrent duplicates. The
-- request HMAC also rejects accidental key reuse across payloads/paths/envs
-- without exposing a plain low-entropy body fingerprint at rest.
CREATE TABLE IF NOT EXISTS idempotency_v2 (
  key TEXT NOT NULL,
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  status INTEGER NOT NULL DEFAULT 0, -- 0 = in progress
  content_type TEXT NOT NULL DEFAULT 'application/json',
  body BLOB,
  created_at INTEGER NOT NULL,
  completed_at INTEGER,
  PRIMARY KEY (key, org_id, env, endpoint)
);

-- Opaque scan tokens: what a printed QR actually carries. The token reveals
-- nothing about the campaign or the code (a customer reading the QR learns
-- nothing), it is short enough to keep printed QR density low, and it can be
-- revoked without reprinting anything else.
CREATE TABLE IF NOT EXISTS scan_tokens (
  token TEXT PRIMARY KEY,
  org_id INTEGER NOT NULL,
  env TEXT NOT NULL,
  campaign_id INTEGER NOT NULL REFERENCES campaigns(id),
  code TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  revoked_at INTEGER,
  UNIQUE (campaign_id, code, revoked_at)
);
CREATE INDEX IF NOT EXISTS idx_scan_tokens_code ON scan_tokens(campaign_id, code);

CREATE TABLE IF NOT EXISTS webhooks (
  id INTEGER PRIMARY KEY,
  org_id INTEGER NOT NULL REFERENCES orgs(id),
  env TEXT NOT NULL CHECK (env IN ('test','live')),
  url TEXT NOT NULL,
  secret_enc BLOB NOT NULL,
  secret_prefix TEXT NOT NULL,
  events TEXT NOT NULL,          -- JSON array of subscribed event names
  active INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL,
  disabled_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_webhooks_org ON webhooks(org_id, env, active);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
  id INTEGER PRIMARY KEY,
  webhook_id INTEGER NOT NULL REFERENCES webhooks(id),
  delivery_id TEXT NOT NULL UNIQUE,
  event_type TEXT NOT NULL,
  payload BLOB NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending', -- pending|retrying|delivered|failed
  attempts INTEGER NOT NULL DEFAULT 0,
  response_status INTEGER,
  last_error TEXT,
  next_attempt_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  delivered_at INTEGER
);
CREATE INDEX IF NOT EXISTS idx_webhook_delivery_due
  ON webhook_deliveries(status, next_attempt_at);
`

// runSecurityMigrations performs one-way transformations that CREATE TABLE
// cannot express safely. Version 1 invalidates pre-hashing sessions and HMACs
// legacy plaintext order references. It deliberately does not fabricate signed
// receipts for historical events.
func runSecurityMigrations(db *sql.DB, refKey []byte) error {
	if len(refKey) < 32 {
		return fmt.Errorf("security migration requires a 32-byte reference key")
	}
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=1`).Scan(&applied); err != nil {
		return fmt.Errorf("read security migration state: %w", err)
	}
	if applied == 1 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin security migration: %w", err)
	}
	defer tx.Rollback()

	// Old rows stored the raw 32-byte session token. There is no reliable marker
	// distinguishing it from a SHA-256 hex digest, so force a one-time logout.
	if _, err = tx.Exec(`DELETE FROM sessions`); err != nil {
		return fmt.Errorf("invalidate legacy sessions: %w", err)
	}

	type legacyOrder struct {
		eventID, sharedID, orgID, chainID int64
		env, code, reference              string
	}
	rows, err := tx.Query(`SELECT e.id, e.shared_code_id, c.org_id, c.env, c.chain_id, sc.code, e.order_ref
	  FROM shared_events e
	  JOIN shared_codes sc ON sc.id=e.shared_code_id
	  JOIN campaigns c ON c.id=sc.campaign_id
	  LEFT JOIN event_receipts er ON er.event_id=e.id
	  WHERE er.event_id IS NULL AND e.order_ref IS NOT NULL AND e.order_ref<>''`)
	if err != nil {
		return fmt.Errorf("load legacy order references: %w", err)
	}
	legacy := []legacyOrder{}
	for rows.Next() {
		var item legacyOrder
		if err = rows.Scan(&item.eventID, &item.sharedID, &item.orgID, &item.env, &item.chainID, &item.code, &item.reference); err != nil {
			rows.Close()
			return fmt.Errorf("decode legacy order reference: %w", err)
		}
		legacy = append(legacy, item)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return fmt.Errorf("read legacy order references: %w", rowsErr)
	}
	for _, item := range legacy {
		domain := fmt.Sprintf("org:%d|env:%s|campaign:%d|code:%s|order", item.orgID, item.env, item.chainID, item.code)
		hashed := opaqueReference(refKey, domain, item.reference)
		if _, err = tx.Exec(`UPDATE shared_events SET order_ref=? WHERE id=?`, hashed, item.eventID); err != nil {
			return fmt.Errorf("migrate legacy order reference: %w", err)
		}
		scope := fmt.Sprintf("shared:%d", item.sharedID)
		if _, err = tx.Exec(`DELETE FROM operation_dedup
		  WHERE org_id=? AND env=? AND scope=? AND reference=?`, item.orgID, item.env, scope, item.reference); err != nil {
			return fmt.Errorf("remove plaintext dedup reference: %w", err)
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO operation_dedup
		  (org_id,env,scope,reference,created_at) VALUES (?,?,?,?,?)`,
			item.orgID, item.env, scope, hashed, time.Now().Unix()); err != nil {
			return fmt.Errorf("index migrated order reference: %w", err)
		}
	}
	// Historical events predate signed receipts. They are preserved for export
	// and totals but quarantined from the new commit queue; signing them now
	// would manufacture an attestation that did not exist when they occurred.
	if _, err = tx.Exec(`UPDATE shared_events SET committed_period=-1
	  WHERE committed_period IS NULL AND NOT EXISTS
	  (SELECT 1 FROM event_receipts er WHERE er.event_id=shared_events.id)`); err != nil {
		return fmt.Errorf("quarantine legacy unsigned events: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO schema_migrations (version,applied_at) VALUES (1,?)`, time.Now().Unix()); err != nil {
		return fmt.Errorf("record security migration: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit security migration: %w", err)
	}
	return nil
}

// runScanDetailMigration (version 3) adds the operational scan columns to
// existing databases. Rows recorded before it keep NULLs: the portal shows
// them as unknown rather than inventing a location or a phone.
func runScanDetailMigration(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=3`).Scan(&applied); err != nil {
		return fmt.Errorf("read scan-detail migration state: %w", err)
	}
	if applied == 1 {
		return nil
	}
	columns := map[string]string{
		"customer_tail": "TEXT", "lat": "REAL", "lon": "REAL",
		"accuracy_m": "REAL", "evidence_type": "TEXT",
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin scan-detail migration: %w", err)
	}
	defer tx.Rollback()
	for name, kind := range columns {
		var present int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('shared_events') WHERE name=?`, name).Scan(&present); err != nil {
			return fmt.Errorf("inspect shared_events.%s: %w", name, err)
		}
		if present == 0 {
			if _, err := tx.Exec(`ALTER TABLE shared_events ADD COLUMN ` + name + ` ` + kind); err != nil {
				return fmt.Errorf("add shared_events.%s: %w", name, err)
			}
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version,applied_at) VALUES (3,?)`, time.Now().Unix()); err != nil {
		return fmt.Errorf("record scan-detail migration: %w", err)
	}
	return tx.Commit()
}

// runContractStampMigration (version 2) adds the per-campaign contract
// deployment stamp. Rows that existed before the stamp were created against
// the deprecated v0.1 deployment, so they are stamped as such and chain
// operations on them fail closed — v0.2 restarts campaign numbering, so a
// legacy chain_id resolved on the current contract would address a different
// campaign.
func runContractStampMigration(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=2`).Scan(&applied); err != nil {
		return fmt.Errorf("read contract migration state: %w", err)
	}
	if applied == 1 {
		return nil
	}
	var hasColumn int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('campaigns') WHERE name='contract_id'`).Scan(&hasColumn); err != nil {
		return fmt.Errorf("inspect campaigns schema: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin contract migration: %w", err)
	}
	defer tx.Rollback()
	if hasColumn == 0 {
		// Pre-stamp table: every existing row was created against v0.1.
		if _, err := tx.Exec(`ALTER TABLE campaigns ADD COLUMN contract_id TEXT NOT NULL DEFAULT '` + legacyContractID + `'`); err != nil {
			return fmt.Errorf("add campaigns.contract_id: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version,applied_at) VALUES (2,?)`, time.Now().Unix()); err != nil {
		return fmt.Errorf("record contract migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit contract migration: %w", err)
	}
	return nil
}
