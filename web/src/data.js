/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — data layer (plain JS, attached to window.SD)
   Testnet constants, simulated contract state, snippet generators.
   Mirrors contracts/coupon-ledger/src/lib.rs (Burn profile).
   ═══════════════════════════════════════════════════════════════════ */
(function () {
  const NET = {
    network: "testnet",
    passphrase: "Test SDF Network ; September 2015",
    rpc: "https://soroban-testnet.stellar.org",
    contractId: "CAGBCE2ETELUJM2CCPMMTTP6XEZQN26LD7TMURRA62UQY7WT7WQ2LZBR",
    ownerWallet: "GCIJM67CD2U6XPI5GYS5VYSNIYOKLH7DZ4XA3W45PID7DYCRYFTRDSV6",
    explorer: "https://stellar.expert/explorer/testnet",
  };

  // ── truncation: G…SV6 / CABN…OZP style ──────────────────────
  function trunc(s, head = 4, tail = 4) {
    if (!s || s.length <= head + tail + 1) return s;
    return s.slice(0, head) + "…" + s.slice(-tail);
  }

  // ── random hex (tx hashes, etc) ─────────────────────────────
  function hex(n) {
    const c = "0123456789abcdef";
    let o = "";
    for (let i = 0; i < n; i++) o += c[(Math.random() * 16) | 0];
    return o;
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

  // ── seed contract state — Demo Cafe (so Verify works offline) ─
  function seedState() {
    const minted = nowUnix() - 60 * 60 * 26; // issued ~26h ago
    const validUntil = nowUnix() + 60 * 60 * 24 * 30; // +30d
    return {
      campCtr: 1,
      tokenCtr: 2,
      campaigns: {
        1: {
          id: 1,
          owner: NET.ownerWallet,
          name: "Demo Cafe",
          discount_type: "percentage",
          discount_value: 1000,
          total_supply: 100,
          minted: 2,
          burned: 0,
          valid_until: validUntil,
        },
      },
      tokens: {
        "1:DEMO0001": { token_id: 1, campaign_id: 1, code: "DEMO0001", is_burned: false, minted_at: minted, redeemer_ref: ZERO32, burned_at: 0 },
        "1:DEMO0002": { token_id: 2, campaign_id: 1, code: "DEMO0002", is_burned: false, minted_at: minted, redeemer_ref: ZERO32, burned_at: 0 },
      },
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
    InvalidTerms:      { code: 9, title: "Invalid terms",        msg: "Supply must be > 0, expiry must be in the future, name must be 1–96 characters, and discount type must be 1–32 characters." },
    BatchTooLarge:     { code: 10, title: "Batch too large",     msg: "Issue at most 100 codes per call." },
    CodeTooLong:       { code: 11, title: "Code too long",       msg: "Coupon codes can be at most 64 characters." },
    SharedNotFound:    { code: 12, title: "Shared code not found", msg: "No shared code is registered with that name in this campaign." },
    AlreadyRegistered: { code: 13, title: "Already registered",   msg: "That shared code is already registered in this campaign." },
    PeriodCommitted:   { code: 14, title: "Period already committed", msg: "A tally for this period was already committed — commitments are append-only." },
    TallyNotFound:     { code: 15, title: "Tally not found",      msg: "No tally has been committed for that code and period." },
    AlreadySettled:    { code: 16, title: "Already settled",      msg: "This tally period was already settled." },
    InvalidTally:      { code: 17, title: "Invalid tally",        msg: "Per-attribution counts cannot exceed the committed total count." },
  };

  // ═══════════════════════════════════════════════════════════
  //  CODE SNIPPET GENERATORS — reflect current form inputs
  //  Languages: stellar CLI · TypeScript (@stellar/stellar-sdk) · Go
  // ═══════════════════════════════════════════════════════════
  const C = NET.contractId;

  function tsHeader() {
    return [
      `import {`,
      `  Contract, TransactionBuilder, nativeToScVal, scValToNative,`,
      `  rpc, Networks, BASE_FEE,`,
      `} from "@stellar/stellar-sdk";`,
      ``,
      `const server = new rpc.Server("${NET.rpc}");`,
      `const contract = new Contract("${C}");`,
      `const source = await server.getAccount(owner);`,
      ``,
    ].join("\n");
  }
  function goHeader() {
    return [
      `// go get github.com/stellar/go/...`,
      `client := sorobanclient.New("${NET.rpc}")`,
      `contractID := "${C}"`,
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
      `const res = await server.sendTransaction(prepared);\n` +
      `console.log(scValToNative(res.returnValue));`
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
        `campaignID, err := client.CreateCampaign(ctx, sorodeal.CampaignTerms{`,
        `    Owner:         "${NET.ownerWallet}",`,
        `    Name:          "${f.name || "Demo Cafe"}",`,
        `    DiscountType:  "${f.discount_type}",`,
        `    DiscountValue: ${f.discount_value || 1000}, // cents`,
        `    TotalSupply:   ${f.total_supply || 100},`,
        `    ValidUntil:    ${validUntil},`,
        `})`,
        `if err != nil { return err }`,
        `log.Printf("campaign_id = %d", campaignID)`,
      ].join("\n");

      return { cli, ts, go };
    },

    issue_unique(f) {
      const codes = (f.codes && f.codes.length ? f.codes : ["DEMO0003", "DEMO0004"]);
      const cliCodes = codes.map((c) => `"${c}"`).join(" ");
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
        `tokenIDs, err := client.IssueUnique(ctx, sorodeal.IssueParams{`,
        `    Owner:      "${NET.ownerWallet}",`,
        `    CampaignID: ${f.campaign_id || 1},`,
        `    Codes:      []string{${codes.map((c) => `"${c}"`).join(", ")}},`,
        `})`,
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
        `// SHA-256(random nonce ∥ reference) — never plaintext PII (ADR-010).`,
        `nonce := make([]byte, 16); rand.Read(nonce)`,
        `sum := sha256.Sum256(append(nonce, []byte("|"+redeemerRef)...))`,
        ``,
        `receipt, err := client.RedeemUnique(ctx, sorodeal.RedeemParams{`,
        `    Authorizer:      "${NET.ownerWallet}",`,
        `    CampaignID:      ${cid},`,
        `    Code:            "${code}",`,
        `    RedeemerRefHash: sum[:], // [32]byte`,
        `})`,
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
        `err := client.AddDelegate(ctx, sorodeal.DelegateParams{`,
        `    Owner:      "${NET.ownerWallet}",`,
        `    CampaignID: ${f.campaign_id || 1},`,
        `    Delegate:   "${f.delegate || "G…"}",`,
        `})`,
      ].join("\n");
      return { cli, ts, go };
    },
  };

  window.SD = {
    NET, ZERO32, ERRORS, DISCOUNT_LABEL, SNIP,
    trunc, hex, randomNonce, redeemerHash, fmtDiscount, unix, nowUnix, fmtTs, fmtClock, seedState,
  };
})();
