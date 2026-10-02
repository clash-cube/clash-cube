export function bytes(n: number, digits = 1): string {
  if (!n || n < 0) return "0 B";
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return (i === 0 ? n.toFixed(0) : n.toFixed(n >= 100 ? 0 : digits)) + " " + u[i];
}

export const speed = (n: number) => bytes(n) + "/s";

export function delayClass(d: number | undefined): string {
  if (d === undefined || d === 0) return "none";
  if (d < 0) return "fail";
  if (d < 200) return "good";
  if (d < 500) return "ok";
  return "bad";
}

export function ago(iso: string | number | Date, t: (s: string, v?: Record<string, string | number>) => string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (!isFinite(ms) || ms < 0) return "";
  const m = Math.floor(ms / 60000);
  if (m < 1) return t("just now");
  if (m < 60) return t("{n}m ago", { n: m });
  const h = Math.floor(m / 60);
  if (h < 24) return t("{n}h ago", { n: h });
  return t("{n}d ago", { n: Math.floor(h / 24) });
}

export function duration(from: string): string {
  const s = Math.max(0, Math.floor((Date.now() - new Date(from).getTime()) / 1000));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m" + String(s % 60).padStart(2, "0") + "s";
  return Math.floor(s / 3600) + "h" + String(Math.floor((s % 3600) / 60)).padStart(2, "0") + "m";
}
