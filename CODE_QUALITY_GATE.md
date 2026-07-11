# Sub2API 代码质量门禁

更新时间：2026-07-11

## 每次变更

- 行为修复先写能稳定复现风险的失败测试，再做最小实现。
- 运行覆盖改动的 Go / Vitest 定向测试。
- `git diff --check` 必须通过。
- 不读取或输出 `.env`、交付密钥、token、cookie；不提交交付包敏感文件或大产物。

## 本轮最低门禁

```text
backend: go vet ./internal/config ./internal/service ./internal/handler ./internal/repository ./cmd/server
backend: go test ./internal/config ./internal/service ./internal/handler ./internal/server/routes ./cmd/server -count=1
frontend: pnpm run build
frontend: npx vitest run src/views/admin/video --reporter=basic
docker: official Dockerfile build/config validation and entrypoint smoke when runtime is available
integration: targeted repository suite must actually execute
lock: pnpm 9 frozen install against package.json + pnpm-lock.yaml must exit 0
image: root Dockerfile build plus su-exec/entrypoint/health smoke when Docker is available
```

为避免本地未跟踪 workspace 文件干扰官方镜像的两文件安装上下文，lock 门禁应在临时目录或容器中只放入 `frontend/package.json` 与 `frontend/pnpm-lock.yaml` 后执行。完整镜像门禁仍以根 `Dockerfile` 为准。

## Integration 诚实策略

- 默认：Docker/Testcontainers 不可用时返回非零并说明依赖。
- 只有显式的本地探测模式才可允许 skip；该模式必须输出机器可识别的 `SKIPPED`，且不得作为质量门禁通过证据。
- CI 不允许 skip integration。
- Integration 用例必须使用唯一 fixture，并只清理/断言本用例创建的 ID；禁止全表 DELETE 或批量取消其他用例任务换绿。

## 状态口径

mock-only 证据最多支持“内部可用 / 可演示”；真实 Provider、支付、生产部署与公网路径未验证时，不得标为生产 READY。
Windows rootless Docker panic 或镜像代理 429 必须记为外部门禁，不得写成代码通过。
