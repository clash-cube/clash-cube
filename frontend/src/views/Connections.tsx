import { useEffect, useMemo, useState, type CSSProperties } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { App } from "../api";
import { closeConnections, useConnectionStore } from "../connectionStore";
import { address, chainOf, compareConnections, filterConnections, groupConnections, hostOf, processName, processOf, ruleOf,
  type Conn, type ConnectionSnapshot, type ConnectionTab } from "../connections";
import { connectionColumns, defaultColumns, isTextColumn, type ConnectionColumn } from "../connectionColumns";
import { ConnectionColumnHeader, ConnectionColumnMenu } from "../components/ConnectionColumns";
import { bytes, duration, speed } from "../format";
import { Chevron, Close, Copy, Search } from "../components/Icons";
import { useConnectionPreferences } from "../useConnectionPreferences";
import { Segmented } from "../components/Segmented";
import { Fold } from "../components/Fold";
import { AppIcon } from "../components/AppIcon";
import { ConnectionSources, useSourceLabels } from "../components/ConnectionSources";
import { VirtualConnections, type VirtualRow } from "../components/VirtualConnections";
import { toast, toastError } from "../components/Toast";

export function Connections() {
  const t = useT();
  const profile = useStore((s) => s.state?.profile);
  const { snapshot: live, closing, error } = useConnectionStore();
  const [frozen, setFrozen] = useState<ConnectionSnapshot | null>(null);
  const snapshot = frozen ?? live;
  const [tab, setTab] = useState<ConnectionTab>("active");
  const [q, setQ] = useState("");
  const { net, by, sort, ascending, columns, widths, update } = useConnectionPreferences();
  const [resizing, setResizing] = useState<Partial<Record<ConnectionColumn, number>>>({});
  const columnWidths = { ...widths, ...resizing };
  const sizes = columns.map((id) => columnWidths[id] ?? connectionColumns.find((c) => c.id === id)!.width);
  const tableStyle = {
    "--conn-grid": sizes.map((width, i) => i === 0 && columnWidths.host === undefined ? `minmax(${width}px, 1fr)` : `${width}px`).join(" ") + " 36px",
    width: `max(100%, ${sizes.reduce((a, b) => a + b, 0) + 36}px)`,
  } as CSSProperties;
  const selectSort = (id: ConnectionColumn) => update({ sort: id, ascending: sort === id ? !ascending : isTextColumn(id) });
  const [folded, setFolded] = useState<Record<string, boolean>>({});
  const [sel, setSel] = useState<Conn | null>(null);
  const [sources, setSources] = useState(new Set<string>());
  const { labels, save } = useSourceLabels();
  useEffect(() => { setFrozen(null); setSel(null); }, [profile]);

  const conns = useMemo(() => tab === "active" ? snapshot.active : tab === "closed" ? snapshot.closed
    : [...snapshot.active, ...snapshot.closed], [snapshot, tab]);
  const shown = useMemo(() => filterConnections(conns, q, net, sources, labels).sort(compareConnections(sort, ascending)), [conns, q, net, sources, labels, sort, ascending]);
  const activeIDs = useMemo(() => new Set(live.active.map((c) => c.id)), [live]);
  const closeable = (list: Conn[]) => list.filter((c) => activeIDs.has(c.id) && !closing.has(c.id));

  const groups = useMemo(() => groupConnections(shown, by), [shown, by]);

  const selected = sel && ([...snapshot.active, ...snapshot.closed].find((c) => c.id === sel.id) ?? sel);
  // Keep the inspected record even when it ages out of the bounded history.
  useEffect(() => { if (selected && selected !== sel) setSel(selected); }, [selected, sel]);

  const close = async (list: Conn[]) => {
    const failures = await closeConnections(closeable(list).map((c) => c.id));
    if (failures.length === 1) toastError(failures[0].error);
    else if (failures.length > 1) toast(t("Failed to close {n} connections", { n: failures.length }), "err", 4000);
  };
  const targets = closeable(shown);

  const cell = (c: Conn, id: ConnectionColumn) => {
    const m = c.metadata;
    if (id === "host") return <span className="cell host" key={id}>
      <span className="conn-host"><AppIcon path={m.processPath} core={m.type === "Inner"} /><span className="name" title={address(hostOf(c), m.destinationPort)}>{address(hostOf(c), m.destinationPort)}</span></span>
      <span className="sub">{m.network.toUpperCase()} · {by === "process" ? ruleOf(c) : processOf(c)}{c.closedAt !== undefined && <> · {t("Closed")}</>}</span>
    </span>;
    let value: string;
    switch (id) {
      case "process": value = processOf(c); break;
      case "source": value = [labels[m.sourceIP], address(m.sourceIP, m.sourcePort)].filter(Boolean).join(" · "); break;
      case "network": value = `${m.network.toUpperCase()} · ${m.type}`; break;
      case "rule": value = ruleOf(c); break;
      case "chain": value = chainOf(c); break;
      case "up": case "down": value = speed(c[id]); break;
      case "upload": case "download": value = bytes(c[id]); break;
      case "total": value = bytes(c.upload + c.download); break;
      case "time": return <span key={id} className="cell num host" title={new Date(c.start).toLocaleString()}>
        <span>{new Date(c.start).toLocaleTimeString([], { hour12: false })}</span><span className="sub">{duration(c.start, c.closedAt ?? snapshot.at)}</span>
      </span>;
    }
    return <span key={id} className={"cell" + (isTextColumn(id) ? "" : " num") + ((id === "up" || id === "down") && c[id] > 0 ? " live" : "")} title={value}>{value || "—"}</span>;
  };
  const row = (c: Conn) => (
    <div className={"trow" + (sel?.id === c.id ? " sel" : "")} key={c.id} onClick={() => setSel(sel?.id === c.id ? null : c)}>
      {columns.map((id) => cell(c, id))}
      <span className="cell r"><button className="icon" title={t("Close connection")} aria-label={t("Close connection")}
        disabled={!activeIDs.has(c.id) || closing.has(c.id)} onClick={(e) => { e.stopPropagation(); close([c]); }}><Close size={12} /></button></span>
    </div>
  );

  const groupHead = (g: NonNullable<typeof groups>[number]) => {
    const open = !folded[by + g.name];
    const icon = g.list[0].metadata;
    const count = closeable(g.list).length;
    const name = by === "source" ? [labels[g.name], g.name || t("No source address")].filter(Boolean).join(" · ") : g.name;
    return <div className="cghead" onClick={() => setFolded((f) => ({ ...f, [by + g.name]: open }))}>
      <Chevron className={"chev" + (open ? " open" : "")} />
      {by === "process" && <AppIcon path={icon.processPath} core={icon.type === "Inner"} />}
      {by === "source" && <AppIcon path="" />}
      <span className="cgname" title={name}>{name}</span><span className="cgcount">{g.list.length}</span>
      <div className="grow" /><span className="num cgspeed">↑ {speed(g.up)} ↓ {speed(g.down)} · {bytes(g.total)}</span>
      <button className="icon" disabled={!count} title={t("Close {n} matching", { n: count })}
        onClick={(e) => { e.stopPropagation(); close(g.list); }}><Close size={12} /></button>
    </div>;
  };
  const virtualRows: VirtualRow[] = [];
  if (shown.length > 200) {
    const add = (c: Conn) => virtualRows.push({ key: c.id, height: 64, render: () => row(c) });
    if (groups) for (const g of groups) {
      virtualRows.push({ key: "group:" + g.name, height: 40, render: () => groupHead(g) });
      if (!folded[by + g.name]) g.list.forEach(add);
    } else shown.forEach(add);
  }

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
        <Segmented className="track small" value={by} onChange={(by) => update({ by })} options={[
          { value: "process", label: t("Process") }, { value: "source", label: t("Device") }, { value: "host", label: t("Host") }, { value: "rule", label: t("Rule") }, { value: "none", label: t("List") },
        ]} />
        <Segmented className="track small" value={net} onChange={(net) => update({ net })} options={[
          { value: "all", label: t("All") }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" },
        ]} />
        <label className="search"><Search /><input aria-label={t("Search connections")} placeholder={t("Search connections")} value={q} onChange={(e) => setQ(e.target.value)} /></label>
        <ConnectionSources connections={conns} selected={sources} onChange={setSources} labels={labels} onSave={save} />
        <ConnectionColumnMenu selected={columns} onChange={(columns) => update({ columns })} onReset={() => { setResizing({}); update({ columns: defaultColumns, widths: {} }); }} />
        <select className="input conn-sort" aria-label={t("Sort by")} value={sort} onChange={(e) => { const sort = e.target.value as ConnectionColumn; update({ sort, ascending: isTextColumn(sort) }); }}>
          {connectionColumns.map((c) => <option key={c.id} value={c.id}>{t(c.label)}</option>)}
        </select>
        <button className="btn small" title={t(ascending ? "Ascending" : "Descending")} aria-label={t(ascending ? "Ascending" : "Descending")} onClick={() => update({ ascending: !ascending })}>{ascending ? "↑" : "↓"}</button>
      </div>
      {(error || frozen) && <div className="conn-notice" role="status">{error ? t("Refresh failed. Showing the last successful snapshot.") : t("Paused. History collection continues.")} {error && <span>{error}</span>}</div>}
      <div className={"conns-body" + (selected ? " with-detail" : "")}>
        <div className="conn-table-scroll"><div className="conn-table-content" style={tableStyle}>
        <ConnectionColumnHeader columns={columns} widths={columnWidths} sort={sort} ascending={ascending} onSort={selectSort}
          onPreview={(id, width) => setResizing(width === null ? {} : { [id]: width })}
          onResize={(id, width) => update({ widths: { ...widths, [id]: width } })} />
        {shown.length > 200 ? <VirtualConnections rows={virtualRows} resetKey={JSON.stringify([q, net, by, tab, sort, ascending, [...sources]])} /> : <div className="conns-list">
          {shown.length === 0 ? (
            <div className="empty-state"><b>{t(q.trim() || net !== "all" || sources.size ? "No matching connections" : "No connections")}</b></div>
          ) : groups ? (
            groups.map((g) => {
              const open = !folded[by + g.name];
              return (
                <div className="list cgroup" key={g.name}>
                  {groupHead(g)}
                  <Fold open={open}><div className="table conns">{g.list.map(row)}</div></Fold>
                </div>
              );
            })
          ) : <div className="list table conns">{shown.map(row)}</div>}
        </div>}
        </div></div>
        {selected && <Detail c={selected} sourceLabel={labels[selected.metadata.sourceIP]} at={snapshot.at} closeDisabled={!activeIDs.has(selected.id) || closing.has(selected.id)} onClose={() => setSel(null)} onKill={() => close([selected])} />}
      </div>
    </div>
  );
}

function Detail({ c, sourceLabel, at, closeDisabled, onClose, onKill }: { c: Conn; sourceLabel?: string; at: number; closeDisabled: boolean; onClose: () => void; onKill: () => void }) {
  const t = useT();
  const m = c.metadata;
  const [raw, setRaw] = useState(false);
  const json = useMemo(() => c.rawJSON ? JSON.stringify(JSON.parse(c.rawJSON), null, 2) : "", [c.rawJSON]);
  const copy = async (s: string) => {
    try {
      if (!await App.CopyText(s)) throw new Error(t("Could not copy to clipboard"));
      toast(t("Copied"));
    } catch (e) { toastError(e); }
  };
  const rows: [string, string][] = [
    [t("Status"), t(c.closedAt !== undefined ? "Closed" : "Active")],
    [t("Connection ID"), c.id],
    [t("Host"), address(hostOf(c), m.destinationPort)], ["IP", m.destinationIP || "—"],
    [t("Network"), `${m.network.toUpperCase()} · ${m.type}`], [t("Rule"), ruleOf(c)], [t("Chain"), chainOf(c)],
    [t("Process"), processName(c) || "—"], [t("Path"), m.processPath || "—"],
    [t("Source"), [sourceLabel, address(m.sourceIP, m.sourcePort)].filter(Boolean).join(" · ")],
    ...([[t("Sniffed host"), m.sniffHost], [t("Remote destination"), m.remoteDestination],
      [t("Inbound"), [m.inboundName, m.inboundIP && address(m.inboundIP, m.inboundPort)].filter(Boolean).join(" · ")],
      [t("Inbound user"), m.inboundUser], [t("DNS mode"), m.dnsMode]] as [string, string][]).filter(([, value]) => value),
    [t("Upload"), `${bytes(c.upload)} · ${speed(c.up)}`], [t("Download"), `${bytes(c.download)} · ${speed(c.down)}`],
    [t("Time"), `${duration(c.start, c.closedAt ?? at)} · ${new Date(c.start).toLocaleTimeString([], { hour12: false })}`],
  ];
  return (
    <aside className="detail list">
      <div className="detail-head"><AppIcon path={m.processPath} core={m.type === "Inner"} /><b title={hostOf(c)}>{hostOf(c)}</b><button className="icon" title={t("Close details")} onClick={onClose}><Close size={12} /></button></div>
      <div className="detail-tabs"><Segmented className="track small" value={raw ? "raw" : "details"} onChange={(v) => setRaw(v === "raw")}
        options={[{ value: "details", label: t("Details") }, { value: "raw", label: t("Raw JSON") }]} /></div>
      <div className="detail-body" key={c.id + (raw ? ":raw" : ":details")}>
        {raw ? <pre className="conn-json">{json}</pre> : <dl>{rows.map(([k, v]) => <div key={k}>
          <dt>{k}</dt><dd onDoubleClick={() => copy(v)} title={t("Double-click to copy")}>{v}</dd>
          <button className="icon" title={t("Copy {field}", { field: k })} aria-label={t("Copy {field}", { field: k })} onClick={() => copy(v)}><Copy size={12} /></button>
        </div>)}</dl>}
      </div>
      <div className="detail-foot">
        {!raw && <button className="btn small" onClick={() => copy(hostOf(c))}>{t("Copy host")}</button>}
        <button className="btn small" onClick={() => copy(raw ? json : rows.map(([k, v]) => `${k}: ${v}`).join("\n"))}>{t(raw ? "Copy JSON" : "Copy details")}</button>
        <button className="btn small danger" disabled={closeDisabled} onClick={onKill}>{t("Close connection")}</button>
      </div>
    </aside>
  );
}
