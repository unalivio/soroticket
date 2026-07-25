# @soroticket/sdk

TypeScript/JavaScript SDK for the **Soroticket** coupon protocol on Stellar
Soroban — Burn (unique tokens) and Tally (shared codes + settlement). Works in
the browser (Freighter) and on the server (keypair).

The typed contract `Client` is generated from v0.2.0 (`stellar contract
bindings typescript`), deployed to testnet on 2026-07-12. The `TESTNET` preset
points to that deployment (`deployments/testnet-v0.2.0.json`), including the
v0.2-only calls `campaigns_page` and `is_settled`. `LEGACY_TESTNET` remains the
deprecated v0.1 contract, where those calls do not exist. Testnet preview —
never point real value at either.

```bash
npm install   # then: npm run build
```

## Read (no signer)

```ts
import { soroticket } from "@soroticket/sdk";

const c = soroticket({ contractId: "C_REVIEWED_DEPLOYMENT" });
const token = (await c.verify({ campaign_id: 1n, code: "DEMO0001" })).result.unwrap();
console.log(token.is_burned ? "BURNED" : "VALID");

const mine = (await c.campaigns_of({ owner: "G..." })).result; // bigint[]
```

## Write — server (keypair)

```ts
import { soroticket, keypairSigner } from "@soroticket/sdk";

const signer = keypairSigner(process.env.SECRET!); // S...
const c = soroticket({
  contractId: "C_REVIEWED_V2_DEPLOYMENT",
  publicKey: signer.publicKey,
  signTransaction: signer.signTransaction,
});

const tx = await c.create_campaign({
  owner: signer.publicKey, name: "Cafe", discount_type: "percentage",
  discount_value: 1000n, total_supply: 100, valid_until: 9999999999n,
});
const campaignId = (await tx.signAndSend()).result.unwrap();
```

## Write — browser (Freighter)

```ts
import { soroticket, freighterSigner, redeemerCommitment } from "@soroticket/sdk";

const signer = await freighterSigner();
const c = soroticket({
  contractId: "C_REVIEWED_V2_DEPLOYMENT",
  publicKey: signer.publicKey,
  signTransaction: signer.signTransaction,
});

// redeem: commit the redeemer reference off-chain (no PII on-chain)
const { hash } = await redeemerCommitment("order-8842");
const tx = await c.redeem_unique({ authorizer: signer.publicKey, campaign_id: 1n, code: "DEMO0001", redeemer_ref_hash: hash });
const receipt = (await tx.signAndSend()).result.unwrap();
```

## Tally (shared codes)

`register_shared` (fixes the payout token + rate), `commit_tally` (count +
`merkle_root` + per-attribution), `get_tally`/`compute_payouts` (public),
`is_settled`, and allowance-based `settle`. For an attributed v0.2 code, the
single attributed count must equal the total. The off-chain signed-receipt /
Merkle layer is the integrator's responsibility; a root does not prove that a
real-world sale was genuine.

## Low-level browser client (`freighterClient`)

The typed `Client` swallows the contract **error code** on simulation (view)
errors — `(await c.verify(...)).result.unwrapErr()` comes back with an empty
message instead of `CouponNotFound`. For browser dapps that need precise
per-error UX, this package also ships a hand-written low-level client that
surfaces it:

```ts
import { freighterClient } from "@soroticket/sdk";

const c = freighterClient({
  contractId: "C...", rpcUrl: "https://soroban-testnet.stellar.org",
  networkPassphrase: "Test SDF Network ; September 2015",
  readSource: "G...", // any funded account; source for read-only sims
});

try {
  c.toNative(await c.read("verify", [c.scU64(1n), c.scStr("NOPE")]));
} catch (e) {
  e.contractCode; // 2  → look up your friendly message
}

const { address } = await c.connectWallet();      // Freighter (lazy-loaded)
const { tx } = await c.registerShared(
  address, 1n, "FALL25", "G_CREATOR", "C_PAYOUT_TOKEN", "10000000",
);

// v0.2 owner: grant a bounded allowance to the Soroticket contract.
await c.approveSettlement(address, "C_PAYOUT_TOKEN", 40_000n, 12_345_678);
// A keeper may then call settleFor(keeperAddress, ownerAddress, ...).
```

This is the exact client the developer playground (`web/`) consumes — one
implementation, no drift. Reads work in Node too (Freighter is only loaded for
writes/`connectWallet`). The standalone `contractErrorCode(raw)` helper lets
typed-`Client` users recover the code the bindings drop.

## Notes

- The generated typed client keeps token amounts and `u64`/`i128` fields as
  `bigint`. The low-level convenience client accepts `number|bigint|string` and
  throws rather than silently returning an unsafe JavaScript integer.
- Errors surface as the contract's `Errors` map (1–19), re-exported here.
- `TESTNET` is the current v0.2.0 testnet deployment. `LEGACY_TESTNET` is the
  vulnerable v0.1 deployment, kept only for historical reads. Never use either
  for real value.
