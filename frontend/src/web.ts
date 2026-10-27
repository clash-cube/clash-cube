// The LuCI page and RPC bridge share an authenticated origin. No core address
// or secret is sent to the browser. Desktop service types keep calls compatible.
import type * as Desktop from "./desktop";
import { translate as t } from "./i18n";
import type { RouterAPI } from "./routerTypes";

const endpoint = document.querySelector<HTMLMetaElement>('meta[name="clashcube-api"]')?.content ?? location.pathname.replace(/\/$/, "") + "/api/";
const token = document.querySelector<HTMLMetaElement>('meta[name="clashcube-token"]')?.content ?? "";

async function request(path: string, args: unknown): Promise<any> {
  const response = await fetch(endpoint + path + "?token=" + encodeURIComponent(token), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-ClashCube-Request": "1" },
    body: JSON.stringify(args),
    signal: AbortSignal.timeout(180_000),
  });
  if (!response.headers.get("Content-Type")?.includes("application/json")) {
    // LuCI returns its login form when the session has expired.
    if (window.parent !== window) window.parent.location.reload(); else location.reload();
    throw new Error(t("Please sign in to LuCI again"));
  }
  const result = await response.json();
  if (!response.ok) throw new Error(result?.error ?? `HTTP ${response.status}`);
  return result;
}

function service<T>(name: string, local: Record<string, (...args: any[]) => any> = {}): T {
  return new globalThis.Proxy(local, {
    get: (target, key: string) => target[key] ?? ((...args: unknown[]) => request(`rpc/${name}/${key}`, args)),
  }) as T;
}

async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) { await navigator.clipboard.writeText(text); return true; }
  // LAN LuCI commonly uses HTTP, where the Clipboard API is unavailable.
  const input = document.createElement("textarea");
  input.value = text;
  input.style.position = "fixed";
  input.style.opacity = "0";
  document.body.append(input);
  input.select();
  const copied = document.execCommand("copy");
  input.remove();
  return copied;
}

async function shellCommand() {
  const settings = await Settings.Get();
  const host = location.hostname;
  const address = host.includes(":") ? `[${host}]` : host;
  return `export http_proxy=http://${address}:${settings.mixedPort} https_proxy=http://${address}:${settings.mixedPort} all_proxy=socks5://${address}:${settings.mixedPort}`;
}

export const App = service<typeof Desktop.App>("App", {
  CopyText: copyText,
  OpenURL: async (url: string) => {
    const parsed = new URL(url);
    if (!["https:", "http:"].includes(parsed.protocol)) throw new Error(t("Only HTTP and HTTPS links are supported"));
    window.open(parsed.href, "_blank", "noopener,noreferrer");
  },
  AppIcon: async () => "",
  LANAddress: async () => location.hostname,
  ProxyCommand: shellCommand,
  LANProxyCommand: shellCommand,
});

async function downloadProfile(id: string) {
  const body = await request("rpc/Profiles/Read", [id]);
  const blob = URL.createObjectURL(new Blob([body], { type: "application/yaml" }));
  const link = document.createElement("a");
  link.href = blob;
  link.download = `clashcube-${id}.yaml`;
  link.click();
  setTimeout(() => URL.revokeObjectURL(blob), 1000);
}

export const Profiles = service<typeof Desktop.Profiles>("Profiles", {
  ImportFile: () => new Promise((resolve, reject) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = ".yaml,.yml";
    input.addEventListener("cancel", () => resolve({ id: "" }), { once: true });
    input.addEventListener("change", async () => {
      const file = input.files?.[0];
      if (!file) { resolve({ id: "" }); return; }
      try { resolve(await request("rpc/Profiles/ImportContent", [file.name.replace(/\.ya?ml$/i, ""), await file.text()])); }
      catch (error) { reject(error); }
    }, { once: true });
    input.click();
  }),
  Reveal: downloadProfile,
  OpenInEditor: downloadProfile,
});
export const Proxy = service<typeof Desktop.Proxy>("Proxy");
export const Settings = service<typeof Desktop.Settings>("Settings");
export const Router = service<RouterAPI>("Router");

type Listener = (event: { data: any }) => void;
const listeners = new Map<string, Set<Listener>>();
let polling = false;

async function poll() {
  let cursor = { stream: "", after: 0 };
  while (listeners.size) {
    try {
      const result = await request("events", cursor);
      cursor = { stream: result.stream, after: result.after };
      for (const event of result.events) {
        for (const listener of listeners.get(event.name) ?? []) {
          try { listener({ data: event.data }); } catch (error) { console.error(error); }
        }
      }
      if (result.reset) window.dispatchEvent(new Event("focus"));
    } catch { await new Promise((resolve) => setTimeout(resolve, 2000)); }
  }
  polling = false;
}

export const Events = {
  On(name: string, listener: Listener) {
    const group = listeners.get(name) ?? new Set<Listener>();
    listeners.set(name, group);
    group.add(listener);
    if (!polling) { polling = true; void poll(); }
    return () => { group.delete(listener); if (!group.size) listeners.delete(name); };
  },
};

export const Window = { Close: async () => window.close() };
