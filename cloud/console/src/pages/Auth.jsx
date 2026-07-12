import React, { useState } from "react";
import { api } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, ErrText, Ic, Pill } from "../ui.jsx";

function TicketArt() {
  return (
    <div className="ticket" style={{ width: 340 }}>
      <div className="ticket-perf" />
      <div style={{ padding: "26px 24px 18px", display: "flex", flexDirection: "column", gap: 12 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <span className="mono" style={{ fontSize: 11, color: "var(--ink-3)", letterSpacing: ".1em" }}>SORODEAL · ILLUSTRATION</span>
          <Pill kind="valid">Example</Pill>
        </div>
        <div className="mono" style={{ fontSize: 36, fontWeight: 600, letterSpacing: ".02em" }}>10OFF</div>
        <div style={{ display: "flex", gap: 16, fontSize: 13, color: "var(--ink-2)" }}>
          <span><strong style={{ color: "var(--ink)" }}>10%</strong> off</span>
          <span>·</span>
          <span><strong style={{ color: "var(--ink)" }}>signed</strong> receipts</span>
        </div>
      </div>
      <div className="ticket-tear"><i className="l" /><i className="r" /></div>
      <div style={{ padding: "14px 24px 18px", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <span className="mono" style={{ fontSize: 12, color: "var(--ink-2)" }}>no live data</span>
        <span className="mono" style={{ fontSize: 12, color: "var(--ink-2)" }}>visual example</span>
      </div>
    </div>
  );
}

export function AuthPage() {
  const { refreshMe, toastErr } = useApp();
  const [mode, setMode] = useState("signin"); // signin | signup
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);

  const submit = async () => {
    setBusy(true); setErr(null);
    try {
      await api.post(mode === "signin" ? "/auth/login" : "/auth/signup", { email, password });
      await refreshMe();
    } catch (e) {
      setErr(e);
    } finally { setBusy(false); }
  };

  return (
    <div className="auth-wrap">
      <div className="auth-left">
        <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
          <div className="side-logo"><Ic.coupon /></div>
          <span style={{ fontFamily: "var(--serif)", fontWeight: 560, fontSize: 21 }}>Sorodeal</span>
        </div>
        <div style={{ flex: 1, display: "grid", placeItems: "center" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 26, maxWidth: 380 }}>
            <TicketArt />
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              <h2 className="display" style={{ fontSize: 30, margin: 0 }}>Coupons with verifiable on-chain state.</h2>
              <p style={{ fontSize: 14, color: "var(--ink-2)", lineHeight: 1.55, margin: 0 }}>
                Unique-code burns are public on-chain. Shared-code receipts are signed off-chain
                and periodically anchored as an aggregate. This preview uses no mainnet funds.
              </p>
            </div>
          </div>
        </div>
        <div className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>Testnet preview · contract v0.2.0 · no mainnet funds</div>
      </div>
      <div className="auth-right">
        <div style={{ width: 400, display: "flex", flexDirection: "column", gap: 18 }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <h2 className="display" style={{ fontSize: 27, margin: 0 }}>{mode === "signin" ? "Sign in" : "Create account"}</h2>
            <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: 0 }}>
              {mode === "signin" ? "Welcome back to your console." : "An email is all it takes — we handle the Stellar side."}
            </p>
          </div>
          <div style={{ display: "inline-flex", background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 999, padding: 3, alignSelf: "flex-start" }}>
            {["signin", "signup"].map((m) => (
              <span key={m} onClick={() => setMode(m)} style={{
                padding: "6px 16px", borderRadius: 999, fontSize: 13, fontWeight: 600, cursor: "pointer",
                ...(mode === m
                  ? { background: "var(--surface)", border: "1px solid var(--line-2)", boxShadow: "var(--shadow-sm)" }
                  : { color: "var(--ink-3)" }),
              }}>{m === "signin" ? "Sign in" : "Create account"}</span>
            ))}
          </div>
          <div className="field">
            <label>Email</label>
            <input className="input" type="email" value={email} placeholder="maria@caferosales.co"
              onChange={(e) => setEmail(e.target.value)} onKeyDown={(e) => e.key === "Enter" && submit()} />
          </div>
          <div className="field">
            <label>Password</label>
            <input className="input" type="password" value={password} placeholder={mode === "signup" ? "15–72 bytes" : "••••••••••"}
              onChange={(e) => setPassword(e.target.value)} onKeyDown={(e) => e.key === "Enter" && submit()} />
          </div>
          <ErrText err={err} />
          <BtnPrimary busy={busy} onClick={submit} style={{ justifyContent: "space-between", width: "100%" }}>
            {mode === "signin" ? "Sign in" : "Create account"}
          </BtnPrimary>
          <p style={{ fontSize: 12.5, color: "var(--ink-3)", lineHeight: 1.5, margin: 0 }}>
            New here? Create an account with just an email — we handle the Stellar side for you.
          </p>
        </div>
      </div>
    </div>
  );
}

export function CreateOrgPage() {
  const { refreshMe, toastErr } = useApp();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    if (!name.trim()) return;
    setBusy(true);
    try {
      await api.post("/orgs", { name });
      await refreshMe();
    } catch (e) { toastErr(e); } finally { setBusy(false); }
  };

  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 40 }}>
      <div style={{ width: 460, display: "flex", flexDirection: "column", gap: 16 }}>
        <span className="eyebrow">One last step</span>
        <h2 className="display" style={{ fontSize: 30, margin: 0 }}>Name your organization</h2>
        <div className="field">
          <label>Organization name</label>
          <input className="input" value={name} placeholder="Cafe Rosales" autoFocus
            onChange={(e) => setName(e.target.value)} onKeyDown={(e) => e.key === "Enter" && submit()} />
          <span className="help">Used to identify the tenant. This preview supports one member per organization.</span>
        </div>
        <div style={{ background: "var(--surface-inset)", border: "1px solid var(--line)", borderRadius: 12, padding: "14px 16px", display: "flex", gap: 11 }}>
          <Ic.info width={18} height={18} style={{ flex: "none", marginTop: 1, color: "var(--ink-2)" }} />
          <p style={{ fontSize: 13, color: "var(--ink-2)", lineHeight: 1.55, margin: 0 }}>
            We create a testnet Stellar account for your organization and hold the key locally. The
            METERED environment uses preview credits; payment recharges and mainnet are disabled.
          </p>
        </div>
        <BtnPrimary busy={busy} onClick={submit} style={{ justifyContent: "space-between", width: "100%" }}>
          Create organization
        </BtnPrimary>
        <p className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", margin: 0, textAlign: "center" }}>
          starts in test mode · friendbot funding may take a few seconds
        </p>
      </div>
    </div>
  );
}
