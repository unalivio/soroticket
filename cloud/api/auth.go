package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
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

func authFrom(r *http.Request) *authCtx {
	v, _ := r.Context().Value(ctxKey{}).(*authCtx)
	return v
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

// ── session auth (console) ──────────────────────────────────────────

func (s *server) createSession(w http.ResponseWriter, userID int64) {
	tok := randHex(24)
	exp := time.Now().Add(30 * 24 * time.Hour).Unix()
	_, _ = s.db.Exec(`INSERT INTO sessions (token, user_id, expires_at) VALUES (?,?,?)`, tok, userID, exp)
	http.SetCookie(w, &http.Cookie{
		Name: "sd_session", Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

func (s *server) userFromSession(r *http.Request) (int64, bool) {
	c, err := r.Cookie("sd_session")
	if err != nil {
		return 0, false
	}
	var uid, exp int64
	err = s.db.QueryRow(`SELECT user_id, expires_at FROM sessions WHERE token = ?`, c.Value).Scan(&uid, &exp)
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
// sk_test_… / sk_live_… — env comes from the key) or from the console session
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
			_, _ = s.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now().Unix(), id)
			ctx := context.WithValue(r.Context(), ctxKey{}, &authCtx{OrgID: orgID, Env: env})
			next(w, r.WithContext(ctx))
			return
		}
		// session path
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
		env := r.Header.Get("X-Env")
		if env != "live" {
			env = "test"
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, &authCtx{UserID: uid, OrgID: oid, Env: env})
		next(w, r.WithContext(ctx))
	}
}

// ── handlers ────────────────────────────────────────────────────────

func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" || !strings.Contains(in.Email, "@") || len(in.Password) < 8 {
		writeProblem(w, 400, "valid email and a password of at least 8 characters are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	res, err := s.db.Exec(`INSERT INTO users (email, pass_hash, created_at) VALUES (?,?,?)`,
		in.Email, string(hash), time.Now().Unix())
	if err != nil {
		writeProblem(w, http.StatusConflict, "an account with that email already exists")
		return
	}
	uid, _ := res.LastInsertId()
	s.createSession(w, uid)
	writeJSON(w, 201, map[string]any{"user": map[string]any{"id": uid, "email": in.Email}})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := readBody(r, &in); err != nil {
		writeProblem(w, 400, err.Error())
		return
	}
	var uid int64
	var hash string
	err := s.db.QueryRow(`SELECT id, pass_hash FROM users WHERE email = ?`,
		strings.ToLower(strings.TrimSpace(in.Email))).Scan(&uid, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		writeProblem(w, http.StatusUnauthorized, "wrong email or password")
		return
	}
	s.createSession(w, uid)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("sd_session"); err == nil {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "sd_session", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.userFromSession(r)
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "not signed in")
		return
	}
	var email string
	_ = s.db.QueryRow(`SELECT email FROM users WHERE id = ?`, uid).Scan(&email)
	out := map[string]any{"user": map[string]any{"id": uid, "email": email}}
	if oid, ok := s.orgOfUser(uid); ok {
		var name string
		_ = s.db.QueryRow(`SELECT name FROM orgs WHERE id = ?`, oid).Scan(&name)
		accounts := map[string]any{}
		rows, _ := s.db.Query(`SELECT env, public_key, funded FROM org_accounts WHERE org_id = ?`, oid)
		for rows.Next() {
			var env, pk string
			var funded int
			_ = rows.Scan(&env, &pk, &funded)
			accounts[env] = map[string]any{"public_key": pk, "funded": funded == 1}
		}
		rows.Close()
		out["org"] = map[string]any{"id": oid, "name": name, "accounts": accounts}
	}
	writeJSON(w, 200, out)
}

func (s *server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
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
	if in.Name == "" {
		writeProblem(w, 400, "organization name is required")
		return
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(`INSERT INTO orgs (name, created_at) VALUES (?,?)`, in.Name, now)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	oid, _ := res.LastInsertId()
	_, _ = s.db.Exec(`INSERT INTO org_members (org_id, user_id) VALUES (?,?)`, oid, uid)

	// custodial accounts for both envs; funding happens in the background
	for _, env := range []string{"test", "live"} {
		pk, encSeed, err := s.newCustodialAccount()
		if err != nil {
			writeProblem(w, 500, err.Error())
			return
		}
		_, _ = s.db.Exec(`INSERT INTO org_accounts (org_id, env, public_key, secret_enc) VALUES (?,?,?,?)`,
			oid, env, pk, encSeed)
		go s.fundAccount(oid, env, pk)
	}
	// live credits: opening monthly grant
	_, _ = s.db.Exec(`INSERT INTO credits (org_id, env, balance_mcr, grant_month) VALUES (?,?,?,?)`,
		oid, "live", monthlyGrantMcr, time.Now().UTC().Format("2006-01"))
	s.ledger(oid, "live", "monthly_grant", "opening grant", monthlyGrantMcr, "")

	writeJSON(w, 201, map[string]any{"org": map[string]any{"id": oid, "name": in.Name}})
}

// ── API keys ────────────────────────────────────────────────────────

func (s *server) handleListKeys(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	rows, err := s.db.Query(`SELECT id, env, label, prefix, created_at, last_used_at, revoked_at
	  FROM api_keys WHERE org_id = ? ORDER BY id DESC`, a.OrgID)
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, created int64
		var lastUsed, revoked sql.NullInt64
		var env, label, prefix string
		_ = rows.Scan(&id, &env, &label, &prefix, &created, &lastUsed, &revoked)
		out = append(out, map[string]any{
			"id": id, "env": env, "label": label, "prefix": prefix,
			"created_at": created, "last_used_at": nullable(lastUsed), "revoked": revoked.Valid,
		})
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
	if strings.TrimSpace(in.Label) == "" {
		in.Label = "Default"
	}
	full := "sk_" + a.Env + "_" + randHex(24)
	prefix := full[:len("sk_"+a.Env+"_")+4] + "…"
	res, err := s.db.Exec(`INSERT INTO api_keys (org_id, env, label, prefix, hash, created_at) VALUES (?,?,?,?,?,?)`,
		a.OrgID, a.Env, strings.TrimSpace(in.Label), prefix, hashKey(full), time.Now().Unix())
	if err != nil {
		writeProblem(w, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	s.logActivity(a, "key", "", "API key created · "+in.Label, "", nil, 0)
	// the full key is returned exactly once
	writeJSON(w, 201, map[string]any{"id": id, "key": full, "prefix": prefix, "label": in.Label, "env": a.Env})
}

func (s *server) handleRevokeKey(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id := r.PathValue("id")
	_, err := s.db.Exec(`UPDATE api_keys SET revoked_at = ? WHERE id = ? AND org_id = ?`,
		time.Now().Unix(), id, a.OrgID)
	if err != nil {
		writeProblem(w, 500, err.Error())
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
