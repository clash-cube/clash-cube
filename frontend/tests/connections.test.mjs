import assert from "node:assert/strict";
import { test } from "node:test";
import { ConnectionTracker, filterConnections, compareConnections, groupConnections, holdOrder, lingering } from "../src/connections.ts";
import { duration } from "../src/format.ts";

const conn = (id, overrides = {}) => ({ id, metadata: { network: "tcp", type: "HTTP", sourceIP: "192.168.1.5",
  destinationIP: "203.0.113.7", sourcePort: "52000", destinationPort: "443", host: "example.test",
  sniffHost: "sniff.test", remoteDestination: "203.0.113.8", process: "curl", processPath: "/usr/bin/curl" },
  start: new Date(0).toISOString(), upload: 0, download: 0, chains: ["node", "group"], rule: "Domain", rulePayload: "example.test", ...overrides });
const deferred = () => { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; };

test("column sorting uses raw totals and stable source addresses, independent of formatted text", () => {
  const a = { ...conn("a", { upload: 1024, download: 1 }), up: 10, down: 20 };
  const b = { ...conn("b", { upload: 100, download: 800 }), up: 20, down: 10,
    metadata: { ...a.metadata, sourceIP: "192.168.1.15", process: "browser" } };
  assert.deepEqual([a, b].sort(compareConnections("total", true)).map((c) => c.id), ["b", "a"]);
  assert.deepEqual([b, a].sort(compareConnections("source", true)).map((c) => c.id), ["a", "b"]);
  assert.deepEqual([a, b].sort(compareConnections("process", true)).map((c) => c.id), ["b", "a"]);
});

test("device groups keep IP identity and aggregate only filtered members in sorted order", () => {
  const a = { ...conn("a"), up: 10, down: 20, upload: 100, download: 200 };
  const b = { ...conn("b"), up: 30, down: 40, upload: 300, download: 400 };
  const c = { ...a, id: "c", metadata: { ...a.metadata, sourceIP: "192.168.1.6" } };
  const unknown = { ...a, id: "unknown", metadata: { ...a.metadata, sourceIP: "" } };
  const labels = { "192.168.1.5": "Office", "192.168.1.6": "Office" };
  const filtered = filterConnections([c, b, a, unknown], "Office", "all", new Set(), labels);
  const groups = groupConnections(filtered, "source");
  assert.deepEqual(groups.map((g) => g.name), ["192.168.1.6", "192.168.1.5"]);
  assert.deepEqual(groups[1].list.map((c) => c.id), ["b", "a"]);
  assert.deepEqual([groups[1].up, groups[1].down, groups[1].total], [40, 60, 1000]);
  assert.equal(groupConnections([unknown], "source")[0].name, "");
});

test("source selection intersects protocol and label search, including archived connections", () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("a"), conn("b", { metadata: { ...conn("").metadata, sourceIP: "192.168.1.6" } })], 1000);
  tracker.collect([], 2000);
  const labels = { "192.168.1.5": "Office Mac" };
  const selected = new Set(["192.168.1.5"]);
  assert.deepEqual(filterConnections(tracker.snapshot.closed, "office curl", "tcp", selected, labels).map((c) => c.id), ["a"]);
  assert.equal(filterConnections(tracker.snapshot.closed, "office", "udp", selected, labels).length, 0);
  assert.equal(filterConnections(tracker.snapshot.closed, "", "all", new Set(["missing"]), labels).length, 0);
  assert.equal(filterConnections(tracker.snapshot.closed, "", "all", new Set(), labels).length, 2);
});

test("history preserves last counters, freezes ended duration and keeps only 500 records", () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("upload")], 1000);
  tracker.collect([conn("upload", { upload: 3000 })], 2000);
  assert.equal(tracker.snapshot.active[0].up, 3000);
  assert.equal(tracker.snapshot.active[0].down, 0);
  const frozen = tracker.snapshot;
  tracker.collect([], 3000);
  tracker.collect([], 4000);
  assert.equal(tracker.snapshot.closed.length, 1);
  const ended = tracker.snapshot.closed[0];
  assert.equal(ended.upload, 3000);
  assert.equal(ended.up, 0);
  assert.equal(duration(ended.start, ended.closedAt), "3s");
  assert.equal(frozen.active.length, 1, "a paused snapshot must stay immutable");
  assert.equal(frozen.active[0].closedAt, undefined);
  for (let i = 0; i < 501; i++) {
    tracker.collect([conn(String(i))], 5000 + i * 1000);
    tracker.collect([], 5500 + i * 1000);
  }
  assert.equal(tracker.snapshot.closed.length, 500);
  assert.equal(tracker.snapshot.closed[0].id, "1");
  assert.equal(tracker.snapshot.closed.at(-1).id, "500");
});

test("multi-word search combines source, port, sniffed host, process and chain", () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("match"), conn("other", { metadata: { ...conn("").metadata, network: "udp", sourceIP: "192.168.1.6" } })], 1000);
  assert.deepEqual(filterConnections(tracker.snapshot.active, "192.168.1.5:52000 SNIFF 443 curl group", "tcp").map((c) => c.id), ["match"]);
  assert.equal(filterConnections(tracker.snapshot.active, "[", "all").length, 0, "literal input is not a regex");
});

test("filtered close preserves hidden and newly arrived connections; failures remain retryable", async () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("wanted"), conn("hidden", { metadata: { ...conn("").metadata, process: "browser", processPath: "/browser" } })], 1000);
  const ids = filterConnections(tracker.snapshot.active, "curl", "all").map((c) => c.id);
  tracker.collect([...tracker.snapshot.active, conn("new")], 2000);
  const calls = [];
  let failures = await tracker.close(ids, async (id) => { calls.push(id); throw new Error("refused"); });
  assert.deepEqual(calls, ["wanted"]);
  assert.equal(failures.length, 1);
  assert.equal(tracker.snapshot.active.length, 3);
  assert.equal(tracker.closing.size, 0);
  failures = await tracker.close(ids, async () => {});
  assert.equal(failures.length, 0);
  assert.deepEqual(tracker.snapshot.active.map((c) => c.id), ["hidden", "new"]);
  assert.equal(tracker.snapshot.closed[0].id, "wanted");
});

test("stale reads cannot resurrect a successful close or overwrite a newer snapshot", async () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("a")], 1000);
  const old = deferred();
  const request = tracker.refresh(() => old.promise);
  await tracker.close(["a"], async () => {});
  old.resolve([conn("a")]);
  await request;
  assert.equal(tracker.snapshot.active.length, 0);
  const older = deferred();
  const pending = tracker.refresh(() => older.promise);
  await tracker.refresh(async () => [conn("latest")], () => 3000);
  older.resolve([conn("stale")]);
  await pending;
  assert.equal(tracker.snapshot.active[0].id, "latest");
  await assert.rejects(tracker.refresh(async () => { throw new Error("offline"); }));
  assert.equal(tracker.snapshot.active[0].id, "latest", "a failed fetch is not a closed connection");
});

test("duplicate closes share pending state and old-session completion cannot close a reused ID", async () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("a")], 1000);
  const response = deferred();
  const pending = tracker.close(["a", "a"], () => response.promise);
  await tracker.close(["a"], async () => assert.fail("duplicate request"));
  assert.equal(tracker.closing.size, 1);
  tracker.reset();
  tracker.collect([conn("a", { upload: 700 })], 2000);
  response.resolve();
  await pending;
  assert.equal(tracker.snapshot.active[0].upload, 700);
  assert.equal(tracker.snapshot.closed.length, 0);
});

test("speed uses elapsed time, clamps counter resets and sorts with a stable tie break", () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("b"), conn("a")], 1000);
  tracker.collect([conn("b", { upload: 6000 }), conn("a", { upload: 1000 })], 3000);
  assert.equal(tracker.snapshot.active[0].up, 3000);
  assert.deepEqual([...tracker.snapshot.active].sort(compareConnections("upload", false)).map((c) => c.id), ["b", "a"]);
  tracker.collect([conn("b"), conn("a")], 4000);
  assert.equal(tracker.snapshot.active[0].up, 0);
  assert.deepEqual([...tracker.snapshot.active].sort(compareConnections("up", false)).map((c) => c.id), ["a", "b"]);
});

test("the speed column sorts by upload and download together", () => {
  const a = { ...conn("a"), up: 30, down: 0 }, b = { ...conn("b"), up: 0, down: 20 }, c = { ...conn("c"), up: 5, down: 10 };
  assert.deepEqual([b, c, a].sort(compareConnections("speed", false)).map((c) => c.id), ["a", "b", "c"]);
});

test("closed rows fade for one poll, and not at all once the core has stopped", () => {
  const tracker = new ConnectionTracker();
  tracker.collect([conn("a"), conn("b")], 1000);
  tracker.collect([conn("b")], 2000);
  assert.deepEqual(lingering(tracker.snapshot, true).map((c) => c.id), ["a"]);
  tracker.collect([conn("b")], 3000);
  assert.deepEqual(lingering(tracker.snapshot, true), []);
  tracker.stop();
  assert.deepEqual(lingering(tracker.snapshot, false), []);
});

test("a held speed order survives new samples but not a new sort choice", () => {
  const a = { ...conn("a"), up: 30, down: 0 }, b = { ...conn("b"), up: 20, down: 0 };
  let held = holdOrder([a, b].sort(compareConnections("speed", false)), "speed:false", false, { key: "", rank: new Map() });
  const faster = { ...b, up: 50 }, list = [a, faster].sort(compareConnections("speed", false));
  held = holdOrder(list, "speed:false", true, held);
  assert.deepEqual(list.map((c) => c.id), ["a", "b"]);
  const flipped = [a, b].sort(compareConnections("speed", true));
  held = holdOrder(flipped, "speed:true", true, held);
  assert.deepEqual(flipped.map((c) => c.id), ["b", "a"]);
  assert.equal(held.key, "speed:true");
  const byHost = [{ ...a, metadata: { ...a.metadata, host: "z.test" } }, b].sort(compareConnections("host", true));
  holdOrder(byHost, "host:true", true, held);
  assert.deepEqual(byHost.map((c) => c.id), ["b", "a"]);
});
