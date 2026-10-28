// Real embedded editor + source/runtime/core checks. Fixtures are removed on exit.
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const { chromium } = await import(process.env.CLASHCUBE_PLAYWRIGHT_MODULE ?? "playwright");
const root = fileURLToPath(new URL("../../", import.meta.url));
const base = "http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube";
const browser = await chromium.launch({ headless: true, executablePath: process.env.CLASHCUBE_BROWSER_PATH });
const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
const page = await context.newPage(), errors = [], fixtures = [];
let saveRequests = 0;
page.on("request", (r) => { if (r.url().includes("/api/rpc/Profiles/SaveContent?")) saveRequests++; });
page.on("pageerror", (e) => errors.push(e.message));
const name = (english, chinese) => new RegExp(`^(${english}|${chinese})$`);
let frame, initial, local, subscription, copied;
const fixturePath = `/www/clashcube-editor-${Date.now()}.yaml`;
const command = (...args) => execFileSync("docker", ["compose", "-f", `${root}/dev/openwrt/compose.yaml`, "exec", "-T", "router", ...args], { encoding: "utf8" });

async function rpc(method, ...args) {
  return frame.evaluate(async ({ method, args }) => {
    const endpoint = document.querySelector('meta[name="clashcube-api"]').content;
    const token = document.querySelector('meta[name="clashcube-token"]').content;
    const r = await fetch(`${endpoint}rpc/${method}?token=${token}`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(args),
    });
    if (!r.ok) throw new Error(`Test RPC failed: ${method}: ${r.status}`);
    return r.json();
  }, { method, args });
}
const dialog = () => frame.getByRole("dialog", { name: name("Configuration editor", "配置编辑器") });
const yaml = () => dialog().getByLabel(name("Configuration YAML", "配置 YAML"));
const text = () => yaml().evaluate((el) => Array.from(el.querySelectorAll(":scope > .cm-line"), (line) => line.textContent).join("\n"));
const waitText = (body) => frame.waitForFunction((body) => Array.from(document.querySelectorAll("dialog .cm-content > .cm-line"), (line) => line.textContent).join("\n") === body, body);
async function replace(body) {
  // Use the editor's own selection/paste path. DOM-based contenteditable fill
  // includes the uneditable deleted-line widgets in a unified diff.
  await yaml().press("ControlOrMeta+a");
  await page.keyboard.insertText(body);
  await waitText(body);
}
async function open(profile) {
  // A previous menu remains mounted for its exit animation. Wait for it to
  // leave before locating the new menu, rather than clicking an old item.
  await frame.waitForFunction(() => !document.querySelector(".pop"));
  const row = frame.locator(".profile").filter({ hasText: profile.name });
  await row.getByRole("button", { name: name("Profile actions", "配置操作") }).click();
  await frame.locator(".pop:not(.leaving) .menu").getByRole("button", { name: name(profile.url ? "View configuration" : "Edit configuration", profile.url ? "查看配置" : "编辑配置") }).click();
  await yaml().waitFor();
  const subtitle = await dialog().locator(".profile-editor-head p").textContent();
  const active = (await rpc("App/State")).profile === profile.id;
  const contexts = profile.url ? ["Subscription", "订阅"] : active
    ? ["In use · saving checks and hot-reloads", "正在使用 · 保存后校验并热更新"]
    : ["Checked by the core when you switch to it", "切换使用时由内核校验"];
  assert.ok(contexts.some((context) => subtitle === `${profile.name} · ${context}`), subtitle);
}
async function save(expected = 200, shortcut = false) {
  await frame.waitForFunction(() => document.querySelector(".profile-editor-foot .primary")?.disabled === false);
  const result = page.waitForResponse((r) => r.url().includes("/api/rpc/Profiles/SaveContent?"));
  if (shortcut) await yaml().press("Control+s");
  else await dialog().getByRole("button", { name: name("Save configuration", "保存配置") }).click();
  const response = await result;
  assert.equal(response.status(), expected);
  if (expected === 200) await dialog().getByText(name("Saved", "已保存"), { exact: true }).waitFor();
  else await dialog().getByRole("alert").waitFor();
  return response.request().postDataJSON();
}
async function shot(file) {
  await page.screenshot({ path: `${root}/bin/openwrt/profile-editor-${file}.png`, animations: "disabled" });
  assert.equal(await dialog().evaluate((el) => el.scrollHeight > el.clientHeight + 1), false, "dialog must not scroll");
  assert.equal(await frame.locator("body").evaluate((b) => b.scrollWidth > b.clientWidth + 1), false);
}

try {
  const password = (await readFile(`${root}/dev/openwrt/root-password`, "utf8")).trim();
  await context.request.post(base, { form: { luci_username: "root", luci_password: password } });
  await page.goto(base);
  await page.locator("iframe").waitFor();
  frame = page.frames().find((f) => f.url().includes("clashcube/app"));
  assert.ok(frame);
  initial = await rpc("App/State");
  assert.equal(initial.core, "running", "lab core must be running");
  assert.deepEqual((await rpc("Router/Devices")).policies, [], "do not interrupt unrelated device policies");
  const original = '# editor fixture\nrules: ["MATCH,DIRECT"]\n';
  local = await rpc("Profiles/ImportContent", "Editor smoke local", original);
  fixtures.push(local.id);
  await rpc("Profiles/Use", local.id);
  await frame.locator('[data-view="profiles"]').click();
  await frame.getByRole("button", { name: name("Edit configuration", "编辑配置") }).click();
  await yaml().waitFor();
  await waitText(original);
  assert.ok(await dialog().locator(".cm-lineNumbers .cm-gutterElement").count() > 1, "line numbers missing");
  assert.ok(await dialog().locator(".yaml-key").count() > 0, "YAML keys not highlighted");
  assert.notEqual(await dialog().locator(".yaml-key").first().evaluate((el) => getComputedStyle(el).color), await yaml().evaluate((el) => getComputedStyle(el).color));
  await yaml().press("Control+Home");
  await yaml().press("Tab");
  await waitText("  " + original);
  await yaml().press("ControlOrMeta+z");
  await waitText(original);
  await yaml().press("ControlOrMeta+Shift+z");
  await waitText("  " + original);
  const changed = '# 中文手动修改\nrules:\n  - DOMAIN,editor.example,DIRECT\n  - MATCH,REJECT\n';
  await replace(changed);
  await dialog().locator(".cm-deletedChunk").first().waitFor();
  assert.ok(await dialog().locator(".cm-changedLine").count() > 0, "insertions missing from diff");
  await shot("diff");
  await dialog().getByRole("switch", { name: name("Show changes", "显示改动") }).click();
  assert.equal(await dialog().locator(".cm-deletedChunk").count(), 0);
  assert.equal(await text(), changed, "hiding diff changed the draft");
  await dialog().getByRole("switch", { name: name("Show changes", "显示改动") }).click();
  await dialog().locator(".cm-deletedChunk").first().waitFor();
  await save(200, true);
  assert.equal(await dialog().locator(".cm-deletedChunk, .cm-changedLine").count(), 0, "save did not reset diff baseline");
  assert.equal(await rpc("Profiles/Read", local.id), changed);
  assert.ok((await rpc("Proxy/Rules")).some((r) => r.payload === "editor.example"));
  await shot("desktop");
  const acceptedRuntime = command("cat", "/etc/clashcube/core/runtime.yaml");
  const rejected = 'rules: ["MATCH,MissingEditorNode"]\n';
  await replace(rejected);
  await save(422);
  assert.equal(await text(), rejected, "refusal lost the draft");
  assert.equal(await rpc("Profiles/Read", local.id), changed, "refusal changed source");
  assert.equal(command("cat", "/etc/clashcube/core/runtime.yaml"), acceptedRuntime, "refusal changed runtime");
  assert.ok((await rpc("Proxy/Rules")).some((r) => r.payload === "editor.example"));
  await replace('rules: [');
  const pendingRequests = saveRequests;
  await yaml().press("Control+s");
  assert.equal(saveRequests, pendingRequests, "unchecked draft reached the save API");
  await dialog().locator(".yaml-check.err").waitFor();
  assert.equal(await dialog().getByRole("button", { name: name("Save configuration", "保存配置") }).isDisabled(), true);
  assert.ok(await dialog().locator(".cm-lintRange-error, .cm-lintPoint-error").count() > 0, "syntax error not marked in source");
  const requestsBefore = saveRequests;
  await yaml().press("Control+s");
  assert.equal(saveRequests, requestsBefore, "syntax-invalid draft reached the save API");
  assert.equal(await rpc("Profiles/Read", local.id), changed);
  await shot("syntax-error");
  await dialog().locator(".profile-editor-close").click();
  await shot("discard");
  assert.equal(await dialog().getByRole("button", { name: name("Keep editing", "继续编辑") }).evaluate((el) => el === document.activeElement), true);
  await dialog().getByRole("button", { name: name("Keep editing", "继续编辑") }).click();
  assert.equal(await text(), 'rules: [');
  await yaml().press("Escape");
  await dialog().getByRole("button", { name: name("Discard changes", "放弃修改") }).click();
  await dialog().waitFor({ state: "hidden" });

  await open(local);
  const before = await rpc("Profiles/ReadDocument", local.id);
  const other = changed + "# saved by another page\n";
  await rpc("Profiles/SaveContent", local.id, before.revision, other);
  await replace(changed + "# stale draft\n");
  await save(422);
  assert.equal(await rpc("Profiles/Read", local.id), other, "stale editor overwrote a save");
  await dialog().getByRole("button", { name: name("Revert", "还原") }).click();
  const reloaded = page.waitForResponse((r) => r.url().includes("/api/rpc/Profiles/ReadDocument?"));
  await dialog().getByRole("button", { name: name("Discard changes", "放弃修改") }).click();
  await reloaded;
  await waitText(other);
  assert.equal(await dialog().locator(".cm-deletedChunk, .cm-changedLine").count(), 0, "reload did not reset diff baseline");
  await replace(other + "# stopped save\n");
  await rpc("App/Stop");
  await save();
  assert.equal((await rpc("App/State")).core, "stopped", "save started a stopped core");
  await rpc("App/Start");
  await dialog().locator(".profile-editor-foot").getByRole("button", { name: name("Close", "关闭") }).click();

  // Real subscription-sized documents must scroll without rendering every
  // line, and validation/save must use the whole source beyond the viewport.
  const largeSource = "# large editor fixture\nrules:\n" + Array.from({ length: 3000 }, (_, i) => `  - DOMAIN,editor-${i}.example,DIRECT\n`).join("") + "  - MATCH,DIRECT\n";
  const large = await rpc("Profiles/ImportContent", "Editor smoke large", largeSource);
  fixtures.push(large.id);
  await open(large);
  await dialog().getByText(name("YAML syntax valid", "YAML 语法有效")).waitFor();
  assert.ok(await dialog().locator(".cm-content > .cm-line").count() < 3000, "large file lost viewport virtualization");
  await yaml().press("ControlOrMeta+End");
  await yaml().pressSequentially("# large document edit");
  const largeRequest = await save();
  assert.equal(largeRequest[0], large.id, "large editor saved a different profile");
  assert.equal(largeRequest[2].endsWith("# large document edit"), true, JSON.stringify({ length: largeRequest[2].length, tail: largeRequest[2].slice(-100) }));
  const savedLarge = await rpc("Profiles/Read", large.id);
  assert.equal(savedLarge === largeSource + "# large document edit", true, JSON.stringify({ savedLength: savedLarge.length, expectedLength: largeSource.length + 21, savedTail: savedLarge.slice(-100) }));
  await dialog().locator(".profile-editor-foot").getByRole("button", { name: name("Close", "关闭") }).click();

  // Subscription viewing/copying must not allow edits to the managed source.
  execFileSync("docker", ["compose", "-f", `${root}/dev/openwrt/compose.yaml`, "exec", "-T", "router", "tee", fixturePath], { input: original, stdio: ["pipe", "ignore", "pipe"] });
  subscription = await rpc("Profiles/ImportURL", `http://127.0.0.1/${fixturePath.split("/").at(-1)}`, "Editor smoke subscription", 0);
  fixtures.push(subscription.id);
  await open(subscription);
  assert.equal(await yaml().getAttribute("contenteditable"), "false");
  assert.equal(await dialog().getByRole("switch").count(), 0);
  await shot("readonly");
  const copyLoaded = page.waitForResponse((r) => r.url().includes("/api/rpc/Profiles/ReadDocument?"));
  await dialog().getByRole("button", { name: name("Make an editable copy", "创建可编辑副本") }).click();
  await copyLoaded;
  await frame.waitForFunction(() => document.querySelector("dialog .cm-content")?.getAttribute("contenteditable") === "true");
  copied = (await rpc("Profiles/List")).find((p) => p.name === `${subscription.name} (copy)` || p.name === `${subscription.name}（副本）`);
  assert.ok(copied, "editable copy missing");
  fixtures.push(copied.id);
  assert.equal(await yaml().getAttribute("contenteditable"), "true");
  await replace(original + "# local copy edit\n");
  await save();
  assert.equal(await rpc("Profiles/Read", subscription.id), original, "copy changed subscription source");
  await page.emulateMedia({ colorScheme: "dark" });
  await frame.evaluate(() => document.documentElement.dataset.theme = "dark");
  await shot("dark");
  await page.setViewportSize({ width: 390, height: 844 });
  await shot("mobile");
  assert.deepEqual(errors, []);
  assert.equal(context.pages().length, 1, "editor opened another window");
  console.log("PASS: YAML highlights/line numbers/undo/redo, inline diff toggle and saved/reloaded baseline, pending/invalid syntax refusal, 3004-line virtualized editing/save, active save/core reload, core refusal rollback, draft retention/discard, stale conflict/reload, stopped save, read-only subscription/editable copy, desktop/dark/390px, no JS errors/new windows");
} finally {
  if (frame && initial) {
    await rpc("Profiles/Use", initial.profile);
    if (initial.core === "running" && (await rpc("App/State")).core !== "running") await rpc("App/Start");
    for (const id of fixtures) await rpc("Profiles/Remove", id);
    command("rm", "-f", fixturePath);
  }
  await browser.close();
}
