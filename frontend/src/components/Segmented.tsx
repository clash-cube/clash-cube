import { useLayoutEffect, useRef } from "react";

type Option<T extends string> = { value: T; label: string; title?: string };

// where each control with an id last put its thumb, for the same control on
// the next page to glide on from there
const lastThumb = new Map<string, { x: number; w: number }>();

// A segmented control whose one thumb glides to the chosen option, rather
// than each option lighting up on its own. One with an id carries its thumb
// over when a page change remounts it.
export function Segmented<T extends string>({ options, value, onChange, className = "", id }: {
  options: Option<T>[];
  value: T;
  onChange: (v: T) => void;
  className?: string;
  id?: string;
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
      // the first placement doesn't slide in from the left edge, but from
      // where the same control last had it
      const from = id && !placed.current ? lastThumb.get(id) : undefined;
      if (!placed.current) th.classList.add("still");
      if (from) {
        th.style.transform = `translateX(${from.x}px)`;
        th.style.width = from.w + "px";
        void th.offsetWidth;
        th.classList.remove("still");
      }
      th.style.transform = `translateX(${on.offsetLeft}px)`;
      th.style.width = on.offsetWidth + "px";
      if (id) lastThumb.set(id, { x: on.offsetLeft, w: on.offsetWidth });
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
        <button type="button" key={o.value} role="tab" aria-selected={o.value === value} title={o.title} className={o.value === value ? "on" : ""} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}
