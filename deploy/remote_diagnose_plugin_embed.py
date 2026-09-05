from __future__ import annotations

import shlex
import sys
from typing import Iterable

import remote_deploy


def build_remote_header_probe_command(target_url: str) -> str:
    """
    构建远端响应头探测命令。
    参数:
        target_url (str): 需要在远端通过 curl 探测的完整 URL，必须包含协议头。示例值: "http://127.0.0.1:8091/plugins/image-generation"

    返回:
        str: 可直接通过 SSH 执行的 shell 命令，成功时输出响应头。
    异常:
        无: 该函数仅拼装命令字符串，不直接校验远端环境。
    """

    quoted_url = shlex.quote(target_url)
    return f"sh -lc \"curl -sSI {quoted_url} || wget -S --spider -O /dev/null {quoted_url} 2>&1 || true\""


def build_remote_file_probe_command() -> str:
    """
    构建远端插件前端文件探测命令。
    参数:
        无

    返回:
        str: 用于检查插件容器内关键前端文件是否存在的 shell 命令。
    异常:
        无: 该函数只返回命令文本。
    """

    expected_paths = [
        "/app/plugins/image-generation/web/index.html",
        "/app/plugins/image-generation/web/assets/app.js",
        "/app/plugins/image-generation/web/assets/app.css",
    ]
    checks = " ".join(shlex.quote(path) for path in expected_paths)
    return (
        "sh -lc "
        f"\"for path in {checks}; do "
        "if [ -f \\\"\\$path\\\" ]; then echo \\\"FOUND \\$path\\\"; "
        "else echo \\\"MISSING \\$path\\\"; fi; "
        "done\""
    )


def build_public_probe_url(base_url: str, path: str) -> str:
    """
    拼接公网探测 URL。
    参数:
        base_url (str): 公网基础地址，允许带路径前缀。示例值: "https://demo.example.com/base/"
        path (str): 需要拼接的站内路径，建议以斜杠开头。示例值: "/plugins/image-generation"

    返回:
        str: 规范化后的完整公网 URL。
    异常:
        无: 输入为空时返回简单拼接结果，由调用方自行决定是否使用。
    """

    normalized_base = base_url.rstrip("/")
    normalized_path = "/" + path.lstrip("/")
    return normalized_base + normalized_path


def get_default_probe_targets() -> list[tuple[str, str]]:
    """
    返回默认需要探测的远端 URL 列表。
    参数:
        无

    返回:
        list[tuple[str, str]]: 诊断标题与目标 URL 的有序列表，覆盖插件服务直连和主站直连路径。
    异常:
        无: 该函数仅返回静态配置。
    """

    return [
        ("Plugin service direct headers", "http://127.0.0.1:8091/plugins/image-generation"),
        ("Backend direct plugin headers", "http://127.0.0.1:8080/plugins/image-generation"),
        ("Backend direct plugin API headers", "http://127.0.0.1:8080/plugins/image-generation/api/config"),
    ]


def get_reverse_proxy_probe_commands() -> list[tuple[str, str]]:
    """
    返回远端反向代理探测命令列表。
    参数:
        无

    返回:
        list[tuple[str, str]]: 诊断标题与 shell 命令的有序列表，覆盖监听端口和常见 Caddy/Nginx 配置位置。
    异常:
        无: 该函数仅返回静态命令集合。
    """

    return [
        (
            "Reverse proxy listeners",
            "sh -lc 'ss -lntp 2>/dev/null | grep -E \":(80|443|8080|8091) \" || netstat -lntp 2>/dev/null | grep -E \":(80|443|8080|8091) \" || true'",
        ),
        (
            "Caddy config candidates",
            "sh -lc 'for path in /etc/caddy/Caddyfile /opt/sub2api/Caddyfile; do if [ -f \"$path\" ]; then echo \"--- $path ---\"; sed -n \"1,220p\" \"$path\"; fi; done'",
        ),
        (
            "Nginx config candidates",
            "sh -lc 'for path in /etc/nginx/nginx.conf /etc/nginx/conf.d/default.conf /etc/nginx/conf.d/sub2api.conf; do if [ -f \"$path\" ]; then echo \"--- $path ---\"; sed -n \"1,220p\" \"$path\"; fi; done'",
        ),
        (
            "Docker containers",
            "docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'",
        ),
    ]


def print_section(title: str) -> None:
    """
    打印诊断分节标题。
    参数:
        title (str): 当前诊断阶段标题，建议为短语。示例值: "Plugin response headers"

    返回:
        None: 该函数仅向标准输出打印文本。
    异常:
        无: 标题为空时也会照常输出分隔线。
    """

    print(f"\n=== {title} ===")


def print_command_output(lines: Iterable[str]) -> None:
    """
    打印命令输出内容。
    参数:
        lines (Iterable[str]): 需要逐行打印的文本集合，可以是 splitlines() 结果。示例值: ["HTTP/1.1 200 OK", "Content-Type: text/html"]

    返回:
        None: 该函数仅向标准输出打印文本。
    异常:
        无: 遇到空集合时不会抛错。
    """

    has_output = False
    for line in lines:
        has_output = True
        print(line)
    if not has_output:
        print("(no output)")


def diagnose_remote_embed(public_base_url: str = "") -> None:
    """
    通过现有远端 SSH 配置诊断插件 iframe 嵌入链路。
    参数:
        public_base_url (str): 可选的公网基础地址，用于额外检查经过反向代理后的响应头。示例值: "https://demo.example.com"

    返回:
        None: 诊断结果会直接打印到标准输出。
    异常:
        RuntimeError: 当 SSH 连接失败或关键远端命令执行失败时抛出。
    """

    config = remote_deploy.load_remote_config()
    client = remote_deploy.connect_ssh(config)

    try:
        print_section("Remote docker status")
        status = remote_deploy.try_exec_remote_command(
            client,
            f"cd {shlex.quote(config.remote_dir)} && docker compose --env-file .env -f {shlex.quote(config.compose_file)} ps",
            timeout=60,
        )
        print_command_output(status.splitlines())

        print_section("Plugin container image")
        image_name = remote_deploy.try_exec_remote_command(
            client,
            "docker inspect --format '{{.Config.Image}}' sub2api-plugin-server",
            timeout=30,
        )
        print_command_output(image_name.splitlines())

        print_section("Plugin frontend files")
        file_status = remote_deploy.try_exec_remote_command(
            client,
            f"docker exec sub2api-plugin-server {build_remote_file_probe_command()}",
            timeout=30,
        )
        print_command_output(file_status.splitlines())

        for title, target_url in get_default_probe_targets():
            print_section(title)
            headers = remote_deploy.try_exec_remote_command(
                client,
                build_remote_header_probe_command(target_url),
                timeout=30,
            )
            print_command_output(headers.splitlines())

        for title, command in get_reverse_proxy_probe_commands():
            print_section(title)
            output = remote_deploy.try_exec_remote_command(
                client,
                command,
                timeout=30,
            )
            print_command_output(output.splitlines())

        if public_base_url.strip():
            print_section("Public plugin headers")
            public_plugin_headers = remote_deploy.try_exec_remote_command(
                client,
                build_remote_header_probe_command(
                    build_public_probe_url(public_base_url, "/plugins/image-generation")
                ),
                timeout=30,
            )
            print_command_output(public_plugin_headers.splitlines())

    finally:
        client.close()


def main() -> int:
    """
    执行命令行入口。
    参数:
        无

    返回:
        int: 0 表示诊断命令执行完成，1 表示出现异常。
    异常:
        无: 所有异常都会被捕获并转为退出码。
    """

    try:
        public_base_url = sys.argv[1].strip() if len(sys.argv) > 1 else ""
        diagnose_remote_embed(public_base_url)
        return 0
    except Exception as exc:  # pragma: no cover - CLI fallback
        print(f"Plugin embed diagnosis failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
