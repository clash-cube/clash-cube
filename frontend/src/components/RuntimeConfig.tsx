import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { App, Profiles as P } from "../api";
import type { RuntimeView } from "../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import { useT } from "../i18n";
import { useStore } from "../store";
import { errText, toast, toastError } from "./Toast";
import { Close, Search } from "./Icons";
import { Segmented } from "./Segmented";

// RuntimeConfig shows, read only, what the core was last given: the profile
// with the modules, user rules and settings merged in, for seeing what a
// module made of it. A configuration the core refused since can be read
// beside it, at the line the error names. Both are read again after each
// reload.
export function RuntimeConfig({ focus }: { focus: number }) {
  const t = useT();
  const busy = useStore((s) => s.state?.busy);
  const refusal = useStore((s) => s.state?.refusal);
  const [view, setView] = useState<RuntimeView | null>(null);
  const [err, setErr] = useState("");
  const [query, setQuery] = useState("");
  const [which, setWhich] = useState<"running" | "refused">(refusal ? "refused" : "running");

  useEffect(() => {
    if (busy) return;
    let live = true;
    P.RuntimeConfig().then((v) => { if (live) { setView(v); setErr(""); } }, (e) => live && setErr(errText(e)));
    return () => { live = false; };
  }, [busy, refusal]);
  // a new refusal, or the banner's Show, opens it
  useEffect(() => { setWhich(refusal ? "refused" : "running"); }, [refusal, focus]);

  const refused = which === "refused" && !!view?.refusal;
  const body = view ? (refused ? view.refused ?? "" : view.body) : null;
  const mark = refused ? view!.refusal!.line : 0;
  const lines = useMemo(() => (body ?? "").replace(/\n$/, "").split("\n"), [body]);
  const q = query.trim().toLowerCase();
  const shown = useMemo(() => lines.map((l, i) => [i + 1, l] as const).filter(([, l]) => !q || l.toLowerCase().includes(q)), [lines, q]);

  // the refused line is scrolled to, a few lines down from the top
  const pre = useRef<HTMLPreElement>(null);
  useLayoutEffect(() => {
    const el = pre.current?.querySelector<HTMLElement>(".bad");
    if (el && pre.current) pre.current.scrollTop = el.offsetTop - 4 * el.offsetHeight;
  }, [body, mark, q, focus]);

  return (
    <div className="runtime-config">
      <div className="entries-bar">
        {view?.refusal && <Segmented className="track small" value={refused ? "refused" : "running"} onChange={setWhich} options={[
          { value: "running", label: t("In effect") },
          { value: "refused", label: view.refusal.source === "profile" ? t("Refused profile") : t("Refused") },
        ]} />}
        <span className="muted">{err ? <span className="err">{err}</span>
          : body === null ? t("Loading…")
          : q ? t("{n} of {total} lines", { n: shown.length, total: lines.length })
          : refused ? (view!.refusal!.source === "profile" ? t("{n} lines of the profile, which isn't valid YAML", { n: lines.length }) : t("{n} lines, refused by the core", { n: lines.length }))
          : t("{n} lines, as given to the core", { n: lines.length })}</span>
        <label className="search"><Search size={13} /><input placeholder={t("Search")} value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <button className="btn small" disabled={!body} onClick={() => { App.CopyText(body!); toast(t("Copied")); }}>{t("Copy")}</button>
      </div>
      {body !== null && <pre ref={pre} className="runtime-yaml mono">{shown.map(([n, l]) => <div key={n} className={n === mark ? "bad" : undefined}><span className="ln">{n}</span>{paint(l)}</div>)}</pre>}
    </div>
  );
}

// RefusalBanner tells that the core refused a configuration, where, and
// that the one before stays; Show opens it at that line.
export function RefusalBanner({ onShow }: { onShow?: () => void }) {
  const t = useT();
  const refusal = useStore((s) => s.state?.refusal);
  const profile = useStore((s) => s.state?.profile);
  if (!refusal) return null;
  return (
    <div className="banner err">
      <div className="grow">
        <b>{refusal.line ? t("The core refused the configuration at line {n}; the previous one stays", { n: refusal.line }) : t("The core refused the configuration; the previous one stays")}</b>
        <div className="mono">{refusal.error}</div>
      </div>
      {refusal.source === "profile" && profile && <button className="btn small" onClick={() => P.OpenInEditor(profile).catch(toastError)}>{t("Open in editor")}</button>}
      {onShow && <button className="btn small primary" onClick={onShow}>{t("Show")}</button>}
      <button className="icon" title={t("Dismiss")} onClick={() => P.DismissRefusal()}><Close size={12} /></button>
    </div>
  );
}

// paint marks a line's key and comment; the rest is left as written.
function paint(line: string) {
  if (/^\s*#/.test(line)) return <span className="cm">{line}</span>;
  const k = line.match(/^(\s*(?:- )?)([^\s#'"{[][^:#]*?|"[^"]*"|'[^']*')(:)(\s.*|$)/);
  if (!k) return line;
  return <>{k[1]}<span className="key">{k[2]}</span>{k[3]}{k[4]}</>;
}
