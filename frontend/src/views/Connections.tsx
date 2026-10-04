import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { App } from "../api";
import { startCore } from "../actions";
import { closeConnections, useConnectionStore } from "../connectionStore";
import { address, chainOf, compareConnections, filterConnections, groupConnections, holdOrder, hostOf, lingering, processName, processOf, ruleOf,
  type Conn, type ConnectionSnapshot, type ConnectionTab, type HeldOrder } from "../connections";
import { connectionColumns, defaultColumns, isTextColumn, type ConnectionColumn } from "../connectionColumns";
import { ConnectionColumnHeader, ConnectionViewMenu } from "../components/ConnectionColumns";
import { bytes, duration, speed } from "../format";
import { Chevron, Close, Copy, Search } from "../components/Icons";
import { useConnectionPreferences } from "../useConnectionPreferences";
import { Segmented } from "../components/Segmented";
import { Fold } from "../components/Fold";
import { AppIcon } from "../components/AppIcon";
import { ConnectionFilter, useSourceLabels } from "../components/ConnectionSources";
import { VirtualConnections, type VirtualRow } from "../components/VirtualConnections";
import { toast, toastError } from "../components/Toast";
import { Popover, Menu } from "../components/Popover";
import { RuleEditor, suggestions } from "../components/RuleEditor";

const ROW = 44, HEAD = 34;

export function Connections() {
  const t = useT();
  const profile = useStore((s) => s.state?.profile);
  const running = useStore((s) => s.state?.core === "running");
  const { snapshot: live, closing, error } = useConnectionStore();
  const [frozen, setFrozen] = useState<ConnectionSnapshot | null>(null);
  const snapshot = frozen ?? live;
  const [tab, setTab] = useState<ConnectionTab>("active");
  const [q, setQ] = useState("");
  const search = useRef<HTMLInputElement>(null);
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
  const [reveal, setReveal] = useState<{ key: string } | null>(null);
  const [sources, setSources] = useState(new Set<string>());
  const { labels, save } = useSourceLabels();
  // the row a right-click opened the menu on, then the rule editor
  const [menu, setMenu] = useState<{ c: Conn; at: HTMLElement } | null>(null);
  const [ruleFor, setRuleFor] = useState<{ c: Conn; at: HTMLElement } | null>(null);
  const [hold, setHold] = useState(false);
  // the connections the first click of a bulk close will close
  const [armed, setArmed] = useState<Conn[] | null>(null);
  useEffect(() => { setFrozen(null); setSel(null); }, [profile]);
  // another page asked to show some connections, such as an app's
  const asked = useStore((s) => s.connQuery);
  useEffect(() => {
    if (!asked) return;
    setQ(asked); setTab("active");
    useStore.setState({ connQuery: "" });
  }, [asked]);
  useEffect(() => setArmed(null), [q, net, sources, tab]);
  useEffect(() => {
    if (!armed) return;
    const tm = setTimeout(() => setArmed(null), 3000);
    return () => clearTimeout(tm);
  }, [armed]);

  const conns = useMemo(() => tab === "active" ? snapshot.active : tab === "closed" ? snapshot.closed
    : [...snapshot.active, ...snapshot.closed], [snapshot, tab]);
  const gone = (c: Conn) => tab === "active" && c.closedAt !== undefined;
  const display = useMemo(() => {
    if (tab !== "active" || frozen) return conns;
    const fading = lingering(snapshot, running);
    return fading.length ? [...conns, ...fading] : conns;
  }, [conns, snapshot, tab, frozen, running]);

  const held = useRef<HeldOrder>({ key: "", rank: new Map() });
  const holding = hold && (sort === "speed" || sort === "up" || sort === "down");
  const shown = useMemo(() => {
    const list = filterConnections(display, q, net, sources, labels).sort(compareConnections(sort, ascending));
    held.current = holdOrder(list, `${sort}:${ascending}`, holding, held.current);
    return list;
  }, [display, q, net, sources, labels, sort, ascending, holding]);
  const count = useMemo(() => shown.filter((c) => !gone(c)).length, [shown, tab]);
  const activeIDs = useMemo(() => new Set(live.active.map((c) => c.id)), [live]);
  const closeable = (list: Conn[]) => list.filter((c) => activeIDs.has(c.id) && !closing.has(c.id));

  const born = useRef<Map<string, number> | null>(null);
  const births = useMemo(() => {
    const now = Date.now(), seen = born.current, next = new Map<string, number>();
    for (const c of live.active) next.set(c.id, seen ? seen.get(c.id) ?? now : 0);
    return born.current = next;
  }, [live]);

  const groups = useMemo(() => groupConnections(shown, by), [shown, by]);
  const sections = groups && groups.length > 1 ? groups : null;
  const order = sections ? sections.flatMap((g) => folded[by + g.name] ? [] : g.list) : shown;

  const selected = sel && ([...snapshot.active, ...snapshot.closed].find((c) => c.id === sel.id) ?? sel);
  // Keep the inspected record even when it ages out of the bounded history.
  useEffect(() => { if (selected && selected !== sel) setSel(selected); }, [selected, sel]);

  const close = async (list: Conn[]) => {
    const failures = await closeConnections(closeable(list).map((c) => c.id));
    if (failures.length === 1) toastError(failures[0].error);
    else if (failures.length > 1) toast(t("Failed to close {n} connections", { n: failures.length }), "err", 4000);
  };
  const targets = closeable(armed ?? shown);
  const closeMatching = () => {
    if (targets.length > 1 && !armed) { setArmed(targets); return; }
    setArmed(null);
    close(targets);
  };
  const filtered = !!q.trim() || net !== "all" || sources.size > 0;
  const clearFilters = () => { setQ(""); setSources(new Set()); update({ net: "all" }); };

  const keys = useRef<(e: KeyboardEvent) => void>(() => {});
  keys.current = (e) => {
    const el = e.target as HTMLElement;
    const typing = el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement;
    if ((e.metaKey || e.ctrlKey) && e.key === "f") { e.preventDefault(); search.current?.focus(); search.current?.select(); return; }
    if (typing) {
      if (e.key === "Escape" && el === search.current) { if (q) setQ(""); else el.blur(); }
      return;
    }
    if (e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      if (!order.length) return;
      e.preventDefault();
      const i = sel ? order.findIndex((c) => c.id === sel.id) : -1;
      const next = order[i < 0 ? (e.key === "ArrowDown" ? 0 : order.length - 1) : Math.max(0, Math.min(order.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))];
      setSel(next);
      setReveal({ key: next.id });
    } else if (e.key === "Escape" && sel) setSel(null);
    else if ((e.key === "Backspace" || e.key === "Delete") && selected) { e.preventDefault(); close([selected]); }
    else if (e.key === " " && !(el instanceof HTMLButtonElement)) { e.preventDefault(); setFrozen(frozen ? null : live); }
  };
  useEffect(() => {
    const key = (e: KeyboardEvent) => keys.current(e);
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  useEffect(() => {
    if (reveal && shown.length <= 200) document.querySelector(`.conns-view .trow[data-id="${CSS.escape(reveal.key)}"]`)?.scrollIntoView({ block: "nearest" });
  }, [reveal]);

  const cell = (c: Conn, id: ConnectionColumn) => {
    const m = c.metadata;
    if (id === "host") return <span className="cell host" key={id}>
      <span className="conn-host"><AppIcon path={m.processPath} core={m.type === "Inner"} /><span className="name" title={address(hostOf(c), m.destinationPort)}>{address(hostOf(c), m.destinationPort)}</span></span>
      <span className="sub">{m.network.toUpperCase()} · {by === "process" ? ruleOf(c) : processOf(c)}{c.closedAt !== undefined && <> · {t("Closed")}</>}</span>
    </span>;
    if (id === "chain") return <Chain key={id} c={c} />;
    if (id === "speed") return <span key={id} className="cell num rate"><Rate up={c.up} down={c.down} /></span>;
    if (id === "time") return <span key={id} className="cell num host" title={new Date(c.start).toLocaleString()}>
      <span>{new Date(c.start).toLocaleTimeString([], { hour12: false })}</span><span className="sub">{duration(c.start, c.closedAt ?? snapshot.at)}</span>
    </span>;
    let value: string;
    let zero = false;
    switch (id) {
      case "process": value = processOf(c); break;
      case "source": value = [labels[m.sourceIP], address(m.sourceIP, m.sourcePort)].filter(Boolean).join(" · "); break;
      case "network": value = `${m.network.toUpperCase()} · ${m.type}`; break;
      case "rule": value = ruleOf(c); break;
      case "up": case "down": zero = !c[id]; value = speed(c[id]); break;
      case "upload": case "download": zero = !c[id]; value = bytes(c[id]); break;
      case "total": zero = !(c.upload + c.download); value = bytes(c.upload + c.download); break;
    }
    if (zero) return <span key={id} className="cell num zero">—</span>;
    return <span key={id} className={"cell" + (isTextColumn(id) ? "" : " num") + (id === "up" ? " up" : id === "down" ? " down" : "")} title={value}>{value || "—"}</span>;
  };
  const row = (c: Conn) => {
    const born = births.get(c.id);
    return (
      <div className={"trow" + (sel?.id === c.id ? " sel" : "") + (menu?.c.id === c.id ? " menu-on" : "") + (gone(c) ? " gone" : born && born > snapshot.at - 1000 ? " fresh" : "")}
        key={c.id} data-id={c.id} onClick={() => setSel(sel?.id === c.id ? null : c)}
        onContextMenu={(e) => { e.preventDefault(); setMenu({ c, at: e.currentTarget }); }}>
        {columns.map((id) => cell(c, id))}
        <span className="cell r"><button className="icon" title={t("Close connection")} aria-label={t("Close connection")}
          disabled={!activeIDs.has(c.id) || closing.has(c.id)} onClick={(e) => { e.stopPropagation(); close([c]); }}><Close size={12} /></button></span>
      </div>
    );
  };

  const groupHead = (g: NonNullable<typeof groups>[number]) => {
    const open = !folded[by + g.name];
    const icon = g.list[0].metadata;
    const n = closeable(g.list).length;
    const name = by === "source" ? [labels[g.name], g.name || t("No source address")].filter(Boolean).join(" · ") : g.name;
    return <div className="cghead" onClick={() => setFolded((f) => ({ ...f, [by + g.name]: open }))}>
      <Chevron className={"chev" + (open ? " open" : "")} />
      {by === "process" && <AppIcon path={icon.processPath} core={icon.type === "Inner"} />}
      {by === "source" && <AppIcon path="" />}
      <span className="cgname" title={name}>{name}</span><span className="cgcount">{g.list.filter((c) => !gone(c)).length}</span>
      <div className="grow" /><span className="num cgspeed rate">{(g.up > 0 || g.down > 0) && <Rate up={g.up} down={g.down} />}<span>{bytes(g.total)}</span></span>
      <button className="icon" disabled={!n} title={t("Close {n} matching", { n })}
        onClick={(e) => { e.stopPropagation(); close(g.list); }}><Close size={12} /></button>
    </div>;
  };
  const virtualRows: VirtualRow[] = [];
  if (shown.length > 200) {
    const add = (c: Conn) => virtualRows.push({ key: c.id, height: ROW, render: () => row(c) });
    if (sections) for (const g of sections) {
      virtualRows.push({ key: "group:" + g.name, height: HEAD, render: () => groupHead(g) });
      if (!folded[by + g.name]) g.list.forEach(add);
    } else shown.forEach(add);
  }

  const empty = !running && !conns.length ? <div className="empty-state">
    <b>{t("Core is not running")}</b>{t("Start the core to see connections.")}
    <div><button className="btn small primary" onClick={startCore}>{t("Start core")}</button></div>
  </div> : filtered ? <div className="empty-state">
    <b>{t("No matching connections")}</b>
    <div><button className="btn small" onClick={clearFilters}>{t("Clear filters")}</button></div>
  </div> : <div className="empty-state"><b>{t("No connections")}</b>{tab !== "closed" && t("Connections through the core appear here.")}</div>;

  return (
    <div className="view conns-view">
      <div className="view-head">
        <h2>{t("Connections")}</h2>
        <span className="sub num">{t("{shown} / {total} connections", { shown: count, total: conns.length })}</span>
        <div className="view-tools">
          <label className="search"><Search /><input ref={search} aria-label={t("Search connections")} placeholder={t("Search connections")} value={q} onChange={(e) => setQ(e.target.value)} />
            {q && <button className="icon clear" aria-label={t("Clear")} onClick={() => { setQ(""); search.current?.focus(); }}><Close size={10} /></button>}</label>
          <Segmented className="track small" value={tab} onChange={setTab} options={[
            { value: "active", label: t("Active") }, { value: "closed", label: t("Closed") }, { value: "all", label: t("All") },
          ]} />
          <ConnectionFilter net={net} onNet={(net) => update({ net })} connections={conns} selected={sources} onChange={setSources} labels={labels} onSave={save} />
          <ConnectionViewMenu by={by} onBy={(by) => update({ by })} selected={columns} onChange={(columns) => update({ columns })}
            onReset={() => { setResizing({}); update({ columns: defaultColumns, widths: {} }); }} />
          <button className={"btn small" + (frozen ? " on" : "")} aria-pressed={!!frozen} onClick={() => setFrozen(frozen ? null : live)}>{t(frozen ? "Resume" : "Pause")}</button>
          <button className={"btn small num" + (armed ? " danger armed" : "")} disabled={!targets.length} onClick={closeMatching}>
            {t(armed ? "Click again to close {n}" : "Close {n} matching", { n: targets.length })}</button>
        </div>
      </div>
      {(error || frozen) && <div className="conn-notice" role="status">{error ? t("Refresh failed. Showing the last successful snapshot.") : t("Paused. History collection continues.")} {error && <span>{error}</span>}</div>}
      <div className={"conns-body" + (selected ? " with-detail" : "")}>
        <div className="conn-table-scroll" onPointerEnter={() => setHold(true)} onPointerLeave={() => setHold(false)}>
          <div className="conn-table-content" style={tableStyle}>
            <ConnectionColumnHeader columns={columns} widths={columnWidths} sort={sort} ascending={ascending} onSort={selectSort}
              onPreview={(id, width) => setResizing(width === null ? {} : { [id]: width })}
              onResize={(id, width) => update({ widths: { ...widths, [id]: width } })} />
            {shown.length > 200 ? <VirtualConnections rows={virtualRows} reveal={reveal} resetKey={JSON.stringify([q, net, by, tab, sort, ascending, [...sources]])} /> : <div className="conns-list">
              {shown.length === 0 ? empty : sections ? (
                sections.map((g) => (
                  <div className="cgroup" key={g.name}>
                    {groupHead(g)}
                    <Fold open={!folded[by + g.name]}><div className="table conns">{g.list.map(row)}</div></Fold>
                  </div>
                ))
              ) : <div className="table conns">{shown.map(row)}</div>}
            </div>}
          </div>
        </div>
        {selected && <Detail c={selected} sourceLabel={labels[selected.metadata.sourceIP]} at={snapshot.at} closeDisabled={!activeIDs.has(selected.id) || closing.has(selected.id)} onClose={() => setSel(null)} onKill={() => close([selected])}
          onAddRule={(at) => setRuleFor({ c: selected, at })} />}
      </div>
      <Popover anchor={menu?.at ?? null} open={!!menu} onClose={() => setMenu(null)}>
        {menu && <Menu close={() => setMenu(null)} items={[
          { label: t("Add rule…"), onClick: () => setRuleFor(menu) },
          { label: t("Copy host"), onClick: () => App.CopyText(hostOf(menu.c)).then(() => toast(t("Copied"))) },
          "sep",
          { label: t("Close connection"), danger: true, onClick: () => close([menu.c]) },
        ]} />}
      </Popover>
      <RuleEditor anchor={ruleFor?.at ?? null} onClose={() => setRuleFor(null)} choices={ruleFor ? suggestions(ruleFor.c) : []} />
    </div>
  );
}

function Rate({ up, down }: { up: number; down: number }) {
  if (!up && !down) return <span className="zero">—</span>;
  return <>{up > 0 && <span className="up">↑ {speed(up)}</span>}{down > 0 && <span className="down">↓ {speed(down)}</span>}</>;
}

function Chain({ c }: { c: Conn }) {
  const [exit, ...via] = c.chains ?? [];
  if (!exit) return <span className="cell zero">—</span>;
  const tone = exit === "DIRECT" ? "direct" : /^REJECT/.test(exit) ? "reject" : "proxy";
  return <span className="cell chain" title={chainOf(c)}>
    {via.length > 0 && <span className="via">{via.slice().reverse().join(" → ")} →</span>}
    <span className={"exit " + tone}>{exit}</span>
  </span>;
}

function Detail({ c, sourceLabel, at, closeDisabled, onClose, onKill, onAddRule }: { c: Conn; sourceLabel?: string; at: number; closeDisabled: boolean; onClose: () => void; onKill: () => void; onAddRule: (at: HTMLElement) => void }) {
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
      <div className="detail-tabs"><Segmented className="track small fill" value={raw ? "raw" : "details"} onChange={(v) => setRaw(v === "raw")}
        options={[{ value: "details", label: t("Details") }, { value: "raw", label: t("Raw JSON") }]} /></div>
      <div className="detail-body" key={c.id + (raw ? ":raw" : ":details")}>
        {raw ? <pre className="conn-json">{json}</pre> : <dl>{rows.map(([k, v]) => <div key={k}>
          <dt>{k}</dt><dd onDoubleClick={() => copy(v)} title={t("Double-click to copy")}>{v}</dd>
          <button className="icon" title={t("Copy {field}", { field: k })} aria-label={t("Copy {field}", { field: k })} onClick={() => copy(v)}><Copy size={12} /></button>
        </div>)}</dl>}
      </div>
      <div className="detail-foot">
        {!raw && <button className="btn small" onClick={(e) => onAddRule(e.currentTarget)}>{t("Add rule…")}</button>}
        <button className="btn small" onClick={() => copy(raw ? json : rows.map(([k, v]) => `${k}: ${v}`).join("\n"))}>{t(raw ? "Copy JSON" : "Copy details")}</button>
        <div className="grow" />
        <button className="btn small danger" disabled={closeDisabled} onClick={onKill}>{t("Close connection")}</button>
      </div>
    </aside>
  );
}
