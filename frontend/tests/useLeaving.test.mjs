import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);
function mount(items, reduced = false) {
  let now = 0, rows, root;
  const timers = new Map();
  const exports = {};
  const source = readFileSync(new URL("../src/useLeaving.ts", import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
  runInNewContext(outputText, {
    exports, require: (id) => id === "./components/Sidebar" ? { calm: () => reduced } : require(id),
    setTimeout: (fn, ms) => { timers.set(fn, now + ms); return fn; },
    clearTimeout: (id) => timers.delete(id),
  });
  function List({ items }) { rows = exports.useLeaving(items, (item) => item); return null; }
  act(() => { root = create(React.createElement(List, { items })); });
  return {
    rows: () => Array.from(rows, ({ item, leaving }) => [item, leaving]),
    update: (items) => act(() => root.update(React.createElement(List, { items }))),
    advance: (ms) => act(() => {
      now += ms;
      for (const [fn, deadline] of [...timers]) if (deadline <= now) { timers.delete(fn); fn(); }
    }),
    close: () => act(() => root.unmount()),
    timers,
  };
}

test("removed rows keep their slots and independent deadlines during reorder", () => {
  const view = mount(["a", "b", "c"]);
  try {
    view.update(["a", "c"]);
    assert.deepEqual(view.rows(), [["a", false], ["b", true], ["c", false]]);
    view.advance(100);
    view.update(["c"]);
    assert.deepEqual(view.rows(), [["a", true], ["b", true], ["c", false]]);
    view.advance(120);
    assert.deepEqual(view.rows(), [["a", true], ["c", false]]);
    view.advance(100);
    assert.deepEqual(view.rows(), [["c", false]]);
  } finally { view.close(); }
});

test("restoring a row cancels its exit and unmount clears outstanding timers", () => {
  const view = mount(["a", "b"]);
  view.update(["b"]);
  view.advance(100);
  view.update(["b", "a"]);
  view.advance(120);
  assert.deepEqual(view.rows(), [["b", false], ["a", false]]);
  assert.equal(view.timers.size, 0);
  view.update([]);
  view.close();
  assert.equal(view.timers.size, 0);
});

test("reduced motion removes rows immediately without timers", () => {
  const view = mount(["a", "b"], true);
  try {
    view.update(["b"]);
    assert.deepEqual(view.rows(), [["b", false]]);
    assert.equal(view.timers.size, 0);
  } finally { view.close(); }
});
