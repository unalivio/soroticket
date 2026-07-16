import React, { useEffect, useState } from "react";
import { api, fmtCr, fmtTime, fmtUnits } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, Ic, Kpi, Pill, TxLink } from "../ui.jsx";

function Chart({ series, test }) {
  const W = 660, H = 190, plotH = 110, base = 170;
  const max = Math.max(4, ...series.map((s) => s.count));
  const pts = series.map((s, i) => {
    const x = (i / Math.max(1, series.length - 1)) * (W - 20);
    const y = base - (s.count / max) * plotH;
    return [x, y];
  });
  const line = pts.map((p) => p.join(",")).join(" ");
  const poly = `${line} ${pts[pts.length - 1][0]},${base} 0,${base}`;
  const color = test ? "var(--pending)" : "var(--ink)";
  const fill = test ? "var(--pending-bg)" : "var(--accent-wash)";
  const last = pts[pts.length - 1];
  return (
    <svg viewBox={`0 0 ${W} ${H}`} width="100%" style={{ overflow: "visible" }}>
      <line x1="0" y1={base} x2={W} y2={base} stroke="var(--line)" strokeWidth="1" />
      <line x1="0" y1={base - plotH / 2} x2={W} y2={base - plotH / 2} stroke="var(--line)" strokeWidth="1" strokeDasharray="2 4" />
      <line x1="0" y1={base - plotH} x2={W} y2={base - plotH} stroke="var(--line)" strokeWidth="1" strokeDasharray="2 4" />
      <text x={W - 6} y={base - 6} fontFamily="var(--mono)" fontSize="10" fill="var(--ink-3)" textAnchor="end">0</text>
      <text x={W - 6} y={base - plotH / 2 - 6} fontFamily="var(--mono)" fontSize="10" fill="var(--ink-3)" textAnchor="end">{Math.round(max / 2)}</text>
      <text x={W - 6} y={base - plotH - 6} fontFamily="var(--mono)" fontSize="10" fill="var(--ink-3)" textAnchor="end">{max}</text>
      <polygon points={poly} fill={fill} opacity=".8" />
      <polyline points={line} fill="none" stroke={color} strokeWidth="1.8" strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={last[0]} cy={last[1]} r="4" fill={test ? "var(--pending-bg)" : "var(--accent)"} stroke={color} strokeWidth="1.6" />
      <text x="0" y={H - 3} fontFamily="var(--mono)" fontSize="10" fill="var(--ink-3)">{series[0]?.date}</text>
      <text x={(W - 20) / 2} y={H - 3} fontFamily="var(--mono)" fontSize="10" fill="var(--ink-3)" textAnchor="middle">{series[Math.floor(series.length / 2)]?.date}</text>
      <text x={W - 20} y={H - 3} fontFamily="var(--mono)" fontSize="10" fill="var(--ink-3)" textAnchor="end">{series[series.length - 1]?.date}</text>
    </svg>
  );
}

function ActivityRow({ a }) {
  const rejected = a.kind === "rejected";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 9, padding: "9px 22px", borderTop: "1px solid var(--line)", fontSize: 12.5 }}>
      <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)", width: 38, flex: "none" }}>{fmtTime(a.ts)}</span>
      {a.code && <span className={"code-chip" + (rejected ? " bad" : "")}>{a.code}</span>}
      <span style={{ color: rejected ? "var(--burned)" : "var(--ink-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{a.message}</span>
      <span style={{ marginLeft: "auto", flex: "none" }}>
        {rejected
          ? <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>#{a.error_code}</span>
          : a.tx_hash ? <TxLink hash={a.tx_hash} /> : <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)" }}>{a.kind === "event" ? "off-chain" : ""}</span>}
      </span>
    </div>
  );
}

function Checklist({ data, nav }) {
  const steps = [
    { t: "Create your first campaign", d: "A coupon, ticket batch or loyalty program — one primitive, six faces.", done: data.active_campaigns > 0, go: "/campaigns?new=1" },
    { t: "Issue codes", d: "Generate a batch or paste your own list — QR payloads included.", done: data.activity?.some((a) => a.kind === "issue"), go: "/campaigns" },
    { t: "Make a test redemption", d: "From the console or with one curl — see the receipt land on testnet.", done: data.activity?.some((a) => a.kind === "redemption"), go: "/redemptions?redeem=1" },
    { t: "Get your API key", d: "Wire redemptions into your POS, bot or backend.", done: (data.api_keys ?? 0) > 0, go: "/keys" },
  ];
  const doneCount = steps.filter((s) => s.done).length;
  const firstOpen = steps.findIndex((s) => !s.done);
  return (
    <div className="card" style={{ padding: "22px 24px", display: "flex", flexDirection: "column", gap: 4 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 10 }}>
        <h2 className="display" style={{ fontSize: 21, margin: 0 }}>Set up your organization</h2>
        <Pill>{doneCount} of 4 done</Pill>
      </div>
      {steps.map((s, i) => (
        <div key={i} style={{ display: "flex", alignItems: "center", gap: 14, padding: "13px 0", borderTop: "1px solid var(--line)" }}>
          <span className="mono" style={{
            width: 28, height: 28, flex: "none", borderRadius: 999, display: "grid", placeItems: "center",
            fontSize: 12.5, fontWeight: 600,
            ...(s.done ? { background: "var(--valid)", color: "#fff" }
              : i === firstOpen ? { background: "var(--accent)", color: "var(--on-accent)" }
                : { border: "1px dashed var(--line-2)", color: "var(--ink-3)" }),
          }}>{s.done ? "✓" : i + 1}</span>
          <div style={{ flex: 1 }}>
            <div style={{ fontSize: 14.5, fontWeight: 650, color: s.done || i === firstOpen ? "var(--ink)" : "var(--ink-2)" }}>{s.t}</div>
            <div style={{ fontSize: 13, color: "var(--ink-3)" }}>{s.d}</div>
          </div>
          {i === firstOpen
            ? <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => nav(s.go)}>{i === 0 ? "New campaign" : "Go"}</BtnPrimary>
            : !s.done && <Ic.chevron width={15} height={15} style={{ color: "var(--ink-3)", transform: "rotate(-90deg)" }} />}
        </div>
      ))}
    </div>
  );
}

export function OverviewPage() {
  const { env, org, nav } = useApp();
  const [data, setData] = useState(null);

  useEffect(() => {
    let on = true;
    const load = () => api.get("/v1/overview").then((d) => on && setData(d)).catch(() => {});
    load();
    const t = setInterval(load, 8000);
    return () => { on = false; clearInterval(t); };
  }, [env]);

  if (!data) return <div className="page"><div className="faint">Loading…</div></div>;
  const isFirstRun = data.active_campaigns === 0 && (!data.activity || data.activity.length === 0);
  const today = new Date().toLocaleDateString([], { weekday: "long", month: "long", day: "numeric" });

  return (
    <div className="page">
      {!data.account_funded && (
        <div style={{ background: "var(--info-bg)", color: "var(--info)", borderRadius: 12, padding: "11px 16px", fontSize: 13, fontWeight: 600, display: "flex", gap: 9, alignItems: "center" }}>
          <Ic.spinner width={15} height={15} /> Setting up your Stellar account on testnet — on-chain actions unlock in a few seconds.
        </div>
      )}
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Overview</h1>
          <p className="page-sub">{env === "test" ? "TEST data is separate from METERED — experiments stay here." : `${today} · all campaigns`}</p>
        </div>
        <div style={{ display: "flex", gap: 9 }}>
          <button className="btn-plain" style={{ fontSize: 13.5 }} onClick={() => nav("/campaigns")}>Issue codes</button>
          <button className="btn-plain" style={{ fontSize: 13.5 }} onClick={() => nav("/keys")}>Create API key</button>
          <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => nav("/campaigns?new=1")}>New campaign</BtnPrimary>
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 14 }}>
        <Kpi label="Redemptions · 30d" value={data.redemptions_30d.toLocaleString()} sub={env === "test" ? "all on testnet" : undefined} />
        <Kpi label="Active campaigns" value={data.active_campaigns} sub={env === "test" ? "test campaigns" : undefined} />
        {env === "test"
          ? <div className="card" style={{ padding: "16px 18px 14px", borderColor: "color-mix(in oklch, var(--pending) 30%, var(--line))" }}>
              <div style={{ fontSize: 12.5, fontWeight: 600, color: "var(--ink-2)" }}>Credits balance</div>
              <div className="mono" style={{ fontSize: 28, fontWeight: 600, marginTop: 9, color: "var(--pending)" }}>Free</div>
              <div style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 3 }}>test mode is never metered</div>
            </div>
          : <Kpi label="Credits balance" value={<>{fmtCr(data.credits_mcr ?? 0)} <span style={{ fontSize: 14, color: "var(--ink-3)", fontWeight: 500 }}>cr</span></>} mono sub="25,000 cr free each month" />}
        <Kpi label="Settled to creators" value={<>{fmtUnits(data.settled_total_base_units)} <span style={{ fontSize: 17, color: "var(--ink-3)" }}>{data.settled_unit}</span></>} />
      </div>

      {isFirstRun
        ? <>
            <Checklist data={data} nav={nav} />
            <div className="empty"><div className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>Activity will appear here in real time — every event with its transaction link.</div></div>
          </>
        : <div style={{ display: "grid", gridTemplateColumns: "1.55fr 1fr", gap: 14, alignItems: "stretch" }}>
            <div className="card" style={{ padding: "20px 22px", display: "flex", flexDirection: "column", gap: 14 }}>
              <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
                <div style={{ fontSize: 14, fontWeight: 650 }}>Redemptions over time</div>
                <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{data.series[0]?.date} — {data.series[data.series.length - 1]?.date}</span>
              </div>
              <Chart series={data.series} test={env === "test"} />
            </div>
            <div className="card" style={{ padding: "20px 0 8px", display: "flex", flexDirection: "column" }}>
              <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", padding: "0 22px 12px" }}>
                <div style={{ fontSize: 14, fontWeight: 650 }}>Activity</div>
                <span style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)", cursor: "pointer" }} onClick={() => nav("/redemptions")}>View all</span>
              </div>
              {data.activity.map((a, i) => <ActivityRow key={i} a={a} />)}
            </div>
          </div>}
    </div>
  );
}
