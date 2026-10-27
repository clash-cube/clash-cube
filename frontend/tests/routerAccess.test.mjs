import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { test } from "node:test";
import { runInNewContext } from "node:vm";
import React from "react";
import { act, create } from "react-test-renderer";
import ts from "typescript";

const require = createRequire(import.meta.url);

async function mount() {
  const saved = [];
  const changed = [];
  const errors = [];
  let reject = false;
  const Segmented = () => null;
  const mocks = {
    "../api": { Router: { SetAccess: async (policy) => {
      saved.push(policy);
      if (reject) throw new Error("firewall unavailable");
      return { access: policy, enabled: true, active: true, interface: "br-lan" };
    } } },
    "../i18n": { useT: () => (text) => text },
    "./Fold": { Fold: ({ open, children }) => open ? children : null },
    "./Segmented": { Segmented },
    "./Toast": { toastError: (error) => errors.push(error) },
  };
  const source = readFileSync(new URL("../src/components/RouterAccess.tsx", import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, { compilerOptions: {
    module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX,
  } });
  const exports = {};
  runInNewContext(outputText, { exports, require: (id) => mocks[id] ?? require(id) });
  let state = { enabled: true, active: true, interface: "br-lan", access: { mode: "exclude", addresses: ["192.168.1.3/32"], macs: [] } };
  const element = () => React.createElement(exports.RouterAccess, { state, onChange: (value) => changed.push(value) });
  let root;
  await act(async () => { root = create(element()); });
  return {
    saved, changed, errors,
    click: (label) => act(async () => { await root.root.findAllByType("button").find((b) => b.props.children === label).props.onClick(); }),
    type: (index, value) => act(async () => root.root.findAllByType("textarea")[index].props.onChange({ target: { value } })),
    mode: (value) => act(async () => root.root.findByType(Segmented).props.onChange(value)),
    draft: () => root.root.findAllByType("textarea").map((area) => area.props.value),
    poll: () => act(async () => { state = { ...state, access: { mode: "all", addresses: [], macs: [] } }; root.update(element()); }),
    reject: (value) => { reject = value; },
    close: () => act(async () => root.unmount()),
  };
}

test("editing a device policy saves trimmed identities and updates the displayed state", async () => {
  const h = await mount();
  try {
    await h.click("Edit");
    await h.mode("include");
    await h.type(0, " 192.168.1.20\n\n10.0.0.0/24 ");
    await h.type(1, "02:00:00:00:00:01");
    await h.click("Save");
    assert.equal(JSON.stringify(h.saved[0]), JSON.stringify({ mode: "include", addresses: ["192.168.1.20", "10.0.0.0/24"], macs: ["02:00:00:00:00:01"] }));
    assert.equal(h.changed.length, 1);
    assert.equal(h.draft().length, 0);
  } finally { await h.close(); }
});

test("status polling and a rejected save preserve the user's unsaved device list", async () => {
  const h = await mount();
  try {
    await h.click("Edit");
    await h.type(0, "192.168.1.99");
    await h.poll();
    assert.equal(h.draft()[0], "192.168.1.99");
    h.reject(true);
    await h.click("Save");
    assert.equal(h.errors.length, 1);
    assert.equal(h.changed.length, 0);
    assert.equal(h.draft()[0], "192.168.1.99");
    h.reject(false);
    await h.click("Save");
    assert.equal(h.changed.length, 1);
  } finally { await h.close(); }
});
