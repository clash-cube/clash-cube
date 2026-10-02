import { useEffect, useMemo, useState } from "react";
import { useT } from "../i18n";
import { useStore } from "../store";
import { Proxy, type Rule } from "../api";
import { Search } from "../components/Icons";

export function Rules() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const profile = useStore((s) => s.state?.profile);
  const busy = useStore((s) => s.state?.busy);
  const [rules, setRules] = useState<Rule[]>([]);
  const [q, setQ] = useState("");
  const [limit, setLimit] = useState(300);

  useEffect(() => {
    if (!running || busy) return;
    Proxy.Rules().then((r) => setRules(r ?? [])).catch(() => {});
  }, [running, profile, busy]);

  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    return s ? rules.filter((r) => [r.type, r.payload, r.proxy].some((v) => v.toLowerCase().includes(s))) : rules;
  }, [rules, q]);

  return (
    <div className="view" onScroll={(e) => {
      const el = e.currentTarget;
      if (el.scrollTop + el.clientHeight > el.scrollHeight - 400 && limit < shown.length) setLimit(limit + 300);
    }}>
      <div className="view-head">
        <h2>{t("Rules")}</h2>
        <span className="sub">{t("{n} rules", { n: rules.length })}</span>
        <div className="grow" />
        <label className="search"><Search /><input placeholder={t("Search")} value={q} onChange={(e) => { setQ(e.target.value); setLimit(300); }} /></label>
      </div>
      {shown.length === 0 ? (
        <div className="empty-state"><b>{t("No rules")}</b></div>
      ) : (
        <div className="list table rules">
          {shown.slice(0, limit).map((r) => (
            <div className="trow" key={r.index}>
              <span className="cell idx num">{r.index + 1}</span>
              <span className="cell"><span className="rtype">{r.type}</span></span>
              <span className="cell mono payload" title={r.payload}>{r.payload || "—"}{r.size > 0 && <span className="sub"> ({r.size})</span>}</span>
              <span className="cell policy">{r.proxy}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
