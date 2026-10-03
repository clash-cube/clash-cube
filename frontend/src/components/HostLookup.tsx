import { useRef, useState } from "react";
import { Proxy, type Lookup, type UserRule } from "../api";
import { useT } from "../i18n";
import { errText } from "./Toast";
import { RuleEditor } from "./RuleEditor";
import { Search } from "./Icons";

// A name's way through the core: what the resolver answers and which rule
// sends a connection where, with a way to add a rule for it.
export function HostLookup({ running }: { running: boolean }) {
  const t = useT();
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState<Lookup | null>(null);
  const [err, setErr] = useState("");
  const [ruleAt, setRuleAt] = useState<HTMLElement | null>(null);
  const seq = useRef(0);

  const run = async () => {
    if (!q.trim()) return;
    const n = ++seq.current;
    setBusy(true); setErr("");
    try {
      const r = await Proxy.LookupHost(q.trim());
      if (n === seq.current) setRes(r);
    } catch (e) { if (n === seq.current) { setRes(null); setErr(errText(e)); } }
    if (n === seq.current) setBusy(false);
  };

  const ip = res && /^[\d.]+$|:/.test(res.host);
  const choices: UserRule[] = !res ? [] : ip
    ? [{ type: res.host.includes(":") ? "IP-CIDR6" : "IP-CIDR", payload: res.host + (res.host.includes(":") ? "/128" : "/32"), policy: "" }]
    : [{ type: "DOMAIN", payload: res.host, policy: "" }, { type: "DOMAIN-SUFFIX", payload: res.host.split(".").slice(-2).join("."), policy: "" }];
  const chain = res?.chain ?? [];
  // the chain from the policy down to the node, as the connections page shows it
  const route = chain.length ? [...chain].reverse().join(" → ") : "";
  const addrs = [...(res?.a ?? []), ...(res?.aaaa ?? [])];

  return (
    <div className="lookup">
      <form className="lookup-bar" onSubmit={(e) => { e.preventDefault(); run(); }}>
        <label className="search"><Search />
          <input autoFocus placeholder={t("A domain, an address, host:port or a URL")} value={q} onChange={(e) => setQ(e.target.value)} />
        </label>
        <button type="submit" className="btn primary" disabled={!running || busy || !q.trim()}>{busy ? t("Looking up…") : t("Look up")}</button>
      </form>
      {!running && <div className="empty-state"><b>{t("The core isn't running")}</b></div>}
      {err && <div className="banner err"><div className="grow mono">{err}</div></div>}
      {res && (
        <div className="list lookup-result">
          <div className="row">
            <div className="who"><div className="name mono">{res.host}:{res.port}</div><div className="sub">{t("How a connection to it goes through the core")}</div></div>
            <button className="btn small" onClick={(e) => setRuleAt(e.currentTarget)}>{t("Add rule…")}</button>
          </div>
          <Field label={t("Rule")}>
            {res.routeError ? <span className="err">{res.routeError}</span>
              : <><span className="rtype">{res.rule}</span>{res.rulePayload && <span className="mono"> {res.rulePayload}</span>}</>}
          </Field>
          <Field label={t("Route")}>{route ? <span className="policy">{route}</span> : "—"}</Field>
          {res.remoteIp && <Field label={t("Connects to")}><span className="mono">{res.remoteIp}</span>
            {res.remoteFake && <div className="muted note">{t("A fake-ip address: another fake-ip resolver, such as a TUN app or the router, answered for this Mac.")}</div>}
          </Field>}
          {!ip && (
            <Field label={res.dnsVia === "system" ? t("DNS (system)") : t("DNS")}>
              {res.dnsError ? <span className="err">{t(res.dnsError)}</span>
                : addrs.length === 0 && (res.cname ?? []).length === 0 ? <span className="muted">{t("No records")}</span>
                : <div className="answers">
                    {(res.cname ?? []).map((c) => <div key={c}><span className="rtype">CNAME</span> <span className="mono">{c}</span></div>)}
                    {addrs.map((a) => <div key={a.data}><span className="mono">{a.data}</span>{a.ttl > 0 && <span className="muted"> TTL {a.ttl}s</span>}</div>)}
                  </div>}
              {res.dnsVia !== "system" && res.dnsMode && <div className="muted note">{res.dnsMode === "fake-ip"
                ? t("fake-ip: apps get an address from the pool; these are the real ones the core resolves.")
                : t("DNS mode: {m}", { m: res.dnsMode })}</div>}
            </Field>
          )}
        </div>
      )}
      <RuleEditor anchor={ruleAt} onClose={() => setRuleAt(null)} choices={choices} />
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="row lookup-field"><span className="lbl">{label}</span><div className="val">{children}</div></div>;
}
