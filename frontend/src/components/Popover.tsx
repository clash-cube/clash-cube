import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

// A popover that grows out of the element it opens from (--ox, the anchor's
// middle) and sinks back toward it as it closes.
export function Popover({ anchor, open, onClose, children, align = "start", width }: {
  anchor: HTMLElement | null;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
  align?: "start" | "end";
  width?: number;
}) {
  const [shown, setShown] = useState(open);
  const [leaving, setLeaving] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState<{ left: number; top: number; ox: number; up: boolean }>({ left: 0, top: 0, ox: 24, up: false });
  // bumped when the content changes size, so it is placed again
  const [size, setSize] = useState(0);

  useEffect(() => {
    if (open) { setShown(true); setLeaving(false); return; }
    if (!shown) return;
    setLeaving(true);
    const t = setTimeout(() => { setShown(false); setLeaving(false); }, 220);
    return () => clearTimeout(t);
  }, [open]);

  useLayoutEffect(() => {
    if (!shown || !anchor || !ref.current) return;
    const a = anchor.getBoundingClientRect();
    // its laid-out size: the pop-in animation scales the box it draws
    const p = { width: ref.current.offsetWidth, height: ref.current.offsetHeight };
    const w = width ?? p.width;
    let left = align === "end" ? a.right - w : a.left;
    left = Math.max(8, Math.min(left, window.innerWidth - w - 8));
    const below = a.bottom + 6;
    const up = below + p.height > window.innerHeight - 8 && a.top - p.height - 6 > 8;
    // with room neither below nor above, it is held inside the window
    const top = up ? a.top - p.height - 6 : Math.max(8, Math.min(below, window.innerHeight - p.height - 8));
    setPos({ left, top, ox: a.left + a.width / 2 - left, up });
  }, [shown, anchor, size]);

  useEffect(() => {
    if (!shown || !ref.current) return;
    const ro = new ResizeObserver(() => setSize((n) => n + 1));
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, [shown]);

  useEffect(() => {
    if (!open) return;
    const down = (e: MouseEvent) => {
      if (ref.current?.contains(e.target as Node) || anchor?.contains(e.target as Node)) return;
      onClose();
    };
    const key = (e: KeyboardEvent) => { if (e.key === "Escape") { e.stopPropagation(); onClose(); } };
    document.addEventListener("mousedown", down, true);
    document.addEventListener("keydown", key, true);
    return () => { document.removeEventListener("mousedown", down, true); document.removeEventListener("keydown", key, true); };
  }, [open, anchor, onClose]);

  if (!shown) return null;
  return createPortal(
    <div
      ref={ref}
      className={"pop" + (pos.up ? " up" : "") + (leaving ? " leaving" : "")}
      style={{ left: pos.left, top: pos.top, width, ["--ox" as string]: pos.ox + "px" }}
      // Portal clicks still bubble through React parents, including clickable rows.
      onClick={(e) => e.stopPropagation()}
    >
      {children}
    </div>,
    document.body,
  );
}

export type MenuItem = { label: string; onClick: () => void; danger?: boolean; checked?: boolean } | "sep";

export function Menu({ items, close }: { items: MenuItem[]; close: () => void }) {
  // a menu with any checkable item gives every item the check's column
  const checks = items.some((it) => it !== "sep" && it.checked !== undefined);
  return (
    <div className="menu">
      {items.map((it, i) =>
        it === "sep" ? <hr key={i} /> : (
          <button key={i} className={it.danger ? "danger" : ""} onClick={() => { close(); it.onClick(); }}>
            {checks && <span className="mcheck">{it.checked ? "✓" : ""}</span>}
            <span className="mlabel">{it.label}</span>
          </button>
        ),
      )}
    </div>
  );
}
