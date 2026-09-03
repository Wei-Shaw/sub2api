from __future__ import annotations

import io
import subprocess
import shlex
import sys
import time
import tarfile
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

PROJECT_ROOT = Path(__file__).resolve().parents[1]
DEPLOY_DIR = Path(__file__).resolve().parent
REMOTE_ENV_FILE = DEPLOY_DIR / ".remote"
DEPLOY_ENV_FILE = DEPLOY_DIR / ".env"
UPLOAD_EXCLUDED_PARTS = {
    ".git",
    ".idea",
    ".venv",
    "__pycache__",
    "node_modules",
    "data",
    "plugin_data",
    "minio_data",
    "postgres_data",
    "redis_data",
    ".DS_Store",
    "Thumbs.db",
}
UPLOAD_EXCLUDED_FILENAMES = {
    ".remote",
    ".remote.example",
}


@dataclass(frozen=True)
class RemoteDeployConfig:
    """
    远程部署配置。
    参数:
        ssh_host (str): 远程服务器 IP 或域名，不能为空。示例值: "192.168.1.10"
        ssh_user (str): SSH 登录用户名，不能为空。示例值: "root"
        ssh_password (str): SSH 密码，可为空但需与 ssh_key_path 二选一。示例值: "P@ssw0rd!"
        ssh_key_path (str): 本地私钥文件路径，可为空但需与 ssh_password 二选一。示例值: "C:/Users/demo/.ssh/id_rsa"
        ssh_port (int): SSH 端口，需为正整数。示例值: 22
        remote_dir (str): 远端部署目录，不能为空。示例值: "/opt/sub2api-deploy"
        compose_file (str): 远端使用的 Compose 文件名，默认本地目录版。示例值: "docker-compose.local.yml"

    返回:
        None: 该数据结构仅承载远程部署配置。
    异常:
        无: 数据结构本身不直接抛出业务异常。
    """

    ssh_host: str
    ssh_user: str
    ssh_password: str
    ssh_key_path: str
    ssh_port: int
    remote_dir: str
    compose_file: str
    deploy_target: str
    image_source: str
    local_image: str
    local_image_tar: str
    build_local_image: bool
    local_image_build_args: tuple[str, ...]
    plugin_local_image: str
    plugin_local_image_tar: str
    build_plugin_local_image: bool
    plugin_local_image_build_args: tuple[str, ...]


@dataclass(frozen=True)
class DeployPlan:
    services: tuple[str, ...]
    force_recreate: bool
    no_deps: bool
    health_checks: tuple[tuple[str, str], ...]


@dataclass(frozen=True)
class ImageAsset:
    service_name: str
    image_name: str
    tar_path: Path
    build_enabled: bool
    dockerfile_path: Path
    build_args: tuple[str, ...]


def parse_env_file(env_path: Path) -> dict[str, str]:
    """
    解析类 dotenv 配置文件。
    参数:
        env_path (Path): 待解析的配置文件路径，需指向文本文件。示例值: Path("deploy/.remote")

    返回:
        dict[str, str]: 解析得到的键值对，不包含注释和空行。
    异常:
        FileNotFoundError: 当 env_path 不存在时抛出。
        ValueError: 当出现缺少等号的非法配置行时抛出。
    """

    env_map: dict[str, str] = {}
    for line_no, raw_line in enumerate(env_path.read_text(encoding="utf-8").splitlines(), start=1):
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("export "):
            line = line[7:].strip()
        if "=" not in line:
            raise ValueError(f"Invalid env line at {env_path}:{line_no}: {raw_line}")
        key, value = line.split("=", 1)
        env_map[key.strip()] = _strip_optional_quotes(value.strip())
    return env_map


def _strip_optional_quotes(value: str) -> str:
    if len(value) >= 2 and value[0] == value[-1] and value[0] in {'"', "'"}:
        return value[1:-1]
    return value


def parse_bool(value: str) -> bool:
    return value.strip().lower() in {"1", "true", "yes", "y", "on"}


def is_valid_hex_secret(secret_value: str, expected_bytes: int) -> bool:
    """
    校验给定密钥是否为指定字节长度的十六进制字符串。
    参数:
        secret_value (str): 待校验的密钥文本，可为空字符串。示例值: "4f3c..."
        expected_bytes (int): 期望的原始字节长度，需为正整数。示例值: 32

    返回:
        bool: True 表示密钥合法，False 表示为空、非 hex 或长度不匹配。
    异常:
        无: 非法输入统一返回 False。
    """

    if not secret_value or expected_bytes <= 0:
        return False
    try:
        decoded = bytes.fromhex(secret_value)
    except ValueError:
        return False
    return len(decoded) == expected_bytes


def load_remote_config(env_path: Path = REMOTE_ENV_FILE) -> RemoteDeployConfig:
    """
    加载并校验远程部署配置。
    参数:
        env_path (Path): 远程部署配置文件路径，默认读取 deploy/.remote。示例值: Path("deploy/.remote")

    返回:
        RemoteDeployConfig: 校验通过后的远程部署配置对象。
    异常:
        FileNotFoundError: 当配置文件不存在时抛出。
        ValueError: 当关键字段缺失、端口非法或认证信息不完整时抛出。
    """

    env_map = parse_env_file(env_path)
    ssh_host = env_map.get("SSH_HOST", "").strip()
    ssh_user = env_map.get("SSH_USER", "root").strip()
    ssh_password = env_map.get("SSH_PASSWORD", "")
    ssh_key_path = env_map.get("SSH_KEY_PATH", "").strip()
    ssh_port_text = env_map.get("SSH_PORT", "22").strip()
    remote_dir = env_map.get("REMOTE_DIR", "/opt/sub2api-deploy").strip()
    compose_file = env_map.get("COMPOSE_FILE", "docker-compose.local.yml").strip()
    deploy_target = env_map.get("DEPLOY_TARGET", "all").strip() or "all"
    image_source = env_map.get("IMAGE_SOURCE", "compose").strip() or "compose"
    local_image = env_map.get("LOCAL_IMAGE", "").strip()
    local_image_tar = env_map.get("LOCAL_IMAGE_TAR", "").strip()
    build_local_image = parse_bool(env_map.get("BUILD_LOCAL_IMAGE", "false"))
    local_image_build_args = tuple(
        f"{key.removeprefix('LOCAL_IMAGE_BUILD_ARG_')}={value.strip()}"
        for key, value in sorted(env_map.items())
        if key.startswith("LOCAL_IMAGE_BUILD_ARG_") and value.strip()
    )
    plugin_local_image = env_map.get("PLUGIN_LOCAL_IMAGE", "").strip()
    plugin_local_image_tar = env_map.get("PLUGIN_LOCAL_IMAGE_TAR", "").strip()
    build_plugin_local_image = parse_bool(env_map.get("BUILD_PLUGIN_LOCAL_IMAGE", "false"))
    plugin_local_image_build_args = tuple(
        f"{key.removeprefix('PLUGIN_LOCAL_IMAGE_BUILD_ARG_')}={value.strip()}"
        for key, value in sorted(env_map.items())
        if key.startswith("PLUGIN_LOCAL_IMAGE_BUILD_ARG_") and value.strip()
    )

    if not ssh_host:
        raise ValueError("Missing required SSH_HOST in deploy/.remote")
    if not ssh_user:
        raise ValueError("Missing required SSH_USER in deploy/.remote")
    if not remote_dir:
        raise ValueError("Missing required REMOTE_DIR in deploy/.remote")
    if not compose_file:
        raise ValueError("Missing required COMPOSE_FILE in deploy/.remote")
    if deploy_target not in {"all", "plugin_service"}:
        raise ValueError(f"Invalid DEPLOY_TARGET: {deploy_target}")
    if image_source not in {"compose", "local_image"}:
        raise ValueError(f"Invalid IMAGE_SOURCE: {image_source}")
    if image_source == "local_image":
        if deploy_target == "all":
            if not local_image:
                raise ValueError("LOCAL_IMAGE must be configured when IMAGE_SOURCE=local_image")
            if not local_image_tar:
                raise ValueError("LOCAL_IMAGE_TAR must be configured when IMAGE_SOURCE=local_image")
        if deploy_target == "plugin_service":
            if not plugin_local_image:
                raise ValueError("PLUGIN_LOCAL_IMAGE must be configured when IMAGE_SOURCE=local_image")
            if not plugin_local_image_tar:
                raise ValueError("PLUGIN_LOCAL_IMAGE_TAR must be configured when IMAGE_SOURCE=local_image")

    try:
        ssh_port = int(ssh_port_text)
    except ValueError as exc:
        raise ValueError(f"Invalid SSH_PORT: {ssh_port_text}") from exc
    if ssh_port <= 0:
        raise ValueError(f"Invalid SSH_PORT: {ssh_port_text}")

    if not ssh_password and not ssh_key_path:
        raise ValueError("Either SSH_PASSWORD or SSH_KEY_PATH must be configured in deploy/.remote")

    return RemoteDeployConfig(
        ssh_host=ssh_host,
        ssh_user=ssh_user,
        ssh_password=ssh_password,
        ssh_key_path=ssh_key_path,
        ssh_port=ssh_port,
        remote_dir=remote_dir,
        compose_file=compose_file,
        deploy_target=deploy_target,
        image_source=image_source,
        local_image=local_image,
        local_image_tar=local_image_tar,
        build_local_image=build_local_image,
        local_image_build_args=local_image_build_args,
        plugin_local_image=plugin_local_image,
        plugin_local_image_tar=plugin_local_image_tar,
        build_plugin_local_image=build_plugin_local_image,
        plugin_local_image_build_args=plugin_local_image_build_args,
    )


def resolve_local_image_tar_path(config: RemoteDeployConfig, project_root: Path = PROJECT_ROOT) -> Path:
    image_tar_path = Path(config.local_image_tar).expanduser()
    if not image_tar_path.is_absolute():
        image_tar_path = project_root / image_tar_path
    return image_tar_path


def resolve_plugin_local_image_tar_path(config: RemoteDeployConfig, project_root: Path = PROJECT_ROOT) -> Path:
    image_tar_path = Path(config.plugin_local_image_tar).expanduser()
    if not image_tar_path.is_absolute():
        image_tar_path = project_root / image_tar_path
    return image_tar_path


def build_compose_up_command(
    remote_dir: str,
    compose_file: str,
    services: tuple[str, ...],
    force_recreate: bool,
    no_deps: bool,
) -> str:
    quoted_remote_dir = shlex.quote(remote_dir)
    quoted_compose = shlex.quote(compose_file)
    command_parts = [
        f"cd {quoted_remote_dir} && docker compose --env-file .env -f {quoted_compose} up -d"
    ]
    if no_deps:
        command_parts.append("--no-deps")
    if force_recreate:
        command_parts.append("--force-recreate")
    command_parts.extend(shlex.quote(service) for service in services)
    return " ".join(command_parts)


def build_compose_pull_command(
    remote_dir: str,
    compose_file: str,
    services: tuple[str, ...],
) -> str:
    quoted_remote_dir = shlex.quote(remote_dir)
    quoted_compose = shlex.quote(compose_file)
    return (
        f"cd {quoted_remote_dir} && docker compose --env-file .env -f {quoted_compose} pull "
        + " ".join(shlex.quote(service) for service in services)
    )


def build_legacy_plugin_cleanup_command() -> str:
    return (
        "if docker container inspect plugin-service >/dev/null 2>&1; "
        "then docker rm -f plugin-service; fi"
    )


def get_deploy_plan(config: RemoteDeployConfig, reuse_existing_data_services: bool) -> DeployPlan:
    return get_deploy_plan_with_assets(config, reuse_existing_data_services, ())


def get_deploy_plan_with_assets(
    config: RemoteDeployConfig,
    reuse_existing_data_services: bool,
    local_image_services: tuple[str, ...],
) -> DeployPlan:
    if config.deploy_target == "plugin_service":
        return DeployPlan(
            services=("minio", "sub2api-plugin-server"),
            force_recreate=True,
            no_deps=False,
            health_checks=(
                ("minio", "sub2api-minio"),
                ("sub2api-plugin-server", "sub2api-plugin-server"),
            ),
        )

    # MinIO is an explicit service dependency for image-generation assets.
    # Include it in every deployment plan so local-image deployments do not
    # leave the storage service stopped when --no-deps is used.
    services = ("minio", "sub2api", "sub2api-plugin-server")
    if config.image_source == "local_image":
        if local_image_services:
            app_services = tuple(service for service in services if service == "minio" or service in local_image_services)
            services = app_services
        else:
            services = ("minio", "sub2api")

    force_recreate = config.image_source == "local_image" or reuse_existing_data_services
    no_deps = force_recreate
    return DeployPlan(
        services=services,
        force_recreate=force_recreate,
        no_deps=no_deps,
        health_checks=tuple(
            (service_name, "sub2api-minio" if service_name == "minio" else service_name)
            for service_name in services
        ),
    )


def build_docker_build_command(
    image_name: str,
    dockerfile_path: Path,
    build_args: tuple[str, ...],
    project_root: Path = PROJECT_ROOT,
) -> list[str]:
    command = ["docker", "build", "-t", image_name, "-f", str(dockerfile_path)]
    for build_arg in build_args:
        command.extend(["--build-arg", build_arg])
    command.append(str(project_root))
    return command


def build_docker_save_command(image_name: str, image_tar_path: Path) -> list[str]:
    return ["docker", "save", "-o", str(image_tar_path), image_name]


def get_image_assets(config: RemoteDeployConfig, project_root: Path = PROJECT_ROOT) -> tuple[ImageAsset, ...]:
    assets: list[ImageAsset] = []
    if config.deploy_target == "all":
        assets.append(
            ImageAsset(
                service_name="sub2api",
                image_name=config.local_image,
                tar_path=resolve_local_image_tar_path(config, project_root),
                build_enabled=config.build_local_image,
                dockerfile_path=project_root / "Dockerfile",
                build_args=config.local_image_build_args,
            )
        )
    if config.plugin_local_image and config.plugin_local_image_tar:
        assets.append(
            ImageAsset(
                service_name="sub2api-plugin-server",
                image_name=config.plugin_local_image,
                tar_path=resolve_plugin_local_image_tar_path(config, project_root),
                build_enabled=config.build_plugin_local_image,
                dockerfile_path=project_root / "plugin-service" / "Dockerfile",
                build_args=config.plugin_local_image_build_args,
            )
        )
    return tuple(assets)


def build_local_source_image(asset: ImageAsset, project_root: Path = PROJECT_ROOT) -> None:
    print(f"Building local source image for {asset.service_name}: {asset.image_name}")
    subprocess.run(
        build_docker_build_command(asset.image_name, asset.dockerfile_path, asset.build_args, project_root),
        check=True,
    )
    print(f"Saving local source image for {asset.service_name}: {asset.tar_path}")
    subprocess.run(build_docker_save_command(asset.image_name, asset.tar_path), check=True)


def iter_upload_paths(project_root: Path = PROJECT_ROOT) -> Iterable[Path]:
    """
    枚举需要上传到远端的部署文件。
    参数:
        project_root (Path): 仓库根目录，用于扫描 deploy 子目录。示例值: Path("D:/repo/sub2api")

    返回:
        Iterable[Path]: 需要打包上传的本地文件路径集合。
    异常:
        无: 仅遍历本地文件并按规则过滤。
    """

    deploy_root = project_root / "deploy"
    for path in sorted(deploy_root.rglob("*")):
        if should_upload_path(path, project_root):
            yield path


def should_upload_path(path: Path, project_root: Path = PROJECT_ROOT) -> bool:
    """
    判断文件是否应参与远程部署上传。
    参数:
        path (Path): 待判断的本地文件路径。示例值: Path("deploy/docker-compose.local.yml")
        project_root (Path): 仓库根目录，用于计算相对路径。示例值: Path("D:/repo/sub2api")

    返回:
        bool: True 表示需要上传，False 表示应过滤。
    异常:
        无: 仅进行本地路径规则判断。
    """

    if not path.is_file():
        return False
    rel = path.relative_to(project_root)
    if rel.parts[0] != "deploy":
        return False
    if any(part in UPLOAD_EXCLUDED_PARTS for part in rel.parts):
        return False
    if rel.name in UPLOAD_EXCLUDED_FILENAMES:
        return False
    return True


def build_deploy_archive(project_root: Path = PROJECT_ROOT) -> io.BytesIO:
    """
    构建用于上传的部署压缩包。
    参数:
        project_root (Path): 仓库根目录，用于收集 deploy 目录内容。示例值: Path("D:/repo/sub2api")

    返回:
        io.BytesIO: 内存中的 tar.gz 压缩包，可直接上传到远端。
    异常:
        无: 打包失败时会由底层文件系统异常直接抛出。
    """

    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as tar:
        for path in iter_upload_paths(project_root):
            rel = path.relative_to(project_root / "deploy")
            tar.add(path, arcname=str(rel))
    buffer.seek(0)
    return buffer


def ensure_runtime_env_exists(env_path: Path = DEPLOY_ENV_FILE) -> None:
    """
    校验部署运行时环境文件存在。
    参数:
        env_path (Path): 需要上传到远端的部署环境文件路径。示例值: Path("deploy/.env")

    返回:
        None: 仅在缺失时抛出异常。
    异常:
        FileNotFoundError: 当部署目录下缺少 .env 文件时抛出。
    """

    if not env_path.is_file():
        raise FileNotFoundError(
            f"Deployment env file not found: {env_path}. Copy deploy/.env.example to deploy/.env first."
        )


def connect_ssh(config: RemoteDeployConfig) -> object:
    """
    建立 SSH 连接。
    参数:
        config (RemoteDeployConfig): 远程部署配置，包含主机、端口和认证方式。示例值: RemoteDeployConfig(...)

    返回:
        Any: 已连接的 paramiko SSHClient 对象。
    异常:
        RuntimeError: 当本地未安装 paramiko 时抛出。
        paramiko.SSHException: 当连接失败时由底层库抛出。
    """

    try:
        import paramiko
    except ImportError as exc:
        raise RuntimeError(
            "Missing dependency 'paramiko'. Please run: pip install paramiko"
        ) from exc

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())

    connect_kwargs: dict[str, object] = {
        "hostname": config.ssh_host,
        "port": config.ssh_port,
        "username": config.ssh_user,
    }
    if config.ssh_key_path:
        key_path = Path(config.ssh_key_path).expanduser()
        if not key_path.is_file():
            raise FileNotFoundError(f"SSH key file not found: {key_path}")
        connect_kwargs["key_filename"] = str(key_path)
    else:
        connect_kwargs["password"] = config.ssh_password

    client.connect(**connect_kwargs)
    return client


def exec_remote_command(client: object, command: str, timeout: int = 120) -> str:
    """
    执行远端命令并在失败时抛出异常。
    参数:
        client (Any): 已建立的 SSH 客户端。示例值: paramiko.SSHClient()
        command (str): 需要在远端执行的 shell 命令。示例值: "docker compose ps"
        timeout (int): 命令超时时间，单位秒。示例值: 120

    返回:
        str: 远端标准输出文本，已去掉首尾空白。
    异常:
        RuntimeError: 当远端命令退出码非 0 时抛出，并附带标准错误输出。
    """

    _, stdout, stderr = client.exec_command(command, timeout=timeout)
    exit_code = stdout.channel.recv_exit_status()
    out = stdout.read().decode("utf-8", errors="replace").strip()
    err = stderr.read().decode("utf-8", errors="replace").strip()
    if exit_code != 0:
        raise RuntimeError(f"Remote command failed (exit={exit_code}): {command}\n{err}")
    return out


def try_exec_remote_command(client: object, command: str, timeout: int = 120) -> str:
    """
    尝试执行远端命令，并在失败时返回空字符串而不是抛出异常。
    参数:
        client (object): 已建立的 SSH 客户端对象。示例值: paramiko.SSHClient()
        command (str): 需要执行的远端 shell 命令。示例值: "ss -lntp | grep 8080"
        timeout (int): 命令超时时间，单位秒。示例值: 120

    返回:
        str: 命令成功时返回标准输出；失败时返回空字符串。
    异常:
        无: 该函数会吞掉执行异常，便于诊断阶段继续收集其他信息。
    """

    try:
        return exec_remote_command(client, command, timeout=timeout)
    except Exception:
        return ""


def wait_for_container_health(
    client: object,
    container_name: str,
    timeout_seconds: int = 180,
    poll_interval_seconds: int = 3,
) -> str:
    """
    等待容器进入稳定状态，并返回最终健康状态。
    参数:
        client (object): 已建立的 SSH 客户端对象。示例值: paramiko.SSHClient()
        container_name (str): 需要等待的容器名称，不能为空。示例值: "sub2api"
        timeout_seconds (int): 最长等待时长，单位秒。示例值: 180
        poll_interval_seconds (int): 轮询间隔，单位秒。示例值: 3

    返回:
        str: 容器最终状态，可能为 healthy、running、starting、unhealthy、exited 或 unknown。
    异常:
        无: 超时或查询失败时返回最后一次观察到的状态，交由上层统一诊断。
    """

    inspect_command = (
        "docker inspect --format "
        "\"{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}\" "
        f"{shlex.quote(container_name)}"
    )
    deadline = time.time() + timeout_seconds
    last_status = "unknown"

    while time.time() < deadline:
        status = try_exec_remote_command(client, inspect_command, timeout=30).strip()
        if status:
            last_status = status
        if status in {"healthy", "running"}:
            return status
        if status in {"unhealthy", "exited", "dead"}:
            return status
        time.sleep(poll_interval_seconds)

    return last_status


def remote_container_exists(client: object, container_name: str) -> bool:
    quoted_name = shlex.quote(container_name)
    command = (
        "sh -lc "
        f"\"docker container inspect {quoted_name} >/dev/null 2>&1 && echo exists || true\""
    )
    return try_exec_remote_command(client, command, timeout=30).strip() == "exists"


def print_remote_runtime_diagnostics(client: object, remote_dir: str, compose_file: str) -> None:
    """
    输出远端运行时诊断信息，帮助定位端口不可访问问题。
    参数:
        client (object): 已建立的 SSH 客户端对象。示例值: paramiko.SSHClient()
        remote_dir (str): 远端部署目录，需为有效 shell 路径。示例值: "/opt/sub2api"
        compose_file (str): 远端 Compose 文件名。示例值: "docker-compose.local.yml"

    返回:
        None: 该函数直接打印诊断信息到标准输出。
    异常:
        无: 各项诊断命令失败时会尽量继续输出其余信息。
    """

    quoted_remote_dir = shlex.quote(remote_dir)
    quoted_compose = shlex.quote(compose_file)

    status = try_exec_remote_command(
        client,
        f"cd {quoted_remote_dir} && docker compose --env-file .env -f {quoted_compose} ps",
        timeout=60,
    )
    if status:
        print(status)

    port_binding = try_exec_remote_command(
        client,
        "docker port sub2api 8080",
        timeout=30,
    )
    if port_binding:
        print("\nDocker port mapping:\n")
        print(port_binding)

    plugin_port_binding = try_exec_remote_command(
        client,
        "docker port sub2api-plugin-server 8091",
        timeout=30,
    )
    if plugin_port_binding:
        print("\nPlugin service port mapping:\n")
        print(plugin_port_binding)

    listeners = try_exec_remote_command(
        client,
        "sh -lc 'ss -lntp 2>/dev/null | grep 8080 || netstat -lntp 2>/dev/null | grep 8080 || true'",
        timeout=30,
    )
    if listeners:
        print("\nPort 8080 listeners:\n")
        print(listeners)

    logs = try_exec_remote_command(
        client,
        f"cd {quoted_remote_dir} && docker compose --env-file .env -f {quoted_compose} logs --tail=80",
        timeout=60,
    )
    if logs:
        print("\nRecent logs:\n")
        print(logs)


def deploy_to_remote(config: RemoteDeployConfig) -> None:
    """
    执行一键远程部署流程。
    参数:
        config (RemoteDeployConfig): 远程部署配置，决定 SSH 连接和远端部署目录。示例值: RemoteDeployConfig(...)

    返回:
        None: 部署成功后输出远端状态和日志摘要。
    异常:
        FileNotFoundError: 当本地缺少 deploy/.env 或 SSH 私钥文件时抛出。
        RuntimeError: 当依赖缺失或远端命令执行失败时抛出。
    """

    ensure_runtime_env_exists()
    image_assets: tuple[ImageAsset, ...] = ()
    if config.image_source == "local_image":
        image_assets = get_image_assets(config)
        for asset in image_assets:
            if asset.build_enabled:
                build_local_source_image(asset)
            if not asset.tar_path.is_file():
                raise FileNotFoundError(
                    f"Local image tar not found for {asset.service_name}: {asset.tar_path}. "
                    f"Build and save {asset.image_name} first."
                )

    archive = build_deploy_archive()
    client = connect_ssh(config)
    sftp = client.open_sftp()
    quoted_remote_dir = shlex.quote(config.remote_dir)

    try:
        plan = get_deploy_plan_with_assets(config, False, ())
        total_steps = 6 + (len(image_assets) * 2 if config.image_source == "local_image" else 0)
        print(f"[1/{total_steps}] Ensuring remote directory: {config.remote_dir}")
        exec_remote_command(client, f"mkdir -p {quoted_remote_dir}")

        print(f"[2/{total_steps}] Uploading deploy package...")
        remote_tar_path = f"{config.remote_dir.rstrip('/')}/deploy.tar.gz"
        sftp.putfo(archive, remote_tar_path)

        print(f"[3/{total_steps}] Uploading deploy/.env ...")
        remote_env_path = f"{config.remote_dir.rstrip('/')}/.env"
        sftp.put(str(DEPLOY_ENV_FILE), remote_env_path)

        print(f"[4/{total_steps}] Extracting files on remote...")
        exec_remote_command(
            client,
            f"cd {quoted_remote_dir} && tar xzf deploy.tar.gz && rm -f deploy.tar.gz",
            timeout=300,
        )

        print(f"[5/{total_steps}] Creating data directories...")
        exec_remote_command(
            client,
            f"cd {quoted_remote_dir} && mkdir -p data postgres_data redis_data plugin_data minio_data",
            timeout=60,
        )

        reuse_existing_data_services = remote_container_exists(client, "sub2api-postgres") or remote_container_exists(
            client, "sub2api-redis"
        )
        plan = get_deploy_plan_with_assets(
            config,
            reuse_existing_data_services,
            tuple(asset.service_name for asset in image_assets),
        )
        if "sub2api-plugin-server" in plan.services:
            exec_remote_command(client, build_legacy_plugin_cleanup_command(), timeout=60)

        if reuse_existing_data_services and config.deploy_target != "plugin_service":
            print("Detected existing postgres/redis containers, will recreate app services only.")

        current_step = 6
        if config.image_source == "local_image":
            for asset in image_assets:
                remote_image_tar_path = f"{config.remote_dir.rstrip('/')}/{asset.tar_path.name}"
                print(f"[{current_step}/{total_steps}] Uploading local image tar for {asset.service_name}: {asset.tar_path.name}")
                sftp.put(str(asset.tar_path), remote_image_tar_path)
                current_step += 1

                print(f"[{current_step}/{total_steps}] Loading local image on remote for {asset.service_name}: {asset.image_name}")
                load_output = exec_remote_command(
                    client,
                    f"cd {quoted_remote_dir} && docker load -i {shlex.quote(asset.tar_path.name)}",
                    timeout=1800,
                )
                if load_output:
                    print(load_output)
                current_step += 1

            print(f"[{current_step}/{total_steps}] Starting Docker Compose with local image...")
        else:
            print(f"[{current_step}/{total_steps}] Starting Docker Compose...")
            if config.deploy_target == "plugin_service":
                pull_output = exec_remote_command(
                    client,
                    build_compose_pull_command(config.remote_dir, config.compose_file, plan.services),
                    timeout=1200,
                )
                if pull_output:
                    print(pull_output)

        exec_remote_command(
            client,
            build_compose_up_command(
                config.remote_dir,
                config.compose_file,
                plan.services,
                plan.force_recreate,
                plan.no_deps,
            ),
            timeout=1200,
        )

        print("")
        for service_name, container_name in plan.health_checks:
            print(f"Waiting for {service_name} container health...")
            final_status = wait_for_container_health(client, container_name)
            print(f"{service_name} health status: {final_status}\n")
        print_remote_runtime_diagnostics(client, config.remote_dir, config.compose_file)
    finally:
        sftp.close()
        client.close()


def main() -> int:
    """
    远程部署脚本入口。
    参数:
        无: 入口函数直接读取 deploy/.remote 与 deploy/.env。

    返回:
        int: 0 表示成功，1 表示失败。
    异常:
        无: 所有异常都会被捕获并转为退出码。
    """

    try:
        config = load_remote_config()
        deploy_to_remote(config)
        return 0
    except Exception as exc:  # pragma: no cover - CLI fallback
        print(f"Remote deploy failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
