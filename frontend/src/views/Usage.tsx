import { useEffect, useMemo, useState } from "react";
import { locale, useT } from "../i18n";
import { useStore, type View } from "../store";
import { App } from "../api";
import type { Report, Row, Bar } from "../../bindings/github.com/localhost-copilot/clashcube/internal/usage/models";
import { Segmented } from "../components/Segmented";
import { AppIcon } from "../components/AppIcon";
import { Chevron, Close, Globe, Route, Wifi } from "../components/Icons";
import { toastError } from "../components/Toast";
import { usePoll } from "../usePoll";
import { bytes } from "../format";

type Period = "day" | "week" | "month";
type Dim = "app" | "host" | "policy" | "network";
const SPAN: Record<Period, number> = { day: 1, week: 7, month: 30 };
const PAGE = 50;

const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const parse = (s: string) => { const [y, m, d] = s.split("-").map(Number); return new Date(y, m - 1, d); };
const shift = (s: string, days: number) => { const d = parse(s); d.setDate(d.getDate() + days); return ymd(d); };

// Usage is the traffic statistics kept across runs, as Surge's: a period's
// total, a bar for each hour (or day), and the apps, hosts, policies and
// networks it went to. A bar narrows the lists to its hour, or opens its day.
export function Usage() {
  const t = useT();
  const today = ymd(new Date());
  const [period, setPeriod] = useState<Period>(location.hash === "#week" ? "week" : location.hash === "#month" ? "month" : "day");
  const [end, setEnd] = useState(today);
  const [hour, setHour] = useState(-1);
  const [dim, setDim] = useState<Dim>("app");
  const [report, setReport] = useState<Report | null>(null);
  const [limit, setLimit] = useState(PAGE);
  const [armed, setArmed] = useState(false);
  const from = shift(end, 1 - SPAN[period]);

  const load = () => App.Usage(from, end, hour).then(setReport).catch(toastError);
  useEffect(() => { load(); }, [from, end, hour]);
  // today's statistics grow while the page shows
  usePoll(() => { if (end === today) load(); }, 5000, [from, end, hour, today]);
  useEffect(() => setLimit(PAGE), [dim, from, end, hour]);
  useEffect(() => { if (!armed) return; const id = setTimeout(() => setArmed(false), 3000); return () => clearTimeout(id); }, [armed]);

  const pick = (p: Period) => { setPeriod(p); setHour(-1); };
  const step = (d: number) => { setHour(-1); setEnd((e) => { const n = shift(e, d * SPAN[period]); return n > today ? today : n; }); };
  const openBar = (b: Bar, i: number) => {
    if (period === "day") setHour(hour === i ? -1 : i);
    else { setPeriod("day"); setEnd(b.key); setHour(-1); }
  };
  const clear = async () => {
    if (!armed) { setArmed(true); return; }
    setArmed(false);
    try { await App.ClearUsage(); load(); } catch (e) { toastError(e); }
  };

  const rows = report?.dims?.[dim] ?? [];
  const total = (report?.up ?? 0) + (report?.down ?? 0);
  const title = period === "day"
    ? (end === today ? t("Today") : end === shift(today, -1) ? t("Yesterday") : label(end))
    : `${label(from)} – ${label(end)}`;

  return (
    <div className="view usage-view">
      <div className="view-head">
        <OverviewTabs value="usage" />
        <div className="view-tools">
          <Segmented className="track small" value={period} onChange={pick} options={[
            { value: "day", label: t("Day") }, { value: "week", label: t("7 days") }, { value: "month", label: t("30 days") },
          ]} />
          <div className="usage-nav">
            <button className="icon" aria-label={t("Earlier")} onClick={() => step(-1)}><Chevron /></button>
            <button className="btn small" disabled={end === today} onClick={() => { setEnd(today); setHour(-1); }}>{t("Today")}</button>
            <button className="icon next" aria-label={t("Later")} disabled={end === today} onClick={() => step(1)}><Chevron /></button>
          </div>
          <button className={"btn small" + (armed ? " danger armed" : "")} onClick={clear}>{t(armed ? "Click again to clear" : "Clear")}</button>
        </div>
      </div>

      <div className="card usage-card">
        <div className="usage-sum">
          <div>
            <div className="usage-when">{title}{hour >= 0 && <button className="chip" onClick={() => setHour(-1)}>{hourLabel(hour)}<Close size={8} /></button>}</div>
            <div className="usage-total num">{bytes(total)}</div>
          </div>
          <div className="grow" />
          <Figure label={t("Upload")} value={bytes(report?.up ?? 0)} tone="up" />
          <Figure label={t("Download")} value={bytes(report?.down ?? 0)} tone="down" />
          {hour < 0 && <Figure label={t("Connections")} value={(report?.conns ?? 0).toLocaleString()} />}
        </div>
        <Bars bars={report?.bars ?? []} period={period} selected={period === "day" ? hour : -1} onPick={openBar} />
      </div>

      <div className="usage-dims">
        <Segmented className="track small" value={dim} onChange={setDim} options={[
          { value: "app", label: t("Apps") }, { value: "host", label: t("Hosts") },
          { value: "policy", label: t("Policies") }, { value: "network", label: t("Networks") },
        ]} />
        <span className="sub num">{rows.length > 0 && t("{n} items", { n: rows.length })}</span>
      </div>
      <div className="list usage-list">
        {rows.length === 0 ? (
          <div className="empty-state"><b>{t("No traffic")}</b>{t("Traffic through the core is counted while it runs, and kept for 90 days.")}</div>
        ) : rows.slice(0, limit).map((r, i) => <UsageRow key={r.key} row={r} dim={dim} max={rows[0].up + rows[0].down} total={total} i={i} />)}
        {rows.length > limit && <button className="row click usage-more" onClick={() => setLimit(limit + PAGE * 4)}>{t("Show {n} more", { n: rows.length - limit })}</button>}
      </div>
    </div>
  );
}

function Figure({ label, value, tone }: { label: string; value: string; tone?: "up" | "down" }) {
  return (
    <div className={"usage-fig" + (tone ? " " + tone : "")}>
      <div className="lbl">{tone && <i />}{label}</div>
      <div className="val num">{value}</div>
    </div>
  );
}

// Bars draws each hour's (or day's) traffic as one bar; the pointer's bar
// shows its numbers, and a click picks it.
function Bars({ bars, period, selected, onPick }: { bars: Bar[]; period: Period; selected: number; onPick: (b: Bar, i: number) => void }) {
  const t = useT();
  const [over, setOver] = useState(-1);
  const max = useMemo(() => Math.max(1, ...bars.map((b) => b.up + b.down)), [bars]);
  const tick = (b: Bar, i: number) => period === "day" ? (i % 6 === 0 ? String(i).padStart(2, "0") + ":00" : "")
    : period === "week" || i % 5 === 0 || i === bars.length - 1 ? short(b.key) : "";
  const tip = over >= 0 ? bars[over] : null;
  return (
    <div className={"usage-bars" + (selected >= 0 ? " picking" : "")} onMouseLeave={() => setOver(-1)}>
      <div className="usage-plot">
        {[.5, 1].map((f) => <span key={f} className="usage-grid" style={{ bottom: f * 100 + "%" }}><b className="num">{bytes(max * f, 0)}</b></span>)}
        {bars.map((b, i) => {
          const v = b.up + b.down;
          return (
            <button key={b.key} className={"usage-bar" + (i === selected ? " on" : "") }
              aria-label={`${period === "day" ? hourLabel(i) : b.key}: ${bytes(v)}`}
              onMouseEnter={() => setOver(i)} onFocus={() => setOver(i)} onClick={() => onPick(b, i)}>
              <i style={{ height: v ? `max(2px, ${(v / max) * 100}%)` : 0, ["--i" as string]: i }}>
                {b.up > 0 && <s style={{ height: (b.up / v) * 100 + "%" }} />}
              </i>
            </button>
          );
        })}
        {tip && (
          <div className={"usage-tip" + (over > bars.length / 2 ? " flip" : "")} style={{ ["--x" as string]: (over + .5) / bars.length }}>
            <div className="at">{period === "day" ? hourLabel(over) : label(tip.key)}</div>
            <div><i className="up" />{t("Upload")}<b className="num">{bytes(tip.up)}</b></div>
            <div><i />{t("Download")}<b className="num">{bytes(tip.down)}</b></div>
          </div>
        )}
      </div>
      <div className="usage-ticks">{bars.map((b, i) => <span key={b.key}>{tick(b, i)}</span>)}</div>
    </div>
  );
}

function UsageRow({ row, dim, max, total, i }: { row: Row; dim: Dim; max: number; total: number; i: number }) {
  const t = useT();
  const v = row.up + row.down;
  const share = total ? Math.round((v / total) * 1000) / 10 : 0;
  return (
    <div className="row usage-row" style={{ ["--i" as string]: Math.min(i, 12) }}>
      <span className="usage-ic">{icon(row, dim)}</span>
      <div className="who">
        <div className="name" title={row.name}>{name(t, row, dim)}</div>
        <span className="usage-share"><i style={{ width: (v / max) * 100 + "%" }} /></span>
      </div>
      <span className="usage-cell num up">↑ {bytes(row.up)}</span>
      <span className="usage-cell num down">↓ {bytes(row.down)}</span>
      <span className="usage-cell num total"><b>{bytes(v)}</b><span>{share < .1 ? "<0.1" : share}%</span></span>
      {dim !== "network" && <span className="usage-cell num conns" title={t("Connections")}>{row.conns ? row.conns.toLocaleString() : "—"}</span>}
    </div>
  );
}

function icon(row: Row, dim: Dim) {
  if (dim === "app") return row.name === "mihomo" ? <AppIcon path="" core /> : row.path ? <AppIcon path={row.path} /> : <Globe />;
  if (dim === "network") return row.key === "wired" ? <Route /> : row.key.startsWith("wifi") ? <Wifi /> : <Globe />;
  if (dim === "policy") return <Route />;
  return <Globe />;
}

function name(t: (s: string) => string, row: Row, dim: Dim) {
  if (dim === "network") {
    if (row.key === "wired") return t("Wired network");
    if (row.key === "wifi") return t("Wi-Fi");
    if (row.key === "unknown") return t("Not connected");
  }
  return row.name || "—";
}

const label = (date: string) => parse(date).toLocaleDateString(locale(), { month: "short", day: "numeric", weekday: "short" });
const short = (date: string) => parse(date).toLocaleDateString(locale(), { month: "numeric", day: "numeric" });
const hourLabel = (h: number) => `${String(h).padStart(2, "0")}:00–${String(h + 1).padStart(2, "0")}:00`;

// The overview and the statistics share a tab; the page's head switches them.
export function OverviewTabs({ value }: { value: "overview" | "usage" }) {
  const t = useT();
  const setView = useStore((s) => s.setView);
  return (
    <Segmented<View>
      className="track small"
      value={value}
      onChange={setView}
      options={[{ value: "overview", label: t("Overview") }, { value: "usage", label: t("Traffic statistics") }]}
    />
  );
}
