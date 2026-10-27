#!/usr/bin/env python3
"""Verify signed core updates, preflight rejection and independent procd jobs."""
import json
import pathlib
import shlex
import subprocess
import time
import uuid

from lab import Lab, command
from smoke import ROOT

COMPOSE = ["docker", "compose", "-f", str(ROOT / "dev/openwrt/compose.yaml")]


def main():
    lab = Lab()
    original = lab.rpc("Router/Maintenance")["config"]
    initial = lab.rpc("App/State")
    fixture = "/tmp/clashcube-update-" + uuid.uuid4().hex
    destination = ROOT / "bin/openwrt/update-fixture"
    port = 18291
    base = f"http://172.30.77.3:{port}"
    feed = base + "/feed.json"
    client = subprocess.check_output(COMPOSE + ["ps", "-q", "client"], text=True).strip()
    server_pid = None
    updater_cache = "/tmp/clashcube-cache/updates"
    backup_cache = updater_cache + ".smoke-backup"
    assert lab.rpc("Router/Updates")["job"]["phase"] not in ("queued", "downloading", "installing"), "wait for the existing core update"
    command("router", "sh", "-c", f"if [ -d {updater_cache} ]; then mv {updater_cache} {backup_cache}; fi")

    def build(version):
        subprocess.run(["python3", "openwrt/package.py", "--skip-build", "--version", version, "--output", str(destination)], cwd=ROOT, check=True, capture_output=True)
        subprocess.run(["go", "run", "./scripts/signrouter", "-base", base, "-out", str(destination / "feed.json"), str(destination / "packages.json")], cwd=ROOT, check=True, capture_output=True)
        manifest = json.loads((destination / "packages.json").read_text())
        core = next(p for p in manifest["packages"] if p["name"] == "clashcube-core")
        subprocess.run(["docker", "cp", str(destination / core["file"]), client + ":" + fixture + "/" + core["file"]], check=True, capture_output=True)
        subprocess.run(["docker", "cp", str(destination / "feed.json"), client + ":" + fixture + "/feed.json"], check=True, capture_output=True)
        return core

    def job():
        result = command("router", "cat", updater_cache + "/job.json", check=False)
        return json.loads(result.stdout) if result.returncode == 0 else {}

    def wait_job(phases, seconds=90):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            state = job()
            if state.get("phase") in phases:
                return state
            time.sleep(0.2)
        raise AssertionError("core update did not reach expected phase")

    def core_pid():
        return command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ", check=False).stdout.strip()

    def wait_supervisor():
        # opkg's postinst returns before procd has opened the replacement
        # supervisor socket, especially when the proxy intentionally stays off.
        deadline = time.monotonic() + 35
        while command("router", "curl", "-sf", "-o", "/dev/null", "--unix-socket",
                      "/var/run/clashcube-core/control.sock", "-d", "[]",
                      "http://localhost/rpc/Core/Status", check=False).returncode:
            assert time.monotonic() < deadline, "replacement supervisor did not become ready"
            time.sleep(0.1)

    try:
        command("client", "mkdir", "-p", fixture)
        stamp = str(time.time_ns())
        version = lab.rpc("Router/Updates")["installed"].split("+test.", 1)[0] + "+test." + stamp
        core = build(version + ".1")
        # The fixture stays on the isolated LAN and deliberately throttles the
        # IPK so management can be stopped during the actual network transfer.
        lua = '''require("nixio.util")
local n=require("nixio")
local root=arg[1]
local server=assert(n.socket("inet","stream"))
server:setsockopt("socket","reuseaddr",1)
assert(server:bind("172.30.77.3",18291)) assert(server:listen(8))
while true do
 local s=assert(server:accept()) s:setsockopt("socket","rcvtimeo",10)
 local request=""
 while not request:find("\\r\\n\\r\\n",1,true) do local b=s:recv(4096) if not b or #b==0 then break end request=request..b end
 local path=request:match("^GET /([A-Za-z0-9.+_-]+) HTTP")
 local f=path and io.open(root.."/"..path,"rb")
 if f then
  local size=f:seek("end") f:seek("set")
  s:sendall("HTTP/1.1 200 OK\\r\\nConnection: close\\r\\nContent-Length: "..size.."\\r\\n\\r\\n")
  while true do local b=f:read(65536) if not b then break end if not s:sendall(b) then break end if path:match("%.ipk$") then n.nanosleep(0,50000000) end end
  f:close()
 else s:sendall("HTTP/1.1 404 Not Found\\r\\nContent-Length: 0\\r\\nConnection: close\\r\\n\\r\\n") end
 s:close()
end
'''
        command("client", "tee", fixture + "/server.lua", input=lua)
        server_pid = command("client", "sh", "-c", f"lua {fixture}/server.lua {fixture} > {fixture}/server.log 2>&1 & echo $!").stdout.strip()
        deadline = time.monotonic() + 5
        while command("router", "curl", "-sf", "--max-time", "1", feed, check=False).returncode:
            assert time.monotonic() < deadline, "fixture server did not start"
            time.sleep(0.1)
        lab.rpc("Router/SetMaintenance", {**original, "geoIntervalHours": 0, "updateFeed": feed})
        signed = json.loads((destination / "feed.json").read_text())
        signed["manifest"]["version"] = "9.9.9-9"
        (destination / "bad.json").write_text(json.dumps(signed))
        subprocess.run(["docker", "cp", str(destination / "bad.json"), client + ":" + fixture + "/bad.json"], check=True, capture_output=True)
        lab.rpc("Router/SetMaintenance", {**original, "geoIntervalHours": 0, "updateFeed": base + "/bad.json"})
        try:
            lab.rpc("Router/CheckUpdates")
            raise AssertionError("accepted a tampered manifest")
        except AssertionError as error:
            assert "HTTP 422" in str(error)
        lab.rpc("Router/SetMaintenance", {**original, "geoIntervalHours": 0, "updateFeed": feed})
        assert lab.rpc("Router/CheckUpdates")["available"]
        pid = core_pid()
        digest = command("router", "sha256sum", "/usr/bin/clashcube-core").stdout
        command("client", "sh", "-c", "printf corrupt > " + shlex.quote(fixture + "/" + core["file"]))
        lab.rpc("Router/UpdateCore")
        assert wait_job({"failed"})["phase"] == "failed"
        assert core_pid() == pid and command("router", "sha256sum", "/usr/bin/clashcube-core").stdout == digest, "preflight failure replaced running core"
        subprocess.run(["docker", "cp", str(destination / core["file"]), client + ":" + fixture + "/" + core["file"]], check=True, capture_output=True)
        lab.rpc("Router/UpdateCore")
        wait_job({"downloading"})
        command("router", "/etc/init.d/clashcube", "stop")
        assert core_pid() == pid, "starting an update interrupted the old core before download finished"
        assert wait_job({"succeeded", "failed"})["phase"] == "succeeded", "offline-management update failed"
        wait_supervisor()
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready()
        assert core_pid() != pid
        state = lab.rpc("Router/Updates")
        assert state["installed"] == version + ".1" and not state["available"]
        # A stopped proxy must stay stopped when only its installed package is updated.
        build(version + ".2")
        assert lab.rpc("Router/CheckUpdates")["available"]
        lab.rpc("App/Stop")
        lab.rpc("Router/UpdateCore")
        assert wait_job({"succeeded", "failed"})["phase"] == "succeeded"
        wait_supervisor()
        lab.ready(core="stopped")
        assert not core_pid(), "core update undid explicit Stop"
        print("PASS: signed feed, tampered manifest rejected, corrupt package preserves old PID/hash, independent procd download/install with management offline, current-version rejection, explicit Stop preserved")
    finally:
        if job().get("phase") in ("queued", "downloading", "installing"):
            wait_job({"succeeded", "failed"})
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready(core=None)
        lab.rpc("Router/SetMaintenance", original)
        wait_supervisor()
        if initial["core"] == "running":
            lab.rpc("App/Start")
            lab.ready()
        if server_pid:
            command("client", "kill", server_pid, check=False)
        command("client", "rm", "-rf", fixture)
        command("router", "rm", "-rf", updater_cache)
        command("router", "sh", "-c", f"if [ -d {backup_cache} ]; then mv {backup_cache} {updater_cache}; fi")


if __name__ == "__main__":
    main()
