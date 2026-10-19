import { useEffect, useState } from "react";
import { App, type AIEgress } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { bytes, speed, fmtDelay, delayClass } from "../format";
import { setMode, openProxyGroup } from "../actions";
import { useGroups } from "../useGroups";
import { useConnectionStore } from "../connectionStore";
import { clientOf } from "../components/TrafficClients";
import { Sparkline } from "../components/Sparkline";
import { ConnectivityCards } from "../components/ConnectivityCards";
import { AIChecks } from "../components/AIChecks";
import { Segmented } from "../components/Segmented";
import { Info, Play, Pause, Route, Globe, Lines, Nodes, Arrow, Chevron, Wifi, Star, Target, Refresh } from "../components/Icons";
import { Stat, useToday } from "../views/Overview";
import { errText, toastError } from "../components/Toast";

export function MobileHome() {
  const t = useT();
  const state = useStore(s => s.state);
  const traffic = useStore(s => s.traffic);
  const samples = useStore(s => s.history);
  const reset = useStore(s => s.networkReset);
  const aiEnabled = useStore(s => !!s.settings?.aiServices?.length);
  const navigate = useStore(s => s.setView);
  const connections = useConnectionStore(s => s.snapshot.active);
  const { groups } = useGroups();
  const today = useToday();
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const [uptime, setUptime] = useState(0);
  const [exit, setExit] = useState<AIEgress | null>(null);
  const core = state?.core ?? "stopped";
  const running = core === "running";
  const busy = pending || !!state?.busy || core === "starting" || core === "stopping";
  const group = groups?.find(g => state?.mode === "global" ? g.name === "GLOBAL" : g.name !== "GLOBAL" && !!g.now);
  const node = state?.mode === "direct" ? "DIRECT" : group?.now ?? (groups ? "DIRECT" : "");
  // A nested selector can change its leaf without changing the main group's name.
  const selections = groups?.map(g => g.name + ":" + g.now).join("|");
  useEffect(() => {
    let alive = true;
    setExit(null);
    if (!running || !node || state?.busy) return;
    const read = () => App.NodeEgress(node).then(e => { if (alive) setExit(e); }).catch(() => { if (alive) setExit(null); });
    void read();
    const timer = setInterval(() => { if (!document.hidden) void read(); }, 180000);
    return () => { alive = false; clearInterval(timer); };
  }, [running, node, selections, state?.profile, state?.busy, reset]);
  useEffect(() => {
    if (!running) { setUptime(0); return; }
    const tick = () => setUptime(window.clashcubeAndroid?.uptimeSeconds() ?? 0);
    tick(); const timer = setInterval(tick, 1000);
    return () => clearInterval(timer);
  }, [running]);
  useEffect(() => { if (running) setError(""); }, [running]);
  const turn = async () => {
    setError(""); setPending(true);
    try { await (running ? App.Stop() : App.Start()); }
    catch (e) { const message = errText(e); setError(/permission|consent|denied|refused/i.test(message) ? t("Allow the VPN connection to turn on") : message); toastError(e); }
    finally { setPending(false); }
  };
  const title = core === "stopping" || (pending && running) ? "Turning off…" : core === "starting" || (pending && !running) ? "Turning on…" : core === "crashed" ? "The core quit unexpectedly" : running ? "Running" : "Stopped";
  const tone = busy ? "busy" : core === "crashed" ? "crashed" : running ? "running" : "stopped";
  const apps = new Set(connections.map(c => clientOf(c).key));
  const coreError = state?.coreError === "Android core service stopped or died"
    ? t("The core service stopped or was killed by the system") : state?.coreError;
  const problem = error || (core === "crashed" ? coreError : "") || (!state?.profile ? t("Import a profile in Profiles first") : "");
  const duration = [Math.floor(uptime / 3600), Math.floor(uptime / 60) % 60, uptime % 60].map(n => String(n).padStart(2, "0")).join(":");
  const summary = groups?.filter(g => g.name !== "GLOBAL" && ["Selector", "URLTest"].includes(g.type)) ?? [];
  const rate = (n: number) => { const [digits, unit] = speed(n).split(" "); return <>{digits}<small> {unit}</small></>; };
  return <div className="mobile-home">
    <section className="card mobile-status-card">
      <h2 className="mobile-card-heading"><Info />{t("Status")}<span className={"mobile-status-chip " + tone}>● {t(title)}</span></h2>
      {problem && <p className="mobile-hero-sub error">{problem}</p>}
      <div className="mobile-exit-row">
        {running && exit?.ip && <span className="mobile-country">{exit.loc || "—"}</span>}
        <div className="mobile-exit-info"><strong>{running ? exit?.ip || "—" : busy ? "—" : t("Not enabled")}</strong>
          {running ? <div className="mobile-exit-tags">
            {exit?.details?.kind && ["Residential", "Datacenter", "Business", "Mobile", "ISP"].includes(exit.details.kind) && <span>{t(exit.details.kind)}</span>}
            {exit?.details?.operator && <span>{exit.details.operator}</span>}
          </div> : !busy && <small>{t("When on, every app's traffic goes through ClashCube")}</small>}
        </div>
        <button className="mobile-power" disabled={busy || !state?.profile} onClick={turn} aria-label={t(busy ? title : running ? "Turn off" : core === "crashed" ? "Turn on again" : "Turn on")}>
          {busy ? <span className="spin-svg mobile-power-spinner"><Refresh size={28} /></span> : running ? <Pause size={28} /> : <Play size={28} />}
        </button>
      </div>
      <div className="mobile-status-footer"><div><small>{t("Run time")}</small><b>{duration}</b></div><button onClick={() => navigate("profiles")}><span><small>{t("Profile")}</small><b>{state?.profileName || t("No profile")}</b></span><Chevron /></button></div>
    </section>
    <div className="mobile-speed-tiles">{(["up", "down"] as const).map(dir => <section key={dir} className={"card mobile-speed-tile " + dir}>
      <h2 className="mobile-card-heading"><Arrow dir={dir} />{t(dir === "up" ? "Upload" : "Download")}</h2>
      <div className="mobile-speed-value">{running ? rate(traffic[dir]) : "—"}</div>
      <small>{t("Total")} {running ? bytes(dir === "up" ? traffic.upTotal : traffic.downTotal) : "—"}</small>
      <div className="mobile-speed-chart">{running && <Sparkline data={dir === "up" ? samples.map(s => ({ ...s, down: s.up, up: 0 })) : samples} height={48} showUp={false} tooltip={false} />}</div>
    </section>)}</div>
    <section className="card mobile-mode-card">
      <h2 className="mobile-card-heading"><Route />{t("Outbound Mode")}</h2>
      <Segmented className="track mobile-mode" value={state?.mode ?? "rule"} onChange={setMode} options={[{ value: "rule", label: t("Rule"), icon: <Lines /> }, { value: "global", label: t("Global"), icon: <Globe /> }, { value: "direct", label: t("Direct"), icon: <Target /> }]} />
    </section>
    <section className="card mobile-group-card">
      <button className="mobile-card-heading" onClick={() => navigate("proxies")}><Nodes />{t("Proxy group summary")}<small>{summary.length}</small><Chevron /></button>
      {summary.slice(0, 3).map(g => { const delay = g.members?.find(m => m.name === g.now)?.delay ?? 0; return <button key={g.name} className="mobile-group-row" onClick={() => openProxyGroup(g.name)}><span className="mobile-group-avatar">{Array.from(g.name)[0]}</span><span className="mobile-group-node"><small>{g.name}</small><b>{g.now || "—"}</b></span><span className={"delay " + delayClass(delay)}>{fmtDelay(delay, t)}</span></button>; })}
      {!summary.length && <p className="sub">{t(running ? "No proxy groups" : "Turn on to see proxy groups")}</p>}
    </section>
    <div className="mobile-stats">
      <Stat label={t("Today")} value={today ? bytes(today.up + today.down) : undefined} onClick={() => navigate("usage")} />
      <Stat label={t("Connections")} value={running ? connections.length + " · " + t(apps.size === 1 ? "{n} app" : "{n} apps", { n: apps.size }) : undefined} onClick={() => navigate("connections")} />
    </div>
    <section className="card mobile-check-card"><ConnectivityCards heading={<span className="mobile-card-title"><Wifi />{t("Network checks")}</span>} /></section>
    {aiEnabled && <section className="card mobile-check-card"><AIChecks icon={<Star />} /></section>}
  </div>;
}
