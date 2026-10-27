import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);
const off = { enabled: false, active: false, interface: "" };
const active = { enabled: true, active: true, interface: "br-lan" };
const deferred = () => {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
};

async function mount() {
  let snapshot = off;
  let read = async () => snapshot;
  let change = async () => { snapshot = active; return snapshot; };
  let poll;
  const errors = [];
  const Switch = () => null;
  const mocks = {
    "../api": { Router: { Get: () => read(), SetEnabled: (on) => change(on) } },
    "../store": { useStore: (select) => select({ state: { core: "running" } }) },
    "../usePoll": { usePoll: (fn) => {
      poll = fn;
      React.useEffect(() => { fn(); }, []);
    } },
    "../i18n": { useT: () => (text) => text },
    "./Switch": { Switch },
    "./Toast": { toastError: (error) => errors.push(error) },
    "./RouterAccess": { RouterAccess: () => null },
    "./RouterMaintenance": { RouterMaintenance: () => null },
    "./RouterDiagnostics": { RouterDiagnostics: () => null },
  };
  const source = readFileSync(new URL("../src/components/RouterPanel.tsx", import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
  });
  const exports = {};
  runInNewContext(outputText, { exports, require: (id) => mocks[id] ?? require(id) });
  let root;
  await act(async () => { root = create(React.createElement(exports.RouterPanel)); });
  return {
    get switch() { return root.root.findByType(Switch).props; },
    get errors() { return errors; },
    setRead: (next) => { read = next; },
    setChange: (next) => { change = next; },
    poll: () => act(async () => { poll(); }),
    toggle: (on) => act(async () => { await root.root.findByType(Switch).props.onChange(on); }),
    close: () => act(async () => { root.unmount(); }),
  };
}

test("an old status read cannot undo a completed LAN activation", async () => {
  const h = await mount();
  try {
    const stale = deferred();
    h.setRead(() => stale.promise);
    await h.poll();
    h.setRead(async () => active);
    await h.toggle(true);
    assert.equal(h.switch.on, true);
    await act(async () => { stale.resolve(off); });
    assert.equal(h.switch.on, true, "stale poll switched the UI back off");
  } finally { await h.close(); }
});

test("rejected activation refreshes the rolled back state and allows retry", async () => {
  const h = await mount();
  try {
    h.setChange(async () => { throw new Error("port occupied"); });
    await h.toggle(true);
    assert.equal(h.switch.on, false);
    assert.equal(h.switch.busy, false);
    assert.equal(h.switch.disabled, false);
    assert.equal(h.errors.length, 1);
    h.setChange(async () => active);
    h.setRead(async () => active);
    await h.toggle(true);
    assert.equal(h.switch.on, true);
  } finally { await h.close(); }
});
