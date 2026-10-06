import { Fragment, useEffect, useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Proxy, type Rule, type RuleProvider, type UserRule } from "../api";
import { usePoll } from "../usePoll";
import { Arrow, Chevron, Close, Plus, Refresh, Search } from "../components/Icons";
import { EntryList, RuleDetail } from "../components/RuleDetail";
import { RuleEditor } from "../components/RuleEditor";
import { AppIcon } from "../components/AppIcon";
import { ruleProgram } from "../components/AppPicker";
import { HostLookup } from "../components/HostLookup";
import { toast, toastError } from "../components/Toast";
import { Segmented } from "../components/Segmented";
import { ago, nodeLabel } from "../format";
import { useGroups } from "../useGroups";

type Tab = "rules" | "providers" | "lookup";

// Rules lists the profile's rules with how often each has matched (Surge
// shows rule usage counts; mihomo counts them on top-level rules), the
// user's own ahead of them, and on a second tab the rule providers.
export function Rules() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [rules, setRules] = useState<Rule[]>([]);
  const [q, setQ] = useState("");
  const [order, setOrder] = useState<"profile" | "hits">("profile");
  const [limit, setLimit] = useState(300);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const toggle = (i: number) => setOpen((o) => { const n = new Set(o); if (n.has(i)) n.delete(i); else n.add(i); return n; });
  useEffect(() => setOpen(new Set()), [profile]);
  const [tab, setTab] = useState<Tab>(location.hash === "#providers" ? "providers" : location.hash === "#lookup" ? "lookup" : "rules");
  const [providers, setProviders] = useState<RuleProvider[]>([]);
  const loadProviders = () => { if (running && !busy) Proxy.RuleProviders().then((p) => setProviders(p ?? [])).catch(() => {}); };
  useEffect(loadProviders, [running, profile, busy]);
  const [mine, setMine] = useState<UserRule[]>([]);
  const [addAt, setAddAt] = useState<HTMLElement | null>(null);
  const [editing, setEditing] = useState<{ i: number; at: HTMLElement } | null>(null);
  const loadMine = () => Proxy.UserRules().then((r) => setMine(r ?? [])).catch(() => {});
  useEffect(() => { loadMine(); }, [profile, busy]);
  const saveMine = async (next: UserRule[]) => {
    const before = mine;
    setMine(next);
    try { await Proxy.SetUserRules(next); } catch (e) { setMine(before); toastError(e); }
  };
  // the edited rule in its place; another for the same type and value goes
  const saveEdit = async (i: number, r: UserRule) => {
    const next = mine.map((o, j) => (j === i ? r : o)).filter((o, j) => j === i || o.type !== r.type || o.payload !== r.payload);
    setMine(next);
    try { await Proxy.SetUserRules(next); } catch (e) { setMine(mine); throw e; }
  };
  const move = (i: number, d: number) => {
    const next = mine.slice();
    [next[i], next[i + d]] = [next[i + d], next[i]];
    saveMine(next);
  };

  const { groups } = useGroups();
  const nowOf = useMemo(() => new Map((groups ?? []).map((g) => [g.name, g.now])), [groups]);

  const load = () => { if (running && !busy) Proxy.Rules().then((r) => setRules(r ?? [])).catch(() => {}); };
  useEffect(load, [running, profile, busy]);
  usePoll(load, 3000, [running, busy]);

  const maxHits = useMemo(() => Math.max(1, ...rules.map((r) => r.extra?.hitCount ?? 0)), [rules]);
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    const list = s ? rules.filter((r) => [r.type, r.payload, r.proxy].some((v) => v.toLowerCase().includes(s))) : rules;
    return order === "hits" ? [...list].sort((a, b) => (b.extra?.hitCount ?? 0) - (a.extra?.hitCount ?? 0)) : list;
  }, [rules, q, order]);

  return (
    <div className="view" onScroll={(e) => {
      const el = e.currentTarget;
      if (el.scrollTop + el.clientHeight > el.scrollHeight - 400 && limit < shown.length) setLimit(limit + 300);
    }}>
      <div className="view-head">
        <Segmented className="track small" value={tab} onChange={setTab} options={[
          { value: "rules", label: `${t("Rules")} ${rules.length}` },
          { value: "providers", label: `${t("Providers")} ${providers.length}` },
          { value: "lookup", label: t("Lookup") },
        ]} />
        <div className="view-tools">
          {tab === "rules" && <Segmented className="track small" value={order} onChange={setOrder} options={[{ value: "profile", label: t("Profile order") }, { value: "hits", label: t("Most hits") }]} />}
          {tab !== "lookup" && <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => { setQ(e.target.value); setLimit(300); }} /></label>}
        </div>
      </div>
      {tab === "lookup" ? <HostLookup running={running} /> : tab === "providers" ? <RuleProviders providers={providers} rules={rules} q={q} reload={loadProviders} /> : <>
      <div className="section-title user-rules-title">
        <span>{t("My rules")}</span>
        <button className="btn small" onClick={(e) => setAddAt(addAt ? null : e.currentTarget)}><Plus size={12} />{t("Add rule…")}</button>
      </div>
      {mine.length === 0 ? (
        <div className="list user-rules-empty">{t("Rules you add here, or from a connection, go ahead of the profile's and stay across updates.")}</div>
      ) : (
        <div className="list table rules mine">
          {mine.map((r, i) => (
            <div className={"trow click" + (editing?.i === i ? " menu-on" : "")} key={r.type + r.payload} title={t("Click to edit")}
              onClick={(e) => setEditing(editing?.i === i ? null : { i, at: e.currentTarget })}>
              <span className="cell idx num">{i + 1}</span>
              <span className="cell"><span className="rtype">{r.type}</span></span>
              <RulePayload r={r} />
              <PolicyRoute policy={r.policy} nowOf={nowOf} />
              <span className="cell r actions" onClick={(e) => e.stopPropagation()}>
                <button className="icon" disabled={i === 0} title={t("Move up")} onClick={() => move(i, -1)}><Arrow dir="up" size={12} /></button>
                <button className="icon" disabled={i === mine.length - 1} title={t("Move down")} onClick={() => move(i, 1)}><Arrow dir="down" size={12} /></button>
                <button className="icon" title={t("Delete")} onClick={() => saveMine(mine.filter((_, j) => j !== i))}><Close size={12} /></button>
              </span>
            </div>
          ))}
        </div>
      )}
      <RuleEditor anchor={addAt} onClose={() => { setAddAt(null); loadMine(); }} />
      <RuleEditor anchor={editing?.at ?? null} onClose={() => setEditing(null)} initial={editing ? mine[editing.i] : undefined}
        onSave={(r) => saveEdit(editing!.i, r)} />
      <div className="section-title">{t("Active rules")}</div>
      {shown.length === 0 ? (
        <div className="empty-state"><b>{t("No rules")}</b></div>
      ) : (
        <div className="list table rules">
          {shown.slice(0, limit).map((r) => {
            const hits = r.extra?.hitCount ?? 0;
            const at = r.extra?.hitAt as unknown as string | undefined;
            return (
              <Fragment key={r.index}>
              <div className={"trow click" + (open.has(r.index) ? " open" : "")} title={t("Click for details")} onClick={() => toggle(r.index)}>
                <span className="cell idx num"><Chevron size={9} className="chev" />{r.index + 1}</span>
                <span className="cell"><span className="rtype">{r.type}</span></span>
                <span className="cell mono payload" title={r.payload}>{r.payload || "—"}{r.size > 0 && <span className="sub"> ({r.size})</span>}</span>
                <PolicyRoute policy={r.proxy} nowOf={nowOf} />
                <span className="cell hits" title={hits && at ? t("Last hit {t}", { t: ago(at, t) }) : undefined}>
                  <span className="hitbar"><i style={{ width: (hits / maxHits) * 100 + "%" }} /></span>
                  <span className="num">{hits || ""}</span>
                </span>
              </div>
              {open.has(r.index) && <RuleDetail r={r} route={routeOf(r.proxy, nowOf)} />}
              </Fragment>
            );
          })}
        </div>
      )}
      </>}
    </div>
  );
}

// The policy and the node it ends at, following each group's selection, as
// the connections page shows a chain: groups muted, the exit as a tag.
function PolicyRoute({ policy, nowOf }: { policy: string; nowOf: Map<string, string> }) {
  const path = routeOf(policy, nowOf);
  const exit = nodeLabel(path[path.length - 1]);
  const via = path.slice(0, -1);
  const tone = exit === "DIRECT" ? "direct" : /^REJECT/.test(exit) ? "reject" : "proxy";
  return <span className="cell chain" title={path.map(nodeLabel).join(" → ")}>
    {via.length > 0 && <span className="via">{via.join(" → ")} →</span>}
    <span className={"exit " + tone}>{exit}</span>
  </span>;
}

// a policy, then each group's selection down to a node
function routeOf(policy: string, nowOf: Map<string, string>) {
  const path = [policy];
  for (let now = nowOf.get(policy); now && !path.includes(now); now = nowOf.get(now)) path.push(now);
  return path;
}

// a rule's value; a process rule shows the program's icon and name
function RulePayload({ r }: { r: UserRule }) {
  const p = ruleProgram(r.type, r.payload);
  if (!p) return <span className="cell mono payload" title={r.payload}>{r.payload}</span>;
  return <span className="cell payload rule-app" title={r.payload}><AppIcon path={p.path} /><span className="name">{p.name}</span></span>;
}

// The profile's rule providers: what each holds, when it was fetched, and
// the policies its RULE-SET rules send it to. Remote ones can be updated.
function RuleProviders({ providers, rules, q, reload }: { providers: RuleProvider[]; rules: Rule[]; q: string; reload: () => void }) {
  const t = useT();
  const [updating, setUpdating] = useState<Record<string, boolean>>({});
  const [open, setOpen] = useState("");
  const policies = useMemo(() => {
    const m: Record<string, Set<string>> = {};
    for (const r of rules) if (r.type === "RuleSet") (m[r.payload] ??= new Set()).add(r.proxy);
    return m;
  }, [rules]);
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? providers.filter((p) => p.name.toLowerCase().includes(s)) : providers;
  }, [providers, q]);
  const remote = providers.filter((p) => p.vehicleType !== "Inline");
  const update = async (names: string[]) => {
    setUpdating((u) => ({ ...u, ...Object.fromEntries(names.map((n) => [n, true])) }));
    const failed: string[] = [];
    for (const n of names) {
      try { await Proxy.UpdateRuleProvider(n); } catch (e) { failed.push(n); if (names.length === 1) toastError(e); }
      setUpdating((u) => ({ ...u, [n]: false }));
    }
    reload();
    if (failed.length > 1 || (failed.length === 1 && names.length > 1)) toast(t("Couldn't update {names}", { names: failed.join(", ") }), "err", 4000);
    else if (!failed.length) toast(names.length === 1 ? t("Updated {name}", { name: names[0] }) : t("Updated {n} providers", { n: names.length }));
  };
  if (!providers.length) return <div className="empty-state"><b>{t("No rule providers")}</b>{t("The profile has no rule-providers.")}</div>;
  return (
    <>
      <div className="section-title user-rules-title">
        <span>{t("Rule providers")}</span>
        {remote.length > 0 && <button className="btn small" disabled={remote.some((p) => updating[p.name])} onClick={() => update(remote.map((p) => p.name))}><Refresh size={12} />{t("Update all")}</button>}
      </div>
      <div className="list table rule-providers">
        {shown.map((p) => {
          const fetched = p.vehicleType !== "Inline" && new Date(p.updatedAt).getFullYear() > 2000;
          const used = [...(policies[p.name] ?? [])];
          return (
            <Fragment key={p.name}>
            <div className={"trow click" + (open === p.name ? " open" : "")} title={t("Click for details")} onClick={() => setOpen(open === p.name ? "" : p.name)}>
              <span className="cell host">
                <span className="name">{p.name}</span>
                <span className="sub">{[behaviorLabel(t, p.behavior), formatLabel(p.format), t("{n} rules", { n: p.ruleCount })].filter(Boolean).join(" · ")}</span>
              </span>
              <span className="cell"><span className="rtype">{vehicleLabel(t, p.vehicleType)}</span></span>
              <span className="cell policy" title={used.join(", ")}>{used.length ? used.join(", ") : <span className="muted">{t("Not used")}</span>}</span>
              <span className="cell muted">{fetched ? t("Updated {t}", { t: ago(p.updatedAt, t) }) : p.vehicleType === "Inline" ? t("In the profile") : ""}</span>
              <span className="cell r" onClick={(e) => e.stopPropagation()}>
                {p.vehicleType !== "Inline" && (
                  <button className={"icon" + (updating[p.name] ? " spin" : "")} disabled={updating[p.name]} title={t("Update")} onClick={() => update([p.name])}><Refresh size={13} /></button>
                )}
              </span>
            </div>
            {open === p.name && <div className="rule-detail provider-detail"><EntryList kind="ruleset" name={p.name} version={String(p.updatedAt)} /></div>}
            </Fragment>
          );
        })}
      </div>
    </>
  );
}

const behaviorLabel = (t: (s: string) => string, b: string) => ({ Domain: t("Domains"), IPCIDR: t("IP ranges"), Classical: t("Mixed rules") } as Record<string, string>)[b] ?? b;
const formatLabel = (f: string) => ({ YamlRule: "YAML", TextRule: "Text", MrsRule: "MRS" } as Record<string, string>)[f] ?? f;
const vehicleLabel = (t: (s: string) => string, v: string) => ({ HTTP: t("Remote"), File: t("Local file"), Inline: t("Inline") } as Record<string, string>)[v] ?? v;
