import assert from "node:assert/strict";
import { test } from "node:test";
import { defaultConnectionPreferences, parseConnectionPreferences } from "../src/connectionPreferences.ts";

test("display preferences survive a round trip and recover invalid fields independently", () => {
  const wanted = { by: "source", net: "udp", sort: "download", ascending: true };
  assert.deepEqual(parseConnectionPreferences(JSON.stringify(wanted)), wanted);
  assert.deepEqual(parseConnectionPreferences(JSON.stringify({ ...wanted, by: "removed", net: 1, ascending: "false" })),
    { ...defaultConnectionPreferences, sort: "download" });
  for (const broken of [null, "{", "null", "[]", "42"]) {
    assert.deepEqual(parseConnectionPreferences(broken), defaultConnectionPreferences);
  }
});
