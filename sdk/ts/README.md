# @sorodeal/sdk

TypeScript/JavaScript SDK for the **Sorodeal** coupon protocol on Stellar
Soroban — Burn (unique tokens) and Tally (shared codes + settlement). Works in
the browser (Freighter) and on the server (keypair).

The typed contract `Client` is **generated from the contract** (`stellar
contract bindings typescript`), so it never drifts from the deployed ABI; this
package adds network presets, signer wiring, and the redeemer commitment.

```bash
npm install   # then: npm run build
```

## Read (no signer)

```ts
import { sorodeal } from "@sorodeal/sdk";

const c = sorodeal(); // testnet by default
const token = (await c.verify({ campaign_id: 1n, code: "DEMO0001" })).result.unwrap();
console.log(token.is_burned ? "BURNED" : "VALID");

const mine = (await c.campaigns_of({ owner: "G..." })).result; // bigint[]
```

## Write — server (keypair)

```ts
import { sorodeal, keypairSigner } from "@sorodeal/sdk";

const signer = keypairSigner(process.env.SECRET!); // S...
const c = sorodeal({ publicKey: signer.publicKey, signTransaction: signer.signTransaction });

const tx = await c.create_campaign({
  owner: signer.publicKey, name: "Cafe", discount_type: "percentage",
  discount_value: 1000n, total_supply: 100, valid_until: 9999999999n,
});
const campaignId = (await tx.signAndSend()).result.unwrap();
```

## Write — browser (Freighter)

```ts
import { sorodeal, freighterSigner, redeemerCommitment } from "@sorodeal/sdk";

const signer = await freighterSigner();
const c = sorodeal({ publicKey: signer.publicKey, signTransaction: signer.signTransaction });

// redeem: commit the redeemer reference off-chain (no PII on-chain)
const { hash } = await redeemerCommitment("order-8842");
const tx = await c.redeem_unique({ authorizer: signer.publicKey, campaign_id: 1n, code: "DEMO0001", redeemer_ref_hash: hash });
const receipt = (await tx.signAndSend()).result.unwrap();
```

## Tally (shared codes)

`register_shared` (fixes the payout token + rate), `commit_tally` (count +
`merkle_root` + per-attribution), `get_tally`/`compute_payouts` (public),
`settle` (pays attributed addresses). The off-chain signed-receipt / Merkle
layer is the integrator's responsibility — see `docs/SPEC.md` §10 (trust model).

## Low-level browser client (`freighterClient`)

The typed `Client` swallows the contract **error code** on simulation (view)
errors — `(await c.verify(...)).result.unwrapErr()` comes back with an empty
message instead of `CouponNotFound`. For browser dapps that need precise
per-error UX, this package also ships a hand-written low-level client that
surfaces it:

```ts
import { freighterClient } from "@sorodeal/sdk";

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
const { tx } = await c.registerShared(address, 1, "FALL25", null, null, "10000000");
```

This is the exact client the developer playground (`web/`) consumes — one
implementation, no drift. Reads work in Node too (Freighter is only loaded for
writes/`connectWallet`). The standalone `contractErrorCode(raw)` helper lets
typed-`Client` users recover the code the bindings drop.

## Notes

- Token amounts and `u64`/`i128` fields are `bigint` — never coerce to `Number`.
- Errors surface as the contract's `Errors` map (1–19), re-exported here.
- Default deployment is testnet (`TESTNET`); pass `contractId`/`rpcUrl`/`networkPassphrase` to target another.
