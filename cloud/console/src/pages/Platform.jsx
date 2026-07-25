/* API keys · Usage & credits · Webhooks · Settings */
import React, { useEffect, useState } from "react";
import { api, fmtCr, fmtDate, fmtDateTime, idemKey, trunc } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, CopyMono, Empty, ErrText, Ic, Modal, Pill } from "../ui.jsx";

/* ── API keys ─────────────────────────────────────────────────────── */
export function ApiKeysPage() {
  const { env, toast, toastErr } = useApp();
  const [data, setData] = useState(null);
  const [create, setCreate] = useState(false);
  const [snippet, setSnippet] = useState("curl");
  const keyMode = env === "live" ? "metered" : "test";

  const load = () => api.get("/v1/keys").then(setData).catch(() => {});
  useEffect(() => { setData(null); load(); }, [env]);

  const revoke = async (id) => {
    try { await api.postIdem(`/v1/keys/${id}/revoke`, {}, idemKey()); toast("Key revoked", "Requests with it now return 401."); load(); }
    catch (e) { toastErr(e); }
  };

  const snippets = {
    curl: `# redeem a unique code — preview testnet; waits for chain confirmation
curl -X POST http://127.0.0.1:8787/v1/redemptions \\
  -H "Authorization: Bearer sk_${keyMode}_…" \\
  -H "Idempotency-Key: <new-uuid-per-operation>" \\
  -H "Content-Type: application/json" \\
  -d '{ "campaign_id": 4, "code": "BURN-9D4X", "redeemer_ref": "order #58291" }'`,
    TypeScript: `const res = await fetch("http://127.0.0.1:8787/v1/redemptions", {
  method: "POST",
  headers: { Authorization: "Bearer sk_${keyMode}_…", "Idempotency-Key": crypto.randomUUID(), "Content-Type": "application/json" },
  body: JSON.stringify({ campaign_id: 4, code: "BURN-9D4X", redeemer_ref: "order #58291" }),
});
const { receipt } = await res.json(); // token_id, ledger_seq, tx…`,
    Go: `req, _ := http.NewRequest("POST", api+"/v1/redemptions", body)
req.Header.Set("Authorization", "Bearer sk_${keyMode}_…")
req.Header.Set("Idempotency-Key", uuid.NewString()) // retries can't double-burn
req.Header.Set("Content-Type", "application/json")
client := &http.Client{Timeout: 15 * time.Second}
resp, err := client.Do(req)`,
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>API keys</h1>
          <p className="page-sub">Bearer keys, hashed at rest · mutating v1 routes accept an <span className="mono" style={{ fontSize: 12 }}>Idempotency-Key</span></p>
        </div>
        <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setCreate(true)}>Create key</BtnPrimary>
      </div>

      {data && data.keys.length > 0 && (
        <div className="card" style={{ overflow: "hidden" }}>
          <div style={{ display: "grid", gridTemplateColumns: "1.5fr 1.8fr 1fr 1fr .8fr", gap: 14, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
            {["Label", "Key", "Created", "Last used", ""].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
          </div>
          {data.keys.map((k) => (
            <div key={k.id} style={{ display: "grid", gridTemplateColumns: "1.5fr 1.8fr 1fr 1fr .8fr", gap: 14, padding: "13px 20px", borderBottom: "1px solid var(--line)", alignItems: "center", opacity: k.revoked ? 0.5 : 1 }}>
              <span style={{ fontSize: 13.5, fontWeight: 650 }}>{k.label}<span className="authtag" style={{ marginLeft: 6 }}>{k.mode || k.env}</span></span>
              <span className="mono" style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{k.prefix}</span>
              <span style={{ fontSize: 12.5, color: "var(--ink-2)" }}>{fmtDate(k.created_at)}</span>
              <span style={{ fontSize: 12.5, color: k.last_used_at ? "var(--valid)" : "var(--ink-3)", fontWeight: k.last_used_at ? 600 : 400 }}>
                {k.last_used_at ? fmtDateTime(k.last_used_at) : "never"}
              </span>
              {k.revoked
                ? <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", justifySelf: "end" }}>revoked</span>
                : <button className="btn-plain" style={{ fontSize: 12, padding: "5px 13px", justifySelf: "end", color: "var(--burned)" }} onClick={() => revoke(k.id)}>Revoke</button>}
            </div>
          ))}
        </div>
      )}
      {data && data.keys.length === 0 && (
        <Empty icon={<Ic.lock width={20} height={20} />} title="No keys yet">
          Create a key to wire redemptions into your POS, bot or backend — the console works without one.
        </Empty>
      )}

      <div style={{ background: "var(--con-bg)", border: "1px solid var(--con-line)", borderRadius: "var(--r)", overflow: "hidden", boxShadow: "var(--shadow)" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 4, padding: "12px 18px", borderBottom: "1px solid var(--con-line)" }}>
          <span style={{ fontSize: 12.5, fontWeight: 650, color: "var(--con-ink)", marginRight: 12 }}>Quickstart — your first redemption</span>
          {Object.keys(snippets).map((s) => (
            <span key={s} className="mono" onClick={() => setSnippet(s)} style={{
              fontSize: 11.5, padding: "4px 12px", borderRadius: 999, cursor: "pointer",
              ...(snippet === s ? { background: "var(--con-surface)", color: "var(--con-ink)", border: "1px solid var(--con-line)" } : { color: "var(--con-ink-3)" }),
            }}>{s}</span>
          ))}
          <span style={{ flex: 1 }} />
          <span className="mono" onClick={() => navigator.clipboard?.writeText(snippets[snippet])}
            style={{ fontSize: 11, color: "var(--con-ink-3)", display: "inline-flex", alignItems: "center", gap: 5, cursor: "pointer" }}>
            <Ic.copy width={12} height={12} />copy
          </span>
        </div>
        <pre className="mono" style={{ margin: 0, padding: "16px 18px 18px", fontSize: 12.5, lineHeight: 1.75, color: "var(--con-ink)", overflowX: "auto" }}>{snippets[snippet]}</pre>
      </div>

      {create && <CreateKeyModal onClose={(changed) => { setCreate(false); if (changed) load(); }} />}
    </div>
  );
}

function CreateKeyModal({ onClose }) {
  const { env } = useApp();
  const [label, setLabel] = useState("");
  const [created, setCreated] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const submit = async () => {
    setBusy(true); setErr(null);
    try { setCreated(await api.postIdem("/v1/keys", { label }, idemKey())); }
    catch (e) { setErr(e); } finally { setBusy(false); }
  };

  return (
    <Modal onClose={() => onClose(!!created)} width={480}>
      {!created ? (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <div>
            <div style={{ fontSize: 16, fontWeight: 650 }}>Create API key</div>
            <div style={{ fontSize: 12.5, color: "var(--ink-3)", marginTop: 1 }}>Scoped to <span className="mono">{env}</span> — create keys per integration so you can revoke one without breaking the rest.</div>
          </div>
          <div className="field">
            <label>Label</label>
            <input className="input" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Backend integration" autoFocus onKeyDown={(e) => e.key === "Enter" && submit()} />
          </div>
          <ErrText err={err} />
          <div style={{ display: "flex", justifyContent: "flex-end", gap: 9 }}>
            <button className="btn-plain" onClick={() => onClose(false)}>Cancel</button>
            <BtnPrimary small busy={busy} onClick={submit}>Create key</BtnPrimary>
          </div>
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <div>
            <div style={{ fontSize: 16, fontWeight: 650 }}>Copy your key now</div>
            <div style={{ fontSize: 12.5, color: "var(--ink-3)", marginTop: 1 }}>{created.label} · <span className="mono">{created.env}</span></div>
          </div>
          <div style={{ background: "var(--con-bg)", borderRadius: 12, padding: "14px 16px", display: "flex", alignItems: "center", gap: 10 }}>
            <span className="mono" style={{ fontSize: 13, color: "var(--con-ink)", wordBreak: "break-all", flex: 1 }}>{created.key}</span>
            <button className="btn-plain" style={{ fontSize: 12, padding: "6px 13px", flex: "none" }}
              onClick={() => navigator.clipboard?.writeText(created.key)}>Copy</button>
          </div>
          <div style={{ background: "var(--burned-bg)", color: "var(--burned)", borderRadius: 10, padding: "10px 13px", fontSize: 12.5, fontWeight: 600 }}>
            You won't see this again — we store only a hash.
          </div>
          <div style={{ display: "flex", justifyContent: "flex-end" }}>
            <BtnPrimary small icon={<Ic.check width={13} height={13} />} onClick={() => onClose(true)}>Done</BtnPrimary>
          </div>
        </div>
      )}
    </Modal>
  );
}

/* ── Usage & credits ──────────────────────────────────────────────── */
export function UsagePage() {
  const { env } = useApp();
  const [credits, setCredits] = useState(null);
  const [usage, setUsage] = useState(null);

  useEffect(() => {
    setCredits(null);
    api.get("/v1/credits").then(setCredits).catch(() => {});
    api.get("/v1/usage").then(setUsage).catch(() => {});
  }, [env]);

  if (!credits) return <div className="page"><div className="faint">Loading…</div></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Usage & credits</h1>
          <p className="page-sub">{env === "test" ? "Test mode is never metered — every operation is free here." : "Preview ledger · nominal 1,000 cr = $1 · payments/recharges disabled"}</p>
        </div>
        {env === "live" && <Pill kind="pending">Billing disabled · testnet preview</Pill>}
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 14 }}>
        <div className="card" style={{ padding: "16px 18px 14px" }}>
          <div style={{ fontSize: 12.5, fontWeight: 600, color: "var(--ink-2)" }}>Balance</div>
          {env === "test"
            ? <div className="mono" style={{ fontSize: 28, fontWeight: 600, marginTop: 9, color: "var(--pending)" }}>Free</div>
            : <div className="mono" style={{ fontSize: 28, fontWeight: 600, marginTop: 9 }}>{fmtCr(credits.balance_mcr ?? 0)} <span style={{ fontSize: 14, color: "var(--ink-3)", fontWeight: 500 }}>cr</span></div>}
        </div>
        <div className="card" style={{ padding: "16px 18px 14px" }}>
          <div style={{ fontSize: 12.5, fontWeight: 600, color: "var(--ink-2)" }}>Monthly free grant</div>
          <div className="mono" style={{ fontSize: 28, fontWeight: 600, marginTop: 9 }}>{fmtCr(credits.monthly_grant_mcr)} <span style={{ fontSize: 14, color: "var(--ink-3)", fontWeight: 500 }}>cr</span></div>
          <div style={{ fontSize: 12, color: "var(--ink-3)", marginTop: 2 }}>non-accumulating · renews monthly</div>
        </div>
        <div className="card" style={{ padding: "16px 18px 14px" }}>
          <div style={{ fontSize: 12.5, fontWeight: 600, color: "var(--ink-2)" }}>Operations · 30d</div>
          <div className="display" style={{ fontSize: 34, marginTop: 6 }}>{usage?.operations_30d ?? "…"}</div>
        </div>
      </div>

      <div className="card" style={{ overflow: "hidden" }}>
        <div style={{ padding: "16px 20px 12px", display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
          <span style={{ fontSize: 14, fontWeight: 650 }}>Credit ledger</span>
          <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>append-only preview usage ledger</span>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "1.2fr 1.4fr 2fr 1fr 1fr", gap: 12, padding: "9px 20px", borderTop: "1px solid var(--line)", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
          {["When", "Operation", "Detail", "Δ credits", "Balance"].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
        </div>
        {(credits.ledger || []).length === 0 && (
          <div style={{ padding: "22px 20px", textAlign: "center" }}>
            <span className="mono" style={{ fontSize: 12, color: "var(--ink-3)" }}>{env === "test" ? "Test operations aren't metered — switch to METERED to exercise the ledger." : "No metered operations yet."}</span>
          </div>
        )}
        {(credits.ledger || []).map((l, i) => (
          <div key={i} style={{ display: "grid", gridTemplateColumns: "1.2fr 1.4fr 2fr 1fr 1fr", gap: 12, padding: "11px 20px", borderBottom: "1px solid var(--line)", alignItems: "center", fontSize: 12.5 }}>
            <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-2)" }}>{fmtDateTime(l.ts)}</span>
            <span className="mono" style={{ fontSize: 12 }}>{l.operation}</span>
            <span style={{ color: "var(--ink-2)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{l.detail}</span>
            <span className="mono" style={{ fontSize: 12, color: l.delta_mcr > 0 ? "var(--valid)" : "var(--ink)", fontWeight: 600 }}>
              {l.delta_mcr > 0 ? "+" : ""}{fmtCr(l.delta_mcr)}
            </span>
            <span className="mono" style={{ fontSize: 12, color: "var(--ink-2)" }}>{fmtCr(l.balance_mcr)}</span>
          </div>
        ))}
      </div>

      <details className="card" style={{ padding: "16px 20px" }}>
        <summary style={{ fontSize: 14, fontWeight: 650, cursor: "pointer" }}>What things cost</summary>
        <div style={{ marginTop: 10 }}>
          {credits.price_table.map((p, i) => (
            <div key={i} style={{ display: "flex", justifyContent: "space-between", padding: "7px 0", borderTop: "1px solid var(--line)", fontSize: 12.5 }}>
              <span style={{ color: "var(--ink-2)" }}>{p.operation}</span>
              <span className="mono">{p.mcr === 0 ? "free" : `${fmtCr(p.mcr)} cr${p.per ? " / " + p.per : ""}`}</span>
            </div>
          ))}
          <p className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", margin: "10px 0 0" }}>Preview pricing only. Automated campaign keep-alive and its billing are not enabled yet.</p>
        </div>
      </details>

    </div>
  );
}

/* ── Webhooks ─────────────────────────────────────────────────────── */
export function WebhooksPage() {
  const { env, toast, toastErr } = useApp();
  const [data, setData] = useState(null);
  const [create, setCreate] = useState(false);
  const load = () => api.get("/v1/webhooks").then(setData).catch(toastErr);
  useEffect(() => { setData(null); load(); }, [env]);

  const action = async (id, name) => {
    try {
      await api.postIdem(`/v1/webhooks/${id}/${name}`, {}, idemKey());
      toast(name === "test" ? "Test queued" : "Endpoint disabled", name === "test" ? "Delivery status will update shortly." : "No new events will be delivered.");
      setTimeout(load, name === "test" ? 900 : 0);
    } catch (e) { toastErr(e); }
  };

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Webhooks</h1>
          <p className="page-sub">HMAC-SHA256 signed · HTTPS only · automatic retries</p>
        </div>
        <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setCreate(true)}>Add endpoint</BtnPrimary>
      </div>

      {data && data.webhooks.length === 0 && <Empty icon={<Ic.zap width={20} height={20} />} title="No endpoints yet">
        Subscribe an HTTPS endpoint to receive redemption, tally, settlement and loyalty events.
      </Empty>}

      {data && data.webhooks.length > 0 && <div className="card" style={{ overflow: "hidden" }}>
        {data.webhooks.map((hook) => <div key={hook.id} style={{ padding: "16px 20px", borderBottom: "1px solid var(--line)", display: "grid", gridTemplateColumns: "2fr 2fr 1fr auto", gap: 14, alignItems: "center", opacity: hook.active ? 1 : .55 }}>
          <div style={{ minWidth: 0 }}>
            <div className="mono" style={{ fontSize: 12.5, overflow: "hidden", textOverflow: "ellipsis" }}>{hook.url}</div>
            <div className="mono" style={{ fontSize: 11, color: "var(--ink-3)", marginTop: 3 }}>{hook.secret_prefix}</div>
          </div>
          <div style={{ display: "flex", gap: 5, flexWrap: "wrap" }}>{hook.events.map((event) => <span key={event} className="authtag">{event}</span>)}</div>
          <div><Pill kind={hook.last_status === "delivered" ? "valid" : hook.last_status === "failed" ? "burned" : "pending"}>{hook.last_status}</Pill></div>
          <div style={{ display: "flex", gap: 7 }}>
            {hook.active && <button className="btn-plain" style={{ fontSize: 12, padding: "6px 10px" }} onClick={() => action(hook.id, "test")}>Test</button>}
            {hook.active && <button className="btn-plain" style={{ fontSize: 12, padding: "6px 10px", color: "var(--burned)" }} onClick={() => action(hook.id, "disable")}>Disable</button>}
          </div>
        </div>)}
      </div>}

      <div className="card" style={{ padding: "16px 20px", fontSize: 12.5, color: "var(--ink-2)", lineHeight: 1.6 }}>
        Verify <span className="mono">X-Soroticket-Signature</span> over <span className="mono">timestamp + "." + raw_body</span>, reject stale timestamps, and deduplicate with <span className="mono">X-Soroticket-Delivery</span>.
      </div>
      {create && <CreateWebhookModal events={data?.supported_events || []} onClose={(changed) => { setCreate(false); if (changed) load(); }} />}
    </div>
  );
}

function CreateWebhookModal({ events, onClose }) {
  const [url, setURL] = useState("");
  const [selected, setSelected] = useState(() => new Set(events));
  const [created, setCreated] = useState(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);
  const toggle = (event) => setSelected((current) => {
    const next = new Set(current); next.has(event) ? next.delete(event) : next.add(event); return next;
  });
  const submit = async () => {
    setBusy(true); setErr(null);
    try { setCreated(await api.postIdem("/v1/webhooks", { url, events: [...selected] }, idemKey())); }
    catch (e) { setErr(e); } finally { setBusy(false); }
  };
  return <Modal onClose={() => onClose(!!created)} width={560}>
    {!created ? <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div><div style={{ fontSize: 16, fontWeight: 650 }}>Add webhook endpoint</div><div style={{ fontSize: 12.5, color: "var(--ink-3)", marginTop: 2 }}>Public HTTPS endpoints only; redirects and private network destinations are blocked.</div></div>
      <div className="field"><label>Endpoint URL</label><input className="input mono" value={url} onChange={(e) => setURL(e.target.value)} placeholder="https://example.com/soroticket/webhooks" autoFocus /></div>
      <div className="field"><label>Events</label><div style={{ display: "flex", flexDirection: "column", gap: 7 }}>{events.map((event) => <label key={event} style={{ display: "flex", gap: 8, alignItems: "center", fontSize: 12.5 }}><input type="checkbox" checked={selected.has(event)} onChange={() => toggle(event)} /><span className="mono">{event}</span></label>)}</div></div>
      <ErrText err={err} />
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 9 }}><button className="btn-plain" onClick={() => onClose(false)}>Cancel</button><BtnPrimary small busy={busy} onClick={submit}>Create endpoint</BtnPrimary></div>
    </div> : <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <div><div style={{ fontSize: 16, fontWeight: 650 }}>Copy the signing secret now</div><div style={{ fontSize: 12.5, color: "var(--ink-3)", marginTop: 2 }}>Only an encrypted copy is kept; the console will not reveal it again.</div></div>
      <div style={{ background: "var(--con-bg)", borderRadius: 12, padding: "14px 16px", display: "flex", alignItems: "center", gap: 10 }}><span className="mono" style={{ fontSize: 12.5, color: "var(--con-ink)", wordBreak: "break-all", flex: 1 }}>{created.webhook.secret}</span><button className="btn-plain" onClick={() => navigator.clipboard?.writeText(created.webhook.secret)}>Copy</button></div>
      <BtnPrimary small onClick={() => onClose(true)} style={{ alignSelf: "flex-end" }}>Done</BtnPrimary>
    </div>}
  </Modal>;
}

/* ── Settings ─────────────────────────────────────────────────────── */
export function SettingsPage() {
  const { env, org, user } = useApp();
  const account = org?.accounts?.[env];
  return (
    <div className="page">
      <div className="page-head"><div>
        <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Settings</h1>
        <p className="page-sub">{org?.name} · signed in as {user?.email}</p>
      </div></div>

      <div className="card" style={{ padding: "20px 24px", display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <span style={{ fontSize: 14, fontWeight: 650 }}>Your Stellar account · {env}</span>
          <span className="authtag"><Ic.lock width={11} height={11} />custodial · export unavailable in preview</span>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <CopyMono value={account?.public_key || ""} display={account?.public_key} size={13} />
          <a className="mono" style={{ fontSize: 12, color: "var(--ink-2)" }} target="_blank" rel="noreferrer"
            href={`https://stellar.expert/explorer/testnet/account/${account?.public_key}`}>↗ stellar.expert</a>
        </div>
        <p style={{ fontSize: 12.5, color: "var(--ink-2)", margin: 0, lineHeight: 1.55 }}>
          This preview holds the key and submits testnet network/storage operations; METERED deducts non-monetary preview credits.
          Campaign and settlement state is inspectable under this address. Shared-event claims additionally require the published signed receipts and Merkle proofs.
        </p>
      </div>

      <div className="card" style={{ padding: "20px 24px", display: "flex", flexDirection: "column", gap: 8 }}>
        <span style={{ fontSize: 14, fontWeight: 650 }}>Environments</span>
        <p style={{ fontSize: 12.5, color: "var(--ink-2)", margin: 0, lineHeight: 1.55 }}>
          <strong>TEST</strong> and <strong>METERED</strong> keep separate data, Stellar accounts and API keys.
          Both currently run on Stellar testnet; METERED only exercises the credit ledger. Mainnet is not enabled.
        </p>
      </div>

      <div className="card" style={{ padding: "20px 24px", borderColor: "color-mix(in oklch, var(--burned) 30%, var(--line))" }}>
        <span style={{ fontSize: 14, fontWeight: 650, color: "var(--burned)" }}>Danger zone</span>
        <p style={{ fontSize: 12.5, color: "var(--ink-2)", margin: "6px 0 0" }}>Organization deletion and a verified data-erasure workflow are not implemented in this preview.</p>
      </div>
    </div>
  );
}
