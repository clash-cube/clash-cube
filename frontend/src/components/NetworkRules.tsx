import { useEffect, useMemo, useRef, useState } from "react";
import { App, Settings, type SettingsT, type NetworkRule, type NetworkActions, type Profile } from "../api";
import { useStore } from "../store";
import { useT } from "../i18n";
import { useGroups } from "../useGroups";
import { Switch } from "./Switch";
import { Segmented } from "./Segmented";
import { Popover } from "./Popover";
import { Close, Plus } from "./Icons";
import { toast, toastError, errText } from "./Toast";

type T = ReturnType<typeof useT>;

// The rule's key, as the backend's Network.match names it.
const keyOf = (r: NetworkRule) => (r.match === "ssid" ? "ssid:" + r.ssid : r.match);

const FIELDS = ["profile", "mode", "systemProxy", "tun"] as const;
type Field = (typeof FIELDS)[number];
const isSet = (a: NetworkActions, f: Field) => (f === "profile" || f === "mode" ? !!a[f] : a[f] != null);

function ruleName(r: NetworkRule, t: T) {
  return r.match === "ssid" ? r.ssid! : r.match === "wired" ? t("Wired network") : t("Other networks");
}

// The name of the rule a Network.match names.
export function matchName(match: string, t: T) {
  return match.startsWith("ssid:") ? match.slice(5) : match === "wired" ? t("Wired network") : t("Other networks");
}

const MODES: Record<string, string> = { rule: "Rule", global: "Global", direct: "Direct" };

function summary(a: NetworkActions, profiles: Profile[], t: T) {
  const parts: string[] = [];
  if (a.profile) parts.push(profiles.find((p) => p.id === a.profile)?.name ?? t("Profile no longer exists"));
  if (a.mode) parts.push(t(MODES[a.mode] ?? a.mode));
  if (a.systemProxy != null) parts.push(t(a.systemProxy ? "System proxy on" : "System proxy off"));
  if (a.tun != null) parts.push(t(a.tun ? "TUN on" : "TUN off"));
  for (const [g, m] of Object.entries(a.groups ?? {})) parts.push(`${g} → ${m}`);
  return parts.length ? parts.join(" · ") : t("Changes nothing");
}

function fieldName(f: string, t: T) {
  if (f.startsWith("group:")) return f.slice(6);
  return t(({ profile: "Profile", mode: "Outbound mode", systemProxy: "System proxy", tun: "Enhanced Mode" } as Record<string, string>)[f] ?? f);
}

export function NetworkRules({ settings }: { settings: SettingsT }) {
  const t = useT();
  const profiles = useStore((s) => s.profiles);
  const net = useStore((s) => s.state?.network);
  const rules = settings.networkRules?.length ? settings.networkRules : [{ match: "other", actions: {} }];
  const [editing, setEditing] = useState<{ anchor: HTMLElement; rule?: NetworkRule; ssid?: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const act = async <R,>(p: Promise<R>) => {
    setBusy(true);
    try { return await p; } catch (e) { toastError(e); } finally { setBusy(false); }
  };
  const setAuto = async (on: boolean) => {
    useStore.setState({ settings: { ...settings, networkAuto: on } });
    const s = await act(Settings.SetNetworkAuto(on));
    useStore.setState({ settings: s ?? await Settings.Get() });
  };
  const remove = async (r: NetworkRule) => {
    const s = await act(Settings.SetNetworkRules(rules.filter((x) => x !== r)));
    if (s) useStore.setState({ settings: s });
  };

  const wifi = net?.wifi;
  const ssid = wifi?.state === "connected" ? wifi.ssid : "";
  const here = net?.kind === "wired" ? t("Wired network") : ssid || (net?.kind === "wifi" ? t("Wi-Fi") : t("Not connected"));
  const hereSub = !net?.kind ? "" :
    wifi?.state === "permission" ? t("Allow location access to read the Wi-Fi name") :
    wifi?.state === "denied" ? t("Location access is disabled, so Wi-Fi rules can't match") :
    wifi?.state === "unavailable" && net.kind === "wifi" ? t("Wi-Fi name unavailable") : "";
  const current = settings.networkAuto ? net?.match ?? "" : "";
  const currentRule = rules.find((r) => keyOf(r) === current);
  const manual = net?.manual ?? [];
  const canAddHere = !!ssid && !rules.some((r) => r.match === "ssid" && r.ssid === ssid);

  return <>
    <div className="section-title">{t("Network rules")}</div>
    <div className="list network-rules">
      <div className="row">
        <div className="who">
          <div className="name">{t("Apply rules by network")}</div>
          <div className="sub wrap">{t("Set the profile, mode, system proxy and groups for each network. Leaving a network returns to the other networks' rule; a change made by hand lasts until the network changes.")}</div>
        </div>
        <Switch on={settings.networkAuto} onChange={(v) => { if (!busy) setAuto(v); }} />
      </div>
      <div className="row">
        <div className="who">
          <div className="name">{t("Current network: {name}", { name: here })}{currentRule && ruleName(currentRule, t) !== here && <span className="badge net">{ruleName(currentRule, t)}</span>}</div>
          {manual.length > 0 && <div className="sub warn wrap">{t("Changed by hand: {fields}", { fields: manual.map((f) => fieldName(f, t)).join(t(", ")) })}</div>}
          {hereSub && <div className="sub wrap">{hereSub}</div>}
        </div>
        <div className="end">
          {manual.length > 0 && <button className="btn small" disabled={busy} onClick={() => act(Settings.ResumeNetworkAuto())}>{t("Resume rule")}</button>}
          {wifi?.state === "permission" && <button className="btn small" onClick={() => Settings.RequestWiFiPermission().catch(toastError)}>{t("Allow access…")}</button>}
          {wifi?.state === "denied" && <button className="btn small" onClick={() => App.OpenURL("x-apple.systempreferences:com.apple.preference.security?Privacy_LocationServices").catch(toastError)}>{t("Open Location Settings…")}</button>}
          {canAddHere && <button className="btn small" onClick={(e) => setEditing({ anchor: e.currentTarget, ssid })}>{t("Add a rule for this network…")}</button>}
        </div>
      </div>
      {rules.map((r) => (
        <div key={keyOf(r)} className={"row click network-rule" + (settings.networkAuto ? "" : " off")} onClick={(e) => setEditing({ anchor: e.currentTarget, rule: r })}>
          <div className="who">
            <div className="name">{ruleName(r, t)}{keyOf(r) === current && <span className="badge net">{t("In effect")}</span>}</div>
            <div className="sub">{summary(r.actions, profiles, t)}</div>
          </div>
          {r.match !== "other" && <div className="end" onClick={(e) => e.stopPropagation()}>
            <button className="icon" title={t("Remove")} disabled={busy} onClick={() => remove(r)}><Close size={12} /></button>
          </div>}
        </div>
      ))}
      <div className="row">
        <button className="btn small" onClick={(e) => setEditing({ anchor: e.currentTarget })}><Plus size={12} />{t("Add network…")}</button>
      </div>
    </div>
    <RuleEditor key={editing ? keyOf(editing.rule ?? { match: "new", actions: {} }) + (editing.ssid ?? "") : "closed"}
      open={editing} rules={rules} onClose={() => setEditing(null)} />
  </>;
}

const SSID_MAX = 32;

function RuleEditor({ open, rules, onClose }: {
  open: { anchor: HTMLElement; rule?: NetworkRule; ssid?: string } | null;
  rules: NetworkRule[];
  onClose: () => void;
}) {
  const t = useT();
  const profiles = useStore((s) => s.profiles);
  const { groups } = useGroups();
  const initial = open?.rule;
  const [rule, setRule] = useState<NetworkRule>(() => initial
    ? { ...initial, actions: { ...initial.actions, groups: { ...initial.actions.groups } } }
    : { match: "ssid", ssid: open?.ssid ?? "", actions: {} });
  const [saved, setSaved] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const a = rule.actions;
  const set = (p: Partial<NetworkActions>) => setRule((r) => ({ ...r, actions: { ...r.actions, ...p } }));
  const others = rules.filter((r) => r !== initial);
  const isOther = rule.match === "other";
  const currentSSID = useStore((s) => s.state?.network.wifi.state === "connected" ? s.state.network.wifi.ssid : "");

  // the other networks' rule must set whatever another rule sets
  const required = useMemo(() => {
    const req = new Set<string>();
    if (!isOther) return req;
    for (const r of others) {
      for (const f of FIELDS) if (isSet(r.actions, f)) req.add(f);
      for (const g of Object.keys(r.actions.groups ?? {})) req.add("group:" + g);
    }
    return req;
  }, [isOther, rules]);

  useEffect(() => {
    if (!open || rule.match !== "ssid") return;
    setTimeout(() => input.current?.focus(), 60);
    Settings.SavedWiFiNetworks().then((n) => setSaved(n ?? [])).catch(() => setSaved([]));
  }, [open, rule.match]);

  const ssidErr = rule.match !== "ssid" ? "" :
    !rule.ssid ? " " :
    new TextEncoder().encode(rule.ssid).length > SSID_MAX ? t("Wi-Fi names must contain 1–32 UTF-8 bytes") :
    others.some((r) => r.match === "ssid" && r.ssid === rule.ssid) ? t("This network already has a rule") : "";
  const used = new Set(rules.filter((r) => r.match === "ssid").map((r) => r.ssid));
  const suggestions = [...new Set([currentSSID, ...saved].filter((s): s is string => !!s && !used.has(s)))].slice(0, 12);
  const selectors = (groups ?? []).filter((g) => g.type === "Selector" && g.name !== "GLOBAL");
  const chosen = Object.entries(a.groups ?? {});

  const submit = async () => {
    if (busy || ssidErr) return;
    setBusy(true);
    const next = initial ? rules.map((r) => (r === initial ? rule : r)) : [...rules, rule];
    try {
      useStore.setState({ settings: await Settings.SetNetworkRules(next) });
      toast(t("Saved"));
      onClose();
    } catch (e) { toast(t(errText(e)), "err", 4000); }
    setBusy(false);
  };

  const tri = (v: boolean | null | undefined) => (v == null ? "" : v ? "on" : "off");
  const fromTri = (v: string) => (v === "" ? null : v === "on");
  const keep = (f: string) => !required.has(f) && <option value="">{t("Don't change")}</option>;

  return (
    <Popover anchor={open?.anchor ?? null} open={!!open} onClose={() => { if (!busy) onClose(); }} align="end" width={380}>
      <form className="pop-form network-rule-form" onSubmit={(e) => { e.preventDefault(); submit(); }}>
        <h3>{initial ? t("Edit {network}", { network: ruleName(initial, t) }) : t("Add network")}</h3>
        {!initial && !rules.some((r) => r.match === "wired") && (
          <Segmented className="track small" value={rule.match} onChange={(m) => setRule((r) => ({ ...r, match: m }))}
            options={[{ value: "ssid", label: t("Wi-Fi") }, { value: "wired", label: t("Wired network") }]} />
        )}
        {rule.match === "ssid" && <>
          <label>{t("Wi-Fi name (SSID)")}
            <input ref={input} className="input" value={rule.ssid ?? ""} spellCheck={false} onChange={(e) => setRule({ ...rule, ssid: e.target.value })} />
          </label>
          {ssidErr.trim() && <div className="hint err">{ssidErr}</div>}
          {suggestions.length > 0 && <div className="rule-choices">
            {suggestions.map((s) => (
              <button type="button" key={s} className={"chip" + (s === rule.ssid ? " on" : "")} onClick={() => setRule({ ...rule, ssid: s })}>
                {s}{s === currentSSID && <span className="rtype">{t("Current")}</span>}
              </button>
            ))}
          </div>}
          <div className="hint">{t("Matches the name exactly, case and spaces included.")}</div>
        </>}
        {rule.match === "wired" && <div className="hint">{t("Any network whose main connection isn't Wi-Fi: Ethernet, a USB or Thunderbolt adapter.")}</div>}
        {isOther && <div className="hint">{t("Every network without a rule of its own. What another rule sets, this one sets too, so leaving a network always returns here.")}</div>}

        <div className="network-actions">
          <label>{t("Profile")}
            <select className="input" value={a.profile ?? ""} onChange={(e) => set({ profile: e.target.value })}>
              {keep("profile")}
              {a.profile && !profiles.some((p) => p.id === a.profile) && <option value={a.profile}>{t("Profile no longer exists")}</option>}
              {profiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
          </label>
          <label>{t("Outbound mode")}
            <select className="input" value={a.mode ?? ""} onChange={(e) => set({ mode: e.target.value })}>
              {keep("mode")}
              {Object.entries(MODES).map(([v, l]) => <option key={v} value={v}>{t(l)}</option>)}
            </select>
          </label>
          <label>{t("System proxy")}
            <select className="input" value={tri(a.systemProxy)} onChange={(e) => set({ systemProxy: fromTri(e.target.value) })}>
              {keep("systemProxy")}
              <option value="on">{t("On")}</option>
              <option value="off">{t("Off")}</option>
            </select>
          </label>
          <label>{t("Enhanced Mode")}
            <select className="input" value={tri(a.tun)} onChange={(e) => set({ tun: fromTri(e.target.value) })}>
              {keep("tun")}
              <option value="on">{t("On")}</option>
              <option value="off">{t("Off")}</option>
            </select>
          </label>
        </div>
        {a.tun && <div className="hint">{t("Turns on only with the privileged helper installed; it never asks for a password.")}</div>}

        <label>{t("Proxy groups")}</label>
        {chosen.map(([g, m]) => {
          const members = (selectors.find((x) => x.name === g)?.members ?? []).map((x) => x.name);
          return (
            <div key={g} className="network-group">
              <span className="gname">{g}</span>
              <select className="input" value={m} onChange={(e) => set({ groups: { ...a.groups, [g]: e.target.value } })}>
                {!members.includes(m!) && <option value={m}>{m}</option>}
                {members.map((x) => <option key={x} value={x}>{x}</option>)}
              </select>
              <button type="button" className="icon" title={t("Remove")} disabled={required.has("group:" + g)}
                onClick={() => { const gs = { ...a.groups }; delete gs[g]; set({ groups: gs }); }}><Close size={12} /></button>
            </div>
          );
        })}
        {groups === null ? <div className="hint">{t("Start the core to choose proxy groups.")}</div> : (
          <select className="input" value="" onChange={(e) => {
            const g = selectors.find((x) => x.name === e.target.value);
            if (g) set({ groups: { ...a.groups, [g.name]: g.now || g.members?.[0]?.name } });
          }}>
            <option value="">{t("Add a group…")}</option>
            {selectors.filter((g) => !(g.name in (a.groups ?? {}))).map((g) => <option key={g.name} value={g.name}>{g.name}</option>)}
          </select>
        )}
        {required.size > 0 && <div className="hint">{t("Options other rules set can't be left unchanged here.")}</div>}

        <div className="foot">
          <button type="button" className="btn" onClick={onClose}>{t("Cancel")}</button>
          <button type="submit" className="btn primary" disabled={busy || !!ssidErr}>{t(initial ? "Save" : "Add")}</button>
        </div>
      </form>
    </Popover>
  );
}
