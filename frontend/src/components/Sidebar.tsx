import { Fragment, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";
import { useStore, type View } from "../store";
import { useT } from "../i18n";
import { groups, settingsTabs, type Tab } from "../nav";
import { coreLabel, coreTone, openSettings } from "../actions";
import { Bars, Bell, Exchange, File, Gauge, Gear, Globe, Lines, Logo, Nodes, Route } from "./Icons";

const ICONS: Partial<Record<View, ReactNode>> = {
  overview: <Gauge />, usage: <Bars />, globe: <Globe />, connections: <Exchange />,
  proxies: <Nodes />, profiles: <File />, rules: <Route />,
  logs: <Lines />, events: <Bell />,
};

// the lists open and fold the way iOS's do: a spring that starts quick and
// settles slowly, the rows coming in one after another
const EASE = "cubic-bezier(.32, .72, 0, 1)";
export const calm = () => matchMedia("(prefers-reduced-motion: reduce)").matches;

// the sidebar's width, the reader's own once they drag its edge; a narrow
// window has it no wider than a third of itself and a little
export const SIDE_MIN = 180, SIDE_MAX = 320;
export const sideFit = (w: number) => Math.round(Math.max(SIDE_MIN, Math.min(w, SIDE_MAX, innerWidth * 0.36)));

// The name and the cube; hovered, a light goes once round it.
export function Brand() {
  const t = useT();
  const [wag, setWag] = useState(0);
  return (
    <div className="brand" onMouseEnter={() => setWag((w) => w + 1)}>
      <span className={"logo" + (wag ? " spin" : "")} key={wag}><Logo /></span>
      <span className="brand-name">{t("ClashCube")}</span>
    </div>
  );
}

// The window's pages down its left, in groups, the open page's own tabs
// under it, Settings and the core at the foot, as a Mac app's sidebar.
// Its edge drags it wider or narrower; dragged well past its narrowest it
// folds away (onHide), the width kept for when it comes back.
export function Sidebar({ width, onWidth, onHide }: { width: number; onWidth: (w: number) => void; onHide: () => void }) {
  const t = useT();
  const view = useStore((s) => s.view);
  const setView = useStore((s) => s.setView);
  const settingsTab = useStore((s) => s.settingsTab);
  const setSettingsTab = useStore((s) => s.setSettingsTab);
  useStore((s) => s.state);
  const side = useRef<HTMLElement>(null);

  const drag = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return;
    e.preventDefault();
    const edge = e.currentTarget, app = side.current?.parentElement;
    if (!app || !side.current) return;
    edge.setPointerCapture(e.pointerId);
    const from = side.current.getBoundingClientRect().width - e.clientX;
    document.body.classList.add("side-drag");
    let w = 0;
    const move = (m: PointerEvent) => {
      const want = from + m.clientX;
      if (want < SIDE_MIN - 64) {
        w = 0;
        end();
        app.style.setProperty("--side-w", width ? sideFit(width) + "px" : "");
        onHide();
        return;
      }
      w = sideFit(want);
      app.style.setProperty("--side-w", w + "px");
    };
    const end = () => {
      edge.removeEventListener("pointermove", move);
      edge.removeEventListener("pointerup", end);
      edge.removeEventListener("pointercancel", end);
      if (edge.hasPointerCapture(e.pointerId)) edge.releasePointerCapture(e.pointerId);
      document.body.classList.remove("side-drag");
      if (w) onWidth(w);
    };
    edge.addEventListener("pointermove", move);
    edge.addEventListener("pointerup", end);
    edge.addEventListener("pointercancel", end);
  };
  const nudge = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = e.shiftKey ? 32 : 8;
    const d = e.key === "ArrowLeft" ? -step : e.key === "ArrowRight" ? step : 0;
    if (!d || !side.current) return;
    e.preventDefault();
    onWidth(sideFit(side.current.getBoundingClientRect().width + d));
  };

  return (
    <aside className="sidebar" ref={side}>
      <div className="side-top"><Brand /></div>
      <nav className="side-scroll" aria-label={t("Pages")}>
        {groups(t).map((g) => (
          <Fragment key={g.label}>
            <div className="nav-gl">{g.label}</div>
            {g.pages.map((p) => (
              <button type="button" key={p.view} className={"side-item" + (view === p.view ? " on" : "")} data-view={p.view}
                aria-current={view === p.view ? "page" : undefined} onClick={() => view !== p.view && setView(p.view)}>
                <span className="side-ic">{ICONS[p.view]}</span><span className="side-l">{p.label}</span>
              </button>
            ))}
          </Fragment>
        ))}
      </nav>
      <div className="side-foot">
        <button type="button" className={"side-item" + (view === "settings" ? " on" : "")} data-view="settings"
          aria-current={view === "settings" ? "page" : undefined} onClick={() => view !== "settings" && setView("settings")}>
          <span className="side-ic"><Gear size={15} /></span><span className="side-l">{t("Settings")}</span>
        </button>
        <SubList open={view === "settings"} label={t("Settings")} items={settingsTabs(t)} value={settingsTab} onPick={setSettingsTab} />
        <button type="button" className="side-core" onClick={() => openSettings("core")}>
          <span className={"cdot " + coreTone()} /><span>{t("Core")} · {coreLabel()}</span>
        </button>
      </div>
      <div className="side-edge" role="separator" aria-orientation="vertical" aria-label={t("Sidebar width")} tabIndex={0}
        aria-valuemin={SIDE_MIN} aria-valuemax={SIDE_MAX} aria-valuenow={width || undefined}
        onPointerDown={drag} onKeyDown={nudge} />
    </aside>
  );
}

// A page's own tabs under it on a thin line. The selection is one shape
// that slides from row to row, setting off as the row is pressed, so it is
// under the finger rather than catching up with it. The list unrolls as its
// page opens and folds as another's does, both at once.
function SubList<V extends string>({ open, label, items, value, onPick }: {
  open: boolean; label: string; items: Tab<V>[]; value: string; onPick: (v: V) => void;
}) {
  const [mounted, setMounted] = useState(open);
  if (open && !mounted) setMounted(true);
  const box = useRef<HTMLDivElement>(null);
  const hl = useRef<HTMLElement>(null);
  const was = useRef(open);
  const placed = useRef(false);

  useLayoutEffect(() => {
    const el = box.current;
    if (was.current === open) return;
    was.current = open;
    if (!el) return;
    el.inert = !open;
    for (const a of el.getAnimations({ subtree: true })) a.cancel();
    if (calm()) { if (!open) setMounted(false); return; }
    if (open) enter(el);
    else leave(el).then(() => setMounted(false), () => {});
  }, [open]);

  const glide = (row: HTMLElement | null | undefined) => {
    const h = hl.current;
    if (!h || !row) return;
    h.style.transform = `translateY(${row.offsetTop}px)`;
    h.style.height = row.offsetHeight + "px";
  };
  useLayoutEffect(() => {
    if (!mounted) { placed.current = false; return; }
    const h = hl.current, cur = box.current?.querySelector<HTMLElement>(".nav-s.cur");
    if (!h) return;
    h.hidden = !cur;
    if (!cur) return;
    if (!placed.current) h.classList.add("still");
    glide(cur);
    if (!placed.current) { void h.offsetWidth; h.classList.remove("still"); placed.current = true; }
  }, [mounted, value, items.map((i) => i.label).join("|")]);

  if (!mounted) return null;
  const cur = () => box.current?.querySelector<HTMLElement>(".nav-s.cur");
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    const all = [...(box.current?.querySelectorAll<HTMLElement>(".nav-s") ?? [])];
    const i = all.indexOf(document.activeElement as HTMLElement);
    const to = ({ ArrowDown: i + 1, ArrowUp: i - 1, Home: 0, End: all.length - 1 } as Record<string, number>)[e.key];
    if (i < 0 || to === undefined) return;
    e.preventDefault();
    all[(to + all.length) % all.length].focus();
  };
  return (
    <div className="nav-sub" ref={box} role="tablist" aria-label={label} onKeyDown={keys}>
      <i className="nav-hl" ref={hl} />
      {items.map((it) => {
        const on = it.value === value;
        return (
          <button type="button" key={it.value} role="tab" aria-selected={on} tabIndex={on ? 0 : -1} className={"nav-s" + (on ? " cur" : "")}
            onPointerDown={(e) => { if (e.button === 0) glide(e.currentTarget); }}
            onPointerCancel={() => glide(cur())}
            onPointerLeave={(e) => { if (e.buttons & 1) glide(cur()); }}
            onClick={(e) => { glide(e.currentTarget); if (!on) onPick(it.value); }}>
            <span>{it.label}</span>
          </button>
        );
      })}
    </div>
  );
}

function enter(el: HTMLElement) {
  const cs = getComputedStyle(el);
  const to = { height: el.offsetHeight + "px", marginTop: cs.marginTop, marginBottom: cs.marginBottom, opacity: 1 };
  el.classList.add("moving");
  el.animate([{ height: "0px", marginTop: "0px", marginBottom: "0px", opacity: 0 }, to], { duration: 460, easing: EASE })
    .finished.then(() => el.classList.remove("moving"), () => {});
  el.querySelectorAll(".nav-s").forEach((s, i) => s.animate(
    [{ opacity: 0, transform: "translateY(-6px)" }, { opacity: 1, transform: "none" }],
    { duration: 380, delay: 60 + i * 28, easing: EASE, fill: "backwards" }));
  el.querySelector(".nav-hl")?.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 300, delay: 120, fill: "backwards" });
}

function leave(el: HTMLElement) {
  const cs = getComputedStyle(el);
  el.classList.add("moving");
  return el.animate([
    { height: el.offsetHeight + "px", marginTop: cs.marginTop, marginBottom: cs.marginBottom, opacity: 1 },
    { height: "0px", marginTop: "0px", marginBottom: "0px", opacity: 0 },
  ], { duration: 380, easing: EASE, fill: "forwards" }).finished;
}
