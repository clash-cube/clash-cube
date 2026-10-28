"""Local-only origin, two distinct HTTP proxies and DNS for leak assertions."""
import json
import select
import socket
import socketserver
import struct
import sys
import threading
import time
from collections import Counter
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

counts = Counter()


def dns_reply(data):
    counts["dns"] += 1
    end = 12
    while data[end]:
        end += data[end] + 1
    end += 5
    question = data[12:end]
    # A deterministic public-looking answer; no external resolver is contacted.
    return data[:2] + b"\x81\x80\x00\x01\x00\x01\x00\x00\x00\x00" + question + b"\xc0\x0c\x00\x01\x00\x01\x00\x00\x00\x1e\x00\x04\xcb\x00\x71\x0a"


def recvn(sock, n):
    data = b""
    while len(data) < n:
        part = sock.recv(n - len(data))
        if not part:
            raise OSError("closed")
        data += part
    return data


class DNS(socketserver.BaseRequestHandler):
    def handle(self):
        try:
            while True:
                size = struct.unpack("!H", recvn(self.request, 2))[0]
                counts["tcp-dns-proxy" if self.client_address[0].startswith("127.") else "tcp-dns-direct"] += 1
                reply = dns_reply(recvn(self.request, size))
                self.request.sendall(struct.pack("!H", len(reply)) + reply)
        except OSError:
            pass


class Proxy(socketserver.StreamRequestHandler):
    def handle(self):
        try:
            method, target, _ = self.rfile.readline().decode().split()
            while self.rfile.readline() not in (b"\r\n", b"\n", b""):
                pass
            if method != "CONNECT":
                return
            host, port = target.rsplit(":", 1)
            port = int(port)
            if (host, port) == ("1.1.1.1", 53):
                host = "127.0.0.1"
            elif host not in ("172.30.76.10", "203.0.113.10") or port != 8080:
                return
            upstream = socket.create_connection((host, port), timeout=4, source_address=(self.server.source, 0))
            counts[self.server.label] += 1
            self.wfile.write(b"HTTP/1.1 200 Connection established\r\n\r\n")
            self.wfile.flush()
            with upstream:
                while True:
                    readable, _, _ = select.select([self.connection, upstream], [], [], 8)
                    if not readable:
                        return
                    for src in readable:
                        data = src.recv(65536)
                        if not data:
                            return
                        (upstream if src is self.connection else self.connection).sendall(data)
        except (OSError, ValueError):
            pass


class Origin(BaseHTTPRequestHandler):
    def do_GET(self):
        source = self.client_address[0]
        marker = {"127.0.0.2": "NODE-A", "127.0.0.3": "NODE-B"}.get(source, "DIRECT")
        counts[marker] += 1
        if self.path == "/stream":
            self.send_response(200)
            self.end_headers()
            try:
                for _ in range(300):
                    self.wfile.write((marker + "\n").encode())
                    self.wfile.flush()
                    time.sleep(0.05)
            except OSError:
                pass
            return
        body = (json.dumps(dict(counts)) if self.path == "/stats" else marker).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


def server():
    class TCP(socketserver.ThreadingTCPServer):
        allow_reuse_address = True
        daemon_threads = True
    dns = TCP(("0.0.0.0", 53), DNS)
    threading.Thread(target=dns.serve_forever, daemon=True).start()
    def udp(port, is_dns):
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.bind(("0.0.0.0", port))
            while True:
                data, addr = s.recvfrom(4096)
                counts["direct-dns" if is_dns else "direct-udp"] += 1
                s.sendto(dns_reply(data) if is_dns else b"DIRECT-UDP", addr)
    for port, flag in [(53, True), (9000, False)]:
        threading.Thread(target=udp, args=(port, flag), daemon=True).start()
    for port, label, source in [(18081, "proxy-A", "127.0.0.2"), (18082, "proxy-B", "127.0.0.3")]:
        s = TCP(("0.0.0.0", port), Proxy)
        s.label, s.source = label, source
        threading.Thread(target=s.serve_forever, daemon=True).start()
    class HTTP6(ThreadingHTTPServer):
        address_family = socket.AF_INET6
        def server_bind(self):
            self.socket.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
            super().server_bind()
    v6 = HTTP6(("::", 8080), Origin)
    threading.Thread(target=v6.serve_forever, daemon=True).start()
    ThreadingHTTPServer(("0.0.0.0", 8080), Origin).serve_forever()


def probe(kind, host="172.30.76.10", port=None):
    try:
        family = socket.AF_INET6 if ":" in host else socket.AF_INET
        with socket.socket(family, socket.SOCK_DGRAM if kind in ("udp", "dns") else socket.SOCK_STREAM) as s:
            s.settimeout(0.65)
            s.connect((host, int(port or (53 if kind.startswith("dns") else 9000 if kind == "udp" else 8080))))
            if kind.startswith("dns"):
                query = b"\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01"
                if kind == "dns-tcp":
                    s.sendall(struct.pack("!H", len(query)) + query)
                    answer = recvn(s, struct.unpack("!H", recvn(s, 2))[0])
                else:
                    s.sendall(query)
                    answer = s.recv(4096)
                if answer[3] & 15:
                    return "SERVFAIL"
                return "DNS-OK" if answer.endswith(socket.inet_aton("203.0.113.10")) else "DNS-WRONG"
            request = b"GET http://172.30.76.10:8080/ HTTP/1.0\r\nHost: 172.30.76.10:8080\r\n\r\n" if kind == "proxy" else b"GET / HTTP/1.0\r\nHost: fixture\r\n\r\n"
            s.sendall(b"echo" if kind == "udp" else request)
            if kind == "udp":
                return s.recv(4096).decode()
            answer = b""
            while chunk := s.recv(4096):
                answer += chunk
            return answer.split(b"\r\n\r\n", 1)[1].decode()
    except (OSError, IndexError):
        return "BLOCKED"


if __name__ == "__main__":
    if sys.argv[1] == "server":
        server()
    elif sys.argv[1] == "probe":
        print(probe(*sys.argv[2:]))
    elif sys.argv[1] == "poll":
        end = time.monotonic() + float(sys.argv[2])
        result = Counter()
        while time.monotonic() < end:
            result[probe("tcp")] += 1
            time.sleep(0.02)
        print(json.dumps(dict(result)), flush=True)
    elif sys.argv[1] == "stream":
        try:
            with socket.create_connection(("172.30.76.10", 8080), timeout=1) as s:
                s.settimeout(1.2)
                s.sendall(b"GET /stream HTTP/1.0\r\nHost: fixture\r\n\r\n")
                data = b""
                while b"\r\n\r\n" not in data:
                    data += s.recv(4096)
                print("CONNECTED", flush=True)
                while data := s.recv(4096):
                    print("DATA", flush=True)
        except OSError:
            pass
        print("CLOSED", flush=True)
    else:
        while True:
            time.sleep(3600)
