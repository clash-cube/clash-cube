export const isWindows = typeof navigator !== "undefined" && /Windows/i.test(navigator.userAgent);
export const isWeb = import.meta.env?.MODE === "web";
