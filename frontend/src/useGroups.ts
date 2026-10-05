import { useCallback, useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { Proxy, type Group, type Member, type Provider } from "./api";
import type { LatencyEvent } from "../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import { useStore } from "./store";
import { toast, toastError } from "./components/Toast";
import { useT } from "./i18n";
import { usePoll } from "./usePoll";

// useGroups loads the proxy groups while the core runs, with the delay tests
// and selection that change them; with providers, their providers too.
export function useGroups({ providers: withProviders = false, testOnOpen = false } = {}) {
  const t = useT();
  const core = useStore((s) => s.state?.core);
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [updating, setUpdating] = useState<Record<string, boolean>>({});
  const [runs, setRuns] = useState<Record<string, LatencyEvent>>({});
  const [flash, setFlash] = useState<string>("");
  const alive = useRef(true);
  const loadSeq = useRef(0);

  const load = useCallback(async () => {
    const seq = ++loadSeq.current;
    const state = useStore.getState().state;
    if (state?.core !== "running") { setGroups(null); setProviders(null); return; }
    if (state.busy) return;
    try {
      const [g, p] = await Promise.all([Proxy.Groups(), withProviders ? Proxy.Providers() : Promise.resolve(null)]);
      if (!alive.current || seq !== loadSeq.current) return;
      setGroups((g ?? []).map((x) => ({ ...x, members: x.members ?? [] })));
      if (withProviders) setProviders((p ?? []).map((x) => ({ ...x, members: x.members ?? [] })));
    } catch { /* the core went away; its state event says so */ }
  }, [withProviders]);

  useEffect(() => { alive.current = true; return () => { alive.current = false; ++loadSeq.current; }; }, []);
  // Invalidate snapshots even while a reload is busy or the window is hidden.
  useEffect(() => {
    ++loadSeq.current;
    if (core !== "running") { setGroups(null); setProviders(null); }
  }, [core, profile, busy]);
  // mihomo's background health checks have no proxy-latency event. Read their
  // results while visible, including when the persistent tray panel reopens.
  usePoll(() => { if (!busy) void load(); }, 5000, [core, profile, busy, load]);
  useEffect(() => { setRuns({}); }, [core, profile]);

  const select = async (group: string, name: string) => {
    // shown at once, put right if the core says otherwise
    setGroups((gs) => gs?.map((g) => (g.name === group ? { ...g, now: name } : g)) ?? gs);
    setFlash(group + "/" + name);
    setTimeout(() => setFlash(""), 900);
    try { await Proxy.Select(group, name); } catch (e) { toastError(e); }
    load();
  };

  // a node's delay is the node's, wherever it is listed
  const setDelays = useCallback((delays: Partial<Record<string, number>>) => {
    const apply = <T extends { members?: Member[] | null }>(xs: T[] | null) =>
      xs?.map((x) => ({ ...x, members: (x.members ?? []).map((m) => {
        const delay = delays[m.name];
        return typeof delay === "number" ? { ...m, delay } : m;
      }) })) ?? xs;
    setGroups(apply);
    setProviders(apply);
  }, []);

  useEffect(() => Events.On("proxy-latency", ({ data: e }) => {
    // Invalidate older snapshots before applying a fresh streamed result.
    ++loadSeq.current;
    setDelays(e.delays ?? {});
    setRuns((prev) => {
      const next = { ...prev };
      if (e.running) next[e.key] = e; else delete next[e.key];
      return next;
    });
    if (!e.running) load();
  }), [setDelays, load]);

  const testing = { ...updating };
  for (const e of Object.values(runs)) {
    const key = e.key.startsWith("group/") ? e.key.slice(6) : e.key;
    testing[key] = true;
    for (const name of e.pending ?? []) testing["#" + name] = true;
  }
  const active = Object.values(runs);
  const progress = active.length ? t("Testing {done}/{total}", {
    done: active.reduce((n, e) => n + e.completed, 0),
    total: active.reduce((n, e) => n + e.total, 0),
  }) : "";

  const requested = useRef(new Set<string>());
  const test = async (kind: string, name = "", automatic = false) => {
    const key = kind + "/" + name;
    if (requested.current.has(key) || runs[key]) return;
    requested.current.add(key);
    try {
      const result = await Proxy.TestLatency(kind, name);
      if (!automatic && kind !== "node") toast(t("Tested {total}: {success} succeeded, {failed} failed", {
        total: result.total, success: result.total - result.failed, failed: result.failed,
      }), result.failed ? "err" : "ok");
    } catch (e) { if (!automatic) toastError(e); }
    finally { requested.current.delete(key); await load(); }
  };
  const testGroup = (g: Group) => test("group", g.name);
  const testProvider = (p: Provider) => test("provider", p.name);
  const testOne = (name: string) => test("node", name);
  const testAll = () => test("all");

  // A persistent panel may first mount hidden. Start its initial probe only
  // after it is shown and discovery has loaded the running profile.
  const initiallyTested = useRef<string | null>(null);
  useEffect(() => {
    if (core !== "running") { initiallyTested.current = null; return; }
    if (!testOnOpen || busy || document.hidden || !groups?.length || initiallyTested.current === (profile ?? "")) return;
    initiallyTested.current = profile ?? "";
    void test("all", "", true);
  }, [core, profile, busy, groups, testOnOpen]);

  // updateProvider fetches a provider again; its nodes come back untested
  const updateProvider = async (name: string) => {
    const key = "provider/" + name;
    setUpdating((t) => ({ ...t, [key + "/update"]: true }));
    try { await Proxy.UpdateProvider(name); await load(); return true; } catch (e) { toastError(e); return false; } finally {
      setUpdating((t) => ({ ...t, [key + "/update"]: false }));
    }
  };

  return { groups, providers, load, select, testGroup, testProvider, testOne, testAll, updateProvider, testing, progress, flash };
}
