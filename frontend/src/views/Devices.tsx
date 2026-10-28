import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useLeaving } from "../useLeaving";
import { useFlip } from "../useFlip";
import { Router } from "../api";
import { useT } from "../i18n";
import { usePoll } from "../usePoll";
import { Shield, Route } from "../components/Icons";
import { Fold } from "../components/Fold";
import { calm } from "../components/Sidebar";
import { Segmented } from "../components/Segmented";
import { Popover, Menu } from "../components/Popover";
import { errText, toast } from "../components/Toast";
import type { DevicePolicy, DeviceRequest, DevicesView } from "../routerTypes";
import "./devices.css";

const connectionLabels: Record<string, string> = {
  "identity-changed": "Device address changed · blocked",
  configured: "Fixed node configured", paused: "Networking paused", pending: "Changes incomplete · blocked",
  "core-stopped": "Core stopped · blocked", "target-missing": "Node unavailable · blocked", unknown: "Status unknown", blocked: "Blocked",
};

export function Devices() {
  const t = useT();
  const [data, setData] = useState<DevicesView | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false), generation = useRef(0);
  const reading=useRef<number|null>(null);
  const [lastSeen,setLastSeen]=useState(0),[now,setNow]=useState(Date.now());
  const accept=(next:DevicesView)=>{const received=Date.now();setData(next);setLastSeen(received);setNow(received);};
  const [editor, setEditor] = useState<DevicePolicy | "new" | null>(null);
  const [release, setRelease] = useState<DevicePolicy | null>(null);
  const [leaving, setLeaving] = useState(false);
  const closeTimer = useRef<ReturnType<typeof setTimeout>>();
  useEffect(() => () => clearTimeout(closeTimer.current), []);
  const closeDialog = () => {
    if (closeTimer.current) return;
    const finish = () => { setEditor(null); setRelease(null); setLeaving(false); closeTimer.current = undefined; };
    if (calm()) { finish(); return; }
    setLeaving(true);
    closeTimer.current = setTimeout(finish, 220);
  };
  const [menu, setMenu] = useState<{ policy: DevicePolicy; anchor: HTMLElement } | null>(null);
  const [filter, setFilter] = useState<"all" | "attention">("all");
  const [search,setSearch]=useState("");
  const refresh = async () => {
    if (!Router || busyRef.current || reading.current!==null) return;
    const ticket = ++generation.current;
    reading.current=ticket;
    try { const next = await Router.Devices(); if (ticket === generation.current) { accept(next); setError(""); } }
    catch (e) { if (ticket === generation.current) setError(errText(e)); }
    finally {if(reading.current===ticket)reading.current=null;}
  };
  // Advance freshness even when a request never responds. A hung poll must not
  // leave an old green protection badge visible indefinitely.
  usePoll(()=>{setNow(Date.now());void refresh();}, 4000);
  const mutate = async (action: () => Promise<DevicesView>) => {
    busyRef.current = true; setBusy(true); ++generation.current;reading.current=null;
    try { accept(await action()); setError(""); toast(t("Device policy saved")); return true; }
    catch (e) {
      setError(errText(e));
      // A rejected reload may have durably added a blocked Pending device.
      // Surface that saved restriction instead of leaving an obsolete list.
      try { const next=await Router!.Devices();accept(next);if(editor==="new"&&next.policies.length>(data?.policies.length??0))closeDialog(); } catch {}
      return false;
    }
    finally { busyRef.current = false; setBusy(false); }
  };
  const policies = useMemo(() => data?.policies ?? [], [data]);
  const stale=!!error||!!data&&now-lastSeen>12000;
  const warning=error||data?.error||(stale?"Status response overdue; protection needs verification":"");
  const lastWarning = useRef(warning);
  if (warning) lastWarning.current = warning;
  const shown = useMemo(() => policies.filter((p) => (filter === "all" || p.connectivity !== "configured" || p.protection !== "protected" || stale) && `${p.name} ${p.ip} ${p.mac}`.toLowerCase().includes(search.toLowerCase())), [policies, filter, stale, search]);
  const rows = useLeaving(shown, (p) => p.id);
  const list = useRef<HTMLDivElement>(null);
  const loaded = useRef(false);
  const initialIDs = useRef(new Set<string>());
  if (data && !loaded.current) { loaded.current = true; initialIDs.current = new Set(policies.map((p) => p.id)); }
  useEffect(() => { initialIDs.current = new Set(policies.map((p) => p.id)); }, [policies]);
  useFlip(list, JSON.stringify(rows.map(({ item, leaving }) => [item.id, leaving])), { enter: false });
  const protectedCount = stale ? 0 : policies.filter((p) => p.protection === "protected").length;
  return <div className="view devices-view">
    <div className="view-head"><div><h2>{t("Devices")}</h2><p>{t("Choose a fixed node for each device. Keep protection through stops and restarts.")}</p></div>
      <button className="btn primary" disabled={!data || busy} onClick={() => setEditor("new")}>{t("Protect a device")}</button></div>
    <div className="device-assurance"><Shield /><div><b>{t("Protection stays on")}</b><p>{t("Stopping ClashCube blocks Internet access for protected devices. Only Release protection allows ordinary routing again.")}</p></div><span className="device-count"><DeviceValue value={`${protectedCount} / ${policies.length}`} /><small>{t("Verified protection")}</small></span></div>
    <Fold open={!!warning}><div className="device-warning" role="alert" ref={(el) => { if (el) el.inert = !warning; }}><b>{t("Review device protection")}</b><p>{t(lastWarning.current)}</p><button className="btn small" disabled={busy} onClick={refresh}>{t("Refresh")}</button></div></Fold>
    <div className="device-toolbar"><Segmented value={filter} onChange={setFilter} options={[{ value: "all", label: t("All devices") }, { value: "attention", label: t("Needs attention") }]} /><input className="input" type="search" aria-label={t("Search devices")} placeholder={t("Search devices")} value={search} onChange={(e)=>setSearch(e.target.value)} /><span>{t("IPv4 TCP + protected DNS")}</span></div>
    <div ref={list} className={"list device-list" + (data ? " motion-d-in" : "")}>{!data ? <div className="empty-state">{t(error ? "Status unknown" : "Loading…")}</div> : !rows.length ? <div className="empty-state"><Shield /><b>{t(policies.length ? "No devices need attention" : "No protected devices yet")}</b><span>{t("Select a LAN device and a fixed remote node to begin.")}</span></div> : rows.map(({ item: p, leaving }) => <article className={"device-row" + (leaving ? " motion-d-leaving" : !initialIDs.current.has(p.id) ? " motion-d-row-in" : "")} key={p.id} data-key={p.id} ref={(el) => { if (el) el.inert = leaving; }}>
      <div className="device-identity"><div className="device-icon"><Route /></div><div><b>{p.name}</b><div className="device-address">{p.ip}<span>·</span>{p.mac}</div></div></div>
      <div className="device-egress"><small>{t("Fixed egress")}</small><b>{p.target.name}</b><span>{p.target.type.toUpperCase()} {p.target.provider ? `· ${p.target.provider}` : ""}</span></div>
      <div className="device-status"><span className={"device-badge " + (!stale && p.protection === "protected" ? "safe" : "unknown")}><Shield /><DeviceValue value={t(!stale && p.protection === "protected" ? "Protected" : "Protection unverified")} /></span><span>{t(stale ? "Status unknown" : connectionLabels[p.connectivity] ?? "Status unknown")}</span></div>
      <button className="btn small" aria-label={t("Device actions")} disabled={busy} onClick={(e) => setMenu({ policy: p, anchor: e.currentTarget })}>•••</button>
    </article>)}</div>
    <p className="device-footnote">{t("Unsupported UDP and IPv6 Internet traffic is blocked. LAN access and router management remain available. A configured node does not confirm its external IP.")}</p>
    <Popover open={!!menu} anchor={menu?.anchor ?? null} onClose={() => setMenu(null)} align="end"><Menu close={() => setMenu(null)} items={menu && data ? [
      { label: t("Change node"), onClick: () => { setEditor(menu.policy); setMenu(null); } },
      { label: t(menu.policy.paused ? "Resume networking" : "Pause networking"), onClick: () => { const p = menu.policy; setMenu(null); void mutate(() => Router!.PauseDevice(p.id, data.revision, !p.paused)); } },
      "sep", { label: t("Release protection"), danger: true, onClick: () => { setRelease(menu.policy); setMenu(null); } },
    ] : []} /></Popover>
    {editor && data && <DeviceEditor key={typeof editor === "string" ? editor : editor.id} policy={editor === "new" ? null : editor} data={data} busy={busy} leaving={leaving} error={error} close={closeDialog} save={async (request) => { if (await mutate(() => Router!.ApplyDevice(request))) closeDialog(); }} />}
    {release && data && <DeviceDialog title={t("Release protection")} busy={busy} leaving={leaving} close={closeDialog}><p><b>{release.name}</b> · {release.ip}</p><p>{t("This device may connect directly when ClashCube stops or restarts. Its DHCP reservation will be kept.")}</p><div className="device-dialog-actions"><button className="btn" disabled={busy} onClick={closeDialog}>{t("Keep protection")}</button><button className="btn danger" disabled={busy} onClick={async () => { if (await mutate(() => Router!.ReleaseDevice(release.id, data.revision))) closeDialog(); }}>{t(busy ? "Saving…" : "Release protection")}</button></div></DeviceDialog>}
  </div>;
}

function DeviceValue({ value }: { value: string }) {
  const previous = useRef(value);
  const changed = useRef(false);
  if (previous.current !== value) { previous.current = value; changed.current = true; }
  return <span key={value} className={changed.current ? "motion-d-value" : undefined}>{value}</span>;
}

function DeviceDialog({ title, busy, leaving, close, children }: { title: string; busy: boolean; leaving: boolean; close: () => void; children: React.ReactNode }) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => { ref.current?.showModal(); return () => ref.current?.close(); }, []);
  return <dialog ref={ref} className={"device-dialog" + (leaving ? " leaving" : "")} aria-label={title} onCancel={(e) => { e.preventDefault(); if (!busy) close(); }}><div ref={(el) => { if (el) el.inert = leaving; }}><h2>{title}</h2>{children}</div></dialog>;
}

function DeviceEditor({ policy, data, busy, leaving, error, save, close }: { policy: DevicePolicy | null; data: DevicesView; busy: boolean; leaving: boolean; error: string; save: (request: DeviceRequest) => Promise<void>; close: () => void }) {
  const t = useT();
  const [manual, setManual] = useState(!!policy);
  const [name, setName] = useState(policy?.name ?? ""), [mac, setMAC] = useState(policy?.mac ?? ""), [ip, setIP] = useState(policy?.ip ?? "");
  const [query, setQuery] = useState(""), [target, setTarget] = useState(policy?.target ?? null);
  const [reserve, setReserve] = useState(!policy);
  const nodes = data.nodes.filter((n) => !n.reason && `${n.name} ${n.provider} ${n.type}`.toLowerCase().includes(query.toLowerCase()));
  const available = data.devices.filter((d) => !data.policies.some((p) => p.mac === d.mac));
  const submit = (e: FormEvent) => { e.preventDefault(); if (target) void save({ revision: data.revision, id: policy?.id ?? "", name, mac, ip, target, reserve }); };
  return <DeviceDialog title={t(policy ? "Change node" : "Protect a device")} busy={busy} leaving={leaving} close={close}><form onSubmit={submit}>
    {!policy && <><Segmented value={manual ? "manual" : "detected"} onChange={(v) => setManual(v === "manual")} options={[{ value: "detected", label: t("LAN devices") }, { value: "manual", label: t("Enter manually") }]} />
      {!manual && <label>{t("LAN device")}<select aria-label={t("LAN device")} required value={mac} onChange={(e) => { const d = available.find((d) => d.mac === e.target.value); if (d) { setName(d.name); setMAC(d.mac); setIP(d.ip); } }}><option value="">{t("Choose a device")}</option>{available.map((d) => <option key={d.mac} value={d.mac}>{d.name} · {d.ip} · {d.mac}</option>)}</select></label>}</>}
    <Fold open={manual}><div className="stagger"><label>{t("Device name")}<input required maxLength={128} value={name} onChange={(e) => setName(e.target.value)} /></label><label>{t("MAC address")}<input required readOnly={!!policy} value={mac} placeholder="AA:BB:CC:DD:EE:FF" onChange={(e) => setMAC(e.target.value)} /></label><label>{t("IPv4 address")}<input required readOnly={!!policy} value={ip} placeholder="192.168.1.100" onChange={(e) => setIP(e.target.value)} /></label></div></Fold>
    <label>{t("Find a node")}<input type="search" value={query} placeholder={t("Search by name or provider")} onChange={(e) => setQuery(e.target.value)} /></label>
    <label>{t("Fixed egress")}<select aria-label={t("Fixed egress")} required value={target ? JSON.stringify(target) : ""} onChange={(e) => setTarget(e.target.value ? JSON.parse(e.target.value) : null)}><option value="">{t("Choose a fixed node")}</option>{nodes.map(({ reason: _, ...n }) => <option key={`${n.provider}/${n.name}/${n.fingerprint}`} value={JSON.stringify(n)}>{n.name} · {n.type.toUpperCase()}{n.provider ? ` · ${n.provider}` : ""}</option>)}</select></label>
    <p className="device-help">{t("Automatic groups and DIRECT are excluded. Missing or changed nodes keep this device blocked.")}</p>
    {!policy && <label className="device-check"><input type="checkbox" checked={reserve} onChange={(e) => setReserve(e.target.checked)} />{t("Reserve this IPv4 address in DHCP")}</label>}
    {!policy && !reserve && <p className="device-help">{t("Set a static address on this device. An address change keeps Internet access blocked until you review its identity.")}</p>}
    <div className="device-preview"><Shield /><p>{t("Only this node carries IPv4 TCP and public DNS. Pause, stop or restart keeps protection on; unsupported UDP and IPv6 is blocked.")}</p></div>
    <p className="device-help">{t("Use a stable MAC address for this network. Changing both MAC and IP creates a new device identity.")}</p>
    {error && <p className="device-error" role="alert">{t(error)}</p>}
    <div className="device-dialog-actions"><button type="button" className="btn" disabled={busy} onClick={close}>{t("Cancel")}</button><button className="btn primary" disabled={busy || !name || !mac || !ip || !target}>{t(busy ? "Saving…" : policy ? "Save fixed egress" : "Save and protect")}</button></div>
  </form></DeviceDialog>;
}
