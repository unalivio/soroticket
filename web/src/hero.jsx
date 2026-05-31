/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — hero + lifecycle diagram + hero illustration
   ═══════════════════════════════════════════════════════════════════ */

/* Stellar-style charcoal + yellow-brush "coupon machine" illustration.
   Built from geometric primitives — rounded tickets, brush highlights,
   a burn stamp, floating code chips, a settlement coin. */
function HeroArt() {
  const ink = "var(--ink)";
  const acc = "var(--accent)";
  // rough highlighter rectangle path (slightly irregular, like a brush swipe)
  const brush = (x, y, w, h) =>
    `M${x - 2},${y + 3} C${x + w * .3},${y - 4} ${x + w * .7},${y + 5} ${x + w + 3},${y - 1} `
    + `C${x + w - 2},${y + h * .4} ${x + w + 4},${y + h - 2} ${x + w - 1},${y + h + 3} `
    + `C${x + w * .6},${y + h - 5} ${x + w * .3},${y + h + 4} ${x - 3},${y + h - 1} `
    + `C${x + 2},${y + h * .6} ${x - 4},${y + h * .3} ${x - 2},${y + 3} Z`;

  const Ticket = ({ x, y, r, lit }) => (
    <g transform={`translate(${x} ${y}) rotate(${r})`}>
      <path d="M0 6a6 6 0 0 1 6-6h70a6 6 0 0 1 6 6v8a5 5 0 0 0 0 10v8a6 6 0 0 1-6 6H6a6 6 0 0 1-6-6v-8a5 5 0 0 0 0-10z"
            fill="var(--surface)" stroke={ink} strokeWidth="2" strokeLinejoin="round"/>
      <line x1="50" y1="4" x2="50" y2="34" stroke={ink} strokeWidth="1.6" strokeDasharray="2 3" opacity=".55"/>
      <circle cx="20" cy="13" r="6" fill={lit ? acc : "none"} stroke={ink} strokeWidth="1.8"/>
      <line x1="60" y1="13" x2="74" y2="13" stroke={ink} strokeWidth="2" strokeLinecap="round"/>
      <line x1="60" y1="22" x2="70" y2="22" stroke={ink} strokeWidth="2" strokeLinecap="round" opacity=".5"/>
    </g>
  );

  return (
    <div className="hero-art">
      <style>{`
        .hero-art { position: relative; width: 100%; max-width: 520px; margin: 0 auto; aspect-ratio: 1.18 / 1; }
        .hero-art svg { width: 100%; height: 100%; overflow: visible; }
        .ha-float { animation: haFloat 6s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
        .ha-float.b { animation-duration: 7.5s; animation-delay: -2s; }
        .ha-float.c { animation-duration: 8.5s; animation-delay: -4s; }
        @keyframes haFloat { 0%,100% { transform: translateY(0); } 50% { transform: translateY(-7px); } }
        .ha-stamp { animation: haStamp 4.5s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
        @keyframes haStamp { 0%,72%,100% { transform: scale(1); } 80% { transform: scale(.9) rotate(-2deg); } 88% { transform: scale(1.04); } }
        .ha-spark { animation: haSpark 4.5s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
        @keyframes haSpark { 0%,78%,100% { opacity: 0; transform: scale(.4); } 84%,94% { opacity: 1; transform: scale(1); } }
        .ha-dash { stroke-dasharray: 4 5; animation: haDash 1.4s linear infinite; }
        @keyframes haDash { to { stroke-dashoffset: -18; } }
        .ha-coin { animation: haCoin 5s ease-in-out infinite; transform-box: fill-box; transform-origin: center; }
        @keyframes haCoin { 0%,100% { transform: translateY(0) rotate(0); } 50% { transform: translateY(-9px) rotate(8deg); } }
      `}</style>
      <svg viewBox="0 0 520 440" fill="none">
        {/* brush highlights behind */}
        <path d={brush(70, 150, 200, 150)} fill="var(--accent)" opacity=".9"/>
        <path d={brush(305, 70, 120, 90)} fill="var(--accent)" opacity=".85"/>
        <path d={brush(330, 300, 110, 80)} fill="var(--accent)" opacity=".8"/>

        {/* conveyor / flow line */}
        <path className="ha-dash" d="M40 360 H470" stroke="var(--ink)" strokeWidth="2" strokeLinecap="round" opacity=".4"/>
        <circle cx="40" cy="360" r="4" fill="var(--ink)" opacity=".4"/>
        <circle cx="470" cy="360" r="4" fill="var(--ink)" opacity=".4"/>

        {/* dotted issue arc from campaign card to tickets */}
        <path d="M150 250 C200 200 250 195 300 215" stroke="var(--ink)" strokeWidth="1.6" strokeDasharray="2 4" opacity=".4" fill="none"/>

        {/* campaign block (source) */}
        <g className="ha-float">
          <rect x="60" y="200" width="120" height="92" rx="12" fill="var(--surface)" stroke="var(--ink)" strokeWidth="2"/>
          <rect x="74" y="216" width="46" height="9" rx="4.5" fill="var(--ink)"/>
          <rect x="74" y="234" width="92" height="6" rx="3" fill="var(--ink)" opacity=".35"/>
          <rect x="74" y="248" width="78" height="6" rx="3" fill="var(--ink)" opacity=".35"/>
          <rect x="74" y="266" width="34" height="14" rx="7" fill="var(--accent)" stroke="var(--ink)" strokeWidth="1.6"/>
        </g>

        {/* floating tickets (codes) */}
        <g className="ha-float b"><Ticket x={300} y={188} r={-7} lit={false} /></g>
        <g className="ha-float c"><Ticket x={322} y={250} r={5} lit={true} /></g>

        {/* burn stamp (redemption) */}
        <g transform="translate(390 130)">
          <g className="ha-stamp">
            <circle cx="0" cy="0" r="42" fill="var(--surface)" stroke="var(--ink)" strokeWidth="2.4"/>
            <circle cx="0" cy="0" r="33" fill="none" stroke="var(--ink)" strokeWidth="1.4" strokeDasharray="2 3" opacity=".55"/>
            <path d="M-16 2 L-5 13 L16 -11" stroke="var(--ink)" strokeWidth="3.4" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
          </g>
          <g className="ha-spark" stroke="var(--ink)" strokeWidth="2.4" strokeLinecap="round">
            <line x1="48" y1="-30" x2="56" y2="-38"/>
            <line x1="52" y1="-8" x2="62" y2="-10"/>
            <line x1="44" y1="-50" x2="49" y2="-58"/>
          </g>
        </g>

        {/* settlement coin */}
        <g className="ha-coin" transform="translate(430 350)">
          <circle cx="0" cy="0" r="26" fill="var(--accent)" stroke="var(--ink)" strokeWidth="2.2"/>
          <circle cx="0" cy="0" r="19" fill="none" stroke="var(--ink)" strokeWidth="1.4" opacity=".5"/>
          <path d="M0 -11 V11 M-6 -5 a6 5 0 0 1 6 -3 a6 5 0 0 1 0 9 a6 5 0 0 0 0 9 a6 5 0 0 0 6 -3" stroke="var(--ink)" strokeWidth="2" fill="none" strokeLinecap="round"/>
        </g>
      </svg>
    </div>
  );
}

/* ── Lifecycle diagram (steps light up in sequence) ─────────── */
const LC_STEPS = [
  { t: "Campaign", d: "Owner sets the terms", ic: Ic.spark },
  { t: "Code", d: "Unique codes issued", ic: Ic.hash },
  { t: "Redemption", d: "Burned, single-use", ic: Ic.fire },
  { t: "Settlement", d: "Optional USDC payout", ic: Ic.globe },
];
function Lifecycle() {
  const [lit, setLit] = useState(0);
  useEffect(() => {
    const iv = setInterval(() => setLit((l) => (l + 1) % 4), 1500);
    return () => clearInterval(iv);
  }, []);
  return (
    <div className="lifecycle">
      {LC_STEPS.map((s, i) => {
        const I = s.ic;
        return (
          <div key={i} className={"lc-step" + (i === lit ? " lit" : "")} onMouseEnter={() => setLit(i)}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <div className="lc-ic"><I style={{ width: 18, height: 18 }} /></div>
              <span className="lc-num">{String(i + 1).padStart(2, "0")}</span>
            </div>
            <div>
              <div className="lc-t">{s.t}</div>
              <div className="lc-d">{s.d}</div>
            </div>
            {i < 3 && <Ic.arrow className="lc-arrow" />}
          </div>
        );
      })}
    </div>
  );
}

/* ── Hero section ───────────────────────────────────────────── */
function Hero() {
  const { wallet, connecting, connect } = useApp();
  const SD = window.SD;
  return (
    <section className="hero">
      <div className="wrap">
        <div className="hero-grid">
          <div>
            <span className="eyebrow">The open coupon protocol on Stellar</span>
            <h1 className="display">Issue, redeem &amp; verify coupons on-chain.</h1>
            <p className="lede">
              Sorodeal is a permissionless protocol for tokenized coupons and redeemable codes on
              Stellar Soroban. This is the live testnet playground — call the contract, see real
              results, and copy the exact integration code for every action.
            </p>
            <div className="cta-row">
              {!wallet ? (
                <Btn variant="primary" loading={connecting} onClick={connect}
                     icon={<Ic.wallet style={{ width: 15, height: 15 }} />}>
                  {connecting ? "Connecting Freighter" : "Connect Freighter"}
                </Btn>
              ) : (
                <Btn variant="primary" onClick={() => document.getElementById("create")?.scrollIntoView({ behavior: "smooth", block: "start" })}>
                  Create a campaign
                </Btn>
              )}
              <a className="btn btn-ghost" href="#verify" onClick={(e) => { e.preventDefault(); document.getElementById("verify")?.scrollIntoView({ behavior: "smooth", block: "start" }); }}>
                <span>Verify a code</span>
                <span className="chip"><Ic.arrow style={{ width: 15, height: 15 }} /></span>
              </a>
            </div>

            <div className="meta-row">
              <div className="meta-item">
                <span className="k">Network</span>
                <span className="net-pill" style={{ alignSelf: "flex-start" }}><span className="dot" />Stellar Testnet</span>
              </div>
              <div className="meta-item">
                <span className="k">Contract</span>
                <CopyValue value={SD.NET.contractId} display={SD.trunc(SD.NET.contractId, 6, 4)}
                           link={SD.NET.explorer + "/contract/" + SD.NET.contractId} />
              </div>
              <div className="meta-item">
                <span className="k">Profile</span>
                <span style={{ fontWeight: 600, fontSize: 14 }}>Burn · synchronous</span>
              </div>
            </div>
          </div>

          <HeroArt />
        </div>

        <Lifecycle />
      </div>
    </section>
  );
}

Object.assign(window, { Hero, HeroArt, Lifecycle });
