import { useEffect, useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Proxy, type Rule, type UserRule } from "../api";
import { usePoll } from "../usePoll";
import { Arrow, Close, Plus, Search } from "../components/Icons";
import { RuleEditor } from "../components/RuleEditor";
import { AppIcon } from "../components/AppIcon";
import { ruleProgram } from "../components/AppPicker";
import { toastError } from "../components/Toast";
import { Segmented } from "../components/Segmented";
import { ago } from "../format";

// Rules lists the profile's rules with how often each has matched (Surge
// shows rule usage counts; mihomo counts them on top-level rules).
export function Rules() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [rules, setRules] = useState<Rule[]>([]);
  const [q, setQ] = useState("");
  const [order, setOrder] = useState<"profile" | "hits">("profile");
  const [limit, setLimit] = useState(300);
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
        <h2>{t("Rules")}</h2>
        <span className="sub">{t("{n} rules", { n: rules.length })}</span>
        <div className="view-tools">
          <Segmented className="track small" value={order} onChange={setOrder} options={[{ value: "profile", label: t("Profile order") }, { value: "hits", label: t("Most hits") }]} />
          <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => { setQ(e.target.value); setLimit(300); }} /></label>
        </div>
      </div>
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
              <span className="cell policy">{r.policy}</span>
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
              <div className="trow" key={r.index}>
                <span className="cell idx num">{r.index + 1}</span>
                <span className="cell"><span className="rtype">{r.type}</span></span>
                <span className="cell mono payload" title={r.payload}>{r.payload || "—"}{r.size > 0 && <span className="sub"> ({r.size})</span>}</span>
                <span className="cell policy">{r.proxy}</span>
                <span className="cell hits" title={hits && at ? t("Last hit {t}", { t: ago(at, t) }) : undefined}>
                  <span className="hitbar"><i style={{ width: (hits / maxHits) * 100 + "%" }} /></span>
                  <span className="num">{hits || ""}</span>
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

// a rule's value; a process rule shows the program's icon and name
function RulePayload({ r }: { r: UserRule }) {
  const p = ruleProgram(r.type, r.payload);
  if (!p) return <span className="cell mono payload" title={r.payload}>{r.payload}</span>;
  return <span className="cell payload rule-app" title={r.payload}><AppIcon path={p.path} /><span className="name">{p.name}</span></span>;
}
