import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);

async function mount(settings, savingData = false, settingProps = null, state = {}) {
  let store = { settings, state: { core: "running", network: { savingData }, ...state } };
  const navigation = [];
  const useStore = (select) => select(store);
  useStore.setState = (patch) => { store = { ...store, ...patch }; };
  const calls = [];
  const request = (kind) => (service, force) => new Promise((resolve) => calls.push({ kind, service, force, resolve }));
  const mocks = {
    "../actions": { openSettings: (...args) => navigation.push(args) },
    "../api": { App: { AIRoutes: request("routes"), AIEgress: request("egress") } },
    "../store": { useStore },
    "../i18n": { useT: () => (text) => text },
    "../format": { flagged: (ip) => ip },
    "./Icons": { Refresh: () => null, Chevron: () => null, Eye: () => null, Close: () => null, ExternalLink: () => null },
    "./Switch": { Switch: ({ onChange }) => React.createElement("button", { role: "switch", onClick: () => onChange(false) }) },
    "./Fold": { Fold: ({ open, children }) => open ? children : null },
    "./ConnectivityCards": { route: (chain) => chain.join(" → ") },
  };
  function load(file) {
    const source = readFileSync(new URL(`../src/${file}`, import.meta.url), "utf8");
    const { outputText } = ts.transpileModule(source, {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    });
    const exports = {};
    runInNewContext(outputText, { exports, require: (id) => id.endsWith(".svg") ? { default: id } : mocks[id] ?? require(id) });
    return exports;
  }
  mocks["../aiServices"] = load("aiServices.ts");
  mocks["../format"] = load("format.ts");
  mocks["./AIServiceName"] = load("components/AIServiceName.tsx");
  mocks["./AIChecks"] = load("components/AIChecks.tsx");
  const Component = settingProps ? load("components/AIServiceSetting.tsx").AIServiceSetting : mocks["./AIChecks"].AIChecks;
  let root;
  await act(async () => { root = create(React.createElement(Component, settingProps)); });
  return {
    root, calls, navigation,
    async update(settings) {
      store = { ...store, settings };
      await act(async () => root.update(React.createElement(Component, settingProps)));
    },
    async close() { await act(async () => root.unmount()); },
    async remount() {
      await act(async () => root.unmount());
      await act(async () => { root = create(React.createElement(Component, settingProps)); });
      this.root = root;
    },
    async updateState(state) {
      store = { ...store, state: { ...store.state, ...state } };
      await act(async () => root.update(React.createElement(Component, settingProps)));
    },
  };
}

test("AI services warning links to leak protection only with system proxy and no TUN", async () => {
  const view = await mount({ aiChecks: true, aiServices: ["Claude"] }, false, null, { systemProxy: true, tun: false });
  const hint = view.root.root.findByProps({ "aria-label": "Leak Protection" });
  await act(async () => hint.props.onClick());
  assert.deepEqual(view.navigation, [["tun", "leak-protection"]]);
  await view.updateState({ tun: true });
  assert.equal(view.root.root.findAllByProps({ className: "ai-leak-hint" }).length, 0);
  await view.updateState({ tun: false, systemProxy: false });
  assert.equal(view.root.root.findAllByProps({ className: "ai-leak-hint" }).length, 0);
  await view.close();
});

test("dismissing the header hint lasts across page visits but not a fresh session", async () => {
  const settings = { aiChecks: true, aiServices: ["Claude"] };
  const state = { systemProxy: true, tun: false };
  const view = await mount(settings, false, null, state);
  await act(async () => view.root.root.findByProps({ "aria-label": "Dismiss until next launch" }).props.onClick());
  await view.remount();
  assert.equal(view.root.root.findAllByProps({ className: "ai-leak-hint" }).length, 0);
  assert.equal(view.navigation.length, 0);
  await view.close();
  const fresh = await mount(settings, false, null, state);
  assert.equal(fresh.root.root.findAllByProps({ className: "ai-leak-hint" }).length, 1);
  await fresh.close();
});

test("AI checks stay off until enabled and only request selected services", async () => {
  const view = await mount({ aiChecks: false, aiServices: ["Claude"] });
  assert.equal(view.root.toJSON(), null);
  assert.equal(view.calls.length, 0);
  await view.update({ aiChecks: true, aiServices: ["Claude", "unknown"] });
  assert.deepEqual(view.calls.map(({ kind, service }) => [kind, service]), [["routes", "Claude"], ["egress", "Claude"]]);
  await view.update({ aiChecks: true, aiServices: [] });
  assert.equal(view.root.toJSON(), null);
  assert.equal(view.calls.length, 2);
  await view.close();
});

test("settings expansion checks domains without changing selection and ignores collapsed requests", async () => {
  let toggles = 0;
  const view = await mount({ aiChecks: true, aiServices: ["Claude"] }, false,
    { service: "Claude", on: true, disabled: false, onChange: async () => { toggles++; } });
  const disclosure = () => view.root.root.findByProps({ "aria-controls": "ai-details-Claude" });
  assert.equal(view.calls.length, 0);
  await act(async () => view.root.root.findByProps({ role: "switch" }).props.onClick());
  assert.equal(toggles, 1);
  assert.equal(disclosure().props["aria-expanded"], false);
  await act(async () => disclosure().props.onClick());
  assert.deepEqual(view.calls.map(({ kind, service }) => [kind, service]), [["routes", "Claude"]]);
  await act(async () => disclosure().props.onClick());
  await act(async () => disclosure().props.onClick());
  await act(async () => {
    view.calls[1].resolve({ node: "proxy", hosts: [{ host: "api.anthropic.com", chain: ["proxy", "AI"], rule: "DomainSuffix", rulePayload: "anthropic.com" }] });
    view.calls[0].resolve({ node: "old", hosts: [{ host: "stale.test", chain: ["old"] }] });
  });
  const rendered = JSON.stringify(view.root.toJSON());
  assert.ok(rendered.includes("api.anthropic.com"));
  assert.ok(rendered.includes("DomainSuffix"));
  assert.ok(rendered.includes("proxy → AI"));
  assert.ok(!rendered.includes("stale.test"));
  assert.equal(toggles, 1);
  await view.close();
});

test("changing selection or disabling discards pending results", async () => {
  const view = await mount({ aiChecks: true, aiServices: ["OpenAI"] });
  const old = [...view.calls];
  await view.update({ aiChecks: true, aiServices: ["Claude"] });
  await act(async () => {
    old[0].resolve({ hosts: [], verdict: "failed" });
    old[1].resolve({ ip: "old-egress" });
    view.calls[2].resolve({ hosts: [], verdict: "failed" });
    view.calls[3].resolve({ ip: "new-egress" });
  });
  const rendered = JSON.stringify(view.root.toJSON());
  assert.ok(rendered.includes("new-egress"));
  assert.ok(!rendered.includes("old-egress"));
  assert.equal(view.root.root.findByProps({ title: "Refresh routes, egress IP and IP attributes" }).props.className, "icon");
  await view.update({ aiChecks: false, aiServices: ["Claude"] });
  assert.equal(view.root.toJSON(), null);
  assert.equal(view.calls.length, 4);
  await view.close();
});

test("metered networks wait for manual refresh and refresh only the selection", async () => {
  const view = await mount({ aiChecks: true, aiServices: ["OpenAI"] }, true);
  assert.equal(view.calls.length, 0);
  assert.ok(JSON.stringify(view.root.toJSON()).includes("Not checked"));
  await act(async () => view.root.root.findByType("button").props.onClick());
  assert.deepEqual(view.calls.map(({ service }) => service), ["OpenAI", "OpenAI"]);
  assert.equal(view.calls[1].force, true);
  await view.close();
});

test("IP attributes enrich the observed egress and remain optional", async () => {
  const view = await mount({ aiChecks: true, aiServices: ["OpenAI", "Claude"] });
  await act(async () => {
    view.calls.find((c) => c.service === "OpenAI" && c.kind === "egress").resolve({
      ip: "72.234.229.123", details: { city: "Aiea", region: "Hawaii", operator: "Hawaiian Telcom", asn: 36149, kind: "Residential", network: "72.234.229.0/24" },
    });
    view.calls.find((c) => c.service === "Claude" && c.kind === "egress").resolve({ ip: "203.0.113.9", details: null });
  });
  const rendered = JSON.stringify(view.root.toJSON());
  for (const value of ["72.234.229.123", "Aiea", "Hawaiian Telcom", "AS36149", "Residential", "203.0.113.9", "IP attributes unavailable"]) {
    assert.ok(rendered.includes(value), `missing ${value}`);
  }
  await view.close();
});

test("automatic checks reuse cache and hiding masks both IPs and network data", async () => {
  const view = await mount({ aiChecks: true, aiServices: ["Claude"] });
  assert.equal(view.calls[1].force, false);
  await act(async () => {
    view.calls[1].resolve({ ip: "72.234.229.123", details: { kind: "Residential", network: "72.234.229.0/24" } });
  });
  let stopped = false;
  await act(async () => view.root.root.findByProps({ "aria-label": "Hide IP address" }).props.onClick({ stopPropagation() { stopped = true; } }));
  assert.ok(stopped);
  const rendered = JSON.stringify(view.root.toJSON());
  assert.ok(rendered.includes("72.234.*.*"));
  assert.ok(!rendered.includes("72.234.229.123"));
  assert.ok(!rendered.includes("72.234.229.0"));
  await act(async () => view.root.root.findByProps({ "aria-label": "Show IP address" }).props.onClick({ stopPropagation() {} }));
  assert.ok(JSON.stringify(view.root.toJSON()).includes("72.234.229.123"));
  await view.close();
});
