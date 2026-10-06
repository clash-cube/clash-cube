import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useStore, type View } from "./store";
import { useT } from "./i18n";
import { Segmented } from "./components/Segmented";
import { Gear, Globe, Logo, Shield } from "./components/Icons";
import { Popover, Menu, type MenuItem } from "./components/Popover";
import { copyCommand, coreLabel, coreTone, openSettings, restartCore, setSystemProxy, setTun, startCore, stopCore } from "./actions";
import { speed } from "./format";
import { Overview } from "./views/Overview";
import { Usage } from "./views/Usage";
import { Proxies } from "./views/Proxies";
import { Profiles } from "./views/Profiles";
import { Connections } from "./views/Connections";
import { Rules } from "./views/Rules";
import { Logs } from "./views/Logs";
import { Events } from "./views/Events";
import { Settings } from "./views/Settings";
import { Prewarm } from "./components/Prewarm";
import { useConnectionFeed } from "./connectionStore";

const VIEWS: Record<View, () => JSX.Element> = {
  overview: Overview, usage: Usage, proxies: Proxies, profiles: Profiles, connections: Connections, rules: Rules, logs: Logs, events: Events, settings: Settings,
};

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
  const [wag, setWag] = useState(0);
  const Page = VIEWS[view] ?? Overview;
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
  const fitKey = `${width}|${lang}|${core}|${view === "overview"}`;
  const [fit, setFit] = useState({ key: "", level: 0 });
  const level = fit.key === fitKey ? fit.level : 0;
  useLayoutEffect(() => {
    const seg = topRef.current?.querySelector(".seg");
    const actions = topRef.current?.querySelector(".actions");
    if (!seg || !actions || level >= 2) return;
    if (actions.getBoundingClientRect().left < seg.getBoundingClientRect().right + 12) setFit({ key: fitKey, level: level + 1 });
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

  return (
    <div className="app window">
      <header className={"top" + (level ? " fit-" + level : "")} ref={topRef}>
        <div className="brand" onMouseEnter={() => setWag((w) => w + 1)}>
          <span className={"logo" + (wag ? " spin" : "")} key={wag}><Logo /></span>
          <span>{t("ClashCube")}</span>
        </div>
        <Segmented
          value={view === "settings" ? ("" as View) : view === "events" ? "logs" : view === "usage" ? "overview" : view}
          onChange={setView}
          options={[
            { value: "overview", label: t("Overview") },
            { value: "proxies", label: t("Proxies") },
            { value: "profiles", label: t("Profiles") },
            { value: "connections", label: t("Connections") },
            { value: "rules", label: t("Rules") },
            { value: "logs", label: t("Logs") },
          ]}
        />
        <div className="actions">
          {running && view !== "overview" && level < 1 && <span className="hdr-speed num">↑ {speed(traffic.up)}  ↓ {speed(traffic.down)}</span>}
          {running ? <>
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
          <button className={"icon" + (view === "settings" ? " on" : "")} title={t("Settings")} onClick={() => setView("settings")}><Gear /></button>
        </div>
        <Popover anchor={menu?.at ?? null} open={!!menu} onClose={close} align="end">
          <Menu close={close} items={menuItems()} />
        </Popover>
      </header>
      <Page key={view} />
      <Prewarm />
    </div>
  );
}
