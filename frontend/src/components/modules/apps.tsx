import { useState } from "react";
import { useT } from "../../i18n";
import type { App, Apps, Module } from "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models";
import type { AppInfo } from "../../api";
import { useStore } from "../../store";
import { useGroups } from "../../useGroups";
import { useConnectionStore } from "../../connectionStore";
import { nodeLabel, speed } from "../../format";
import { Fold } from "../Fold";
import { Close, Plus } from "../Icons";
import { AppIcon } from "../AppIcon";
import { AppPicker, processPayload, ruleProgram } from "../AppPicker";
import { toastError } from "../Toast";
import { asYAML, FormOrYAML, NodeSelect, useNodes, type FormProps } from "./shared";
import type { ModuleKind, RowCtx } from "./kinds";

const targetOf = (a: Apps, profile: string) => a.target?.[profile] ?? "";
const withTarget = (a: Apps, profile: string, name: string): Apps => ({ ...a, target: { ...a.target, [profile]: name } });
const appsOf = (a: Apps) => a.apps ?? [];
const sameApp = (x: App, y: App) => x.type === y.type && x.payload === y.payload;

// An app bundle as a whole, helpers included; a bare program by its name,
// which holds wherever it is installed or moved by an update.
function appOf(a: AppInfo): App {
  const type = a.bundle ? "PROCESS-PATH-REGEX" : "PROCESS-NAME";
  return { type, payload: processPayload(type, a) };
}

// the program an App names, for its icon and name
function shown(app: App) {
  const p = ruleProgram(app.type, app.payload);
  return p ?? { path: "", name: app.payload };
}

// groupNames is the groups the programs can leave by: the profile's, without GLOBAL
const groupNames = (ctx: Pick<RowCtx, "groups">) => (ctx.groups ?? []).filter((g) => g.name !== "GLOBAL" && !g.hidden).map((g) => g.name);

function Summary({ m, ctx }: { m: Module; ctx: RowCtx }) {
  const t = useT();
  const a = m.apps!;
  const target = targetOf(a, ctx.profile);
  const apps = appsOf(a);
  const snapshot = useConnectionStore((s) => s.snapshot);
  if (!target) return <div className="sub warn">{t("Choose a node for this profile; until then its rules are left as they are")}</div>;
  if (!!ctx.nodes && !ctx.nodes.some((n) => n.name === target) && !groupNames(ctx).includes(target))
    return <div className="sub warn">{t("{node} is gone; the programs' connections are refused", { node: nodeLabel(target) })}</div>;
  const conns = snapshot.active.filter((c) => apps.some((x) => x.type === c.rule && x.payload === c.rulePayload));
  const rate = conns.reduce((n, c) => n + c.up + c.down, 0);
  const names = apps.map((x) => shown(x).name);
  return (
    <div className="sub port-sub">
      <span>{names.slice(0, 3).join(", ")}{names.length > 3 && " " + t("and {n} more", { n: names.length - 3 })}</span>
      <span className="badge muted">{t("Others direct")}</span>
      {m.enabled && conns.length > 0 && <span className="live">{t("{n} connections", { n: conns.length })} · {speed(rate)}</span>}
    </div>
  );
}

function End({ m, ctx }: { m: Module; ctx: RowCtx }) {
  const t = useT();
  const a = m.apps!;
  return (
    <NodeSelect value={targetOf(a, ctx.profile)} nodes={ctx.nodes} groups={groupNames(ctx)} align="end"
      title={t("The programs leave by this, for this profile only")}
      onChange={(name) => ctx.onSave({ ...m, apps: withTarget(a, ctx.profile, name) })} />
  );
}

type AppsForm = { name: string; apps: Apps };

function AppsEditor({ module, form, onCancel, onSave, onYAML }: FormProps<AppsForm>) {
  const t = useT();
  const profile = useStore((s) => s.state?.profile ?? "");
  const tun = useStore((s) => !!s.state?.tun);
  const { groups } = useGroups();
  const nodes = useNodes();
  const [a, setA] = useState<Apps>(form?.apps ?? module.apps!);
  const [name, setName] = useState(form?.name ?? module.name);
  const [picking, setPicking] = useState(!appsOf(module.apps!).length);
  const [busy, setBusy] = useState(false);
  const apps = appsOf(a);
  const target = targetOf(a, profile);
  const out: Apps = { apps, target: a.target ?? {} };
  const was = module.apps!;
  const dirty = name.trim() !== module.name || JSON.stringify(out) !== JSON.stringify({ apps: appsOf(was), target: was.target ?? {} });
  const ok = !!name.trim() && apps.length > 0 && !!target;
  const add = (info: AppInfo) => {
    const app = appOf(info);
    if (!apps.some((x) => sameApp(x, app))) setA({ ...a, apps: [...apps, app] });
    setPicking(false);
  };
  const submit = async () => {
    if (busy || !ok) return;
    setBusy(true);
    try { await onSave({ ...module, name: name.trim(), apps: out }); } catch (e) { toastError(e); }
    setBusy(false);
  };
  const toYAML = async () => {
    try { onYAML(await asYAML({ ...module, name: name.trim() || module.name, apps: out }), { name, apps: a }); } catch (e) { toastError(e); }
  };
  const groupList = groupNames({ groups });
  return (
    <form className="module-editor route-editor apps-editor stagger" onSubmit={(e) => { e.preventDefault(); submit(); }}
      onKeyDown={(e) => { if (e.key === "s" && (e.metaKey || e.ctrlKey)) { e.preventDefault(); submit(); } if (e.key === "Escape") onCancel(); }}>
      <input className="input" placeholder={t("Module name")} value={name} onChange={(e) => setName(e.target.value)} />
      <div className="field">
        <span className="label">{t("Programs")}</span>
        <div className="scope">
          {apps.length > 0 && (
            <div className="apps-list">
              {apps.map((x) => {
                const p = shown(x);
                return (
                  <div key={x.type + x.payload} className="apps-row" title={`${x.type},${x.payload}`}>
                    {p.path ? <AppIcon path={p.path} /> : <span className="pcicon" />}
                    <span className="name">{p.name}</span>
                    <span className="kind">{x.type === "PROCESS-PATH-REGEX" ? t("Whole app") : x.type === "PROCESS-PATH" ? t("This path") : t("By name")}</span>
                    <button type="button" className="icon" title={t("Remove")} onClick={() => setA({ ...a, apps: apps.filter((y) => !sameApp(x, y)) })}><Close size={9} /></button>
                  </div>
                );
              })}
            </div>
          )}
          <button type="button" className={"btn small add-app" + (picking ? " on" : "")} aria-pressed={picking} onClick={() => setPicking(!picking)}>
            <Plus size={11} />{t("Add program…")}
          </button>
          <Fold open={picking}>
            <AppPicker onPick={add} />
          </Fold>
          <div className="pick-note">{t("An app is taken whole, its helpers included; a command-line tool by its name, wherever it runs from.")}</div>
        </div>
      </div>
      <div className="field">
        <span className="label">{t("Node")}</span>
        <div className="scope">
          <NodeSelect value={target} nodes={nodes} groups={groupList} title={t("The programs leave by this, for this profile only")}
            onChange={(n) => setA(withTarget(a, profile, n))} />
          <div className="pick-note">{t("Chosen for this profile only; each profile has its own. A group follows its choice.")}</div>
        </div>
      </div>
      <div className="foot">
        <span className={"hint" + (!tun && ok ? " warn" : "")}>
          {!apps.length ? t("Add the programs to proxy.")
            : !target ? t("Choose the node the programs leave by.")
            : !tun ? t("Programs that ignore the system proxy are only caught with TUN on.")
            : t("These programs leave by {node}; everything else goes direct, in place of the profile's rules and other modules'. My rules still come first.", { node: nodeLabel(target) })}
        </span>
        <div className="grow" />
        <button type="button" className="btn small" onClick={toYAML}>{t("Edit as YAML")}</button>
        <button type="button" className="btn small" onClick={onCancel}>{t("Cancel")}</button>
        <button type="submit" className="btn small primary" disabled={busy || !ok || (!!module.id && !dirty)}>{busy ? t("Checking…") : module.id ? t("Save") : t("Add")}</button>
      </div>
    </form>
  );
}

// Proxies only the programs it names, as Proxifier does: they leave by
// one node or group, everything else goes direct.
export const appsKind: ModuleKind = {
  id: "apps",
  is: (m) => !!m.apps,
  Summary,
  End,
  Editor: (props) => <FormOrYAML<AppsForm> {...props} Form={AppsEditor} />,
  useNew: (mods) => {
    const t = useT();
    return [{
      title: t("Proxy programs"),
      entries: [{
        key: "apps", name: t("Only these programs"), hint: t("Chosen programs leave by one node; everything else goes direct, like Proxifier"),
        added: mods.some((m) => !!m.apps),
        make: (profile) => ({ id: "", name: t("Only these programs"), enabled: true, body: "", profile, apps: { apps: [], target: {} } }),
      }],
    }];
  },
  // a second one would throw away the first's rules
  duplicate: false,
};
