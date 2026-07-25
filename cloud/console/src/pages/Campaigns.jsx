import React, { useEffect, useMemo, useState } from "react";
import { api, decimalToBaseUnits, fmtDate, idemKey, trunc } from "../api.js";
import { useApp } from "../store.jsx";
import { BtnPrimary, ErrText, Ic, Pill } from "../ui.jsx";

/* ── the six faces of the primitive ─────────────────────────────── */
export const FACES = [
  { kind: "coupon", name: "General coupon", desc: "One shared code, redeemed by many — signed events are anchored by period.", example: "10OFF", icon: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" width="21" height="21" {...p}><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18" /></svg>) },
  { kind: "creator", name: "Creator / referral code", desc: "A shared code with an exact attributed period count and optional token payout.", example: "PEDROPROMO10", icon: Ic.users },
  { kind: "gift", name: "Gift / delivery proof", desc: "Prove a gifted product reached real customers — one code per venue, scan per serving, optional payout per verified event.", example: "COPA-ROSALES", icon: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" width="21" height="21" {...p}><rect x="4" y="10" width="16" height="10" rx="1.6" /><path d="M4 14.2h16M12 10v10" /><path d="M12 10c-4.1 0-4.7-4.5-1.6-4.5C12.4 5.5 12 10 12 10zm0 0c4.1 0 4.7-4.5 1.6-4.5C11.6 5.5 12 10 12 10z" /></svg>) },
  { kind: "voucher", name: "Unique vouchers", desc: "Issued in batches; each code works exactly once, then it's gone.", example: "A7K9-QX2M", icon: Ic.coupon },
  { kind: "ticket", name: "Event tickets", desc: "Unique codes burned at the door — real-time single-use check-in.", example: "TICKET-0042", icon: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" width="21" height="21" {...p}><rect x="4" y="4" width="6" height="6" rx="1.2" /><rect x="14" y="4" width="6" height="6" rx="1.2" /><rect x="4" y="14" width="6" height="6" rx="1.2" /><path d="M14 14h2.6v2.6H14zM17.4 17.4H20V20h-2.6z" /></svg>) },
  { kind: "loyalty", name: "Loyalty program", desc: "Signed punches can be anchored by period; thresholds auto-issue a reward voucher.", example: "10 → 1 free", icon: (p) => (<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" width="21" height="21" {...p}><rect x="3.5" y="6" width="17" height="12" rx="2.5" /><circle cx="8" cy="12" r="1" fill="currentColor" /><circle cx="12" cy="12" r="1" fill="currentColor" /><circle cx="16" cy="12" r="1" strokeDasharray="1.5 1.5" /></svg>) },
];

export const kindTag = (c) => {
  if (c.kind === "voucher") return "unique vouchers";
  if (c.kind === "ticket") return "event tickets";
  if (c.kind === "creator") return `creator code · ${trunc(c.attributed_to)}`;
  if (c.kind === "gift") return c.attributed_to ? `gift proof · ${trunc(c.attributed_to)}` : "gift proof";
  if (c.kind === "loyalty") return "loyalty";
  return "shared code";
};

const isUnique = (c) => c.kind === "voucher" || c.kind === "ticket" || c.kind === "loyalty";

export function statusOf(c) {
  if (c.archived) return ["", "Archived"];
  if (c.valid_until * 1000 < Date.now()) return ["pending", "Expired"];
  return ["valid", "Active"];
}

/* ── index ────────────────────────────────────────────────────────── */
export function CampaignsPage() {
  const { env, nav, route } = useApp();
  const [data, setData] = useState(null);
  const [wizard, setWizard] = useState(route.includes("new=1"));
  const [q, setQ] = useState("");

  const load = () => api.get("/v1/campaigns").then(setData).catch(() => {});
  useEffect(() => { setData(null); load(); }, [env]);

  const list = useMemo(() => {
    if (!data) return [];
    return data.campaigns.filter((c) => !q || c.name.toLowerCase().includes(q.toLowerCase()) || (c.shared_code || "").toLowerCase().includes(q.toLowerCase()));
  }, [data, q]);

  const active = data ? data.campaigns.filter((c) => statusOf(c)[1] === "Active").length : 0;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h1 className="display" style={{ fontSize: 27, margin: 0 }}>Campaigns</h1>
          <p className="page-sub">{data ? `${data.campaigns.length} total · ${active} active` : "…"}{env === "test" ? " · creating is free here" : ""}</p>
        </div>
        <BtnPrimary small icon={<Ic.plus width={13} height={13} />} onClick={() => setWizard(true)}>New campaign</BtnPrimary>
      </div>

      <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
        <input className="input" placeholder="Search campaigns" value={q} onChange={(e) => setQ(e.target.value)}
          style={{ width: 280, fontSize: 13.5, paddingTop: 9, paddingBottom: 9 }} />
        <div style={{ flex: 1 }} />
      </div>

      {data && list.length === 0 && !q
        ? <div className="empty">
            <h2 className="display" style={{ fontSize: 22, margin: 0 }}>No campaigns yet</h2>
            <p style={{ fontSize: 13.5, color: "var(--ink-2)", margin: 0 }}>One primitive, six faces — coupons, creator codes, gift proofs, vouchers, tickets, punch cards.</p>
            <BtnPrimary icon={<Ic.plus width={15} height={15} />} onClick={() => setWizard(true)}>Create your first campaign</BtnPrimary>
          </div>
        : <div className="card" style={{ overflow: "hidden" }}>
            <div style={{ display: "grid", gridTemplateColumns: "2.2fr 1fr 1.6fr .8fr .6fr 40px", gap: 14, padding: "11px 20px", borderBottom: "1px solid var(--line)", background: "var(--surface-inset)" }}>
              {["Campaign", "Status", "Redemptions", "Created", "Env", ""].map((h, i) => <span key={i} className="eyebrow" style={{ fontSize: 11 }}>{h}</span>)}
            </div>
            {list.map((c) => {
              const [pk, pl] = statusOf(c);
              const dim = pl !== "Active";
              return (
                <div key={c.id} onClick={() => nav(`/campaigns/${c.id}`)}
                  style={{ display: "grid", gridTemplateColumns: "2.2fr 1fr 1.6fr .8fr .6fr 40px", gap: 14, padding: "13px 20px", borderBottom: "1px solid var(--line)", alignItems: "center", cursor: "pointer", opacity: dim ? 0.7 : 1 }}>
                  <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                    <span style={{ fontSize: 14, fontWeight: 650 }}>{c.kind === "coupon" || c.kind === "creator" || c.kind === "gift" ? c.shared_code : c.name}</span>
                    <span className="authtag" style={{ alignSelf: "flex-start" }}>{kindTag(c)}</span>
                  </div>
                  <Pill kind={pk || undefined}>{pl}</Pill>
                  {isUnique(c)
                    ? <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
                        <div className="progressbar"><span style={{ width: `${c.minted ? Math.round((c.burned / Math.max(1, c.total_supply)) * 100) : 0}%` }} /></div>
                        <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-2)" }}>{c.burned} / {c.total_supply} {c.kind === "ticket" ? "burned at the door" : "redeemed"}</span>
                      </div>
                    : <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
                        <span className="mono" style={{ fontSize: 13 }}>{(c.events_total || 0).toLocaleString()} <span style={{ color: "var(--ink-3)", fontSize: 11.5 }}>counted</span></span>
                        <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>one code · many uses</span>
                      </div>}
                  <span style={{ fontSize: 13, color: "var(--ink-2)" }}>{fmtDate(c.created_at)}</span>
                  <span className="mono" style={{ fontSize: 11, color: env === "test" ? "var(--pending)" : "var(--ink-3)", fontWeight: env === "test" ? 600 : 400 }}>{env}</span>
                  <Ic.chevron width={15} height={15} style={{ color: "var(--ink-3)", justifySelf: "end", transform: "rotate(-90deg)" }} />
                </div>
              );
            })}
          </div>}

      {wizard && <Wizard onDone={(c) => { setWizard(false); load(); if (c) nav(`/campaigns/${c.id}`); }} />}
    </div>
  );
}

/* ── the 3-step wizard (full-screen) ─────────────────────────────── */
export function Wizard({ onDone, fixedKind }) {
  const { env, toast, toastErr, nav } = useApp();
  const [step, setStep] = useState(fixedKind ? 2 : 1);
  const [kind, setKind] = useState(fixedKind || "creator");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(null);
  const [f, setF] = useState({
    name: "", code: "", attributed_to: "", rate: "0.25",
    discount_type: "percentage", discount_value: "10",
    total_supply: "100", months: "12", threshold: "10",
    reward_type: "free_item", reward_value: "1",
  });
  const face = FACES.find((x) => x.kind === kind);
  const set = (k) => (e) => setF({ ...f, [k]: e.target.value });
  const isShared = kind === "coupon" || kind === "creator" || kind === "gift";

  const cost = kind === "creator" || kind === "loyalty" ? 30 : kind === "coupon" || kind === "gift" ? 30 : 20;

  const create = async () => {
    setBusy(true); setErr(null);
    try {
      let created;
      if (kind === "loyalty") {
        const threshold = Number(f.threshold);
        const rewardValue = Number(f.reward_value);
        if (!Number.isSafeInteger(threshold) || threshold < 1 || threshold > 1_000_000) {
          throw new RangeError("Punch threshold must be an integer between 1 and 1,000,000.");
        }
        if (!Number.isSafeInteger(rewardValue) || rewardValue < 0) {
          throw new RangeError("Reward value must be a non-negative safe integer.");
        }
        const programName = f.name.trim() || "Loyalty program";
        const r = await api.postIdem("/v1/loyalty/programs", {
          name: programName, threshold,
          reward_discount_type: f.reward_type, reward_discount_value: rewardValue,
        }, idemKey());
        toast("Program created", `${programName} · earn anchor ${r.program.earn_code} registered on-chain.`);
        onDone(null); nav("/loyalty"); return;
      }
      const attributes = kind === "creator" || (kind === "gift" && f.attributed_to.trim());
      const payoutRate = attributes && f.rate ? decimalToBaseUnits(f.rate, 7) : "0";
      created = await api.postIdem("/v1/campaigns", {
        kind, name: f.name || (isShared ? f.code : "Campaign"),
        discount_type: f.discount_type, discount_value: Number(f.discount_value) || 0,
        total_supply: isShared ? 0 : Number(f.total_supply) || 100,
        valid_until: Math.floor(Date.now() / 1000) + (Number(f.months) || 12) * 30 * 86400,
        shared: isShared ? {
          code: f.code, attributed_to: attributes ? f.attributed_to.trim() : "",
          payout_rate: payoutRate,
        } : undefined,
      }, idemKey());
      toast("Campaign created", `${created.name} was created on the testnet contract${isShared ? ` · ${created.shared_code} registered` : ""}.`);
      onDone(created);
    } catch (e) { setErr(e); setBusy(false); }
  };

  const stepDot = (n, label) => {
    const done = step > n, on = step === n;
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
        {done
          ? <span style={{ width: 22, height: 22, borderRadius: 999, background: "var(--accent)", color: "var(--on-accent)", display: "grid", placeItems: "center" }}><Ic.check width={12} height={12} /></span>
          : <span className="mono" style={{ width: 22, height: 22, borderRadius: 999, display: "grid", placeItems: "center", fontSize: 11, fontWeight: 600, ...(on ? { background: "var(--ink)", color: "var(--bg)" } : { border: "1px dashed var(--line-2)", color: "var(--ink-3)" }) }}>{n}</span>}
        <span style={{ fontSize: 13, fontWeight: on ? 650 : 600, color: on ? "var(--ink)" : done ? "var(--ink-2)" : "var(--ink-3)" }}>{label}</span>
      </span>
    );
  };

  return (
    <div style={{ position: "fixed", inset: 0, zIndex: 300, background: "var(--bg)", display: "flex", flexDirection: "column", overflowY: "auto" }}>
      <div style={{ height: 58, flex: "none", borderBottom: "1px solid var(--line)", display: "flex", alignItems: "center", gap: 14, padding: "0 22px", position: "sticky", top: 0, background: "var(--bg)", zIndex: 5 }}>
        <div className="side-logo"><Ic.coupon /></div>
        <span style={{ fontSize: 14, fontWeight: 650 }}>New campaign</span>
        <div style={{ flex: 1, display: "flex", justifyContent: "center", gap: 22, alignItems: "center" }}>
          {stepDot(1, "Choose type")}
          <span style={{ width: 26, height: 1, background: "var(--line-2)" }} />
          {stepDot(2, "Configure")}
          <span style={{ width: 26, height: 1, background: "var(--line-2)" }} />
          {stepDot(3, "Review")}
        </div>
        <span className="mono" style={{ fontSize: 11.5, color: env === "test" ? "var(--pending)" : "var(--ink-3)", fontWeight: 600 }}>{env.toUpperCase()}</span>
        <button onClick={() => onDone(null)} style={{ width: 30, height: 30, borderRadius: 999, border: "1px solid var(--line-2)", display: "grid", placeItems: "center", color: "var(--ink-2)" }}><Ic.x width={14} height={14} /></button>
      </div>

      <div style={{ padding: "40px 24px 48px", display: "grid", placeItems: "center", flex: 1 }}>
        {step === 1 && (
          <div style={{ width: 860, maxWidth: "100%", display: "flex", flexDirection: "column", gap: 22 }}>
            <div>
              <h1 className="display" style={{ fontSize: 30, margin: 0 }}>What do you want to run?</h1>
              <p style={{ fontSize: 14, color: "var(--ink-2)", margin: "6px 0 0" }}>One primitive under the hood — pick how it should behave. The kind can't change later.</p>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              {FACES.map((fc) => {
                const on = kind === fc.kind;
                return (
                  <button key={fc.kind} onClick={() => setKind(fc.kind)} style={{
                    display: "flex", alignItems: "center", textAlign: "left", background: "var(--surface)",
                    border: on ? "1px solid var(--ink)" : "1px solid var(--line)", borderRadius: 14,
                    boxShadow: on ? "0 0 0 3px color-mix(in oklch, var(--accent) 45%, transparent), var(--shadow)" : "var(--shadow-sm)",
                    padding: "16px 20px", gap: 18, cursor: "pointer",
                  }}>
                    <div style={{ width: 44, height: 44, flex: "none", borderRadius: 12, background: on ? "var(--accent)" : "var(--surface-inset)", display: "grid", placeItems: "center", color: on ? "var(--on-accent)" : "var(--ink-2)" }}>
                      {fc.icon({ width: 21, height: 21 })}
                    </div>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: 15, fontWeight: 650 }}>{fc.name}</div>
                      <div style={{ fontSize: 13, color: "var(--ink-2)" }}>{fc.desc}</div>
                    </div>
                    <span className="mono" style={{ fontSize: 12.5, background: on ? "var(--accent-wash)" : "var(--surface-inset)", border: `1px dashed ${on ? "var(--accent-2)" : "var(--line-2)"}`, borderRadius: 8, padding: "4px 10px" }}>{fc.example}</span>
                    <span style={{ width: 20, height: 20, borderRadius: 999, flex: "none", ...(on ? { border: "6px solid var(--ink)", background: "var(--accent)" } : { border: "1.5px solid var(--line-2)" }) }} />
                  </button>
                );
              })}
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <span className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)" }}>Step 1 of 3</span>
              <BtnPrimary onClick={() => setStep(2)}>Continue</BtnPrimary>
            </div>
          </div>
        )}

        {step === 2 && (
          <div style={{ width: 980, maxWidth: "100%", display: "flex", gap: 24, alignItems: "flex-start" }}>
            <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 20 }}>
              <div>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <h1 className="display" style={{ fontSize: 30, margin: 0 }}>{face.name}</h1>
                  <span className="authtag">{isShared ? (kind === "creator" ? "shared · attributed" : kind === "gift" ? "shared · delivery proof" : "shared") : kind === "loyalty" ? "earn + reward" : "unique · single-use"}</span>
                </div>
                <p style={{ fontSize: 14, color: "var(--ink-2)", margin: "6px 0 0" }}>{face.desc}</p>
              </div>
              <div className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }}>
                <div className="field">
                  <label>Campaign name <span className="req">*</span></label>
                  <input className="input" value={f.name} onChange={set("name")} placeholder={kind === "loyalty" ? "Cafetero Frecuente" : kind === "ticket" ? "Noche de Jazz" : "Summer promo"} autoFocus />
                </div>
                {isShared && <>
                  <div className="field">
                    <label>The code <span className="req">*</span></label>
                    <input className="input mono" value={f.code} onChange={(e) => setF({ ...f, code: e.target.value.toUpperCase() })} placeholder={face.example} />
                    <span className="help">1–64 UTF-8 bytes, unique in this campaign. This is what customers type at checkout.</span>
                  </div>
                  {kind === "creator" && <>
                    <div className="field">
                      <label>Creator's Stellar address <span className="req">*</span></label>
                      <input className="input mono" value={f.attributed_to} onChange={set("attributed_to")} placeholder="G…" />
                      <span className="help">Attribution is bound to this address. The creator can verify published receipts and committed totals outside this dashboard; the receipt signer still attests that each event occurred.</span>
                    </div>
                    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
                      <div className="field">
                        <label>Payout token</label>
                        <select className="select mono" style={{ fontSize: 13.5 }}><option>XLM · legacy testnet preview</option></select>
                      </div>
                      <div className="field">
                        <label>Rate per redemption</label>
                        <input className="input mono" value={f.rate} onChange={set("rate")} />
                        <span className="help">XLM amount · up to 7 decimal places in this testnet preview.</span>
                      </div>
                    </div>
                    <div style={{ background: "var(--pending-bg)", border: "1px solid color-mix(in oklch, var(--pending) 26%, transparent)", borderRadius: 12, padding: "13px 15px", display: "flex", gap: 11 }}>
                      <Ic.lock width={17} height={17} style={{ flex: "none", marginTop: 1, color: "var(--pending)" }} />
                      <p style={{ fontSize: 13, color: "var(--pending)", lineHeight: 1.5, margin: 0 }}><strong>Payout terms are immutable after creation.</strong> The creator can verify them on-chain before promoting the code — that's the point.</p>
                    </div>
                  </>}
                  {kind === "gift" && <>
                    <div className="field">
                      <label>Venue's Stellar address <span style={{ color: "var(--ink-3)", fontWeight: 500 }}>(optional)</span></label>
                      <input className="input mono" value={f.attributed_to} onChange={set("attributed_to")} placeholder="G…" />
                      <span className="help">The delivery point that received the gift (a restaurant, a shop). Attributing it binds every verified scan to this address; add more venue codes to the campaign after creation.</span>
                    </div>
                    {f.attributed_to.trim() && (
                      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
                        <div className="field">
                          <label>Payout token</label>
                          <select className="select mono" style={{ fontSize: 13.5 }}><option>XLM · testnet preview</option></select>
                        </div>
                        <div className="field">
                          <label>Rate per verified serving</label>
                          <input className="input mono" value={f.rate} onChange={set("rate")} />
                          <span className="help">Optional incentive: the venue gets paid per verified scan at settlement. 0 disables payout.</span>
                        </div>
                      </div>
                    )}
                    <div style={{ background: "var(--pending-bg)", border: "1px solid color-mix(in oklch, var(--pending) 26%, transparent)", borderRadius: 12, padding: "13px 15px", display: "flex", gap: 11 }}>
                      <Ic.lock width={17} height={17} style={{ flex: "none", marginTop: 1, color: "var(--pending)" }} />
                      <p style={{ fontSize: 13, color: "var(--pending)", lineHeight: 1.5, margin: 0 }}><strong>Scans prove attestation, not consumption.</strong> Signed receipts and the anchored tally make the scan set tamper-evident; detecting abuse (one phone scanning everything) is your integration's job — Soroticket gives you the customer commitments to do it.</p>
                    </div>
                  </>}
                </>}
                {kind === "loyalty"
                  ? <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 16 }}>
                      <div className="field">
                        <label>Punches to reward</label>
                        <input className="input mono" value={f.threshold} onChange={set("threshold")} />
                        <span className="help">e.g. 10 → the 10th punch auto-issues the voucher.</span>
                      </div>
                      <div className="field">
                        <label>Reward type</label>
                        <select className="select mono" value={f.reward_type} onChange={set("reward_type")}>
                          <option value="free_item">free_item</option>
                          <option value="percentage">percentage</option>
                          <option value="fixed_amount">fixed_amount</option>
                        </select>
                      </div>
                      <div className="field">
                        <label>Reward value</label>
                        <input className="input mono" value={f.reward_value} onChange={set("reward_value")} />
                        <span className="help">Opaque integer interpreted by your POS.</span>
                      </div>
                    </div>
                  : <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
                      <div className="field">
                        <label>Discount type</label>
                        <select className="select" style={{ fontSize: 13.5 }} value={f.discount_type} onChange={set("discount_type")}>
                          <option value="percentage">percentage</option>
                          <option value="fixed_amount">fixed amount</option>
                          <option value="free_item">free item</option>
                        </select>
                      </div>
                      <div className="field">
                        <label>{f.discount_type === "percentage" ? "Percent off" : "Value"}</label>
                        <input className="input mono" value={f.discount_value} onChange={set("discount_value")} />
                      </div>
                    </div>}
                {!isShared && kind !== "loyalty" && (
                  <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
                    <div className="field">
                      <label>{kind === "ticket" ? "Seats / supply" : "Total supply"}</label>
                      <input className="input mono" value={f.total_supply} onChange={set("total_supply")} />
                      <span className="help">Maximum codes this campaign can ever issue.</span>
                    </div>
                    <div className="field">
                      <label>Valid for</label>
                      <select className="select" style={{ fontSize: 13.5 }} value={f.months} onChange={set("months")}>
                        <option value="1">1 month</option><option value="3">3 months</option>
                        <option value="6">6 months</option><option value="12">12 months</option>
                      </select>
                    </div>
                  </div>
                )}
              </div>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                {fixedKind ? <span /> : <button className="btn-plain" onClick={() => setStep(1)}>← Back</button>}
                <BtnPrimary disabled={!f.name && !(isShared && f.code)} onClick={() => setStep(3)}>Continue</BtnPrimary>
              </div>
            </div>
            <div style={{ width: 300, flex: "none", display: "flex", flexDirection: "column", gap: 14 }}>
              <div className="card" style={{ padding: "18px 20px" }}>
                <div className="eyebrow" style={{ fontSize: 11, marginBottom: 12 }}>What this costs</div>
                {[["Create campaign", "20 cr"],
                  ...(isShared ? [["Register shared code", "10 cr"], ["Recorded event", "0.2 cr"], ["Commit tally", "15 cr / period"]] : []),
                  ...(kind === "creator" || kind === "gift" ? [["Settle period", "25 cr"]] : []),
                  ...(kind === "loyalty" ? [["Register earn anchor", "10 cr"], ["Punch", "0.2 cr"], ["Reward voucher", "2 cr"]] : []),
                  ...(!isShared && kind !== "loyalty" ? [["Issue unique code", "2 cr / code"], ["Redeem", "5 cr"]] : []),
                ].map(([k, v], i) => (
                  <div key={i} style={{ display: "flex", justifyContent: "space-between", padding: "7px 0", borderTop: "1px solid var(--line)", fontSize: 12.5 }}>
                    <span style={{ color: "var(--ink-2)" }}>{k}</span><span className="mono">{v}</span>
                  </div>
                ))}
                <div style={{ display: "flex", justifyContent: "space-between", padding: "9px 0 2px", borderTop: "1px dashed var(--line-2)", fontSize: 13, fontWeight: 650 }}>
                  <span>Due today</span><span className="mono">{env === "test" ? "free (test)" : `${cost} cr`}</span>
                </div>
              </div>
              <p className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", lineHeight: 1.6, margin: 0, padding: "0 4px" }}>Preview credits only — no payment processor is enabled. Test mode is always unmetered.</p>
            </div>
          </div>
        )}

        {step === 3 && (
          <div style={{ width: 620, maxWidth: "100%", display: "flex", flexDirection: "column", gap: 20 }}>
            <div className="ticket" style={{ borderRadius: 18 }}>
              <div className="ticket-perf" />
              <div style={{ padding: "30px 28px 20px", display: "flex", flexDirection: "column", gap: 14 }}>
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <span className="eyebrow" style={{ fontSize: 11 }}>Ready to create</span>
                  <span className="authtag">{face.name.toLowerCase()}</span>
                </div>
                <div>
                  <div className="mono" style={{ fontSize: 32, fontWeight: 600, letterSpacing: ".01em" }}>{isShared ? (f.code || "—") : (f.name || "—")}</div>
                  <div style={{ fontSize: 13.5, color: "var(--ink-2)", marginTop: 2 }}>{isShared ? f.name : kind === "loyalty" ? `${f.threshold} punches → ${f.reward_type} ${f.reward_value}` : `${f.discount_value}${f.discount_type === "percentage" ? "%" : ""} · supply ${f.total_supply}`}</div>
                </div>
                <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "10px 20px", fontSize: 13 }}>
                  {kind === "creator" && <>
                    <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 8 }}><span style={{ color: "var(--ink-3)" }}>Creator</span><span className="mono" style={{ fontSize: 12 }}>{trunc(f.attributed_to)}</span></div>
                    <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 8 }}><span style={{ color: "var(--ink-3)" }}>Payout</span><span className="mono" style={{ fontSize: 12 }}>{f.rate} XLM / redemption</span></div>
                  </>}
                  {kind === "gift" && f.attributed_to.trim() && <>
                    <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 8 }}><span style={{ color: "var(--ink-3)" }}>Venue</span><span className="mono" style={{ fontSize: 12 }}>{trunc(f.attributed_to)}</span></div>
                    <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 8 }}><span style={{ color: "var(--ink-3)" }}>Payout</span><span className="mono" style={{ fontSize: 12 }}>{Number(f.rate) > 0 ? `${f.rate} XLM / serving` : "none"}</span></div>
                  </>}
                  <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 8 }}><span style={{ color: "var(--ink-3)" }}>Environment</span><span className="mono" style={{ fontSize: 12 }}>{env}</span></div>
                  <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px solid var(--line)", paddingTop: 8 }}><span style={{ color: "var(--ink-3)" }}>Valid for</span><span>{f.months} months</span></div>
                </div>
              </div>
              <div className="ticket-tear"><i className="l" style={{ background: "var(--bg)" }} /><i className="r" style={{ background: "var(--bg)" }} /></div>
              <div style={{ padding: "18px 28px 24px", display: "flex", flexDirection: "column", gap: 9 }}>
                <div className="eyebrow" style={{ fontSize: 11 }}>When you press create</div>
                <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 13, color: "var(--ink-2)" }}>
                  <span className="mono" style={{ width: 20, height: 20, flex: "none", borderRadius: 999, background: "var(--surface-inset)", border: "1px solid var(--line)", display: "grid", placeItems: "center", fontSize: 10.5 }}>1</span>
                  <span style={{ flex: 1 }}>Campaign is written on-chain, owned by your org account</span>
                  <span className="mono" style={{ fontSize: 12 }}>{env === "test" ? "free" : "20 cr"}</span>
                </div>
                {(isShared || kind === "loyalty") && (
                  <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 13, color: "var(--ink-2)" }}>
                    <span className="mono" style={{ width: 20, height: 20, flex: "none", borderRadius: 999, background: "var(--surface-inset)", border: "1px solid var(--line)", display: "grid", placeItems: "center", fontSize: 10.5 }}>2</span>
                    <span style={{ flex: 1 }}>{kind === "loyalty" ? "The earn code is registered — signed punches can be committed manually by period" : <><span className="mono" style={{ fontSize: 12 }}>{f.code}</span> is registered{kind === "creator" ? " with its payout terms — locked" : kind === "gift" && f.attributed_to.trim() ? " — attributed to the venue" : ""}</>}</span>
                    <span className="mono" style={{ fontSize: 12 }}>{env === "test" ? "free" : "10 cr"}</span>
                  </div>
                )}
                <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 13, color: "var(--ink-2)" }}>
                  <span className="mono" style={{ width: 20, height: 20, flex: "none", borderRadius: 999, background: "var(--surface-inset)", border: "1px solid var(--line)", display: "grid", placeItems: "center", fontSize: 10.5 }}>3</span>
                  <span style={{ flex: 1 }}>Anyone can verify everything on <span className="mono" style={{ fontSize: 12 }}>stellar.expert ↗</span></span>
                  <span className="mono" style={{ fontSize: 12 }}>—</span>
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", borderTop: "1px dashed var(--line-2)", paddingTop: 10, marginTop: 4, fontSize: 13.5, fontWeight: 650 }}>
                  <span>Total</span><span className="mono">{env === "test" ? "free" : `${cost} cr`}</span>
                </div>
              </div>
            </div>
            <ErrText err={err} />
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <button className="btn-plain" onClick={() => setStep(2)}>← Back</button>
              <BtnPrimary busy={busy} icon={<Ic.check width={15} height={15} />} onClick={create}>Create campaign</BtnPrimary>
            </div>
            <p className="mono" style={{ fontSize: 11.5, color: "var(--ink-3)", margin: 0, textAlign: "center" }}>You can archive a campaign anytime. Automated TTL keep-alive is not enabled yet.</p>
          </div>
        )}
      </div>
    </div>
  );
}
