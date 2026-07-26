// Command soroticket-cloud is the hosted-platform API for the Soroticket protocol
// (docs/CLOUD.md): a REST layer + console backend over the coupon-ledger
// contract, with per-org custodial Stellar accounts, prepaid-credits metering,
// and both coupon profiles plus loyalty programs.
//
// v1 targets testnet for both environments ("test" is free, "live" is a
// metered preview). Neither environment claims to be a mainnet deployment.
//
//	go run .   # listens on 127.0.0.1:8787, data in ./data/
package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type server struct {
	db            *sql.DB
	kek           []byte
	refKey        []byte
	locks         map[string]*sync.Mutex
	locksMu       sync.Mutex
	loginMu       sync.Mutex
	loginAttempts map[string]loginAttempt
	rateMu        sync.Mutex
	rateWindows   map[string]rateWindow
	webhookMu     sync.Mutex
}

func main() {
	dataDir := envOr("SOROTICKET_DATA", "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	db, err := openStore(dataDir + "/console.db")
	if err != nil {
		log.Fatal(err)
	}
	kek, err := loadOrCreateKEK(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	refKey, err := loadOrCreateReferenceKey(dataDir, kek)
	if err != nil {
		log.Fatal(err)
	}
	if err = runSecurityMigrations(db, refKey); err != nil {
		log.Fatal(err)
	}
	if err = runContractStampMigration(db); err != nil {
		log.Fatal(err)
	}
	s := &server{
		db: db, kek: kek, refKey: refKey, locks: map[string]*sync.Mutex{},
		loginAttempts: map[string]loginAttempt{}, rateWindows: map[string]rateWindow{},
	}

	mux := http.NewServeMux()

	// health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /v1/audit/tallies/{chain_id}/{code}/{period}", s.limitPublicAudit(s.handleAuditTally))

	// auth + org (session)
	mux.HandleFunc("POST /auth/signup", s.handleSignup)
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("GET /auth/me", s.handleMe)
	mux.HandleFunc("POST /orgs", s.handleCreateOrg)

	// v1 API (session w/ X-Env, or Bearer sk_test_/sk_metered_;
	// legacy sk_live_ hashes remain valid for backward compatibility)
	auth := s.requireAuth
	mux.HandleFunc("GET /v1/overview", auth(s.handleOverview))
	mux.HandleFunc("GET /v1/activity", auth(s.handleActivity))

	mux.HandleFunc("POST /v1/campaigns", auth(s.idempotent("campaigns", s.handleCreateCampaign)))
	mux.HandleFunc("GET /v1/campaigns", auth(s.handleListCampaigns))
	mux.HandleFunc("GET /v1/campaigns/{id}", auth(s.handleGetCampaign))
	mux.HandleFunc("POST /v1/campaigns/{id}/archive", auth(s.idempotent("campaign_archive", s.handleArchiveCampaign)))
	mux.HandleFunc("POST /v1/campaigns/{id}/codes", auth(s.idempotent("codes", s.handleIssueCodes)))
	mux.HandleFunc("POST /v1/campaigns/{id}/shared-codes", auth(s.idempotent("shared_codes", s.handleRegisterShared)))

	mux.HandleFunc("GET /v1/verify", auth(s.handleVerify))
	mux.HandleFunc("GET /v1/codes/resolve", auth(s.handleResolveCode))
	mux.HandleFunc("POST /v1/redemptions", auth(s.idempotent("redemptions", s.handleRedeem)))
	mux.HandleFunc("GET /v1/redemptions", auth(s.handleListRedemptions))

	mux.HandleFunc("POST /v1/shared-codes/{cid}/{code}/events", auth(s.idempotent("events", s.handleRecordEvents)))
	mux.HandleFunc("POST /v1/shared-codes/{cid}/{code}/commits", auth(s.idempotent("tally_commits", s.handleCommitTally)))
	mux.HandleFunc("GET /v1/settlements", auth(s.handleListSettlements))
	mux.HandleFunc("POST /v1/settlements", auth(s.idempotent("settlements", s.handleSettle)))

	mux.HandleFunc("POST /v1/loyalty/programs", auth(s.idempotent("loyalty_programs", s.handleCreateProgram)))
	mux.HandleFunc("GET /v1/loyalty/programs", auth(s.handleListPrograms))
	mux.HandleFunc("GET /v1/loyalty/programs/{id}", auth(s.handleGetProgram))
	mux.HandleFunc("POST /v1/loyalty/programs/{id}/punches", auth(s.idempotent("punches", s.handlePunch)))

	mux.HandleFunc("GET /v1/keys", auth(s.handleListKeys))
	mux.HandleFunc("POST /v1/keys", auth(s.idempotent("api_keys", s.handleCreateKey)))
	mux.HandleFunc("POST /v1/keys/{id}/revoke", auth(s.idempotent("api_key_revoke", s.handleRevokeKey)))

	mux.HandleFunc("GET /v1/credits", auth(s.handleCredits))
	mux.HandleFunc("GET /v1/usage", auth(s.handleUsage))
	mux.HandleFunc("POST /v1/credits/recharges", auth(s.handleRecharge))
	mux.HandleFunc("GET /v1/webhooks", auth(s.handleListWebhooks))
	mux.HandleFunc("POST /v1/webhooks", auth(s.idempotent("webhooks", s.handleCreateWebhook)))
	mux.HandleFunc("POST /v1/webhooks/{id}/disable", auth(s.idempotent("webhook_disable", s.handleDisableWebhook)))
	mux.HandleFunc("POST /v1/webhooks/{id}/test", auth(s.idempotent("webhook_test", s.handleTestWebhook)))

	go s.webhookLoop()

	addr := envOr("SDCLOUD_ADDR", "127.0.0.1:8787")
	log.Printf("soroticket-cloud listening on http://%s (data: %s)", addr, dataDir)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      90 * time.Second, // Soroban submissions may span ledgers
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	log.Fatal(httpServer.ListenAndServe())
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
