package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
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

// v1 runs BOTH environments against the Stellar testnet contract: "test" is
// free, "live" exercises the credits metering. Mainnet swaps in here later.

// loadOrCreateKEK loads the local key-encryption key (32 bytes) used to seal
// custodial seeds at rest. v1: a 0600 file next to the DB; production: KMS.
func loadOrCreateKEK(dir string) ([]byte, error) {
	p := filepath.Join(dir, "kek.bin")
	if b, err := os.ReadFile(p); err == nil && len(b) == 32 {
		return b, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
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

// fundAccount friendbot-funds a custodial account in the background and marks
// it funded once visible on Horizon.
func (s *server) fundAccount(orgID int64, env, addr string) {
	_, _ = http.Get("https://friendbot.stellar.org/?addr=" + url.QueryEscape(addr))
	for i := 0; i < 60; i++ {
		resp, err := http.Get("https://horizon-testnet.stellar.org/accounts/" + addr)
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
	if err != nil {
		return nil, nil, err
	}
	c, err := sd.Testnet(kp)
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
