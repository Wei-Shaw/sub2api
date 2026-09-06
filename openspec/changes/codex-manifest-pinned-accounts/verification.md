# 普通模型列表扩展验证（2026-09-05）

## 已实现

普通 `/v1/models` 与 `/models` 共用分组固定账号配置。API Key 使用标准模型端点；OAuth/Agent Identity 继续使用现有 Codex 上游认证链路。两种目录共享固定账号筛选、并发、部分失败处理和缓存策略；固定账号发现先于本地目录，映射、分组过滤和 ETag 在缓存之后应用。

普通列表保留上游未知模型、媒体模型及模型元数据。有效空目录/过滤为空不会触发静态默认或调度器回退。无可用账号返回 503，上游失败遵守回退配置；客户端错误使用 OpenAI 错误信封。

## 自动验证

- `GOCACHE=/tmp/sub2api-go-cache go test -tags=unit ./... -timeout=5m`：通过，55 个含测试的包成功。
- 最后补充响应格式缓存隔离和冷缓存 304 错误处理后，执行 `GOCACHE=/tmp/sub2api-go-cache go test -race -tags=unit -p 2 ./internal/service ./internal/handler ./internal/server/routes -run 'Test.*(OpenAIModels|CodexModels|OrdinaryPinned|PinnedModels|ProjectAccountModels)' -count=1 -timeout=3m`：三个包全部通过。
- 最终代码执行 `GOCACHE=/tmp/sub2api-go-cache GOLANGCI_LINT_CACHE=/tmp/sub2api-models-lint-cache golangci-lint run ./... --timeout=5m`：0 issues。
- 前端使用已安装依赖执行 `./node_modules/.bin/vitest run src/components/admin/group/__tests__/CodexManifestAccountsField.spec.ts src/views/admin/__tests__/GroupsView.duplicate.spec.ts`：2 个文件、10 个测试通过。
- `./node_modules/.bin/vue-tsc --noEmit`：通过。
- 两个修改的 i18n 文件 ESLint：通过。
- `openspec validate codex-manifest-pinned-accounts --strict`、`git diff --check`：通过。

回归覆盖普通两条路由（含空 client_version）、三个 Codex 入口、API Key/OAuth 混合账号、影子账号凭据解析、优先级/临时限流、停用/过期/非成员跳过、部分失败/全部失败/调度器回退、映射别名和通配规则、跨分组并发缓存隔离、OAuth 跨入口缓存复用、API Key 协议隔离、三段时效/单飞/ETag 续期、最终响应 304、空目录、媒体别名不能绕过 Codex 专用过滤。

## 验证边界

测试使用本地模拟上游和仓储，没有访问真实 OpenAI/ChatGPT 账号或部署服务。原任务 6.4 的真实客户端手动联调仍未勾选；本次新增 7.1—7.6 已完成。
