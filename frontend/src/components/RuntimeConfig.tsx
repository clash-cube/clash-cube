import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
  const { hits, byRow } = useMemo(() => findAll(rows, q), [rows, q]);
  const [cur, setCur] = useState(0);
  const now = hits.length ? hits[Math.min(cur, hits.length - 1)] : null;
  const label = (src: string) => src.startsWith("module:") ? `${t("Module")} · ${src.slice(7)}`
    : src === "settings" ? t("Settings") : src === "rules" ? t("My rules") : src === "router" ? t("Router") : src === "app" ? "ClashCube" : t("Pending reload");

  // the refused line is scrolled to, a few lines down from the top
  const pre = useRef<HTMLPreElement>(null);
  useLayoutEffect(() => {
    const el = pre.current?.querySelector<HTMLElement>(".bad");
    if (el && pre.current) pre.current.scrollTop = el.offsetTop - 4 * el.offsetHeight;
  }, [body, mark, focus]);
  // a new query starts at the match that was current, or else at the
  // first one from the top of the view on, as typing on narrows it down
  const from = useRef(-1);
  useEffect(() => {
    let row = from.current;
    if (row < 0 && pre.current) {
      const top = pre.current.scrollTop;
      row = Array.from(pre.current.children).findIndex((el) => (el as HTMLElement).offsetTop >= top);
    }
    const i = hits.findIndex((h) => h.row >= row);
    setCur(i < 0 ? 0 : i);
  }, [q]);
  useEffect(() => { from.current = q && now ? now.row : -1; });
  // the current match is scrolled into view only when it is out of it
  useLayoutEffect(() => {
    const box = pre.current, el = box?.querySelector<HTMLElement>(".hit.cur");
    if (!box || !el) return;
    const row = el.parentElement!.closest<HTMLElement>(".runtime-yaml > div")!;
    if (row.offsetTop < box.scrollTop || row.offsetTop + row.offsetHeight > box.scrollTop + box.clientHeight) box.scrollTop = row.offsetTop - 4 * row.offsetHeight;
    if (el.offsetLeft < box.scrollLeft + 60 || el.offsetLeft + el.offsetWidth > box.scrollLeft + box.clientWidth) box.scrollLeft = Math.max(0, el.offsetLeft - 60);
  }, [now?.row, now?.at, q]);
  // the change the arrows went to last; one past either end wraps
  const at = useRef(-1);
  useEffect(() => { at.current = -1; }, [rows]);
  const jump = (by: number) => {
    if (q) { if (hits.length) setCur((c) => (Math.min(c, hits.length - 1) + by + hits.length) % hits.length); return; }
    const all = pre.current?.querySelectorAll<HTMLElement>(".hunk");
    if (!all?.length || !pre.current) return;
    at.current = (at.current + by + all.length) % all.length;
    pre.current.scrollTop = all[at.current].offsetTop - 4 * all[at.current].offsetHeight;
  };
  const search = useRef<HTMLInputElement>(null);
  const keys = useRef<(e: KeyboardEvent) => void>(() => {});
  keys.current = (e) => {
    const mod = e.metaKey || e.ctrlKey;
    if (mod && e.key === "f") { e.preventDefault(); search.current?.focus(); search.current?.select(); }
    else if (mod && e.key === "g" && q) { e.preventDefault(); jump(e.shiftKey ? -1 : 1); }
    else if (e.target === search.current && e.key === "Enter") { e.preventDefault(); jump(e.shiftKey ? -1 : 1); }
    else if (e.target === search.current && e.key === "Escape") { if (query) setQuery(""); else search.current?.blur(); }
  };
  useEffect(() => {
    const on = (e: KeyboardEvent) => keys.current(e);
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  }, []);

  return (
    <div className="runtime-config">
      <div className="entries-bar">
        {view?.refusal && <Segmented className="track small" value={refused ? "refused" : "running"} onChange={setWhich} options={[
          { value: "running", label: t("In effect") },
          { value: "refused", label: view.refusal.source === "profile" ? t("Refused profile") : t("Refused") },
        ]} />}
        {(() => {
          const status = err ? "" : body === null ? t("Loading…")
            : q ? (hits.length ? t("{i} / {n} matches", { i: Math.min(cur, hits.length - 1) + 1, n: hits.length }) : t("No matches"))
            : refused ? (view!.refusal!.source === "profile" ? t("{n} lines of the profile, which isn't valid YAML", { n: total }) : t("{n} lines, refused by the core", { n: total }))
            : hunks > 0 ? `${t("{n} lines", { n: total })} · ${t("{n} changes from the profile", { n: hunks })}`
            : t("{n} lines, as given to the core", { n: total });
          return <span className="muted status" title={err || status}>{err ? <span className="err">{err}</span> : status}</span>;
        })()}
        {(q ? hits.length > 0 : hunks > 0) && <span className="hunk-nav">
          <button className="icon" title={q ? t("Previous match") : t("Previous change")} onClick={() => jump(-1)}><Arrow size={12} dir="up" /></button>
          <button className="icon" title={q ? t("Next match") : t("Next change")} onClick={() => jump(1)}><Arrow size={12} dir="down" /></button>
        </span>}
        <label className="search"><Search size={13} /><input ref={search} placeholder={t("Search")} value={query} onChange={(e) => setQuery(e.target.value)} /></label>
        <button className="btn small" disabled={!body} onClick={() => { App.CopyText(body!); toast(t("Copied")); }}>{t("Copy")}</button>
      </div>
      {body !== null && <pre ref={pre} className={"runtime-yaml mono" + (hunks ? " diff" : "")}>{rows.map((r, i) => r.hunk
        ? <div key={i} className="hunk"><span className="ln" /><span className="sg" />{label(r.hunk)}<span className="counts">{r.add ? ` +${r.add}` : ""}{r.del ? ` −${r.del}` : ""}</span></div>
        : <div key={i} className={r.n === mark && mark ? "bad" : r.op === "+" ? "add" : r.op === "-" ? "del" : undefined}>
          <span className="ln">{r.n ?? ""}</span>{hunks > 0 && <span className="sg">{r.op ?? ""}</span>}{paint(r.text, q.length, byRow.get(i), now?.row === i ? now.at : -1)}
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

type Hit = { row: number; at: number };

// findAll is every place q is found in the rows' text, removed lines too,
// in order, and per row where on it.
function findAll(rows: Row[], q: string) {
  const hits: Hit[] = [], byRow = new Map<number, number[]>();
  if (!q) return { hits, byRow };
  rows.forEach((r, row) => {
    if (r.hunk) return;
    const text = r.text.toLowerCase(), on: number[] = [];
    for (let at = text.indexOf(q); at >= 0; at = text.indexOf(q, at + q.length)) { on.push(at); hits.push({ row, at }); }
    if (on.length) byRow.set(row, on);
  });
  return { hits, byRow };
}

// paint marks a line's key and comment, and the matches of len at hits,
// the one at cur as current; the rest is left as written.
function paint(line: string, len: number, hits?: number[], cur = -1) {
  const k = /^\s*#/.test(line) ? null : line.match(/^(\s*(?:- )?)([^\s#'"{[][^:#]*?|"[^"]*"|'[^']*')(:)(\s.*|$)/);
  const segs: [string, string?][] = /^\s*#/.test(line) ? [[line, "cm"]] : k ? [[k[1]], [k[2], "key"], [k[3] + k[4]]] : [[line]];
  let from = 0;
  return segs.map(([s, cls], i) => {
    const parts = hits ? cut(s, from, len, hits, cur) : s;
    from += s.length;
    return cls ? <span key={i} className={cls}>{parts}</span> : <Fragment key={i}>{parts}</Fragment>;
  });
}

// cut is s, which starts at from on its line, with the matches on it marked.
function cut(s: string, from: number, len: number, hits: number[], cur: number) {
  const out: ReactNode[] = [];
  let i = 0;
  for (const h of hits) {
    const a = Math.max(h - from, 0), b = Math.min(h + len - from, s.length);
    if (a >= b) continue;
    if (a > i) out.push(s.slice(i, a));
    out.push(<mark key={h} className={h === cur ? "hit cur" : "hit"}>{s.slice(a, b)}</mark>);
    i = b;
  }
  if (!out.length) return s;
  out.push(s.slice(i));
  return out;
}
