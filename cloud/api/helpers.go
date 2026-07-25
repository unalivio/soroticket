package main

import (
	"encoding/json"
	"net/http"

	sd "github.com/soroticket/soroticket-go"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// payoutToken is the settlement token for the testnet-only preview. A future
// mainnet release must make the production asset an explicit deployment config.
const payoutToken = sd.TestnetNativeSAC

// payoutUnit labels settlement amounts in the API/console.
const payoutUnit = "XLM"

func externalMode(env string) string {
	if env == "live" {
		return "metered"
	}
	return env
}

func contractCodeOf(err error) (int, bool) {
	c, ok := sd.CodeOf(err)
	return int(c), ok
}

func codeName(n int) string { return sd.Code(n).String() }

func friendlyErrByNum(n int) string { return friendlyErr[sd.Code(n)] }

func writeJSONBody(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func validStellarAddress(address string) bool {
	if address == "" {
		return false
	}
	if address[0] == 'C' {
		decoded, err := strkey.Decode(strkey.VersionByteContract, address)
		return err == nil && len(decoded) == 32
	}
	_, err := xdr.AddressToAccountId(address)
	return err == nil
}
