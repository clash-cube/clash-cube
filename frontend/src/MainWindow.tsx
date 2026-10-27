import { Fragment, lazy, Suspense, useEffect, useLayoutEffect, useRef, useState, type ComponentType, type CSSProperties } from "react";
import { flushSync } from "react-dom";
import { useStore, type View } from "./store";
import { useT } from "./i18n";
import { Segmented } from "./components/Segmented";
import { Gear, Globe, Shield, SidebarIcon } from "./components/Icons";
import { Brand, calm, Sidebar, sideFit } from "./components/Sidebar";
import { crumb, pageOf, pages } from "./nav";
import { isWindows, isWeb } from "./platform";
import { Popover, Menu, type MenuItem } from "./components/Popover";
import { copyCommand, coreLabel, coreTone, openSettings, restartCore, setSystemProxy, setTun, startCore, stopCore } from "./actions";
import { speed } from "./format";
import { Overview } from "./views/Overview";
import { OverviewTabs, Usage } from "./views/Usage";
import { Proxies } from "./views/Proxies";
import { Profiles } from "./views/Profiles";
import { Connections } from "./views/Connections";
import { Rules } from "./views/Rules";
import { Logs } from "./views/Logs";
import { Events } from "./views/Events";
import { Settings } from "./views/Settings";
import { Prewarm } from "./components/Prewarm";
import { useConnectionFeed } from "./connectionStore";

// three.js is loaded only once the overview shows, ahead of its globe tab
const loadGlobe = () => import("./views/Globe");
const GlobeView = lazy(() => loadGlobe().then((m) => ({ default: m.GlobeView })));
// the pages sharing the overview's tab: switching among them keeps the head
const OVERVIEW_TABS = new Set<View>(["overview", "usage", "globe"]);

const VIEWS: Record<View, ComponentType> = {
  overview: Overview, usage: Usage, globe: GlobeView, proxies: Proxies, profiles: Profiles, connections: Connections, rules: Rules, logs: Logs, events: Events, settings: Settings,
};

// The pages are down a sidebar or along the header, the reader's pick with
// the title bar's button (⌃⌘S) kept for the window, as a Mac app keeps its
// sidebar shown or hidden; none picked, the sidebar from this width.
const SIDE_AT = 820;
const sideQuery = matchMedia(`(min-width: ${SIDE_AT}px)`);
const read = (k: string) => { try { return localStorage.getItem(k) ?? ""; } catch { return ""; } };
const keep = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch {} };

// The two layouts trade places as one motion (app.css .nav-morph): the
// sidebar slides in from the window's edge or back to it, each page's tab
// flies to its place in the other, and the page moves over. A webview
// without view transitions, or a reader who asked for less motion, has the
// new one at once; a transition that never ends is cut short.
function morph(change: () => void) {
  const doc = document as Document & { startViewTransition?: (cb: () => void) => { finished: Promise<void>; skipTransition: () => void } };
  if (calm() || !doc.startViewTransition) { change(); return; }
  const root = document.documentElement;
  root.classList.add("nav-morph");
  const vt = doc.startViewTransition(() => flushSync(change));
  const stuck = setTimeout(() => vt.skipTransition(), 1500);
  vt.finished.finally(() => { clearTimeout(stuck); root.classList.remove("nav-morph"); });
}

export function MainWindow() {
  useConnectionFeed();
  const t = useT();
  const view = useStore((s) => s.view);
  const setView = useStore((s) => s.setView);
  const state = useStore((s) => s.state);
  const traffic = useStore((s) => s.traffic);
  const [menu, setMenu] = useState<{ kind: "proxy" | "tun" | "core"; at: HTMLElement } | null>(null);
  // the switch being turned, its dot blinking until the state says so
  const [pending, setPending] = useState<"proxy" | "tun" | null>(null);
  const settingsTab = useStore((s) => s.settingsTab);
  const [pick, setPick] = useState(() => read("nav.layout"));
  const [wide, setWide] = useState(sideQuery.matches);
  useEffect(() => {
    const on = () => setWide(sideQuery.matches);
    sideQuery.addEventListener("change", on);
    return () => sideQuery.removeEventListener("change", on);
  }, []);
  const side = pick === "side" || pick === "top" ? pick === "side" : wide;
  const [sideW, setSideW] = useState(() => Math.round(+read("sidebar.width")) || 0);
  const toggleSide = () => {
    const next = side ? "top" : "side";
    keep("nav.layout", next);
    morph(() => setPick(next));
  };
  const keepWidth = (w: number) => { setSideW(w); keep("sidebar.width", String(w)); };
  const toggleRef = useRef(toggleSide);
  toggleRef.current = toggleSide;
  // ⌃⌘S as a Mac's View menu has it, Ctrl+Shift+S on Windows
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      const mod = isWindows ? e.ctrlKey && e.shiftKey && !e.metaKey : e.metaKey && e.ctrlKey && !e.shiftKey;
      if (!mod || e.altKey || e.repeat || e.code !== "KeyS" || e.defaultPrevented) return;
      e.preventDefault();
      toggleRef.current();
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  const Page = VIEWS[view] ?? Overview;
  // a switch among the overview's tabs moves only what is under the head.
  // Decided once per page and kept: dropping the class on a later render
  // would hand the page its entrance again, and it would play then.
  const entered = useRef({ view, tabSwitch: false });
  if (entered.current.view !== view) {
    entered.current = { view, tabSwitch: OVERVIEW_TABS.has(view) && OVERVIEW_TABS.has(entered.current.view) };
  }
  const tabSwitch = entered.current.tabSwitch;
  useEffect(() => { if (OVERVIEW_TABS.has(view)) loadGlobe().catch(() => {}); }, [view]);
  const core = state?.core ?? "stopped";
  const running = core === "running";
  const close = () => setMenu(null);

  // How much the header gives up so its right side clears the centred tabs:
  // 1 drops the speed, 2 also the pills' labels. Measured again from 0 when
  // anything that changes the widths does.
  const topRef = useRef<HTMLElement>(null);
  const [width, setWidth] = useState(window.innerWidth);
  useEffect(() => {
    const on = () => setWidth(window.innerWidth);
    window.addEventListener("resize", on);
    return () => window.removeEventListener("resize", on);
  }, []);
  const lang = useStore((s) => s.settings?.lang);
  const fitKey = `${width}|${lang}|${core}|${side ? `${view}|${settingsTab}|${sideW}` : view === "overview"}`;
  const [fit, setFit] = useState({ key: "", level: 0 });
  const level = fit.key === fitKey ? fit.level : 0;
  // beside the sidebar, the room is short once where the reader is (.crumb)
  // can't be read whole
  useLayoutEffect(() => {
    const seg = topRef.current?.querySelector(side ? ".crumb" : ".seg");
    const actions = topRef.current?.querySelector(".actions");
    if (!seg || !actions || level >= 2) return;
    const short = side ? seg.scrollWidth > seg.clientWidth + 1 : actions.getBoundingClientRect().left < seg.getBoundingClientRect().right + 12;
    if (short) setFit({ key: fitKey, level: level + 1 });
  }, [fitKey, level]);
  const turn = async (kind: "proxy" | "tun", p: Promise<unknown>) => {
    setPending(kind);
    try { await p; } finally { setPending(null); }
  };
  const menuItems = (): MenuItem[] => {
    switch (menu?.kind) {
      case "proxy": return [
        state?.systemProxy && !state.proxyLost
          ? { label: t("Turn off System Proxy"), onClick: () => turn("proxy", setSystemProxy(false)) }
          : { label: state?.proxyLost ? t("Take it back") : t("Set as System Proxy"), onClick: () => turn("proxy", setSystemProxy(true)) },
        ...(state?.proxyLost ? [{ label: t("Turn off System Proxy"), onClick: () => turn("proxy", setSystemProxy(false)) }] : []),
        "sep",
        { label: t("Copy Shell Export Command"), onClick: () => copyCommand(false) },
        { label: t("Copy Shell Export Command with LAN IP"), onClick: () => copyCommand(true) },
        "sep",
        { label: t("Network Settings…"), onClick: () => openSettings("network") },
      ];
      case "tun": return [
        { label: state?.tun ? t("Turn off Enhanced Mode") : t("Turn on Enhanced Mode"), onClick: () => turn("tun", setTun(!state?.tun)) },
        "sep",
        { label: t("Component State…"), onClick: () => openSettings("tun") },
      ];
      case "core": return running ? [
        { label: t("Restart core"), onClick: restartCore },
        { label: t("Stop core"), onClick: stopCore, danger: true },
      ] : [
        { label: t("Start core"), onClick: startCore },
      ];
      default: return [];
    }
  };

  const sideLabel = t(side ? "Hide sidebar" : "Show sidebar");
  const parts = side ? crumb(t, view, settingsTab) : [];

  return (
    <div className={"app window" + (tabSwitch ? " tab-switch" : "") + (side ? " side-nav" : "")}
      style={side && sideW ? { "--side-w": sideFit(sideW) + "px" } as CSSProperties : undefined}>
      {side && <Sidebar width={sideW} onWidth={keepWidth} onHide={toggleSide} />}
      <header className={"top" + (level ? " fit-" + level : "")} ref={topRef}>
        {!side && <Brand />}
        <button className="icon side-toggle" onClick={toggleSide} aria-pressed={side} aria-label={sideLabel}
          title={`${sideLabel} (${isWindows ? "Ctrl+Shift+S" : "⌃⌘S"})`}><SidebarIcon /></button>
        {side ? (
          <div className="crumb">
            {parts.map((p, i) => (
              <Fragment key={i}>{i > 0 && <i>/</i>}{i === parts.length - 1 ? <b>{p}</b> : <span>{p}</span>}</Fragment>
            ))}
          </div>
        ) : (
          <Segmented
            value={view === "settings" ? ("" as View) : pageOf(view)}
            onChange={setView}
            options={pages(t).map((p) => ({ value: p.view, label: p.label }))}
          />
        )}
        <div className="actions">
          {running && view !== "overview" && level < 1 && <span className="hdr-speed num">↑ {speed(traffic.up)}  ↓ {speed(traffic.down)}</span>}
          {running && !isWeb ? <>
            <button className="pill-status mode" onClick={(e) => setMenu(menu?.kind === "proxy" ? null : { kind: "proxy", at: e.currentTarget })}
              title={state?.proxyLost ? t("Taken by another app") : t("System Proxy")}>
              <span className={"cdot " + (pending === "proxy" ? "ok testing" : state?.proxyLost ? "ok" : state?.systemProxy ? "good" : "none")} />
              <span className="pill-ic"><Globe size={13} /></span><span className="pill-label">{t("System Proxy")}</span>
            </button>
            <button className="pill-status mode" onClick={(e) => setMenu(menu?.kind === "tun" ? null : { kind: "tun", at: e.currentTarget })} title={t("Enhanced Mode")}>
              <span className={"cdot " + (pending === "tun" ? "ok testing" : state?.tun ? "good" : "none")} />
              <span className="pill-ic"><Shield size={13} /></span><span className="pill-label">{t("Enhanced Mode")}</span>
            </button>
          </> : (
            <button className="pill-status" onClick={(e) => setMenu(menu?.kind === "core" ? null : { kind: "core", at: e.currentTarget })} title={state?.coreError}>
              <span className={"cdot " + coreTone()} />{coreLabel()}
            </button>
          )}
          {!side && <button className={"icon gear" + (view === "settings" ? " on" : "")} title={t("Settings")} onClick={() => setView("settings")}><Gear /></button>}
        </div>
        <Popover anchor={menu?.at ?? null} open={!!menu} onClose={close} align="end">
          <Menu close={close} items={menuItems()} />
        </Popover>
      </header>
      <Suspense fallback={<div className="view">{OVERVIEW_TABS.has(view) && <div className="view-head"><OverviewTabs value={view as "overview" | "usage" | "globe"} /></div>}</div>}>
        <Page key={view} />
      </Suspense>
      <Prewarm />
    </div>
  );
}
