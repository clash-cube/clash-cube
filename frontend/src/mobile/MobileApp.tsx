import { lazy, Suspense, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useStore, type View, type SettingsTab } from "../store";
import { useT } from "../i18n";
import { useConnectionFeed } from "../connectionStore";
import { Logo, Tools, Route, File, Exchange, Lines, Bell, Bars, Globe as GlobeIcon, Gear, Wifi, Shield, Gauge } from "../components/Icons";
import { Proxies } from "../views/Proxies";
import { Connections } from "../views/Connections";
import { Profiles } from "../views/Profiles";
import { Rules } from "../views/Rules";
import { Logs } from "../views/Logs";
import { Events } from "../views/Events";
import { Usage } from "../views/Usage";
import { Settings } from "../views/Settings";
import { MobileHome } from "./MobileHome";
import { useSystemBars } from "./systemBars";
import { useTouchMenus } from "./useTouchMenus";

const Globe = lazy(() => import("../views/Globe").then(m => ({ default: m.GlobeView })));
type Tab = "overview" | "proxies" | "profiles" | "tools";
type Page = View | "tools";
type RouteState = { tab: Tab; page: Page; settings?: SettingsTab; depth: number };
const home: RouteState = { tab: "overview", page: "overview", depth: 0 };
const labels: Record<Page, string> = { overview: "Home", tools: "Tools", proxies: "Proxies", connections: "Connections", profiles: "Profiles", rules: "Rules", logs: "Logs", events: "Events", usage: "Traffic statistics", globe: "Global connections", settings: "Settings" };
const settingLabels: Partial<Record<SettingsTab, string>> = { general: "General", network: "Network", tun: "VPN & Leak Protection", core: "Core" };
const tabs = [ ["overview", "Home", Logo], ["proxies", "Proxies", Route], ["profiles", "Profiles", File], ["tools", "Tools", Tools] ] as const;
const toolIcons = { connections: Exchange, rules: Route, logs: Lines, events: Bell, usage: Bars, globe: GlobeIcon, general: Gear, network: Wifi, tun: Shield, core: Gauge };

export function MobileApp() {
  useConnectionFeed();
  useSystemBars();
  useTouchMenus();
  const t = useT();
  const [route, setRoute] = useState<RouteState>(home);
  const current = useRef(route); current.current = route;
  const stacks = useRef<Partial<Record<Tab, RouteState[]>>>({});
  const scrolls = useRef<Record<string, number>>({});
  const content = useRef<HTMLDivElement>(null);
  const [scrolled, setScrolled] = useState(false);
  const [keyboard, setKeyboard] = useState(false);
  const [sheet, setSheet] = useState(false);
  const [backward, setBackward] = useState(false);
  const pending = useRef<(() => void) | null>(null);
  const key = `${route.tab}/${route.page}/${route.settings ?? ""}`;
  const apply = (r: RouteState) => {
    setRoute(r);
    useStore.setState({ view: r.page === "tools" ? "overview" : r.page, ...(r.settings ? { settingsTab: r.settings } : {}) });
  };
  const push = (r: RouteState) => { setBackward(false); history.pushState({ mobile: r }, ""); apply(r); };
  const pickTab = (tab: Tab) => {
    if (history.state?.search || history.state?.sheet || history.state?.editor) { pending.current = () => pickTab(tab); history.back(); return; }
    const old = current.current;
    if (tab === old.tab && old.page === tab) { content.current?.scrollTo({ top: 0, behavior: "smooth" }); return; }
    const restore = tab === old.tab ? [] : stacks.current[tab] ?? [];
    if (tab === old.tab) stacks.current[tab] = [];
    const go = () => {
      apply(home);
      if (tab !== "overview") push({ tab, page: tab, depth: 1 });
      for (const r of restore) push(r);
    };
    if (old.depth) { pending.current = go; history.go(-old.depth); } else go();
  };
  const navigate = (page: Page, settings?: SettingsTab) => {
    if (history.state?.sheet || history.state?.search) { pending.current = () => navigateRef.current(page, settings); history.back(); return; }
    if (tabs.some(([tab]) => tab === page)) { pickTab(page as Tab); return; }
    const old = current.current;
    const r = { tab: old.tab, page, settings, depth: old.depth + 1 };
    stacks.current[old.tab] = [...(stacks.current[old.tab] ?? []), r];
    push(r);
  };
  const navigateRef = useRef(navigate); navigateRef.current = navigate;
  useEffect(() => {
    history.replaceState({ mobile: home }, "");
    const original = useStore.getState().setView;
    useStore.setState({ setView: view => navigateRef.current(view, view === "settings" ? useStore.getState().settingsTab : undefined) });
    const back = (e: PopStateEvent) => {
      if (e.state?.sheet || e.state?.editor) return;
      const r = e.state?.mobile ?? home;
      setBackward(true);
      apply(r);
      if (pending.current) { const next = pending.current; pending.current = null; next(); }
      else if (current.current.page !== r.page || current.current.settings !== r.settings) stacks.current[r.tab] = (stacks.current[r.tab] ?? []).filter(p => p.depth <= r.depth);
    };
    window.addEventListener("popstate", back);
    const resize = () => setKeyboard((window.visualViewport?.height ?? window.innerHeight) < screen.height * .65 && /INPUT|TEXTAREA/.test(document.activeElement?.tagName ?? ""));
    window.visualViewport?.addEventListener("resize", resize);
    window.addEventListener("resize", resize);
    return () => { useStore.setState({ setView: original }); window.removeEventListener("popstate", back); window.removeEventListener("resize", resize); window.visualViewport?.removeEventListener("resize", resize); };
  }, []);
  useLayoutEffect(() => {
    if (content.current) content.current.scrollTop = scrolls.current[key] ?? 0;
    setScrolled((scrolls.current[key] ?? 0) > 0);
  }, [key]);
  useEffect(() => {
    // Sheets live in portals, including during their dismissal animation.
    const update = () => setSheet(!!document.querySelector(".mobile-sheet-scrim"));
    const observer = new MutationObserver(update);
    observer.observe(document.body, { childList: true });
    update();
    return () => observer.disconnect();
  }, []);
  const state = useStore(s => s.state);
  const sub = route.page !== route.tab;
  const title = route.settings ? settingLabels[route.settings] ?? "Settings" : labels[route.page];
  const pages = { proxies: <Proxies />, connections: <Connections />, profiles: <Profiles />, rules: <Rules />, logs: <Logs />, events: <Events />, usage: <Usage />, globe: <Globe />, settings: <Settings /> };
  const row = (page: Page, label: string, settings?: SettingsTab) => {
    const Icon = toolIcons[(settings ?? page) as keyof typeof toolIcons];
    return <button key={label} className="mobile-row mobile-tool" onClick={() => navigate(page, settings)}><Icon size={22} /><span>{t(label)}</span><span aria-hidden>›</span></button>;
  };
  return <div className="mobile-app">
    <header className={"mobile-bar" + (scrolled ? " scrolled" : "") + (route.page === "overview" ? " mobile-home-bar" : "")}>
      {sub && <button className="icon" aria-label={t("Back")} onClick={() => history.back()}>←</button>}
      <h1>{t(title)}</h1>
      <span id="mobile-page-count" className="mobile-count" />
      <div id="mobile-actions" />
    </header>
    <main ref={content} className="mobile-content" onScroll={e => { const y = e.currentTarget.scrollTop; scrolls.current[key] = y; setScrolled(y > 0); }}>
      <div key={key} className={"mobile-page" + (sub || backward ? " mobile-subpage" : "") + (backward ? " mobile-back" : "")}>
        <Suspense fallback={null}>
          {route.page === "overview" ? <MobileHome /> : route.page === "tools" ? <>
            <div className="list">{row("connections", "Connections")}{row("rules", "Rules")}{row("logs", "Logs")}{row("events", "Events")}{row("usage", "Traffic statistics")}{row("globe", "Global connections")}</div>
            <div className="section-title">{t("Settings")}</div><div className="list">{Object.entries(settingLabels).map(([s, label]) => row("settings", label, s as SettingsTab))}</div>
            <p className="mobile-version">{t("ClashCube")} {state?.appVersion} · {t("mihomo")} {state?.coreVersion || "—"}</p>
          </> : pages[route.page]}
        </Suspense>
      </div>
    </main>
    <nav className="mobile-tabs" hidden={keyboard || sheet} aria-label={t("Navigation")}><span className="mobile-dock-selection" style={{ transform: `translateX(${tabs.findIndex(([tab]) => tab === route.tab) * 100}%)` }} />{tabs.map(([tab, label, Icon]) => <button key={tab} aria-current={route.tab === tab ? "page" : undefined} onClick={() => pickTab(tab)}><Icon size={24} /><span>{t(label)}</span></button>)}</nav>
  </div>;
}
