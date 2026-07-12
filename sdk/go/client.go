// Package sorodeal is the Go SDK for the Sorodeal coupon protocol on Stellar
// Soroban — the Burn profile (unique single-use codes) and the Tally profile
// (shared codes + optional SAC token settlement).
//
// Unlike the prototype's `stellar` CLI shell-out, this client signs in-process
// with the Go Stellar SDK and talks to Soroban RPC directly: it simulates,
// assembles (footprint + resource fee + auth), signs, and submits with retries
// and idempotent re-submission keyed on the transaction hash (so a network
// retry cannot submit a different transaction for the same signed envelope.
package sorodeal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stellar/go-stellar-sdk/clients/rpcclient"
	"github.com/stellar/go-stellar-sdk/keypair"
	protocol "github.com/stellar/go-stellar-sdk/protocols/rpc"
	"github.com/stellar/go-stellar-sdk/strkey"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// TestnetPassphrase is the Stellar testnet network passphrase.
const TestnetPassphrase = "Test SDF Network ; September 2015"

// TestnetRPC is the public Soroban testnet RPC endpoint.
const TestnetRPC = "https://soroban-testnet.stellar.org"

// LegacyTestnetContractID is the immutable v0.1 testnet deployment. It is
// retained only for compatibility and is deprecated by the 2026-07-11 security
// review; do not use it for real-value integrations.
const LegacyTestnetContractID = "CBSTBPSCSUXWK57OBQN7QKGS56WUDNJBURV5PD5ZDUHTR2KQYC52QDBX"

// TestnetContractID is a compatibility alias for LegacyTestnetContractID.
// Deprecated: pass an explicitly reviewed deployment in Config.ContractID.
const TestnetContractID = LegacyTestnetContractID

// TestnetNativeSAC is the testnet native-XLM Stellar Asset Contract — a handy
// settlement token for Tally payouts (see deployments/testnet.json).
const TestnetNativeSAC = "CDLZFC3SYJYDZT7K67VZ75HPJVIEUVNIXF47ZG2FB2RMQQVU2HHGCYSC"

// defaultReadSource is an existing testnet account used purely as the source for
// read-only simulations (no signature, sequence irrelevant) — the deployer.
const defaultReadSource = "GCIJM67CD2U6XPI5GYS5VYSNIYOKLH7DZ4XA3W45PID7DYCRYFTRDSV6"

// Config configures a Client.
type Config struct {
	ContractID        string        // C… contract address (defaults to TestnetContractID)
	RPCURL            string        // Soroban RPC endpoint (defaults to TestnetRPC)
	NetworkPassphrase string        // defaults to TestnetPassphrase
	Signer            *keypair.Full // required for writes; reads work without it
	ReadSource        string        // optional source account for read sims
}

// Client invokes the Sorodeal coupon-ledger contract. It is bound to at most one
// signing identity (Config.Signer); construct several Clients for several actors
// (owner, delegate, …). Safe for sequential use; not designed for concurrent
// writes from one Client (sequence numbers are loaded per call).
type Client struct {
	cfg        Config
	rpc        *rpcclient.Client
	contractID xdr.ContractId
	lastTxHash string
}

// New builds a Client. Missing network fields default to testnet.
func New(cfg Config) (*Client, error) {
	if cfg.ContractID == "" {
		cfg.ContractID = TestnetContractID
	}
	if cfg.RPCURL == "" {
		cfg.RPCURL = TestnetRPC
	}
	if cfg.NetworkPassphrase == "" {
		cfg.NetworkPassphrase = TestnetPassphrase
	}
	cid, err := parseContractID(cfg.ContractID)
	if err != nil {
		return nil, fmt.Errorf("invalid contract id %q: %w", cfg.ContractID, err)
	}
	return &Client{
		cfg:        cfg,
		rpc:        rpcclient.NewClient(cfg.RPCURL, nil),
		contractID: cid,
	}, nil
}

// Testnet builds a testnet Client signing with the given keypair (nil = read-only).
func Testnet(signer *keypair.Full) (*Client, error) {
	return New(Config{Signer: signer})
}

// Address returns the signer's account address, or "" for a read-only client.
func (c *Client) Address() string {
	if c.cfg.Signer == nil {
		return ""
	}
	return c.cfg.Signer.Address()
}

// LastTransactionHash returns the hash of the most recent successful write
// performed by this client, or an empty string if the last write failed or no
// write has run. Client writes are intentionally sequential; see Client docs.
func (c *Client) LastTransactionHash() string { return c.lastTxHash }

// Close releases the underlying RPC client.
func (c *Client) Close() error { return c.rpc.Close() }

// LatestLedger returns the most recent ledger sequence reported by RPC.
func (c *Client) LatestLedger(ctx context.Context) (uint32, error) {
	ledger, err := c.rpc.GetLatestLedger(ctx)
	if err != nil {
		return 0, err
	}
	return ledger.Sequence, nil
}

func parseContractID(address string) (xdr.ContractId, error) {
	decoded, err := strkey.Decode(strkey.VersionByteContract, address)
	if err != nil {
		return xdr.ContractId{}, err
	}
	var id xdr.ContractId
	copy(id[:], decoded)
	return id, nil
}

// hostOp builds the InvokeHostFunction operation for a contract call.
func (c *Client) hostOpAt(contractID xdr.ContractId, method string, args []xdr.ScVal, source string) *txnbuild.InvokeHostFunction {
	return &txnbuild.InvokeHostFunction{
		HostFunction: xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
			InvokeContract: &xdr.InvokeContractArgs{
				ContractAddress: xdr.ScAddress{Type: xdr.ScAddressTypeScAddressTypeContract, ContractId: &contractID},
				FunctionName:    xdr.ScSymbol(method),
				Args:            args,
			},
		},
		SourceAccount: source,
	}
}

func (c *Client) buildTx(source string, seq int64, op *txnbuild.InvokeHostFunction) (*txnbuild.Transaction, error) {
	return txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount:        &txnbuild.SimpleAccount{AccountID: source, Sequence: seq},
		IncrementSequenceNum: true,
		Operations:           []txnbuild.Operation{op},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewTimeout(300)},
	})
}

// read simulates a contract call and returns the decoded return ScVal. No
// signature, no submission. Surfaces contract traps as *ContractError.
func (c *Client) read(ctx context.Context, method string, args []xdr.ScVal) (xdr.ScVal, error) {
	return c.readAt(ctx, c.contractID, method, args)
}

func (c *Client) readAt(ctx context.Context, contractID xdr.ContractId, method string, args []xdr.ScVal) (xdr.ScVal, error) {
	source := c.cfg.ReadSource
	if source == "" {
		if c.cfg.Signer != nil {
			source = c.cfg.Signer.Address()
		} else {
			source = defaultReadSource
		}
	}
	tx, err := c.buildTx(source, 0, c.hostOpAt(contractID, method, args, source))
	if err != nil {
		return xdr.ScVal{}, err
	}
	b64, err := tx.Base64()
	if err != nil {
		return xdr.ScVal{}, err
	}
	resp, err := c.rpc.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{Transaction: b64})
	if err != nil {
		return xdr.ScVal{}, fmt.Errorf("simulate %s: %w", method, err)
	}
	if resp.Error != "" {
		return xdr.ScVal{}, classifyHostError(resp.Error)
	}
	return returnValueFromSim(resp)
}

// invoke runs a state-changing call: simulate → assemble → sign → submit → poll,
// returning the contract's actual return value (read from the applied tx meta,
// not from simulation — the global counters can advance between the two).
func (c *Client) invoke(ctx context.Context, method string, args []xdr.ScVal) (xdr.ScVal, error) {
	return c.invokeAt(ctx, c.contractID, method, args)
}

func (c *Client) invokeAt(ctx context.Context, contractID xdr.ContractId, method string, args []xdr.ScVal) (xdr.ScVal, error) {
	c.lastTxHash = ""
	if c.cfg.Signer == nil {
		return xdr.ScVal{}, errors.New("a signer is required for writes")
	}
	pub := c.cfg.Signer.Address()

	acct, err := c.rpc.LoadAccount(ctx, pub)
	if err != nil {
		return xdr.ScVal{}, fmt.Errorf("load account %s: %w", pub, err)
	}
	seq, err := acct.GetSequenceNumber()
	if err != nil {
		return xdr.ScVal{}, err
	}

	// 1) Simulate to obtain footprint, resource fee, and auth entries. Most
	//    contract errors (Unauthorized, AlreadyRedeemed, …) surface here.
	simTx, err := c.buildTx(pub, seq, c.hostOpAt(contractID, method, args, pub))
	if err != nil {
		return xdr.ScVal{}, err
	}
	simB64, err := simTx.Base64()
	if err != nil {
		return xdr.ScVal{}, err
	}
	sim, err := c.rpc.SimulateTransaction(ctx, protocol.SimulateTransactionRequest{Transaction: simB64})
	if err != nil {
		return xdr.ScVal{}, fmt.Errorf("simulate %s: %w", method, err)
	}
	if sim.Error != "" {
		return xdr.ScVal{}, classifyHostError(sim.Error)
	}

	// 2) Assemble: attach Soroban data (footprint + resource fee) and auth.
	var sorobanData xdr.SorobanTransactionData
	if err := xdr.SafeUnmarshalBase64(sim.TransactionDataXDR, &sorobanData); err != nil {
		return xdr.ScVal{}, fmt.Errorf("decode soroban data: %w", err)
	}
	var auth []xdr.SorobanAuthorizationEntry
	if len(sim.Results) > 0 && sim.Results[0].AuthXDR != nil {
		for _, a := range *sim.Results[0].AuthXDR {
			var e xdr.SorobanAuthorizationEntry
			if err := xdr.SafeUnmarshalBase64(a, &e); err != nil {
				return xdr.ScVal{}, fmt.Errorf("decode auth entry: %w", err)
			}
			auth = append(auth, e)
		}
	}

	op := c.hostOpAt(contractID, method, args, pub)
	op.Auth = auth
	op.Ext = xdr.TransactionExt{V: 1, SorobanData: &sorobanData}

	// NewTransaction adds SorobanData.ResourceFee to BaseFee*numOps automatically.
	finalTx, err := c.buildTx(pub, seq, op)
	if err != nil {
		return xdr.ScVal{}, err
	}
	signed, err := finalTx.Sign(c.cfg.NetworkPassphrase, c.cfg.Signer)
	if err != nil {
		return xdr.ScVal{}, fmt.Errorf("sign: %w", err)
	}
	env, err := signed.Base64()
	if err != nil {
		return xdr.ScVal{}, err
	}
	hash, err := signed.HashHex(c.cfg.NetworkPassphrase)
	if err != nil {
		return xdr.ScVal{}, err
	}

	// 3) Submit + poll. The same signed envelope is re-sent on transient
	//    failures — re-submitting an identical tx is idempotent (the network
	//    dedups by hash), so a retry never produces a second redemption.
	return c.submitAndPoll(ctx, env, hash, method)
}

func (c *Client) submitAndPoll(ctx context.Context, env, hash, method string) (xdr.ScVal, error) {
	const maxSend = 5
	var lastErr error
	for attempt := 0; attempt < maxSend; attempt++ {
		resp, err := c.rpc.SendTransaction(ctx, protocol.SendTransactionRequest{Transaction: env})
		if err != nil {
			lastErr = err
			sleepCtx(ctx, backoff(attempt))
			continue
		}
		switch strings.ToUpper(resp.Status) {
		case "ERROR":
			return xdr.ScVal{}, classifyFailure(resp.ErrorResultXDR, resp.DiagnosticEventsXDR,
				fmt.Sprintf("send %s rejected", method))
		case "TRY_AGAIN_LATER":
			lastErr = fmt.Errorf("send %s: TRY_AGAIN_LATER", method)
			sleepCtx(ctx, backoff(attempt))
			continue
		default: // PENDING, DUPLICATE → submitted; go poll by hash
			got, err := c.rpc.PollTransaction(ctx, hash)
			if err != nil {
				return xdr.ScVal{}, fmt.Errorf("poll %s (%s): %w", method, hash, err)
			}
			switch strings.ToUpper(got.Status) {
			case protocol.TransactionStatusSuccess:
				value, err := returnValueFromMeta(got.ResultMetaXDR)
				if err != nil {
					return xdr.ScVal{}, err
				}
				c.lastTxHash = hash
				return value, nil
			case protocol.TransactionStatusFailed:
				return xdr.ScVal{}, classifyFailure(got.ResultXDR, got.DiagnosticEventsXDR,
					fmt.Sprintf("tx %s failed", hash))
			default:
				return xdr.ScVal{}, fmt.Errorf("tx %s ended in status %s", hash, got.Status)
			}
		}
	}
	return xdr.ScVal{}, fmt.Errorf("submit %s exhausted retries: %w", method, lastErr)
}

func backoff(attempt int) time.Duration {
	return time.Duration(300*(1<<attempt)) * time.Millisecond
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// returnValueFromSim extracts the return ScVal from a simulation response.
func returnValueFromSim(resp protocol.SimulateTransactionResponse) (xdr.ScVal, error) {
	if len(resp.Results) == 0 || resp.Results[0].ReturnValueXDR == nil {
		return scVoid(), nil
	}
	var v xdr.ScVal
	if err := xdr.SafeUnmarshalBase64(*resp.Results[0].ReturnValueXDR, &v); err != nil {
		return xdr.ScVal{}, fmt.Errorf("decode return value: %w", err)
	}
	return v, nil
}

// returnValueFromMeta extracts the host-function return value from an applied
// transaction's result meta.
func returnValueFromMeta(metaB64 string) (xdr.ScVal, error) {
	if metaB64 == "" {
		return scVoid(), nil
	}
	var meta xdr.TransactionMeta
	if err := xdr.SafeUnmarshalBase64(metaB64, &meta); err != nil {
		return xdr.ScVal{}, fmt.Errorf("decode tx meta: %w", err)
	}
	// Protocol 23 testnet emits TransactionMeta V4 (SorobanTransactionMetaV2,
	// with a nullable ReturnValue); older ledgers use V3. Try V4 first.
	if v4, ok := meta.GetV4(); ok && v4.SorobanMeta != nil && v4.SorobanMeta.ReturnValue != nil {
		return *v4.SorobanMeta.ReturnValue, nil
	}
	if v3, ok := meta.GetV3(); ok && v3.SorobanMeta != nil {
		return v3.SorobanMeta.ReturnValue, nil
	}
	return scVoid(), nil
}

// classifyFailure tries to recover a contract Code from a failed submission's
// result/diagnostics; otherwise returns a descriptive error.
func classifyFailure(resultXDR string, diagnostics []string, prefix string) error {
	for _, d := range diagnostics {
		var ev xdr.DiagnosticEvent
		if err := xdr.SafeUnmarshalBase64(d, &ev); err == nil {
			if m := contractErrRe.FindString(ev.String()); m != "" {
				return classifyHostError(ev.String())
			}
		}
	}
	if resultXDR != "" {
		var res xdr.TransactionResult
		if err := xdr.SafeUnmarshalBase64(resultXDR, &res); err == nil {
			return fmt.Errorf("%s: %s", prefix, res.Result.Code.String())
		}
	}
	return errors.New(prefix)
}
