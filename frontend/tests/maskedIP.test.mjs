import assert from "node:assert/strict";
import { test } from "node:test";
import { maskedIP } from "../src/format.ts";

test("mask IPs and CIDRs including compressed and mapped IPv6 without leaking host bits", () => {
  assert.equal(maskedIP("14.127.12.34"), "14.127.*.*");
  assert.equal(maskedIP("14.127.12.0/24"), "14.127.*.*/24");
  assert.equal(maskedIP("2001:db8:abcd::1234"), "2001:db8:*:*");
  assert.ok(!maskedIP("::ffff:192.168.12.34").includes("192.168.12.34"));
  assert.ok(!maskedIP("2001::abcd").includes("abcd"));
  assert.equal(maskedIP("invalid"), "***");
});
