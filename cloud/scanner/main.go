// Command soroticket-scanner is the WhatsApp scan endpoint of Soroticket
// (docs: the product PDF's Camino A / Camino B split). It is a CONSUMER of the
// Soroticket Cloud API — it never creates coupons, it only receives scans and
// answers (per the product doc: "El bot no fabrica cupones").
//
//   Camino A (customer scans a fixed QR): the scan IS the redemption — a
//   shared event is recorded with the customer's phone as opaque reference.
//   Per-person limits ride on the API's order_ref dedup ("scan|CODE|phone").
//   Gift (proof-of-delivery) campaigns require a second step: the customer
//   shares their WhatsApp location, which is committed into the signed
//   receipt as evidence (context_hash); raw coordinates stay in this layer.
//
//   Camino B (employee scans the customer's unique QR): the scan IS the
//   validation — the code burns on-chain; green (VÁLIDO) or red (YA USADO).
//
// Blockchain stays invisible: replies never mention hashes or addresses.
package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type config struct {
	listen     string
	apiBase    string // Soroticket Cloud API (internal)
	apiKey     string // sk_test_… — the org this scanner serves
	publicURL  string // exact webhook URL Twilio signs (behind TLS proxy)
	twilioAuth string // Twilio auth token; empty = signature not enforced
	pathSecret string // shared secret required in the webhook path; empty = not required
}

type scanner struct {
	cfg    config
	client *http.Client

	mu      sync.Mutex
	pending map[string]pendingScan // phone → gift scan awaiting location
	seenSid map[string]time.Time   // Twilio MessageSid dedup (webhook retries)
	done    map[string]time.Time   // phone|code → completed scans (UX cache):
	// answers "ya lo usaste" BEFORE asking for location again. The API's
	// order_ref dedup remains the source of truth (this cache is lost on
	// restart; the 409 path then covers it after the location step).
}

type pendingScan struct {
	code       string
	campaignID int64
	name       string
	expires    time.Time
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	cfg := config{
		listen:     envOr("SCANNER_LISTEN", "127.0.0.1:8090"),
		apiBase:    envOr("SOROTICKET_API", "http://127.0.0.1:8787"),
		apiKey:     os.Getenv("SOROTICKET_API_KEY"),
		publicURL:  envOr("SCANNER_PUBLIC_URL", ""),
		twilioAuth: os.Getenv("TWILIO_AUTH_TOKEN"),
		pathSecret: os.Getenv("SCANNER_PATH_SECRET"),
	}
	if cfg.apiKey == "" {
		log.Fatal("SOROTICKET_API_KEY is required (sk_test_… of the org this scanner serves)")
	}
	// Two independent gates; either one keeps strangers from injecting scans.
	// The Twilio signature is stronger (it proves who signed each request); the
	// path secret needs no Twilio credentials on this host.
	if cfg.twilioAuth == "" && cfg.pathSecret == "" {
		log.Print("WARNING: neither TWILIO_AUTH_TOKEN nor SCANNER_PATH_SECRET is set — the webhook is UNAUTHENTICATED (dev only)")
	}
	if cfg.twilioAuth == "" {
		log.Print("note: TWILIO_AUTH_TOKEN unset — request signatures are not verified")
	}
	s := &scanner{
		cfg:     cfg,
		client:  &http.Client{Timeout: 60 * time.Second},
		pending: map[string]pendingScan{},
		seenSid: map[string]time.Time{},
		done:    map[string]time.Time{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"service":"soroticket-scanner"}`))
	})
	// The bare path stays registered so a misconfigured Twilio sender fails
	// loudly (403 + log) instead of silently dropping scans.
	mux.HandleFunc("POST /bot/whatsapp/webhook", s.handleWebhook)
	mux.HandleFunc("POST /bot/whatsapp/webhook/{secret}", s.handleWebhook)
	log.Printf("soroticket-scanner listening on %s (api %s)", cfg.listen, cfg.apiBase)
	log.Fatal(http.ListenAndServe(cfg.listen, mux))
}

// ── Twilio webhook ───────────────────────────────────────────────────

func (s *scanner) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	if !s.validPathSecret(r) {
		log.Printf("rejected webhook: bad or missing path secret (from %s)", r.RemoteAddr)
		http.Error(w, "not found", 404)
		return
	}
	if !s.validSignature(r) {
		log.Printf("rejected webhook: invalid Twilio signature (from %s)", r.RemoteAddr)
		http.Error(w, "invalid signature", 403)
		return
	}
	sid := r.PostForm.Get("MessageSid")
	if sid != "" && s.alreadySeen(sid) {
		reply(w, "") // webhook retry — already answered
		return
	}
	from := r.PostForm.Get("From") // "whatsapp:+58412…"
	phone := strings.TrimPrefix(from, "whatsapp:")
	body := strings.TrimSpace(r.PostForm.Get("Body"))
	lat, lon := r.PostForm.Get("Latitude"), r.PostForm.Get("Longitude")

	switch {
	case lat != "" && lon != "":
		reply(w, s.onLocation(phone, lat, lon))
	case body != "":
		reply(w, s.onText(phone, body))
	default:
		reply(w, "Envía el código de tu cupón o entrada 🎟️")
	}
}

// scanTokenPattern matches the opaque token a printed QR carries (see
// cloud/api/handlers_qr.go). Checked before codePattern because the prefilled
// WhatsApp text may also contain surrounding words.
var scanTokenPattern = regexp.MustCompile(`\bST[A-Z0-9]{14}\b`)

var codePattern = regexp.MustCompile(`[A-Z0-9][A-Z0-9\-]{2,63}`)

// onText resolves the scan token (from a printed QR) or a typed code and runs
// the right path.
func (s *scanner) onText(phone, body string) string {
	upper := strings.ToUpper(body)
	token := scanTokenPattern.FindString(upper)
	code := token
	if token == "" {
		code = codePattern.FindString(upper)
	}
	if code == "" {
		return "🤔 No reconocimos ningún código en tu mensaje. Escanea el QR o escribe el código tal como aparece."
	}
	res, status, err := s.resolve(code, token != "")
	// A token-shaped string that no campaign knows may still be a typed code:
	// retry as a code before telling the customer it does not exist.
	if token != "" && status == 404 {
		res, status, err = s.resolve(code, false)
	}
	if err != nil {
		log.Printf("resolve %q: %v", code, err)
		return "Tuvimos un problema técnico. Intenta de nuevo en un momento."
	}
	switch {
	case status == 404:
		if token != "" {
			return "🤔 Este QR no está activo. Puede ser de una promo que ya terminó — pregunta en el local."
		}
		// codePattern also matches ordinary words ("hola"), so a plain-letter
		// miss is far more likely to be conversation than a mistyped code:
		// don't tell someone their greeting is an invalid coupon.
		if !strings.ContainsAny(code, "0123456789-") {
			return "👋 ¡Hola! Escanea el QR de la promoción o envía el código exactamente como aparece."
		}
		return fmt.Sprintf("🤔 El código %s no existe. Revisa que esté escrito exactamente como aparece.", code)
	case res.Archived:
		return fmt.Sprintf("Esta promo (%s) ya no está activa.", res.CampaignName)
	case res.Expired:
		return fmt.Sprintf("⏰ %s venció. Pregunta en el local por la promo vigente.", res.CampaignName)
	}

	if res.Type == "unique" {
		return s.redeemUnique(phone, res)
	}
	if s.alreadyDone(phone, res.Code) {
		return "Ya usaste este código — es una vez por persona. 😉"
	}
	if res.Kind == "gift" {
		// proof-of-delivery needs the second factor: real device location
		s.setPending(phone, pendingScan{code: res.Code, campaignID: res.CampaignID, name: res.CampaignName, expires: time.Now().Add(10 * time.Minute)})
		return "📍 Para validar tu escaneo, comparte tu ubicación:\n\nToca el clip 📎 → Ubicación → «Enviar tu ubicación actual»."
	}
	return s.recordShared(phone, res.CampaignID, res.Code, res.CampaignName, res.DiscountType, res.DiscountValue, evidence{Type: "whatsapp_scan", Policy: "scan-v1"})
}

// onLocation completes a pending gift scan with the shared location.
func (s *scanner) onLocation(phone, lat, lon string) string {
	p, ok := s.takePending(phone)
	if !ok {
		return "Recibimos tu ubicación, pero no hay ningún escaneo esperándola. Envía primero el código del QR."
	}
	// Raw coordinates stay in this layer (integrator layer): only a
	// commitment reaches the signed receipt. journald keeps the raw pair.
	log.Printf("geo-evidence phone=%s code=%s lat=%s lon=%s", phone, p.code, lat, lon)
	h := sha256.Sum256([]byte(lat + "|" + lon + "|" + phone + "|" + p.code))
	ev := evidence{Type: "whatsapp_scan_geo", Policy: "geo-v1", ContextHash: hex.EncodeToString(h[:])}
	return s.recordShared(phone, p.campaignID, p.code, p.name, "", 0, ev)
}

// ── Cloud API calls ──────────────────────────────────────────────────

type resolved struct {
	Type          string `json:"type"`
	Code          string `json:"code"`
	CampaignID    int64  `json:"campaign_id"`
	Kind          string `json:"kind"`
	CampaignName  string `json:"campaign_name"`
	DiscountType  string `json:"discount_type"`
	DiscountValue int64  `json:"discount_value"`
	Archived      bool   `json:"archived"`
	Expired       bool   `json:"expired"`
	Status        string `json:"status"`
}

type evidence struct {
	Type, Policy, ContextHash string
}

func (s *scanner) resolve(value string, asToken bool) (resolved, int, error) {
	param := "code"
	if asToken {
		param = "token"
	}
	var out resolved
	status, err := s.api("GET", "/v1/codes/resolve?"+param+"="+url.QueryEscape(value), nil, &out)
	return out, status, err
}

func (s *scanner) recordShared(phone string, campaignID int64, code, name, discountType string, discountValue int64, ev evidence) string {
	payload := map[string]any{
		"customer_ref": phone,
		// order_ref implements "una vez por persona": the API deduplicates the
		// same business reference, so a second scan answers 409 (already used).
		"order_ref":      "scan|" + code + "|" + phone,
		"evidence_type":  ev.Type,
		"policy_version": ev.Policy,
	}
	if ev.ContextHash != "" {
		payload["context_hash"] = ev.ContextHash
	}
	var out map[string]any
	status, err := s.api("POST", fmt.Sprintf("/v1/shared-codes/%d/%s/events", campaignID, url.PathEscape(code)), payload, &out)
	switch {
	case err != nil:
		log.Printf("record event %s: %v", code, err)
		return "Tuvimos un problema técnico registrando tu escaneo. Intenta de nuevo."
	case status == 409:
		s.markDone(phone, code)
		return "Ya usaste este código — es una vez por persona. 😉"
	case status != 201:
		log.Printf("record event %s: unexpected status %d: %v", code, status, out)
		return "No pudimos registrar el escaneo. Intenta de nuevo en un momento."
	}
	s.markDone(phone, code)
	return "✓ Registrado — " + discountCopy(name, discountType, discountValue) + "\n\nMuestra este mensaje en el mostrador si te lo piden."
}

func (s *scanner) redeemUnique(phone string, res resolved) string {
	payload := map[string]any{"campaign_id": res.CampaignID, "code": res.Code, "redeemer_ref": phone}
	var out map[string]any
	status, err := s.api("POST", "/v1/redemptions", payload, &out)
	switch {
	case err != nil:
		log.Printf("redeem %s: %v", res.Code, err)
		return "Tuvimos un problema técnico validando el código. Intenta de nuevo."
	case status == 201:
		return fmt.Sprintf("✅ VÁLIDO — %s\n%s · recién marcado como usado.", res.CampaignName, res.Code)
	}
	if errCode, ok := out["code"].(float64); ok && int(errCode) == 3 { // AlreadyRedeemed
		return fmt.Sprintf("❌ YA USADO — %s ya fue canjeado antes.", res.Code)
	}
	if msg, ok := out["message"].(string); ok && msg != "" {
		return "❌ " + msg
	}
	return "❌ No se pudo validar este código."
}

func (s *scanner) api(method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, s.cfg.apiBase+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode, nil
}

func discountCopy(name, discountType string, value int64) string {
	switch discountType {
	case "percentage":
		return fmt.Sprintf("tienes %d%% de descuento (%s).", value, name)
	case "fixed_amount":
		return fmt.Sprintf("tienes %d de descuento (%s).", value, name)
	default:
		return "«" + name + "»."
	}
}

// ── plumbing ─────────────────────────────────────────────────────────

// validPathSecret requires the configured secret as the last path segment.
// Compared in constant time; answers 404 (not 403) so probing the bare path
// cannot distinguish "wrong secret" from "no such endpoint".
func (s *scanner) validPathSecret(r *http.Request) bool {
	if s.cfg.pathSecret == "" {
		return true
	}
	got := r.PathValue("secret")
	return len(got) == len(s.cfg.pathSecret) &&
		hmac.Equal([]byte(got), []byte(s.cfg.pathSecret))
}

// validSignature checks X-Twilio-Signature: base64(HMAC-SHA1(token,
// publicURL + concat(sorted(param+value)))). Dev mode (no token) accepts all.
func (s *scanner) validSignature(r *http.Request) bool {
	if s.cfg.twilioAuth == "" {
		return true
	}
	keys := make([]string, 0, len(r.PostForm))
	for k := range r.PostForm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(s.cfg.publicURL)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(r.PostForm.Get(k))
	}
	mac := hmac.New(sha1.New, []byte(s.cfg.twilioAuth))
	_, _ = mac.Write([]byte(b.String()))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(r.Header.Get("X-Twilio-Signature")))
}

func (s *scanner) alreadySeen(sid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.seenSid {
		if now.Sub(t) > time.Hour {
			delete(s.seenSid, k)
		}
	}
	if _, ok := s.seenSid[sid]; ok {
		return true
	}
	s.seenSid[sid] = now
	return false
}

func (s *scanner) markDone(phone, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.done {
		if now.Sub(t) > 24*time.Hour {
			delete(s.done, k)
		}
	}
	s.done[phone+"|"+code] = now
}

func (s *scanner) alreadyDone(phone, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.done[phone+"|"+code]
	return ok
}

func (s *scanner) setPending(phone string, p pendingScan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[phone] = p
}

func (s *scanner) takePending(phone string) (pendingScan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[phone]
	if ok {
		delete(s.pending, phone)
	}
	if !ok || time.Now().After(p.expires) {
		return pendingScan{}, false
	}
	return p, true
}

type twiml struct {
	XMLName xml.Name `xml:"Response"`
	Message string   `xml:"Message,omitempty"`
}

func reply(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/xml")
	raw, _ := xml.Marshal(twiml{Message: message})
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(raw)
}
