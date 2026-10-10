import { useEffect, useRef, useState } from "react";
import { Window } from "@wailsio/runtime";
import { App, Proxy, type UserRule } from "./api";
import { useT } from "./i18n";
import { useStore } from "./store";
import { Segmented } from "./components/Segmented";
import { RuleFields, usePolicies } from "./components/RuleEditor";
import { toastError } from "./components/Toast";
import { Logo } from "./components/Icons";
import type { Webpage } from "../bindings/github.com/localhost-copilot/clashcube/internal/gui/models";

type Kind = "permanent" | "temporary";

// The window "Add Rule for Current Webpage" opens, after Surge's: the
// page's address, a rule for its site, and whether it is kept or lasts
// until the profile is switched or updated.
export function WebpageRule() {
  const t = useT();
  const running = useStore((s) => s.state?.core === "running");
  const policies = usePolicies();
  const [page, setPage] = useState<Webpage | null>(null);
  const [rule, setRule] = useState<UserRule>({ type: "DOMAIN-SUFFIX", payload: "", policy: "" });
  const [kind, setKind] = useState<Kind>("permanent");
  const [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    App.Webpage().then((p) => {
      setPage(p);
      const first = p.rules?.[0];
      if (first) setRule((r) => ({ ...r, type: first.type, payload: first.payload }));
      setTimeout(() => input.current?.select(), 60);
    }).catch(toastError);
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") Window.Close(); };
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, []);
  // the policy defaults to DIRECT, as Surge's does
  useEffect(() => {
    if (!rule.policy && policies.length) setRule((r) => ({ ...r, policy: policies.includes("DIRECT") ? "DIRECT" : policies[0] }));
  }, [policies]);

  const submit = async () => {
    const r = { ...rule, payload: rule.payload.trim() };
    if (!r.payload || !r.policy) return;
    setBusy(true);
    try {
      await (kind === "temporary" ? Proxy.AddTempRule(r) : Proxy.AddUserRule(r));
      Window.Close();
    } catch (e) { toastError(e); setBusy(false); }
  };

  if (page?.error) {
    const automation = page.error === "automation";
    return (
      <div className="webpage-rule">
        <WebpageHead title={t("Add Rule for Current Webpage")} sub="" />
        <div className="webpage-error">
          {automation ? t("ClashCube isn't allowed to read the browser's page. Allow it under Automation in Privacy & Security.")
            : page.error === "none" ? t("The browser has no page open.")
            : t("Couldn't read the browser's page: {error}", { error: page.error })}
        </div>
        <div className="foot">
          {automation && <button className="btn" onClick={() => App.OpenAutomationSettings().catch(toastError)}>{t("Open Privacy & Security…")}</button>}
          <button className="btn primary" onClick={() => Window.Close()}>{t("OK")}</button>
        </div>
      </div>
    );
  }

  return (
    <form className="webpage-rule" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      <WebpageHead title={t("Add Rule for Current Webpage")} sub={page?.url ?? ""} />
      <div className="pop-form webpage-fields">
        <RuleFields rule={rule} setRule={setRule} choices={page?.rules ?? []} policies={policies} input={input} />
        <Segmented className="track small fill" value={kind} onChange={setKind} options={[
          { value: "permanent", label: t("Permanent Rule") },
          { value: "temporary", label: t("Temporary Rule") },
        ]} />
        <div className="hint">{kind === "temporary"
          ? t("A temporary rule isn't saved. It goes when the profile is switched or updated, or the app quits.")
          : t("Goes ahead of the profile's rules and stays across profile updates.")}</div>
        {!running && <div className="hint warn">{t("The core isn't running; the rule takes effect once it starts.")}</div>}
      </div>
      <div className="foot">
        <button type="button" className="btn" onClick={() => Window.Close()}>{t("Cancel")}</button>
        <button type="submit" className="btn primary" disabled={busy || !page || !rule.payload.trim() || !rule.policy}>{t("Add")}</button>
      </div>
    </form>
  );
}

function WebpageHead({ title, sub }: { title: string; sub: string }) {
  return (
    <div className="webpage-head">
      <div className="titles">
        <h2>{title}</h2>
        {sub && <div className="url" title={sub}>{sub}</div>}
      </div>
      <span className="logo"><Logo /></span>
    </div>
  );
}
