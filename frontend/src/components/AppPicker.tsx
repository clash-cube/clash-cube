import { useEffect, useMemo, useState } from "react";
import { App, type AppInfo } from "../api";
import { useT } from "../i18n";
import { useConnectionStore } from "../connectionStore";
import { AppIcon } from "./AppIcon";
import { Search } from "./Icons";
import { toastError } from "./Toast";

// The programs a process rule can name: the apps running now, and the
// processes seen in connections (command-line tools aren't apps), the
// latter first as they are what a rule is usually for.
function usePrograms() {
  const [running, setRunning] = useState<AppInfo[]>([]);
  const snapshot = useConnectionStore((s) => s.snapshot);
  useEffect(() => { App.RunningApps().then((r) => setRunning(r ?? [])).catch(() => {}); }, []);
  return useMemo(() => {
    const seen = new Set<string>();
    const out: (AppInfo & { live?: boolean })[] = [];
    for (const c of [...snapshot.active, ...snapshot.closed]) {
      const exe = c.metadata.processPath;
      if (!exe || seen.has(exe)) continue;
      seen.add(exe);
      const i = exe.indexOf(".app/");
      const bundle = i < 0 ? "" : exe.slice(0, i + 4);
      out.push({ name: c.metadata.process || exe.split("/").pop()!, bundle, executable: exe, live: true });
    }
    // an app already listed through one of its processes isn't again
    const bundles = new Set(out.map((a) => a.bundle).filter(Boolean));
    for (const a of running) if (!seen.has(a.executable) && !(a.bundle && bundles.has(a.bundle))) { seen.add(a.executable); out.push(a); }
    return out;
  }, [running, snapshot]);
}

// what a program is as the payload of a process rule of type
export function processPayload(type: string, a: AppInfo) {
  if (type === "PROCESS-PATH") return a.executable;
  if (type === "PROCESS-PATH-REGEX") {
    // the whole app, helpers included, or just the binary
    const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    return a.bundle ? `^${esc(a.bundle)}/` : `^${esc(a.executable)}$`;
  }
  return a.executable.split("/").pop() ?? a.name;
}

export const isProcessType = (type: string) => type.startsWith("PROCESS-");

// The program a process rule names, to show its icon and name: the path,
// or for a regex made by processPayload, the path it was made from.
export function ruleProgram(type: string, payload: string): { path: string; name: string } | null {
  let path = "";
  if (type === "PROCESS-PATH") path = payload;
  else if (type === "PROCESS-PATH-REGEX") {
    const m = /^\^(.*?)(\/|\$)$/.exec(payload);
    if (!m) return null;
    path = m[1].replace(/\\(.)/g, "$1");
    if (/[\\[\](){}*+?|]/.test(m[1].replace(/\\./g, ""))) return null; // a regex of one's own
  } else return null;
  const base = path.split("/").filter(Boolean).pop() ?? path;
  return { path, name: base.replace(/\.app$/, "") };
}

// A searchable list of programs, and a way to pick one from disk.
export function AppPicker({ onPick }: { onPick: (a: AppInfo) => void }) {
  const t = useT();
  const programs = usePrograms();
  const [q, setQ] = useState("");
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? programs.filter((p) => p.name.toLowerCase().includes(s) || p.executable.toLowerCase().includes(s)) : programs;
  }, [programs, q]);
  const choose = async () => {
    try {
      const a = await App.ChooseApp();
      if (a?.executable) onPick(a);
    } catch (e) { toastError(e); }
  };
  return (
    <div className="app-picker">
      <div className="app-picker-bar">
        <label className="search"><Search size={12} /><input placeholder={t("Search apps")} value={q} onChange={(e) => setQ(e.target.value)} /></label>
        <button type="button" className="btn small" onClick={choose}>{t("Choose app…")}</button>
      </div>
      <div className="app-picker-list">
        {shown.length === 0 ? <div className="app-picker-empty">{t("No matches")}</div> : shown.map((a) => (
          <button type="button" key={a.executable} className="app-picker-row" title={a.executable} onClick={() => onPick(a)}>
            <AppIcon path={a.bundle || a.executable} />
            <span className="name">{a.name}</span>
            {"live" in a && a.live && <span className="tag">{t("In connections")}</span>}
          </button>
        ))}
      </div>
    </div>
  );
}
