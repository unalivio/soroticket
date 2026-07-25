/* ═══════════════════════════════════════════════════════════════════
   Soroticket Playground — Redeem · Verify / Inspect
   ═══════════════════════════════════════════════════════════════════ */

/* ── 4 · Redeem ────────────────────────────────────────────── */
function Redeem() {
  const { wallet, redeem, toast, highlight } = useApp();
  const SD = window.SD;
  const makeNonce = () => {
    try { return SD.randomNonce(); }
    catch (_) { return ""; }
  };
  const [cid, setCid] = useState("1");
  const [code, setCode] = useState("DEMO0001");
  const [ref, setRef] = useState("order-8842");
  const [nonce, setNonce] = useState(makeNonce);
  const [hash, setHash] = useState(SD.ZERO32);
  const [hashError, setHashError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);

  // follow the campaign you most recently created/issued
  useEffect(() => { if (highlight) setCid(String(highlight)); }, [highlight]);

  // compute the commitment in-browser, live: SHA-256(random nonce ∥ ref) (ADR-010)
  useEffect(() => {
    let live = true;
    setHashError(null);
    if (!ref.trim() || !nonce) { setHash(SD.ZERO32); return; }
    SD.redeemerHash(ref, nonce)
      .then((h) => { if (live) setHash(h); })
      .catch((e) => {
        if (!live) return;
        setHash(SD.ZERO32);
        setHashError(e.message || "Could not compute the redeemer commitment.");
      });
    return () => { live = false; };
  }, [ref, nonce]);

  const submit = async () => {
    setBusy(true); setError(null); setResult(null);
    try {
      const { result: r } = await redeem(cid, code.trim().toUpperCase(), hash);
      setResult(r);
      setNonce(makeNonce()); // fresh nonce for the next redemption
      toast({ kind: "success", title: "Coupon redeemed", msg: `${code.toUpperCase()} burned at ledger ${r.ledger_seq.toLocaleString()}.` });
    } catch (err) {
      setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg });
    } finally { setBusy(false); }
  };

  return (
    <ActionCard num="04" id="redeem" label="Redeem" title="Redeem" auth="delegate"
      desc="Burn a unique code — the synchronous Burn path. Irreversible and single-use: a second redemption fails with AlreadyRedeemed. Authorized by the campaign owner or a delegate."
      locked={!wallet} snippets={SD.SNIP.redeem_unique({ campaign_id: cid, code: code.toUpperCase(), hash })}>
      <div className="form-grid">
        <div className="field">
          <label>Campaign ID <span className="req">*</span></label>
          <input className="input mono" type="number" min="1" value={cid} onChange={(e) => setCid(e.target.value)} placeholder="1" />
          <span className="help">Codes are scoped per campaign — a QR/link carries this.</span>
        </div>
        <div className="field">
          <label>Code <span className="req">*</span></label>
          <input className="input mono" value={code} onChange={(e) => setCode(e.target.value)} placeholder="DEMO0001" />
          <span className="help">A code issued under this campaign</span>
        </div>
        <div className="field span2">
          <label>Redeemer reference</label>
          <input className="input mono" value={ref} onChange={(e) => setRef(e.target.value)} placeholder="order-8842 / phone / email" />
          <span className="help">Any identifier you use off-chain (order #, phone, email)</span>
        </div>
        <div className="field span2">
          <label><Ic.shield style={{ width: 13, height: 13, opacity: .6 }} /> redeemer_ref_hash <span className="authtag" style={{ fontSize: 10, padding: "1px 6px" }}>committed in your browser</span></label>
          <div className="copy" style={{ width: "100%", padding: "10px 12px" }}>
            <span className="val mono" style={{ fontSize: 12.5, color: hash === SD.ZERO32 ? "var(--ink-3)" : "var(--ink)" }}>{hash}</span>
            <CopyBare value={hash} display="" />
          </div>
          <span className="help">SHA-256(random nonce ∥ reference) — an opaque, non-reversible commitment. The nonce (<span className="mono">{SD.trunc(nonce, 6, 4)}</span>) stays off-chain; only this hash is sent. No plaintext PII ever goes on-chain.</span>
          {hashError && <span className="help" style={{ color: "var(--burned)" }}>{hashError}</span>}
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" loading={busy} onClick={submit} disabled={!code.trim() || !cid || !!hashError || hash === SD.ZERO32} icon={<Ic.fire style={{ width: 15, height: 15 }} />}>{busy ? "Burning" : "Redeem & burn"}</Btn>
        <span className="help">Irreversible — this permanently burns the token</span>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
      {result && (
        <ResultPanel title="Redemption receipt" onClose={() => setResult(null)}>
          <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", marginBottom: 4 }}>
            <Pill kind="burned"><Ic.fire style={{ width: 12, height: 12 }} /> Burned</Pill>
            <span className="mono" style={{ fontSize: 14, fontWeight: 600 }}>{result.code}</span>
            <span className="faint mono" style={{ fontSize: 12 }}>token #{result.token_id}</span>
          </div>
          <KV k="campaign"><span style={{ fontWeight: 600 }}>{result.campaign_name}</span> <span className="faint mono" style={{ fontSize: 12 }}>#{result.campaign_id}</span></KV>
          <KV k="discount"><span style={{ fontWeight: 600 }}>{SD.fmtDiscount(result.discount_type, result.discount_value)}</span> <span className="faint" style={{ fontSize: 12.5 }}>· {SD.DISCOUNT_LABEL[result.discount_type]}</span></KV>
          <KV k="redeemer_ref"><CopyValue value={result.redeemer_ref} display={SD.trunc(result.redeemer_ref, 10, 8)} /></KV>
          <KV k="burned_at"><span className="mono" style={{ fontSize: 13 }}>{SD.fmtTs(result.burned_at)}</span></KV>
          <KV k="ledger_seq"><span className="mono" style={{ fontSize: 13 }}>{result.ledger_seq.toLocaleString()}</span> <span className="faint" style={{ fontSize: 12 }}>· the on-chain anchor</span></KV>
        </ResultPanel>
      )}
    </ActionCard>
  );
}

/* ── 5 · Verify / Inspect (public) ─────────────────────────── */
function Verify() {
  const { verify, campaignStats } = useApp();
  const SD = window.SD;
  const [cid, setCid] = useState("1");
  const [code, setCode] = useState("DEMO0001");
  const [busy, setBusy] = useState(false);
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);

  const lookup = async () => {
    setBusy(true); setError(null); setData(null);
    try {
      const { result: r } = await verify(cid, code.trim().toUpperCase());
      setData({ ...r, stats: campaignStats(r.token.campaign_id) });
    } catch (err) { setError(err.sd); }
    finally { setBusy(false); }
  };

  const statusPill = (st) => st === "valid" ? <Pill kind="valid"><Ic.ok style={{ width: 12, height: 12 }} /> Valid</Pill>
    : st === "burned" ? <Pill kind="burned"><Ic.fire style={{ width: 12, height: 12 }} /> Burned</Pill>
    : <Pill kind="pending"><Ic.alert style={{ width: 12, height: 12 }} /> Expired</Pill>;

  return (
    <ActionCard num="05" id="verify" label="Verify / Inspect" title="Verify / Inspect" auth="public"
      desc="Look up any code's live status — no wallet required. Anyone can independently verify a coupon against the contract; this is what makes the protocol auditable."
      snippets={SD.SNIP.verify({ campaign_id: cid, code: code.toUpperCase() })}>
      <div className="form-grid">
        <div className="field">
          <label>Campaign ID</label>
          <input className="input mono" type="number" min="1" value={cid} onChange={(e) => setCid(e.target.value)} onKeyDown={(e) => e.key === "Enter" && lookup()} placeholder="1" />
        </div>
        <div className="field">
          <label>Code</label>
          <div style={{ display: "flex", gap: 10 }}>
            <input className="input mono" value={code} onChange={(e) => setCode(e.target.value)} onKeyDown={(e) => e.key === "Enter" && lookup()} placeholder="DEMO0001" style={{ flex: 1 }} />
            <Btn variant="primary" loading={busy} onClick={lookup}>{busy ? "Reading" : "Verify"}</Btn>
          </div>
        </div>
        <div className="field span2">
          <span className="help">Public read — a simulation, costs nothing, needs no signature. Codes are scoped per campaign: try campaign <CopyBare value="1" /> with <CopyBare value="DEMO0001" /> or <CopyBare value="DEMO0002" />.</span>
        </div>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
      {data && (
        <div className="result" style={{ background: "var(--surface)" }}>
          <div className="result-head" style={{ color: "var(--ink-2)" }}>
            <Ic.hash style={{ width: 15, height: 15 }} /><span style={{ flex: 1 }}>Coupon status</span>
            <button onClick={() => setData(null)} className="faint" style={{ display: "grid", placeItems: "center" }}><Ic.x style={{ width: 15, height: 15 }} /></button>
          </div>
          <div className="result-body">
            <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
              {statusPill(data.status)}
              <span className="mono" style={{ fontSize: 15, fontWeight: 600 }}>{data.token.code}</span>
              <span className="faint mono" style={{ fontSize: 12 }}>token #{data.token.token_id}</span>
            </div>
            <div className="divider" style={{ margin: "4px 0" }} />
            <KV k="campaign"><span style={{ fontWeight: 600 }}>{data.campaign?.name}</span> <span className="faint mono" style={{ fontSize: 12 }}>#{data.token.campaign_id}</span></KV>
            <KV k="minted_at"><span className="mono" style={{ fontSize: 13 }}>{SD.fmtTs(data.token.minted_at)}</span></KV>
            <KV k="burned_at"><span className="mono" style={{ fontSize: 13 }}>{data.token.is_burned ? SD.fmtTs(data.token.burned_at) : "—"}</span></KV>
            {data.token.is_burned && <KV k="redeemer_ref"><CopyValue value={data.token.redeemer_ref} display={SD.trunc(data.token.redeemer_ref, 10, 8)} /></KV>}

            {data.stats && (<>
              <div className="divider" style={{ margin: "6px 0 2px" }} />
              <div className="kv-k" style={{ marginBottom: 2 }}>Campaign stats</div>
              <div className="stat-grid">
                <div className="stat"><div className="sv">{data.stats.total_supply}</div><div className="sk">Supply</div></div>
                <div className="stat"><div className="sv">{data.stats.minted}</div><div className="sk">Minted</div></div>
                <div className="stat"><div className="sv" style={{ color: "var(--burned)" }}>{data.stats.burned}</div><div className="sk">Burned</div></div>
                <div className="stat"><div className="sv" style={{ color: "var(--valid)" }}>{data.stats.available}</div><div className="sk">Available</div></div>
                <div className="stat"><div className="sv">{data.stats.expired}</div><div className="sk">Expired</div></div>
              </div>
            </>)}
          </div>
        </div>
      )}
    </ActionCard>
  );
}

Object.assign(window, { Redeem, Verify });
