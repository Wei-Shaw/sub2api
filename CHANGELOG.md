# Changelog

## Unreleased

### Fix: Docker 构建失败（docs/legal 文件缺失）

- **Dockerfile**: 添加 `COPY docs/legal/ /app/docs/legal/`，解决前端 `LegalDocumentView.vue` 通过相对路径引用 `docs/legal/admin-compliance.*.md` 在容器内找不到文件导致 vite build 失败的问题。
- **.dockerignore**: 添加 `!docs/legal/` 和 `!docs/legal/*.md` 例外规则，使 `docs/legal/` 目录不被全局的 `docs/` 和 `*.md` 排除规则过滤掉。
- **删除 `frontend/pnpm-workspace.yaml`**: 该文件由本地 pnpm v11 的 `approve-builds` 生成，与 Dockerfile 中 pnpm@9 不兼容（pnpm@9 要求 workspace 文件必须包含 `packages` 字段），导致 `pnpm install` 和 `pnpm run build` 报 "packages field missing or empty"。

- Added a config script download action to each API key row, matching the existing API key action area.
- Added a config script dropdown with Codex CLI, Claude Code, and OpenCode options, styled after the provided reference image.
- Added automatic OS detection so macOS downloads `.sh` scripts and Windows downloads `.bat` scripts.
- Added config script generation for Codex CLI, Claude Code, and OpenCode with the current API endpoint and API key injected into the generated files.
- Set the generated script site name to `look2eye`.
- Added Chinese and English i18n text for the config script button, menu, hints, and download states.
- Added focused unit coverage for config script generation and client availability rules.


本地 docker 部署
export http_proxy=http://127.0.0.1:7890                                                                                            
export https_proxy=http://127.0.0.1:7890
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
docker compose -f docker-compose.dev.yml up --build -d

上传自己的 docker hub
export http_proxy=http://127.0.0.1:7890                                                                                            
export https_proxy=http://127.0.0.1:7890
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
docker buildx build --platform linux/amd64 -t docker.io/doctor11ma/sub2api:latest -t docker.io/doctor11ma/sub2api:v0.1.138 --push .

feat
上传自己的 docker hub
export http_proxy=http://127.0.0.1:7890                                                                                            
export https_proxy=http://127.0.0.1:7890
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
docker buildx build --platform linux/amd64 -t docker.io/doctor11ma/sub2api:v0.1.138feat --push .

