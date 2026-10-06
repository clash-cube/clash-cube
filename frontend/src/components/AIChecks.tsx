import { useEffect, useRef, useState } from "react";
import { App, type AIEgress, type AIHost, type AIRoute } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { flagged, maskedIP } from "../format";
import { Close, ExternalLink, Eye, Refresh, Shield } from "./Icons";
import { Fold } from "./Fold";
import { route } from "./ConnectivityCards";
import { AI_SERVICES } from "../aiServices";
import { AIServiceName } from "./AIServiceName";
import { openSettings } from "../actions";

type Lookup<T> = T | "loading" | "failed";

const verdictClass: Record<string, string> = { consistent: "good", split: "warn", direct: "bad", refused: "warn", failed: "muted" };
const verdictLabel: Record<string, string> = {
  consistent: "Consistent", split: "Split", direct: "Leaks", refused: "Refused", failed: "Failed",
};

// Whether every name an AI service's apps talk to leaves by the same
// node. Each check also asks the service for its observed egress address.
export function AIChecks() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const systemProxyOnly = useStore((s) => !!s.state?.systemProxy && !s.state?.tun);
  const hintDismissed = useStore((s) => s.aiLeakHintDismissed);
  const mode = useStore((s) => s.state?.mode);
  const profile = useStore((s) => s.state?.profile);
  const reset = useStore((s) => s.networkReset);
  const enabled = useStore((s) => s.settings?.aiChecks ?? false);
  const selected = useStore((s) => s.settings?.aiServices);
  const savingData = useStore((s) => s.state?.network.savingData ?? false);
  const services = AI_SERVICES.filter((service) => selected?.includes(service));
  const selectionKey = JSON.stringify(services);
  const [routes, setRoutes] = useState<Record<string, Lookup<AIRoute>>>({});
  const [egress, setEgress] = useState<Record<string, Lookup<AIEgress>>>({});
  const [open, setOpen] = useState<string | null>(null);
  const [hidden, setHidden] = useState(() => {
    try { return localStorage.getItem("ai.hideIPs") === "true"; } catch { return false; }
  });
  const toggleHidden = () => setHidden((old) => {
    try { localStorage.setItem("ai.hideIPs", String(!old)); } catch {}
    return !old;
  });
  const seq = useRef(0);
  const busy = Object.values(routes).includes("loading") || Object.values(egress).includes("loading");

  const check = (force = false) => {
    if (!enabled || !running) return;
    const n = ++seq.current;
    setRoutes({});
    setEgress({});
    for (const s of services) {
      setRoutes((r) => ({ ...r, [s]: "loading" }));
      setEgress((e) => ({ ...e, [s]: "loading" }));
      App.AIRoutes(s)
        .then((r) => { if (n === seq.current) setRoutes((p) => ({ ...p, [s]: r })); })
        .catch(() => { if (n === seq.current) setRoutes((p) => ({ ...p, [s]: "failed" })); });
      App.AIEgress(s, force)
        .then((r) => { if (n === seq.current) setEgress((e) => ({ ...e, [s]: r })); })
        .catch(() => { if (n === seq.current) setEgress((e) => ({ ...e, [s]: "failed" })); });
    }
  };

  useEffect(() => {
    seq.current++;
    setRoutes({});
    setEgress({});
    // on a metered network only what is asked for is checked
    if (enabled && running && !savingData) check();
    // Ignore replies from a previous selection or an unmounted Overview.
    return () => { seq.current++; };
  }, [running, mode, profile, reset, enabled, selectionKey, savingData]);

  if (!enabled || services.length === 0) return null;

  return (
    <>
      <div className="section-title section-head">
        <span>{t("AI services")}</span>
        <button className={"icon" + (busy ? " spin" : "")} title={t("Refresh routes, egress IP and IP attributes")} disabled={!running || busy} onClick={() => check(true)}><Refresh size={13} /></button>
      </div>
      {systemProxyOnly && !hintDismissed && <div className="banner warn ai-leak">
        <Shield size={15} />
        <div className="grow">{t("WebRTC sends UDP, which the system proxy doesn't carry, so websites can see your real IP. Apps that ignore the proxy look up names with the system's DNS.")}</div>
        <button className="btn small" onClick={() => openSettings("tun", "leak-protection")}>{t("Leak Protection")}<ExternalLink size={11} /></button>
        <button className="icon" title={t("Dismiss until next launch")} aria-label={t("Dismiss until next launch")} onClick={() => useStore.setState({ aiLeakHintDismissed: true })}><Close size={11} /></button>
      </div>}
      <div className="list ai-checks">
        {services.map((s) => (
          <ServiceRow key={s} service={s} r={routes[s]} e={egress[s]} running={running}
            hidden={hidden} onToggleHidden={toggleHidden}
            open={open === s} onOpen={() => setOpen(open === s ? null : s)} />
        ))}
      </div>
    </>
  );
}

function ServiceRow({ service, r, e, running, open, onOpen, hidden, onToggleHidden }: {
  service: string; r?: Lookup<AIRoute>; e?: Lookup<AIEgress>; running: boolean; open: boolean; onOpen: () => void;
  hidden: boolean; onToggleHidden: () => void;
}) {
  const t = useT();
  const done = typeof r === "object";
  const hosts = done ? r.hosts ?? [] : [];
  const routed = hosts.filter((h) => !h.error && h.chain?.length && !isRefused(h));
  const off = routed.filter((h) => h.chain![0] !== (r as AIRoute).node);
  const direct = routed.filter((h) => h.chain![0] === "DIRECT");
  const auto = done ? r.auto ?? [] : [];

  let sub: React.ReactNode = !running ? t("The core isn't running") : r === "failed" ? t("Failed") : r === "loading" ? t("Checking routes…") : t("Not checked");
  let warn = false;
  if (done) {
    const node = r.node;
    switch (r.verdict) {
      case "consistent":
        sub = t("All {n} names leave by {node}", { n: routed.length, node: node });
        break;
      case "split":
        warn = true;
        sub = t("{n} of {total} names leave by another node than {node}", { n: off.length, total: routed.length, node });
        break;
      case "direct": {
        warn = true;
        // proxied names that also leave by another node than most
        const elsewhere = node === "DIRECT" ? 0 : off.length - direct.length;
        sub = direct.length === routed.length
          ? t("Every name goes DIRECT: the service sees this Mac's address")
          : t(direct.length === 1 ? "{n} name goes DIRECT and shows this Mac's address" : "{n} names go DIRECT and show this Mac's address", { n: direct.length })
            + (elsewhere > 0 ? t("; {n} more leave by another node than {node}", { n: elsewhere, node }) : "");
        break;
      }
      case "refused":
        warn = true;
        sub = t("Every name is refused");
        break;
      default:
        sub = t("No name could be routed");
    }
  }

  let egressLine: React.ReactNode = null;
  if (e === "loading") egressLine = t("Looking up…");
  else if (e === "failed") egressLine = <span className="err">{t("Failed")}</span>;
  else if (e) egressLine = <>
    {t("Egress")} <span className="ai-ip"><span className="mono">{flagged(hidden ? maskedIP(e.ip) : e.ip, e.loc)}</span>
      <button className="icon" title={t(hidden ? "Show IP address" : "Hide IP address")} aria-label={t(hidden ? "Show IP address" : "Hide IP address")} aria-pressed={hidden}
        onClick={(event) => { event.stopPropagation(); onToggleHidden(); }}><Eye size={14} off={hidden} /></button>
    </span>
    {e.unsupported && <span className="err"> · {t("{service} doesn't serve this region", { service })}</span>}
  </>;

  return (
    <div className={"ai-service" + (open ? " open" : "")}>
      <div className="row click" onClick={onOpen}>
        <div className="who">
          <div className="name"><AIServiceName service={service} /></div>
          <div className={"sub" + (warn ? " warn" : "")}>{sub}</div>
          {egressLine && <div className="sub">{egressLine}</div>}
          {e && typeof e === "object" && <div className="sub wrap" title={t("IP attributes provided by Net.Coffee")}>{e.details ? <>
            {t("IP type")}: {t(e.details.kind)} · {t("City")}: {[e.details.city, e.details.region].filter(Boolean).join(", ") || t("Unknown")}
            {" · "}{t("ISP")}: {e.details.operator || t("Unknown")}{e.details.asn > 0 && ` (AS${e.details.asn})`}
            {e.details.network && <> · {t("Network data: {cidr}", { cidr: hidden ? maskedIP(e.details.network) : e.details.network })}</>}
          </> : t("IP attributes unavailable")}</div>}
        </div>
        <div className="end" onClick={(ev) => ev.stopPropagation()}>
          {auto.length > 0 && <span className="badge muted" title={t("{groups} pick a node by themselves; the route may change", { groups: auto.join(", ") })}>{t("Auto")}</span>}
          {done && <span className={"badge " + (verdictClass[r.verdict] ?? "muted")}>{t(verdictLabel[r.verdict] ?? r.verdict)}</span>}
        </div>
      </div>
      <Fold open={open && done}>
        {done && <AIRouteDetails result={r} />}
        <div className="ai-hosts">
          <div className="note stagger" style={{ ["--i" as string]: hosts.length }}>
            {t("Each check queries the service through the core for its egress IP. The check comes from ClashCube, so PROCESS-NAME rules apply only to the real apps.")}
          </div>
        </div>
      </Fold>
    </div>
  );
}

const isRefused = (h: AIHost) => h.chain?.[0] === "REJECT" || h.chain?.[0] === "REJECT-DROP";

export function AIRouteDetails({ result }: { result: AIRoute }) {
  const t = useT();
  return (
    <div className="ai-hosts">
      <div className="ai-host muted"><span>{t("Domain")}</span><span>{t("Route")}</span><span>{t("Rule")}</span></div>
      {(result.hosts ?? []).map((h, i) => <HostLine key={h.host} h={h} node={result.node} i={i} />)}
      <div className="note">{t("Checks representative domains over TCP 443. Subdomains may differ; UDP, process and IP/ASN rules are not independently verified.")}</div>
    </div>
  );
}

function HostLine({ h, node, i }: { h: AIHost; node: string; i: number }) {
  const t = useT();
  const chain = h.chain ?? [];
  const cls = h.error ? "fail" : isRefused(h) ? "refused" : chain[0] === "DIRECT" ? "bad" : chain[0] !== node ? "warn" : "";
  return (
    <div className={"ai-host stagger " + cls} style={{ ["--i" as string]: i }}>
      <span className="mono host">{h.host}</span>
      <span className="via" title={chain.length ? [...chain].reverse().join(" → ") : h.error}>{h.error ? t("Failed") : route(chain)}</span>
      <span className="mono rule" title={[h.rule, h.rulePayload].filter(Boolean).join(" ")}>{h.rule}{h.rulePayload && ` ${h.rulePayload}`}</span>
    </div>
  );
}
