import React, { useEffect, useState } from "react";
import { api, fmtDateTime, fmtTime, idemKey, trunc } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, ErrText, Ic, Modal, Pill, TxLink } from "../ui.jsx";

export function RedemptionsPage() {
  const { env, route } = useApp();
  const [data, setData] = useState(null);
  const [campaigns, setCampaigns] = useState([]);
  const [open, setOpen] = useState(null); // expanded row id
  const [redeem, setRedeem] = useState(route.includes("redeem=1"));
  const [filterCamp, setFilterCamp] = useState("");
  const [filterResult, setFilterResult] = useState("");

  const load = () => {
    api.get("/v1/redemptions").then(setData).catch(() => {});
    api.get("/v1/campaigns").then((d) => setCampaigns(d.campaigns)).catch(() => {});
  };
  useEffect(() => { setData(null); load(); }, [env]);

  const list = (data?.redemptions || []).filter((r) =>
    (!filterCamp || String(r.campaign_id) === filterCamp) &&
    (!filterResult || (filterResult === "ok" ? r.ok : !r.ok)));

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Redemptions</h1>
          <p className="page-sub">Every burn and rejection, across all campaigns · updates live</p>
        </div>
        <BtnPrimary small icon={<Ic.check width={13} height={13} />} onClick={() => setRedeem(true)}>Redeem a code</BtnPrimary>
      </div>

      <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
        <select className="select" style={{ width: 200, fontSize: 13.5, paddingTop: 9, paddingBottom: 9 }} value={filterCamp} onChange={(e) => setFilterCamp(e.target.value)}>
          <option value="">All campaigns</option>
          {campaigns.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <select className="select" style={{ width: 150, fontSize: 13.5, paddingTop: 9, paddingBottom: 9 }} value={filterResult} onChange={(e) => setFilterResult(e.target.value)}>
          <option value="">Any result</option>
          <option value="ok">Redeemed</option>
          <option value="rejected">Rejected</option>
        </select>
        <div style={{ flex: 1 }} />
      </div>

      {data && list.length === 0
        ? <div className="empty">
            <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: 0 }}>No redemptions yet — redeem a code from the console or with one curl.</p>
            <BtnPrimary small icon={<Ic.check width={13} height={13} />} onClick={() => setRedeem(true)}>Redeem a code</BtnPrimary>
          </div>
        : <div className="card" style={{ overflow: "hidden" }}>
            <div style={{ display: "grid", gridTemplateColumns: ".9fr 1.3fr 1.3fr 1.3fr 1.2fr .9fr 30px", gap: 12, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
              {["Time", "Code", "Campaign", "Result", "Redeemer ref", "Ledger", ""].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
            </div>
            {list.map((r) => {
              const expanded = open === r.id;
              return (
                <React.Fragment key={r.id}>
                  <div onClick={() => setOpen(expanded ? null : r.id)} style={{
                    display: "grid", gridTemplateColumns: ".9fr 1.3fr 1.3fr 1.3fr 1.2fr .9fr 30px", gap: 12,
                    padding: "12px 20px", borderBottom: "1px solid var(--line)", alignItems: "center",
                    cursor: "pointer", background: expanded ? "var(--surface-inset)" : undefined,
                  }}>
                    <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-2)" }}>{fmtTime(r.created_at)}</span>
                    <span className="mono" style={{ fontSize: 12.5, fontWeight: 500 }}>{r.code}</span>
                    <span style={{ fontSize: 13, color: "var(--ink-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{r.campaign_name}</span>
                    {r.ok
                      ? <Pill kind="valid">Redeemed</Pill>
                      : <Pill kind="burned">{r.error_name === "AlreadyRedeemed" ? "Already redeemed" : r.error_name || "Rejected"}</Pill>}
                    <span className="mono" title="Stored as a privacy-preserving commitment — no personal data on-chain."
                      style={{ fontSize: 11.5, color: r.redeemer_ref ? "var(--ink-2)" : "var(--ink-3)", borderBottom: r.redeemer_ref ? "1px dashed var(--ink-3)" : "none", justifySelf: "start", cursor: "help" }}>
                      {r.redeemer_ref ? trunc(r.redeemer_ref, 4, 4) : "—"}
                    </span>
                    <span className="mono" style={{ fontSize: 11.5, color: r.ledger_seq ? "var(--ink-2)" : "var(--ink-3)" }}>{r.ledger_seq ? r.ledger_seq.toLocaleString() : `#${r.error_code}`}</span>
                    <Ic.chevron width={14} height={14} style={{ color: "var(--ink-3)", justifySelf: "end", transform: expanded ? "rotate(180deg)" : undefined }} />
                  </div>
                  {expanded && (
                    <div style={{ padding: "18px 20px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
                      {r.ok ? (
                        <div style={{ background: "var(--surface)", border: "1px solid var(--line)", borderRadius: 14, padding: "18px 20px", display: "flex", flexDirection: "column", gap: 14, boxShadow: "inset 0 0 0 1px color-mix(in oklch, var(--valid) 18%, transparent)" }}>
                          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                            <span className="eyebrow" style={{ fontSize: 11, color: "var(--valid)" }}>✓ Redemption receipt</span>
                            <div style={{ display: "flex", alignItems: "center", gap: 10 }}><TxLink hash={r.tx_hash} /><button className="btn-plain" style={{ fontSize: 12, padding: "6px 13px" }} onClick={(e) => {
                              e.stopPropagation();
                              navigator.clipboard?.writeText(JSON.stringify(r, null, 2));
                            }}>Copy receipt JSON</button></div>
                          </div>
                          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: "12px 20px", fontSize: 13 }}>
                            {[["token_id", r.token_id], ["code", r.code], ["burned_at", fmtDateTime(r.created_at)], ["ledger_seq", r.ledger_seq.toLocaleString()]].map(([k, v]) => (
                              <div key={k}>
                                <div style={{ fontSize: 11, fontWeight: 600, color: "var(--ink-3)", textTransform: "uppercase", letterSpacing: ".08em" }}>{k}</div>
                                <div className="mono" style={{ fontSize: 13, marginTop: 3 }}>{v}</div>
                              </div>
                            ))}
                          </div>
                          <div style={{ borderTop: "1px dashed var(--line-2)", paddingTop: 12, display: "flex", flexDirection: "column", gap: 8 }}>
                            <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
                              <span style={{ fontSize: 11, fontWeight: 600, color: "var(--ink-3)", textTransform: "uppercase", letterSpacing: ".08em", width: 170, flex: "none" }}>redeemer commitment</span>
                              <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-2)", wordBreak: "break-all" }}>{r.redeemer_ref || "(random)"}</span>
                            </div>
                            <p style={{ fontSize: 12, color: "var(--ink-3)", margin: "2px 0 0", lineHeight: 1.5 }}>
                              The reference you passed was hashed with a random nonce before it touched the chain — <strong style={{ color: "var(--ink-2)" }}>no personal data on-chain, ever</strong>.
                            </p>
                          </div>
                        </div>
                      ) : (
                        <div style={{ background: "var(--surface)", border: "1px solid var(--line)", borderRadius: 14, padding: "16px 18px", boxShadow: "inset 0 0 0 1px color-mix(in oklch, var(--burned) 22%, transparent)" }}>
                          <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
                            <span style={{ width: 26, height: 26, borderRadius: 999, background: "var(--burned-bg)", color: "var(--burned)", display: "grid", placeItems: "center", fontSize: 13, fontWeight: 700 }}>⚠</span>
                            <div>
                              <div style={{ fontSize: 14, fontWeight: 650, color: "var(--burned)" }}>{r.error_name === "AlreadyRedeemed" ? "Already redeemed" : r.error_name}</div>
                              <div className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{r.code} · {fmtDateTime(r.created_at)}</div>
                            </div>
                            <span className="authtag" style={{ marginLeft: "auto" }}>{r.error_name} · #{r.error_code}</span>
                          </div>
                          <p style={{ fontSize: 12.5, color: "var(--ink-2)", margin: "10px 0 0", lineHeight: 1.5 }}>
                            {r.error_name === "AlreadyRedeemed"
                              ? "Single-use codes burn once. Nothing was written, nothing was charged — show the customer the first receipt."
                              : "The contract rejected this at simulation. Nothing was written, nothing was charged."}
                          </p>
                        </div>
                      )}
                    </div>
                  )}
                </React.Fragment>
              );
            })}
          </div>}

      {redeem && <RedeemModal campaigns={campaigns} onClose={(changed) => { setRedeem(false); if (changed) load(); }} />}
    </div>
  );
}

function RedeemModal({ campaigns, onClose }) {
  const { env, toast } = useApp();
  const unique = campaigns.filter((c) => (c.kind === "voucher" || c.kind === "ticket" || c.kind === "loyalty") && !c.archived);
  const [campID, setCampID] = useState(unique[0]?.id || "");
  const [code, setCode] = useState("");
  const [ref, setRef] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const submit = async () => {
    setBusy(true); setErr(null);
    try {
      const r = await api.postIdem("/v1/redemptions", { campaign_id: Number(campID), code, redeemer_ref: ref }, idemKey());
      toast("Redeemed ✓", `${r.receipt.code} · ${r.receipt.campaign_name} · ledger ${r.receipt.ledger_seq.toLocaleString()}`);
      onClose(true);
    } catch (e) { setErr(e); setBusy(false); }
  };

  return (
    <Modal onClose={() => onClose(false)} width={460}>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div style={{ fontSize: 16, fontWeight: 650 }}>Redeem a code</div>
        <div className="field">
          <label>Campaign</label>
          <select className="select" style={{ fontSize: 13.5 }} value={campID} onChange={(e) => setCampID(e.target.value)}>
            {unique.map((c) => <option key={c.id} value={c.id}>{c.name} — {c.kind}</option>)}
          </select>
        </div>
        <div className="field">
          <label>Code <span className="req">*</span></label>
          <input className="input mono" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} placeholder="BURN-9D4X" autoFocus />
        </div>
        <div className="field">
          <label>Reference <span style={{ fontWeight: 500, color: "var(--ink-3)" }}>optional</span></label>
          <input className="input" value={ref} onChange={(e) => setRef(e.target.value)} placeholder="order #58291" />
          <span className="help">Hashed with a random nonce before it touches the chain — never stored raw.</span>
        </div>
        <ErrText err={err} />
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{env === "test" ? "free (test)" : "5 cr"} · ~5s to finality</span>
          <BtnPrimary small busy={busy} disabled={!code || !campID} icon={<Ic.check width={13} height={13} />} onClick={submit}>Redeem</BtnPrimary>
        </div>
      </div>
    </Modal>
  );
}
