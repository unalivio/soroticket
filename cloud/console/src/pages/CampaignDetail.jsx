import React, { useEffect, useMemo, useState } from "react";
import { api, fmtDate, fmtDateTime, idemKey, trunc } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, ErrText, Ic, Modal, Pill, TxLink } from "../ui.jsx";
import { kindTag, statusOf } from "./Campaigns.jsx";

const isUniqueKind = (k) => k === "voucher" || k === "ticket" || k === "loyalty";

/* ── The printed QR ─────────────────────────────────────────────────
   Scanning it with the phone camera opens WhatsApp with the message ready:
   the customer only presses send. The QR carries an opaque token, never the
   coupon code, so reading it with any scanner app reveals nothing. */
function QRPanel({ c }) {
  const { toast, toastErr } = useApp();
  const [qr, setQr] = useState(null);
  const [err, setErr] = useState(null);

  useEffect(() => {
    setQr(null); setErr(null);
    api.get(`/v1/campaigns/${c.id}/qr`).then(setQr).catch(setErr);
  }, [c.id]);

  if (err) return <div className="card" style={{ padding: 24 }}><ErrText err={err} /></div>;
  if (!qr) return <div className="card" style={{ padding: 24 }}><div className="faint">Generando el QR…</div></div>;

  const pngURL = `/api/v1/campaigns/${c.id}/qr.png?code=${encodeURIComponent(qr.code)}`;
  const copy = async (text, what) => {
    try { await navigator.clipboard.writeText(text); toast("Copiado", what); } catch (e) { toastErr(e); }
  };

  return (
    <div style={{ display: "flex", gap: 20, alignItems: "flex-start", flexWrap: "wrap" }}>
      <div className="card" style={{ padding: 24, display: "flex", gap: 22, alignItems: "flex-start", flex: "1 1 460px" }}>
        <img src={pngURL} alt={`QR de ${qr.code}`} width={190} height={190}
          style={{ imageRendering: "pixelated", borderRadius: 12, border: "1px solid var(--line)", background: "#fff", padding: 8, flex: "none" }} />
        <div style={{ display: "flex", flexDirection: "column", gap: 12, minWidth: 0 }}>
          <div>
            <div className="eyebrow" style={{ fontSize: 11 }}>Un solo QR para todas las unidades</div>
            <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: "6px 0 0", lineHeight: 1.55 }}>
              Pégalo en la botella, la mesa o el volante. La cámara del teléfono abre WhatsApp
              con el mensaje cargado — el cliente solo aprieta enviar.
            </p>
          </div>
          <div className="mono" style={{ fontSize: 11.5, background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 10, padding: "9px 11px", wordBreak: "break-all" }}>
            {qr.deep_link}
          </div>
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            <a className="btn-plain" style={{ fontSize: 13, textDecoration: "none" }} href={pngURL} download={`soroticket-${qr.code.toLowerCase()}.png`}>Descargar QR</a>
            <button className="btn-plain" style={{ fontSize: 13 }} onClick={() => copy(qr.deep_link, "El link de WhatsApp está en tu portapapeles.")}>Copiar link</button>
            <a className="btn-plain" style={{ fontSize: 13, textDecoration: "none" }} href={qr.deep_link} target="_blank" rel="noreferrer">Probarlo yo</a>
          </div>
          <span className="help">
            El QR lleva un identificador opaco (<span className="mono">{qr.scan_token}</span>), no el código
            <span className="mono"> {qr.code}</span>: quien lea el QR no ve de qué promo se trata.
          </span>
        </div>
      </div>
      <div className="card" style={{ padding: "18px 20px", flex: "0 1 300px", display: "flex", flexDirection: "column", gap: 10 }}>
        <div className="eyebrow" style={{ fontSize: 11 }}>Antes de mandarlo a imprenta</div>
        <p style={{ fontSize: 13, color: "var(--ink-2)", margin: 0, lineHeight: 1.55 }}>
          Escanea este QR con tu propio teléfono y completa el flujo una vez. Si el día del
          evento hay gente esperando, no es el momento de descubrir que algo no estaba conectado.
        </p>
        <p style={{ fontSize: 13, color: "var(--ink-2)", margin: 0, lineHeight: 1.55 }}>
          Bot de WhatsApp: <span className="mono">+{qr.wa_number}</span> — lo provee Soroticket,
          no tienes que configurar nada.
        </p>
      </div>
    </div>
  );
}

export function CampaignDetailPage({ id }) {
  const { env, nav, toast, toastErr } = useApp();
  const [data, setData] = useState(null);
  const [tab, setTab] = useState("codes");
  const [issue, setIssue] = useState(false);
  const [events, setEvents] = useState(false);
  const [q, setQ] = useState("");

  const load = () => api.get(`/v1/campaigns/${id}`).then(setData).catch(() => nav("/campaigns"));
  useEffect(() => { setData(null); load(); }, [id, env]);

  if (!data) return <div className="page"><div className="faint">Loading…</div></div>;
  const c = data.campaign;
  const [pk, pl] = statusOf(c);
  const unique = isUniqueKind(c.kind);
  const shared = !!c.shared_code;
  const codes = (data.codes || []).filter((cd) => !q || cd.code.toLowerCase().includes(q.toLowerCase()));
  const pct = unique ? Math.round((c.burned / Math.max(1, c.total_supply)) * 100) : 0;
  const expiresDays = Math.max(0, Math.round((c.valid_until * 1000 - Date.now()) / 86400000));

  const archive = async () => {
    try {
      await api.postIdem(`/v1/campaigns/${id}/archive`, {}, idemKey());
      toast("Archived", "New Cloud issues, redemptions and events are blocked; existing chain state is unchanged.");
      load();
    } catch (e) { toastErr(e); }
  };

  return (
    <div className="page">
      <div style={{ display: "flex", alignItems: "center", gap: 7, fontSize: 12.5, fontWeight: 600, color: "var(--ink-3)" }}>
        <span style={{ cursor: "pointer" }} onClick={() => nav("/campaigns")}>Campaigns</span>
        <Ic.chevron width={12} height={12} style={{ transform: "rotate(-90deg)" }} />
        <span style={{ color: "var(--ink)" }}>{c.name}</span>
      </div>

      {/* ticket header */}
      <div className="ticket" style={{ borderRadius: 18, boxShadow: "var(--shadow)", overflow: "hidden" }}>
        <div className="ticket-perf" />
        <div style={{ padding: "28px 26px 22px", display: "flex", flexDirection: "column", gap: 18 }}>
          <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 16 }}>
            <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 11 }}>
                <h1 className="display" style={{ fontSize: 28, margin: 0 }}>{c.name}</h1>
                <Pill kind={pk || undefined}>{pl}</Pill>
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: 9, flexWrap: "wrap" }}>
                <span className="authtag">{kindTag(c)} · {c.discount_value}{c.discount_type === "percentage" ? "% off" : ` ${c.discount_type}`}</span>
                <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>
                  campaign #{c.chain_id} · created {fmtDate(c.created_at)} ·{" "}
                  <a href={`https://stellar.expert/explorer/testnet/contract/${c.contract_id || "CCXNPRC4C2DX2W7Z2AW35NC6WORZPTI5JWJCTQIVRJ2FLMI3ZZ32MKRF"}`} target="_blank" rel="noreferrer">↗ stellar.expert</a>
                </span>
              </div>
            </div>
            <div style={{ display: "flex", gap: 8 }}>
              {!c.archived && <button className="btn-plain" style={{ fontSize: 13 }} onClick={archive}>Archive</button>}
              {shared && <button className="btn-plain" style={{ fontSize: 13 }} onClick={() => setEvents(true)}>Record events</button>}
              {unique && <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setIssue(true)}>Issue codes</BtnPrimary>}
            </div>
          </div>
          <div style={{ display: "flex", gap: 44, alignItems: "flex-end" }}>
            {unique ? <>
              <div><div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Issued</div><div className="display" style={{ fontSize: 34 }}>{c.minted}</div></div>
              <div><div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Redeemed</div><div className="display" style={{ fontSize: 34 }}>{c.burned}</div></div>
              <div><div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Available</div><div className="display" style={{ fontSize: 34 }}>{c.minted - c.burned}</div></div>
            </> : <>
              <div><div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Code</div><div className="mono" style={{ fontSize: 28, fontWeight: 600 }}>{c.shared_code}</div></div>
              <div><div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Events counted</div><div className="display" style={{ fontSize: 34 }}>{(c.events_total || 0).toLocaleString()}</div></div>
              {c.attributed_to && <div><div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Creator</div><div className="mono" style={{ fontSize: 15, paddingBottom: 8 }}>{trunc(c.attributed_to)}</div></div>}
            </>}
            <div style={{ flex: 1 }} />
            <div style={{ textAlign: "right" }}>
              <div style={{ fontSize: 12, fontWeight: 600, color: "var(--ink-2)" }}>Expires</div>
              <div className="display" style={{ fontSize: 22, paddingBottom: 4 }}>in {expiresDays} days <span style={{ fontSize: 14, color: "var(--ink-3)" }}>· {fmtDate(c.valid_until)}</span></div>
            </div>
          </div>
          {c.legacy_unsigned_events > 0 && (
            <div style={{ background: "var(--pending-bg)", color: "var(--pending)", borderRadius: 10, padding: "10px 13px", fontSize: 12.5 }}>
              {c.legacy_unsigned_events.toLocaleString()} pre-audit events are preserved but quarantined: they have no historical signature and cannot be presented as signed receipts. Export/reconcile them explicitly.
            </div>
          )}
          {unique && (
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              <div className="progressbar" style={{ height: 6 }}><span style={{ width: `${pct}%` }} /></div>
              <div className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{pct}% redeemed · archive blocks new Cloud operations but does not alter existing on-chain state or TTL</div>
            </div>
          )}
        </div>
      </div>

      {/* tabs */}
      <div style={{ display: "flex", gap: 2, borderBottom: "1px solid var(--line)" }}>
        {[["codes", unique ? "Codes" : "Shared code"], ...(shared ? [["qr", "QR de WhatsApp"]] : []), ["settlement", "Settlement"], ["activity", "Activity"]].map(([k, label]) => {
          const disabled = k === "settlement" && !c.attributed_to;
          return (
            <span key={k} onClick={() => !disabled && (k === "settlement" ? nav("/settlements") : setTab(k))} style={{
              padding: "9px 16px", fontSize: 13.5, cursor: disabled ? "default" : "pointer",
              fontWeight: tab === k ? 650 : 600,
              color: tab === k ? "var(--ink)" : "var(--ink-3)",
              borderBottom: tab === k ? "2px solid var(--ink)" : "none", marginBottom: -1,
              opacity: disabled ? 0.5 : 1,
            }}>{label}{k === "settlement" && <span style={{ fontSize: 11 }}> · shared only</span>}</span>
          );
        })}
      </div>

      {tab === "codes" && unique && <>
        <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
          <input className="input mono" placeholder="Find a code" value={q} onChange={(e) => setQ(e.target.value)}
            style={{ width: 240, fontSize: 12.5, paddingTop: 9, paddingBottom: 9 }} />
          <div style={{ flex: 1 }} />
          <button className="btn-plain" style={{ fontSize: 13 }} onClick={() => {
            const csv = "code,status,token_id,qr_payload\n" + data.codes.map((cd) =>
              `${cd.code},${cd.status},${cd.token_id},"soroticket:${c.chain_id}:${cd.code}"`).join("\n");
            const a = document.createElement("a");
            a.href = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
            a.download = `${c.name.replace(/\s+/g, "-")}-codes.csv`; a.click();
          }}>Export CSV</button>
        </div>
        {codes.length === 0
          ? <div className="empty">
              <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: 0 }}>No codes yet — issue a batch to get started.</p>
              <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setIssue(true)}>Issue codes</BtnPrimary>
            </div>
          : <div className="card" style={{ overflow: "hidden" }}>
              <div style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr 1fr 1fr 44px", gap: 14, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
                {["Code", "Status", "Issued", "Redeemed", "QR"].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11, textAlign: i === 4 ? "right" : "left" }}>{h}</span>)}
              </div>
              {codes.slice(0, 100).map((cd) => (
                <div key={cd.code} style={{ display: "grid", gridTemplateColumns: "1.6fr 1fr 1fr 1fr 44px", gap: 14, padding: "12px 20px", borderBottom: "1px solid var(--line)", alignItems: "center" }}>
                  <span className="mono" style={{ fontSize: 13, fontWeight: 500 }}>{cd.code}</span>
                  {cd.status === "redeemed"
                    ? <Pill>Redeemed</Pill>
                    : <Pill kind="valid">Active</Pill>}
                  <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{fmtDate(cd.created_at)}</span>
                  <span style={{ fontSize: 12.5, color: cd.redeemed_at ? "var(--ink-2)" : "var(--ink-3)" }}>{cd.redeemed_at ? fmtDateTime(cd.redeemed_at) : "—"}</span>
                  <button title="Copy QR payload" style={{ justifySelf: "end", color: "var(--ink-3)" }}
                    onClick={() => { navigator.clipboard?.writeText(`soroticket:${c.chain_id}:${cd.code}`); toast("QR payload copied", `soroticket:${c.chain_id}:${cd.code}`); }}>
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" width="16" height="16"><rect x="4" y="4" width="6" height="6" rx="1.2" /><rect x="14" y="4" width="6" height="6" rx="1.2" /><rect x="4" y="14" width="6" height="6" rx="1.2" /><path d="M14 14h2.6v2.6H14zM17.4 17.4H20V20h-2.6z" /></svg>
                  </button>
                </div>
              ))}
              <div style={{ padding: "10px 20px" }}>
                <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>Showing {Math.min(100, codes.length)} of {c.minted} codes</span>
              </div>
            </div>}
      </>}

      {tab === "codes" && !unique && (
        <div className="card" style={{ padding: "22px 24px", display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
            <span className="mono" style={{ fontSize: 24, fontWeight: 600 }}>{c.shared_code}</span>
            {c.attributed_to
              ? <span className="authtag"><Ic.users width={11} height={11} />attributed · {trunc(c.attributed_to)}</span>
              : <span className="authtag">unattributed · count-only</span>}
            {c.payout_rate !== "0" && <span className="authtag"><Ic.lock width={11} height={11} />rate immutable</span>}
          </div>
          <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: 0, lineHeight: 1.55 }}>
            One code, many uses. Record each conversion via the API (or the button above); totals anchor
            on-chain per period with a Merkle root{c.attributed_to ? " and settle to the creator" : ""}.
          </p>
          <div className="mono" style={{ fontSize: 12, background: "var(--con-bg)", color: "var(--con-ink)", borderRadius: 10, padding: "12px 14px", overflowX: "auto" }}>
            curl -X POST /v1/shared-codes/{c.id}/{c.shared_code}/events -d {"'{\"count\":1,\"order_ref\":\"ORD-1001\"}'"}
          </div>
        </div>
      )}

      {tab === "qr" && <QRPanel c={c} />}

      {tab === "activity" && <ActivityList campaignID={c.id} />}

      {issue && <IssueModal c={c} onClose={(changed) => { setIssue(false); if (changed) load(); }} />}
      {events && <EventsModal c={c} onClose={(changed) => { setEvents(false); if (changed) load(); }} />}
    </div>
  );
}

function ActivityList({ campaignID }) {
  const { env } = useApp();
  const [items, setItems] = useState([]);
  useEffect(() => {
    api.get("/v1/activity").then((d) => setItems(d.activity.filter((a) => a.campaign_id === campaignID))).catch(() => {});
  }, [campaignID, env]);
  if (!items.length) return <div className="empty"><span className="mono" style={{ fontSize: 12.5, color: "var(--ink-3)" }}>No activity for this campaign yet.</span></div>;
  return (
    <div className="card" style={{ padding: "8px 0" }}>
      {items.map((a, i) => (
        <div key={i} style={{ display: "flex", alignItems: "center", gap: 9, padding: "9px 22px", borderTop: i ? "1px solid var(--line)" : "none", fontSize: 12.5 }}>
          <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)", width: 76, flex: "none" }}>{fmtDateTime(a.ts)}</span>
          {a.code && <span className={"code-chip" + (a.kind === "rejected" ? " bad" : "")}>{a.code}</span>}
          <span style={{ color: a.kind === "rejected" ? "var(--burned)" : "var(--ink-2)" }}>{a.message}</span>
          <span style={{ marginLeft: "auto" }}><TxLink hash={a.tx_hash} /></span>
        </div>
      ))}
    </div>
  );
}

/* ── batch issue modal ────────────────────────────────────────────── */
function IssueModal({ c, onClose }) {
  const { env, toast } = useApp();
  const [mode, setMode] = useState("generate");
  const [count, setCount] = useState("10");
  const [prefix, setPrefix] = useState(c.kind === "ticket" ? "TICKET" : "BURN");
  const [pasted, setPasted] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);
  const remaining = c.total_supply - c.minted;
  const n = mode === "generate" ? Number(count) || 0 : pasted.split(/[\s,]+/).filter(Boolean).length;

  const codeFormat = `${prefix ? prefix + "-" : ""}XXXXXXXXXXXX`;

  const submit = async () => {
    setBusy(true); setErr(null);
    try {
      const body = mode === "generate"
        ? { generate: { count: Number(count), prefix } }
        : { codes: pasted.split(/[\s,]+/).filter(Boolean) };
      const r = await api.postIdem(`/v1/campaigns/${c.id}/codes`, body, idemKey());
      if (r.error) {
        // HTTP 207: earlier chunks landed on-chain, a later one failed
        toast(`${r.issued.length} of ${n} codes issued — batch stopped early`,
          r.error.message || "The remaining codes were not issued; retry them.", "error");
      } else {
        toast(`${r.issued.length} codes issued`, "Written on-chain — QR payloads in the CSV export.");
      }
      onClose(true);
    } catch (e) { setErr(e); setBusy(false); }
  };

  return (
    <Modal onClose={() => onClose(false)} width={520}>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div>
          <div style={{ fontSize: 16, fontWeight: 650 }}>Issue codes</div>
          <div style={{ fontSize: 12.5, color: "var(--ink-3)", marginTop: 1 }}>{c.name} · {remaining} of {c.total_supply} supply remaining</div>
        </div>
        <div style={{ display: "inline-flex", background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 999, padding: 3, alignSelf: "flex-start" }}>
          {[["generate", "Generate"], ["paste", "Paste a list"]].map(([m, label]) => (
            <span key={m} onClick={() => setMode(m)} style={{
              padding: "6px 16px", borderRadius: 999, fontSize: 13, fontWeight: 600, cursor: "pointer",
              ...(mode === m ? { background: "var(--surface)", border: "1px solid var(--line-2)", boxShadow: "var(--shadow-sm)" } : { color: "var(--ink-3)" }),
            }}>{label}</span>
          ))}
        </div>
        {mode === "generate" ? <>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
            <div className="field">
              <label>How many</label>
              <input className="input mono" value={count} onChange={(e) => setCount(e.target.value)} />
            </div>
            <div className="field">
              <label>Prefix <span style={{ fontWeight: 500, color: "var(--ink-3)" }}>optional</span></label>
              <input className="input mono" value={prefix} onChange={(e) => setPrefix(e.target.value.toUpperCase())} />
            </div>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
            <span style={{ fontSize: 12.5, fontWeight: 600, color: "var(--ink-2)" }}>Generated format</span>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
              <span className="mono" style={{ fontSize: 12, background: "var(--surface-inset)", border: "1px dashed var(--line-2)", borderRadius: 7, padding: "3px 9px" }}>{codeFormat}</span>
              <span className="mono" style={{ fontSize: 12, color: "var(--ink-3)", padding: "3px 2px" }}>actual values are generated securely by the API</span>
            </div>
          </div>
        </> : (
          <div className="field">
            <label>Codes <span style={{ fontWeight: 500, color: "var(--ink-3)" }}>one per line or comma-separated</span></label>
            <textarea className="textarea mono" rows={5} value={pasted} onChange={(e) => setPasted(e.target.value.toUpperCase())} placeholder={"VIP-001\nVIP-002"} />
          </div>
        )}
        <div style={{ background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 10, padding: "11px 14px", fontSize: 12.5, color: "var(--ink-2)", lineHeight: 1.5 }}>
          Written on-chain in chunks of 100 per transaction. <span className="mono" style={{ fontSize: 12 }}>{n} codes × 2 cr = {env === "test" ? "free (test)" : `${n * 2} cr`}</span> · QR payloads included in the CSV.
        </div>
        <ErrText err={err} />
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 9 }}>
          <button className="btn-plain" onClick={() => onClose(false)}>Cancel</button>
          <BtnPrimary small busy={busy} disabled={n === 0 || n > remaining} onClick={submit}>Issue {n} codes</BtnPrimary>
        </div>
      </div>
    </Modal>
  );
}

/* ── record shared events modal ───────────────────────────────────── */
function EventsModal({ c, onClose }) {
  const { env, toast, toastErr } = useApp();
  const [count, setCount] = useState("1");
  const [orderRef, setOrderRef] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const submit = async () => {
    setBusy(true); setErr(null);
    try {
      const parsedCount = Number(count);
      if (!Number.isSafeInteger(parsedCount) || parsedCount < 1 || parsedCount > 10_000) {
        throw new RangeError("Conversions must be an integer between 1 and 10,000.");
      }
      const r = await api.postIdem(`/v1/shared-codes/${c.id}/${c.shared_code}/events`,
        { count: parsedCount, order_ref: orderRef }, idemKey());
      toast(`+${parsedCount} events recorded`, r.pending_events === undefined ? "Signed receipt stored." : `${r.pending_events} pending for the next on-chain anchor.`);
      onClose(true);
    } catch (e) { setErr(e); setBusy(false); }
  };

  return (
    <Modal onClose={() => onClose(false)} width={460}>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div>
          <div style={{ fontSize: 16, fontWeight: 650 }}>Record events</div>
          <div className="mono" style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 2 }}>{c.shared_code} · {c.name}</div>
        </div>
        <div className="field">
          <label>How many conversions</label>
          <input className="input mono" value={count} onChange={(e) => setCount(e.target.value)} autoFocus />
          <span className="help">The hot path is off-chain (0.2 cr each) — totals anchor on-chain per period with a Merkle root.</span>
        </div>
        <div className="field">
          <label>Order reference <span style={{ fontWeight: 500, color: "var(--ink-3)" }}>optional</span></label>
          <input className="input" value={orderRef} onChange={(e) => setOrderRef(e.target.value)} placeholder="ORD-1001" />
          <span className="help">Use a stable order ID to reject duplicate counting even if another idempotency key is used.</span>
        </div>
        <ErrText err={err} />
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 9 }}>
          <button className="btn-plain" onClick={() => onClose(false)}>Cancel</button>
          <BtnPrimary small busy={busy} icon={<Ic.check width={13} height={13} />} onClick={submit}>Record</BtnPrimary>
        </div>
      </div>
    </Modal>
  );
}
