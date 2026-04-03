# 未分组 Key OpenAI 全局池 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为未分组 API key 增加一个后台 settings 开关；开启后，这些 key 在请求期被解析为 OpenAI 平台，并使用全部 OpenAI 账号池，而不是未分组账号池。

**Architecture:** 不改数据库 schema，不伪造 `Group`。新增一个后台 setting，并在请求期解析 `effective platform`；路由分流、模型视图、OpenAI 账号选择统一读取这一派生值。关闭开关时保持当前默认行为。

**Tech Stack:** Go, Gin, Ent/SQL repository, existing settings system, OpenAI gateway handlers/services, Go tests

---

## File Map

- Modify: `backend/internal/service/domain_constants.go`
  - 增加新 setting key 常量
- Modify: `backend/internal/service/settings_view.go`
  - 在 `SystemSettings` 中增加布尔字段
- Modify: `backend/internal/service/setting_service.go`
  - 读取/写入/默认值/解析新 setting
- Modify: `backend/internal/handler/admin/setting_handler.go`
  - 后台 settings 请求/响应 DTO 接入新字段
- Modify: `backend/internal/server/middleware/middleware.go`
  - 增加 request-scoped effective platform 上下文键与读写 helper
- Modify: `backend/internal/server/middleware/api_key_auth.go`
  - 在 API key 鉴权后设置 request-scoped effective platform
- Modify: `backend/internal/server/routes/gateway.go`
  - 所有当前依赖 `group.platform` 分流的入口改读 effective platform
- Modify: `backend/internal/handler/gateway_handler.go`
  - `GET /v1/models` 改读 effective platform，并在 OpenAI 模式下向 `GetAvailableModels` 传 `platform=openai`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
  - 从请求上下文或 API key 上下文识别“未分组 key + OpenAI 全局池开关开启”状态，并把它传到 OpenAI 选择逻辑
- Modify: `backend/internal/service/openai_gateway_service.go`
  - 将 `groupID == nil` 的 OpenAI 查询分成两条：默认未分组账号池 / 开关开启时全部 OpenAI 账号池
- Modify: `backend/internal/service/gateway_service.go`
  - 如果有与 `groupID == nil + platform` 相关的通用模型列表或选择辅助逻辑，也要同步使用新语义
- Test: `backend/internal/service/setting_service_update_test.go`
  - 新 setting 的读写与默认值
- Test: `backend/internal/server/middleware/api_key_auth_test.go`
  - API key 鉴权后 effective platform 注入
- Test: `backend/internal/server/routes/gateway_test.go`
  - 自动分流入口改读 effective platform
- Test: `backend/internal/handler/openai_gateway_handler_test.go`
  - OpenAI handler 在未分组 key + 开关开启时进入正确选择路径
- Test: `backend/internal/service/openai_gateway_service_test.go`
  - `groupID == nil` 的两种账号池语义
- Test: `backend/internal/handler/gateway_handler_models_test.go`
  - `/v1/models` 在该模式下返回 OpenAI 模型视图

## Implementation Notes

- 不要提交 git，除非用户明确要求。
- 保持 `apiKey.GroupID == nil` 的真实语义不变。
- 不要在请求中伪造 `apiKey.Group`。
- 所有分流只依赖 request-scoped `effective platform`。
- `GET /v1/models` 这一处要特别小心：模型白名单查询不能再把全部平台模型混在一起。

### Task 1: 接入后台 setting

**Files:**
- Modify: `backend/internal/service/domain_constants.go`
- Modify: `backend/internal/service/settings_view.go`
- Modify: `backend/internal/service/setting_service.go`
- Modify: `backend/internal/handler/admin/setting_handler.go`
- Test: `backend/internal/service/setting_service_update_test.go`

- [ ] **Step 1: 先写 setting service 的失败测试**

```go
func TestSettingService_UpdateSettings_OpenAIGlobalPoolForUngroupedKeys(t *testing.T) {
	svc := newSettingServiceForTest()
	err := svc.UpdateSettings(context.Background(), &SystemSettings{
		OpenAIGlobalPoolForUngroupedKeys: true,
	})
	require.NoError(t, err)

	got, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, got.OpenAIGlobalPoolForUngroupedKeys)
}
```

- [ ] **Step 2: 运行单测确认失败**

Run: `C:\Users\34404\.local\go\bin\go.exe test ./internal/service -run TestSettingService_UpdateSettings_OpenAIGlobalPoolForUngroupedKeys -count=1`

Expected: FAIL，提示 `SystemSettings` 没有该字段，或 setting 未被持久化/解析。

- [ ] **Step 3: 增加 setting 常量、视图字段、默认值、解析与更新逻辑**

```go
// domain_constants.go
const SettingKeyOpenAIGlobalPoolForUngroupedKeys = "openai_global_pool_for_ungrouped_keys"

// settings_view.go
type SystemSettings struct {
	AllowUngroupedKeyScheduling          bool
	OpenAIGlobalPoolForUngroupedKeys     bool
}

// setting_service.go
updates[SettingKeyOpenAIGlobalPoolForUngroupedKeys] = strconv.FormatBool(settings.OpenAIGlobalPoolForUngroupedKeys)

defaults := map[string]string{
	SettingKeyAllowUngroupedKeyScheduling:      "false",
	SettingKeyOpenAIGlobalPoolForUngroupedKeys: "false",
}

result.OpenAIGlobalPoolForUngroupedKeys = settings[SettingKeyOpenAIGlobalPoolForUngroupedKeys] == "true"
```

- [ ] **Step 4: 接入后台 settings handler DTO**

```go
type UpdateSettingsRequest struct {
	AllowUngroupedKeyScheduling      bool `json:"allow_ungrouped_key_scheduling"`
	OpenAIGlobalPoolForUngroupedKeys bool `json:"openai_global_pool_for_ungrouped_keys"`
}
```

- [ ] **Step 5: 重跑单测确认通过**

Run: `C:\Users\34404\.local\go\bin\go.exe test ./internal/service -run TestSettingService_UpdateSettings_OpenAIGlobalPoolForUngroupedKeys -count=1`

Expected: PASS

### Task 2: 在请求上下文注入 effective platform

**Files:**
- Modify: `backend/internal/server/middleware/middleware.go`
- Modify: `backend/internal/server/middleware/api_key_auth.go`
- Test: `backend/internal/server/middleware/api_key_auth_test.go`

- [ ] **Step 1: 先写 API key 鉴权后的失败测试**

```go
func TestAPIKeyAuthSetsEffectivePlatformForUngroupedOpenAIKey(t *testing.T) {
	apiKey := &service.APIKey{ID: 1, GroupID: nil, Group: nil, Status: service.StatusActive, User: &service.User{ID: 1, Status: service.StatusActive}}
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAPIKeyAuthMiddleware(apiKeyServiceStubReturning(apiKey), nil, cfgWithOpenAIGlobalPoolForUngroupedKeys(true))))
	router.GET("/probe", func(c *gin.Context) {
		platform, ok := GetEffectivePlatformFromContext(c)
		require.True(t, ok)
		require.Equal(t, service.PlatformOpenAI, platform)
		c.Status(http.StatusNoContent)
	})
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `C:\Users\34404\.local\go\bin\go.exe test ./internal/server/middleware -run TestAPIKeyAuthSetsEffectivePlatformForUngroupedOpenAIKey -count=1`

Expected: FAIL，提示上下文中取不到 effective platform。

- [ ] **Step 3: 增加中间件上下文键与 helper**

```go
const ContextKeyEffectivePlatform ContextKey = "effective_platform"

func GetEffectivePlatformFromContext(c *gin.Context) (string, bool) {
	v, ok := c.Get(string(ContextKeyEffectivePlatform))
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	return s, s != ""
}
```

- [ ] **Step 4: 在 API key 鉴权后设置 effective platform**

```go
effectivePlatform := ""
if apiKey.Group != nil {
	effectivePlatform = strings.TrimSpace(apiKey.Group.Platform)
}
if effectivePlatform == "" && apiKey.GroupID == nil && settingService.IsOpenAIGlobalPoolForUngroupedKeys(ctx) {
	effectivePlatform = service.PlatformOpenAI
}
if effectivePlatform != "" {
	c.Set(string(ContextKeyEffectivePlatform), effectivePlatform)
}
```

- [ ] **Step 5: 重跑 middleware 测试确认通过**

Run: `C:\Users\34404\.local\go\bin\go.exe test ./internal/server/middleware -run TestAPIKeyAuthSetsEffectivePlatformForUngroupedOpenAIKey -count=1`

Expected: PASS

### Task 3: 用 effective platform 统一驱动分流与模型视图

**Files:**
- Modify: `backend/internal/server/routes/gateway.go`
- Modify: `backend/internal/handler/gateway_handler.go`
- Test: `backend/internal/server/routes/gateway_test.go`
- Test: `backend/internal/handler/gateway_handler_models_test.go`

- [ ] **Step 1: 先写路由分流测试**

```go
func TestResponsesRouteUsesEffectivePlatformOpenAI(t *testing.T) {
	calledOpenAI := false
	calledGeneric := false

	r := gin.New()
	r.POST("/v1/responses", func(c *gin.Context) {
		c.Set("effective_platform", service.PlatformOpenAI)
		if getEffectivePlatform(c) == service.PlatformOpenAI {
			calledOpenAI = true
			c.Status(http.StatusNoContent)
			return
		}
		calledGeneric = true
	})

	// 断言最终是 OpenAI 分支
	require.True(t, calledOpenAI)
	require.False(t, calledGeneric)
}
```

- [ ] **Step 2: 写 `/v1/models` 的失败测试**

```go
func TestModelsUsesEffectivePlatformForUngroupedOpenAIKey(t *testing.T) {
	h := newGatewayHandlerForModelsTest()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(middleware2.ContextKeyEffectivePlatform), service.PlatformOpenAI)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 1, GroupID: nil, Group: nil})

	h.Models(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "gpt-5.4", gjson.Get(w.Body.String(), "data.0.id").String())
}
```

- [ ] **Step 3: 运行两组测试确认失败**

Run:

- `C:\Users\34404\.local\go\bin\go.exe test ./internal/server/routes -run EffectivePlatform -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/handler -run TestModelsUsesEffectivePlatformForUngroupedOpenAIKey -count=1`

Expected: FAIL，说明当前仍直接读取 `apiKey.Group.Platform`。

- [ ] **Step 4: 在路由层统一改读 effective platform**

```go
func getEffectivePlatform(c *gin.Context) string {
	if forced, ok := middleware.GetForcePlatformFromContext(c); ok && strings.TrimSpace(forced) != "" {
		return forced
	}
	if platform, ok := middleware.GetEffectivePlatformFromContext(c); ok {
		return platform
	}
	return ""
}
```

并将以下分支从 `getGroupPlatform(c)` 改成 `getEffectivePlatform(c)`：

- `/v1/messages`
- `/v1/messages/count_tokens`
- `/v1/responses`
- `/responses`
- `/v1/chat/completions`
- `/chat/completions`

- [ ] **Step 5: 修正 `/v1/models`**

```go
var groupID *int64
platform, _ := middleware2.GetEffectivePlatformFromContext(c)
if apiKey != nil && apiKey.GroupID != nil {
	groupID = apiKey.GroupID
}
availableModels := h.gatewayService.GetAvailableModels(c.Request.Context(), groupID, platform)
```

- [ ] **Step 6: 重跑路由和 models 测试确认通过**

Run:

- `C:\Users\34404\.local\go\bin\go.exe test ./internal/server/routes -run EffectivePlatform -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/handler -run TestModelsUsesEffectivePlatformForUngroupedOpenAIKey -count=1`

Expected: PASS

### Task 4: OpenAI 账号选择改成全平台账号池

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/handler/openai_gateway_handler_test.go`
- Test: `backend/internal/service/openai_gateway_service_test.go`

- [ ] **Step 1: 先写 service 级失败测试**

```go
func TestListSchedulableAccounts_UngroupedOpenAIGlobalPoolUsesAllOpenAIAccounts(t *testing.T) {
	repo := stubOpenAIAccountRepo{
		listSchedulableUngroupedByPlatformFunc: func(ctx context.Context, platform string) ([]Account, error) {
			return []Account{{ID: 64}}, nil
		},
		listSchedulableByPlatformFunc: func(ctx context.Context, platform string) ([]Account, error) {
			return []Account{{ID: 64}, {ID: 65}, {ID: 66}}, nil
		},
	}
	service := newOpenAIGatewayServiceForTest(repo)
	accounts, err := service.listSchedulableAccounts(ctx, nil, service.TargetGroupActive, true)
	require.NoError(t, err)
	require.Len(t, accounts, 3)
}
```

- [ ] **Step 2: 写 handler 级失败测试**

```go
func TestOpenAIHandler_UngroupedOpenAIGlobalPoolPassesFlag(t *testing.T) {
	calledWithGlobalPool := false
	stub := &capturingOpenAIGatewayService{
		selectAccountWithSchedulerFunc: func(ctx context.Context, groupID *int64, sessionHash string, requestedModel string, excludedIDs map[int64]struct{}, targetGroup service.AccountTargetGroup, useGlobalPool bool) (*service.AccountSelectionResult, *service.ScheduleDecision, error) {
			calledWithGlobalPool = useGlobalPool
			return nil, nil, service.ErrNoAvailableAccounts
		},
	}
	h := newOpenAIGatewayHandlerForTest(stub)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(string(middleware.ContextKeyEffectivePlatform), service.PlatformOpenAI)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 1, GroupID: nil, Group: nil})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Responses(c)

	require.True(t, calledWithGlobalPool)
}
```

- [ ] **Step 3: 运行测试确认失败**

Run:

- `C:\Users\34404\.local\go\bin\go.exe test ./internal/service -run UngroupedOpenAIGlobalPool -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/handler -run UngroupedOpenAIGlobalPool -count=1`

Expected: FAIL，当前 `groupID=nil` 仍只走未分组账号分支。

- [ ] **Step 4: 在 handler 到 service 之间传递全局池语义**

建议签名形态：

```go
func (s *OpenAIGatewayService) listSchedulableAccounts(ctx context.Context, groupID *int64, targetGroup AccountTargetGroup, useAllOpenAIAccounts bool) ([]Account, error)
```

或等价的 request-scoped helper，但必须满足：

- `groupID == nil` 且 `useAllOpenAIAccounts == false` -> 旧行为
- `groupID == nil` 且 `useAllOpenAIAccounts == true` -> 查全部 OpenAI 账号

- [ ] **Step 5: 在 OpenAI service 中切换查询分支**

```go
if groupID != nil {
	accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, *groupID, PlatformOpenAI)
} else if useAllOpenAIAccounts {
	accounts, err = s.accountRepo.ListSchedulableByPlatform(ctx, PlatformOpenAI)
} else {
	accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, PlatformOpenAI)
}
```

同样处理 exhausted merge 的 broad source 分支，保证使用 `ListByPlatform(ctx, PlatformOpenAI)` 而不是未分组版本。

- [ ] **Step 6: 重跑 handler/service 测试确认通过**

Run:

- `C:\Users\34404\.local\go\bin\go.exe test ./internal/service -run UngroupedOpenAIGlobalPool -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/handler -run UngroupedOpenAIGlobalPool -count=1`

Expected: PASS

### Task 5: 回归验证与现场验证

**Files:**
- Test-only / runtime verification

- [ ] **Step 1: 跑相关包回归测试**

Run:

- `C:\Users\34404\.local\go\bin\go.exe test ./internal/server/middleware -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/server/routes -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/handler -count=1`
- `C:\Users\34404\.local\go\bin\go.exe test ./internal/service -count=1`

Expected: PASS

- [ ] **Step 2: 跑格式/补丁检查**

Run:

- `gofmt -w backend/internal/service/domain_constants.go backend/internal/service/settings_view.go backend/internal/service/setting_service.go backend/internal/handler/admin/setting_handler.go backend/internal/server/middleware/middleware.go backend/internal/server/middleware/api_key_auth.go backend/internal/server/routes/gateway.go backend/internal/handler/gateway_handler.go backend/internal/handler/openai_gateway_handler.go backend/internal/service/openai_gateway_service.go`
- `git diff --check`

Expected: 无格式与空白错误

- [ ] **Step 3: 做现场验证（Claude 协议）**

操作：

1. 后台打开 `openai_global_pool_for_ungrouped_keys`
2. 将测试 key 解绑为未分组
3. 将 OpenAI 测试账号解绑为未分组
4. 用 `/v1/messages` 打一条 OpenAI 目标请求

Expected:

- 请求成功
- 命中某个 OpenAI 账号
- access log/usage log 可见 `account_id`

- [ ] **Step 4: 做现场验证（OpenAI 协议）**

操作：

1. 保持上一步实验态
2. 用 `/v1/responses` 或 `/v1/chat/completions` 打一条请求

Expected:

- 请求成功
- 命中某个 OpenAI 账号

- [ ] **Step 5: 恢复实验态**

操作：

1. 关闭 `openai_global_pool_for_ungrouped_keys`
2. 将测试 key 和账号重新绑回原测试组

Expected:

- 恢复到验证前状态

## Self-Review

- Spec coverage:
  - 新后台 setting：Task 1
  - request-scoped effective platform：Task 2
  - 路由/模型视图统一改读平台解析结果：Task 3
  - `groupID=nil` 改成全 OpenAI 池：Task 4
  - 现场验证 Claude/OpenAI 兼容入口：Task 5
- Placeholder scan:
  - 无 `TODO` / `TBD`
  - 每个代码步骤都给出具体代码形态
  - 每个验证步骤都给出明确命令和预期
- Type consistency:
  - 统一使用 `OpenAIGlobalPoolForUngroupedKeys` 作为 settings 字段名
  - 统一使用 `effective platform` 作为请求级派生概念
  - `GroupID` 始终是身份语义，不能被 synthetic/fake 值替代
