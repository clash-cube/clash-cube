import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Search } from "../components/Icons";
import { Segmented } from "../components/Segmented";
import { LogsTabs } from "./Events";
import { calm } from "../components/Sidebar";

const LEVELS = ["debug", "info", "warning", "error"] as const;
type Level = (typeof LEVELS)[number] | "all";

export function Logs() {
  const t = useT();
  const logs = useStore((s) => s.logs);
  const clear = useStore((s) => s.clearLogs);
  const [level, setLevel] = useState<Level>("all");
  const [q, setQ] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const [follow, setFollow] = useState(true);

  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    const min = level === "all" ? -1 : LEVELS.indexOf(level);
    return logs.filter((l) => LEVELS.indexOf(l.type as (typeof LEVELS)[number]) >= min && (!s || l.payload.toLowerCase().includes(s)));
  }, [logs, level, q]);
  const [leaving, setLeaving] = useState<typeof shown | null>(null);
  const painted = useRef(false);
  useEffect(() => { painted.current = true; }, []);
  useEffect(() => {
    if (!leaving) return;
    const timer = setTimeout(() => setLeaving(null), 200);
    return () => clearTimeout(timer);
  }, [leaving]);
  const clearRows = () => {
    if (!calm() && shown.length) setLeaving(shown);
    clear();
  };
  const displayed = leaving ?? shown;

  useLayoutEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [shown, follow]);
  useEffect(() => { setFollow(true); }, [level, q]);

  return (
    <div className="view logs-view">
      <div className="view-head">
        <LogsTabs value="logs" />
        {!follow && <span className="badge">{t("Paused")}</span>}
        <div className="view-tools">
          <Segmented className="track small" value={level} onChange={setLevel} options={[{ value: "all", label: "All" }, { value: "info", label: "Info" }, { value: "warning", label: "Warn" }, { value: "error", label: "Error" }]} />
          <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => setQ(e.target.value)} /></label>
          <button className="btn small" disabled={!!leaving} onClick={clearRows}>{t("Clear")}</button>
        </div>
      </div>
      <div className={"list logbox" + (leaving ? " c-clearing" : "")} ref={box} onScroll={(e) => {
        const el = e.currentTarget;
        setFollow(el.scrollTop + el.clientHeight >= el.scrollHeight - 8);
      }}>
        {displayed.length === 0 ? (
          <div className={"empty-state" + (painted.current ? " c-empty-in" : "")}><b>{t("No logs yet")}</b>{t("Logs appear here as the core writes them.")}</div>
        ) : displayed.map((l) => (
          <div className={"logline " + l.type} key={l.id}>
            <span className="at">{new Date(l.at).toLocaleTimeString([], { hour12: false })}</span>
            <span className="lv">{l.type.slice(0, 4)}</span>
            <span className="msg">{l.payload}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
