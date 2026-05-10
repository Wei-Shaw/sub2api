#!/bin/bash
# ============================================================
# Sub2API 一键构建部署包脚本
# 用法: ./build.sh
# 输出: backend/bin/darwin-arm64/  +  backend/bin/linux-amd64/
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_DIR="$PROJECT_DIR/frontend"
BIN_DIR="$BACKEND_DIR/bin"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log()  { echo -e "${GREEN}[BUILD]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()  { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# ---- 0. 预检 ----
command -v go    >/dev/null 2>&1 || err "需要安装 Go"
command -v pnpm  >/dev/null 2>&1 || err "需要安装 pnpm"

# ---- 1. 构建前端 ----
log "构建前端..."
cd "$FRONTEND_DIR"
pnpm install --frozen-lockfile 2>/dev/null || pnpm install
pnpm build
log "前端构建完成"

# ---- 2. 构建 Go 后端 ----
cd "$BACKEND_DIR"

PLATFORMS=(
  "linux/amd64:linux-amd64"
  "darwin/arm64:darwin-arm64"
)

for entry in "${PLATFORMS[@]}"; do
  GOOS="${entry%%:*}"
  REST="${entry#*:}"
  GOARCH="${REST%%:*}"
  DIR="${REST#*:}"

  log "编译 $GOOS/$GOARCH ..."
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -tags embed -ldflags="-s -w" -o "$BIN_DIR/$DIR/sub2api" ./cmd/server
done
log "Go 编译完成"

# ---- 3. 组装部署包 ----
log "组装部署包..."

for DIR in darwin-arm64 linux-amd64; do
  TARGET="$BIN_DIR/$DIR"

  # 清理并重建
  rm -rf "$TARGET"
  mkdir -p "$TARGET/data/pages"

  # 拷贝二进制（上面已编译到此处）
  chmod +x "$TARGET/sub2api"

  # 配置文件
  if [ -f "$BACKEND_DIR/config.yaml" ]; then
    cp "$BACKEND_DIR/config.yaml" "$TARGET/"
  else
    cp "$PROJECT_DIR/deploy/config.example.yaml" "$TARGET/config.yaml"
    warn "$DIR: 使用 config.example.yaml，请根据实际环境修改"
  fi

  # 数据文件
  for f in model_pricing.json model_pricing.sha256; do
    [ -f "$BACKEND_DIR/data/$f" ] && cp "$BACKEND_DIR/data/$f" "$TARGET/data/"
  done
  [ -d "$BACKEND_DIR/data/pages" ] && cp -r "$BACKEND_DIR/data/pages/"* "$TARGET/data/pages/" 2>/dev/null || true

  # 启动脚本
  cat > "$TARGET/start.sh" << 'STARTSCRIPT'
#!/bin/bash
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"
mkdir -p data
echo "=== CloseAI Sub2API ==="
echo "启动中..."
./sub2api
STARTSCRIPT
  chmod +x "$TARGET/start.sh"

  # Linux 额外文件
  if [ "$DIR" = "linux-amd64" ]; then
    # systemd service
    cat > "$TARGET/sub2api.service" << 'SERVICE'
[Unit]
Description=CloseAI Sub2API Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/sub2api
ExecStart=/opt/sub2api/sub2api
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICE

    # deploy.sh 一键部署脚本
    cat > "$TARGET/deploy.sh" << 'DEPLOYSCRIPT'
#!/bin/bash
# 一键部署到服务器
# 用法: ssh root@YOUR_SERVER "bash -s" < deploy.sh
set -e

INSTALL_DIR="/opt/sub2api"
SERVICE_NAME="sub2api"

echo "=== CloseAI Sub2API 部署 ==="

# 停止旧服务
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
    echo "停止旧服务..."
    systemctl stop "$SERVICE_NAME"
fi

# 创建目录
mkdir -p "$INSTALL_DIR/data"

# 拷贝文件
cp sub2api "$INSTALL_DIR/"
cp config.yaml "$INSTALL_DIR/"
cp -r data/* "$INSTALL_DIR/data/" 2>/dev/null || true
chmod +x "$INSTALL_DIR/sub2api"

# 安装 systemd 服务
cp sub2api.service /etc/systemd/system/
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl start "$SERVICE_NAME"

echo "=== 部署完成 ==="
systemctl status "$SERVICE_NAME" --no-pager
DEPLOYSCRIPT
    chmod +x "$TARGET/deploy.sh"
  fi

  log "$DIR: 打包完成"
done

# ---- 4. 汇总 ----
echo ""
echo -e "${GREEN}============================================${NC}"
echo -e "${GREEN}  构建完成！${NC}"
echo -e "${GREEN}============================================${NC}"
echo ""
echo "  macOS ARM64:  $BIN_DIR/darwin-arm64/"
echo "  Linux AMD64:  $BIN_DIR/linux-amd64/"
echo ""
echo "  Linux 一键部署:"
echo "    cd $BIN_DIR/linux-amd64"
echo "    scp -r . root@SERVER:/opt/sub2api/"
echo "    ssh root@SERVER 'bash /opt/sub2api/deploy.sh'"
echo ""
echo "  macOS 本地启动:"
echo "    cd $BIN_DIR/darwin-arm64 && ./start.sh"
echo ""