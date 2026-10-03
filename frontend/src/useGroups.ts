import { useCallback, useEffect, useRef, useState } from "react";
import { Proxy, type Group, type Member, type Provider } from "./api";
import { useStore } from "./store";
import { toastError } from "./components/Toast";

// nodeTests is how many of a group's nodes are tested at once.
const nodeTests = 16;

// Testable is what a delay test runs over: a group, or a provider under
// its own key so the two can share a name.
export type Testable = { key: string; testUrl?: string; members?: Member[] | null };

// useGroups loads the proxy groups while the core runs, with the delay tests
// and selection that change them; with providers, their providers too.
export function useGroups({ providers: withProviders = false } = {}) {
  const core = useStore((s) => s.state?.core);
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [testing, setTesting] = useState<Record<string, boolean>>({});
  const [flash, setFlash] = useState<string>("");
  const alive = useRef(true);

  const load = useCallback(async () => {
    if (useStore.getState().state?.core !== "running") { setGroups(null); setProviders(null); return; }
    try {
      const [g, p] = await Promise.all([Proxy.Groups(), withProviders ? Proxy.Providers() : Promise.resolve(null)]);
      if (!alive.current) return;
      setGroups((g ?? []).map((x) => ({ ...x, members: x.members ?? [] })));
      if (withProviders) setProviders((p ?? []).map((x) => ({ ...x, members: x.members ?? [] })));
    } catch { /* the core went away; its state event says so */ }
  }, [withProviders]);

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

  // a node's delay is the node's, wherever it is listed
  const setDelays = (delays: Record<string, number>) => {
    const apply = <T extends { members?: Member[] | null }>(xs: T[] | null) =>
      xs?.map((x) => ({ ...x, members: (x.members ?? []).map((m) => (m.name in delays ? { ...m, delay: delays[m.name] } : m)) })) ?? xs;
    setGroups(apply);
    setProviders(apply);
  };

  // testGroup tests a group's nodes one by one, a few at a time, so each
  // delay shows as it comes in rather than when the slowest answers
  // (mihomo's group test answers all at once)
  const busyGroups = useRef(new Set<string>());
  const testGroup = async (g: Group | Testable) => {
    const key = "key" in g ? g.key : g.name;
    if (busyGroups.current.has(key)) return;
    busyGroups.current.add(key);
    const names = (g.members ?? []).map((m) => m.name);
    setTesting((t) => ({ ...t, [key]: true, ...Object.fromEntries(names.map((n) => ["#" + n, true])) }));
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
    busyGroups.current.delete(key);
    setTesting((t) => ({ ...t, [key]: false }));
  };

  const testOne = async (name: string) => {
    setTesting((t) => ({ ...t, ["#" + name]: true }));
    try { setDelays({ [name]: await Proxy.Delay(name, "") }); } catch (e) { toastError(e); }
    setTesting((t) => ({ ...t, ["#" + name]: false }));
  };

  // updateProvider fetches a provider again; its nodes come back untested
  const updateProvider = async (name: string) => {
    const key = "provider/" + name;
    setTesting((t) => ({ ...t, [key + "/update"]: true }));
    try { await Proxy.UpdateProvider(name); await load(); return true; } catch (e) { toastError(e); return false; } finally {
      setTesting((t) => ({ ...t, [key + "/update"]: false }));
    }
  };

  return { groups, providers, load, select, testGroup, testOne, updateProvider, testing, flash };
}
