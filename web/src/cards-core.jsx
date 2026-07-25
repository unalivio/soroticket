/* ═══════════════════════════════════════════════════════════════════
   Soroticket Playground — action card shell + result primitives
   ═══════════════════════════════════════════════════════════════════ */

function ActionCard({ num, id, title, desc, auth, locked, lockMsg, snippets, label, children }) {
  const [tab, setTab] = useState("run");
  return (
    <div id={id} className={"acard card" + (locked ? " dimmed" : "")} data-screen-label={label}>
      <div className="acard-head">
        <div className="acard-num">{num}</div>
        <div className="acard-titles">
          <h3>{title}</h3>
          <p>{desc}</p>
        </div>
        <div className="head-right">
          <AuthTag auth={auth} />
          <RunCodeTabs tab={tab} setTab={setTab} />
        </div>
      </div>
      <div className="acard-body">
        {tab === "run"
          ? (locked ? <LockNotice msg={lockMsg} /> : children)
          : <CodeBlock snippets={snippets} />}
      </div>
    </div>
  );
}

function LockNotice({ msg }) {
  const { connect, connecting } = useApp();
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16, alignItems: "flex-start" }}>
      <div className="lock-overlay">
        <Ic.lock />
        <span>{msg || "Connect Freighter to run this call. The Code tab is available without a wallet."}</span>
      </div>
      <Btn variant="primary" small loading={connecting} onClick={connect}
           icon={<Ic.wallet style={{ width: 13, height: 13 }} />}>
        {connecting ? "Connecting" : "Connect Freighter"}
      </Btn>
    </div>
  );
}

/* success result wrapper */
function ResultPanel({ title, onClose, children }) {
  return (
    <div className="result">
      <div className="result-head">
        <Ic.ok />
        <span style={{ flex: 1 }}>{title || "Result"}</span>
        {onClose && <button onClick={onClose} className="faint" style={{ display: "grid", placeItems: "center", padding: 2 }}><Ic.x style={{ width: 15, height: 15 }} /></button>}
      </div>
      <div className="result-body">{children}</div>
    </div>
  );
}

/* error result wrapper */
function ErrorPanel({ error, onClose }) {
  if (!error) return null;
  return (
    <div className="result" style={{ borderColor: "var(--burned)" }}>
      <div className="result-head err">
        <Ic.alert />
        <span style={{ flex: 1 }}>{error.title}</span>
        <span className="mono" style={{ fontSize: 11, opacity: .7 }}>Error #{error.code}</span>
        {onClose && <button onClick={onClose} className="faint" style={{ display: "grid", placeItems: "center", padding: 2 }}><Ic.x style={{ width: 15, height: 15 }} /></button>}
      </div>
      <div className="result-body"><span style={{ fontSize: 14, color: "var(--ink-2)", lineHeight: 1.5 }}>{error.msg}</span></div>
    </div>
  );
}

/* key/value row */
function KV({ k, children }) {
  return (
    <div className="kv">
      <div className="kv-k">{k}</div>
      <div className="kv-v">{children}</div>
    </div>
  );
}

Object.assign(window, { ActionCard, LockNotice, ResultPanel, ErrorPanel, KV });
