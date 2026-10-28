import { flushSync } from "react-dom";
import { calm } from "./components/Sidebar";

let running: { skipTransition: () => void; cleanup: () => void } | undefined;

// Unsupported webviews and reduced motion apply the change immediately.
// A newer morph takes ownership of the root class and the stuck guard.
export function morph(change: () => void, rootClass = "nav-morph") {
  running?.skipTransition();
  running?.cleanup();
  const doc = document as Document & { startViewTransition?: (cb: () => void) => { finished: Promise<void>; skipTransition: () => void } };
  if (calm() || !doc.startViewTransition) { change(); return; }
  const root = document.documentElement;
  root.classList.add(rootClass);
  const vt = doc.startViewTransition(() => flushSync(change));
  const stuck = setTimeout(() => vt.skipTransition(), 1500);
  const current = { skipTransition: () => vt.skipTransition(), cleanup: () => {
    clearTimeout(stuck);
    if (running !== current) return;
    root.classList.remove(rootClass);
    running = undefined;
  } };
  running = current;
  void vt.finished.then(current.cleanup, current.cleanup);
}
