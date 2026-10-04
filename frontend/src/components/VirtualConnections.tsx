import { useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { rowOffsets, visibleRows } from "../virtualRows";

export type VirtualRow = { key: string; height: number; render: () => ReactNode };

export function VirtualConnections({ rows, resetKey, reveal }: { rows: VirtualRow[]; resetKey: string; reveal?: { key: string } | null }) {
  const viewport = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(0);
  const offsets = useMemo(() => rowOffsets(rows.map((row) => row.height)), [rows]);
  const range = visibleRows(offsets, scrollTop, height);
  useLayoutEffect(() => {
    const element = viewport.current!;
    const observer = new ResizeObserver(() => setHeight(element.clientHeight));
    setHeight(element.clientHeight);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  useLayoutEffect(() => { viewport.current!.scrollTop = 0; setScrollTop(0); }, [resetKey]);
  useLayoutEffect(() => {
    // Filtering, closing or folding can shorten the list below the old scroll
    // position. Clamp both the rendered range and the actual scroll offset.
    if (scrollTop !== range.top) { viewport.current!.scrollTop = range.top; setScrollTop(range.top); }
  }, [scrollTop, range.top]);
  useLayoutEffect(() => {
    const i = reveal ? rows.findIndex((row) => row.key === reveal.key) : -1;
    if (i < 0) return;
    const element = viewport.current!;
    const top = offsets[i], bottom = top + rows[i].height;
    const next = top < element.scrollTop ? top : bottom > element.scrollTop + element.clientHeight ? bottom - element.clientHeight : element.scrollTop;
    if (next !== element.scrollTop) { element.scrollTop = next; setScrollTop(next); }
  }, [reveal]);
  return <div ref={viewport} className="conns-list conn-virtual" onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}>
    <div className="conn-virtual-space" style={{ height: range.total }}>
      {rows.slice(range.start, range.end).map((row, i) => <div className="conn-virtual-row conns" key={row.key}
        style={{ top: offsets[range.start + i], height: row.height }}>{row.render()}</div>)}
    </div>
  </div>;
}
