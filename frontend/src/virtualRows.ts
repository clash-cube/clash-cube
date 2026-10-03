// Prefix offsets allow mixed-height headers and connection rows to share one
// scroll viewport without mounting every expanded group's children.
export function rowOffsets(heights: number[]) {
  const offsets = [0];
  for (const height of heights) offsets.push(offsets[offsets.length - 1] + height);
  return offsets;
}

export function visibleRows(offsets: number[], scrollTop: number, height: number, overscan = 200) {
  const count = offsets.length - 1;
  const total = offsets[count];
  const top = Math.max(0, Math.min(scrollTop, total - height));
  const find = (position: number) => {
    let lo = 0, hi = count;
    while (lo < hi) {
      const mid = (lo + hi) >>> 1;
      if (offsets[mid + 1] <= position) lo = mid + 1; else hi = mid;
    }
    return lo;
  };
  return { start: find(Math.max(0, top - overscan)), end: Math.min(count, find(top + height + overscan) + 1), top, total };
}
