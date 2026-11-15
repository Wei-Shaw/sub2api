import copy
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
from contextlib import redirect_stderr

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("preflight", ROOT / "deploy/check-router-production.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


def fixture():
    rows = []
    for name in check.NAMES:
        nets = {net: {} for net in check.NETWORKS[name]}
        mounts = []
        host_config = {"PortBindings": copy.deepcopy(check.PORTS.get(name, {}))}
        config = {}
        if name in check.VOLUMES:
            volume, dest = check.VOLUMES[name]
            mounts.append({"Type": "volume", "Name": volume, "Destination": dest})
        if name in check.BINDS:
            source, dest = check.BINDS[name]
            mounts.append({"Type": "bind", "Source": source, "Destination": dest, "RW": False})
        if name in check.ENV:
            config["Env"] = [key + "=" + value for key, value in check.ENV[name].items()]
        if name in check.MEMORY:
            host_config["Memory"], host_config["MemorySwap"] = check.MEMORY[name]
        if name == "nanafox-studio-formal-app":
            config["StopTimeout"] = 600
        rows.append({"Name": "/" + name, "State": {"Running": True}, "NetworkSettings": {"Networks": nets}, "Mounts": mounts, "HostConfig": host_config, "Config": config})
    return rows


class ProductionTargetTests(unittest.TestCase):
    def test_expected_layout(self):
        self.assertEqual(check.validate(check.HOST, fixture()), [])

    def test_wrong_host_never_accesses_docker(self):
        with patch.object(check.socket, "gethostname", return_value="old-host"), patch.object(check.subprocess, "run") as run, redirect_stderr(io.StringIO()):
            self.assertEqual(check.main(), 1)
            run.assert_not_called()

    def test_rejects_old_database_volume(self):
        rows = fixture(); rows[1]["Mounts"][0]["Name"] = "fx-production-pgdata"
        self.assertIn("nanafox-router-formal-postgres: unexpected data volume", check.validate(check.HOST, rows))

    def test_rejects_old_config(self):
        rows = fixture(); rows[0]["Mounts"][1]["Source"] = "/home/nio/recovery/live-production-20260906/router.preview.json"
        self.assertIn("nanafox-router-formal-router: unexpected config mount", check.validate(check.HOST, rows))

    def test_rejects_missing_egress(self):
        rows = fixture(); del rows[0]["NetworkSettings"]["Networks"]["nanafox-router-formal_egress"]
        self.assertTrue(check.validate(check.HOST, rows))

    def test_rejects_public_database_port(self):
        rows = fixture(); rows[1]["HostConfig"]["PortBindings"] = {"5432/tcp": [{"HostIp": "0.0.0.0", "HostPort": "5432"}]}
        self.assertTrue(check.validate(check.HOST, rows))

    def test_rejects_old_app_port(self):
        rows = fixture(); rows[0]["HostConfig"]["PortBindings"]["8080/tcp"][0]["HostPort"] = "18080"
        self.assertTrue(check.validate(check.HOST, rows))

    def test_rejects_old_router_memory_limit(self):
        rows = fixture(); rows[0]["HostConfig"]["Memory"] = 805306368
        self.assertIn("nanafox-router-formal-router: unexpected memory limit", check.validate(check.HOST, rows))

    def test_rejects_writable_caddyfile(self):
        rows = fixture(); rows[5]["Mounts"][0]["RW"] = True
        self.assertIn("nanafox-gateway: unexpected config mount", check.validate(check.HOST, rows))

    def test_rejects_missing_or_stopped_container(self):
        self.assertTrue(check.validate(check.HOST, fixture()[:-1]))
        rows = fixture(); rows[0]["State"]["Running"] = False
        self.assertTrue(check.validate(check.HOST, rows))

    def test_origin_failure_does_not_print_value(self):
        rows = fixture(); rows[3]["Config"]["Env"][0] = "STUDIO_PUBLIC_ORIGIN=secret-sentinel"
        errors = check.validate(check.HOST, rows)
        self.assertTrue(errors)
        self.assertNotIn("secret-sentinel", str(errors))

    def test_legacy_prod_exits_before_tools(self):
        # Any accidental git/docker invocation is an observable failure.
        import os
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); marker = root / "called"
            for tool in ("git", "docker", "hostname", "mkdir"):
                p = root / tool
                p.write_text("#!/bin/sh\n: > '" + str(marker) + "'\nexit 99\n")
                p.chmod(0o755)
            result = subprocess.run(["/bin/bash", str(ROOT / "deploy/deploy-server.sh"), "prod"], env={**os.environ, "PATH": tmp}, capture_output=True)
            self.assertEqual(result.returncode, 64)
            self.assertFalse(marker.exists())

    def test_legacy_test_refuses_production_host(self):
        import os
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp) / "hostname"; p.write_text("#!/bin/sh\necho " + check.HOST + "\n"); p.chmod(0o755)
            result = subprocess.run(["/bin/bash", str(ROOT / "deploy/deploy-server.sh"), "test"], env={**os.environ, "PATH": tmp}, capture_output=True)
            self.assertEqual(result.returncode, 64)


if __name__ == "__main__":
    unittest.main()
