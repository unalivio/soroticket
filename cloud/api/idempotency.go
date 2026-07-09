package main

import (
	"bytes"
	"net/http"
	"time"
)

// recorder captures a handler's response so it can be stored and replayed for
// idempotent retries.
type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }
func (r *recorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

// idempotent wraps a POST handler: when the request carries an
// Idempotency-Key, the first response is stored and any retry with the same
// key gets the stored response back — a network retry can never double-issue
// or double-redeem. Keys are scoped per org + endpoint and kept for 24h.
func (s *server) idempotent(endpoint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			next(w, r)
			return
		}
		a := authFrom(r)
		var status int
		var body []byte
		err := s.db.QueryRow(`SELECT status, body FROM idempotency WHERE key = ? AND org_id = ? AND endpoint = ?`,
			key, a.OrgID, endpoint).Scan(&status, &body)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(status)
			_, _ = w.Write(body)
			return
		}
		rec := &recorder{ResponseWriter: w, status: 200}
		next(rec, r)
		_, _ = s.db.Exec(`INSERT OR IGNORE INTO idempotency (key, org_id, endpoint, status, body, created_at)
		  VALUES (?,?,?,?,?,?)`, key, a.OrgID, endpoint, rec.status, rec.buf.Bytes(), time.Now().Unix())
		// opportunistic 24h sweep
		_, _ = s.db.Exec(`DELETE FROM idempotency WHERE created_at < ?`, time.Now().Add(-24*time.Hour).Unix())
	}
}
