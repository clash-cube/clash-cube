import { useEffect, useMemo, useRef, useState } from "react";
import { Proxy, type Connection, type UserRule } from "../api";
import { useT } from "../i18n";
import { Popover, type Point } from "./Popover";
import { toast, toastError } from "./Toast";
import { useGroups } from "../useGroups";
import { AppPicker, isProcessType, processPayload } from "./AppPicker";

// What a connection suggests a rule for: its domain, its address, or the app.
export function suggestions(c: Connection): UserRule[] {
  const m = c.metadata;
  const host = m.host || m.sniffHost;
  const out: UserRule[] = [];
  if (host) {
    out.push({ type: "DOMAIN", payload: host, policy: "" });
    const parts = host.split(".");
    // example.com from a.b.example.com; the last two labels is a guess,
    // which the field lets one correct (co.uk)
    if (parts.length > 2) out.push({ type: "DOMAIN-SUFFIX", payload: parts.slice(-2).join("."), policy: "" });
    else out.push({ type: "DOMAIN-SUFFIX", payload: host, policy: "" });
  }
  if (m.destinationIP) {
    const v6 = m.destinationIP.includes(":");
    out.push({ type: v6 ? "IP-CIDR6" : "IP-CIDR", payload: m.destinationIP + (v6 ? "/128" : "/32"), policy: "" });
  }
  if (m.process) out.push({ type: "PROCESS-NAME", payload: m.process, policy: "" });
  return out;
}

const BUILTIN = ["DIRECT", "REJECT"];

// A popover to add a rule ahead of the profile's, or with onSave to edit
// one: the type and value prefilled from what it was opened on, the policy
// picked from the groups, a process picked from the running programs.
export function RuleEditor({ anchor, point, onClose, initial, choices = [], onSave }: {
  anchor: HTMLElement | null;
  point?: Point | null;
  onClose: () => void;
  initial?: UserRule;
  choices?: UserRule[];
  onSave?: (r: UserRule) => Promise<void>;
}) {
  const t = useT();
  const { groups } = useGroups();
  const [types, setTypes] = useState<string[]>([]);
  const [rule, setRule] = useState<UserRule>({ type: "DOMAIN-SUFFIX", payload: "", policy: "" });
  const [busy, setBusy] = useState(false);
  const [picking, setPicking] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const policies = useMemo(() => [...(groups ?? []).filter((g) => g.name !== "GLOBAL").map((g) => g.name), ...BUILTIN], [groups]);

  useEffect(() => { Proxy.RuleTypes().then((r) => setTypes(r ?? [])).catch(() => {}); }, []);
  useEffect(() => {
    if (!anchor && !point) return;
    const start = initial ?? choices[0] ?? { type: "DOMAIN-SUFFIX", payload: "", policy: "" };
    setRule({ ...start, policy: start.policy || policies[0] || "DIRECT" });
    setPicking(false);
    setTimeout(() => input.current?.focus(), 60);
  }, [anchor, point]);
  useEffect(() => { if (!rule.policy && policies.length) setRule((r) => ({ ...r, policy: policies[0] })); }, [policies]);

  const submit = async () => {
    const r = { ...rule, payload: rule.payload.trim() };
    if (!r.payload || !r.policy) return;
    setBusy(true);
    try {
      if (onSave) await onSave(r);
      else {
        await Proxy.AddUserRule(r);
        toast(t("Rule added: {rule}", { rule: `${r.type},${r.payload},${r.policy}` }));
      }
      onClose();
    } catch (e) { toastError(e); }
    setBusy(false);
  };

  return (
    <Popover anchor={anchor} point={point} open={!!(anchor || point)} onClose={onClose} align="end" width={360}>
      <form className="pop-form" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <h3>{t(onSave ? "Edit rule" : "Add rule")}</h3>
        {choices.length > 1 && (
          <div className="rule-choices">
            {choices.map((c) => (
              <button type="button" key={c.type + c.payload} className={"chip" + (c.type === rule.type && c.payload === rule.payload ? " on" : "")}
                onClick={() => setRule((r) => ({ ...r, type: c.type, payload: c.payload }))}>
                <span className="rtype">{c.type}</span>{c.payload}
              </button>
            ))}
          </div>
        )}
        <label>{t("Type")}
          <select className="input" value={rule.type} onChange={(e) => setRule({ ...rule, type: e.target.value })}>
            {(types.length ? types : [rule.type]).map((ty) => <option key={ty} value={ty}>{ty}</option>)}
          </select>
        </label>
        <label>{t("Value")}
          <span className="value-row">
            <input ref={input} className="input mono" value={rule.payload} onChange={(e) => setRule({ ...rule, payload: e.target.value })} />
            {isProcessType(rule.type) && <button type="button" className={"btn" + (picking ? " on" : "")} aria-pressed={picking} onClick={() => setPicking(!picking)}>{t("Pick…")}</button>}
          </span>
        </label>
        {isProcessType(rule.type) && picking && <AppPicker onPick={(a) => { setRule({ ...rule, payload: processPayload(rule.type, a) }); setPicking(false); }} />}
        {rule.type === "PROCESS-NAME" && <div className="hint">{t("Matches the executable's name. Helpers of an app have their own names; PROCESS-PATH-REGEX with the app picked covers them all.")}</div>}
        <label>{t("Policy")}
          <select className="input" value={rule.policy} onChange={(e) => setRule({ ...rule, policy: e.target.value })}>
            {policies.includes(rule.policy) || !rule.policy ? null : <option value={rule.policy}>{rule.policy}</option>}
            {policies.map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </label>
        <div className="hint">{t("Goes ahead of the profile's rules and stays across profile updates.")}</div>
        <div className="foot">
          <button type="button" className="btn" onClick={onClose}>{t("Cancel")}</button>
          <button type="submit" className="btn primary" disabled={busy || !rule.payload.trim() || !rule.policy}>{t(onSave ? "Save" : "Add")}</button>
        </div>
      </form>
    </Popover>
  );
}
