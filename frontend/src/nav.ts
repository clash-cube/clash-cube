import type { SettingsTab, View } from "./store";
import { isWeb } from "./platform";

// The window's pages and their own tabs: the header's tabs and the sidebar
// list the same ones, and the pages' tab strips switch them.
type T = (s: string) => string;
export type Tab<V extends string> = { value: V; label: string };
export type Page = { view: View; label: string };
export type Group = { label: string; pages: Page[] };

export const overviewTabs = (t: T): Tab<View>[] => [
  { value: "overview", label: t("Overview") },
  { value: "usage", label: t("Traffic statistics") },
  { value: "globe", label: t("Global connections") },
];
export const logsTabs = (t: T): Tab<View>[] => [{ value: "logs", label: t("Logs") }, { value: "events", label: t("Events") }];
export const settingsTabs = (t: T): Tab<SettingsTab>[] => ([
  { value: "general", label: t("General") },
  { value: "network", label: t("Network") },
  { value: "rules", label: t("Network rules") },
  { value: "tun", label: t("Enhanced Mode") },
  { value: "core", label: t("Core") },
] as Tab<SettingsTab>[]).filter((tab) => !isWeb || (tab.value !== "rules" && tab.value !== "tun"));

// in the header's order
export const pages = (t: T): Page[] => [
  { view: "overview", label: t("Overview") },
  { view: "proxies", label: t("Proxies") },
  ...(isWeb ? [{ view: "devices" as const, label: t("Devices") }] : []),
  { view: "profiles", label: t("Profiles") },
  { view: "connections", label: t("Connections") },
  { view: "rules", label: t("Rules") },
  { view: "logs", label: t("Logs") },
];

// The sidebar's: what goes on, what decides where it goes, and what
// happened. The tabs a page shares in the header are rows of their own
// here: under a page they would repeat its name ("Overview / Overview").
export function groups(t: T): Group[] {
  const tabs = [...overviewTabs(t), ...logsTabs(t)].map((x): Page => ({ view: x.value, label: x.label }));
  const by = new Map([...pages(t), ...tabs].map((p) => [p.view, p]));
  const of = (...v: View[]) => v.map((x) => by.get(x)!);
  return [
    { label: t("Activity"), pages: of("overview", "usage", "globe", "connections") },
    { label: t("Routing"), pages: of("proxies", ...(isWeb ? ["devices" as const] : []), "profiles", "rules") },
    { label: t("Diagnostics"), pages: of("logs", "events") },
  ];
}

// the page a view is one of the tabs of
export function pageOf(view: View): View {
  if (view === "usage" || view === "globe") return "overview";
  if (view === "events") return "logs";
  return view;
}

// where the reader is, as the header beside the sidebar says it:
// the group and the page, or Settings and its tab
export function crumb(t: T, view: View, settingsTab: SettingsTab): string[] {
  if (view === "settings") return [t("Settings"), settingsTabs(t).find((x) => x.value === settingsTab)?.label ?? ""].filter(Boolean);
  const group = groups(t).find((g) => g.pages.some((p) => p.view === view));
  return [group?.label ?? "", group?.pages.find((p) => p.view === view)?.label ?? ""].filter(Boolean);
}
