/* ═══════════════════════════════════════════════════════════════════
   Soroticket Playground — data layer (plain JS, attached to window.SD)
   Testnet constants, empty UI state, snippet generators.
   Mirrors contracts/coupon-ledger/src/lib.rs (Burn profile).
   ═══════════════════════════════════════════════════════════════════ */
(function () {
  const NET = {
    network: "testnet",
    passphrase: "Test SDF Network ; September 2015",
    rpc: "https://soroban-testnet.stellar.org",
    contractId: "CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF",
    contractStatus: "v0.2.0-testnet-preview",
    ownerWallet: "GCIJM67CD2U6XPI5GYS5VYSNIYOKLH7DZ4XA3W45PID7DYCRYFTRDSV6",
    explorer: "https://stellar.expert/explorer/testnet",
  };

  // ── truncation: G…SV6 / CABN…OZP style ──────────────────────
  function trunc(s, head = 4, tail = 4) {
    if (!s || s.length <= head + tail + 1) return s;
    return s.slice(0, head) + "…" + s.slice(-tail);
  }

  // ── random per-redemption nonce (the salt) ──────────────────
  function randomNonce() {
    const a = new Uint8Array(16);
    if (typeof crypto === "undefined" || !crypto.getRandomValues) {
      throw new Error("Secure randomness is unavailable in this browser context.");
    }
    crypto.getRandomValues(a);
    return [...a].map((b) => b.toString(16).padStart(2, "0")).join("");
  }

  // ── opaque, non-reversible commitment → 64 hex (redeemer_ref_hash) ──
  //   SHA-256(nonce ∥ reference) with a RANDOM per-redemption nonce (ADR-010).
  //   A public/constant salt would be brute-forceable for low-entropy refs
  //   (email / phone / order#); a random nonce makes the on-chain value
  //   non-reversible and unlinkable. The nonce lives off-chain — only the hash
  //   is sent on-chain. (Production may instead HMAC with a merchant pepper.)
  async function redeemerHash(ref, salt) {
    const s = salt || randomNonce();
    const data = new TextEncoder().encode(s + "|" + (ref || ""));
    if (typeof crypto === "undefined" || !crypto.subtle) {
      throw new Error("WebCrypto SHA-256 is unavailable in this browser context.");
    }
    const buf = await crypto.subtle.digest("SHA-256", data);
    return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
  }

  // ── format discount value (cents) for humans ────────────────
  function fmtDiscount(type, valueCents) {
    const v = Number(valueCents) || 0;
    if (type === "percentage") return (v / 100).toFixed(2) + "%";
    if (type === "fixed_amount") return "$" + (v / 100).toFixed(2);
    return "Free item";
  }
  const DISCOUNT_LABEL = {
    percentage: "Percentage",
    fixed_amount: "Fixed amount",
    free_item: "Free item",
  };

  // ── time helpers ────────────────────────────────────────────
  function unix(dt) { return Math.floor(new Date(dt).getTime() / 1000); }
  function nowUnix() { return Math.floor(Date.now() / 1000); }
  function fmtTs(u) {
    if (!u) return "—";
    const d = new Date(u * 1000);
    return d.toISOString().replace("T", " ").slice(0, 19) + "Z";
  }
  function fmtClock(d) {
    return new Date(d).toLocaleTimeString("en-GB", { hour12: false });
  }

  // UI mirrors only values actually read from or written to the contract.
  // There is intentionally no seeded/demo chain state.
  function initialState() {
    return {
      campCtr: 0,
      tokenCtr: 0,
      campaigns: {},
      tokens: {},
      delegates: {}, // `${campaignId}:${addr}` -> true
    };
  }
  const ZERO32 = "0".repeat(64);

  // ── friendly error mapping (matches contracterror enum) ─────
  const ERRORS = {
    CampaignNotFound:  { code: 1, title: "Campaign not found",   msg: "No campaign exists with that ID on this contract." },
    CouponNotFound:    { code: 2, title: "Code not found",       msg: "No coupon with that code has been issued." },
    AlreadyRedeemed:   { code: 3, title: "Already redeemed",     msg: "This code was already redeemed — single-use codes can only be burned once." },
    CampaignExpired:   { code: 4, title: "Campaign expired",     msg: "This campaign is past its valid-until date and can no longer be redeemed." },
    SupplyExhausted:   { code: 5, title: "Supply exhausted",     msg: "Issuing these codes would exceed the campaign's total supply." },
    Unauthorized:      { code: 6, title: "Unauthorized",         msg: "Only the campaign owner (or a delegate) can do this." },
    InvalidCode:       { code: 7, title: "Invalid code",         msg: "Provide at least one non-empty coupon code." },
    DuplicateCode:     { code: 8, title: "Duplicate code",       msg: "That code was already issued in this campaign — codes are unique per campaign." },
    InvalidTerms:      { code: 9, title: "Invalid terms",        msg: "Supply must be > 0, expiry must be in the future, name must be 1–96 UTF-8 bytes, and discount type must be 1–32 UTF-8 bytes." },
    BatchTooLarge:     { code: 10, title: "Batch too large",     msg: "Issue at most 100 codes per call." },
    CodeTooLong:       { code: 11, title: "Code too long",       msg: "Coupon codes can be at most 64 UTF-8 bytes." },
    SharedNotFound:    { code: 12, title: "Shared code not found", msg: "No shared code is registered with that name in this campaign." },
    AlreadyRegistered: { code: 13, title: "Already registered",   msg: "That shared code is already registered in this campaign." },
    PeriodCommitted:   { code: 14, title: "Period already committed", msg: "A tally for this period was already committed — commitments are append-only." },
    TallyNotFound:     { code: 15, title: "Tally not found",      msg: "No tally has been committed for that code and period." },
    AlreadySettled:    { code: 16, title: "Already settled",      msg: "This tally period was already settled." },
    InvalidTally:      { code: 17, title: "Invalid tally",        msg: "For an attributed code, the attributed count must equal the committed total." },
    InvalidSettlement: { code: 18, title: "Invalid settlement",   msg: "Inconsistent settlement config: a payout token requires rate > 0, no token requires rate 0, and an unattributed code can't set a payout token (or the amount overflowed)." },
    AttributionMismatch: { code: 19, title: "Attribution mismatch", msg: "An attributed code can only credit its registered creator/referrer; an unattributed code can't carry per-attribution counts." },
  };

  // ═══════════════════════════════════════════════════════════
  //  CODE SNIPPET GENERATORS — reflect current form inputs
  //  Languages: stellar CLI · TypeScript (@stellar/stellar-sdk) · Go
  // ═══════════════════════════════════════════════════════════
  const C = NET.contractId;

  function tsHeader() {
    return [
      `import {`,
      `  Contract, Keypair, TransactionBuilder, nativeToScVal, scValToNative,`,
      `  rpc, Networks, BASE_FEE,`,
      `} from "@stellar/stellar-sdk";`,
      ``,
      `const ownerKeypair = Keypair.fromSecret(process.env.SOROTICKET_SECRET);`,
      `const owner = ownerKeypair.publicKey();`,
      `const server = new rpc.Server("${NET.rpc}");`,
      `const contract = new Contract("${C}");`,
      `const source = await server.getAccount(owner);`,
      ``,
    ].join("\n");
  }
  function goHeader() {
    return [
      `// go get github.com/soroticket/soroticket-go`,
      `kp := keypair.MustParseFull(os.Getenv("SOROTICKET_SECRET"))`,
      `// Testnet preset: current v0.2.0 testnet deployment.`,
      `client, err := soroticket.Testnet(kp)`,
      `if err != nil { return err }`,
      `defer client.Close()`,
      ``,
    ].join("\n");
  }

  // helper to build a TS invocation
  function tsInvoke(method, argsLines) {
    return (
      tsHeader() +
      `const tx = new TransactionBuilder(source, {\n` +
      `  fee: BASE_FEE,\n` +
      `  networkPassphrase: Networks.TESTNET,\n` +
      `})\n` +
      `  .addOperation(contract.call(\n` +
      `    "${method}",\n` +
      argsLines.map((l) => "    " + l).join("\n") + (argsLines.length ? "\n" : "") +
      `  ))\n` +
      `  .setTimeout(30)\n` +
      `  .build();\n\n` +
      `const prepared = await server.prepareTransaction(tx);\n` +
      `prepared.sign(ownerKeypair);\n` +
      `const submitted = await server.sendTransaction(prepared);\n` +
      `if (submitted.status === "ERROR") throw new Error("transaction rejected");\n` +
      `let result;\n` +
      `do {\n` +
      `  await new Promise((resolve) => setTimeout(resolve, 1000));\n` +
      `  result = await server.getTransaction(submitted.hash);\n` +
      `} while (result.status === "NOT_FOUND");\n` +
      `if (result.status !== "SUCCESS") throw new Error("transaction failed");\n` +
      `console.log(scValToNative(result.returnValue));`
    );
  }

  const SNIP = {
    create_campaign(f) {
      const validUntil = f.valid_until ? unix(f.valid_until) : "VALID_UNTIL_UNIX";
      const cli = [
        `stellar contract invoke \\`,
        `  --id ${C} \\`,
        `  --source owner \\`,
        `  --network testnet \\`,
        `  -- create_campaign \\`,
        `  --owner ${NET.ownerWallet} \\`,
        `  --name '${f.name || "Demo Cafe"}' \\`,
        `  --discount_type ${f.discount_type} \\`,
        `  --discount_value ${f.discount_value || 1000} \\`,
        `  --total_supply ${f.total_supply || 100} \\`,
        `  --valid_until ${validUntil}`,
      ].join("\n");

      const ts = tsInvoke("create_campaign", [
        `nativeToScVal(owner, { type: "address" }),`,
        `nativeToScVal("${f.name || "Demo Cafe"}", { type: "string" }),`,
        `nativeToScVal("${f.discount_type}", { type: "string" }),`,
        `nativeToScVal(${f.discount_value || 1000}n, { type: "u64" }),`,
        `nativeToScVal(${f.total_supply || 100}, { type: "u32" }),`,
        `nativeToScVal(${validUntil}n, { type: "u64" }),`,
      ]);

      const go = goHeader() + [
        `campaignID, err := client.CreateCampaign(ctx,`,
        `    "${f.name || "Demo Cafe"}", "${f.discount_type}",`,
        `    ${f.discount_value || 1000}, ${f.total_supply || 100}, ${validUntil})`,
        `if err != nil { return err }`,
        `log.Printf("campaign_id = %d", campaignID)`,
      ].join("\n");

      return { cli, ts, go };
    },

    issue_unique(f) {
      const codes = (f.codes && f.codes.length ? f.codes : ["DEMO0003", "DEMO0004"]);
      const cli = [
        `stellar contract invoke \\`,
        `  --id ${C} \\`,
        `  --source owner \\`,
        `  --network testnet \\`,
        `  -- issue_unique \\`,
        `  --owner ${NET.ownerWallet} \\`,
        `  --campaign_id ${f.campaign_id || 1} \\`,
        `  --codes '[${codes.map((c) => `"${c}"`).join(", ")}]'`,
      ].join("\n");

      const ts = tsInvoke("issue_unique", [
        `nativeToScVal(owner, { type: "address" }),`,
        `nativeToScVal(${f.campaign_id || 1}n, { type: "u64" }),`,
        `nativeToScVal(`,
        `  [${codes.map((c) => `"${c}"`).join(", ")}].map(c =>`,
        `    nativeToScVal(c, { type: "string" })), { type: "vec" }),`,
      ]);

      const go = goHeader() + [
        `tokenIDs, err := client.IssueUnique(ctx, ${f.campaign_id || 1},`,
        `    []string{${codes.map((c) => `"${c}"`).join(", ")}})`,
        `if err != nil { return err }`,
        `log.Printf("issued token_ids = %v", tokenIDs)`,
      ].join("\n");

      return { cli, ts, go };
    },

    redeem_unique(f) {
      const code = f.code || "DEMO0001";
      const cid = f.campaign_id || 1;
      const cli = [
        `# redeemer_ref_hash = SHA-256(random nonce ∥ reference), computed off-chain.`,
        `stellar contract invoke \\`,
        `  --id ${C} \\`,
        `  --source authorizer \\`,
        `  --network testnet \\`,
        `  -- redeem_unique \\`,
        `  --authorizer ${NET.ownerWallet} \\`,
        `  --campaign_id ${cid} \\`,
        `  --code "${code}" \\`,
        `  --redeemer_ref_hash ${f.hash || "<64-hex commitment>"}`,
      ].join("\n");

      const ts =
        `// Commit the redeemer ref off-chain — random nonce, no PII on-chain (ADR-010).\n` +
        `const nonce = crypto.getRandomValues(new Uint8Array(16));\n` +
        `const enc = new TextEncoder().encode(\n` +
        `  [...nonce].map(b => b.toString(16).padStart(2,"0")).join("") + "|" + redeemerRef);\n` +
        `const refHash = Buffer.from(await crypto.subtle.digest("SHA-256", enc)); // 32 bytes\n\n` +
        tsInvoke("redeem_unique", [
          `nativeToScVal(authorizer, { type: "address" }),`,
          `nativeToScVal(${cid}n, { type: "u64" }),`,
          `nativeToScVal("${code}", { type: "string" }),`,
          `nativeToScVal(refHash, { type: "bytes" }),`,
        ]);

      const go = goHeader() + [
        `// Randomized commitment — never plaintext PII on-chain (ADR-010).`,
        `commitment, nonce, err := soroticket.RedeemerCommitment(redeemerRef)`,
        `if err != nil { return err }`,
        `_ = nonce // store off-chain only if later proof is required`,
        ``,
        `receipt, err := client.RedeemUnique(ctx, ${cid}, "${code}", commitment)`,
        `if err != nil { return err }`,
        `log.Printf("burned token=%d ledger=%d", receipt.TokenID, receipt.LedgerSeq)`,
      ].join("\n");

      return { cli, ts, go };
    },

    verify(f) {
      const code = f.code || "DEMO0001";
      const cid = f.campaign_id || 1;
      const cli = [
        `# Public read — no wallet, no signature required.`,
        `stellar contract invoke \\`,
        `  --id ${C} \\`,
        `  --source any \\`,
        `  --network testnet \\`,
        `  -- verify \\`,
        `  --campaign_id ${cid} \\`,
        `  --code "${code}"`,
      ].join("\n");

      const ts =
        `import { Contract, rpc, scValToNative, nativeToScVal } from "@stellar/stellar-sdk";\n\n` +
        `const server = new rpc.Server("${NET.rpc}");\n` +
        `const contract = new Contract("${C}");\n\n` +
        `// verify() is read-only — simulate, no signing needed.\n` +
        `const sim = await server.simulateTransaction(\n` +
        `  buildReadTx(contract.call("verify",\n` +
        `    nativeToScVal(${cid}n, { type: "u64" }),\n` +
        `    nativeToScVal("${code}", { type: "string" }))));\n` +
        `const token = scValToNative(sim.result.retval);\n` +
        `console.log(token.is_burned ? "BURNED" : "VALID");`;

      const go = goHeader() + [
        `// Read-only — simulate against the RPC, no signature.`,
        `token, err := client.Verify(ctx, ${cid}, "${code}")`,
        `if err != nil { return err }`,
        `if token.IsBurned {`,
        `    log.Printf("BURNED, ref=%x", token.RedeemerRef)`,
        `} else {`,
        `    log.Printf("VALID — token %d", token.TokenID)`,
        `}`,
      ].join("\n");

      return { cli, ts, go };
    },

    add_delegate(f) {
      const cli = [
        `stellar contract invoke \\`,
        `  --id ${C} \\`,
        `  --source owner \\`,
        `  --network testnet \\`,
        `  -- add_delegate \\`,
        `  --owner ${NET.ownerWallet} \\`,
        `  --campaign_id ${f.campaign_id || 1} \\`,
        `  --delegate ${f.delegate || "G…"}`,
      ].join("\n");
      const ts = tsInvoke("add_delegate", [
        `nativeToScVal(owner, { type: "address" }),`,
        `nativeToScVal(${f.campaign_id || 1}n, { type: "u64" }),`,
        `nativeToScVal("${f.delegate || "G…"}", { type: "address" }),`,
      ]);
      const go = goHeader() + [
        `err := client.AddDelegate(ctx, ${f.campaign_id || 1}, "${f.delegate || "G…"}")`,
      ].join("\n");
      return { cli, ts, go };
    },
  };

  window.SD = {
    NET, ZERO32, ERRORS, DISCOUNT_LABEL, SNIP,
    trunc, randomNonce, redeemerHash, fmtDiscount, unix, nowUnix, fmtTs, fmtClock, initialState,
  };
})();
