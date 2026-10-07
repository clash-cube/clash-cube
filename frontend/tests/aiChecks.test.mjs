import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);

async function mount(settings, savingData = false, state = {}) {
  let store = { settings, state: { core: "running", mode: "rule", network: { savingData }, ...state } };
  const navigation = [];
  const useStore = (select) => select(store);
  useStore.setState = (patch) => { store = { ...store, ...patch }; };
  const calls = [];
  const mocks = {
    "../actions": { openSettings: (...args) => navigation.push(args) },
    "../api": { App: { AICheck: (service, force) => new Promise((resolve, reject) => calls.push({ service, force, resolve, reject })) } },
    "../store": { useStore },
    "../i18n": { useT: () => (text, vars) => text.replace(/\{(\w+)\}/g, (m, k) => vars?.[k] ?? m) },
    "./Icons": { Refresh: () => null, Eye: () => null, Close: () => null, ExternalLink: () => null, Shield: () => null },
    "./Fold": { Fold: ({ open, children }) => open ? children : null },
    "./Popover": { Popover: ({ open, children }) => open ? children : null },
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
  // one module per mount: its result cache lives as long as a window
  const { AIChecks } = load("components/AIChecks.tsx");
  let root;
  const render = () => React.createElement(AIChecks);
  await act(async () => { root = create(render()); });
  return {
    get root() { return root; }, calls, navigation,
    text: () => JSON.stringify(root.toJSON()),
    find: (props) => root.root.findAllByProps(props),
    async update(patch) {
      store = { ...store, ...patch };
      await act(async () => root.update(render()));
    },
    async updateState(patch) {
      store = { ...store, state: { ...store.state, ...patch } };
      await act(async () => root.update(render()));
    },
    async remount() {
      await act(async () => root.unmount());
      await act(async () => { root = create(render()); });
    },
    async close() { await act(async () => root.unmount()); },
  };
}

const host = (name, ...chain) => ({ host: name, chain, rule: "DomainSuffix", rulePayload: name });

test("services checked through their nodes show the node's egress and explain where it comes from", async () => {
  const view = await mount({ aiServices: ["Google AI", "Meta AI"] });
  assert.deepEqual(view.calls.map((c) => c.service), ["Google AI", "Meta AI"]);
  await act(async () => {
    for (const call of view.calls) call.resolve({
      route: { service: call.service, node: "Proxy", verdict: "consistent", auto: [], hosts: [host(call.service === "Google AI" ? "gemini.google.com" : "meta.ai", "Proxy")] },
      nodeEgress: true, status: "consistent", level: "good",
      egress: [{ node: "Proxy", names: 1, ip: "203.0.113.9", loc: "US", chain: ["Proxy"], details: null }],
    });
  });
  assert.equal(view.find({ className: "ai-ip" }).length, 2);
  await act(async () => view.root.root.findAllByProps({ className: "row click" })[0].props.onClick());
  assert.ok(view.text().includes("gemini.google.com"));
  assert.ok(view.text().includes("each node's is asked of Cloudflare through it"));
  assert.ok(!view.text().includes("Each check queries the service through the core for its egress IP."));
  await view.close();
});
const check = (over = {}) => ({
  route: { node: "us1", verdict: "consistent", auto: [], hosts: [host("claude.ai", "us1", "AI"), host("api.anthropic.com", "us1", "AI")] },
  egress: [{ node: "us1", names: 2, ip: "203.0.113.9", loc: "US", chain: ["us1", "AI"], details: null }],
  status: "consistent", level: "good", ...over,
});

test("the leak shield explains and links to leak protection only with the system proxy and no TUN", async () => {
  const view = await mount({ aiServices: ["Claude"] }, false, { systemProxy: true, tun: false });
  assert.equal(view.find({ className: "ai-leak-pop" }).length, 0);
  await act(async () => view.root.root.findByProps({ className: "icon ai-leak-btn" }).props.onClick());
  const pop = view.root.root.findByProps({ className: "ai-leak-pop" });
  assert.ok(view.text().includes("WebRTC sends UDP"));
  await act(async () => pop.findByProps({ className: "btn small" }).props.onClick());
  assert.deepEqual(view.navigation, [["tun", "leak-protection"]]);
  assert.equal(view.find({ className: "ai-leak-pop" }).length, 0);
  await view.updateState({ tun: true });
  assert.equal(view.find({ className: "icon ai-leak-btn" }).length, 0);
  await view.updateState({ tun: false, systemProxy: false });
  assert.equal(view.find({ className: "icon ai-leak-btn" }).length, 0);
  await view.close();
});

test("only selected services are checked, and none hides the section", async () => {
  const view = await mount({ aiServices: [] });
  assert.equal(view.root.toJSON(), null);
  await view.update({ settings: { aiServices: ["Claude", "unknown"] } });
  assert.deepEqual(view.calls.map(({ service, force }) => [service, force]), [["Claude", false]]);
  await view.update({ settings: { aiServices: [] } });
  assert.equal(view.root.toJSON(), null);
  assert.equal(view.calls.length, 1);
  await view.close();
});

test("changing the selection drops replies for the previous one", async () => {
  const view = await mount({ aiServices: ["OpenAI"] });
  await view.update({ settings: { aiServices: ["Claude"] } });
  await act(async () => {
    view.calls[0].resolve(check({ egress: [{ node: "old", ip: "198.51.100.1" }] }));
    view.calls[1].resolve(check());
  });
  assert.ok(view.text().includes("203.0.113.9"));
  assert.ok(!view.text().includes("198.51.100.1"));
  assert.equal(view.root.root.findByProps({ title: "Refresh routes, egress IP and IP attributes" }).props.className, "icon");
  await view.close();
});

test("results outlive the page for their scope, and refresh forces a new check", async () => {
  const view = await mount({ aiServices: ["Claude"] });
  await act(async () => view.calls[0].resolve(check()));
  await view.remount();
  assert.equal(view.calls.length, 1, "returning to Overview checked again");
  assert.ok(view.text().includes("203.0.113.9"));
  await act(async () => view.root.root.findByProps({ title: "Refresh routes, egress IP and IP attributes" }).props.onClick());
  assert.equal(view.calls.length, 2);
  assert.equal(view.calls[1].force, true);
  await view.updateState({ mode: "global" });
  assert.equal(view.calls.length, 3, "another mode wasn't checked");
  await view.close();
});

test("metered networks wait to be asked, per service or for all", async () => {
  const view = await mount({ aiServices: ["OpenAI", "Claude"] }, true);
  assert.equal(view.calls.length, 0);
  assert.ok(view.text().includes("Not checked"));
  const now = view.find({ children: "Check now" }).filter((n) => n.type === "button");
  assert.equal(now.length, 2);
  await act(async () => now[0].props.onClick());
  assert.deepEqual(view.calls.map(({ service, force }) => [service, force]), [["OpenAI", true]]);
  // a second service asked while the first is pending doesn't strand it
  await act(async () => view.find({ children: "Check now" }).filter((n) => n.type === "button")[0].props.onClick());
  await act(async () => { view.calls[0].resolve(check()); view.calls[1].resolve(check()); });
  assert.ok(!view.text().includes("Checking routes…"));
  await view.close();
});

test("the badge weighs the region and shared addresses, not only the routes", async () => {
  const view = await mount({ aiServices: ["OpenAI", "Claude"] });
  await act(async () => {
    view.calls[0].resolve(check({
      status: "unsupported", level: "bad",
      egress: [{ node: "hk1", names: 2, ip: "203.0.113.9", loc: "HK", unsupported: true }],
    }));
    view.calls[1].resolve(check({
      route: { node: "us1", verdict: "split", auto: [], hosts: [host("claude.ai", "us1"), host("sentry.io", "us2")] },
      egress: [{ node: "us1", names: 1, ip: "192.0.2.1" }, { node: "us2", names: 1, ip: "192.0.2.1" }],
    }));
  });
  const badges = view.root.root.findAll((n) => typeof n.props.className === "string" && n.props.className.startsWith("badge "));
  assert.deepEqual(badges.map((b) => [b.props.className, b.props.children]), [["badge bad", "Unsupported region"], ["badge good", "Consistent"]]);
  assert.ok(view.text().includes("OpenAI doesn't serve this region"));
  assert.ok(view.text().includes("2 names leave by 2 nodes that share one egress IP"));
  // several nodes are named on their addresses
  assert.equal(view.find({ className: "ai-ip-node" }).length, 2);
  await view.close();
});

test("IP attributes show as a tag and in the details; hiding masks addresses", async () => {
  const view = await mount({ aiServices: ["Claude"] });
  await act(async () => view.calls[0].resolve(check({
    egress: [{ node: "us1", names: 2, ip: "72.234.229.123", loc: "US", details: { city: "Aiea", region: "Hawaii", operator: "Hawaiian Telcom", asn: 36149, kind: "Datacenter", network: "72.234.229.0/24" } }],
  })));
  assert.equal(view.find({ className: "ai-kind warn" }).length, 1);
  await act(async () => view.root.root.findByProps({ className: "row click" }).props.onClick());
  for (const value of ["Aiea", "Hawaiian Telcom", "AS36149", "claude.ai"]) assert.ok(view.text().includes(value), `missing ${value}`);
  await act(async () => view.root.root.findByProps({ "aria-label": "Hide IP address" }).props.onClick());
  assert.ok(view.text().includes("72.234.*.*"));
  assert.ok(!view.text().includes("72.234.229.123"));
  assert.ok(!view.text().includes("72.234.229.0"));
  await act(async () => view.root.root.findByProps({ "aria-label": "Show IP address" }).props.onClick());
  assert.ok(view.text().includes("72.234.229.123"));
  await view.close();
});
