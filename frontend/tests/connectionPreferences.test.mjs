import assert from "node:assert/strict";
import { test } from "node:test";
import { defaultConnectionPreferences, parseConnectionPreferences } from "../src/connectionPreferences.ts";
import { clampColumnWidth } from "../src/connectionColumns.ts";

test("display preferences survive a round trip and recover invalid fields independently", () => {
  const wanted = { ...defaultConnectionPreferences, by: "source", net: "udp", sort: "download", ascending: true };
  assert.deepEqual(parseConnectionPreferences(JSON.stringify(wanted)), wanted);
  assert.deepEqual(parseConnectionPreferences(JSON.stringify({ ...wanted, by: "removed", net: 1, ascending: "false" })),
    { ...defaultConnectionPreferences, sort: "download" });
  for (const broken of [null, "{", "null", "[]", "42"]) {
    assert.deepEqual(parseConnectionPreferences(broken), defaultConnectionPreferences);
  }
});

test("old preferences migrate and malformed column layouts cannot hide destinations or break widths", () => {
  const old = parseConnectionPreferences('{"by":"source","sort":"up"}');
  assert.equal(old.by, "source");
  assert.deepEqual(old.columns, defaultConnectionPreferences.columns);
  const saved = parseConnectionPreferences(JSON.stringify({ columns: ["process", "process", "unknown", "total"],
    widths: { host: -100, process: 999999, total: "wide", rule: null, unknown: 500 } }));
  assert.deepEqual(saved.columns, ["host", "process", "total"]);
  assert.deepEqual(saved.widths, { host: 140, process: 600 });
  assert.deepEqual(parseConnectionPreferences(JSON.stringify(saved)), saved);
  assert.deepEqual(parseConnectionPreferences('{"columns":[]}').columns, ["host"]);
  assert.equal(clampColumnWidth("host", Infinity), 220);
});
