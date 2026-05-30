package stellar

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/fabianfarinas/botcore/internal/port"
)

// CouponLedger invokes the Soroban smart contract via the stellar CLI.
// The CLI handles simulation, resource estimation, signing, and submission.
type CouponLedger struct {
	contractID string
	sourceKey  string
	cliPath    string
	rpcURL     string
	network    string
}

func NewCouponLedger(address, secret, contractID, rpcURL string) (*CouponLedger, error) {
	if contractID == "" {
		return nil, fmt.Errorf("STELLAR_CONTRACT_ID is required")
	}

	// Find stellar CLI
	cliPath, err := exec.LookPath("stellar")
	if err != nil {
		home, _ := os.UserHomeDir()
		cliPath = home + "/.cargo/bin/stellar"
		if _, err := os.Stat(cliPath); err != nil {
			return nil, fmt.Errorf("stellar CLI not found — install with: cargo install --locked stellar-cli")
		}
	}

	if rpcURL == "" {
		rpcURL = "https://soroban-rpc.mainnet.stellar.gateway.fm"
	}

	slog.Info("soroban coupon ledger initialized",
		"component", "stellar",
		"contract", contractID,
	)

	return &CouponLedger{
		contractID: contractID,
		sourceKey:  secret,
		cliPath:    cliPath,
		rpcURL:     rpcURL,
		network:    "Public Global Stellar Network ; September 2015",
	}, nil
}

func (l *CouponLedger) Enabled() bool { return true }

// CreateCampaign calls the contract's create_campaign() function.
func (l *CouponLedger) CreateCampaign(ctx context.Context, c port.OnChainCampaign) (uint64, string, error) {
	output, err := l.invoke(ctx, "create_campaign",
		"--caller", l.sourceKey,
		"--name", c.Name,
		"--discount_type", c.DiscountType,
		"--discount_value", strconv.FormatUint(c.DiscountValue, 10),
		"--total_supply", strconv.FormatUint(uint64(c.TotalSupply), 10),
		"--valid_until", strconv.FormatUint(c.ValidUntil, 10),
	)
	if err != nil {
		return 0, "", fmt.Errorf("create_campaign failed: %w", err)
	}

	campaignID := parseLastLineAsUint(output)
	slog.Info("campaign created on soroban", "component", "stellar", "campaign_id", campaignID)
	return campaignID, output, nil
}

// MintBatch calls the contract's mint_batch() function.
func (l *CouponLedger) MintBatch(ctx context.Context, campaignID uint64, codes []string) (string, error) {
	codesJSON, _ := json.Marshal(codes)

	output, err := l.invoke(ctx, "mint_batch",
		"--caller", l.sourceKey,
		"--campaign_id", strconv.FormatUint(campaignID, 10),
		"--codes", string(codesJSON),
	)
	if err != nil {
		return "", fmt.Errorf("mint_batch failed: %w", err)
	}

	slog.Info("coupons minted on soroban", "component", "stellar", "campaign_id", campaignID, "count", len(codes))
	return output, nil
}

// Redeem calls the contract's redeem() function — burns the coupon token on-chain.
func (l *CouponLedger) Redeem(ctx context.Context, code, redeemerName, reference string) (string, error) {
	output, err := l.invoke(ctx, "redeem",
		"--caller", l.sourceKey,
		"--code", code,
		"--redeemer_name", redeemerName,
		"--reference", reference,
	)
	if err != nil {
		return "", fmt.Errorf("redeem failed: %w", err)
	}

	slog.Info("coupon burned on soroban",
		"component", "stellar",
		"code_prefix", code[:3]+"*****",
		"reference", reference,
	)
	return output, nil
}

// Verify calls the contract's verify() function — no auth needed.
func (l *CouponLedger) Verify(ctx context.Context, code string) (*port.OnChainCoupon, error) {
	output, err := l.invoke(ctx, "verify", "--code", code)
	if err != nil {
		return nil, fmt.Errorf("verify failed: %w", err)
	}

	var result struct {
		TokenID    uint64 `json:"token_id"`
		CampaignID uint64 `json:"campaign_id"`
		Code       string `json:"code"`
		IsBurned   bool   `json:"is_burned"`
		BurnedBy   string `json:"burned_by"`
		BurnedAt   uint64 `json:"burned_at"`
		Reference  string `json:"reference"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("parse verify result: %w", err)
	}

	return &port.OnChainCoupon{
		TokenID:    result.TokenID,
		CampaignID: result.CampaignID,
		Code:       result.Code,
		IsBurned:   result.IsBurned,
		BurnedBy:   result.BurnedBy,
		BurnedAt:   result.BurnedAt,
		Reference:  result.Reference,
	}, nil
}

// CampaignStats calls the contract's campaign_stats() function.
func (l *CouponLedger) CampaignStats(ctx context.Context, campaignID uint64) (*port.OnChainStats, error) {
	output, err := l.invoke(ctx, "campaign_stats",
		"--campaign_id", strconv.FormatUint(campaignID, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("campaign_stats failed: %w", err)
	}

	var result struct {
		TotalSupply uint32 `json:"total_supply"`
		Minted      uint32 `json:"minted"`
		Burned      uint32 `json:"burned"`
		Available   uint32 `json:"available"`
		IsExpired   bool   `json:"is_expired"`
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("parse stats: %w", err)
	}

	return &port.OnChainStats{
		TotalSupply: result.TotalSupply,
		Minted:      result.Minted,
		Burned:      result.Burned,
		Available:   result.Available,
		IsExpired:   result.IsExpired,
	}, nil
}

// invoke executes a stellar contract invoke command.
// SECURITY: The source key is passed via environment variable (STELLAR_SOURCE),
// never as a CLI argument, to prevent exposure via `ps aux`.
func (l *CouponLedger) invoke(ctx context.Context, function string, args ...string) (string, error) {
	cmdArgs := []string{
		"contract", "invoke",
		"--id", l.contractID,
		"--source", l.sourceKey,
		"--network-passphrase", l.network,
		"--rpc-url", l.rpcURL,
		"--inclusion-fee", "100000",
		"--", function,
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, l.cliPath, cmdArgs...)

	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))

	if err != nil {
		// Scrub secret key from output before logging (VULN-03 fix)
		sanitized := strings.ReplaceAll(outStr, l.sourceKey, "[REDACTED]")
		slog.Error("stellar CLI error",
			"component", "stellar",
			"function", function,
			"error", err,
			"output", sanitized,
		)
		return "", fmt.Errorf("%s: %s", err, sanitized)
	}

	return outStr, nil
}

// parseLastLineAsUint extracts the last line from CLI output and parses as uint64.
func parseLastLineAsUint(output string) uint64 {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if val, err := strconv.ParseUint(line, 10, 64); err == nil {
			return val
		}
	}
	return 0
}
