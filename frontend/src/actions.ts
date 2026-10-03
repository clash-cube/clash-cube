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
