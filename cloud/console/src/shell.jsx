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
  const { org, env, setEnv, route, nav, user } = useApp();
  const [credits, setCredits] = useState(null);

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
            <div className="side-word">Sorodeal</div>
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
          <div className="test-banner">Test mode — deprecated v0.1 testnet contract, free. Candidate v0.2 is not deployed; nothing touches mainnet.</div>
        </> : <>
          <div className="test-strip" />
          <div className="test-banner">Metered preview — deprecated v0.1 testnet contract. Candidate v0.2, mainnet and production billing are disabled.</div>
        </>}
        <div className="topbar2">
          <button className="org-btn">
            <span className="org-avatar">{initials}</span>
            <span style={{ fontSize: 13.5, fontWeight: 650 }}>{org?.name}</span>
            {account && <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>{trunc(account.public_key)}</span>}
            <Ic.chevron width={13} height={13} style={{ color: "var(--ink-3)" }} />
          </button>
          <div style={{ flex: 1 }} />
          <div className="env-toggle">
            <span className={"env-opt" + (env === "test" ? " on-test" : "")} onClick={() => setEnv("test")}>TEST</span>
            <span className={"env-opt" + (env === "live" ? " on-live" : "")} onClick={() => setEnv("live")}>METERED</span>
          </div>
          <span className="credits-chip mono" style={env === "test" ? { color: "var(--ink-2)" } : undefined}>
            {env === "test" ? "testnet · free" : <><Ic.coins width={13} height={13} style={{ color: "var(--ink-2)" }} />testnet · {credits == null ? "…" : fmtCr(credits) + " cr"}</>}
          </span>
          <span className="user-dot">{userInitials}</span>
        </div>
        {children}
      </div>
    </div>
  );
}
