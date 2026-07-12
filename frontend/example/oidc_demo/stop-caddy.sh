#!/usr/bin/env bash
# 方案 B：停止后台运行的 Caddy。
#
# 用法：
#   ./stop-caddy.sh
set -euo pipefail

cd "$(dirname "$0")"

if ! command -v caddy >/dev/null 2>&1; then
	echo "[error] 未找到 caddy" >&2
	exit 1
fi

echo "[info] 停止 Caddy"
caddy stop
