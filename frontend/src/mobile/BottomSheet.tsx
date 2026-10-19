import { useEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useT } from "../i18n";
import { useOverlayHistory } from "./useOverlayHistory";

export function BottomSheet({ open, onClose, children }: { open: boolean; onClose: () => void; children: ReactNode }) {
  const t = useT();
  useOverlayHistory("sheet", open, onClose);
  const [shown, setShown] = useState(open);
  const [leaving, setLeaving] = useState(false);
  const touch = useRef(0);
  const box = useRef<HTMLDivElement>(null);
  const dragging = useRef(false);
  useEffect(() => {
    if (!open) return;
    setShown(true); setLeaving(false);
    const previous = document.activeElement as HTMLElement | null;
    const frame = requestAnimationFrame(() => box.current?.focus());
    return () => {
      cancelAnimationFrame(frame);
      previous?.focus();
    };
  }, [open]);
  useEffect(() => {
    if (open) return;
    setLeaving(true);
    const timer = setTimeout(() => setShown(false), 280);
    return () => clearTimeout(timer);
  }, [open]);
  if (!shown) return null;
  return createPortal(<div className={"mobile-sheet-scrim" + (leaving ? " leaving" : "")} onClick={e => { e.stopPropagation(); if (e.target === e.currentTarget) onClose(); }}>
    <div ref={box} tabIndex={-1} className="mobile-sheet" role="dialog" aria-modal="true" aria-label={t("Actions")}
      onTouchStart={e => { dragging.current = (box.current?.querySelector(".mobile-sheet-body")?.scrollTop ?? 0) === 0; touch.current = e.touches[0].clientY; }}
      onTouchEnd={e => { if (dragging.current && e.changedTouches[0].clientY - touch.current > 80) onClose(); }}
      onKeyDown={e => {
        if (e.key === "Escape") onClose();
        if (e.key !== "Tab") return;
        const targets = box.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]');
        if (!targets?.length) return;
        const first = targets[0], last = targets[targets.length - 1];
        if (e.shiftKey && (document.activeElement === first || document.activeElement === box.current)) { e.preventDefault(); last.focus(); }
        else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
      }}>
      <button className="mobile-sheet-handle" aria-label={t("Close")} onClick={onClose} onTouchStart={e => { touch.current = e.touches[0].clientY; }} onTouchEnd={e => { if (e.changedTouches[0].clientY - touch.current > 80) onClose(); }}><span /></button>
      <div className="mobile-sheet-body">{children}</div>
    </div>
  </div>, document.body);
}
