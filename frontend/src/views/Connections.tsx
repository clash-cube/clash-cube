import { useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Proxy, type Connection } from "../api";
import { usePoll } from "../usePoll";
import { bytes, duration } from "../format";
import { Close, Search } from "../components/Icons";
import { Segmented } from "../components/Segmented";
import { run } from "../actions";

export function Connections() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const [conns, setConns] = useState<Connection[]>([]);
  const [q, setQ] = useState("");
  const [net, setNet] = useState<"all" | "tcp" | "udp">("all");
  const [closing, setClosing] = useState<Record<string, boolean>>({});

  usePoll(async () => {
    if (!running) { setConns([]); return; }
    try { setConns((await Proxy.Connections()).connections ?? []); } catch {}
  }, 1000, [running]);

  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return conns.filter((c) => {
      if (net !== "all" && c.metadata.network !== net) return false;
      if (!s) return true;
      const m = c.metadata;
      return [m.host, m.destinationIP, m.process, c.rule, c.rulePayload, ...(c.chains ?? [])].some((v) => v?.toLowerCase().includes(s));
    });
  }, [conns, q, net]);

  const close = async (id: string) => {
    setClosing((c) => ({ ...c, [id]: true }));
    await run(Proxy.CloseConnection(id));
    setTimeout(() => setConns((cs) => cs.filter((c) => c.id !== id)), 220);
  };

  return (
    <div className="view">
      <div className="view-head">
        <h2>{t("Connections")}</h2>
        <span className="sub">{t("{n} connections", { n: conns.length })}</span>
        <div className="grow" />
        <Segmented className="track small" value={net} onChange={setNet} options={[{ value: "all", label: "All" }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" }]} />
        <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => setQ(e.target.value)} /></label>
        <button className="btn small danger" disabled={!conns.length} onClick={() => run(Proxy.CloseAllConnections())}>{t("Close all")}</button>
      </div>
      {shown.length === 0 ? (
        <div className="empty-state"><b>{t("No connections")}</b></div>
      ) : (
        <div className="list table conns">
          <div className="thead">
            <span>{t("Host")}</span><span>{t("Rule")}</span><span>{t("Chain")}</span><span className="r">↑ / ↓</span><span className="r">{t("Time")}</span><span />
          </div>
          {shown.map((c) => {
            const m = c.metadata;
            const host = (m.host || m.destinationIP) + ":" + m.destinationPort;
            return (
              <div className={"trow" + (closing[c.id] ? " leaving" : "")} key={c.id}>
                <span className="cell host" title={host}>
                  <span className="name">{host}</span>
                  <span className="sub">{m.network.toUpperCase()} · {m.process || m.sourceIP}</span>
                </span>
                <span className="cell" title={c.rulePayload}>{c.rule}{c.rulePayload ? <span className="sub"> {c.rulePayload}</span> : null}</span>
                <span className="cell" title={(c.chains ?? []).slice().reverse().join(" → ")}>{(c.chains ?? []).slice().reverse().join(" → ")}</span>
                <span className="cell r num">{bytes(c.upload)} / {bytes(c.download)}</span>
                <span className="cell r num">{duration(c.start as unknown as string)}</span>
                <span className="cell r"><button className="icon" onClick={() => close(c.id)}><Close size={12} /></button></span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
