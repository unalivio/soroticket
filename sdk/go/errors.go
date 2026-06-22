package sorodeal

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Code is a Sorodeal coupon-ledger contract error code (mirrors the contract's
// `Error` enum, abi-v0.1.0). Returned inside a *ContractError.
type Code uint32

const (
	ErrCampaignNotFound    Code = 1
	ErrCouponNotFound      Code = 2
	ErrAlreadyRedeemed     Code = 3
	ErrCampaignExpired     Code = 4
	ErrSupplyExhausted     Code = 5
	ErrUnauthorized        Code = 6
	ErrInvalidCode         Code = 7
	ErrDuplicateCode       Code = 8
	ErrInvalidTerms        Code = 9
	ErrBatchTooLarge       Code = 10
	ErrCodeTooLong         Code = 11
	ErrSharedNotFound      Code = 12
	ErrAlreadyRegistered   Code = 13
	ErrPeriodCommitted     Code = 14
	ErrTallyNotFound       Code = 15
	ErrAlreadySettled      Code = 16
	ErrInvalidTally        Code = 17
	ErrInvalidSettlement   Code = 18
	ErrAttributionMismatch Code = 19
)

var codeNames = map[Code]string{
	1: "CampaignNotFound", 2: "CouponNotFound", 3: "AlreadyRedeemed",
	4: "CampaignExpired", 5: "SupplyExhausted", 6: "Unauthorized",
	7: "InvalidCode", 8: "DuplicateCode", 9: "InvalidTerms",
	10: "BatchTooLarge", 11: "CodeTooLong", 12: "SharedNotFound",
	13: "AlreadyRegistered", 14: "PeriodCommitted", 15: "TallyNotFound",
	16: "AlreadySettled", 17: "InvalidTally", 18: "InvalidSettlement",
	19: "AttributionMismatch",
}

// String returns the contract's name for this code (e.g. "Unauthorized").
func (c Code) String() string {
	if n, ok := codeNames[c]; ok {
		return n
	}
	return fmt.Sprintf("Error(%d)", uint32(c))
}

// ContractError is a typed contract trap surfaced from a simulation or an
// applied transaction. The contract returns Result<_, Error>, which Soroban
// encodes as a host trap carrying "Error(Contract, #N)"; we recover N here so
// callers get the real error code (and name) instead of an opaque host string.
type ContractError struct {
	Code Code
	Raw  string // the underlying host-error string, for diagnostics
}

func (e *ContractError) Error() string {
	return fmt.Sprintf("contract error #%d (%s)", uint32(e.Code), e.Code)
}

// CodeOf reports the contract Code of err, if it is (or wraps) a *ContractError.
func CodeOf(err error) (Code, bool) {
	var ce *ContractError
	if errors.As(err, &ce) {
		return ce.Code, true
	}
	return 0, false
}

var contractErrRe = regexp.MustCompile(`Error\(Contract,\s*#(\d+)\)`)

// classifyHostError turns a raw host-error string into a *ContractError when it
// carries an "Error(Contract, #N)" tag, or a plain error otherwise.
func classifyHostError(raw string) error {
	if m := contractErrRe.FindStringSubmatch(raw); m != nil {
		n, _ := strconv.Atoi(m[1])
		return &ContractError{Code: Code(n), Raw: raw}
	}
	return fmt.Errorf("host error: %s", raw)
}
