/* Sorodeal Console — API client. Same-origin via the Vite proxy (/api → :8787),
   session cookie auth, environment via X-Env. Contract errors arrive as
   problem+json {status, code, name, message} and are re-thrown enriched. */

let currentEnv = localStorage.getItem("sd_env") || "test";

export function getEnv() { return currentEnv; }
export function setEnv(env) { currentEnv = env; localStorage.setItem("sd_env", env); }

export class ApiError extends Error {
  constructor(problem, status) {
    super(problem.message || `HTTP ${status}`);
    this.status = status;
    this.code = problem.code || 0;       // contract error #
    this.name = problem.name || "ApiError";
  }
}

async function req(method, path, body, opts = {}) {
  const headers = { "X-Env": currentEnv, ...(opts.headers || {}) };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch("/api" + path, {
    method, headers, credentials: "same-origin",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let data = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = { message: text }; }
  if (!res.ok) throw new ApiError(data, res.status);
  return data;
}

export const api = {
  get: (p) => req("GET", p),
  post: (p, body, opts) => req("POST", p, body ?? {}, opts),
  // idempotent post: caller-supplied key survives retries
  postIdem: (p, body, key) => req("POST", p, body ?? {}, { headers: { "Idempotency-Key": key } }),
};

export const idemKey = () => crypto.randomUUID();

/* ── formatting helpers ────────────────────────────────────────── */
export const trunc = (s, a = 4, b = 4) =>
  !s || s.length <= a + b + 1 ? s || "" : s.slice(0, a) + "…" + s.slice(-b);

export const fmtCr = (mcr) => {
  const cr = mcr / 1000;
  return cr.toLocaleString(undefined, { maximumFractionDigits: cr < 10 ? 1 : 0 });
};

export const decimalToBaseUnits = (input, decimals = 7) => {
  const raw = String(input ?? "").trim();
  const match = /^(\d+)(?:\.(\d+))?$/.exec(raw);
  if (!match || decimals < 0 || decimals > 18) throw new RangeError("Enter a non-negative decimal amount.");
  const fraction = match[2] || "";
  if (fraction.length > decimals) throw new RangeError(`Amount supports at most ${decimals} decimal places.`);
  const scale = 10n ** BigInt(decimals);
  const fractional = fraction.padEnd(decimals, "0") || "0";
  return (BigInt(match[1]) * scale + BigInt(fractional)).toString();
};

export const fmtUnits = (base) => {
  try {
    const value = BigInt(base ?? 0);
    const negative = value < 0n;
    const absolute = negative ? -value : value;
    // Seven base-unit decimals → two display decimals, rounded half-up without
    // ever passing through an imprecise JavaScript Number.
    const hundredths = (absolute + 50_000n) / 100_000n;
    const whole = hundredths / 100n;
    const fraction = (hundredths % 100n).toString().padStart(2, "0");
    return `${negative ? "-" : ""}${whole.toLocaleString()}.${fraction}`;
  } catch {
    return "—";
  }
};

export const fmtTime = (ts) => new Date(ts * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
export const fmtDate = (ts) => new Date(ts * 1000).toLocaleDateString([], { month: "short", day: "numeric" });
export const fmtDateTime = (ts) => new Date(ts * 1000).toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });

export const expertTx = (hash) => `https://stellar.expert/explorer/testnet/tx/${hash}`;
export const expertAccount = (addr) => `https://stellar.expert/explorer/testnet/account/${addr}`;
