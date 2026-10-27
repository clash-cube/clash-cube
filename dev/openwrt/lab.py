"""Authenticated RPC and bounded shell access to the isolated QWRT lab."""
import http.cookiejar
import json
import re
import subprocess
import time
import urllib.parse
import urllib.request

from smoke import ROOT, BASE, response

COMPOSE = ["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml"), "exec", "-T"]


def command(service, *args, check=True, input=None):
    result = subprocess.run(COMPOSE + [service, *args], input=input, capture_output=True, text=True, timeout=45)
    if check and result.returncode:
        raise AssertionError(f"{service} {args[0]} failed: {result.stderr[:500]}")
    return result


class Lab:
    def __init__(self):
        self.client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        response(self.client, BASE, urllib.parse.urlencode({"luci_username": "root", "luci_password": (ROOT / "dev/openwrt/root-password").read_text().strip()}).encode())
        _, page, _ = response(self.client, BASE + "/app")
        self.token = re.search(rb'name="clashcube-token" content="([a-fA-F0-9]+)"', page).group(1).decode()

    def rpc(self, method, *args):
        code, body, _ = response(self.client, BASE + "/api/rpc/" + method + "?token=" + self.token, json.dumps(args).encode(), {"Content-Type": "application/json"})
        assert code == 200, f"{method}: HTTP {code}: {body.decode()[:250]}"
        return json.loads(body)

    def ready(self, core="running"):
        deadline = time.monotonic() + 35
        while time.monotonic() < deadline:
            code, body, _ = response(self.client, BASE + "/api/rpc/App/State?token=" + self.token, b"[]", {"Content-Type": "application/json"})
            if code == 200 and (core is None or json.loads(body)["core"] == core):
                return
            time.sleep(0.2)
        raise AssertionError("services did not restore expected state")


def uci(method, **args):
    return json.loads(command("router", "ubus", "call", "uci", method, json.dumps({"config": "dhcp", **args})).stdout or "{}")
