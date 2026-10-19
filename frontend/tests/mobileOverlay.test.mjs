import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { test } from "node:test";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

test("replacing a sheet with an editor waits for the sheet's pending Back", async () => {
  const listeners = new Set(), frames = new Map();
  const entries = [{ mobile: { page: "profiles" } }];
  let index = 0, sequence = 0, pendingBack = false;
  const history = {
    get state() { return entries[index]; },
    pushState(state) { entries.splice(++index); entries[index] = state; },
    back() { pendingBack = true; },
  };
  const exports = {};
  const source = readFileSync(new URL("../src/mobile/useOverlayHistory.ts", import.meta.url), "utf8");
  runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText, {
    exports, require: () => React, history,
    requestAnimationFrame: fn => { frames.set(++sequence, fn); return sequence; },
    cancelAnimationFrame: id => frames.delete(id),
    window: { addEventListener: (_, fn) => listeners.add(fn), removeEventListener: (_, fn) => listeners.delete(fn) },
  });
  let dismissed = 0;
  function Overlay({ kind }) { exports.useOverlayHistory(kind, true, () => dismissed++); return null; }
  const flushFrames = () => { const next = [...frames.values()]; frames.clear(); next.forEach(fn => fn()); };
  const flushBack = () => { assert.equal(pendingBack, true); pendingBack = false; index--; [...listeners].forEach(fn => fn()); };
  let root;
  await act(async () => { root = create(React.createElement(Overlay, { key: "menu", kind: "sheet" })); });
  flushFrames(); assert.ok(history.state.sheet);
  await act(async () => { root.update(React.createElement(Overlay, { key: "form", kind: "editor" })); });
  flushFrames(); // WebView has not delivered the old sheet's Back yet.
  assert.equal(history.state.editor, undefined);
  flushBack();
  assert.ok(history.state.editor);
  assert.equal(history.state.sheet, undefined);
  assert.equal(dismissed, 0);
  history.back(); flushBack();
  assert.equal(dismissed, 1);
  await act(async () => root.unmount());
  assert.equal(pendingBack, false);
});
