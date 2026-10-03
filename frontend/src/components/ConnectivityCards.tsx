import { useEffect, useRef, useState } from "react";
import { App, type Connectivity } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { delayClass } from "../format";
import { Refresh } from "./Icons";

// The four latencies Surge's overview leads with: the router, DNS, the
// internet directly, and through the proxy. Measured on open, every 30s
// while shown, and on demand.
export function ConnectivityCards() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const mode = useStore((s) => s.state?.mode);
  const profile = useStore((s) => s.state?.profile);
  const [c, setC] = useState<Connectivity | null>(null);
  const [busy, setBusy] = useState(false);
  const seq = useRef(0);

  const measure = async () => {
    const n = ++seq.current;
    setBusy(true);
    try {
      const r = await App.Connectivity();
      if (n === seq.current) setC(r);
    } catch { /* the core stopped; the cards show dashes */ }
    if (n === seq.current) setBusy(false);
  };

  useEffect(() => {
    if (!running) { setC(null); return; }
    measure();
    const id = setInterval(() => { if (!document.hidden) measure(); }, 30000);
    return () => clearInterval(id);
  }, [running, mode, profile]);

  const cards: { key: keyof Connectivity; label: string; sub?: string }[] = [
    { key: "router", label: t("Router"), sub: c?.gateway },
    { key: "dns", label: "DNS", sub: c?.dnsVia === "system" ? t("System resolver") : c?.dnsVia ? "mihomo" : undefined },
    { key: "internet", label: t("Internet") },
    { key: "proxy", label: t("Proxy"), sub: c?.via },
  ];
  return (
    <div className="conn-cards">
      {cards.map(({ key, label, sub }) => {
        const v = c ? (c[key] as number) : 0;
        return (
          <div className="card conn-card" key={key}>
            <div className="lbl">
              <span className={"cdot " + (busy && !c ? "testing" : delayClass(v))} />
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
