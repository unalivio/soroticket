package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type authCtx struct {
	UserID int64
	OrgID  int64
	Env    string // test|live
}

type ctxKey struct{}

var dummyPasswordHash = func() []byte {
	hash, _ := bcrypt.GenerateFromPassword([]byte("not-a-real-sorodeal-password"), bcrypt.DefaultCost)
	return hash
}()

func authFrom(r *http.Request) *authCtx {
	v, _ := r.Context().Value(ctxKey{}).(*authCtx)
	return v
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

// ── session auth (console) ──────────────────────────────────────────

func (s *server) createSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	tok, err := randHex(32)
	if err != nil {
		return err
	}
	exp := time.Now().Add(8 * time.Hour).Unix()
	if _, err = s.db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?,?,?)`, hashKey(tok), userID, exp); err != nil {
		return err
	}
	// Bound stolen-session exposure and per-user state.
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE user_id=? AND token NOT IN
	  (SELECT token FROM sessions WHERE user_id=? ORDER BY expires_at DESC LIMIT 5)`, userID, userID)
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name: "sd_session", Value: tok, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: 8 * 3600,
	})
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
	return nil
}

func (s *server) userFromSession(r *http.Request) (int64, bool) {
	c, err := r.Cookie("sd_session")
	if err != nil {
		return 0, false
	}
	var uid, exp int64
	err = s.db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token = ?`, hashKey(c.Value)).Scan(&uid, &exp)
	if err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	return uid, true
}

func (s *server) orgOfUser(uid int64) (int64, bool) {
	var oid int64
	err := s.db.QueryRow(`SELECT org_id FROM org_members WHERE user_id = ? LIMIT 1`, uid).Scan(&oid)
	return oid, err == nil
}

// requireAuth resolves the caller either from an API key (Authorization: Bearer
// sk_test_… / sk_metered_… — env comes from the key) or from the console session
// cookie (env from the X-Env header, default test).
func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// API key path
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer sk_") {
			key := strings.TrimPrefix(h, "Bearer ")
			var id, orgID int64
			var env string
			var revoked sql.NullInt64
			err := s.db.QueryRow(`SELECT id, org_id, env, revoked_at FROM api_keys WHERE hash = ?`, hashKey(key)).
				Scan(&id, &orgID, &env, &revoked)
			if err != nil || revoked.Valid {
				writeProblem(w, http.StatusUnauthorized, "invalid or revoked API key")
				return
			}
			if !s.allowRequest(w, r, "key:"+strconv.FormatInt(id, 10)) {
				return
			}
			_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now().Unix(), id)
			ctx := context.WithValue(r.Context(), ctxKey{}, &authCtx{OrgID: orgID, Env: env})
			next(w, r.WithContext(ctx))
			return
		}
		// session path
		if !sameOriginMutation(r) {
			writeProblem(w, http.StatusForbidden, "cross-site session request rejected")
			return
		}
		uid, ok := s.userFromSession(r)
		if !ok {
			writeProblem(w, http.StatusUnauthorized, "sign in required")
			return
		}
		oid, ok := s.orgOfUser(uid)
		if !ok {
			writeProblem(w, http.StatusPreconditionRequired, "create an organization first")
			return
		}
		if !s.allowRequest(w, r, "session:"+strconv.FormatInt(uid, 10)) {
			return
		}
		env := r.Header.Get("X-Env")
		if env != "live" {
			env = "test"
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, &authCtx{UserID: uid, OrgID: oid, Env: env})
		next(w, r.WithContext(ctx))
	}
}

type rateWindow struct {
	Count int
	Reset time.Time
}

func (s *server) allowRequest(w http.ResponseWriter, r *http.Request, principal string) bool {
	limit := 300
	class := "write"
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		limit = 1200
		class = "read"
	}
	return s.allowRateWindow(w, principal+":"+class, limit)
}

func (s *server) limitPublicAudit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.allowRateWindow(w, "audit:"+remoteIP(r), 30) {
			return
		}
		next(w, r)
	}
}

func (s *server) allowRateWindow(w http.ResponseWriter, windowKey string, limit int) bool {
	now := time.Now()
	s.rateMu.Lock()
	if s.rateWindows == nil {
		s.rateWindows = map[string]rateWindow{}
	}
	window := s.rateWindows[windowKey]
	if window.Reset.IsZero() || !now.Before(window.Reset) {
		window = rateWindow{Reset: now.Add(time.Minute)}
	}
	allowed := window.Count < limit
	if allowed {
		window.Count++
	}
	s.rateWindows[windowKey] = window
	if len(s.rateWindows) > 10_000 {
		for key, candidate := range s.rateWindows {
			if !now.Before(candidate.Reset) {
				delete(s.rateWindows, key)
			}
		}
	}
	remaining := limit - window.Count
	resetUnix := window.Reset.Unix()
	s.rateMu.Unlock()

	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(remaining, 0)))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetUnix, 10))
	if !allowed {
		retry := max(int(time.Until(window.Reset).Seconds()), 1)
		w.Header().Set("Retry-After", strconv.Itoa(retry))
		writeProblem(w, http.StatusTooManyRequests, "rate limit exceeded")
		return false
	}
	return true
}

func sameOriginMutation(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// SameSite=Lax already withholds this cookie from cross-site POSTs;
		// non-browser clients may legitimately omit Origin.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || !strings.EqualFold(u.Host, r.Host) {
		return false
	}
	expectedScheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		expectedScheme = "https"
	}
	return strings.EqualFold(u.Scheme, expectedScheme)
}

// ── handlers ────────────────────────────────────────────────────────

func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !sameOriginMutation(r) {
		writeProblem(w, http.StatusForbidden, "cross-site request rejected")
		return
	}
	if !s.signupAllowed(r) {
		w.Header().Set("Retry-After", "3600")
		writeProblem(w, http.StatusTooManyRequests, "too many signup attempts; try again later")
		return
	}
	var in struct{ Email, Password string }
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" || len(in.Email) > 254 || !strings.Contains(in.Email, "@") || len(in.Password) < 15 || len(in.Password) > 72 {
		writeProblem(w, 400, "valid email and a password of 15-72 bytes are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternal(w, err, "hash signup password")
		return
	}
	res, err := s.db.Exec(`INSERT INTO users (email, pass_hash, created_at) VALUES (?,?,?)`,
		in.Email, string(hash), time.Now().Unix())
	if err != nil {
		writeProblem(w, http.StatusConflict, "an account with that email already exists")
		return
	}
	uid, _ := res.LastInsertId()
	if err := s.createSession(w, r, uid); err != nil {
		writeProblem(w, 500, "could not create session")
		return
	}
	writeJSON(w, 201, map[string]any{"user": map[string]any{"id": uid, "email": in.Email}})
}

// signupAllowed bounds the expensive bcrypt + database path independently of
// authenticated API limits. It counts every attempt so rotating email values
// cannot bypass the limit.
func (s *server) signupAllowed(r *http.Request) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	const limit = 20
	now := time.Now()
	key := "signup:ip:" + remoteIP(r)
	a := s.loginAttempts[key]
	if now.After(a.Reset) {
		a = loginAttempt{Reset: now.Add(time.Hour)}
	}
	if a.Count >= limit {
		return false
	}
	a.Count++
	s.loginAttempts[key] = a
	return true
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOriginMutation(r) {
		writeProblem(w, http.StatusForbidden, "cross-site request rejected")
		return
	}
	var in struct{ Email, Password string }
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if !s.loginAllowed(r, email) {
		w.Header().Set("Retry-After", "900")
		writeProblem(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}
	var uid int64
	var hash string
	err := s.db.QueryRow(`SELECT id, pass_hash FROM users WHERE email = ?`,
		email).Scan(&uid, &hash)
	found := err == nil
	if err != nil && err != sql.ErrNoRows {
		log.Printf("login lookup failed: %v", err)
	}
	if !found {
		hash = string(dummyPasswordHash)
	}
	valid := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil
	if !found || !valid {
		s.loginFailed(r, email)
		log.Printf("login_failure account=%s ip=%s", hashKey(email)[:12], remoteIP(r))
		writeProblem(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	s.loginSucceeded(email)
	log.Printf("login_success user=%d ip=%s", uid, remoteIP(r))
	if err := s.createSession(w, r, uid); err != nil {
		writeProblem(w, 500, "could not create session")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type loginAttempt struct {
	Count int
	Reset time.Time
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func (s *server) loginAllowed(r *http.Request, email string) bool {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	checks := []struct {
		key   string
		limit int
	}{{"ip:" + remoteIP(r), 20}, {"account:" + hashKey(email), 5}}
	for _, check := range checks {
		a := s.loginAttempts[check.key]
		if now.After(a.Reset) {
			delete(s.loginAttempts, check.key)
			continue
		}
		if a.Count >= check.limit {
			return false
		}
	}
	return true
}

func (s *server) loginFailed(r *http.Request, email string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	now := time.Now()
	for _, key := range []string{"ip:" + remoteIP(r), "account:" + hashKey(email)} {
		a := s.loginAttempts[key]
		if now.After(a.Reset) {
			a = loginAttempt{Reset: now.Add(15 * time.Minute)}
		}
		a.Count++
		s.loginAttempts[key] = a
	}
}

func (s *server) loginSucceeded(email string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	delete(s.loginAttempts, "account:"+hashKey(email))
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !sameOriginMutation(r) {
		writeProblem(w, http.StatusForbidden, "cross-site request rejected")
		return
	}
	uid, authenticated := s.userFromSession(r)
	if c, err := r.Cookie("sd_session"); err == nil {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, hashKey(c.Value))
	}
	if authenticated {
		log.Printf("logout user=%d ip=%s", uid, remoteIP(r))
	}
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name: "sd_session", Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.userFromSession(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var email string
	if err := s.db.QueryRow(`SELECT email FROM users WHERE id = ?`, uid).Scan(&email); err != nil {
		writeInternal(w, err, "load session user")
		return
	}
	out := map[string]any{"user": map[string]any{"id": uid, "email": email}}
	if oid, ok := s.orgOfUser(uid); ok {
		var name string
		if err := s.db.QueryRow(`SELECT name FROM orgs WHERE id = ?`, oid).Scan(&name); err != nil {
			writeInternal(w, err, "load organization")
			return
		}
		accounts := map[string]any{}
		rows, err := s.db.Query(`SELECT env, public_key, funded FROM org_accounts WHERE org_id = ?`, oid)
		if err != nil {
			writeInternal(w, err, "load organization accounts")
			return
		}
		for rows.Next() {
			var env, pk string
			var funded int
			if err := rows.Scan(&env, &pk, &funded); err != nil {
				rows.Close()
				writeInternal(w, err, "decode organization account")
				return
			}
			accounts[env] = map[string]any{"public_key": pk, "funded": funded == 1}
		}
		rowsErr := rows.Err()
		rows.Close()
		if rowsErr != nil {
			writeInternal(w, rowsErr, "read organization accounts")
			return
		}
		out["org"] = map[string]any{"id": oid, "name": name, "accounts": accounts}
	}
	writeJSON(w, 200, out)
}

func (s *server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	if !sameOriginMutation(r) {
		writeProblem(w, http.StatusForbidden, "cross-site request rejected")
		return
	}
	uid, ok := s.userFromSession(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "sign in required")
		return
	}
	if _, exists := s.orgOfUser(uid); exists {
		writeProblem(w, http.StatusConflict, "you already have an organization")
		return
	}
	var in struct{ Name string }
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 96 {
		writeProblem(w, 400, "organization name must contain 1-96 bytes")
		return
	}
	now := time.Now().Unix()
	type generatedAccount struct {
		env, publicKey, receiptPublicKey string
		seed, receiptSeed                []byte
	}
	accounts := make([]generatedAccount, 0, 2)
	for _, env := range []string{"test", "live"} {
		pk, seed, err := s.newCustodialAccount()
		if err != nil {
			writeInternal(w, err, "create custodial account")
			return
		}
		rpk, receiptSeed, err := s.newCustodialAccount()
		if err != nil {
			writeInternal(w, err, "create receipt signing account")
			return
		}
		accounts = append(accounts, generatedAccount{env: env, publicKey: pk, seed: seed, receiptPublicKey: rpk, receiptSeed: receiptSeed})
	}

	tx, err := s.db.Begin()
	if err != nil {
		writeInternal(w, err, "begin organization creation")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO orgs (name, created_at) VALUES (?,?)`, in.Name, now)
	if err != nil {
		writeInternal(w, err, "create organization")
		return
	}
	oid, _ := res.LastInsertId()
	if _, err = tx.Exec(`INSERT INTO org_members (org_id, user_id) VALUES (?,?)`, oid, uid); err != nil {
		writeInternal(w, err, "add organization member")
		return
	}
	for _, account := range accounts {
		if _, err = tx.Exec(`INSERT INTO org_accounts (org_id, env, public_key, secret_enc) VALUES (?,?,?,?)`,
			oid, account.env, account.publicKey, account.seed); err != nil {
			writeInternal(w, err, "persist custodial account")
			return
		}
		if _, err = tx.Exec(`INSERT INTO org_receipt_keys (org_id, env, public_key, secret_enc) VALUES (?,?,?,?)`,
			oid, account.env, account.receiptPublicKey, account.receiptSeed); err != nil {
			writeInternal(w, err, "persist receipt signing key")
			return
		}
	}
	month := time.Now().UTC().Format("2006-01")
	if _, err = tx.Exec(`INSERT INTO credits (org_id, env, balance_mcr, grant_month) VALUES (?, 'live', ?, ?)`,
		oid, monthlyGrantMcr, month); err != nil {
		writeInternal(w, err, "create opening credits")
		return
	}
	if err = insertLedgerTx(tx, oid, "live", "monthly_grant", "opening grant", monthlyGrantMcr, monthlyGrantMcr, ""); err != nil {
		writeInternal(w, err, "create opening credit ledger")
		return
	}
	if err = tx.Commit(); err != nil {
		writeInternal(w, err, "commit organization creation")
		return
	}
	for _, account := range accounts {
		go s.fundAccount(oid, account.env, account.publicKey)
	}

	writeJSON(w, 201, map[string]any{"org": map[string]any{"id": oid, "name": in.Name}})
}

// ── API keys ────────────────────────────────────────────────────────

func (s *server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	rows, err := s.db.Query(`SELECT id, env, label, prefix, created_at, last_used_at, revoked_at
	  FROM api_keys WHERE org_id = ? ORDER BY id DESC`, a.OrgID)
	if err != nil {
		writeInternal(w, err, "list API keys")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, created int64
		var lastUsed, revoked sql.NullInt64
		var env, label, prefix string
		if err := rows.Scan(&id, &env, &label, &prefix, &created, &lastUsed, &revoked); err != nil {
			writeInternal(w, err, "decode API key")
			return
		}
		out = append(out, map[string]any{
			"id": id, "env": env, "mode": externalMode(env), "label": label, "prefix": prefix,
			"created_at": created, "last_used_at": nullable(lastUsed), "revoked": revoked.Valid,
		})
	}
	if err := rows.Err(); err != nil {
		writeInternal(w, err, "read API keys")
		return
	}
	writeJSON(w, 200, map[string]any{"keys": out})
}

func (s *server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var in struct{ Label string }
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		in.Label = "Default"
	}
	if len(in.Label) > 96 {
		writeProblem(w, http.StatusBadRequest, "API key label must not exceed 96 bytes")
		return
	}
	random, err := randHex(24)
	if err != nil {
		writeProblem(w, 500, "secure randomness unavailable")
		return
	}
	keyMode := externalMode(a.Env)
	full := "sk_" + keyMode + "_" + random
	prefix := full[:len("sk_"+keyMode+"_")+4] + "…"
	res, err := s.db.Exec(`INSERT INTO api_keys (org_id, env, label, prefix, hash, created_at) VALUES (?,?,?,?,?,?)`,
		a.OrgID, a.Env, in.Label, prefix, hashKey(full), time.Now().Unix())
	if err != nil {
		writeInternal(w, err, "create API key")
		return
	}
	id, _ := res.LastInsertId()
	s.logActivity(a, "key", "", "API key created · "+in.Label, "", nil, 0)
	// the full key is returned exactly once
	writeJSON(w, 201, map[string]any{"id": id, "key": full, "prefix": prefix, "label": in.Label, "env": a.Env, "mode": keyMode})
}

func (s *server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeProblem(w, http.StatusNotFound, "API key not found")
		return
	}
	result, err := s.db.Exec(`UPDATE api_keys SET revoked_at = ? WHERE id = ? AND org_id = ? AND revoked_at IS NULL`,
		time.Now().Unix(), id, a.OrgID)
	if err != nil {
		writeInternal(w, err, "revoke API key")
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		writeInternal(w, err, "confirm API key revocation")
		return
	}
	if changed == 0 {
		writeProblem(w, http.StatusNotFound, "active API key not found")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func nullable(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}
