import { useCallback, useEffect, useRef, useState } from "react";
import { Proxy, type Group } from "./api";
import { useStore } from "./store";
import { toastError } from "./components/Toast";

// nodeTests is how many of a group's nodes are tested at once.
const nodeTests = 16;

// useGroups loads the proxy groups while the core runs, with the delay tests
// and selection that change them.
export function useGroups() {
  const core = useStore((s) => s.state?.core);
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [testing, setTesting] = useState<Record<string, boolean>>({});
  const [flash, setFlash] = useState<string>("");
  const alive = useRef(true);

  const load = useCallback(async () => {
    if (useStore.getState().state?.core !== "running") { setGroups(null); return; }
    try {
      const g = await Proxy.Groups();
      if (alive.current) setGroups((g ?? []).map((x) => ({ ...x, members: x.members ?? [] })));
    } catch { /* the core went away; its state event says so */ }
  }, []);

  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => { if (!busy) load(); }, [core, profile, busy]);

  const select = async (group: string, name: string) => {
    // shown at once, put right if the core says otherwise
    setGroups((gs) => gs?.map((g) => (g.name === group ? { ...g, now: name } : g)) ?? gs);
    setFlash(group + "/" + name);
    setTimeout(() => setFlash(""), 900);
    try { await Proxy.Select(group, name); } catch (e) { toastError(e); }
    load();
  };

  const setDelays = (delays: Record<string, number>) =>
    setGroups((gs) => gs?.map((g) => ({ ...g, members: (g.members ?? []).map((m) => (m.name in delays ? { ...m, delay: delays[m.name] } : m)) })) ?? gs);

  // testGroup tests a group's nodes one by one, a few at a time, so each
  // delay shows as it comes in rather than when the slowest answers
  // (mihomo's group test answers all at once)
  const busyGroups = useRef(new Set<string>());
  const testGroup = async (g: Group) => {
    if (busyGroups.current.has(g.name)) return;
    busyGroups.current.add(g.name);
    const names = (g.members ?? []).map((m) => m.name);
    setTesting((t) => ({ ...t, [g.name]: true, ...Object.fromEntries(names.map((n) => ["#" + n, true])) }));
    let next = 0, failed: unknown;
    const worker = async () => {
      while (next < names.length) {
        const name = names[next++];
        try { setDelays({ [name]: await Proxy.Delay(name, g.testUrl ?? "") }); } catch (e) { failed ??= e; }
        setTesting((t) => ({ ...t, ["#" + name]: false }));
      }
    };
    await Promise.all(Array.from({ length: Math.min(nodeTests, names.length) }, worker));
    if (failed) toastError(failed);
    busyGroups.current.delete(g.name);
    setTesting((t) => ({ ...t, [g.name]: false }));
  };

  const testOne = async (name: string) => {
    setTesting((t) => ({ ...t, ["#" + name]: true }));
    try { setDelays({ [name]: await Proxy.Delay(name, "") }); } catch (e) { toastError(e); }
    setTesting((t) => ({ ...t, ["#" + name]: false }));
  };

  return { groups, load, select, testGroup, testOne, testing, flash };
}
