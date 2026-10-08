import { useEffect, useRef, useState } from "react";
import { useT } from "../../i18n";
import { Profiles as P } from "../../api";
import type { Module, Template } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import { toastError, errText } from "../Toast";
import type { ModuleKind } from "./kinds";

const EXAMPLE = `dns:
  +fake-ip-filter:
    - "+.lan"
hosts:
  router.lan: 192.168.1.1
prepend-rules:
  - DOMAIN-SUFFIX,example.com,DIRECT`;

// Keep validation tied to the exact source; an earlier successful check
// must not allow a changed draft to be saved while its check is pending.
export function useModuleValidation(body: string): { pending: boolean; problem: string; keys: string[] } {
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

export function ModuleEditor({ module, converted, onBack, onCancel, onSave }: {
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

function Summary({ m }: { m: Module }) {
  const t = useT();
  const { keys } = useModuleValidation(m.body);
  return <div className="sub mono">{keys.length ? keys.join(" · ") : t("Empty")}</div>;
}

// blank is a new YAML module, or one from a template
export const blank = (profile: string, tpl?: { name: string; body: string }): Module =>
  ({ id: "", name: tpl?.name ?? "", enabled: true, body: tpl?.body ?? "", profile });

// The kind every module is that no other claims: YAML as written.
export const yamlKind: ModuleKind = {
  id: "yaml",
  is: () => true,
  Summary,
  Editor: ({ module, onCancel, onSave }) => <ModuleEditor module={module} onCancel={onCancel} onSave={onSave} />,
  useNew: (mods) => {
    const t = useT();
    const [templates, setTemplates] = useState<Template[]>([]);
    useEffect(() => { P.ModuleTemplates().then((ts) => setTemplates(ts ?? [])).catch(() => {}); }, []);
    const added = new Set(mods.map((m) => m.name));
    return [{
      title: t("Common"),
      entries: templates.map((tpl) => ({
        key: "tpl:" + tpl.name, name: t(tpl.name), hint: t(tpl.hint), added: added.has(t(tpl.name)),
        make: (profile) => blank(profile, { name: t(tpl.name), body: tpl.body }),
      })),
    }];
  },
};
