//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// ---- composite generic Messages/CountTokens 池化（B1）测试 ----
//
// 构造手法对齐 gateway_handler_warmup_intercept_unit_test.go（真实 GatewayService +
// fake scheduler cache / group repo / concurrency cache，RunModeSimple 跳过计费）与
// openai_gateway_credential_failover_loop_test.go（fake HTTPUpstream 假上游）。

// compositePoolUpstreamCall 记录一次假上游调用。
type compositePoolUpstreamCall struct {
	Path          string
	Body          []byte
	Authorization string
	APIKeyHeader  string
	AccountID     int64
}

// compositePoolFakeUpstream 实现 service.HTTPUpstream，按调用序回放构造的响应。
type compositePoolFakeUpstream struct {
	mu      sync.Mutex
	calls   []compositePoolUpstreamCall
	respond func(call compositePoolUpstreamCall, index int) *http.Response
}

func (u *compositePoolFakeUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	u.mu.Lock()
	call := compositePoolUpstreamCall{
		Path:          req.URL.Path,
		Body:          body,
		Authorization: req.Header.Get("Authorization"),
		APIKeyHeader:  req.Header.Get("x-api-key"),
		AccountID:     accountID,
	}
	u.calls = append(u.calls, call)
	respond := u.respond
	u.mu.Unlock()
	if respond != nil {
		return respond(call, len(u.calls)-1), nil
	}
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"api_error","message":"upstream failed"}}`)),
	}, nil
}

func (u *compositePoolFakeUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func (u *compositePoolFakeUpstream) snapshot() []compositePoolUpstreamCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]compositePoolUpstreamCall(nil), u.calls...)
}

// chatCompletionsOKResponse 返回一份带 usage 的非流式 Chat Completions 响应。
func chatCompletionsOKResponse(model string, promptTokens, completionTokens int) *http.Response {
	body := fmt.Sprintf(
		`{"id":"chatcmpl-pool","object":"chat.completion","created":1,"model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":"hi from %s"},"finish_reason":"stop"}],"usage":{"prompt_tokens":%d,"completion_tokens":%d}}`,
		model, model, promptTokens, completionTokens)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req-cc-pool"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// anthropicMessagesOKResponse 返回一份 Anthropic Messages 成功响应。
func anthropicMessagesOKResponse() *http.Response {
	body := `{"id":"msg_pool_native","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"hi from anthropic"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":5}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req-native-pool"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// countTokensOKResponse 返回一份 Anthropic count_tokens 成功响应。
func countTokensOKResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"input_tokens":42}`)),
	}
}

// compositePoolUsageLogRepo 捕获 usage_log 写入（RunModeSimple 只走 best-effort 写入）。
type compositePoolUsageLogRepo struct {
	service.UsageLogRepository
	mu   sync.Mutex
	logs []*service.UsageLog
}

func (r *compositePoolUsageLogRepo) CreateBestEffort(_ context.Context, log *service.UsageLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, log)
	return nil
}

func (r *compositePoolUsageLogRepo) snapshot() []*service.UsageLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]*service.UsageLog(nil), r.logs...)
}

// compositePoolMessagesHarness 组装 generic Messages/CountTokens 池化测试的 handler。
type compositePoolMessagesHarness struct {
	handler        *GatewayHandler
	group          *service.Group
	openAIUpstream *compositePoolFakeUpstream
	gwUpstream     *compositePoolFakeUpstream
	usageRepo      *compositePoolUsageLogRepo
}

func newCompositePoolMessagesHarness(t *testing.T, accounts []*service.Account) *compositePoolMessagesHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	groupID := int64(41011)
	group := &service.Group{ID: groupID, Platform: service.PlatformComposite, Status: service.StatusActive, Hydrated: true}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Security = config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}

	usageRepo := &compositePoolUsageLogRepo{}
	openAIUpstream := &compositePoolFakeUpstream{}
	gwUpstream := &compositePoolFakeUpstream{}

	schedulerCache := &fakeSchedulerCache{accounts: accounts}
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)
	billingService := service.NewBillingService(cfg, nil)

	gatewayService := service.NewGatewayService(
		nil, // accountRepo：调度走 scheduler snapshot
		&fakeGroupRepo{group: group},
		usageRepo,          // usageLogRepo：捕获 usage 行
		nil, nil, nil, nil, // usageBillingRepo / userRepo / userSubRepo / userGroupRateRepo
		nil, // cache：禁用 sticky
		cfg,
		schedulerSnapshot,
		nil, // concurrencyService：负载感知降级
		billingService,
		nil, nil, nil, // rateLimitService / billingCacheService / identityService
		gwUpstream, // httpUpstream：generic Forward / CountTokens 假上游
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	)

	openAIGatewayService := service.NewOpenAIGatewayService(
		nil, nil, nil, nil, nil, nil, nil,
		cfg,
		nil, nil,
		billingService,
		nil, nil,
		openAIUpstream, // httpUpstream：委派链假上游
		nil, nil, nil, nil, nil, nil, nil, nil,
	)

	billingCacheService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCacheService.Stop)

	handler := &GatewayHandler{
		gatewayService:       gatewayService,
		openAIGatewayService: openAIGatewayService,
		billingCacheService:  billingCacheService,
		concurrencyHelper:    NewConcurrencyHelper(service.NewConcurrencyService(&fakeConcurrencyCache{}), SSEPingFormatClaude, 0),
		maxAccountSwitches:   3,
		cfg:                  cfg,
	}
	return &compositePoolMessagesHarness{
		handler:        handler,
		group:          group,
		openAIUpstream: openAIUpstream,
		gwUpstream:     gwUpstream,
		usageRepo:      usageRepo,
	}
}

// newCompositePoolGatewayRequest 构造带候选池 ctx 与 composite 分组身份的请求。
func (h *compositePoolMessagesHarness) newRequest(t *testing.T, path, body string, platforms ...string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := service.WithCompositeCandidatePlatforms(context.Background(), platforms)
	ctx = context.WithValue(ctx, ctxkey.Group, h.group)
	req = req.WithContext(ctx)
	c.Request = req

	apiKey := &service.APIKey{
		ID:      41012,
		UserID:  41013,
		GroupID: &h.group.ID,
		Status:  service.StatusActive,
		User:    &service.User{ID: 41013, Concurrency: 10, Balance: 100},
		Group:   h.group,
	}
	c.Set(string(middleware2.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})
	return c, rec
}

func poolMessagesBody() string {
	return `{"model":"my-model","max_tokens":64,"stream":false,"messages":[{"role":"user","content":"hello"}]}`
}

func poolCountTokensBody() string {
	return `{"model":"my-model","messages":[{"role":"user","content":"hello"}]}`
}

// poolKimiAccount 纯 OpenAI 族（CC 协议）kimi 账号：池选中后委派 ForwardAsAnthropic。
func poolKimiAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("kimi-%d", id), Platform: service.PlatformKimi,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "kimi-key",
			"base_url":      "https://kimi.example/v1",
			"api_protocol":  service.APIProtocolChatCompletions,
			"model_mapping": map[string]any{"my-model": "kimi-k3"},
		},
	}
}

// poolAnthropicAccount anthropic 原生 APIKey 账号：走既有 generic Forward。
func poolAnthropicAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("anthropic-%d", id), Platform: service.PlatformAnthropic,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "anthropic-key",
			"base_url":      "https://relay.example",
			"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"},
		},
	}
}

// poolAdaptiveKimiAccount 多协议 adaptive kimi 账号（Anthropic 协议 base 可解析）：
// 走既有 generic Forward（service 层协议化 base）。
func poolAdaptiveKimiAccount(id int64, groupID int64, priority int) *service.Account {
	return &service.Account{
		ID: id, Name: fmt.Sprintf("kimi-adaptive-%d", id), Platform: service.PlatformKimi,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true,
		Concurrency: 1, Priority: priority,
		AccountGroups: []service.AccountGroup{{AccountID: id, GroupID: groupID}},
		Credentials: map[string]any{
			"api_key":       "adaptive-key",
			"api_protocol":  service.APIProtocolAdaptive,
			"api_base_urls": map[string]any{"anthropic": "https://adaptive.example"},
			"model_mapping": map[string]any{"my-model": "claude-sonnet-4-5"},
		},
	}
}

// 池选中纯 OpenAI 族账号：委派 OpenAIGatewayService.ForwardAsAnthropic 转发（假上游
// 收到 Chat Completions 请求），响应以 Anthropic Messages 格式写回，usage 适配进
// 既有 RecordUsage 流（token 计数与模型映射链不丢）。
func TestCompositePoolMessagesDelegatesPureOpenAIFamilyAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolKimiAccount(41101, 41011, 0)})
	h.openAIUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return chatCompletionsOKResponse("kimi-k3", 11, 7)
	}

	c, rec := h.newRequest(t, "/v1/messages", poolMessagesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Messages(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "message", gjson.GetBytes(rec.Body.Bytes(), "type").String(),
		"委派链响应必须是 Anthropic Messages 格式")
	require.Equal(t, int64(11), gjson.GetBytes(rec.Body.Bytes(), "usage.input_tokens").Int())
	require.Equal(t, int64(7), gjson.GetBytes(rec.Body.Bytes(), "usage.output_tokens").Int())

	// 假上游收到的是 Chat Completions 请求（协议转换链生效）。
	calls := h.openAIUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/chat/completions"), "upstream path=%s", calls[0].Path)
	require.Contains(t, string(calls[0].Body), `"model":"kimi-k3"`, "账号级 model_mapping 必须生效")
	require.Contains(t, calls[0].Authorization, "kimi-key")

	// 选中账号 ops 记录为 kimi 账号。
	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 41101, selected)

	// usage 适配记录已提交：token 计数与模型映射链进入 usage 行。
	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 11, logs[0].InputTokens)
	require.Equal(t, 7, logs[0].OutputTokens)
	require.EqualValues(t, 41101, logs[0].AccountID)
	require.Equal(t, "my-model", logs[0].Model, "公开请求模型作为计费/展示模型记录")
	require.NotNil(t, logs[0].UpstreamModel)
	require.Equal(t, "kimi-k3", *logs[0].UpstreamModel)
}

// 池选中多协议账号（adaptive，Anthropic 协议 base 可解析）：走既有 generic Forward
// （假上游收到 {base}/v1/messages 请求），不委派 OpenAI 链。
func TestCompositePoolMessagesMultiProtocolAccountUsesGenericForward(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolAdaptiveKimiAccount(41201, 41011, 0)})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesOKResponse()
	}

	c, rec := h.newRequest(t, "/v1/messages", poolMessagesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Messages(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "msg_pool_native", gjson.GetBytes(rec.Body.Bytes(), "id").String())

	calls := h.gwUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/v1/messages"), "upstream path=%s", calls[0].Path)
	require.False(t, strings.HasSuffix(calls[0].Path, "/chat/completions"))
	require.Equal(t, "adaptive-key", calls[0].APIKeyHeader, "多协议账号默认 x-api-key 认证")
	require.Empty(t, h.openAIUpstream.snapshot(), "多协议账号不得委派 OpenAI 链")

	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 41201, selected)

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.Equal(t, 12, logs[0].InputTokens)
	require.Equal(t, 5, logs[0].OutputTokens)
}

// 委派链返回 UpstreamFailoverError：走既有 failover 语义换号重选，改用 anthropic
// 原生账号的 generic Forward 完成请求。
func TestCompositePoolMessagesDelegationFailoverSwitchesAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{
		poolKimiAccount(41301, 41011, 0),
		poolAnthropicAccount(41302, 41011, 5),
	})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return anthropicMessagesOKResponse()
	}
	// kimi 假上游默认回 500（respond 为 nil 时的兜底响应即 500）。

	c, rec := h.newRequest(t, "/v1/messages", poolMessagesBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.Messages(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "msg_pool_native", gjson.GetBytes(rec.Body.Bytes(), "id").String(),
		"failover 后应由 anthropic 原生账号完成请求")

	// 第一次委派失败（CC 上游 500），第二次走 generic Forward。
	require.Len(t, h.openAIUpstream.snapshot(), 1)
	require.Len(t, h.gwUpstream.snapshot(), 1)
	require.True(t, strings.HasSuffix(h.gwUpstream.snapshot()[0].Path, "/v1/messages"))

	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 41302, selected, "ops 记录最终选中的 anthropic 账号")

	logs := h.usageRepo.snapshot()
	require.Len(t, logs, 1)
	require.EqualValues(t, 41302, logs[0].AccountID)
}

// count_tokens 池请求选中纯 OpenAI 族账号：跳过该账号重选（排除集），而不是对
// /v1/messages/count_tokens 报 500。池内无可计数账号时按无可用账号终止。
func TestCompositePoolCountTokensSkipsUncountableOpenAIAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{poolKimiAccount(41401, 41011, 0)})

	c, rec := h.newRequest(t, "/v1/messages/count_tokens", poolCountTokensBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.CountTokens(c)

	require.NotEqual(t, http.StatusOK, rec.Code, "纯 OpenAI 族账号被跳过后按无可用账号终止")
	require.Empty(t, h.gwUpstream.snapshot(), "不可计数账号不得转发 count_tokens")
	require.Empty(t, h.openAIUpstream.snapshot())
}

// count_tokens 池请求跳过纯 OpenAI 族账号后改选可计数的 anthropic 账号并成功转发。
func TestCompositePoolCountTokensSkipsThenForwardsCountableAccount(t *testing.T) {
	h := newCompositePoolMessagesHarness(t, []*service.Account{
		poolKimiAccount(41501, 41011, 0),
		poolAnthropicAccount(41502, 41011, 5),
	})
	h.gwUpstream.respond = func(compositePoolUpstreamCall, int) *http.Response {
		return countTokensOKResponse()
	}

	c, rec := h.newRequest(t, "/v1/messages/count_tokens", poolCountTokensBody(), service.PlatformAnthropic, service.PlatformKimi)
	h.handler.CountTokens(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), gjson.GetBytes(rec.Body.Bytes(), "input_tokens").Int())

	calls := h.gwUpstream.snapshot()
	require.Len(t, calls, 1)
	require.True(t, strings.HasSuffix(calls[0].Path, "/v1/messages/count_tokens"), "upstream path=%s", calls[0].Path)
	require.Equal(t, "anthropic-key", calls[0].APIKeyHeader)

	selected, ok := c.Get(opsAccountIDKey)
	require.True(t, ok)
	require.EqualValues(t, 41502, selected, "ops 记录最终选中的可计数账号")
}

// 池账号委派分发谓词与结果适配器的纯函数契约。
func TestCompositePoolAccountDelegationPredicateAndAdapter(t *testing.T) {
	// 纯 OpenAI 族账号（含 openai/grok/国产 CC 供应商）→ 委派。
	require.True(t, compositePoolAccountDelegatesToOpenAI(poolKimiAccount(1, 1, 0)))
	require.True(t, compositePoolAccountDelegatesToOpenAI(&service.Account{Platform: service.PlatformOpenAI}))
	require.True(t, compositePoolAccountDelegatesToOpenAI(&service.Account{Platform: service.PlatformGrok}))

	// anthropic 原生 / gemini / antigravity → 既有 Forward。
	require.False(t, compositePoolAccountDelegatesToOpenAI(poolAnthropicAccount(2, 1, 0)))
	require.False(t, compositePoolAccountDelegatesToOpenAI(&service.Account{Platform: service.PlatformGemini}))
	require.False(t, compositePoolAccountDelegatesToOpenAI(&service.Account{Platform: service.PlatformAntigravity}))

	// 多协议（adaptive / anthropic-协议）账号 → 既有 Forward（service 协议化 base）。
	require.False(t, compositePoolAccountDelegatesToOpenAI(poolAdaptiveKimiAccount(3, 1, 0)))
	require.False(t, compositePoolAccountDelegatesToOpenAI(nil))

	// 结果适配：usage token 计数与元数据逐字段映射，nil 安全。
	require.Nil(t, adaptOpenAIForwardResultToForwardResult(nil))
	adapted := adaptOpenAIForwardResultToForwardResult(&service.OpenAIForwardResult{
		RequestID:     "rid",
		Usage:         service.OpenAIUsage{InputTokens: 3, OutputTokens: 4, CacheCreationInputTokens: 5, CacheReadInputTokens: 6},
		Model:         "public",
		UpstreamModel: "upstream",
		Stream:        true,
		SearchCount:   2,
	})
	require.Equal(t, "rid", adapted.RequestID)
	require.Equal(t, 3, adapted.Usage.InputTokens)
	require.Equal(t, 4, adapted.Usage.OutputTokens)
	require.Equal(t, 5, adapted.Usage.CacheCreationInputTokens)
	require.Equal(t, 6, adapted.Usage.CacheReadInputTokens)
	require.Equal(t, "public", adapted.Model)
	require.Equal(t, "upstream", adapted.UpstreamModel)
	require.True(t, adapted.Stream)
	require.Equal(t, 2, adapted.SearchCount)
}
