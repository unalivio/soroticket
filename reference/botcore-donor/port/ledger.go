package port

import "context"

// CouponLedger is the on-chain source of truth for all coupon operations.
// Soroban smart contract handles: campaigns, minting, redemption, verification.
// PostgreSQL is only a read cache.
type CouponLedger interface {
	// CreateCampaign registers a campaign on-chain. Returns campaign ID from contract.
	CreateCampaign(ctx context.Context, campaign OnChainCampaign) (campaignID uint64, txHash string, err error)

	// MintBatch mints coupon tokens on-chain. Returns token IDs from contract.
	MintBatch(ctx context.Context, campaignID uint64, codes []string) (txHash string, err error)

	// Redeem burns a coupon token on-chain. Returns the tx hash as proof.
	Redeem(ctx context.Context, code, redeemerName, reference string) (txHash string, err error)

	// Verify checks a coupon's status on-chain. Anyone can call this.
	Verify(ctx context.Context, code string) (*OnChainCoupon, error)

	// CampaignStats returns on-chain stats for a campaign.
	CampaignStats(ctx context.Context, campaignID uint64) (*OnChainStats, error)

	// Enabled returns true if the ledger is configured and operational.
	Enabled() bool
}

type OnChainCampaign struct {
	Name          string
	DiscountType  string
	DiscountValue uint64 // cents (e.g., 2000 = 20.00 or 20%)
	TotalSupply   uint32
	ValidUntil    uint64 // unix timestamp
}

type OnChainCoupon struct {
	TokenID    uint64
	CampaignID uint64
	Code       string
	IsBurned   bool
	BurnedBy   string
	BurnedAt   uint64
	Reference  string
}

type OnChainStats struct {
	TotalSupply uint32
	Minted      uint32
	Burned      uint32
	Available   uint32
	IsExpired   bool
}

// NoopLedger is a no-op implementation when Stellar is not configured.
type NoopLedger struct{}

func (n *NoopLedger) CreateCampaign(_ context.Context, _ OnChainCampaign) (uint64, string, error) {
	return 0, "", nil
}
func (n *NoopLedger) MintBatch(_ context.Context, _ uint64, _ []string) (string, error) {
	return "", nil
}
func (n *NoopLedger) Redeem(_ context.Context, _, _, _ string) (string, error) {
	return "", nil
}
func (n *NoopLedger) Verify(_ context.Context, _ string) (*OnChainCoupon, error) {
	return nil, nil
}
func (n *NoopLedger) CampaignStats(_ context.Context, _ uint64) (*OnChainStats, error) {
	return nil, nil
}
func (n *NoopLedger) Enabled() bool { return false }
