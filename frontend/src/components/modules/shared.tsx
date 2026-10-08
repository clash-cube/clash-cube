import { useEffect, useState, type ComponentType } from "react";
import { useT } from "../../i18n";
import { Profiles as P } from "../../api";
import type { Module } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { Node } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import type { Group } from "../../api";
import { useStore } from "../../store";
import { delayClass, fmtDelay, nodeLabel } from "../../format";
import { Popover } from "../Popover";
import { Chevron, Search } from "../Icons";
import { ModuleEditor } from "./yaml";
import type { EditorProps } from "./kinds";

// useNodes is the running profile's nodes; null with the core stopped.
export function useNodes() {
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

// a module's group with no node in the profile holds REJECT alone
export const refusing = (g: Group) => g.members?.length === 1 && g.members[0].name === "REJECT";

// NodePicker is a module group's node, on its row: a choice for a group to
// choose from, the one it picked for the fastest.
export function NodePicker({ group, onPick }: { group: Group; onPick: (name: string) => void }) {
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

// NodeSelect chooses one of the profile's nodes, or groups when given:
// from a menu with each node's latency, searched when long. none labels
// the choice of nothing; without it there is none.
export function NodeSelect({ value, nodes, groups, none, placeholder, title, align, onChange }: {
  value: string; nodes: Node[] | null; groups?: string[]; none?: string; placeholder?: string; title: string; align?: "start" | "end";
  onChange: (name: string) => void;
}) {
  const t = useT();
  const [at, setAt] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");
  const now = nodes?.find((n) => n.name === value);
  const gone = !!value && !!nodes && !now && !groups?.includes(value);
  const match = (name: string) => !query || name.toLowerCase().includes(query.toLowerCase());
  const shown = (nodes ?? []).filter((n) => match(n.name));
  const shownGroups = (groups ?? []).filter(match);
  const choose = (name: string) => { setAt(null); setQuery(""); if (name !== value) onChange(name); };
  const close = () => { setAt(null); setQuery(""); };
  return (
    <>
      <button type="button" className={"node-pick" + (value ? "" : " unset") + (gone ? " warn" : "") + (at ? " on" : "")} disabled={nodes === null && !value}
        title={nodes === null ? t("Start the core to list the profile's nodes.") : title}
        onClick={(e) => { e.stopPropagation(); setAt(at ? null : e.currentTarget); }}>
        <span className="nname">{value ? nodeLabel(value) : placeholder ?? t("Choose a node…")}</span>
        {gone ? <span className="badge muted">{t("Gone")}</span>
          : now && now.delay !== 0 && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay, t)}</span>}
        <Chevron size={10} className="chev" />
      </button>
      <Popover anchor={at} open={!!at} onClose={close} align={align} width={280}>
        <div className="menu node-menu upstream-menu" onClick={(e) => e.stopPropagation()}>
          {(nodes?.length ?? 0) + (groups?.length ?? 0) > 8 && <label className="search"><Search size={13} /><input autoFocus placeholder={t("Search nodes")} value={query} onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); const first = shown[0]?.name ?? shownGroups[0]; if (first) choose(first); } }} /></label>}
          {none !== undefined && !query && <><button type="button" className={value ? "" : "on"} onClick={() => choose("")}><span className="nname">{none}</span></button><hr /></>}
          {groups && shown.length > 0 && <div className="mhead">{t("Nodes")}</div>}
          {shown.map((n) => (
            <button type="button" key={n.name} className={n.name === value ? "on" : ""} onClick={() => choose(n.name)}>
              <span className="nname">{n.name}</span>
              <span className={"delay " + delayClass(n.delay)}>{fmtDelay(n.delay, t)}</span>
            </button>
          ))}
          {shownGroups.length > 0 && <div className="mhead">{t("Groups")}</div>}
          {shownGroups.map((g) => (
            <button type="button" key={"g:" + g} className={g === value ? "on" : ""} onClick={() => choose(g)}>
              <span className="nname">{g}</span>
            </button>
          ))}
          {nodes === null && <div className="mnote">{t("Start the core to list the profile's nodes.")}</div>}
        </div>
      </Popover>
    </>
  );
}

// asYAML is the YAML module a kind's module makes over the profile in
// use, to go on editing freely.
export async function asYAML(m: Module): Promise<Module> {
  const body = await P.ModuleBody(m);
  return { id: m.id, name: m.name, enabled: m.enabled, profile: m.profile, body };
}

// FormProps is a kind's form: form is its state, kept while the YAML it
// was taken to is edited, to come back to as it was left.
export type FormProps<F> = {
  module: Module; form?: F; onCancel: () => void; onSave: (m: Module) => Promise<void>;
  onYAML: (m: Module, form: F) => void;
};

// FormOrYAML is a kind's form, or the YAML it was taken to. Until that
// YAML is saved, the form can be gone back to.
export function FormOrYAML<F extends { name: string }>({ module, open, onCancel, onSave, Form }: EditorProps & { Form: ComponentType<FormProps<F>> }) {
  const [form, setForm] = useState<F | undefined>();
  const [yaml, setYAML] = useState<Module | null>(null);
  useEffect(() => { if (!open) { setYAML(null); setForm(undefined); } }, [open]);
  return yaml
    ? <ModuleEditor key="yaml" module={yaml} converted onCancel={onCancel} onSave={onSave}
        onBack={(name) => { setForm((f) => f && { ...f, name }); setYAML(null); }} />
    : <Form key="form" module={module} form={form} onCancel={onCancel} onSave={onSave}
        onYAML={(m, f) => { setForm(f); setYAML(m); }} />;
}
