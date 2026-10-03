import assert from "node:assert/strict";
import { test } from "node:test";
import { rowOffsets, visibleRows } from "../src/virtualRows.ts";

test("mixed-height virtual rows cover the viewport at boundaries and after shrinking", () => {
  const offsets = rowOffsets(Array.from({ length: 10000 }, (_, i) => i % 100 === 0 ? 40 : 64));
  for (const top of [0, 40, 104, 200000, offsets.at(-1)]) {
    const range = visibleRows(offsets, top, 500);
    assert.ok(offsets[range.start] <= range.top);
    assert.ok(offsets[range.end] >= Math.min(range.top + 500, range.total));
    assert.ok(range.end - range.start < 30, "DOM size stays bounded");
  }
  const shrunk = visibleRows(rowOffsets([40, 64]), 200000, 500);
  assert.deepEqual(shrunk, { start: 0, end: 2, top: 0, total: 104 });
  assert.deepEqual(visibleRows([0], 100, 500), { start: 0, end: 0, top: 0, total: 0 });
});
