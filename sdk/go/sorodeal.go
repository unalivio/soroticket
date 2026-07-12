package sorodeal

import (
	"context"
	"fmt"
	"math/big"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// ═══════════════════════════════════════════════════════════════════
// Domain types (mirror contracts/coupon-ledger/src/lib.rs).
// ═══════════════════════════════════════════════════════════════════

// Campaign is a collection of unique coupon codes with shared terms.
type Campaign struct {
	ID            uint64
	Owner         string
	Name          string
	DiscountType  string
	DiscountValue uint64
	TotalSupply   uint32
	Minted        uint32
	Burned        uint32
	ValidUntil    uint64
}

// CouponToken is an individual unique single-use coupon (Burn profile).
type CouponToken struct {
	TokenID     uint64
	CampaignID  uint64
	Code        string
	IsBurned    bool
	MintedAt    uint64
	RedeemerRef []byte // 32-byte opaque commitment; all-zero until burned
	BurnedAt    uint64
}

// RedemptionReceipt is returned after a successful burn.
type RedemptionReceipt struct {
	TokenID       uint64
	Code          string
	CampaignID    uint64
	CampaignName  string
	DiscountType  string
	DiscountValue uint64
	RedeemerRef   []byte
	BurnedAt      uint64
	LedgerSeq     uint32
}

// CampaignStats is a dashboard summary.
type CampaignStats struct {
	TotalSupply uint32
	Minted      uint32
	Burned      uint32
	Available   uint32
	IsExpired   bool
}

// SharedCode is a registered multi-use code (Tally profile). AttributedTo and
// PayoutToken are nil when unset (Option<Address> = None).
type SharedCode struct {
	CampaignID   uint64
	Code         string
	AttributedTo *string
	PayoutToken  *string
	PayoutRate   *big.Int
	RegisteredAt uint64
}

// TallyCommitment is a per-period on-chain commitment of off-chain redemptions.
type TallyCommitment struct {
	Period         uint64
	Count          uint32
	MerkleRoot     []byte
	PerAttribution map[string]uint32
}

// Payout is a settlement amount to an attributed address.
type Payout struct {
	To     string
	Amount *big.Int
}

// ═══════════════════════════════════════════════════════════════════
// Campaigns
// ═══════════════════════════════════════════════════════════════════

// CreateCampaign creates a campaign owned by the signer and returns its id.
func (c *Client) CreateCampaign(ctx context.Context, name, discountType string, discountValue uint64, totalSupply uint32, validUntil uint64) (uint64, error) {
	owner, err := scAddress(c.Address())
	if err != nil {
		return 0, err
	}
	v, err := c.invoke(ctx, "create_campaign", []xdr.ScVal{
		owner, scString(name), scString(discountType), scU64(discountValue), scU32(totalSupply), scU64(validUntil),
	})
	if err != nil {
		return 0, err
	}
	n, err := native(v)
	if err != nil {
		return 0, err
	}
	id, _ := n.(uint64)
	return id, nil
}

// GetCampaign reads a campaign by id (public).
func (c *Client) GetCampaign(ctx context.Context, id uint64) (Campaign, error) {
	v, err := c.read(ctx, "get_campaign", []xdr.ScVal{scU64(id)})
	if err != nil {
		return Campaign{}, err
	}
	m, err := nativeMap(v)
	if err != nil {
		return Campaign{}, err
	}
	return Campaign{
		ID:            asU64(m, "id"),
		Owner:         asString(m, "owner"),
		Name:          asString(m, "name"),
		DiscountType:  asString(m, "discount_type"),
		DiscountValue: asU64(m, "discount_value"),
		TotalSupply:   asU32(m, "total_supply"),
		Minted:        asU32(m, "minted"),
		Burned:        asU32(m, "burned"),
		ValidUntil:    asU64(m, "valid_until"),
	}, nil
}

// CampaignsOf lists the campaign ids created by owner (public).
func (c *Client) CampaignsOf(ctx context.Context, owner string) ([]uint64, error) {
	addr, err := scAddress(owner)
	if err != nil {
		return nil, err
	}
	v, err := c.read(ctx, "campaigns_of", []xdr.ScVal{addr})
	if err != nil {
		return nil, err
	}
	n, err := native(v)
	if err != nil {
		return nil, err
	}
	items, _ := n.([]interface{})
	out := make([]uint64, 0, len(items))
	for _, it := range items {
		if u, ok := it.(uint64); ok {
			out = append(out, u)
		}
	}
	return out, nil
}

// CampaignsPage returns a bounded page of an owner's campaign ids. Cursor is a
// zero-based slot; limit must be between 1 and 100 on contract v0.2+.
func (c *Client) CampaignsPage(ctx context.Context, owner string, cursor uint64, limit uint32) ([]uint64, error) {
	addr, err := scAddress(owner)
	if err != nil {
		return nil, err
	}
	v, err := c.read(ctx, "campaigns_page", []xdr.ScVal{addr, scU64(cursor), scU32(limit)})
	if err != nil {
		return nil, err
	}
	n, err := native(v)
	if err != nil {
		return nil, err
	}
	items, _ := n.([]interface{})
	out := make([]uint64, 0, len(items))
	for _, item := range items {
		if id, ok := item.(uint64); ok {
			out = append(out, id)
		}
	}
	return out, nil
}

// CampaignStats reads campaign statistics (public).
func (c *Client) CampaignStats(ctx context.Context, id uint64) (CampaignStats, error) {
	v, err := c.read(ctx, "campaign_stats", []xdr.ScVal{scU64(id)})
	if err != nil {
		return CampaignStats{}, err
	}
	m, err := nativeMap(v)
	if err != nil {
		return CampaignStats{}, err
	}
	return CampaignStats{
		TotalSupply: asU32(m, "total_supply"),
		Minted:      asU32(m, "minted"),
		Burned:      asU32(m, "burned"),
		Available:   asU32(m, "available"),
		IsExpired:   asBool(m, "is_expired"),
	}, nil
}

// BumpCampaign re-extends a campaign's storage TTL (public, signer pays rent).
func (c *Client) BumpCampaign(ctx context.Context, id uint64) error {
	_, err := c.invoke(ctx, "bump_campaign", []xdr.ScVal{scU64(id)})
	return err
}

// BumpCodes re-extends specific coupons' storage TTL (public). Unknown codes are skipped.
func (c *Client) BumpCodes(ctx context.Context, id uint64, codes []string) error {
	_, err := c.invoke(ctx, "bump_codes", []xdr.ScVal{scU64(id), strVec(codes)})
	return err
}

// BumpDelegates re-extends specific delegate authorizations (public). Unknown
// addresses are skipped by the contract.
func (c *Client) BumpDelegates(ctx context.Context, id uint64, delegates []string) error {
	items := make([]xdr.ScVal, 0, len(delegates))
	for _, delegate := range delegates {
		addr, err := scAddress(delegate)
		if err != nil {
			return err
		}
		items = append(items, addr)
	}
	_, err := c.invoke(ctx, "bump_delegates", []xdr.ScVal{scU64(id), scVec(items)})
	return err
}

// ═══════════════════════════════════════════════════════════════════
// Delegates
// ═══════════════════════════════════════════════════════════════════

// AddDelegate authorizes delegate to redeem coupons of a campaign (owner only).
func (c *Client) AddDelegate(ctx context.Context, id uint64, delegate string) error {
	owner, err := scAddress(c.Address())
	if err != nil {
		return err
	}
	d, err := scAddress(delegate)
	if err != nil {
		return err
	}
	_, err = c.invoke(ctx, "add_delegate", []xdr.ScVal{owner, scU64(id), d})
	return err
}

// RemoveDelegate revokes a delegate (owner only).
func (c *Client) RemoveDelegate(ctx context.Context, id uint64, delegate string) error {
	owner, err := scAddress(c.Address())
	if err != nil {
		return err
	}
	d, err := scAddress(delegate)
	if err != nil {
		return err
	}
	_, err = c.invoke(ctx, "remove_delegate", []xdr.ScVal{owner, scU64(id), d})
	return err
}

// IsDelegate reports whether who is an authorized delegate (public).
func (c *Client) IsDelegate(ctx context.Context, id uint64, who string) (bool, error) {
	addr, err := scAddress(who)
	if err != nil {
		return false, err
	}
	v, err := c.read(ctx, "is_delegate", []xdr.ScVal{scU64(id), addr})
	if err != nil {
		return false, err
	}
	n, err := native(v)
	if err != nil {
		return false, err
	}
	b, _ := n.(bool)
	return b, nil
}

// ═══════════════════════════════════════════════════════════════════
// Issuance & redemption (Burn)
// ═══════════════════════════════════════════════════════════════════

// IssueUnique issues a batch of unique codes under a campaign (owner only).
func (c *Client) IssueUnique(ctx context.Context, id uint64, codes []string) ([]uint64, error) {
	owner, err := scAddress(c.Address())
	if err != nil {
		return nil, err
	}
	v, err := c.invoke(ctx, "issue_unique", []xdr.ScVal{owner, scU64(id), strVec(codes)})
	if err != nil {
		return nil, err
	}
	n, err := native(v)
	if err != nil {
		return nil, err
	}
	items, _ := n.([]interface{})
	out := make([]uint64, 0, len(items))
	for _, it := range items {
		if u, ok := it.(uint64); ok {
			out = append(out, u)
		}
	}
	return out, nil
}

// RedeemUnique burns a unique coupon (authorized by owner or a delegate — the
// signer). redeemerRefHash is the opaque off-chain commitment (no PII on-chain).
func (c *Client) RedeemUnique(ctx context.Context, id uint64, code string, redeemerRefHash [32]byte) (RedemptionReceipt, error) {
	authorizer, err := scAddress(c.Address())
	if err != nil {
		return RedemptionReceipt{}, err
	}
	v, err := c.invoke(ctx, "redeem_unique", []xdr.ScVal{authorizer, scU64(id), scString(code), scBytes(redeemerRefHash[:])})
	if err != nil {
		return RedemptionReceipt{}, err
	}
	m, err := nativeMap(v)
	if err != nil {
		return RedemptionReceipt{}, err
	}
	return RedemptionReceipt{
		TokenID:       asU64(m, "token_id"),
		Code:          asString(m, "code"),
		CampaignID:    asU64(m, "campaign_id"),
		CampaignName:  asString(m, "campaign_name"),
		DiscountType:  asString(m, "discount_type"),
		DiscountValue: asU64(m, "discount_value"),
		RedeemerRef:   asBytes(m, "redeemer_ref"),
		BurnedAt:      asU64(m, "burned_at"),
		LedgerSeq:     asU32(m, "ledger_seq"),
	}, nil
}

// Verify reads a coupon's token by (campaign_id, code) (public). Returns
// *ContractError with CouponNotFound when absent.
func (c *Client) Verify(ctx context.Context, id uint64, code string) (CouponToken, error) {
	v, err := c.read(ctx, "verify", []xdr.ScVal{scU64(id), scString(code)})
	if err != nil {
		return CouponToken{}, err
	}
	m, err := nativeMap(v)
	if err != nil {
		return CouponToken{}, err
	}
	return CouponToken{
		TokenID:     asU64(m, "token_id"),
		CampaignID:  asU64(m, "campaign_id"),
		Code:        asString(m, "code"),
		IsBurned:    asBool(m, "is_burned"),
		MintedAt:    asU64(m, "minted_at"),
		RedeemerRef: asBytes(m, "redeemer_ref"),
		BurnedAt:    asU64(m, "burned_at"),
	}, nil
}

// IsValid reports whether (campaign_id, code) is valid and redeemable (public).
func (c *Client) IsValid(ctx context.Context, id uint64, code string) (bool, error) {
	v, err := c.read(ctx, "is_valid", []xdr.ScVal{scU64(id), scString(code)})
	if err != nil {
		return false, err
	}
	n, err := native(v)
	if err != nil {
		return false, err
	}
	b, _ := n.(bool)
	return b, nil
}

// TotalCampaigns returns the global campaign count (public).
func (c *Client) TotalCampaigns(ctx context.Context) (uint64, error) {
	return c.readU64(ctx, "total_campaigns")
}

// TotalMinted returns the global minted-coupon count (public).
func (c *Client) TotalMinted(ctx context.Context) (uint64, error) {
	return c.readU64(ctx, "total_minted")
}

func (c *Client) readU64(ctx context.Context, method string) (uint64, error) {
	v, err := c.read(ctx, method, nil)
	if err != nil {
		return 0, err
	}
	n, err := native(v)
	if err != nil {
		return 0, err
	}
	u, _ := n.(uint64)
	return u, nil
}

// ═══════════════════════════════════════════════════════════════════
// Tally (shared codes) + settlement
// ═══════════════════════════════════════════════════════════════════

// RegisterShared registers a shared code (owner only). attributedTo/payoutToken
// are nil to omit; payoutRate must be > 0 iff payoutToken is set, and 0 otherwise.
func (c *Client) RegisterShared(ctx context.Context, id uint64, code string, attributedTo, payoutToken *string, payoutRate *big.Int) error {
	owner, err := scAddress(c.Address())
	if err != nil {
		return err
	}
	attr, err := scOptAddress(attributedTo)
	if err != nil {
		return err
	}
	tok, err := scOptAddress(payoutToken)
	if err != nil {
		return err
	}
	if payoutRate == nil {
		payoutRate = big.NewInt(0)
	}
	rate, err := scI128(payoutRate)
	if err != nil {
		return err
	}
	_, err = c.invoke(ctx, "register_shared", []xdr.ScVal{owner, scU64(id), scString(code), attr, tok, rate})
	return err
}

// GetShared reads a registered shared code (public).
func (c *Client) GetShared(ctx context.Context, id uint64, code string) (SharedCode, error) {
	v, err := c.read(ctx, "get_shared", []xdr.ScVal{scU64(id), scString(code)})
	if err != nil {
		return SharedCode{}, err
	}
	m, err := nativeMap(v)
	if err != nil {
		return SharedCode{}, err
	}
	return SharedCode{
		CampaignID:   asU64(m, "campaign_id"),
		Code:         asString(m, "code"),
		AttributedTo: asOptString(m, "attributed_to"),
		PayoutToken:  asOptString(m, "payout_token"),
		PayoutRate:   asBig(m, "payout_rate"),
		RegisteredAt: asU64(m, "registered_at"),
	}, nil
}

// CommitTally commits an epoch's off-chain tally (owner only).
func (c *Client) CommitTally(ctx context.Context, id uint64, code string, period uint64, count uint32, merkleRoot [32]byte, perAttribution map[string]uint32) error {
	owner, err := scAddress(c.Address())
	if err != nil {
		return err
	}
	attr, err := scAttributionMap(perAttribution)
	if err != nil {
		return err
	}
	_, err = c.invoke(ctx, "commit_tally", []xdr.ScVal{
		owner, scU64(id), scString(code), scU64(period), scU32(count), scBytes(merkleRoot[:]), attr,
	})
	return err
}

// GetTally reads a committed tally for (code, period) (public).
func (c *Client) GetTally(ctx context.Context, id uint64, code string, period uint64) (TallyCommitment, error) {
	v, err := c.read(ctx, "get_tally", []xdr.ScVal{scU64(id), scString(code), scU64(period)})
	if err != nil {
		return TallyCommitment{}, err
	}
	m, err := nativeMap(v)
	if err != nil {
		return TallyCommitment{}, err
	}
	return TallyCommitment{
		Period:         asU64(m, "period"),
		Count:          asU32(m, "count"),
		MerkleRoot:     asBytes(m, "merkle_root"),
		PerAttribution: attributionFromMap(m["per_attribution"]),
	}, nil
}

// IsSettled reports whether a tally period has already been paid (public).
func (c *Client) IsSettled(ctx context.Context, id uint64, code string, period uint64) (bool, error) {
	v, err := c.read(ctx, "is_settled", []xdr.ScVal{scU64(id), scString(code), scU64(period)})
	if err != nil {
		return false, err
	}
	n, err := native(v)
	if err != nil {
		return false, err
	}
	b, _ := n.(bool)
	return b, nil
}

// ComputePayouts previews payouts for a committed tally (public, no transfer).
func (c *Client) ComputePayouts(ctx context.Context, id uint64, code string, period uint64) ([]Payout, error) {
	v, err := c.read(ctx, "compute_payouts", []xdr.ScVal{scU64(id), scString(code), scU64(period)})
	if err != nil {
		return nil, err
	}
	return payoutsFromScVal(v)
}

// Settle pays each attributed address from the signer's balance. The signer is
// used as both campaign owner and fee payer. On contract v0.2+, settlement
// consumes the allowance previously granted to the Sorodeal contract.
func (c *Client) Settle(ctx context.Context, id uint64, code string, period uint64) ([]Payout, error) {
	return c.SettleFor(ctx, c.Address(), id, code, period)
}

// SettleFor triggers a campaign owner's settlement while the client's signer
// only pays the transaction fee. This enables permissionless keepers on
// contract v0.2+; the owner must first call ApproveSettlement.
func (c *Client) SettleFor(ctx context.Context, ownerAddress string, id uint64, code string, period uint64) ([]Payout, error) {
	owner, err := scAddress(ownerAddress)
	if err != nil {
		return nil, err
	}
	v, err := c.invoke(ctx, "settle", []xdr.ScVal{owner, scU64(id), scString(code), scU64(period)})
	if err != nil {
		return nil, err
	}
	return payoutsFromScVal(v)
}

// ApproveSettlement grants the Sorodeal contract an allowance on payoutToken,
// enabling permissionless settlements up to amount until expirationLedger.
func (c *Client) ApproveSettlement(ctx context.Context, payoutToken string, amount *big.Int, expirationLedger uint32) error {
	from, err := scAddress(c.Address())
	if err != nil {
		return err
	}
	spender, err := scAddress(c.cfg.ContractID)
	if err != nil {
		return err
	}
	encodedAmount, err := scI128(amount)
	if err != nil {
		return err
	}
	tokenID, err := parseContractID(payoutToken)
	if err != nil {
		return fmt.Errorf("invalid payout token contract %q: %w", payoutToken, err)
	}
	_, err = c.invokeAt(ctx, tokenID, "approve", []xdr.ScVal{
		from, spender, encodedAmount, scU32(expirationLedger),
	})
	return err
}

// SettlementAllowance returns the owner's token allowance granted to the
// Sorodeal contract.
func (c *Client) SettlementAllowance(ctx context.Context, payoutToken, ownerAddress string) (*big.Int, error) {
	owner, err := scAddress(ownerAddress)
	if err != nil {
		return nil, err
	}
	spender, err := scAddress(c.cfg.ContractID)
	if err != nil {
		return nil, err
	}
	tokenID, err := parseContractID(payoutToken)
	if err != nil {
		return nil, fmt.Errorf("invalid payout token contract %q: %w", payoutToken, err)
	}
	v, err := c.readAt(ctx, tokenID, "allowance", []xdr.ScVal{owner, spender})
	if err != nil {
		return nil, err
	}
	n, err := native(v)
	if err != nil {
		return nil, err
	}
	amount, ok := n.(*big.Int)
	if !ok {
		return nil, fmt.Errorf("expected i128 allowance, got %T", n)
	}
	return amount, nil
}

// BumpTally re-extends a shared code and the given periods' storage TTL (public).
func (c *Client) BumpTally(ctx context.Context, id uint64, code string, periods []uint64) error {
	items := make([]xdr.ScVal, 0, len(periods))
	for _, p := range periods {
		items = append(items, scU64(p))
	}
	_, err := c.invoke(ctx, "bump_tally", []xdr.ScVal{scU64(id), scString(code), scVec(items)})
	return err
}

// ─── small helpers ───────────────────────────────────────────────────

func strVec(codes []string) xdr.ScVal {
	items := make([]xdr.ScVal, 0, len(codes))
	for _, s := range codes {
		items = append(items, scString(s))
	}
	return scVec(items)
}

func attributionFromMap(v interface{}) map[string]uint32 {
	out := map[string]uint32{}
	m, ok := v.(map[string]interface{})
	if !ok {
		return out
	}
	for k, val := range m {
		switch x := val.(type) {
		case uint32:
			out[k] = x
		case uint64:
			out[k] = uint32(x)
		}
	}
	return out
}

func payoutsFromScVal(v xdr.ScVal) ([]Payout, error) {
	n, err := native(v)
	if err != nil {
		return nil, err
	}
	items, ok := n.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected a vec of payouts, got %T", n)
	}
	out := make([]Payout, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, Payout{To: asString(m, "to"), Amount: asBig(m, "amount")})
	}
	return out, nil
}
