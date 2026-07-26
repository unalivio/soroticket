package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testServer(t *testing.T) *server {
	t.Helper()
	db, err := openStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &server{
		db: db, kek: []byte("0123456789abcdef0123456789abcdef"),
		refKey: []byte("abcdef0123456789abcdef0123456789"),
		locks:  map[string]*sync.Mutex{}, loginAttempts: map[string]loginAttempt{},
	}
}

func authedRequest(method, target, body, key, env string) *http.Request {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	ctx := context.WithValue(r.Context(), ctxKey{}, &authCtx{OrgID: 7, Env: env})
	return r.WithContext(ctx)
}

func TestIdempotencyRejectsParameterReuseAndSeparatesEnvironments(t *testing.T) {
	s := testServer(t)
	var calls atomic.Int32
	h := s.idempotent("test", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
	})

	first := httptest.NewRecorder()
	h(first, authedRequest(http.MethodPost, "/v1/x/1", `{"n":1}`, "same-key", "test"))
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d", first.Code)
	}
	var storedFingerprint string
	if err := s.db.QueryRow(`SELECT request_hash FROM idempotency_v2 WHERE key='same-key'`).Scan(&storedFingerprint); err != nil {
		t.Fatal(err)
	}
	material := []byte("POST\n/v1/x/1\n{\"n\":1}")
	plain := sha256.Sum256(material)
	mac := hmac.New(sha256.New, s.refKey)
	_, _ = mac.Write(material)
	if storedFingerprint == hex.EncodeToString(plain[:]) || storedFingerprint != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("idempotency body fingerprint was not a server-keyed HMAC")
	}

	replay := httptest.NewRecorder()
	h(replay, authedRequest(http.MethodPost, "/v1/x/1", `{"n":1}`, "same-key", "test"))
	if replay.Code != http.StatusCreated || replay.Header().Get("Idempotency-Replayed") != "true" || calls.Load() != 1 {
		t.Fatalf("replay status=%d replay=%q calls=%d", replay.Code, replay.Header().Get("Idempotency-Replayed"), calls.Load())
	}

	mismatch := httptest.NewRecorder()
	h(mismatch, authedRequest(http.MethodPost, "/v1/x/1", `{"n":2}`, "same-key", "test"))
	if mismatch.Code != http.StatusConflict || calls.Load() != 1 {
		t.Fatalf("mismatch status=%d calls=%d", mismatch.Code, calls.Load())
	}

	live := httptest.NewRecorder()
	h(live, authedRequest(http.MethodPost, "/v1/x/1", `{"n":1}`, "same-key", "live"))
	if live.Code != http.StatusCreated || calls.Load() != 2 {
		t.Fatalf("live status=%d calls=%d", live.Code, calls.Load())
	}
}

func TestIdempotencyReservesBeforeConcurrentExecution(t *testing.T) {
	s := testServer(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	h := s.idempotent("slow", func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		close(started)
		<-release
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
	})

	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		h(first, authedRequest(http.MethodPost, "/v1/slow", `{}`, "concurrent", "test"))
		close(done)
	}()
	<-started

	second := httptest.NewRecorder()
	h(second, authedRequest(http.MethodPost, "/v1/slow", `{}`, "concurrent", "test"))
	if second.Code != http.StatusConflict || calls.Load() != 1 {
		t.Fatalf("second status=%d calls=%d", second.Code, calls.Load())
	}
	close(release)
	<-done
}

func TestIdempotencyAcknowledgesOnlyAfterDurableResult(t *testing.T) {
	s := testServer(t)
	h := s.idempotent("durability", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
		// Simulate SQLite becoming unavailable after the business handler has
		// completed but before the replay response can be persisted.
		if err := s.db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	w := httptest.NewRecorder()
	h(w, authedRequest(http.MethodPost, "/v1/durable", `{}`, "durable-key", "test"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatal("handler success leaked before the idempotency result was durable")
	}
}

func TestConcurrentChargesNeverOverdraw(t *testing.T) {
	s := testServer(t)
	nowMonth := time.Now().UTC().Format("2006-01")
	if _, err := s.db.Exec(`INSERT INTO credits (org_id, env, balance_mcr, grant_month) VALUES (7,'live',?,?)`, monthlyGrantMcr, nowMonth); err != nil {
		t.Fatal(err)
	}
	a := &authCtx{OrgID: 7, Env: "live"}
	results := make(chan bool, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.charge(httptest.NewRecorder(), a, "concurrent", "test", 15_000_000, "")
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful charges = %d, want 1", successes)
	}
	var balance int64
	if err := s.db.QueryRow(`SELECT balance_mcr FROM credits WHERE org_id=7 AND env='live'`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if balance != monthlyGrantMcr-15_000_000 {
		t.Fatalf("balance=%d", balance)
	}
}

func TestLowCreditAlertOnlyAfterCommittedCharge(t *testing.T) {
	s := testServer(t)
	nowMonth := time.Now().UTC().Format("2006-01")
	if _, err := s.db.Exec(`INSERT INTO credits (org_id,env,balance_mcr,grant_month) VALUES (7,'live',?,?)`, monthlyGrantMcr, nowMonth); err != nil {
		t.Fatal(err)
	}
	a := &authCtx{OrgID: 7, Env: "live"}
	reservation, ok := s.reserveCharge(httptest.NewRecorder(), a, "test", "refund", 23_000_000)
	if !ok {
		t.Fatal("reservation failed")
	}
	reservation.Refund()
	var alerts int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM credit_alerts WHERE org_id=7`).Scan(&alerts); err != nil {
		t.Fatal(err)
	}
	if alerts != 0 {
		t.Fatal("refunded reservation emitted a low-credit alert")
	}
	committed, ok := s.reserveCharge(httptest.NewRecorder(), a, "test", "commit", 23_000_000)
	if !ok {
		t.Fatal("committed reservation failed")
	}
	committed.Commit()
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM credit_alerts WHERE org_id=7`).Scan(&alerts); err != nil {
		t.Fatal(err)
	}
	if alerts != 1 {
		t.Fatalf("committed low balance alerts=%d, want 1", alerts)
	}
}

func TestSignedReceiptsAndMerkleProofs(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	commitment := s.opaqueRef("domain", "customer@example.com")
	if commitment == s.opaqueRef("other", "customer@example.com") || len(commitment) != 64 {
		t.Fatal("opaque references must be domain-separated full HMACs")
	}
	r1, err := s.signReceipt(7, "test", 42, "SAVE", 1, commitment, "", 100, receiptEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := s.signReceipt(7, "test", 42, "SAVE", 2, commitment, "", 101, receiptEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	pub, err := keypair.ParseAddress(r1.Signer)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := base64.StdEncoding.DecodeString(r1.Signature)
	if err != nil || pub.Verify(r1.Payload, sig) != nil {
		t.Fatal("receipt signature did not verify")
	}
	if sha256.Sum256(r1.Payload) != r1.Leaf {
		t.Fatal("leaf must hash the canonical payload")
	}
	leaves := [][32]byte{r1.Leaf, r2.Leaf}
	root := merkleRoot(leaves)
	for i, leaf := range leaves {
		got := leaf
		for _, step := range merkleProof(leaves, i) {
			siblingBytes, _ := hex.DecodeString(step.Hash)
			var sibling [32]byte
			copy(sibling[:], siblingBytes)
			if step.Position == "left" {
				got = sha256.Sum256(append(append([]byte{}, sibling[:]...), got[:]...))
			} else {
				got = sha256.Sum256(append(append([]byte{}, got[:]...), sibling[:]...))
			}
		}
		if got != root {
			t.Fatalf("proof %d did not reach root", i)
		}
	}
}

func TestPublicTallyAuditRejectsPayloadLeafMismatch(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','coupon','Audit','percentage',1000,100,9999999999,100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'SAVE','0',100)`); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.signReceipt(7, "test", 42, "SAVE", 2, "", "", 100, receiptEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	root := hexRoot(merkleRoot([][32]byte{receipt.Leaf}))
	if _, err = s.db.Exec(`INSERT INTO shared_events (id,shared_code_id,count,committed_period,created_at) VALUES (1,1,2,202601,100)`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO event_receipts (event_id,payload,leaf_hash,signature,signer) VALUES (1,?,?,?,?)`,
		receipt.Payload, hexRoot(receipt.Leaf), receipt.Signature, receipt.Signer); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO tallies (shared_code_id,period,count,merkle_root,committed_at) VALUES (1,202601,2,?,100)`, root); err != nil {
		t.Fatal(err)
	}

	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/audit/tallies/42/SAVE/202601", nil)
		r.SetPathValue("chain_id", "42")
		r.SetPathValue("code", "SAVE")
		r.SetPathValue("period", "202601")
		return r
	}
	valid := httptest.NewRecorder()
	s.handleAuditTally(valid, request())
	if valid.Code != http.StatusOK {
		t.Fatalf("valid audit status=%d body=%s", valid.Code, valid.Body.String())
	}
	if _, err = s.db.Exec(`UPDATE event_receipts SET payload='{}' WHERE event_id=1`); err != nil {
		t.Fatal(err)
	}
	corrupt := httptest.NewRecorder()
	s.handleAuditTally(corrupt, request())
	if corrupt.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt audit status=%d body=%s", corrupt.Code, corrupt.Body.String())
	}
}

func TestReceiptV2CarriesDeploymentIdentityAndEvidence(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.signReceipt(7, "test", 42, "SAVE", 1, "cust", "order", 100,
		receiptEvidence{Type: "whatsapp_scan", ContextHash: "abc123", PolicyVersion: "v3"})
	if err != nil {
		t.Fatal(err)
	}
	var p receiptPayload
	if err := json.Unmarshal(receipt.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 || p.Network != cloudNetwork || p.ContractID != currentContractID {
		t.Fatalf("receipt lacks deployment identity: %+v", p)
	}
	if p.EvidenceType != "whatsapp_scan" || p.ContextHash != "abc123" || p.PolicyVersion != "v3" {
		t.Fatalf("integrator evidence was not embedded: %+v", p)
	}
	// empty evidence must not leak empty keys into the canonical payload
	bare, err := s.signReceipt(7, "test", 42, "SAVE", 1, "", "", 100, receiptEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bare.Payload), "evidence_type") || strings.Contains(string(bare.Payload), "context_hash") {
		t.Fatalf("empty evidence fields serialized: %s", bare.Payload)
	}
}

func TestPublicTallyAuditIsScopedByContractDeployment(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	// Two deployments share chain_id 42 + code SAVE + period 202601: the old
	// contract's tally (quarantined, no signed receipts) and the current one.
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+legacyContractID+`','coupon','Old','percentage',1000,100,9999999999,100),
	         (2,7,'test',42,'`+currentContractID+`','coupon','New','percentage',1000,100,9999999999,100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at)
	  VALUES (1,1,'SAVE','0',100),(2,2,'SAVE','0',100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO tallies (shared_code_id,period,count,merkle_root,committed_at)
	  VALUES (1,202601,5,'00',100)`); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.signReceipt(7, "test", 42, "SAVE", 2, "", "", 100, receiptEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	root := hexRoot(merkleRoot([][32]byte{receipt.Leaf}))
	if _, err = s.db.Exec(`INSERT INTO shared_events (id,shared_code_id,count,committed_period,created_at) VALUES (1,2,2,202601,100)`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO event_receipts (event_id,payload,leaf_hash,signature,signer) VALUES (1,?,?,?,?)`,
		receipt.Payload, hexRoot(receipt.Leaf), receipt.Signature, receipt.Signer); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO tallies (shared_code_id,period,count,merkle_root,committed_at) VALUES (2,202601,2,?,100)`, root); err != nil {
		t.Fatal(err)
	}

	request := func(contract string) *http.Request {
		target := "/v1/audit/tallies/42/SAVE/202601"
		if contract != "" {
			target += "?contract=" + contract
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.SetPathValue("chain_id", "42")
		r.SetPathValue("code", "SAVE")
		r.SetPathValue("period", "202601")
		return r
	}

	// default: deterministic resolution to the CURRENT deployment
	w := httptest.NewRecorder()
	s.handleAuditTally(w, request(""))
	if w.Code != http.StatusOK {
		t.Fatalf("default audit status=%d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		ContractID string `json:"contract_id"`
		Network    string `json:"network"`
		MerkleRoot string `json:"merkle_root"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ContractID != currentContractID || out.Network != cloudNetwork || out.MerkleRoot != root {
		t.Fatalf("default audit resolved ambiguously: %+v", out)
	}

	// explicit legacy: its quarantined tally has no signed receipt set → 410
	legacy := httptest.NewRecorder()
	s.handleAuditTally(legacy, request(legacyContractID))
	if legacy.Code != http.StatusGone {
		t.Fatalf("legacy audit status=%d body=%s", legacy.Code, legacy.Body.String())
	}

	// unknown deployment → 404
	unknown := httptest.NewRecorder()
	s.handleAuditTally(unknown, request("CUNKNOWNDEPLOYMENT"))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown contract status=%d body=%s", unknown.Code, unknown.Body.String())
	}
}

func TestPublicTallyAuditIsPaginatedWithReusableMerkleLevels(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','coupon','Paged audit','percentage',1000,100,9999999999,100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'SAVE','0',100)`); err != nil {
		t.Fatal(err)
	}

	leaves := make([][32]byte, 0, 3)
	for i := int64(1); i <= 3; i++ {
		receipt, err := s.signReceipt(7, "test", 42, "SAVE", 1, "", "", 100+i, receiptEvidence{})
		if err != nil {
			t.Fatal(err)
		}
		leaves = append(leaves, receipt.Leaf)
		if _, err = s.db.Exec(`INSERT INTO shared_events (id,shared_code_id,count,committed_period,created_at)
		  VALUES (?,1,1,202601,?)`, i, 100+i); err != nil {
			t.Fatal(err)
		}
		if _, err = s.db.Exec(`INSERT INTO event_receipts (event_id,payload,leaf_hash,signature,signer)
		  VALUES (?,?,?,?,?)`, i, receipt.Payload, hexRoot(receipt.Leaf), receipt.Signature, receipt.Signer); err != nil {
			t.Fatal(err)
		}
	}
	root := merkleRoot(leaves)
	if _, err := s.db.Exec(`INSERT INTO tallies (shared_code_id,period,count,merkle_root,committed_at)
	  VALUES (1,202601,3,?,100)`, hexRoot(root)); err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/v1/audit/tallies/42/SAVE/202601?cursor=1&limit=1", nil)
	r.SetPathValue("chain_id", "42")
	r.SetPathValue("code", "SAVE")
	r.SetPathValue("period", "202601")
	w := httptest.NewRecorder()
	s.handleAuditTally(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		ReceiptTotal int64  `json:"receipt_total"`
		Cursor       int64  `json:"cursor"`
		NextCursor   *int64 `json:"next_cursor"`
		Receipts     []struct {
			Leaf  string       `json:"leaf_hash"`
			Proof []merkleStep `json:"proof"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ReceiptTotal != 3 || response.Cursor != 1 || response.NextCursor == nil || *response.NextCursor != 2 || len(response.Receipts) != 1 {
		t.Fatalf("unexpected page: %+v", response)
	}
	got, err := decodeHash(response.Receipts[0].Leaf)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range response.Receipts[0].Proof {
		sibling, err := decodeHash(step.Hash)
		if err != nil {
			t.Fatal(err)
		}
		if step.Position == "left" {
			got = merkleParent(sibling, got)
		} else {
			got = merkleParent(got, sibling)
		}
	}
	if got != root {
		t.Fatal("paginated inclusion proof did not reach the committed root")
	}
}

func TestPublicAuditHasIndependentIPRateLimit(t *testing.T) {
	s := testServer(t)
	reset := time.Now().Add(time.Minute)
	s.rateWindows = map[string]rateWindow{"audit:203.0.113.20": {Count: 29, Reset: reset}}
	h := s.limitPublicAudit(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	request := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/v1/audit/tallies/42/SAVE/202601", nil)
		r.RemoteAddr = "203.0.113.20:4242"
		return r
	}
	last := httptest.NewRecorder()
	h(last, request())
	if last.Code != http.StatusOK || last.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("last allowed status=%d headers=%v", last.Code, last.Header())
	}
	denied := httptest.NewRecorder()
	h(denied, request())
	if denied.Code != http.StatusTooManyRequests {
		t.Fatalf("denied status=%d body=%s", denied.Code, denied.Body.String())
	}
}

func TestInvalidKEKIsNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kek.bin")
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateKEK(dir); err == nil {
		t.Fatal("expected invalid KEK error")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "broken" {
		t.Fatal("invalid KEK was overwritten")
	}
}

func TestReferenceKeySurvivesKEKRotation(t *testing.T) {
	dir := t.TempDir()
	first, err := loadOrCreateReferenceKey(dir, bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateReferenceKey(dir, bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("reference identity key changed with the encryption KEK")
	}
}

func TestSecurityMigrationInvalidatesRawSessionsAndHashesLegacyOrders(t *testing.T) {
	db, err := openStore(filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO users (id,email,pass_hash,created_at) VALUES (1,'a@example.test','x',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO sessions (token,user_id,expires_at) VALUES ('raw-session-token',1,9999999999)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'coupon','Legacy','percentage',1000,100,9999999999,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'SAVE','0',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO shared_events (id,shared_code_id,count,order_ref,created_at)
	  VALUES (1,1,1,'ORDER-42',100)`); err != nil {
		t.Fatal(err)
	}

	key := bytes.Repeat([]byte{9}, 32)
	if err = runSecurityMigrations(db, key); err != nil {
		t.Fatal(err)
	}
	var sessions int
	if err = db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions=%d err=%v", sessions, err)
	}
	expected := opaqueReference(key, "org:7|env:test|campaign:42|code:SAVE|order", "ORDER-42")
	var migrated string
	var quarantined int64
	if err = db.QueryRow(`SELECT order_ref, committed_period FROM shared_events WHERE id=1`).Scan(&migrated, &quarantined); err != nil {
		t.Fatal(err)
	}
	if migrated != expected || migrated == "ORDER-42" || quarantined != -1 {
		t.Fatalf("migrated order_ref=%q period=%d expected=%q/-1", migrated, quarantined, expected)
	}
	var indexed int
	if err = db.QueryRow(`SELECT COUNT(*) FROM operation_dedup
	  WHERE org_id=7 AND env='test' AND scope='shared:1' AND reference=?`, expected).Scan(&indexed); err != nil || indexed != 1 {
		t.Fatalf("indexed=%d err=%v", indexed, err)
	}
	if err = runSecurityMigrations(db, key); err != nil {
		t.Fatal(err)
	}
	var unchanged string
	if err = db.QueryRow(`SELECT order_ref FROM shared_events WHERE id=1`).Scan(&unchanged); err != nil || unchanged != expected {
		t.Fatalf("migration was not idempotent: %q err=%v", unchanged, err)
	}
}

func TestEncryptedSigningKeyMustMatchStoredAddress(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	declared, _ := keypair.Random()
	actual, _ := keypair.Random()
	sealed, err := s.seal([]byte(actual.Seed()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO org_receipt_keys (org_id,env,public_key,secret_enc) VALUES (7,'test',?,?)`, declared.Address(), sealed); err != nil {
		t.Fatal(err)
	}
	if _, err = s.receiptSigner(7, "test"); err == nil {
		t.Fatal("mismatched encrypted signing key was accepted")
	}
}

func TestWebhookDeliveryIsSignedOverTimestampAndRawBody(t *testing.T) {
	type received struct {
		body      []byte
		timestamp string
		signature string
		delivery  string
		event     string
	}
	got := make(chan received, 1)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		got <- received{
			body: body, timestamp: r.Header.Get("X-Soroticket-Timestamp"),
			signature: r.Header.Get("X-Soroticket-Signature"),
			delivery:  r.Header.Get("X-Soroticket-Delivery"), event: r.Header.Get("X-Soroticket-Event"),
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	})}

	payload := []byte(`{"id":"whd_test","type":"redemption.created"}`)
	secret := []byte("whsec_test_secret")
	status, err := sendWebhook(context.Background(), client, "https://webhook.example.test/soroticket", secret,
		"whd_test", "redemption.created", payload)
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("send status=%d err=%v", status, err)
	}
	receivedRequest := <-got
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(receivedRequest.timestamp + "."))
	_, _ = mac.Write(payload)
	wantSignature := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(receivedRequest.signature), []byte(wantSignature)) {
		t.Fatalf("signature=%q want=%q", receivedRequest.signature, wantSignature)
	}
	if !bytes.Equal(receivedRequest.body, payload) || receivedRequest.delivery != "whd_test" || receivedRequest.event != "redemption.created" {
		t.Fatalf("unexpected webhook request: %+v", receivedRequest)
	}
}

func TestMeteredWebhookNeverClaimsProductionLiveMode(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO webhooks
	  (id,org_id,env,url,secret_enc,secret_prefix,events,created_at)
	  VALUES (1,7,'live','https://8.8.8.8/hook',X'00','whsec_x','["redemption.created"]',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.queueWebhookAt(1, "redemption.created", map[string]any{"ok": true}, "live", 100); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := s.db.QueryRow(`SELECT payload FROM webhook_deliveries WHERE webhook_id=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Mode       string `json:"mode"`
		Network    string `json:"network"`
		Production bool   `json:"production"`
		LiveMode   bool   `json:"livemode"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Mode != "metered" || envelope.Network != "testnet" || envelope.Production || envelope.LiveMode {
		t.Fatalf("misleading webhook environment metadata: %+v", envelope)
	}
}

func TestMeteredAPIKeyUsesHonestPublicPrefix(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	r := authedRequest(http.MethodPost, "/v1/keys", `{"label":"backend"}`, "", "live")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleCreateKey(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Key  string `json:"key"`
		Env  string `json:"env"`
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(response.Key, "sk_metered_") || response.Env != "live" || response.Mode != "metered" {
		t.Fatalf("misleading key response: %+v", response)
	}
}

func TestWebhookURLsRejectSSRFAndInsecureDestinations(t *testing.T) {
	rejected := []string{
		"http://example.com/hook",
		"https://127.0.0.1/hook",
		"https://10.0.0.1/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://user:pass@example.com/hook",
		"https://example.com:8443/hook",
	}
	for _, endpoint := range rejected {
		if _, err := validateWebhookURL(endpoint); err == nil {
			t.Errorf("expected %q to be rejected", endpoint)
		}
	}
	if got, err := validateWebhookURL("HTTPS://8.8.8.8/webhook"); err != nil || got != "https://8.8.8.8/webhook" {
		t.Fatalf("public HTTPS endpoint got=%q err=%v", got, err)
	}
}

func TestAuthenticatedRateLimitReturnsStandardHeaders(t *testing.T) {
	s := testServer(t)
	s.rateWindows = map[string]rateWindow{
		"key:7:write": {Count: 299, Reset: time.Now().Add(time.Minute)},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/redemptions", strings.NewReader(`{}`))
	lastAllowed := httptest.NewRecorder()
	if !s.allowRequest(lastAllowed, req, "key:7") || lastAllowed.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("last allowed request headers=%v", lastAllowed.Header())
	}
	denied := httptest.NewRecorder()
	if s.allowRequest(denied, req, "key:7") || denied.Code != http.StatusTooManyRequests {
		t.Fatalf("denied status=%d headers=%v", denied.Code, denied.Header())
	}
	if denied.Header().Get("Retry-After") == "" || denied.Header().Get("X-RateLimit-Reset") == "" {
		t.Fatal("rate-limit response is missing retry metadata")
	}
}

func TestSignupRateLimitCountsEveryAttemptPerIP(t *testing.T) {
	s := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/auth/signup", nil)
	request.RemoteAddr = "203.0.113.10:4242"
	for i := 0; i < 20; i++ {
		if !s.signupAllowed(request) {
			t.Fatalf("attempt %d was denied early", i+1)
		}
	}
	if s.signupAllowed(request) {
		t.Fatal("21st signup attempt from one IP was allowed")
	}
	other := httptest.NewRequest(http.MethodPost, "/auth/signup", nil)
	other.RemoteAddr = "203.0.113.11:4242"
	if !s.signupAllowed(other) {
		t.Fatal("independent IP was incorrectly denied")
	}
}

func TestSessionBearerIsHashedAtRest(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO users (id,email,pass_hash,created_at) VALUES (9,'a@example.test','x',0)`); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{}`))
	if err := s.createSession(recorder, req, 9); err != nil {
		t.Fatal(err)
	}
	response := recorder.Result()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	var stored string
	if err := s.db.QueryRow(`SELECT token FROM sessions WHERE user_id=9`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == cookies[0].Value || stored != hashKey(cookies[0].Value) {
		t.Fatal("raw session bearer was stored in the database")
	}
	authed := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	authed.AddCookie(cookies[0])
	if uid, ok := s.userFromSession(authed); !ok || uid != 9 {
		t.Fatal("hashed session lookup failed")
	}
}

func TestGeneratedCodeBatchIsBoundedBeforeAllocation(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','ticket','Bounded','fixed_amount',1,100,9999999999,0)`); err != nil {
		t.Fatal(err)
	}
	r := authedRequest(http.MethodPost, "/v1/campaigns/1/codes", `{"generate":{"count":1000000000}}`, "", "test")
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleIssueCodes(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestResolveCodeRoutesSharedThenUniqueWithinOrg(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',41,'`+currentContractID+`','gift','Copas','free_item',1,100,9999999999,0),
	         (2,7,'test',42,'`+currentContractID+`','ticket','Jazz','entry',1,100,9999999999,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'COPA-ROSALES','0',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO codes (id,campaign_id,code,status,created_at) VALUES (1,2,'JAZZ-0042','issued',0)`); err != nil {
		t.Fatal(err)
	}
	resolve := func(code string) (int, map[string]any) {
		r := authedRequest(http.MethodGet, "/v1/codes/resolve?code="+code, "", "", "test")
		w := httptest.NewRecorder()
		s.handleResolveCode(w, r)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		return w.Code, body
	}
	if code, b := resolve("COPA-ROSALES"); code != 200 || b["type"] != "shared" || b["kind"] != "gift" {
		t.Fatalf("shared resolve: %d %v", code, b)
	}
	if code, b := resolve("JAZZ-0042"); code != 200 || b["type"] != "unique" || b["status"] != "issued" {
		t.Fatalf("unique resolve: %d %v", code, b)
	}
	if code, _ := resolve("GHOST"); code != 404 {
		t.Fatalf("unknown code should 404, got %d", code)
	}
}

func TestLegacyContractRowsFailClosedOnChainOperations(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+legacyContractID+`','coupon','Old deployment','percentage',1000,100,9999999999,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at)
	  VALUES (1,1,'SAVE10','0',0)`); err != nil {
		t.Fatal(err)
	}

	// Burn-side write: issuing codes on a legacy-stamped campaign must 409
	// before any chain client is constructed.
	r := authedRequest(http.MethodPost, "/v1/campaigns/1/codes", `{"generate":{"count":1}}`, "", "test")
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleIssueCodes(w, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "previous contract deployment") {
		t.Fatalf("issue: status=%d body=%s", w.Code, w.Body.String())
	}

	// Tally-side write: recording an event resolves through
	// sharedByCampaignCode and must also fail closed.
	r = authedRequest(http.MethodPost, "/v1/shared-codes/1/SAVE10/events", `{}`, "", "test")
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("cid", "1")
	r.SetPathValue("code", "SAVE10")
	w = httptest.NewRecorder()
	s.handleRecordEvents(w, r)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "previous contract deployment") {
		t.Fatalf("event: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestGenericCampaignRejectsPartialLoyaltyAndMissingUniqueSupply(t *testing.T) {
	s := testServer(t)
	for name, body := range map[string]string{
		"loyalty": `{"kind":"loyalty","name":"Incomplete","discount_type":"free_item","valid_until":9999999999}`,
		"supply":  `{"kind":"ticket","name":"No supply","discount_type":"entry","valid_until":9999999999}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := authedRequest(http.MethodPost, "/v1/campaigns", body, "", "test")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.handleCreateCampaign(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestOversizedRedeemerReferenceRejectedBeforeChain(t *testing.T) {
	s := testServer(t)
	body := `{"campaign_id":1,"code":"A","redeemer_ref":"` + strings.Repeat("x", maxReferenceLen+1) + `"}`
	r := authedRequest(http.MethodPost, "/v1/redemptions", body, "", "test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleRedeem(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestArchivedAndExpiredCampaignsRejectNewSharedEvents(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for _, args := range [][]any{
		{1, int64(42), "Archived", now + 3600, 1},
		{2, int64(43), "Expired", now - 1, 0},
		{3, int64(44), "Active", now + 3600, 0},
	} {
		if _, err := s.db.Exec(`INSERT INTO campaigns
		  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,archived,created_at)
		  VALUES (?,7,'test',?,'`+currentContractID+`','coupon',?,'percentage',1000,100,?,?,0)`, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]any{{1, 1, "ARCH"}, {2, 2, "OLD"}, {3, 3, "ACTIVE"}} {
		if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (?,?,?,'0',0)`, args...); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name, cid, code, body string
		want                  int
	}{
		{name: "archived", cid: "1", code: "ARCH", body: `{}`, want: http.StatusConflict},
		{name: "expired", cid: "2", code: "OLD", body: `{}`, want: http.StatusConflict},
		{name: "negative count", cid: "3", code: "ACTIVE", body: `{"count":-1}`, want: http.StatusBadRequest},
		{name: "zero count", cid: "3", code: "ACTIVE", body: `{"count":0}`, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := authedRequest(http.MethodPost, "/v1/campaigns/"+tt.cid+"/shared/"+tt.code+"/events", tt.body, "", "test")
			r.Header.Set("Content-Type", "application/json")
			r.SetPathValue("cid", tt.cid)
			r.SetPathValue("code", tt.code)
			w := httptest.NewRecorder()
			s.handleRecordEvents(w, r)
			if w.Code != tt.want {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tt.want, w.Body.String())
			}
		})
	}

	// Archive is an off-chain operational switch, not deletion: historical
	// detail remains readable for audit and settlement workflows.
	r := authedRequest(http.MethodGet, "/v1/campaigns/1", "", "", "test")
	r.SetPathValue("id", "1")
	w := httptest.NewRecorder()
	s.handleGetCampaign(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("archived campaign detail status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestArchiveCampaignReturnsNotFoundForUnknownID(t *testing.T) {
	s := testServer(t)
	r := authedRequest(http.MethodPost, "/v1/campaigns/999/archive", `{}`, "", "test")
	r.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	s.handleArchiveCampaign(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCampaignSeparatesTotalAndPendingSharedEvents(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','coupon','Counts','percentage',1000,100,9999999999,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'SAVE','0',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_events (shared_code_id,count,committed_period,created_at)
	  VALUES (1,2,202601,0),(1,3,NULL,0)`); err != nil {
		t.Fatal(err)
	}
	campaign, err := s.campaignByID(&authCtx{OrgID: 7, Env: "test"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if campaign.EventsTotal != 5 || campaign.PendingEvents != 3 {
		t.Fatalf("events_total=%d pending_events=%d", campaign.EventsTotal, campaign.PendingEvents)
	}
}

func TestCloudUsesBoundedBatchPeriodsWithinOneISOWeek(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','coupon','Batches','percentage',1000,100,9999999999,0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'SAVE','0',0)`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.July, 9, 12, 0, 0, 0, time.UTC)
	base := isoWeekPeriod(now)
	if base != 202628 || periodLabel(base) != "2026-W28" || !validCloudPeriod(base) {
		t.Fatalf("unexpected base period %d label=%s", base, periodLabel(base))
	}
	if next, err := s.nextCommitPeriod(1, now); err != nil || next != base {
		t.Fatalf("first period=%d err=%v", next, err)
	}
	root := strings.Repeat("0", 64)
	if _, err := s.db.Exec(`INSERT INTO tallies (shared_code_id,period,count,merkle_root,committed_at)
	  VALUES (1,?,1,?,0)`, base, root); err != nil {
		t.Fatal(err)
	}
	batch1, err := s.nextCommitPeriod(1, now)
	if err != nil || batch1 != base*100+1 || periodLabel(batch1) != "2026-W28.01" || !validCloudPeriod(batch1) {
		t.Fatalf("batch1=%d label=%s err=%v", batch1, periodLabel(batch1), err)
	}
	if validCloudPeriod(base * 100) {
		t.Fatal("zero batch suffix was accepted")
	}
	if validCloudPeriod(202153) {
		t.Fatal("week 53 was accepted for an ISO year that has only 52 weeks")
	}
}

func TestTokenUnitFormattingKeepsI128Precision(t *testing.T) {
	value, ok := new(big.Int).SetString("123456789012345678901234", 10)
	if !ok {
		t.Fatal("test integer did not parse")
	}
	if got, want := formatUnits(value), "12345678901234567.8901234"; got != want {
		t.Fatalf("formatUnits=%q want=%q", got, want)
	}
	if got := formatUnits(new(big.Int).Neg(value)); got != "-12345678901234567.8901234" {
		t.Fatalf("negative formatUnits=%q", got)
	}
}

func TestSharedOrderReferenceCannotBeCountedTwice(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','coupon','Dedup','percentage',1000,100,?,0)`, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'SAVE','0',0)`); err != nil {
		t.Fatal(err)
	}

	request := func() *http.Request {
		r := authedRequest(http.MethodPost, "/v1/shared-codes/1/SAVE/events", `{"count":1,"order_ref":"ORDER-42"}`, "", "test")
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("cid", "1")
		r.SetPathValue("code", "SAVE")
		return r
	}
	first := httptest.NewRecorder()
	s.handleRecordEvents(first, request())
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	duplicate := httptest.NewRecorder()
	s.handleRecordEvents(duplicate, request())
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	var events, dedup int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM shared_events`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM operation_dedup`).Scan(&dedup); err != nil {
		t.Fatal(err)
	}
	if events != 1 || dedup != 1 {
		t.Fatalf("events=%d dedup=%d", events, dedup)
	}
}

func TestLoyaltyEventReferenceCannotPunchTwice(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.Exec(`INSERT INTO orgs (id,name,created_at) VALUES (7,'test',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO campaigns
	  (id,org_id,env,chain_id,contract_id,kind,name,discount_type,discount_value,total_supply,valid_until,created_at)
	  VALUES (1,7,'test',42,'`+currentContractID+`','loyalty','Coffee','free_item',1,100,?,0)`, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO shared_codes (id,campaign_id,code,payout_rate,created_at) VALUES (1,1,'PUNCH','0',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO loyalty_programs
	  (id,org_id,env,name,threshold,campaign_id,earn_code,reward_discount_type,reward_discount_value,created_at)
	  VALUES (1,7,'test','Coffee',100,1,'PUNCH','free_item',1,0)`); err != nil {
		t.Fatal(err)
	}

	request := func() *http.Request {
		r := authedRequest(http.MethodPost, "/v1/loyalty/programs/1/punches", `{"customer_ref":"customer-7","event_ref":"sale-99"}`, "", "test")
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("id", "1")
		return r
	}
	first := httptest.NewRecorder()
	s.handlePunch(first, request())
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	duplicate := httptest.NewRecorder()
	s.handlePunch(duplicate, request())
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	zero := authedRequest(http.MethodPost, "/v1/loyalty/programs/1/punches", `{"customer_ref":"customer-8","event_ref":"sale-100","count":0}`, "", "test")
	zero.Header.Set("Content-Type", "application/json")
	zero.SetPathValue("id", "1")
	zeroResponse := httptest.NewRecorder()
	s.handlePunch(zeroResponse, zero)
	if zeroResponse.Code != http.StatusBadRequest {
		t.Fatalf("zero-count status=%d body=%s", zeroResponse.Code, zeroResponse.Body.String())
	}
	var punches int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM punches`).Scan(&punches); err != nil {
		t.Fatal(err)
	}
	if punches != 1 {
		t.Fatalf("punch rows=%d", punches)
	}
}
