// reveal scrolls el to the top of its page, clear of the page's head when
// that sticks, however many lines it wraps to.
export function reveal(el: Element | null) {
  if (!(el instanceof HTMLElement)) return;
  const head = el.closest(".view")?.querySelector<HTMLElement>(".view-head");
  const sticky = head && getComputedStyle(head).position === "sticky";
  el.style.scrollMarginTop = `${(sticky ? head.offsetHeight : 0) + 12}px`;
  el.scrollIntoView({ block: "start", behavior: "smooth" });
}
