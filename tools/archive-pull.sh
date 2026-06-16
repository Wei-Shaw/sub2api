#!/usr/bin/env bash
#
# archive-pull.sh — 把 VPS 上的请求/响应归档分片增量拉取到本地（外接硬盘）。
#
# 归档分片一旦关闭即不可变（append-only 的 .jsonl.zst），用 rsync 增量续传安全；
# --remove-source-files 在每个文件成功传完后删除源文件，自动释放 VPS 空间。
# 文件已是 zstd 压缩，故不加 rsync 的 -z（避免无谓 CPU）。
#
# 用法：
#   archive-pull.sh <user@host> <remote_base> <local_base> [YYYY/MM] [bwlimit_kbps]
#
# 示例：
#   # 拉取 2026 年 6 月整月到外接硬盘，限速 3 MB/s（与业务共享 50M 带宽时建议限速）
#   archive-pull.sh root@vps /app/data/archive /Volumes/HDD/sub2api-archive 2026/06 3000
#
#   # 不限速拉取全部（建议挑业务低谷时段）
#   archive-pull.sh root@vps /app/data/archive /Volumes/HDD/sub2api-archive
#
set -euo pipefail

if [[ $# -lt 3 ]]; then
  echo "用法: $0 <user@host> <remote_base> <local_base> [YYYY/MM] [bwlimit_kbps]" >&2
  exit 1
fi

REMOTE_HOST="$1"
REMOTE_BASE="${2%/}"
LOCAL_BASE="${3%/}"
SUBPATH="${4:-}"          # 例如 2026/06；留空则整个归档目录
BWLIMIT="${5:-}"          # 例如 3000（KB/s）；留空则不限速

REMOTE_PATH="${REMOTE_BASE}"
LOCAL_PATH="${LOCAL_BASE}"
if [[ -n "${SUBPATH}" ]]; then
  REMOTE_PATH="${REMOTE_BASE}/${SUBPATH}"
  LOCAL_PATH="${LOCAL_BASE}/${SUBPATH}"
fi

mkdir -p "${LOCAL_PATH}"

RSYNC_OPTS=(-av --prune-empty-dirs --remove-source-files --partial --info=progress2)
# 仅传输已关闭的压缩分片，跳过正在写入的临时文件与盐文件。
RSYNC_OPTS+=(--include='*/' --include='*.jsonl.zst' --exclude='*')
if [[ -n "${BWLIMIT}" ]]; then
  RSYNC_OPTS+=(--bwlimit="${BWLIMIT}")
fi

echo ">> 拉取 ${REMOTE_HOST}:${REMOTE_PATH}/ -> ${LOCAL_PATH}/"
rsync "${RSYNC_OPTS[@]}" "${REMOTE_HOST}:${REMOTE_PATH}/" "${LOCAL_PATH}/"

# 清理 VPS 上传完后残留的空目录（--remove-source-files 只删文件不删目录）。
echo ">> 清理远端空目录"
ssh "${REMOTE_HOST}" "find '${REMOTE_PATH}' -type d -empty -delete 2>/dev/null || true"

echo ">> 完成。本地查看示例："
echo "   zstdcat ${LOCAL_PATH}/<file>.jsonl.zst | jq            # 美化浏览"
echo "   zstd -d ${LOCAL_PATH}/<file>.jsonl.zst                 # 解压成明文 .jsonl"
echo "   zstdcat ${LOCAL_PATH}/*.jsonl.zst | jq 'select(.model==\"claude-opus-4-8\")'"
