import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { runInNewContext } from "node:vm";
import assert from "node:assert/strict";
import { test } from "node:test";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);
function load(file, mocks, globals = {}) {
  const source = readFileSync(new URL(`../src/${file}`, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX } });
  const exports = {};
  runInNewContext(outputText, { exports, require: id => mocks[id] ?? require(id), ...globals });
  return exports;
}

async function mount(core, coreError, language = "zh", egress = null) {
  const store = { state: { core, coreError, profile: "default" }, traffic: { up: 0, down: 0 }, history: [] };
  const useStore = select => select(store);
  const { translate } = load("i18n.ts", { "./store": { useStore } });
  const empty = () => null;
  const mocks = {
    "../api": { App: { NodeEgress: async () => egress } },
    "../store": { useStore }, "../i18n": { useT: () => text => translate(text, undefined, language) },
    "../format": { speed: () => "0 B/s", bytes: String },
    "../actions": {}, "../useGroups": { useGroups: () => ({ groups: [] }) },
    "../connectionStore": { useConnectionStore: select => select({ snapshot: { active: [] } }) },
    "../components/TrafficClients": {}, "../components/Toast": {},
    "../components/Icons": new Proxy({}, { get: () => empty }),
    "../views/Overview": { Stat: empty, useToday: () => null },
  };
  for (const name of ["Sparkline", "ConnectivityCards", "AIChecks", "Segmented"]) mocks[`../components/${name}`] = { [name]: empty };
  const { MobileHome } = load("mobile/MobileHome.tsx", mocks, {
    window: {}, document: {}, setInterval: () => 1, clearInterval() {},
  });
  let root;
  await act(async () => { root = create(React.createElement(MobileHome)); });
  return { root, store, update: async patch => act(async () => { Object.assign(store.state, patch); root.update(React.createElement(MobileHome)); }) };
}

test("Home translates Android service death in both languages and preserves other errors", async () => {
  for (const [language, expected] of [["en", "The core service stopped or was killed by the system"], ["zh", "内核服务已停止或被系统结束"]]) {
    const m = await mount("crashed", "Android core service stopped or died", language);
    const problem = () => m.root.root.findByProps({ className: "mobile-hero-sub error" }).children.join("");
    assert.equal(problem(), expected);
    await m.update({ coreError: "config.yaml: invalid proxy address" });
    assert.equal(problem(), "config.yaml: invalid proxy address");
    await act(async () => m.root.unmount());
  }
});

test("Home shows the country square only with a running exit IP, including after a crash", async () => {
  const m = await mount("running", "", "zh", { ip: "203.0.113.1", loc: "JP" });
  const country = () => m.root.root.findAllByProps({ className: "mobile-country" });
  assert.equal(country()[0].children.join(""), "JP");
  for (const core of ["crashed", "stopped"]) {
    await m.update({ core });
    assert.equal(country().length, 0);
    assert.equal(m.root.root.findByProps({ className: "mobile-exit-info" }).findByType("strong").children.join(""), "未开启");
  }
  await act(async () => m.root.unmount());
  const withoutIP = await mount("running", "", "zh", { loc: "JP" });
  assert.equal(withoutIP.root.root.findAllByProps({ className: "mobile-country" }).length, 0);
  await act(async () => withoutIP.root.unmount());
});
