import { useLayoutEffect, useRef, type RefObject } from "react";

// Transforms follow keyed items without changing their layout mid-animation.
export function useFlip(box: RefObject<HTMLElement | null>, order: string,
  opts?: { selector?: string; axis?: "y" | "xy"; enter?: boolean }): void {
  const positions = useRef(new Map<string, { top: number; left: number }>());
  const { selector = ":scope > [data-key]", axis = "y", enter = true } = opts ?? {};
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    const still = matchMedia("(prefers-reduced-motion: reduce)").matches
      || document.documentElement.classList.contains("proxies-morph");
    const easing = getComputedStyle(el).getPropertyValue("--ease-out").trim() || "ease-out";
    const first = positions.current.size === 0;
    const next = new Map<string, { top: number; left: number }>();
    el.querySelectorAll<HTMLElement>(selector).forEach((row, i) => {
      const key = row.dataset.key!, top = row.offsetTop, left = row.offsetLeft, was = positions.current.get(key);
      next.set(key, { top, left });
      if (still) return;
      if (was === undefined) {
        if (enter) row.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 280, easing, delay: first ? Math.min(i, 10) * 25 : 0, fill: "backwards" });
      } else if (was.top !== top || (axis === "xy" && was.left !== left)) {
        row.animate([{ transform: `translate(${axis === "xy" ? was.left - left : 0}px, ${was.top - top}px)` }, { transform: "none" }], { duration: 420, easing });
      }
    });
    positions.current = next;
  }, [order, selector, axis, enter]);
}
