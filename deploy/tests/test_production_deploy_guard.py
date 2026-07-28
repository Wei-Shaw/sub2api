import os
from pathlib import Path
import shutil
import subprocess
import unittest


DEPLOY = Path(__file__).resolve().parents[1]
GUARD = DEPLOY / "production-deploy-guard.sh"
DISK_GUARD = DEPLOY / "production-disk-readback.sh"
COMPOSE = DEPLOY / "docker-compose.local.yml"
MANIFEST = DEPLOY / "production-deploy.conf.example"
RUNBOOK = DEPLOY / "PRODUCTION_DISK_AND_DEPLOY.md"
SYSTEMD_UNIT = DEPLOY / "sub2api.service"


def bash_executable():
    found = shutil.which("bash")
    if found:
        return found
    candidate = Path(r"C:\Program Files\Git\bin\bash.exe")
    return str(candidate) if candidate.exists() else None


class ProductionDeployGuardTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.bash = bash_executable()

    def run_guard_test(self, *args):
        if not self.bash:
            self.skipTest("bash is unavailable")
        env = os.environ.copy()
        env["SUB2API_DEPLOY_TEST_MODE"] = "1"
        return subprocess.run(
            [self.bash, str(GUARD), *map(str, args)],
            text=True,
            capture_output=True,
            env=env,
        )

    def run_disk_test(self, *args):
        if not self.bash:
            self.skipTest("bash is unavailable")
        env = os.environ.copy()
        env["SUB2API_DISK_GUARD_TEST_MODE"] = "1"
        return subprocess.run(
            [self.bash, str(DISK_GUARD), *map(str, args)],
            text=True,
            capture_output=True,
            env=env,
        )

    def test_shell_scripts_parse(self):
        if not self.bash:
            self.skipTest("bash is unavailable")
        for script in (GUARD, DISK_GUARD):
            result = subprocess.run(
                [self.bash, "-n", str(script)], text=True, capture_output=True
            )
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_failure_injection_invokes_recovery_contract(self):
        for point in (
            "after_backup",
            "after_migration_restart",
            "after_infrastructure_probes",
            "after_paid_probes",
            "after_policy_readback",
        ):
            result = self.run_guard_test("--self-test-failure", point)
            self.assertNotEqual(result.returncode, 0)
            output = result.stdout + result.stderr
            self.assertIn("SELF_TEST_ROLLBACK=self-test", output)
            self.assertIn(f"failure injection: {point}", output)

    def test_compose_and_systemd_runner_detection(self):
        compose = self.run_guard_test("--self-test-runner", "compose", 1, 1, 0)
        self.assertEqual(compose.returncode, 0, compose.stderr)
        self.assertEqual(compose.stdout.strip(), "compose")

        systemd = self.run_guard_test("--self-test-runner", "systemd", 0, 0, 1)
        self.assertEqual(systemd.returncode, 0, systemd.stderr)
        self.assertEqual(systemd.stdout.strip(), "systemd")

    def test_required_grok_media_tables_fail_closed_for_both_runners(self):
        required_tables = (
            "public.grok_video_request_owners",
            "public.grok_video_create_idempotency",
            "public.grok_image_create_idempotency",
        )
        script = GUARD.read_text(encoding="utf-8")
        compose_probe = script.split("probe_compose_infrastructure() {", 1)[1].split(
            "probe_systemd_infrastructure() {", 1
        )[0]
        systemd_probe = script.split("probe_systemd_infrastructure() {", 1)[1].split(
            "deploy_compose_app() {", 1
        )[0]

        for runner, probe in (
            ("compose", compose_probe),
            ("systemd", systemd_probe),
        ):
            with self.subTest(runner=runner, case="all-present"):
                result = self.run_guard_test(
                    "--self-test-required-grok-media-tables", runner, "t", "t", "t"
                )
                self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(
                f'required_grok_media_tables_present {runner} "$table_probe"', probe
            )
            for table in required_tables:
                self.assertIn(table, probe)

            for missing_index, missing_table in enumerate(required_tables):
                values = ["t", "t", "t"]
                values[missing_index] = "f"
                with self.subTest(runner=runner, missing=missing_table):
                    result = self.run_guard_test(
                        "--self-test-required-grok-media-tables", runner, *values
                    )
                    self.assertNotEqual(result.returncode, 0)

    def test_compose_data_digest_drift_fails_before_quiesce_or_app_change(self):
        digest_a = "repo@sha256:" + "a" * 64
        digest_b = "repo@sha256:" + "b" * 64
        cases = (
            (digest_b, digest_a, digest_a, digest_a, "POSTGRES_IMAGE"),
            (digest_a, digest_b, digest_a, digest_a, "REDIS_IMAGE"),
        )
        for target_pg, target_redis, running_pg, running_redis, name in cases:
            with self.subTest(name=name):
                result = self.run_guard_test(
                    "--self-test-compose-data-targets",
                    target_pg,
                    target_redis,
                    running_pg,
                    running_redis,
                )
                self.assertNotEqual(result.returncode, 0)
                output = result.stdout + result.stderr
                self.assertIn(name, output)
                self.assertIn("PREQUIESCE_CHECK=failed", output)
                self.assertIn("QUIESCE_WRITES=0", output)
                self.assertIn("APP_CHANGES=0", output)

        matched = self.run_guard_test(
            "--self-test-compose-data-targets",
            digest_a,
            digest_a,
            digest_a,
            digest_a,
        )
        self.assertEqual(matched.returncode, 0, matched.stderr)
        self.assertIn("PREQUIESCE_CHECK=passed", matched.stdout)

    def test_zero_dual_ambiguous_and_mismatched_runner_fail_closed(self):
        cases = (
            ("compose", 0, 0, 0),
            ("systemd", 1, 1, 1),
            ("compose", 0, 0, 1),
            ("systemd", 1, 1, 0),
            ("compose", 1, 2, 0),
        )
        for case in cases:
            with self.subTest(case=case):
                result = self.run_guard_test("--self-test-runner", *case)
                self.assertNotEqual(result.returncode, 0)

    def test_70_80_90_states_and_recovery_hysteresis(self):
        expected = {
            ("normal", 69): "normal",
            ("normal", 70): "alert",
            ("normal", 79): "alert",
            ("normal", 80): "block",
            ("normal", 89): "block",
            ("normal", 90): "incident",
            ("block", 75): "block",
            ("block", 74): "alert",
            ("incident", 70): "incident",
            ("incident", 69): "normal",
        }
        for (prior, percent), state in expected.items():
            with self.subTest(prior=prior, percent=percent):
                result = self.run_disk_test(
                    "--self-test-transition", prior, percent
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.strip(), state)

    def test_block_and_incident_have_real_runner_actions(self):
        script = DISK_GUARD.read_text(encoding="utf-8")
        self.assertIn('stop_runner "$detected_runner"', script)
        self.assertIn('start_runner "$detected_runner"', script)
        self.assertIn('systemctl stop "${manifest[SYSTEMD_UNIT]}"', script)
        self.assertIn('"${compose[@]}" stop sub2api', script)
        self.assertIn("/run/sub2api-prod-quiesce.state", script)
        for action in ("stop", "start"):
            for runner in ("compose", "systemd"):
                result = self.run_disk_test("--self-test-action", action, runner)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_recovery_failure_is_nonzero_and_retains_quiesce(self):
        result = self.run_guard_test("--self-test-recovery-failure")
        self.assertNotEqual(result.returncode, 0)
        output = result.stdout + result.stderr
        self.assertIn("STATE=recovery_failed", output)
        self.assertIn("QUIESCED=1", output)
        self.assertIn("automatic recovery failed", output)
        self.assertNotIn("automatic recovery finished", output)

    def test_guard_is_fixed_fail_closed_entrypoint(self):
        script = GUARD.read_text(encoding="utf-8")
        for token in (
            "/run/lock/sub2api-prod-deploy.lock",
            "/run/sub2api-prod-quiesce.state",
            "--runner",
            "--commit",
            "--app-image",
            "--app-binary",
            "--app-sha256",
            "deploy_block_percent=70",
            "findmnt",
            "blkid",
            "pg_dump",
            "grok_video_request_owners",
            "grok_video_create_idempotency",
            "grok_image_create_idempotency",
            "redis-cli --no-auth-warning ping",
            "/health",
            "/v1/videos/generations",
            "/v1/videos/$request_id",
            "/v1/videos/$request_id/content",
            "automatic recovery started",
            "automatic recovery failed",
            "old app digest mismatch",
            "old app version mismatch",
        ):
            self.assertIn(token, script)
        self.assertNotIn('"$@"', script)
        self.assertNotIn("|| true", script)
        self.assertNotIn("automatic recovery finished", script)

    def test_automatic_recovery_only_rolls_back_app_and_stays_quiesced(self):
        script = GUARD.read_text(encoding="utf-8")
        compose_recovery = script.split("recover_compose() {", 1)[1].split(
            "recover_systemd() {", 1
        )[0]
        systemd_recovery = script.split("recover_systemd() {", 1)[1].split(
            "rollback() {", 1
        )[0]
        rollback = script.split("rollback() {", 1)[1].split("on_error() {", 1)[0]

        for body in (compose_recovery, systemd_recovery):
            for forbidden in ("pg_restore", "dropdb", "createdb"):
                self.assertNotIn(forbidden, body)
            self.assertNotIn("probe_health", body)

        self.assertIn('"${compose[@]}" stop sub2api', compose_recovery)
        self.assertIn("docker image inspect", compose_recovery)
        self.assertIn("create --no-build --no-deps --force-recreate sub2api", compose_recovery)
        self.assertIn("ps --status running -q sub2api", compose_recovery)
        self.assertIn("probe_compose_data_services", compose_recovery)
        self.assertNotIn("docker cp", compose_recovery)
        self.assertNotIn("up -d", compose_recovery)

        self.assertIn('systemctl stop "${manifest[SYSTEMD_UNIT]}"', systemd_recovery)
        self.assertIn("mv -Tf", systemd_recovery)
        self.assertIn("old_systemd_sha", systemd_recovery)
        self.assertIn("old_systemd_version", systemd_recovery)
        self.assertIn("probe_systemd_data_services", systemd_recovery)
        self.assertNotIn("systemctl start", systemd_recovery)
        self.assertNotIn("redis.rdb", systemd_recovery)

        self.assertIn('write_state recovery_failed "$requested_runner" 1', rollback)
        self.assertIn("compatibility/data-safety unproven", rollback)
        self.assertIn("application remains quiesced", rollback)
        self.assertNotIn("write_state normal", rollback)
        self.assertNotIn("recovery_verified", rollback)
        self.assertIn('pg_dump -U "$POSTGRES_USER"', script)
        self.assertIn('docker cp "$old_redis_id:/data/dump.rdb" "$backup_dir/redis.rdb"', script)
        self.assertNotIn("migration_started", script)
        self.assertNotIn("backup_ready", script)

    def test_compose_normal_deploy_command_record_is_app_only(self):
        script = GUARD.read_text(encoding="utf-8")
        app_deploy = script.split("deploy_compose_app() {", 1)[1].split(
            "probe_compose_data_services() {", 1
        )[0]
        preflight_call = (
            'compose_data_images_unchanged "$resolved_postgres_image" '
            '"$resolved_redis_image" "$old_postgres_image" "$old_redis_image"'
        )

        self.assertIn("config --format json", script)
        self.assertIn(".services.postgres.image", script)
        self.assertIn(".services.redis.image", script)
        self.assertIn(preflight_call, script)
        self.assertLess(script.rindex(preflight_call), script.index("write_state deploying"))
        self.assertIn('"${compose[@]}" pull --quiet sub2api', app_deploy)
        self.assertIn(
            '"${compose[@]}" up -d --no-deps --force-recreate sub2api',
            app_deploy,
        )
        self.assertIn("probe_compose_infrastructure", app_deploy)
        self.assertNotIn("pull --quiet postgres", app_deploy)
        self.assertNotIn("pull --quiet redis", app_deploy)
        self.assertNotIn("up -d postgres", app_deploy)
        self.assertNotIn("up -d redis", app_deploy)

    def test_mount_identity_manifest_is_external_non_secret_data(self):
        manifest = MANIFEST.read_text(encoding="utf-8")
        for key in (
            "EXPECTED_MOUNT_TARGET=",
            "EXPECTED_MOUNT_SOURCE=",
            "EXPECTED_MOUNT_UUID=",
            "SYSTEMD_RELEASE_ROOT=",
            "SYSTEMD_CURRENT_LINK=",
            "POSTGRES_SYSTEMD_UNIT=",
            "REDIS_SYSTEMD_UNIT=",
        ):
            self.assertIn(key, manifest)
        self.assertNotIn("PASSWORD", manifest)
        self.assertNotIn("TOKEN", manifest)
        self.assertNotIn("SECRET", manifest)

    def test_systemd_uses_atomic_immutable_release_link(self):
        unit = SYSTEMD_UNIT.read_text(encoding="utf-8")
        script = GUARD.read_text(encoding="utf-8")
        self.assertIn("ExecStart=/opt/sub2api/current", unit)
        self.assertIn("SYSTEMD_RELEASE_ROOT", script)
        self.assertIn("mv -Tf", script)
        self.assertIn("--version", script)
        self.assertIn("sha256sum", script)

    def test_compose_has_no_latest_and_enforces_rotation(self):
        compose = COMPOSE.read_text(encoding="utf-8")
        self.assertNotIn(":latest", compose)
        self.assertIn("name: sub2api-prod", compose)
        self.assertIn("SUB2API_IMAGE:?", compose)
        self.assertIn("POSTGRES_IMAGE:?", compose)
        self.assertIn("REDIS_IMAGE:?", compose)
        self.assertIn('max-size: "100m"', compose)
        self.assertIn('max-file: "10"', compose)
        self.assertIn('compress: "true"', compose)

    def test_disk_budgets_are_encoded(self):
        script = DISK_GUARD.read_text(encoding="utf-8")
        for token in (
            "percent >= 90",
            "percent >= 80",
            "percent >= 70",
            "percent < 75",
            "percent < 70",
            "os_runtime 20971520",
            "postgres_redis 26214400",
            "releases 8388608",
            "media_temp 20971520",
            "app_logs 5242880",
            "journald 4194304",
            "backups 8388608",
            "available_kb >= forced_free_kb",
            "-mtime +7",
            "roll_size",
            "roll_keep",
        ):
            self.assertIn(token, script)

    def test_runbook_has_no_credential_looking_assignment(self):
        runbook = RUNBOOK.read_text(encoding="utf-8")
        self.assertNotIn("SUB2API_SMOKE_API_KEY=", runbook)
        self.assertNotIn("export SUB2API_SMOKE_API_KEY", runbook)


if __name__ == "__main__":
    unittest.main()
