import { useState, type ReactNode } from "react";
import { App } from "../api";
import { useStore } from "../store";
import { usePoll } from "../usePoll";
import { TrafficClients, clientOf } from "../components/TrafficClients";
import { useT } from "../i18n";
import { bytes, speed } from "../format";
import { Sparkline } from "../components/Sparkline";
import { ConnectivityCards } from "../components/ConnectivityCards";
import { AIChecks } from "../components/AIChecks";
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
  const apps = useConnectionStore((s) => new Set(s.snapshot.active.map((c) => clientOf(c).key)).size);
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
          {state.refusal
            ? <button className="btn small" onClick={() => { location.hash = "merged"; setView("profiles"); }}>{t("Show")}</button>
            : <button className="btn small" onClick={() => setView("logs")}>{t("Show logs")}</button>}
          <button className="btn small primary" onClick={restartCore}>{t("Restart")}</button>
        </div>
      )}

      {running && state?.proxyLost && (
        <div className="banner warn">
          <div className="grow">
            <b>{t("Another app changed the system proxy")}</b>
            <div>{t("Apps that follow the system proxy no longer go through ClashCube.")}</div>
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
          <Stat
            label={t("Today")}
            value={today ? bytes(today.up + today.down) : undefined}
            sub={today && <UpDown up={today.up} down={today.down} />}
            onClick={() => setView("usage")}
            title={t("Show traffic statistics")}
          />
          <Stat
            label={t("Proxied")}
            value={today?.share === undefined ? undefined : pct(today.share)}
            ring={today?.share}
            sub={today?.share === undefined ? undefined : today.proxied === 0 ? t("All direct today")
              : <><i className="key" />{t("Proxy")} {bytes(today.proxied)} · {t("Direct")} {bytes(today.direct)}</>}
            title={t("Today's traffic through a proxy rather than DIRECT")}
          />
          <Stat
            label={t("This run")}
            value={running ? bytes(traffic.upTotal + traffic.downTotal) : undefined}
            sub={running && <UpDown up={traffic.upTotal} down={traffic.downTotal} />}
            title={running ? `${t("Since the core started")}\n${t("Core memory")} ${bytes(memory)}` : undefined}
          />
          <Stat
            label={t("Connections")}
            value={running ? String(conns) : undefined}
            sub={running && apps > 0 ? t(apps === 1 ? "{n} app" : "{n} apps", { n: apps }) : undefined}
            onClick={() => setView("connections")}
            title={t("Show connections")}
          />
        </div>
      </div>

      <ConnectivityCards />

      <AIChecks />

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

// Stat is a cell of the strip under the chart, each laid out alike: a
// label, a figure with its unit set small, and a faint line of detail. The
// proxied share alone also draws, a small ring, so the row has one thing
// to look at. A cell that leads somewhere is a button with a faint chevron.
function Stat({ label, value, sub, ring, onClick, title }: {
  label: string; value?: string; sub?: ReactNode; ring?: number; onClick?: () => void; title?: string;
}) {
  // "51.7 MB" → 51.7 and a small MB; "23%" → 23 and a small %
  const m = value?.match(/^([<>]?[\d.,]+)\s*(.*)$/);
  const body = (
    <>
      <div className="lbl">{label}{onClick && <Chevron size={10} className="go" />}</div>
      <div className="val num">
        <span>{m ? <>{m[1]}{m[2] && <small>{m[2] === "%" ? "%" : " " + m[2]}</small>}</> : value ?? "—"}</span>
        {ring !== undefined && <Ring share={ring} />}
      </div>
      <div className="tsub num">{sub}</div>
    </>
  );
  return onClick
    ? <button className="tstat click" title={title} onClick={onClick}>{body}</button>
    : <div className="tstat" title={title}>{body}</div>;
}

const UpDown = ({ up, down }: { up: number; down: number }) => <><span className="ud up"><b>↑</b> {bytes(up)}</span><span className="ud down"><b>↓</b> {bytes(down)}</span></>;

// Ring is a share as a small arc over a faint full circle.
function Ring({ share }: { share: number }) {
  const r = 5.25, c = 2 * Math.PI * r;
  return (
    <svg className="tring" viewBox="0 0 14 14" width="14" height="14" aria-hidden>
      <circle cx="7" cy="7" r={r} />
      {/* a round cap would leave a dot at none */}
      {share > 0 && <circle className="arc" cx="7" cy="7" r={r} strokeDasharray={`${share * c} ${c}`} />}
    </svg>
  );
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
