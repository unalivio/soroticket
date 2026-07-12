/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — Tally profile (shared codes) · ADR-003/004/011
   Register a shared code (with a FIXED settlement token + rate) → commit
   periodic on-chain tallies (count + merkle_root + per-attribution) →
   audit / compute payouts / settle. Token + rate are immutable from
   registration, so settle can't lock a period with a bad payout.
   ═══════════════════════════════════════════════════════════════════ */

function tallyCli(method, lines) {
  return [
    "stellar contract invoke \\",
    "  --id <CONTRACT> \\",
    "  --source owner \\",
    "  --network testnet \\",
    "  -- " + method + " \\",
    ...lines,
  ].join("\n");
}

/* ── 07 · Register shared code (+ settlement config) + commit a tally ── */
function TallyRegister() {
  const { wallet, registerShared, commitTally, toast, highlight } = useApp();
  const SD = window.SD;
  const [cid, setCid] = useState("1");
  const [code, setCode] = useState("ROBERTOX");
  const [attr, setAttr] = useState("");
  const [token, setToken] = useState("");
  const [rate, setRate] = useState("50000");
  const [period, setPeriod] = useState("1");
  const [count, setCount] = useState("40");
  const [attrCount, setAttrCount] = useState("40");
  const [merkle, setMerkle] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState(null);
  const [result, setResult] = useState(null);

  const attributedArg = attr.trim() ? `'"${attr.trim()}"'` : "null";
  const tokenArg = attr.trim() && token.trim() ? `'"${token.trim()}"'` : "null";
  const effectiveRate = tokenArg === "null" ? "0" : (rate || "0");
  const goRegister = attr.trim()
    ? token.trim()
      ? `creator := "${attr.trim()}"\npayoutToken := "${token.trim()}"\nrate, _ := new(big.Int).SetString("${effectiveRate}", 10)\nerr := client.RegisterShared(ctx, ${cid}, "${code.toUpperCase()}", &creator, &payoutToken, rate)`
      : `creator := "${attr.trim()}"\nerr := client.RegisterShared(ctx, ${cid}, "${code.toUpperCase()}", &creator, nil, big.NewInt(0))`
    : `err := client.RegisterShared(ctx, ${cid}, "${code.toUpperCase()}", nil, nil, big.NewInt(0))`;

  useEffect(() => { if (highlight) setCid(String(highlight)); }, [highlight]);
  useEffect(() => { if (wallet && !attr) setAttr(wallet.address); }, [wallet]);

  const run = async (kind) => {
    setBusy(kind); setError(null); setResult(null);
    try {
      if (kind === "register") {
        // enforce the contract's invariants client-side: no payout without a
        // recipient, and rate 0 unless a token is set
        const attributed = attr.trim() || null;
        const tok = attributed ? (token.trim() || null) : null;
        const rt = tok ? rate : "0"; // keep as string — i128, never coerce to Number
        await registerShared(cid, code.trim().toUpperCase(), attributed, tok, rt);
        toast({ kind: "success", title: "Shared code registered", msg: `${code.toUpperCase()} is live under campaign #${cid}.` });
        setResult({ kind: "register" });
      } else {
        if (!/^[0-9a-fA-F]{64}$/.test(merkle.trim())) {
          const invalid = new Error("A real 32-byte Merkle root is required.");
          invalid.sd = { title: "Invalid Merkle root", msg: invalid.message, code: "—" };
          throw invalid;
        }
        if (attr.trim() && Number(attrCount) !== Number(count)) {
          const invalid = new Error("For an attributed code, attributed count must equal total redemptions.");
          invalid.sd = { title: "Invalid tally", msg: invalid.message, code: 17 };
          throw invalid;
        }
        const per = attr.trim() ? [[attr.trim(), Number(attrCount)]] : [];
        const attributed = per.length ? attrCount : 0; // toast must match what's actually sent
        await commitTally(cid, code.trim().toUpperCase(), period, count, merkle.trim(), per);
        toast({ kind: "success", title: "Tally committed", msg: `Period ${period}: ${count} redemptions (${attributed} attributed).` });
        setResult({ kind: "commit" });
      }
    } catch (err) { setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg }); }
    finally { setBusy(""); }
  };

  const snippets = {
    cli: tallyCli("register_shared", [
      `  --owner ${wallet ? wallet.address : "G…"} \\`,
      `  --campaign_id ${cid} \\`,
      `  --code "${code.toUpperCase()}" \\`,
      `  --attributed_to ${attributedArg} \\`,
      `  --payout_token ${tokenArg} \\`,
      `  --payout_rate ${effectiveRate}`,
    ]),
    ts: `// Token + rate are FIXED at registration (immutable). Then commit\n// periodic tallies: commit_tally(campaign_id, code, period, count,\n// merkle_root, per_attribution) — Merkle root of the epoch's signed receipts.`,
    go: `// import "math/big"\n${goRegister}\nif err != nil { return err }`,
  };

  return (
    <ActionCard num="07" id="tally-register" label="Register shared code" title="Register shared code" auth="owner"
      desc="Tally profile: register a shared, multi-use code (e.g. a creator code), crediting a creator/referrer and fixing the settlement token + rate up front (immutable). Then commit periodic on-chain tallies of off-chain redemptions — count + a Merkle root + per-attribution."
      locked={!wallet} snippets={snippets}>
      <div className="form-grid">
        <div className="field">
          <label>Campaign ID</label>
          <input className="input mono" type="number" min="1" value={cid} onChange={(e) => setCid(e.target.value)} />
        </div>
        <div className="field">
          <label>Shared code</label>
          <input className="input mono" value={code} onChange={(e) => setCode(e.target.value)} placeholder="ROBERTOX" />
        </div>
        <div className="field span2">
          <label>Attributed to <span className="authtag" style={{ fontSize: 10, padding: "1px 6px" }}>creator / referrer</span></label>
          <input className="input mono" value={attr} onChange={(e) => setAttr(e.target.value)} placeholder="G… (paid at settlement)" />
          <span className="help">The only address this code can credit (binding). Blank = an unattributed promo.</span>
        </div>
        <div className="field">
          <label>Payout token <span className="authtag" style={{ fontSize: 10, padding: "1px 6px" }}>optional</span></label>
          <input className="input mono" value={token} onChange={(e) => setToken(e.target.value)} placeholder="C… (USDC / test SAC)" />
          <span className="help">Fixed now, immutable. Blank = count-only (no settlement).</span>
        </div>
        <div className="field">
          <label>Payout rate</label>
          <input className="input mono" type="number" min="1" value={rate} onChange={(e) => setRate(e.target.value)} />
          <span className="help">Token base-units per attributed redemption (must be &gt; 0 if a token is set).</span>
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" small loading={busy === "register"} onClick={() => run("register")} icon={<Ic.plus style={{ width: 13, height: 13 }} />}>Register code</Btn>
      </div>

      <div className="divider" style={{ margin: "8px 0" }} />
      <div className="kv-k" style={{ marginBottom: 8 }}>Commit a period tally</div>
      <div className="form-grid">
        <div className="field">
          <label>Period</label>
          <input className="input mono" type="number" min="1" value={period} onChange={(e) => setPeriod(e.target.value)} />
        </div>
        <div className="field">
          <label>Total redemptions</label>
          <input className="input mono" type="number" min="0" value={count} onChange={(e) => setCount(e.target.value)} />
        </div>
        <div className="field">
          <label>Attributed to creator</label>
          <input className="input mono" type="number" min="0" value={attrCount} onChange={(e) => setAttrCount(e.target.value)} />
          <span className="help">For an attributed code this must equal the total; partial attribution is rejected.</span>
        </div>
        <div className="field">
          <label>Merkle root <span className="req">*</span></label>
          <input className="input mono" value={merkle} onChange={(e) => setMerkle(e.target.value)} placeholder="64 hex characters" />
          <span className="help">SHA-256 Merkle root computed from the epoch's signed receipt payloads; no random/demo roots.</span>
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" loading={busy === "commit"} onClick={() => run("commit")} disabled={!/^[0-9a-fA-F]{64}$/.test(merkle.trim()) || (!!attr.trim() && Number(attrCount) !== Number(count))} icon={<Ic.hash style={{ width: 15, height: 15 }} />}>Commit tally</Btn>
        <span className="help">Append-only — one commitment per period</span>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
      {result && (
        <ResultPanel title={result.kind === "register" ? "Shared code registered" : "Tally committed"} onClose={() => setResult(null)}>
          <span style={{ fontSize: 14, color: "var(--ink-2)" }}>
            {result.kind === "register"
              ? `${code.toUpperCase()} registered under campaign #${cid}.`
              : `Period ${period} committed: ${count} redemptions, ${attrCount} attributed.`}
          </span>
        </ResultPanel>
      )}
    </ActionCard>
  );
}

/* ── 08 · Audit a tally + compute payouts + settle ───────────── */
function TallySettle() {
  const { wallet, getShared, getTally, computePayouts, settle, bumpTally, toast } = useApp();
  const SD = window.SD;
  const [cid, setCid] = useState("1");
  const [code, setCode] = useState("ROBERTOX");
  const [period, setPeriod] = useState("1");
  const [busy, setBusy] = useState("");
  const [shared, setShared] = useState(null);
  const [tally, setTally] = useState(null);
  const [payouts, setPayouts] = useState(null);
  const [error, setError] = useState(null);

  const view = async () => {
    setBusy("view"); setError(null); setPayouts(null); setTally(null); setShared(null);
    try {
      const c = code.trim().toUpperCase();
      const { result: sh } = await getShared(cid, c);
      const { result: t } = await getTally(cid, c, period);
      const { result: p } = await computePayouts(cid, c, period);
      setShared(sh); setTally(t); setPayouts(p);
    } catch (err) { setError(err.sd); }
    finally { setBusy(""); }
  };

  const doSettle = async () => {
    setBusy("settle"); setError(null);
    try {
      const { result } = await settle(cid, code.trim().toUpperCase(), period);
      toast({ kind: "success", title: "Settled", msg: `${result.payouts.length} payout(s) sent.` });
      setPayouts(result.payouts);
    } catch (err) { setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg }); }
    finally { setBusy(""); }
  };

  const doBump = async () => {
    setBusy("bump"); setError(null);
    try {
      await bumpTally(cid, code.trim().toUpperCase(), period);
      toast({ kind: "success", title: "TTL extended", msg: `Storage for ${code.toUpperCase()} (period ${period}) re-extended.` });
    } catch (err) { setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg }); }
    finally { setBusy(""); }
  };

  const snippets = {
    cli: tallyCli("get_tally", [`  --campaign_id ${cid} \\`, `  --code "${code.toUpperCase()}" \\`, `  --period ${period}`]),
    ts: `// Public reads expose the root and payout preview. A complete audit also\n// needs the signed receipts and Merkle inclusion proofs. Contract v0.2 lets any\n// fee payer settle after the owner approves a bounded token allowance.`,
    go: `tally, _ := client.GetTally(ctx, ${cid}, "${code.toUpperCase()}", ${period})`,
  };

  return (
    <ActionCard num="08" id="tally-settle" label="Audit & settle" title="Audit & settle" auth="public"
      desc="Read a committed tally and preview payouts. Verifying the root requires the signed receipt set and inclusion proofs; the signer remains the trust anchor for genuine off-chain events. This legacy v0.1 playground still requires the owner to settle."
      snippets={snippets}>
      <div className="form-grid">
        <div className="field">
          <label>Campaign ID</label>
          <input className="input mono" type="number" min="1" value={cid} onChange={(e) => setCid(e.target.value)} />
        </div>
        <div className="field">
          <label>Shared code</label>
          <input className="input mono" value={code} onChange={(e) => setCode(e.target.value)} placeholder="ROBERTOX" />
        </div>
        <div className="field">
          <label>Period</label>
          <input className="input mono" type="number" min="1" value={period} onChange={(e) => setPeriod(e.target.value)} />
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" loading={busy === "view"} onClick={view}>{busy === "view" ? "Reading" : "View tally & payouts"}</Btn>
        <span className="help">Public read on the legacy seeded deployment; its 40/30 tally demonstrates the v0.1 underpayment flaw and is rejected by v0.2.</span>
      </div>

      {tally && (
        <ResultPanel title={`Tally — period ${tally.period}`} onClose={() => { setTally(null); setPayouts(null); setShared(null); }}>
          <KV k="count"><span className="mono" style={{ fontWeight: 600 }}>{tally.count}</span> <span className="faint" style={{ fontSize: 12 }}>redemptions committed</span></KV>
          <KV k="merkle_root"><CopyValue value={tally.merkle_root} display={SD.trunc(tally.merkle_root, 10, 8)} /></KV>
          {shared && (
            <KV k="settlement">
              {shared.payout_token
                ? <span><span className="mono" style={{ fontWeight: 600 }}>{shared.payout_rate}</span> <span className="faint" style={{ fontSize: 12 }}>per redemption ·</span> <CopyBare value={shared.payout_token} display={SD.trunc(shared.payout_token, 4, 4)} /></span>
                : <span className="faint">count-only (no payout token)</span>}
            </KV>
          )}
          <KV k="attribution">
            {tally.per_attribution.length
              ? <div className="chips">{tally.per_attribution.map(([a, n]) => (<span key={a} className="chip-tok"><CopyBare value={a} display={SD.trunc(a, 4, 4)} /><span className="tid">×{n}</span></span>))}</div>
              : <span className="faint">none</span>}
          </KV>
          {payouts && (<>
            <div className="divider" style={{ margin: "6px 0 2px" }} />
            <div className="kv-k" style={{ marginBottom: 6 }}>Payouts (at the code's fixed rate)</div>
            {payouts.length
              ? payouts.map((p) => (<KV key={p.to} k={SD.trunc(p.to, 6, 6)}><span className="mono" style={{ fontWeight: 600 }}>{p.amount}</span></KV>))
              : <span className="faint" style={{ fontSize: 13 }}>No attributed payouts for this period.</span>}
          </>)}
        </ResultPanel>
      )}

      <div className="divider" style={{ margin: "10px 0 8px" }} />
      <div className="form-actions">
        <span className="authtag" style={{ marginRight: "auto" }}><Ic.lock />owner only · token &amp; rate from the registered code</span>
        <button className="btn-plain" onClick={doBump} disabled={!wallet || busy === "bump"}>{busy === "bump" ? <Spinner size={14} /> : <Ic.hash style={{ width: 14, height: 14 }} />} Bump TTL</button>
        <Btn variant="primary" loading={busy === "settle"} onClick={doSettle} disabled={!wallet} icon={<Ic.globe style={{ width: 15, height: 15 }} />}>{busy === "settle" ? "Settling" : "Settle & pay"}</Btn>
        <span className="help">{wallet ? "Transfers real tokens — once per period, irreversible" : "Connect Freighter to settle"}</span>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
    </ActionCard>
  );
}

Object.assign(window, { TallyRegister, TallySettle });
