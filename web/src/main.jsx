/* Vite entry. Imports run in textual order, so globals.js sets up window.React
   before any design file or the Soroban SDK module evaluates. Each subsequent
   module attaches its symbols to window; app.jsx (last) renders the tree. */
import "./styles.css";
import "./layout.css";
import "./console.css";

import "./globals.js";       // window.React + hooks + ReactDOM
import "./data.js";          // window.SD  (testnet constants, snippets, helpers)
import "./lib/soroban.js";   // window.SDK (real Soroban RPC + Freighter signing)

import "./tweaks-panel.jsx";
import "./components.jsx";
import "./store.jsx";        // window.AppProvider / useApp — real contract calls
import "./topbar.jsx";
import "./hero.jsx";
import "./cards-core.jsx";
import "./cards-a.jsx";
import "./cards-b.jsx";
import "./cards-c.jsx";
import "./cards-tally.jsx";  // Tally profile cards (shared codes + settlement)
import "./console.jsx";
import "./app.jsx";          // ReactDOM.createRoot(...).render(<Root/>)
