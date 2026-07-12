#!/usr/bin/env bash
# 方案 B：启动 Caddy，在 https://localhost:8443 反代后端，使 OIDC issuer 满足 https 校验。
#
# 用法：
#   ./start-caddy.sh
#
# 前置：
#   1. 已安装 caddy（https://caddyserver.com/docs/install）
#   2. 已在本目录签发证书：
#        mkcert -install
#        mkcert localhost 127.0.0.1 ::1   # 生成 localhost+2.pem / localhost+2-key.pem
#
# 启动后验证：
#   curl https://localhost:8443/.well-known/openid-configuration
#
# 停止：
#   ./stop-caddy.sh    （或 caddy stop）
set -euo pipefail

cd "$(dirname "$0")"

if ! command -v caddy >/dev/null 2>&1; then
	echo "[error] 未找到 caddy，请先安装：https://caddyserver.com/docs/install" >&2
	exit 1
fi

for f in localhost+2.pem localhost+2-key.pem; do
	if [[ ! -f "${f}" ]]; then
		echo "[error] 未找到证书 ${f}，请先在本目录执行：" >&2
		echo "          mkcert -install && mkcert localhost 127.0.0.1 ::1" >&2
		exit 1
	fi
done

# caddy start 会把进程 fork 到后台运行（由 Caddy 自带 admin API 管理），不占用终端。
echo "[info] 后台启动 Caddy：https://localhost:8443 -> 127.0.0.1:8080"
caddy start --config ./Caddyfile
echo "[info] 已在后台运行。验证：curl https://localhost:8443/.well-known/openid-configuration"
echo "[info] 停止：./stop-caddy.sh（或 caddy stop）"
