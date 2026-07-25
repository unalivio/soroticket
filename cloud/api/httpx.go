package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"

	sd "github.com/soroticket/soroticket-go"
)

// friendlyErr maps contract error codes to merchant-readable copy. The numeric
// code and canonical name always travel alongside (problem+json).
var friendlyErr = map[sd.Code]string{
	sd.ErrCampaignNotFound:    "Campaign not found on-chain.",
	sd.ErrCouponNotFound:      "That code doesn't exist in this campaign.",
	sd.ErrAlreadyRedeemed:     "Already redeemed — this code was used before.",
	sd.ErrCampaignExpired:     "This campaign has expired.",
	sd.ErrSupplyExhausted:     "Supply exhausted — no codes left to issue.",
	sd.ErrUnauthorized:        "Not authorized for this campaign.",
	sd.ErrInvalidCode:         "Invalid code (empty or malformed).",
	sd.ErrDuplicateCode:       "That code was already issued in this campaign.",
	sd.ErrInvalidTerms:        "Invalid campaign terms.",
	sd.ErrBatchTooLarge:       "Batch too large.",
	sd.ErrCodeTooLong:         "Code is too long (max 64 UTF-8 bytes).",
	sd.ErrSharedNotFound:      "Shared code not registered.",
	sd.ErrAlreadyRegistered:   "That shared code is already registered.",
	sd.ErrPeriodCommitted:     "This period was already committed.",
	sd.ErrTallyNotFound:       "No tally committed for that period.",
	sd.ErrAlreadySettled:      "This period was already settled.",
	sd.ErrInvalidTally:        "For an attributed code, attributed count must equal the total.",
	sd.ErrInvalidSettlement:   "Settlement isn't configured for this code.",
	sd.ErrAttributionMismatch: "Attribution doesn't match the registered creator.",
}

type problem struct {
	Status  int    `json:"status"`
	Code    int    `json:"code,omitempty"` // contract error #
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Status: status, Message: msg})
}

// writeErr surfaces contract errors as structured problems; everything else 500.
func writeErr(w http.ResponseWriter, err error) {
	var ce *sd.ContractError
	if errors.As(err, &ce) {
		msg := friendlyErr[ce.Code]
		if msg == "" {
			msg = ce.Code.String()
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(problem{
			Status: http.StatusConflict, Code: int(ce.Code), Name: ce.Code.String(), Message: msg,
		})
		return
	}
	writeInternal(w, err, "request failed")
}

func writeInternal(w http.ResponseWriter, err error, context string) {
	log.Printf("%s: %v", context, err)
	writeProblem(w, http.StatusInternalServerError, "internal server error")
}

func readBody(r *http.Request, v any) error {
	return decodeBody(r, v, false)
}

func readOptionalBody(r *http.Request, v any) error {
	return decodeBody(r, v, true)
}

func decodeBody(r *http.Request, v any, optional bool) error {
	defer r.Body.Close()
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid JSON body: multiple values are not allowed")
	}
	return nil
}
