import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useLeaving } from "../../useLeaving";
import { useFlip } from "../../useFlip";
import { create } from "zustand";
import { App } from "../../api";
import { useStore } from "../../store";
import { useT } from "../../i18n";
import { openSettings, setMode } from "../../actions";
import { delayClass } from "../../format";
import type { Module, Tailscale } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import { Fold } from "../Fold";
import { Bolt, Chevron, Copy, Search } from "../Icons";
import { Switch } from "../Switch";
import { toast, toastError } from "../Toast";
import { asYAML, FormOrYAML, type FormProps } from "./shared";
import type { ModuleKind } from "./kinds";

const proxyName = (t: Tailscale) => `ClashCube Tailscale ${t.identity}`;
const stateText: Record<string, string> = {
  Starting: "Connecting…", Running: "Connected", NeedsLogin: "Login required",
  NeedsMachineAuth: "Device approval required", Stopped: "Logged out", Failed: "Connection failed",
  NoState: "Connecting…", InUseOtherUser: "Identity in use",
};
const pathText: Record<string, string> = { direct: "Direct", relay: "Relayed" };
const HOSTNAME = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/;

function useTailnet(ts: Tailscale) {
  const status = useStore((s) => s.state?.tailscales?.find((n) => n.name === proxyName(ts)));
  return useMemo(() => status && { ...status, ips: status.ips ?? [], health: status.health ?? [], peers: (status.peers ?? []).map((p) => ({ ...p, ips: p.ips ?? [], routes: p.routes ?? [] })) }, [status]);
}
type Status = NonNullable<ReturnType<typeof useTailnet>>;
type Peer = Status["peers"][number];

// the connections a login was asked for, until its page opens. asked is
// false for one just added: its login is asked for once the core runs it.
// Outside the editor, so the row added goes on with it.
type Waiting = { since: number; asked: boolean };
const useLogins = create<{ waiting: Record<string, Waiting> }>(() => ({ waiting: {} }));
const awaitLogin = (identity: string, asked = true) => useLogins.setState((s) => ({ waiting: { ...s.waiting, [identity]: { since: Date.now(), asked } } }));
const stopWaiting = (identity: string) => useLogins.setState((s) => { const { [identity]: _, ...rest } = s.waiting; return { waiting: rest }; });

async function openLogin(url: string, t: ReturnType<typeof useT>) {
  const parsed = new URL(url);
  if (parsed.protocol !== "https:" || parsed.username || parsed.password) throw new Error(t("The login URL must use HTTPS."));
  await App.OpenURL(parsed.href);
}

// useLoginPage opens the login page once a login asked for brings one.
// An auth URL arriving on its own never opens a browser.
function useLoginPage(ts: Tailscale, status: Status | undefined) {
  const t = useT();
  const waiting = useLogins((s) => s.waiting[ts.identity]);
  useEffect(() => {
    if (!waiting) return;
    if (status?.state === "Running") { stopWaiting(ts.identity); return; }
    if (!waiting.asked) {
      // added: once the core runs it, or quietly not if it never does
      if (status?.state === "NeedsLogin" || status?.state === "Stopped") { login(ts).catch(toastError); return; }
      const timeout = setTimeout(() => stopWaiting(ts.identity), Math.max(0, waiting.since + 30000 - Date.now()));
      return () => clearTimeout(timeout);
    }
    if (status?.authURL) {
      stopWaiting(ts.identity);
      openLogin(status.authURL, t).catch(toastError);
      return;
    }
    const timeout = setTimeout(() => { stopWaiting(ts.identity); toastError(t("Could not get a login link. Check the connection and try again.")); }, Math.max(0, waiting.since + 20000 - Date.now()));
    return () => clearTimeout(timeout);
  }, [waiting, status?.state, status?.authURL]);
}

async function login(ts: Tailscale) {
  awaitLogin(ts.identity);
  try { await App.TailscaleLogin(proxyName(ts)); } catch (e) { stopWaiting(ts.identity); throw e; }
}

function Summary({ m }: { m: Module }) {
  const t = useT();
  const ts = m.tailscale!;
  const status = useTailnet(ts);
  const core = useStore((s) => s.state?.core);
  useLoginPage(ts, status);
  const subnets = ts.routes?.length ?? 0;
  const label = status ? t(stateText[status.state] ?? status.state)
    : !m.enabled ? t("Off") : core !== "running" ? t("Core stopped") : t("Not connected");
  return <div className={"sub" + (status?.state === "Failed" ? " err" : "")}>{ts.hostname} · {label}
    {!!subnets && ` · ${t("{count} subnets", { count: subnets })}`}
    {!!status?.ips.length && <> · <span className="mono">{primaryAddress(status.ips)}</span></>}</div>;
}

const primaryAddress = (ips: string[]) => ips.find((ip) => !ip.includes(":")) ?? ips[0] ?? "";

// IPv4 CIDRs and addresses as [first, last]; other entries give null
function v4Range(s: string): [number, number] | null {
  const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)(?:\/(\d+))?$/.exec(s.trim());
  if (!m) return null;
  const n = m.slice(1, 5).reduce((a, x) => a * 256 + Number(x), 0);
  const bits = m[5] === undefined ? 32 : Number(m[5]);
  const size = 2 ** (32 - bits);
  const first = Math.floor(n / size) * size;
  return [first, first + size - 1];
}
const overlaps = (a: string, b: string) => {
  const x = v4Range(a), y = v4Range(b);
  return !!x && !!y && x[0] <= y[1] && y[0] <= x[1];
};
const TAILNET_V4 = "100.64.0.0/10";

// the system proxy's bypass entries that reach the tailnet or the subnets
function bypassed(bypass: string[], routes: string[], dnsSuffix: string) {
  const suffix = dnsSuffix.toLowerCase().replace(/\.$/, "");
  return bypass.filter((b) => {
    const host = b.toLowerCase().replace(/^\*?\./, "");
    if ([TAILNET_V4, ...routes].some((r) => overlaps(b, r))) return true;
    return host === "ts.net" || (!!suffix && (suffix === host || suffix.endsWith("." + host)));
  });
}

// Notice is the one thing in the way of reaching the tailnet, with what fixes it
function Notice({ routes, dnsSuffix }: { routes: string[]; dnsSuffix: string }) {
  const t = useT();
  const mode = useStore((s) => s.state?.mode);
  const tun = useStore((s) => s.state?.tun);
  const bypass = useStore((s) => s.settings?.bypass) ?? [];
  if (mode && mode !== "rule") return <div className="pick-note warn tailscale-notice">
    {t("Tailscale routing works in Rule mode only.")}
    <button type="button" className="link" onClick={() => setMode("rule")}>{t("Switch to Rule mode")}</button>
  </div>;
  const skipped = tun ? [] : bypassed(bypass, routes, dnsSuffix);
  if (skipped.length) return <div className="pick-note warn tailscale-notice">
    {t("The system proxy skips {items}, so browsers reach those addresses without Tailscale.", { items: skipped.join(", ") })}
    <button type="button" className="link" onClick={() => openSettings("network")}>{t("Bypass settings")}</button>
  </div>;
  return null;
}

async function copyText(value: string, t: ReturnType<typeof useT>) {
  try {
    if (await App.CopyText(value)) toast(t("Copied"));
    else toastError(t("Could not copy to clipboard"));
  } catch (e) { toastError(e); }
}

type Check = { ms: number; path: string };

function PeerRow({ ts, peer, leaving, added }: { ts: Tailscale; peer: Peer; leaving: boolean; added: boolean }) {
  const t = useT();
  const detailsID = useId();
  const [expanded, setExpanded] = useState(false);
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<Check | null>(null);
  const addresses = [
    ...(peer.dnsName ? [{ label: "MagicDNS", value: peer.dnsName }] : []),
    ...peer.ips.map((ip) => ({ label: ip.includes(":") ? "IPv6" : "IPv4", value: ip })),
  ];
  const copy = (value: string) => copyText(value, t);
  const check = async () => {
    setChecking(true);
    setResult(null);
    try {
      const r = await App.TailscalePing(proxyName(ts), primaryAddress(peer.ips));
      setResult({ ms: Math.max(1, Math.round(r.milliseconds)), path: r.path });
    } catch (e) { toastError(e); }
    finally { setChecking(false); }
  };
  const path = result?.path ?? peer.path;
  return <div data-key={peer.id} ref={(el) => { if (el) el.inert = leaving; }} className={"tailscale-peer" + (leaving ? " motion-d-leaving" : added ? " motion-d-row-in" : "")} role="listitem">
    <div className="row tailscale-peer-row">
      <button type="button" className="tailscale-peer-toggle" aria-expanded={expanded} aria-controls={detailsID} onClick={() => setExpanded(!expanded)}>
        <span className={"tailscale-peer-dot" + (peer.online ? " online" : "")} aria-hidden="true" />
        <span className="who"><span className="name" title={peer.name}>{peer.name}</span>
          <span className="sub mono">{primaryAddress(peer.ips) || peer.dnsName || t("No address")}</span>
        </span>
        <span className="tailscale-peer-state"><StatusText value={!peer.online ? t("Offline") : t(pathText[path] ?? "Online")} /></span>
        {result && <span className={"delay " + delayClass(result.ms)}>{result.ms} ms</span>}
        <Chevron size={10} className={"chev" + (expanded ? " open" : "")} />
      </button>
      <button type="button" className={"icon" + (checking ? " zap" : "")} title={t("Check connection")} aria-label={t("Check connection to {name}", { name: peer.name })}
        disabled={checking || !peer.online || !peer.ips.length} onClick={check}><Bolt size={14} /></button>
    </div>
    <Fold open={expanded}>
      <dl className="tailscale-peer-details" id={detailsID}>{addresses.map(({ label, value }) => <div key={value}>
        <dt>{t(label)}</dt><dd className="mono">{value}</dd>
        <button type="button" className="icon" title={t("Copy {field}", { field: label })} aria-label={t("Copy {field}", { field: label })} onClick={() => copy(value)}><Copy size={12} /></button>
      </div>)}
      {peer.routes.map((route) => <div key={route}><dt>{t("Subnet")}</dt><dd className="mono">{route}</dd></div>)}</dl>
    </Fold>
  </div>;
}

function StatusText({ value }: { value: string }) {
  const previous = useRef(value);
  const changed = useRef(false);
  if (previous.current !== value) { previous.current = value; changed.current = true; }
  return <span key={value} className={changed.current ? "motion-d-value" : undefined}>{value}</span>;
}

const SEARCH_FROM = 8;

function PeerList({ ts, peers }: { ts: Tailscale; peers: Peer[] }) {
  const t = useT();
  const [query, setQuery] = useState("");
  const search = peers.length > SEARCH_FROM ? query.trim().toLowerCase() : "";
  const shown = peers.filter((peer) => [peer.name, peer.dnsName, ...peer.ips].some((value) => value.toLowerCase().includes(search)))
    .sort((a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name));
  const rows = useLeaving(shown, (peer) => String(peer.id));
  const list = useRef<HTMLDivElement>(null);
  const initialIDs = useRef(new Set(peers.map((peer) => peer.id)));
  useEffect(() => { initialIDs.current = new Set(peers.map((peer) => peer.id)); }, [peers]);
  useFlip(list, JSON.stringify(rows.map(({ item, leaving }) => [item.id, leaving])), { enter: false });
  return <div className="field"><span className="label">{t("Devices")}</span><div className="scope">
    <div className="tailscale-peer-toolbar">
      <span className="pick-note">{t("{online} online · {total} devices", { online: peers.filter((peer) => peer.online).length, total: peers.length })}</span>
      {peers.length > SEARCH_FROM && <label className="search"><Search size={12} /><input aria-label={t("Search devices")} placeholder={t("Search devices")} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") e.preventDefault(); }} /></label>}
    </div>
    <div ref={list} className="tailscale-peers" role="list" aria-label={t("Devices")}>
      {rows.map(({ item: peer, leaving }) => <PeerRow key={peer.id} ts={ts} peer={peer} leaving={leaving} added={!initialIDs.current.has(peer.id)} />)}
      {!rows.length && <div className="tailscale-peers-empty">{t("No matching devices")}</div>}
    </div>
  </div></div>;
}

// Connection is what takes effect at once: logging in and out, the devices
function Connection({ module, status }: { module: Module; status: Status | undefined }) {
  const t = useT();
  const ts = module.tailscale!;
  const core = useStore((s) => s.state?.core);
  const waiting = useLogins((s) => !!s.waiting[ts.identity]);
  const [busy, setBusy] = useState(false);
  const [armed, setArmed] = useState(false);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);
  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try { await fn(); } catch (e) { toastError(e); }
    finally { setBusy(false); }
  };
  if (!status) return <div className="field"><span className="label">{t("Status")}</span><div className="scope">
    <div className="pick-note tailscale-line">{!module.id ? t("Add the module to connect.") : !module.enabled ? t("Turn the module on to connect.")
      : core !== "running" ? t("Start the core to connect.") : t("This module isn't in use by the running profile.")}</div>
  </div></div>;
  const state = status.state;
  const loggedIn = state === "Running" || state === "NeedsMachineAuth";
  const action = state === "Running" || state === "Starting" || state === "NoState" ? null
    : status.authURL && state !== "NeedsMachineAuth"
      ? <button className="btn small primary" type="button" disabled={busy} onClick={() => act(() => openLogin(status.authURL!, t))}>{t("Open login page")}</button>
      : state !== "NeedsMachineAuth" && <button className="btn small primary" type="button" disabled={busy || waiting} onClick={() => act(() => login(ts))}>{waiting ? t("Waiting for login link…") : t("Log in")}</button>;
  return <>
    <div className="field"><span className="label">{t("Status")}</span><div className="scope">
      <div className="tailscale-actions tailscale-line">
        <span className={"tailscale-peer-dot" + (state === "Running" ? " online" : "")} aria-hidden="true" />
        <span className={state === "Failed" ? "err" : ""}><StatusText value={t(stateText[state] ?? state)} /></span>
        {!!status.ips.length && <button type="button" className="tailscale-self mono" title={t("Copy {field}", { field: "IPv4" })}
          onClick={() => copyText(primaryAddress(status.ips), t)}>{primaryAddress(status.ips)}<Copy size={11} /></button>}
        <div className="grow" />
        {action}
        {loggedIn && (armed
          ? <button className="btn small danger armed" type="button" disabled={busy} onClick={() => { setArmed(false); act(() => App.TailscaleLogout(proxyName(ts))); }}>{t("Click again to log out")}</button>
          : <button className="btn small" type="button" disabled={busy} onClick={() => setArmed(true)}>{t("Log out")}</button>)}
      </div>
      {state === "NeedsMachineAuth" && <div className="pick-note">{t("Approve this device in your tailnet admin console.")}</div>}
      {status.error && <div className="pick-note err">{status.error}</div>}
      {status.health.map((h) => <div key={h} className="pick-note warn">{h}</div>)}
      <Notice routes={ts.routes ?? []} dnsSuffix={status.dnsSuffix ?? ""} />
    </div></div>
    <Fold open={!!status.peers.length}>
      <PeerList ts={ts} peers={status.peers} />
    </Fold>
  </>;
}

const parseRoutes = (text: string) => [...new Set(text.split(/[\s,]+/).filter(Boolean))];

type Form = { name: string; tailscale: Tailscale; extra: string };
function TailscaleEditor({ module, form, onCancel, onSave, onYAML }: FormProps<Form>) {
  const t = useT();
  const ts = module.tailscale!;
  const status = useTailnet(ts);
  const [name, setName] = useState(form?.name ?? module.name);
  const [config, setConfig] = useState<Tailscale>(form?.tailscale ?? ts);
  const discovered = [...new Map((status?.peers ?? []).flatMap((p) => p.routes.map((r) => [r, p.name] as const))).entries()].sort(([a], [b]) => a.localeCompare(b));
  const known = new Set(discovered.map(([r]) => r));
  const [picked, setPicked] = useState<string[]>(() => (config.routes ?? []).filter((r) => known.has(r)));
  const [extra, setExtra] = useState(form?.extra ?? (config.routes ?? []).filter((r) => !known.has(r)).join("\n"));
  const [advanced, setAdvanced] = useState(!!config.controlURL);
  const [busy, setBusy] = useState(false);
  const [local, setLocal] = useState<string[]>([]);
  useEffect(() => { let active = true; App.TailscaleLocalNetworks().then((nets) => { if (active) setLocal(nets ?? []); }).catch(() => {}); return () => { active = false; }; }, []);
  // a completed login (approval included) surfaces as a transition to Running
  const prevState = useRef(status?.state);
  useEffect(() => {
    const prev = prevState.current;
    prevState.current = status?.state;
    if (status?.state === "Running" && (prev === "NeedsLogin" || prev === "NeedsMachineAuth")) toast(t("Connected"));
  }, [status?.state]);

  // a subnet typed in before its device showed it stays where it was typed
  const typed = parseRoutes(extra);
  const isOn = (route: string) => picked.includes(route) || typed.includes(route);
  const routes = [...new Set([...picked, ...typed])];
  // the subnets devices show, and those picked from them that no device shows now
  const choices: [string, string | undefined][] = [...discovered, ...picked.filter((r) => !known.has(r)).map((r) => [r, undefined] as [string, undefined])];
  const out: Tailscale = { ...config, routes, controlURL: config.controlURL?.trim() || "" };
  const draft = { ...module, name: name.trim(), tailscale: out };
  const hostOK = HOSTNAME.test(config.hostname);
  const valid = !!name.trim() && hostOK;
  const reconnects = !!status && (out.hostname !== ts.hostname || out.controlURL !== (ts.controlURL ?? ""));
  const overlap = (route: string) => local.find((n) => overlaps(route, n));
  const extraOverlaps = typed.filter((r) => !known.has(r)).map((r) => [r, overlap(r)] as const).filter(([, n]) => n);
  const save = async () => {
    setBusy(true);
    try {
      if (!module.id) awaitLogin(out.identity, false);
      await onSave(draft);
    } catch (e) { toastError(e); }
    finally { setBusy(false); }
  };
  const toggle = (route: string, on: boolean) => {
    setPicked(on ? [...picked, route] : picked.filter((r) => r !== route));
    if (!on && typed.includes(route)) setExtra(typed.filter((r) => r !== route).join("\n"));
  };

  return <form className="module-editor route-editor tailscale-editor stagger" onSubmit={(e) => { e.preventDefault(); save(); }}>
    <Connection module={module} status={status} />
    <div className="tailscale-sep" />
    <div className="field"><span className="label">{t("Name")}</span><div className="scope">
      <input className="input" aria-label={t("Module name")} placeholder={t("Module name")} value={name} onChange={(e) => setName(e.target.value)} />
    </div></div>
    <div className="field"><span className="label">{t("Remote subnets")}</span><div className="scope">
      {choices.map(([route, from]) => {
        const lan = overlap(route);
        return <div className="tailscale-route" key={route}>
          <Switch label={route} on={isOn(route)} onChange={(on) => toggle(route, on)} />
          <span className="mono">{route}</span>
          <span className="pick-note">{from ? t("from {device}", { device: from }) : t("not advertised right now")}</span>
          {lan && (isOn(route)
            ? <span className="pick-note warn">{t("Devices on your network {network} are reached through Tailscale too", { network: lan })}</span>
            : <span className="pick-note">{route === lan ? t("· your current network") : t("· overlaps your network {network}", { network: lan })}</span>)}
        </div>;
      })}
      <textarea className="input mono" aria-label={choices.length ? t("Other subnets") : t("Remote subnets")} placeholder={choices.length ? t("Other subnets, one per line") : "192.168.50.0/24"}
        value={extra} onChange={(e) => setExtra(e.target.value)} />
      {extraOverlaps.map(([r, n]) => <div key={r} className="pick-note warn">{t("{route} overlaps your network {network}; matching addresses go through Tailscale.", { route: r, network: n! })}</div>)}
      <div className="pick-note">{t("Only these subnets go through Tailscale, even while their device is offline. Tailscale IPs and MagicDNS names always do.")}</div>
    </div></div>
    <div className="field"><span className="label">{t("Hostname")}</span><div className="scope">
      <input className="input mono" value={config.hostname} aria-label={t("Hostname")} aria-invalid={!hostOK} onChange={(e) => setConfig({ ...config, hostname: e.target.value })} />
      {!hostOK && <div className="pick-note err">{t("Use up to 63 letters, digits and hyphens, not starting or ending with a hyphen.")}</div>}
    </div></div>
    <button type="button" className="link tailscale-advanced" aria-expanded={advanced} onClick={() => setAdvanced(!advanced)}>
      <Chevron size={10} className={"chev" + (advanced ? " open" : "")} />{t("Advanced")}
    </button>
    <Fold open={advanced}>
      <div className="field"><span className="label">{t("Control server")}</span><div className="scope">
        <input className="input mono" type="url" aria-label={t("Control server")} placeholder="https://controlplane.tailscale.com" value={config.controlURL ?? ""} onChange={(e) => setConfig({ ...config, controlURL: e.target.value })} />
        <div className="pick-note">{t("Leave empty for Tailscale, or enter your Headscale HTTPS URL.")}</div>
      </div></div>
    </Fold>
    <div className="foot">
      {reconnects && <span className="hint">{t("Saving restarts the core to reconnect.")}</span>}
      <div className="grow" />
      <button type="button" className="btn small" onClick={async () => { try { onYAML(await asYAML(draft), { name, tailscale: out, extra }); } catch (e) { toastError(e); } }}>{t("Edit as YAML")}</button>
      <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
      <button className="btn small primary" type="submit" disabled={busy || !valid}>{module.id ? t("Save") : t("Add and log in")}</button>
    </div>
  </form>;
}

export const tailscaleKind: ModuleKind = {
  id: "tailscale", is: (m) => !!m.tailscale, Summary,
  Editor: (props) => <FormOrYAML<Form> {...props} Form={TailscaleEditor} />,
  duplicate: false,
  useNew: () => {
    const t = useT();
    return [{ title: t("Private networks"), entries: [{ key: "tailscale", name: t("Tailscale"), hint: t("Access tailnet devices and selected remote subnets"),
      make: async (profile) => ({ id: "", name: t("Tailscale"), enabled: true, body: "", profile,
        tailscale: { identity: crypto.randomUUID(), hostname: await App.TailscaleHostname().catch(() => "clashcube"), routes: [] } }),
    }] }];
  },
};
