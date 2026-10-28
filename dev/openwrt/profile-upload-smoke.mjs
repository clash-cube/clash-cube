// Real file-picker uploads through the authenticated LuCI iframe and bridge.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const { chromium } = await import(process.env.CLASHCUBE_PLAYWRIGHT_MODULE ?? "playwright");
const root = fileURLToPath(new URL("../../", import.meta.url));
const base = "http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube";
const limit = 4 << 20;
const browser = await chromium.launch({ headless: true, executablePath: process.env.CLASHCUBE_BROWSER_PATH });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage(), errors = [], imported = [];
page.on("pageerror", (error) => errors.push(error.message));
let frame;

async function rpc(method, ...args) {
  return frame.evaluate(async ({ method, args }) => {
    const endpoint = document.querySelector('meta[name="clashcube-api"]').content;
    const token = document.querySelector('meta[name="clashcube-token"]').content;
    const response = await fetch(`${endpoint}rpc/${method}?token=${token}`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(args),
    });
    if (!response.ok) throw new Error(`Test RPC failed: ${method}: ${response.status}`);
    return response.json();
  }, { method, args });
}

async function upload(name, body, expectedStatus = 200) {
  const form = frame.locator(".pop-form");
  if (!await form.isVisible()) await frame.getByRole("button", { name: /^(Import|导入)$/ }).click();
  const chooser = page.waitForEvent("filechooser");
  await frame.getByRole("button", { name: /^(Import a file…|导入本地文件…)$/ }).click();
  const response = page.waitForResponse((r) => r.url().includes("/api/rpc/Profiles/ImportContent?") && r.request().method() === "POST");
  await (await chooser).setFiles({ name: `${name}.yaml`, mimeType: "application/yaml", buffer: Buffer.from(body) });
  const result = await response;
  assert.equal(result.status(), expectedStatus, `upload ${name}`);
  const value = await result.json();
  if (expectedStatus === 200) {
    imported.push(value.id);
    assert.equal(value.name, name);
    assert.equal(await rpc("Profiles/Read", value.id), body, "uploaded content was changed or truncated");
    await form.waitFor({ state: "hidden" });
    await frame.getByText(name, { exact: true }).waitFor();
  } else {
    assert.equal(value.error, "Request too large");
    assert.notEqual(result.statusText(), "OK", "HTTP error retained a misleading reason phrase");
    await frame.getByText("Request too large", { exact: true }).waitFor();
  }
}

try {
  const password = (await readFile(`${root}/dev/openwrt/root-password`, "utf8")).trim();
  await context.request.post(base, { form: { luci_username: "root", luci_password: password } });
  await page.goto(base);
  await page.locator("iframe").waitFor();
  frame = page.frames().find((f) => f.url().includes("clashcube/app"));
  assert.ok(frame, "ClashCube is not embedded in LuCI");
  const initialProfile = (await rpc("App/State")).profile;
  await frame.locator('[data-view="profiles"]').click();
  const prefix = `Upload regression ${Date.now()}`;
  const content = ['# 中文配置，包含引号 "', "rules:", ...Array.from({ length: 4096 }, (_, i) => `  - DOMAIN,upload-${i}.example,DIRECT`)].join("\n") + "\n";
  await upload(`${prefix} multiline`, content);
  // Exactly the existing JSON bridge limit, including encoding overhead.
  const boundaryName = `${prefix} boundary`;
  const padding = limit - Buffer.byteLength(JSON.stringify([boundaryName, content + "# "]));
  const boundary = content + "# " + "x".repeat(padding);
  assert.equal(Buffer.byteLength(JSON.stringify([boundaryName, boundary])), limit);
  await upload(boundaryName, boundary);
  await upload(`${prefix} oversized`, "# " + "x".repeat(limit), 413);
  await upload(`${prefix} retry`, 'rules: ["MATCH,DIRECT"]\n');
  const malformed = await frame.evaluate(async () => {
    const endpoint = document.querySelector('meta[name="clashcube-api"]').content;
    const token = document.querySelector('meta[name="clashcube-token"]').content;
    const response = await fetch(`${endpoint}rpc/App/State?token=${token}`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: "[",
    });
    return { status: response.status, reason: response.statusText };
  });
  assert.deepEqual(malformed, { status: 400, reason: "Bad Request" });
  assert.equal((await rpc("App/State")).profile, initialProfile, "upload switched the active profile");
  assert.deepEqual(errors, []);
  assert.equal(context.pages().length, 1, "upload opened another window");
  console.log("PASS: real file uploads above 8 KiB, Unicode/multiline content preserved, exact 4 MiB JSON accepted, oversized rejected, retry succeeds, active profile unchanged, no JS errors/new windows");
} finally {
  if (frame) for (const id of imported) await rpc("Profiles/Remove", id);
  await browser.close();
}
