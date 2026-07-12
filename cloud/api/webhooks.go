package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	webhookMaxAttempts = 8
	webhookTimeout     = 10 * time.Second
)

var supportedWebhookEvents = map[string]struct{}{
	"redemption.created":    {},
	"tally.committed":       {},
	"settlement.paid":       {},
	"loyalty.reward_issued": {},
	"credits.low":           {},
}

type webhookRow struct {
	ID           int64    `json:"id"`
	URL          string   `json:"url"`
	Events       []string `json:"events"`
	Active       bool     `json:"active"`
	SecretPrefix string   `json:"secret_prefix"`
	CreatedAt    int64    `json:"created_at"`
	LastStatus   string   `json:"last_status"`
	LastAttempt  int64    `json:"last_attempt"`
}

func validateWebhookURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return "", errors.New("webhook URL must not exceed 2048 bytes")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" {
		return "", errors.New("webhook URL must be an absolute https URL")
	}
	u.Scheme = "https"
	if u.User != nil || u.Fragment != "" {
		return "", errors.New("webhook URL cannot contain credentials or a fragment")
	}
	if port := u.Port(); port != "" && port != "443" {
		return "", errors.New("webhook URL must use HTTPS port 443")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !isPublicWebhookIP(ip) {
		return "", errors.New("webhook URL cannot target a private or reserved address")
	}
	return u.String(), nil
}

func isPublicWebhookIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	// Shared address space and benchmarking ranges are not public webhook
	// destinations even though older Go releases classify them as unicast.
	for _, raw := range []string{"100.64.0.0/10", "198.18.0.0/15"} {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func resolvePublicWebhookHost(ctx context.Context, hostname string) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return fmt.Errorf("resolve webhook host: %w", err)
	}
	for _, candidate := range ips {
		if isPublicWebhookIP(candidate.IP) {
			return nil
		}
	}
	return errors.New("webhook host has no public address")
}

func secureWebhookClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range ips {
				if !isPublicWebhookIP(candidate.IP) {
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				err = dialErr
			}
			if err != nil {
				return nil, err
			}
			return nil, errors.New("webhook host has no public address")
		},
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   webhookTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("webhook redirects are not followed")
		},
	}
}

func webhookSignature(secret []byte, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(payload)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func sendWebhook(ctx context.Context, client *http.Client, endpoint string, secret []byte, deliveryID, eventType string, payload []byte) (int, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Sorodeal-Webhooks/1.0")
	req.Header.Set("X-Sorodeal-Delivery", deliveryID)
	req.Header.Set("X-Sorodeal-Event", eventType)
	req.Header.Set("X-Sorodeal-Timestamp", timestamp)
	req.Header.Set("X-Sorodeal-Signature", webhookSignature(secret, timestamp, payload))
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("endpoint returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func normalizeWebhookEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, errors.New("select at least one webhook event")
	}
	seen := make(map[string]struct{}, len(events))
	out := make([]string, 0, len(events))
	for _, event := range events {
		event = strings.TrimSpace(event)
		if _, ok := supportedWebhookEvents[event]; !ok {
			return nil, fmt.Errorf("unsupported webhook event %q", event)
		}
		if _, duplicate := seen[event]; duplicate {
			continue
		}
		seen[event] = struct{}{}
		out = append(out, event)
	}
	sort.Strings(out)
	return out, nil
}

func (s *server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	var activeCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM webhooks WHERE org_id=? AND env=? AND active=1`, a.OrgID, a.Env).Scan(&activeCount); err != nil {
		writeInternal(w, err, "count webhooks")
		return
	}
	if activeCount >= 20 {
		writeProblem(w, http.StatusConflict, "maximum 20 active webhook endpoints per environment")
		return
	}
	var in struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := readBody(r, &in); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	endpoint, err := validateWebhookURL(in.URL)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	u, _ := url.Parse(endpoint)
	if err = resolvePublicWebhookHost(r.Context(), u.Hostname()); err != nil {
		writeProblem(w, http.StatusBadRequest, "webhook host must resolve to a public address")
		return
	}
	events, err := normalizeWebhookEvents(in.Events)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	random, err := randHex(32)
	if err != nil {
		writeInternal(w, err, "generate webhook secret")
		return
	}
	secret := "whsec_" + random
	sealed, err := s.seal([]byte(secret))
	if err != nil {
		writeInternal(w, err, "seal webhook secret")
		return
	}
	encodedEvents, _ := json.Marshal(events)
	now := time.Now().Unix()
	result, err := s.db.Exec(`INSERT INTO webhooks
	  (org_id, env, url, secret_enc, secret_prefix, events, active, created_at)
	  VALUES (?,?,?,?,?,?,1,?)`, a.OrgID, a.Env, endpoint, sealed, secret[:14]+"…", encodedEvents, now)
	if err != nil {
		writeInternal(w, err, "create webhook")
		return
	}
	id, _ := result.LastInsertId()
	writeJSON(w, http.StatusCreated, map[string]any{
		"webhook": map[string]any{
			"id": id, "url": endpoint, "events": events, "active": true,
			"secret": secret, "created_at": now,
		},
		"warning": "Copy the signing secret now; it is not returned again.",
	})
}

func (s *server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	rows, err := s.db.Query(`SELECT w.id, w.url, w.events, w.active, w.secret_prefix, w.created_at,
	  COALESCE((SELECT d.status FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY d.id DESC LIMIT 1),'never'),
	  COALESCE((SELECT d.created_at FROM webhook_deliveries d WHERE d.webhook_id=w.id ORDER BY d.id DESC LIMIT 1),0)
	  FROM webhooks w WHERE w.org_id=? AND w.env=? ORDER BY w.id DESC`, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "list webhooks")
		return
	}
	defer rows.Close()
	out := make([]webhookRow, 0)
	for rows.Next() {
		var row webhookRow
		var active int
		var encoded []byte
		if err = rows.Scan(&row.ID, &row.URL, &encoded, &active, &row.SecretPrefix, &row.CreatedAt, &row.LastStatus, &row.LastAttempt); err != nil {
			writeInternal(w, err, "scan webhook")
			return
		}
		row.Active = active == 1
		_ = json.Unmarshal(encoded, &row.Events)
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": out, "supported_events": sortedWebhookEvents()})
}

func sortedWebhookEvents() []string {
	out := make([]string, 0, len(supportedWebhookEvents))
	for event := range supportedWebhookEvents {
		out = append(out, event)
	}
	sort.Strings(out)
	return out
}

func (s *server) handleDisableWebhook(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeProblem(w, http.StatusBadRequest, "invalid webhook id")
		return
	}
	result, err := s.db.Exec(`UPDATE webhooks SET active=0, disabled_at=?
	  WHERE id=? AND org_id=? AND env=? AND active=1`, time.Now().Unix(), id, a.OrgID, a.Env)
	if err != nil {
		writeInternal(w, err, "disable webhook")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		writeProblem(w, http.StatusNotFound, "active webhook not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"disabled": true})
}

func (s *server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeProblem(w, http.StatusBadRequest, "invalid webhook id")
		return
	}
	var active int
	if err = s.db.QueryRow(`SELECT active FROM webhooks WHERE id=? AND org_id=? AND env=?`, id, a.OrgID, a.Env).Scan(&active); err != nil || active != 1 {
		writeProblem(w, http.StatusNotFound, "active webhook not found")
		return
	}
	deliveryID, err := s.queueWebhook(id, "endpoint.test", map[string]any{
		"object": "webhook_test", "message": "Sorodeal webhook delivery is working.",
	}, a.Env)
	if err != nil {
		writeInternal(w, err, "queue test webhook")
		return
	}
	s.kickWebhookDelivery()
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "delivery_id": deliveryID})
}

func activityWebhookType(kind string) string {
	switch kind {
	case "redemption", "event":
		return "redemption.created"
	case "tally":
		return "tally.committed"
	case "settle":
		return "settlement.paid"
	case "reward":
		return "loyalty.reward_issued"
	case "credits_low":
		return "credits.low"
	default:
		return ""
	}
}

func containsEvent(encoded []byte, eventType string) bool {
	var events []string
	if json.Unmarshal(encoded, &events) != nil {
		return false
	}
	for _, event := range events {
		if event == eventType {
			return true
		}
	}
	return false
}

func (s *server) enqueueActivityWebhook(a *authCtx, activityID int64, kind, code, message, txHash string, campaignID *int64, errorCode int, createdAt int64) {
	eventType := activityWebhookType(kind)
	if eventType == "" {
		return
	}
	rows, err := s.db.Query(`SELECT id, events FROM webhooks WHERE org_id=? AND env=? AND active=1`, a.OrgID, a.Env)
	if err != nil {
		log.Printf("list webhook subscriptions: %v", err)
		return
	}
	type subscription struct {
		id     int64
		events []byte
	}
	var subscriptions []subscription
	for rows.Next() {
		var item subscription
		if rows.Scan(&item.id, &item.events) == nil {
			subscriptions = append(subscriptions, item)
		}
	}
	rows.Close()
	data := map[string]any{
		"activity_id": activityID, "kind": kind, "code": code, "message": message,
		"tx_hash": txHash, "error_code": errorCode,
	}
	if campaignID != nil {
		data["campaign_id"] = *campaignID
	}
	queued := false
	for _, subscription := range subscriptions {
		if !containsEvent(subscription.events, eventType) {
			continue
		}
		if _, err = s.queueWebhookAt(subscription.id, eventType, data, a.Env, createdAt); err != nil {
			log.Printf("queue webhook org=%d event=%s: %v", a.OrgID, eventType, err)
		} else {
			queued = true
		}
	}
	if queued {
		s.kickWebhookDelivery()
	}
}

func (s *server) queueWebhook(webhookID int64, eventType string, data map[string]any, env string) (string, error) {
	return s.queueWebhookAt(webhookID, eventType, data, env, time.Now().Unix())
}

func (s *server) queueWebhookAt(webhookID int64, eventType string, data map[string]any, env string, createdAt int64) (string, error) {
	random, err := randHex(16)
	if err != nil {
		return "", err
	}
	deliveryID := "whd_" + random
	payload, err := json.Marshal(map[string]any{
		"id": deliveryID, "type": eventType, "created_at": createdAt,
		"mode": externalMode(env), "network": "testnet", "production": false,
		"livemode": false, "data": data,
	})
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO webhook_deliveries
	  (webhook_id, delivery_id, event_type, payload, status, attempts, next_attempt_at, created_at)
	  VALUES (?,?,?,?, 'pending',0,?,?)`, webhookID, deliveryID, eventType, payload, createdAt, createdAt)
	return deliveryID, err
}

func (s *server) webhookLoop() {
	s.deliverDueWebhooks(context.Background())
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.deliverDueWebhooks(context.Background())
	}
}

func (s *server) kickWebhookDelivery() {
	go s.deliverDueWebhooks(context.Background())
}

func (s *server) deliverDueWebhooks(ctx context.Context) {
	if !s.webhookMu.TryLock() {
		return
	}
	defer s.webhookMu.Unlock()

	type delivery struct {
		id, webhookID int64
		deliveryID    string
		eventType     string
		payload       []byte
		attempts      int
		endpoint      string
		secretBox     []byte
	}
	rows, err := s.db.Query(`SELECT d.id, d.webhook_id, d.delivery_id, d.event_type, d.payload,
	  d.attempts, w.url, w.secret_enc
	  FROM webhook_deliveries d JOIN webhooks w ON w.id=d.webhook_id
	  WHERE d.status IN ('pending','retrying') AND d.next_attempt_at<=? AND w.active=1
	  ORDER BY d.id LIMIT 20`, time.Now().Unix())
	if err != nil {
		log.Printf("read due webhooks: %v", err)
		return
	}
	var due []delivery
	for rows.Next() {
		var item delivery
		if rows.Scan(&item.id, &item.webhookID, &item.deliveryID, &item.eventType, &item.payload,
			&item.attempts, &item.endpoint, &item.secretBox) == nil {
			due = append(due, item)
		}
	}
	rows.Close()

	client := secureWebhookClient()
	for _, item := range due {
		secret, decryptErr := s.unseal(item.secretBox)
		status := 0
		deliveryErr := decryptErr
		if deliveryErr == nil {
			attemptCtx, cancel := context.WithTimeout(ctx, webhookTimeout)
			status, deliveryErr = sendWebhook(attemptCtx, client, item.endpoint, secret, item.deliveryID, item.eventType, item.payload)
			cancel()
		}
		for i := range secret {
			secret[i] = 0
		}
		attempts := item.attempts + 1
		now := time.Now().Unix()
		if deliveryErr == nil {
			_, _ = s.db.Exec(`UPDATE webhook_deliveries SET status='delivered', attempts=?, response_status=?,
			  last_error=NULL, delivered_at=? WHERE id=?`, attempts, status, now, item.id)
			continue
		}
		errText := deliveryErr.Error()
		if len(errText) > 500 {
			errText = errText[:500]
		}
		if attempts >= webhookMaxAttempts {
			_, _ = s.db.Exec(`UPDATE webhook_deliveries SET status='failed', attempts=?, response_status=?,
			  last_error=? WHERE id=?`, attempts, nullableStatus(status), errText, item.id)
			continue
		}
		delay := int64(30 * (1 << min(attempts-1, 9)))
		_, _ = s.db.Exec(`UPDATE webhook_deliveries SET status='retrying', attempts=?, response_status=?,
		  last_error=?, next_attempt_at=? WHERE id=?`, attempts, nullableStatus(status), errText, now+delay, item.id)
	}
}

func nullableStatus(status int) any {
	if status == 0 {
		return sql.NullInt64{}
	}
	return status
}
