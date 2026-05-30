package domain

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

type CouponStatus string

const (
	CouponAvailable CouponStatus = "available"
	CouponRedeemed  CouponStatus = "redeemed"
	CouponExpired   CouponStatus = "expired"
	CouponCancelled CouponStatus = "cancelled"
)

type CampaignStatus string

const (
	CampaignActive    CampaignStatus = "active"
	CampaignPaused    CampaignStatus = "paused"
	CampaignExpired   CampaignStatus = "expired"
	CampaignExhausted CampaignStatus = "exhausted"
)

type DiscountType string

const (
	DiscountPercentage  DiscountType = "percentage"
	DiscountFixedAmount DiscountType = "fixed_amount"
	DiscountFreeItem    DiscountType = "free_item"
	DiscountCustom      DiscountType = "custom"
)

type CouponCampaign struct {
	ID              int64
	BotID           int64
	Name            string
	Description     string
	DiscountType    DiscountType
	DiscountValue   float64
	TotalQuantity   int
	RedeemedCount   int
	ValidFrom       time.Time
	ValidUntil      time.Time
	TermsConditions string
	Status          CampaignStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsValid checks if a campaign is currently valid for redemptions.
func (c *CouponCampaign) IsValid() (bool, string) {
	now := time.Now()
	if c.Status != CampaignActive {
		return false, fmt.Sprintf("la campaña está %s", c.Status)
	}
	if now.Before(c.ValidFrom) {
		return false, fmt.Sprintf("la campaña aún no inicia (comienza el %s)", c.ValidFrom.Format("02/01/2006"))
	}
	if now.After(c.ValidUntil) {
		return false, fmt.Sprintf("la campaña venció el %s", c.ValidUntil.Format("02/01/2006"))
	}
	if c.RedeemedCount >= c.TotalQuantity {
		return false, "todos los cupones de esta campaña han sido canjeados"
	}
	return true, ""
}

// DiscountLabel returns a human-readable discount description.
func (c *CouponCampaign) DiscountLabel() string {
	switch c.DiscountType {
	case DiscountPercentage:
		return fmt.Sprintf("%.0f%% de descuento", c.DiscountValue)
	case DiscountFixedAmount:
		return fmt.Sprintf("$%.2f de descuento", c.DiscountValue)
	case DiscountFreeItem:
		return "artículo gratis"
	default:
		return c.Description
	}
}

type CouponCode struct {
	ID              int64
	CampaignID      int64
	BotID           int64
	Code            string
	QRPayload       string
	Status          CouponStatus
	RedeemedByPhone string
	RedeemedByName  string
	ReferenceNumber string
	RedeemedAt      *time.Time
	CreatedAt       time.Time
}

type AuthorizedRedeemer struct {
	ID        int64
	BotID     int64
	Phone     string
	Name      string
	IsActive  bool
	CreatedAt time.Time
}

// GenerateCouponCode creates a cryptographically random 8-character alphanumeric code.
func GenerateCouponCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I to avoid confusion
	code := make([]byte, 8)
	for i := range code {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		code[i] = charset[n.Int64()]
	}
	return string(code)
}

// GenerateReferenceNumber creates a unique reference for a redemption.
func GenerateReferenceNumber() string {
	now := time.Now()
	suffix := make([]byte, 4)
	const digits = "0123456789"
	for i := range suffix {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		suffix[i] = digits[n.Int64()]
	}
	return fmt.Sprintf("REF-%s-%s", now.Format("20060102"), string(suffix))
}

// QRPayloadPrefix is the prefix used in QR codes to identify BotCore coupons.
const QRPayloadPrefix = "BOTCORE:"

// MakeQRPayload creates a wa.me deep link for the QR code.
// When scanned, it opens WhatsApp and pre-fills the coupon code as a message.
// The phone's native QR scanner handles decoding — much more reliable than photo-based.
// botPhone should be the bot's WhatsApp number without + (e.g., "19713902577").
func MakeQRPayload(code, botPhone string) string {
	if botPhone != "" {
		phone := strings.TrimPrefix(botPhone, "+")
		return fmt.Sprintf("https://wa.me/%s?text=%s%s", phone, QRPayloadPrefix, code)
	}
	return QRPayloadPrefix + code
}

// ParseQRPayload extracts the coupon code from a QR payload or text message.
// Handles both formats:
//   - "BOTCORE:PK486CZQ" (direct prefix)
//   - "https://wa.me/19713902577?text=BOTCORE:PK486CZQ" (wa.me link)
// Returns the code and true if valid, empty string and false otherwise.
func ParseQRPayload(payload string) (string, bool) {
	// Check for wa.me URL format
	if strings.Contains(payload, "wa.me/") {
		if idx := strings.Index(payload, QRPayloadPrefix); idx >= 0 {
			return payload[idx+len(QRPayloadPrefix):], true
		}
	}
	// Check for direct prefix
	if len(payload) > len(QRPayloadPrefix) && payload[:len(QRPayloadPrefix)] == QRPayloadPrefix {
		return payload[len(QRPayloadPrefix):], true
	}
	return "", false
}
