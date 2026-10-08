import { useEffect, useRef, useState } from "react";

// Switch flips at once; when onChange returns a promise it shows the new
// side as pending (a spinner on the thumb) until `on` catches up, and
// springs back if it never does.
export function Switch({ on, onChange, busy, disabled, label }: { on: boolean; onChange: (v: boolean) => void | Promise<unknown>; busy?: boolean; disabled?: boolean; label?: string }) {
  const [pending, setPending] = useState<boolean | null>(null);
  const settle = useRef(0);
  useEffect(() => { if (pending !== null && on === pending) setPending(null); }, [on, pending]);
  useEffect(() => () => clearTimeout(settle.current), []);
  const shown = pending ?? on;
  return (
    <button
      // never a form's submit, which a button in a form is by default
      type="button"
      role="switch"
      aria-checked={shown}
      aria-busy={pending !== null}
      aria-label={label}
      disabled={disabled}
      className={"switch" + (shown ? " on" : "") + (busy ? " busy" : "") + (pending !== null ? " pending" : "")}
      onClick={(e) => {
        e.stopPropagation();
        if (busy || pending !== null) return;
        const p = onChange(!on);
        if (!(p instanceof Promise)) return;
        setPending(!on);
        // the state event may land just after the call returns
        p.finally(() => { settle.current = window.setTimeout(() => setPending(null), 400); });
      }}
    />
  );
}
