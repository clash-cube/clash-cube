import { useEffect, useRef, useState } from "react";
import { Proxy, type Rule } from "../api";
import type { RuleEntries } from "../../bindings/github.com/localhost-copilot/clashcube/internal/backend/models";
import { useT } from "../i18n";
import { ago, nodeLabel } from "../format";
import { errText } from "./Toast";
import { Chevron, Search } from "./Icons";

// The rule types whose payload names a list the core reads from a file.
const listKind = (type: string) => ({ RuleSet: "ruleset", GeoSite: "geosite", GeoIP: "geoip", SrcGeoIP: "geoip" } as Record<string, string>)[type];

// What a rule is made of, unfolded under its row: where it sends traffic,
// how often it matched, and what it matches: the conditions of AND, OR and
// NOT, and the entries of a rule set, GeoSite or GeoIP list.
export function RuleDetail({ r, route }: { r: Rule; route: string[] }) {
  const t = useT();
  const hits = r.extra?.hitCount ?? 0;
  const misses = r.extra?.missCount ?? 0;
  const at = r.extra?.hitAt as unknown as string | undefined;
  const logic = ["AND", "OR", "NOT"].includes(r.type) ? parseLogic(r.type, r.payload) : null;
  return (
    <div className="rule-detail">
      <div className="rule-facts">
        <Fact label={t("Route")}>{route.slice(0, -1).map((g) => nodeLabel(g) + " → ").join("")}<span className={"exit " + tone(route[route.length - 1])}>{nodeLabel(route[route.length - 1])}</span></Fact>
        <Fact label={t("Matched")}>{hits ? t("{n} times, last {t}", { n: hits, t: ago(at!, t) }) : t("Not yet")}</Fact>
        {misses > 0 && <Fact label={t("Checked")}>{t("{n} times", { n: hits + misses })}</Fact>}
      </div>
      {logic ? <LogicTree node={logic} />
        : listKind(r.type) ? <EntryList kind={listKind(r.type)} name={r.payload} />
        : <div className="rule-plain mono">{r.type},{r.payload}</div>}
    </div>
  );
}

const tone = (exit: string) => exit === "DIRECT" ? "direct" : /^REJECT/.test(exit) ? "reject" : "proxy";

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="rule-fact"><span className="lbl">{label}</span><span className="val">{children}</span></div>;
}

type Cond = { type: string; payload: string; kids?: Cond[] };

// parseLogic reads mihomo's payload for a logic rule back into its
// conditions: "((T,p) && (T,p))", "((T,p) || …)" and "(!(T,p))", where a
// condition may be a logic rule in turn, written as "(OR,(…))".
export function parseLogic(type: string, payload: string): Cond {
  let s = payload.trim();
  if (s.startsWith("(") && s.endsWith(")")) s = s.slice(1, -1);
  if (type === "NOT" && s.startsWith("!")) s = s.slice(1);
  const kids: Cond[] = [];
  let depth = 0, from = -1;
  for (let i = 0; i < s.length; i++) {
    if (s[i] === "(") { if (depth++ === 0) from = i; }
    else if (s[i] === ")" && --depth === 0 && from >= 0) {
      const inner = s.slice(from + 1, i);
      const comma = inner.indexOf(",");
      const kt = comma < 0 ? inner : inner.slice(0, comma), kp = comma < 0 ? "" : inner.slice(comma + 1);
      kids.push(["AND", "OR", "NOT"].includes(kt) ? parseLogic(kt, kp) : { type: kt, payload: kp });
      from = -1;
    }
  }
  return { type, payload, kids };
}

function LogicTree({ node }: { node: Cond }) {
  const t = useT();
  const joint = node.type === "AND" ? t("all of") : node.type === "OR" ? t("any of") : t("none of");
  return (
    <div className="logic">
      <div className="logic-head"><span className="rtype">{node.type}</span><span className="muted">{joint}</span></div>
      <div className="logic-kids">
        {(node.kids ?? []).map((k, i) => k.kids ? <LogicTree key={i} node={k} /> : <LogicLeaf key={i} c={k} />)}
      </div>
    </div>
  );
}

// a condition; one naming a list unfolds it
function LogicLeaf({ c }: { c: Cond }) {
  const kind = listKind(c.type);
  const [open, setOpen] = useState(false);
  return (
    <div className="logic-leaf">
      <div className={"logic-cond" + (kind ? " click" : "")} onClick={() => kind && setOpen(!open)}>
        {kind ? <Chevron size={10} className={"chev" + (open ? " open" : "")} /> : <span className="chev" />}
        <span className="rtype">{c.type}</span><span className="mono">{c.payload}</span>
      </div>
      {open && <EntryList kind={kind!} name={c.payload} />}
    </div>
  );
}

const PAGE = 300;

// A list's entries, searched in the backend (read again when version, the
// time it was fetched, changes), so a GeoSite list with tens of thousands
// of names doesn't reach the window whole.
export function EntryList({ kind, name, version }: { kind: string; name: string; version?: string }) {
  const t = useT();
  const [q, setQ] = useState("");
  const [limit, setLimit] = useState(PAGE);
  const [res, setRes] = useState<RuleEntries | null>(null);
  const [err, setErr] = useState("");
  const seq = useRef(0);
  useEffect(() => {
    const n = ++seq.current;
    const timer = setTimeout(() => {
      Proxy.RuleEntries(kind, name, q, limit)
        .then((r) => { if (n === seq.current) { setRes(r); setErr(""); } })
        .catch((e) => { if (n === seq.current) setErr(errText(e)); });
    }, res ? 150 : 0);
    return () => clearTimeout(timer);
  }, [kind, name, q, limit, version]);

  if (err) return <div className="entries"><div className="err">{err}</div></div>;
  if (!res) return <div className="entries"><div className="muted">{t("Loading…")}</div></div>;
  const entries = res.entries ?? [];
  const source = res.source === "inline" ? t("In the profile") : res.source;
  return (
    <div className="entries">
      <div className="entries-bar">
        <span className="muted">
          {[source, res.note ? "" : q ? t("{n} of {total}", { n: res.matched, total: res.total }) : t("{n} entries", { n: res.total }),
            res.not ? t("matches everything else") : ""].filter(Boolean).join(" · ")}
        </span>
        {!res.note && res.total > 8 && <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => { setQ(e.target.value); setLimit(PAGE); }} /></label>}
      </div>
      {res.note ? <div className="muted">{t(res.note)}</div>
        : entries.length === 0 ? <div className="muted">{t("No matches")}</div>
        : <div className="entries-scroll"><div className="entries-grid mono">{entries.map((e, i) => <span key={i} title={e}>{e}</span>)}</div></div>}
      {res.matched > entries.length && (
        <button className="btn small" onClick={() => setLimit(limit + PAGE * 4)}>{t("Show {n} more", { n: Math.min(PAGE * 4, res.matched - entries.length) })}</button>
      )}
    </div>
  );
}
