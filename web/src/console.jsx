/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — console / activity log · toasts · footer
   ═══════════════════════════════════════════════════════════════════ */

function fmtJSON(obj) {
  return JSON.stringify(obj, (k, v) => typeof v === "bigint" ? v.toString() : v, 2);
}

function StatusDot({ status }) {
  return <span className={"con-dot con-" + status} />;
}

function ConsoleEntry({ e }) {
  const SD = window.SD;
  const [open, setOpen] = useState(false);
  const has = e.args || e.result || e.error;
  return (
    <div className={"con-entry" + (open ? " open" : "")}>
      <button className="con-row" onClick={() => has && setOpen((o) => !o)}>
        <StatusDot status={e.status} />
        <span className="con-method mono">{e.method}</span>
        {e.read && <span className="con-badge">read</span>}
        <span className="con-status">{e.status === "pending" ? "pending…" : e.status === "ok" ? "success" : (e.error?.title || "error")}</span>
        <span className="con-spacer" />
        {e.tx && <a href={SD.NET.explorer + "/tx/" + e.tx} target="_blank" rel="noopener" className="con-tx mono" onClick={(ev) => ev.stopPropagation()}>{SD.trunc(e.tx, 6, 4)}<Ic.ext style={{ width: 11, height: 11 }} /></a>}
        <span className="con-time mono">{SD.fmtClock(e.at)}</span>
        {has && <Ic.chevron className="con-chev" style={{ width: 14, height: 14 }} />}
      </button>
      {open && has && (
        <div className="con-detail">
          {e.args && <div className="con-json"><span className="con-jlabel">args</span><pre className="mono">{fmtJSON(e.args)}</pre></div>}
          {e.status === "ok" && e.result && <div className="con-json"><span className="con-jlabel">result</span><pre className="mono">{fmtJSON(e.result)}</pre></div>}
          {e.status === "err" && e.error && <div className="con-json"><span className="con-jlabel" style={{ color: "var(--burned)" }}>error #{e.error.code}</span><pre className="mono" style={{ color: "#F0A89E" }}>{e.error.title} — {e.error.msg}</pre></div>}
        </div>
      )}
    </div>
  );
}

function Console({ placement }) {
  const { log, clearLog } = useApp();
  const [open, setOpen] = useState(false);
  const pending = log.filter((e) => e.status === "pending").length;
  const latest = log[0];

  return (
    <div className={"console console-" + placement + (open ? " open" : "")}>
      <button className="con-head" onClick={() => setOpen((o) => !o)}>
        <Ic.terminal style={{ width: 16, height: 16 }} />
        <span className="con-title">Console</span>
        <span className="con-count">{log.length}</span>
        {!open && latest && (
          <span className="con-peek">
            <StatusDot status={latest.status} />
            <span className="mono">{latest.method}</span>
            <span className="faint">{latest.status === "pending" ? "pending…" : latest.status === "ok" ? "ok" : "error"}</span>
          </span>
        )}
        {pending > 0 && <span className="con-live"><span className="con-dot con-pending" />{pending} running</span>}
        <span className="con-spacer" />
        {open && log.length > 0 && <span className="con-clear" onClick={(e) => { e.stopPropagation(); clearLog(); }}>Clear</span>}
        <Ic.chevron className="con-headchev" style={{ width: 16, height: 16, transform: open ? "rotate(180deg)" : "none" }} />
      </button>
      {open && (
        <div className="con-list">
          {log.length === 0
            ? <div className="con-empty"><Ic.terminal style={{ width: 22, height: 22, opacity: .4 }} /><span>No calls yet — connect Freighter and create your first campaign.</span></div>
            : log.map((e) => <ConsoleEntry key={e.id} e={e} />)}
        </div>
      )}
    </div>
  );
}

/* ── Toasts ─────────────────────────────────────────────────── */
function Toasts() {
  const { toasts, dismissToast } = useApp();
  const ico = { success: Ic.ok, error: Ic.alert, info: Ic.info };
  return (
    <div className="toast-wrap">
      {toasts.map((t) => {
        const I = ico[t.kind] || Ic.info;
        return (
          <div key={t.id} className={"toast toast-" + t.kind} onClick={() => dismissToast(t.id)}>
            <I className="ic" />
            <div style={{ flex: 1 }}>
              <div className="t-title">{t.title}</div>
              {t.msg && <div className="t-msg">{t.msg}</div>}
            </div>
          </div>
        );
      })}
    </div>
  );
}

/* ── Benefits band ──────────────────────────────────────────── */
const BENEFITS = [
  { ic: Ic.lock, h: "Single-use, enforced on-chain", p: "The Burn profile guarantees each unique code can be redeemed exactly once. Double-spend is rejected by the contract itself — not your backend." },
  { ic: Ic.shield, h: "No plaintext PII on-chain", p: "Redeemer identity can be committed as an opaque 32-byte value computed with a random nonce. Keep the nonce and source reference off-chain." },
  { ic: Ic.globe, h: "Permissionless ownership", p: "Anyone can create a campaign from their own keypair. Each campaign is owned by its creator — there is no global admin." },
  { ic: Ic.users, h: "Delegated redemption", p: "Owners grant explicit delegates the right to redeem at the point of sale, while keeping supply issuance owner-only." },
  { ic: Ic.spark, h: "Token settlement primitive", p: "Contract v0.2 settles a configured Stellar Asset Contract from an owner-approved allowance — any keeper can trigger it. Network fees and asset choice remain deployment decisions." },
  { ic: Ic.terminal, h: "Generated integration examples", p: "Actions produce Stellar CLI, TypeScript and Go examples from your inputs. Review deployment IDs, addresses and signer handling before running them." },
];
function Benefits() {
  return (
    <section className="band-dark" style={{ padding: "84px 0" }}>
      <div className="wrap">
        <div className="section-head">
          <div className="eyebrow">Why Sorodeal</div>
          <h2 className="display">A standard, not a silo</h2>
        </div>
        <div className="ben-grid">
          {BENEFITS.map((b, i) => {
            const I = b.ic;
            return (
              <div className="ben" key={i}>
                <I className="ben-ic" />
                <h4>{b.h}</h4>
                <p>{b.p}</p>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}

/* ── Footer ─────────────────────────────────────────────────── */
function Footer() {
  const SD = window.SD;
  const links = [
    { t: "Protocol Spec · docs/SPEC.md", h: null },
    { t: "Candidate SEP · docs/SEP.md", h: null },
    { t: "Contract on stellar.expert", h: SD.NET.explorer + "/contract/" + SD.NET.contractId },
  ];
  return (
    <footer style={{ padding: "56px 0 120px", borderTop: "1px solid var(--line)" }}>
      <div className="wrap">
        <div style={{ display: "flex", gap: 32, flexWrap: "wrap", alignItems: "flex-start", justifyContent: "space-between" }}>
          <div style={{ maxWidth: 320 }}>
            <div className="brand" style={{ marginBottom: 12 }}>
              <span className="mark"><Logo className="logo" /><span className="word">Sorodeal</span></span>
            </div>
            <p className="muted" style={{ fontSize: 13.5, lineHeight: 1.55 }}>The open coupon protocol on Stellar — legacy v0.1 testnet preview.</p>
          </div>
          <div style={{ display: "flex", gap: 40, flexWrap: "wrap" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: 11 }}>
              <span className="eyebrow" style={{ marginBottom: 4 }}>Protocol</span>
              {links.map((l) => l.h ? (
                <a key={l.t} href={l.h} target="_blank" rel="noopener"
                   style={{ fontSize: 14, fontWeight: 500, display: "inline-flex", alignItems: "center", gap: 6 }}
                   className="footer-link">{l.t}<Ic.arrowUR style={{ width: 13, height: 13, opacity: .45 }} /></a>
              ) : <span key={l.t} className="mono" style={{ fontSize: 12, color: "var(--ink-3)" }}>{l.t}</span>)}
            </div>
          </div>
        </div>
        <div className="divider" style={{ margin: "36px 0 20px" }} />
        <div style={{ display: "flex", justifyContent: "space-between", gap: 16, flexWrap: "wrap", alignItems: "center" }}>
          <span className="faint" style={{ fontSize: 12.5 }}>Public-good protocol. Testnet only — no real value.</span>
          <span className="net-pill"><span className="dot" />Stellar Testnet</span>
        </div>
      </div>
    </footer>
  );
}

Object.assign(window, { Console, Toasts, Benefits, Footer });
