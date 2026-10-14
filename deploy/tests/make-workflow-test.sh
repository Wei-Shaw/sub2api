#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
cd "$repo_root"

python3 - <<'PY'
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

root = Path.cwd()
docker_executable = shutil.which("docker")
env = dict(os.environ, POSTGRES_PASSWORD="workflow-check-only")
render = subprocess.run(
    ["docker", "compose", "--env-file", "/dev/null", "-p", "sub2api-dev",
     "-f", "deploy/docker-compose.dev.yml", "-f", "deploy/docker-compose.dev.override.yml",
     "config", "--format", "json"],
    env=env, check=True, capture_output=True, text=True,
)
services = json.loads(render.stdout)["services"]
assert services["sub2api"]["image"] == "sub2api:local"
for name in ("sub2api", "postgres", "redis"):
    assert all(v["type"] == "volume" for v in services[name]["volumes"]), "NTFS bind mount remains"

with tempfile.TemporaryDirectory() as directory:
    fixture = Path(directory)
    (fixture / "backend/bin").mkdir(parents=True)
    (fixture / "tools").mkdir()
    shutil.copy(root / "Makefile", fixture / "Makefile")
    shutil.copy(root / "backend/Makefile", fixture / "backend/Makefile")
    (fixture / "tools/pnpm").write_text("#!/bin/sh\nprintf 'frontend\\n' >> \"$WORKFLOW_EVENTS\"\n")
    (fixture / "tools/go").write_text('''#!/bin/sh
case "$GOFLAGS" in *-tags=embed*) ;; *) exit 1 ;; esac
printf 'backend\n' >> "$WORKFLOW_EVENTS"
cat > bin/server <<'SERVER'
#!/bin/sh
printf 'run\n' >> "$WORKFLOW_EVENTS"
SERVER
chmod +x bin/server
''')
    for tool in ("pnpm", "go"):
        (fixture / "tools" / tool).chmod(0o755)
    test_env = dict(os.environ, PATH=f"{fixture / 'tools'}:{os.environ['PATH']}",
                    WORKFLOW_EVENTS=str(fixture / "events"))
    subprocess.run(["make", "run", "VERSION=workflow-test"], cwd=fixture,
                   env=test_env, check=True, capture_output=True)
    assert (fixture / "events").read_text().splitlines() == ["frontend", "backend", "run"]

# Exercise server credential persistence without SSH or real Docker operations.
remote_program = (root / "deploy/deploy.sh").read_text().split("<<'REMOTE'\n", 1)[1].split("\nREMOTE\n", 1)[0]
with tempfile.TemporaryDirectory() as directory:
    folder = Path(directory)
    (folder / "tools").mkdir()
    docker_stub = folder / "tools/docker"
    docker_stub.write_text("#!/bin/sh\nexit 0\n")
    docker_stub.chmod(0o755)
    test_env = dict(os.environ, PATH=f"{folder / 'tools'}:{os.environ['PATH']}")
    def configure(image, port):
        subprocess.run(
            ["bash", "-s", "--", str(folder), image, "my-sub2api", str(port), "127.0.0.1"],
            input=remote_program, env=test_env, check=True, capture_output=True, text=True,
        )
        return dict(line.split("=", 1) for line in (folder / ".env").read_text().splitlines())
    first = configure("my-sub2api:first", 8083)
    assert (folder / ".env").stat().st_mode & 0o777 == 0o600
    assert first["BIND_HOST"] == "127.0.0.1" and first["SERVER_PORT"] == "8083"
    second = configure("my-sub2api:second", 8084)
    assert second.pop("SERVER_PORT") == "8084"
    first.pop("SERVER_PORT")
    assert first == second, "update replaced server credentials"
    override = (folder / "docker-compose.override.yml").read_text()
    assert "image: my-sub2api:second" in override and "container_name: my-sub2api-postgres" in override
    shutil.copy(root / "deploy/docker-compose.local.yml", folder / "docker-compose.yml")
    result = subprocess.run(
        [docker_executable, "compose", "-p", "my-sub2api", "--env-file", ".env",
         "-f", "docker-compose.yml", "-f", "docker-compose.override.yml", "config", "--format", "json"],
        cwd=folder, check=True, capture_output=True, text=True,
    )
    deployed = json.loads(result.stdout)["services"]
    assert deployed["sub2api"]["image"] == "my-sub2api:second"
    assert deployed["sub2api"]["container_name"] == "my-sub2api"
    assert deployed["sub2api"]["ports"][0]["host_ip"] == "127.0.0.1"
    assert str(deployed["sub2api"]["ports"admin/bug-fix][0]["published"]) == "8084"
    for name in ("postgres", "redis"):
        assert deployed[name]["container_name"] == f"my-sub2api-{name}"
        assert not deployed[name].get("ports")
        assert deployed[name]["volumes"][0]["source"].startswith(str(folder) + "/")

print("make workflow test passed")
PY
