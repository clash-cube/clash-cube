import { create } from "zustand";
import { Events } from "@wailsio/runtime";
import { App, Profiles, Settings, type State, type Profile, type SettingsT, type Log } from "./api";

export type View = "overview" | "proxies" | "profiles" | "connections" | "rules" | "logs" | "settings";

// a point of the traffic chart
export type Sample = { up: number; down: number };
const HISTORY = 60;

export type LogLine = Log & { id: number; at: number };
const MAX_LOGS = 1000;

type Store = {
  state: State | null;
  settings: SettingsT | null;
  profiles: Profile[];
  traffic: { up: number; down: number; upTotal: number; downTotal: number };
  history: Sample[];
  memory: number;
  logs: LogLine[];
  view: View;
  setView: (v: View) => void;
  refreshSettings: () => Promise<void>;
  clearLogs: () => void;
};

let logSeq = 0;

export const useStore = create<Store>((set) => ({
  state: null,
  settings: null,
  profiles: [],
  traffic: { up: 0, down: 0, upTotal: 0, downTotal: 0 },
  history: Array.from({ length: HISTORY }, () => ({ up: 0, down: 0 })),
  memory: 0,
  logs: [],
  view: (new URLSearchParams(location.search).get("view") as View) || "overview",
  setView: (view) => set({ view }),
  refreshSettings: async () => set({ settings: await Settings.Get() }),
  clearLogs: () => set({ logs: [] }),
}));

// boot reads the app once and follows its events.
export async function boot() {
  const [state, settings, profiles] = await Promise.all([App.State(), Settings.Get(), Profiles.List()]);
  useStore.setState({ state, settings, profiles: profiles ?? [] });
  applyTheme(settings.theme);

  Events.On("state", (e) => {
    const prev = useStore.getState().state;
    useStore.setState({ state: e.data });
    // a stopped core has no traffic
    if (e.data.core !== "running" && prev?.core === "running") {
      useStore.setState({ traffic: { up: 0, down: 0, upTotal: 0, downTotal: 0 }, memory: 0 });
    }
  });
  Events.On("traffic", (e) => {
    const t = e.data;
    useStore.setState((s) => ({ traffic: t, history: [...s.history.slice(1), { up: t.up, down: t.down }] }));
  });
  Events.On("memory", (e) => useStore.setState({ memory: e.data.inuse }));
  Events.On("profiles", (e) => useStore.setState({ profiles: e.data ?? [] }));
  Events.On("navigate", (e) => useStore.setState({ view: e.data as View }));
  Events.On("log", (e) => {
    const line: LogLine = { ...e.data, id: ++logSeq, at: Date.now() };
    useStore.setState((s) => {
      const logs = s.logs.length >= MAX_LOGS ? s.logs.slice(-MAX_LOGS + 1) : s.logs.slice();
      logs.push(line);
      return { logs };
    });
  });
  // settings changed in the other window
  window.addEventListener("focus", () => useStore.getState().refreshSettings());
}

export function applyTheme(theme: string) {
  const root = document.documentElement;
  if (theme === "light" || theme === "dark") root.dataset.theme = theme;
  else delete root.dataset.theme;
  try { localStorage.setItem("theme", theme); } catch {}
}
