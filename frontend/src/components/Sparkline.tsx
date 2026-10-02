import { useMemo } from "react";
import type { Sample } from "../store";

// Sparkline draws upload and download over the last minute, smoothed, the
// download filled under its line.
export function Sparkline({ data, height = 64, showUp = true }: { data: Sample[]; height?: number; showUp?: boolean }) {
  const W = 300;
  const { up, down, downArea, idle } = useMemo(() => {
    const peak = Math.max(...data.map((d) => Math.max(d.up, d.down)));
    const max = Math.max(1024, peak) * 1.15;
    const pts = (k: "up" | "down") => data.map((d, i) => [(i / (data.length - 1)) * W, height - (d[k] / max) * (height - 4) - 2] as const);
    const path = (p: readonly (readonly [number, number])[]) => {
      let s = `M${p[0][0].toFixed(1)},${p[0][1].toFixed(1)}`;
      for (let i = 1; i < p.length; i++) {
        const [x0, y0] = p[i - 1], [x1, y1] = p[i];
        const cx = (x0 + x1) / 2;
        s += ` C${cx.toFixed(1)},${y0.toFixed(1)} ${cx.toFixed(1)},${y1.toFixed(1)} ${x1.toFixed(1)},${y1.toFixed(1)}`;
      }
      return s;
    };
    const d = path(pts("down"));
    return { up: path(pts("up")), down: d, downArea: d + ` L${W},${height} L0,${height} Z`, idle: peak === 0 };
  }, [data, height]);
  return (
    <svg className={"spark" + (idle ? " idle" : "")} viewBox={`0 0 ${W} ${height}`} preserveAspectRatio="none" width="100%" height={height}>
      <defs>
        <linearGradient id="spark-down" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="var(--down)" stopOpacity=".28" />
          <stop offset="1" stopColor="var(--down)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={downArea} fill="url(#spark-down)" />
      <path d={down} fill="none" stroke="var(--down)" strokeWidth="1.6" vectorEffect="non-scaling-stroke" />
      {showUp && <path d={up} fill="none" stroke="var(--up)" strokeWidth="1.4" strokeDasharray="0" vectorEffect="non-scaling-stroke" opacity=".9" />}
    </svg>
  );
}
