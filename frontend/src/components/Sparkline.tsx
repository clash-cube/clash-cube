import { useEffect, useId, useRef, useState } from "react";
import type { Sample } from "../store";
import { useT } from "../i18n";
import { speed } from "../format";

const W = 300;
const GRID = [1 / 3, 2 / 3];
const still = () => matchMedia("(prefers-reduced-motion: reduce)").matches;

// a smooth line through the points that never overshoots them
function line(xs: number[], ys: number[]) {
  let s = `M${xs[0].toFixed(1)},${ys[0].toFixed(1)}`;
  for (let i = 1; i < xs.length; i++) {
    const cx = ((xs[i - 1] + xs[i]) / 2).toFixed(1);
    s += ` C${cx},${ys[i - 1].toFixed(1)} ${cx},${ys[i].toFixed(1)} ${xs[i].toFixed(1)},${ys[i].toFixed(1)}`;
  }
  return s;
}

export const clock = (at: number) => new Date(at).toLocaleTimeString([], { hour12: false });

// Sparkline draws upload and download over the last minute, the download
// filled under its line. As a time axis does, it glides left between
// samples, the newest coming in from beyond the right edge, and its scale
// eases to a new peak rather than jumping. Zero sits just below the edge,
// so a quiet moment leaves only the faint baseline. Under the pointer it
// marks the nearest sample and shows it in a tip, or hands it to onHover.
export function Sparkline({ data, height = 64, showUp = true, grid = false, tooltip = true, onHover }: {
  data: Sample[]; height?: number; showUp?: boolean; grid?: boolean; tooltip?: boolean; onHover?: (s: Sample | null) => void;
}) {
  const t = useT();
  const id = "spark" + useId().replace(/:/g, "");
  const floor = height + 2;
  const box = useRef<HTMLDivElement>(null);
  const area = useRef<SVGPathElement>(null);
  const down = useRef<SVGPathElement>(null);
  const up = useRef<SVGPathElement>(null);
  const mark = useRef<HTMLDivElement>(null);
  const dotUp = useRef<HTMLSpanElement>(null);
  const dotDown = useRef<HTMLSpanElement>(null);
  const labels = useRef<(HTMLSpanElement | null)[]>([]);
  const [tip, setTip] = useState<Sample | null>(null);
  const hover = useRef(onHover);
  hover.current = onHover;
  const a = useRef({ data, at: 0, every: 1000, max: 0, last: 0, mouse: -1, raf: 0, shown: null as Sample | null });
  const kick = useRef(() => {});

  useEffect(() => {
    const s = a.current;
    const frame = (now: number) => {
      s.raf = 0;
      const d = s.data, n = d.length;
      if (n < 3) return;
      const calm = still();
      const step = W / (n - 2);
      const offset = calm ? 1 : Math.min(1, (now - s.at) / s.every);
      const target = Math.max(1024, ...d.map((p) => Math.max(p.up, p.down))) * 1.15;
      const dt = s.last ? Math.min(now - s.last, 100) : 100;
      s.last = now;
      s.max = calm || !s.max || Math.abs(target - s.max) < target * .002 ? target : s.max + (target - s.max) * Math.min(1, dt / 260);

      // the oldest point slides off the left edge as the newest comes in
      const x0 = W + step * (1 - offset) - (n - 1) * step;
      const xs = d.map((_, i) => x0 + i * step);
      const y = (v: number) => floor - (v / s.max) * floor;
      const yd = d.map((p) => y(p.down)), yu = d.map((p) => y(p.up));
      const dl = line(xs, yd);
      down.current?.setAttribute("d", dl);
      area.current?.setAttribute("d", `${dl} L${xs[n - 1].toFixed(1)},${floor} L${xs[0].toFixed(1)},${floor} Z`);
      up.current?.setAttribute("d", line(xs, yu));
      GRID.forEach((f, k) => {
        const el = labels.current[k], text = speed((1 - (height * f) / floor) * s.max);
        if (el && el.textContent !== text) el.textContent = text;
      });

      const el = box.current, m = mark.current;
      if (s.mouse >= 0 && el && m) {
        const px = el.clientWidth / W;
        let i = Math.max(0, Math.min(n - 1, Math.round((s.mouse / px - x0) / step)));
        while (i > 0 && xs[i] > W) i--;
        while (i < n - 1 && xs[i] < 0) i++;
        const gx = xs[i] * px;
        m.hidden = false;
        m.style.transform = `translateX(${gx.toFixed(1)}px)`;
        m.classList.toggle("flip", gx > el.clientWidth / 2);
        if (dotDown.current) dotDown.current.style.top = Math.min(yd[i], height - 1) + "px";
        if (dotUp.current) dotUp.current.style.top = Math.min(yu[i], height - 1) + "px";
        if (s.shown !== d[i]) { s.shown = d[i]; setTip(d[i]); hover.current?.(d[i]); }
      }
      if ((offset < 1 || s.max !== target) && !document.hidden) s.raf = requestAnimationFrame(frame);
    };
    kick.current = () => { if (!s.raf) s.raf = requestAnimationFrame(frame); };
    const onVis = () => { if (!document.hidden) kick.current(); };
    document.addEventListener("visibilitychange", onVis);
    kick.current();
    return () => { cancelAnimationFrame(s.raf); s.raf = 0; document.removeEventListener("visibilitychange", onVis); };
  }, [height]);

  // a new sample: start the glide to it, at the pace samples have come
  useEffect(() => {
    const s = a.current, now = performance.now();
    const gap = now - s.at;
    if (s.at && gap > 400 && gap < 3000) s.every = s.every * .7 + gap * .3;
    s.data = data;
    s.at = now;
    kick.current();
  }, [data]);

  const move = (e: React.MouseEvent) => {
    a.current.mouse = e.clientX - e.currentTarget.getBoundingClientRect().left;
    kick.current();
  };
  const leave = () => {
    const s = a.current;
    s.mouse = -1;
    s.shown = null;
    if (mark.current) mark.current.hidden = true;
    setTip(null);
    hover.current?.(null);
  };

  return (
    <div className="spark-box" ref={box} style={{ height }} onMouseMove={move} onMouseLeave={leave}>
      <svg className="spark" viewBox={`0 0 ${W} ${height}`} preserveAspectRatio="none" width="100%" height={height}>
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="var(--down)" stopOpacity=".28" />
            <stop offset="1" stopColor="var(--down)" stopOpacity="0" />
          </linearGradient>
        </defs>
        {grid && GRID.map((f) => <line key={f} className="spark-grid" x1="0" x2={W} y1={height * f} y2={height * f} vectorEffect="non-scaling-stroke" />)}
        <line className="spark-base" x1="0" x2={W} y1={height - .5} y2={height - .5} vectorEffect="non-scaling-stroke" />
        <path ref={area} fill={`url(#${id})`} />
        <path ref={down} fill="none" stroke="var(--down)" strokeWidth="1.6" vectorEffect="non-scaling-stroke" />
        {showUp && <path ref={up} fill="none" stroke="var(--up)" strokeWidth="1.4" vectorEffect="non-scaling-stroke" opacity=".9" />}
      </svg>
      {grid && GRID.map((f, k) => <span key={f} className="spark-label num" style={{ top: height * f }} ref={(el) => { labels.current[k] = el; }} />)}
      <div className="spark-mark" ref={mark} hidden>
        <span className="spark-dot down" ref={dotDown} />
        {showUp && <span className="spark-dot up" ref={dotUp} />}
        {tooltip && tip && (
          <div className="spark-tip num">
            <div className="at">{clock(tip.at)}</div>
            <div><i className="up" />{t("Upload")}<b>{speed(tip.up)}</b></div>
            <div><i className="down" />{t("Download")}<b>{speed(tip.down)}</b></div>
          </div>
        )}
      </div>
    </div>
  );
}
