import { Platform } from "../bindings/github.com/localhost-copilot/clashcube/internal/gui/appservice";

export let platform = "unknown";
export let isWindows = false;
export let isAndroid = false;

export async function initPlatform() {
  platform = await Platform();
  isWindows = platform === "windows";
  isAndroid = platform === "android";
  document.documentElement.dataset.platform = platform === "darwin" ? "macos" : platform;
}
