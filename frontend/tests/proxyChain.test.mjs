import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import ts from "typescript";

const source = readFileSync(new URL("../src/proxyChain.ts", import.meta.url), "utf8");
const { outputText } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } });
const { chain } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`);

test("plain nodes are terminal", () => {
  assert.deepEqual(chain("node", []), ["node"]);
});

test("two groups resolve to their selected leaf without changing the groups", () => {
  const groups = Object.freeze([
    Object.freeze({ name: "select", now: "auto" }),
    Object.freeze({ name: "auto", now: "node" }),
  ]);
  assert.deepEqual(chain("select", groups), ["select", "auto", "node"]);
});

test("cycles stop before a repeated name", () => {
  assert.deepEqual(chain("a", [{ name: "a", now: "b" }, { name: "b", now: "a" }]), ["a", "b"]);
  assert.deepEqual(chain("a", [{ name: "a", now: "a" }]), ["a"]);
});

test("missing groups stop traversal at the unresolved name", () => {
  assert.deepEqual(chain("a", [{ name: "a", now: "missing", members: [{ name: "missing", group: true }] }]), ["a", "missing"]);
});

test("empty selections stop traversal", () => {
  assert.deepEqual(chain("a", [{ name: "a", now: "" }]), ["a"]);
  assert.deepEqual(chain("", []), [""]);
});

test("long chains are capped at eight segments", () => {
  const groups = Array.from({ length: 10 }, (_, i) => ({ name: String(i), now: String(i + 1) }));
  assert.deepEqual(chain("0", groups), ["0", "1", "2", "3", "4", "5", "6", "7"]);
});
