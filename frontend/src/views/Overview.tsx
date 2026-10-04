import { useState } from "react";
import { App } from "../api";
import { useStore } from "../store";
import { usePoll } from "../usePoll";
import { TrafficClients } from "../components/TrafficClients";
import { useT } from "../i18n";
import { bytes, speed } from "../format";
import { Sparkline } from "../components/Sparkline";
import { ConnectivityCards } from "../components/ConnectivityCards";
import { Segmented } from "../components/Segmented";
import { Switch } from "../components/Switch";
import { Arrow, Globe, Shield, File, Chevron, Route } from "../components/Icons";
import { setMode, setSystemProxy, setTun, startCore, restartCore } from "../actions";
import { useConnectionStore } from "../connectionStore";
import { OverviewTabs } from "./Usage";

const modeHint: Record<string, string> = {
  rule: "Rules pick the policy for each connection",
  global: "Everything goes through the GLOBAL group",
  direct: "Everything connects directly",
};

export function Overview() {
  const t = useT();
  const state = useStore((s) => s.state);
  const traffic = useStore((s) => s.traffic);
  const history = useStore((s) => s.history);
  const memory = useStore((s) => s.memory);
  const setView = useStore((s) => s.setView);
  const conns = useConnectionStore((s) => s.snapshot.active.length);
  const running = state?.core === "running";
  const mode = state?.mode ?? "rule";
  const today = useToday();

  return (
    <div className="view overview">
      <div className="view-head"><OverviewTabs value="overview" /></div>
      {state?.core === "crashed" && (
        <div className="banner err">
          <div className="grow">
            <b>{t("Core stopped with an error")}</b>
            <div className="mono">{state.coreError}</div>
          </div>
          <button className="btn small" onClick={() => setView("logs")}>{t("Show logs")}</button>
          <button className="btn small primary" onClick={restartCore}>{t("Restart")}</button>
        </div>
      )}

      {running && state?.proxyLost && (
        <div className="banner warn">
          <div className="grow">
            <b>{t("Another app changed the system proxy")}</b>
            <div>{t("Apps that follow the system proxy no longer go through MihomoBar.")}</div>
          </div>
          <button className="btn small primary" onClick={() => setSystemProxy(true)}>{t("Take it back")}</button>
        </div>
      )}

      <div className="card traffic-card">
        <div className="traffic-main">
          <div className="traffic-chart">
            <div className="traffic-head">
              <div className="rate up"><Arrow dir="up" size={13} /><span className="num">{speed(traffic.up)}</span><span className="lbl">{t("Upload")}</span></div>
              <div className="rate down"><Arrow dir="down" size={13} /><span className="num">{speed(traffic.down)}</span><span className="lbl">{t("Download")}</span></div>
              <div className="grow" />
              {!running && state?.core !== "starting" && <button className="btn small primary" onClick={startCore}>{t("Start core")}</button>}
            </div>
            <Sparkline data={history} height={88} grid />
          </div>
          <TrafficClients />
        </div>
        <div className="traffic-stats">
          <Stat label={t("Today")} up={today?.up} down={today?.down} onClick={() => setView("usage")} title={t("Show traffic statistics")} />
          <Stat
            label={t("Proxied today")}
            value={today?.share === undefined ? "—" : pct(today.share)}
            share={today?.share}
            title={today ? `${t("Proxy")} ${bytes(today.proxied)}   ${t("Direct")} ${bytes(today.direct)}` : undefined}
          />
          <Stat label={t("This run")} up={running ? traffic.upTotal : undefined} down={running ? traffic.downTotal : undefined}
            title={running ? `${t("Core memory")} ${bytes(memory)}` : undefined} />
          <Stat label={t("Connections")} value={running ? String(conns) : "—"} onClick={() => setView("connections")} title={t("Show connections")} />
        </div>
      </div>

      <ConnectivityCards />

      <div className="list controls">
        <div className="row">
          <span className="ic"><Route /></span>
          <div className="who"><div className="name">{t("Outbound Mode")}</div><div className="sub">{t(modeHint[mode] ?? "")}</div></div>
          <Segmented
            className="track small"
            value={mode}
            onChange={setMode}
            options={[{ value: "rule", label: t("Rule") }, { value: "global", label: t("Global") }, { value: "direct", label: t("Direct") }]}
          />
        </div>
        <div className="row">
          <span className={"ic" + (state?.systemProxy ? " on" : "")}><Globe /></span>
          <div className="who"><div className="name">{t("System Proxy")}</div><div className="sub">{t("Route apps that respect the macOS proxy settings")} · 127.0.0.1:{state?.mixedPort}</div></div>
          <Switch on={!!state?.systemProxy} onChange={setSystemProxy} label={t("System Proxy")} />
        </div>
        <div className="row">
          <span className={"ic" + (state?.tun ? " on" : "")}><Shield /></span>
          <div className="who"><div className="name">{t("Enhanced Mode")}</div><div className="sub">{state?.serviceMode ? t("TUN: capture all traffic, including terminals and games") : t("Installs a privileged helper on first use")}</div></div>
          <Switch on={!!state?.tun} onChange={setTun} label={t("Enhanced Mode")} />
        </div>
        <button className="row click" style={{ width: "100%", textAlign: "left" }} onClick={() => setView("profiles")}>
          <span className="ic"><File /></span>
          <div className="who"><div className="name">{state?.profileName}</div><div className="sub">{t("Current profile")}</div></div>
          <span className="end"><Chevron /></span>
        </button>
      </div>
    </div>
  );
}

// Stat is a cell of the strip under the chart: a figure, or a total with a
// thin bar of how it splits into upload and download, or a share with a
// bar of it. A cell that leads somewhere is a button.
function Stat({ label, value, up, down, share, onClick, title }: {
  label: string; value?: string; up?: number; down?: number; share?: number; onClick?: () => void; title?: string;
}) {
  const t = useT();
  const split = up !== undefined && down !== undefined;
  const sum = split ? up + down : 0;
  const tip = [title, split && `↑ ${bytes(up)}   ↓ ${bytes(down)}`].filter(Boolean).join("\n") || undefined;
  const body = (
    <>
      <div className="lbl">{label}</div>
      <div className="val num">{split ? bytes(sum) : value ?? "—"}</div>
      {split && (
        <div className="tsplit" aria-label={`${t("Upload")} ${bytes(up)}, ${t("Download")} ${bytes(down)}`}>
          <i className="up" style={{ width: sum ? `${(up / sum) * 100}%` : 0 }} />
          <i className="down" style={{ width: sum ? `${(down / sum) * 100}%` : 0 }} />
        </div>
      )}
      {share !== undefined && <div className="tsplit"><i className="share" style={{ width: `${share * 100}%` }} /></div>}
    </>
  );
  return onClick
    ? <button className="tstat click" title={tip} onClick={onClick}>{body}</button>
    : <div className="tstat" title={tip}>{body}</div>;
}

const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

// whole percent, but a little is never shown as none, nor most as all
const pct = (f: number) => (f > 0 && f < .01 ? "<1%" : f < 1 && f > .99 ? ">99%" : `${Math.round(f * 100)}%`);

// what carries no traffic of its own, or none worth a share
const UNROUTED = new Set(["", "REJECT", "REJECT-DROP"]);

// useToday is today's traffic from the statistics kept across runs, and
// how much of it went through a proxy rather than DIRECT; the share is
// undefined while nothing has moved.
function useToday() {
  const [today, setToday] = useState<{ up: number; down: number; proxied: number; direct: number; share?: number } | null>(null);
  usePoll(() => {
    const d = ymd(new Date());
    App.Usage(d, d, -1).then((r) => {
      let proxied = 0, direct = 0;
      for (const p of r.dims?.policy ?? []) {
        if (p.key === "DIRECT") direct += p.up + p.down;
        else if (!UNROUTED.has(p.key)) proxied += p.up + p.down;
      }
      const sum = proxied + direct;
      setToday({ up: r.up, down: r.down, proxied, direct, share: sum ? proxied / sum : undefined });
    }).catch(() => {});
  }, 10000);
  return today;
}
