import { useMemo, useRef, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { App, Proxy, type Connection } from "../api";
import { usePoll } from "../usePoll";
import { bytes, duration, speed } from "../format";
import { Chevron, Close, Search } from "../components/Icons";
import { Segmented } from "../components/Segmented";
import { Fold } from "../components/Fold";
import { toast } from "../components/Toast";
import { run } from "../actions";

type GroupBy = "none" | "process" | "host" | "rule";

// a connection with its speed since the last poll
type Conn = Connection & { up: number; down: number };

const hostOf = (c: Connection) => c.metadata.host || c.metadata.destinationIP;
// Inner connections originate in the core and have no OS process metadata.
const processName = (c: Connection) => c.metadata.type === "Inner" ? "mihomo" : c.metadata.process;
const processOf = (c: Connection) => processName(c) || c.metadata.sourceIP || "—";
const ruleOf = (c: Connection) => (c.rulePayload ? `${c.rule}(${c.rulePayload})` : c.rule);
const chainOf = (c: Connection) => (c.chains ?? []).slice().reverse().join(" → ");

// Connections lists what goes through the core, grouped as Surge's
// dashboard groups requests: by process, host or rule, with the selected
// connection's details beside the list.
export function Connections() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const [conns, setConns] = useState<Conn[]>([]);
  const [q, setQ] = useState("");
  const [net, setNet] = useState<"all" | "tcp" | "udp">("all");
  const [by, setBy] = useState<GroupBy>("process");
  const [folded, setFolded] = useState<Record<string, boolean>>({});
  const [sel, setSel] = useState<string>("");
  const [closing, setClosing] = useState<Record<string, boolean>>({});
  const hist = useRef<Record<string, [number, number, number][]>>({});

  usePoll(async () => {
    if (!running) { setConns([]); return; }
    try {
      const next = (await Proxy.Connections()).connections ?? [];
      // speed is averaged over the last few polls: the core's counters
      // move in bursts, and a one-second delta flickers to zero between them
      const now = Date.now();
      for (const c of next) {
        const h = (hist.current[c.id] ??= []);
        h.push([now, c.upload, c.download]);
        while (h.length > 1 && now - h[0][0] > 4000) h.shift();
      }
      const alive = new Set(next.map((c) => c.id));
      for (const id in hist.current) if (!alive.has(id)) delete hist.current[id];
      setConns(next.map((c) => {
        const h = hist.current[c.id];
        const [t0, u0, d0] = h[0];
        const secs = (now - t0) / 1000;
        return { ...c, up: secs > 0 ? (c.upload - u0) / secs : 0, down: secs > 0 ? (c.download - d0) / secs : 0 };
      }));
    } catch {}
  }, 1000, [running]);

  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return conns.filter((c) => {
      if (net !== "all" && c.metadata.network !== net) return false;
      if (!s) return true;
      const m = c.metadata;
      return [m.host, m.destinationIP, processName(c), m.processPath, c.rule, c.rulePayload, ...(c.chains ?? [])].some((v) => v?.toLowerCase().includes(s));
    });
  }, [conns, q, net]);

  const groups = useMemo(() => {
    if (by === "none") return null;
    const key = by === "process" ? processOf : by === "host" ? hostOf : ruleOf;
    const m = new Map<string, Conn[]>();
    for (const c of shown) {
      const k = key(c);
      const list = m.get(k);
      if (list) list.push(c); else m.set(k, [c]);
    }
    return [...m.entries()]
      .map(([name, list]) => ({
        name, list,
        up: list.reduce((n, c) => n + c.up, 0), down: list.reduce((n, c) => n + c.down, 0),
        total: list.reduce((n, c) => n + c.upload + c.download, 0),
      }))
      .sort((a, b) => b.down + b.up - (a.down + a.up) || b.total - a.total);
  }, [shown, by]);

  const selected = conns.find((c) => c.id === sel);

  const close = async (id: string) => {
    setClosing((c) => ({ ...c, [id]: true }));
    await run(Proxy.CloseConnection(id));
    setTimeout(() => setConns((cs) => cs.filter((c) => c.id !== id)), 220);
  };
  const closeGroup = (list: Conn[]) => list.forEach((c) => close(c.id));

  const row = (c: Conn) => {
    const m = c.metadata;
    return (
      <div className={"trow" + (closing[c.id] ? " leaving" : "") + (sel === c.id ? " sel" : "")} key={c.id} onClick={() => setSel(sel === c.id ? "" : c.id)}>
        <span className="cell host">
          <span className="name">{hostOf(c)}:{m.destinationPort}</span>
          <span className="sub">{m.network.toUpperCase()} · {by === "process" ? ruleOf(c) : processOf(c)}</span>
        </span>
        <span className="cell" title={chainOf(c)}>{chainOf(c)}</span>
        <span className="cell r num">{c.up || c.down ? <span className="live">{speed(c.down)}</span> : bytes(c.upload + c.download)}</span>
        <span className="cell r num">{duration(c.start as unknown as string)}</span>
        <span className="cell r"><button className="icon" onClick={(e) => { e.stopPropagation(); close(c.id); }}><Close size={12} /></button></span>
      </div>
    );
  };

  return (
    <div className="view conns-view">
      <div className="view-head">
        <h2>{t("Connections")}</h2>
        <span className="sub">{t("{n} connections", { n: conns.length })}</span>
        <div className="view-tools">
          <Segmented className="track small" value={by} onChange={setBy} options={[
            { value: "process", label: t("Process") }, { value: "host", label: t("Host") }, { value: "rule", label: t("Rule") }, { value: "none", label: t("List") },
          ]} />
          <Segmented className="track small" value={net} onChange={setNet} options={[{ value: "all", label: "All" }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }]} />
          <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => setQ(e.target.value)} /></label>
          <button className="btn small danger" disabled={!conns.length} onClick={() => run(Proxy.CloseAllConnections())}>{t("Close all")}</button>
        </div>
      </div>
      <div className={"conns-body" + (selected ? " with-detail" : "")}>
        <div className="conns-list">
          {shown.length === 0 ? (
            <div className="empty-state"><b>{t("No connections")}</b></div>
          ) : groups ? (
            groups.map((g) => {
              const open = !folded[by + g.name];
              return (
                <div className="list cgroup" key={g.name}>
                  <div className="cghead" onClick={() => setFolded((f) => ({ ...f, [by + g.name]: open }))}>
                    <Chevron className={"chev" + (open ? " open" : "")} />
                    <span className="cgname" title={g.name}>{g.name}</span>
                    <span className="cgcount">{g.list.length}</span>
                    <div className="grow" />
                    <span className="num cgspeed">{g.up || g.down ? <>↑ {speed(g.up)} ↓ {speed(g.down)}</> : bytes(g.total)}</span>
                    <button className="icon" title={t("Close all")} onClick={(e) => { e.stopPropagation(); closeGroup(g.list); }}><Close size={12} /></button>
                  </div>
                  <Fold open={open}><div className="table conns">{g.list.map(row)}</div></Fold>
                </div>
              );
            })
          ) : (
            <div className="list table conns">{shown.map(row)}</div>
          )}
        </div>
        {selected && <Detail c={selected} onClose={() => setSel("")} onKill={() => close(selected.id)} />}
      </div>
    </div>
  );
}

function Detail({ c, onClose, onKill }: { c: Conn; onClose: () => void; onKill: () => void }) {
  const t = useT();
  const m = c.metadata;
  const copy = (s: string) => { App.CopyText(s); toast(t("Copied")); };
  const rows: [string, string][] = [
    [t("Host"), `${hostOf(c)}:${m.destinationPort}`],
    ["IP", m.destinationIP || "—"],
    [t("Network"), `${m.network.toUpperCase()} · ${m.type}`],
    [t("Rule"), ruleOf(c)],
    [t("Chain"), chainOf(c)],
    [t("Process"), processName(c) || "—"],
    [t("Path"), m.processPath || "—"],
    [t("Source"), `${m.sourceIP}:${m.sourcePort}`],
    [t("Upload"), `${bytes(c.upload)} · ${speed(c.up)}`],
    [t("Download"), `${bytes(c.download)} · ${speed(c.down)}`],
    [t("Time"), `${duration(c.start as unknown as string)} · ${new Date(c.start as unknown as string).toLocaleTimeString([], { hour12: false })}`],
  ];
  return (
    <aside className="detail list">
      <div className="detail-head">
        <b title={hostOf(c)}>{hostOf(c)}</b>
        <button className="icon" onClick={onClose}><Close size={12} /></button>
      </div>
      <dl>
        {rows.map(([k, v]) => (
          <div key={k} onDoubleClick={() => copy(v)} title={t("Double-click to copy")}><dt>{k}</dt><dd>{v}</dd></div>
        ))}
      </dl>
      <div className="detail-foot">
        <button className="btn small" onClick={() => copy(hostOf(c))}>{t("Copy host")}</button>
        <button className="btn small danger" onClick={onKill}>{t("Close connection")}</button>
      </div>
    </aside>
  );
}
