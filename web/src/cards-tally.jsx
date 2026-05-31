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
  const [attrCount, setAttrCount] = useState("30");
  const [merkle] = useState(() => SD.hex(64));
  const [busy, setBusy] = useState("");
  const [error, setError] = useState(null);
  const [result, setResult] = useState(null);

  useEffect(() => { if (highlight) setCid(String(highlight)); }, [highlight]);
  useEffect(() => { if (wallet && !attr) setAttr(wallet.address); }, [wallet]);

  const run = async (kind) => {
    setBusy(kind); setError(null); setResult(null);
    try {
      if (kind === "register") {
        await registerShared(cid, code.trim().toUpperCase(), attr.trim() || null, token.trim() || null, rate);
        toast({ kind: "success", title: "Shared code registered", msg: `${code.toUpperCase()} is live under campaign #${cid}.` });
        setResult({ kind: "register" });
      } else {
        const per = attr.trim() ? [[attr.trim(), Number(attrCount)]] : [];
        await commitTally(cid, code.trim().toUpperCase(), period, count, merkle, per);
        toast({ kind: "success", title: "Tally committed", msg: `Period ${period}: ${count} redemptions (${attrCount} attributed).` });
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
      `  --attributed_to '"${attr || "G…"}"' \\`,
      `  --payout_token '"${token || "C…"}"' \\`,
      `  --payout_rate ${rate || 0}`,
    ]),
    ts: `// Token + rate are FIXED at registration (immutable). Then commit\n// periodic tallies: commit_tally(campaign_id, code, period, count,\n// merkle_root, per_attribution) — Merkle root of the epoch's signed receipts.`,
    go: `client.RegisterShared(ctx, ${cid}, "${code.toUpperCase()}", "${attr || "G…"}", "${token || "C…"}", ${rate || 0})`,
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
          <span className="help">Of the total, how many credit the address above</span>
        </div>
        <div className="field">
          <label>Merkle root <span className="authtag" style={{ fontSize: 10, padding: "1px 6px" }}>demo</span></label>
          <div className="copy" style={{ padding: "10px 12px" }}>
            <span className="val mono" style={{ fontSize: 12 }}>{SD.trunc(merkle, 10, 8)}</span>
            <CopyBare value={merkle} display="" />
          </div>
          <span className="help">In production: the Merkle root of the epoch's signed receipts (auditable).</span>
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" loading={busy === "commit"} onClick={() => run("commit")} icon={<Ic.hash style={{ width: 15, height: 15 }} />}>Commit tally</Btn>
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
  const { wallet, getShared, getTally, computePayouts, settle, toast } = useApp();
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

  const snippets = {
    cli: tallyCli("get_tally", [`  --campaign_id ${cid} \\`, `  --code "${code.toUpperCase()}" \\`, `  --period ${period}`]),
    ts: `// get_shared / get_tally / compute_payouts are public reads (anyone audits).\n// settle(owner, campaign_id, code, period) pays each attributed address —\n// the token + rate are read from the registered code, not passed in.`,
    go: `tally, _ := client.GetTally(ctx, ${cid}, "${code.toUpperCase()}", ${period})`,
  };

  return (
    <ActionCard num="08" id="tally-settle" label="Audit & settle" title="Audit & settle" auth="public"
      desc="Read a committed tally (anyone can audit the count against the Merkle root), preview payouts at the code's fixed rate, and — as the owner — settle: pay each attributed address in the code's fixed token. Trustless attribution + automatic payout (ADR-004)."
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
        <span className="help">Public read — try campaign 1 · ROBERTOX · period 1</span>
      </div>

      {tally && (
        <ResultPanel title={`Tally — period ${tally.period}`} onClose={() => { setTally(null); setPayouts(null); setShared(null); }}>
          <KV k="count"><span className="mono" style={{ fontWeight: 600 }}>{tally.count}</span> <span className="faint" style={{ fontSize: 12 }}>redemptions committed</span></KV>
          <KV k="merkle_root"><CopyValue value={tally.merkle_root} display={SD.trunc(tally.merkle_root, 10, 8)} /></KV>
          {shared && (
            <KV k="settlement">
              {shared.payout_token
                ? <span><span className="mono" style={{ fontWeight: 600 }}>{Number(shared.payout_rate).toLocaleString()}</span> <span className="faint" style={{ fontSize: 12 }}>per redemption ·</span> <CopyBare value={shared.payout_token} display={SD.trunc(shared.payout_token, 4, 4)} /></span>
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
              ? payouts.map((p) => (<KV key={p.to} k={SD.trunc(p.to, 6, 6)}><span className="mono" style={{ fontWeight: 600 }}>{p.amount.toLocaleString()}</span></KV>))
              : <span className="faint" style={{ fontSize: 13 }}>No attributed payouts for this period.</span>}
          </>)}
        </ResultPanel>
      )}

      <div className="divider" style={{ margin: "10px 0 8px" }} />
      <div className="form-actions">
        <span className="authtag" style={{ marginRight: "auto" }}><Ic.lock />owner only · token &amp; rate from the registered code</span>
        <Btn variant="primary" loading={busy === "settle"} onClick={doSettle} disabled={!wallet} icon={<Ic.globe style={{ width: 15, height: 15 }} />}>{busy === "settle" ? "Settling" : "Settle & pay"}</Btn>
        <span className="help">{wallet ? "Transfers real tokens — once per period, irreversible" : "Connect Freighter to settle"}</span>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
    </ActionCard>
  );
}

Object.assign(window, { TallyRegister, TallySettle });
