package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"

	sd "github.com/sorodeal/sorodeal-go"
)

// v1 runs both environments against the Stellar testnet contract: "test" is
// free and "live" is only a metered preview. Mainnet is intentionally disabled.

// currentContractID pins the reviewed v0.2.0 testnet deployment
// (2026-07-12; see deployments/testnet-v0.2.0.json).
const currentContractID = "CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF"

// legacyContractID is the deprecated v0.1 deployment. Rows stamped with it (or
// with an empty/unknown stamp) fail closed on chain operations: v0.2 restarts
// campaign numbering, so a legacy chain_id resolved on the current contract
// would address a different campaign.
const legacyContractID = "CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX"

// errLegacyContract marks a row created on a superseded deployment.
var errLegacyContract = errors.New("resource belongs to a superseded contract deployment")

// requireCurrentContract fails closed (409) for rows not stamped with the
// current deployment.
func (s *server) requireCurrentContract(w http.ResponseWriter, rowContractID string) bool {
	if rowContractID != currentContractID {
		writeLegacyContractProblem(w)
		return false
	}
	return true
}

func writeLegacyContractProblem(w http.ResponseWriter) {
	writeProblem(w, http.StatusConflict,
		"this resource was created on a previous contract deployment and is read-only in this preview; create a new campaign on the current deployment")
}

// loadOrCreateKEK loads the local key-encryption key (32 bytes) used to seal
// custodial seeds at rest. v1: a 0600 file next to the DB; production: KMS.
func loadOrCreateKEK(dir string) ([]byte, error) {
	p := filepath.Join(dir, "kek.bin")
	if b, err := os.ReadFile(p); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("KEK %s has invalid length %d; refusing to replace it", p, len(b))
		}
		return b, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read KEK: %w", err)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := writeSecretFile(p, b); err != nil {
		return nil, err
	}
	return b, nil
}

// loadOrCreateReferenceKey persists a dedicated HMAC key for customer/order
// commitments. The first value is deterministically derived from the existing
// KEK, then persisted separately so future KEK rotation does not change new
// opaque identifiers. Legacy truncated hashes cannot be reversed; see the
// explicit data-migration caveat in docs/CLOUD.md.
func loadOrCreateReferenceKey(dir string, kek []byte) ([]byte, error) {
	p := filepath.Join(dir, "reference-key.bin")
	if b, err := os.ReadFile(p); err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("reference key %s has invalid length %d", p, len(b))
		}
		return b, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read reference key: %w", err)
	}
	mac := hmac.New(sha256.New, kek)
	_, _ = mac.Write([]byte("sorodeal/reference-key/v1"))
	b := mac.Sum(nil)
	if err := writeSecretFile(p, b); err != nil {
		return nil, err
	}
	return b, nil
}

func writeSecretFile(path string, value []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create secret file: %w", err)
	}
	if _, err = f.Write(value); err != nil {
		f.Close()
		return fmt.Errorf("write secret file: %w", err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync secret file: %w", err)
	}
	if err = f.Close(); err != nil {
		return err
	}
	return nil
}

func (s *server) seal(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plain, nil)...), nil
}

func (s *server) unseal(box []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.kek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(box) < gcm.NonceSize() {
		return nil, fmt.Errorf("sealed box too short")
	}
	return gcm.Open(nil, box[:gcm.NonceSize()], box[gcm.NonceSize():], nil)
}

func (s *server) newCustodialAccount() (publicKey string, sealedSeed []byte, err error) {
	kp, err := keypair.Random()
	if err != nil {
		return "", nil, err
	}
	sealed, err := s.seal([]byte(kp.Seed()))
	if err != nil {
		return "", nil, err
	}
	return kp.Address(), sealed, nil
}

func (s *server) ensureReceiptSigner(orgID int64, env string) (string, error) {
	mu := s.lockFor(orgID, "receipt/"+env)
	mu.Lock()
	defer mu.Unlock()
	var publicKey string
	if err := s.db.QueryRow(`SELECT public_key FROM org_receipt_keys WHERE org_id = ? AND env = ?`, orgID, env).Scan(&publicKey); err == nil {
		return publicKey, nil
	} else if err != sql.ErrNoRows {
		return "", err
	}
	publicKey, sealed, err := s.newCustodialAccount()
	if err != nil {
		return "", err
	}
	if _, err = s.db.Exec(`INSERT INTO org_receipt_keys (org_id, env, public_key, secret_enc) VALUES (?,?,?,?)`,
		orgID, env, publicKey, sealed); err != nil {
		return "", err
	}
	return publicKey, nil
}

func (s *server) receiptSigner(orgID int64, env string) (*keypair.Full, error) {
	if _, err := s.ensureReceiptSigner(orgID, env); err != nil {
		return nil, err
	}
	var publicKey string
	var sealed []byte
	if err := s.db.QueryRow(`SELECT public_key, secret_enc FROM org_receipt_keys WHERE org_id = ? AND env = ?`, orgID, env).Scan(&publicKey, &sealed); err != nil {
		return nil, err
	}
	seed, err := s.unseal(sealed)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range seed {
			seed[i] = 0
		}
	}()
	signer, err := keypair.ParseFull(string(seed))
	if err != nil {
		return nil, err
	}
	if signer.Address() != publicKey {
		return nil, errors.New("receipt signing key does not match its stored public key")
	}
	return signer, nil
}

// fundAccount friendbot-funds a custodial account in the background and marks
// it funded once visible on Horizon.
func (s *server) fundAccount(orgID int64, env, addr string) {
	client := &http.Client{Timeout: 10 * time.Second}
	if response, err := client.Get("https://friendbot.stellar.org/?addr=" + url.QueryEscape(addr)); err == nil {
		response.Body.Close()
	}
	for i := 0; i < 60; i++ {
		resp, err := client.Get("https://horizon-testnet.stellar.org/accounts/" + addr)
		if err == nil {
			code := resp.StatusCode
			resp.Body.Close()
			if code == 200 {
				_, _ = s.db.Exec(`UPDATE org_accounts SET funded = 1 WHERE org_id = ? AND env = ?`, orgID, env)
				return
			}
		}
		time.Sleep(time.Second)
	}
}

// clientFor builds a sorodeal client signing as the org's custodial account.
// Writes per org+env are serialized (sequence numbers) via a per-account mutex.
func (s *server) clientFor(orgID int64, env string) (*sd.Client, func(), error) {
	var pk string
	var sealed []byte
	var funded int
	err := s.db.QueryRow(`SELECT public_key, secret_enc, funded FROM org_accounts WHERE org_id = ? AND env = ?`,
		orgID, env).Scan(&pk, &sealed, &funded)
	if err != nil {
		return nil, nil, fmt.Errorf("org account: %w", err)
	}
	if funded == 0 {
		return nil, nil, errAccountFunding
	}
	seed, err := s.unseal(sealed)
	if err != nil {
		return nil, nil, fmt.Errorf("unseal: %w", err)
	}
	kp, err := keypair.ParseFull(string(seed))
	for i := range seed {
		seed[i] = 0
	}
	if err != nil {
		return nil, nil, err
	}
	if kp.Address() != pk {
		return nil, nil, errors.New("custodial signing key does not match its stored public key")
	}
	// Pin the reviewed deployment explicitly. Cloud must never inherit a future
	// SDK default and silently switch contract semantics; the move from v0.1 to
	// v0.2.0 was a deliberate migration (allowance-based settlement gate,
	// contract-stamped rows, fail-closed legacy guard).
	c, err := sd.New(sd.Config{
		ContractID:        currentContractID,
		RPCURL:            sd.TestnetRPC,
		NetworkPassphrase: sd.TestnetPassphrase,
		Signer:            kp,
	})
	if err != nil {
		return nil, nil, err
	}
	mu := s.lockFor(orgID, env)
	mu.Lock()
	release := func() { mu.Unlock(); c.Close() }
	return c, release, nil
}

var errAccountFunding = fmt.Errorf("your Stellar account is still being set up — try again in a few seconds")

func (s *server) lockFor(orgID int64, env string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	k := fmt.Sprintf("%d/%s", orgID, env)
	if s.locks[k] == nil {
		s.locks[k] = &sync.Mutex{}
	}
	return s.locks[k]
}
