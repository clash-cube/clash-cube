import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { Profiles as P } from "../api";
import type { Module, Route, Service, Template } from "../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { Node, RegionNodes } from "../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import type { Group } from "../api";
import { useGroups } from "../useGroups";
import { useStore } from "../store";
import { delayClass, fmtDelay, nodeLabel } from "../format";
import { Popover, Menu } from "./Popover";
import { Segmented } from "./Segmented";
import { Switch } from "./Switch";
import { Fold } from "./Fold";
import { Chevron, Close, Grip, More, Plus, Search } from "./Icons";
import { toast, toastError, errText } from "./Toast";

const EXAMPLE = `dns:
  +fake-ip-filter:
    - "+.lan"
hosts:
  router.lan: 192.168.1.1
prepend-rules:
  - DOMAIN-SUFFIX,example.com,DIRECT`;

// Modules are YAML laid over the profiles in order, as Surge's are: the
// global ones over every profile, then the profile in use's own over it
// alone. Each is a row with its switch, unrolling into its editor when
// clicked. A route sends a service through a group of its own; its row
// picks the node.
export function Modules() {
  const t = useT();
  const profile = useStore((s) => s.state?.profile ?? "");
  const profileName = useStore((s) => s.profiles.find((p) => p.id === profile)?.name ?? "");
  const [mods, setMods] = useState<Module[] | null>(null);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const { groups, select, load: loadGroups } = useGroups();
  const nodes = useNodes();
  const [open, setOpen] = useState("");
  // a new module being written, blank or from a template; its profile says
  // which section it goes in
  const [draft, setDraft] = useState<Module | null>(null);
  const [flash, setFlash] = useState("");
  // the add menu, and the section it adds to: "" for global
  const [newAt, setNewAt] = useState<{ at: HTMLElement; profile: string } | null>(null);
  const load = () => P.Modules().then((m) => setMods(m ?? [])).catch(toastError);
  useEffect(() => {
    load();
    P.ModuleTemplates().then((ts) => setTemplates(ts ?? [])).catch(() => {});
    P.RouteServices().then((ss) => setServices(ss ?? [])).catch(() => {});
  }, []);
  useEffect(() => { setDraft(null); setOpen(""); }, [profile]);
  const global = mods?.filter((m) => !m.profile) ?? [];
  const own = profile ? mods?.filter((m) => m.profile === profile) ?? [] : [];
  // the sections, each a list the rows are dragged in and between
  const sections = profile ? [global, own] : [global];
  const sort = useSort(sections.map((x) => x.length), (from, to) => drop(from, to));

  // the core checks every change; one it refuses leaves the list as it was
  const save = async (next: Module[]) => {
    const before = mods;
    setMods(next);
    try { await P.SetModules(next); await load(); loadGroups(); return true; } catch (e) { setMods(before); toastError(e); return false; }
  };
  // a template opens in the editor, and is added only once saved there
  const use = (tpl: Template | null, to: string) => {
    setNewAt(null);
    setOpen("");
    setDraft({ id: "", name: tpl ? t(tpl.name) : "", enabled: true, body: tpl?.body ?? "", profile: to });
  };
  const route = (svc: Service, to: string) => {
    setNewAt(null);
    setOpen("");
    setDraft({ id: "", name: t(svc.name), enabled: true, body: "", profile: to, route: { service: svc.name, policy: "select", region: svc.region ?? "" } });
  };

  if (!mods) return null;
  const shown = [...global, ...own];
  const added = new Set(shown.map((m) => m.name));
  const routed = new Set(shown.flatMap((m) => (m.route ? [m.route.service] : [])));
  // puts a module at a place in a section, taking it to that section's
  // scope. To global keeps a route's picks, which are by profile already.
  function drop(from: Spot, to: Spot) {
    if (!mods) return;
    const m = sections[from.s][from.i];
    const scope = to.s === 0 ? "" : profile;
    const rest = mods.filter((o) => o.id !== m.id);
    const target = sections[to.s].filter((o) => o.id !== m.id);
    const at = to.i < target.length ? rest.indexOf(target[to.i])
      : target.length ? rest.indexOf(target[target.length - 1]) + 1 : rest.length;
    rest.splice(at, 0, { ...m, profile: scope });
    save(rest).then((ok) => ok && from.s !== to.s && toast(scope
      ? t("{name} is now laid over {profile} alone", { name: m.name, profile: profileName })
      : t("{name} is now laid over every profile", { name: m.name })));
  }
  const setScope = (m: Module, s: number) => drop({ s: m.profile ? 1 : 0, i: sections[m.profile ? 1 : 0].indexOf(m) }, { s, i: sections[s].length });
  const add = async (m: Module) => {
    if (!(await save([...mods, m]))) return;
    setDraft(null);
    toast(t("Added {name}", { name: m.name }));
    setFlash(m.name); setTimeout(() => setFlash(""), 900);
  };
  const ownGroup = (m: Module) => groups?.find((g) => g.module === m.id);
  const draftIn = (to: string) => draft && (draft.profile ?? "") === to && (
    <div className="list modules">
      {draft.route
        ? <RouteModuleEditor module={draft} onCancel={() => setDraft(null)} onSave={add} />
        : <ModuleEditor module={draft} onCancel={() => setDraft(null)} onSave={add} />}
    </div>
  );
  // a section's rows, or what it says empty, which a row can be dropped on
  const rows = (s: number, empty: string) => sections[s].length === 0
    ? <div ref={sort.zone(s)} className={"list user-rules-empty" + sort.zoneClass(s)}>{empty}</div>
    : (
    <div ref={sort.zone(s)} style={sort.zoneStyle(s)} className={"list modules" + (sort.dragging ? " sorting" : "")}>
      {sections[s].map((m, i) => (
        <ModuleRow key={m.id} m={m} group={ownGroup(m)} nodes={nodes} onPick={(g, n) => select(g.name, n)} open={open === m.id} flash={flash === m.name}
          sort={sort} s={s} i={i} profileName={profile ? profileName : ""}
          onOpen={() => { setDraft(null); setOpen(open === m.id ? "" : m.id); }}
          onToggle={(on) => save(mods.map((o) => (o.id === m.id ? { ...o, enabled: on } : o)))}
          onScope={() => setScope(m, m.profile ? 0 : 1)}
          onRemove={() => save(mods.filter((o) => o.id !== m.id))}
          onSave={async (n) => { if (await save(mods.map((o) => (o.id === m.id ? n : o)))) { setOpen(""); toast(t("Saved")); } }} />
      ))}
    </div>
  );
  const head = (title: string, to: string) => (
    <div className="section-title user-rules-title modules-title">
      <span>{title}</span>
      <button className="btn small" onClick={(e) => setNewAt(newAt ? null : { at: e.currentTarget, profile: to })}><Plus size={12} />{t("New module")}</button>
    </div>
  );
  const to = newAt?.profile ?? "";
  return (
    <>
      <Popover anchor={newAt?.at ?? null} open={!!newAt} onClose={() => setNewAt(null)} align="end" width={340}>
        <div className="menu templates">
          <div className="mhead">{t("Common")}</div>
          {templates.map((tpl) => (
            <button key={tpl.name} onClick={() => use(tpl, to)}>
              <span className="tname">{t(tpl.name)}{added.has(t(tpl.name)) && <span className="badge muted">{t("Added")}</span>}</span>
              <span className="thint">{t(tpl.hint)}</span>
            </button>
          ))}
          <hr />
          <div className="mhead">{t("Route a service")}</div>
          {services.map((svc) => (
            <button key={svc.name} onClick={() => route(svc, to)}>
              <span className="tname">{t(svc.name)}{routed.has(svc.name) && <span className="badge muted">{t("Added")}</span>}</span>
              <span className="thint">{t(svc.hint)}</span>
            </button>
          ))}
          <hr />
          <button onClick={() => use(null, to)}><span className="tname">{t("Blank module…")}</span></button>
        </div>
      </Popover>
      {head(t("Global modules"), "")}
      {draftIn("")}
      {shown.length === 0 && !draft ? (
        <div className="list modules-empty">
          <div className="lead">
            <b>{t("Change every profile, and keep it across updates")}</b>
            {t("Global modules add DNS, hosts, rules and more over every profile. The app's ports, mode and TUN still win. Start with one of these:")}
          </div>
          {templates.map((tpl, i) => (
            <button className="row click" key={tpl.name} style={{ ["--i" as string]: i }} onClick={() => use(tpl, "")}>
              <div className="who"><div className="name">{t(tpl.name)}</div><div className="sub">{t(tpl.hint)}</div></div>
              <span className="btn small">{t("Add…")}</span>
            </button>
          ))}
          <div className="sect" style={{ ["--i" as string]: templates.length }}>{t("Route a service")}</div>
          {services.map((svc, i) => (
            <button className="row click" key={svc.name} style={{ ["--i" as string]: templates.length + 1 + i }} onClick={() => route(svc, "")}>
              <div className="who"><div className="name">{t(svc.name)}</div><div className="sub">{t(svc.hint)}</div></div>
              <span className="btn small">{t("Add…")}</span>
            </button>
          ))}
          <button className="row click blank" onClick={() => use(null, "")}>{t("Blank module…")}</button>
        </div>
      ) : (global.length > 0 || draft?.profile !== "") && rows(0, t("Global modules are laid over every profile."))}
      {profile && <>
        {head(t("Modules of {profile}", { profile: profileName }), profile)}
        {draftIn(profile)}
        {(own.length > 0 || draft?.profile !== profile) && rows(1, t("Modules here are laid over this profile alone, after the global ones, so their rules go first. Each profile has its own."))}
      </>}
    </>
  );
}

type Sort = ReturnType<typeof useSort>;
// a row's place: its section, and where in it
type Spot = { s: number; i: number };

// useSort moves rows by their handles, within a section or into another.
// Within one, the held row follows the pointer and the others make room;
// over another, a line shows where it lands. It lands on release. The
// arrow keys move it a place at a time, past a section's ends into the
// next.
function useSort(counts: number[], onDrop: (from: Spot, to: Spot) => void) {
  const rows = useRef<(HTMLElement | null)[][]>([]);
  const zones = useRef<(HTMLElement | null)[]>([]);
  type Box = { top: number; height: number };
  const start = useRef<{ y: number; rows: Box[][]; zones: ({ top: number; bottom: number } | null)[] } | null>(null);
  const [drag, setDrag] = useState<{ from: Spot; to: Spot; dy: number } | null>(null);
  const end = (drop: boolean) => {
    const d = drag;
    start.current = null;
    setDrag(null);
    if (drop && d && (d.from.s !== d.to.s || d.from.i !== d.to.i)) onDrop(d.from, d.to);
  };
  const box = (el: HTMLElement | null) => { const r = el!.getBoundingClientRect(); return { top: r.top, height: r.height }; };
  const handle = (s: number, i: number) => ({
    onClick: (e: React.MouseEvent) => e.stopPropagation(),
    onPointerDown: (e: React.PointerEvent) => {
      if (e.button !== 0) return;
      const sole = counts.length < 2 && counts[s] < 2;
      if (sole) return;
      e.preventDefault();
      e.currentTarget.setPointerCapture(e.pointerId);
      start.current = {
        y: e.clientY,
        rows: counts.map((n, k) => (rows.current[k] ?? []).slice(0, n).map(box)),
        // a section that isn't shown, as while a module is added to it, takes none
        zones: counts.map((_, k) => { const r = zones.current[k]?.getBoundingClientRect(); return r ? { top: r.top, bottom: r.bottom } : null; }),
      };
      setDrag({ from: { s, i }, to: { s, i }, dy: 0 });
    },
    onPointerMove: (e: React.PointerEvent) => {
      const st = start.current;
      if (!st) return;
      const dy = e.clientY - st.y;
      const held = st.rows[s][i];
      const mid = held.top + dy + held.height / 2;
      // the section nearest to it
      const far = (k: number) => { const z = st.zones[k]; return z ? Math.max(0, z.top - mid, mid - z.bottom) : Infinity; };
      const to = counts.reduce((best, _, k) => (far(k) < far(best) ? k : best), s);
      // its place is after every other row there whose middle it has passed
      const at = st.rows[to].filter((r, j) => !(to === s && j === i) && r.top + r.height / 2 < mid).length;
      setDrag({ from: { s, i }, to: { s: to, i: at }, dy });
    },
    onPointerUp: () => end(true),
    onLostPointerCapture: () => start.current && end(false),
    onKeyDown: (e: React.KeyboardEvent) => {
      const d = e.key === "ArrowUp" ? -1 : e.key === "ArrowDown" ? 1 : 0;
      if (!d) return;
      const to = i + d >= 0 && i + d < counts[s] ? { s, i: i + d }
        : d > 0 && s + 1 < counts.length ? { s: s + 1, i: 0 }
        : d < 0 && s > 0 ? { s: s - 1, i: counts[s - 1] } : null;
      if (!to) return;
      e.preventDefault();
      onDrop({ s, i }, to);
    },
  });
  const across = !!drag && drag.from.s !== drag.to.s;
  // the held row where the pointer has it; in its own section, the ones
  // it passed a row's height the other way; leaving it, the ones after it
  // close up and those after its place in the other open a gap for it
  const style = (s: number, j: number): React.CSSProperties | undefined => {
    if (!drag || !start.current) return;
    const { from, to, dy } = drag, h = start.current.rows[from.s][from.i].height;
    if (s !== from.s) return across && s === to.s && j >= to.i ? { transform: `translateY(${h}px)` } : undefined;
    // a section above that opened for it pushed this one down
    if (j === from.i) return { transform: `translateY(${across && to.s < from.s ? dy - h : dy}px)` };
    if (across) return j > from.i ? { transform: `translateY(${-h}px)` } : undefined;
    if (from.i < j && j <= to.i) return { transform: `translateY(${-h}px)` };
    if (to.i <= j && j < from.i) return { transform: `translateY(${h}px)` };
  };
  const rowClass = (s: number, j: number) => (drag && drag.from.s === s && drag.from.i === j ? " held" : "");
  // the section it is over, a row's height taller to make room for it
  const zoneStyle = (s: number): React.CSSProperties | undefined =>
    across && drag!.to.s === s && counts[s] > 0 ? { paddingBottom: start.current!.rows[drag!.from.s][drag!.from.i].height } : undefined;
  const zoneClass = (s: number) => (across && drag!.to.s === s && counts[s] === 0 ? " drop-target" : "");
  const row = (s: number, i: number) => (el: HTMLElement | null) => { (rows.current[s] ??= [])[i] = el; };
  const zone = (s: number) => (el: HTMLElement | null) => { zones.current[s] = el; };
  return { row, zone, handle, style, rowClass, zoneStyle, zoneClass, dragging: !!drag };
}

function ModuleRow({ m, group, nodes, onPick, open, flash, sort, s, i, profileName, onOpen, onToggle, onScope, onRemove, onSave }: {
  m: Module; group?: Group; nodes: Node[] | null; onPick: (g: Group, name: string) => void; open: boolean; flash: boolean; sort: Sort; s: number; i: number;
  profileName: string; onOpen: () => void; onToggle: (on: boolean) => Promise<unknown>; onScope: () => void; onRemove: () => void; onSave: (m: Module) => Promise<void>;
}) {
  const t = useT();
  const { keys } = useModuleValidation(m.route ? "" : m.body);
  const regions = useRegions(!!m.route);
  const profile = useStore((s) => s.state?.profile ?? "");
  // a front that left the profile refuses the route, as an empty scope does
  const front = m.route && m.route.policy !== "DIRECT" ? m.route.upstream?.[profile] ?? "" : "";
  const frontGone = !!front && !!nodes && !nodes.some((n) => n.name === front);
  const [armed, setArmed] = useState(false);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);
  const [menuAt, setMenuAt] = useState<HTMLElement | null>(null);
  return (
    <div ref={sort.row(s, i)} style={sort.style(s, i)}
      className={"module" + (open ? " open" : "") + (m.enabled ? "" : " off") + sort.rowClass(s, i)}>
      <div className={"row click" + (flash ? " flash" : "")} onClick={onOpen}>
        <button className="grip" title={t("Drag to reorder")} aria-label={t("Drag to reorder")} {...sort.handle(s, i)}><Grip size={14} /></button>
        <div className="who">
          <div className="name">{m.name}</div>
          {m.route
            ? frontGone
              ? <div className="sub warn">{t("Upstream {node} is gone; connections are refused", { node: front })}</div>
              : <div className="sub">{routeSummary(m.route, profile, regions, t)}</div>
            : <div className="sub mono">{keys.length ? keys.join(" · ") : t("Empty")}</div>}
        </div>
        <div className="end" onClick={(e) => e.stopPropagation()}>
          {group && (refusing(group)
            ? <button className="node-pick warn" title={t("Its connections are refused until it has a node in the profile in use.")} onClick={() => !open && onOpen()}>
                {m.route?.pick && !picksOf(m.route, profile).length ? t("Pick nodes for this profile") : t("No node")}
              </button>
            : <NodePicker group={group} onPick={(n) => onPick(group, n)} />)}
          {(m.profile || profileName) && <button className="icon" title={t("More")} onClick={(e) => setMenuAt(menuAt ? null : e.currentTarget)}><More size={12} /></button>}
          <Popover anchor={menuAt} open={!!menuAt} onClose={() => setMenuAt(null)} align="end">
            <Menu close={() => setMenuAt(null)} items={[m.profile
              ? { label: t("Make global"), onClick: onScope }
              : { label: t("Move to {profile}", { profile: profileName }), onClick: onScope }]} />
          </Popover>
          {armed
            ? <button className="btn small danger armed" onClick={onRemove}>{t("Click again to remove")}</button>
            : <button className="icon" title={t("Delete")} onClick={() => setArmed(true)}><Close size={12} /></button>}
          <Switch on={m.enabled} onChange={onToggle} label={m.name} />
        </div>
      </div>
      <Fold open={open}>
        {m.route
          ? <RouteModuleEditor module={m} open={open} onCancel={onOpen} onSave={onSave} />
          : <ModuleEditor module={m} onCancel={onOpen} onSave={onSave} />}
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

// RouteModuleEditor is a route's form, or the YAML it was taken to. Until
// that YAML is saved, the form can be gone back to as it was left.
function RouteModuleEditor({ module, open = true, onCancel, onSave }: {
  module: Module; open?: boolean; onCancel: () => void; onSave: (m: Module) => Promise<void>;
}) {
  const [form, setForm] = useState<RouteForm | undefined>();
  const [yaml, setYAML] = useState<Module | null>(null);
  useEffect(() => { if (!open) { setYAML(null); setForm(undefined); } }, [open]);
  return yaml
    ? <ModuleEditor key="yaml" module={yaml} converted onCancel={onCancel} onSave={onSave}
        onBack={(name) => { setForm((f) => f && { ...f, name }); setYAML(null); }} />
    : <RouteEditor key="form" module={module} form={form} onCancel={onCancel} onSave={onSave}
        onYAML={(m, f) => { setForm(f); setYAML(m); }} />;
}

function ModuleEditor({ module, converted, onBack, onCancel, onSave }: {
  module: Module; converted?: boolean; onBack?: (name: string) => void; onCancel: () => void; onSave: (m: Module) => Promise<void>;
}) {
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
  // going back drops the YAML's own edits, so a changed one asks first
  const [armed, setArmed] = useState(false);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);
  const back = () => (body === module.body || armed ? onBack!(name.trim() || module.name) : setArmed(true));
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
        {onBack && (armed
          ? <button type="button" className="btn small danger armed" onClick={back}>{t("Click again to discard YAML edits")}</button>
          : <button type="button" className="btn small" onClick={back}>{t("Back to form")}</button>)}
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
  return [policyLabel(r.policy, t), r.policy !== "DIRECT" && scope,
    r.policy !== "DIRECT" && r.upstream?.[profile] && t("Via {node}", { node: r.upstream[profile] })].filter(Boolean).join(" · ");
}

// takes is Route.Takes in Go, for the nodes the editor lists
function takes(picks: string[], keywords: string[], name: string) {
  const low = name.toLowerCase();
  return picks.includes(name) || keywords.some((k) => low.includes(k.toLowerCase()));
}

// useNodes is the running profile's nodes; null with the core stopped.
function useNodes() {
  const [nodes, setNodes] = useState<Node[] | null>(null);
  const profile = useStore((s) => s.state?.profile);
  const core = useStore((s) => s.state?.core);
  useEffect(() => {
    let active = true;
    setNodes(null);
    P.RouteNodes().then((n) => { if (active) setNodes(n); }).catch(() => { if (active) setNodes(null); });
    return () => { active = false; };
  }, [profile, core]);
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
        <span className="nname">{nodeLabel(group.now) || "—"}</span>
        {now && now.delay !== 0 && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay, t)}</span>}
        {pick && <Chevron size={10} className="chev" />}
      </button>
      <Popover anchor={at} open={!!at} onClose={() => setAt(null)} align="end" width={280}>
        <div className="menu node-menu">
          {(group.members ?? []).map((x) => (
            <button key={x.name} className={x.name === group.now ? "on" : ""} onClick={() => { setAt(null); if (x.name !== group.now) onPick(x.name); }}>
              <span className="nname">{nodeLabel(x.name)}</span>
              <span className={"delay " + delayClass(x.delay)}>{fmtDelay(x.delay, t)}</span>
            </button>
          ))}
        </div>
      </Popover>
    </>
  );
}

// UpstreamPicker is the node a route's exits dial through, chosen like a
// row's node: from a menu with each node's latency, searched when long.
function UpstreamPicker({ value, nodes, onChange }: { value: string; nodes: Node[] | null; onChange: (name: string) => void }) {
  const t = useT();
  const [at, setAt] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");
  const now = nodes?.find((n) => n.name === value);
  const gone = !!value && !!nodes && !now;
  const shown = (nodes ?? []).filter((n) => !query || n.name.toLowerCase().includes(query.toLowerCase()));
  const choose = (name: string) => { setAt(null); setQuery(""); if (name !== value) onChange(name); };
  return (
    <>
      <button type="button" className={"node-pick" + (value ? "" : " unset") + (gone ? " warn" : "") + (at ? " on" : "")} disabled={nodes === null && !value}
        title={nodes === null ? t("Start the core to list upstream nodes.") : t("Exits dial through this node, for this profile only")}
        onClick={(e) => setAt(at ? null : e.currentTarget)}>
        <span className="nname">{value || t("None")}</span>
        {gone ? <span className="badge muted">{t("Gone")}</span>
          : now && now.delay !== 0 && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay, t)}</span>}
        <Chevron size={10} className="chev" />
      </button>
      <Popover anchor={at} open={!!at} onClose={() => { setAt(null); setQuery(""); }} width={280}>
        <div className="menu node-menu upstream-menu">
          {(nodes?.length ?? 0) > 8 && <label className="search"><Search size={13} /><input autoFocus placeholder={t("Search nodes")} value={query} onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); if (shown[0]) choose(shown[0].name); } }} /></label>}
          {!query && <button type="button" className={value ? "" : "on"} onClick={() => choose("")}><span className="nname">{t("No upstream")}</span></button>}
          {!query && <hr />}
          {shown.map((n) => (
            <button type="button" key={n.name} className={n.name === value ? "on" : ""} onClick={() => choose(n.name)}>
              <span className="nname">{n.name}</span>
              <span className={"delay " + delayClass(n.delay)}>{fmtDelay(n.delay, t)}</span>
            </button>
          ))}
          {nodes === null && <div className="mnote">{t("Start the core to list upstream nodes.")}</div>}
        </div>
      </Popover>
    </>
  );
}

// RouteEditor writes a route as choices, not YAML: how the service goes,
// and through which nodes: all, a region's, or ones picked by name and by
// keyword. The YAML it makes can be taken to edit freely.
// RouteForm is a route editor's state, kept while its YAML is edited
type RouteForm = { name: string; route: Route; scope: Scope };

function RouteEditor({ module, form, onCancel, onSave, onYAML }: {
  module: Module; form?: RouteForm; onCancel: () => void; onSave: (m: Module) => Promise<void>; onYAML: (m: Module, form: RouteForm) => void;
}) {
  const t = useT();
  const [name, setName] = useState(form?.name ?? module.name);
  const [r, setR] = useState<Route>(form?.route ?? module.route!);
  const [scope, setScope] = useState<Scope>(form?.scope ?? scopeOf(module.route!));
  const [busy, setBusy] = useState(false);
  const [query, setQuery] = useState("");
  const [keyword, setKeyword] = useState("");
  const regions = useRegions();
  const nodes = useNodes();
  // the nodes are picked for the profile in use; other profiles' stay
  const profile = useStore((s) => s.state?.profile ?? "");
  const picks = picksOf(r, profile);
  const upstream = r.upstream?.[profile] ?? "";
  const keywords = r.keywords ?? [];
  const setPicks = (next: string[]) => setR({ ...r, nodes: { ...r.nodes, [profile]: next } });
  const setUpstream = (name: string) => {
    const next = { ...r.upstream };
    if (name) next[profile] = name; else delete next[profile];
    setR({ ...r, upstream: next });
  };
  // what is saved: only the chosen scope's part
  const pick = scope === "pick";
  const out: Route = {
    service: r.service, policy: r.policy,
    region: scope === "region" ? r.region || regions[0]?.key || "" : "",
    pick,
    nodes: pick ? Object.fromEntries(Object.entries(r.nodes ?? {}).filter(([, v]) => v?.length)) : {},
    keywords: pick ? keywords : [],
    upstream: r.upstream ?? {},
  };
  const was = module.route!;
  const dirty = name.trim() !== module.name || JSON.stringify(out) !== JSON.stringify({
    service: was.service, policy: was.policy, region: was.region ?? "", pick: !!was.pick, nodes: was.nodes ?? {}, keywords: was.keywords ?? [], upstream: was.upstream ?? {},
  });
  const empty = r.policy !== "DIRECT" && pick && !picks.length && !keywords.length;
  const submit = async () => {
    if (busy || !name.trim() || empty) return;
    setBusy(true);
    try { await onSave({ ...module, name: name.trim(), route: out }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  const asYAML = async () => {
    try { onYAML({ ...module, name: name.trim() || module.name, route: null, body: await P.RouteBody(out) }, { name, route: r, scope }); } catch (e) { toastError(e); }
  };
  // picking starts from none: most routes want one node or a few, and
  // the search with "Tick shown" takes a whole region when wanted
  const pickScope = setScope;
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
  const missing = !!upstream && !!nodes && !names.has(upstream);
  const exits = nodes?.filter((n) => n.name !== upstream);
  const count = exits ? exits.filter((n) => scope === "region" ? n.region === reg?.key : !pick || takes(picks, keywords, n.name)).length : -1;
  const shown = (nodes ?? []).filter((n) => !query || n.name.toLowerCase().includes(query.toLowerCase()));
  const tickable = shown.map((n) => n.name).filter((n) => n !== upstream && !picks.includes(n));
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
                    <div className="pick-bar">
                      <label className="search"><Search size={13} /><input placeholder={t("Search nodes")} value={query} onChange={(e) => setQuery(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") e.preventDefault(); }} /></label>
                      <span className="pick-count">{t("{n} picked", { n: picks.length })}</span>
                      <button type="button" className="link" disabled={!tickable.length} onClick={() => setPicks([...picks, ...tickable])}>{query ? t("Tick shown") : t("Tick all")}</button>
                      <button type="button" className="link" disabled={!picks.length} onClick={() => setPicks([])}>{t("Clear")}</button>
                    </div>
                    <div className="pick-list">
                      {shown.map((n) => {
                        const isUpstream = n.name === upstream;
                        const on = !isUpstream && picks.includes(n.name);
                        const byKeyword = !isUpstream && !on && takes([], keywords, n.name);
                        return (
                          <label key={n.name} className={"pick-row" + (on || byKeyword ? " on" : "")}>
                            <input type="checkbox" checked={on || byKeyword} disabled={byKeyword || isUpstream} onChange={() => toggle(n.name)} />
                            <span className="nname">{n.name}</span>
                            {byKeyword && <span className="badge muted">{t("By keyword")}</span>}
                            {isUpstream && <span className="badge muted">{t("Upstream")}</span>}
                            <span className={"delay " + delayClass(n.delay)}>{fmtDelay(n.delay, t)}</span>
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
        <div className="field">
          <span className="label">{t("Via")}</span>
          <div className="scope">
            <UpstreamPicker value={upstream} nodes={nodes} onChange={setUpstream} />
            {upstream && <div className={"route-path" + (missing ? " err" : "")}>
              {missing ? t("The upstream node is gone, so {service} is refused until it returns or you choose another.", { service: r.service })
                : <>{t("This Mac")}<span className="sep">→</span><b>{upstream}</b><span className="sep">→</span>{t("exit node")}<span className="sep">→</span>{r.service}</>}
            </div>}
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
