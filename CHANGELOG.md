# Changelog

## Unreleased

### Fix: Docker 构建失败

- **Dockerfile**: 将 pnpm 版本从 v9 升级到 v11，与本地开发环境对齐，解决 lockfile 格式不兼容问题。
- **Dockerfile**: COPY 阶段补充 `pnpm-workspace.yaml` 和 `.npmrc`，确保容器内 pnpm install 能读到完整配置。
- **Dockerfile**: 添加 `COPY docs/legal/`，解决前端构建时引用 `docs/legal/*.md` 文件找不到的问题。
- **frontend/package.json**: 移除 `pnpm.overrides` 字段（pnpm v10+ 不再从此处读取）。
- **frontend/pnpm-workspace.yaml**: 将 `overrides` 配置迁移至此文件，符合 pnpm v10+ 规范。
- **frontend/pnpm-lock.yaml**: 重新生成，与新配置位置匹配。
- **.dockerignore**: 添加 `!docs/legal/` 和 `!docs/legal/*.md` 例外规则，允许 legal 文档进入 Docker 构建上下文。

- Added a config script download action to each API key row, matching the existing API key action area.
- Added a config script dropdown with Codex CLI, Claude Code, and OpenCode options, styled after the provided reference image.
- Added automatic OS detection so macOS downloads `.sh` scripts and Windows downloads `.bat` scripts.
- Added config script generation for Codex CLI, Claude Code, and OpenCode with the current API endpoint and API key injected into the generated files.
- Set the generated script site name to `look2eye`.
- Added Chinese and English i18n text for the config script button, menu, hints, and download states.
- Added focused unit coverage for config script generation and client availability rules.
