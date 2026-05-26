#!/usr/bin/env bash

set -euo pipefail

# 用法：
#   sudo ./install-datamanagementd.sh --binary /path/to/datamanagementd
# 或：
#   sudo ./install-datamanagementd.sh --source /path/to/sub2api/repo

BIN_PATH=""
SOURCE_PATH=""
INSTALL_DIR="/opt/sub2api"
DATA_DIR="/var/lib/sub2api/datamanagement"
SERVICE_FILE_NAME="sub2api-datamanagementd.service"

function print_help() {
  cat <<'EOF'
用法:
  install-datamanagementd.sh [--binary <datamanagementd二进制路径>] [--source <仓库路径>]

参数:
  --binary  指定已构建的 datamanagementd 二进制路径
  --source  指定 sub2api 仓库路径（脚本会执行 go build）
  -h, --help 显示帮助

示例:
  sudo ./install-datamanagementd.sh --binary ./datamanagement/datamanagementd
  sudo ./install-datamanagementd.sh --source /opt/sub2api-src
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary)
      BIN_PATH="${2:-}"
      shift 2
      ;;
    --source)
      SOURCE_PATH="${2:-}"
      shift 2
      ;;
    -h|--help)
      print_help
      exit 0
      ;;
    *)
      echo "未知参数: $1"
      print_help
      exit 1
      ;;
  esac
done

if [[ -n "$BIN_PATH" && -n "$SOURCE_PATH" ]]; then
  echo "错误: --binary 与 --source 只能二选一"
  exit 1
fi

if [[ -z "$BIN_PATH" && -z "$SOURCE_PATH" ]]; then
  echo "错误: 必须提供 --binary 或 --source"
  exit 1
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "错误: 请使用 root 权限执行（例如 sudo）"
  exit 1
fi

if [[ -n "$SOURCE_PATH" ]]; then
  if [[ ! -d "$SOURCE_PATH/datamanagement" ]]; then
    echo "错误: 无效仓库路径，未找到 $SOURCE_PATH/datamanagement"
    exit 1
  fi
  echo "[1/6] 从源码构建 datamanagementd..."
  (cd "$SOURCE_PATH/datamanagement" && go build -o datamanagementd ./cmd/datamanagementd)
  BIN_PATH="$SOURCE_PATH/datamanagement/datamanagementd"
fi

if [[ ! -f "$BIN_PATH" ]]; then
  echo "错误: 二进制文件不存在: $BIN_PATH"
  exit 1
fi

if ! id sub2api >/dev/null 2>&1; then
  echo "[2/6] 创建系统用户 sub2api..."
  useradd --system --no-create-home --shell /usr/sbin/nologin sub2api
else
  echo "[2/6] 系统用户 sub2api 已存在，跳过创建"
fi

echo "[3/6] 安装 datamanagementd 二进制..."
mkdir -p "$INSTALL_DIR"
install -m 0755 "$BIN_PATH" "$INSTALL_DIR/datamanagementd"

echo "[4/6] 准备数据目录..."
mkdir -p "$DATA_DIR"
chown -R sub2api:sub2api /var/lib/sub2api
chmod 0750 "$DATA_DIR"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_TEMPLATE="$SCRIPT_DIR/$SERVICE_FILE_NAME"
if [[ ! -f "$SERVICE_TEMPLATE" ]]; then
  echo "错误: 未找到服务模板 $SERVICE_TEMPLATE"
  exit 1
fi

echo "[5/6] 安装 systemd 服务..."
cp "$SERVICE_TEMPLATE" "/etc/systemd/system/$SERVICE_FILE_NAME"
systemctl daemon-reload
systemctl enable --now sub2api-datamanagementd

echo "[6/6] 完成，当前状态："
systemctl --no-pager --full status sub2api-datamanagementd || true

cat <<'EOF'

下一步建议：
1. 查看日志：sudo journalctl -u sub2api-datamanagementd -f
2. 在 sub2api（容器部署时）挂载 socket:
   /tmp/sub2api-datamanagement.sock:/tmp/sub2api-datamanagement.sock
3. 进入管理后台“数据管理”页面确认 agent=enabled

EOF
