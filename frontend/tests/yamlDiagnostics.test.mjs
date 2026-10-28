import assert from "node:assert/strict";
import { test } from "node:test";
import { checkYaml } from "../src/components/yamlDiagnostics.ts";

test("the YAML checker accepts comments, Unicode, block strings and aliases without expanding values", () => {
  assert.deepEqual(checkYaml('# 中文配置\nbase: &base {type: socks5, port: 1080}\nproxies:\n  - *base\nnotes: |\n  rules: [ is text here\nrules: ["MATCH,DIRECT"]\n'), []);
  assert.deepEqual(checkYaml('a: &a [*a]\n'), []);
});

test("duplicate keys identify the source location and malformed YAML has bounded editor ranges", () => {
  const duplicate = checkYaml('# 中文\nrules: []\nrules: ["MATCH,DIRECT"]\n');
  assert.equal(duplicate.length, 1);
  assert.equal(duplicate[0].line, 3);
  assert.equal(duplicate[0].column, 1);
  for (const source of ['rules: [', 'proxies:\n\t- name: broken\n', 'notes: "unterminated\n']) {
    const problems = checkYaml(source);
    assert.ok(problems.length > 0);
    for (const p of problems) {
      assert.ok(p.from >= 0 && p.from <= p.to && p.to <= source.length);
      assert.ok(p.line >= 1 && p.column >= 1 && p.message.length > 0);
    }
  }
});
