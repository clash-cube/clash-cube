import { useEffect, useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { App } from "../api";
import { closeConnections, useConnectionStore } from "../connectionStore";
import { address, chainOf, compareConnections, filterConnections, hostOf, processName, processOf, ruleOf,
  type Conn, type ConnectionSnapshot, type ConnectionSort, type ConnectionTab } from "../connections";
import { bytes, duration, speed } from "../format";
import { Chevron, Close, Search } from "../components/Icons";
import { Segmented } from "../components/Segmented";
import { Fold } from "../components/Fold";
import { toast, toastError } from "../components/Toast";

type GroupBy = "none" | "process" | "host" | "rule";

export function Connections() {
  const t = useT();
  const profile = useStore((s) => s.state?.profile);
  const { snapshot: live, closing, error } = useConnectionStore();
  const [frozen, setFrozen] = useState<ConnectionSnapshot | null>(null);
  const snapshot = frozen ?? live;
  const [tab, setTab] = useState<ConnectionTab>("active");
  const [q, setQ] = useState("");
  const [net, setNet] = useState<"all" | "tcp" | "udp">("all");
  const [by, setBy] = useState<GroupBy>("process");
  const [folded, setFolded] = useState<Record<string, boolean>>({});
  const [sel, setSel] = useState<Conn | null>(null);
  const [sort, setSort] = useState<ConnectionSort>("time");
  const [ascending, setAscending] = useState(false);
  useEffect(() => { setFrozen(null); setSel(null); }, [profile]);

  const conns = useMemo(() => tab === "active" ? snapshot.active : tab === "closed" ? snapshot.closed
    : [...snapshot.active, ...snapshot.closed], [snapshot, tab]);
  const shown = useMemo(() => filterConnections(conns, q, net).sort(compareConnections(sort, ascending)), [conns, q, net, sort, ascending]);
  const activeIDs = useMemo(() => new Set(live.active.map((c) => c.id)), [live]);
  const closeable = (list: Conn[]) => list.filter((c) => activeIDs.has(c.id) && !closing.has(c.id));

  const groups = useMemo(() => {
    if (by === "none") return null;
    const key = by === "process" ? processOf : by === "host" ? hostOf : ruleOf;
    const m = new Map<string, Conn[]>();
    for (const c of shown) {
      const k = key(c);
      const list = m.get(k);
      if (list) list.push(c); else m.set(k, [c]);
    }
    // The first member determines group order, keeping the selected sort
    // meaningful both within groups and across them.
    return [...m.entries()].map(([name, list]) => ({ name, list,
      up: list.reduce((n, c) => n + c.up, 0), down: list.reduce((n, c) => n + c.down, 0),
      total: list.reduce((n, c) => n + c.upload + c.download, 0),
    }));
  }, [shown, by]);

  const selected = sel && ([...snapshot.active, ...snapshot.closed].find((c) => c.id === sel.id) ?? sel);
  // Keep the inspected record even when it ages out of the bounded history.
  useEffect(() => { if (selected && selected !== sel) setSel(selected); }, [selected, sel]);

  const close = async (list: Conn[]) => {
    const failures = await closeConnections(closeable(list).map((c) => c.id));
    if (failures.length === 1) toastError(failures[0].error);
    else if (failures.length > 1) toast(t("Failed to close {n} connections", { n: failures.length }), "err", 4000);
  };
  const targets = closeable(shown);

  const row = (c: Conn) => (
    <div className={"trow" + (sel?.id === c.id ? " sel" : "")} key={c.id} onClick={() => setSel(sel?.id === c.id ? null : c)}>
      <span className="cell host">
        <span className="name" title={address(hostOf(c), c.metadata.destinationPort)}>{address(hostOf(c), c.metadata.destinationPort)}</span>
        <span className="sub">{c.metadata.network.toUpperCase()} · {by === "process" ? ruleOf(c) : processOf(c)}{c.closedAt !== undefined && <> · {t("Closed")}</>}</span>
      </span>
      <span className="cell" title={chainOf(c)}>{chainOf(c)}</span>
      <span className="cell r num conn-rates">
        <span className={c.up ? "live" : ""}>↑ {speed(c.up)}</span>
        <span className={c.down ? "live" : ""}>↓ {speed(c.down)}</span>
        <span className="sub" title={t("Total")}>{bytes(c.upload + c.download)}</span>
      </span>
      <span className="cell r num">{duration(c.start, c.closedAt ?? snapshot.at)}</span>
      <span className="cell r"><button className="icon" title={t("Close connection")} aria-label={t("Close connection")}
        disabled={!activeIDs.has(c.id) || closing.has(c.id)} onClick={(e) => { e.stopPropagation(); close([c]); }}><Close size={12} /></button></span>
    </div>
  );

  return (
    <div className="view conns-view">
      <div className="view-head">
        <h2>{t("Connections")}</h2>
        <span className="sub">{t("{shown} / {total} connections", { shown: shown.length, total: conns.length })}</span>
        <div className="view-tools">
          <Segmented className="track small" value={tab} onChange={setTab} options={[
            { value: "active", label: t("Active") }, { value: "closed", label: t("Closed") }, { value: "all", label: t("All") },
          ]} />
          <button className={"btn small" + (frozen ? " on" : "")} aria-pressed={!!frozen} onClick={() => setFrozen(frozen ? null : live)}>{t(frozen ? "Resume" : "Pause")}</button>
          <button className="btn small danger" disabled={!targets.length} onClick={() => close(shown)}>{t("Close {n} matching", { n: targets.length })}</button>
        </div>
      </div>
      <div className="conn-controls">
        <Segmented className="track small" value={by} onChange={setBy} options={[
          { value: "process", label: t("Process") }, { value: "host", label: t("Host") }, { value: "rule", label: t("Rule") }, { value: "none", label: t("List") },
        ]} />
        <Segmented className="track small" value={net} onChange={setNet} options={[
          { value: "all", label: t("All") }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" },
        ]} />
        <label className="search"><Search /><input aria-label={t("Search connections")} placeholder={t("Search connections")} value={q} onChange={(e) => setQ(e.target.value)} /></label>
        <select className="input conn-sort" aria-label={t("Sort by")} value={sort} onChange={(e) => { const key = e.target.value as ConnectionSort; setSort(key); setAscending(key === "host"); }}>
          <option value="time">{t("Start time")}</option><option value="host">{t("Host")}</option>
          <option value="down">{t("Download speed")}</option><option value="up">{t("Upload speed")}</option>
          <option value="download">{t("Downloaded")}</option><option value="upload">{t("Uploaded")}</option>
        </select>
        <button className="btn small" title={t(ascending ? "Ascending" : "Descending")} aria-label={t(ascending ? "Ascending" : "Descending")} onClick={() => setAscending(!ascending)}>{ascending ? "↑" : "↓"}</button>
      </div>
      {(error || frozen) && <div className="conn-notice" role="status">{error ? t("Refresh failed. Showing the last successful snapshot.") : t("Paused. History collection continues.")} {error && <span>{error}</span>}</div>}
      <div className={"conns-body" + (selected ? " with-detail" : "")}>
        <div className="conns-list">
          {shown.length === 0 ? (
            <div className="empty-state"><b>{t(q.trim() || net !== "all" ? "No matching connections" : "No connections")}</b></div>
          ) : groups ? (
            groups.map((g) => {
              const open = !folded[by + g.name];
              return (
                <div className="list cgroup" key={g.name}>
                  <div className="cghead" onClick={() => setFolded((f) => ({ ...f, [by + g.name]: open }))}>
                    <Chevron className={"chev" + (open ? " open" : "")} />
                    <span className="cgname" title={g.name}>{g.name}</span><span className="cgcount">{g.list.length}</span>
                    <div className="grow" />
                    <span className="num cgspeed">↑ {speed(g.up)} ↓ {speed(g.down)} · {bytes(g.total)}</span>
                    <button className="icon" disabled={!closeable(g.list).length} title={t("Close {n} matching", { n: closeable(g.list).length })} onClick={(e) => { e.stopPropagation(); close(g.list); }}><Close size={12} /></button>
                  </div>
                  <Fold open={open}><div className="table conns">{g.list.map(row)}</div></Fold>
                </div>
              );
            })
          ) : <div className="list table conns">{shown.map(row)}</div>}
        </div>
        {selected && <Detail c={selected} at={snapshot.at} closeDisabled={!activeIDs.has(selected.id) || closing.has(selected.id)} onClose={() => setSel(null)} onKill={() => close([selected])} />}
      </div>
    </div>
  );
}

function Detail({ c, at, closeDisabled, onClose, onKill }: { c: Conn; at: number; closeDisabled: boolean; onClose: () => void; onKill: () => void }) {
  const t = useT();
  const m = c.metadata;
  const copy = async (s: string) => { try { await App.CopyText(s); toast(t("Copied")); } catch (e) { toastError(e); } };
  const rows: [string, string][] = [
    [t("Status"), t(c.closedAt !== undefined ? "Closed" : "Active")],
    [t("Host"), address(hostOf(c), m.destinationPort)], ["IP", m.destinationIP || "—"],
    [t("Network"), `${m.network.toUpperCase()} · ${m.type}`], [t("Rule"), ruleOf(c)], [t("Chain"), chainOf(c)],
    [t("Process"), processName(c) || "—"], [t("Path"), m.processPath || "—"],
    [t("Source"), address(m.sourceIP, m.sourcePort)],
    [t("Upload"), `${bytes(c.upload)} · ${speed(c.up)}`], [t("Download"), `${bytes(c.download)} · ${speed(c.down)}`],
    [t("Time"), `${duration(c.start, c.closedAt ?? at)} · ${new Date(c.start).toLocaleTimeString([], { hour12: false })}`],
  ];
  return (
    <aside className="detail list">
      <div className="detail-head"><b title={hostOf(c)}>{hostOf(c)}</b><button className="icon" title={t("Close details")} onClick={onClose}><Close size={12} /></button></div>
      <dl>{rows.map(([k, v]) => <div key={k} onDoubleClick={() => copy(v)} title={t("Double-click to copy")}><dt>{k}</dt><dd>{v}</dd></div>)}</dl>
      <div className="detail-foot">
        <button className="btn small" onClick={() => copy(hostOf(c))}>{t("Copy host")}</button>
        <button className="btn small danger" disabled={closeDisabled} onClick={onKill}>{t("Close connection")}</button>
      </div>
    </aside>
  );
}
