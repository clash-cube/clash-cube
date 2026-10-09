import { useEffect, useState } from "react";
import { useT } from "../../i18n";
import { App, Profiles as P, Proxy } from "../../api";
import type { Module, Port } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import { useStore } from "../../store";
import { useConnectionStore } from "../../connectionStore";
import { nodeLabel, speed } from "../../format";
import { Popover, Menu } from "../Popover";
import { Segmented } from "../Segmented";
import { Switch } from "../Switch";
import { Fold } from "../Fold";
import { Chevron, Copy } from "../Icons";
import { toast, toastError } from "../Toast";
import { asYAML, FormOrYAML, NodeSelect, useNodes, type FormProps } from "./shared";
import type { ModuleKind, RowCtx } from "./kinds";

// modules.Listens and modules.PortPrefix in Go
const LOCAL = "127.0.0.1";
const LAN = "0.0.0.0";
const PREFIX = "ClashCube port ";

const addrOf = (p: Port) => p.listen || LOCAL;
const isLAN = (p: Port) => addrOf(p) === LAN;
const targetOf = (p: Port, profile: string) => p.target?.[profile] ?? "";
const withTarget = (p: Port, profile: string, name: string): Port => ({ ...p, target: { ...p.target, [profile]: name } });

// a password for a port opened to the LAN: 16 letters and digits
function password() {
  const abc = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  return Array.from(crypto.getRandomValues(new Uint32Array(16)), (n) => abc[n % abc.length]).join("");
}

// useLANAddress is this Mac's LAN address, "" without one or until known.
function useLANAddress(want: boolean) {
  const [ip, setIP] = useState("");
  useEffect(() => { if (want) App.LANAddress().then(setIP).catch(() => {}); }, [want]);
  return ip;
}

// the address another program reaches the port at
const hostOf = (p: Port, lanIP: string) => (isLAN(p) ? lanIP || LAN : LOCAL);

// socksURL is the port as socks5://user:pass@host:port, the user and
// password escaped
function socksURL(p: Port, host: string) {
  const auth = p.user ? `${encodeURIComponent(p.user)}:${encodeURIComponent(p.pass ?? "")}@` : "";
  return `socks5://${auth}${host}:${p.port}`;
}

// The port written out for where it is used: as is, as a URL, for a
// shell, and as a node for Clash and Surge profiles on other devices.
function formats(name: string, p: Port, host: string, t: ReturnType<typeof useT>) {
  const url = socksURL(p, host);
  const q = JSON.stringify;
  return [
    { label: t("Address"), text: `${host}:${p.port}` },
    { label: t("SOCKS5 URL"), text: url },
    { label: t("Terminal proxy command"), text: `export https_proxy=${url} http_proxy=${url} all_proxy=${url}` },
    { label: t("Clash node"), text: `- {name: ${q(name)}, type: socks5, server: ${host}, port: ${p.port}${p.user ? `, username: ${q(p.user)}, password: ${q(p.pass ?? "")}` : ""}, udp: ${p.udp}}` },
    { label: t("Surge node"), text: `${name} = socks5, ${host}, ${p.port}${p.user ? `, ${p.user}, ${p.pass ?? ""}` : ""}${p.udp ? ", udp-relay=true" : ""}` },
  ];
}

// CopyMenu copies the port as a socks5:// URL at a click, the form most
// clients and tools take; its arrow offers the other forms.
function CopyMenu({ name, port }: { name: string; port: Port }) {
  const t = useT();
  const [at, setAt] = useState<HTMLElement | null>(null);
  const lanIP = useLANAddress(!!at && isLAN(port));
  const copy = async (text: string, done = t("Copied")) => {
    if (await App.CopyText(text)) toast(done); else toastError(t("Could not copy to clipboard"));
  };
  const copyURL = async () => {
    const ip = isLAN(port) ? lanIP || await App.LANAddress().catch(() => "") : "";
    copy(socksURL(port, hostOf(port, ip)), t("Copied SOCKS5 URL"));
  };
  return (
    <span className="copy-split">
      <button className="icon" title={t("Copy SOCKS5 URL")} onClick={copyURL}><Copy size={13} /></button>
      <button className={"icon more" + (at ? " on" : "")} title={t("Other formats")} onClick={(e) => setAt(at ? null : e.currentTarget)}><Chevron size={9} className="chev" /></button>
      <Popover anchor={at} open={!!at} onClose={() => setAt(null)} align="end">
        <Menu close={() => setAt(null)} items={formats(name, port, hostOf(port, lanIP), t).map((f) => ({ label: f.label, onClick: () => copy(f.text) }))} />
      </Popover>
    </span>
  );
}

// groupNames is the groups a port can serve: the profile's, without GLOBAL
const groupNames = (ctx: Pick<RowCtx, "groups">) => (ctx.groups ?? []).filter((g) => g.name !== "GLOBAL" && !g.hidden).map((g) => g.name);

function Summary({ m, ctx }: { m: Module; ctx: RowCtx }) {
  const t = useT();
  const p = m.port!;
  const target = targetOf(p, ctx.profile);
  const snapshot = useConnectionStore((s) => s.snapshot);
  const conns = snapshot.active.filter((c) => c.metadata.inboundName === PREFIX + p.port);
  const rate = conns.reduce((n, c) => n + c.up + c.down, 0);
  const lanIP = useLANAddress(isLAN(p));
  // how the port came up in the running core (backend.PortState)
  const state = useStore((s) => s.state?.ports?.find((x) => x.port === p.port));
  const [fixing, setFixing] = useState(false);
  const gone = !!target && !!ctx.nodes && !ctx.nodes.some((n) => n.name === target) && !groupNames(ctx).includes(target);
  if (!target) return <div className="sub warn">{t("Choose a node for this profile; until then the port is closed")}</div>;
  if (m.enabled && state?.status === "taken") {
    // moved to the next free port, which the save checks again
    const move = async (e: React.MouseEvent) => {
      e.stopPropagation();
      setFixing(true);
      try {
        const port = await P.FreePort(p.port + 1);
        if (await ctx.onSave({ ...m, port: { ...p, port } })) toast(t("Moved to port {port}", { port }));
      } catch (err) { toastError(err); }
      setFixing(false);
    };
    return (
      <div className="sub warn">
        {state.holder ? t("Port {port} is held by {app}, so it isn't open", { port: p.port, app: state.holder }) : t("Port {port} is held by another app, so it isn't open", { port: p.port })}
        {" · "}<button className="link" disabled={fixing} onClick={move}>{t("Use a free port")}</button>
      </div>
    );
  }
  if (m.enabled && state?.status === "closed") {
    const retry = async (e: React.MouseEvent) => {
      e.stopPropagation();
      setFixing(true);
      try { await App.Reload(); } catch (err) { toastError(err); }
      setFixing(false);
    };
    return (
      <div className="sub warn">
        {t("Port {port} didn't open", { port: p.port })}{" · "}<button className="link" disabled={fixing} onClick={retry}>{t("Retry")}</button>
      </div>
    );
  }
  if (gone) return <div className="sub warn">{t("{node} is gone; connections to the port are refused", { node: nodeLabel(target) })}</div>;
  return (
    <div className="sub port-sub">
      <span className="mono">{hostOf(p, lanIP)}:{p.port}</span>
      {isLAN(p) && <span className="badge muted">{t("LAN")}</span>}
      {p.user && <span className="badge muted">{t("Password")}</span>}
      {m.enabled && conns.length > 0 && <span className="live">{t("{n} connections", { n: conns.length })} · {speed(rate)}</span>}
    </div>
  );
}

function End({ m, ctx }: { m: Module; ctx: RowCtx }) {
  const t = useT();
  const p = m.port!;
  return (
    <>
      <NodeSelect value={targetOf(p, ctx.profile)} nodes={ctx.nodes} groups={groupNames(ctx)} align="end"
        title={t("What connects to the port leaves by this, for this profile only")}
        onChange={(name) => ctx.onSave({ ...m, port: withTarget(p, ctx.profile, name) })} />
      <CopyMenu name={m.name} port={p} />
    </>
  );
}

type PortForm = { name: string; named: boolean; port: Port };

function PortEditor({ module, form, onCancel, onSave, onYAML }: FormProps<PortForm>) {
  const t = useT();
  const profile = useStore((s) => s.state?.profile ?? "");
  const mixed = useStore((s) => s.settings?.mixedPort ?? 0);
  const { groups } = useGroupNames();
  const nodes = useNodes();
  const [p, setP] = useState<Port>(form?.port ?? module.port!);
  const [name, setName] = useState(form?.name ?? module.name);
  // a new one is named after its node until named by hand
  const [named, setNamed] = useState(form?.named ?? !!module.id);
  const [portText, setPortText] = useState(String(p.port));
  const [busy, setBusy] = useState(false);
  const lanIP = useLANAddress(isLAN(p));
  const target = targetOf(p, profile);
  const shownName = named ? name : target ? nodeLabel(target) : name;
  const portErr = !(p.port >= 1 && p.port <= 65535 && String(p.port) === portText.trim()) ? t("A port is a number from 1 to 65535")
    : p.port === mixed ? t("The mixed port already uses {port}", { port: p.port }) : "";
  const out: Port = { port: p.port, listen: addrOf(p), target: p.target ?? {}, udp: p.udp, user: p.user ?? "", pass: p.user ? p.pass ?? "" : "" };
  const was = module.port!;
  const dirty = shownName.trim() !== module.name || JSON.stringify(out) !== JSON.stringify({
    port: was.port, listen: addrOf(was), target: was.target ?? {}, udp: was.udp, user: was.user ?? "", pass: was.user ? was.pass ?? "" : "",
  });
  const ok = !!shownName.trim() && !!target && !portErr && (!p.user || !!p.pass);
  const submit = async () => {
    if (busy || !ok) return;
    setBusy(true);
    try { await onSave({ ...module, name: shownName.trim(), port: out }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  const toYAML = async () => {
    try { onYAML(await asYAML({ ...module, name: shownName.trim() || module.name, port: out }), { name: shownName, named: true, port: p }); } catch (e) { toastError(e); }
  };
  // opened to the LAN, a port gets a password unless it has one
  const setListen = (listen: string) => setP(listen === LAN && !p.user ? { ...p, listen, user: "clashcube", pass: password() } : { ...p, listen });
  return (
    <form className="module-editor route-editor port-editor stagger" onSubmit={(e) => { e.preventDefault(); submit(); }}
      onKeyDown={(e) => { if (e.key === "s" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); submit(); } if (e.key === "Escape") onCancel(); }}>
      <input className="input" placeholder={t("Module name")} value={shownName} onChange={(e) => { setNamed(true); setName(e.target.value); }} />
      <div className="field">
        <span className="label">{t("Node")}</span>
        <div className="scope">
          <NodeSelect value={target} nodes={nodes} groups={groups} title={t("What connects to the port leaves by this, for this profile only")}
            onChange={(n) => setP(withTarget(p, profile, n))} />
          <div className="pick-note">{t("Chosen for this profile only; each profile has its own. A group follows its choice.")}</div>
        </div>
      </div>
      <div className="field">
        <span className="label">{t("Port")}</span>
        <div className="scope">
          <input className="input num mono" inputMode="numeric" value={portText} aria-invalid={!!portErr}
            onChange={(e) => { setPortText(e.target.value); setP({ ...p, port: Number(e.target.value) }); }} />
          {portErr && <div className="pick-note err">{portErr}</div>}
        </div>
      </div>
      <div className="field">
        <span className="label">{t("Listen")}</span>
        <div className="scope">
          <Segmented className="track small" value={addrOf(p)} onChange={setListen} options={[
            { value: LOCAL, label: t("This Mac only") }, { value: LAN, label: t("LAN") },
          ]} />
          <Fold open={isLAN(p)}>
            <div className="pick-note">{lanIP ? t("Other devices connect to {addr}.", { addr: `${lanIP}:${p.port}` }) : t("Other devices connect to this Mac's LAN address.")}</div>
          </Fold>
        </div>
      </div>
      <div className="field">
        <span className="label">{t("Password")}</span>
        <div className="scope">
          <Switch on={!!p.user} label={t("Password")} onChange={(on) => setP(on ? { ...p, user: p.user || "clashcube", pass: p.pass || password() } : { ...p, user: "", pass: "" })} />
          <Fold open={!!p.user}>
            <div className="port-auth">
              <input className="input mono" placeholder={t("User")} value={p.user ?? ""} onChange={(e) => setP({ ...p, user: e.target.value })} />
              <input className="input mono" placeholder={t("Password")} value={p.pass ?? ""} onChange={(e) => setP({ ...p, pass: e.target.value })} />
              <button type="button" className="link" onClick={() => setP({ ...p, pass: password() })}>{t("New password")}</button>
            </div>
          </Fold>
        </div>
      </div>
      <div className="field">
        <span className="label">UDP</span>
        <div className="scope row-inline">
          <Switch on={p.udp} label="UDP" onChange={(udp) => setP({ ...p, udp })} />
          <span className="pick-note">{t("Relay UDP too, for games, calls and QUIC")}</span>
        </div>
      </div>
      <div className="foot">
        <span className={"hint" + (isLAN(p) && !p.user ? " err" : "")}>
          {!target ? t("Choose the node the port leaves by.")
            : isLAN(p) && !p.user ? t("Anyone on your network can use this port without a password.")
            : t("{addr} leaves by {node}, whatever the rules and the outbound mode say.", { addr: `${hostOf(p, lanIP)}:${p.port}`, node: nodeLabel(target) })}
        </span>
        <div className="grow" />
        <button type="button" className="btn small" onClick={toYAML}>{t("Edit as YAML")}</button>
        <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
        <button type="submit" className="btn small primary" disabled={busy || !ok || (!!module.id && !dirty)}>{busy ? t("Checking…") : module.id ? t("Save") : t("Add")}</button>
      </div>
    </form>
  );
}

// useGroupNames is the groups of the running profile a port can serve.
function useGroupNames() {
  const core = useStore((s) => s.state?.core);
  const profile = useStore((s) => s.state?.profile);
  const [groups, setGroups] = useState<string[]>([]);
  useEffect(() => {
    if (core !== "running") { setGroups([]); return; }
    Proxy.Groups().then((gs) => setGroups(groupNames({ groups: gs ?? [] }))).catch(() => {});
  }, [core, profile]);
  return { groups };
}

// newPort is a draft port module serving target (none yet when ""),
// on the first free port after the mixed port.
export async function newPort(profile: string, target = ""): Promise<Module> {
  const st = useStore.getState();
  const at = st.state?.profile ?? "";
  const port = await P.FreePort((st.settings?.mixedPort ?? 7890) + 1);
  return {
    id: "", name: target ? nodeLabel(target) : "", enabled: true, body: "", profile,
    port: { port, listen: LOCAL, target: target ? { [at]: target } : {}, udp: true },
  };
}

// openPortDraft opens the modules page on a new global port serving
// target, to add once its port and listening address are as wanted.
export async function openPortDraft(target: string) {
  try {
    const m = await newPort("", target);
    useStore.setState({ moduleDraft: m });
    useStore.getState().setView("profiles");
  } catch (e) { toastError(e); }
}

// A port serves one node or group as SOCKS5, around the rules; its row
// changes the node and copies the port for where it is used.
export const portKind: ModuleKind = {
  id: "port",
  is: (m) => !!m.port,
  Summary,
  End,
  Editor: (props) => <FormOrYAML<PortForm> {...props} Form={PortEditor} />,
  useNew: () => {
    const t = useT();
    return [{
      title: t("Serve a node"),
      entries: [{
        key: "port", name: t("SOCKS5 port"), hint: t("One node or group on a port of its own, for this Mac or the LAN, whatever the rules say"),
        make: (profile) => newPort(profile),
      }],
    }];
  },
  duplicate: async (m) => {
    const port = await P.FreePort(m.port!.port + 1);
    return { ...m, id: "", port: { ...m.port!, port } };
  },
};
