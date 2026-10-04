import { useRef, useState, type CSSProperties } from "react";
import type { LatencySample } from "../api";
import { useT } from "../i18n";
import { delayClass } from "../format";
import { clock } from "./Sparkline";

const H = 20;

// LatencyBars draws an item's recent measures beside its figure, newest at
// the right, each coloured as the figure would be, and a failure as a full,
// faint red bar. The scale is the highest measure, but never below floor,
// so a steady network stays flat rather than having its noise blown up. A
// thin rule marks where the network changed. What comes in after mount
// slides in from the right; what was there unrolls from now back.
export function LatencyBars({ data, floor, fmt }: { data: LatencySample[]; floor: number; fmt: (v: number) => string }) {
  const t = useT();
  const mounted = useRef(Date.now());
  const [tip, setTip] = useState<{ i: number; x: number } | null>(null);
  if (!data.length) return null;
  const top = Math.max(floor, ...data.map((d) => d.ms));
  const shown = tip && data[tip.i];
  return (
    // an empty title keeps the card's own tooltip off the bars
    <div className="lbars" title="" onMouseLeave={() => setTip(null)}>
      <div className="lbars-track">
        {data.map((d, i) => {
          const fresh = d.at > mounted.current;
          const cls = ["lbar", delayClass(d.ms), i === data.length - 1 && "last", d.break && "brk", fresh && "fresh", tip?.i === i && "on"];
          return (
            <span
              key={d.at}
              className={cls.filter(Boolean).join(" ")}
              style={{ "--i": fresh ? 0 : data.length - 1 - i } as CSSProperties}
              onMouseEnter={(e) => setTip({ i, x: e.currentTarget.offsetLeft + e.currentTarget.offsetWidth })}
            >
              <i style={{ height: d.ms < 0 ? H : Math.max(3, Math.round((d.ms / top) * H)) }} />
            </span>
          );
        })}
      </div>
      {shown && (
        <div className="spark-tip lbar-tip num" style={{ right: `calc(100% - ${tip.x}px)` }}>
          <div className="at">{clock(shown.at)}</div>
          <div><i className={delayClass(shown.ms)} />{shown.ms < 0 ? t("Failed") : <b>{fmt(shown.ms)} ms</b>}</div>
          {shown.break && <div className="brk-note">{t("Network changed")}</div>}
        </div>
      )}
    </div>
  );
}
