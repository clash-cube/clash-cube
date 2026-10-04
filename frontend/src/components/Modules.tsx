import { useEffect, useRef, useState } from "react";
import { useT } from "../i18n";
import { Profiles as P } from "../api";
import type { Module, Template } from "../../bindings/github.com/localhost-copilot/mihomobar/internal/modules/models";
import { Popover } from "./Popover";
import { Switch } from "./Switch";
import { Fold } from "./Fold";
import { Arrow, Close } from "./Icons";
import { toast, toastError, errText } from "./Toast";

const EXAMPLE = `dns:
  +fake-ip-filter:
    - "+.lan"
hosts:
  router.lan: 192.168.1.1
prepend-rules:
  - DOMAIN-SUFFIX,example.com,DIRECT`;

// Modules are YAML laid over every profile in order, as Surge's are: each
// one a row with its switch, unrolling into its editor when clicked.
export function Modules({ newAt, onNewClose }: { newAt: HTMLElement | null; onNewClose: () => void }) {
  const t = useT();
  const [mods, setMods] = useState<Module[] | null>(null);
  const [templates, setTemplates] = useState<Template[]>([]);
  const [open, setOpen] = useState("");
  // a new module being written, blank or from a template
  const [draft, setDraft] = useState<Module | null>(null);
  const [flash, setFlash] = useState("");
  const load = () => P.Modules().then((m) => setMods(m ?? [])).catch(toastError);
  useEffect(() => { load(); P.ModuleTemplates().then((ts) => setTemplates(ts ?? [])).catch(() => {}); }, []);

  // the core checks every change; one it refuses leaves the list as it was
  const save = async (next: Module[]) => {
    const before = mods;
    setMods(next);
    try { await P.SetModules(next); await load(); return true; } catch (e) { setMods(before); toastError(e); return false; }
  };
  // a template is added and on at once, unless it holds an example to fill in
  const use = async (tpl: Template | null) => {
    onNewClose();
    if (!tpl || tpl.draft) { setOpen(""); setDraft({ id: "", name: tpl ? t(tpl.name) : "", enabled: true, body: tpl?.body ?? "" }); return; }
    const m: Module = { id: "", name: t(tpl.name), enabled: true, body: tpl.body };
    if (await save([...(mods ?? []), m])) {
      toast(t("Added {name}", { name: m.name }));
      setFlash(m.name); setTimeout(() => setFlash(""), 900);
    }
  };
  const move = (i: number, d: number) => {
    const next = mods!.slice();
    [next[i], next[i + d]] = [next[i + d], next[i]];
    save(next);
  };

  if (!mods) return null;
  const added = new Set(mods.map((m) => m.name));
  return (
    <>
      <Popover anchor={newAt} open={!!newAt} onClose={onNewClose} align="end" width={340}>
        <div className="menu templates">
          {templates.map((tpl) => (
            <button key={tpl.name} onClick={() => use(tpl)}>
              <span className="tname">{t(tpl.name)}{added.has(t(tpl.name)) && <span className="badge muted">{t("Added")}</span>}</span>
              <span className="thint">{t(tpl.hint)}</span>
            </button>
          ))}
          <hr />
          <button onClick={() => use(null)}><span className="tname">{t("Blank module…")}</span></button>
        </div>
      </Popover>
      {draft && (
        <div className="list modules">
          <ModuleEditor module={draft} onCancel={() => setDraft(null)}
            onSave={async (m) => { if (await save([...mods, m])) { setDraft(null); toast(t("Added {name}", { name: m.name })); } }} />
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
              <span className="btn small">{tpl.draft ? t("Edit…") : t("Add")}</span>
            </button>
          ))}
          <button className="row click blank" onClick={() => use(null)}>{t("Blank module…")}</button>
        </div>
      ) : mods.length > 0 && (
        <div className="list modules">
          {mods.map((m, i) => (
            <ModuleRow key={m.id} m={m} open={open === m.id} flash={flash === m.name} first={i === 0} last={i === mods.length - 1}
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

function ModuleRow({ m, open, flash, first, last, onOpen, onToggle, onMove, onRemove, onSave }: {
  m: Module; open: boolean; flash: boolean; first: boolean; last: boolean;
  onOpen: () => void; onToggle: (on: boolean) => Promise<unknown>; onMove: (d: number) => void; onRemove: () => void; onSave: (m: Module) => Promise<void>;
}) {
  const t = useT();
  const keys = useKeys(m.body)[0];
  const [armed, setArmed] = useState(false);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);
  return (
    <div className={"module" + (open ? " open" : "") + (m.enabled ? "" : " off")}>
      <div className={"row click" + (flash ? " flash" : "")} onClick={onOpen}>
        <div className="who">
          <div className="name">{m.name}</div>
          <div className="sub mono">{keys.length ? keys.join(" · ") : t("Empty")}</div>
        </div>
        <div className="end" onClick={(e) => e.stopPropagation()}>
          <button className="icon" disabled={first} title={t("Move up")} onClick={() => onMove(-1)}><Arrow dir="up" size={12} /></button>
          <button className="icon" disabled={last} title={t("Move down")} onClick={() => onMove(1)}><Arrow dir="down" size={12} /></button>
          {armed
            ? <button className="btn small danger armed" onClick={onRemove}>{t("Click again to remove")}</button>
            : <button className="icon" title={t("Delete")} onClick={() => setArmed(true)}><Close size={12} /></button>}
          <Switch on={m.enabled} onChange={onToggle} label={m.name} />
        </div>
      </div>
      <Fold open={open}>
        <ModuleEditor module={m} onCancel={onOpen} onSave={onSave} />
      </Fold>
    </div>
  );
}

// useKeys is the top-level keys body sets and why it isn't a module, as
// Go reads it, a moment after the typing stops.
function useKeys(body: string): [string[], string] {
  const [out, setOut] = useState<[string[], string]>([[], ""]);
  useEffect(() => {
    let live = true;
    const id = setTimeout(() => {
      P.ModuleKeys(body).then((k) => live && setOut([k ?? [], ""]), (e) => live && setOut([[], errText(e)]));
    }, 250);
    return () => { live = false; clearTimeout(id); };
  }, [body]);
  return out;
}

function ModuleEditor({ module, onCancel, onSave }: { module: Module; onCancel: () => void; onSave: (m: Module) => Promise<void> }) {
  const t = useT();
  const [name, setName] = useState(module.name);
  const [body, setBody] = useState(module.body);
  const [busy, setBusy] = useState(false);
  const [, problem] = useKeys(body);
  const nameRef = useRef<HTMLInputElement>(null);
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  // a new one starts at its name, or at the example to fill in
  useEffect(() => { if (!module.id) setTimeout(() => (module.name ? bodyRef.current : nameRef.current)?.focus(), 60); }, []);
  const dirty = name.trim() !== module.name || body !== module.body;
  const submit = async () => {
    if (busy || !name.trim() || problem) return;
    setBusy(true);
    try { await onSave({ ...module, name: name.trim(), body }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  return (
    <form className="module-editor stagger" onSubmit={(e) => { e.preventDefault(); submit(); }}
      onKeyDown={(e) => { if (e.key === "s" && e.metaKey) { e.preventDefault(); submit(); } if (e.key === "Escape") onCancel(); }}>
      <input ref={nameRef} className="input" placeholder={t("Module name")} value={name} onChange={(e) => setName(e.target.value)} />
      <textarea ref={bodyRef} className="input" placeholder={EXAMPLE} rows={Math.min(18, Math.max(8, body.split("\n").length + 1))} value={body} spellCheck={false}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={(e) => {
          // a tab indents, as YAML wants spaces
          if (e.key !== "Tab") return;
          e.preventDefault();
          const el = e.currentTarget, a = el.selectionStart;
          setBody(body.slice(0, a) + "  " + body.slice(el.selectionEnd));
          requestAnimationFrame(() => el.setSelectionRange(a + 2, a + 2));
        }} />
      <div className="foot">
        <span className={"hint" + (problem ? " err" : "")}>{problem || t("Mappings merge; +key puts a list first, key+ last, key! replaces. ⌘S saves.")}</span>
        <div className="grow" />
        <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
        <button type="submit" className="btn small primary" disabled={busy || !name.trim() || !!problem || (!!module.id && !dirty)}>{busy ? t("Checking…") : module.id ? t("Save") : t("Add")}</button>
      </div>
    </form>
  );
}
