import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);
async function mount({ ping = async () => ({ milliseconds: 12, path: "direct" }), state = { mode: "rule", tun: false, core: "running" }, bypass = ["192.168.0.0/16"], fresh = false } = {}) {
  let status = { name: "ClashCube Tailscale identity-001", state: "Running", ips: ["100.64.0.1"], health: [], peers: [
    { id: "router", name: "router", dnsName: "router.tail.ts.net", ips: ["100.64.0.2"], routes: ["192.168.50.0/24"], online: true, path: "direct" },
  ] };
  const module = { id: fresh ? "" : "saved", name: "Private", enabled: true, body: "", profile: "", tailscale: { identity: "identity-001", hostname: "clashcube", routes: ["192.168.50.0/24"] } };
  const saved = [], opened = [], copied = [], pings = [];
  const errors = [], timers = new Set();
  let logins = 0, logouts = 0, modes = [];
  const t = (key, values = {}) => key.replace(/\{(\w+)\}/g, (_, name) => String(values[name] ?? `{${name}}`));
  const mocks = {
    "../../api": { App: {
      TailscaleLocalNetworks: async () => ["192.168.1.0/24"],
      TailscaleLogin: async () => { logins++; }, TailscaleLogout: async () => { logouts++; },
      TailscaleHostname: async () => "studio", SetMode: async (m) => { modes.push(m); },
      OpenURL: async (url) => { opened.push(url); },
      CopyText: async (text) => { copied.push(text); return true; },
      TailscalePing: async (name, ip) => { pings.push({ name, ip }); return ping(); },
    } },
    "../../store": { useStore: (select) => select({ state: { ...state, tailscales: status ? [status] : [] }, settings: { bypass } }) },
    "../../actions": { setMode: (m) => { modes.push(m); }, openSettings() {} },
    "../../format": { delayClass: () => "good" },
    "zustand": require("zustand"),
    "../../i18n": { useT: () => t },
    "../Fold": { Fold: ({ open, children }) => open ? children : null },
    "../Icons": Object.fromEntries(["Bolt", "Chevron", "Copy", "Search"].map((name) => [name, () => React.createElement("svg")])),
    "../Switch": { Switch: ({ label, on, onChange }) => React.createElement("button", { type: "button", "aria-label": label, "aria-checked": on, onClick: () => onChange(!on) }) },
    "../Toast": { toast() {}, toastError(err) { errors.push(err); } },
    "./shared": { FormOrYAML: ({ Form, ...props }) => React.createElement(Form, props) },
    "../../../bindings/github.com/localhost-copilot/clashcube/internal/modules/models": {},
  };
  // Motion hooks run normally; reduced motion keeps these behavior assertions synchronous.
  for (const name of ["useLeaving", "useFlip"]) {
    const source = readFileSync(new URL(`../src/${name}.ts`, import.meta.url), "utf8");
    const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } });
    const exports = {};
    runInNewContext(outputText, { exports, require: (id) => id === "./components/Sidebar" ? { calm: () => true } : require(id), setTimeout, clearTimeout });
    mocks[`../../${name}`] = exports;
  }
  const source = readFileSync(new URL("../src/components/modules/tailscale.tsx", import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX } });
  const exports = {};
  runInNewContext(outputText, { exports, require: (id) => mocks[id] ?? require(id), URL, Date, crypto,
    setTimeout: (fn) => { timers.add(fn); return fn; }, clearTimeout: (fn) => timers.delete(fn),
  });
  const Editor = exports.tailscaleKind.Editor;
  const { Summary } = exports.tailscaleKind;
  // the row's summary is always shown; it opens the login page
  const view = () => React.createElement(React.Fragment, null,
    React.createElement(Summary, { m: module, ctx: {} }),
    React.createElement(Editor, { module, open: true, onSave: async (m) => saved.push(m), onCancel() {} }));
  let root;
  await act(async () => { root = create(view()); });
  const button = (text) => root.root.findAllByType("button").find((b) => b.children.includes(text));
  return {
    root, saved, opened, copied, pings, errors, modes, kind: exports.tailscaleKind, get logins() { return logins; }, get logouts() { return logouts; }, button,
    text: () => JSON.stringify(root.toJSON()),
    expireLogin: () => act(async () => { for (const fn of [...timers]) { timers.delete(fn); fn(); } }),
    update: (next) => act(async () => { status = next; root.update(view()); }),
    get status() { return status; },
    close: () => act(async () => root.unmount()),
  };
}

const many = (router) => [router, ...Array.from({ length: 8 }, (_, i) => ({ ...router, id: `p${i}`, name: `peer-${i}`, dnsName: `peer-${i}.tail.ts.net`, ips: [`100.64.1.${i}`], routes: [] }))];

test("a picked subnet stays picked and saved after its device disappears", async () => {
  const h = await mount();
  try {
    const route = () => h.root.root.findByProps({ "aria-label": "192.168.50.0/24", role: undefined });
    assert.equal(route().props["aria-checked"], true);
    await h.update({ ...h.status, peers: [] });
    assert.equal(route().props["aria-checked"], true);
    assert.match(h.text(), /not advertised right now/);
    assert.equal(h.saved.length, 0);
    await act(async () => h.root.root.findByType("form").props.onSubmit({ preventDefault() {} }));
    assert.deepEqual(Array.from(h.saved[0].tailscale.routes), ["192.168.50.0/24"]);
  } finally { await h.close(); }
});

test("subnets are switched in one place and typed ones join them", async () => {
  const h = await mount();
  try {
    const sw = h.root.root.findByProps({ "aria-label": "192.168.50.0/24" });
    await act(async () => sw.props.onClick());
    await act(async () => h.root.root.findByType("textarea").props.onChange({ target: { value: "10.9.0.0/16" } }));
    await act(async () => h.root.root.findByType("form").props.onSubmit({ preventDefault() {} }));
    assert.deepEqual(Array.from(h.saved[0].tailscale.routes), ["10.9.0.0/16"]);
    assert.equal(h.root.root.findAllByProps({ "aria-label": "Add 192.168.50.0/24 to remote subnets" }).length, 0);
  } finally { await h.close(); }
});

test("search shows only for long device lists and finds hidden addresses", async () => {
  const h = await mount();
  try {
    assert.equal(h.root.root.findAllByProps({ "aria-label": "Search devices" }).length, 0);
    const router = { ...h.status.peers[0], ips: ["fd7a:115c:a1e0::2", "100.64.0.2"] };
    await h.update({ ...h.status, peers: many(router) });
    const rows = () => h.root.root.findAllByProps({ role: "listitem" });
    const search = h.root.root.findByProps({ "aria-label": "Search devices" });
    for (const query of ["ROUTER.TAIL.TS.NET", "fd7a:115c:a1e0::2"]) {
      await act(async () => search.props.onChange({ target: { value: query } }));
      assert.equal(rows().length, 1);
      assert.equal(rows()[0].findByProps({ className: "sub mono" }).children[0], "100.64.0.2");
    }
    await act(async () => search.props.onChange({ target: { value: "missing" } }));
    assert.match(h.text(), /No matching devices/);
  } finally { await h.close(); }
});

test("details copy an address without saving the form", async () => {
  const h = await mount();
  try {
    await h.update({ ...h.status, peers: [{ ...h.status.peers[0], ips: ["fd7a:115c:a1e0::2", "100.64.0.2"] }] });
    const row = h.root.root.findAllByProps({ role: "listitem" })[0];
    assert.equal(h.root.root.findAllByType("dd").length, 0);
    await act(async () => row.findByProps({ "aria-expanded": false }).props.onClick());
    await act(async () => h.root.root.findByProps({ "aria-label": "Copy IPv6" }).props.onClick());
    assert.deepEqual(h.copied, ["fd7a:115c:a1e0::2"]);
    assert.equal(h.saved.length, 0);
  } finally { await h.close(); }
});

test("a device check shows its latency on the row, and recovers after failure", async () => {
  let settle;
  const h = await mount({ ping: () => new Promise((resolve, reject) => { settle = { resolve, reject }; }) });
  try {
    await h.update({ ...h.status, peers: [{ ...h.status.peers[0], ips: ["fd7a:115c:a1e0::2", "100.64.0.2"] }] });
    const check = () => h.root.root.findByProps({ "aria-label": "Check connection to router" });
    let pending;
    await act(async () => { pending = check().props.onClick(); });
    assert.equal(check().props.disabled, true);
    assert.deepEqual(h.pings, [{ name: "ClashCube Tailscale identity-001", ip: "100.64.0.2" }]);
    await act(async () => { settle.resolve({ milliseconds: 31.6, path: "relay" }); await pending; });
    assert.match(h.text(), /"32"," ms"/);
    assert.match(h.text(), /Relayed/);
    await act(async () => { pending = check().props.onClick(); });
    await act(async () => { settle.reject(new Error("Device unreachable")); await pending; });
    assert.equal(check().props.disabled, false);
    assert.equal(h.errors[0].message, "Device unreachable");
    assert.equal(h.saved.length, 0);
  } finally { await h.close(); }
});

test("one login action shows at a time, and logging out asks twice", async () => {
  const h = await mount();
  try {
    assert.equal(h.button("Log in"), undefined);
    await act(async () => h.button("Log out").props.onClick());
    assert.equal(h.logouts, 0);
    await act(async () => h.button("Click again to log out").props.onClick());
    assert.equal(h.logouts, 1);
    await h.update({ ...h.status, state: "NeedsLogin", ips: [] });
    assert.ok(h.button("Log in"));
    assert.equal(h.button("Log out"), undefined);
    await h.update({ ...h.status, authURL: "https://login.tailscale.com/a/test" });
    assert.ok(h.button("Open login page"));
    assert.equal(h.button("Log in"), undefined);
  } finally { await h.close(); }
});

test("login pages open only after a login action; incoming auth URLs alone never open a browser", async () => {
  const h = await mount();
  try {
    await h.update({ ...h.status, state: "NeedsLogin" });
    await act(async () => { await h.button("Log in").props.onClick(); });
    assert.equal(h.logins, 1);
    await h.update({ ...h.status, authURL: "https://login.tailscale.com/a/test" });
    assert.deepEqual(h.opened, ["https://login.tailscale.com/a/test"]);
    await h.update({ ...h.status, authURL: "https://login.tailscale.com/a/another" });
    assert.equal(h.opened.length, 1);
  } finally { await h.close(); }
});

test("a missing login link times out and leaves login available for retry", async () => {
  const h = await mount();
  try {
    await h.update({ ...h.status, state: "NeedsLogin" });
    await act(async () => { await h.button("Log in").props.onClick(); });
    assert.ok(h.button("Waiting for login link…").props.disabled);
    await h.expireLogin();
    assert.equal(h.button("Log in").props.disabled, false);
    assert.equal(h.errors.length, 1);
    await act(async () => { await h.button("Log in").props.onClick(); });
    assert.equal(h.logins, 2);
  } finally { await h.close(); }
});

test("an unsafe auth URL is shown as an error and never sent to the browser", async () => {
  const h = await mount();
  try {
    await h.update({ ...h.status, state: "NeedsLogin" });
    await act(async () => { await h.button("Log in").props.onClick(); });
    await h.update({ ...h.status, authURL: "file:///etc/passwd" });
    assert.equal(h.opened.length, 0);
    assert.equal(h.errors.length, 1);
  } finally { await h.close(); }
});

test("adding a module logs in once the core runs it", async () => {
  const h = await mount({ fresh: true });
  try {
    await h.update(null);
    assert.ok(h.button("Add and log in"));
    await act(async () => h.root.root.findByType("form").props.onSubmit({ preventDefault() {} }));
    assert.equal(h.saved.length, 1);
    assert.equal(h.logins, 0);
    await h.update({ name: "ClashCube Tailscale identity-001", state: "NeedsLogin", ips: [], health: [], peers: [] });
    assert.equal(h.logins, 1);
    await h.update({ ...h.status, authURL: "https://login.tailscale.com/a/new" });
    assert.deepEqual(h.opened, ["https://login.tailscale.com/a/new"]);
  } finally { await h.close(); }
});

test("a changed hostname says the core restarts, and a bad one says why", async () => {
  const h = await mount();
  try {
    const host = () => h.root.root.findByProps({ "aria-label": "Hostname" });
    await act(async () => host().props.onChange({ target: { value: "studio" } }));
    assert.match(h.text(), /Saving restarts the core to reconnect/);
    await act(async () => host().props.onChange({ target: { value: "-bad" } }));
    assert.match(h.text(), /Use up to 63 letters/);
    assert.equal(h.button("Save").props.disabled, true);
  } finally { await h.close(); }
});

test("only what is in the way is said: Rule mode, then a bypass that reaches the tailnet", async () => {
  const global = await mount({ state: { mode: "global", tun: false, core: "running" } });
  try {
    assert.match(global.text(), /Rule mode only/);
    await act(async () => global.button("Switch to Rule mode").props.onClick());
    assert.deepEqual(global.modes, ["rule"]);
  } finally { await global.close(); }
  const lan = await mount();
  try {
    assert.match(lan.text(), /skips 192\.168\.0\.0\/16/);
    assert.doesNotMatch(lan.text(), /TUN is off/);
  } finally { await lan.close(); }
  const clear = await mount({ bypass: ["127.0.0.1", "10.0.0.0/8", "*.local"] });
  try { assert.doesNotMatch(clear.text(), /skips/); } finally { await clear.close(); }
});
