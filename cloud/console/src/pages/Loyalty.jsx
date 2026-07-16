import React, { useEffect, useState } from "react";
import { api, fmtDate, fmtDateTime, idemKey, trunc } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, ErrText, Ic, Modal, Pill } from "../ui.jsx";
import { Wizard } from "./Campaigns.jsx";

// derive the real program status from its backing campaign (was hardcoded
// "Active"): archived and expired programs reject new punches with 409.
const programStatus = (p) =>
  p.archived ? ["", "Archived"] : p.valid_until * 1000 < Date.now() ? ["pending", "Expired"] : ["valid", "Active"];

function PunchDots({ n, of, size = 13 }) {
  const shown = Math.min(of, 10);
  return (
    <span style={{ display: "inline-flex", gap: 4 }}>
      {Array.from({ length: shown }, (_, i) => (
        <span key={i} style={{
          width: size, height: size, borderRadius: 99,
          ...(i < n ? { background: "var(--accent)", border: "1px solid var(--accent-2)" } : { border: "1px dashed var(--line-2)" }),
        }} />
      ))}
    </span>
  );
}

export function LoyaltyPage() {
  const { env, nav } = useApp();
  const [data, setData] = useState(null);
  const [create, setCreate] = useState(false);

  const load = () => api.get("/v1/loyalty/programs").then(setData).catch(() => {});
  useEffect(() => { setData(null); load(); }, [env]);

  if (!data) return <div className="page"><div className="faint">Loading…</div></div>;

  if (data.programs.length === 0) return (
    <div className="page">
      <div style={{ flex: 1, display: "grid", placeItems: "center", minHeight: "60vh" }}>
        <div style={{ width: 440, display: "flex", flexDirection: "column", alignItems: "center", gap: 20, textAlign: "center" }}>
          <div style={{ width: 340, background: "var(--surface)", border: "1px solid var(--line)", borderRadius: 16, boxShadow: "var(--shadow)", padding: "18px 22px", display: "flex", flexDirection: "column", gap: 12, transform: "rotate(-1.5deg)" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <span className="eyebrow" style={{ fontSize: 10.5 }}>Punch card</span>
              <span className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)" }}>3/10</span>
            </div>
            <div style={{ display: "flex", gap: 7, justifyContent: "center" }}>
              {Array.from({ length: 10 }, (_, i) => i < 3
                ? <span key={i} style={{ width: 22, height: 22, borderRadius: 99, background: "var(--accent)", border: "1px solid var(--accent-2)", display: "grid", placeItems: "center", color: "var(--on-accent)", fontSize: 11, fontWeight: 700 }}>✓</span>
                : <span key={i} style={{ width: 22, height: 22, borderRadius: 99, border: "1.5px dashed var(--line-2)" }} />)}
            </div>
            <div className="mono" style={{ fontSize: 10.5, color: "var(--ink-3)", borderTop: "1px dashed var(--line-2)", paddingTop: 9 }}>signed receipts · manual period anchor</div>
          </div>
          <div>
            <h2 className="display" style={{ fontSize: 26, margin: 0 }}>Buy 10, get 1 free — with signed punches.</h2>
            <p style={{ fontSize: 13.5, color: "var(--ink-2)", lineHeight: 1.55, margin: "8px 0 0" }}>
              Punches produce signed receipts. You can commit a period total on-chain manually; crossing the threshold issues the reward voucher.
            </p>
          </div>
          <BtnPrimary icon={<Ic.plus width={15} height={15} />} onClick={() => setCreate(true)}>Create a program</BtnPrimary>
        </div>
      </div>
      {create && <Wizard fixedKind="loyalty" onDone={() => { setCreate(false); load(); }} />}
    </div>
  );

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Loyalty</h1>
          <p className="page-sub">Punch programs · signed earn receipts, manual period commits, automatic reward issuance</p>
        </div>
        <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setCreate(true)}>New program</BtnPrimary>
      </div>
      <div className="card" style={{ overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "1.8fr 2fr 1fr 1.1fr 1.1fr .8fr", gap: 14, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
          {["Program", "Rule", "Status", "Customers", "Rewards issued", "Created"].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
        </div>
        {data.programs.map((p) => (
          <div key={p.id} onClick={() => nav(`/loyalty/${p.id}`)}
            style={{ display: "grid", gridTemplateColumns: "1.8fr 2fr 1fr 1.1fr 1.1fr .8fr", gap: 14, padding: "14px 20px", borderBottom: "1px solid var(--line)", alignItems: "center", cursor: "pointer" }}>
            <span style={{ fontSize: 14, fontWeight: 650 }}>{p.name}</span>
            <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
              <PunchDots n={3} of={Math.min(p.threshold, 5)} size={9} />
              <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{p.threshold} punches → reward</span>
            </div>
            {(([k, l]) => <Pill kind={k || undefined}>{l}</Pill>)(programStatus(p))}
            <span className="mono" style={{ fontSize: 13 }}>{p.customers}</span>
            <span className="mono" style={{ fontSize: 13 }}>{p.rewards_issued}</span>
            <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{fmtDate(p.created_at)}</span>
          </div>
        ))}
      </div>
      {create && <Wizard fixedKind="loyalty" onDone={() => { setCreate(false); load(); }} />}
    </div>
  );
}

export function LoyaltyDetailPage({ id }) {
  const { env, nav, toast } = useApp();
  const [data, setData] = useState(null);
  const [punch, setPunch] = useState(false);

  const load = () => api.get(`/v1/loyalty/programs/${id}`).then(setData).catch(() => nav("/loyalty"));
  useEffect(() => { setData(null); load(); }, [id, env]);

  if (!data) return <div className="page"><div className="faint">Loading…</div></div>;
  const p = data.program;
  const outstanding = p.rewards_issued - p.rewards_redeemed;
  const frac = p.rewards_issued ? p.rewards_redeemed / p.rewards_issued : 0;
  const circ = 2 * Math.PI * 64;

  return (
    <div className="page">
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 7, fontSize: 12.5, fontWeight: 600, color: "var(--ink-3)", flex: 1 }}>
          <span style={{ cursor: "pointer" }} onClick={() => nav("/loyalty")}>Loyalty</span>
          <Ic.chevron width={12} height={12} style={{ transform: "rotate(-90deg)" }} />
          <span style={{ color: "var(--ink)" }}>{p.name}</span>
        </div>
        {(([k, l]) => <Pill kind={k || undefined}>{l}</Pill>)(programStatus(p))}
        <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setPunch(true)}>Record punches</BtnPrimary>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1.6fr", gap: 14, alignItems: "stretch" }}>
        <div className="card" style={{ padding: 24, display: "flex", alignItems: "center", gap: 24 }}>
          <svg viewBox="0 0 160 160" width="150" height="150" style={{ flex: "none" }}>
            <circle cx="80" cy="80" r="64" fill="none" stroke="var(--surface-inset)" strokeWidth="13" />
            <circle cx="80" cy="80" r="64" fill="none" stroke="var(--line)" strokeWidth="13" opacity=".6" />
            {p.rewards_issued > 0 && (
              <circle cx="80" cy="80" r="64" fill="none" stroke="var(--accent)" strokeWidth="13" strokeLinecap="round"
                strokeDasharray={`${Math.max(2, frac * circ)} ${circ}`} transform="rotate(-90 80 80)" />
            )}
            <text x="80" y="76" textAnchor="middle" fontFamily="var(--serif)" fontSize="36" fontWeight="600" fill="var(--ink)">{p.rewards_issued}</text>
            <text x="80" y="97" textAnchor="middle" fontFamily="var(--sans)" fontSize="11" fontWeight="600" fill="var(--ink-3)">rewards issued</text>
          </svg>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}><span style={{ width: 10, height: 10, borderRadius: 3, background: "var(--accent)" }} /><span style={{ fontSize: 13, color: "var(--ink-2)" }}><strong style={{ color: "var(--ink)" }}>{p.rewards_redeemed}</strong> redeemed</span></div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}><span style={{ width: 10, height: 10, borderRadius: 3, background: "var(--line)" }} /><span style={{ fontSize: 13, color: "var(--ink-2)" }}><strong style={{ color: "var(--ink)" }}>{outstanding}</strong> outstanding</span></div>
            <div className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", borderTop: "1px dashed var(--line-2)", paddingTop: 10 }}>
              {p.punches} punches total<br />{p.pending_anchor_events > 0 ? `${p.pending_anchor_events} pending anchor` : "no pending receipts"}
            </div>
          </div>
        </div>
        <div className="card" style={{ padding: "20px 24px", display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <span style={{ fontSize: 14, fontWeight: 650 }}>Reward settings</span>
            <span className="authtag"><Ic.lock width={11} height={11} />locked after creation</span>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14 }}>
            {[["Threshold", `${p.threshold} punches`], ["Reward", p.reward_discount_type.replace("_", " ")], ["Earn anchor", p.earn_code]].map(([k, v]) => (
              <div key={k} style={{ background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 12, padding: "13px 15px" }}>
                <div style={{ fontSize: 11.5, fontWeight: 600, color: "var(--ink-3)" }}>{k}</div>
                <div className="display" style={{ fontSize: 20, marginTop: 3 }}>{v}</div>
              </div>
            ))}
          </div>
          <p style={{ fontSize: 12.5, color: "var(--ink-2)", margin: 0, lineHeight: 1.5 }}>
            Crossing the threshold auto-issues a single-use voucher on the reward campaign and fires{" "}
            <span className="mono" style={{ fontSize: 11.5 }}>loyalty.reward_issued</span>. Period totals can be committed on-chain manually;
            per-customer balances stay private.
          </p>
        </div>
      </div>

      <div className="card" style={{ overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr 2fr 1fr 1fr", gap: 14, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
          {["Customer ref", "Punches", "Rewards earned", "Last punch"].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
        </div>
        {data.customers.length === 0 && (
          <div style={{ padding: "26px 20px", textAlign: "center" }}>
            <span className="mono" style={{ fontSize: 12, color: "var(--ink-3)" }}>No punches yet — record the first one.</span>
          </div>
        )}
        {data.customers.map((cu) => {
          const oneAway = cu.progress === p.threshold - 1;
          return (
            <div key={cu.customer_ref} style={{
              display: "grid", gridTemplateColumns: "1.4fr 2fr 1fr 1fr", gap: 14, padding: "13px 20px",
              borderBottom: "1px solid var(--line)", alignItems: "center",
              background: oneAway ? "color-mix(in oklch, var(--accent-wash) 30%, var(--surface))" : undefined,
            }}>
              <span className="mono" style={{ fontSize: 12.5 }}>{trunc(cu.customer_ref, 4, 4)}</span>
              <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                <PunchDots n={cu.progress} of={p.threshold} />
                <span className="mono" style={{ fontSize: 12, fontWeight: 600 }}>
                  {cu.progress}/{p.threshold}
                  {oneAway && <span style={{ color: "var(--pending)", fontWeight: 500 }}> · one away</span>}
                  {cu.rewards_earned > 0 && cu.progress < 2 && <span style={{ color: "var(--valid)", fontWeight: 500 }}> · reward issued</span>}
                </span>
              </div>
              <span className="mono" style={{ fontSize: 12.5 }}>{cu.rewards_earned}</span>
              <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{fmtDateTime(cu.last_punch_at)}</span>
            </div>
          );
        })}
      </div>

      {data.rewards.length > 0 && (
        <div className="card" style={{ overflow: "hidden" }}>
          <div style={{ padding: "14px 20px 0" }}><span style={{ fontSize: 14, fontWeight: 650 }}>Reward vouchers</span></div>
          {data.rewards.map((r) => (
            <div key={r.code} style={{ display: "flex", alignItems: "center", gap: 12, padding: "11px 20px", borderTop: "1px solid var(--line)", fontSize: 12.5 }}>
              <span className="code-chip" style={{ fontSize: 12.5 }}>{r.code}</span>
              <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{trunc(r.customer_ref, 4, 4)}</span>
              <span style={{ color: "var(--ink-3)" }}>{fmtDateTime(r.issued_at)}</span>
              <span style={{ marginLeft: "auto" }}>{r.redeemed ? <Pill>Redeemed</Pill> : <Pill kind="valid">Outstanding</Pill>}</span>
            </div>
          ))}
        </div>
      )}

      <p className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", margin: 0 }}>
        Customer refs are opaque hashes — you keep the mapping in your POS; the chain never sees who they are.
      </p>

      {punch && <PunchModal p={p} onClose={(done) => { setPunch(false); if (done) load(); }} />}
    </div>
  );
}

function PunchModal({ p, onClose }) {
  const { toast } = useApp();
  const [ref, setRef] = useState("");
  const [eventRef, setEventRef] = useState("");
  const [count, setCount] = useState("1");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const submit = async () => {
    setBusy(true); setErr(null);
    try {
      const parsedCount = Number(count);
      if (!Number.isSafeInteger(parsedCount) || parsedCount < 1 || parsedCount > 1_000) {
        throw new RangeError("Punches must be an integer between 1 and 1,000.");
      }
      const r = await api.postIdem(`/v1/loyalty/programs/${p.id}/punches`, { customer_ref: ref, event_ref: eventRef, count: parsedCount }, idemKey());
      if (r.rewards_issued.length > 0) {
        toast("Reward issued! 🎟", `Voucher ${r.rewards_issued.join(", ")} auto-issued on-chain — ${r.punches} punches total.`);
      } else {
        toast("Punches recorded", `${r.progress}/${r.threshold} toward the next reward.`);
      }
      onClose(true);
    } catch (e) { setErr(e); setBusy(false); }
  };

  return (
    <Modal onClose={() => onClose(false)} width={440}>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div>
          <div style={{ fontSize: 16, fontWeight: 650 }}>Record punches</div>
          <div className="mono" style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 2 }}>{p.name} · {p.threshold} → reward</div>
        </div>
        <div className="field">
          <label>Customer <span className="req">*</span></label>
          <input className="input" value={ref} onChange={(e) => setRef(e.target.value)} placeholder="phone, email or POS id" autoFocus />
          <span className="help">Hashed before storage — the chain never sees who they are.</span>
        </div>
        <div className="field">
          <label>Sale / event ID <span style={{ color: "var(--ink-3)" }}>optional</span></label>
          <input className="input mono" value={eventRef} onChange={(e) => setEventRef(e.target.value)} placeholder="POS-ORDER-1042" />
          <span className="help">Use a stable POS reference to reject the same event even if it arrives with a different idempotency key.</span>
        </div>
        <div className="field">
          <label>Punches</label>
          <input className="input mono" value={count} onChange={(e) => setCount(e.target.value)} />
        </div>
        <ErrText err={err} />
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 9 }}>
          <button className="btn-plain" onClick={() => onClose(false)}>Cancel</button>
          <BtnPrimary small busy={busy} disabled={!ref} icon={<Ic.check width={13} height={13} />} onClick={submit}>Record</BtnPrimary>
        </div>
      </div>
    </Modal>
  );
}
