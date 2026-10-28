// Real LuCI iframe + real policy RPC. Run after device-egress-smoke.py.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const { chromium } = await import(process.env.CLASHCUBE_PLAYWRIGHT_MODULE ?? "playwright");
const root = fileURLToPath(new URL("../../", import.meta.url));
const base = "http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube";
const browser = await chromium.launch({ headless: true, executablePath: process.env.CLASHCUBE_BROWSER_PATH });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage(), errors = [];
page.on("pageerror", (e) => errors.push(e.message));
let frame, originalProfile, fixture;
async function rpc(method, ...args) {
  return frame.evaluate(async ({ method, args }) => {
    const endpoint = document.querySelector('meta[name="clashcube-api"]').content;
    const token = document.querySelector('meta[name="clashcube-token"]').content;
    const r = await fetch(`${endpoint}rpc/${method}?token=${token}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(args) });
    if (!r.ok) throw new Error(`Test RPC failed: ${method}: ${r.status}`);
    return r.json();
  }, { method, args });
}
const name = (english, chinese) => new RegExp(`^(${english}|${chinese})$`);
const traffic = () => execFileSync("docker", ["exec", "clashcube-egress-tests-peer-1", "python3", "/lab/peer.py", "probe", "tcp"], { encoding: "utf8" }).trim();
async function screenshot(file) {
  const body = frame.locator("body");
  await body.screenshot({ path: `${root}/bin/openwrt/${file}.png`, animations: "disabled" });
  assert.equal(await body.evaluate((b) => b.scrollWidth > b.clientWidth + 1), false, "app has horizontal overflow");
}
try {
  const password = (await readFile(`${root}/dev/openwrt/root-password`, "utf8")).trim();
  await context.request.post(base, { form: { luci_username: "root", luci_password: password } });
  await page.goto(base);
  await page.locator("iframe").waitFor();
  frame = page.frames().find((f) => f.url().includes("clashcube/app"));
  assert.ok(frame, "ClashCube did not stay embedded in LuCI");
  originalProfile = (await rpc("Settings/Get")).profile;
  assert.deepEqual((await rpc("Router/Devices")).policies, [], "lab must have no unrelated protected devices");
  if (process.argv.includes("--fresh")) {
    // Exercise the first-policy path, before a devices.json/revision exists.
    execFileSync("docker", ["compose", "-f", `${root}/dev/openwrt/compose.yaml`, "exec", "-T", "router", "rm", "-f", "/etc/clashcube/devices.json"]);
    assert.equal((await rpc("Router/Devices")).revision, "");
  }
  fixture = await rpc("Profiles/ImportContent", "Device UI fixture", JSON.stringify({ proxies: [
    { name: "UI node A", type: "http", server: "172.30.76.10", port: 18081 },
    { name: "UI node B", type: "http", server: "172.30.76.10", port: 18082 },
  ], rules: ["MATCH,DIRECT"] }));
  await rpc("Profiles/Use", fixture.id);
  await frame.locator('[data-view="devices"]').click();
  await frame.getByRole("button", { name: name("Protect a device", "保护一台设备") }).click();
  let dialog = frame.getByRole("dialog");
  await dialog.getByRole("tab", { name: name("Enter manually", "手动填写") }).click();
  await dialog.getByLabel(name("Device name", "设备名称")).fill("UI test phone");
  await dialog.getByLabel(name("MAC address", "MAC 地址")).fill("02:cc:77:00:00:04");
  await dialog.getByLabel(name("IPv4 address", "IPv4 地址")).fill("172.30.77.4");
  await dialog.getByLabel(name("Fixed egress", "固定出口")).selectOption({ label: "UI node A · HTTP" });
  await screenshot("device-egress-add");
  await dialog.getByRole("button", { name: name("Save and protect", "保存并开启保护") }).click();
  await dialog.waitFor({ state: "hidden" });
  assert.equal(traffic(), "NODE-A");
  await screenshot("device-egress-desktop");
  const actions = () => frame.getByRole("button", { name: name("Device actions", "设备操作") });
  await actions().click();
  await frame.getByText(name("Pause networking", "暂停联网"), { exact: true }).click();
  await frame.getByText(name("Networking paused", "已暂停联网"), { exact: true }).waitFor();
  assert.equal(traffic(), "BLOCKED");
  await actions().click();
  await frame.getByText(name("Resume networking", "恢复联网"), { exact: true }).click();
  await frame.getByText(name("Fixed node configured", "固定节点已配置"), { exact: true }).waitFor();
  await actions().click();
  await frame.getByText(name("Change node", "更换节点"), { exact: true }).click();
  dialog = frame.getByRole("dialog");
  await dialog.getByLabel(name("Fixed egress", "固定出口")).selectOption({ label: "UI node B · HTTP" });
  await dialog.getByRole("button", { name: name("Save fixed egress", "保存固定出口") }).click();
  await dialog.waitFor({ state: "hidden" });
  assert.equal(traffic(), "NODE-B");
  let resumeStatus;
  const heldStatus = new Promise((resolve) => { resumeStatus = resolve; });
  const statusRoute="**/api/rpc/Router/Devices?*";
  await context.route(statusRoute, async (route) => { await heldStatus; await route.continue(); });
  await frame.getByText(name("Protection unverified","保护待核验"),{exact:true}).waitFor({timeout:21000});
  assert.equal(await frame.locator(".device-badge.safe").count(),0,"hung API retained a green badge");
  assert.equal(traffic(),"NODE-B","stale management changed kernel protection");
  await screenshot("device-egress-stale");
  resumeStatus();
  await context.unrouteAll({ behavior: "wait" });
  await frame.getByText(name("Protected","保护已生效"),{exact:true}).waitFor();
  await page.emulateMedia({ colorScheme: "dark" });
  await frame.evaluate(() => document.documentElement.dataset.theme = "dark");
  await screenshot("device-egress-dark");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot("device-egress-mobile");
  await frame.getByRole("button", { name: name("Protect a device", "保护一台设备") }).click();
  await screenshot("device-egress-mobile-add");
  await frame.getByRole("dialog").getByRole("button", { name: name("Cancel", "取消") }).click();
  await actions().click();
  await frame.getByText(name("Release protection", "解除保护"), { exact: true }).click();
  await screenshot("device-egress-release");
  await frame.getByRole("dialog").getByRole("button", { name: name("Keep protection", "保留保护") }).click();
  assert.equal((await rpc("Router/Devices")).policies.length, 1, "cancel unexpectedly released protection");
  await actions().click();
  await frame.getByText(name("Release protection", "解除保护"), { exact: true }).click();
  await frame.getByRole("dialog").getByRole("button", { name: name("Release protection", "解除保护") }).click();
  await frame.getByRole("dialog").waitFor({ state: "hidden" });
  assert.equal(traffic(), "DIRECT");
  assert.equal(context.pages().length, 1, "an internal action opened another window");
  assert.deepEqual(errors, []);
  console.log("PASS: embedded device UI, manual add, reservation, pause/resume, node change, release confirmation/cancel, desktop/dark/390px, no overflow, JS errors or new windows");
} finally {
  if (frame) {
    for (const p of (await rpc("Router/Devices")).policies) {
      assert.equal(p.name, "UI test phone", "refusing to release an unrelated device");
      await rpc("Router/ReleaseDevice", p.id, (await rpc("Router/Devices")).revision);
    }
    if (originalProfile) await rpc("Profiles/Use", originalProfile);
    if (fixture) await rpc("Profiles/Remove", fixture.id);
  }
  await browser.close();
}
