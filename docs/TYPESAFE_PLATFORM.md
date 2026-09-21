# TypeSafe 平台（platform=typesafe）交接：上线、行为边界与 integration 合并清单

> **整理日期**: 2026-09-21
> **分支**: `feature/typesafe-platform`（基于本地 `main`；HEAD = `a5769ffa4`）
> **范围**: 一级平台 `platform=typesafe`（TypeSafe AI 的 Jev 判断题服务）+ 原生透传端点 `POST /v1/systemone`
> **文档位置**: 本文已从本地研究笔记目录 `docs/research/`（`docs/*` 默认忽略、不入库）归位到 `docs/TYPESAFE_PLATFORM.md`，并在 `.gitignore` 的 `!docs/...` 白名单中登记；这次归位本身是紧随 `a5769ffa4` 之后的一个提交。
> **证据基线**: 本文所有路径、行号、测试名、环境变量均在本分支工作区逐条核实过；带「未验证」标注的除外。行号对应上述 HEAD `a5769ffa4` 的工作区状态。

---

## 1. 这是什么

新增一级平台 `platform=typesafe`，承载 **TypeSafe AI 的 Jev 判断题服务**，并提供一个原生透传端点：

| 项 | 值 | 依据 |
| --- | --- | --- |
| 平台值 | `typesafe` | `backend/internal/domain/constants.go:36`；service 层别名 `backend/internal/service/domain_constants.go:53` |
| 端点 | `POST /v1/systemone` | 路由与平台门 `backend/internal/server/routes/gateway.go:308-321` |
| 请求体 | `{model, state, questions}`（`questions` 是 `map[string]{type, instructions}`） | `backend/internal/pkg/typesafe/client.go:18-24` |
| 响应体 | `{model, usage, answers}`（`answers[id] = {type, noul}`） | `backend/internal/pkg/typesafe/client.go:66-74` |
| 形态 | 同步、无流式、不生成文本、非 OpenAI 兼容 | `backend/internal/handler/openai_systemone.go:21-26`；`backend/internal/service/openai_systemone.go:193`（`Stream: false`） |
| 上游地址 | `{base_url}/v1/systemone` | `backend/internal/service/openai_systemone.go:280-282` |

**为什么是一级平台，而不是挂在 `openai` 平台下**

本仓库的平台标签表示「上游厂商 / 凭据池」，不是「请求走哪个网关」。调度器按平台做精确匹配：`NormalizeOpenAICompatiblePlatform` 对每个平台返回自身，账号池才会按平台隔离（`backend/internal/service/openai_gateway_scheduling.go:294-302`）。TypeSafe 是**另一个厂商的域名 + 另一套 key**，所以它必须有自己的平台标签。

对照：`embeddings`（本分支 `backend/internal/server/routes/gateway.go:292-304`）与 `rerank`（本分支树里没有，见集成分支上的 `feature/openai-rerank-endpoint`；`integration:backend/internal/handler/endpoint.go:22` 有 `EndpointRerank`）之所以挂在 `openai` 平台下，是因为它们是**同一批 openai 系账号上的能力位**，用账号凭据里的 `credentials["openai_capabilities"]` 表达（键名常量 `backend/internal/service/account.go:109`；能力判定 `backend/internal/service/account.go:1850-1920`）。能力位的前提是「同一个上游厂商」，对 TypeSafe 不成立。

---

## 2. 改了什么（按区域）

### 域名常量 / 谓词 / 凭据读取

- `backend/internal/service/domain_constants.go:90-93` — 新增 `DefaultTypeSafeBaseURL = "https://api.typesafe.ai"` 与 `DefaultTypeSafeTestModel = "jev-latest"`。
- `backend/internal/service/domain_constants.go:124-129` — `IsTypeSafe(platform)`；注释明确 typesafe **刻意不纳入** `IsMultiProtocolAPIKeyProvider`（`backend/internal/service/domain_constants.go:131-135`）。
- `backend/internal/service/account.go:297-302` — `(*Account).IsTypeSafe()`。
- `backend/internal/service/account.go:983-999` — `GetBaseURL()` 对 typesafe **早退返回空串**（Anthropic 协议路径 fail-closed，不再回落 `https://api.anthropic.com`）。
- `backend/internal/service/account.go:1370-1410` — `GetOpenAIBaseURL()` 纳入 typesafe，缺失 `credentials["base_url"]` 时回落 `DefaultTypeSafeBaseURL`。
- `backend/internal/service/account.go:1779-1788` — `GetOpenAIProtocolAPIKey()` 覆盖 typesafe（`credentials["api_key"]`）。
- `backend/internal/domain/constants.go:34-36` — domain 层常量。

### 迁移

- `backend/migrations/240_typesafe_platform.sql` — 只放宽 `user_platform_quotas.platform` 的 CHECK 约束，写成「main 与 integration 已知平台并集 + typesafe」（显式含 `ollama_cloud`），可重入、零 DML。
- `backend/ent/schema/user_platform_quota.go:42-47` — ent 构建期 `Validate` 白名单同步加 `typesafe`。

### 路由门禁与对话端点拒绝

- `backend/internal/server/routes/gateway.go:59-84` — `isTypeSafeGatewayPlatform` 与 `rejectTypeSafeConversationalEndpoint`（显式 404，文案提示 `POST /v1/systemone` 是唯一可用端点）。
- 调用点：`backend/internal/server/routes/gateway.go:86`（count_tokens）、`:217`（Responses WebSocket ingress）、`:237`（messages）、`:260`/`:270`（responses POST 与其 subpath 别名）、`:283`（chat/completions）、`:424`（根路径/codex 的 responses 别名）、`:463`（根路径 chat/completions）。
- `backend/internal/server/routes/gateway.go:308-321` — `/v1/systemone` 的独立平台门（非 typesafe 分组 404）。

### 端点 handler / service

- `backend/internal/handler/openai_systemone.go` — handler（鉴权、只校验 `model` 必填、调度、计费、失败切换）。
- `backend/internal/service/openai_systemone.go` — 透传与错误体净化。
- `backend/internal/handler/endpoint.go:22`、`:94-95`、`:240-242` — `EndpointSystemOne` 常量、路径识别、typesafe 入站端点即上游端点。

### 端点常量与路由覆盖测试

- `backend/internal/handler/endpoint_test.go:29`、`:144-147` — 端点识别与上游端点映射。
- `backend/internal/server/routes/prompt_audit_route_coverage_test.go:37` — `/systemone` 路由覆盖登记。

### admin 测试连接修复

- `backend/internal/service/account_test_service.go:52-57` — typesafe 最小良性探测体常量。
- `backend/internal/service/account_test_service.go:400-402` — 分派到 typesafe 专用探测。
- `backend/internal/service/account_test_service.go:457-512` — `testTypeSafeAccountConnection`：用 `GetOpenAIBaseURL()` + `buildOpenAISystemOneURL` 打到 `/v1/systemone`，`Authorization: Bearer <key>`，不再走 Anthropic 形状的 `/v1/messages?beta=true` 探测。

### 前端面

- **建号入口** `frontend/src/components/account/CreateAccountModal.vue:232-245`（平台卡片）、`:4295-4302`（`selectTypeSafePlatform`：设 `type='apikey'`、`accountCategory='apikey'`、base_url 预设）。
- **base_url 兜底** `frontend/src/components/account/credentialsBuilder.ts:255-280` — `TYPESAFE_BASE_URL = 'https://api.typesafe.ai'` 与 `defaultApiKeyBaseUrlForPlatform` 的 typesafe 分支。
- **平台色表 / 图标** `frontend/src/utils/platformColors.ts`（新增 16 处，含 `getPlatformLabel`）、`frontend/src/components/common/PlatformIcon.vue:57-58`。
- **分组平台名** `frontend/src/constants/platforms.ts:24`（`CONCRETE_PLATFORM_OPTIONS`，`GROUP_PLATFORM_OPTIONS:28-31` 自动inherit）、`frontend/src/types/index.ts:541`（`GroupPlatform`）、`:921`（`AccountPlatform`）。
- **渠道定价列表** `frontend/src/views/admin/ChannelsView.vue:766` — `platformOrder` 加 `typesafe`；`:769` 的 `compositePlatforms` **不加**（typesafe 不是 composite 可路由的对话上游，见 `frontend/src/views/admin/GroupsView.vue:4617-4620`）。
- 其他：`frontend/src/components/keys/UseKeyModal.vue:1255`、`frontend/src/utils/keyGroupProviders.ts:20`、`frontend/src/i18n/locales/{en,zh}/admin/accounts.ts`（各 1 行）、`frontend/src/i18n/locales/{en,zh}/admin/overview.ts`（各 1 行）、`frontend/src/composables/useModelWhitelist.ts:470-471`、`frontend/src/api/admin/settings.ts`（配额平台镜像）、`frontend/src/views/admin/GroupsView.vue`。

### commit 列表（`git log --oneline main..HEAD` 的真实输出，截至 `a5769ffa4`）

```
a5769ffa4 docs(typesafe): align ws ingress test comment with current base-url behavior
b85066940 docs(typesafe): document onboarding, limitations and integration merge checklist
77688108b fix(typesafe): correct stale base-url comment in the gateway route gate
39a8d23a8 feat(typesafe): expose account onboarding and platform surfaces in admin UI
4c1a93b72 fix(typesafe): reject responses websocket ingress for typesafe groups
34727633f fix(typesafe): keep account test connection on the systemone upstream
73ced85f1 test(typesafe): cover /v1/systemone contract, routing and scheduling
f09a07449 feat(typesafe): add /v1/systemone passthrough endpoint
827ceeaf7 feat(typesafe): add typesafe platform skeleton
```

紧随 `a5769ffa4` 之后是本次文档归位提交：把本文件从 `docs/research/typesafe-platform-merge-and-ops.md` 移到 `docs/TYPESAFE_PLATFORM.md`，并在 `.gitignore` 的 `!docs/...` 白名单中登记（`b85066940` 是它上一次落库的位置，当时写在 `docs/research/` 下）。

---

## 3. 怎么上线（可执行步骤）

1. **建 typesafe 分组**：管理后台 →「分组 / Groups」→ 新建，平台下拉选 `TypeSafe`（该下拉来自 `GROUP_PLATFORM_OPTIONS`，`frontend/src/constants/platforms.ts:24-31`）。后端 `CreateGroupRequest.Platform` 的 binding 白名单已含 `typesafe`（`backend/internal/handler/admin/group_handler.go:187`）。
2. **建账号**：管理后台 →「账号 / Accounts」→ 新建，平台卡片选 `TypeSafe`（`frontend/src/components/account/CreateAccountModal.vue:232-245`）。字段：
   - 类型 / `type` = `apikey`（由 `selectTypeSafePlatform` 固定，`CreateAccountModal.vue:4299-4300`）；
   - `base_url` = `https://api.typesafe.ai`（默认值来自 `TYPESAFE_BASE_URL`，`frontend/src/components/account/credentialsBuilder.ts:261`；与后端 `DefaultTypeSafeBaseURL` 一致）；
   - `api_key` = TypeSafe 侧签发的 key。
   落库形态：`accounts.credentials = {"base_url": ..., "api_key": ...}`。
3. **模型名**：用 `jev-latest`（后端默认探测模型 `backend/internal/service/domain_constants.go:93`，审核引擎默认模型 `backend/internal/service/content_moderation_engines.go:64`）。TypeSafe 没有模型目录，模型名以账号映射 / 渠道映射 / 分组定价卡上写的为准。
4. **定价**，两条路：
   - **分组模型定价卡**：分组新建 / 编辑表单里的 `model_pricing` 列表（`frontend/src/views/admin/GroupsView.vue:1494`、`:3144`，组件 `PricingEntryCard`）。
   - **渠道按模型定价**：渠道页面顶部按平台切换的标签页由 `platformOrder` 驱动（`frontend/src/views/admin/ChannelsView.vue:236`、`:766`），在渠道表单里加 pricing rule（`ChannelsView.vue:446-450`）。**本分支这条已可用**（`platformOrder` 已含 typesafe）；**在 integration 上需要合并后把 typesafe 加进 `platformOrder`**（integration 当前是 `[..., 'opencode_go', 'ollama_cloud']`，`integration:frontend/src/views/admin/ChannelsView.vue:766`）。
   - 注意：渠道里的「从 LiteLLM 目录同步模型」按钮**不支持 typesafe** —— `platformToLiteLLMProvider`（`backend/internal/handler/admin/channel_handler.go:637-648`）没有 typesafe，`SyncPricingModels` 会返回 `400 UNSUPPORTED_PLATFORM`（同文件 `:650-670`）。所以 typesafe 的定价只能手填。
   - 具体界面字段名如与本文不符，**以当前界面为准**。
5. **建绑定该分组的 API Key**：管理后台 →「API Keys」→ 新建，分组选第 1 步的 typesafe 分组。该分组的平台标签决定 `/v1/systemone` 的平台门是否放行。
6. **客户端调用**：`POST /v1/systemone`。请求形状见第 1 节表格与第 6 节的 curl 示例。

---

## 4. 行为与边界（均有代码依据）

- **同步、无流式。** 透传结果固定 `Stream: false`（`backend/internal/service/openai_systemone.go:193`）；服务层注释明确「无流式、不生成文本」（同文件 `:21-23`）。请求体只解析 `model`，`state` / `questions` 逐字节透传（`backend/internal/handler/openai_systemone.go:72-77`）。
- **`/v1/models` 对 typesafe 有意返回空清单。** `backend/internal/handler/gateway_handler.go:1188-1193`（`writeModelsList(c, platform, nil)`，注释说明「绝不回落 `claude.DefaultModels`」）；同链路 `defaultModelIDsForPlatform` 在 `:1463-1465` 返回 `nil`；分组可用模型解析 `backend/internal/service/admin_group.go:299-302` 同样返回 `nil`。前端白名单同步返回空（`frontend/src/composables/useModelWhitelist.ts:470-471`）。
- **对话端点对 typesafe 分组显式 404。** `/v1/messages`、`/v1/messages/count_tokens`、`/v1/chat/completions`、`/v1/responses`（POST 与 `/v1/responses/*subpath`）、`GET /v1/responses`（Responses WebSocket ingress），以及不带 `/v1` 前缀的别名与 `/backend-api/codex/responses`，统一由 `rejectTypeSafeConversationalEndpoint` 在入口拒绝（`backend/internal/server/routes/gateway.go:72-84` 及第 2 节列出的 8 个调用点），响应体为自造的 `not_found_error`，message 形如 `<Api> is not supported for TypeSafe groups; POST /v1/systemone is the only available endpoint`（`gateway.go:78-80`）。
  门禁**现在**存在的理由：typesafe 分组没有对话端点，通用 Anthropic 网关按 platform 过滤后仍可能选中 typesafe 账号，不拦就会落到通用网关 / 上游并返回语义不清的错误；这里改为干净的显式 404。曾经担心的「把 typesafe 的 key 当 Anthropic key 发到 `https://api.anthropic.com`」已由 `GetBaseURL()` 对 typesafe 早退返回空串（`backend/internal/service/account.go:997-999`）在 base URL 层关闭。
- **上游错误体绝不透传。** 凡上游 `status >= 400`，只回自造错误体（`{"error":{"type":"upstream_error","message":"Upstream returned status N (upstream request id: ...)"}}`），message 只带上游状态码与上游 request id：`backend/internal/service/openai_systemone.go:126-174`（含 failover 分支把 `failoverErr.ResponseBody` 覆盖为自造体，`:167-169`）、`:219-254`（`writeOpenAISystemOneError` / `openAISystemOneUpstreamErrorMessage` / `buildOpenAISystemOneUpstreamErrorBody`）。原因写在服务层注释（同文件 `:25-29`）：TypeSafe 错误响应可能回显请求内容甚至凭据，而出站 `Authorization` 用的是平台账号的 key；上游 body 只用于内部判定（failover 分类 / 熔断 / ops 事件）。同类注释也在 `backend/internal/pkg/typesafe/client.go:61-63`（「Do not log provider error bodies: they may echo user input or credentials」）。
- **`openai_capabilities` 白名单不作用于 `/v1/systemone`。** handler 调用 `SelectAccountWithSchedulerForCapability` 时 `requiredCapability` 传空串（`backend/internal/handler/openai_systemone.go:126-142`），而 `SupportsOpenAIEndpointCapability("")` 恒真（`backend/internal/service/account.go:1859-1861`），能力过滤链（`backend/internal/service/openai_ws_forwarder_support.go:577`）因此被跳过。调度隔离由平台门 + 平台归一保证：`NormalizeOpenAICompatiblePlatform` 对 typesafe 返回自身（`backend/internal/service/openai_gateway_scheduling.go:294-302`）。
- **WS ingress 另有独立拒绝路径。** typesafe 账号在 WSv2 ingress 模式解析上恒为 `off`（非 openai 平台），因此 `isOpenAIAccountTransportCompatible` 判定其与 ingress transport 不兼容（钉在 `backend/internal/service/openai_ws_ingress_typesafe_test.go:10-33`）。

### 其他入口的现状（未逐一加 typesafe 断言，行为由既有门禁决定）

| 入口 | typesafe 分组的行为 | 依据 |
| --- | --- | --- |
| `GET /v1/models/:model` | **返回空清单，不是 404**（该路由直接挂 `h.Gateway.Models`，与 `GET /v1/models` 同一处理器，且不读 `:model`） | 路由 `backend/internal/server/routes/gateway.go:254`、`:448`；处理器 `backend/internal/handler/gateway_handler.go:1188-1193` |
| `POST /v1/live`、`GET /v1/live/:call_id`、`POST /backend-api/codex/realtime/calls`、`GET /backend-api/codex/:call_id` | 404 `Live is not supported for this platform`（handler 内部门禁只放行 openai / composite）；sideband 另有 `liveEnabledForAPIKey` 门 | `backend/internal/handler/openai_live.go:33-35`、`:245-250` |
| `POST /v1/alpha/search`、`POST /alpha/search`、`POST /backend-api/codex/alpha/search` | 404 `Codex alpha search is only available for OpenAI and Composite groups` | `backend/internal/handler/openai_alpha_search.go:33-35` |
| `POST /v1/embeddings`、`POST /embeddings` | 404 `Embeddings API is not supported for this platform`（路由级门 `isOpenAIOnlyEndpointGatewayPlatform` == openai） | `backend/internal/server/routes/gateway.go:292-304`、`:472-483` |

---

## 5. 两个 key 存放处（重要运维提示）

TypeSafe 的 key 在本仓库有**两套互不相通**的存储，**当前不共享、不迁移**：

1. **内容审核引擎的 TypeSafe 配置**（设置里的 engine profile）
   - 存储：`settings` 表的 `content_moderation_config`（JSON）。键名常量 `backend/internal/service/domain_constants.go:255`（`SettingKeyContentModerationConfig = "content_moderation_config"`）。
   - 结构：`engine` + `engine_configs.typesafe{base_url, model, api_keys, timeout_ms, retry_count, thresholds}`，见 `backend/internal/service/content_moderation_engines.go:13-25`、`:46-68`；`api_keys` 在服务端以哈希标识用于删除匹配（`backend/internal/service/content_moderation.go:2580-2586`、`:2908-2925`）。
   - 控制台入口：风控中心 / Risk Control 页面（`frontend/src/views/admin/RiskControlView.vue:1255-1256`、`:415-424`；类型定义 `frontend/src/api/admin/riskControl.ts:4`、`:20-21`）。
   - 用途：审核链路的 `typesafe.Evaluate` 直连（`backend/internal/service/content_moderation_typesafe.go:60-75`），**不经** `/v1/systemone` 网关端点。
2. **平台账号的 `credentials`**
   - 存储：`accounts.credentials` JSONB，键 `base_url` + `api_key`；读取路径 `backend/internal/service/account.go:1779-1788`（`GetOpenAIProtocolAPIKey`）与 `:1370-1410`（`GetOpenAIBaseURL`）。
   - 用途：`POST /v1/systemone` 转发鉴权（`backend/internal/service/openai_systemone.go:61-71`）。

**运维结论**：如果审核引擎与平台账号都在用 TypeSafe，**轮换 key 必须两边都换**。两处配置在代码里没有任何同步逻辑，改一处不影响另一处。

---

## 6. 已知限制与未验证项

- **没有对真实 TypeSafe 上游做过端到端调用。** 本环境**没有** `TYPESAFE_API_KEY` / `TYPESAFE_LIVE_TEST`（grep 全仓库只有下面那一个 opt-in 测试读这两个变量）。现有证据是 httptest 契约测试 + 路由级测试 + 调度隔离测试（见第 8 节）。
- **仓库里既有的 live 测试入口（覆盖的是审核引擎，不是网关端点）：**
  - 测试：`TestContentModerationTypeSafeLive`，`backend/internal/service/content_moderation_typesafe_live_test.go:15-36`。
  - 开关：`TYPESAFE_LIVE_TEST=1` + `TYPESAFE_API_KEY=<真实 key>`（同文件 `:16-20`）。默认 `t.Skip`。
  - 调用路径：`typesafe.Evaluate(ctx, http.DefaultClient, "https://api.typesafe.ai", key, ...)`（同文件 `:28`），即**直连上游**，绕开 `/v1/systemone`。它验证「TypeSafe 上游可达、协议与 13 条 noul 判定可用」，**不验证**本分支新增的网关端点、账号调度、计费或错误体净化。
- **网关端点目前没有 live 测试**，需要人工用 curl 跑一次。示例（key 用占位符，**不要把真实凭据写进命令历史或文档**）：

  ```bash
  curl -sS -X POST "https://<your-sub2api-host>/v1/systemone" \
    -H "Authorization: Bearer <SUB2API_API_KEY>" \
    -H "Content-Type: application/json" \
    -d '{
      "model": "jev-latest",
      "state": "请帮我写一个 Go 函数，对整数数组排序。",
      "questions": {
        "harassment": {
          "type": "noul",
          "instructions": "文本是否包含针对个人或群体的辱骂、贬损或骚扰？"
        }
      }
    }'
  ```

  期望：HTTP 200，响应 `{"model": "...", "usage": {"input_tokens": N, "output_tokens": M}, "answers": {"harassment": {"type": "noul", "noul": 0.x}}}`（`questions` 是对象 map、`answers` 按 question id 回填，见 `backend/internal/pkg/typesafe/client.go:18-24`、`:66-74`）。失败时若返回 `upstream_error`，message 只会带状态码与上游 request id，不带上游 body —— 这是设计行为，不是 bug。
- **`/v1/models/:model` 返回空清单而非 404**（第 4 节表格）。需要 404 的客户端不要依赖该路径判断 typesafe 分组的可用性。
- **`/v1/live*`、`/alpha/search`、`/embeddings`** 现状见第 4 节表格；这几条**没有** typesafe 专属测试钉住，它们的 404 来自既有平台门，**未在本次改动中回归验证**。
- **未验证**：`POST /v1/systemone` 在真实上游下的错误码分布、超时表现、以及上游是否真的在错误体里回显请求内容 —— 只按上游文档与代码注释做了防护性设计，没有实测样本。

---

## 7. 合并 integration 的人工清单

> **本节是本 fork 的 integration 工作流专属，提上游 PR 时可删除。**
> 背景：本分支基于本地 `main`，而 `integration` 上已经并入了 `feature/composite-ollama-unified`（`ollama_cloud` 平台）等分支。两边都在往同一批「平台列表」里追加，必须做**并集**而不是取一边。

1. **`QUOTA_PLATFORMS` 加 `typesafe`**。该常量只存在于 integration：`integration:frontend/src/api/admin/settings.ts:25-37`（当前 `[..., 'opencode_go', 'ollama_cloud']`），`PlatformType` 由它派生（`:39`）。本分支同名位置叫 `PLATFORMS`（本分支 `frontend/src/api/admin/settings.ts:38`）。合并时以 integration 的 `QUOTA_PLATFORMS` 为单一来源，补 `"typesafe"`。它必须与后端 `service.AllowedQuotaPlatforms` 一致，否则用户平台配额是整体替换语义、保存时会静默丢行（本分支 `settings.ts:20-24` 的注释）。
2. **`CONCRETE_PLATFORM_OPTIONS` 需同时含 `ollama_cloud` 与 `typesafe`**：`frontend/src/constants/platforms.ts:13-25`。integration 当前该数组有 `ollama_cloud`（`integration:frontend/src/constants/platforms.ts:24`），本分支有 `typesafe`（本分支同文件 `:24`）。
3. **「双方各自追加」的并集文件清单**（每处两边各加了一行/一项，直接冲突或需并集）：
   - `frontend/src/types/index.ts`：`GroupPlatform`（本分支 `:541`）与 `AccountPlatform`（本分支 `:921`）。
   - `frontend/src/utils/keyGroupProviders.ts`：`PROVIDER_BY_PLATFORM`（本分支 `:20` 加 `typesafe: 'other'`；integration `:20` 加 `ollama_cloud: 'other'`）。
   - `frontend/src/components/keys/UseKeyModal.vue`：平台显示名表（本分支 `:1255`；integration `:1255` 加 `ollama_cloud`）。
   - `frontend/src/i18n/locales/{en,zh}/admin/accounts.ts`：平台名（本分支各 1 行；integration `en:112`、`zh:330` 加 `ollama_cloud`）。
   - `frontend/src/constants/__tests__/platforms.spec.ts`：期望的平台值数组（本分支 `:15` 加 `'typesafe'`；integration `:15` 加 `'ollama_cloud'`）。
4. **`platformColors.ts` 的 16 处全表并入**：本分支在 `frontend/src/utils/platformColors.ts` 追加 16 行（平台联合类型 + 15 张色表，行号 `:19,34,51,67,84,102,119,136,153,170,187,204,221,237,256,325`）；integration 同样有 16 处 `ollama_cloud`。两边逐表并入，不要覆盖任一边。
5. **`ChannelsView.vue` 两个数组合并后仍应不同**（`frontend/src/views/admin/ChannelsView.vue:766`、`:769`）：
   - `platformOrder` **含** `typesafe`（渠道定价/映射的标签页要能选到它）；
   - `compositePlatforms` **不含** `typesafe`（composite 白名单不接受它，`frontend/src/views/admin/GroupsView.vue:4617-4620`）；
   - 两边合并后 integration 版本应是「`ollama_cloud` 与 `typesafe` 都在 `platformOrder`，而 `compositePlatforms` 只拿到 integration 原有的 `ollama_cloud`、**不含** `typesafe`」（integration 现状两数组都含 `ollama_cloud`，见 `integration:frontend/src/views/admin/ChannelsView.vue:766`、`:769`）。
6. **后端列表并存**：
   - `service.AllowedQuotaPlatforms`：本分支 `backend/internal/service/domain_constants.go:141-153`（含 `PlatformTypeSafe`），integration 同位置含 `PlatformOllamaCloud`（`integration:backend/internal/service/domain_constants.go:141-153`）→ 合并后两者都要有。
   - `service.IsMultiProtocolAPIKeyProvider`：本分支 `backend/internal/service/domain_constants.go:131-135`（`IsCNProvider || OpenCodeGo`，**刻意不含 typesafe**），integration 是 `IsCNProvider || OpenCodeGo || IsOllamaCloud`（`integration:backend/internal/service/domain_constants.go:134-136`）→ 合并后保留 integration 的 `IsOllamaCloud` 项，同时**不要**把 typesafe 加进去。
   - `ent/schema/user_platform_quota.go` 的构建期 `Validate` 白名单同样要并集（本分支 `:42-47` 已加 `typesafe`；integration 加 `ollama_cloud`）。
7. **迁移顺序无关**：`backend/migrations/240_typesafe_platform.sql` 的 CHECK 已写成「main 与 integration 已知平台并集 + typesafe」（含 `ollama_cloud`），与 integration 上的 `239_ollama_cloud_platform.sql` 谁先应用都成立；合并后不需要再改这个迁移（也不要改它 —— 迁移一旦应用不可修改）。

---

## 8. 验证证据

### 关键测试文件与测试名（均从仓库 grep 到的真实名字）

| 层 | 文件 | 测试名 |
| --- | --- | --- |
| 路由 / 门禁 | `backend/internal/server/routes/gateway_typesafe_test.go` | `TestGatewayRoutesTypeSafeGroupCanReachSystemone`、`TestGatewayRoutesSystemoneIsRegistered`、`TestGatewayRoutesSystemoneRejectedForOtherPlatforms`、`TestGatewayRoutesTypeSafeGroupCannotReachConversationalEndpoints`、`TestGatewayRoutesOtherPlatformsKeepConversationalEndpoints`、`TestGatewayRoutesTypeSafeGroupCannotReachResponsesWebSocketIngress`、`TestGatewayRoutesOtherPlatformsKeepResponsesWebSocketIngress` |
| 端点契约 | `backend/internal/service/openai_systemone_test.go` | `TestBuildOpenAISystemOneURL`、`TestExtractOpenAISystemOneUsage`、`TestForwardSystemOne_APIKeyPassthroughRecordsUsage`、`TestForwardSystemOne_UnmappedBodyPassesThroughUnchanged`、`TestForwardSystemOne_DefaultBaseURLDoesNotFallBackToOpenAI`、`TestForwardSystemOne_UpstreamErrorDoesNotLeakUpstreamBody`、`TestForwardSystemOne_FailoverErrorCarriesNoUpstreamBody` |
| 调度隔离 | `backend/internal/service/typesafe_scheduling_test.go` | `TestNormalizeOpenAICompatiblePlatform_TypeSafeKeepsOwnValue`、`TestSelectAccountWithSchedulerForCapability_TypeSafePlatformIsExactMatch`、`TestSelectAccountWithSchedulerForCapability_TypeSafePoolIsNotShared` |
| WS ingress | `backend/internal/service/openai_ws_ingress_typesafe_test.go`、`backend/internal/handler/openai_ws_ingress_typesafe_test.go` | `TestOpenAIWSIngressTransportRejectsTypeSafeAccount`、`TestOpenAIResponsesWebSocket_TypeSafeGroupSelectsNoAccountAndCallsNoUpstream`、`TestOpenAIResponsesWebSocket_OpenAIAccountStillDialsUpstream` |
| base URL | `backend/internal/service/account_base_url_test.go` | `TestGetBaseURL`、`TestGetBaseURLTypeSafeKeepsUpstreamOnTypeSafeHost` |
| 测试连接 | `backend/internal/service/account_test_service_typesafe_test.go` | `TestAccountTestService_TypeSafeConnectionProbesSystemOneWithBearerKey`、`TestAccountTestService_TypeSafeConnectionWithoutBaseURLNeverTargetsAnthropic`、`TestAccountTestService_NonTypeSafeConnectionStillUsesAnthropicProbe` |
| 端点常量 | `backend/internal/handler/endpoint_test.go` | `TestNormalizeInboundEndpoint`、`TestDeriveUpstreamEndpoint`（含 `typesafe systemone` / `openai family systemone stays native` 子用例） |
| 路由覆盖 | `backend/internal/server/routes/prompt_audit_route_coverage_test.go` | `TestEveryGatewayPOSTRouteIsClassifiedForPromptAuditCoverage`（`/systemone` 登记） |
| 迁移 | `backend/migrations/typesafe_platform_migration_test.go` | `TestTypeSafePlatformMigration`、`TestTypeSafePlatformMigrationIsStrictSupersetOf238` |
| 分组平台白名单 | `backend/internal/handler/admin/group_handler_platform_test.go` | `TestGroupPlatformBinding_AllowedPlatforms`、`TestGroupPlatformBinding_RejectsInvalidPlatforms` |
| 前端平台目录 | `frontend/src/constants/__tests__/platforms.spec.ts` | `platform option catalogs > exposes every concrete account platform` |
| 前端配额平台 | `frontend/src/api/__tests__/settings.authSourceDefaults.spec.ts` | `normalizePlatformQuotasMap > 无参数时返回全 6 平台全 null`、`sanitizePlatformQuotasMap > 缺失平台填充为全 null` |
| 审核引擎（真实调用入口） | `backend/internal/service/content_moderation_typesafe_live_test.go` | `TestContentModerationTypeSafeLive`（opt-in，见第 6 节） |

### 验收命令

```bash
# 后端：单测（unit build tag）、构建、格式
cd backend && go test -tags unit ./internal/... ./migrations/...
cd backend && go build ./... && gofmt -l internal/ migrations/

# 前端：类型检查与单测（必须带 --config.verify-deps-before-run=false）
cd frontend && pnpm --config.verify-deps-before-run=false typecheck
cd frontend && pnpm --config.verify-deps-before-run=false exec vitest run
```

本环境实测（本轮）：`go build ./...` 通过；`gofmt -l internal/server/routes/gateway.go` 无输出；`go test -tags unit ./internal/server/routes/` `ok`；`pnpm --config.verify-deps-before-run=false typecheck` 通过；`pnpm --config.verify-deps-before-run=false exec vitest run` `299 passed (299) / 2266 passed (2266)`，且**没有**改动 `frontend/pnpm-lock.yaml`。

### 本环境两个既存故障（与本改动无关，但会挡住上面的命令）

1. **`go generate ./...` 的 wire go.sum 问题。** `backend/cmd/server/main.go:3` 的 `//go:generate go run github.com/google/wire/cmd/wire`（注意这条**没有** `-mod=mod`）在本环境直接失败：

   ```
   .../wire@v0.7.0/cmd/wire/main.go:34:2: missing go.sum entry for module providing package
   github.com/google/subcommands (imported by github.com/google/wire/cmd/wire); to add:
       go get github.com/google/wire/cmd/wire@v0.7.0
   ```

   同一命令在 `backend/cmd/server/wire_gen.go:3` 上是带 `-mod=mod` 的版本。**不要**为了让 `go generate` 跑通而修改 `go.mod` / `go.sum`。
2. **裸 `pnpm typecheck` 的 pnpm 11 问题（实测会破坏 lockfile）。** 本环境 pnpm 版本 `11.7.0`，它已不再读取 `package.json` 的 `pnpm` 字段（`frontend/package.json:63-70` 的 `pnpm.overrides`），并会打印：

   ```
   [WARN] The "pnpm" field in package.json is no longer read by pnpm. The following keys were ignored: "pnpm.overrides".
   ```

   裸跑 `pnpm typecheck` 会先隐式执行依赖校验/安装，实测**删掉了 `frontend/pnpm-lock.yaml:7-11` 的安全 overrides 块**（`js-cookie` / `form-data` / `postcss` / `dompurify`）并把 `postcss` 的 `specifier` 从 `'>=8.5.18'` 改回 `^8.4.32`。因此：

   - 所有前端命令都要带 `--config.verify-deps-before-run=false`；
   - **禁止 `pnpm install`** —— 同样会删掉 lockfile 里的安全 overrides 块（该块只存在于 `pnpm-lock.yaml`，`package.json` 里的副本对 pnpm 11 已失效）。
