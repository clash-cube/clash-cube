import { useRef, useState } from "react";
import { Router } from "../api";
import type { CoreUpdate, RouterMaintenance as Maintenance } from "../routerTypes";
import { bytes } from "../format";
import { usePoll } from "../usePoll";
import { useT } from "../i18n";
import { Fold } from "./Fold";
import { toastError } from "./Toast";

export function RouterMaintenance() {
  const t = useT();
  const [maintenance, setMaintenance] = useState<Maintenance | null>(null);
  const [update, setUpdate] = useState<CoreUpdate | null>(null);
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState(false);
  const [feed, setFeed] = useState("");
  const [interval, setInterval] = useState(0);
  const revision = useRef(0);
  const load = async () => {
    if (!Router || busy) return;
    const request = ++revision.current;
    const [m, u] = await Promise.all([Router.Maintenance(), Router.Updates()]);
    if (request === revision.current) { setMaintenance(m); setUpdate(u); }
  };
  usePoll(() => { void load().catch(toastError); }, 3000, [busy]);
  const active = ["queued", "downloading", "installing"].includes(update?.job.phase ?? "");
  const act = async (operation: () => Promise<CoreUpdate>) => {
    revision.current++;
    setBusy(true);
    try { setUpdate(await operation()); } catch (error) { toastError(error); }
    finally { setBusy(false); }
  };
  const save = async () => {
    if (!Router) return;
    revision.current++;
    setBusy(true);
    try { setMaintenance(await Router.SetMaintenance({ geoIntervalHours: interval, updateFeed: feed.trim() })); setEditing(false); }
    catch (error) { toastError(error); }
    finally { setBusy(false); }
  };
  const phases: Record<string, string> = { queued: "Update queued", downloading: "Downloading core package…", installing: "Installing core package…", succeeded: "Core package updated", failed: "Core update failed" };
  return <>
    <div className="section-title">{t("Router maintenance")}</div>
    <div className="list">
      <div className="row">
        <div className="who"><div className="name">{t("Scheduled GEO updates")}</div>
          <div className="sub wrap">{maintenance?.config.geoIntervalHours ? t("Every {n} hours, including when management is offline", { n: maintenance.config.geoIntervalHours }) : t("Off")}</div>
          {maintenance?.geo.failed && <div className="sub warn wrap">{t("Last scheduled GEO update failed")}</div>}
        </div>
        <div className="end"><button className="btn small" disabled={!maintenance || editing || busy} onClick={() => {
          setFeed(maintenance!.config.updateFeed); setInterval(maintenance!.config.geoIntervalHours); setEditing(true);
        }}>{t("Configure")}</button></div>
      </div>
      <Fold open={editing}><div className="router-editor stagger" style={{ "--i": 0 } as React.CSSProperties}>
        <label>{t("GEO update interval")}
          <select className="input" value={interval} disabled={busy} onChange={(e) => setInterval(Number(e.target.value))}>
            {[0, 6, 12, 24, 48, 168].map((n) => <option key={n} value={n}>{n ? t("Every {n} hours", { n }) : t("Off")}</option>)}
          </select>
        </label>
        <label>{t("Signed core update feed")}
          <input className="input" type="url" value={feed} disabled={busy} onChange={(e) => setFeed(e.target.value)} />
        </label>
        <div className="end"><button className="btn small" disabled={busy} onClick={() => setEditing(false)}>{t("Cancel")}</button>
          <button className="btn small primary" disabled={busy} onClick={save}>{t(busy ? "Saving…" : "Save")}</button></div>
      </div></Fold>
      <div className="row">
        <div className="who"><div className="name">{t("Core package")}</div>
          <div className="sub wrap">{update?.installed || "—"}{update?.available && ` → ${update.latest} (${bytes(update.bytes ?? 0)})`}</div>
          <div className="sub wrap">{t(maintenance?.config.updateFeed ? "Only the core is updated. Management stays running." : "Configure a signed update feed to check for core updates.")}</div>
          {update?.job.phase !== "idle" && update?.job.phase && <div className={"sub wrap" + (update.job.phase === "failed" ? " warn" : "")}>{t(phases[update.job.phase] ?? update.job.phase)}</div>}
          {update?.job.error && <div className="sub warn wrap">{t(update.job.error)}</div>}
          {update?.available && !active && <div className="sub wrap">{t("Updating the core closes existing proxy connections.")}</div>}
        </div>
        <div className="end">
          {update?.available && !active ? <button className="btn small primary" disabled={busy} onClick={() => void act(() => Router!.UpdateCore())}>{t("Update core package")}</button>
            : <button className="btn small" disabled={busy || active || !maintenance?.config.updateFeed} onClick={() => void act(() => Router!.CheckUpdates())}>{t(busy ? "Checking for updates…" : "Check for Updates")}</button>}
        </div>
      </div>
    </div>
  </>;
}
