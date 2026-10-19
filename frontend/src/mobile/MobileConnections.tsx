import { useEffect, useState } from "react";
import { useStore } from "../store";
import { closeConnections, useConnectionStore } from "../connectionStore";
import { address, hostOf, ruleOf, groupConnections, filterConnections, type Conn, type ConnectionSnapshot, type ConnectionTab } from "../connections";
import { bytes, speed } from "../format";
import { useT } from "../i18n";
import { Fold } from "../components/Fold";
import { Segmented } from "../components/Segmented";
import { Popover } from "../components/Popover";
import { RuleEditor, suggestions } from "../components/RuleEditor";
import { toastError } from "../components/Toast";
import { Detail } from "../views/Connections";
import { PageActions } from "./PageActions";

export function MobileConnections() {
  const t = useT();
  const { snapshot: live, closing, error } = useConnectionStore();
  const running = useStore(s => s.state?.core === "running");
  const asked = useStore(s => s.connQuery);
  const [frozen, setFrozen] = useState<ConnectionSnapshot | null>(null);
  const [tab, setTab] = useState<ConnectionTab>("active");
  const [q, setQ] = useState("");
  const [grouped, setGrouped] = useState(true);
  const [folded, setFolded] = useState<Record<string, boolean>>({});
  const [selected, setSelected] = useState("");
  const [confirm, setConfirm] = useState<string[] | null>(null);
  const [rule, setRule] = useState<{ c: Conn; at: HTMLElement } | null>(null);
  useEffect(() => { if (asked) { setQ(asked); useStore.setState({ connQuery: "" }); } }, [asked]);
  useEffect(() => { if (!running) { setFrozen(null); setConfirm(null); setRule(null); } }, [running]);
  useEffect(() => {
    if (rule && ![...live.active, ...live.closed].some(c => c.id === rule.c.id)) setRule(null);
  }, [live, rule]);
  const snapshot = frozen ?? live;
  const all = tab === "active" ? snapshot.active : tab === "closed" ? snapshot.closed : [...snapshot.active, ...snapshot.closed];
  const shown = filterConnections(all, q, "all", new Set(), {});
  const groups = grouped ? groupConnections(shown, "process") : null;
  const ids = new Set(live.active.map(c => c.id));
  const targets = shown.filter(c => ids.has(c.id) && !closing.has(c.id)).map(c => c.id);
  const close = async (values: string[]) => { for (const failure of await closeConnections(values.filter(id => ids.has(id)))) toastError(failure.error); };
  const row = (c: Conn, i: number) => <div className="mobile-connection stagger" style={{ ["--i" as string]: Math.min(i, 24) }} key={c.id}>
    <button className="mobile-connection-row" aria-expanded={selected === c.id} onClick={() => setSelected(selected === c.id ? "" : c.id)}>
      <span className="who"><span className="name">{address(hostOf(c), c.metadata.destinationPort)}</span><span className="sub">{c.metadata.network.toUpperCase()} · {ruleOf(c)} · <span className="badge">{c.chains?.[0]}</span></span></span>
      <span className="mobile-rate">{c.closedAt === undefined ? (c.up > c.down ? `↑ ${speed(c.up)}` : `↓ ${speed(c.down)}`) : bytes(c.upload + c.download)}<small>{c.closedAt === undefined ? bytes(c.upload + c.download) : t("Closed")}</small></span>
    </button>
    <Fold open={selected === c.id}><Detail c={c} leaving={false} at={snapshot.at} closeDisabled={!ids.has(c.id) || closing.has(c.id)} onClose={() => setSelected("")} onKill={() => close([c.id])} onAddRule={at => setRule({ c, at })} groups={null} picking="" onPick={() => {}} /></Fold>
  </div>;
  return <div className="mobile-connections">
    <PageActions query={q} onQuery={setQ} count={shown.length} items={[
      { label: t("Group by app"), checked: grouped, onClick: () => setGrouped(!grouped) },
      { label: t("Close {n} matching", { n: targets.length }), danger: true, onClick: () => { if (targets.length) setConfirm(targets); } },
    ]}><button className="icon" aria-label={t(frozen ? "Resume" : "Pause")} onClick={() => setFrozen(frozen ? null : live)}>{frozen ? "▶" : "Ⅱ"}</button></PageActions>
    <Segmented className="track mobile-mode" value={tab} onChange={setTab} options={[{ value: "active", label: t("Active") }, { value: "closed", label: t("Closed") }, { value: "all", label: t("All") }]} />
    {q && <button className="btn" onClick={() => setQ("")}>{t("Filter")} · {q} ×</button>}
    {(error || frozen) && <p className="sub">{error || t("Paused. History collection continues.")}</p>}
    {!shown.length ? <div className="empty-state"><b>{t("No connections")}</b>{t(running ? "No active connections right now" : "When on, ongoing connections appear here")}</div> : groups ? groups.map(g => <section key={g.name}><button className="mobile-connection-group" onClick={() => setFolded({ ...folded, [g.name]: !folded[g.name] })}><span>{folded[g.name] ? "›" : "⌄"} {g.name}</span><small>{g.list.length} · {speed(g.up + g.down)}</small></button><Fold open={!folded[g.name]}>{g.list.map(row)}</Fold></section>) : shown.map(row)}
    <Popover anchor={null} open={!!confirm} onClose={() => setConfirm(null)}><div className="pop-form"><h3>{t("Close {n} connections?", { n: confirm?.length ?? 0 })}</h3><div className="foot"><button className="btn primary danger" onClick={() => { close(confirm ?? []); setConfirm(null); }}>{t("Close")}</button></div></div></Popover>
    <RuleEditor anchor={rule?.at ?? null} onClose={() => setRule(null)} choices={rule ? suggestions(rule.c) : []} />
  </div>;
}
