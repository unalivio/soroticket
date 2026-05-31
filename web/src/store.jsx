/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — app store (REAL contract on Stellar testnet)
   Replaces the simulated store: every write is signed with Freighter and
   submitted to the configured testnet contract; reads are RPC simulations.
   Campaigns are loaded from the contract's owner index. A small in-memory
   token mirror, keyed by campaign_id:code, powers recent UI stats.
   Context shape + method signatures are unchanged, so the design's
   components work untouched.
   ═══════════════════════════════════════════════════════════════════ */
const AppCtx = React.createContext(null);
const useApp = () => useContext(AppCtx);

const SD = window.SD;
const sdk = () => window.SDK;

// reverse map: contract error code (#N) → SD.ERRORS key
const CODE2KEY = Object.fromEntries(
  Object.entries(SD.ERRORS).map(([k, v]) => [v.code, k])
);

/* friendly error with a `.sd` payload (cards read err.sd.title/msg/code) */
const SDErr = (key) => {
  const e = SD.ERRORS[key];
  const err = new Error(e.msg);
  err.sd = e; err.key = key;
  return err;
};
function failErr(title, msg, code = "—") {
  const err = new Error(msg);
  err.sd = { title, msg, code };
  return err;
}
/* normalize any thrown thing into an Error carrying `.sd` */
function toFriendly(raw) {
  if (raw && raw.sd) return raw;                       // already friendly
  if (raw && raw.contractCode != null && CODE2KEY[raw.contractCode]) return SDErr(CODE2KEY[raw.contractCode]);
  const s = String((raw && raw.message) || raw || "");
  const m = /Error\(Contract,\s*#(\d+)\)/.exec(s);
  if (m && CODE2KEY[Number(m[1])]) return SDErr(CODE2KEY[Number(m[1])]);
  return failErr("Transaction failed", s.slice(0, 180) || "Unknown error");
}

function AppProvider({ children }) {
  const [wallet, setWallet] = useState(null);          // { address, balance }
  const [connecting, setConnecting] = useState(false);
  const seedRef = useRef(null);
  if (!seedRef.current) seedRef.current = SD.seedState();
  const [state, setState] = useState(seedRef.current);
  const stateRef = useRef(seedRef.current);            // always-fresh mirror for sync reads
  const [log, setLog] = useState([]);
  const [toasts, setToasts] = useState([]);
  const [highlight, setHighlight] = useState(null);

  const applyState = useCallback((updater) => {
    setState((prev) => { const next = updater(prev); stateRef.current = next; return next; });
  }, []);

  /* ── toasts ───────────────────────────────────────────────── */
  const toast = useCallback((t) => {
    const id = Math.random().toString(36).slice(2);
    setToasts((ts) => [...ts, { id, ...t }]);
    setTimeout(() => setToasts((ts) => ts.filter((x) => x.id !== id)), t.duration || 4600);
  }, []);
  const dismissToast = (id) => setToasts((ts) => ts.filter((x) => x.id !== id));

  /* ── activity log ─────────────────────────────────────────── */
  const pushLog = useCallback((entry) => {
    setLog((l) => [{ id: Math.random().toString(36).slice(2), at: Date.now(), ...entry }, ...l].slice(0, 60));
  }, []);
  const clearLog = () => setLog([]);

  /* run a real invocation: log pending → ok/err; returns {result, tx, seq} */
  const invoke = useCallback(async (method, auth, args, fn, { read = false } = {}) => {
    const logId = Math.random().toString(36).slice(2);
    setLog((l) => [{ id: logId, at: Date.now(), method, auth, args, status: "pending", read, tx: null }, ...l].slice(0, 60));
    try {
      const out = await fn();                          // { result, tx?, seq? }
      const tx = out.tx ?? null, seq = out.seq ?? null;
      setLog((l) => l.map((e) => e.id === logId ? { ...e, status: "ok", result: out.result, tx, seq } : e));
      return { result: out.result, tx, seq };
    } catch (raw) {
      const err = toFriendly(raw);
      setLog((l) => l.map((e) => e.id === logId ? { ...e, status: "err", error: err.sd, tx: null } : e));
      throw err;
    }
  }, []);

  /* ── load the connected wallet's campaigns FROM THE CHAIN (campaigns_of) ──
     The contract's owner index is the source of truth — no localStorage. */
  const loadOwnedCampaigns = useCallback(async (address) => {
    try {
      const ids = await sdk().campaignsOf(address);
      const fetched = await Promise.all(ids.map((id) => sdk().getCampaign(id).catch(() => null)));
      applyState((st) => {
        const campaigns = { ...st.campaigns };
        let campCtr = st.campCtr;
        fetched.filter(Boolean).forEach((c) => { campaigns[c.id] = { ...campaigns[c.id], ...c }; campCtr = Math.max(campCtr, c.id); });
        return { ...st, campaigns, campCtr };
      });
    } catch (e) { /* non-fatal — you can still create campaigns */ }
  }, [applyState]);

  /* ── wallet ───────────────────────────────────────────────── */
  const connect = useCallback(async () => {
    setConnecting(true);
    try {
      const { address, balance } = await sdk().connectWallet();
      setWallet({ address, balance });
      toast({ kind: "success", title: "Freighter connected", msg: "Signed in as " + SD.trunc(address, 4, 4) + " on Testnet." });
      loadOwnedCampaigns(address); // hydrate your campaigns from the chain
    } catch (raw) {
      const err = toFriendly(raw);
      toast({ kind: "error", title: err.sd.title, msg: err.sd.msg });
    } finally {
      setConnecting(false);
    }
  }, [toast, loadOwnedCampaigns]);
  const disconnect = useCallback(() => {
    setWallet(null);
    toast({ kind: "info", title: "Wallet disconnected", msg: "You can still use public read calls like verify()." });
  }, [toast]);

  const requireWallet = () => {
    if (!wallet) throw failErr("Connect a wallet", "Connect Freighter to sign this transaction.", 6);
  };

  /* ── CREATE CAMPAIGN (owner) ──────────────────────────────── */
  const createCampaign = useCallback(async (f) => {
    return invoke("create_campaign", "owner",
      { owner: wallet?.address, name: f.name, discount_type: f.discount_type, discount_value: Number(f.discount_value), total_supply: Number(f.total_supply), valid_until: SD.unix(f.valid_until) },
      async () => {
        requireWallet();
        const s = sdk();
        const validUntil = SD.unix(f.valid_until);
        const { retval, tx, seq } = await s.write(wallet.address, "create_campaign", [
          s.scAddr(wallet.address),
          s.scStr(f.name),
          s.scStr(f.discount_type),
          s.scU64(Number(f.discount_value)),
          s.scU32(Number(f.total_supply)),
          s.scU64(validUntil),
        ]);
        const id = Number(s.toNative(retval));
        applyState((st) => ({
          ...st,
          campCtr: Math.max(st.campCtr, id),
          campaigns: { ...st.campaigns, [id]: { id, owner: wallet.address, name: f.name, discount_type: f.discount_type, discount_value: Number(f.discount_value), total_supply: Number(f.total_supply), minted: 0, burned: 0, valid_until: validUntil } },
        }));
        setHighlight(id);
        return { result: { campaign_id: id }, tx, seq };
      });
  }, [wallet, invoke, applyState]);

  /* ── ISSUE CODES (owner only) ─────────────────────────────── */
  const issueCodes = useCallback(async (campaignId, codes) => {
    return invoke("issue_unique", "owner",
      { owner: wallet?.address, campaign_id: Number(campaignId), codes },
      async () => {
        requireWallet();
        const s = sdk();
        const cid = Number(campaignId);
        const { retval, tx, seq } = await s.write(wallet.address, "issue_unique", [
          s.scAddr(wallet.address),
          s.scU64(cid),
          s.scVecStr(codes),
        ]);
        const ids = (s.toNative(retval) || []).map(Number);
        applyState((st) => {
          const now = SD.nowUnix();
          const tokens = { ...st.tokens };
          codes.forEach((code, i) => {
            tokens[cid + ":" + code] = { token_id: ids[i], campaign_id: cid, code, is_burned: false, minted_at: now, redeemer_ref: SD.ZERO32, burned_at: 0 };
          });
          const camp = st.campaigns[cid];
          const campaigns = camp ? { ...st.campaigns, [cid]: { ...camp, minted: camp.minted + codes.length } } : st.campaigns;
          return { ...st, tokenCtr: Math.max(st.tokenCtr, ...ids, st.tokenCtr), tokens, campaigns };
        });
        setHighlight(cid);
        return { result: { token_ids: ids }, tx, seq };
      });
  }, [wallet, invoke, applyState]);

  /* ── REDEEM (owner or delegate) ───────────────────────────── */
  const redeem = useCallback(async (campaignId, code, hash) => {
    // hash is precomputed by the card (SHA-256 of a random nonce ∥ ref) — ADR-010
    return invoke("redeem_unique", "delegate",
      { authorizer: wallet?.address, campaign_id: Number(campaignId), code, redeemer_ref_hash: hash },
      async () => {
        requireWallet();
        const s = sdk();
        const { retval, tx, seq } = await s.write(wallet.address, "redeem_unique", [
          s.scAddr(wallet.address),
          s.scU64(Number(campaignId)),
          s.scStr(code),
          s.scBytes32(hash),
        ]);
        const r = s.toNative(retval) || {};
        const receipt = {
          token_id: Number(r.token_id ?? 0),
          code: r.code || code,
          campaign_id: Number(r.campaign_id ?? campaignId),
          campaign_name: r.campaign_name || (state.campaigns[Number(r.campaign_id ?? campaignId)] || {}).name || "",
          discount_type: r.discount_type || "",
          discount_value: Number(r.discount_value ?? 0),
          redeemer_ref: s.bytesToHex(r.redeemer_ref) || hash,
          burned_at: Number(r.burned_at ?? SD.nowUnix()),
          ledger_seq: Number(r.ledger_seq ?? seq ?? 0),
        };
        applyState((st) => {
          const tk = Number(campaignId) + ":" + code;
          const tok = st.tokens[tk];
          const tokens = tok ? { ...st.tokens, [tk]: { ...tok, is_burned: true, redeemer_ref: receipt.redeemer_ref, burned_at: receipt.burned_at } } : st.tokens;
          const camp = st.campaigns[receipt.campaign_id];
          const campaigns = camp ? { ...st.campaigns, [receipt.campaign_id]: { ...camp, burned: camp.burned + 1 } } : st.campaigns;
          return { ...st, tokens, campaigns };
        });
        setHighlight(receipt.campaign_id);
        return { result: receipt, tx, seq: receipt.ledger_seq };
      });
  }, [wallet, invoke, applyState, state]);

  /* ── VERIFY (public, read) ────────────────────────────────── */
  const verify = useCallback(async (campaignId, code) => {
    return invoke("verify", "public", { campaign_id: Number(campaignId), code },
      async () => {
        const s = sdk();
        const tn = s.toNative(await s.read("verify", [s.scU64(Number(campaignId)), s.scStr(code)])); // throws CouponNotFound (#2)
        const token = {
          token_id: Number(tn.token_id), campaign_id: Number(tn.campaign_id), code: tn.code,
          is_burned: !!tn.is_burned, minted_at: Number(tn.minted_at),
          redeemer_ref: s.bytesToHex(tn.redeemer_ref), burned_at: Number(tn.burned_at),
        };
        let campaign = null;
        try {
          const c = s.toNative(await s.read("get_campaign", [s.scU64(token.campaign_id)]));
          campaign = { id: Number(c.id), owner: c.owner, name: c.name, discount_type: c.discount_type, discount_value: Number(c.discount_value), total_supply: Number(c.total_supply), minted: Number(c.minted), burned: Number(c.burned), valid_until: Number(c.valid_until) };
        } catch (_) {}
        const expired = campaign && SD.nowUnix() > campaign.valid_until;
        const status = token.is_burned ? "burned" : (expired ? "expired" : "valid");
        applyState((st) => ({
          ...st,
          tokens: { ...st.tokens, [Number(campaignId) + ":" + code]: token },
          campaigns: campaign ? { ...st.campaigns, [campaign.id]: { ...st.campaigns[campaign.id], ...campaign } } : st.campaigns,
        }));
        return { result: { token, campaign, status } };
      }, { read: true });
  }, [invoke, applyState]);

  /* synchronous stats from the (always-fresh) local mirror */
  const campaignStats = useCallback((cid) => {
    const c = stateRef.current.campaigns[cid];
    if (!c) return null;
    const expired = SD.nowUnix() > c.valid_until;
    return {
      total_supply: c.total_supply, minted: c.minted, burned: c.burned,
      available: Math.max(0, c.minted - c.burned),
      expired: expired ? Math.max(0, c.total_supply - c.minted) : 0,
      is_expired: expired,
    };
  }, []);

  /* ── DELEGATES (owner) ────────────────────────────────────── */
  const addDelegate = useCallback(async (campaignId, delegate) => {
    return invoke("add_delegate", "owner", { owner: wallet?.address, campaign_id: Number(campaignId), delegate },
      async () => {
        requireWallet();
        const s = sdk();
        const { tx, seq } = await s.write(wallet.address, "add_delegate", [
          s.scAddr(wallet.address), s.scU64(Number(campaignId)), s.scAddr(delegate),
        ]);
        applyState((st) => ({ ...st, delegates: { ...st.delegates, [campaignId + ":" + delegate]: true } }));
        return { result: { ok: true }, tx, seq };
      });
  }, [wallet, invoke, applyState]);

  const removeDelegate = useCallback(async (campaignId, delegate) => {
    return invoke("remove_delegate", "owner", { owner: wallet?.address, campaign_id: Number(campaignId), delegate },
      async () => {
        requireWallet();
        const s = sdk();
        const { tx, seq } = await s.write(wallet.address, "remove_delegate", [
          s.scAddr(wallet.address), s.scU64(Number(campaignId)), s.scAddr(delegate),
        ]);
        applyState((st) => { const d = { ...st.delegates }; delete d[campaignId + ":" + delegate]; return { ...st, delegates: d }; });
        return { result: { ok: true }, tx, seq };
      });
  }, [wallet, invoke, applyState]);

  const isDelegate = useCallback(async (campaignId, who) => {
    return invoke("is_delegate", "public", { campaign_id: Number(campaignId), who },
      async () => {
        const s = sdk();
        const v = s.toNative(await s.read("is_delegate", [s.scU64(Number(campaignId)), s.scAddr(who)]));
        return { result: { is_delegate: !!v } };
      }, { read: true });
  }, [invoke]);

  /* ── TALLY (shared codes) — ADR-003/004/011 ───────────────── */
  const registerShared = useCallback(async (campaignId, code, attributedTo, payoutToken, payoutRate) => {
    const rate = String(payoutRate ?? 0); // i128 — keep as string, never coerce to Number
    return invoke("register_shared", "owner",
      { owner: wallet?.address, campaign_id: Number(campaignId), code, attributed_to: attributedTo || null, payout_token: payoutToken || null, payout_rate: rate },
      async () => {
        requireWallet();
        const { tx, seq } = await sdk().registerShared(wallet.address, Number(campaignId), code, attributedTo || null, payoutToken || null, rate);
        return { result: { ok: true }, tx, seq };
      });
  }, [wallet, invoke]);

  const getShared = useCallback(async (campaignId, code) => {
    return invoke("get_shared", "public", { campaign_id: Number(campaignId), code },
      async () => {
        const sh = await sdk().getShared(Number(campaignId), code);
        return { result: { attributed_to: sh.attributed_to ? String(sh.attributed_to) : null, payout_token: sh.payout_token ? String(sh.payout_token) : null, payout_rate: String(sh.payout_rate) } };
      }, { read: true });
  }, [invoke]);

  const commitTally = useCallback(async (campaignId, code, period, count, merkleHex, perAttribution) => {
    return invoke("commit_tally", "owner",
      { owner: wallet?.address, campaign_id: Number(campaignId), code, period: Number(period), count: Number(count) },
      async () => {
        requireWallet();
        const { tx, seq } = await sdk().commitTally(wallet.address, Number(campaignId), code, Number(period), Number(count), merkleHex, perAttribution);
        return { result: { ok: true }, tx, seq };
      });
  }, [wallet, invoke]);

  const getTally = useCallback(async (campaignId, code, period) => {
    return invoke("get_tally", "public", { campaign_id: Number(campaignId), code, period: Number(period) },
      async () => {
        const s = sdk();
        const t = await s.getTally(Number(campaignId), code, Number(period));
        const perAttr = [];
        try { for (const [k, v] of t.per_attribution.entries()) perAttr.push([String(k), Number(v)]); } catch (_) {}
        return { result: { period: Number(t.period), count: Number(t.count), merkle_root: s.bytesToHex(t.merkle_root), per_attribution: perAttr } };
      }, { read: true });
  }, [invoke]);

  const computePayouts = useCallback(async (campaignId, code, period) => {
    return invoke("compute_payouts", "public", { campaign_id: Number(campaignId), code, period: Number(period) },
      async () => {
        const list = await sdk().computePayouts(Number(campaignId), code, Number(period));
        return { result: (list || []).map((p) => ({ to: String(p.to), amount: String(p.amount) })) };
      }, { read: true });
  }, [invoke]);

  const settle = useCallback(async (campaignId, code, period) => {
    return invoke("settle", "owner",
      { owner: wallet?.address, campaign_id: Number(campaignId), code, period: Number(period) },
      async () => {
        requireWallet();
        const s = sdk();
        const { retval, tx, seq } = await s.settle(wallet.address, Number(campaignId), code, Number(period));
        const payouts = (s.toNative(retval) || []).map((p) => ({ to: String(p.to), amount: String(p.amount) }));
        return { result: { payouts }, tx, seq };
      });
  }, [wallet, invoke]);

  const bumpTally = useCallback(async (campaignId, code, period) => {
    return invoke("bump_tally", "public", { campaign_id: Number(campaignId), code, periods: [Number(period)] },
      async () => {
        requireWallet(); // a signer/fee-payer is needed (anyone may bump)
        const { tx, seq } = await sdk().bumpTally(wallet.address, Number(campaignId), code, [Number(period)]);
        return { result: { ok: true }, tx, seq };
      });
  }, [wallet, invoke]);

  const value = {
    wallet, connecting, connect, disconnect,
    state, log, toasts, toast, dismissToast, pushLog, clearLog, highlight,
    createCampaign, issueCodes, redeem, verify, campaignStats,
    addDelegate, removeDelegate, isDelegate,
    registerShared, getShared, commitTally, getTally, computePayouts, settle, bumpTally,
  };
  return <AppCtx.Provider value={value}>{children}</AppCtx.Provider>;
}

Object.assign(window, { AppCtx, useApp, AppProvider, SDErr });
