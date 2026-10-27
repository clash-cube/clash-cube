// Run with Playwright installed, or CLASHCUBE_PLAYWRIGHT_MODULE pointing to it.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const { chromium } = await import(process.env.CLASHCUBE_PLAYWRIGHT_MODULE ?? "playwright");
const root = fileURLToPath(new URL("../../", import.meta.url));
const base = "http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube";
const browser = await chromium.launch({ headless: true, executablePath: process.env.CLASHCUBE_BROWSER_PATH });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage();
const errors = [];
page.on("pageerror", (error) => errors.push(error.message));
let frame, original, maintenance;

async function rpc(method, ...args) {
  return frame.evaluate(async ({ method, args }) => {
    const endpoint = document.querySelector('meta[name="clashcube-api"]').content;
    const token = document.querySelector('meta[name="clashcube-token"]').content;
    const response = await fetch(`${endpoint}rpc/${method}?token=${token}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(args) });
    if (!response.ok) throw new Error("test RPC failed");
    return response.json();
  }, { method, args });
}

try {
  const password = (await readFile(`${root}/dev/openwrt/root-password`, "utf8")).trim();
  await context.request.post(base, { form: { luci_username: "root", luci_password: password } });
  await page.goto(base);
  await page.locator("iframe").waitFor();
  frame = page.frames().find((f) => f.url().includes("clashcube/app"));
  assert.ok(frame, "app is not embedded in LuCI");
  await frame.locator('[data-view="settings"]').click();
  await frame.getByRole("tab", { name: /^(Network|网络)$/ }).click();
  original = await rpc("Router/Get");
  maintenance = await rpc("Router/Maintenance");
  const accessRow = frame.getByText(/^(Device access|设备访问控制)$/).locator("..").locator("..");
  await accessRow.getByRole("button", { name: /^(Edit|编辑)$/ }).click();
  await frame.getByRole("tab", { name: /^(Bypass listed devices|列表中的设备直连)$/ }).click();
  const addresses = frame.getByLabel(/^(IPv4 addresses or subnets|IPv4 地址或网段)/);
  await addresses.fill("172.30.77.3");
  await addresses.locator("..").locator("..").getByRole("button", { name: /^(Save|保存)$/ }).click();
  assert.equal((await rpc("Router/Get")).access.mode, "exclude");
  const maintenanceRow = frame.getByText(/^(Scheduled GEO updates|GEO 定时更新)$/).locator("..").locator("..");
  await maintenanceRow.getByRole("button", { name: /^(Configure|配置)$/ }).click();
  const feed = frame.getByLabel(/^(Signed core update feed|内核签名更新源)$/);
  await feed.fill("https://example.com/router-update.json");
  await feed.locator("..").locator("..").getByRole("button", { name: /^(Save|保存)$/ }).click();
  assert.equal((await rpc("Router/Maintenance")).config.updateFeed, "https://example.com/router-update.json");
  await frame.getByText(/^(Router maintenance|路由器维护)$/).scrollIntoViewIfNeeded();
  await page.screenshot({ path: `${root}/bin/openwrt/p1-settings.png`, animations: "disabled" });
  const diagnosticRow = frame.getByText(/^(System and storage|系统与存储)$/).locator("..").locator("..");
  await diagnosticRow.getByRole("button", { name: /^(Check|检查)$/ }).click();
  const download = page.waitForEvent("download");
  await frame.getByRole("button", { name: /^(Download report|下载诊断报告)$/ }).click();
  await (await download).saveAs(`${root}/bin/openwrt/p1-diagnostics.json`);
  await frame.getByText(/^(Router diagnostics|路由器诊断)$/).scrollIntoViewIfNeeded();
  await page.screenshot({ path: `${root}/bin/openwrt/p1-diagnostics.png`, animations: "disabled" });
  assert.deepEqual(errors, []);
  assert.equal(context.pages().length, 1, "internal actions opened another window");
  console.log("PASS: embedded LuCI settings, device edit/save, maintenance configuration, diagnostic download, no JS errors or new windows");
} finally {
  if (original) await rpc("Router/SetAccess", original.access);
  if (maintenance) await rpc("Router/SetMaintenance", maintenance.config);
  await browser.close();
}
