// .ts: the tests load this file in node directly
import { nodeLabel } from "./format.ts";
import type { Connection } from "../bindings/github.com/localhost-copilot/clashcube/internal/mihomoapi/models";

export type Conn = Connection & { up: number; down: number; closedAt?: number };
export type ConnectionSnapshot = { active: Conn[]; closed: Conn[]; at: number };
export type ConnectionSort = "time" | "host" | "speed" | "up" | "down" | "upload" | "download" | "total" | "process" | "source" | "rule" | "chain" | "network";
export type ConnectionTab = "active" | "closed" | "all";
export type ConnectionGroupBy = "none" | "process" | "host" | "rule" | "source";

export const hostOf = (c: Connection) => c.metadata.host || c.metadata.sniffHost || c.metadata.destinationIP;
export const processName = (c: Connection) => c.metadata.type === "Inner" ? "mihomo"
  : c.metadata.process || c.metadata.processPath.split(/[\\/]/).pop() || "";
export const processOf = (c: Connection) => processName(c) || c.metadata.sourceIP || "—";
export const ruleOf = (c: Connection) => c.rulePayload ? `${c.rule}(${c.rulePayload})` : c.rule;
export const chainOf = (c: Connection) => (c.chains ?? []).map(nodeLabel).reverse().join(" → ");
export const address = (host: string, port: string) => host ? `${host.includes(":") ? `[${host}]` : host}${port ? ":" + port : ""}` : "—";

export function groupConnections(conns: Conn[], by: ConnectionGroupBy) {
  if (by === "none") return null;
  const key = by === "source" ? (c: Conn) => c.metadata.sourceIP
    : by === "process" ? processOf : by === "host" ? hostOf : ruleOf;
  const groups = new Map<string, Conn[]>();
  for (const c of conns) {
    const name = key(c);
    const list = groups.get(name);
    if (list) list.push(c); else groups.set(name, [c]);
  }
  // Keep the sorted first-member order. Device identity is always its IP;
  // two devices with the same editable label must remain separate groups.
  return [...groups].map(([name, list]) => ({ name, list,
    up: list.reduce((n, c) => n + c.up, 0), down: list.reduce((n, c) => n + c.down, 0),
    total: list.reduce((n, c) => n + c.upload + c.download, 0),
  }));
}

export function filterConnections(conns: Conn[], query: string, network: string,
  sources: ReadonlySet<string> = new Set(), labels: Record<string, string> = {}): Conn[] {
  const words = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  return conns.filter((c) => {
    const m = c.metadata;
    if (sources.size && !sources.has(m.sourceIP)) return false;
    if (network !== "all" && m.network.toLowerCase() !== network) return false;
    const values = [c.id, m.host, m.sniffHost, m.destinationIP, m.destinationPort, m.remoteDestination,
      m.sourceIP, m.sourcePort, address(m.sourceIP, m.sourcePort), address(hostOf(c), m.destinationPort),
      labels[m.sourceIP], m.network, m.type, processName(c), m.processPath, c.rule, c.rulePayload, ...(c.chains ?? []).map(nodeLabel)]
      .map((v) => (v || "").toLowerCase());
    return words.every((word) => values.some((v) => v.includes(word)));
  });
}

export function compareConnections(key: ConnectionSort, ascending: boolean) {
  return (a: Conn, b: Conn) => {
    const value = (c: Conn): string | number => {
      switch (key) {
        case "host": return hostOf(c);
        case "process": return processOf(c);
        case "source": return address(c.metadata.sourceIP, c.metadata.sourcePort);
        case "rule": return ruleOf(c);
        case "chain": return chainOf(c);
        case "network": return c.metadata.network;
        case "speed": return c.up + c.down;
        case "total": return c.upload + c.download;
        case "time": return Date.parse(c.start);
        default: return c[key];
      }
    };
    const av = value(a), bv = value(b);
    const order = typeof av === "string" ? av.localeCompare(String(bv), undefined, { numeric: true }) : av - Number(bv);
    return (ascending ? order : -order) || a.id.localeCompare(b.id);
  };
}

// Tracks snapshots observed during this app session. End times are detection
// times and counters are the last observed values, not a core-supplied ledger.
export class ConnectionTracker {
  snapshot: ConnectionSnapshot = { active: [], closed: [], at: Date.now() };
  readonly closing = new Set<string>();
  private samples = new Map<string, [number, number, number][]>();
  private revision = 0;
  private session = 0;

  reset() {
    this.session++;
    this.revision++;
    this.samples.clear();
    this.closing.clear();
    this.snapshot = { active: [], closed: [], at: Date.now() };
  }

  // Invalidates outstanding reads when the core stops or the feed unmounts.
  invalidate() { this.revision++; }

  stop() {
    this.session++;
    this.invalidate();
    this.closing.clear();
    this.collect([], Date.now());
  }

  async refresh(fetch: () => Promise<Connection[]>, now = Date.now): Promise<void> {
    const revision = ++this.revision;
    const next = await fetch();
    if (revision === this.revision) this.collect(next, now());
  }

  collect(next: Connection[], now: number) {
    const ids = new Set(next.map((c) => c.id));
    const closed = this.snapshot.closed.filter((c) => !ids.has(c.id));
    for (const c of this.snapshot.active) {
      if (!ids.has(c.id)) closed.push({ ...c, up: 0, down: 0, closedAt: now });
    }
    for (const id of this.samples.keys()) if (!ids.has(id)) this.samples.delete(id);
    const active = next.map((c) => {
      let h = this.samples.get(c.id) ?? [];
      const last = h[h.length - 1];
      if (last && (c.upload < last[1] || c.download < last[2])) h = [];
      h.push([now, c.upload, c.download]);
      while (h.length > 1 && h[0][0] < now - 4000) h.shift();
      this.samples.set(c.id, h);
      const [at, up, down] = h[0];
      const seconds = (now - at) / 1000;
      return { ...c, up: seconds > 0 ? Math.max(0, (c.upload - up) / seconds) : 0,
        down: seconds > 0 ? Math.max(0, (c.download - down) / seconds) : 0 };
    });
    this.snapshot = { active, closed: closed.slice(-500), at: now };
  }

  // Always close captured IDs, never a global endpoint: new or filtered-out
  // connections must survive a bulk action. Failed closes remain active.
  async close(ids: string[], closeOne: (id: string) => Promise<unknown>, changed: () => void = () => {}) {
    const session = this.session;
    const active = new Set(this.snapshot.active.map((c) => c.id));
    const targets = [...new Set(ids)].filter((id) => active.has(id) && !this.closing.has(id));
    for (const id of targets) this.closing.add(id);
    changed();
    const failures: { id: string; error: unknown }[] = [];
    let next = 0;
    const worker = async () => {
      while (next < targets.length && session === this.session) {
        const id = targets[next++];
        try {
          await closeOne(id);
          if (session !== this.session) return;
          this.revision++;
          const now = Date.now();
          const c = this.snapshot.active.find((c) => c.id === id);
          if (c) {
            this.samples.delete(id);
            this.snapshot = { active: this.snapshot.active.filter((c) => c.id !== id),
              closed: [...this.snapshot.closed, { ...c, up: 0, down: 0, closedAt: now }].slice(-500), at: now };
          }
        } catch (error) {
          failures.push({ id, error });
        } finally {
          if (session === this.session) { this.closing.delete(id); changed(); }
        }
      }
    };
    await Promise.all(Array.from({ length: Math.min(5, targets.length) }, worker));
    return failures;
  }
}

// Connections closed in the last poll stay one cycle so they can fade out.
// Without a running core no later snapshot comes to retire them.
export function lingering(s: ConnectionSnapshot, running: boolean): Conn[] {
  return running ? s.closed.filter((c) => s.at - c.closedAt! < 900) : [];
}

// While the pointer rests on the table, speed sorts keep the order they last
// showed, so rows don't jump under the cursor. A new sort choice always
// re-ranks: the held order belongs to the sort it was taken for.
export type HeldOrder = { key: string; rank: Map<string, number> };
export function holdOrder(sorted: Conn[], key: string, hold: boolean, held: HeldOrder): HeldOrder {
  if (hold && held.key === key) {
    const at = (c: Conn) => held.rank.get(c.id) ?? Infinity;
    sorted.sort((a, b) => (at(a) - at(b)) || 0);
    return held;
  }
  return { key, rank: new Map(sorted.map((c, i) => [c.id, i])) };
}
