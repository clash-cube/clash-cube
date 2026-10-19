import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { runInNewContext } from "node:vm";
import { test } from "node:test";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);
function load(file, mocks, globals) {
  const source = readFileSync(new URL(`../src/${file}`, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX } });
  const exports = {};
  runInNewContext(outputText, { exports, require: id => mocks[id] ?? require(id), ...globals });
  return exports;
}

async function mount() {
  const listeners = new Set();
  const entries = [null];
  let at = 0;
  const history = {
    get state() { return entries[at]; },
    replaceState(s) { entries[at] = s; },
    pushState(s) { entries.splice(++at); entries[at] = s; },
    go(n) { at = Math.max(0, at + n); for (const fn of listeners) fn({ state: entries[at] }); },
    back() { this.go(-1); },
  };
  let store = { state: { appVersion: "test", coreVersion: "test" }, setView() {} };
  const useStore = select => select(store);
  useStore.getState = () => store;
  useStore.setState = patch => { store = { ...store, ...patch }; };
  const empty = () => null;
  const mocks = {
    "../store": { useStore }, "../i18n": { useT: () => s => s },
    "../connectionStore": { useConnectionFeed: empty },
    "../components/Icons": new Proxy({}, { get: () => empty }),
    "./MobileHome": { MobileHome: empty }, "./systemBars": { useSystemBars: empty }, "./useTouchMenus": { useTouchMenus: empty },
  };
  for (const name of ["Proxies", "Connections", "Profiles", "Rules", "Logs", "Events", "Usage", "Settings"]) mocks[`../views/${name}`] = { [name]: empty };
  const resizes = new Set();
  const viewport = { height: 830, addEventListener: (_, fn) => resizes.add(fn), removeEventListener: (_, fn) => resizes.delete(fn) };
  const document = { body: {}, activeElement: { tagName: "INPUT" }, querySelector: () => null };
  const window = { visualViewport: viewport, addEventListener: (name, fn) => { if (name === "popstate") listeners.add(fn); }, removeEventListener: (name, fn) => listeners.delete(fn) };
  const { MobileApp } = load("mobile/MobileApp.tsx", mocks, { history, window, document, screen: { height: 854 }, MutationObserver: class { observe() {} disconnect() {} } });
  let root;
  await act(async () => { root = create(React.createElement(MobileApp)); });
  const click = async text => {
    const button = root.root.findAllByType("button").find(b => b.findAllByType("span").some(s => s.children.includes(text)));
    assert.ok(button, `button ${text}`);
    await act(async () => button.props.onClick());
  };
  return { root, history, click, get store() { return store; }, back: async () => act(async () => history.back()), resize: async height => act(async () => { viewport.height = height; for (const fn of resizes) fn(); }) };
}

test("the dock hides for a focused input's resized viewport and returns on dismissal", async () => {
  const m = await mount();
  await m.click("Tools"); await m.click("Logs");
  await m.resize(430);
  assert.equal(m.root.root.findByType("nav").props.hidden, true);
  await m.resize(830);
  assert.equal(m.root.root.findByType("nav").props.hidden, false);
  assert.equal(m.history.state.mobile.page, "logs");
  await act(async () => m.root.unmount());
});

test("switching tabs while searching removes the search entry before changing tabs", async () => {
  const m = await mount();
  await m.click("Tools");
  await m.click("Connections");
  m.history.pushState({ ...m.history.state, search: true });
  await m.click("Profiles");
  assert.equal(m.history.state.mobile.page, "profiles");
  assert.equal(m.history.state.search, undefined);
  await m.back();
  assert.equal(m.history.state.mobile.page, "overview");
  await act(async () => m.root.unmount());
});

test("Connections belongs to the opening tab and Back returns to that tab", async () => {
  const m = await mount();
  await act(async () => m.store.setView("connections"));
  assert.equal(m.history.state.mobile.tab, "overview");
  await m.back();
  assert.equal(m.history.state.mobile.page, "overview");
  await m.click("Tools"); await m.click("Connections");
  assert.equal(m.history.state.mobile.tab, "tools");
  await m.click("Profiles"); await m.click("Tools");
  assert.equal(m.history.state.mobile.page, "connections");
  await m.back();
  assert.equal(m.history.state.mobile.page, "tools");
  await act(async () => m.root.unmount());
});

test("a sheet over search consumes two back presses before leaving the page", async () => {
  const m = await mount();
  await m.click("Proxies");
  m.history.pushState({ ...m.history.state, search: true });
  m.history.pushState({ ...m.history.state, sheet: "test" });
  await m.back();
  assert.equal(m.history.state.search, true);
  assert.equal(m.history.state.mobile.page, "proxies");
  await m.back();
  assert.equal(m.history.state.search, undefined);
  assert.equal(m.history.state.mobile.page, "proxies");
  await m.back();
  assert.equal(m.history.state.mobile.page, "overview");
  await act(async () => m.root.unmount());
});

test("Android back visits settings → Tools → Home without cycling old tabs", async () => {
  const m = await mount();
  await m.click("Tools");
  await m.click("General");
  assert.equal(m.history.state.mobile.settings, "general");
  await m.back();
  assert.equal(m.history.state.mobile.page, "tools");
  await m.click("Proxies");
  await m.click("Profiles");
  await m.back();
  assert.equal(m.history.state.mobile.page, "overview");
  await act(async () => m.root.unmount());
});

test("each tab restores its stack, and tapping the selected tab returns to its root", async () => {
  const m = await mount();
  await m.click("Tools"); await m.click("Network");
  await m.click("Profiles"); await m.click("Tools");
  assert.equal(m.history.state.mobile.settings, "network");
  await m.click("Tools");
  assert.equal(m.history.state.mobile.page, "tools");
  await act(async () => m.store.setView("logs"));
  assert.equal(m.history.state.mobile.page, "logs");
  await m.back(); assert.equal(m.history.state.mobile.page, "tools");
  await act(async () => m.root.unmount());
});

test("a sheet consumes Back before its page, and sheet navigation removes the overlay entry", async () => {
  const m = await mount();
  await m.click("Tools"); await m.click("Network");
  m.history.pushState({ ...m.history.state, sheet: "test" });
  await m.back();
  assert.equal(m.history.state.mobile.settings, "network");
  m.history.pushState({ ...m.history.state, sheet: "test" });
  await act(async () => m.store.setView("proxies"));
  assert.equal(m.history.state.mobile.page, "proxies");
  assert.equal(m.history.state.sheet, undefined);
  await m.back(); assert.equal(m.history.state.mobile.page, "overview");
  await act(async () => m.root.unmount());
});

test("Home refuses to start without a profile and keeps VPN permission errors beside the hero", async () => {
  let state = { core: "stopped", profile: "", mode: "rule" };
  const store = { traffic: {}, history: [], setView() {} };
  const useStore = select => select({ ...store, state });
  const empty = () => null;
  let starts = 0;
  const mocks = {
    "../store": { useStore }, "../i18n": { useT: () => s => s },
    "../api": { App: { Start: async () => { starts++; throw Error("VPN permission was refused"); } } },
    "../components/Icons": new Proxy({}, { get: () => empty }),
    "../format": { bytes: String, speed: String }, "../actions": {},
    "../useGroups": { useGroups: () => ({ groups: null }) },
    "../connectionStore": { useConnectionStore: select => select({ snapshot: { active: [] } }) },
    "../components/TrafficClients": { clientOf: empty },
    "../components/Toast": { errText: e => e.message, toastError: empty },
    "../views/Overview": { Stat: empty, useToday: () => null },
  };
  for (const name of ["Sparkline", "ConnectivityCards", "AIChecks", "Segmented"]) mocks[`../components/${name}`] = { [name]: empty };
  const { MobileHome } = load("mobile/MobileHome.tsx", mocks, {});
  let root;
  await act(async () => { root = create(React.createElement(MobileHome)); });
  const power = () => root.root.findAllByType("button").find(b => b.props.className?.includes("mobile-power"));
  assert.equal(power().props.disabled, true);
  assert.match(JSON.stringify(root.toJSON()), /Import a profile in Profiles first/);
  state = { ...state, profile: "default", profileName: "Default" };
  await act(async () => root.update(React.createElement(MobileHome)));
  assert.equal(power().props.disabled, false);
  await act(async () => power().props.onClick());
  assert.equal(starts, 1);
  assert.match(JSON.stringify(root.toJSON()), /Allow the VPN connection to turn on/);
  assert.equal(power().props.disabled, false);
  await act(async () => root.unmount());
});
