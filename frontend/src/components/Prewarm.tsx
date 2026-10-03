import { useEffect, useState } from "react";
import { Proxy, type Group } from "../api";
import { useStore } from "../store";
import { GroupCard } from "../views/Proxies";

const noop = () => {};
// how many groups are drawn a tick, and how far apart
const PER_TICK = 4;
const TICK = 30;

// Prewarm draws the proxy groups out of sight once the core runs, so the
// Proxies page shows at once. A web view's first drawing of a colour emoji
// is slow (some 50 ms each, in names: flags, icons), and a profile with
// dozens held that page's first showing for seconds. It draws the real
// cards, a few a tick so the window keeps answering, and keeps them until
// the last is in, as dropping them early let some of the gain go.
export function Prewarm() {
  const core = useStore((s) => s.state?.core);
  const busy = useStore((s) => s.state?.busy);
  const profile = useStore((s) => s.state?.profile);
  const [groups, setGroups] = useState<Group[]>([]);
  const [upTo, setUpTo] = useState(0);

  useEffect(() => {
    if (core !== "running" || busy) return;
    let gone = false, timer = 0;
    Proxy.Groups().then((gs) => {
      if (gone || !gs?.length) return;
      setGroups(gs);
      const step = (n: number) => {
        setUpTo(n);
        timer = window.setTimeout(() => (n < gs.length ? step(n + PER_TICK) : setGroups([])), TICK);
      };
      step(PER_TICK);
    }).catch(noop);
    return () => { gone = true; clearTimeout(timer); setGroups([]); };
  }, [core, busy, profile]);

  if (!groups.length) return null;
  return (
    <div aria-hidden style={{ position: "fixed", left: 0, top: 0, width: 600, zIndex: -1, opacity: 0.01, pointerEvents: "none" }}>
      {groups.slice(0, upTo).map((g) => (
        <GroupCard key={g.name} g={g} open toggle={noop} sorted={false} onSelect={noop} onTest={noop} onTestOne={noop} testing={{}} flash="" />
      ))}
    </div>
  );
}
