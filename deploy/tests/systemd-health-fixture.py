#!/usr/bin/env python3

import argparse
import hashlib
import http.server
import json
import os
import re
import socketserver
import subprocess
import sys


IDENTITY = re.compile(
    r"^Sub2API Deployer (?P<version>\S+) \(commit: (?P<commit>[^,]+), "
    r"built: (?P<date>[^,]+), type: (?P<type>[^,]+), arch: (?P<arch>[^)]+)\)$"
)


def file_digest(path):
    digest = hashlib.sha256()
    with open(path, "rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return "sha256:" + digest.hexdigest()


def read_identity(binary):
    output = subprocess.check_output([binary, "--version"], text=True).strip()
    match = IDENTITY.match(output)
    if not match:
        raise RuntimeError("invalid deployer build identity: " + output)
    return match.groupdict()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--socket", required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--state", required=True)
    parser.add_argument("--reject-sha-file", required=True)
    args = parser.parse_args()

    identity = read_identity(args.binary)
    binary_sha = file_digest(args.binary)
    if os.path.exists(args.reject_sha_file):
        with open(args.reject_sha_file, encoding="utf-8") as source:
            if source.read().strip() == binary_sha:
                return 42

    with open(args.state, encoding="utf-8") as source:
        state = json.load(source)

    health = {
        "status": "ok",
        "active_container_id": state["active_container_id"],
        "active_version": state["active_version"],
        "job_running": False,
        "degraded": False,
        "control_plane_upgrade_ready": True,
        "control_plane": {
            "activator": "go-v1",
            "payload_schema_min": 1,
            "payload_schema_max": 1,
            "installed_sha256": binary_sha,
        },
        "build": {
            **identity,
            "sha256": binary_sha,
        },
    }

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path != "/v1/health":
                self.send_error(404)
                return
            body = json.dumps(health, separators=(",", ":")).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, _format, *_args):
            return

    class Server(socketserver.UnixStreamServer):
        allow_reuse_address = True

    os.makedirs(os.path.dirname(args.socket), mode=0o755, exist_ok=True)
    try:
        os.unlink(args.socket)
    except FileNotFoundError:
        pass
    with Server(args.socket, Handler) as server:
        server.serve_forever()
    return 0


if __name__ == "__main__":
    sys.exit(main())
