import { useEffect } from "react";

// Existing context-menu handlers also serve a stationary half-second touch.
export function useTouchMenus() {
  useEffect(() => {
    let timer = 0, x = 0, y = 0, fired = false;
    const cancel = () => clearTimeout(timer);
    const down = (e: PointerEvent) => {
      if (e.pointerType !== "touch") return;
      // Reset even on SVG icons and form controls: suppress only the click
      // belonging to the long press, never the user's next tap.
      cancel(); fired = false;
      if (!(e.target instanceof Element) || e.target.closest("input, textarea, select")) return;
      x = e.clientX; y = e.clientY;
      const target = e.target;
      timer = window.setTimeout(() => {
        const menu = new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: x, clientY: y });
        fired = !target.dispatchEvent(menu);
      }, 500);
    };
    const move = (e: PointerEvent) => { if (Math.hypot(e.clientX - x, e.clientY - y) > 10) cancel(); };
    const click = (e: MouseEvent) => { if (fired) { e.preventDefault(); e.stopPropagation(); fired = false; } };
    document.addEventListener("pointerdown", down);
    document.addEventListener("pointermove", move);
    document.addEventListener("pointerup", cancel);
    document.addEventListener("pointercancel", cancel);
    document.addEventListener("click", click, true);
    return () => { cancel(); document.removeEventListener("pointerdown", down); document.removeEventListener("pointermove", move); document.removeEventListener("pointerup", cancel); document.removeEventListener("pointercancel", cancel); document.removeEventListener("click", click, true); };
  }, []);
}
