import { useStore } from "../store";
import { useT } from "../i18n";
import { bytes, speed } from "../format";
import { Sparkline } from "../components/Sparkline";
import { ConnectivityCards } from "../components/ConnectivityCards";
import { Segmented } from "../components/Segmented";
import { Switch } from "../components/Switch";
import { Arrow, Globe, Shield, File, Chevron } from "../components/Icons";
import { setMode, setSystemProxy, setTun, startCore, restartCore } from "../actions";

export function Overview() {
  const t = useT();
  const state = useStore((s) => s.state);
  const traffic = useStore((s) => s.traffic);
  const history = useStore((s) => s.history);
  const memory = useStore((s) => s.memory);
  const setView = useStore((s) => s.setView);
  const running = state?.core === "running";

  return (
    <div className="view overview">
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

      <div className="card traffic-card">
        <div className="traffic-head">
          <div className="rate up"><Arrow dir="up" size={13} /><span className="num">{speed(traffic.up)}</span><span className="lbl">{t("Upload")}</span></div>
          <div className="rate down"><Arrow dir="down" size={13} /><span className="num">{speed(traffic.down)}</span><span className="lbl">{t("Download")}</span></div>
          <div className="grow" />
          {!running && state?.core !== "starting" && <button className="btn small primary" onClick={startCore}>{t("Start core")}</button>}
        </div>
        <Sparkline data={history} height={96} />
      </div>

      <ConnectivityCards />

      <div className="stats">
        <Stat label={t("Total") + " ↑"} value={bytes(traffic.upTotal)} />
        <Stat label={t("Total") + " ↓"} value={bytes(traffic.downTotal)} />
        <Stat label={t("Memory")} value={bytes(memory)} />
        <Stat label={t("Mode")} value={t(cap(state?.mode ?? "rule"))} />
      </div>

      <div className="section-title">{t("Outbound Mode")}</div>
      <Segmented
          className="track fill mode-seg"
          value={state?.mode ?? "rule"}
          onChange={setMode}
          options={[{ value: "rule", label: t("Rule") }, { value: "global", label: t("Global") }, { value: "direct", label: t("Direct") }]}
        />

      <div className="list">
        <div className="row">
          <span className="ic"><Globe /></span>
          <div className="who"><div className="name">{t("System Proxy")}</div><div className="sub">{t("Route apps that respect the macOS proxy settings")} · 127.0.0.1:{state?.mixedPort}</div></div>
          <Switch on={!!state?.systemProxy} onChange={setSystemProxy} label={t("System Proxy")} />
        </div>
        <div className="row">
          <span className="ic"><Shield /></span>
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

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="card stat">
      <div className="lbl">{label}</div>
      <div className="val">{value}</div>
    </div>
  );
}
