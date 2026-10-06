import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { App, Profiles as P } from "../api";
import type { Line, RuntimeView } from "../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import { useT } from "../i18n";
import { useStore } from "../store";
import { errText, toast, toastError } from "./Toast";
import { Arrow, Close, Search } from "./Icons";
import { Segmented } from "./Segmented";

// RuntimeConfig shows, read only, what the core was last given: the profile
// with the modules, user rules and settings merged in, for seeing what a
// module made of it. It is shown as a diff against the profile, each change
// under what made it. A configuration the core refused since can be read
// beside it, at the line the error names. Both are read again after each
// reload.
export function RuntimeConfig({ focus }: { focus: number }) {
  const t = useT();
  const busy = useStore((s) => s.state?.busy);
  // every state event brings a new object: the refusal is told apart by
  // what it says, so only a different one reloads and reopens the view
  const refusal = useStore((s) => { const r = s.state?.refusal; return r ? `${r.source}\n${r.line}\n${r.error}` : ""; });
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
  const { rows, total, hunks } = useMemo(() => toRows(body ?? "", refused ? null : view?.lines), [body, refused, view]);
  const q = query.trim().toLowerCase();
  const shown = useMemo(() => q ? rows.filter((r) => !r.hunk && r.text.toLowerCase().includes(q)) : rows, [rows, q]);
  const label = (src: string) => src.startsWith("module:") ? `${t("Module")} · ${src.slice(7)}`
    : src === "settings" ? t("Settings") : src === "rules" ? t("My rules") : src === "app" ? "ClashCube" : t("Pending reload");

  // the refused line is scrolled to, a few lines down from the top
  const pre = useRef<HTMLPreElement>(null);
  useLayoutEffect(() => {
    const el = pre.current?.querySelector<HTMLElement>(".bad");
    if (el && pre.current) pre.current.scrollTop = el.offsetTop - 4 * el.offsetHeight;
  }, [body, mark, q, focus]);
  // the change the arrows went to last; one past either end wraps
  const at = useRef(-1);
  useEffect(() => { at.current = -1; }, [rows, q]);
  const jump = (by: number) => {
    const all = pre.current?.querySelectorAll<HTMLElement>(".hunk");
    if (!all?.length || !pre.current) return;
    at.current = (at.current + by + all.length) % all.length;
    pre.current.scrollTop = all[at.current].offsetTop - 4 * all[at.current].offsetHeight;
  };

  return (
    <div className="runtime-config">
      <div className="entries-bar">
        {view?.refusal && <Segmented className="track small" value={refused ? "refused" : "running"} onChange={setWhich} options={[
          { value: "running", label: t("In effect") },
          { value: "refused", label: view.refusal.source === "profile" ? t("Refused profile") : t("Refused") },
        ]} />}
        <span className="muted">{err ? <span className="err">{err}</span>
          : body === null ? t("Loading…")
          : q ? t("{n} of {total} lines", { n: shown.length, total })
          : refused ? (view!.refusal!.source === "profile" ? t("{n} lines of the profile, which isn't valid YAML", { n: total }) : t("{n} lines, refused by the core", { n: total }))
          : t("{n} lines, as given to the core", { n: total })}
          {!err && body !== null && !q && hunks > 0 && <> · {t("{n} changes from the profile", { n: hunks })}</>}</span>
        {hunks > 0 && !q && <span className="hunk-nav">
          <button className="icon" title={t("Previous change")} onClick={() => jump(-1)}><Arrow size={12} dir="up" /></button>
          <button className="icon" title={t("Next change")} onClick={() => jump(1)}><Arrow size={12} dir="down" /></button>
        </span>}
        <label className="search"><Search size={13} /><input placeholder={t("Search")} value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <button className="btn small" disabled={!body} onClick={() => { App.CopyText(body!); toast(t("Copied")); }}>{t("Copy")}</button>
      </div>
      {body !== null && <pre ref={pre} className={"runtime-yaml mono" + (hunks ? " diff" : "")}>{shown.map((r, i) => r.hunk
        ? <div key={i} className="hunk"><span className="ln" /><span className="sg" />{label(r.hunk)}<span className="counts">{r.add ? ` +${r.add}` : ""}{r.del ? ` −${r.del}` : ""}</span></div>
        : <div key={i} className={r.n === mark && mark ? "bad" : r.op === "+" ? "add" : r.op === "-" ? "del" : undefined}>
          <span className="ln">{r.n ?? ""}</span>{hunks > 0 && <span className="sg">{r.op ?? ""}</span>}{paint(r.text)}
        </div>)}</pre>}
    </div>
  );
}

type Row = { text: string; n?: number; op?: string; hunk?: string; add?: number; del?: number };

// toRows is body's lines numbered, or, given them against the profile,
// the profile's removed lines between them unnumbered and a header ahead
// of each run of changes from one source.
function toRows(body: string, lines?: Line[] | null) {
  if (!lines?.length) {
    const rows: Row[] = body.replace(/\n$/, "").split("\n").map((text, i) => ({ text, n: i + 1 }));
    return { rows, total: rows.length, hunks: 0 };
  }
  const rows: Row[] = [];
  let n = 0, hunks = 0, head: Row | null = null;
  for (const l of lines) {
    if (!l.op) { head = null; rows.push({ text: l.text, n: ++n }); continue; }
    const src = l.source ?? "";
    if (!head || head.hunk !== src) { head = { text: "", hunk: src, add: 0, del: 0 }; rows.push(head); hunks++; }
    if (l.op === "+") { head.add!++; rows.push({ text: l.text, n: ++n, op: "+" }); }
    else { head.del!++; rows.push({ text: l.text, op: "-" }); }
  }
  return { rows, total: n, hunks };
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
