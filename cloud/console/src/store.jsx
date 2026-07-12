import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import { api, getEnv, setEnv as persistEnv } from "./api.js";
import { Toast } from "./ui.jsx";

const Ctx = createContext(null);
export const useApp = () => useContext(Ctx);

export function AppProvider({ children }) {
  const [booting, setBooting] = useState(true);
  const [user, setUser] = useState(null);
  const [org, setOrg] = useState(null);
  const [env, setEnvState] = useState(getEnv());
  const [route, setRoute] = useState(location.hash.slice(1) || "/overview");
  const [toasts, setToasts] = useState([]);

  useEffect(() => {
    const onHash = () => setRoute(location.hash.slice(1) || "/overview");
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const nav = useCallback((r) => { location.hash = r; }, []);

  const setEnv = useCallback((e) => { persistEnv(e); setEnvState(e); }, []);

  const toast = useCallback((title, msg, type = "success") => {
    const id = crypto.randomUUID();
    setToasts((t) => [...t, { id, title, msg, type }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 5200);
  }, []);

  const toastErr = useCallback((e) => {
    toast(e.code ? `#${e.code} ${e.name}` : "Something went wrong", e.message, "error");
  }, [toast]);

  const refreshMe = useCallback(async () => {
    try {
      const me = await api.get("/auth/me");
      setUser(me.user); setOrg(me.org || null);
      return me;
    } catch {
      setUser(null); setOrg(null);
      return null;
    }
  }, []);

  useEffect(() => { refreshMe().finally(() => setBooting(false)); }, [refreshMe]);

  const value = { booting, user, org, env, setEnv, route, nav, toast, toastErr, refreshMe };
  return (
    <Ctx.Provider value={value}>
      {children}
      <div className="toast-wrap">{toasts.map((t) => <Toast key={t.id} t={t} />)}</div>
    </Ctx.Provider>
  );
}
