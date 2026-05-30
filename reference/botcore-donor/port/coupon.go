package port

import (
	"context"

	"github.com/fabianfarinas/botcore/internal/domain"
)

type CouponRepository interface {
	// Campaigns
	GetCampaignByID(ctx context.Context, id int64) (*domain.CouponCampaign, error)
	ListCampaignsByBot(ctx context.Context, botID int64) ([]domain.CouponCampaign, error)
	CreateCampaign(ctx context.Context, campaign *domain.CouponCampaign) error
	UpdateCampaignStatus(ctx context.Context, id int64, status domain.CampaignStatus) error

	// Codes
	GetCouponByCode(ctx context.Context, botID int64, code string) (*domain.CouponCode, error)
	CreateCouponCodes(ctx context.Context, codes []domain.CouponCode) error
	RedeemCoupon(ctx context.Context, codeID int64, redeemerPhone, redeemerName, referenceNumber string) error
	IncrementRedeemedCount(ctx context.Context, campaignID int64) error

	// Redeemers
	GetRedeemerByPhone(ctx context.Context, botID int64, phone string) (*domain.AuthorizedRedeemer, error)
	ListRedeemersByBot(ctx context.Context, botID int64) ([]domain.AuthorizedRedeemer, error)
	CreateRedeemer(ctx context.Context, redeemer *domain.AuthorizedRedeemer) error
	DeactivateRedeemer(ctx context.Context, id int64) error
}

// QRDecoder decodes QR codes from image data.
type QRDecoder interface {
	DecodeFromDataURI(dataURI string) (string, error)
}
