import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { useGroups } from "../useGroups";
import { Fold } from "../components/Fold";
import { Segmented } from "../components/Segmented";
import { toast } from "../components/Toast";
import { Bolt, Chevron, Columns, Refresh, Rows, Search, Sort } from "../components/Icons";
import { ago, bytes, delayClass, fmtDelay } from "../format";
import type { Group, Member, Provider } from "../api";
import { startCore } from "../actions";

type Tab = "groups" | "providers";

// below this width two columns would leave a card one node wide
const TWO_COLUMNS_MIN = 760;

export function Proxies() {
  const t = useT();
  const core = useStore((s) => s.state?.core);
  const { groups, providers, select, testGroup, testProvider, testOne, testAll, updateProvider, testing, progress, flash } = useGroups({ providers: true, testOnOpen: true });
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [sorted, setSorted] = useState(false);
  // ?view=proxies#providers opens on the providers
  const [tab, setTab] = useState<Tab>(location.hash === "#providers" ? "providers" : "groups");
  const [updatingAll, setUpdatingAll] = useState(false);
  const [twoCols, setTwoCols] = useState(() => { try { return localStorage.getItem("proxies.columns") !== "1"; } catch { return true; } });
  const [box, width] = useWidth();
  const [q, setQ] = useState("");
  // while searching every match is open; a card folded then stays folded
  // only until the query changes
  const [folded, setFolded] = useState<Record<string, boolean>>({});
  useEffect(() => setFolded({}), [q]);
  const toggleCols = () => {
    setTwoCols(!twoCols);
    try { localStorage.setItem("proxies.columns", twoCols ? "1" : "2"); } catch {}
  };

  if (core !== "running") {
    return (
      <div className="view">
        <div className="empty-state"><b>{t("Core is not running")}</b>{t("Start the core to see proxies.")}<div><button className="btn primary" onClick={startCore}>{t("Start core")}</button></div></div>
      </div>
    );
  }
  if (!groups) return <div className="view"><GroupsSkeleton /></div>;
  const shown = groups.filter((g) => !g.hidden);
  const pvs = providers ?? [];
  // the tabs show only when there is a provider to switch to
  const view: Tab = pvs.length ? tab : "groups";
  if (view === "groups" && shown.length === 0 && !pvs.length) return <div className="view"><div className="empty-state"><b>{t("No proxy groups")}</b>{t("This profile has no proxy groups.")}</div></div>;

  // a group whose name matches shows all its nodes, any other only the
  // nodes whose names match, and not at all when none do
  const needle = q.trim().toLowerCase();
  const has = (name: string) => name.toLowerCase().includes(needle);
  const narrow = <T extends { name: string; members?: Member[] | null }>(x: T): T | null => {
    if (!needle || has(x.name)) return x;
    const members = (x.members ?? []).filter((m) => has(m.name));
    return members.length ? { ...x, members } : null;
  };
  const groupsShown = shown.map(narrow).filter((g): g is Group => !!g);
  const pvsShown = pvs.map(narrow).filter((p): p is Provider => !!p);

  const isOpen = (key: string, dflt: boolean) => (needle ? !folded[key] : open[key] ?? dflt);
  const toggle = (key: string, dflt: boolean) => needle ? setFolded((f) => ({ ...f, [key]: !f[key] })) : setOpen((o) => ({ ...o, [key]: !(o[key] ?? dflt) }));
  const pkey = (p: Provider) => "provider/" + p.name;
  const updatable = pvs.filter((p) => p.vehicleType !== "Inline");
  const updateAll = async () => {
    setUpdatingAll(true);
    let ok = true;
    for (const p of updatable) ok = (await updateProvider(p.name)) && ok;
    setUpdatingAll(false);
    if (ok) toast(t("Updated {name}", { name: t("Providers") }));
  };

  const wide = width >= TWO_COLUMNS_MIN;
  const cols = twoCols && wide ? 2 : 1;

  return (
    <div className="view proxies-view" ref={box}>
      <div className="view-head">
        <h2>{t("Proxies")}</h2>
        {pvs.length ? (
          <Segmented className="track small" value={view} onChange={setTab} options={[
            { value: "groups", label: `${t("Groups")} ${shown.length}` },
            { value: "providers", label: `${t("Providers")} ${pvs.length}` },
          ]} />
        ) : <span className="sub">{shown.length}</span>}
        <div className="view-tools">
          <label className="search"><Search /><input placeholder={t("Search groups and nodes")} value={q} onChange={(e) => setQ(e.target.value)} onKeyDown={(e) => e.key === "Escape" && setQ("")} /></label>
          {wide && <button className="icon" title={twoCols ? t("One column") : t("Two columns")} onClick={toggleCols}>{twoCols ? <Rows size={14} /> : <Columns size={14} />}</button>}
          <button className={"btn small" + (sorted ? " on" : "")} onClick={() => setSorted(!sorted)}><Sort size={13} />{sorted ? t("Sort by latency") : t("Default order")}</button>
          {view === "providers" && updatable.length > 0 && (
            <button className="btn small" disabled={updatingAll} onClick={updateAll}><Refresh size={13} />{updatingAll ? t("Updating…") : t("Update all")}</button>
          )}
          <button className={"btn small" + (progress ? " zap" : "")} disabled={!!testing["all/"]} onClick={testAll}><Bolt size={13} />{progress || t("Test all")}</button>
        </div>
      </div>
      {needle && (view === "groups" ? groupsShown : pvsShown).length === 0 && <div className="empty-state"><b>{t("No matches")}</b>{t("Nothing is named like “{q}”.", { q: q.trim() })}</div>}
      {view === "groups" ? (
        <Masonry cols={cols} items={groupsShown.map((g) => ({
          key: g.name,
          // a card's column follows its size when open by default, not its
          // state now: folding a card doesn't send the others across
          weight: g.name === "GLOBAL" ? 1 : 1 + Math.ceil((g.members?.length ?? 0) / 2),
          node: (
            <GroupCard
              g={g}
              open={isOpen(g.name, g.name !== "GLOBAL")}
              toggle={() => toggle(g.name, g.name !== "GLOBAL")}
              sorted={sorted}
              onSelect={(n) => select(g.name, n)}
              onTest={() => testGroup(g)}
              onTestOne={testOne}
              testing={testing}
              flash={flash}
            />
          ),
        }))} />
      ) : (
        <Masonry cols={cols} items={pvsShown.map((p) => ({
          key: pkey(p),
          weight: 1,
          node: (
            <ProviderCard
              p={p}
              open={isOpen(pkey(p), false)}
              toggle={() => toggle(pkey(p), false)}
              sorted={sorted}
              onTest={() => testProvider(p)}
              onUpdate={async () => { if (await updateProvider(p.name)) toast(t("Updated {name}", { name: p.name })); }}
              onTestOne={testOne}
              testing={testing}
              testKey={pkey(p)}
            />
          ),
        }))} />
      )}
    </div>
  );
}

// Cards in columns, each to the shortest so far, keeping their order within
// a column; one column is the plain list.
function Masonry({ cols, items }: { cols: number; items: { key: string; weight: number; node: ReactNode }[] }) {
  if (cols === 1) return <>{items.map((it) => <Keyed key={it.key}>{it.node}</Keyed>)}</>;
  const lanes: (typeof items)[] = Array.from({ length: cols }, () => []);
  const load = new Array(cols).fill(0);
  for (const it of items) {
    const i = load.indexOf(Math.min(...load));
    lanes[i].push(it);
    load[i] += it.weight;
  }
  return (
    <div className="pcols">
      {lanes.map((lane, i) => <div className="pcol" key={i}>{lane.map((it) => <Keyed key={it.key}>{it.node}</Keyed>)}</div>)}
    </div>
  );
}
const Keyed = ({ children }: { children: ReactNode }) => <>{children}</>;

// the width of an element, through a callback ref: the page mounts its box
// only once the core and the groups are there
function useWidth() {
  const [width, setWidth] = useState(0);
  const ro = useRef<ResizeObserver | null>(null);
  const ref = useCallback((el: HTMLDivElement | null) => {
    ro.current?.disconnect();
    ro.current = null;
    if (!el) return;
    ro.current = new ResizeObserver(() => setWidth(el.clientWidth));
    ro.current.observe(el);
  }, []);
  return [ref, width] as const;
}

export function GroupCard({ g, open, toggle, sorted, onSelect, onTest, onTestOne, testing, flash, compact }: {
  g: Group; open: boolean; toggle: () => void; sorted: boolean;
  onSelect: (name: string) => void; onTest: () => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; flash: string; compact?: boolean;
}) {
  const t = useT();
  const now = (g.members ?? []).find((m) => m.name === g.now);
  return (
    <div className={"group" + (open ? " open" : "")}>
      <div className="group-head" onClick={toggle}>
        <Chevron className={"chev" + (open ? " open" : "")} />
        <div className="who">
          <div className="name">{g.name}<span className="gtype">{g.type}</span>{g.module && <span className="gtype">{t("Module")}</span>}</div>
          <div className="sub">{g.now || "—"}</div>
        </div>
        {now && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay, t)}</span>}
        <button className={"icon" + (testing[g.name] ? " zap" : "")} title={t("Test")} onClick={(e) => { e.stopPropagation(); onTest(); }}><Bolt size={14} /></button>
      </div>
      <Fold open={open}>
        <NodeGrid members={g.members} sorted={sorted} now={g.now} selectable={g.type === "Selector"} flashKey={flash.startsWith(g.name + "/") ? flash.slice(g.name.length + 1) : ""} onSelect={onSelect} onTestOne={onTestOne} testing={testing} compact={compact} />
      </Fold>
    </div>
  );
}

// A provider's nodes, under a head that says how many, when it was fetched
// and how much of the subscription is used, with a test and an update.
function ProviderCard({ p, open, toggle, sorted, onTest, onUpdate, onTestOne, testing, testKey }: {
  p: Provider; open: boolean; toggle: () => void; sorted: boolean;
  onTest: () => void; onUpdate: () => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; testKey: string;
}) {
  const t = useT();
  const members = p.members ?? [];
  const tested = members.filter((m) => m.delay !== 0);
  const alive = tested.filter((m) => m.delay > 0);
  // an inline provider is never fetched: its time is when the core loaded it
  const fetched = p.vehicleType !== "Inline" && p.updatedAt && new Date(p.updatedAt).getFullYear() > 2000;
  const expired = p.expire ? p.expire * 1000 < Date.now() : false;
  const sub = [
    t("{n} nodes", { n: members.length }),
    fetched ? t("Updated {t}", { t: ago(p.updatedAt, t) }) : "",
    p.expire ? (expired ? t("Expired") : t("Expires {d}", { d: new Date(p.expire * 1000).toLocaleDateString() })) : "",
  ].filter(Boolean).join(" · ");
  const used = (p.upload ?? 0) + (p.download ?? 0);
  const total = p.total ?? 0;
  const pct = total ? Math.min(100, (used / total) * 100) : 0;
  const updating = testing[testKey + "/update"];
  return (
    <div className={"group" + (open ? " open" : "")}>
      <div className="group-head" onClick={toggle}>
        <Chevron className={"chev" + (open ? " open" : "")} />
        <div className="who">
          <div className="name">{p.name}<span className="gtype">{p.vehicleType}</span></div>
          <div className="sub">{sub}</div>
          {total > 0 && (
            <div className="usage" title={`${bytes(used)} / ${bytes(total)}`}>
              <div className="bar"><i style={{ width: pct + "%" }} className={pct > 90 ? "hot" : ""} /></div>
              <span>{bytes(used)} / {bytes(total)}</span>
            </div>
          )}
        </div>
        {tested.length > 0 && <span className={"delay " + (alive.length === 0 ? "fail" : alive.length < tested.length ? "ok" : "good")} title={t("Reachable nodes")}>{alive.length}/{tested.length}</span>}
        {p.vehicleType !== "Inline" && (
          <button className={"icon" + (updating ? " spin" : "")} title={t("Update")} disabled={updating} onClick={(e) => { e.stopPropagation(); onUpdate(); }}><Refresh size={14} /></button>
        )}
        <button className={"icon" + (testing[testKey] ? " zap" : "")} title={t("Test")} onClick={(e) => { e.stopPropagation(); onTest(); }}><Bolt size={14} /></button>
      </div>
      <Fold open={open}>
        <NodeGrid members={members} sorted={sorted} selectable={false} onTestOne={onTestOne} testing={testing} />
      </Fold>
    </div>
  );
}

function NodeGrid({ members: ms, sorted, now, selectable, flashKey, onSelect, onTestOne, testing, compact }: {
  members?: Member[] | null; sorted: boolean; now?: string; selectable: boolean; flashKey?: string;
  onSelect?: (name: string) => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; compact?: boolean;
}) {
  const t = useT();
  const members = useMemo(() => {
    const list = ms ?? [];
    if (!sorted) return list;
    const key = (d: number) => (d > 0 ? d : d < 0 ? 1e9 : 1e8);
    return [...list].sort((a, b) => key(a.delay) - key(b.delay));
  }, [ms, sorted]);
  return (
    <div className={"nodes" + (compact ? " compact" : "")}>
      {members.map((m, i) => (
        <button
          key={m.name}
          className={"node stagger" + (m.name === now ? " on" : "") + (selectable ? "" : " fixed") + (flashKey === m.name ? " flash" : "")}
          style={{ ["--i" as string]: Math.min(i, 24) }}
          // with nothing to select (a provider's nodes), a click tests
          onClick={(e) => (e.altKey || !onSelect ? onTestOne(m.name) : selectable && m.name !== now && onSelect(m.name))}
          title={m.name + "\n" + (onSelect ? "⌥-click: " + t("test this node only") : t("Click to test this node"))}
        >
          <span className="nname">{m.name}</span>
          <span className="nmeta">
            <span className="ntype">{m.type}{m.udp ? " · UDP" : ""}</span>
            <span
              className={"delay " + (testing["#" + m.name] ? "testing" : delayClass(m.delay))}
              onClick={(e) => { e.stopPropagation(); onTestOne(m.name); }}
            >
              {testing["#" + m.name] ? "···" : fmtDelay(m.delay, t)}
            </span>
          </span>
        </button>
      ))}
    </div>
  );
}


function GroupsSkeleton() {
  return (
    <>
      {[0, 1, 2].map((i) => (
        <div className="group" key={i} style={{ padding: 14 }}>
          <div className="skel" style={{ width: 120, height: 14 }} />
          <div className="skel" style={{ width: 80, height: 11, marginTop: 6 }} />
        </div>
      ))}
    </>
  );
}
