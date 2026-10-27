import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import ts from "typescript";

const require = createRequire(import.meta.url);

test("focus during a management update retains settings and a later focus refreshes them", async () => {
  const listeners = new Map();
  const previous = { theme: "light", lang: "en" };
  const current = { theme: "dark", lang: "zh" };
  let read = async () => previous;
  const mocks = {
    "./platform": { isWeb: true },
    "./api": {
      App: { State: async () => ({ core: "running" }), Events: async () => [], ConnectivityHistory: async () => ({}) },
      Settings: { Get: () => read() },
      Profiles: { List: async () => [], TakeImportRequests: async () => [] },
      Events: { On() {} },
    },
    "./i18n": { translate: (text) => text },
    "./components/Toast": { toast() {}, toastError() {} },
  };
  const source = readFileSync(new URL("../src/store.ts", import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
  });
  const exports = {};
  runInNewContext(outputText, {
    exports, require: (id) => mocks[id] ?? require(id), URLSearchParams,
    location: { search: "", hash: "" }, localStorage: { getItem() { return null; }, setItem() {} },
    document: { documentElement: { dataset: {} } },
    window: { addEventListener: (name, fn) => listeners.set(name, fn) },
  });
  await exports.boot();
  read = async () => { throw new Error("management is restarting"); };
  listeners.get("focus")();
  // Drain one event-loop turn so an unhandled rejection fails this regression.
  await new Promise(setImmediate);
  assert.equal(exports.useStore.getState().settings, previous);
  read = async () => current;
  listeners.get("focus")();
  await new Promise(setImmediate);
  assert.equal(exports.useStore.getState().settings, current);
});
