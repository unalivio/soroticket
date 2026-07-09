package main

import (
	"encoding/json"
	"net/http"

	sd "github.com/sorodeal/sorodeal-go"
)

// payoutToken is the settlement token for creator codes — the testnet native
// XLM SAC in v1 (see deployments/testnet.json); USDC swaps in on mainnet.
const payoutToken = sd.TestnetNativeSAC

// payoutUnit labels settlement amounts in the API/console.
const payoutUnit = "XLM"

func contractCodeOf(err error) (int, bool) {
	c, ok := sd.CodeOf(err)
	return int(c), ok
}

func codeName(n int) string { return sd.Code(n).String() }

func friendlyErrByNum(n int) string { return friendlyErr[sd.Code(n)] }

func writeJSONBody(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}
