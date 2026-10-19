import { platform } from "../platform";

// Keep platform policy together; shared settings render only supported controls.
export function capabilities() {
  const desktop = platform !== "android";
  return { desktop, notifications: desktop, helper: desktop, listeners: desktop, networkRules: desktop,
    hotkeys: desktop, updater: desktop, desktopFiles: desktop, phone: !desktop };
}
