import { useEffect, useRef, useState } from "react";
import type { Group } from "../api";
import { chainOf, type Conn } from "../connections";
import { useT } from "../i18n";
import { delayClass, fmtDelay, nodeLabel } from "../format";
import { Bolt, Chevron, Search } from "./Icons";
import { Popover, type Point } from "./Popover";

// a policy, then each group's selection down to a node
export function routeOf(policy: string, nowOf: Map<string, string>) {
  const path = [policy];
  for (let now = nowOf.get(policy); now && !path.includes(now); now = nowOf.get(now)) path.push(now);
  return path;
}

// The policy a connection's rule sent it to: the last of its chains, which
// run from the node back to the rule's policy.
export const policyOf = (c: Conn) => c.chains?.length ? c.chains[c.chains.length - 1] : "";

// The groups on the way a connection's rule sends new connections, each one
// a group a node can be picked in.
export const routeGroups = (c: Conn, groups: Group[] | null) => {
  if (!groups) return [];
  const byName = new Map(groups.map((g) => [g.name, g]));
  return routeOf(policyOf(c), new Map(groups.map((g) => [g.name, g.now]))).filter((n) => byName.has(n));
};

const tone = (exit: string) => exit === "DIRECT" ? "direct" : /^REJECT/.test(exit) ? "reject" : "proxy";

// RouteChain shows, in a connection's details, the rule it matched and the
// way that rule's policy goes now: each group a button that opens its nodes,
// the node it ends at as a tag. When the way has changed since the
// connection opened, the way it went is shown under it.
export function RouteChain({ c, groups, open, onOpen }: {
  c: Conn;
  groups: Group[] | null;
  open: string;
  onOpen: (group: string, at: HTMLElement) => void;
}) {
  const t = useT();
  const byName = new Map((groups ?? []).map((g) => [g.name, g]));
  const was = [...(c.chains ?? [])].reverse();
  const path = groups && was.length ? routeOf(was[0], new Map(groups.map((g) => [g.name, g.now]))) : was;
  if (!path.length) return null;
  const exit = path[path.length - 1];
  const delay = path.length > 1 ? byName.get(path[path.length - 2])?.members?.find((m) => m.name === exit)?.delay : undefined;
  const moved = path.join("\0") !== was.join("\0");
  return (
    <div className="route-chain">
      {c.rule && <div className="route-rule">
        <span className="rtype">{c.rule}</span>
        {c.rulePayload && <span className="mono" title={c.rulePayload}>{c.rulePayload}</span>}
      </div>}
      {/* each hop keeps the arrow into it, so a wrapped line starts with one */}
      <div className="route-hops">
        {path.slice(0, -1).map((name, i) => {
          const g = byName.get(name);
          return (
            <span className="hop" key={name}>
              {i > 0 && <Chevron size={10} className="route-sep" />}
              <button className={"node-pick" + (open === name ? " on" : "") + (g ? "" : " fixed")} disabled={!g} data-group={name}
                title={g?.type === "Selector" ? t("Choose a node") : g ? t("Picked by the latency test") : undefined}
                onClick={(e) => g && onOpen(name, e.currentTarget)}>
                <span className="nname">{nodeLabel(name)}</span>
                {g && <Chevron size={10} className="chev" />}
              </button>
            </span>
          );
        })}
        <span className="hop">
          {path.length > 1 && <Chevron size={10} className="route-sep" />}
          <span className={"exit " + tone(exit)} title={nodeLabel(exit)}>{nodeLabel(exit)}</span>
          {delay ? <span className={"delay " + delayClass(delay)}>{fmtDelay(delay, t)}</span> : null}
        </span>
      </div>
      {moved && <div className="route-was">{t("Opened through {chain}", { chain: chainOf(c) })}</div>}
    </div>
  );
}

// GroupPicker lists a group's nodes to pick one, opened from a group in a
// connection's route or from the connection's context menu. Picking applies
// to every rule that uses the group, which it says, with the way to send
// only this connection's host elsewhere.
export function GroupPicker({ group, anchor, point, affected, host, testing, onClose, onSelect, onTest, onOnly }: {
  group: Group | null;
  anchor?: HTMLElement | null;
  point?: Point | null;
  affected: number;
  host: string;
  testing: (name: string) => boolean;
  onClose: () => void;
  onSelect: (group: string, name: string) => void;
  onTest: (group: Group) => void;
  onOnly: () => void;
}) {
  const t = useT();
  const [q, setQ] = useState("");
  const list = useRef<HTMLDivElement>(null);
  // keeps the content while it closes
  const last = useRef(group);
  if (group) last.current = group;
  const g = group ?? last.current;
  useEffect(() => {
    if (!group) return setQ("");
    requestAnimationFrame(() => list.current?.querySelector<HTMLElement>("button.on")?.scrollIntoView({ block: "nearest" }));
  }, [group?.name]);
  if (!g) return null;
  const members = g.members ?? [];
  const pick = g.type === "Selector";
  const s = q.trim().toLowerCase();
  const shown = s ? members.filter((m) => nodeLabel(m.name).toLowerCase().includes(s)) : members;
  const choose = (name: string) => { onClose(); if (name !== g.now) onSelect(g.name, name); };
  return (
    <Popover anchor={anchor ?? null} point={point} open={!!group} onClose={onClose} width={300}>
      <div className="menu node-menu group-menu">
        <div className="gm-head">
          <b title={g.name}>{nodeLabel(g.name)}</b>
          <span className="muted num">{pick ? `${members.length} · ${g.type}` : t("Picked by the latency test")}</span>
          <button className={"icon" + (testing(g.name) ? " zap" : "")} title={t("Test")} aria-label={t("Test")} onClick={() => onTest(g)}><Bolt size={13} /></button>
        </div>
        {members.length > 12 && <label className="search"><Search size={13} /><input autoFocus placeholder={t("Search nodes")} value={q} onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && pick && shown[0]) { e.preventDefault(); choose(shown[0].name); } }} /></label>}
        <div className="gm-list" ref={list}>
          {shown.map((m) => (
            <button key={m.name} className={m.name === g.now ? "on" : ""} disabled={!pick} onClick={() => choose(m.name)}>
              <span className="nname" title={nodeLabel(m.name)}>{nodeLabel(m.name)}</span>
              <span className={"delay " + (testing("#" + m.name) ? "testing" : delayClass(m.delay))}>{testing("#" + m.name) ? "···" : fmtDelay(m.delay, t)}</span>
            </button>
          ))}
          {shown.length === 0 && <div className="mnote">{t("No matches")}</div>}
        </div>
        <hr />
        <div className="mnote">{affected
          ? t("Applies to every rule that uses {group}; {n} connections through it reconnect.", { group: nodeLabel(g.name), n: affected })
          : t("Applies to every rule that uses {group}.", { group: nodeLabel(g.name) })}</div>
        {host && <button onClick={() => { onClose(); onOnly(); }}><span className="mlabel">{t("Send only {host} elsewhere…", { host })}</span></button>}
      </div>
    </Popover>
  );
}
