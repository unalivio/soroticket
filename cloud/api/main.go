// Command sorodeal-cloud is the hosted-platform API for the Sorodeal protocol
// (docs/CLOUD.md): a REST layer + console backend over the coupon-ledger
// contract, with per-org custodial Stellar accounts, prepaid-credits metering,
// and both coupon profiles plus loyalty programs.
//
// v1 targets the LIVE TESTNET contract for both environments ("test" is free,
// "live" is metered); mainnet swaps in via chain.go when deployed.
//
//	go run .   # listens on 127.0.0.1:8787, data in ./data/
package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"sync"
)

type server struct {
	db      *sql.DB
	kek     []byte
	locks   map[string]*sync.Mutex
	locksMu sync.Mutex
}

func main() {
	dataDir := envOr("SDCLOUD_DATA", "data")
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
	s := &server{db: db, kek: kek, locks: map[string]*sync.Mutex{}}

	mux := http.NewServeMux()

	// health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})

	// auth + org (session)
	mux.HandleFunc("POST /auth/signup", s.handleSignup)
	mux.HandleFunc("POST /auth/login", s.handleLogin)
	mux.HandleFunc("POST /auth/logout", s.handleLogout)
	mux.HandleFunc("GET /auth/me", s.handleMe)
	mux.HandleFunc("POST /orgs", s.handleCreateOrg)

	// v1 API (session w/ X-Env, or Bearer sk_test_/sk_live_)
	auth := s.requireAuth
	mux.HandleFunc("GET /v1/overview", auth(s.handleOverview))
	mux.HandleFunc("GET /v1/activity", auth(s.handleActivity))

	mux.HandleFunc("POST /v1/campaigns", auth(s.idempotent("campaigns", s.handleCreateCampaign)))
	mux.HandleFunc("GET /v1/campaigns", auth(s.handleListCampaigns))
	mux.HandleFunc("GET /v1/campaigns/{id}", auth(s.handleGetCampaign))
	mux.HandleFunc("POST /v1/campaigns/{id}/archive", auth(s.handleArchiveCampaign))
	mux.HandleFunc("POST /v1/campaigns/{id}/codes", auth(s.idempotent("codes", s.handleIssueCodes)))
	mux.HandleFunc("POST /v1/campaigns/{id}/shared-codes", auth(s.handleRegisterShared))

	mux.HandleFunc("GET /v1/verify", auth(s.handleVerify))
	mux.HandleFunc("POST /v1/redemptions", auth(s.idempotent("redemptions", s.handleRedeem)))
	mux.HandleFunc("GET /v1/redemptions", auth(s.handleListRedemptions))

	mux.HandleFunc("POST /v1/shared-codes/{cid}/{code}/events", auth(s.idempotent("events", s.handleRecordEvents)))
	mux.HandleFunc("POST /v1/shared-codes/{cid}/{code}/commits", auth(s.handleCommitTally))
	mux.HandleFunc("GET /v1/settlements", auth(s.handleListSettlements))
	mux.HandleFunc("POST /v1/settlements", auth(s.handleSettle))

	mux.HandleFunc("POST /v1/loyalty/programs", auth(s.handleCreateProgram))
	mux.HandleFunc("GET /v1/loyalty/programs", auth(s.handleListPrograms))
	mux.HandleFunc("GET /v1/loyalty/programs/{id}", auth(s.handleGetProgram))
	mux.HandleFunc("POST /v1/loyalty/programs/{id}/punches", auth(s.idempotent("punches", s.handlePunch)))

	mux.HandleFunc("GET /v1/keys", auth(s.handleListKeys))
	mux.HandleFunc("POST /v1/keys", auth(s.handleCreateKey))
	mux.HandleFunc("POST /v1/keys/{id}/revoke", auth(s.handleRevokeKey))

	mux.HandleFunc("GET /v1/credits", auth(s.handleCredits))
	mux.HandleFunc("GET /v1/usage", auth(s.handleUsage))
	mux.HandleFunc("POST /v1/credits/recharges", auth(s.handleRecharge))

	addr := envOr("SDCLOUD_ADDR", "127.0.0.1:8787")
	log.Printf("sorodeal-cloud listening on http://%s (data: %s)", addr, dataDir)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
