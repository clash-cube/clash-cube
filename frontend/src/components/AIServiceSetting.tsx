import { useEffect, useState } from "react";
import { App, type AIRoute } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { AIServiceName } from "./AIServiceName";
import { AIRouteDetails } from "./AIChecks";
import { Chevron, Refresh } from "./Icons";
import { Fold } from "./Fold";
import { Switch } from "./Switch";

export function AIServiceSetting({ service, on, disabled, onChange }: {
  service: string; on: boolean; disabled: boolean; onChange: (on: boolean) => Promise<unknown>;
}) {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const mode = useStore((s) => s.state?.mode);
  const profile = useStore((s) => s.state?.profile);
  const reset = useStore((s) => s.networkReset);
  const [open, setOpen] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [result, setResult] = useState<AIRoute | "loading" | "failed" | null>(null);

  useEffect(() => {
    let current = true;
    setResult(null);
    // Opening a service explicitly requests its routes, independently of
    // the switches that control automatic Overview checks.
    if (open && running) {
      setResult("loading");
      App.AIRoutes(service)
        .then((r) => { if (current) setResult(r); })
        .catch(() => { if (current) setResult("failed"); });
    }
    return () => { current = false; };
  }, [open, running, service, mode, profile, reset, refresh]);

  return (
    <div className="ai-service">
      <div className="row">
        <button className="ai-service-disclosure" aria-expanded={open} aria-controls={`ai-details-${service}`} onClick={() => setOpen(!open)}>
          <Chevron className={"chev" + (open ? " open" : "")} />
          <AIServiceName service={service} />
        </button>
        <div className="end"><Switch label={t(service)} on={on} disabled={disabled} onChange={onChange} /></div>
      </div>
      <Fold open={open}>
        <div id={`ai-details-${service}`}>
          <div className="ai-details-status">
            <span className="sub">{!running ? t("The core isn't running") : result === "failed" ? t("Failed") : result === "loading" ? t("Checking routes…") : t("Detected routes")}</span>
            <button className={"icon" + (result === "loading" ? " spin" : "")} title={t("Refresh routes")} disabled={!running || result === "loading"} onClick={() => setRefresh((n) => n + 1)}><Refresh size={13} /></button>
          </div>
          {running && result && typeof result === "object" && <AIRouteDetails result={result} />}
          <div className="ai-hosts"><div className="note">{t("Routes are checked from ClashCube; PROCESS-NAME rules may differ for the actual app.")}</div></div>
        </div>
      </Fold>
    </div>
  );
}
