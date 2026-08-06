#!/usr/bin/env bash
# =============================================================================
# sub2api -> ccdirect 生产部署脚本（薄镜像方案）
# =============================================================================
# 背景：目标服务器内存仅 1GB，不能在其上跑前端/Go 重型构建（曾踩 OOM）。
# 方案：本机交叉编译 linux/amd64 二进制（前端 embed 进二进制），服务器只做
#       COPY-only 薄镜像构建 + 容器替换，内存压力可忽略。
#
# 流程：
#   1. [本机] pnpm build 前端（dist 已存在时可用 --skip-frontend 跳过）
#   2. [本机] GOOS=linux GOARCH=amd64 交叉编译（-tags embed + 版本 ldflags）
#   3. [本机] 打包二进制 + resources + docker-entrypoint.sh，tar 管道传服务器
#   4. [服务器] 薄 Dockerfile：COPY 二进制/resources + 从"当前运行镜像"继承
#      /app/data（config.yaml / logs / pages 等运行时数据）+ postgres 备份工具
#   5. [服务器] docker run 替换容器（env-file / 网络 / 端口 / healthcheck 全复用）
#   6. 验证：容器 healthy + /health 200 + 公网入口 200
#
# 回滚：替换前脚本会打印旧镜像 tag；失败时 `docker run` 回滚旧镜像即可。
#
# 用法：
#   bash deploy/deploy.sh                 # 全流程（含前端构建）
#   bash deploy/deploy.sh --skip-frontend # dist 已是最新时跳过前端构建
#   bash deploy/deploy.sh --tag <sha>     # 指定镜像 tag（默认当前 HEAD 短 sha）
#   bash deploy/deploy.sh --host <别名>   # SSH 目标（默认 ccdirect）
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BACKEND_DIR="${REPO_ROOT}/backend"
FRONTEND_DIR="${REPO_ROOT}/frontend"

HOST="ccdirect"
TAG="$(git -C "${REPO_ROOT}" rev-parse --short HEAD)"
SKIP_FRONTEND=false
SERVER_STAGE="/tmp/s2a-deploy"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-frontend) SKIP_FRONTEND=true; shift ;;
        --tag) TAG="$2"; shift 2 ;;
        --host) HOST="$2"; shift 2 ;;
        *) echo "usage: $0 [--skip-frontend] [--tag <sha>] [--host <ssh-alias>]" >&2; exit 2 ;;
    esac
done

if [[ ! "${TAG}" =~ ^[0-9a-f]{7,40}$ ]]; then
    echo "error: 非法 tag '${TAG}'，应为 git 短 sha" >&2
    exit 2
fi

echo "==> 部署目标: ${HOST}  镜像 tag: ccdirect:${TAG}"

# 1. 前端构建（可选）
if [[ "${SKIP_FRONTEND}" == "false" ]]; then
    echo "==> [1/6] 构建前端 (${FRONTEND_DIR})"
    (cd "${FRONTEND_DIR}" && pnpm build)
else
    echo "==> [1/6] 跳过前端构建（--skip-frontend），沿用现有 dist"
fi

# 2. 交叉编译后端（embed 前端 + 版本信息）
echo "==> [2/6] 交叉编译 linux/amd64（-tags embed）"
BIN="${REPO_ROOT}/.cache/deploy/sub2api-linux-amd64"
mkdir -p "$(dirname "${BIN}")"
(cd "${BACKEND_DIR}" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
    -tags embed \
    -ldflags "-s -w \
        -X main.Version=$(cat "${BACKEND_DIR}/cmd/server/VERSION") \
        -X main.Commit=${TAG} \
        -X main.Date=$(date +"%Y-%m-%dT%H:%M:%S%z") \
        -X main.BuildType=release" \
    -o "${BIN}" ./cmd/server)
echo "    二进制: $(ls -lh "${BIN}" | awk '{print $5}')"

# 3. 打包并传服务器
echo "==> [3/6] 打包并传输到 ${HOST}"
tar czf - -C "${REPO_ROOT}/.cache/deploy" sub2api-linux-amd64 \
    -C "${REPO_ROOT}" deploy/docker-entrypoint.sh \
    -C "${BACKEND_DIR}" resources | \
    ssh "${HOST}" "rm -rf ${SERVER_STAGE} && mkdir -p ${SERVER_STAGE} && \
        tar xzf - -C ${SERVER_STAGE} && \
        mv ${SERVER_STAGE}/deploy/docker-entrypoint.sh ${SERVER_STAGE}/ && \
        rmdir ${SERVER_STAGE}/deploy && \
        chmod +x ${SERVER_STAGE}/sub2api-linux-amd64 ${SERVER_STAGE}/docker-entrypoint.sh"

# 4. 服务器构建薄镜像（COPY-only，无编译）
echo "==> [4/6] 服务器构建薄镜像 ccdirect:${TAG}"
ssh "${HOST}" "cd ${SERVER_STAGE} && \
    BASE_IMAGE=\$(docker ps --filter name=s2a-api --format '{{.Image}}' 2>/dev/null | head -1 || true) && \
    [ -n \"\${BASE_IMAGE}\" ] || BASE_IMAGE='alpine:3.21' && \
    cat > Dockerfile <<'DEOF'
FROM alpine:3.21
RUN apk add --no-cache tzdata su-exec libpq zstd-libs lz4-libs krb5-libs libldap libedit \\
    && rm -rf /var/cache/apk/*
COPY --from=postgres:18-alpine /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=postgres:18-alpine /usr/local/bin/psql /usr/local/bin/psql
COPY --from=postgres:18-alpine /usr/local/lib/libpq.so.5* /usr/local/lib/
RUN addgroup -g 1000 sub2api && adduser -u 1000 -G sub2api -s /bin/sh -D sub2api
WORKDIR /app
COPY sub2api-linux-amd64 /app/sub2api
COPY resources /app/resources
DEOF
    if [ \"\${BASE_IMAGE}\" != 'alpine:3.21' ]; then
        echo 'COPY --from='\${BASE_IMAGE}' /app/data /app/data' >> Dockerfile
    else
        echo 'warning: 无现有 s2a-api 容器，/app/data 留空（首次部署需手动放入 config.yaml）' >&2
    fi
    cat >> Dockerfile <<'DEOF'
RUN chown -R sub2api:sub2api /app/data
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \\
    CMD wget -q -T 5 -O /dev/null http://localhost:\${SERVER_PORT:-8080}/health || exit 1
ENTRYPOINT [\"/app/docker-entrypoint.sh\"]
CMD [\"/app/sub2api\"]
DEOF
    docker build -t ccdirect:${TAG} ."

# 5. 容器替换（复用旧容器全部运行参数，仅 env-file / 网络 / 端口 / healthcheck 硬编码）
echo "==> [5/6] 替换容器 s2a-api -> ccdirect:${TAG}"
OLD_IMAGE="$(ssh "${HOST}" "docker ps --filter name=s2a-api --format '{{.Image}}' 2>/dev/null | head -1 || echo 'none'")"
echo "    旧镜像: ${OLD_IMAGE}（回滚用：docker run --name s2a-api --network s2a -p 18080:8080 --restart unless-stopped --env-file /home/zero/ccdirect.env <旧镜像>）"
ssh "${HOST}" "if [ -f /home/zero/ccdirect.env ]; then ENV_ARGS='--env-file /home/zero/ccdirect.env'; else echo 'warning: 服务器缺少 /home/zero/ccdirect.env' >&2; ENV_ARGS=''; fi; \
    docker rm -f s2a-api 2>/dev/null; \
    docker run -d --name s2a-api --network s2a -p 18080:8080 --restart unless-stopped \
        \${ENV_ARGS} \
        --health-cmd 'wget -q -T 5 -O /dev/null http://localhost:\${SERVER_PORT:-8080}/health || exit 1' \
        --health-interval 30s --health-timeout 10s --health-start-period 10s --health-retries 3 \
        ccdirect:${TAG}"

# 6. 验证
echo "==> [6/6] 验证"
for i in $(seq 1 12); do
    STATUS="$(ssh "${HOST}" "docker inspect s2a-api --format '{{.State.Health.Status}}' 2>/dev/null || echo unknown")"
    if [[ "${STATUS}" == "healthy" ]]; then break; fi
    sleep 5
done
ssh "${HOST}" "docker ps --filter name=s2a-api --format '{{.Status}} {{.Image}}'; \
    curl -s -o /dev/null -w 'health=%{http_code}\n' http://localhost:18080/health"
curl -s -o /dev/null -w "public=%{http_code}\n" "https://ccdirect.dev/"

echo "==> 部署完成: ccdirect:${TAG}"
