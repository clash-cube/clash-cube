import { useEffect, useRef, useState, type CSSProperties } from "react";
import { useConnectionStore } from "../connectionStore";
import type { Conn } from "../connections";
import { useStore } from "../store";
import { useT } from "../i18n";
import { speed } from "../format";
import { AppIcon } from "./AppIcon";

const ROWS = 4;
const ROW_H = 22;
// the order changes at most this often, so the rows don't shuffle each second
const RESORT = 3000;
// how long an app that went quiet stays, faded, before it leaves
const LINGER = 5000;

type Client = { key: string; name: string; path: string; core: boolean };
type Row = Client & { up: number; down: number; idle: number; gone: boolean };

// clientOf names the app a connection belongs to as the backend's appOf
// does: the outermost .app bundle on its path, else the process, else the
// client's address; the core's own connections are mihomo.
export function clientOf(c: Conn): Client {
  const m = c.metadata;
  if (m.type === "Inner") return { key: "\0core", name: "mihomo", path: "", core: true };
  if (m.processPath) {
    const i = m.processPath.indexOf(".app/");
    if (i >= 0) {
      const bundle = m.processPath.slice(0, i + 4);
      return { key: bundle, name: bundle.slice(bundle.lastIndexOf("/") + 1, -4), path: bundle, core: false };
    }
    return { key: m.processPath, name: m.process || m.processPath.split("/").pop() || "", path: m.processPath, core: false };
  }
  const name = m.process || m.sourceIP || m.type;
  return { key: "\0" + name, name, path: "", core: false };
}

// useClients follows the main window's connection feed, whose speeds are
// already averaged over a few seconds, and keeps a calm list of it: new
// apps join at once, the order settles every few seconds, and one that
// went quiet fades out rather than vanishing.
function useClients() {
  const snapshot = useConnectionStore((s) => s.snapshot);
  const [state, setState] = useState<{ rows: Row[]; total: number }>({ rows: [], total: 0 });
  const rows = useRef<Row[]>([]);
  const sorted = useRef(0);
  useEffect(() => {
    const now = snapshot.at;
    const rates = new Map<string, Client & { up: number; down: number }>();
    let total = 0;
    for (const c of snapshot.active) {
      const k = clientOf(c);
      const r = rates.get(k.key) ?? { ...k, up: 0, down: 0 };
      r.up += c.up;
      r.down += c.down;
      total += c.up + c.down;
      rates.set(k.key, r);
    }
    const next: Row[] = [];
    for (const r of rows.current) {
      const at = rates.get(r.key);
      rates.delete(r.key);
      const up = at?.up ?? 0, down = at?.down ?? 0;
      const moving = up + down >= 1;
      // faded out on the last round: gone now
      if (r.gone && !moving) continue;
      const idle = moving ? 0 : r.idle || now;
      next.push({ ...r, up, down, idle, gone: !moving && now - idle > LINGER });
    }
    let joined = false;
    for (const r of rates.values()) {
      if (r.up + r.down < 1) continue;
      next.push({ ...r, idle: 0, gone: false });
      joined = true;
    }
    if (joined || now - sorted.current >= RESORT) {
      next.sort((a, b) => (b.gone ? -1 : b.up + b.down) - (a.gone ? -1 : a.up + a.down));
      sorted.current = now;
    }
    rows.current = next;
    setState({ rows: next, total });
  }, [snapshot]);
  return state;
}

// TrafficClients is the traffic card's right column: the apps moving the
// most now, each with its icon, speed and a thin bar of its share, upload
// and download in the chart's colours. A click shows its connections.
export function TrafficClients() {
  const t = useT();
  const setView = useStore((s) => s.setView);
  const { rows, total } = useClients();
  const live = rows.slice(0, ROWS).some((r) => !r.gone);
  const pct = (n: number) => (total > 0 ? `${(n / total) * 100}%` : "0%");
  return (
    <div className="tclients">
      <div className="tclients-head">{t("Top Clients")}</div>
      <div className="tc-list" style={{ height: ROWS * ROW_H }}>
        {!live && <div className="tc-empty">{t("No active apps")}</div>}
        {rows.slice(0, ROWS + 2).map((r, i) => (
          <button
            key={r.key}
            className={"tc-row" + (i >= ROWS || r.gone ? " out" : r.idle ? " idle" : "")}
            style={{ translate: `0 ${Math.min(i, ROWS) * ROW_H}px` } as CSSProperties}
            title={`${r.name}\n↑ ${speed(r.up)}   ↓ ${speed(r.down)}`}
            tabIndex={i >= ROWS || r.gone ? -1 : 0}
            onClick={() => { useStore.setState({ connQuery: r.name }); setView("connections"); }}
          >
            <AppIcon path={r.path} core={r.core} />
            <span className="tc-body">
              <span className="tc-line">
                <span className="tc-name">{r.name}</span>
                <span className="tc-speed num">{speed(r.up + r.down)}</span>
              </span>
              <span className="tc-share"><i className="up" style={{ width: pct(r.up) }} /><i className="down" style={{ width: pct(r.down) }} /></span>
            </span>
          </button>
        ))}
      </div>
    </div>
  );
}
