import { useEffect, useRef, useState, type ReactNode } from "react";
import { App, type Connectivity, type DNSEgress, type Egress } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { delayClass } from "../format";
import { Refresh } from "./Icons";

type Item = "router" | "dns" | "internet" | "proxy";
const items: Item[] = ["router", "dns", "internet", "proxy"];
const fields: Record<Item, (keyof Connectivity)[]> = {
  router: ["router", "gateway"],
  dns: ["dns", "dnsVia", "dnsMode"],
  internet: ["internet"],
  proxy: ["proxy", "via"],
};

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
  const [c, setC] = useState<Partial<Connectivity>>({});
  const [pending, setPending] = useState<Set<Item>>(new Set());
  const busy = pending.size > 0;
  const seq = useRef(0);
  // the Internet card shows DIRECT, or on a click where direct traffic
  // leaves: the interface and the public address
  const direct = useDetail<Egress>(App.DirectEgress);
  // the DNS card shows the resolver, or on a click its upstream and the
  // address authoritative servers see the queries from
  const dns = useDetail<DNSEgress>(App.DNSEgress);

  // each item lands on its own, so a slow proxy doesn't hold the router back
  const measure = () => {
    const n = ++seq.current;
    direct.refresh();
    dns.refresh();
    setPending(new Set(items));
    for (const key of items) {
      App.ConnectivityItem(key).then((r) => {
        if (n !== seq.current) return;
        setC((prev) => {
          const next = { ...prev };
          for (const f of fields[key]) (next as any)[f] = r[f];
          return next;
        });
      }).catch(() => { /* the core stopped; the card keeps a dash */ }).finally(() => {
        if (n !== seq.current) return;
        setPending((p) => { const s = new Set(p); s.delete(key); return s; });
      });
    }
  };

  useEffect(() => {
    if (!running) { seq.current++; setC({}); setPending(new Set()); return; }
    measure();
    const id = setInterval(() => { if (!document.hidden) measure(); }, 30000);
    return () => clearInterval(id);
  }, [running, mode, profile]);

  let internetSub: ReactNode = "DIRECT";
  let internetTitle = t("Click to show the interface and egress IP");
  if (direct.shown) {
    const egress = direct.data;
    if (egress === "loading") internetSub = t("Looking up…");
    else if (egress === "failed") internetSub = t("Failed");
    else {
      const port = egress.service || egress.interface;
      const overseas = egress.ip + (egress.loc ? " · " + egress.loc : "");
      // a different overseas address: something upstream proxies it
      const split = !!egress.domesticIp && !!egress.ip && egress.domesticIp !== egress.ip;
      internetSub = split
        ? <>{port} · {egress.domesticIp}<br />{t("Overseas")} {overseas}</>
        : `${port} · ${egress.domesticIp || overseas || "—"}`;
      internetTitle = [
        `${port} (${egress.interface})`,
        egress.domesticIp && `${t("Domestic")} ${egress.domesticIp}`,
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
      dnsSub = <>{upstream}<br />{t("Egress")} {e.ip || "—"}</>;
      dnsTitle = [
        e.via === "system" ? t("System resolver") : `mihomo${e.mode ? ` (${e.mode})` : ""}`,
        ...servers,
        e.ip && `${t("Egress")} ${e.ip}`,
        e.ecs && `ECS ${e.ecs}`,
        t("The address authoritative servers see the queries come from"),
      ].filter(Boolean).join("\n");
    }
  }

  const cards: { key: Item; label: string; sub?: ReactNode; title?: string; onClick?: () => void; shown?: boolean }[] = [
    { key: "router", label: t("Router"), sub: c.gateway },
    { key: "dns", label: "DNS", sub: dnsSub, title: dnsTitle, onClick: dns.toggle, shown: dns.shown },
    { key: "internet", label: t("Internet"), sub: internetSub, title: internetTitle, onClick: direct.toggle, shown: direct.shown },
    { key: "proxy", label: t("Proxy"), sub: c.via },
  ];
  return (
    <div className="conn-cards">
      {cards.map(({ key, label, sub, title, onClick, shown }) => {
        const v = c[key] ?? 0;
        return (
          <div className={"card conn-card" + (onClick ? " toggles" : "")} key={key} title={title} onClick={onClick}>
            <div className="lbl">
              <span className={"cdot " + (pending.has(key) && !v ? "testing" : delayClass(v))} />
              {label}
            </div>
            <div className={"val " + delayClass(v)}>{v > 0 ? <>{v}<small> ms</small></> : v < 0 ? t("Failed") : "—"}</div>
            {sub && <div className="sub" key={onClick ? String(shown) : undefined}>{sub}</div>}
          </div>
        );
      })}
      <button className={"icon conn-refresh" + (busy ? " spin" : "")} title={t("Test")} disabled={!running} onClick={measure}><Refresh size={14} /></button>
    </div>
  );
}
