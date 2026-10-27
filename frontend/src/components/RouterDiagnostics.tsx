import { useState } from "react";
import { Router } from "../api";
import type { RouterDiagnostics as Diagnostics } from "../routerTypes";
import { bytes } from "../format";
import { useT } from "../i18n";
import { Fold } from "./Fold";
import { toastError } from "./Toast";

export function RouterDiagnostics() {
  const t = useT();
  const [report, setReport] = useState<Diagnostics | null>(null);
  const [busy, setBusy] = useState(false);
  const load = async () => {
    if (!Router) return;
    setBusy(true);
    try { setReport(await Router.Diagnostics()); } catch (error) { toastError(error); }
    finally { setBusy(false); }
  };
  const download = () => {
    const url = URL.createObjectURL(new Blob([JSON.stringify(report, null, 2)], { type: "application/json" }));
    const link = document.createElement("a");
    link.href = url; link.download = "clashcube-diagnostics.json"; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return <>
    <div className="section-title">{t("Router diagnostics")}</div>
    <div className="list">
      <div className="row"><div className="who"><div className="name">{t("System and storage")}</div>
        <div className="sub wrap">{t("Checks package versions, storage, memory and DNS services.")}</div></div>
        <div className="end">{report && <button className="btn small" onClick={download}>{t("Download report")}</button>}
          <button className="btn small" disabled={busy} onClick={load}>{t(busy ? "Checking…" : "Check")}</button></div>
      </div>
      <Fold open={!!report}>{report && <div className="router-editor stagger" style={{ "--i": 0 } as React.CSSProperties}>
        <div className="sub wrap">{report.system} · {report.kernel}</div>
        <div className="sub wrap">{t("Available memory: {free} of {total}", { free: bytes(report.memoryFree), total: bytes(report.memoryTotal) })}</div>
        {report.storage.map((s, index) => <div key={index} className="sub wrap mono">{s.path}<br />{t("Free: {free} of {total}", { free: bytes(s.free), total: bytes(s.total) })}</div>)}
        {report.packages.map((p) => <div key={p.name} className="sub wrap mono">{p.name} {p.version}</div>)}
        {report.checks.map((c) => <div key={c.name} className="sub wrap"><span className={"cdot " + (c.ok ? "good" : "none")} />{t(c.name)} · {t(c.ok ? "OK" : "Unavailable")}</div>)}
      </div>}</Fold>
    </div>
  </>;
}
