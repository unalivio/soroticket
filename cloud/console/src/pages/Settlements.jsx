import React, { useEffect, useMemo, useState } from "react";
import { api, fmtUnits, trunc } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, ErrText, Ic, Modal, Pill, TxLink } from "../ui.jsx";

export function SettlementsPage() {
  const { env, toast } = useApp();
  const [data, setData] = useState(null);
  const [campaigns, setCampaigns] = useState([]);
  const [sel, setSel] = useState("");
  const [confirm, setConfirm] = useState(null); // row to settle
  const [commitBusy, setCommitBusy] = useState(false);

  const load = () => {
    api.get("/v1/settlements").then(setData).catch(() => {});
    api.get("/v1/campaigns").then((d) => setCampaigns(d.campaigns.filter((c) => c.attributed_to))).catch(() => {});
  };
  useEffect(() => { setData(null); setSel(""); load(); }, [env]);

  const codes = useMemo(() => {
    const seen = new Map();
    for (const c of campaigns) seen.set(`${c.id}:${c.shared_code}`, c);
    return [...seen.values()];
  }, [campaigns]);

  const current = codes.find((c) => `${c.id}:${c.shared_code}` === sel) || codes[0];
  const rows = (data?.settlements || []).filter((s) => !current || (s.campaign_id === current.id && s.code === current.shared_code));
  const payable = rows.filter((s) => !s.settled && s.attributed_count > 0);
  const settledTotal = rows.filter((s) => s.settled).reduce((acc, s) => acc + Number(s.payout_amount || 0), 0);
  const pendingEvents = current?.events_30d || 0;

  const commitNow = async () => {
    if (!current) return;
    setCommitBusy(true);
    try {
      const r = await api.post(`/v1/shared-codes/${current.id}/${current.shared_code}/commits`, {});
      toast("Tally committed", `${r.period_label} · ${r.count} events anchored with Merkle root ${trunc(r.merkle_root, 6, 6)}.`);
      load();
    } catch (e) { toast(e.code ? `#${e.code} ${e.name}` : "Commit failed", e.message, "error"); }
    finally { setCommitBusy(false); }
  };

  if (codes.length === 0) return (
    <div className="page">
      <div className="page-head"><div>
        <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Settlements</h1>
        <p className="page-sub">Creator & referral codes · pay per conversion</p>
      </div></div>
      <div className="empty">
        <h2 className="display" style={{ fontSize: 22, margin: 0 }}>No creator codes yet</h2>
        <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: 0, maxWidth: "36em" }}>
          Create a <strong>Creator / referral code</strong> campaign — every conversion credits the creator on-chain,
          and each committed period settles here with one click.
        </p>
      </div>
    </div>
  );

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Settlements</h1>
          <p className="page-sub">Creator & referral codes · pay per conversion in {data?.settlements?.[0]?.payout_unit || "XLM"}</p>
        </div>
        <select className="select mono" style={{ width: 260, fontSize: 12.5, paddingTop: 9, paddingBottom: 9 }}
          value={sel || (current ? `${current.id}:${current.shared_code}` : "")} onChange={(e) => setSel(e.target.value)}>
          {codes.map((c) => <option key={c.id} value={`${c.id}:${c.shared_code}`}>{c.shared_code} — {c.name}</option>)}
        </select>
      </div>

      {current && (
        <div className="card" style={{ padding: "20px 24px", display: "flex", alignItems: "center", gap: 28, flexWrap: "wrap" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Creator</span>
            <span className="mono" style={{ fontSize: 12.5, background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 8, padding: "4px 10px" }}>{trunc(current.attributed_to, 8, 5)}</span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Rate</span>
            <span style={{ display: "flex", alignItems: "center", gap: 7 }}>
              <span className="mono" style={{ fontSize: 14, fontWeight: 600 }}>{fmtUnits(current.payout_rate)} XLM</span>
              <span style={{ fontSize: 12, color: "var(--ink-3)" }}>/ redemption</span>
              <span className="authtag"><Ic.lock width={11} height={11} />immutable</span>
            </span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <span style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Settled to date</span>
            <span className="mono" style={{ fontSize: 14, fontWeight: 600 }}>{fmtUnits(settledTotal)} XLM</span>
          </div>
          <div style={{ flex: 1 }} />
          {payable.length > 0
            ? <div style={{ textAlign: "right", background: "var(--accent-wash)", border: "1px dashed var(--accent-2)", borderRadius: 12, padding: "10px 16px" }}>
                <div style={{ fontSize: 11.5, fontWeight: 600, color: "var(--ink-2)" }}>Payable now · {payable[0].period_label}</div>
                <div className="display" style={{ fontSize: 24 }}>{fmtUnits(payable[0].payout_preview)} <span style={{ fontSize: 14, color: "var(--ink-2)" }}>XLM</span></div>
              </div>
            : <button className="btn-plain" style={{ fontSize: 13 }} disabled={commitBusy} onClick={commitNow}>
                {commitBusy ? "Committing…" : "Commit current period"}
              </button>}
        </div>
      )}

      <div className="card" style={{ overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr 1.3fr 1.1fr 1fr 1.3fr", gap: 12, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
          {["Period", "Count", "Creator count", "Merkle root", "Status", "Amount", ""].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
        </div>
        {/* accruing row */}
        {current && pendingEvents > 0 && (
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr 1.3fr 1.1fr 1fr 1.3fr", gap: 12, padding: "13px 20px", borderBottom: "1px solid var(--line)", alignItems: "center", opacity: 0.75 }}>
            <span className="mono" style={{ fontSize: 12.5 }}>current</span>
            <span className="mono" style={{ fontSize: 12.5 }}>{pendingEvents} <span style={{ color: "var(--ink-3)", fontSize: 11 }}>so far</span></span>
            <span className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>—</span>
            <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>commit anytime</span>
            <Pill>Accruing</Pill>
            <span className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>—</span>
            <button className="btn-plain" style={{ fontSize: 12, padding: "5px 13px", justifySelf: "end" }} disabled={commitBusy} onClick={commitNow}>Commit now</button>
          </div>
        )}
        {rows.map((s) => (
          <div key={s.tally_id} style={{
            display: "grid", gridTemplateColumns: "1fr 1fr 1fr 1.3fr 1.1fr 1fr 1.3fr", gap: 12, padding: "13px 20px",
            borderBottom: "1px solid var(--line)", alignItems: "center",
            background: !s.settled && s.attributed_count > 0 ? "color-mix(in oklch, var(--accent-wash) 34%, var(--surface))" : undefined,
          }}>
            <span className="mono" style={{ fontSize: 12.5, fontWeight: !s.settled ? 600 : 400 }}>{s.period_label}</span>
            <span className="mono" style={{ fontSize: 12.5 }}>{s.count}</span>
            <span className="mono" style={{ fontSize: 12.5 }}>{s.attributed_count}</span>
            <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-2)" }}>{trunc(s.merkle_root, 4, 4)}</span>
            {s.settled ? <Pill kind="valid">Settled</Pill> : s.attributed_count > 0 ? <Pill kind="pending">Payable</Pill> : <Pill>Committed</Pill>}
            <span className="mono" style={{ fontSize: 12.5, fontWeight: !s.settled && s.attributed_count > 0 ? 600 : 400 }}>
              {s.settled ? `${fmtUnits(s.payout_amount)} XLM` : s.attributed_count > 0 ? `${fmtUnits(s.payout_preview)} XLM` : "—"}
            </span>
            {!s.settled && s.attributed_count > 0
              ? <BtnPrimary small style={{ justifySelf: "end" }} onClick={() => setConfirm(s)}>Settle</BtnPrimary>
              : <span style={{ justifySelf: "end" }}><TxLink hash={s.settle_tx} /></span>}
          </div>
        ))}
        {rows.length === 0 && pendingEvents === 0 && (
          <div style={{ padding: "26px 20px", textAlign: "center" }}>
            <span className="mono" style={{ fontSize: 12, color: "var(--ink-3)" }}>No committed periods yet — record events, then commit the period to anchor it on-chain.</span>
          </div>
        )}
      </div>
      <p className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", margin: 0 }}>
        Each period's count is anchored with a Merkle root — the creator can verify their numbers on-chain without trusting this dashboard.
      </p>

      {confirm && <SettleConfirm s={confirm} onClose={(done) => { setConfirm(null); if (done) load(); }} />}
    </div>
  );
}

function SettleConfirm({ s, onClose }) {
  const { env, org, toast } = useApp();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);
  const account = org?.accounts?.[env];

  const settle = async () => {
    setBusy(true); setErr(null);
    try {
      const r = await api.post("/v1/settlements", { campaign_id: s.campaign_id, code: s.code, period: s.period });
      toast("Settled ✓", `${fmtUnits(r.total)} ${r.unit} paid on-chain to ${trunc(s.attributed_to)}.`);
      onClose(true);
    } catch (e) { setErr(e); setBusy(false); }
  };

  return (
    <Modal onClose={() => onClose(false)} width={460}>
      <div className="ticket-perf" style={{ borderRadius: "18px 18px 0 0", margin: "-26px -28px 0", position: "relative", inset: "auto" }} />
      <div style={{ display: "flex", flexDirection: "column", gap: 12, paddingTop: 14 }}>
        <div>
          <div style={{ fontSize: 16, fontWeight: 650 }}>Settle {s.period_label}</div>
          <div className="mono" style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 2 }}>{s.code} · {s.campaign_name}</div>
        </div>
        <div style={{ textAlign: "center", padding: "14px 0 10px" }}>
          <div className="display" style={{ fontSize: 44 }}>{fmtUnits(s.payout_preview)} <span style={{ fontSize: 20, color: "var(--ink-2)" }}>XLM</span></div>
          <div className="mono" style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 4 }}>{s.attributed_count} redemptions × {fmtUnits(s.payout_rate)} XLM</div>
        </div>
        {[["From", `${trunc(account?.public_key)} · your org account`], ["To", `${trunc(s.attributed_to)} · creator`], ["Fee", env === "test" ? "free (test)" : "25 cr"]].map(([k, v]) => (
          <div key={k} style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 10, fontSize: 13 }}>
            <span style={{ color: "var(--ink-3)" }}>{k}</span><span className="mono" style={{ fontSize: 12 }}>{v}</span>
          </div>
        ))}
        <div style={{ background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 10, padding: "10px 13px", fontSize: 12.5, color: "var(--ink-2)", lineHeight: 1.5 }}>
          Settlement happens on-chain and is final — this period can't be paid twice (<span className="mono" style={{ fontSize: 11.5 }}>AlreadySettled</span> guards it).
        </div>
        <ErrText err={err} />
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 9 }}>
          <button className="btn-plain" onClick={() => onClose(false)}>Cancel</button>
          <BtnPrimary small busy={busy} icon={<Ic.coins width={13} height={13} />} onClick={settle}>Settle {fmtUnits(s.payout_preview)} XLM</BtnPrimary>
        </div>
      </div>
    </Modal>
  );
}
