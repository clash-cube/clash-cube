import { useState } from "react";
import { useStore, type View } from "./store";
import { useT } from "./i18n";
import { Segmented } from "./components/Segmented";
import { Gear, Logo } from "./components/Icons";
import { Popover, Menu } from "./components/Popover";
import { coreLabel, coreTone, restartCore, startCore, stopCore } from "./actions";
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
  const [coreAt, setCoreAt] = useState<HTMLElement | null>(null);
  const [wag, setWag] = useState(0);
  const Page = VIEWS[view] ?? Overview;
  const core = state?.core ?? "stopped";
  const running = core === "running";

  return (
    <div className="app window">
      <header className="top">
        <div className="brand" onMouseEnter={() => setWag((w) => w + 1)}>
          <span className={"logo" + (wag ? " spin" : "")} key={wag}><Logo /></span>
          <span>MihomoBar</span>
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
          {running && view !== "overview" && <span className="hdr-speed num">↑ {speed(traffic.up)}  ↓ {speed(traffic.down)}</span>}
          <button className="pill-status" onClick={(e) => setCoreAt(coreAt ? null : e.currentTarget)} title={state?.coreError}>
            <span className={"cdot " + coreTone()} />{coreLabel()}
          </button>
          <button className={"icon" + (view === "settings" ? " on" : "")} title={t("Settings")} onClick={() => setView("settings")}><Gear /></button>
        </div>
        <Popover anchor={coreAt} open={!!coreAt} onClose={() => setCoreAt(null)} align="end">
          <Menu close={() => setCoreAt(null)} items={running ? [
            { label: t("Restart core"), onClick: restartCore },
            { label: t("Stop core"), onClick: stopCore, danger: true },
          ] : [
            { label: t("Start core"), onClick: startCore },
          ]} />
        </Popover>
      </header>
      <Page key={view} />
      <Prewarm />
    </div>
  );
}
