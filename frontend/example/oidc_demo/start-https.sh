#!/usr/bin/env bash
# 方案 B：以信任 mkcert 私有 CA 的方式启动 demo。
#
# demo 用 Node 全局 fetch 访问 issuer。Node 默认不认 mkcert 的私有根证书，
# 直接 npm start 会报 "fetch failed" / "self-signed certificate"。
# 这里通过 NODE_EXTRA_CA_CERTS 注入 mkcert 根证书后再启动。
#
# 用法：
#   ./start-https.sh
#
# 前置：已执行 mkcert -install，且 .env 里 SUB2API_ISSUER_URL=https://localhost:8443
set -euo pipefail

cd "$(dirname "$0")"

if ! command -v mkcert >/dev/null 2>&1; then
	echo "[error] 未找到 mkcert，请先安装并执行 mkcert -install" >&2
	exit 1
fi

CAROOT="$(mkcert -CAROOT)"
ROOT_CA="${CAROOT}/rootCA.pem"

if [[ ! -f "${ROOT_CA}" ]]; then
	echo "[error] 未找到 mkcert 根证书：${ROOT_CA}，请先执行 mkcert -install" >&2
	exit 1
fi

echo "[info] NODE_EXTRA_CA_CERTS=${ROOT_CA}"
NODE_EXTRA_CA_CERTS="${ROOT_CA}" npm start
