import { useEffect, useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Proxy, type Rule } from "../api";
import { usePoll } from "../usePoll";
import { Search } from "../components/Icons";
import { Segmented } from "../components/Segmented";
import { ago } from "../format";

// Rules lists the profile's rules with how often each has matched (Surge
// shows rule usage counts; mihomo counts them on top-level rules).
export function Rules() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [rules, setRules] = useState<Rule[]>([]);
  const [q, setQ] = useState("");
  const [order, setOrder] = useState<"profile" | "hits">("profile");
  const [limit, setLimit] = useState(300);

  const load = () => { if (running && !busy) Proxy.Rules().then((r) => setRules(r ?? [])).catch(() => {}); };
  useEffect(load, [running, profile, busy]);
  usePoll(load, 3000, [running, busy]);

  const maxHits = useMemo(() => Math.max(1, ...rules.map((r) => r.extra?.hitCount ?? 0)), [rules]);
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    const list = s ? rules.filter((r) => [r.type, r.payload, r.proxy].some((v) => v.toLowerCase().includes(s))) : rules;
    return order === "hits" ? [...list].sort((a, b) => (b.extra?.hitCount ?? 0) - (a.extra?.hitCount ?? 0)) : list;
  }, [rules, q, order]);

  return (
    <div className="view" onScroll={(e) => {
      const el = e.currentTarget;
      if (el.scrollTop + el.clientHeight > el.scrollHeight - 400 && limit < shown.length) setLimit(limit + 300);
    }}>
      <div className="view-head">
        <h2>{t("Rules")}</h2>
        <span className="sub">{t("{n} rules", { n: rules.length })}</span>
        <div className="view-tools">
          <Segmented className="track small" value={order} onChange={setOrder} options={[{ value: "profile", label: t("Profile order") }, { value: "hits", label: t("Most hits") }]} />
          <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => { setQ(e.target.value); setLimit(300); }} /></label>
        </div>
      </div>
      {shown.length === 0 ? (
        <div className="empty-state"><b>{t("No rules")}</b></div>
      ) : (
        <div className="list table rules">
          {shown.slice(0, limit).map((r) => {
            const hits = r.extra?.hitCount ?? 0;
            const at = r.extra?.hitAt as unknown as string | undefined;
            return (
              <div className="trow" key={r.index}>
                <span className="cell idx num">{r.index + 1}</span>
                <span className="cell"><span className="rtype">{r.type}</span></span>
                <span className="cell mono payload" title={r.payload}>{r.payload || "—"}{r.size > 0 && <span className="sub"> ({r.size})</span>}</span>
                <span className="cell policy">{r.proxy}</span>
                <span className="cell hits" title={hits && at ? t("Last hit {t}", { t: ago(at, t) }) : undefined}>
                  <span className="hitbar"><i style={{ width: (hits / maxHits) * 100 + "%" }} /></span>
                  <span className="num">{hits || ""}</span>
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
