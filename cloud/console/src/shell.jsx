import React, { useEffect, useState } from "react";
import { api, fmtCr, trunc } from "./api.js";
import { useApp } from "./store.jsx";
import { Ic } from "./ui.jsx";

const NAV = [
  ["/overview", "Overview", Ic.overview],
  ["/campaigns", "Campaigns", Ic.coupon],
  ["/redemptions", "Redemptions", Ic.check],
  ["/settlements", "Settlements", Ic.bank],
  ["/loyalty", "Loyalty", Ic.users],
  ["/keys", "API keys", Ic.lock],
  ["/usage", "Usage & credits", Ic.coins],
  ["/webhooks", "Webhooks", Ic.zap],
  ["/settings", "Settings", Ic.sliders],
];

export function Shell({ children }) {
  const { org, env, setEnv, route, nav, user, refreshMe } = useApp();
  const [credits, setCredits] = useState(null);
  const [userMenu, setUserMenu] = useState(false);

  const signOut = async () => {
    setUserMenu(false);
    try { await api.post("/auth/logout"); } catch { /* session may already be gone */ }
    await refreshMe(); // user → null re-renders the auth screen
    nav("/overview");
  };

  useEffect(() => {
    let live = true;
    if (env === "live") {
      api.get("/v1/credits").then((d) => live && setCredits(d.balance_mcr)).catch(() => {});
    } else setCredits(null);
  }, [env, route]);

  const initials = (org?.name || "??").split(/\s+/).map((w) => w[0]).join("").slice(0, 2).toUpperCase();
  const userInitials = (user?.email || "?").slice(0, 2).toUpperCase();
  const account = org?.accounts?.[env];

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="side-brand">
          <div className="side-logo"><Ic.coupon /></div>
          <div>
            <div className="side-word">Soroticket</div>
            <div className="side-sub">Console</div>
          </div>
        </div>
        <nav className="side-nav">
          {NAV.map(([path, label, Icon]) => (
            <button key={path} className={"side-link" + (route.startsWith(path) ? " on" : "")} onClick={() => nav(path)}>
              <Icon />{label}
            </button>
          ))}
        </nav>
        <div className="side-link" title="Local repository: docs/CLOUD.md and docs/SPEC.md">
          <Ic.doc />Docs<span className="mono" style={{ marginLeft: "auto", color: "var(--ink-3)", fontSize: 10 }}>repo</span>
        </div>
      </aside>

      <div className="main">
        {env === "test" ? <>
          <div className="test-strip" />
          <div className="test-banner">Test mode — v0.2.0 testnet contract, free. Nothing touches mainnet.</div>
        </> : <>
          <div className="test-strip" />
          <div className="test-banner">Metered preview — v0.2.0 testnet contract. Mainnet and production billing are disabled.</div>
        </>}
        <div className="topbar2">
          {/* display-only: single-org preview — no switcher until orgs/teams land */}
          <div className="org-btn" style={{ cursor: "default" }}>
            <span className="org-avatar">{initials}</span>
            <span style={{ fontSize: 13.5, fontWeight: 650 }}>{org?.name}</span>
            {account && <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{trunc(account.public_key)}</span>}
          </div>
          <div style={{ flex: 1 }} />
          <div className="env-toggle">
            <span className={"env-opt" + (env === "test" ? " on-test" : "")} onClick={() => setEnv("test")}>TEST</span>
            <span className={"env-opt" + (env === "live" ? " on-live" : "")} onClick={() => setEnv("live")}>METERED</span>
          </div>
          <span className="credits-chip mono" style={env === "test" ? { color: "var(--ink-2)" } : undefined}>
            {env === "test" ? "testnet · free" : <><Ic.coins width={13} height={13} style={{ color: "var(--ink-2)" }} />testnet · {credits == null ? "…" : fmtCr(credits) + " cr"}</>}
          </span>
          <div style={{ position: "relative" }}>
            <button className="user-dot" title={user?.email} onClick={() => setUserMenu((m) => !m)}
              style={{ cursor: "pointer", border: "none", font: "inherit" }}>{userInitials}</button>
            {userMenu && <>
              <div style={{ position: "fixed", inset: 0, zIndex: 90 }} onClick={() => setUserMenu(false)} />
              <div className="card" style={{ position: "absolute", right: 0, top: "calc(100% + 8px)", zIndex: 91, minWidth: 230, padding: 6, boxShadow: "var(--shadow)" }}>
                <div className="mono" style={{ padding: "9px 12px", fontSize: 12, color: "var(--ink-3)", borderBottom: "1px solid var(--line)", overflow: "hidden", textOverflow: "ellipsis" }}>{user?.email}</div>
                <button className="btn-plain" style={{ width: "100%", textAlign: "left", fontSize: 13.5, padding: "9px 12px" }} onClick={signOut}>Sign out</button>
              </div>
            </>}
          </div>
        </div>
        {children}
      </div>
    </div>
  );
}
