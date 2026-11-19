#!/bin/sh
# 在 Linux 服务器上执行：sh deploy/custom/deploy.sh [check|up|status|logs]
# 本地构建镜像时设置 PULL_IMAGES=false，避免从 GHCR 拉取同名镜像。
set -eu

task_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
action=${1:-up}
if [ "$#" -gt 1 ]; then
    printf '%s\n' '用法：sh deploy.sh [check|up|status|logs]' >&2
    exit 2
fi
case "$action" in
    check|up|status|logs) ;;
    *) printf '%s\n' '用法：sh deploy.sh [check|up|status|logs]' >&2; exit 2 ;;
esac

if [ ! -f "$task_dir/.env" ]; then
    printf '%s\n' '请先复制 deploy/custom/.env.example 为 .env，并填写镜像版本和凭据。' >&2
    exit 1
fi
command -v docker >/dev/null 2>&1 || {
    printf '%s\n' '未找到 docker，请先安装 Docker Engine 和 Compose 插件。' >&2
    exit 1
}
docker compose version >/dev/null

# 不执行 source .env，避免把配置值作为 Shell 代码运行。
compose() {
    docker compose --project-directory "$task_dir" --env-file "$task_dir/.env" \
        -f "$task_dir/compose.yml" "$@"
}

pull_images=${PULL_IMAGES:-true}
case "$pull_images" in
    true|false) ;;
    *) printf '%s\n' 'PULL_IMAGES 只能设置为 true 或 false。' >&2; exit 2 ;;
esac

# --quiet 避免把解析后的密码和密钥打印到终端。
compose config --quiet
case "$action" in
    check) printf '%s\n' 'Compose 配置检查通过。' ;;
    up)
        if [ "$pull_images" = true ]; then
            compose pull
        fi
        compose up -d --wait --wait-timeout 300
        compose ps
        ;;
    status) compose ps ;;
    logs) compose logs --tail=100 -f sub2api ;;
esac
