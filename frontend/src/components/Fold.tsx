import { useEffect, useRef, useState, type ReactNode } from "react";

// Fold unrolls its children like a scroll: the height opens on a long
// ease-out and each .stagger child settles a beat after the one above.
// Closed, the children leave the DOM once the fold has rolled up.
export function Fold({ open, children, onSettled }: { open: boolean; children: ReactNode; onSettled?: () => void }) {
  const [mounted, setMounted] = useState(open);
  const [unrolled, setUnrolled] = useState(open);
  const first = useRef(true);
  useEffect(() => {
    if (first.current) { first.current = false; return; }
    if (open) {
      setMounted(true);
      // a frame closed first, so the transition has somewhere to start
      const r = requestAnimationFrame(() => requestAnimationFrame(() => setUnrolled(true)));
      return () => cancelAnimationFrame(r);
    }
    setUnrolled(false);
    const t = setTimeout(() => { setMounted(false); onSettled?.(); }, 440);
    return () => clearTimeout(t);
  }, [open]);
  useEffect(() => {
    if (!unrolled) return;
    const t = setTimeout(() => onSettled?.(), 640);
    return () => clearTimeout(t);
  }, [unrolled]);
  return (
    <div className={"fold" + (unrolled ? " open" : "")}>
      <div className="fold-inner">{mounted && children}</div>
    </div>
  );
}
