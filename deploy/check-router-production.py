#!/usr/bin/env python3
"""Read-only production target check. Never prints Docker env or runs mutations."""
import json
import socket
import subprocess
import sys

HOST = "cuii930539864895"
ROUTER_DIR = "/srv/nanafox-router-formal"
STUDIO_DIR = "/srv/nanafox-studio-formal"
GATEWAY_DIR = "/srv/nanafox-gateway"
NAMES = [
    "nanafox-router-formal-router",
    "nanafox-router-formal-postgres",
    "nanafox-router-formal-redis",
    "nanafox-studio-formal-app",
    "nanafox-studio-formal-postgres",
    "nanafox-gateway",
]
NETWORKS = {
    "nanafox-router-formal-router": {"nanafox-router-formal_database", "nanafox-router-formal_egress"},
    "nanafox-router-formal-postgres": {"nanafox-router-formal_database"},
    "nanafox-router-formal-redis": {"nanafox-router-formal_database"},
    "nanafox-studio-formal-app": {"nanafox-studio-formal_internal", "nanafox-studio-formal_egress"},
    "nanafox-studio-formal-postgres": {"nanafox-studio-formal_internal"},
    "nanafox-gateway": {"host"},
}
VOLUMES = {
    "nanafox-router-formal-router": ("nanafox-router-formal_routerdata", "/app/data"),
    "nanafox-router-formal-postgres": ("nanafox-router-formal_pgdata", "/var/lib/postgresql"),
    "nanafox-router-formal-redis": ("nanafox-router-formal_redisdata", "/data"),
    "nanafox-studio-formal-app": ("nanafox-studio-formal-data-20261110T021929Z", "/data"),
    "nanafox-studio-formal-postgres": ("nanafox-studio-formal-pg-20261110T021929Z", "/var/lib/postgresql"),
}
BINDS = {
    "nanafox-router-formal-router": (ROUTER_DIR + "/config.original.json", "/app/data/config.yaml"),
    "nanafox-gateway": (GATEWAY_DIR + "/Caddyfile", "/etc/caddy/Caddyfile"),
}
PORTS = {
    "nanafox-router-formal-router": {"8080/tcp": [{"HostIp": "127.0.0.1", "HostPort": "18083"}]},
    "nanafox-studio-formal-app": {"8788/tcp": [{"HostIp": "127.0.0.1", "HostPort": "18790"}]},
}
ENV = {
    "nanafox-router-formal-router": {"GOMEMLIMIT": "900MiB", "GATEWAY_REQUEST_BODY_MEMORY_BUDGET_BYTES": "268435456"},
    "nanafox-studio-formal-app": {"STUDIO_PUBLIC_ORIGIN": "https://studio.nanafox.com", "ROUTER_AUTH_BASE_URL": "https://router.nanafox.com"},
}
# Router: mem_limit 1g, memswap_limit 1280m (see docs/ops/2026-11-15-router-oom-memory-limits.md).
MEMORY = {"nanafox-router-formal-router": (1073741824, 1342177280)}


def validate(hostname, containers):
    errors = []
    if hostname != HOST:
        return ["wrong production hostname"]
    rows = {row.get("Name", "").lstrip("/"): row for row in containers}
    for name in NAMES:
        if name not in rows:
            errors.append(name + ": missing")
            continue
        row = rows[name]
        if not row.get("State", {}).get("Running"):
            errors.append(name + ": not running")
        nets = set(row.get("NetworkSettings", {}).get("Networks", {}))
        if nets != NETWORKS[name]:
            errors.append(name + ": unexpected networks")
        mounts = row.get("Mounts", [])
        if name in VOLUMES:
            volume, dest = VOLUMES[name]
            if not any(m.get("Type") == "volume" and m.get("Name") == volume and m.get("Destination") == dest for m in mounts):
                errors.append(name + ": unexpected data volume")
        if name in BINDS:
            source, dest = BINDS[name]
            if not any(m.get("Type") == "bind" and m.get("Source") == source
                       and m.get("Destination") == dest and m.get("RW") is False for m in mounts):
                errors.append(name + ": unexpected config mount")
        ports = row.get("HostConfig", {}).get("PortBindings") or {}
        if ports != PORTS.get(name, {}):
            errors.append(name + ": unexpected published ports")
        if name in ENV:
            env = dict(v.split("=", 1) for v in row.get("Config", {}).get("Env", []) if "=" in v)
            for key, value in ENV[name].items():
                if env.get(key) != value:
                    errors.append(name + ": unexpected " + key)
        if name in MEMORY:
            host_config = row.get("HostConfig", {})
            if (host_config.get("Memory"), host_config.get("MemorySwap")) != MEMORY[name]:
                errors.append(name + ": unexpected memory limit")
        if name == "nanafox-studio-formal-app" and row.get("Config", {}).get("StopTimeout") != 600:
            errors.append(name + ": unexpected stop timeout")
    return errors


def main():
    if socket.gethostname() != HOST:
        print("FAIL: wrong production hostname; Docker was not accessed", file=sys.stderr)
        return 1
    try:
        result = subprocess.run(["docker", "inspect", *NAMES], capture_output=True, text=True, timeout=30, check=True)
        errors = validate(socket.gethostname(), json.loads(result.stdout))
    except (OSError, ValueError, subprocess.SubprocessError):
        print("FAIL: cannot inspect required production containers", file=sys.stderr)
        return 1
    for error in errors:
        print("FAIL: " + error, file=sys.stderr)
    if errors:
        return 1
    print("PASS: production host, Router/Studio/gateway containers, ports, mounts, networks, Router limits and Studio origin match")
    print("Read-only targeting check only; app health, DB connections and backup freshness still require verification")
    return 0


if __name__ == "__main__":
    sys.exit(main())
