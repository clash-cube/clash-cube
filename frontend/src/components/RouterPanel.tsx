import { useRef, useState } from "react";
import { Router } from "../api";
import type { RouterState } from "../routerTypes";
import { useStore } from "../store";
import { usePoll } from "../usePoll";
import { useT } from "../i18n";
import { Switch } from "./Switch";
import { toastError } from "./Toast";
import { RouterAccess } from "./RouterAccess";
import { RouterMaintenance } from "./RouterMaintenance";
import { RouterDiagnostics } from "./RouterDiagnostics";

export function RouterPanel() {
  const t = useT();
  const core = useStore((s) => s.state?.core);
  const [state, setState] = useState<RouterState | null>(null);
  const [busy, setBusy] = useState(false);
  const changing = useRef(false);
  const revision = useRef(0);
  const api = Router;
  const refresh = async () => {
    if (!api || changing.current) return;
    const request = ++revision.current;
    const next = await api.Get();
    if (revision.current === request) setState(next);
  };
  usePoll(() => { void refresh().catch(toastError); }, 3000, []);
  if (!api) return null;
  const toggle = async (on: boolean) => {
    changing.current = true;
    revision.current++;
    setBusy(true);
    try { setState(await api.SetEnabled(on)); }
    catch (error) { toastError(error); }
    finally {
      changing.current = false;
      setBusy(false);
      await refresh().catch(toastError);
    }
  };
  return <>
    <div className="section-title">{t("Router")}</div>
    <div className="list">
      <div className="row">
        <div className="who">
          <div className="name">{t("LAN proxy")}</div>
          <div className="sub wrap">{t("Route LAN TCP traffic and DNS through the selected profile.")}</div>
        </div>
        <div className="end"><Switch label={t("LAN proxy")} on={!!state?.enabled} busy={busy}
          disabled={!state || (!state.enabled && core !== "running")} onChange={toggle} /></div>
      </div>
      <div className="row">
        <div className="who">
          <div className="name">{t("Status")}</div>
          <div className="sub wrap">{t("Other UDP traffic and IPv6 are not intercepted.")}</div>
          {state?.error && <div className="sub warn wrap">{t(state.error)}</div>}
        </div>
        <div className="end"><span className={"cdot " + (state?.active ? "good" : "none")} />
          {t(state?.active ? "Active" : state?.enabled ? "Waiting for core" : "Off")}</div>
      </div>
      {state && <RouterAccess state={state} onChange={(next) => { revision.current++; setState(next); }} />}
    </div>
    <RouterMaintenance />
    <RouterDiagnostics />
  </>;
}
