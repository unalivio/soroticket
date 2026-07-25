import React from "react";
import { createRoot } from "react-dom/client";
import "./tokens.css";
import "./app.css";
import { AppProvider, useApp } from "./store.jsx";
import { Shell } from "./shell.jsx";
import { AuthPage, CreateOrgPage } from "./pages/Auth.jsx";
import { OverviewPage } from "./pages/Overview.jsx";
import { CampaignsPage } from "./pages/Campaigns.jsx";
import { CampaignDetailPage } from "./pages/CampaignDetail.jsx";
import { RedemptionsPage } from "./pages/Redemptions.jsx";
import { SettlementsPage } from "./pages/Settlements.jsx";
import { LoyaltyPage, LoyaltyDetailPage } from "./pages/Loyalty.jsx";
import { ApiKeysPage, UsagePage, WebhooksPage, SettingsPage } from "./pages/Platform.jsx";

function Router() {
  const { booting, user, org, route } = useApp();
  if (booting) return <div style={{ minHeight: "100vh", display: "grid", placeItems: "center" }}><span className="mono faint">Soroticket Console…</span></div>;
  if (!user) return <AuthPage />;
  if (!org) return <CreateOrgPage />;

  const campDetail = route.match(/^\/campaigns\/(\d+)/);
  const loyaltyDetail = route.match(/^\/loyalty\/(\d+)/);

  let page;
  if (campDetail) page = <CampaignDetailPage id={Number(campDetail[1])} />;
  else if (loyaltyDetail) page = <LoyaltyDetailPage id={Number(loyaltyDetail[1])} />;
  else if (route.startsWith("/campaigns")) page = <CampaignsPage />;
  else if (route.startsWith("/redemptions")) page = <RedemptionsPage />;
  else if (route.startsWith("/settlements")) page = <SettlementsPage />;
  else if (route.startsWith("/loyalty")) page = <LoyaltyPage />;
  else if (route.startsWith("/keys")) page = <ApiKeysPage />;
  else if (route.startsWith("/usage")) page = <UsagePage />;
  else if (route.startsWith("/webhooks")) page = <WebhooksPage />;
  else if (route.startsWith("/settings")) page = <SettingsPage />;
  else page = <OverviewPage />;

  return <Shell>{page}</Shell>;
}

createRoot(document.getElementById("root")).render(
  <AppProvider><Router /></AppProvider>
);
