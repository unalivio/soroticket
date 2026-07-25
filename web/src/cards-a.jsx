/* ═══════════════════════════════════════════════════════════════════
   Soroticket Playground — Create Campaign · Issue Codes
   ═══════════════════════════════════════════════════════════════════ */

function dtLocal(offsetDays) {
  const d = new Date(Date.now() + offsetDays * 864e5);
  d.setSeconds(0, 0);
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

/* ── 2 · Create Campaign ───────────────────────────────────── */
function CreateCampaign() {
  const { wallet, createCampaign, toast } = useApp();
  const SD = window.SD;
  const [f, setF] = useState({ name: "Demo Cafe", discount_type: "percentage", discount_value: "1000", total_supply: "100", valid_until: dtLocal(30) });
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);
  const set = (k) => (e) => setF((s) => ({ ...s, [k]: e.target.value }));

  const helper = f.discount_type === "percentage"
    ? "In hundredths of a percent — 1000 = 10.00%"
    : f.discount_type === "fixed_amount" ? "In cents — 1000 = $10.00" : "Value ignored for free-item deals";

  const submit = async () => {
    setBusy(true); setError(null); setResult(null);
    try {
      const { result: r, tx, seq } = await createCampaign(f);
      setResult({ ...r, tx, seq });
      toast({ kind: "success", title: "Campaign created", msg: `campaign_id = ${r.campaign_id} · “${f.name}” is live on testnet.` });
    } catch (err) {
      setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg });
    } finally { setBusy(false); }
  };

  return (
    <ActionCard num="02" id="create" label="Create Campaign" title="Create Campaign" auth="owner"
      desc="Define the terms of a promotion — reward, supply and validity. The campaign is owned by your connected wallet; there is no global admin."
      locked={!wallet} snippets={SD.SNIP.create_campaign(f)}>
      <div className="form-grid">
        <div className="field span2">
          <label>Name <span className="req">*</span></label>
          <input className="input" value={f.name} onChange={set("name")} placeholder="Demo Cafe" />
        </div>
        <div className="field">
          <label>Discount type</label>
          <select className="select" value={f.discount_type} onChange={set("discount_type")}>
            <option value="percentage">Percentage</option>
            <option value="fixed_amount">Fixed amount</option>
            <option value="free_item">Free item</option>
          </select>
        </div>
        <div className="field">
          <label>Discount value</label>
          <input className="input mono" type="number" value={f.discount_value} onChange={set("discount_value")} disabled={f.discount_type === "free_item"} />
          <span className="help">{helper}{f.discount_type !== "free_item" && f.discount_value ? ` → ${SD.fmtDiscount(f.discount_type, f.discount_value)}` : ""}</span>
        </div>
        <div className="field">
          <label>Total supply</label>
          <input className="input mono" type="number" value={f.total_supply} onChange={set("total_supply")} />
          <span className="help">Max codes that can ever be issued</span>
        </div>
        <div className="field">
          <label>Valid until</label>
          <input className="input mono" type="datetime-local" value={f.valid_until} onChange={set("valid_until")} />
          <span className="help">Redemptions fail after this time</span>
        </div>
        <div className="field span2">
          <label>Owner <span className="authtag" style={{ fontSize: 10, padding: "1px 6px" }}>read-only</span></label>
          <input className="input mono input-readonly" value={wallet ? wallet.address : ""} readOnly />
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" loading={busy} onClick={submit}>{busy ? "Signing & sending" : "Create on testnet"}</Btn>
        <span className="help">Signs with Freighter · ~5s on testnet</span>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
      {result && (
        <ResultPanel title="Campaign created" onClose={() => setResult(null)}>
          <KV k="campaign_id">
            <span className="bigid">#{result.campaign_id} <small>u64</small></span>
          </KV>
          <KV k="transaction"><CopyValue value={result.tx} display={SD.trunc(result.tx, 8, 6)} link={SD.NET.explorer + "/tx/" + result.tx} /></KV>
          <KV k="ledger_seq"><span className="mono" style={{ fontSize: 13.5 }}>{result.seq?.toLocaleString()}</span></KV>
          <KV k="event"><span className="mono" style={{ fontSize: 12.5, color: "var(--ink-2)" }}>(campaign, create) → ({result.campaign_id}, {SD.trunc(wallet?.address, 4, 3)}, "{f.name}", {f.total_supply})</span></KV>
        </ResultPanel>
      )}
    </ActionCard>
  );
}

/* ── chip input ────────────────────────────────────────────── */
function ChipInput({ codes, setCodes }) {
  const [draft, setDraft] = useState("");
  const ref = useRef(null);
  const commit = (raw) => {
    const parts = raw.split(/[\s,]+/).map((x) => x.trim().toUpperCase()).filter(Boolean);
    if (parts.length) setCodes([...codes, ...parts.filter((p) => !codes.includes(p))]);
    setDraft("");
  };
  return (
    <div className="chip-input" onClick={() => ref.current?.focus()}>
      {codes.map((c) => (
        <span key={c} className="chip-code">{c}<button onClick={(e) => { e.stopPropagation(); setCodes(codes.filter((x) => x !== c)); }}><Ic.x style={{ width: 11, height: 11 }} /></button></span>
      ))}
      <input ref={ref} value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === "," || e.key === "Tab") { if (draft.trim()) { e.preventDefault(); commit(draft); } }
          else if (e.key === "Backspace" && !draft && codes.length) setCodes(codes.slice(0, -1));
        }}
        onPaste={(e) => { const t = e.clipboardData.getData("text"); if (/[\s,]/.test(t)) { e.preventDefault(); commit(t); } }}
        onBlur={() => draft.trim() && commit(draft)}
        placeholder={codes.length ? "" : "Type a code, press Enter…"} />
    </div>
  );
}

/* ── 3 · Issue Codes ───────────────────────────────────────── */
function IssueCodes() {
  const { wallet, state, issueCodes, toast, highlight } = useApp();
  const SD = window.SD;
  // issue_unique is owner-only → only offer campaigns the connected wallet owns
  const ownedIds = Object.keys(state.campaigns).filter((id) => wallet && state.campaigns[id].owner === wallet.address);
  const [cid, setCid] = useState("");
  // keep the selection valid: prefer the just-created campaign, else the latest one you own
  useEffect(() => {
    if (cid && ownedIds.includes(cid)) return;
    setCid((highlight && ownedIds.includes(String(highlight))) ? String(highlight) : (ownedIds[ownedIds.length - 1] || ""));
  }, [wallet && wallet.address, highlight, ownedIds.join(",")]);
  const [mode, setMode] = useState("chips");
  const [codes, setCodes] = useState(["DEMO0003", "DEMO0004"]);
  const [bulk, setBulk] = useState("DEMO0003\nDEMO0004");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);
  const [error, setError] = useState(null);

  const effCodes = mode === "chips" ? codes : bulk.split(/\n+/).map((x) => x.trim().toUpperCase()).filter(Boolean);
  const camp = state.campaigns[Number(cid)];
  const remaining = camp ? camp.total_supply - camp.minted : 0;

  const submit = async () => {
    setBusy(true); setError(null); setResult(null);
    try {
      const { result: r, tx, seq } = await issueCodes(cid, effCodes);
      setResult({ ...r, tx, seq, codes: effCodes });
      toast({ kind: "success", title: "Codes issued", msg: `${r.token_ids.length} unique token${r.token_ids.length > 1 ? "s" : ""} minted under campaign #${cid}.` });
    } catch (err) {
      setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg });
    } finally { setBusy(false); }
  };

  return (
    <ActionCard num="03" id="issue" label="Issue Codes" title="Issue Codes" auth="owner"
      desc="Mint a batch of unique single-use codes under a campaign. Each becomes an on-chain token. Issuing is owner-only — delegates can redeem but never mint supply."
      locked={!wallet} snippets={SD.SNIP.issue_unique({ campaign_id: cid, codes: effCodes })}>
      <div className="form-grid">
        <div className="field">
          <label>Campaign ID</label>
          <select className="select mono" value={cid} onChange={(e) => setCid(e.target.value)} disabled={!ownedIds.length}>
            {ownedIds.length
              ? ownedIds.map((id) => <option key={id} value={id}>#{id} — {state.campaigns[id].name}</option>)
              : <option value="">No campaigns you own yet</option>}
          </select>
          {camp
            ? <span className="help">{camp.minted}/{camp.total_supply} minted · {remaining} left in supply</span>
            : <span className="help">Create a campaign first — issuing is owner-only.</span>}
        </div>
        <div className="field" style={{ justifyContent: "flex-end" }}>
          <label>Input mode</label>
          <div className="seg">
            <button className={mode === "chips" ? "on" : ""} onClick={() => setMode("chips")}>Chips</button>
            <button className={mode === "bulk" ? "on" : ""} onClick={() => setMode("bulk")}>One per line</button>
          </div>
        </div>
        <div className="field span2">
          <label>Codes <span className="req">*</span><span className="help" style={{ marginLeft: "auto" }}>{effCodes.length} code{effCodes.length !== 1 ? "s" : ""}</span></label>
          {mode === "chips"
            ? <ChipInput codes={codes} setCodes={setCodes} />
            : <textarea className="textarea mono" value={bulk} onChange={(e) => setBulk(e.target.value)} placeholder={"DEMO0003\nDEMO0004\nDEMO0005"} />}
          <span className="help">Codes are unique within this campaign — duplicates are rejected with DuplicateCode.</span>
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" loading={busy} onClick={submit} disabled={!effCodes.length || !cid}>{busy ? "Minting" : `Issue ${effCodes.length || ""} code${effCodes.length !== 1 ? "s" : ""}`}</Btn>
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
      {result && (
        <ResultPanel title={`${result.token_ids.length} token${result.token_ids.length > 1 ? "s" : ""} minted`} onClose={() => setResult(null)}>
          <KV k="token_ids">
            <div className="chips">
              {result.token_ids.map((id, i) => (
                <span key={id} className="chip-tok"><span className="tid">#{id}</span><CopyBare value={result.codes[i]} /></span>
              ))}
            </div>
          </KV>
          <KV k="transaction"><CopyValue value={result.tx} display={SD.trunc(result.tx, 8, 6)} link={SD.NET.explorer + "/tx/" + result.tx} /></KV>
          <KV k="event"><span className="mono" style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{result.token_ids.length}× (coupon, issue)</span></KV>
        </ResultPanel>
      )}
    </ActionCard>
  );
}

Object.assign(window, { CreateCampaign, IssueCodes, ChipInput });
