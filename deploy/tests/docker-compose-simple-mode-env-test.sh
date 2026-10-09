#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

python3 - <<'PY'
import json
import os
from pathlib import Path
import subprocess

key = "SIMPLE_MODE_AUTO_CREATE_DEFAULT_GROUPS"
sample = Path("deploy/.env.example").read_text().splitlines()
assert [line for line in sample if line.startswith(key + "=")] == [key + "="], \
    ".env.example must preserve YAML configuration by leaving the override empty"

for filename in (
    "docker-compose.yml",
    "docker-compose.local.yml",
    "docker-compose.standalone.yml",
    "docker-compose.dev.yml",
):
    path = Path("deploy") / filename
    expected = "      - " + key + "=${" + key + ":-}"
    assert path.read_text().splitlines().count(expected) == 1, \
        f"{path} must pass the override with an empty fallback exactly once"
    for value in (None, "", "true", "false"):
        env = dict(
            os.environ, POSTGRES_PASSWORD="compose-test-password",
            DATABASE_HOST="postgres", DATABASE_PASSWORD="compose-test-password",
            REDIS_HOST="redis",
        )
        env.pop(key, None)
        if value is not None:
            env[key] = value
        result = subprocess.run(
            ["docker", "compose", "--env-file", "/dev/null", "-f", str(path),
             "config", "--format", "json"],
            env=env, check=True, capture_output=True, text=True,
        )
        actual = json.loads(result.stdout)["services"]["sub2api"]["environment"][key]
        assert actual == (value or ""), f"{path}: override {value!r} rendered as {actual!r}"

print("docker compose simple mode environment test passed")
PY
