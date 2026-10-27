#!/usr/bin/env python3
"""Verify real GEO downloads continue with the management service stopped."""
import json
import time
import uuid

from lab import Lab, command


def main():
    lab = Lab()
    original = lab.rpc("Router/Maintenance")["config"]
    initial = lab.rpc("App/State")
    path = "/etc/clashcube/geo-schedule.json"
    saved = command("router", "cat", path, check=False)
    fixture = "/tmp/clashcube-geo-" + uuid.uuid4().hex
    pid = profile = None
    try:
        command("router", "mkdir", fixture)
        command("router", "cp", "/etc/clashcube/core/geoip.metadb", fixture + "/geoip.metadb")
        command("router", "mkdir", fixture + "/cgi-bin")
        command("router", "tee", fixture + "/cgi-bin/geoip.metadb", input=
                f"#!/bin/sh\necho fetched >> {fixture}/fetched\nprintf 'Content-Type: application/octet-stream\\r\\n\\r\\n'\ncat {fixture}/geoip.metadb\n")
        command("router", "chmod", "755", fixture + "/cgi-bin/geoip.metadb")
        pid = command("router", "sh", "-c", f"uhttpd -f -p 127.0.0.1:18292 -h {fixture} > {fixture}/http.log 2>&1 & echo $!").stdout.strip()
        deadline = time.monotonic() + 5
        while command("router", "curl", "-sf", "-o", "/dev/null", "http://127.0.0.1:18292/", check=False).returncode:
            assert time.monotonic() < deadline, "fixture server did not start"
            time.sleep(0.1)
        profile = lab.rpc("Profiles/ImportContent", "P1 scheduled GEO smoke", 'geox-url: {mmdb: "http://127.0.0.1:18292/cgi-bin/geoip.metadb"}\nrules: ["GEOIP,CN,DIRECT", "MATCH,DIRECT"]\n')
        lab.rpc("Profiles/Use", profile["id"])
        core_pid = command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout
        command("router", "rm", "-f", path)
        lab.rpc("Router/SetMaintenance", {**original, "geoIntervalHours": 1})
        command("router", "/etc/init.d/clashcube", "stop")
        deadline = time.monotonic() + 50
        while time.monotonic() < deadline:
            result = command("router", "cat", path, check=False)
            state = json.loads(result.stdout) if result.returncode == 0 else {}
            if state.get("success", 0) > 0:
                break
            time.sleep(0.2)
        else:
            raise AssertionError("scheduled GEO update did not complete while management was offline")
        command("router", "test", "-s", fixture + "/fetched")
        assert command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core core -d ").stdout == core_pid
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready()
        assert lab.rpc("Router/Maintenance")["geo"]["success"] == state["success"]
        print("PASS: core-owned scheduled GEO download with management offline, unchanged core PID, persisted result visible after reconnection")
    finally:
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready()
        lab.rpc("Router/SetMaintenance", original)
        lab.rpc("Profiles/Use", initial["profile"])
        if profile:
            lab.rpc("Profiles/Remove", profile["id"])
        if pid:
            command("router", "kill", pid, check=False)
        command("router", "rm", "-rf", fixture)
        if saved.returncode == 0:
            command("router", "tee", path, input=saved.stdout)
        else:
            command("router", "rm", "-f", path)


if __name__ == "__main__":
    main()
