#!/usr/bin/env bash
# 一键构建包含网页界面的 Ubuntu x86_64 可执行文件。
# 用法：./build-ubuntu.sh
# 跳过 UPX 压缩：SKIP_UPX=1 ./build-ubuntu.sh
set -euo pipefail

PROJECT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="$PROJECT_DIR/backend/bin"
BINARY_NAME="sub2api-linux-amd64"
SKIP_UPX="${SKIP_UPX:-0}"

fail() {
  printf '错误：%s\n' "$*" >&2
  exit 1
}

for dependency in go node pnpm; do
  command -v "$dependency" >/dev/null 2>&1 || fail "请先安装 $dependency。"
done

case "$SKIP_UPX" in
  0)
    command -v upx >/dev/null 2>&1 || fail \
      '请先安装 UPX（macOS：brew install upx；Ubuntu：sudo apt install upx-ucl），或使用 SKIP_UPX=1 跳过压缩。'
    ;;
  1) ;;
  *) fail 'SKIP_UPX 只能设置为 0 或 1。' ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
  CHECKSUM_COMMAND=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  CHECKSUM_COMMAND=(shasum -a 256)
else
  fail '请先安装 sha256sum 或 shasum，用于生成校验文件。'
fi

printf '\n[1/4] 安装前端依赖并构建网页界面\n'
(
  cd "$PROJECT_DIR/frontend"
  pnpm install --frozen-lockfile
  pnpm run build
)
[[ -s "$PROJECT_DIR/backend/internal/web/dist/index.html" ]] || fail '未找到前端构建产物 index.html。'

mkdir -p "$OUTPUT_DIR"
BUILD_DIR="$(mktemp -d "$OUTPUT_DIR/.ubuntu-build.XXXXXX")"
trap 'rm -rf -- "$BUILD_DIR"' EXIT

BUILD_VERSION="$(sh "$PROJECT_DIR/backend/scripts/resolve-version.sh")"
BUILD_COMMIT="$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'unknown')"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

printf '\n[2/4] 编译 Ubuntu x86_64 后端，并嵌入网页界面\n'
(
  cd "$PROJECT_DIR/backend"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOAMD64=v1 go build \
    -tags=embed \
    -trimpath \
    -buildvcs=false \
    -ldflags="-s -w -buildid= -X main.Version=$BUILD_VERSION -X main.Commit=$BUILD_COMMIT -X main.Date=$BUILD_DATE -X main.BuildType=release" \
    -o "$BUILD_DIR/$BINARY_NAME" \
    ./cmd/server
)
cp "$BUILD_DIR/$BINARY_NAME" "$BUILD_DIR/$BINARY_NAME.unpacked"

if [[ "$SKIP_UPX" == 0 ]]; then
  printf '\n[3/4] 使用 UPX 高压缩，并检查压缩文件完整性\n'
  upx --best --lzma "$BUILD_DIR/$BINARY_NAME"
  upx -t "$BUILD_DIR/$BINARY_NAME"
else
  printf '\n[3/4] 已按 SKIP_UPX=1 跳过压缩\n'
fi

printf '\n[4/4] 生成 SHA-256 校验文件并保存构建产物\n'
(
  cd "$BUILD_DIR"
  "${CHECKSUM_COMMAND[@]}" "$BINARY_NAME" > "$BINARY_NAME.sha256"
)
for artifact in "$BINARY_NAME" "$BINARY_NAME.unpacked" "$BINARY_NAME.sha256"; do
  mv -f "$BUILD_DIR/$artifact" "$OUTPUT_DIR/$artifact"
done

printf '\n构建完成（包含前端，Linux amd64 静态可执行文件）：\n'
ls -lh "$OUTPUT_DIR/$BINARY_NAME" "$OUTPUT_DIR/$BINARY_NAME.unpacked"
printf '\n上传可执行文件到 Ubuntu 后运行：\n  chmod +x %s\n  ./%s\n' "$BINARY_NAME" "$BINARY_NAME"
printf '\n校验文件：%s/%s.sha256\n' "$OUTPUT_DIR" "$BINARY_NAME"
