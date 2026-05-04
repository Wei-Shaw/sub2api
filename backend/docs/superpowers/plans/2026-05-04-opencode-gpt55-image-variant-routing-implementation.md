# OpenCode GPT-5.5 Image 变体与路由实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。本仓库全局规则高于计划模板：除非用户在当前会话明确要求，不创建 git commit。

**目标：** 让普通 GPT-5.5 默认只启用 `web_search`，只有新 carrier 格式显式启用 `image_generation.enabled === true` 时才注入 OpenAI Responses 生图工具，并把请求路由到同时支持主模型与 `gpt-image-2` 的账号；同时在 OpenCode 推荐配置里提供 Image 变体和 `agent.image`。

**架构：** 后端把本地 `builtin_tools` carrier 解析收敛到同一组 helper：先用 `enabled:true + model:gpt-image-2` 门槛决定是否生成最终 Responses `image_generation` tool，再由 handler 在调度前基于“最终有效 Responses tools”构造 `OpenAIResponsesImageGenerationRequirement`。调度器用独立 requirement 过滤 sticky、previous-response、load-balance、projection、fresh DB recheck 和 failover，不复用 `/v1/images/*` 的 native/basic fallback 语义。前端 OpenCode 配置把 image 从普通 GPT-5.5 默认 options 移到 `variants.image`，并新增专门的 `agent.image`。

**技术栈：** Go 1.25.7、Gin、`tidwall/gjson/sjson`、`testify/require`、Vue 3、Vitest、OpenCode JSON config。

---

## 参考输入

- 规格：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\backend\docs\superpowers\specs\2026-05-02-opencode-gpt55-image-variant-routing-design.md`
- 计划：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\backend\docs\superpowers\plans\2026-05-04-opencode-gpt55-image-variant-routing-implementation.md`
- 实施 worktree 目录：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\gpt55-image-routing`
- Go 命令：`C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe`
- 前端目录：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\frontend`

---

## 文件职责

**后端 carrier 与转发**
- 修改：`backend/internal/service/openai_builtin_tools.go`：收紧 `image_generation` carrier 解析；新增 image config 门槛、字段白名单、`input_fidelity` 支持；旧 image 形态 strip-only。
- 修改：`backend/internal/service/openai_builtin_tools_test.go`：锁定 `enabled:true` 门槛、旧 image 失效、`web_search` 兼容、字段白名单。
- 修改：`backend/internal/service/openai_gateway_service.go`：复用最终工具合并逻辑；清理不可用的 `tool_choice.image_generation`；提供调度前 requirement 派生 helper；保持 compact/passthrough strip-only。
- 修改：`backend/internal/service/openai_gateway_service_test.go`：覆盖 Responses 正常转发、旧 carrier、双 carrier、tool_choice 清理、compact/passthrough 边界、Chat Completions 转发后的工具形态。
- 修改：`backend/internal/service/openai_gateway_chat_completions.go`：让 Chat Completions compat carrier 使用同一 image 门槛；正常 Chat Completions 转 Responses 后能追加 image tool；responses-shape 分支仍复用 Responses transform。
- 修改：`backend/internal/pkg/apicompat/types.go`：给 `ResponsesTool` 增加 image built-in 允许字段，避免 Chat Completions 转换路径丢失 `model`、`output_format` 等属性。
- 修改：`backend/internal/pkg/apicompat/chatcompletions_to_responses.go`：允许 `ChatTool{Type:"image_generation"}` 或 service 后置追加的 Responses tool 被保留；不要把 image tool 转成 function。

**后端调度与账号能力**
- 修改：`backend/internal/service/openai_account_scheduler.go`：新增 `OpenAIResponsesImageGenerationRequirement` 和 `OpenAIAccountScheduleRequest.RequiredResponsesImageGeneration`；在 sticky、previous-response、load-balance、projection、fresh recheck、failover 路径统一过滤。
- 修改：`backend/internal/service/account.go`：新增 `SupportsOpenAIResponsesImageGeneration(mainModel, imageModel string) bool`；要求主模型按现有规则支持，`gpt-image-2` 只接受精确或 image-specific 正向证据；OAuth `plan_type:"free"` 与缺失层级 fail-closed。
- 修改：`backend/internal/service/openai_model_subset_projection.go`：暴露或复用 image-specific wildcard 判断；确保宽泛 `*`、`gpt-*`、default allow 不能证明 `gpt-image-2`。
- 修改：`backend/internal/service/openai_account_scheduler_test.go`：覆盖免费账号、付费账号、API Key、sticky 清理、previous-response 跳过、projection/fresh recheck/failover。
- 修改：`backend/internal/service/openai_images_test.go`：保留 `/v1/images/*` 原有 native/basic fallback 断言，防止 Responses built-in 约束污染旧路径。

**后端 handler**
- 修改：`backend/internal/handler/openai_gateway_handler.go`：扩展 `buildOpenAIAccountScheduleRequest` 参数，把 requirement 传入 scheduler；Responses、Chat Completions、Responses WebSocket v2 首轮调度都基于最终有效 tools 生成 requirement；image-aware no-available 错误使用主模型和 `gpt-image-2`。
- 修改：`backend/internal/handler/openai_chat_completions.go`：在默认模型 fallback 前判断 image requirement；有 image requirement 时禁止把主模型替换成 group default，或确保 requirement 的 `MainModel` 保留用户原始规范化模型。
- 修改：`backend/internal/handler/openai_gateway_handler_test.go`：捕获调度请求，断言 Responses/Chat Completions/native tools/旧 carrier/default fallback 的 requirement 语义。

**前端 OpenCode 推荐配置**
- 修改：`frontend/src/components/keys/UseKeyModal.vue`：普通 GPT-5.5 默认只写 `{ web_search: true }`；新增 `openCodeImageGenerationBuiltinTools()`、`openCodeImageVariant()`；给 GPT-5.5 系列含 `-Sys` 派生注入 `variants.image`；新增 `agent.image`，保留 `store:false`。
- 修改：`frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`：断言普通 GPT-5.5 不含 image、image variant 存在、fast id override 保留、reasoning variants 保留、base 与 `-Sys` variants 不共享对象、`agent.image` 正确。

---

## 任务 0：建立隔离 worktree

**文件：**
- 读取：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.gitignore`
- 使用：`C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\gpt55-image-routing`

- [ ] **步骤 0.1：验证项目本地 worktree 目录已被忽略**

运行：

```powershell
rtk git check-ignore -q .worktrees
```

预期：exit 0。若 exit 非 0，先在仓库根 `.gitignore` 增加 `.worktrees/`，再运行 `rtk git diff --check -- .gitignore`；不要创建 commit。

- [ ] **步骤 0.2：创建新 worktree**

运行：

```powershell
rtk git worktree add ".worktrees/gpt55-image-routing" -b "feature/gpt55-image-routing"
```

预期：输出包含 `Preparing worktree`，并创建 `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\gpt55-image-routing`。

- [ ] **步骤 0.3：把已审查设计文档和本计划复制到 worktree**

运行：

```powershell
Copy-Item "backend/docs/superpowers/specs/2026-05-02-opencode-gpt55-image-variant-routing-design.md" ".worktrees/gpt55-image-routing/backend/docs/superpowers/specs/2026-05-02-opencode-gpt55-image-variant-routing-design.md"
Copy-Item "backend/docs/superpowers/plans/2026-05-04-opencode-gpt55-image-variant-routing-implementation.md" ".worktrees/gpt55-image-routing/backend/docs/superpowers/plans/2026-05-04-opencode-gpt55-image-variant-routing-implementation.md"
```

预期：worktree 内两个路径均存在。

- [ ] **步骤 0.4：验证 worktree 基线测试可运行**

在 `C:\Users\34404\Documents\GitHub\workbench\repos\sub2api\.worktrees\gpt55-image-routing\backend` 运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run TestNormalizeOpenAIBuiltinTools -count=1
```

预期：当前基线测试通过。若基线失败，记录失败测试名与错误文本，先排查是否为环境问题；不要进入实现。

---

## 任务 1：后端 builtin_tools image 门槛

**文件：**
- 修改：`backend/internal/service/openai_builtin_tools_test.go`
- 修改：`backend/internal/service/openai_builtin_tools.go`

- [ ] **步骤 1.1：先写失败测试锁定新门槛**

在 `TestNormalizeOpenAIBuiltinTools` 中更新旧断言，并新增这些用例：

```go
{
    name: "string slice keeps only web search",
    raw:  []string{"web_search", "image_generation"},
    want: webSearch,
},
{
    name: "map image generation true is ignored",
    raw:  map[string]any{"web_search": true, "image_generation": true},
    want: webSearch,
},
{
    name: "configured image generation requires enabled true",
    raw: map[string]any{"web_search": true, "image_generation": map[string]any{
        "model":         "gpt-image-2",
        "output_format": "png",
    }},
    want: webSearch,
},
{
    name: "enabled configured image generation keeps allowed fields",
    raw: map[string]any{"image_generation": map[string]any{
        "enabled":            true,
        "model":              " GPT-IMAGE-2 ",
        "size":               "1024x1024",
        "quality":            "low",
        "output_format":      "webp",
        "output_compression": 75,
        "input_fidelity":     "high",
        "ignored":            "drop-me",
    }},
    want: []map[string]any{{
        "type":               "image_generation",
        "model":              "gpt-image-2",
        "size":               "1024x1024",
        "quality":            "low",
        "output_format":      "webp",
        "output_compression": 75,
        "input_fidelity":     "high",
    }},
},
{
    name: "enabled image generation rejects non gpt image 2 model",
    raw: map[string]any{"web_search": true, "image_generation": map[string]any{
        "enabled": true,
        "model":   "gpt-image-1",
    }},
    want: webSearch,
},
```

也保留 `true enables web search`，用于证明 `builtin_tools:true` 不隐式启用 image。

- [ ] **步骤 1.2：运行红灯测试**

在 `backend` 目录运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run TestNormalizeOpenAIBuiltinTools -count=1
```

预期：FAIL，至少旧实现会在数组或 `image_generation:true` 用例中生成 image tool。

- [ ] **步骤 1.3：实现最小 parser 改动**

在 `openai_builtin_tools.go` 中：

1. 给 `openAIImageGenerationBuiltinAllowedFields` 增加 `input_fidelity`。
2. 删除字符串、数组字符串、bool `true` 对 image 的启用语义。
3. 新增门槛 helper：

```go
func extractOpenAIImageGenerationBuiltinToolConfig(raw any) (map[string]any, bool) {
    config, ok := raw.(map[string]any)
    if !ok {
        return nil, false
    }
    enabled, ok := config["enabled"].(bool)
    if !ok || !enabled {
        return nil, false
    }
    model, ok := config["model"].(string)
    if !ok || normalizeOpenAIImageGenerationBuiltinModel(model) != openAIImageGenerationBuiltinDefaultModel {
        return nil, false
    }
    return config, true
}

func normalizeOpenAIImageGenerationBuiltinModel(model string) string {
    return strings.ToLower(strings.TrimSpace(model))
}
```

4. 在 `openAIImageGenerationBuiltinTool` 中只接受 `map[string]any{"image_generation": config}` 或 `map[string]any{"type":"image_generation", ...}` 的对象 config，且都调用 `extractOpenAIImageGenerationBuiltinToolConfig`。
5. `buildOpenAIImageGenerationBuiltinTool` 跳过本地字段 `enabled`，并把 `model` 写成规范化后的 `gpt-image-2`。

- [ ] **步骤 1.4：运行绿灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run TestNormalizeOpenAIBuiltinTools -count=1
```

预期：PASS。

---

## 任务 2：Responses request transform 与 tool_choice 清理

**文件：**
- 修改：`backend/internal/service/openai_gateway_service_test.go`
- 修改：`backend/internal/service/openai_gateway_service.go`

- [ ] **步骤 2.1：先写失败测试覆盖旧 carrier strip-only**

在 `openai_gateway_service_test.go` 的 builtin_tools 测试附近新增或替换用例：

```go
func TestForwardResponsesRequest_OldImageGenerationCarrierStripsAndDropsToolChoice(t *testing.T) {
    body := []byte(`{"model":"gpt-5.5-Sys","input":"draw","metadata":{"builtin_tools":{"web_search":true,"image_generation":true}},"tool_choice":{"type":"image_generation"}}`)
    c, _ := newOpenAITestContext(t, "/v1/responses", body)
    upstream := &stubHTTPUpstream{}
    svc := newOpenAITestGatewayService(upstream)
    account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "test-key"}}

    _, err := svc.Forward(context.Background(), c, account, body)
    require.NoError(t, err)

    upstreamBody := decodeJSONMap(t, upstream.lastBody)
    require.NotContains(t, upstreamBody, "metadata")
    require.NotContains(t, upstreamBody, "tool_choice")
    tools := upstreamBody["tools"].([]any)
    require.Len(t, tools, 1)
    require.Equal(t, "web_search", tools[0].(map[string]any)["type"])
}
```

把现有使用旧 carrier 的测试体更新为新格式：

```json
"image_generation": {"enabled": true, "model":"gpt-image-2", "output_format":"png"}
```

`TestForwardResponsesRequest_BuiltinToolsArrayAddsWebSearchAndImageGeneration` 应改成只添加 `web_search`，不添加 image。

- [ ] **步骤 2.2：运行红灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestForwardResponsesRequest_(OldImageGenerationCarrierStripsAndDropsToolChoice|BuiltinToolsArrayAddsWebSearchAndImageGeneration|OpenCodeMetadataImageGenerationCarrierAddsToolAndStripsMetadata|ImageGenerationBuiltinToolsAugmentsConfiguredTool|OAuthImageGenerationBuiltinToolChoicePreserved)" -count=1
```

预期：FAIL，旧实现会保留 image tool 或保留无效 `tool_choice`。

- [ ] **步骤 2.3：实现最终 tools 合并和 tool_choice 清理**

在 `openai_gateway_service.go` 中新增 helper，并在 `applyOpenAIBuiltinToolsAugmentation` 结束前调用：

```go
func dropUnavailableOpenAIImageGenerationToolChoice(reqBody map[string]any) bool {
    if reqBody == nil || !isOpenAIImageGenerationToolChoice(reqBody["tool_choice"]) {
        return false
    }
    tools, _ := reqBody["tools"].([]any)
    if hasOpenAIBuiltinTool(tools, "image_generation") {
        return false
    }
    delete(reqBody, "tool_choice")
    return true
}

func isOpenAIImageGenerationToolChoice(raw any) bool {
    switch value := raw.(type) {
    case string:
        return strings.TrimSpace(value) == "image_generation"
    case map[string]any:
        return strings.TrimSpace(fmt.Sprint(value["type"])) == "image_generation"
    default:
        return false
    }
}
```

`applyOpenAIBuiltinToolsAugmentation` 在 `augmented` 为空、`tools` 不是 `[]any`、或合并后都要调用该清理 helper；返回值仍表示私有 carrier 被消费或 body 改写。

- [ ] **步骤 2.4：运行绿灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestForwardResponsesRequest_(OldImageGenerationCarrierStripsAndDropsToolChoice|BuiltinToolsArrayAddsWebSearchAndImageGeneration|OpenCodeMetadataImageGenerationCarrierAddsToolAndStripsMetadata|ImageGenerationBuiltinToolsAugmentsConfiguredTool|OAuthImageGenerationBuiltinToolChoicePreserved)" -count=1
```

预期：PASS，且上游 image tool 不包含 `enabled` 或未知字段。

---

## 任务 3：调度前派生 Responses image requirement

**文件：**
- 修改：`backend/internal/service/openai_gateway_service.go`
- 修改：`backend/internal/service/openai_account_scheduler.go`
- 修改：`backend/internal/handler/openai_gateway_handler.go`
- 修改：`backend/internal/handler/openai_chat_completions.go`
- 修改：`backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **步骤 3.1：先写失败测试捕获 Responses requirement**

在 `openai_gateway_handler_test.go` 新增测试，使用 `openAIAccountSchedulerStub` 捕获 `OpenAIAccountScheduleRequest`：

```go
func TestOpenAIResponses_ImageCarrierSetsScheduleRequirement(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.5-Sys","input":"draw","metadata":{"builtin_tools":{"image_generation":{"enabled":true,"model":"gpt-image-2","output_format":"png"}}}}`))
    c.Request.Header.Set("Content-Type", "application/json")
    groupID := int64(91)
    c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 901, GroupID: &groupID, User: &service.User{ID: 1}})
    c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})

    var capturedReq service.OpenAIAccountScheduleRequest
    gatewayService := &service.OpenAIGatewayService{}
    setUnexportedFieldForTest(t, gatewayService, "openaiScheduler", &openAIAccountSchedulerStub{selectFn: func(ctx context.Context, req service.OpenAIAccountScheduleRequest) (*service.AccountSelectionResult, service.OpenAIAccountScheduleDecision, error) {
        capturedReq = req
        return nil, service.OpenAIAccountScheduleDecision{Layer: "load_balance"}, service.ErrNoAvailableAccounts
    }})
    billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimple})
    defer billingService.Stop()
    h := &OpenAIGatewayHandler{gatewayService: gatewayService, billingCacheService: billingService, apiKeyService: &service.APIKeyService{}, concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(&concurrencyCacheMock{acquireUserSlotFn: func(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) { return true, nil }}), SSEPingFormatNone, time.Second)}

    h.Responses(c)

    require.NotNil(t, capturedReq.RequiredResponsesImageGeneration)
    require.True(t, capturedReq.RequiredResponsesImageGeneration.Enabled)
    require.Equal(t, "gpt-5.5", capturedReq.RequiredResponsesImageGeneration.MainModel)
    require.Equal(t, "gpt-image-2", capturedReq.RequiredResponsesImageGeneration.ImageModel)
}
```

再新增两个相邻测试：原生 `tools:[{"type":"image_generation"}]` 也生成 requirement；旧 carrier `image_generation:true` 不生成 requirement。

- [ ] **步骤 3.2：运行红灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/handler -run "TestOpenAIResponses_.*Image.*ScheduleRequirement" -count=1
```

预期：FAIL，`RequiredResponsesImageGeneration` 字段不存在或为空。

- [ ] **步骤 3.3：新增调度 request 字段与 requirement helper**

在 `openai_account_scheduler.go` 顶部 request 附近新增：

```go
type OpenAIResponsesImageGenerationRequirement struct {
    Enabled    bool
    MainModel  string
    ImageModel string
}

func (r *OpenAIResponsesImageGenerationRequirement) normalized() *OpenAIResponsesImageGenerationRequirement {
    if r == nil || !r.Enabled {
        return nil
    }
    mainModel := NormalizeOpenAIProjectionModelKey(r.MainModel)
    imageModel := normalizeOpenAIImageGenerationBuiltinModel(r.ImageModel)
    if mainModel == "" || imageModel != openAIImageGenerationBuiltinDefaultModel {
        return nil
    }
    return &OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: mainModel, ImageModel: imageModel}
}
```

给 `OpenAIAccountScheduleRequest` 增加：

```go
RequiredResponsesImageGeneration *OpenAIResponsesImageGenerationRequirement
```

在 `openai_gateway_service.go` 新增只读派生 helper：

```go
func BuildOpenAIResponsesImageGenerationRequirementFromBody(body []byte, mainModel string, augmentBuiltin bool) *OpenAIResponsesImageGenerationRequirement {
    var reqBody map[string]any
    if len(body) == 0 || json.Unmarshal(body, &reqBody) != nil {
        return nil
    }
    if augmentBuiltin {
        applyOpenAIBuiltinToolsAugmentation(reqBody)
    }
    return BuildOpenAIResponsesImageGenerationRequirement(reqBody, mainModel)
}

func BuildOpenAIResponsesImageGenerationRequirement(reqBody map[string]any, mainModel string) *OpenAIResponsesImageGenerationRequirement {
    for _, tool := range openAIResponsesImageGenerationTools(reqBody) {
        imageModel := openAIResponsesImageGenerationToolModel(tool)
        if imageModel == "" {
            imageModel = openAIImageGenerationBuiltinDefaultModel
        }
        requirement := (&OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: mainModel, ImageModel: imageModel}).normalized()
        if requirement != nil {
            return requirement
        }
    }
    return nil
}
```

`openAIResponsesImageGenerationTools` 只读取 `reqBody["tools"].([]any)` 中 `type == "image_generation"` 的对象；缺 `model` 按 `gpt-image-2`。

- [ ] **步骤 3.4：把 requirement 传入所有 OpenAI 调度入口**

扩展 `buildOpenAIAccountScheduleRequest` 签名，在 `RequireCompact` 前后加入 `requiredResponsesImageGeneration *service.OpenAIResponsesImageGenerationRequirement`，并写入 struct。

Responses handler 在 `prepareResponsesRequestForSchedulingFn` 和 channel mapping 之后计算：

```go
requiredResponsesImage := service.BuildOpenAIResponsesImageGenerationRequirementFromBody(body, reqModel, !isOpenAIRemoteCompactPath(c))
```

Chat Completions handler 在 target group、channel mapping 之后计算：

```go
requiredResponsesImage := h.gatewayService.BuildOpenAIChatCompletionsResponsesImageGenerationRequirement(body, reqModel)
```

如果 `requiredResponsesImage != nil`，默认模型 fallback 不得把 `RequiredResponsesImageGeneration.MainModel` 换成 group default。最小策略：有 requirement 时跳过 fallback 分支，并返回 image-aware no-available 错误。

Responses WebSocket v2 首轮用 `firstMessage` 计算 requirement；第一轮选中的账号后续 turns 复用同一账号，因此不需要每 turn 重新调度。

- [ ] **步骤 3.5：运行 handler 绿灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/handler -run "TestOpenAI(Responses|ChatCompletions|ResponsesWebSocket).*Image.*(ScheduleRequirement|Fallback)" -count=1
```

预期：PASS。

---

## 任务 4：账号能力与 scheduler 过滤

**文件：**
- 修改：`backend/internal/service/account.go`
- 修改：`backend/internal/service/openai_model_subset_projection.go`
- 修改：`backend/internal/service/openai_account_scheduler.go`
- 修改：`backend/internal/service/openai_account_scheduler_test.go`
- 修改：`backend/internal/service/openai_images_test.go`

- [ ] **步骤 4.1：先写账号能力失败测试**

在 `openai_account_scheduler_test.go` 或新建相邻测试块中新增：

```go
func TestAccountSupportsOpenAIResponsesImageGenerationRequiresPositiveImageEvidence(t *testing.T) {
    makeAccount := func(accountType string, planType string, mapping map[string]any, extra map[string]any) *Account {
        creds := map[string]any{"model_mapping": mapping}
        if planType != "" {
            creds["plan_type"] = planType
        }
        return &Account{Platform: PlatformOpenAI, Type: accountType, Credentials: creds, Extra: extra}
    }

    free := makeAccount(AccountTypeOAuth, "free", map[string]any{"gpt-5.5": "gpt-5.5", "gpt-image-2": "gpt-image-2"}, nil)
    require.False(t, free.SupportsOpenAIResponsesImageGeneration("gpt-5.5", "gpt-image-2"))

    missingPlan := makeAccount(AccountTypeOAuth, "", map[string]any{"gpt-5.5": "gpt-5.5", "gpt-image-2": "gpt-image-2"}, nil)
    require.False(t, missingPlan.SupportsOpenAIResponsesImageGeneration("gpt-5.5", "gpt-image-2"))

    paidExact := makeAccount(AccountTypeOAuth, "plus", map[string]any{"gpt-5.5": "gpt-5.5", "gpt-image-2": "gpt-image-2"}, nil)
    require.True(t, paidExact.SupportsOpenAIResponsesImageGeneration("gpt-5.5", "gpt-image-2"))

    paidWide := makeAccount(AccountTypeOAuth, "plus", map[string]any{"gpt-5.5": "gpt-5.5", "gpt-*": "gpt-*"}, nil)
    require.False(t, paidWide.SupportsOpenAIResponsesImageGeneration("gpt-5.5", "gpt-image-2"))

    apiKeyImageWildcard := makeAccount(AccountTypeAPIKey, "", map[string]any{"gpt-5.5": "gpt-5.5", "gpt-image-*": "gpt-image-*"}, nil)
    require.True(t, apiKeyImageWildcard.SupportsOpenAIResponsesImageGeneration("gpt-5.5", "gpt-image-2"))

    apiKeyDefaultAllow := makeAccount(AccountTypeAPIKey, "", map[string]any{"gpt-5.5": "gpt-5.5"}, map[string]any{"openai_capability_default_allow": true})
    require.False(t, apiKeyDefaultAllow.SupportsOpenAIResponsesImageGeneration("gpt-5.5", "gpt-image-2"))
}
```

- [ ] **步骤 4.2：运行红灯能力测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run TestAccountSupportsOpenAIResponsesImageGenerationRequiresPositiveImageEvidence -count=1
```

预期：FAIL，方法不存在。

- [ ] **步骤 4.3：实现账号正向证据 helper**

在 `account.go` 中新增：

```go
func (a *Account) SupportsOpenAIResponsesImageGeneration(mainModel string, imageModel string) bool {
    if a == nil || !a.IsOpenAI() {
        return false
    }
    if !a.IsModelSupported(mainModel) {
        return false
    }
    if normalizeOpenAIImageGenerationBuiltinModel(imageModel) != openAIImageGenerationBuiltinDefaultModel {
        return false
    }
    if a.IsOpenAIOAuth() {
        planType := strings.ToLower(strings.TrimSpace(a.GetCredential("plan_type")))
        if planType == "" || planType == "free" {
            return false
        }
    }
    return a.hasExplicitOpenAIResponsesImageModelSupport(imageModel)
}
```

`hasExplicitOpenAIResponsesImageModelSupport` 的规则：

1. 精确 `model_mapping` key 或 value 规范化后等于 `gpt-image-2` 通过。
2. `model_mapping` 中 `gpt-image-*` 这类 image-specific wildcard key 或 value 匹配 `gpt-image-2` 通过。
3. `openai_capability_explicit_models` 或 `openai_capability_catalog_models` 含 `gpt-image-2` 通过。
4. `openai_capability_wildcard_rules` 中 image-specific wildcard 匹配通过。
5. `*`、`gpt-*`、空 mapping 的 default allow、group default、`openai_capability_default_allow` 均不通过。

可新增：

```go
func isOpenAIImageSpecificModelPattern(pattern string) bool {
    normalized := normalizeOpenAIProjectionPattern(pattern)
    return strings.HasPrefix(normalized, "gpt-image-")
}
```

- [ ] **步骤 4.4：先写 scheduler 失败测试**

新增 sticky 与 load-balance 用例：

```go
func TestDefaultOpenAIAccountScheduler_Select_ImageRequirementSkipsFreeStickyAndSelectsPaidImageAccount(t *testing.T) {
    ctx := context.Background()
    groupID := int64(912)
    free := Account{ID: 9121, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"plan_type":"free", "model_mapping": map[string]any{"gpt-5.5":"gpt-5.5"}}}
    paid := Account{ID: 9122, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Credentials: map[string]any{"plan_type":"plus", "model_mapping": map[string]any{"gpt-5.5":"gpt-5.5", "gpt-image-2":"gpt-image-2"}}}
    cache := &stubGatewayCache{sessionBindings: map[string]int64{"openai:image_sticky": free.ID}}
    svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{free, paid}}, cache: cache, cfg: &config.Config{}, concurrencyService: NewConcurrencyService(stubConcurrencyCache{})}
    schedulerAny := newDefaultOpenAIAccountScheduler(svc, nil)
    scheduler := schedulerAny.(*defaultOpenAIAccountScheduler)

    selection, decision, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{GroupID: &groupID, SessionHash: "image_sticky", RequestedModel: "gpt-5.5", RequiredResponsesImageGeneration: &OpenAIResponsesImageGenerationRequirement{Enabled: true, MainModel: "gpt-5.5", ImageModel: "gpt-image-2"}})

    require.NoError(t, err)
    require.NotNil(t, selection)
    require.Equal(t, paid.ID, selection.Account.ID)
    require.Equal(t, openAIStickyEvalResultMissBindingInvalid, decision.Sticky.EvalResult)
    require.Equal(t, 1, cache.deletedSessions["openai:image_sticky"])
    if selection.ReleaseFunc != nil { selection.ReleaseFunc() }
}
```

另加 no-available 用例：只有 free 或宽泛 wildcard 账号时返回 `ErrNoAvailableAccounts`。

- [ ] **步骤 4.5：运行红灯 scheduler 测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestDefaultOpenAIAccountScheduler_Select_ImageRequirement|TestAccountSupportsOpenAIResponsesImageGeneration" -count=1
```

预期：FAIL，scheduler 未检查新 requirement。

- [ ] **步骤 4.6：把 requirement 接入 scheduler 所有过滤点**

在 `openai_account_scheduler.go` 中新增统一检查：

```go
func accountSupportsOpenAIRequestRequirements(account *Account, req OpenAIAccountScheduleRequest) bool {
    if account == nil {
        return false
    }
    if req.RequestedModel != "" && !account.IsModelSupported(req.RequestedModel) {
        return false
    }
    if !account.SupportsOpenAIImageCapability(req.RequiredImageCapability) {
        return false
    }
    if requirement := req.RequiredResponsesImageGeneration.normalized(); requirement != nil {
        return account.SupportsOpenAIResponsesImageGeneration(requirement.MainModel, requirement.ImageModel)
    }
    return true
}
```

用该 helper 替换以下点位的局部检查：

- `defaultOpenAIAccountScheduler.isAccountRequestCompatible`
- sticky 命中后 `account.IsModelSupported` 与 `SupportsOpenAIImageCapability`
- fresh DB recheck 后的兼容检查
- `SelectAccountWithSchedulerRequest` 的无高级 scheduler load-awareness fallback 循环
- `shouldUseDefaultOpenAIAccountScheduler` 增加 `req.RequiredResponsesImageGeneration != nil`

当 `RequiredResponsesImageGeneration` 非空且选择失败，保留 `ErrNoAvailableAccounts`，由 handler 转换为 image-aware message。

- [ ] **步骤 4.7：运行 scheduler 绿灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestDefaultOpenAIAccountScheduler_Select_ImageRequirement|TestAccountSupportsOpenAIResponsesImageGeneration" -count=1
```

预期：PASS。

- [ ] **步骤 4.8：确认 `/v1/images/*` 行为不被污染**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestAccountSupportsOpenAIImageCapability|TestOpenAIGatewayServiceParseOpenAIImagesRequest|TestOpenAIGatewayServiceForwardImages" -count=1
```

预期：PASS。`TestAccountSupportsOpenAIImageCapability_OAuthSupportsNative` 仍可保留原断言，说明旧 `/v1/images/*` fallback 未被本轮收紧。

---

## 任务 5：Chat Completions compat image carrier

**文件：**
- 修改：`backend/internal/service/openai_gateway_service_test.go`
- 修改：`backend/internal/service/openai_gateway_chat_completions.go`
- 修改：`backend/internal/pkg/apicompat/types.go`
- 修改：`backend/internal/pkg/apicompat/chatcompletions_to_responses.go`
- 修改：`backend/internal/pkg/apicompat/chatcompletions_responses_test.go`

- [ ] **步骤 5.1：先写失败测试覆盖 Chat Completions 新 carrier**

在 `openai_gateway_service_test.go` 增加：

```go
func TestChatCompletionsBuiltinTools_ImageGenerationRequiresEnabledAndAddsResponsesTool(t *testing.T) {
    body := []byte(`{"model":"gpt-5.5","messages":[{"role":"user","content":"draw"}],"metadata":{"builtin_tools":{"web_search":true,"image_generation":{"enabled":true,"model":"gpt-image-2","output_format":"png"}}},"tool_choice":{"type":"image_generation"}}`)
    c, rec := newOpenAITestContext(t, "/v1/chat/completions", body)
    upstream := &stubHTTPUpstream{response: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-5.5","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n" + `data: [DONE]` + "\n"))}}
    svc := newOpenAITestGatewayService(upstream)
    account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key":"test-key"}}

    _, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
    require.NoError(t, err)
    require.Equal(t, http.StatusOK, rec.Code)
    upstreamBody := decodeJSONMap(t, upstream.lastBody)
    require.NotContains(t, upstreamBody, "metadata")
    tools := upstreamBody["tools"].([]any)
    require.True(t, hasOpenAIBuiltinTool(tools, "web_search"))
    require.True(t, hasOpenAIBuiltinTool(tools, "image_generation"))
    choice := upstreamBody["tool_choice"].(map[string]any)
    require.Equal(t, "image_generation", choice["type"])
}
```

再把旧 `ResponsesShapeChatCompletionsBuiltinToolsAugmentsAndStripsPrivateField` 中 `image_generation:true` 改成 `enabled:true` 新对象，并新增旧 carrier 清理 `tool_choice` 的测试。

- [ ] **步骤 5.2：运行红灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestChatCompletionsBuiltinTools_ImageGenerationRequiresEnabledAndAddsResponsesTool|TestResponsesShapeChatCompletionsBuiltinTools" -count=1
```

预期：FAIL，当前 Chat Completions normal path 只会追加 `web_search`。

- [ ] **步骤 5.3：实现 Chat Completions image tool 保留**

推荐实现方式：

1. 将 `applyOpenAICompatBuiltinToolsAugmentation` 改为返回被消费的 `[]map[string]any`，或新增 `extractNormalizedOpenAICompatBuiltinTools(req)`，避免丢失 image tool config。
2. normal path `ChatCompletionsToResponses` 后，marshal 成 `responsesBody` 前或后追加 image tool。若通过 `ResponsesTool`，先给 `ResponsesTool` 增加字段：

```go
Model             string `json:"model,omitempty"`
Size              string `json:"size,omitempty"`
Quality           string `json:"quality,omitempty"`
Background        string `json:"background,omitempty"`
OutputFormat      string `json:"output_format,omitempty"`
OutputCompression any    `json:"output_compression,omitempty"`
Moderation        string `json:"moderation,omitempty"`
Style             string `json:"style,omitempty"`
PartialImages     any    `json:"partial_images,omitempty"`
InputFidelity     string `json:"input_fidelity,omitempty"`
```

3. 更小改动方式是在 `responsesBody` marshal 后反序列化为 `map[string]any`，调用同一个 `mergeOpenAIBuiltinToolsIntoRequestBody`，再 marshal。这样无需扩大 `ChatTool`。
4. normal path 和 responses-shape path 最终都调用 `dropUnavailableOpenAIImageGenerationToolChoice`。

- [ ] **步骤 5.4：运行 Chat Completions 绿灯测试**

运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -run "TestChatCompletionsBuiltinTools|TestResponsesShapeChatCompletionsBuiltinTools|TestToolChoiceWhen.*BuiltinTools" -count=1
```

预期：PASS，`web_search` 旧行为保持。

---

## 任务 6：前端 OpenCode Image variant 与 agent.image

**文件：**
- 修改：`frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
- 修改：`frontend/src/components/keys/UseKeyModal.vue`

- [ ] **步骤 6.1：先写失败测试更新 OpenCode 配置期望**

在 `UseKeyModal.spec.ts` 的 `renders sub2api-openai provider config with Sys models in OpenCode example` 测试中替换 GPT-5.5 默认断言：

```ts
for (const model of [gpt55, gpt55Fast, gpt55Sys, gpt55FastSys]) {
  expect(model.options.metadata.builtin_tools).toEqual({ web_search: true })
  expect(model.variants.image).toBeDefined()
  expect(model.variants.image.metadata.builtin_tools).toEqual({
    web_search: true,
    image_generation: {
      enabled: true,
      model: 'gpt-image-2',
      output_format: 'png'
    }
  })
}

expect(gpt55Variants.none).toBeDefined()
expect(gpt55Variants.low).toBeDefined()
expect(gpt55Variants.image).toBeDefined()
expect(gpt55Sys.variants).not.toBe(gpt55.variants)
expect(gpt55FastSys.variants).not.toBe(gpt55Fast.variants)
expect(parsed.agent.image).toEqual({
  mode: 'subagent',
  description: expect.stringContaining('Generate images with GPT-5.5 Image Fast (Sys)'),
  model: 'sub2api-openai/gpt-5.5-fast-Sys',
  variant: 'image',
  options: { store: false }
})
expect(models[parsed.agent.image.model.replace('sub2api-openai/', '')].variants.image).toBeDefined()
```

保留 fast id 断言：`gpt-5.5-fast -> id:'gpt-5.5'`，`gpt-5.5-fast-Sys -> id:'gpt-5.5-Sys'`。

- [ ] **步骤 6.2：运行红灯前端测试**

在 `frontend` 目录运行：

```powershell
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

预期：FAIL，普通 GPT-5.5 当前仍默认含 image，且没有 `agent.image`。

- [ ] **步骤 6.3：实现前端配置生成**

在 `UseKeyModal.vue` 中调整 helper：

```ts
const openCodeBuiltinToolsForModel = (_id: string) => ({
  web_search: true
})

const openCodeImageGenerationBuiltinTools = () => ({
  web_search: true,
  image_generation: {
    enabled: true,
    model: 'gpt-image-2',
    output_format: 'png'
  }
})

const openCodeImageVariant = () => ({
  metadata: {
    builtin_tools: openCodeImageGenerationBuiltinTools()
  }
})
```

`withSysVariants` 必须 clone 可能共享的对象：

```ts
expanded[`${id}-Sys`] = {
  ...config,
  id: `${config.id}-Sys`,
  name: `${config.name} (Sys)`,
  options: config.options ? { ...config.options } : undefined,
  headers: config.headers ? { ...config.headers } : undefined,
  variants: config.variants ? { ...config.variants } : undefined
}
```

在生成 `openaiModels` 后注入 GPT-5.5 Image variant：

```ts
for (const [id, model] of Object.entries(openaiModels)) {
  if (!id.toLowerCase().startsWith('gpt-5.5')) continue
  model.variants = {
    ...(model.variants ?? {}),
    image: openCodeImageVariant()
  }
}
```

给 OpenAI/sub2api-openai agent 增加：

```ts
image: {
  mode: 'subagent',
  description: 'Generate images with GPT-5.5 Image Fast (Sys). Use this when the user asks to create, draw, render, or edit an image. Download the generated image immediately because the sub2api image URL is short-lived.',
  model: 'sub2api-openai/gpt-5.5-fast-Sys',
  variant: 'image',
  options: { store: false }
}
```

- [ ] **步骤 6.4：运行前端绿灯测试**

运行：

```powershell
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

预期：PASS。

---

## 任务 7：完整验证与手工 QA

**文件：**
- 检查：所有已修改文件

- [ ] **步骤 7.1：运行 changed-file diagnostics**

对所有修改过的 Go/Vue/TS 文件运行 LSP diagnostics。预期：changed files 无 error。

- [ ] **步骤 7.2：运行后端聚焦测试**

在 `backend` 目录运行：

```powershell
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/service -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/pkg/apicompat -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe test -tags unit ./internal/handler -count=1
C:\Users\34404\Documents\GitHub\workbench\toolchains\go\bin\go.exe build ./cmd/server
```

预期：全部 exit 0。

- [ ] **步骤 7.3：运行前端验证**

在 `frontend` 目录运行：

```powershell
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
pnpm typecheck
pnpm build
```

预期：全部 exit 0。

- [ ] **步骤 7.4：运行空白与占位符扫描**

在 worktree 根运行：

```powershell
rtk git diff --check
rtk grep -n "TB[D]|TO[D]O|待[定]|不确[定]" backend/docs/superpowers/plans/2026-05-04-opencode-gpt55-image-variant-routing-implementation.md backend/docs/superpowers/specs/2026-05-02-opencode-gpt55-image-variant-routing-design.md
```

预期：`rtk git diff --check` 无输出；`rtk grep` 返回 0 matches。

- [ ] **步骤 7.5：手工 QA 后端 surface**

用最小 Go 或 HTTP driver 走 handler/scheduler surface，不只看单元测试。推荐新增临时 driver 放在 `C:\Users\34404\AppData\Local\Temp\opencode\gpt55-image-routing-smoke`，使用测试 helper 难以跨 package 调用时，改用已存在 handler tests 的 `go test -run` 作为 surface driver，并记录以下观察：

1. 普通 `gpt-5.5` + `{ "metadata":{"builtin_tools":{"web_search":true}} }` 调度 request 中 `RequiredResponsesImageGeneration == nil`。
2. `gpt-5.5-Sys` + 新 image carrier 调度 request 中 `MainModel == "gpt-5.5"`、`ImageModel == "gpt-image-2"`。
3. 旧 image carrier + `tool_choice.image_generation` 转发上游前删除 `tool_choice`。
4. compact/passthrough 请求剥离 carrier 但不追加 tools。

- [ ] **步骤 7.6：手工 QA 前端 surface**

在 `frontend` 启动或用 Vitest DOM 渲染 surface 验证 OpenCode tab：

```powershell
pnpm exec vitest run src/components/keys/__tests__/UseKeyModal.spec.ts --pool=forks --poolOptions.forks.singleFork --reporter=verbose
```

在测试输出或渲染出的 JSON 中确认：普通 GPT-5.5 默认 `metadata.builtin_tools` 只有 `web_search`；`gpt-5.5-fast-Sys.variants.image` 含新 image carrier；`agent.image` 指向 `sub2api-openai/gpt-5.5-fast-Sys` 和 `variant:"image"`。

---

## 自检清单

- 规格目标 1、4、7：任务 1、2、5 锁定旧 image 失效与 `web_search` 兼容。
- 规格目标 2、6：任务 6 锁定 Image 变体与 `agent.image`。
- 规格目标 3：任务 1、2、5 锁定 `enabled:true + gpt-image-2` 门槛。
- 规格目标 5：任务 3、4 锁定调度 requirement 与账号能力过滤。
- Chat Completions、compact、passthrough、原生 Responses tools 边界：任务 2、3、5 覆盖。
- `/v1/images/*` 非目标路径：任务 4.8 覆盖。
- 验证命令与手工 QA：任务 7 覆盖。
- 本计划不包含占位符；执行时按每个任务的红灯、绿灯、验证顺序推进。
