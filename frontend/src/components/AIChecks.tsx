import { useEffect, useRef, useState } from "react";
import { App, type AICheck, type AIEgress, type AIHost, type AIRoute } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { flagged, maskedIP, nodeLabel } from "../format";
import { ExternalLink, Eye, Refresh, Shield } from "./Icons";
import { Fold } from "./Fold";
import { Popover } from "./Popover";
import { route } from "./ConnectivityCards";
import { AI_SERVICES } from "../aiServices";
import { AIServiceName } from "./AIServiceName";
import { openSettings } from "../actions";

type Lookup<T> = T | "loading" | "failed";

const statusLabel: Record<string, string> = {
  consistent: "Consistent", direct: "Direct", partlyDirect: "Partly direct", split: "Split",
  unsupported: "Unsupported region", refused: "Refused", failed: "Failed",
};
// what the address looks like to a service weighing it for abuse
const kindClass: Record<string, string> = { Residential: "good", Mobile: "good", Datacenter: "warn", Business: "muted" };

// Results outlive Overview, so returning to it doesn't check again. Each is
// kept for the mode, profile and network it was made on, for a while.
const TTL = 3 * 60_000;
const cache = new Map<string, { at: number; result: AICheck }>();

// Whether every name an AI service's apps talk to leaves by the same
// address, and whether the service serves that address's region.
export function AIChecks() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const systemProxyOnly = useStore((s) => !!s.state?.systemProxy && !s.state?.tun);
  const mode = useStore((s) => s.state?.mode);
  const profile = useStore((s) => s.state?.profile);
  const reset = useStore((s) => s.networkReset);
  const selected = useStore((s) => s.settings?.aiServices);
  const savingData = useStore((s) => s.state?.network.savingData ?? false);
  const services = AI_SERVICES.filter((service) => selected?.includes(service));
  const selectionKey = JSON.stringify(services);
  const scope = JSON.stringify([mode, profile, reset]);
  const [results, setResults] = useState<Record<string, Lookup<AICheck>>>({});
  const [open, setOpen] = useState<string | null>(null);
  const [hidden, setHidden] = useState(() => {
    try { return localStorage.getItem("ai.hideIPs") === "true"; } catch { return false; }
  });
  const toggleHidden = () => setHidden((old) => {
    try { localStorage.setItem("ai.hideIPs", String(!old)); } catch {}
    return !old;
  });
  // the request each service waits for; cleared to drop replies for a
  // previous selection, scope or an unmounted Overview
  const seq = useRef(0);
  const pending = useRef<Record<string, number>>({});
  const busy = Object.values(results).includes("loading");
  const anyIP = Object.values(results).some((r) => typeof r === "object" && r.egress?.some((e) => e.ip));

  const check = (list: string[], force: boolean) => {
    if (!running || list.length === 0) return;
    setResults((r) => ({ ...r, ...Object.fromEntries(list.map((s) => [s, "loading" as const])) }));
    for (const s of list) {
      const n = pending.current[s] = ++seq.current;
      App.AICheck(s, force)
        .then((r) => {
          // a check that routed nothing, as while the core starts, is retried
          if (r.status !== "failed") cache.set(s + scope, { at: Date.now(), result: r });
          if (pending.current[s] === n) setResults((p) => ({ ...p, [s]: r }));
        })
        .catch(() => { if (pending.current[s] === n) setResults((p) => ({ ...p, [s]: "failed" })); });
    }
  };

  useEffect(() => {
    const fresh: Record<string, AICheck> = {};
    for (const s of services) {
      const c = cache.get(s + scope);
      if (c && Date.now() - c.at < TTL) fresh[s] = c.result;
    }
    setResults(fresh);
    // on a metered network only what is asked for is checked
    if (!savingData) check(services.filter((s) => !fresh[s]), false);
    return () => { pending.current = {}; };
  }, [running, scope, selectionKey, savingData]);

  if (services.length === 0) return null;

  return (
    <>
      <div className="section-title section-head">
        <span className="ai-head-title">{t("AI services")}{systemProxyOnly && <LeakHint />}</span>
        <span className="ai-head-actions">
          {anyIP && <button className="icon" title={t(hidden ? "Show IP address" : "Hide IP address")} aria-label={t(hidden ? "Show IP address" : "Hide IP address")} aria-pressed={hidden} onClick={toggleHidden}><Eye size={14} off={hidden} /></button>}
          <button className={"icon" + (busy ? " spin" : "")} title={t("Refresh routes, egress IP and IP attributes")} disabled={!running || busy} onClick={() => check(services, true)}><Refresh size={13} /></button>
        </span>
      </div>
      <div className="list ai-checks">
        {services.map((s) => (
          <ServiceRow key={s} service={s} r={results[s]} running={running} hidden={hidden}
            open={open === s} onOpen={() => setOpen(open === s ? null : s)} onCheck={() => check([s], true)} />
        ))}
      </div>
    </>
  );
}

// With only the system proxy, what it can't carry leaks. A quiet shield by
// the title says so; the explanation and the way out wait behind a click.
function LeakHint() {
  const t = useT();
  const anchor = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  return <>
    <button ref={anchor} className="icon ai-leak-btn" title={t("Your real IP may leak")} aria-label={t("Your real IP may leak")} aria-expanded={open} onClick={() => setOpen(!open)}>
      <Shield size={13} />
    </button>
    <Popover anchor={anchor.current} open={open} onClose={() => setOpen(false)} width={300}>
      <div className="ai-leak-pop">
        <h3><Shield size={13} />{t("Your real IP may leak")}</h3>
        <p>{t("WebRTC sends UDP, which the system proxy doesn't carry, so websites can see your real IP. Apps that ignore the proxy look up names with the system's DNS.")}</p>
        <button className="btn small" onClick={() => { setOpen(false); openSettings("tun", "leak-protection"); }}>{t("Leak Protection")}<ExternalLink size={11} /></button>
      </div>
    </Popover>
  </>;
}

function ServiceRow({ service, r, running, open, onOpen, hidden, onCheck }: {
  service: string; r?: Lookup<AICheck>; running: boolean; open: boolean; onOpen: () => void; hidden: boolean; onCheck: () => void;
}) {
  const t = useT();
  const done = typeof r === "object";
  const result = done ? r.route : undefined;
  const egress = done ? r.egress ?? [] : [];
  const hosts = result?.hosts ?? [];
  const routed = hosts.filter((h) => !h.error && h.chain?.length && !isRefused(h));
  const off = routed.filter((h) => h.chain![0] !== result?.node);
  const direct = routed.filter((h) => h.chain![0] === "DIRECT");
  const auto = result?.auto ?? [];
  const status = done ? r.status : "";

  let sub: React.ReactNode = !running ? t("The core isn't running") : r === "failed" ? t("Failed") : r === "loading" ? t("Checking routes…") : t("Not checked");
  if (result) {
    const node = nodeLabel(result.node);
    switch (result.verdict) {
      case "consistent":
        sub = t("All {n} names leave by {node}", { n: routed.length, node });
        break;
      case "split":
        sub = status === "consistent"
          ? t("{n} names leave by {k} nodes that share one egress IP", { n: routed.length, k: egress.length })
          : t("{n} of {total} names leave by another node than {node}", { n: off.length, total: routed.length, node });
        break;
      case "direct": {
        // proxied names that also leave by another node than most
        const elsewhere = result.node === "DIRECT" ? 0 : off.length - direct.length;
        sub = direct.length === routed.length
          ? t("Every name goes DIRECT: the service sees this Mac's address")
          : status === "consistent"
            ? t("{n} names leave by {k} nodes that share one egress IP", { n: routed.length, k: egress.length })
            : t(direct.length === 1 ? "{n} name goes DIRECT and shows this Mac's address" : "{n} names go DIRECT and show this Mac's address", { n: direct.length })
              + (elsewhere > 0 ? t("; {n} more leave by another node than {node}", { n: elsewhere, node }) : "");
        break;
      }
      case "refused":
        sub = t("Every name is refused");
        break;
      default:
        sub = t("No name could be routed");
    }
  }
  const warn = ["partlyDirect", "split", "refused"].includes(status);
  const unsupported = egress.some((e) => e.unsupported);

  return (
    <div className={"ai-service" + (open ? " open" : "")}>
      <div className={"row" + (done ? " click" : "")} onClick={done ? onOpen : undefined}>
        <div className="who">
          <div className="name"><AIServiceName service={service} /></div>
          <div className={"sub" + (warn ? " warn" : "")}>{sub}</div>
          {egress.length > 0 && <div className="ai-egress">
            {egress.map((e) => <EgressChip key={e.node} e={e} service={service} multi={egress.length > 1} hidden={hidden} />)}
            {unsupported && <span className="err">{t("{service} doesn't serve this region", { service })}</span>}
          </div>}
        </div>
        <div className="end" onClick={(ev) => ev.stopPropagation()}>
          {auto.length > 0 && <span className="badge muted" title={t("{groups} pick a node by themselves; the route may change", { groups: auto.join(", ") })}>{t("Auto")}</span>}
          {done
            ? <span className={"badge " + (r.level || "muted")}>{t(statusLabel[status] ?? status)}</span>
            : r === undefined && running && <button className="btn small" onClick={onCheck}>{t("Check now")}</button>}
        </div>
      </div>
      <Fold open={open && done}>
        {done && <EgressDetails egress={egress} hidden={hidden} />}
        {result && <AIRouteDetails result={result} />}
        <div className="ai-hosts">
          <div className="note stagger" style={{ ["--i" as string]: hosts.length }}>
            {done && r.nodeEgress
              ? t("This service's domains don't report an egress IP, so each node's is asked of Cloudflare through it. A node that routes by destination may show the service another address, and region availability isn't checked.")
              : t("Each check queries the service through the core for its egress IP. The check comes from ClashCube, so PROCESS-NAME rules apply only to the real apps.")}
          </div>
        </div>
      </Fold>
    </div>
  );
}

// the address one node shows the service, with the kind of network it is in
function EgressChip({ e, service, multi, hidden }: { e: AIEgress; service: string; multi: boolean; hidden: boolean }) {
  const t = useT();
  const kind = e.details?.kind;
  const title = e.unsupported ? t("{service} doesn't serve this region", { service })
    : e.chain?.length ? [...e.chain].reverse().map(nodeLabel).join(" → ") : undefined;
  return (
    <span className={"ai-ip" + (e.unsupported ? " bad" : e.ip ? "" : " muted")} title={title}>
      {multi && <span className="ai-ip-node">{nodeLabel(e.node)}</span>}
      {e.ip ? <span className="mono">{flagged(hidden ? maskedIP(e.ip) : e.ip, e.loc)}</span> : t("Failed")}
      {kind && kind !== "Unknown" && <span className={"ai-kind " + (kindClass[kind] ?? "muted")}>{t(kind)}</span>}
    </span>
  );
}

function EgressDetails({ egress, hidden }: { egress: AIEgress[]; hidden: boolean }) {
  const t = useT();
  const known = egress.filter((e) => e.ip);
  if (known.length === 0) return null;
  return (
    <div className="ai-hosts ai-egress-details" title={t("IP attributes provided by Net.Coffee")}>
      {known.map((e, i) => (
        <div key={e.node} className="stagger" style={{ ["--i" as string]: i }}>
          {egress.length > 1 && <span className="ai-egress-node">{nodeLabel(e.node)}</span>}
          {e.details ? <>
            {t("IP type")}: {t(e.details.kind)} · {t("City")}: {[e.details.city, e.details.region].filter(Boolean).join(", ") || t("Unknown")}
            {" · "}{t("ISP")}: {e.details.operator || t("Unknown")}{e.details.asn > 0 && ` (AS${e.details.asn})`}
            {e.details.network && <> · {t("Network data: {cidr}", { cidr: hidden ? maskedIP(e.details.network) : e.details.network })}</>}
          </> : t("IP attributes unavailable")}
        </div>
      ))}
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
