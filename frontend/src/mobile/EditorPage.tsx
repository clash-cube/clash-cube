import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { useOverlayHistory } from "./useOverlayHistory";

// Keep the existing form's validation and submit handler as the single owner
// of saving, while presenting that action in the phone's app bar.
export function EditorPage({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const t = useT();
  const box = useRef<HTMLDivElement>(null);
  useOverlayHistory("editor", true, onClose);
  const [disabled, setDisabled] = useState(true);
  useEffect(() => {
    const update = () => setDisabled(box.current?.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled ?? true);
    const observer = new MutationObserver(update);
    if (box.current) observer.observe(box.current, { subtree: true, childList: true, attributes: true, attributeFilter: ["disabled"] });
    update(); return () => observer.disconnect();
  }, []);
  return createPortal(<div className="mobile-editor-page">
    <header className="mobile-bar"><button className="icon" aria-label={t("Back")} onClick={() => history.back()}>←</button><h1>{title}</h1><button className="btn" disabled={disabled} onClick={() => box.current?.querySelector<HTMLButtonElement>('button[type="submit"]')?.click()}>{t("Save")}</button></header>
    <div ref={box} className="mobile-editor-body">{children}</div>
  </div>, document.body);
}
