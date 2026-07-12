package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// recorder captures a handler's response so it can be stored and replayed for
// idempotent retries.
type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	if r.status != 0 {
		return
	}
	r.status = code
}
func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.buf.Write(b)
}

var idempotencyKeyRE = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,255}$`)

// idempotent wraps a POST handler: when the request carries an
// Idempotency-Key, the first response is stored and any retry with the same
// key gets the stored response back — a network retry can never double-issue
// or double-redeem. Keys are scoped per org + endpoint and kept for 24h.
func (s *server) idempotent(endpoint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			next(w, r)
			return
		}
		if !idempotencyKeyRE.MatchString(key) {
			writeProblem(w, http.StatusBadRequest, "Idempotency-Key must be 1-255 letters, digits, '.', '_', ':' or '-'")
			return
		}
		a := authFrom(r)

		// Hash the exact request target + body, then restore the body for the
		// real handler. Reusing a key for different parameters is an error.
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeProblem(w, http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB")
			return
		}
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
		mac := hmac.New(sha256.New, s.refKey)
		_, _ = mac.Write([]byte(r.Method + "\n" + r.URL.RequestURI() + "\n"))
		_, _ = mac.Write(body)
		requestHash := hex.EncodeToString(mac.Sum(nil))
		now := time.Now().Unix()

		res, err := s.db.Exec(`INSERT OR IGNORE INTO idempotency_v2
		  (key, org_id, env, endpoint, request_hash, created_at) VALUES (?,?,?,?,?,?)`,
			key, a.OrgID, a.Env, endpoint, requestHash, now)
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "could not reserve idempotency key")
			return
		}
		inserted, _ := res.RowsAffected()
		if inserted == 0 {
			var storedHash, contentType string
			var status int
			var storedBody []byte
			err := s.db.QueryRow(`SELECT request_hash, status, content_type, body FROM idempotency_v2
			  WHERE key = ? AND org_id = ? AND env = ? AND endpoint = ?`,
				key, a.OrgID, a.Env, endpoint).Scan(&storedHash, &status, &contentType, &storedBody)
			if err != nil {
				writeProblem(w, http.StatusInternalServerError, "could not read idempotency result")
				return
			}
			if storedHash != requestHash {
				writeProblem(w, http.StatusConflict, "Idempotency-Key was already used with different parameters")
				return
			}
			if status == 0 {
				w.Header().Set("Retry-After", "1")
				writeProblem(w, http.StatusConflict, "a request with this Idempotency-Key is still in progress")
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Idempotency-Replayed", "true")
			w.WriteHeader(status)
			_, _ = w.Write(storedBody)
			return
		}

		rec := &recorder{ResponseWriter: w}
		next(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		contentType := rec.Header().Get("Content-Type")
		if contentType == "" {
			contentType = "application/json"
		}
		result, err := s.db.Exec(`UPDATE idempotency_v2 SET status = ?, content_type = ?, body = ?, completed_at = ?
		  WHERE key = ? AND org_id = ? AND env = ? AND endpoint = ? AND request_hash = ?`,
			rec.status, contentType, rec.buf.Bytes(), time.Now().Unix(),
			key, a.OrgID, a.Env, endpoint, requestHash)
		if err != nil {
			writeInternal(w, err, "persist idempotency result")
			return
		}
		updated, err := result.RowsAffected()
		if err != nil || updated != 1 {
			if err == nil {
				err = fmt.Errorf("idempotency result updated %d rows", updated)
			}
			writeInternal(w, err, "persist idempotency result")
			return
		}

		// A successful mutation is acknowledged only after its replay record is
		// durable. If persistence failed above, the reserved in-progress key
		// remains fail-closed instead of falsely advertising a replayable result.
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.buf.Bytes())
		// opportunistic 24h sweep
		_, _ = s.db.Exec(`DELETE FROM idempotency_v2 WHERE created_at < ?`, time.Now().Add(-24*time.Hour).Unix())
	}
}
