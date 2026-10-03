import { useEffect, useState, type ReactNode } from "react";
import { useT } from "../i18n";
import { applyTheme, useStore } from "../store";
import { App, Proxy, Settings as S, type Patch, type HelperStatus } from "../api";
import { Switch } from "../components/Switch";
import { Segmented } from "../components/Segmented";
import { toast, toastError } from "../components/Toast";
import { run } from "../actions";

export function Settings() {
  const t = useT();
  const s = useStore((st) => st.settings);
  const state = useStore((st) => st.state);
  // ?view=settings#tun opens on Enhanced Mode
  const tunRef = (el: HTMLDivElement | null) => { if (el && location.hash === "#tun") el.scrollIntoView(); };
  if (!s) return <div className="view" />;

  const patch = async (p: Patch) => {
    // shown at once
    useStore.setState({ settings: { ...s, ...(p as object) } as typeof s });
    if (p.theme) applyTheme(p.theme);
    try { useStore.setState({ settings: await S.Patch(p) }); } catch (e) { toastError(e); useStore.getState().refreshSettings(); }
  };

  return (
    <div className="view settings">
      <Section title={t("General")}>
        <Row label={t("Start core when the app opens")}><Switch on={s.autoStart} onChange={(v) => patch({ autoStart: v })} /></Row>
        <Row label={t("Open at login")}><Switch on={s.launchAtLogin} onChange={(v) => patch({ launchAtLogin: v })} /></Row>
        <Row label={t("Show speed in the menu bar")}><Switch on={s.traySpeed} onChange={(v) => patch({ traySpeed: v })} /></Row>
        <Row label={t("Show in Dock")}>
          <select className="input" value={s.dock} onChange={(e) => patch({ dock: e.target.value })}>
            <option value="window">{t("While the window is open")}</option>
            <option value="always">{t("Always")}</option>
            <option value="never">{t("Never")}</option>
          </select>
        </Row>
      </Section>

      <Section title={t("Appearance")}>
        <Row label={t("Theme")}>
          <Segmented className="track small" value={s.theme} onChange={(v) => patch({ theme: v })} options={[{ value: "system", label: t("System") }, { value: "light", label: t("Light") }, { value: "dark", label: t("Dark") }]} />
        </Row>
        <Row label={t("Language")}>
          <Segmented className="track small" value={s.lang} onChange={(v) => patch({ lang: v })} options={[{ value: "system", label: t("System") }, { value: "zh", label: "中文" }, { value: "en", label: "English" }]} />
        </Row>
      </Section>

      <Section title={t("Network")}>
        <Row label={t("Mixed port")}><NumberInput value={s.mixedPort} onCommit={(v) => patch({ mixedPort: v })} /></Row>
        <Row label={t("Allow LAN")}><Switch on={s.allowLan} onChange={(v) => patch({ allowLan: v })} /></Row>
        <Row label={t("IPv6")}><Switch on={s.ipv6} onChange={(v) => patch({ ipv6: v })} /></Row>
        <Row label={t("Log level")}>
          <select className="input" value={s.logLevel} onChange={(e) => patch({ logLevel: e.target.value })}>
            {["debug", "info", "warning", "error", "silent"].map((l) => <option key={l}>{l}</option>)}
          </select>
        </Row>
        <Row label={t("Identify processes")} sub={t("Show which app made each connection")}><Switch on={s.findProcess} onChange={(v) => patch({ findProcess: v })} /></Row>
        <Row label={t("Latency test URL")}><TextInput value={s.testUrl} onCommit={(v) => patch({ testUrl: v })} /></Row>
        <BypassRow value={s.bypass ?? []} onSave={(v) => patch({ bypass: v })} />
      </Section>

      <div ref={tunRef} />
      <Section title={t("Enhanced Mode")}>
        <ServiceModeRow />
        <Row label={t("TUN stack")}>
          <Segmented className="track small" value={s.tunStack} onChange={(v) => patch({ tunStack: v })} options={[{ value: "system", label: "System" }, { value: "gvisor", label: "gVisor" }, { value: "mixed", label: "Mixed" }]} />
        </Row>
        <Row label={t("ICMP forwarding")} sub={t("Pings go out directly, never through a proxy. Off: the core answers every ping itself. Pinging a domain under fake-ip always gets a local answer.")} wrap>
          <Switch on={s.icmpForwarding} onChange={(v) => patch({ icmpForwarding: v })} />
        </Row>
      </Section>

      <Section title={t("Core")}>
        <Row label={t("Flush DNS cache")}><button className="btn small" disabled={state?.core !== "running"} onClick={() => run(Proxy.FlushDNS(), t("Saved"))}>{t("Clear")}</button></Row>
        <Row label={t("Update GEO databases")}><button className="btn small" disabled={state?.core !== "running"} onClick={() => run(Proxy.UpdateGeo(), t("Saved"))}>{t("Update")}</button></Row>
        <Row label={t("Copy shell export command")}><button className="btn small" title={t("⌥-click: use this Mac's LAN address")} onClick={async (e) => { App.CopyText(await (e.altKey ? App.LANProxyCommand() : App.ProxyCommand())); toast(t("Copied")); }}>{t("Copy")}</button></Row>
        <Row label={t("Open data folder")}><button className="btn small" onClick={() => run(App.RevealData())}>Finder</button></Row>
      </Section>

      <Section title={t("About")}>
        <Row label="MihomoBar"><span className="mono muted">{state?.appVersion}</span></Row>
        <Row label={t("mihomo")}><span className="mono muted">{state?.coreVersion}</span></Row>
      </Section>
    </div>
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

function Row({ label, sub, wrap, children }: { label: string; sub?: string; wrap?: boolean; children: ReactNode }) {
  return (
    <div className="row">
      <div className="who"><div className="name">{label}</div>{sub && <div className={wrap ? "sub wrap" : "sub"}>{sub}</div>}</div>
      <div className="end">{children}</div>
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
