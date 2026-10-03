import { useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { useGroups } from "../useGroups";
import { Fold } from "../components/Fold";
import { Bolt, Chevron, Sort } from "../components/Icons";
import { delayClass } from "../format";
import type { Group } from "../api";
import { startCore } from "../actions";

export function Proxies() {
  const t = useT();
  const core = useStore((s) => s.state?.core);
  const { groups, select, testGroup, testOne, testing, flash } = useGroups();
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [sorted, setSorted] = useState(false);

  if (core !== "running") {
    return (
      <div className="view">
        <div className="empty-state"><b>{t("Core is not running")}</b>{t("Start the core to see proxies.")}<div><button className="btn primary" onClick={startCore}>{t("Start core")}</button></div></div>
      </div>
    );
  }
  if (!groups) return <div className="view"><GroupsSkeleton /></div>;
  const shown = groups.filter((g) => !g.hidden);
  if (shown.length === 0) return <div className="view"><div className="empty-state"><b>{t("No proxy groups")}</b>{t("This profile has no proxy groups.")}</div></div>;

  return (
    <div className="view">
      <div className="view-head">
        <h2>{t("Proxies")}</h2>
        <span className="sub">{shown.length}</span>
        <div className="view-tools">
          <button className={"btn small" + (sorted ? " on" : "")} onClick={() => setSorted(!sorted)}><Sort size={13} />{sorted ? t("Sort by latency") : t("Default order")}</button>
          <button className="btn small" onClick={() => shown.forEach((g) => testGroup(g))}><Bolt size={13} />{t("Test all")}</button>
        </div>
      </div>
      {shown.map((g) => (
        <GroupCard
          key={g.name}
          g={g}
          open={open[g.name] ?? g.name !== "GLOBAL"}
          toggle={() => setOpen((o) => ({ ...o, [g.name]: !(o[g.name] ?? g.name !== "GLOBAL") }))}
          sorted={sorted}
          onSelect={(n) => select(g.name, n)}
          onTest={() => testGroup(g)}
          onTestOne={testOne}
          testing={testing}
          flash={flash}
        />
      ))}
    </div>
  );
}

export function GroupCard({ g, open, toggle, sorted, onSelect, onTest, onTestOne, testing, flash, compact }: {
  g: Group; open: boolean; toggle: () => void; sorted: boolean;
  onSelect: (name: string) => void; onTest: () => void; onTestOne: (name: string) => void;
  testing: Record<string, boolean>; flash: string; compact?: boolean;
}) {
  const t = useT();
  const selectable = g.type === "Selector";
  const members = useMemo(() => {
    const ms = g.members ?? [];
    if (!sorted) return ms;
    const key = (d: number) => (d > 0 ? d : d < 0 ? 1e9 : 1e8);
    return [...ms].sort((a, b) => key(a.delay) - key(b.delay));
  }, [g.members, sorted]);
  const now = (g.members ?? []).find((m) => m.name === g.now);
  return (
    <div className={"group" + (open ? " open" : "")}>
      <div className="group-head" onClick={toggle}>
        <Chevron className={"chev" + (open ? " open" : "")} />
        <div className="who">
          <div className="name">{g.name}<span className="gtype">{g.type}</span></div>
          <div className="sub">{g.now || "—"}</div>
        </div>
        {now && <span className={"delay " + delayClass(now.delay)}>{fmtDelay(now.delay)}</span>}
        <button className={"icon" + (testing[g.name] ? " spin" : "")} title={t("Test")} onClick={(e) => { e.stopPropagation(); onTest(); }}><Bolt size={14} /></button>
      </div>
      <Fold open={open}>
        <div className={"nodes" + (compact ? " compact" : "")}>
          {(members ?? []).map((m, i) => (
            <button
              key={m.name}
              className={"node stagger" + (m.name === g.now ? " on" : "") + (selectable ? "" : " fixed") + (flash === g.name + "/" + m.name ? " flash" : "")}
              style={{ ["--i" as string]: Math.min(i, 24) }}
              onClick={(e) => (e.altKey ? onTestOne(m.name) : selectable && m.name !== g.now && onSelect(m.name))}
              title={m.name + "\n⌥-click: " + t("test this node only")}
            >
              <span className="nname">{m.name}</span>
              <span className="nmeta">
                <span className="ntype">{m.type}{m.udp ? " · UDP" : ""}</span>
                <span
                  className={"delay " + (testing["#" + m.name] || testing[g.name] ? "testing" : delayClass(m.delay))}
                  onClick={(e) => { e.stopPropagation(); onTestOne(m.name); }}
                >
                  {testing["#" + m.name] || testing[g.name] ? "···" : fmtDelay(m.delay)}
                </span>
              </span>
            </button>
          ))}
        </div>
      </Fold>
    </div>
  );
}

export const fmtDelay = (d: number) => (d > 0 ? d + " ms" : d < 0 ? "timeout" : "—");

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
