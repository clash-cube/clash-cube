import { useEffect, useMemo, useRef, useState } from "react";
import { App, ProfileFiles, type Profile } from "../api";
import type { ProfileDocument } from "../profileTypes";
import { useT } from "../i18n";
import { errText, toast } from "./Toast";
import { YamlEditor } from "./YamlEditor";
import { Close } from "./Icons";
import "./profileEditor.css";

export function ProfileEditor({ profile, active, onClose, onCopy }: {
  profile: Profile; active: boolean; onClose: () => void; onCopy: () => Promise<void>;
}) {
  const t = useT();
  const dialog = useRef<HTMLDialogElement>(null);
  const keepEditing = useRef<HTMLButtonElement>(null);
  const [document, setDocument] = useState<ProfileDocument | null>(null);
  const [body, setBody] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [load, setLoad] = useState(0);
  const [discard, setDiscard] = useState<"close" | "reload" | null>(null);
  const [cursor, setCursor] = useState({ line: 1, column: 1 });
  const [syntax, setSyntax] = useState({ pending: true, invalid: false });
  const dirty = !!document && body !== document.body;
  const lines = useMemo(() => body.split("\n").length, [body]);
  const saveShortcut = /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘S" : "Ctrl S";

  useEffect(() => { if (discard) keepEditing.current?.focus(); }, [discard]);

  useEffect(() => {
    const el = dialog.current!;
    el.showModal();
    return () => el.close();
  }, []);
  useEffect(() => {
    let live = true;
    setBusy(true);
    ProfileFiles!.ReadDocument(profile.id).then((doc) => {
      if (live) { setDocument(doc); setBody(doc.body); setError(""); setCursor({ line: 1, column: 1 }); }
    }, (e) => { if (live) setError(errText(e)); }).finally(() => { if (live) setBusy(false); });
    return () => { live = false; };
  }, [profile.id, load]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ""; };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const leave = (action: "close" | "reload") => {
    if (busy) return;
    if (dirty) { setDiscard(action); return; }
    if (action === "close") onClose(); else setLoad((n) => n + 1);
  };
  const save = async () => {
    if (!document || document.readOnly || !dirty || busy || discard || syntax.pending || syntax.invalid) return;
    setBusy(true); setError("");
    try {
      const doc = await ProfileFiles!.SaveContent(profile.id, document.revision, body);
      setDocument(doc); setBody(doc.body); toast(t("Configuration saved"));
    } catch (e) { setError(errText(e)); }
    finally { setBusy(false); }
  };

  return <dialog ref={dialog} className="profile-editor" aria-label={t("Configuration editor")} onCancel={(e) => { e.preventDefault(); leave("close"); }}>
    <form onSubmit={(e) => { e.preventDefault(); void save(); }} onKeyDown={(e) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") { e.preventDefault(); void save(); }
    }}>
      <div className="profile-editor-head">
        <div className="profile-editor-heading">
          <div className="profile-editor-title"><h2>{t(document?.readOnly ? "View configuration" : "Edit configuration")}</h2>
            {document && <span className={"profile-editor-pill" + (document.readOnly ? "" : busy ? " checking" : dirty ? " dirty" : "")}>{t(document.readOnly ? "Read-only" : busy ? "Checking…" : dirty ? "Unsaved" : "Saved")}</span>}
          </div>
          <p>{profile.name} · {t(document?.readOnly ? "Subscription" : active ? "In use · saving checks and hot-reloads" : "Checked by the core when you switch to it")}</p>
        </div>
        <button type="button" className="profile-editor-icon profile-editor-close" aria-label={t("Close")} disabled={busy} onClick={() => leave("close")}><Close /></button>
      </div>
      {!document && <div className="profile-editor-loading" role="status">{!error && t("Loading…")}</div>}
      {document && <YamlEditor body={body} original={document.body} readOnly={document.readOnly || busy}
        subscription={document.readOnly} cursor={cursor} lines={lines} copyDisabled={busy}
        onCopy={() => { void App.CopyText(body).then(() => toast(t("Copied")), (e) => setError(errText(e))); }}
        onChange={(value) => { setBody(value); setError(""); setDiscard(null); }} onCursor={setCursor}
        onCheck={(pending, invalid) => setSyntax({ pending, invalid })} />}
      {error && <div className="profile-editor-error" role="alert"><span>{t(error)}</span><button type="button" className="profile-editor-icon" aria-label={t("Close")} onClick={() => setError("")}><Close size={14} /></button></div>}
      <div className="profile-editor-foot">
        {discard ? <><p>{t("Discard your unsaved configuration changes?")}</p><button ref={keepEditing} type="button" className="btn" onClick={() => setDiscard(null)}>{t("Keep editing")}</button><button type="button" className="btn danger armed" onClick={() => { if (discard === "close") onClose(); else { setDiscard(null); setLoad((n) => n + 1); } }}>{t("Discard changes")}</button></> : <>
          <div className="profile-editor-foot-start">{dirty && !document?.readOnly && <button type="button" className="btn" disabled={busy} onClick={() => leave("reload")}>{t("Revert")}</button>}</div>
          <button type="button" className="btn" disabled={busy} onClick={() => leave("close")}>{t("Close")}</button>
          {document?.readOnly ? <button type="button" className="btn primary" disabled={busy} onClick={async () => { setBusy(true); try { await onCopy(); } catch (e) { setError(errText(e)); setBusy(false); } }}>{t("Make an editable copy")}</button>
            : <button className="btn primary" aria-label={t("Save configuration")} disabled={!document || !dirty || busy || syntax.pending || syntax.invalid}>{t(busy ? "Checking…" : "Save")}{!busy && <span className="profile-editor-keycap" aria-hidden="true">{saveShortcut}</span>}</button>}
        </>}
      </div>
    </form>
  </dialog>;
}
