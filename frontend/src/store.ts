import { create } from "zustand";
import { Events } from "@wailsio/runtime";
import { App, Profiles, Settings, type State, type Profile, type ImportRequest, type SettingsT, type Log, type Event } from "./api";
import { translate as t } from "./i18n";
import { toast, toastError } from "./components/Toast";

export type View = "overview" | "proxies" | "profiles" | "connections" | "rules" | "logs" | "events" | "settings";

// a point of the traffic chart
export type Sample = { up: number; down: number; at: number };
const HISTORY = 60;

export type LogLine = Log & { id: number; at: number };
const MAX_LOGS = 1000;

type Store = {
  state: State | null;
  settings: SettingsT | null;
  profiles: Profile[];
  imports: ImportRequest[];
  traffic: { up: number; down: number; upTotal: number; downTotal: number };
  history: Sample[];
  memory: number;
  logs: LogLine[];
  events: Event[];
  // bumped when the network was reset, for what measures it to again
  networkReset: number;
  view: View;
  setView: (v: View) => void;
  refreshSettings: () => Promise<void>;
  clearLogs: () => void;
  clearEvents: () => void;
};

let logSeq = 0;

export const useStore = create<Store>((set) => ({
  state: null,
  settings: null,
  profiles: [],
  imports: [],
  traffic: { up: 0, down: 0, upTotal: 0, downTotal: 0 },
  history: Array.from({ length: HISTORY }, (_, i) => ({ up: 0, down: 0, at: Date.now() - (HISTORY - i) * 1000 })),
  memory: 0,
  logs: [],
  events: [],
  networkReset: 0,
  view: (new URLSearchParams(location.search).get("view") as View) || "overview",
  setView: (view) => set({ view }),
  refreshSettings: async () => set({ settings: await Settings.Get() }),
  clearLogs: () => set({ logs: [] }),
  clearEvents: () => { App.ClearEvents(); set({ events: [] }); },
}));

const MAX_EVENTS = 200;
// what the backend says the network reset with (backend.ResetText)
const RESET_TEXT = "Closed connections and flushed DNS";

// boot reads the app once and follows its events.
export async function boot() {
  const [state, settings, profiles, events] = await Promise.all([App.State(), Settings.Get(), Profiles.List(), App.Events()]);
  useStore.setState({ state, settings, profiles: profiles ?? [], events: events ?? [] });
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
    useStore.setState((s) => ({ traffic: t, history: [...s.history.slice(1), { up: t.up, down: t.down, at: Date.now() }] }));
  });
  Events.On("memory", (e) => useStore.setState({ memory: e.data.inuse }));
  Events.On("profiles", (e) => useStore.setState({ profiles: e.data ?? [] }));
  Events.On("event", (e) => {
    const ev: Event = e.data;
    useStore.setState((s) => ({
      events: [...s.events.slice(-MAX_EVENTS + 1), ev],
      networkReset: ev.text === RESET_TEXT ? s.networkReset + 1 : s.networkReset,
    }));
  });
  Events.On("navigate", (e) => useStore.setState({ view: e.data as View }));
  if (new URLSearchParams(location.search).get("mode") !== "panel") {
    // Subscribe before draining so a cold-start link cannot fall between the
    // initial read and the event listener. Serialize drains to preserve order.
    let draining = Promise.resolve();
    const receiveImports = () => {
      draining = draining.then(async () => {
        const requests = await Profiles.TakeImportRequests();
        for (const request of requests ?? []) {
          if (request.error) { toast(t(request.error), "err", 4000); continue; }
          useStore.setState((s) => ({
            view: "profiles",
            imports: s.imports.some((r) => r.url === request.url && r.name === request.name)
              ? s.imports : [...s.imports, request],
          }));
        }
      }).catch(toastError);
    };
    Events.On("import-request", receiveImports);
    receiveImports();
  }
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
