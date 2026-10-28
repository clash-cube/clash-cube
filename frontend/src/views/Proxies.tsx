import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { useGroups } from "../useGroups";
import { Fold } from "../components/Fold";
import { Segmented } from "../components/Segmented";
import { toast } from "../components/Toast";
import { Popover, Menu, type Point } from "../components/Popover";
import { openPortDraft } from "../components/modules/port";
import { Bolt, Chevron, Close, Columns, FoldAll, UnfoldAll, Refresh, Rows, Search, Sort } from "../components/Icons";
import { ago, bytes, delayClass, fmtDelay, nodeLabel, shortType } from "../format";
import { useFlip } from "../useFlip";
import { App, type Group, type Member, type Provider } from "../api";
import { startCore } from "../actions";
import { reveal } from "../reveal";
import { chain } from "../proxyChain";
import { morph } from "../morph";
import { calm } from "../components/Sidebar";

type Tab = "groups" | "providers";

// below this width two columns would leave a card one node wide
const TWO_COLUMNS_MIN = 760;

export function Proxies() {
  const t = useT();
  const core = useStore((s) => s.state?.core);
  const { groups, providers, select, testGroup, testProvider, testOne, testAll, updateProvider, testing, progress, flash } = useGroups({ providers: true, testOnOpen: true });
  const [open, setOpen] = useState<Record<string, boolean>>(() => {
    try {
      const saved = JSON.parse(localStorage.getItem("proxies.open") ?? "{}");
      return saved && typeof saved === "object" && !Array.isArray(saved)
        ? Object.fromEntries(Object.entries(saved).filter((entry): entry is [string, boolean] => typeof entry[1] === "boolean")) : {};
    } catch { return {}; }
  });
  const initialOpen = useRef(open);
  const rememberedOpen = useRef(open);
  const rememberOpen = useCallback((key: string, value: boolean) => {
    const next = { ...rememberedOpen.current, [key]: value };
    rememberedOpen.current = next;
    setOpen(next);
    try { localStorage.setItem("proxies.open", JSON.stringify(next)); } catch {}
  }, []);
  const [sorted, setSorted] = useState(false);
  const [hideTimeouts, setHideTimeouts] = useState(() => { try { return localStorage.getItem("proxies.hideTimeouts") === "true"; } catch { return false; } });
  const [sortMenu, setSortMenu] = useState(false);
  const sortButton = useRef<HTMLButtonElement>(null);
  const [tested, setTested] = useState<{ name: string; owner: string; serial: number }>();
  const testNode = async (name: string, owner: string) => {
    await testOne(name);
    setTested((prev) => ({ name, owner, serial: (prev?.serial ?? 0) + 1 }));
  };
  // ?view=proxies#providers opens on the providers
  const [tab, setTab] = useState<Tab>(location.hash === "#providers" ? "providers" : "groups");
  const [updatingAll, setUpdatingAll] = useState(false);
  const [twoCols, setTwoCols] = useState(() => { try { return localStorage.getItem("proxies.columns") !== "1"; } catch { return true; } });
  const [box, width] = useWidth();
  const [q, setQ] = useState("");
  // a node's or group's context menu
  const [menu, setMenu] = useState<{ name: string; point: Point } | null>(null);
  const onMenu = (name: string, point: Point) => setMenu({ name, point });
  // while searching every match is open; a card folded then stays folded
  // only until the query changes
  const [folded, setFolded] = useState<Record<string, boolean>>({});
  useEffect(() => setFolded({}), [q]);
  const toggleCols = () => {
    morph(() => setTwoCols(!twoCols), "proxies-morph");
    try { localStorage.setItem("proxies.columns", twoCols ? "1" : "2"); } catch {}
  };
  // a group asked for from elsewhere (an AI service's route) is shown open,
  // scrolled to and marked for a moment, once the groups are there
  const target = useStore((s) => s.proxiesTarget);
  const [marked, setMarked] = useState("");
  useEffect(() => {
    if (!target || !groups) return;
    useStore.setState({ proxiesTarget: null });
    setTab("groups");
    setQ("");
    rememberOpen(target, true);
    setMarked(target);
    requestAnimationFrame(() => reveal(document.querySelector(`.group[data-group="${CSS.escape(target)}"]`)));
  }, [target, groups, rememberOpen]);
  useEffect(() => {
    if (!marked) return;
    const t = setTimeout(() => setMarked(""), 1600);
    return () => clearTimeout(t);
  }, [marked]);

  const shown = (groups ?? []).filter((g) => !g.hidden);
  const pvs = providers ?? [];
  const view: Tab = pvs.length ? tab : "groups";
  const needle = q.trim().toLowerCase();
  const has = (name: string) => name.toLowerCase().includes(needle);
  const narrow = <T extends { name: string; members?: Member[] | null }>(x: T): T | null => {
    if (!needle || has(x.name)) return x;
    const members = (x.members ?? []).filter((m) => has(m.name));
    return members.length ? { ...x, members } : null;
  };
  const groupsShown = shown.map(narrow).filter((g): g is Group => !!g);
  const pvsShown = pvs.map(narrow).filter((p): p is Provider => !!p);
  const visible = new Set((view === "groups" ? groupsShown : pvsShown).map((card) => card.name));
  const previous = useRef<{ view: Tab; needle: string; visible: Set<string>; entered: Set<string> }>();
  // Keep the entry class through unrelated renders (including search folds).
  const entered = previous.current?.view !== view ? new Set<string>()
    : previous.current.needle === needle ? previous.current.entered
    : new Set([...visible].filter((name) => !previous.current!.visible.has(name)));
  useEffect(() => {
    if (groups && core === "running") previous.current = { view, needle, visible, entered };
  });

  if (core !== "running") {
    return (
      <div className="view">
        <div className="empty-state"><b>{t("Core is not running")}</b>{t("Start the core to see proxies.")}<div><button className="btn primary" onClick={startCore}>{t("Start core")}</button></div></div>
      </div>
    );
  }
  if (!groups) return <div className="view"><GroupsSkeleton /></div>;
  if (view === "groups" && shown.length === 0 && !pvs.length) return <div className="view"><div className="empty-state"><b>{t("No proxy groups")}</b>{t("This profile has no proxy groups.")}</div></div>;

  const isOpen = (key: string) => (needle ? folded[key] !== true : open[key] === true);
  const toggle = (key: string) => needle ? setFolded((f) => ({ ...f, [key]: f[key] !== true })) : rememberOpen(key, rememberedOpen.current[key] !== true);
  const pkey = (p: Provider) => "provider/" + p.name;
  const shownKeys = view === "groups" ? groupsShown.map((g) => g.name) : pvsShown.map(pkey);
  const anyOpen = shownKeys.some(isOpen);
  const toggleAll = () => {
    const next = { ...rememberedOpen.current };
    for (const key of shownKeys) next[key] = !anyOpen;
    rememberedOpen.current = next;
    setOpen(next);
    try { localStorage.setItem("proxies.open", JSON.stringify(next)); } catch {}
  };
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
    <div className="view proxies-view" ref={box} data-motion={!calm() && typeof document.startViewTransition === "function"}>
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
          <button className="icon" disabled={!!needle} title={t(anyOpen ? "Collapse all" : "Expand all")} onClick={toggleAll}><span className="proxies-fold-icon" key={String(anyOpen)}>{anyOpen ? <FoldAll /> : <UnfoldAll />}</span></button>
          {wide && <button className="icon" title={twoCols ? t("One column") : t("Two columns")} onClick={toggleCols}><span className="proxies-column-icon" key={String(twoCols)}>{twoCols ? <Rows size={14} /> : <Columns size={14} />}</span></button>}
          <button ref={sortButton} className={"btn small proxies-sort" + (sorted ? " on" : "")} onClick={() => setSortMenu(!sortMenu)} aria-haspopup="menu" aria-expanded={sortMenu}><Sort size={13} />{sorted ? t("Sort by latency") : t("Default order")}{hideTimeouts && <i />}</button>
          {view === "providers" && updatable.length > 0 && (
            <button className="btn small" disabled={updatingAll} onClick={updateAll}><Refresh size={13} />{updatingAll ? t("Updating…") : t("Update all")}</button>
          )}
          <button className={"btn small" + (progress ? " zap" : "")} disabled={!!testing["all/"]} onClick={testAll}><Bolt size={13} />{progress || t("Test all")}</button>
        </div>
      </div>
      {needle && (view === "groups" ? groupsShown : pvsShown).length === 0 && <div className="empty-state"><b>{t("No matches")}</b>{t("Nothing is named like “{q}”.", { q: q.trim() })}</div>}
      <div className="proxies-list" key={view}>
      {view === "groups" ? (
        <Masonry cols={cols} items={groupsShown.map((g) => ({
          key: g.name,
          // Freeze expansion weights at mount so toggles never change lanes.
          weight: initialOpen.current[g.name] === true ? 1 + Math.ceil((g.members?.length ?? 0) / 2) : 1,
          node: (
            <GroupCard
              g={g}
              cardIndex={groups.findIndex((card) => card.name === g.name)}
              entering={entered.has(g.name)}
              groups={groups}
              fullMembers={groups.find((group) => group.name === g.name)?.members}
              marked={marked === g.name}
              open={isOpen(g.name)}
              toggle={() => toggle(g.name)}
              sorted={sorted}
              onSelect={(n) => select(g.name, n)}
              onTest={() => testGroup(g)}
              hideTimeouts={hideTimeouts} searching={!!needle} tested={tested?.owner === "group/" + g.name ? tested : undefined}
              onTestOne={(name) => testNode(name, "group/" + g.name)}
              testing={testing}
              flash={flash}
              onMenu={onMenu}
            />
          ),
        }))} />
      ) : (
        <Masonry cols={cols} items={pvsShown.map((p) => ({
          key: pkey(p),
          weight: initialOpen.current[pkey(p)] === true ? 1 + Math.ceil((p.members?.length ?? 0) / 2) : 1,
          node: (
            <ProviderCard
              p={p}
              cardIndex={pvs.findIndex((card) => card.name === p.name)}
              entering={entered.has(p.name)}
              fullMembers={pvs.find((provider) => provider.name === p.name)?.members}
              groups={groups}
              open={isOpen(pkey(p))}
              toggle={() => toggle(pkey(p))}
              sorted={sorted}
              onTest={() => testProvider(p)}
              onUpdate={async () => { if (await updateProvider(p.name)) toast(t("Updated {name}", { name: p.name })); }}
              hideTimeouts={hideTimeouts} searching={!!needle} tested={tested?.owner === pkey(p) ? tested : undefined}
              onTestOne={(name) => testNode(name, pkey(p))}
              testing={testing}
              testKey={pkey(p)}
              onMenu={onMenu}
            />
          ),
        }))} />
      )}
      </div>
      <Popover anchor={sortButton.current} open={sortMenu} onClose={() => setSortMenu(false)} align="end">
        <Menu close={() => setSortMenu(false)} items={[
          { label: t("Default order"), checked: !sorted, onClick: () => { if (sorted) morph(() => setSorted(false), "proxies-morph"); } },
          { label: t("Sort by latency"), checked: sorted, onClick: () => { if (!sorted) morph(() => setSorted(true), "proxies-morph"); } },
          "sep",
          { label: t("Hide timed-out nodes"), checked: hideTimeouts, onClick: () => {
            setHideTimeouts(!hideTimeouts);
            try { localStorage.setItem("proxies.hideTimeouts", String(!hideTimeouts)); } catch {}
          } },
        ]} />
      </Popover>
      <Popover anchor={null} point={menu?.point} open={!!menu} onClose={() => setMenu(null)}>
        {menu && <Menu close={() => setMenu(null)} items={[
          ...(menu.name !== "GLOBAL" ? [{ label: t("Serve on a SOCKS5 port…"), onClick: () => openPortDraft(menu.name) }] : []),
          { label: t("Copy name"), onClick: () => App.CopyText(nodeLabel(menu.name)).then(() => toast(t("Copied"))) },
        ]} />}
      </Popover>
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

function Availability({ members }: { members?: Member[] | null }) {
  const t = useT();
  if (!members?.some((m) => m.delay !== 0)) return null;
  return <span className="group-availability" title={t("Reachable nodes")}>{members.filter((m) => m.delay > 0).length}/{members.length}</span>;
}

function LatencyPreview({ members, now, testing, testingAll, onPick, flashKey }: {
  members: Member[]; now?: string; testing: Record<string, boolean>; testingAll?: boolean;
  onPick: (name: string) => void; flashKey?: string;
}) {
  const t = useT();
  const [box, width] = useWidth();
  const buckets = ["good", "ok", "bad", "fail", "none"].map((kind) => ({
    kind, count: members.filter((m) => delayClass(m.delay) === kind).length,
  }));
  return (
    <div className="latency-preview" ref={box}>
      {members.length * 14 <= width ? <div className="latency-dots">
        {members.map((m) => {
          const label = `${nodeLabel(m.name)} · ${fmtDelay(m.delay, t)}`;
          return <button key={m.name} tabIndex={-1} title={label} aria-label={label}
            className={"latency-dot " + delayClass(m.delay) + (m.name === now ? " selected" : "") + (testingAll || testing["#" + m.name] ? " testing" : "") + (flashKey === m.name ? " flash" : "")}
            onClick={(e) => { e.stopPropagation(); onPick(m.name); }} />;
        })}
      </div> : <div className="latency-bar" title={t("{a} reachable · {b} timed out · {c} untested", {
        a: members.filter((m) => m.delay > 0).length, b: buckets[3].count, c: buckets[4].count,
      })}>
        {buckets.map((b) => <span key={b.kind} className={b.kind} style={{ flexGrow: b.count }} />)}
      </div>}
    </div>
  );
}

function GroupChain({ start, groups, owner }: { start: string; groups: Group[]; owner: string }) {
  const names = chain(start, groups);
  return <div className="sub group-chain">{names.map((name, i) => {
    const last = i === names.length - 1;
    const target = !last && name !== owner && groups.some((g) => g.name === name && !g.hidden);
    return <span className={"chain-part" + (last ? " last" : "")} key={name}>
      {i > 0 && <span className="chain-separator">›</span>}
      {target ? <button className="chain-name" title={nodeLabel(name)} onClick={(e) => { e.stopPropagation(); useStore.setState({ proxiesTarget: name }); }}>{nodeLabel(name)}</button>
        : <span className="chain-name" title={nodeLabel(name)}>{nodeLabel(name) || "—"}</span>}
    </span>;
  })}</div>;
}

type GridOptions = { hideTimeouts?: boolean; searching?: boolean; tested?: { name: string; serial: number } };

// Only move the view when the entire tile isn't already inside it.
function scrollNode(node: HTMLElement | null) {
  const view = node?.closest(".view");
  if (!node || !view) return;
  const tile = node.getBoundingClientRect(), bounds = view.getBoundingClientRect();
  if (tile.top < bounds.top || tile.bottom > bounds.bottom || tile.left < bounds.left || tile.right > bounds.right) {
    node.scrollIntoView({ block: "nearest", inline: "nearest", behavior: calm() ? "instant" : "smooth" });
  }
}

export function GroupCard({ g, groups, fullMembers, cardIndex, entering, open, toggle, sorted, onSelect, onTest, onTestOne, testing, flash, compact, marked, onMenu, ...gridOptions }: GridOptions & {
  cardIndex?: number; entering?: boolean;
  g: Group; open: boolean; toggle: () => void; sorted: boolean;
  groups?: Group[]; fullMembers?: Member[] | null;
  onSelect: (name: string) => void; onTest: () => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; flash: string; compact?: boolean; marked?: boolean;
  onMenu?: (name: string, at: Point) => void;
}) {
  const t = useT();
  const card = useRef<HTMLDivElement>(null);
  const scrollOnOpen = useRef(false);
  useEffect(() => { if (!open) scrollOnOpen.current = false; }, [open]);
  const members = fullMembers === undefined ? g.members : fullMembers;
  const now = (members ?? []).find((m) => m.name === g.now);
  const flashKey = flash.startsWith(g.name + "/") ? flash.slice(g.name.length + 1) : "";
  return (
    <div ref={card} className={"group" + (open ? " open" : "") + (marked ? " marked" : "") + (entering ? " search-enter" : "")} data-group={g.name} style={{ ["--vt" as string]: cardIndex === undefined ? undefined : `pc-${cardIndex}` }}>
      <div className="group-head" onClick={() => { scrollOnOpen.current = !open && ["Selector", "URLTest", "Fallback"].includes(g.type); toggle(); }} onContextMenu={onMenu && ((e) => { e.preventDefault(); onMenu(g.name, { x: e.clientX, y: e.clientY }); })}>
        <Chevron className={"chev" + (open ? " open" : "")} />
        <div className="who">
          <div className="name">{g.name}<span className="gtype">{g.type}</span>{g.module && <span className="gtype">{t("Module")}</span>}</div>
          {groups ? <GroupChain start={g.now} groups={groups} owner={g.name} /> : <div className="sub">{nodeLabel(g.now) || "—"}</div>}
        </div>
        <Availability members={members} />
        {now && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay, t)}</span>}
        <button className={"icon" + (testing[g.name] ? " zap" : "")} title={t("Test")} onClick={(e) => { e.stopPropagation(); onTest(); }}><Bolt size={14} /></button>
      </div>
      {!!members?.length && <Fold open={!open}>
        <LatencyPreview members={members} now={g.now} testing={testing} testingAll={testing[g.name]} flashKey={flashKey}
          onPick={(name) => g.type === "Selector" ? name !== g.now && onSelect(name) : onTestOne(name)} />
      </Fold>}
      <Fold open={open} onSettled={() => {
        if (scrollOnOpen.current && card.current?.classList.contains("open")) scrollNode(card.current.querySelector(".node.on"));
        scrollOnOpen.current = false;
      }}>
        <NodeGrid {...gridOptions} cardIndex={cardIndex} fullMembers={members} members={g.members} groups={groups} sorted={sorted} now={g.now} selectable={g.type === "Selector"} flashKey={flashKey} onSelect={onSelect} onTestOne={onTestOne} testing={testing} compact={compact} onMenu={onMenu} />
      </Fold>
    </div>
  );
}

// A provider's nodes, under a head that says how many, when it was fetched
// and how much of the subscription is used, with a test and an update.
function ProviderCard({ p, groups, fullMembers, cardIndex, entering, open, toggle, sorted, onTest, onUpdate, onTestOne, testing, testKey, onMenu, ...gridOptions }: GridOptions & {
  cardIndex: number; entering?: boolean;
  p: Provider; open: boolean; toggle: () => void; sorted: boolean;
  groups?: Group[]; fullMembers?: Member[] | null;
  onTest: () => void; onUpdate: () => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; testKey: string; onMenu?: (name: string, at: Point) => void;
}) {
  const t = useT();
  const members = p.members ?? [];
  const previewMembers = (fullMembers === undefined ? p.members : fullMembers) ?? [];
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
    <div className={"group" + (open ? " open" : "") + (entering ? " search-enter" : "")} style={{ ["--vt" as string]: `pc-${cardIndex}` }}>
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
        <Availability members={previewMembers} />
        {p.vehicleType !== "Inline" && (
          <button className={"icon" + (updating ? " spin" : "")} title={t("Update")} disabled={updating} onClick={(e) => { e.stopPropagation(); onUpdate(); }}><Refresh size={14} /></button>
        )}
        <button className={"icon" + (testing[testKey] ? " zap" : "")} title={t("Test")} onClick={(e) => { e.stopPropagation(); onTest(); }}><Bolt size={14} /></button>
      </div>
      {previewMembers.length > 0 && <Fold open={!open}>
        <LatencyPreview members={previewMembers} testing={testing} testingAll={testing[testKey]} onPick={onTestOne} />
      </Fold>}
      <Fold open={open}>
        <NodeGrid {...gridOptions} cardIndex={cardIndex} fullMembers={previewMembers} members={members} groups={groups} sorted={sorted} selectable={false} onTestOne={onTestOne} testing={testing} onMenu={onMenu} />
      </Fold>
    </div>
  );
}

function NodeGrid({ members: ms, fullMembers, cardIndex, groups, sorted, now, selectable, flashKey, onSelect, onTestOne, testing, compact, onMenu, hideTimeouts = false, searching, tested }: GridOptions & {
  cardIndex?: number; fullMembers?: Member[] | null;
  groups?: Group[];
  members?: Member[] | null; sorted: boolean; now?: string; selectable: boolean; flashKey?: string;
  onSelect?: (name: string) => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; compact?: boolean; onMenu?: (name: string, at: Point) => void;
}) {
  const t = useT();
  const grid = useRef<HTMLDivElement>(null);
  const [filter, setFilter] = useState("");
  const needle = searching ? "" : filter.toLowerCase();
  const hidden = (m: Member) => hideTimeouts && !m.group && m.delay < 0 && m.name !== now;
  const matching = (ms ?? []).filter((m) => nodeLabel(m.name).toLowerCase().includes(needle));
  const hiddenCount = matching.filter(hidden).length;
  const members = useMemo(() => {
    const list = (ms ?? []).filter((m) => nodeLabel(m.name).toLowerCase().includes(needle) && !hidden(m));
    if (!sorted) return list;
    const key = (d: number) => (d > 0 ? d : d < 0 ? 1e9 : 1e8);
    return [...list].sort((a, b) => key(a.delay) - key(b.delay));
  }, [ms, sorted, needle, hideTimeouts, now]);
  const previousHide = useRef(hideTimeouts);
  useFlip(grid, JSON.stringify(members.map((m) => m.name)), { axis: "xy", enter: previousHide.current !== hideTimeouts, selector: ":scope > .node" });
  useEffect(() => { previousHide.current = hideTimeouts; }, [hideTimeouts]);
  const lastTest = useRef(tested);
  useEffect(() => {
    if (lastTest.current === tested) return;
    lastTest.current = tested;
    if (!tested || !sorted) return;
    const tile = grid.current?.querySelector<HTMLElement>(`.node[data-key="${CSS.escape(tested.name)}"]`);
    if (!tile) return;
    // Restart the existing flash even when the same node is tested twice.
    tile.classList.remove("tested");
    void tile.offsetWidth;
    tile.classList.add("tested");
    const timer = setTimeout(() => tile.classList.remove("tested"), 900);
    // FLIP's transform temporarily draws the old position; inspect the final one.
    const scroll = setTimeout(() => scrollNode(tile), calm() ? 0 : 420);
    return () => { clearTimeout(timer); clearTimeout(scroll); tile.classList.remove("tested"); };
  }, [tested, sorted]);
  return (
    <div ref={grid} className={"nodes" + (compact ? " compact" : "")}>
      {!searching && (fullMembers ?? ms ?? []).length >= 20 && <label className="search node-filter"><Search size={12} /><input
        placeholder={t("Filter {n} nodes", { n: (fullMembers ?? ms ?? []).length })} value={filter}
        onChange={(e) => setFilter(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); setFilter(""); } }} />
        {filter && <button className="icon clear" title={t("Clear")} onClick={() => setFilter("")}><Close size={11} /></button>}
      </label>}
      {!members.length && hiddenCount === 0 && <div className="node-note">{t("No matching nodes")}</div>}
      {members.map((m, i) => {
        const path = m.group && groups ? chain(m.name, groups) : [m.name];
        const type = m.type + (m.udp ? " · UDP" : "");
        return (
        <button
          key={m.name} data-key={m.name}
          className={"node stagger" + (m.name === now ? " on" : "") + (m.delay < 0 && m.name !== now ? " timed-out" : "") + (selectable ? "" : " fixed") + (flashKey === m.name ? " flash" : "")}
          style={{ ["--i" as string]: Math.min(i, 24), ["--vt" as string]: cardIndex === undefined ? undefined : `pn-${cardIndex}-${(fullMembers ?? ms ?? []).findIndex((member) => member.name === m.name)}` }}
          // with nothing to select (a provider's nodes), a click tests
          onClick={(e) => (e.altKey || !onSelect ? onTestOne(m.name) : selectable && m.name !== now && onSelect(m.name))}
          onContextMenu={onMenu && ((e) => { e.preventDefault(); onMenu(m.name, { x: e.clientX, y: e.clientY }); })}
          title={nodeLabel(m.name) + "\n" + type + "\n" + (onSelect ? "⌥-click: " + t("test this node only") : t("Click to test this node"))}
        >
          <span className="nname">{nodeLabel(m.name)}</span>
          <span className="nmeta">
            <span className="ntype">{path.length > 1 ? "→ " + nodeLabel(path[path.length - 1]) : <>{shortType(m.type)}{m.udp && <span className="node-udp"> · UDP</span>}</>}</span>
            <span
              className={"delay " + (testing["#" + m.name] ? "testing" : delayClass(m.delay))}
              onClick={(e) => { e.stopPropagation(); onTestOne(m.name); }}
            >
              {testing["#" + m.name] ? "···" : fmtDelay(m.delay, t)}
            </span>
          </span>
        </button>
      ); })}
      {hiddenCount > 0 && <div className="node-note">{t("{n} timed-out nodes hidden", { n: hiddenCount })}</div>}
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
