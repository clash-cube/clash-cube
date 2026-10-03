import { useLayoutEffect, useRef } from "react";

type Option<T extends string> = { value: T; label: string };

// A segmented control whose one thumb glides to the chosen option, rather
// than each option lighting up on its own.
export function Segmented<T extends string>({ options, value, onChange, className = "" }: {
  options: Option<T>[];
  value: T;
  onChange: (v: T) => void;
  className?: string;
}) {
  const box = useRef<HTMLDivElement>(null);
  const thumb = useRef<HTMLSpanElement>(null);
  const placed = useRef(false);

  useLayoutEffect(() => {
    const place = () => {
      const on = box.current?.querySelector<HTMLElement>(":scope > button.on");
      const th = thumb.current;
      if (!th) return;
      if (!on) { th.style.opacity = "0"; return; }
      th.style.opacity = "";
      // the first placement doesn't slide in from the left edge
      if (!placed.current) th.classList.add("still");
      th.style.transform = `translateX(${on.offsetLeft}px)`;
      th.style.width = on.offsetWidth + "px";
      if (!placed.current) { void th.offsetWidth; th.classList.remove("still"); placed.current = true; }
    };
    place();
    const ro = new ResizeObserver(place);
    if (box.current) ro.observe(box.current);
    return () => ro.disconnect();
  }, [value, options.map((o) => o.label).join("|")]);

  return (
    <div className={"seg " + className} ref={box} role="tablist">
      <span className="thumb" ref={thumb} />
      {options.map((o) => (
        <button key={o.value} role="tab" aria-selected={o.value === value} className={o.value === value ? "on" : ""} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}
