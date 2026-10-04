import { useEffect, useRef, useState, type ReactNode } from "react";
import { App, type Connectivity, type DNSEgress, type Egress, type ProxyEgress } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { delayClass, flagged } from "../format";
import { Refresh } from "./Icons";
import { LatencyBars } from "./LatencyBars";

type Item = "router" | "dns" | "internet" | "proxy";
const items: Item[] = ["router", "dns", "internet", "proxy"];
const fields: Record<Item, (keyof Connectivity)[]> = {
  router: ["router", "gateway"],
  dns: ["dns", "dnsVia", "dnsMode"],
  internet: ["internet"],
  proxy: ["proxy", "via", "chain"],
};

// a chain, node first, as "policy → node"; the policy alone when the rule
// named the node itself
const route = (chain: string[]) =>
  chain.length > 1 ? `${chain[chain.length - 1]} → ${chain[0]}` : chain[0] ?? "—";

// the least each card's bars scale to: a router answers in a few ms, a
// cached DNS answer in under one, a site in a couple of hundred
const floors: Record<Item, number> = { router: 5, dns: 50, internet: 200, proxy: 200 };

// whole ms, or hundredths under 1 ms (a wired router answers in 0.3)
const fmtMS = (v: number) => (v < 1 ? v.toFixed(2) : String(Math.round(v)));

type Lookup<T> = T | "loading" | "failed";

// a card's second line that a click swaps for something looked up then,
// and again on every measure while it is shown
function useDetail<T>(fetch: () => Promise<T>) {
  const [shown, setShown] = useState(false);
  const [data, setData] = useState<Lookup<T>>("loading");
  const seq = useRef(0);
  const look = () => {
    const n = ++seq.current;
    setData((d) => (typeof d === "object" ? d : "loading"));
    fetch()
      .then((r) => { if (n === seq.current) setData(r); })
      .catch(() => { if (n === seq.current) setData("failed"); });
  };
  const toggle = () => { if (!shown) look(); setShown(!shown); };
  return { shown, data, toggle, refresh: () => { if (shown) look(); } };
}

// a nameserver as the card names it: the host of a URL, without the
// #policy suffix; plain addresses and dhcp://, system as written
function shortServer(s: string) {
  s = s.split("#")[0];
  try {
    const u = new URL(s);
    return /^(https?|tls|quic|udp|tcp):$/.test(u.protocol) && u.hostname ? u.hostname : s;
  } catch { return s; }
}

// The four latencies Surge's overview leads with: the router, DNS, the
// internet directly, and through the proxy. Measured on open, every 30s
// while shown, and on demand.
export function ConnectivityCards() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const mode = useStore((s) => s.state?.mode);
  const profile = useStore((s) => s.state?.profile);
  const reset = useStore((s) => s.networkReset);
  const latency = useStore((s) => s.latency);
  const [c, setC] = useState<Partial<Connectivity>>({});
  const [pending, setPending] = useState<Set<Item>>(new Set());
  const busy = pending.size > 0;
  // one per item, so retesting one card drops only that card's late answer
  const seq = useRef<Record<Item, number>>({ router: 0, dns: 0, internet: 0, proxy: 0 });
  // the Internet card shows DIRECT, or on a click where direct traffic
  // leaves: the interface and the public address
  const direct = useDetail<Egress>(App.DirectEgress);
  // the DNS card shows the resolver, or on a click its upstream and the
  // address authoritative servers see the queries from
  const dns = useDetail<DNSEgress>(App.DNSEgress);
  // the Proxy card shows the route, or on a click where proxied traffic
  // leaves
  const proxy = useDetail<ProxyEgress>(App.ProxyEgress);

  // each item lands on its own, so a slow proxy doesn't hold the router back
  const details: Partial<Record<Item, { refresh: () => void }>> = { dns, internet: direct, proxy };
  const measureOne = (key: Item) => {
    const n = ++seq.current[key];
    details[key]?.refresh();
    setPending((p) => new Set(p).add(key));
    App.ConnectivityItem(key).then((r) => {
      if (n !== seq.current[key]) return;
      setC((prev) => {
        const next = { ...prev };
        for (const f of fields[key]) (next as any)[f] = r[f];
        return next;
      });
    }).catch(() => { /* the core stopped; the card keeps a dash */ }).finally(() => {
      if (n !== seq.current[key]) return;
      setPending((p) => { const s = new Set(p); s.delete(key); return s; });
    });
  };
  const measure = () => items.forEach(measureOne);

  useEffect(() => {
    if (!running) { for (const k of items) seq.current[k]++; setC({}); setPending(new Set()); return; }
    measure();
    // on a metered network only what is asked for is measured
    const id = setInterval(() => { if (!document.hidden && !useStore.getState().state?.network.savingData) measure(); }, 30000);
    return () => clearInterval(id);
  }, [running, mode, profile, reset]);

  let internetSub: ReactNode = "DIRECT";
  let internetTitle = t("Click to show the interface and egress IP");
  if (direct.shown) {
    const egress = direct.data;
    if (egress === "loading") internetSub = t("Looking up…");
    else if (egress === "failed") internetSub = t("Failed");
    else {
      const port = egress.service || egress.interface;
      const domestic = flagged(egress.domesticIp, egress.domesticLoc);
      const overseas = flagged(egress.ip, egress.loc);
      // a different overseas address: something upstream proxies it
      const split = !!egress.domesticIp && !!egress.ip && egress.domesticIp !== egress.ip;
      internetSub = split
        ? <>{port} · {domestic}<br />{t("Overseas")} {overseas}</>
        : `${port} · ${egress.domesticIp ? domestic : overseas}`;
      internetTitle = [
        `${port} (${egress.interface})`,
        egress.domesticIp && `${t("Domestic")} ${domestic}`,
        egress.ip && `${t("Overseas")} ${overseas}`,
        split && t("Overseas traffic leaves elsewhere: something upstream, such as the router, proxies it"),
      ].filter(Boolean).join("\n");
    }
  }

  let dnsSub: ReactNode = c.dnsVia === "system" ? t("System resolver") : c.dnsVia ? ["mihomo", c.dnsMode].filter(Boolean).join(" · ") : undefined;
  let dnsTitle = t("Click to show the upstream and egress IP");
  if (dns.shown) {
    const e = dns.data;
    if (e === "loading") dnsSub = t("Looking up…");
    else if (e === "failed") dnsSub = t("Failed");
    else {
      const servers = e.servers ?? [];
      const upstream = e.via === "system" ? t("System resolver")
        : servers.length ? shortServer(servers[0]) + (servers.length > 1 ? ` +${servers.length - 1}` : "") : "mihomo";
      dnsSub = <>{upstream}<br />{t("Egress")} {flagged(e.ip, e.loc)}</>;
      dnsTitle = [
        e.via === "system" ? t("System resolver") : `mihomo${e.mode ? ` (${e.mode})` : ""}`,
        ...servers,
        e.ip && `${t("Egress")} ${flagged(e.ip, e.loc)}`,
        e.ecs && `ECS ${e.ecs}`,
        t("The address authoritative servers see the queries come from"),
      ].filter(Boolean).join("\n");
    }
  }

  // the policy the rule named, then the node it picked
  let proxySub: ReactNode = c.chain?.length ? route(c.chain) : c.via;
  let proxyTitle = [c.chain?.length && [...c.chain].reverse().join(" → "), t("Click to show the egress IP")].filter(Boolean).join("\n");
  if (proxy.shown) {
    const e = proxy.data;
    if (e === "loading") proxySub = t("Looking up…");
    else if (e === "failed") proxySub = t("Failed");
    else {
      const chain = e.chain ?? [];
      proxySub = <>{route(chain)}<br />{t("Egress")} {flagged(e.ip, e.loc)}</>;
      proxyTitle = [
        [...chain].reverse().join(" → "),
        e.ip && `${t("Egress")} ${flagged(e.ip, e.loc)}`,
        // the trace is a site of its own, and the rules may send it elsewhere
        c.via && chain.length && chain[chain.length - 1] !== c.via && t("Looked up through {p}, not {q} that the test URL takes", { p: chain[chain.length - 1], q: c.via }),
      ].filter(Boolean).join("\n");
    }
  }

  const cards: { key: Item; label: string; sub?: ReactNode; title?: string; onClick?: () => void; shown?: boolean }[] = [
    { key: "router", label: t("Router"), sub: c.gateway },
    { key: "dns", label: "DNS", sub: dnsSub, title: dnsTitle, onClick: dns.toggle, shown: dns.shown },
    { key: "internet", label: t("Internet"), sub: internetSub, title: internetTitle, onClick: direct.toggle, shown: direct.shown },
    { key: "proxy", label: t("Proxy"), sub: proxySub, title: proxyTitle, onClick: proxy.toggle, shown: proxy.shown },
  ];
  return (
    <>
      <div className="section-title section-head">
        {t("Network")}
        <button className={"icon" + (busy ? " spin" : "")} title={t("Test")} disabled={!running} onClick={measure}><Refresh size={13} /></button>
      </div>
      <div className="conn-cards">
        {cards.map(({ key, label, sub, title, onClick, shown }) => {
          // until this page's first measure lands, the last one made
          const h = latency[key];
          const v = c[key] ?? (running && h?.length ? h[h.length - 1].ms : 0);
          const testing = pending.has(key);
          return (
            <div className={"card conn-card" + (onClick ? " toggles" : "")} key={key} title={title} onClick={onClick}>
              <div className="lbl">
                <span className={"cdot " + (testing && !v ? "testing" : delayClass(v))} />
                {label}
              </div>
              <div className="conn-row">
                <button
                  className={"val " + delayClass(v) + (testing ? " testing" : "")}
                  title={t("Click to test again")}
                  disabled={!running || testing}
                  onClick={(e) => { e.stopPropagation(); measureOne(key); }}
                >
                  {v > 0 ? <>{fmtMS(v)}<small> ms</small></> : v < 0 ? t("Failed") : "—"}
                </button>
                <LatencyBars data={latency[key] ?? []} floor={floors[key]} fmt={fmtMS} />
              </div>
              {sub && <div className="sub" key={onClick ? String(shown) : undefined}>{sub}</div>}
            </div>
          );
        })}
      </div>
    </>
  );
}
