/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — top bar
   ═══════════════════════════════════════════════════════════════════ */

/* coupon/ticket mark */
function Logo(props) {
  return (
    <svg viewBox="0 0 32 32" fill="none" {...props}>
      <path d="M5 9.5A2.5 2.5 0 0 1 7.5 7h17A2.5 2.5 0 0 1 27 9.5v3a2.6 2.6 0 0 0 0 7v3a2.5 2.5 0 0 1-2.5 2.5h-17A2.5 2.5 0 0 1 5 22.5v-3a2.6 2.6 0 0 0 0-7z"
        fill="var(--accent)" stroke="var(--ink)" strokeWidth="1.6" strokeLinejoin="round"/>
      <path d="M16 10.5v11" stroke="var(--ink)" strokeWidth="1.5" strokeLinecap="round" strokeDasharray="1.4 2.6"/>
      <circle cx="11" cy="16" r="1.5" fill="var(--ink)"/>
    </svg>
  );
}

function TopBar() {
  const { wallet, connecting, connect, disconnect } = useApp();
  const SD = window.SD;
  const [menu, setMenu] = useState(false);
  const wrapRef = useRef(null);

  useEffect(() => {
    if (!menu) return;
    const h = (e) => { if (wrapRef.current && !wrapRef.current.contains(e.target)) setMenu(false); };
    document.addEventListener("mousedown", h);
    return () => document.removeEventListener("mousedown", h);
  }, [menu]);

  return (
    <header className="topbar">
      <div className="topbar-in">
        <div className="brand">
          <span className="mark">
            <Logo className="logo" />
            <span className="word">Sorodeal</span>
          </span>
          <span className="sub">Playground</span>
        </div>

        <div className="tb-spacer" />

        <div className="tb-right">
          <span className="net-pill tb-net"><span className="dot" /><span className="lbl">Stellar&nbsp;</span>Testnet</span>

          <span className="tb-cid">
            <CopyValue
              value={SD.NET.contractId}
              display={SD.trunc(SD.NET.contractId, 4, 3)}
              label="Contract ID"
              link={SD.NET.explorer + "/contract/" + SD.NET.contractId}
            />
          </span>

          {!wallet ? (
            <Btn variant="primary" small loading={connecting} onClick={connect}
                 icon={<Ic.wallet style={{ width: 15, height: 15 }} />}>
              {connecting ? "Connecting" : "Connect Freighter"}
            </Btn>
          ) : (
            <div className="wallet-wrap" ref={wrapRef}>
              <button className="wallet-btn" onClick={() => setMenu((m) => !m)}>
                <span className="av" />
                <span className="mono">{SD.trunc(wallet.address, 4, 3)}</span>
                <Ic.chevron style={{ width: 14, height: 14, opacity: .5, transform: menu ? "rotate(180deg)" : "none", transition: "transform .18s" }} />
              </button>
              {menu && (
                <div className="wallet-menu">
                  <div className="wm-head">
                    <div className="faint" style={{ fontSize: 11, fontWeight: 600, letterSpacing: ".08em", textTransform: "uppercase", marginBottom: 8 }}>Connected account</div>
                    <CopyValue value={wallet.address} display={SD.trunc(wallet.address, 6, 6)} label="Address"
                               link={SD.NET.explorer + "/account/" + wallet.address} />
                  </div>
                  <div className="divider" />
                  <div className="wm-row"><span className="muted">XLM balance</span><span className="mono" style={{ fontWeight: 600 }}>{wallet.balance} XLM</span></div>
                  <div className="wm-row"><span className="muted">Network</span><span style={{ fontWeight: 600 }}>Testnet</span></div>
                  <div className="divider" />
                  <button className="wm-btn" onClick={() => { setMenu(false); disconnect(); }}>
                    <Ic.x style={{ width: 15, height: 15 }} />Disconnect
                  </button>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </header>
  );
}

Object.assign(window, { TopBar, Logo });
