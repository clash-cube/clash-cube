import { useMemo, useRef, useState } from "react";
import { useT } from "../i18n";
import type { Conn } from "../connections";
import { Popover } from "./Popover";
import { Segmented } from "./Segmented";
import { toastError } from "./Toast";

const storageKey = "connection-source-labels";

export function useSourceLabels() {
  const [labels, setLabels] = useState<Record<string, string>>(() => {
    try {
      const stored = JSON.parse(localStorage.getItem(storageKey) ?? "{}");
      return Object.fromEntries(Object.entries(stored).filter((entry) => typeof entry[1] === "string")) as Record<string, string>;
    } catch { return {}; }
  });
  const save = (ip: string, name: string) => {
    const next = { ...labels };
    if (name.trim()) next[ip] = name.trim(); else delete next[ip];
    try { localStorage.setItem(storageKey, JSON.stringify(next)); setLabels(next); }
    catch (error) { toastError(error); }
  };
  return { labels, save };
}

type Net = "all" | "tcp" | "udp";

export function ConnectionFilter({ net, onNet, connections, selected, onChange, labels, onSave }: {
  net: Net; onNet: (net: Net) => void;
  connections: Conn[]; selected: Set<string>; onChange: (ips: Set<string>) => void;
  labels: Record<string, string>; onSave: (ip: string, name: string) => void;
}) {
  const t = useT();
  const anchor = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const sources = useMemo(() => {
    const counts = new Map<string, number>();
    for (const c of connections) counts.set(c.metadata.sourceIP, (counts.get(c.metadata.sourceIP) ?? 0) + 1);
    for (const ip of [...selected, ...Object.keys(labels)]) if (!counts.has(ip)) counts.set(ip, 0);
    return [...counts].sort(([a], [b]) => a.localeCompare(b, undefined, { numeric: true }));
  }, [connections, selected, labels]);
  const set = (net !== "all" ? 1 : 0) + selected.size;
  return <>
    <button ref={anchor} className={"btn small" + (set ? " on" : "")} aria-expanded={open}
      onClick={() => setOpen(!open)}>{t("Filter")}{set > 0 && <span className="num">· {set}</span>}</button>
    <Popover anchor={anchor.current} open={open} onClose={() => setOpen(false)} width={300} align="end">
      <div className="conn-pop">
        <div className="conn-pop-sec">
          <div className="conn-pop-title">{t("Protocol")}</div>
          <Segmented className="track small fill" value={net} onChange={onNet} options={[
            { value: "all", label: t("All") }, { value: "tcp", label: "TCP" }, { value: "udp", label: "UDP" },
          ]} />
        </div>
        <div className="conn-pop-sec">
          <div className="conn-pop-title">{t("Sources")}
            {selected.size > 0 && <button className="link" onClick={() => onChange(new Set())}>{t("All sources")}</button>}</div>
          <div className="source-options">
            {sources.map(([ip, count]) => <div className="source-option" key={ip}>
              <label><input type="checkbox" checked={selected.has(ip)} onChange={() => {
                const next = new Set(selected); if (next.has(ip)) next.delete(ip); else next.add(ip); onChange(next);
              }} /><span>{ip || t("No source address")}</span><span className="sub">{count}</span></label>
              {ip && <input className="input" key={ip + labels[ip]} aria-label={t("Device label for {ip}", { ip })}
                placeholder={t("Device label")} defaultValue={labels[ip] ?? ""}
                onBlur={(e) => { if (e.target.value.trim() !== (labels[ip] ?? "")) onSave(ip, e.target.value); }}
                onKeyDown={(e) => { if (e.key === "Enter") e.currentTarget.blur(); }} />}
            </div>)}
            {sources.length === 0 && <span className="sub">{t("No connections")}</span>}
          </div>
        </div>
      </div>
    </Popover>
  </>;
}
