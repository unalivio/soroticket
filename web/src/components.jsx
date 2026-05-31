/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — shared primitives + icons  → window
   ═══════════════════════════════════════════════════════════════════ */
const { useState, useEffect, useRef, useCallback, createContext, useContext } = React;

/* ── Icons (simple geometric strokes) ───────────────────────── */
const Ic = {
  copy: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><rect x="9" y="9" width="11" height="11" rx="2.5"/><path d="M5 15V5.5A1.5 1.5 0 0 1 6.5 4H15"/></svg>),
  check: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" {...p}><polyline points="4 12.5 9.5 18 20 6"/></svg>),
  arrow: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" {...p}><line x1="5" y1="12" x2="19" y2="12"/><polyline points="13 6 19 12 13 18"/></svg>),
  arrowUR: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" {...p}><line x1="7" y1="17" x2="17" y2="7"/><polyline points="8 7 17 7 17 16"/></svg>),
  ext: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" {...p}><path d="M14 5h5v5"/><path d="M19 5l-8 8"/><path d="M18 13.5V18a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 6 18V9a1.5 1.5 0 0 1 1.5-1.5H12"/></svg>),
  wallet: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><rect x="3" y="6" width="18" height="13" rx="2.5"/><path d="M3 9h18"/><circle cx="17" cy="13.5" r="1.3" fill="currentColor" stroke="none"/></svg>),
  lock: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><rect x="5" y="11" width="14" height="9" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>),
  globe: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></svg>),
  shield: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><path d="M12 3l7 3v5c0 4.5-3 8-7 10-4-2-7-5.5-7-10V6z"/></svg>),
  users: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0"/><path d="M16 6.2a3 3 0 0 1 0 5.6M17.5 19a5.5 5.5 0 0 0-3-4.9"/></svg>),
  spark: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><path d="M12 3v4M12 17v4M3 12h4M17 12h4M5.6 5.6l2.8 2.8M15.6 15.6l2.8 2.8M18.4 5.6l-2.8 2.8M8.4 15.6l-2.8 2.8"/></svg>),
  chevron: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" {...p}><polyline points="6 9 12 15 18 9"/></svg>),
  x: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" {...p}><line x1="6" y1="6" x2="18" y2="18"/><line x1="18" y1="6" x2="6" y2="18"/></svg>),
  plus: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.1" strokeLinecap="round" strokeLinejoin="round" {...p}><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>),
  terminal: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><rect x="3" y="4" width="18" height="16" rx="2.5"/><polyline points="7 9 10 12 7 15"/><line x1="12.5" y1="15" x2="16" y2="15"/></svg>),
  alert: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" {...p}><circle cx="12" cy="12" r="9"/><line x1="12" y1="8" x2="12" y2="13"/><circle cx="12" cy="16" r="0.6" fill="currentColor" stroke="currentColor"/></svg>),
  ok: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" {...p}><circle cx="12" cy="12" r="9"/><polyline points="8 12.5 11 15.5 16 9"/></svg>),
  fire: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" {...p}><path d="M12 3c1 3-1.5 4-1.5 6.5A2.5 2.5 0 0 0 13 12c.5-.8.4-1.6.2-2.2 1.8 1 3.3 3 3.3 5.4a4.5 4.5 0 1 1-9 0C7.5 11 11 9 12 3z"/></svg>),
  info: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" {...p}><circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16"/><circle cx="12" cy="8" r="0.6" fill="currentColor" stroke="currentColor"/></svg>),
  hash: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" {...p}><line x1="9" y1="4" x2="7" y2="20"/><line x1="17" y1="4" x2="15" y2="20"/><line x1="4" y1="9" x2="20" y2="9"/><line x1="3.5" y1="15" x2="19.5" y2="15"/></svg>),
  spinner: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" {...p}><path d="M12 3a9 9 0 1 0 9 9" opacity="1"/><path d="M12 3a9 9 0 0 1 9 9" opacity="0.25"/></svg>),
};

/* ── clipboard ──────────────────────────────────────────────── */
function useCopy() {
  const [copied, setCopied] = useState(false);
  const tRef = useRef(null);
  const copy = useCallback((text) => {
    const t = String(text);
    const done = () => { setCopied(true); clearTimeout(tRef.current); tRef.current = setTimeout(() => setCopied(false), 1300); };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(t).then(done).catch(() => fallback(t, done));
    } else fallback(t, done);
  }, []);
  return [copied, copy];
}
function fallback(text, done) {
  const ta = document.createElement("textarea");
  ta.value = text; ta.style.position = "fixed"; ta.style.opacity = "0";
  document.body.appendChild(ta); ta.select();
  try { document.execCommand("copy"); done(); } catch (e) {}
  document.body.removeChild(ta);
}

/* CopyValue — boxed mono value w/ copy + optional truncation + explorer link */
function CopyValue({ value, display, label, link, mono = true, className = "" }) {
  const [copied, copy] = useCopy();
  return (
    <span className={"copy " + (copied ? "copied " : "") + className} onClick={(e) => { e.stopPropagation(); copy(value); }}
          title={label ? label + ": " + value : value} role="button">
      <span className={"val" + (mono ? " mono" : "")}>{display || value}</span>
      {link && (
        <a href={link} target="_blank" rel="noopener" className="ci" onClick={(e) => e.stopPropagation()} title="Open in stellar.expert">
          <Ic.ext />
        </a>
      )}
      <span className="ci">{copied ? <Ic.check /> : <Ic.copy />}</span>
    </span>
  );
}

/* CopyBare — inline mono text with subtle copy affordance */
function CopyBare({ value, display, mono = true }) {
  const [copied, copy] = useCopy();
  return (
    <span className={"copy-bare " + (copied ? "copied" : "")} onClick={(e) => { e.stopPropagation(); copy(value); }}
          title={"Copy: " + value} role="button" style={mono ? { fontFamily: "var(--mono)" } : {}}>
      {display || value}
      <span className="ci">{copied ? <Ic.check /> : <Ic.copy />}</span>
    </span>
  );
}

/* ── Pill ───────────────────────────────────────────────────── */
function Pill({ kind, children }) {
  return <span className={"pill pill-" + kind}><span className="dot" />{children}</span>;
}

/* ── Auth tag (owner / owner or delegate / public) ──────────── */
function AuthTag({ auth }) {
  const map = {
    owner: { icon: Ic.lock, text: "owner" },
    delegate: { icon: Ic.users, text: "owner or delegate" },
    public: { icon: Ic.globe, text: "public" },
  };
  const m = map[auth] || map.public;
  const I = m.icon;
  return <span className="authtag"><I />{m.text}</span>;
}

/* ── Spinner ────────────────────────────────────────────────── */
function Spinner({ size = 16 }) {
  return <Ic.spinner className="spin" style={{ width: size, height: size }} />;
}

/* ── Button with chip arrow ─────────────────────────────────── */
function Btn({ variant = "primary", loading, disabled, children, icon, small, ...rest }) {
  const cls = `btn btn-${variant}` + (small ? " btn-sm" : "");
  return (
    <button className={cls} disabled={disabled || loading} {...rest}>
      <span>{children}</span>
      <span className="chip">{loading ? <Spinner size={small ? 13 : 15} /> : (icon || <Ic.arrow style={{ width: small ? 13 : 15, height: small ? 13 : 15 }} />)}</span>
    </button>
  );
}

/* ═══════════════════════════════════════════════════════════
   CodeBlock — CLI / TypeScript / Go sub-tabs, highlighted + copy
   ═══════════════════════════════════════════════════════════ */
const LANGS = [
  { id: "cli", label: "stellar CLI", lang: "bash" },
  { id: "ts", label: "TypeScript", lang: "ts" },
  { id: "go", label: "Go", lang: "go" },
];

const KEYWORDS = new Set([
  "import","from","const","let","var","await","async","new","return","if","else","function",
  "err","nil","client","ctx","log","range","map","Buffer","crypto","require","type",
]);

function highlight(code, lang) {
  // tokenize into spans; order matters: comments, strings, numbers, keywords
  const out = [];
  const lines = code.split("\n");
  lines.forEach((line, li) => {
    let i = 0;
    const pushText = (txt) => out.push(txt);
    // comment detection (whole-rest-of-line)
    const commentIdx = (() => {
      // find # or // not inside a string — simple heuristic
      const hashes = [];
      let inStr = null;
      for (let k = 0; k < line.length; k++) {
        const ch = line[k];
        if (inStr) { if (ch === inStr) inStr = null; continue; }
        if (ch === '"' || ch === "'" || ch === "`") inStr = ch;
        else if (ch === "#") return k;
        else if (ch === "/" && line[k + 1] === "/") return k;
      }
      return -1;
    })();

    const codePart = commentIdx >= 0 ? line.slice(0, commentIdx) : line;
    const commentPart = commentIdx >= 0 ? line.slice(commentIdx) : "";

    // regex split codePart into tokens: strings, numbers, words, other
    const re = /("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`)|(\b\d[\d_]*n?\b)|([A-Za-z_$][\w$]*)|(\s+)|([^\sA-Za-z_$"'`0-9])/g;
    let m;
    const frag = [];
    let key = 0;
    while ((m = re.exec(codePart)) !== null) {
      if (m[1]) frag.push(<span key={li + "-" + key++} className="hl-str">{m[1]}</span>);
      else if (m[2]) frag.push(<span key={li + "-" + key++} className="hl-num">{m[2]}</span>);
      else if (m[3]) {
        if (KEYWORDS.has(m[3])) frag.push(<span key={li + "-" + key++} className="hl-kw">{m[3]}</span>);
        else if (/^[A-Z]/.test(m[3])) frag.push(<span key={li + "-" + key++} className="hl-type">{m[3]}</span>);
        else frag.push(m[3]);
      }
      else frag.push(m[0]);
    }
    if (commentPart) frag.push(<span key={li + "-c"} className="hl-com">{commentPart}</span>);
    out.push(<div key={li} className="cl">{frag.length ? frag : "\u00A0"}</div>);
  });
  return out;
}

function CodeBlock({ snippets }) {
  const [lang, setLang] = useState("ts");
  const [copied, copy] = useCopy();
  const code = (snippets && snippets[lang]) || "";
  return (
    <div className="codeblock">
      <div className="cb-head">
        <div className="cb-tabs">
          {LANGS.map((l) => (
            <button key={l.id} className={"cb-tab" + (lang === l.id ? " on" : "")} onClick={() => setLang(l.id)}>
              {l.label}
            </button>
          ))}
        </div>
        <button className={"cb-copy" + (copied ? " copied" : "")} onClick={() => copy(code)}>
          {copied ? <Ic.check /> : <Ic.copy />}<span>{copied ? "Copied" : "Copy"}</span>
        </button>
      </div>
      <pre className="cb-body mono"><code>{highlight(code, lang)}</code></pre>
    </div>
  );
}

/* ── Run / Code tab switch (used by every action card) ──────── */
function RunCodeTabs({ tab, setTab }) {
  return (
    <div className="rc-tabs" role="tablist">
      <button role="tab" className={"rc-tab" + (tab === "run" ? " on" : "")} onClick={() => setTab("run")}>
        <Ic.spark style={{ width: 14, height: 14 }} />Run
      </button>
      <button role="tab" className={"rc-tab" + (tab === "code" ? " on" : "")} onClick={() => setTab("code")}>
        <Ic.terminal style={{ width: 14, height: 14 }} />Code
      </button>
    </div>
  );
}

Object.assign(window, {
  Ic, useCopy, CopyValue, CopyBare, Pill, AuthTag, Spinner, Btn, CodeBlock, RunCodeTabs, highlight,
});
