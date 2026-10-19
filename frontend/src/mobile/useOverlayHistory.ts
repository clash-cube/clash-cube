import { useEffect, useRef } from "react";

const active = new Set<string>();

// Closing one overlay and opening another can happen in the same React commit.
// Wait for the old overlay's asynchronous Back before installing the new entry.
// An active parent overlay, by contrast, remains in history for nested sheets.
export function useOverlayHistory(kind: "sheet" | "editor", open: boolean, onClose: () => void) {
  const id = useRef(`${kind}-${Math.random()}`);
  const close = useRef(onClose); close.current = onClose;
  useEffect(() => {
    if (!open) return;
    let entered = false;
    active.add(id.current);
    const enter = () => {
      if (entered) return;
      const state = history.state;
      const owner = state?.sheet ?? state?.editor;
      if (owner && !active.has(owner)) return;
      history.pushState({ ...state, [kind]: id.current }, "");
      entered = true;
    };
    const frame = requestAnimationFrame(enter);
    const back = () => {
      if (!entered) { enter(); return; }
      if (history.state?.[kind] !== id.current) close.current();
    };
    window.addEventListener("popstate", back);
    return () => {
      active.delete(id.current);
      cancelAnimationFrame(frame);
      window.removeEventListener("popstate", back);
      if (entered && history.state?.[kind] === id.current) history.back();
    };
  }, [kind, open]);
}
