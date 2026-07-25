/* ═══════════════════════════════════════════════════════════════════
   Soroticket Playground — Soroban integration → window.SDK
   Thin bridge: the real client now lives ONCE in @soroticket/sdk
   (`freighterClient`). Here we just bind it to this deployment
   (window.SD.NET, set by data.js) and expose it as window.SDK, the
   shape store.jsx consumes. No client logic lives in the web anymore.
   ═══════════════════════════════════════════════════════════════════ */
import { freighterClient } from "@soroticket/sdk";

const NET = (window.SD && window.SD.NET) || {};

window.SDK = freighterClient({
  contractId: NET.contractId,
  rpcUrl: NET.rpc,
  networkPassphrase: NET.passphrase,
  readSource: NET.ownerWallet, // any funded account; source for read-only sims
});
