import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);
const deferred = () => {
  let resolve;
  const promise = new Promise((done) => { resolve = done; });
  return { promise, resolve };
};
const groups = (delay) => [{ name: "group", members: [{ name: "node", delay }] }];

// Exercise the real hooks with controlled Wails responses and browser timers.
// No browser or running core is needed, and races never depend on wall time.
async function mount({ hidden = false } = {}) {
  const document = new EventTarget();
  document.hidden = hidden;
  const timers = new Set();
  const listeners = new Map();
  let state = { core: "running", profile: "one", busy: false };
  let snapshot = groups(-1);
  let requests = 0;
  let tests = 0;
  let read = async () => snapshot;
  const useStore = (select) => select({ state });
  useStore.getState = () => ({ state });
  const mocks = {
    "./store": { useStore },
    "./api": { Proxy: {
      TestLatency: async () => { tests++; return { total: 1, failed: 0 }; },
      Groups: () => { requests++; return read(); },
      Providers: async () => [{ name: "provider", members: snapshot[0].members }],
    } },
    "./i18n": { useT: () => (text) => text },
    "./components/Toast": { toast() {}, toastError() {} },
    "@wailsio/runtime": { Events: { On: (name, fn) => {
      listeners.set(name, fn);
      return () => listeners.delete(name);
    } } },
  };
  const load = (name) => {
    const source = readFileSync(new URL(`../src/${name}.ts`, import.meta.url), "utf8");
    const { outputText } = ts.transpileModule(source, {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
    });
    const exports = {};
    runInNewContext(outputText, {
      exports, require: (id) => mocks[id] ?? require(id), document,
      window: { setInterval: (fn) => { timers.add(fn); return fn; } },
      clearInterval: (fn) => timers.delete(fn), setTimeout, clearTimeout,
    });
    return exports;
  };
  mocks["./usePoll"] = load("usePoll");
  const { useGroups } = load("useGroups");
  let value;
  function View() { value = useGroups({ providers: true, testOnOpen: true }); return null; }
  let root;
  await act(async () => { root = create(React.createElement(View)); });
  return {
    get value() { return value; },
    get requests() { return requests; },
    get tests() { return tests; },
    get timerCount() { return timers.size; },
    setSnapshot: (next) => { snapshot = next; },
    setRead: (next) => { read = next; },
    tick: () => act(async () => { for (const fn of timers) fn(); }),
    visibility: (hidden) => act(async () => {
      document.hidden = hidden;
      document.dispatchEvent(new Event("visibilitychange"));
    }),
    state: (next) => act(async () => { state = { ...state, ...next }; root.update(React.createElement(View)); }),
    emit: (data) => act(async () => { listeners.get("proxy-latency")({ data }); }),
    close: () => act(async () => { root.unmount(); }),
  };
}

test("background recovery and reopening update stale failures without running a new probe", async () => {
  const h = await mount();
  try {
    assert.equal(h.value.groups[0].members[0].delay, -1);
    assert.equal(h.tests, 1);
    h.setSnapshot(groups(166));
    await h.tick();
    assert.equal(h.value.groups[0].members[0].delay, 166);
    assert.equal(h.value.providers[0].members[0].delay, 166);
    await h.visibility(true);
    const requests = h.requests;
    h.setSnapshot(groups(203));
    await h.tick();
    assert.equal(h.requests, requests);
    await h.visibility(false);
    assert.equal(h.value.groups[0].members[0].delay, 203);
    assert.equal(h.tests, 1, "polling and reopening must not keep probing");
  } finally { await h.close(); }
  assert.equal(h.timerCount, 0);
});

test("initial testing waits for visibility and streams each completed node independently", async () => {
  const h = await mount({ hidden: true });
  try {
    assert.equal(h.tests, 0);
    h.setSnapshot([{ name: "group", members: [{ name: "fast", delay: -1 }, { name: "slow", delay: -1 }] }]);
    await h.visibility(false);
    assert.equal(h.tests, 1);
    await h.emit({ key: "all/", running: true, pending: ["fast", "slow"], delays: {}, total: 2, completed: 0 });
    await h.emit({ key: "all/", running: true, pending: ["slow"], delays: { fast: 42 }, total: 2, completed: 1 });
    assert.equal(h.value.groups[0].members[0].delay, 42);
    assert.equal(h.value.groups[0].members[1].delay, -1);
    assert.equal(h.value.testing["#fast"], undefined);
    assert.equal(h.value.testing["#slow"], true);
  } finally { await h.close(); }
});

test("a streamed result wins over an older pending snapshot", async () => {
  const h = await mount();
  try {
    const pending = deferred();
    h.setRead(() => pending.promise);
    await h.tick();
    await h.emit({ key: "node/node", running: true, pending: [], delays: { node: 42 } });
    await act(async () => { pending.resolve(groups(-1)); });
    assert.equal(h.value.groups[0].members[0].delay, 42);
  } finally { await h.close(); }
});

test("reload and stop invalidate pending reads even when hidden", async () => {
  const h = await mount();
  try {
    const pending = deferred();
    h.setRead(() => pending.promise);
    await h.tick();
    await h.visibility(true);
    await h.state({ busy: true, profile: "two" });
    await act(async () => { pending.resolve(groups(999)); });
    assert.equal(h.value.groups[0].members[0].delay, -1);
    const requests = h.requests;
    await h.visibility(false);
    await h.tick();
    assert.equal(h.requests, requests);
    h.setRead(async () => groups(20));
    await h.state({ busy: false });
    assert.equal(h.value.groups[0].members[0].delay, 20);
    await h.visibility(true);
    await h.state({ core: "stopped" });
    assert.equal(h.value.groups, null);
    assert.equal(h.value.providers, null);
  } finally { await h.close(); }
});
