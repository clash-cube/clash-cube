import { useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore, type View } from "../store";
import { Segmented } from "../components/Segmented";
import type { Event } from "../api";

type Filter = "all" | "warning";
const KINDS: Record<string, string> = { core: "Core", proxy: "System Proxy", network: "Network", profile: "Profiles", group: "Proxy groups" };

// What happened while the app ran, newest first: the core stopping, the
// network changing, an automatic group moving, a subscription failing.
export function Events() {
  const t = useT();
  const events = useStore((s) => s.events);
  const clear = useStore((s) => s.clearEvents);
  const [filter, setFilter] = useState<Filter>("all");

  const shown = useMemo(
    () => events.filter((e) => filter === "all" || e.level !== "info").slice().reverse(),
    [events, filter],
  );

  return (
    <div className="view events-view">
      <div className="view-head">
        <LogsTabs value="events" />
        <div className="view-tools">
          <Segmented className="track small" value={filter} onChange={setFilter} options={[{ value: "all", label: t("All") }, { value: "warning", label: t("Problems") }]} />
          <button className="btn small" onClick={clear}>{t("Clear")}</button>
        </div>
      </div>
      <div className="list eventbox">
        {shown.length === 0 ? (
          <div className="empty-state"><b>{t("No events")}</b>{t("Network changes, automatic group switches and errors appear here.")}</div>
        ) : shown.map((e) => (
          <div className={"event " + e.level} key={e.id}>
            <span className="dot" />
            <div>
              <div className="text">{eventText(t, e)}</div>
              <div className="kind">{t(KINDS[e.kind] ?? e.kind)}</div>
            </div>
            <span className="at num">{when(e.time)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// The logs and the events share a tab; the page's head switches them.
export function LogsTabs({ value }: { value: "logs" | "events" }) {
  const t = useT();
  const setView = useStore((s) => s.setView);
  return (
    <Segmented<View>
      className="track small"
      value={value}
      onChange={setView}
      options={[{ value: "logs", label: t("Logs") }, { value: "events", label: t("Events") }]}
    />
  );
}

function eventText(t: (s: string, vars?: Record<string, string>) => string, e: Event) {
  const args: Record<string, string> = {};
  for (const [k, v] of Object.entries(e.args ?? {})) if (v != null) args[k] = v;
  return t(e.text, args);
}

// the time of day, with the date when it isn't today
function when(at: string) {
  const d = new Date(at);
  const time = d.toLocaleTimeString([], { hour12: false });
  return d.toDateString() === new Date().toDateString() ? time : d.toLocaleDateString() + " " + time;
}
