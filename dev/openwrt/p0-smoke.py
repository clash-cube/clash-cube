#!/usr/bin/env python3
"""Verify dnsmasq integration and persistent/RAM storage in the isolated lab."""
import struct
import json
import ipaddress
import time
import uuid

from lab import Lab, command, uci


def dns_test(lab):
    enabled = lab.rpc("Router/Get")["enabled"]
    lab.rpc("Router/SetEnabled", False)
    dns_before = uci("get", section="@dnsmasq[0]")["values"]
    lan_before = uci("get", section="lan")["values"]
    fixture = "cube_p0_" + uuid.uuid4().hex[:8]
    script = "/tmp/" + fixture + ".sh"
    leases = command("router", "cat", "/tmp/dhcp.leases", check=False)
    try:
        uci("add", type="domain", name=fixture, values={"name": fixture + ".lan", "ip": "192.168.77.42"})
        uci("set", section="lan", values={"ignore": "0", "dhcpv4": "server", "force": "1", "start": "100", "limit": "10", "leasetime": "2m"})
        command("router", "/etc/init.d/dnsmasq", "reload")
        lab.rpc("Router/SetEnabled", True)
        for server in ("172.30.77.2", "8.8.8.8"):
            answer = command("client", "nslookup", fixture + ".lan", server).stdout
            assert "192.168.77.42" in answer, "local hosts escaped dnsmasq"
            command("client", "nslookup", "example.com", server)
        query = struct.pack("!6H", 1234, 256, 1, 0, 0, 0) + b"".join(bytes([len(p)]) + p.encode() for p in (fixture, "lan")) + b"\0\0\1\0\1"
        wire = struct.pack("!H", len(query)) + query
        lua = '''require("nixio.util") local s=require("nixio").socket("inet","stream")
s:setsockopt("socket","rcvtimeo",5)
assert(s:connect("8.8.8.8",53))
assert(s:sendall(string.char(%s)))
local function read(n) local out="" while #out<n do local b=s:recv(n-#out) assert(b and #b>0) out=out..b end return out end
local h=read(2) local b=read(h:byte(1)*256+h:byte(2))
for i=1,#b do io.write(string.format("%%02x",b:byte(i))) end
s:close()''' % ",".join(str(b) for b in wire)
        reply = bytes.fromhex(command("client", "lua", "-e", lua).stdout)
        assert ipaddress.ip_address("192.168.77.42").packed in reply, "forced TCP DNS lost local records"
        # A real DHCP handshake records the hostname without replacing Docker's
        # static client address/routes: the lease script deliberately does not
        # apply the offered address to eth0.
        command("client", "tee", script, input='#!/bin/sh\nexit 0\n')
        command("client", "chmod", "755", script)
        hostname = fixture.replace("_", "-")
        command("client", "udhcpc", "-i", "eth0", "-f", "-n", "-q", "-t", "3", "-T", "2", "-x", "hostname:" + hostname, "-s", script)
        answer = command("client", "nslookup", hostname + ".lan", "8.8.8.8").stdout
        assert "172.30.77.1" in answer, "DHCP hostname was not answered locally"
        result = lab.rpc("Profiles/ImportContent", "P0 DNS loop rejection", 'dns: {nameserver: [127.0.0.1]}\nrules: ["MATCH,DIRECT"]\n')
        try:
            old_profile = lab.rpc("App/State")["profile"]
            try:
                lab.rpc("Profiles/Use", result["id"])
                raise AssertionError("accepted recursive DNS configuration")
            except AssertionError as error:
                assert "HTTP 422" in str(error), str(error)
            assert lab.rpc("App/State")["profile"] == old_profile
            command("client", "nslookup", "example.com", "8.8.8.8")
        finally:
            lab.rpc("Profiles/Remove", result["id"])
        lab.rpc("App/Stop")
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            if uci("get", section="@dnsmasq[0]")["values"] == dns_before:
                break
            time.sleep(0.1)
        assert uci("get", section="@dnsmasq[0]")["values"] == dns_before, "core stop did not restore original DNS options"
        command("client", "nslookup", "example.com", "172.30.77.2")
        lab.rpc("App/Start")
        print("PASS: public and forced DNS, local hosts, real DHCP hostname, recursive-upstream rollback, core-stop DNS restoration", flush=True)
    finally:
        lab.rpc("Router/SetEnabled", False)
        command("router", "/etc/init.d/dnsmasq", "stop")
        uci("delete", section=fixture)
        restore = {k: lan_before[k] for k in ("ignore", "dhcpv4", "force", "start", "limit", "leasetime") if k in lan_before}
        uci("set", section="lan", values=restore)
        for key in ("ignore", "dhcpv4", "force", "start", "limit", "leasetime"):
            if key not in lan_before:
                uci("delete", section="lan", option=key)
        if leases.returncode == 0:
            command("router", "tee", "/tmp/dhcp.leases", input=leases.stdout)
        command("router", "/etc/init.d/dnsmasq", "start")
        command("client", "rm", "-f", script)
        if enabled:
            lab.rpc("Router/SetEnabled", True)


def storage_test(lab):
    config = command("router", "cat", "/etc/config/clashcube").stdout
    marker = "/etc/clashcube/p0-persistent-marker"
    initial = lab.rpc("App/State")
    enabled = lab.rpc("Router/Get")["enabled"]
    auto_start = lab.rpc("Settings/Get")["autoStart"]
    lab.rpc("Settings/Patch", {"autoStart": True})
    lab.rpc("Router/SetEnabled", False)
    command("router", "touch", marker)

    def stop():
        command("router", "/etc/init.d/clashcube", "stop")
        command("router", "/etc/init.d/clashcube-core", "stop")

    def start():
        command("router", "/etc/init.d/clashcube-core", "boot")
        command("router", "/etc/init.d/clashcube", "start")
        lab.ready()

    try:
        stop()
        command("router", "uci", "set", "clashcube.main.storage_mode=ram")
        command("router", "uci", "commit", "clashcube")
        start()
        assert "-d /tmp/clashcube-data/core " in command("router", "pgrep", "-af", r"^/usr/bin/clashcube-core core -d ").stdout
        lab.rpc("App/Reload")  # Persistent runtime outside core home must remain a safe reload path.
        lab.rpc("App/Stop")  # A real boot honors AutoStart after a prior explicit stop.
        stop()
        command("router", "rm", "-rf", "/tmp/clashcube-data", "/tmp/clashcube-cache")
        start()
        command("router", "test", "-f", marker)
        assert lab.rpc("App/State")["profile"] == initial["profile"], "RAM loss removed selected profile"
        lab.rpc("App/Reload")
        stop()
        command("router", "uci", "set", "clashcube.main.storage_mode=external")
        command("router", "uci", "set", "clashcube.main.external_mount=/mnt/clashcube-p0-missing")
        command("router", "uci", "commit", "clashcube")
        command("router", "/etc/init.d/clashcube-core", "start", check=False)
        assert command("router", "pgrep", "-f", r"^/usr/bin/clashcube-core$", check=False).returncode != 0
        assert command("router", "test", "-e", "/mnt/clashcube-p0-missing", check=False).returncode != 0, "missing disk fell back to internal flash"
        print("PASS: RAM-mode reload, persistent configuration after cache loss, absent external disk fails closed", flush=True)
    finally:
        stop()
        command("router", "tee", "/etc/config/clashcube", input=config)
        start()
        command("router", "rm", "-f", marker)
        lab.rpc("Settings/Patch", {"autoStart": auto_start})
        if enabled:
            lab.rpc("Router/SetEnabled", True)


def main():
    lab = Lab()
    dns_test(lab)
    storage_test(lab)
    restore_test(lab)


def restore_test(lab):
    runtime = "/etc/clashcube/core/runtime.yaml"
    backup = runtime + ".p0-backup"
    enabled = lab.rpc("Router/Get")["enabled"]
    lab.rpc("Router/SetEnabled", False)
    command("router", "/etc/init.d/clashcube", "stop")
    command("router", "/etc/init.d/clashcube-core", "stop")
    command("router", "cp", "-p", runtime, backup)
    runtime_restored = False
    try:
        # Valid YAML and controller, rejected by mihomo's mode validation.
        command("router", "sed", "-i", "s/^mode: .*/mode: p0-invalid/", runtime)
        command("router", "/etc/init.d/clashcube-core", "start")
        command("router", "/etc/init.d/clashcube", "start")
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            result = command("router", "curl", "-sf", "--unix-socket", "/var/run/clashcube-core/control.sock", "http://localhost/rpc/Core/Status", "-d", "[]", check=False)
            if result.returncode == 0 and json.loads(result.stdout).get("error"):
                break
            time.sleep(0.2)
        else:
            raise AssertionError("failed startup did not report a pending restore error")
        lab.ready(core=None)
        lab.rpc("App/Stop")
        command("router", "cp", "-p", backup, runtime)
        runtime_restored = True
        # Wait beyond the supervisor's real retry interval; fixing the config
        # must not undo an explicit Stop made during failed boot restoration.
        time.sleep(6)
        assert lab.rpc("App/State")["core"] == "stopped", "pending restore ignored explicit Stop"
        lab.rpc("App/Start")
        lab.ready()
        print("PASS: explicit Stop cancels failed boot restoration after its configuration is repaired", flush=True)
    finally:
        if not runtime_restored:
            command("router", "cp", "-p", backup, runtime)
        command("router", "rm", "-f", backup)
        command("router", "/etc/init.d/clashcube", "restart")
        lab.ready(core=None)
        lab.rpc("App/Start")
        lab.ready()
        lab.rpc("Router/SetEnabled", enabled)


if __name__ == "__main__":
    main()
