// Command e2e is a consumer tester app for the Sorodeal Go SDK
// (github.com/sorodeal/sorodeal-go). It generates and funds ephemeral testnet
// accounts and exercises every contract path against the LIVE testnet
// deployment: both coupon profiles (Burn + Tally), every discount variant,
// the permissioned actions (owner / delegate / stranger), the settlement flow
// with a real token transfer, and every documented error (codes #1–#19).
//
// Exit code 0 ⇒ all scenarios passed.
//
//	go run .            # uses testnet defaults
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"

	sd "github.com/sorodeal/sorodeal-go"
)

// nativeSAC is the testnet native-XLM Stellar Asset Contract — the settlement
// token for the Tally payout scenario (see deployments/testnet.json).
const nativeSAC = "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"

func main() {
	ctx := context.Background()
	h := &harness{}

	fmt.Println("Sorodeal Go SDK — end-to-end against testnet")
	fmt.Println("contract:", sd.TestnetContractID)
	fmt.Println()

	// ── actors ──────────────────────────────────────────────────────
	owner := randKP()
	delegate := randKP()
	stranger := randKP()
	creator := randKP()

	fmt.Println("funding 4 ephemeral accounts via friendbot…")
	if err := fundAll(owner, delegate, stranger, creator); err != nil {
		fmt.Println("FATAL: funding failed:", err)
		os.Exit(1)
	}
	fmt.Println("  owner   ", owner.Address())
	fmt.Println("  delegate", delegate.Address())
	fmt.Println("  stranger", stranger.Address())
	fmt.Println("  creator ", creator.Address())
	fmt.Println()

	mk := func(kp *keypair.Full) *sd.Client {
		c, err := sd.Testnet(kp)
		if err != nil {
			panic(err)
		}
		return c
	}
	ownerC, delegateC, strangerC, creatorC := mk(owner), mk(delegate), mk(stranger), mk(creator)
	pub, _ := sd.New(sd.Config{}) // read-only

	// ════════════════════════════════════════════════════════════════
	// EXPIRY (kicked off early; asserted late after it lapses)
	// ════════════════════════════════════════════════════════════════
	var expID uint64
	expValidUntil := uint64(time.Now().Unix()) + 30
	h.step("setup: short-lived campaign for expiry checks", func() error {
		var err error
		expID, err = ownerC.CreateCampaign(ctx, "Expiring", "percentage", 1000, 5, expValidUntil)
		if err != nil {
			return err
		}
		_, err = ownerC.IssueUnique(ctx, expID, []string{"EXP00001"})
		return err
	})

	// ════════════════════════════════════════════════════════════════
	// BURN PROFILE — happy path + variants
	// ════════════════════════════════════════════════════════════════
	var campPct uint64
	h.step("create_campaign (percentage)", func() error {
		var err error
		campPct, err = ownerC.CreateCampaign(ctx, "Cafe Promo", "percentage", 1500, 100, farFuture())
		if err != nil {
			return err
		}
		return assert(campPct > 0, "campaign id should be > 0")
	})

	h.step("get_campaign round-trips fields", func() error {
		c, err := pub.GetCampaign(ctx, campPct)
		if err != nil {
			return err
		}
		return all(
			assert(c.Owner == owner.Address(), "owner mismatch"),
			assert(c.Name == "Cafe Promo", "name mismatch"),
			assert(c.DiscountType == "percentage", "discount_type mismatch"),
			assert(c.DiscountValue == 1500, "discount_value mismatch"),
			assert(c.TotalSupply == 100, "total_supply mismatch"),
		)
	})

	h.step("campaigns_of(owner) includes new campaign", func() error {
		ids, err := pub.CampaignsOf(ctx, owner.Address())
		if err != nil {
			return err
		}
		return assert(contains(ids, campPct) && contains(ids, expID), "owner index missing campaigns")
	})

	h.step("issue_unique batch", func() error {
		ids, err := ownerC.IssueUnique(ctx, campPct, []string{"BURN0001", "BURN0002", "BURN0003"})
		if err != nil {
			return err
		}
		return assert(len(ids) == 3, "expected 3 token ids")
	})

	h.step("is_valid true for issued code", func() error {
		ok, err := pub.IsValid(ctx, campPct, "BURN0001")
		if err != nil {
			return err
		}
		return assert(ok, "BURN0001 should be valid")
	})

	h.step("redeem_unique (owner) success + receipt", func() error {
		r, err := ownerC.RedeemUnique(ctx, campPct, "BURN0001", refHash("order-1"))
		if err != nil {
			return err
		}
		return all(
			assert(r.CampaignID == campPct, "receipt campaign id"),
			assert(r.DiscountValue == 1500, "receipt discount value"),
			assert(r.Code == "BURN0001", "receipt code"),
			assert(r.LedgerSeq > 0, "receipt ledger seq"),
		)
	})

	h.step("verify shows burned; is_valid now false", func() error {
		tok, err := pub.Verify(ctx, campPct, "BURN0001")
		if err != nil {
			return err
		}
		ok, err := pub.IsValid(ctx, campPct, "BURN0001")
		if err != nil {
			return err
		}
		return all(assert(tok.IsBurned, "should be burned"), assert(!ok, "should be invalid after burn"))
	})

	h.step("campaign_stats reflects 1 burn", func() error {
		s, err := pub.CampaignStats(ctx, campPct)
		if err != nil {
			return err
		}
		return all(assert(s.Minted == 3, "minted"), assert(s.Burned == 1, "burned"), assert(!s.IsExpired, "not expired"))
	})

	h.step("redeem_unique double → #3 AlreadyRedeemed", func() error {
		_, err := ownerC.RedeemUnique(ctx, campPct, "BURN0001", refHash("order-1b"))
		return wantCode(err, sd.ErrAlreadyRedeemed)
	})

	h.step("redeem_unique unknown code → #2 CouponNotFound", func() error {
		_, err := ownerC.RedeemUnique(ctx, campPct, "NOSUCH", refHash("x"))
		return wantCode(err, sd.ErrCouponNotFound)
	})

	h.step("redeem_unique by stranger → #6 Unauthorized", func() error {
		_, err := strangerC.RedeemUnique(ctx, campPct, "BURN0002", refHash("x"))
		return wantCode(err, sd.ErrUnauthorized)
	})

	// ── discount variants ────────────────────────────────────────────
	h.step("variant: fixed_amount campaign create+redeem", func() error {
		id, err := ownerC.CreateCampaign(ctx, "Fixed 5 off", "fixed_amount", 500, 10, farFuture())
		if err != nil {
			return err
		}
		if _, err := ownerC.IssueUnique(ctx, id, []string{"FIX00001"}); err != nil {
			return err
		}
		r, err := ownerC.RedeemUnique(ctx, id, "FIX00001", refHash("fix"))
		if err != nil {
			return err
		}
		return assert(r.DiscountType == "fixed_amount" && r.DiscountValue == 500, "fixed_amount receipt")
	})

	h.step("variant: free_item campaign create+redeem", func() error {
		id, err := ownerC.CreateCampaign(ctx, "Free coffee", "free_item", 0, 10, farFuture())
		if err != nil {
			return err
		}
		if _, err := ownerC.IssueUnique(ctx, id, []string{"FREE0001"}); err != nil {
			return err
		}
		r, err := ownerC.RedeemUnique(ctx, id, "FREE0001", refHash("free"))
		if err != nil {
			return err
		}
		return assert(r.DiscountType == "free_item", "free_item receipt")
	})

	// ── create_campaign validation (#9) ──────────────────────────────
	h.step("create_campaign supply=0 → #9 InvalidTerms", func() error {
		_, err := ownerC.CreateCampaign(ctx, "Bad", "percentage", 1000, 0, farFuture())
		return wantCode(err, sd.ErrInvalidTerms)
	})
	h.step("create_campaign empty discount_type → #9 InvalidTerms", func() error {
		_, err := ownerC.CreateCampaign(ctx, "Bad", "", 1000, 10, farFuture())
		return wantCode(err, sd.ErrInvalidTerms)
	})
	h.step("create_campaign valid_until in past → #9 InvalidTerms", func() error {
		_, err := ownerC.CreateCampaign(ctx, "Bad", "percentage", 1000, 10, 1)
		return wantCode(err, sd.ErrInvalidTerms)
	})

	// ── issue_unique validation ──────────────────────────────────────
	h.step("issue_unique duplicate code → #8 DuplicateCode", func() error {
		_, err := ownerC.IssueUnique(ctx, campPct, []string{"BURN0002"}) // already issued
		return wantCode(err, sd.ErrDuplicateCode)
	})
	h.step("issue_unique empty batch → #7 InvalidCode", func() error {
		_, err := ownerC.IssueUnique(ctx, campPct, []string{})
		return wantCode(err, sd.ErrInvalidCode)
	})
	h.step("issue_unique code too long → #11 CodeTooLong", func() error {
		_, err := ownerC.IssueUnique(ctx, campPct, []string{longCode(70)})
		return wantCode(err, sd.ErrCodeTooLong)
	})
	h.step("issue_unique by stranger → #6 Unauthorized", func() error {
		_, err := strangerC.IssueUnique(ctx, campPct, []string{"HIJACK01"})
		return wantCode(err, sd.ErrUnauthorized)
	})
	h.step("issue_unique exceeding supply → #5 SupplyExhausted", func() error {
		id, err := ownerC.CreateCampaign(ctx, "Tiny", "percentage", 1000, 2, farFuture())
		if err != nil {
			return err
		}
		_, err = ownerC.IssueUnique(ctx, id, []string{"A1", "A2", "A3"}) // 3 > 2
		return wantCode(err, sd.ErrSupplyExhausted)
	})

	// ── cross-campaign code scoping (ADR-009) ────────────────────────
	h.step("same code in two campaigns is independent (ADR-009)", func() error {
		c1, err := ownerC.CreateCampaign(ctx, "X1", "percentage", 1000, 10, farFuture())
		if err != nil {
			return err
		}
		c2, err := ownerC.CreateCampaign(ctx, "X2", "percentage", 1000, 10, farFuture())
		if err != nil {
			return err
		}
		if _, err := ownerC.IssueUnique(ctx, c1, []string{"SHARED"}); err != nil {
			return err
		}
		if _, err := ownerC.IssueUnique(ctx, c2, []string{"SHARED"}); err != nil {
			return err // same string, different campaign → must NOT be a duplicate
		}
		if _, err := ownerC.RedeemUnique(ctx, c1, "SHARED", refHash("c1")); err != nil {
			return err
		}
		v1, err := pub.IsValid(ctx, c1, "SHARED")
		if err != nil {
			return err
		}
		v2, err := pub.IsValid(ctx, c2, "SHARED")
		if err != nil {
			return err
		}
		return all(assert(!v1, "burned in c1"), assert(v2, "still valid in c2"))
	})

	// ── delegates (ADR-002) ──────────────────────────────────────────
	h.step("delegate: add → redeem → remove → denied", func() error {
		if err := ownerC.AddDelegate(ctx, campPct, delegate.Address()); err != nil {
			return err
		}
		isDel, err := pub.IsDelegate(ctx, campPct, delegate.Address())
		if err != nil {
			return err
		}
		if err := assert(isDel, "delegate should be registered"); err != nil {
			return err
		}
		if _, err := delegateC.RedeemUnique(ctx, campPct, "BURN0002", refHash("by-delegate")); err != nil {
			return err
		}
		if err := ownerC.RemoveDelegate(ctx, campPct, delegate.Address()); err != nil {
			return err
		}
		stillDel, err := pub.IsDelegate(ctx, campPct, delegate.Address())
		if err != nil {
			return err
		}
		if err := assert(!stillDel, "delegate should be revoked"); err != nil {
			return err
		}
		_, err = delegateC.RedeemUnique(ctx, campPct, "BURN0003", refHash("after-revoke"))
		return wantCode(err, sd.ErrUnauthorized)
	})

	// ── TTL bumps ────────────────────────────────────────────────────
	h.step("bump_campaign (public) ok; unknown → #1", func() error {
		if err := creatorC.BumpCampaign(ctx, campPct); err != nil { // anyone may pay rent
			return err
		}
		_, err := pub.GetCampaign(ctx, 0) // sanity: 0 is not found
		if e := wantCode(err, sd.ErrCampaignNotFound); e != nil {
			return e
		}
		return wantCode(ownerC.BumpCampaign(ctx, 9_000_000), sd.ErrCampaignNotFound)
	})
	h.step("bump_codes existing+unknown ok; empty → #7", func() error {
		if err := creatorC.BumpCodes(ctx, campPct, []string{"BURN0001", "UNKNOWN9"}); err != nil {
			return err
		}
		return wantCode(creatorC.BumpCodes(ctx, campPct, []string{}), sd.ErrInvalidCode)
	})

	// ════════════════════════════════════════════════════════════════
	// TALLY PROFILE (shared codes) + settlement
	// ════════════════════════════════════════════════════════════════
	var ugc uint64
	h.step("setup: campaign for Tally", func() error {
		var err error
		ugc, err = ownerC.CreateCampaign(ctx, "Creator program", "percentage", 1000, 1000, farFuture())
		return err
	})

	creatorAddr := creator.Address()

	h.step("register_shared attributed + settlement (token+rate)", func() error {
		rate := big.NewInt(1000) // 1000 stroops per attributed redemption
		tok := nativeSAC
		if err := ownerC.RegisterShared(ctx, ugc, "ROBERTOX", &creatorAddr, &tok, rate); err != nil {
			return err
		}
		s, err := pub.GetShared(ctx, ugc, "ROBERTOX")
		if err != nil {
			return err
		}
		return all(
			assert(s.AttributedTo != nil && *s.AttributedTo == creatorAddr, "attributed_to"),
			assert(s.PayoutToken != nil && *s.PayoutToken == nativeSAC, "payout_token"),
			assert(s.PayoutRate.Cmp(big.NewInt(1000)) == 0, "payout_rate"),
		)
	})

	h.step("register_shared attributed, count-only (no token, rate 0)", func() error {
		return ownerC.RegisterShared(ctx, ugc, "COUNTONLY", &creatorAddr, nil, big.NewInt(0))
	})
	h.step("register_shared unattributed, count-only", func() error {
		return ownerC.RegisterShared(ctx, ugc, "NOATTRIB", nil, nil, big.NewInt(0))
	})

	// register_shared validation
	h.step("register_shared unattributed + token → #18 InvalidSettlement", func() error {
		tok := nativeSAC
		return wantCode(ownerC.RegisterShared(ctx, ugc, "BAD1", nil, &tok, big.NewInt(1000)), sd.ErrInvalidSettlement)
	})
	h.step("register_shared token + rate 0 → #18 InvalidSettlement", func() error {
		tok := nativeSAC
		return wantCode(ownerC.RegisterShared(ctx, ugc, "BAD2", &creatorAddr, &tok, big.NewInt(0)), sd.ErrInvalidSettlement)
	})
	h.step("register_shared no token + rate != 0 → #18 InvalidSettlement", func() error {
		return wantCode(ownerC.RegisterShared(ctx, ugc, "BAD3", &creatorAddr, nil, big.NewInt(5)), sd.ErrInvalidSettlement)
	})
	h.step("register_shared duplicate → #13 AlreadyRegistered", func() error {
		return wantCode(ownerC.RegisterShared(ctx, ugc, "ROBERTOX", &creatorAddr, nil, big.NewInt(0)), sd.ErrAlreadyRegistered)
	})
	h.step("register_shared by stranger → #6 Unauthorized", func() error {
		return wantCode(strangerC.RegisterShared(ctx, ugc, "SNEAK", nil, nil, big.NewInt(0)), sd.ErrUnauthorized)
	})
	h.step("register_shared empty code → #7; long code → #11", func() error {
		if e := wantCode(ownerC.RegisterShared(ctx, ugc, "", nil, nil, big.NewInt(0)), sd.ErrInvalidCode); e != nil {
			return e
		}
		return wantCode(ownerC.RegisterShared(ctx, ugc, longCode(70), nil, nil, big.NewInt(0)), sd.ErrCodeTooLong)
	})
	h.step("get_shared not found → #12 SharedNotFound", func() error {
		_, err := pub.GetShared(ctx, ugc, "GHOST")
		return wantCode(err, sd.ErrSharedNotFound)
	})

	// commit_tally — v0.2 requires EXACT attribution: an attributed code must
	// credit its creator for the full committed count (attributed == count).
	h.step("commit_tally (attributed, exact) → get_tally matches", func() error {
		attr := map[string]uint32{creatorAddr: 3}
		if err := ownerC.CommitTally(ctx, ugc, "ROBERTOX", 1, 3, refHash("merkle-1"), attr); err != nil {
			return err
		}
		t, err := pub.GetTally(ctx, ugc, "ROBERTOX", 1)
		if err != nil {
			return err
		}
		return all(
			assert(t.Count == 3, "count"),
			assert(t.PerAttribution[creatorAddr] == 3, "per_attribution"),
		)
	})
	h.step("commit_tally partial attribution → #17 InvalidTally (v0.2)", func() error {
		attr := map[string]uint32{creatorAddr: 3}
		return wantCode(ownerC.CommitTally(ctx, ugc, "ROBERTOX", 4, 5, refHash("m"), attr), sd.ErrInvalidTally)
	})
	h.step("commit_tally same period again → #14 PeriodCommitted", func() error {
		attr := map[string]uint32{creatorAddr: 1}
		return wantCode(ownerC.CommitTally(ctx, ugc, "ROBERTOX", 1, 2, refHash("m"), attr), sd.ErrPeriodCommitted)
	})
	h.step("commit_tally crediting wrong addr → #19 AttributionMismatch", func() error {
		attr := map[string]uint32{stranger.Address(): 1}
		return wantCode(ownerC.CommitTally(ctx, ugc, "ROBERTOX", 2, 5, refHash("m"), attr), sd.ErrAttributionMismatch)
	})
	h.step("commit_tally on unattributed code w/ attribution → #19", func() error {
		attr := map[string]uint32{creatorAddr: 1}
		return wantCode(ownerC.CommitTally(ctx, ugc, "NOATTRIB", 1, 5, refHash("m"), attr), sd.ErrAttributionMismatch)
	})
	h.step("commit_tally sum > count → #17 InvalidTally", func() error {
		attr := map[string]uint32{creatorAddr: 9}
		return wantCode(ownerC.CommitTally(ctx, ugc, "COUNTONLY", 1, 5, refHash("m"), attr), sd.ErrInvalidTally)
	})
	h.step("commit_tally by stranger → #6 Unauthorized", func() error {
		attr := map[string]uint32{creatorAddr: 1}
		return wantCode(strangerC.CommitTally(ctx, ugc, "ROBERTOX", 3, 5, refHash("m"), attr), sd.ErrUnauthorized)
	})
	h.step("get_tally missing period → #15 TallyNotFound", func() error {
		_, err := pub.GetTally(ctx, ugc, "ROBERTOX", 99)
		return wantCode(err, sd.ErrTallyNotFound)
	})

	// payouts + settlement
	h.step("compute_payouts previews count*rate", func() error {
		ps, err := pub.ComputePayouts(ctx, ugc, "ROBERTOX", 1)
		if err != nil {
			return err
		}
		return all(
			assert(len(ps) == 1, "one payout"),
			assert(ps[0].To == creatorAddr, "payout recipient"),
			assert(ps[0].Amount.Cmp(big.NewInt(3000)) == 0, "3 * 1000 = 3000 stroops"),
		)
	})
	// v0.2 settlement is allowance-based and permissionless: the owner grants
	// the contract an exact allowance, then ANY keeper can trigger the payout.
	h.step("settle without allowance → #18 InvalidSettlement (v0.2)", func() error {
		return wantCode(mustErr(ownerC.Settle(ctx, ugc, "ROBERTOX", 1)), sd.ErrInvalidSettlement)
	})
	h.step("owner approves the exact settlement allowance", func() error {
		latest, err := ownerC.LatestLedger(ctx)
		if err != nil {
			return err
		}
		if err := ownerC.ApproveSettlement(ctx, nativeSAC, big.NewInt(3000), latest+200); err != nil {
			return err
		}
		a, err := pub.SettlementAllowance(ctx, nativeSAC, owner.Address())
		if err != nil {
			return err
		}
		return assert(a.Cmp(big.NewInt(3000)) == 0, "allowance equals the exact payout total")
	})
	h.step("third-party keeper settles for the owner (real transfer)", func() error {
		ps, err := strangerC.SettleFor(ctx, owner.Address(), ugc, "ROBERTOX", 1)
		if err != nil {
			return err
		}
		remaining, err := pub.SettlementAllowance(ctx, nativeSAC, owner.Address())
		if err != nil {
			return err
		}
		return all(
			assert(len(ps) == 1, "one payout made"),
			assert(ps[0].To == creatorAddr, "paid creator"),
			assert(ps[0].Amount.Cmp(big.NewInt(3000)) == 0, "paid 3000 stroops"),
			assert(remaining.Sign() == 0, "allowance fully consumed"),
		)
	})
	h.step("settle again → #16 AlreadySettled", func() error {
		return wantCode(mustErr(ownerC.Settle(ctx, ugc, "ROBERTOX", 1)), sd.ErrAlreadySettled)
	})
	h.step("settle count-only code → #18 InvalidSettlement", func() error {
		attr := map[string]uint32{creatorAddr: 1}
		if err := ownerC.CommitTally(ctx, ugc, "COUNTONLY", 2, 1, refHash("m2"), attr); err != nil {
			return err
		}
		return wantCode(mustErr(ownerC.Settle(ctx, ugc, "COUNTONLY", 2)), sd.ErrInvalidSettlement)
	})
	h.step("bump_tally ok; unknown shared → #12", func() error {
		if err := creatorC.BumpTally(ctx, ugc, "ROBERTOX", []uint64{1}); err != nil {
			return err
		}
		return wantCode(creatorC.BumpTally(ctx, ugc, "GHOST", []uint64{1}), sd.ErrSharedNotFound)
	})

	// ════════════════════════════════════════════════════════════════
	// EXPIRY — assert the short-lived campaign now rejects mutations (#4)
	// ════════════════════════════════════════════════════════════════
	h.step("wait for short-lived campaign to expire, then reject writes (#4)", func() error {
		waitUntil(expValidUntil + 3)
		if e := wantCode(mustErr(ownerC.IssueUnique(ctx, expID, []string{"LATE0001"})), sd.ErrCampaignExpired); e != nil {
			return fmt.Errorf("issue: %w", e)
		}
		if _, err := ownerC.RedeemUnique(ctx, expID, "EXP00001", refHash("late")); wantCode(err, sd.ErrCampaignExpired) != nil {
			return fmt.Errorf("redeem: expected #4, got %v", err)
		}
		if e := wantCode(ownerC.RegisterShared(ctx, expID, "LATESHARE", nil, nil, big.NewInt(0)), sd.ErrCampaignExpired); e != nil {
			return fmt.Errorf("register_shared: %w", e)
		}
		return nil
	})

	// ── summary ──────────────────────────────────────────────────────
	fmt.Printf("\n%d passed, %d failed (of %d scenarios)\n", h.pass, h.fail, h.pass+h.fail)
	if h.fail > 0 {
		os.Exit(1)
	}
	fmt.Println("ALL E2E SCENARIOS PASSED ✔")
}

// ═══════════════════════════════════════════════════════════════════
// harness + helpers
// ═══════════════════════════════════════════════════════════════════

type harness struct{ pass, fail int }

func (h *harness) step(name string, fn func() error) {
	start := time.Now()
	err := fn()
	dur := time.Since(start).Round(time.Millisecond)
	if err != nil {
		h.fail++
		fmt.Printf("FAIL  %-58s (%s)\n        ↳ %v\n", name, dur, err)
		return
	}
	h.pass++
	fmt.Printf("ok    %-58s (%s)\n", name, dur)
}

func assert(cond bool, msg string) error {
	if !cond {
		return fmt.Errorf("assertion failed: %s", msg)
	}
	return nil
}

func all(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func wantCode(err error, want sd.Code) error {
	got, ok := sd.CodeOf(err)
	if !ok {
		return fmt.Errorf("expected contract #%d (%s), got: %v", uint32(want), want, err)
	}
	if got != want {
		return fmt.Errorf("expected #%d (%s), got #%d (%s)", uint32(want), want, uint32(got), got)
	}
	return nil
}

// mustErr adapts a (T, error) returning call so wantCode can consume just the error.
func mustErr[T any](_ T, err error) error { return err }

func contains(xs []uint64, x uint64) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func refHash(s string) [32]byte { return sha256.Sum256([]byte(s)) }

func longCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'A'
	}
	return string(b)
}

func farFuture() uint64 { return uint64(time.Now().Unix()) + 10*365*24*3600 }

func randKP() *keypair.Full {
	kp, err := keypair.Random()
	if err != nil {
		panic(err)
	}
	return kp
}

func waitUntil(unixSec uint64) {
	for uint64(time.Now().Unix()) < unixSec {
		time.Sleep(time.Second)
	}
}

// ── funding ──────────────────────────────────────────────────────────

func fundAll(kps ...*keypair.Full) error {
	var wg sync.WaitGroup
	errs := make([]error, len(kps))
	for i, kp := range kps {
		wg.Add(1)
		go func(i int, addr string) {
			defer wg.Done()
			errs[i] = fund(addr)
		}(i, kp.Address())
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func fund(addr string) error {
	resp, err := http.Get("https://friendbot.stellar.org/?addr=" + url.QueryEscape(addr))
	if err != nil {
		return err
	}
	resp.Body.Close()
	// Poll horizon until the account is visible (funded), regardless of the
	// friendbot status (it returns 400 if already funded).
	for i := 0; i < 40; i++ {
		r, err := http.Get("https://horizon-testnet.stellar.org/accounts/" + addr)
		if err == nil {
			code := r.StatusCode
			r.Body.Close()
			if code == 200 {
				return nil
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("account %s not visible after funding", addr)
}
