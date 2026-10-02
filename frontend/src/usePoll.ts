import { useEffect, useRef } from "react";

// usePoll calls fn now and every ms while the page is visible.
export function usePoll(fn: () => void, ms: number, deps: unknown[] = []) {
  const f = useRef(fn);
  f.current = fn;
  useEffect(() => {
    let timer: number | undefined;
    const tick = () => { if (!document.hidden) f.current(); };
    tick();
    timer = window.setInterval(tick, ms);
    const vis = () => { if (!document.hidden) f.current(); };
    document.addEventListener("visibilitychange", vis);
    return () => { clearInterval(timer); document.removeEventListener("visibilitychange", vis); };
  }, deps);
}
