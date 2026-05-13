# 对话聊天功能开发记录

日期：2026-05-12

## 目标

新增用户侧“对话聊天”功能。用户只能选择自己在平台创建的 Sub2API API Key 发起聊天，不支持填写上游厂商密钥。功能由后台开关控制，菜单位置放在“AI生图”下方。

## 本次修改

### 后端

- 新增系统设置键：`chat_completion_enabled`。
- 默认值为 `false`，与 AI 生图一样需要管理员显式开启。
- 接入公开设置、后台设置、SSR 注入配置、设置更新和审计变更列表。
- 新增 `/api/v1/chat/completions` 网关别名，供前端应用在同一 API base path 下发起流式聊天请求；原有 `/v1/chat/completions` 和 `/chat/completions` 仍保留。

### 前端

- 新增 feature flag：`FeatureFlags.chatCompletion`，按 opt-in 处理。
- 用户侧新增路由：`/chat`。
- 侧边栏新增“对话聊天”菜单，位于 `/images` 后、`/usage` 前。
- 调整 Vite 开发代理默认后端地址为 `http://127.0.0.1:8080`，避免 `localhost` 在本地开发环境中解析到不可用地址。
- 二开项目不再需要官方更新检查：已移除侧边栏版本组件对 `/api/v1/admin/system/check-updates` 的自动调用，版本号仅展示公开设置注入的当前版本，避免 GitHub 访问不通时拖慢页面。
- 新增 `frontend/src/api/chat.ts`：
  - `createChatCompletion`：非流式兼容 helper。
  - `streamChatCompletion`：使用 `fetch` 读取 `${VITE_API_BASE_URL || '/api/v1'}/chat/completions` SSE 流。
- 新增 `ChatCompletionView.vue`：
  - 加载当前用户 active 且已绑定分组的 API Key。
  - 记住上次选择的 Key。
  - 选中 API Key 后按其 `group_id` 从用户可见渠道定价数据中汇总支持模型并提供下拉选择，不再要求用户手动输入模型名。
  - 将页面重构为左右工作台布局：左侧为 Key/模型上下文，右侧为对话区和输入区；页面标题和外层间距对齐模型广场。
  - 默认使用流式响应追加助手内容。
  - 支持停止生成、清空对话、错误提示；清空对话移动到页面顶部操作区。

### 测试

- 新增/更新设置、网关路由、feature flag、侧边栏顺序、路由声明和 chat API 测试。

## 验证结果

- `go test -tags unit ./internal/service -run 'TestSettingService_GetPublicSettings_.*ChatCompletion|TestSettingService_GetPublicSettings_.*ImageGeneration'`：通过。
- `go test ./internal/handler/dto -run TestPublicSettingsInjectionPayload_SchemaDoesNotDrift`：通过。
- `go test ./internal/handler/admin -run TestSetting`：通过。
- `go test ./internal/server/routes -run TestGatewayRoutesOpenAI`：通过。
- `pnpm vitest run src/utils/__tests__/featureFlags.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts src/router/__tests__/title.spec.ts src/api/__tests__/chat.spec.ts`：通过，4 个文件、14 个测试。
- `pnpm typecheck`：通过。
- `git diff --check`：通过。

## 注意事项

- `.gitignore` 已为本文件增加例外规则，后续可随本次功能代码一起提交。
- 本地曾存在被 `.gitignore` 忽略的 `frontend/vite.config.js` / `frontend/vite.config.d.ts` 生成文件，会覆盖 `vite.config.ts` 的代理配置；已删除这些本地生成文件。修改代理配置后需要重启前端 dev server。
