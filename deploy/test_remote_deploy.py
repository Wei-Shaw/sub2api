from __future__ import annotations

import tempfile
import unittest
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))

import remote_deploy
import remote_diagnose_plugin_embed


class LoadRemoteConfigTests(unittest.TestCase):
    def test_load_remote_config_defaults_to_compose_upstream_mode(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            env_path = Path(tmpdir) / ".remote"
            env_path.write_text(
                "\n".join(
                    [
                        "SSH_HOST=192.168.0.10",
                        "SSH_USER=root",
                        'SSH_PASSWORD="secret"',
                        "REMOTE_DIR=/opt/sub2api",
                        "COMPOSE_FILE=docker-compose.local.yml",
                    ]
                ),
                encoding="utf-8",
            )

            config = remote_deploy.load_remote_config(env_path)

            self.assertEqual(config.image_source, "compose")
            self.assertEqual(config.deploy_target, "all")
            self.assertEqual(config.local_image, "")
            self.assertEqual(config.local_image_tar, "")
            self.assertFalse(config.build_local_image)
            self.assertEqual(config.local_image_build_args, ())
            self.assertEqual(config.plugin_local_image, "")
            self.assertEqual(config.plugin_local_image_tar, "")
            self.assertFalse(config.build_plugin_local_image)
            self.assertEqual(config.plugin_local_image_build_args, ())

    def test_load_remote_config_reads_plugin_service_deploy_target(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            env_path = Path(tmpdir) / ".remote"
            env_path.write_text(
                "\n".join(
                    [
                        "SSH_HOST=192.168.0.10",
                        "SSH_USER=root",
                        'SSH_PASSWORD="secret"',
                        "REMOTE_DIR=/opt/sub2api",
                        "COMPOSE_FILE=docker-compose.local.yml",
                        "DEPLOY_TARGET=plugin_service",
                    ]
                ),
                encoding="utf-8",
            )

            config = remote_deploy.load_remote_config(env_path)

            self.assertEqual(config.deploy_target, "plugin_service")

    def test_load_remote_config_reads_local_image_mode_fields(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            env_path = Path(tmpdir) / ".remote"
            env_path.write_text(
                "\n".join(
                    [
                        "SSH_HOST=192.168.0.10",
                        "SSH_USER=root",
                        'SSH_PASSWORD="secret"',
                        "REMOTE_DIR=/opt/sub2api",
                        "COMPOSE_FILE=docker-compose.local.yml",
                        "IMAGE_SOURCE=local_image",
                        "LOCAL_IMAGE=weishaw/sub2api:latest",
                        "LOCAL_IMAGE_TAR=deploy/weishaw-sub2api-latest.tar",
                        "BUILD_LOCAL_IMAGE=true",
                        "LOCAL_IMAGE_BUILD_ARG_NODE_IMAGE=docker.m.daocloud.io/library/node:24-alpine",
                        "PLUGIN_LOCAL_IMAGE=weishaw/sub2api-plugin-service:latest",
                        "PLUGIN_LOCAL_IMAGE_TAR=deploy/weishaw-sub2api-plugin-service-latest.tar",
                    ]
                ),
                encoding="utf-8",
            )

            config = remote_deploy.load_remote_config(env_path)

            self.assertEqual(config.image_source, "local_image")
            self.assertEqual(config.local_image, "weishaw/sub2api:latest")
            self.assertEqual(config.local_image_tar, "deploy/weishaw-sub2api-latest.tar")
            self.assertTrue(config.build_local_image)
            self.assertEqual(
                config.local_image_build_args,
                ("NODE_IMAGE=docker.m.daocloud.io/library/node:24-alpine",),
            )
            self.assertEqual(config.plugin_local_image, "weishaw/sub2api-plugin-service:latest")
            self.assertEqual(config.plugin_local_image_tar, "deploy/weishaw-sub2api-plugin-service-latest.tar")

    def test_load_remote_config_rejects_local_image_mode_without_image_name(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            env_path = Path(tmpdir) / ".remote"
            env_path.write_text(
                "\n".join(
                    [
                        "SSH_HOST=192.168.0.10",
                        "SSH_USER=root",
                        'SSH_PASSWORD="secret"',
                        "REMOTE_DIR=/opt/sub2api",
                        "COMPOSE_FILE=docker-compose.local.yml",
                        "IMAGE_SOURCE=local_image",
                        "LOCAL_IMAGE_TAR=deploy/weishaw-sub2api-latest.tar",
                        "PLUGIN_LOCAL_IMAGE=weishaw/sub2api-plugin-service:latest",
                        "PLUGIN_LOCAL_IMAGE_TAR=deploy/weishaw-sub2api-plugin-service-latest.tar",
                    ]
                ),
                encoding="utf-8",
            )

            with self.assertRaisesRegex(ValueError, "LOCAL_IMAGE"):
                remote_deploy.load_remote_config(env_path)

    def test_load_remote_config_allows_missing_plugin_local_image_for_all_target(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            env_path = Path(tmpdir) / ".remote"
            env_path.write_text(
                "\n".join(
                    [
                        "SSH_HOST=192.168.0.10",
                        "SSH_USER=root",
                        'SSH_PASSWORD="secret"',
                        "REMOTE_DIR=/opt/sub2api",
                        "COMPOSE_FILE=docker-compose.local.yml",
                        "DEPLOY_TARGET=all",
                        "IMAGE_SOURCE=local_image",
                        "LOCAL_IMAGE=weishaw/sub2api:latest",
                        "LOCAL_IMAGE_TAR=deploy/weishaw-sub2api-latest.tar",
                    ]
                ),
                encoding="utf-8",
            )

            config = remote_deploy.load_remote_config(env_path)

            self.assertEqual(config.deploy_target, "all")
            self.assertEqual(config.plugin_local_image, "")
            self.assertEqual(config.plugin_local_image_tar, "")


class HelperTests(unittest.TestCase):
    def test_remote_example_uses_the_backend_go_toolchain_for_local_builds(self) -> None:
        project_root = Path(__file__).resolve().parent.parent
        backend_go_mod = (project_root / "backend" / "go.mod").read_text(encoding="utf-8")
        remote_example = (project_root / "deploy" / ".remote.example").read_text(encoding="utf-8")

        go_directive = next(
            line.split(maxsplit=1)[1]
            for line in backend_go_mod.splitlines()
            if line.startswith("go ")
        )

        self.assertIn(
            f"LOCAL_IMAGE_BUILD_ARG_GOLANG_IMAGE=docker.m.daocloud.io/library/golang:{go_directive}-alpine",
            remote_example,
        )

    def test_plugin_service_receives_minio_configuration_in_release_compose_files(self) -> None:
        project_root = Path(__file__).resolve().parent.parent
        expected_variables = (
            "MINIO_ENDPOINT=minio:9000",
            "MINIO_ACCESS_KEY=${MINIO_ROOT_USER:-sub2api-plugin}",
            "MINIO_SECRET_KEY=${MINIO_ROOT_PASSWORD:?MINIO_ROOT_PASSWORD is required}",
            "MINIO_BUCKET=${MINIO_BUCKET:-plugin-media}",
            "MINIO_USE_SSL=${MINIO_USE_SSL:-false}",
        )

        for compose_name in ("docker-compose.local.yml", "docker-compose.yml", "docker-compose.standalone.yml"):
            compose_content = (project_root / "deploy" / compose_name).read_text(encoding="utf-8")
            plugin_service = compose_content.split("  sub2api-plugin-server:\n", 1)[1].split("\n  minio:\n", 1)[0]

            for variable in expected_variables:
                self.assertIn(variable, plugin_service, f"{compose_name} must configure {variable} for sub2api-plugin-server")

            self.assertIn("container_name: sub2api-plugin-server", plugin_service)
            self.assertIn("aliases:", plugin_service)
            self.assertIn("- plugin-server", plugin_service)

    def test_should_upload_path_skips_runtime_data_directories(self) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            project_root = Path(tmpdir)
            deploy_dir = project_root / "deploy"
            postgres_file = deploy_dir / "postgres_data" / "PG_VERSION"
            minio_file = deploy_dir / "minio_data" / "buckets" / "plugin-media"
            runtime_file = deploy_dir / "data" / "config.yaml"
            regular_file = deploy_dir / "docker-compose.local.yml"

            postgres_file.parent.mkdir(parents=True)
            minio_file.parent.mkdir(parents=True)
            runtime_file.parent.mkdir(parents=True)
            regular_file.parent.mkdir(parents=True, exist_ok=True)
            postgres_file.write_text("18\n", encoding="utf-8")
            minio_file.write_text("demo\n", encoding="utf-8")
            runtime_file.write_text("demo\n", encoding="utf-8")
            regular_file.write_text("services:\n", encoding="utf-8")

            self.assertFalse(remote_deploy.should_upload_path(postgres_file, project_root))
            self.assertFalse(remote_deploy.should_upload_path(minio_file, project_root))
            self.assertFalse(remote_deploy.should_upload_path(runtime_file, project_root))
            self.assertTrue(remote_deploy.should_upload_path(regular_file, project_root))

    def test_remote_deploy_creates_minio_bind_mount_directory(self) -> None:
        source = (Path(remote_deploy.__file__).read_text(encoding="utf-8"))
        self.assertIn("mkdir -p data postgres_data redis_data plugin_data minio_data", source)

    def test_resolve_local_image_tar_path_uses_project_root_for_relative_paths(self) -> None:
        config = remote_deploy.RemoteDeployConfig(
            ssh_host="127.0.0.1",
            ssh_user="root",
            ssh_password="secret",
            ssh_key_path="",
            ssh_port=22,
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            deploy_target="all",
            image_source="local_image",
            local_image="weishaw/sub2api:latest",
            local_image_tar="deploy/weishaw-sub2api-latest.tar",
            build_local_image=False,
            local_image_build_args=(),
            plugin_local_image="weishaw/sub2api-plugin-service:latest",
            plugin_local_image_tar="deploy/weishaw-sub2api-plugin-service-latest.tar",
            build_plugin_local_image=False,
            plugin_local_image_build_args=(),
        )

        resolved = remote_deploy.resolve_local_image_tar_path(config, Path("/repo"))

        self.assertEqual(resolved, Path("/repo/deploy/weishaw-sub2api-latest.tar"))

    def test_build_compose_up_command_force_recreates_app_and_plugin_for_local_image_mode(self) -> None:
        command = remote_deploy.build_compose_up_command(
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            services=("sub2api", "sub2api-plugin-server"),
            force_recreate=True,
            no_deps=False,
        )

        self.assertIn("up -d --force-recreate sub2api sub2api-plugin-server", command)
        self.assertNotIn(" up -d\"", command)

    def test_build_compose_up_command_keeps_normal_up_for_compose_mode(self) -> None:
        command = remote_deploy.build_compose_up_command(
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            services=("sub2api", "sub2api-plugin-server"),
            force_recreate=False,
            no_deps=False,
        )

        self.assertIn("up -d sub2api sub2api-plugin-server", command)
        self.assertNotIn("--force-recreate", command)

    def test_build_compose_up_command_reuses_existing_data_services(self) -> None:
        command = remote_deploy.build_compose_up_command(
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            services=("sub2api", "sub2api-plugin-server"),
            force_recreate=True,
            no_deps=True,
        )

        self.assertIn("up -d --no-deps --force-recreate sub2api sub2api-plugin-server", command)

    def test_build_compose_up_command_deploys_only_plugin_service(self) -> None:
        command = remote_deploy.build_compose_up_command(
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            services=("sub2api-plugin-server",),
            force_recreate=True,
            no_deps=True,
        )

        self.assertIn("up -d --no-deps --force-recreate sub2api-plugin-server", command)
        self.assertNotIn(" sub2api ", command.split("up -d", 1)[1])

    def test_build_compose_pull_command_targets_plugin_service_only(self) -> None:
        command = remote_deploy.build_compose_pull_command(
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            services=("sub2api-plugin-server",),
        )

        self.assertIn("pull sub2api-plugin-server", command)

    def test_build_legacy_plugin_cleanup_command_removes_old_container_only(self) -> None:
        command = remote_deploy.build_legacy_plugin_cleanup_command()

        self.assertIn("docker container inspect plugin-service", command)
        self.assertIn("docker rm -f plugin-service", command)
        self.assertNotIn("sub2api-plugin-server", command)

    def test_get_deploy_plan_for_plugin_service_only(self) -> None:
        config = remote_deploy.RemoteDeployConfig(
            ssh_host="127.0.0.1",
            ssh_user="root",
            ssh_password="secret",
            ssh_key_path="",
            ssh_port=22,
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            deploy_target="plugin_service",
            image_source="compose",
            local_image="",
            local_image_tar="",
            build_local_image=False,
            local_image_build_args=(),
            plugin_local_image="",
            plugin_local_image_tar="",
            build_plugin_local_image=False,
            plugin_local_image_build_args=(),
        )

        plan = remote_deploy.get_deploy_plan(config, reuse_existing_data_services=True)

        self.assertEqual(plan.services, ("minio", "sub2api-plugin-server"))
        self.assertTrue(plan.force_recreate)
        self.assertFalse(plan.no_deps)
        self.assertEqual(plan.health_checks, (("minio", "sub2api-minio"), ("sub2api-plugin-server", "sub2api-plugin-server")))

    def test_get_deploy_plan_local_image_without_plugin_asset_recreates_only_sub2api(self) -> None:
        config = remote_deploy.RemoteDeployConfig(
            ssh_host="127.0.0.1",
            ssh_user="root",
            ssh_password="secret",
            ssh_key_path="",
            ssh_port=22,
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            deploy_target="all",
            image_source="local_image",
            local_image="weishaw/sub2api:latest",
            local_image_tar="deploy/weishaw-sub2api-latest.tar",
            build_local_image=False,
            local_image_build_args=(),
            plugin_local_image="",
            plugin_local_image_tar="",
            build_plugin_local_image=False,
            plugin_local_image_build_args=(),
        )

        assets = remote_deploy.get_image_assets(config, Path("/repo"))
        plan = remote_deploy.get_deploy_plan_with_assets(
            config,
            reuse_existing_data_services=True,
            local_image_services=tuple(asset.service_name for asset in assets),
        )

        self.assertEqual(plan.services, ("sub2api",))
        self.assertTrue(plan.force_recreate)
        self.assertTrue(plan.no_deps)
        self.assertEqual(plan.health_checks, (("sub2api", "sub2api"),))

    def test_get_image_assets_skips_plugin_when_plugin_local_image_not_configured(self) -> None:
        config = remote_deploy.RemoteDeployConfig(
            ssh_host="127.0.0.1",
            ssh_user="root",
            ssh_password="secret",
            ssh_key_path="",
            ssh_port=22,
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            deploy_target="all",
            image_source="local_image",
            local_image="weishaw/sub2api:latest",
            local_image_tar="deploy/weishaw-sub2api-latest.tar",
            build_local_image=False,
            local_image_build_args=(),
            plugin_local_image="",
            plugin_local_image_tar="",
            build_plugin_local_image=False,
            plugin_local_image_build_args=(),
        )

        assets = remote_deploy.get_image_assets(config, Path("/repo"))

        self.assertEqual(len(assets), 1)
        self.assertEqual(assets[0].service_name, "sub2api")

    def test_build_docker_build_command_includes_build_args(self) -> None:
        config = remote_deploy.RemoteDeployConfig(
            ssh_host="127.0.0.1",
            ssh_user="root",
            ssh_password="secret",
            ssh_key_path="",
            ssh_port=22,
            remote_dir="/opt/sub2api",
            compose_file="docker-compose.local.yml",
            deploy_target="all",
            image_source="local_image",
            local_image="weishaw/sub2api:latest",
            local_image_tar="deploy/weishaw-sub2api-latest.tar",
            build_local_image=True,
            local_image_build_args=("NODE_IMAGE=docker.m.daocloud.io/library/node:24-alpine",),
            plugin_local_image="weishaw/sub2api-plugin-service:latest",
            plugin_local_image_tar="deploy/weishaw-sub2api-plugin-service-latest.tar",
            build_plugin_local_image=False,
            plugin_local_image_build_args=(),
        )

        command = remote_deploy.build_docker_build_command(
            config.local_image,
            Path("/repo/Dockerfile"),
            config.local_image_build_args,
            Path("/repo"),
        )

        self.assertEqual(command[0:3], ["docker", "build", "-t"])
        self.assertIn("weishaw/sub2api:latest", command)
        self.assertIn("--build-arg", command)
        self.assertIn("NODE_IMAGE=docker.m.daocloud.io/library/node:24-alpine", command)
        self.assertEqual(Path(command[-1]), Path("/repo"))

    def test_remote_container_exists_returns_true_for_existing_container(self) -> None:
        class FakeClient:
            def exec_command(self, command: str, timeout: int = 120):
                self.command = command

                class FakeChannel:
                    def recv_exit_status(self) -> int:
                        return 0

                class FakeStream:
                    channel = FakeChannel()

                    def __init__(self, payload: bytes) -> None:
                        self.payload = payload

                    def read(self) -> bytes:
                        return self.payload

                return None, FakeStream(b"exists\n"), FakeStream(b"")

        client = FakeClient()

        self.assertTrue(remote_deploy.remote_container_exists(client, "sub2api-postgres"))
        self.assertIn("docker container inspect sub2api-postgres", client.command)

    def test_build_remote_header_probe_command_targets_plugin_service(self) -> None:
        command = remote_diagnose_plugin_embed.build_remote_header_probe_command(
            "http://127.0.0.1:8091/plugins/image-generation"
        )

        self.assertIn("curl -sSI", command)
        self.assertIn("http://127.0.0.1:8091/plugins/image-generation", command)

    def test_build_remote_file_probe_command_checks_expected_assets(self) -> None:
        command = remote_diagnose_plugin_embed.build_remote_file_probe_command()

        self.assertIn("/app/plugins/image-generation/web/index.html", command)
        self.assertIn("/app/plugins/image-generation/web/assets/app.js", command)

    def test_build_public_probe_url_joins_path(self) -> None:
        url = remote_diagnose_plugin_embed.build_public_probe_url(
            "https://demo.example.com/base/",
            "/plugins/image-generation",
        )

        self.assertEqual(url, "https://demo.example.com/base/plugins/image-generation")

    def test_get_default_probe_targets_includes_backend_and_plugin_paths(self) -> None:
        targets = remote_diagnose_plugin_embed.get_default_probe_targets()

        self.assertIn(("Plugin service direct headers", "http://127.0.0.1:8091/plugins/image-generation"), targets)
        self.assertIn(("Backend direct plugin headers", "http://127.0.0.1:8080/plugins/image-generation"), targets)
        self.assertIn(("Backend direct plugin API headers", "http://127.0.0.1:8080/plugins/image-generation/api/config"), targets)

    def test_get_reverse_proxy_probe_commands_covers_common_configs(self) -> None:
        commands = remote_diagnose_plugin_embed.get_reverse_proxy_probe_commands()

        joined = "\n".join(command for _, command in commands)
        self.assertIn("ss -lntp", joined)
        self.assertIn("/etc/caddy", joined)
        self.assertIn("/etc/nginx", joined)

if __name__ == "__main__":
    unittest.main()
