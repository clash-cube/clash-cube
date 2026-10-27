#!/usr/bin/env python3
"""Exercise the deployed plugin through LuCI; keep passwords and tokens private."""
import http.cookiejar
import http.client
import json
import pathlib
import re
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = pathlib.Path(__file__).resolve().parents[2]
BASE = "http://127.0.0.1:18080/cgi-bin/luci/admin/services/clashcube"


def response(client, url, data=None, headers=None):
    request = urllib.request.Request(url, data=data, headers=headers or {})
    try:
        result = client.open(request, timeout=60)
    except urllib.error.HTTPError as error:
        result = error
    return result.code, result.read(), result.headers


def main():
    anonymous = urllib.request.build_opener()
    deadline = time.monotonic() + 30
    while True:
        try:
            status, body, _ = response(anonymous, BASE)
            if status in (200, 403):
                break
        except (OSError, urllib.error.URLError, http.client.HTTPException):
            pass
        if time.monotonic() >= deadline:
            raise RuntimeError("LuCI did not become ready")
        time.sleep(0.25)
    assert status in (200, 403) and b'luci_password' in body, "page is not authenticated"
    status, body, _ = response(anonymous, BASE + "/api/rpc/App/State", b"[]", {"Content-Type": "application/json"})
    assert b'luci_password' in body, "API is not authenticated"

    cookies = http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))
    password = (ROOT / "dev/openwrt/root-password").read_text().strip()
    login = urllib.parse.urlencode({"luci_username": "root", "luci_password": password}).encode()
    response(client, BASE, login)
    status, page, _ = response(client, BASE)
    assert status == 200 and b'id="clashcube-app"' in page, "embedded page missing"
    status, page, _ = response(client, BASE + "/app")
    assert status == 200 and b'id="root"' in page, "app frame missing"
    token = re.search(rb'name="clashcube-token" content="([a-fA-F0-9]+)"', page).group(1).decode()
    headers = {"Content-Type": "application/json", "X-ClashCube-Request": "1"}

    def rpc(method, *args, expected=200):
        status, body, _ = response(client, BASE + "/api/rpc/" + method + "?token=" + token, json.dumps(args).encode(), headers)
        assert status == expected, f"{method}: HTTP {status}"
        return json.loads(body)

    status, _, _ = response(client, BASE + "/api/rpc/App/State", b"[]", headers)
    assert status == 403, "missing CSRF token accepted"
    # Older uhttpd may omit Origin from CGI. The session token remains mandatory.
    status, _, _ = response(client, BASE + "/api/rpc/App/State?token=invalid", b"[]", headers)
    assert status == 403, "invalid CSRF token accepted"
    rpc("App/SetTun", True, expected=404)
    rpc("Profiles/ImportFile", "/etc/shadow", expected=404)
    rpc("Settings/Patch", {"mixedPort": 0}, expected=422)
    rpc("Settings/Patch", {"systemProxy": True}, expected=400)

    initial = rpc("App/State")
    deadline = time.monotonic() + 30
    while initial["core"] in ("starting", "stopped") and time.monotonic() < deadline:
        time.sleep(0.25)
        initial = rpc("App/State")
    assert initial["core"] == "running", "core is not running"
    settings = rpc("Settings/Get")
    original_modules = rpc("Profiles/Modules")
    imported = None
    subscription = None
    fixture = "/www/clashcube-smoke-" + uuid.uuid4().hex + ".yaml"
    compose = ["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml")]
    try:
        imported = rpc("Profiles/ImportContent", "QWRT smoke test", "proxy-groups:\n  - name: Lab\n    type: select\n    proxies: [DIRECT, REJECT]\nrules:\n  - MATCH,Lab\n")
        rpc("Profiles/Use", imported["id"])
        groups = rpc("Proxy/Groups")
        assert any(g["name"] == "Lab" for g in groups)
        content = b"proxy-groups:\n  - name: Downloaded\n    type: select\n    proxies: [DIRECT]\nrules:\n  - MATCH,Downloaded\n"
        subprocess.run(compose + ["exec", "-T", "router", "tee", fixture], input=content, stdout=subprocess.DEVNULL, check=True)
        subscription = rpc("Profiles/ImportURL", "http://127.0.0.1/" + pathlib.Path(fixture).name, "Lab subscription", 0)
        assert rpc("Profiles/Update", subscription["id"])["id"] == subscription["id"]
        rpc("Settings/Patch", {"logLevel": "warning"})
        assert rpc("Settings/Get")["logLevel"] == "warning"
        rpc("Settings/Patch", {"logLevel": settings["logLevel"]})
        rpc("Proxy/Select", "Lab", "DIRECT")
        rpc("App/SetMode", "direct")
        assert rpc("App/State")["mode"] == "direct"
        rpc("App/SetMode", "rule")
        module = {"id": "lab-smoke", "name": "Lab test", "enabled": True, "body": "tcp-concurrent: true\n"}
        rpc("Profiles/SetModules", original_modules + [module])
        assert any(m["id"] == "lab-smoke" for m in rpc("Profiles/Modules"))
        rpc("Profiles/SetModules", original_modules + [{**module, "body": "proxy-groups:\n  - name: Broken\n    type: impossible\n"}], expected=422)
        assert rpc("Profiles/Modules")[-1]["body"] == module["body"], "refused module replaced the accepted one"
        view = rpc("Profiles/RuntimeConfig")
        assert "secret: [private]" in view["body"], "controller secret was not redacted"
        assert all("secret:" not in line["text"] or "[private]" in line["text"] for line in view.get("lines", []))

        result = subprocess.run([
            "docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml"), "exec", "-T", "client",
            "curl", "-sS", "--max-time", "30", "-x", f'http://172.30.77.2:{settings["mixedPort"]}',
            "-o", "/dev/null", "-w", "%{http_code}", "https://example.com",
        ], check=True, capture_output=True, text=True)
        assert result.stdout == "200", "explicit proxy HTTPS failed"
        rpc("Proxy/Select", "Lab", "REJECT")
        rejected = subprocess.run(compose + ["exec", "-T", "client", "curl", "-sS", "--max-time", "10", "-x",
            f'http://172.30.77.2:{settings["mixedPort"]}', "-o", "/dev/null", "https://example.com"], capture_output=True)
        assert rejected.returncode != 0, "REJECT selection did not block the request"
        rpc("Proxy/Select", "Lab", "DIRECT")
        rpc("App/Stop")
        assert rpc("App/State")["core"] == "stopped"
        rpc("App/Start")
        assert rpc("App/State")["core"] == "running"
        rpc("App/Restart")
        assert rpc("App/State")["core"] == "running"
        print("PASS: LuCI login and CSRF, RPC boundary, upload and subscription update, settings, groups, mode, modules and rollback, redaction, HTTPS proxy and REJECT, core lifecycle")
    finally:
        rpc("Profiles/SetModules", original_modules)
        rpc("Profiles/Use", initial["profile"])
        rpc("App/SetMode", initial["mode"])
        if imported:
            rpc("Profiles/Remove", imported["id"])
        if subscription:
            rpc("Profiles/Remove", subscription["id"])
        rpc("Settings/Patch", {"logLevel": settings["logLevel"]})
        subprocess.run(compose + ["exec", "-T", "router", "rm", "-f", fixture], check=True)
        rpc("Profiles/DismissRefusal")


if __name__ == "__main__":
    main()
