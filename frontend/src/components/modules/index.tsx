import { useEffect, useRef, useState } from "react";
import { useT } from "../../i18n";
import { Profiles as P } from "../../api";
import type { Module } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { Group } from "../../api";
import { useGroups } from "../../useGroups";
import { useStore } from "../../store";
import { Popover, Menu, type MenuItem } from "../Popover";
import { Switch } from "../Switch";
import { Fold } from "../Fold";
import { Close, Grip, More, Plus } from "../Icons";
import { toast, toastError } from "../Toast";
import { kindOf, MENU } from "./registry";
import { blank } from "./yaml";
import { useNodes } from "./shared";
import type { NewEntry, NewSection, RowCtx } from "./kinds";

// Modules are laid over the profiles in order, as Surge's are: the global
// ones over every profile, then the profile in use's own over it alone.
// Each is a row with its switch, unrolling into its editor when clicked.
// What a row shows and how it is edited is its kind's (registry.ts).
export function Modules() {
  const t = useT();
  const profile = useStore((s) => s.state?.profile ?? "");
  const profileName = useStore((s) => s.profiles.find((p) => p.id === profile)?.name ?? "");
  const [mods, setMods] = useState<Module[] | null>(null);
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
  useEffect(() => { load(); }, []);
  useEffect(() => { setDraft(null); setOpen(""); }, [profile]);
  // a module asked for from elsewhere, as a node's port from the proxies
  const pending = useStore((s) => s.moduleDraft);
  useEffect(() => {
    if (!pending) return;
    useStore.setState({ moduleDraft: null });
    setOpen(""); setDraft(pending);
  }, [pending]);
  const global = mods?.filter((m) => !m.profile) ?? [];
  const own = profile ? mods?.filter((m) => m.profile === profile) ?? [] : [];
  // every kind's entries, in the registry's order (hooks, so always all)
  const sectionsNew: NewSection[] = MENU.flatMap((k) => k.useNew?.([...global, ...own]) ?? []);
  // the sections, each a list the rows are dragged in and between
  const sections = profile ? [global, own] : [global];
  const sort = useSort(sections.map((x) => x.length), (from, to) => drop(from, to));

  // the core checks every change; one it refuses leaves the list as it was
  const save = async (next: Module[]) => {
    const before = mods;
    setMods(next);
    try { await P.SetModules(next); await load(); loadGroups(); return true; } catch (e) { setMods(before); toastError(e); return false; }
  };
  // an entry opens its draft in the editor, added only once saved there
  const use = async (e: NewEntry | null, to: string) => {
    setNewAt(null);
    setOpen("");
    try { setDraft(e ? await e.make(to) : blank(to)); } catch (err) { toastError(err); }
  };

  if (!mods) return null;
  const shown = [...global, ...own];
  // puts a module at a place in a section, taking it to that section's
  // scope. To global keeps a kind's choices, which are by profile already.
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
  const duplicate = async (m: Module) => {
    const make = kindOf(m).duplicate;
    if (make === false) return;
    try {
      const copy = { ...(make ? await make(m) : { ...m, id: "" }), name: t("{name} copy", { name: m.name }) };
      const next = [...mods];
      next.splice(mods.indexOf(m) + 1, 0, copy);
      if (await save(next)) { toast(t("Added {name}", { name: copy.name })); setFlash(copy.name); setTimeout(() => setFlash(""), 900); }
    } catch (e) { toastError(e); }
  };
  const ownGroup = (m: Module) => groups?.find((g) => g.module === m.id);
  const draftIn = (to: string) => {
    if (!draft || (draft.profile ?? "") !== to) return null;
    const { Editor } = kindOf(draft);
    return <div className="list modules"><Editor module={draft} open onCancel={() => setDraft(null)} onSave={add} /></div>;
  };
  const replace = async (m: Module, n: Module) => save(mods.map((o) => (o.id === m.id ? n : o)));
  // a section's rows, or what it says empty, which a row can be dropped on
  const rows = (s: number, empty: string) => sections[s].length === 0
    ? <div ref={sort.zone(s)} className={"list user-rules-empty" + sort.zoneClass(s)}>{empty}</div>
    : (
    <div ref={sort.zone(s)} style={sort.zoneStyle(s)} className={"list modules" + (sort.dragging ? " sorting" : "")}>
      {sections[s].map((m, i) => (
        <ModuleRow key={m.id} m={m} open={open === m.id} flash={flash === m.name}
          sort={sort} s={s} i={i} profileName={profile ? profileName : ""}
          ctx={{ profile, nodes, groups, group: ownGroup(m), open: open === m.id, onOpen: () => { setDraft(null); setOpen(open === m.id ? "" : m.id); },
            onPick: (g: Group, n: string) => select(g.name, n), onSave: (n: Module) => replace(m, n) }}
          onToggle={(on) => save(mods.map((o) => (o.id === m.id ? { ...o, enabled: on } : o)))}
          onScope={() => setScope(m, m.profile ? 0 : 1)}
          onDuplicate={kindOf(m).duplicate === false ? undefined : () => duplicate(m)}
          onRemove={() => save(mods.filter((o) => o.id !== m.id))}
          onSave={async (n) => { if (await replace(m, n)) { setOpen(""); toast(t("Saved")); } }} />
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
  let i = 0;
  return (
    <>
      <Popover anchor={newAt?.at ?? null} open={!!newAt} onClose={() => setNewAt(null)} align="end" width={340}>
        <div className="menu templates">
          {sectionsNew.filter((sec) => sec.entries.length).map((sec) => (
            <div key={sec.title} className="msection">
              <div className="mhead">{sec.title}</div>
              {sec.entries.map((e) => (
                <button key={e.key} onClick={() => use(e, to)}>
                  <span className="tname">{e.name}{e.added && <span className="badge muted">{t("Added")}</span>}</span>
                  <span className="thint">{e.hint}</span>
                </button>
              ))}
              <hr />
            </div>
          ))}
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
          {sectionsNew.filter((sec) => sec.entries.length).map((sec, k) => (
            <div key={sec.title} className="msection">
              {k > 0 && <div className="sect" style={{ ["--i" as string]: i++ }}>{sec.title}</div>}
              {sec.entries.map((e) => (
                <button className="row click" key={e.key} style={{ ["--i" as string]: i++ }} onClick={() => use(e, "")}>
                  <div className="who"><div className="name">{e.name}</div><div className="sub">{e.hint}</div></div>
                  <span className="btn small">{t("Add…")}</span>
                </button>
              ))}
            </div>
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

function ModuleRow({ m, ctx, open, flash, sort, s, i, profileName, onToggle, onScope, onDuplicate, onRemove, onSave }: {
  m: Module; ctx: RowCtx; open: boolean; flash: boolean; sort: Sort; s: number; i: number; profileName: string;
  onToggle: (on: boolean) => Promise<unknown>; onScope: () => void; onDuplicate?: () => void; onRemove: () => void; onSave: (m: Module) => Promise<void>;
}) {
  const t = useT();
  const { Summary, End, Editor } = kindOf(m);
  const [armed, setArmed] = useState(false);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);
  const [menuAt, setMenuAt] = useState<HTMLElement | null>(null);
  const items: MenuItem[] = [
    ...(onDuplicate ? [{ label: t("Duplicate"), onClick: onDuplicate }] : []),
    ...(m.profile ? [{ label: t("Make global"), onClick: onScope }]
      : profileName ? [{ label: t("Move to {profile}", { profile: profileName }), onClick: onScope }] : []),
  ];
  return (
    <div ref={sort.row(s, i)} style={sort.style(s, i)}
      className={"module" + (open ? " open" : "") + (m.enabled ? "" : " off") + sort.rowClass(s, i)}>
      <div className={"row click" + (flash ? " flash" : "")} onClick={ctx.onOpen}>
        <button className="grip" title={t("Drag to reorder")} aria-label={t("Drag to reorder")} {...sort.handle(s, i)}><Grip size={14} /></button>
        <div className="who">
          <div className="name">{m.name}</div>
          <Summary m={m} ctx={ctx} />
        </div>
        <div className="end" onClick={(e) => e.stopPropagation()}>
          {End && <End m={m} ctx={ctx} />}
          {items.length > 0 && <button className="icon" title={t("More")} onClick={(e) => setMenuAt(menuAt ? null : e.currentTarget)}><More size={12} /></button>}
          <Popover anchor={menuAt} open={!!menuAt} onClose={() => setMenuAt(null)} align="end">
            <Menu close={() => setMenuAt(null)} items={items} />
          </Popover>
          {armed
            ? <button className="btn small danger armed" onClick={onRemove}>{t("Click again to remove")}</button>
            : <button className="icon" title={t("Delete")} onClick={() => setArmed(true)}><Close size={12} /></button>}
          <Switch on={m.enabled} onChange={onToggle} label={m.name} />
        </div>
      </div>
      <Fold open={open}>
        <Editor module={m} open={open} onCancel={ctx.onOpen} onSave={onSave} />
      </Fold>
    </div>
  );
}
