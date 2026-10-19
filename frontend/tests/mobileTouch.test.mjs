import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import { test } from "node:test";
import ts from "typescript";

test("long press suppresses its click, but the next SVG icon tap still works", () => {
  const handlers = new Map();
  let timer;
  let cleanup;
  class Element {
    closest() { return null; }
    dispatchEvent() { return false; }
  }
  const exports = {};
  const source = readFileSync(new URL("../src/mobile/useTouchMenus.ts", import.meta.url), "utf8");
  runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText, {
    exports, require: () => ({ useEffect: fn => { cleanup = fn(); } }), Element,
    MouseEvent: class {}, clearTimeout: () => { timer = null; },
    window: { setTimeout: fn => { timer = fn; return 1; } },
    document: { addEventListener: (name, fn) => handlers.set(name, fn), removeEventListener: name => handlers.delete(name) },
  });
  exports.useTouchMenus();
  const down = () => handlers.get("pointerdown")({ pointerType: "touch", target: new Element(), clientX: 20, clientY: 20 });
  let suppressed = false;
  const click = () => handlers.get("click")({ preventDefault: () => { suppressed = true; }, stopPropagation() {} });
  down(); timer(); click();
  assert.equal(suppressed, true);
  down(); timer(); // a menu is dismissed with Back, so no click arrives
  down(); // next tap lands on SVG, an Element but not an HTMLElement
  suppressed = false; click();
  assert.equal(suppressed, false);
  cleanup();
  assert.equal(handlers.size, 0);
});
