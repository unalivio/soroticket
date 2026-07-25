/* ═══════════════════════════════════════════════════════════════════
   Soroticket Playground — Delegates (advanced)
   ═══════════════════════════════════════════════════════════════════ */

function Delegates() {
  const { wallet, state, addDelegate, removeDelegate, isDelegate, toast, highlight } = useApp();
  const SD = window.SD;
  // delegate management is owner-only → only offer campaigns the connected wallet owns
  const ownedIds = Object.keys(state.campaigns).filter((id) => wallet && state.campaigns[id].owner === wallet.address);
  const [cid, setCid] = useState("");
  useEffect(() => {
    if (cid && ownedIds.includes(cid)) return;
    setCid((highlight && ownedIds.includes(String(highlight))) ? String(highlight) : (ownedIds[ownedIds.length - 1] || ""));
  }, [wallet && wallet.address, highlight, ownedIds.join(",")]);
  const [addr, setAddr] = useState("GDELEGATE7XMPLZ4K2J3QF5VWN6YH8R9TCBA1SDLPUEMXKO2WQZ3RV5T");
  const [busy, setBusy] = useState("");
  const [check, setCheck] = useState(null);
  const [error, setError] = useState(null);

  const current = Object.keys(state.delegates)
    .filter((k) => k.startsWith(cid + ":") && state.delegates[k])
    .map((k) => k.split(":")[1]);

  const run = async (kind) => {
    setBusy(kind); setError(null); setCheck(null);
    try {
      if (kind === "add") { await addDelegate(cid, addr.trim()); toast({ kind: "success", title: "Delegate added", msg: `${SD.trunc(addr.trim(), 4, 4)} can now redeem codes for campaign #${cid}.` }); }
      else if (kind === "remove") { await removeDelegate(cid, addr.trim()); toast({ kind: "info", title: "Delegate removed", msg: `${SD.trunc(addr.trim(), 4, 4)} can no longer redeem.` }); }
      else { const { result } = await isDelegate(cid, addr.trim()); setCheck(result.is_delegate); }
    } catch (err) { setError(err.sd); toast({ kind: "error", title: err.sd.title, msg: err.sd.msg }); }
    finally { setBusy(""); }
  };

  return (
    <ActionCard num="06" id="delegates" label="Delegates" title="Delegates" auth="owner"
      desc="Grant other accounts the right to redeem your campaign's codes — useful for staff at the point of redemption. Delegates can redeem codes but cannot issue supply."
      locked={!wallet} snippets={SD.SNIP.add_delegate({ campaign_id: cid, delegate: addr })}>
      <div className="form-grid">
        <div className="field">
          <label>Campaign ID</label>
          <select className="select mono" value={cid} onChange={(e) => { setCid(e.target.value); setCheck(null); }} disabled={!ownedIds.length}>
            {ownedIds.length
              ? ownedIds.map((id) => <option key={id} value={id}>#{id} — {state.campaigns[id].name}</option>)
              : <option value="">No campaigns you own yet</option>}
          </select>
        </div>
        <div className="field">
          <label>Current delegates</label>
          <div style={{ display: "flex", alignItems: "center", minHeight: 44 }}>
            {current.length
              ? <div className="chips">{current.map((a) => <span key={a} className="chip-tok"><CopyBare value={a} display={SD.trunc(a, 4, 4)} /></span>)}</div>
              : <span className="faint" style={{ fontSize: 13 }}>None yet for this campaign</span>}
          </div>
        </div>
        <div className="field span2">
          <label>Delegate address</label>
          <input className="input mono" value={addr} onChange={(e) => { setAddr(e.target.value); setCheck(null); }} placeholder="G…" />
          <span className="help">A Stellar account (G…) you authorize to call redeem_unique on this campaign.</span>
        </div>
      </div>
      <div className="form-actions">
        <Btn variant="primary" small loading={busy === "add"} onClick={() => run("add")} icon={<Ic.plus style={{ width: 13, height: 13 }} />}>Add delegate</Btn>
        <button className="btn-plain" onClick={() => run("remove")} disabled={!!busy}>{busy === "remove" ? <Spinner size={14} /> : <Ic.x style={{ width: 14, height: 14 }} />} Remove</button>
        <button className="btn-plain" onClick={() => run("check")} disabled={!!busy}>{busy === "check" ? <Spinner size={14} /> : <Ic.shield style={{ width: 14, height: 14 }} />} is delegate?</button>
        {check !== null && (
          <span className="pill" style={{ background: check ? "var(--valid-bg)" : "var(--surface-inset)", color: check ? "var(--valid)" : "var(--ink-2)", borderColor: "transparent" }}>
            {check ? <><Ic.check style={{ width: 13, height: 13 }} /> authorized</> : <><Ic.x style={{ width: 13, height: 13 }} /> not a delegate</>}
          </span>
        )}
      </div>

      {error && <ErrorPanel error={error} onClose={() => setError(null)} />}
    </ActionCard>
  );
}

Object.assign(window, { Delegates });
