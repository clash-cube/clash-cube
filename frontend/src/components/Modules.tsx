import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { Profiles as P } from "../api";
import type { Module, Route, Service, Template } from "../../bindings/github.com/localhost-copilot/clashferry/internal/modules/models";
import type { Node, RegionNodes } from "../../bindings/github.com/localhost-copilot/clashferry/internal/backend/models";
import type { Group } from "../api";
import { useGroups } from "../useGroups";
import { useStore } from "../store";
import { delayClass, fmtDelay } from "../format";
import { Popover } from "./Popover";
import { Segmented } from "./Segmented";
import { Switch } from "./Switch";
import { Fold } from "./Fold";
import { Arrow, Chevron, Close, Search } from "./Icons";
import { toast, toastError, errText } from "./Toast";

const EXAMPLE = `dns:
  +fake-ip-filter:
    - "+.lan"
hosts:
  router.lan: 192.168.1.1
prepend-rules:
  - DOMAIN-SUFFIX,example.com,DIRECT`;

// Modules are YAML laid over every profile in order, as Surge's are: each
// one a row with its switch, unrolling into its editor when clicked. A
// route sends a service through a group of its own; its row picks the node.
export function Modules({ newAt, onNewClose }: { newAt: HTMLElement | null; onNewClose: () => void }) {
  const t = useT();
  const [mods, setMods] = useState<Module[] | null>(null);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const { groups, select, load: loadGroups } = useGroups();
  const [open, setOpen] = useState("");
  // a new module being written, blank or from a template
  const [draft, setDraft] = useState<Module | null>(null);
  const [flash, setFlash] = useState("");
  const load = () => P.Modules().then((m) => setMods(m ?? [])).catch(toastError);
  useEffect(() => {
    load();
    P.ModuleTemplates().then((ts) => setTemplates(ts ?? [])).catch(() => {});
    P.RouteServices().then((ss) => setServices(ss ?? [])).catch(() => {});
  }, []);

  // the core checks every change; one it refuses leaves the list as it was
  const save = async (next: Module[]) => {
    const before = mods;
    setMods(next);
    try { await P.SetModules(next); await load(); loadGroups(); return true; } catch (e) { setMods(before); toastError(e); return false; }
  };
  // a template opens in the editor, and is added only once saved there
  const use = (tpl: Template | null) => {
    onNewClose();
    setOpen("");
    setDraft({ id: "", name: tpl ? t(tpl.name) : "", enabled: true, body: tpl?.body ?? "" });
  };
  const route = (svc: Service) => {
    onNewClose();
    setOpen("");
    setDraft({ id: "", name: t(svc.name), enabled: true, body: "", route: { service: svc.name, policy: "select", region: svc.region ?? "" } });
  };
  const added = new Set(mods?.map((m) => m.name) ?? []);
  const routed = new Set(mods?.flatMap((m) => (m.route ? [m.route.service] : [])) ?? []);
  const move = (i: number, d: number) => {
    const next = mods!.slice();
    [next[i], next[i + d]] = [next[i + d], next[i]];
    save(next);
  };

  if (!mods) return null;
  const add = async (m: Module) => {
    if (!(await save([...mods, m]))) return;
    setDraft(null);
    toast(t("Added {name}", { name: m.name }));
    setFlash(m.name); setTimeout(() => setFlash(""), 900);
  };
  const ownGroup = (m: Module) => groups?.find((g) => g.module === m.id);
  return (
    <>
      <Popover anchor={newAt} open={!!newAt} onClose={onNewClose} align="end" width={340}>
        <div className="menu templates">
          <div className="mhead">{t("Common")}</div>
          {templates.map((tpl) => (
            <button key={tpl.name} onClick={() => use(tpl)}>
              <span className="tname">{t(tpl.name)}{added.has(t(tpl.name)) && <span className="badge muted">{t("Added")}</span>}</span>
              <span className="thint">{t(tpl.hint)}</span>
            </button>
          ))}
          <hr />
          <div className="mhead">{t("Route a service")}</div>
          {services.map((svc) => (
            <button key={svc.name} onClick={() => route(svc)}>
              <span className="tname">{t(svc.name)}{routed.has(svc.name) && <span className="badge muted">{t("Added")}</span>}</span>
              <span className="thint">{t(svc.hint)}</span>
            </button>
          ))}
          <hr />
          <button onClick={() => use(null)}><span className="tname">{t("Blank module…")}</span></button>
        </div>
      </Popover>
      {draft && (
        <div className="list modules">
          {draft.route
            ? <RouteEditor module={draft} onCancel={() => setDraft(null)} onSave={add} onYAML={setDraft} />
            : <ModuleEditor module={draft} onCancel={() => setDraft(null)} onSave={add} />}
        </div>
      )}
      {mods.length === 0 && !draft ? (
        <div className="list modules-empty">
          <div className="lead">
            <b>{t("Change every profile, and keep it across updates")}</b>
            {t("Modules add DNS, hosts, rules and more over the profile in use. The app's ports, mode and TUN still win. Start with one of these:")}
          </div>
          {templates.map((tpl, i) => (
            <button className="row click" key={tpl.name} style={{ ["--i" as string]: i }} onClick={() => use(tpl)}>
              <div className="who"><div className="name">{t(tpl.name)}</div><div className="sub">{t(tpl.hint)}</div></div>
              <span className="btn small">{t("Add…")}</span>
            </button>
          ))}
          <div className="sect" style={{ ["--i" as string]: templates.length }}>{t("Route a service")}</div>
          {services.map((svc, i) => (
            <button className="row click" key={svc.name} style={{ ["--i" as string]: templates.length + 1 + i }} onClick={() => route(svc)}>
              <div className="who"><div className="name">{t(svc.name)}</div><div className="sub">{t(svc.hint)}</div></div>
              <span className="btn small">{t("Add…")}</span>
            </button>
          ))}
          <button className="row click blank" onClick={() => use(null)}>{t("Blank module…")}</button>
        </div>
      ) : mods.length > 0 && (
        <div className="list modules">
          {mods.map((m, i) => (
            <ModuleRow key={m.id} m={m} group={ownGroup(m)} onPick={(g, n) => select(g.name, n)} open={open === m.id} flash={flash === m.name} first={i === 0} last={i === mods.length - 1}
              onOpen={() => { setDraft(null); setOpen(open === m.id ? "" : m.id); }}
              onToggle={(on) => save(mods.map((o) => (o.id === m.id ? { ...o, enabled: on } : o)))}
              onMove={(d) => move(i, d)}
              onRemove={() => save(mods.filter((o) => o.id !== m.id))}
              onSave={async (n) => { if (await save(mods.map((o) => (o.id === m.id ? n : o)))) { setOpen(""); toast(t("Saved")); } }} />
          ))}
        </div>
      )}
    </>
  );
}

function ModuleRow({ m, group, onPick, open, flash, first, last, onOpen, onToggle, onMove, onRemove, onSave }: {
  m: Module; group?: Group; onPick: (g: Group, name: string) => void; open: boolean; flash: boolean; first: boolean; last: boolean;
  onOpen: () => void; onToggle: (on: boolean) => Promise<unknown>; onMove: (d: number) => void; onRemove: () => void; onSave: (m: Module) => Promise<void>;
}) {
  const t = useT();
  const { keys } = useModuleValidation(m.route ? "" : m.body);
  const regions = useRegions(!!m.route);
  const profile = useStore((s) => s.state?.profile ?? "");
  // a route taken to YAML, not saved until it is
  const [yaml, setYAML] = useState<Module | null>(null);
  useEffect(() => { if (!open) setYAML(null); }, [open]);
  const [armed, setArmed] = useState(false);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);
  return (
    <div className={"module" + (open ? " open" : "") + (m.enabled ? "" : " off")}>
      <div className={"row click" + (flash ? " flash" : "")} onClick={onOpen}>
        <div className="who">
          <div className="name">{m.name}</div>
          {m.route
            ? <div className="sub">{routeSummary(m.route, profile, regions, t)}</div>
            : <div className="sub mono">{keys.length ? keys.join(" · ") : t("Empty")}</div>}
        </div>
        <div className="end" onClick={(e) => e.stopPropagation()}>
          {group && (refusing(group)
            ? <button className="node-pick warn" title={t("Its connections are refused until it has a node in the profile in use.")} onClick={() => !open && onOpen()}>
                {m.route?.pick && !picksOf(m.route, profile).length ? t("Pick nodes for this profile") : t("No node")}
              </button>
            : <NodePicker group={group} onPick={(n) => onPick(group, n)} />)}
          <button className="icon" disabled={first} title={t("Move up")} onClick={() => onMove(-1)}><Arrow dir="up" size={12} /></button>
          <button className="icon" disabled={last} title={t("Move down")} onClick={() => onMove(1)}><Arrow dir="down" size={12} /></button>
          {armed
            ? <button className="btn small danger armed" onClick={onRemove}>{t("Click again to remove")}</button>
            : <button className="icon" title={t("Delete")} onClick={() => setArmed(true)}><Close size={12} /></button>}
          <Switch on={m.enabled} onChange={onToggle} label={m.name} />
        </div>
      </div>
      <Fold open={open}>
        {m.route && !yaml
          ? <RouteEditor module={m} onCancel={onOpen} onSave={onSave} onYAML={setYAML} />
          : <ModuleEditor key={yaml ? "yaml" : "body"} module={yaml ?? m} converted={!!yaml} onCancel={onOpen} onSave={onSave} />}
      </Fold>
    </div>
  );
}

// Keep validation tied to the exact source; an earlier successful check
// must not allow a changed draft to be saved while its check is pending.
function useModuleValidation(body: string): { pending: boolean; problem: string; keys: string[] } {
  const [out, setOut] = useState<{ body: string; problem: string; keys: string[] } | null>(null);
  useEffect(() => {
    let live = true;
    const id = setTimeout(() => {
      P.ModuleKeys(body).then((keys) => live && setOut({ body, problem: "", keys: keys ?? [] }), (e) => live && setOut({ body, problem: errText(e), keys: [] }));
    }, 250);
    return () => { live = false; clearTimeout(id); };
  }, [body]);
  return { pending: out?.body !== body, problem: out?.body === body ? out.problem : "", keys: out?.body === body ? out.keys : [] };
}

function ModuleEditor({ module, converted, onCancel, onSave }: { module: Module; converted?: boolean; onCancel: () => void; onSave: (m: Module) => Promise<void> }) {
  const t = useT();
  const [name, setName] = useState(module.name);
  const [body, setBody] = useState(module.body);
  const [busy, setBusy] = useState(false);
  const { pending, problem } = useModuleValidation(body);
  const nameRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  // a new one starts at its name, or at the example to fill in
  useEffect(() => { if (!module.id) setTimeout(() => (module.name ? bodyRef.current : nameRef.current)?.focus(), 60); }, []);
  const dirty = converted || name.trim() !== module.name || body !== module.body;
  const submit = async () => {
    if (busy || pending || !name.trim() || problem) return;
    setBusy(true);
    try { await onSave({ ...module, name: name.trim(), body }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  return (
    <form className="module-editor stagger" onSubmit={(e) => { e.preventDefault(); submit(); }}
      onKeyDown={(e) => { if (e.key === "s" && e.metaKey) { e.preventDefault(); submit(); } if (e.key === "Escape") onCancel(); }}>
      <input ref={nameRef} className="input" placeholder={t("Module name")} value={name} onChange={(e) => setName(e.target.value)} />
      <textarea ref={bodyRef} className="input" aria-label={t("Module YAML")} aria-invalid={!!problem} aria-describedby="module-yaml-status" placeholder={EXAMPLE} rows={Math.min(18, Math.max(8, body.split("\n").length + 1))} value={body} spellCheck={false}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          // a tab indents, as YAML wants spaces
          if (e.key !== "Tab") return;
          e.preventDefault();
          const el = e.currentTarget, a = el.selectionStart;
          setBody(body.slice(0, a) + "  " + body.slice(el.selectionEnd));
          requestAnimationFrame(() => el.setSelectionRange(a + 2, a + 2));
        }} />
      <div id="module-yaml-status" className={"hint yaml-status" + (problem ? " err" : "")} role="status">
        {pending ? t("Checking YAML…") : problem || t("YAML syntax valid")}
      </div>
      <div className="foot">
        <span className="hint">{t("Mappings merge; +key puts a list first, key+ last, key! replaces. ⌘S saves.")}</span>
        <div className="grow" />
        <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
        <button type="submit" className="btn small primary" disabled={busy || pending || !name.trim() || !!problem || (!!module.id && !dirty)}>{busy ? t("Checking…") : module.id ? t("Save") : t("Add")}</button>
      </div>
    </form>
  );
}

type T = ReturnType<typeof useT>;

const POLICIES = ["select", "url-test", "DIRECT"] as const;
const policyLabel = (p: string, t: T) => (p === "url-test" ? t("Fastest") : p === "DIRECT" ? t("Direct") : t("Choose a node"));

type Scope = "all" | "region" | "pick";
const scopeOf = (r: Route): Scope => (r.region ? "region" : r.pick ? "pick" : "all");
// the nodes picked for a profile; picks are a profile's, keywords all's
const picksOf = (r: Route, profile: string) => r.nodes?.[profile] ?? [];

// "Choose a node · Hong Kong": how the route goes, for its row
function routeSummary(r: Route, profile: string, regions: RegionNodes[], t: T) {
  const reg = regions.find((x) => x.key === r.region);
  const n = picksOf(r, profile).length + (r.keywords?.length ?? 0);
  const scope = reg ? t(reg.name) : r.pick ? (n ? t("{n} picked", { n }) : t("None picked here")) : t("All nodes");
  return [policyLabel(r.policy, t), r.policy !== "DIRECT" && scope].filter(Boolean).join(" · ");
}

// takes is Route.Takes in Go, for the nodes the editor lists
function takes(picks: string[], keywords: string[], name: string) {
  const low = name.toLowerCase();
  return picks.includes(name) || keywords.some((k) => low.includes(k.toLowerCase()));
}

// useNodes is the running profile's nodes; null with the core stopped.
function useNodes() {
  const [nodes, setNodes] = useState<Node[] | null>(null);
  useEffect(() => { P.RouteNodes().then((n) => setNodes(n)).catch(() => setNodes(null)); }, []);
  return nodes;
}

// useRegions is the regions a route's group can take, each with how many
// of the running profile's nodes it would.
function useRegions(want = true) {
  const [regions, setRegions] = useState<RegionNodes[]>([]);
  useEffect(() => { if (want) P.RouteRegions().then((r) => setRegions(r ?? [])).catch(() => {}); }, [want]);
  return regions;
}

// a route's group with no node in the profile holds REJECT alone
const refusing = (g: Group) => g.members?.length === 1 && g.members[0].name === "REJECT";

// NodePicker is a route group's node, on its row: a choice for a group to
// choose from, the one it picked for the fastest.
function NodePicker({ group, onPick }: { group: Group; onPick: (name: string) => void }) {
  const t = useT();
  const [at, setAt] = useState<HTMLElement | null>(null);
  const now = (group.members ?? []).find((x) => x.name === group.now);
  const pick = group.type === "Selector";
  return (
    <>
      <button className={"node-pick" + (pick ? "" : " fixed") + (at ? " on" : "")} title={pick ? t("Choose a node") : t("Picked by the latency test")}
        onClick={(e) => pick && setAt(at ? null : e.currentTarget)}>
        <span className="nname">{group.now || "—"}</span>
        {now && now.delay !== 0 && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay)}</span>}
        {pick && <Chevron size={10} className="chev" />}
      </button>
      <Popover anchor={at} open={!!at} onClose={() => setAt(null)} align="end" width={280}>
        <div className="menu node-menu">
          {(group.members ?? []).map((x) => (
            <button key={x.name} className={x.name === group.now ? "on" : ""} onClick={() => { setAt(null); if (x.name !== group.now) onPick(x.name); }}>
              <span className="nname">{x.name}</span>
              <span className={"delay " + delayClass(x.delay)}>{fmtDelay(x.delay)}</span>
            </button>
          ))}
        </div>
      </Popover>
    </>
  );
}

// RouteEditor writes a route as choices, not YAML: how the service goes,
// and through which nodes: all, a region's, or ones picked by name and by
// keyword. The YAML it makes can be taken to edit freely.
function RouteEditor({ module, onCancel, onSave, onYAML }: {
  module: Module; onCancel: () => void; onSave: (m: Module) => Promise<void>; onYAML: (m: Module) => void;
}) {
  const t = useT();
  const [name, setName] = useState(module.name);
  const [r, setR] = useState<Route>(module.route!);
  const [scope, setScope] = useState<Scope>(scopeOf(module.route!));
  const [busy, setBusy] = useState(false);
  const [query, setQuery] = useState("");
  const [keyword, setKeyword] = useState("");
  const regions = useRegions();
  const nodes = useNodes();
  // the nodes are picked for the profile in use; other profiles' stay
  const profile = useStore((s) => s.state?.profile ?? "");
  const picks = picksOf(r, profile);
  const keywords = r.keywords ?? [];
  const setPicks = (next: string[]) => setR({ ...r, nodes: { ...r.nodes, [profile]: next } });
  // what is saved: only the chosen scope's part
  const pick = scope === "pick";
  const out: Route = {
    service: r.service, policy: r.policy,
    region: scope === "region" ? r.region || regions[0]?.key || "" : "",
    pick,
    nodes: pick ? Object.fromEntries(Object.entries(r.nodes ?? {}).filter(([, v]) => v?.length)) : {},
    keywords: pick ? keywords : [],
  };
  const was = module.route!;
  const dirty = name.trim() !== module.name || JSON.stringify(out) !== JSON.stringify({
    service: was.service, policy: was.policy, region: was.region ?? "", pick: !!was.pick, nodes: was.nodes ?? {}, keywords: was.keywords ?? [],
  });
  const empty = r.policy !== "DIRECT" && pick && !picks.length && !keywords.length;
  const submit = async () => {
    if (busy || !name.trim() || empty) return;
    setBusy(true);
    try { await onSave({ ...module, name: name.trim(), route: out }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  const asYAML = async () => {
    try { onYAML({ ...module, name: name.trim() || module.name, route: null, body: await P.RouteBody(out) }); } catch (e) { toastError(e); }
  };
  // picking from a region starts with its nodes ticked, to untick the
  // ones that won't do
  const pickScope = (next: Scope) => {
    if (next === "pick" && scope === "region" && !picks.length && !keywords.length && nodes) {
      setPicks(nodes.filter((n) => n.region === out.region).map((n) => n.name));
    }
    setScope(next);
  };
  const toggle = (n: string) => setPicks(picks.includes(n) ? picks.filter((x) => x !== n) : [...picks, n]);
  const addKeyword = () => {
    const k = keyword.trim();
    if (k && !keywords.includes(k)) setR({ ...r, keywords: [...keywords, k] });
    setKeyword("");
  };
  const reg = regions.find((x) => x.key === out.region);
  const names = new Set(nodes?.map((n) => n.name));
  // picked for this profile, but gone from it since (renamed or dropped)
  const gone = picks.filter((n) => nodes && !names.has(n));
  const count = scope === "region" ? reg?.count ?? -1 : pick ? (nodes ? nodes.filter((n) => takes(picks, keywords, n.name)).length : -1) : nodes?.length ?? -1;
  const shown = (nodes ?? []).filter((n) => !query || n.name.toLowerCase().includes(query.toLowerCase()));
  return (
    <form className="module-editor route-editor stagger" onSubmit={(e) => { e.preventDefault(); submit(); }}
      onKeyDown={(e) => { if (e.key === "s" && e.metaKey) { e.preventDefault(); submit(); } if (e.key === "Escape") onCancel(); }}>
      <input className="input" placeholder={t("Module name")} value={name} onChange={(e) => setName(e.target.value)} />
      <div className="field">
        <span className="label">{t("Goes")}</span>
        <Segmented className="track small" value={r.policy} onChange={(policy) => setR({ ...r, policy })}
          options={POLICIES.map((p) => ({ value: p, label: policyLabel(p, t) }))} />
      </div>
      <Fold open={r.policy !== "DIRECT"}>
        <div className="field">
          <span className="label">{t("Through")}</span>
          <div className="scope">
            <Segmented className="track small" value={scope} onChange={pickScope} options={[
              { value: "all", label: t("All nodes") }, { value: "region", label: t("A region") }, { value: "pick", label: t("Picked nodes") },
            ]} />
            <Fold open={scope === "region"}>
              <div className="chips">
                {regions.map((x) => (
                  <button type="button" key={x.key} className={"chip" + (out.region === x.key ? " on" : "") + (x.count === 0 ? " none" : "")}
                    onClick={() => setR({ ...r, region: x.key })}>
                    {t(x.name)}{x.count >= 0 && <span className="count">{x.count}</span>}
                  </button>
                ))}
              </div>
            </Fold>
            <Fold open={scope === "pick"}>
              <div className="pick">
                {nodes === null ? <div className="pick-note">{t("Start the core to list the profile's nodes. Keywords work without it.")}</div> : (
                  <>
                    <div className="pick-note">{t("Ticked for this profile only; each profile has its own. Keywords below hold for all.")}</div>
                    <label className="search"><Search size={13} /><input placeholder={t("Search nodes")} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") e.preventDefault(); }} /></label>
                    <div className="pick-list">
                      {shown.map((n) => {
                        const on = picks.includes(n.name);
                        const byKeyword = !on && takes([], keywords, n.name);
                        return (
                          <label key={n.name} className={"pick-row" + (on || byKeyword ? " on" : "")}>
                            <input type="checkbox" checked={on || byKeyword} disabled={byKeyword} onChange={() => toggle(n.name)} />
                            <span className="nname">{n.name}</span>
                            {byKeyword && <span className="badge muted">{t("By keyword")}</span>}
                            <span className={"delay " + delayClass(n.delay)}>{fmtDelay(n.delay)}</span>
                          </label>
                        );
                      })}
                      {gone.map((n) => (
                        <label key={n} className="pick-row gone" title={t("The profile no longer has this node")}>
                          <input type="checkbox" checked onChange={() => toggle(n)} />
                          <span className="nname">{n}</span>
                          <span className="badge muted">{t("Gone")}</span>
                        </label>
                      ))}
                    </div>
                  </>
                )}
                <div className="keywords">
                  <span className="klabel">{t("Or names containing")}</span>
                  {keywords.map((k) => (
                    <span key={k} className="kw">{k}<button type="button" title={t("Delete")} onClick={() => setR({ ...r, keywords: keywords.filter((x) => x !== k) })}><Close size={9} /></button></span>
                  ))}
                  <input className="kw-input" placeholder={t("IPLC, 专线 …")} value={keyword} onChange={(e) => setKeyword(e.target.value)}
                    onBlur={addKeyword} onKeyDown={(e) => { if (e.key === "Enter" || e.key === ",") { e.preventDefault(); addKeyword(); } else if (e.key === "Backspace" && !keyword && keywords.length) setR({ ...r, keywords: keywords.slice(0, -1) }); }} />
                </div>
              </div>
            </Fold>
          </div>
        </div>
      </Fold>
      <div className="foot">
        <span className={"hint" + ((count === 0 || empty) && r.policy !== "DIRECT" ? " err" : "")}>
          {r.policy === "DIRECT" ? t("{service} skips the proxy.", { service: r.service })
            : empty ? t("Pick nodes for this profile, or add a keyword. Until then {service} is refused.", { service: r.service })
            : count === 0 ? t("None of these nodes is in the profile in use, so {service} is refused rather than going direct.", { service: r.service })
            : count > 0 && scope !== "all" ? t("{n} nodes of the profile in use. With none of them left, {service} is refused rather than going direct.", { n: count, service: r.service })
            : r.policy === "select" ? t("A {service} group of its own; choose its node on this row, or on the Proxies page.", { service: r.service })
            : t("A {service} group of its own, on the node with the lowest latency.", { service: r.service })}
        </span>
        <div className="grow" />
        <button type="button" className="btn small" onClick={asYAML}>{t("Edit as YAML")}</button>
        <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
        <button type="submit" className="btn small primary" disabled={busy || !name.trim() || empty || (!!module.id && !dirty)}>{busy ? t("Checking…") : module.id ? t("Save") : t("Add")}</button>
      </div>
    </form>
  );
}
