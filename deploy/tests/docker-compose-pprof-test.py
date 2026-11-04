#!/usr/bin/env python3
"""Render every Compose variant and verify opt-in diagnostics wiring.

Uses Docker Compose's own interpolation/validation; no containers or dependencies
are started, and no YAML parser dependency is required.
"""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
FILES = (
    "docker-compose.yml",
    "docker-compose.local.yml",
    "docker-compose.standalone.yml",
    "docker-compose.dev.yml",
)


class DiagnosticComposeTest(unittest.TestCase):
    def test_default_enabled_and_custom_port(self):
        for filename in FILES:
            for enabled, port in ((None, None), ("true", "6060"), ("true", "16060")):
                with self.subTest(file=filename, enabled=enabled, port=port):
                    env = os.environ.copy()
                    for key in ("SUB2API_PPROF_ENABLED", "SUB2API_PPROF_PORT"):
                        env.pop(key, None)
                    env.update(
                        POSTGRES_PASSWORD="compose-test-only",
                        DATABASE_HOST="postgres",
                        DATABASE_PASSWORD="compose-test-only",
                        REDIS_HOST="redis",
                        SERVER_PORT="18080",
                        BIND_HOST="127.0.0.1",
                    )
                    if enabled is not None:
                        env["SUB2API_PPROF_ENABLED"] = enabled
                    if port is not None:
                        env["SUB2API_PPROF_PORT"] = port
                    # Avoid implicitly loading an operator's deploy/.env.
                    with tempfile.NamedTemporaryFile() as empty_env:
                        result = subprocess.run(
                            [
                                "docker", "compose", "--env-file", empty_env.name,
                                "-f", str(ROOT / "deploy" / filename),
                                "config", "--format", "json",
                            ],
                            env=env, capture_output=True, text=True, check=True,
                        )
                    service = json.loads(result.stdout)["services"]["sub2api"]
                    self.assertEqual(
                        service["environment"]["SUB2API_PPROF_ENABLED"],
                        enabled or "false",
                    )
                    expected_port = port or "6060"
                    self.assertEqual(
                        service["environment"]["SUB2API_PPROF_PORT"], expected_port,
                    )
                    diagnostic_ports = [
                        p for p in service["ports"] if str(p["target"]) == expected_port
                    ]
                    self.assertEqual(len(diagnostic_ports), 1)
                    published = diagnostic_ports[0]
                    self.assertEqual(str(published["published"]), expected_port)
                    self.assertEqual(published["protocol"], "tcp")
                    self.assertIn(published.get("host_ip"), (None, "", "0.0.0.0"))
                    business_ports = [p for p in service["ports"] if p["target"] == 8080]
                    self.assertEqual(len(business_ports), 1)
                    self.assertEqual(str(business_ports[0]["published"]), "18080")
                    self.assertEqual(business_ports[0]["host_ip"], "127.0.0.1")


if __name__ == "__main__":
    unittest.main()
