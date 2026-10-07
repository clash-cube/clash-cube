import { App } from "./api";
import { toast, toastError } from "./components/Toast";
import { translate as t } from "./i18n";
import { useStore } from "./store";

// run awaits p, showing its error as a toast; the result, or undefined.
export async function run<T>(p: Promise<T>, ok?: string): Promise<T | undefined> {
  try {
    const v = await p;
    if (ok) toast(ok);
    return v;
  } catch (e) {
    toastError(e);
    return undefined;
  }
}

export const setMode = (m: string) => run(App.SetMode(m));
export const setSystemProxy = (on: boolean) => run(App.SetSystemProxy(on));
let tunBusy = false;
// setTun turns TUN on, installing the privileged helper first if need be
// (macOS asks for a password then); a cancelled prompt is not an error.
export async function setTun(on: boolean) {
  if (tunBusy) return;
  tunBusy = true;
  try {
    await App.SetTun(on);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    if (!/cancelled/.test(msg)) toastError(e);
  } finally {
    tunBusy = false;
  }
}
export const startCore = () => run(App.Start());
export const stopCore = () => run(App.Stop());
export const restartCore = () => run(App.Restart());

// coreTone is the colour of the core's status dot, as the delay classes.
export function coreTone(): string {
  const s = useStore.getState().state;
  if (!s) return "none";
  if (s.busy || s.core === "starting" || s.core === "stopping") return "ok testing";
  return s.core === "running" ? "good" : s.core === "crashed" ? "bad" : "none";
}

export function coreLabel(): string {
  const s = useStore.getState().state;
  if (!s) return "";
  if (s.busy === "restarting") return t("Restarting…");
  if (s.busy === "reloading") return t("Reloading…");
  switch (s.core) {
    case "running": return t("Running");
    case "starting": return t("Starting…");
    case "stopping": return t("Stopping…");
    case "crashed": return t("Error");
    default: return t("Stopped");
  }
}

// copyCommand copies the shell export line, for this Mac's LAN address when
// lan, saying when that address isn't there or isn't reachable yet.
export async function copyCommand(lan: boolean) {
  let cmd = lan ? await App.LANProxyCommand() : "";
  const fellBack = lan && !cmd;
  if (!cmd) cmd = await App.ProxyCommand();
  if (!await App.CopyText(cmd)) return toastError(t("Could not copy to clipboard"));
  if (fellBack) toast(t("No LAN address; copied the local one"), "", 3500);
  else if (lan && !useStore.getState().settings?.allowLan) toast(t("Copied. Other devices need Allow LAN on"), "", 3500);
  else toast(t("Copied"));
}

// openProxyGroup shows a group on the proxies page, open, to pick its node.
export function openProxyGroup(group: string) {
  useStore.setState({ proxiesTarget: group });
  useStore.getState().setView("proxies");
}

// openRule shows the first active rule of a type and value on the rules
// page, open: the one a connection that names them matched.
export function openRule(type: string, payload: string) {
  useStore.setState({ rulesTarget: { type, payload } });
  useStore.getState().setView("rules");
}

// openSettings shows the settings on one tab, also when they are open.
export function openSettings(tab: string, section?: string) {
  useStore.setState({ settingsTarget: { tab, section } });
  try { localStorage.setItem("settings.tab", tab); } catch {}
  window.dispatchEvent(new CustomEvent("settings-tab", { detail: tab }));
  useStore.getState().setView("settings");
}
