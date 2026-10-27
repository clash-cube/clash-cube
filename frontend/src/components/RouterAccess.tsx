import { useState } from "react";
import { Router } from "../api";
import type { AccessPolicy, RouterState } from "../routerTypes";
import { useT } from "../i18n";
import { Fold } from "./Fold";
import { Segmented } from "./Segmented";
import { toastError } from "./Toast";

export function RouterAccess({ state, onChange }: { state: RouterState; onChange: (state: RouterState) => void }) {
  const t = useT();
  const policy = state.access ?? { mode: "all", addresses: [], macs: [] };
  const [editing, setEditing] = useState(false);
  const [mode, setMode] = useState<AccessPolicy["mode"]>("all");
  const [addresses, setAddresses] = useState("");
  const [macs, setMACs] = useState("");
  const [busy, setBusy] = useState(false);
  const modes = [
    { value: "all" as const, label: t("All devices") },
    { value: "exclude" as const, label: t("Bypass listed devices") },
    { value: "include" as const, label: t("Only listed devices") },
  ];
  const edit = () => {
    setMode(policy.mode); setAddresses(policy.addresses.join("\n")); setMACs(policy.macs.join("\n")); setEditing(true);
  };
  const save = async () => {
    if (!Router) return;
    const lines = (text: string) => text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    setBusy(true);
    try {
      onChange(await Router.SetAccess({ mode, addresses: lines(addresses), macs: lines(macs) }));
      setEditing(false);
    } catch (error) { toastError(error); }
    finally { setBusy(false); }
  };
  return <>
    <div className="row">
      <div className="who"><div className="name">{t("Device access")}</div>
        <div className="sub wrap">{modes.find((m) => m.value === policy.mode)?.label}</div>
      </div>
      <div className="end"><button className="btn small" disabled={editing} onClick={edit}>{t("Edit")}</button></div>
    </div>
    <Fold open={editing}><div className="router-editor">
      <div className="stagger" style={{ "--i": 0 } as React.CSSProperties}>
        <Segmented options={modes} value={mode} onChange={setMode} />
      </div>
      <div className="sub wrap stagger" style={{ "--i": 1 } as React.CSSProperties}>
        {t("Applies to transparent TCP and forced DNS. Explicit proxy access and the router's shared DNS are unchanged.")}
      </div>
      <label className="stagger" style={{ "--i": 2 } as React.CSSProperties}>{t("IPv4 addresses or subnets, one per line")}
        <textarea className="input" rows={3} value={addresses} onChange={(e) => setAddresses(e.target.value)} spellCheck={false} disabled={busy} />
      </label>
      <label className="stagger" style={{ "--i": 3 } as React.CSSProperties}>{t("MAC addresses, one per line")}
        <textarea className="input" rows={3} value={macs} onChange={(e) => setMACs(e.target.value)} spellCheck={false} disabled={busy} />
      </label>
      <div className="end stagger" style={{ "--i": 4 } as React.CSSProperties}>
        <button className="btn small" disabled={busy} onClick={() => setEditing(false)}>{t("Cancel")}</button>
        <button className="btn small primary" disabled={busy} onClick={save}>{t(busy ? "Saving…" : "Save")}</button>
      </div>
    </div></Fold>
  </>;
}
