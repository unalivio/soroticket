/* ═══════════════════════════════════════════════════════════════════
   Sorodeal Playground — app root + Tweaks
   ═══════════════════════════════════════════════════════════════════ */

const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "theme": "light",
  "accent": "yellow",
  "console": "bottom",
  "serifDisplay": true
}/*EDITMODE-END*/;

function Root() {
  const [t, setTweak] = useTweaks(TWEAK_DEFAULTS);

  useEffect(() => {
    const r = document.documentElement;
    r.dataset.theme = t.theme;
    r.dataset.accent = t.accent;
    r.dataset.display = t.serifDisplay ? "serif" : "sans";
    document.body.classList.toggle("dock-bottom", t.console === "bottom");
  }, [t.theme, t.accent, t.serifDisplay, t.console]);

  return (
    <AppProvider>
      <TopBar />
      <main>
        <Hero />
        <section className="band-gray" style={{ padding: "8px 0 48px" }}>
          <div className="wrap">
            <div className="section-head" style={{ paddingTop: 56, marginBottom: 18 }}>
              <div className="eyebrow">The protocol lifecycle</div>
              <h2 className="display" style={{ fontSize: "clamp(30px,4.4vw,46px)" }}>Call the contract, step by step</h2>
            </div>
            <div className="actions">
              <CreateCampaign />
              <IssueCodes />
              <Redeem />
              <Verify />
              <Delegates />
              <TallyRegister />
              <TallySettle />
            </div>
          </div>
        </section>
        <Benefits />
        <Footer />
      </main>

      <Console placement={t.console} />
      <Toasts />

      <TweaksPanel>
        <TweakSection label="Theme direction" />
        <TweakRadio label="Mode" value={t.theme} options={["light", "dark"]} onChange={(v) => setTweak("theme", v)} />
        <TweakRadio label="Accent" value={t.accent} options={["yellow", "cyan"]} onChange={(v) => setTweak("accent", v)} />
        <TweakToggle label="Serif display headings" value={t.serifDisplay} onChange={(v) => setTweak("serifDisplay", v)} />
        <TweakSection label="Console" />
        <TweakRadio label="Placement" value={t.console} options={["bottom", "right"]} onChange={(v) => setTweak("console", v)} />
      </TweaksPanel>
    </AppProvider>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<Root />);
