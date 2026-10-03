import { useEffect, useRef, useState } from "react";
import { App, type Connectivity } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { delayClass } from "../format";
import { Refresh } from "./Icons";

type Item = "router" | "dns" | "internet" | "proxy";
const items: Item[] = ["router", "dns", "internet", "proxy"];
const fields: Record<Item, (keyof Connectivity)[]> = {
  router: ["router", "gateway"],
  dns: ["dns", "dnsVia"],
  internet: ["internet"],
  proxy: ["proxy", "via"],
};

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

  // each item lands on its own, so a slow proxy doesn't hold the router back
  const measure = () => {
    const n = ++seq.current;
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

  const cards: { key: Item; label: string; sub?: string }[] = [
    { key: "router", label: t("Router"), sub: c.gateway },
    { key: "dns", label: "DNS", sub: c.dnsVia === "system" ? t("System resolver") : c.dnsVia ? "mihomo" : undefined },
    { key: "internet", label: t("Internet") },
    { key: "proxy", label: t("Proxy"), sub: c.via },
  ];
  return (
    <div className="conn-cards">
      {cards.map(({ key, label, sub }) => {
        const v = c[key] ?? 0;
        return (
          <div className="card conn-card" key={key}>
            <div className="lbl">
              <span className={"cdot " + (pending.has(key) && !v ? "testing" : delayClass(v))} />
              {label}
            </div>
            <div className={"val " + delayClass(v)}>{v > 0 ? <>{v}<small> ms</small></> : v < 0 ? t("Failed") : "—"}</div>
            {sub && <div className="sub">{sub}</div>}
          </div>
        );
      })}
      <button className={"icon conn-refresh" + (busy ? " spin" : "")} title={t("Test")} disabled={!running} onClick={measure}><Refresh size={14} /></button>
    </div>
  );
}
