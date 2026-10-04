import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { Hotkeys } from "../components/Hotkeys";
import { translate, useT } from "../i18n";
import { applyTheme, useStore } from "../store";
import { App, Proxy, Settings as S, type Patch, type HelperStatus, type GeoInfo } from "../api";
import { Switch } from "../components/Switch";
import { Segmented } from "../components/Segmented";
import { Refresh } from "../components/Icons";
import { errText, toast, toastError } from "../components/Toast";
import { coreLabel, coreTone, restartCore, run, setTun, startCore, stopCore } from "../actions";
import { ago } from "../format";
import { NetworkRules } from "../components/NetworkRules";

const TABS = ["general", "network", "rules", "tun", "core"] as const;
type Tab = (typeof TABS)[number];

// the settings the core runs with: changing one reloads it (PatchSettings)
const CORE_KEYS = new Set<string>(["mixedPort", "allowLan", "ipv6", "logLevel", "tunStack", "icmpForwarding", "findProcess", "guardIPv6", "guardDNS", "blockSTUN", "dnsRespectRules"]);
// the core settings being applied now, for their rows to say so
const Applying = createContext<ReadonlySet<string>>(new Set());

// the tab ?view=settings#<tab> names, else the one last open
function firstTab(): Tab {
  const h = location.hash.slice(1) as Tab;
  if (TABS.includes(h)) return h;
  try { const v = localStorage.getItem("settings.tab") as Tab; if (TABS.includes(v)) return v; } catch {}
  return "general";
}

export function Settings() {
  const t = useT();
  const s = useStore((st) => st.settings);
  const state = useStore((st) => st.state);
  const [tab, setTab] = useState<Tab>(firstTab);
  const [applying, setApplying] = useState<ReadonlySet<string>>(new Set());
  const pick = (v: Tab) => {
    setTab(v);
    try { localStorage.setItem("settings.tab", v); } catch {}
  };
  if (!s) return <div className="view" />;

  const tunStacks = [
    { value: "system", label: "System", title: t("Uses the system network stack.") },
    { value: "gvisor", label: "gVisor", title: t("Uses gVisor's userspace network stack for TCP and UDP.") },
    { value: "mixed", label: "Mixed", title: t("Uses the system stack for TCP and gVisor for UDP.") },
    { value: "mips", label: t("MIPS"), title: t("Uses mihomo's own pure-Go userspace IP stack.") },
  ];

  const patch = async (p: Patch) => {
    // shown at once
    useStore.setState({ settings: { ...s, ...(p as object) } as typeof s });
    if (p.theme) applyTheme(p.theme);
    // a running core reloads for these, which takes a moment and may fail
    const keys = state?.core === "running" ? Object.keys(p).filter((k) => CORE_KEYS.has(k)) : [];
    const mark = (on: boolean) => setApplying((a) => { const n = new Set(a); keys.forEach((k) => on ? n.add(k) : n.delete(k)); return n; });
    if (keys.length) mark(true);
    try {
      useStore.setState({ settings: await S.Patch(p) });
    } catch (e) {
      // the backend put the settings back; show them as they are
      if (keys.length) toast(t("Not applied: {error}", { error: errText(e) }), "err", 5000); else toastError(e);
      useStore.getState().refreshSettings();
    } finally {
      if (keys.length) mark(false);
    }
  };

  return (
    <Applying.Provider value={applying}>
    <div className="view settings">
      <div className="view-head">
        <Segmented className="track small" value={tab} onChange={pick} options={[
          { value: "general", label: t("General") },
          { value: "network", label: t("Network") },
          { value: "rules", label: t("Network rules") },
          { value: "tun", label: t("Enhanced Mode") },
          { value: "core", label: t("Core") },
        ]} />
      </div>

      {tab === "general" && <>
        <Section title={t("General")}>
          <Row label={t("Start core when the app opens")}><Switch on={s.autoStart} onChange={(v) => patch({ autoStart: v })} /></Row>
          <Row label={t("Open at login")}><Switch on={s.launchAtLogin} onChange={(v) => patch({ launchAtLogin: v })} /></Row>
          <Row label={t("Show speed in the menu bar")}><Switch on={s.traySpeed} onChange={(v) => patch({ traySpeed: v })} /></Row>
          <Row label={t("Notifications")} sub={t("Core errors, failed updates, the system proxy taken by another app, and network rules applied")}><Switch on={s.notify} onChange={(v) => patch({ notify: v })} /></Row>
          <Row label={t("Show in Dock")}>
            <select className="input" value={s.dock} onChange={(e) => patch({ dock: e.target.value })}>
              <option value="window">{t("While the window is open")}</option>
              <option value="always">{t("Always")}</option>
              <option value="never">{t("Never")}</option>
            </select>
          </Row>
        </Section>

        <Section title={t("Keyboard shortcuts")}>
          <Hotkeys />
        </Section>

        <Section title={t("Appearance")}>
          <Row label={t("Theme")}>
            <Segmented className="track small" value={s.theme} onChange={(v) => patch({ theme: v })} options={[{ value: "system", label: t("System") }, { value: "light", label: t("Light") }, { value: "dark", label: t("Dark") }]} />
          </Row>
          <Row label={t("Language")}>
            <Segmented className="track small" value={s.lang} onChange={(v) => patch({ lang: v })} options={[{ value: "system", label: t("System") }, { value: "zh", label: "中文" }, { value: "en", label: "English" }]} />
          </Row>
        </Section>

        <Section title={t("About")}>
          <Row label="MihomoBar"><span className="mono muted">{state?.appVersion}</span></Row>
          <Row label={t("mihomo")}><span className="mono muted">{state?.coreVersion}</span></Row>
        </Section>
      </>}

      {tab === "network" && <>
        <Section title={t("Network")}>
          <Row field="mixedPort" label={t("Mixed port")}><NumberInput value={s.mixedPort} onCommit={(v) => patch({ mixedPort: v })} /></Row>
          <Row field="allowLan" label={t("Allow LAN")}><Switch on={s.allowLan} onChange={(v) => patch({ allowLan: v })} /></Row>
          <Row field="ipv6" label={t("IPv6")}><Switch on={s.ipv6} onChange={(v) => patch({ ipv6: v })} /></Row>
          <Row field="findProcess" label={t("Identify processes")} sub={t("Show which app made each connection")}><Switch on={s.findProcess} onChange={(v) => patch({ findProcess: v })} /></Row>
          <Row label={t("Latency test URL")}><TextInput value={s.testUrl} onCommit={(v) => patch({ testUrl: v })} /></Row>
          <Row label={t("Save data on metered networks")} sub={t("On a personal hotspot or in Low Data Mode, subscriptions aren't updated and connectivity isn't measured in the background.")} wrap>
            {state?.network.savingData && <span className="badge">{t("Saving data")}</span>}
            <Switch on={s.saveData} onChange={(v) => patch({ saveData: v })} />
          </Row>
          <BypassRow value={s.bypass ?? []} onSave={(v) => patch({ bypass: v })} />
        </Section>
      </>}

      {tab === "rules" && <NetworkRules settings={s} />}

      {tab === "tun" && <>
        <Section title={t("Enhanced Mode")}>
          <ServiceModeRow />
          <Row field="tunStack" label={t("TUN stack")} sub={tunStacks.find((stack) => stack.value === s.tunStack)?.title} wrap>
            <Segmented className="track small" value={s.tunStack} onChange={(v) => patch({ tunStack: v })} options={tunStacks} />
          </Row>
          <Row field="icmpForwarding" label={t("ICMP forwarding")} sub={t("Pings go out directly, never through a proxy. Off: the core answers every ping itself. Pinging a domain under fake-ip always gets a local answer.")} wrap>
            <Switch on={s.icmpForwarding} onChange={(v) => patch({ icmpForwarding: v })} />
          </Row>
        </Section>

        <Section title={t("Leak Protection")}>
          {state?.systemProxy && !state.tun && (
            <div className="row">
              <div className="who">
                <div className="name">{t("System proxy only")}</div>
                <div className="sub warn wrap">{t("WebRTC sends UDP, which the system proxy doesn't carry, so websites can see your real IP. Apps that ignore the proxy look up names with the system's DNS.")}</div>
              </div>
              <div className="end"><button className="btn small primary" onClick={async () => { await patch({ guardIPv6: true, guardDNS: true }); await setTun(true); }}>{t("Turn on TUN and protection")}</button></div>
            </div>
          )}
          <Row field="guardIPv6" label={t("Route IPv6 into TUN")} sub={t("With IPv6 off, IPv6 traffic would go around TUN, exposing your IPv6 address to WebRTC and sending lookups to an IPv6 DNS server. Names still get no IPv6 answers; the core's own connections may use IPv6.")} wrap>
            <Switch on={s.guardIPv6} onChange={(v) => patch({ guardIPv6: v })} />
          </Row>
          <Row field="guardDNS" label={t("Take over DNS")} sub={t("Turns on the core's DNS and, under TUN, hijacks every lookup to port 53, whatever the profile says.")} wrap>
            <Switch on={s.guardDNS} onChange={(v) => patch({ guardDNS: v })} />
          </Row>
          <Row field="blockSTUN" label={t("Block STUN over UDP")} sub={t("A node without UDP lets WebRTC go direct. Rejected, WebRTC falls back to relays over TCP; some video calls may connect slower or fail.")} wrap>
            <Switch on={s.blockSTUN} onChange={(v) => patch({ blockSTUN: v })} />
          </Row>
          <Row field="dnsRespectRules" label={t("Look up names along the rules")} sub={t("The core's DNS queries go out through the policy their domain matches, so domestic DNS servers don't see the domains you proxy. Lookups get slower.")} wrap>
            <Switch on={s.dnsRespectRules} onChange={(v) => patch({ dnsRespectRules: v })} />
          </Row>
        </Section>
      </>}

      {tab === "core" && <>
        <CoreStatus />
        <Section title={t("Tools")}>
          <Row field="logLevel" label={t("Log level")}>
            <select className="input" value={s.logLevel} onChange={(e) => patch({ logLevel: e.target.value })}>
              {["debug", "info", "warning", "error", "silent"].map((l) => <option key={l}>{l}</option>)}
            </select>
          </Row>
          <Row label={t("Flush DNS cache")} sub={state?.core === "running" ? t("Also clears the fake-ip pool") : t("Needs the core running")}>
            <ActionButton label={t("Clear")} busyLabel={t("Clearing…")} disabled={state?.core !== "running"} onClick={() => run(Proxy.FlushDNS(), t("DNS cache cleared"))} />
          </Row>
          <GeoRow running={state?.core === "running"} />
          <Row label={t("Copy shell export command")} sub={t("⌥-click: use this Mac's LAN address")}>
            <button className="btn small" onClick={(e) => copyCommand(e.altKey, s.allowLan)}>{t("Copy")}</button>
          </Row>
          <Row label={t("Open data folder")}><button className="btn small" onClick={() => run(App.RevealData())}>Finder</button></Row>
        </Section>
      </>}
    </div>
    </Applying.Provider>
  );
}

// copyCommand copies the shell export line, for this Mac's LAN address when
// lan, saying when that address isn't there or isn't reachable yet.
async function copyCommand(lan: boolean, allowLan: boolean) {
  const t = translate;
  let cmd = lan ? await App.LANProxyCommand() : "";
  const fellBack = lan && !cmd;
  if (!cmd) cmd = await App.ProxyCommand();
  if (!await App.CopyText(cmd)) return toastError(t("Could not copy to clipboard"));
  if (fellBack) toast(t("No LAN address; copied the local one"), "", 3500);
  else if (lan && !allowLan) toast(t("Copied. Other devices need Allow LAN on"), "", 3500);
  else toast(t("Copied"));
}

// CoreStatus is the core at a glance, with what to do about it.
function CoreStatus() {
  const t = useT();
  const state = useStore((st) => st.state);
  const setView = useStore((st) => st.setView);
  if (!state) return null;
  const core = state.core;
  const running = core === "running";
  const busy = !!state.busy || core === "starting" || core === "stopping";
  const sub = running
    ? [`mihomo ${state.coreVersion}`, state.serviceMode ? t("Core runs as root") : t("Core runs as you"), `127.0.0.1:${state.mixedPort}`].join(" · ")
    : core === "crashed" ? t("Core stopped with an error") : t("Proxies, DNS and the tools below need the core running");
  return (
    <>
      <div className="section-title">{t("Status")}</div>
      <div className="list">
        <div className="row core-status">
          <div className="who">
            <div className="name"><span className={"cdot " + coreTone()} />{coreLabel()}</div>
            <div className={"sub" + (core === "crashed" ? " warn" : "")}>{sub}</div>
          </div>
          <div className="end">
            {busy ? <button className="btn small" disabled><Spinner />{coreLabel()}</button>
              : running ? <>
                <ActionButton label={t("Restart")} busyLabel={t("Restarting…")} onClick={restartCore} />
                <ActionButton className="danger" label={t("Stop")} busyLabel={t("Stopping…")} onClick={stopCore} />
              </> : <ActionButton className="primary" label={t("Start")} busyLabel={t("Starting…")} onClick={startCore} />}
          </div>
        </div>
        {core === "crashed" && state.coreError && (
          <div className="row core-error">
            <div className="who"><div className="mono">{state.coreError}</div></div>
            <div className="end"><button className="btn small" onClick={() => setView("logs")}>{t("Show logs")}</button></div>
          </div>
        )}
      </div>
    </>
  );
}

// GeoRow updates the GEO databases, which takes as long as the download.
function GeoRow({ running }: { running: boolean }) {
  const t = useT();
  const [info, setInfo] = useState<GeoInfo | null>(null);
  const [mine, setMine] = useState(false);
  const load = () => Proxy.GeoInfo().then(setInfo).catch(() => {});
  useEffect(() => { load(); }, []);
  // someone else's update (another window): follow it until it's done
  useEffect(() => {
    if (!info?.updating || mine) return;
    const id = setInterval(load, 1500);
    return () => clearInterval(id);
  }, [info?.updating, mine]);

  const update = async () => {
    setMine(true);
    try {
      const r = await Proxy.UpdateGeo();
      setInfo(r);
      if (r.updating) toast(t("An update is already under way"), "", 3000);
      else toast(t("GEO databases updated"));
    } catch (e) {
      toastError(e);
      load();
    } finally {
      setMine(false);
    }
  };
  const when = info?.updated ? t("Updated {t}", { t: ago(info.updated, t) }) : t("Not downloaded yet");
  return (
    <Row label={t("Update GEO databases")} sub={running ? when : t("Needs the core running")}>
      <ActionButton label={t("Update")} busyLabel={t("Updating…")} busy={mine || !!info?.updating} disabled={!running} onClick={update} />
    </Row>
  );
}

const Spinner = () => <span className="spin-svg" style={{ display: "inline-flex" }}><Refresh size={12} /></span>;

// ActionButton is a small button that, while its action runs, spins and
// says so, and can't be pressed again.
function ActionButton({ label, busyLabel, busy, disabled, className, onClick }: {
  label: string; busyLabel: string; busy?: boolean; disabled?: boolean; className?: string; onClick: () => Promise<unknown>;
}) {
  const [own, setOwn] = useState(false);
  const on = own || !!busy;
  const press = async () => {
    setOwn(true);
    try { await onClick(); } finally { setOwn(false); }
  };
  return (
    <button className={"btn small" + (className ? " " + className : "")} disabled={disabled || on} onClick={press}>
      {on && <Spinner />}{on ? busyLabel : label}
    </button>
  );
}

function ServiceModeRow() {
  const t = useT();
  const state = useStore((st) => st.state);
  const [hs, setHs] = useState<HelperStatus | null>(null);
  const [busy, setBusy] = useState(false);
  const load = () => App.HelperStatus().then(setHs).catch(() => {});
  useEffect(() => { load(); }, [state?.serviceMode, state?.core]);

  const act = async (p: Promise<void>) => {
    setBusy(true);
    try { await p; } catch (e) { if (!/cancelled/.test(String(e instanceof Error ? e.message : e))) toastError(e); }
    setBusy(false);
    load();
  };
  const status = !hs ? "" : !hs.installed ? t("Not installed") : !hs.current ? t("Needs update") : t("Installed");
  return (
    <div className="row service">
      <div className="who">
        <div className="name">{t("Privileged helper")}<span className={"badge" + (hs?.installed && hs.current ? "" : " muted")}>{status}</span></div>
        <div className="sub wrap">{t("Runs the core as root through a LaunchDaemon, which TUN needs. macOS asks for an administrator password once.")}</div>
        {hs?.enabled && <div className="sub wrap">{hs.active ? t("Core runs as root") : t("Core runs as you")} · {t("The root core reads its configuration from your user folder, so programs running as you can influence it.")}</div>}
      </div>
      <div className="end">
        {(!hs?.installed || !hs.current || !hs.enabled) && <button className="btn small primary" disabled={busy} onClick={() => act(hs?.installed && !hs.current ? App.EnableServiceMode() : App.SetTun(true))}>{hs?.installed && !hs.current ? t("Update") : t("Install and turn on TUN")}</button>}
        {hs?.installed && <button className="btn small danger" disabled={busy} onClick={() => act(App.DisableServiceMode(true))}>{t("Uninstall")}</button>}
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <>
      <div className="section-title">{title}</div>
      <div className="list">{children}</div>
    </>
  );
}

// Row is a setting; field names the core setting it changes, so the row
// says so while the core reloads for it.
function Row({ label, sub, wrap, field, children }: { label: string; sub?: string; wrap?: boolean; field?: keyof Patch; children: ReactNode }) {
  const t = useT();
  const applying = useContext(Applying).has(field ?? "");
  return (
    <div className="row">
      <div className="who"><div className="name">{label}</div>{sub && <div className={wrap ? "sub wrap" : "sub"}>{sub}</div>}</div>
      <div className="end">{applying && <span className="applying"><Spinner />{t("Applying…")}</span>}{children}</div>
    </div>
  );
}

function NumberInput({ value, onCommit }: { value: number; onCommit: (v: number) => void }) {
  const [v, setV] = useState(String(value));
  useEffect(() => setV(String(value)), [value]);
  const commit = () => { const n = parseInt(v, 10); if (n && n !== value) onCommit(n); else setV(String(value)); };
  return <input className="input num" value={v} inputMode="numeric" onChange={(e) => setV(e.target.value.replace(/\D/g, ""))} onBlur={commit} onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()} />;
}

function TextInput({ value, onCommit }: { value: string; onCommit: (v: string) => void }) {
  const [v, setV] = useState(value);
  useEffect(() => setV(value), [value]);
  return <input className="input" style={{ width: 280 }} value={v} onChange={(e) => setV(e.target.value)} onBlur={() => v.trim() !== value && onCommit(v.trim())} onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()} />;
}

function BypassRow({ value, onSave }: { value: string[]; onSave: (v: string[]) => void }) {
  const t = useT();
  const [text, setText] = useState(value.join("\n"));
  useEffect(() => setText(value.join("\n")), [value.join("\n")]);
  const dirty = text.trim() !== value.join("\n").trim();
  return (
    <div className="row bypass">
      <div className="who">
        <div className="name">{t("Bypass")}</div>
        <div className="sub">{t("One host or network per line")}</div>
        <textarea className="input" rows={5} value={text} onChange={(e) => setText(e.target.value)} spellCheck={false} />
      </div>
      {dirty && <button className="btn small primary save" onClick={() => onSave(text.split("\n"))}>{t("Save")}</button>}
    </div>
  );
}
