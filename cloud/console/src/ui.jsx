/* Shared primitives — icon set recreated 1:1 from the design's inline SVGs. */
import React from "react";

const S = (props) => ({
  viewBox: "0 0 24 24", fill: "none", stroke: "currentColor",
  strokeWidth: 1.8, strokeLinecap: "round", strokeLinejoin: "round",
  width: 16, height: 16, ...props,
});

export const Ic = {
  coupon: (p) => (<svg {...S(p)}><path d="M3.5 6.5h17v3.1a2.4 2.4 0 0 0 0 4.8v3.1h-17v-3.1a2.4 2.4 0 0 0 0-4.8z"/><path d="M15 7.5v1.4M15 11.3v1.4M15 15.1v1.4"/></svg>),
  overview: (p) => (<svg {...S(p)}><path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8"/></svg>),
  check: (p) => (<svg {...S({ strokeWidth: 2.2, ...p })}><polyline points="4 12.5 9.5 18 20 6"/></svg>),
  bank: (p) => (<svg {...S({ strokeWidth: 1.7, ...p })}><path d="M4 10l8-5 8 5"/><path d="M5 10v8M9 10v8M15 10v8M19 10v8"/><path d="M3 21h18"/></svg>),
  users: (p) => (<svg {...S(p)}><circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0"/><path d="M16 6.2a3 3 0 0 1 0 5.6M17.5 19a5.5 5.5 0 0 0-3-4.9"/></svg>),
  lock: (p) => (<svg {...S(p)}><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>),
  coins: (p) => (<svg {...S({ strokeWidth: 1.7, ...p })}><ellipse cx="9" cy="7" rx="6" ry="3"/><path d="M3 7v5c0 1.7 2.7 3 6 3s6-1.3 6-3V7"/><path d="M9 12v5c0 1.7 2.7 3 6 3s6-1.3 6-3v-5c0-1.7-2.7-3-6-3"/></svg>),
  zap: (p) => (<svg {...S(p)}><path d="M13 2L4 13.5h6L11 22l9-11.5h-6z"/></svg>),
  sliders: (p) => (<svg {...S(p)}><path d="M4 7h16M4 12h16M4 17h16"/><circle cx="15" cy="7" r="2.1" fill="var(--bg)"/><circle cx="9" cy="12" r="2.1" fill="var(--bg)"/><circle cx="16" cy="17" r="2.1" fill="var(--bg)"/></svg>),
  doc: (p) => (<svg {...S(p)}><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/></svg>),
  arrow: (p) => (<svg {...S({ strokeWidth: 2.1, ...p })}><line x1="5" y1="12" x2="19" y2="12"/><polyline points="13 6 19 12 13 18"/></svg>),
  plus: (p) => (<svg {...S({ strokeWidth: 2.1, ...p })}><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>),
  chevron: (p) => (<svg {...S({ strokeWidth: 2.1, ...p })}><polyline points="6 9 12 15 18 9"/></svg>),
  x: (p) => (<svg {...S({ strokeWidth: 2, ...p })}><line x1="6" y1="6" x2="18" y2="18"/><line x1="18" y1="6" x2="6" y2="18"/></svg>),
  copy: (p) => (<svg {...S(p)}><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>),
  info: (p) => (<svg {...S({ strokeWidth: 1.9, ...p })}><circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16"/><circle cx="12" cy="8" r="0.6" fill="currentColor" stroke="currentColor"/></svg>),
  spark: (p) => (<svg {...S(p)}><path d="M12 3l1.9 5.6L19.5 10l-5.6 1.9L12 17.5l-1.9-5.6L4.5 10l5.6-1.4z"/></svg>),
  ticket: (p) => (<svg {...S(p)}><path d="M3.5 8.5v-2a1 1 0 0 1 1-1h15a1 1 0 0 1 1 1v2a2.5 2.5 0 0 0 0 7v2a1 1 0 0 1-1 1h-15a1 1 0 0 1-1-1v-2a2.5 2.5 0 0 0 0-7z"/><path d="M14 6v2M14 11v2M14 16v2"/></svg>),
  gift: (p) => (<svg {...S(p)}><rect x="4" y="10" width="16" height="10" rx="1.5"/><path d="M4 10h16M12 10v10"/><path d="M12 10s-4.2.2-5.4-1.3a2 2 0 0 1 2.8-2.8C10.9 7.1 12 10 12 10zM12 10s4.2.2 5.4-1.3a2 2 0 0 0-2.8-2.8C13.1 7.1 12 10 12 10z"/></svg>),
  pin: (p) => (<svg {...S(p)}><path d="M12 21s-6.5-5.4-6.5-10.2a6.5 6.5 0 0 1 13 0C18.5 15.6 12 21 12 21z"/><circle cx="12" cy="10.5" r="2.3"/></svg>),
  ext: (p) => (<svg {...S({ strokeWidth: 2, ...p })}><path d="M8 6h10v10"/><path d="M18 6L6 18"/></svg>),
  ok: (p) => (<svg {...S({ strokeWidth: 2, ...p })}><circle cx="12" cy="12" r="9"/><polyline points="8 12.5 11 15.5 16 9.5"/></svg>),
  alert: (p) => (<svg {...S({ strokeWidth: 2, ...p })}><circle cx="12" cy="12" r="9"/><line x1="12" y1="8" x2="12" y2="13"/><circle cx="12" cy="16.4" r="0.6" fill="currentColor" stroke="currentColor"/></svg>),
  spinner: (p) => (<svg {...S({ strokeWidth: 2.2, ...p })} className={"spin " + (p?.className || "")}><path d="M12 3a9 9 0 1 0 9 9"/></svg>),
};

export function Toast({ t }) {
  const ic = t.type === "error" ? <Ic.alert width={20} height={20} /> :
    t.type === "info" ? <Ic.info width={20} height={20} /> : <Ic.ok width={20} height={20} />;
  return (
    <div className={`toast toast-${t.type || "success"}`}>
      <span className="ic">{ic}</span>
      <div>
        <div className="t-title">{t.title}</div>
        {t.msg && <div className="t-msg">{t.msg}</div>}
      </div>
    </div>
  );
}

export function Modal({ onClose, width = 640, children }) {
  return (
    <div className="overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose?.()}>
      <div className="modal" style={{ width }}>
        <button className="modal-x" onClick={onClose}><Ic.x width={16} height={16} /></button>
        {children}
      </div>
    </div>
  );
}

export function Kpi({ label, value, sub, mono, subColor }) {
  return (
    <div className="card" style={{ padding: "16px 18px 14px" }}>
      <div style={{ fontSize: 12.5, fontWeight: 600, color: "var(--ink-2)" }}>{label}</div>
      {mono
        ? <div className="mono" style={{ fontSize: 28, fontWeight: 600, marginTop: 9 }}>{value}</div>
        : <div className="display" style={{ fontSize: 36, marginTop: 6 }}>{value}</div>}
      {sub && <div style={{ fontSize: 12, color: subColor || "var(--ink-3)", marginTop: 2, fontWeight: subColor ? 600 : 400 }}>{sub}</div>}
    </div>
  );
}

export function CopyMono({ value, display, size = 12 }) {
  const [copied, setCopied] = React.useState(false);
  return (
    <button className={"copy-bare" + (copied ? " copied" : "")} style={{ fontSize: size }}
      onClick={(e) => { e.stopPropagation(); navigator.clipboard?.writeText(value); setCopied(true); setTimeout(() => setCopied(false), 1200); }}>
      {display || value}
      <span className="ci">{copied ? <Ic.check width={11} height={11} /> : <Ic.copy width={11} height={11} />}</span>
    </button>
  );
}

export function TxLink({ hash, children }) {
  if (!hash) return null;
  return (
    <a className="mono" style={{ fontSize: 11, color: "var(--ink-3)", whiteSpace: "nowrap" }}
      href={`https://stellar.expert/explorer/testnet/tx/${hash}`} target="_blank" rel="noreferrer">
      {children || (hash.slice(0, 4) + "…" + hash.slice(-2) + " ↗")}
    </a>
  );
}

export function Empty({ icon, title, children, cta }) {
  return (
    <div className="empty">
      {icon && <div className="face-ic" style={{ width: 46, height: 46 }}>{icon}</div>}
      {title && <h2 className="display" style={{ fontSize: 22, margin: 0 }}>{title}</h2>}
      {children && <p style={{ fontSize: 13.5, color: "var(--ink-2)", maxWidth: "38em", margin: 0, lineHeight: 1.55 }}>{children}</p>}
      {cta}
    </div>
  );
}

export function BtnPrimary({ children, onClick, busy, disabled, icon, small, style }) {
  return (
    <button className={`btn btn-primary ${small ? "btn-sm" : ""} ${busy ? "btn-busy" : ""}`}
      style={style} disabled={disabled || busy} onClick={onClick}>
      {children}
      <span className="chip">{busy ? <Ic.spinner width={13} height={13} /> : (icon || <Ic.arrow width={small ? 13 : 15} height={small ? 13 : 15} />)}</span>
    </button>
  );
}

export function Pill({ kind, children }) {
  return <span className={`pill ${kind ? "pill-" + kind : ""}`}><span className="dot" style={!kind ? { background: "var(--accent)" } : undefined}></span>{children}</span>;
}

export function ErrText({ err }) {
  if (!err) return null;
  return (
    <div style={{ background: "var(--burned-bg)", color: "var(--burned)", borderRadius: 10, padding: "10px 13px", fontSize: 13, fontWeight: 600 }}>
      {err.code ? `#${err.code} ${err.name} — ` : ""}{err.message}
    </div>
  );
}
