## 1. Setting layer：常量、DTO、默认值、校验

- [x] 1.1 在 `backend/internal/service/domain_constants.go` 增加 `SettingKeySupportChatLLMBaseURL = "support_chat_llm_base_url"` 与 `SettingKeySupportChatLLMAPIKey = "support_chat_llm_api_key"` 两个常量；**移除** `SettingKeySupportChatAPIKeyID`（grep 全仓没有第二处引用后再删）
- [x] 1.2 在 `backend/internal/handler/dto/settings.go` 的 `SettingsResponse` 中新增 `SupportChatLLMBaseURL string` 与 `SupportChatLLMAPIKey string`（必填）；在 `SettingsUpdateRequest` 中新增同名 `*string` 字段（可选/局部更新）；**移除** 两个 struct 中的 `SupportChatAPIKeyID` 字段
- [x] 1.3 在 `backend/internal/service/setting_service.go` 的 fresh-install seed map 中：删除 `SettingKeySupportChatAPIKeyID: "0"` 行；新增 `SettingKeySupportChatLLMBaseURL: ""`、`SettingKeySupportChatLLMAPIKey: ""`
- [x] 1.4 在 `setting_service.go` 的 typed-view loader（解析 setting rows → `SettingsResponse`）：移除 `support_chat_api_key_id` 解析；新增 base_url / api_key 字段读取；**对 api_key 做掩码处理**——`maskAPIKey(value)` 返回 `len>=4 ? "sk-***"+last4 : "***"`，空值返回 `""`
- [x] 1.5 在 `setting_service.go` 的 update validation 中：移除 `SupportChatAPIKeyID` 相关校验（"llm_enabled 时 api_key_id > 0"、"non-negative"）；新增校验：`base_url ≤500 chars`、非空时必须 `http(s)://` 前缀；`api_key 1..500 chars` 当非空；当 `support_chat_llm_enabled = true` 时，**effective**（请求值或既存存储值）的 base_url 与 api_key 都必须非空，否则返回 `400 INVALID_SUPPORT_CHAT_LLM_CREDENTIALS`
- [x] 1.6 在 `setting_service.go` 的 update apply 阶段：当 `SupportChatLLMAPIKey` 字段值等于"当前存储值的掩码"时，跳过该字段写入（leave-unchanged 语义）；其它情况（包括 `""` 和任意非掩码字符串）均原样写入
- [x] 1.7 在 `setting_service.go` 的 `GetSupportChatRuntime`（chat handler 用的运行时视图）中：移除 `APIKeyID int`；新增 `LLMBaseURL string`、`LLMAPIKey string`（cleartext，仅运行时用）
- [x] 1.8 在 `setting_service.go` 的 PublicSettings 投影函数中确认 `support_chat_llm_base_url` 与 `support_chat_llm_api_key` **不会**被写入公开 payload（已默认不在 PublicSettings 字段集，但要 grep 加一条保险断言/注释）
- [x] 1.9 在 `backend/internal/handler/admin/setting_handler.go` 中：移除注释里"深度校验 api_key_id 存在 / enabled / admin-owned 留给 admin handler 层"那段未实现的 TODO；该层不再需要做 api_key_id 二次校验

## 2. Chat handler：移除自调，改为外部转发

- [x] 2.1 在 `backend/internal/handler/support_chat_handler.go` 中，移除依赖注入字段 `apiKeyService` 与构造函数参数（grep 确认没有其它消费点后删）
- [x] 2.2 移除 helper `selfCallURL`；新增 helper `buildUpstreamChatURL(baseURL string) string`：去掉末尾 `/`，拼 `/chat/completions`
- [x] 2.3 改写 `(*SupportChatHandler).PostChat` 的 pre-flight：在原"`rt.APIKeyID <= 0` → 503 config_error"那一段，替换为"`rt.LLMBaseURL == "" || rt.LLMAPIKey == ""` → 503 config_error" with body `{"error":"config_error","message":"LLM credentials not configured"}`
- [x] 2.4 改写转发段：删除 `apiKeyService.GetByID(...)` 与 status 校验；`upstreamURL := buildUpstreamChatURL(rt.LLMBaseURL)`；`upstreamReq.Header.Set("Authorization", "Bearer "+rt.LLMAPIKey)`；保留原有 timeout / SSE 透传逻辑
- [x] 2.5 在 SSE 错误透传分支中识别 upstream `401`：当上游响应 status=401 时，emit `event: error\ndata: {"error":{"message":"Upstream authentication failed — verify support_chat_llm_api_key in admin settings", "type":"upstream_auth"}}\n\n` 并关闭 stream
- [x] 2.6 跑 `go generate ./...` 重生成 `wire_gen.go`；在 `backend/cmd/server/wire_gen.go` 与 `backend/internal/handler/wire.go` 检查 `SupportChatHandler` 构造点已不再传入 `apiKeyService`

## 3. Embedding service：复用同一对凭据

- [x] 3.1 在 `backend/internal/service/embedding_service.go` 中，移除依赖 `apiKeyService`（grep 确认无其它消费）；构造函数改为依赖 `settingService`（用于运行时读 base_url + api_key）
- [x] 3.2 把 embed 调用改写为：`baseURL, apiKey := settingService.GetSupportChatLLMCredentials(ctx)`（新增便捷读取方法或复用 runtime view），endpoint = `<baseURL>/embeddings`，header `Authorization: Bearer <apiKey>`
- [x] 3.3 当 `baseURL == "" || apiKey == ""` 时，embed 函数立刻返回带哨兵的 `(nil, ErrEmbeddingCredentialsMissing)`；调用方（FAQ CRUD / doc pipeline / retrieval helper）按既有失败兜底路径处理（`embedding = NULL` + warning / 空结果）
- [x] 3.4 在 doc pipeline 批量 embed 处增加：循环开始前若凭据缺失则直接 short-circuit 整个 batch（不浪费 HTTP 重试），把所有该批 chunks 标记为 `embedding = NULL` + 在 status.errors 加一条 `"missing_credentials"`
- [x] 3.5 在 retrieval helper 中：凭据缺失时返回 `(nil, nil)`（空结果而非错误），调用方继续走"无相关知识"分支

## 4. Test-connection 端点

- [x] 4.1 新增路由：`POST /api/admin/support/chat/test-llm-connection`，注册到 admin router（参考 `backend/internal/server/routes/admin.go` 中支持现有 admin support 路由的写法）
- [x] 4.2 新增 handler `(*SupportChatHandler).TestLLMConnection`（或独立的 admin handler）：接受 `{base_url, api_key, model}`；当 `api_key == "<masked sentinel of stored value>"` 时替换为存储的 cleartext
- [x] 4.3 实现 probe：5s timeout HTTP client，POST `<base_url>/chat/completions` body `{"model":<model>,"messages":[{"role":"user","content":"ping"}],"max_tokens":1,"stream":false}`；返回 `{ok, latency_ms, status_code, error}`
- [x] 4.4 base_url 校验：空 / 非 `http(s)://` 直接返回 `{ok:false, status_code:null, error:"invalid_base_url", latency_ms:0}` 不发出网络请求
- [x] 4.5 错误归一化：超时 → `error="timeout"`；DNS / TLS / connection refused → 对应分类；非 2xx 时若响应 body 是 JSON 取 `error.message`，否则 `"upstream non-2xx"`

## 5. 启动期 legacy detection

- [x] 5.1 在应用启动钩子（参考 `support_faq_migration.go` 注册位置）中加一个 `detectLegacySupportChatAPIKeyID` 一次性检查：通过通用 settings 仓库读 `support_chat_api_key_id` 原始值，若 `> 0` AND 新 `support_chat_llm_base_url` 为空，则 `log.Warn("legacy support_chat_api_key_id detected; please reconfigure support_chat_llm_base_url + support_chat_llm_api_key in admin Settings ; LLM chat will be disabled until reconfigured")`
- [x] 5.2 不删除 DB 中的 legacy 行（保留以支持回滚）；不写任何新值

## 6. 前端 admin SettingsView

- [x] 6.1 在 `frontend/src/api/admin/settings.ts` 的 `SettingsResponse` 与 `SettingsUpdateRequest` 中：移除 `support_chat_api_key_id`；新增 `support_chat_llm_base_url: string` / `support_chat_llm_api_key: string`（response 必填，update 可选）
- [x] 6.2 新增 API client 函数 `adminTestSupportChatLLMConnection(payload: {base_url, api_key, model})`，POST `/api/admin/support/chat/test-llm-connection`
- [x] 6.3 在 `frontend/src/views/admin/SettingsView.vue` 的 supportChat 区段：移除 ApiKey 选择下拉框 + 相关 candidate keys 加载逻辑（grep `support_chat_api_key_id` 全部清掉）
- [x] 6.4 新增两个表单控件：base_url 文本输入（placeholder `https://api.openai.com/v1`）+ api_key 密码输入（`type="password"` + 显示/隐藏切换按钮）；两个字段都加帮助文本指明"OpenAI-compatible upstream"
- [x] 6.5 表单状态：`apiKeyChanged` ref 默认 `false`；用户在 api_key 输入框 `@input` 触发后置 `true`；`buildPayload` 仅在 `apiKeyChanged` 为 true 时才 include `support_chat_llm_api_key` 字段（避免把掩码回写）
- [x] 6.6 form 初始化时：`form.support_chat_llm_base_url = ""`、`form.support_chat_llm_api_key = ""`；既有的 `Object.entries(settings) → form[k]=v` loader 会自动覆盖（api_key 加载的是掩码）
- [x] 6.7 新增"Test connection"按钮：disabled 当 base_url 为空；点击触发 `adminTestSupportChatLLMConnection({base_url: form.value, api_key: form.value, model: form.support_chat_model})`；展示 `ok ? "✓ <latency>ms" : "✗ <error>"` 横幅 5s 后消失
- [x] 6.8 顶部 banner：当 `form.support_chat_llm_enabled = true` 且（base_url 为空 OR api_key 等于掩码且未改动且原存储为空——通过返回空字符串体现）时，显示黄色警告"启用 LLM 但未配置凭据"
- [x] 6.9 i18n: `frontend/src/i18n/locales/zh.ts` + `en.ts` 增加 `admin.settings.supportChat.llm.{baseUrlLabel,baseUrlHint,apiKeyLabel,apiKeyHint,testBtn,testing,testOk,testFailed,credentialsMissingWarn}`；移除旧的 `apiKeyIdLabel` / `apiKeySelectHint` 等键

## 7. 校验 + 归档

- [x] 7.1 `openspec validate change-support-chat-external-llm --strict` 通过
- [x] 7.2 后端 build：`cd backend && go build ./...` 通过
- [x] 7.3 前端类型检查：`cd frontend && npx vue-tsc --noEmit` 通过
- [x] 7.4 手测：admin 页保存 `base_url=https://api.openai.com/v1` + `api_key=sk-real` + `llm_enabled=true`；客服浮窗发起一次对话；后端 access log 应看到出站 `https://api.openai.com/v1/chat/completions`，本机不再有 `127.0.0.1:<port>/v1/chat/completions` 自调日志
- [x] 7.5 手测：清空 base_url 同时保持 llm_enabled=true → PUT 返回 400；保存后再开 chat → 503 config_error
- [x] 7.6 手测：FAQ 创建一条 → 检查 DB 行 `embedding IS NOT NULL`（外部 embeddings 调用成功）
- [x] 7.7 sync delta specs 进 main specs：将 `change-support-chat-external-llm/specs/support-chat/spec.md` 中的 ADDED + MODIFIED 套用进 `openspec/specs/support-chat/spec.md`；将 `support-knowledge-rag` 的 ADDED requirement 追加进 `openspec/specs/support-knowledge-rag/spec.md`
- [x] 7.8 归档：`mv openspec/changes/change-support-chat-external-llm openspec/changes/archive/<YYYY-MM-DD>-change-support-chat-external-llm`
