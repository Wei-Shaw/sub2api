# QA 经验

## pnpm 11 安装被依赖构建审批阻断

- 现象：`pnpm install --frozen-lockfile` 返回 `ERR_PNPM_IGNORED_BUILDS`，并提示 `esbuild`、`vue-demi` 的构建脚本未获批准。
- 原因：pnpm 11 不再读取 `package.json#pnpm.overrides`，依赖构建权限也必须写入 `pnpm-workspace.yaml`。
- 处理：在 `frontend/pnpm-workspace.yaml` 的 `allowBuilds` 中只允许 `esbuild` 和 `vue-demi`，并将安全版本覆盖迁到同一文件的 `overrides`。
- 验证：重新运行 `pnpm install --frozen-lockfile`，不得使用交互式 `pnpm approve-builds` 生成机器相关配置。

## pnpm 运行 Vite 时端口参数未生效

- 现象：`pnpm run preview -- --host 127.0.0.1 --port 4177` 实际启动在默认的 4173 端口。
- 原因：该脚本下额外的 `--` 被传给 Vite，后续参数未被解析为 CLI 选项。
- 处理：直接使用 `pnpm run preview --host 127.0.0.1 --port 4177`。
- 验证：启动日志必须显示 `http://127.0.0.1:4177/`，再执行浏览器 QA。

## Docker 中 pnpm 9 无法读取 pnpm 11 workspace 配置

- 现象：依赖安装成功，但 `pnpm run build` 报 `packages field missing or empty`；提前复制 workspace 配置后，pnpm 9 frozen install 可能继续报 overrides 不匹配。
- 原因：pnpm 9 要求 workspace 明确声明 `packages`，并从 `package.json#pnpm.overrides` 读取覆盖；pnpm 11 则从 `pnpm-workspace.yaml` 读取覆盖。
- 处理：单包项目声明 `packages: ['.']`，在两个位置保留同值 overrides，并让 Docker 在安装前复制 `pnpm-workspace.yaml`。
- 验证：分别使用根 Dockerfile 和 `deploy/Dockerfile` 构建 `frontend-builder`，其中 frozen install 必须显示锁文件已是最新。
