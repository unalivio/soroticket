package main

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"rsc.io/qr"
)

// The printed QR carries a wa.me deep link whose prefilled text is an opaque
// scan token — never the coupon code. Scanning with the phone camera opens
// WhatsApp with the message ready; the customer only presses send. Reading the
// QR with any scanner app reveals nothing about the campaign.
//
// Why an opaque token and not an encrypted payload: a ciphertext carrying
// campaign+code would be far longer (denser QR, worse on a curved bottle
// label), and rotating the key would invalidate every bottle already printed.
// A short random token is unguessable, reveals nothing, survives key rotation
// and can be revoked on its own.

// tokenAlphabet excludes 0/O/1/I/L (same reasoning as codeAlphabet) and is
// uppercase so the QR can use alphanumeric mode where possible.
const tokenAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const (
	tokenPrefix = "ST"
	tokenBody   = 14 // ~69 bits of entropy
)

func newScanToken() (string, error) {
	out := make([]byte, tokenBody)
	bound := big.NewInt(int64(len(tokenAlphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, bound)
		if err != nil {
			return "", err
		}
		out[i] = tokenAlphabet[v.Int64()]
	}
	return tokenPrefix + string(out), nil
}

// waNumber is the WhatsApp number of the Soroticket-provided bot. Soroticket
// hands the bot to merchants so they configure nothing (see docs/USE_CASES.md);
// without it there is no honest deep link to hand out, so the endpoint fails
// closed rather than returning a link that goes nowhere.
func (s *server) waNumber() string { return strings.TrimSpace(envOr("SOROTICKET_WA_NUMBER", "")) }

func deepLink(number, token string) string {
	return "https://wa.me/" + url.PathEscape(number) + "?text=" + url.QueryEscape(token)
}

// scanTokenFor returns the live token for (campaign, code), minting one on
// first use. The same QR is printed on every bottle of a campaign, so the
// token must be stable: never rotate it implicitly.
func (s *server) scanTokenFor(a *authCtx, campaignID int64, code string) (string, error) {
	mu := s.lockFor(a.OrgID, "scan-token/"+a.Env)
	mu.Lock()
	defer mu.Unlock()
	var token string
	err := s.db.QueryRow(`SELECT token FROM scan_tokens
	  WHERE campaign_id = ? AND code = ? AND revoked_at IS NULL`, campaignID, code).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	token, err = newScanToken()
	if err != nil {
		return "", err
	}
	if _, err = s.db.Exec(`INSERT INTO scan_tokens (token, org_id, env, campaign_id, code, created_at)
	  VALUES (?,?,?,?,?,?)`, token, a.OrgID, a.Env, campaignID, code, time.Now().Unix()); err != nil {
		return "", err
	}
	return token, nil
}

// resolveScanToken maps a token back to its campaign/code within an org+env.
func (s *server) resolveScanToken(a *authCtx, token string) (campaignID int64, code string, err error) {
	err = s.db.QueryRow(`SELECT campaign_id, code FROM scan_tokens
	  WHERE token = ? AND org_id = ? AND env = ? AND revoked_at IS NULL`,
		token, a.OrgID, a.Env).Scan(&campaignID, &code)
	return
}

// qrTarget picks which code of a campaign the QR points at: the requested one,
// or the campaign's shared code when omitted (the common "one QR for every
// bottle" case).
func (s *server) qrTarget(a *authCtx, r *http.Request) (campaign *campaignOut, code string, problem string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, "", "campaign not found"
	}
	c, err := s.campaignByID(a, id)
	if err != nil {
		return nil, "", "campaign not found"
	}
	requested := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if requested == "" {
		if c.SharedCode == "" {
			return nil, "", "this campaign has no shared code; pass ?code= for a specific code"
		}
		requested = c.SharedCode
	}
	// the code must belong to this campaign (shared or unique)
	var exists int
	if err := s.db.QueryRow(`SELECT EXISTS(
	    SELECT 1 FROM shared_codes WHERE campaign_id = ? AND code = ?
	    UNION ALL
	    SELECT 1 FROM codes WHERE campaign_id = ? AND code = ?)`,
		c.ID, requested, c.ID, requested).Scan(&exists); err != nil || exists == 0 {
		return nil, "", "code not found in this campaign"
	}
	return c, requested, ""
}

// handleCampaignQR returns the deep link plus its token, so an integrator can
// render the QR itself or print the link.
func (s *server) handleCampaignQR(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	number := s.waNumber()
	if number == "" {
		writeProblem(w, http.StatusNotImplemented,
			"no WhatsApp bot number is configured for this deployment (SOROTICKET_WA_NUMBER)")
		return
	}
	c, code, problem := s.qrTarget(a, r)
	if problem != "" {
		writeProblem(w, 404, problem)
		return
	}
	token, err := s.scanTokenFor(a, c.ID, code)
	if err != nil {
		writeInternal(w, err, "mint scan token")
		return
	}
	writeJSON(w, 200, map[string]any{
		"campaign_id": c.ID, "campaign_name": c.Name, "code": code,
		"wa_number": number, "scan_token": token, "deep_link": deepLink(number, token),
		"png_url": "/v1/campaigns/" + strconv.FormatInt(c.ID, 10) + "/qr.png?code=" + url.QueryEscape(code),
	})
}

// handleCampaignQRPNG renders the QR itself. Error correction level Q survives
// a scratched or curved label better than the usual default.
func (s *server) handleCampaignQRPNG(w http.ResponseWriter, r *http.Request) {
	a := authFrom(r)
	number := s.waNumber()
	if number == "" {
		writeProblem(w, http.StatusNotImplemented,
			"no WhatsApp bot number is configured for this deployment (SOROTICKET_WA_NUMBER)")
		return
	}
	c, code, problem := s.qrTarget(a, r)
	if problem != "" {
		writeProblem(w, 404, problem)
		return
	}
	token, err := s.scanTokenFor(a, c.ID, code)
	if err != nil {
		writeInternal(w, err, "mint scan token")
		return
	}
	png, err := qr.Encode(deepLink(number, token), qr.Q)
	if err != nil {
		writeInternal(w, err, "encode QR")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", `inline; filename="soroticket-`+strings.ToLower(code)+`.png"`)
	_, _ = w.Write(png.PNG())
}
