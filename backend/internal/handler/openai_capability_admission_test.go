//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const capabilityAdmissionBlueManifest = `{"models":[{"slug":"gpt-6-sol","service_tiers":[{"id":"priority"},{"id":"ultrafast"}],"available_access_programs":{"cyber":["standard","daybreak_blue"]}}]}`
const capabilityAdmissionStandardManifest = `{"models":[{"slug":"gpt-6-sol","service_tiers":[{"id":"priority"}],"available_access_programs":{"cyber":["standard"]}}]}`

// capabilityAdmissionAccountRepo 同时服务候选集合与持久池（ListModelAvailabilityCandidates）。
type capabilityAdmissionAccountRepo struct {
	openAIImagesFailoverAccountRepo
}

func (r capabilityAdmissionAccountRepo) ListModelAvailabilityCandidates(_ context.Context, _ *int64, platforms []string, _ bool) ([]service.Account, error) {
	out := make([]service.Account, 0)
	for _, platform := range platforms {
		out = append(out, r.accountsForPlatform(platform)...)
	}
	return out, nil
}

type capabilityAdmissionUpstream struct {
	service.HTTPUpstream
	mu     sync.Mutex
	bodies [][]byte
	auths  []string
	urls   []string
}

func (u *capabilityAdmissionUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	body, _ := io.ReadAll(req.Body)
	u.bodies = append(u.bodies, body)
	u.auths = append(u.auths, req.Header.Get("Authorization"))
	u.urls = append(u.urls, req.URL.String())
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"type":"invalid_request_error","message":"stop before response parsing"}}`)),
	}, nil
}

func (u *capabilityAdmissionUpstream) calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.bodies)
}

func capabilityAdmissionAccount(id int64, accountType, token, manifest string) service.Account {
	account := service.Account{
		ID: id, Name: "capability-" + token, Platform: service.PlatformOpenAI, Type: accountType,
		Status: service.StatusActive, Schedulable: true, Concurrency: 0, Priority: int(id),
		Credentials: map[string]any{"access_token": token, "api_key": token},
		Extra:       map[string]any{},
	}
	if manifest != "" {
		caps, _ := service.DeriveCodexAccountCapabilities([]byte(manifest))
		account.Extra[service.CodexCapabilitiesExtraKey] = caps
	}
	return account
}

func newCapabilityAdmissionHandler(t *testing.T, accounts []service.Account, upstream service.HTTPUpstream) *OpenAIGatewayHandler {
	t.Helper()
	cfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewOpenAIGatewayService(
		capabilityAdmissionAccountRepo{openAIImagesFailoverAccountRepo{accounts: accounts}},
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream, nil, nil, nil, nil, nil, nil, nil, nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	handler := NewOpenAIGatewayHandler(
		gatewayService, service.NewConcurrencyService(nil), billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg), nil, nil, nil, nil, cfg,
	)
	handler.maxAccountSwitches = 3
	return handler
}

func newCapabilityAdmissionContext(t *testing.T, path, body string, group *service.Group) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	groupID := group.ID
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID: 7101, GroupID: &groupID, Group: group, User: &service.User{ID: 7102, Status: service.StatusActive},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7102, Concurrency: 0})
	return c, rec
}

func capabilityAdmissionGroup(allowlist service.GroupCapabilityAllowlist) *service.Group {
	return &service.Group{ID: 7100, Platform: service.PlatformOpenAI, AllowMessagesDispatch: true, CapabilityAllowlist: allowlist}
}

var capabilityAdmissionFastOnly = service.GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority"}}

type capabilityAdmissionEntry struct {
	name    string
	path    string
	body    string
	call    func(h *OpenAIGatewayHandler, c *gin.Context)
	errType string
}

func capabilityAdmissionEntries(body string) []capabilityAdmissionEntry {
	return []capabilityAdmissionEntry{
		{name: "responses", path: "/v1/responses", body: body, call: func(h *OpenAIGatewayHandler, c *gin.Context) { h.Responses(c) }, errType: "invalid_request_error"},
		{name: "chat completions responses-shape", path: "/v1/chat/completions", body: body, call: func(h *OpenAIGatewayHandler, c *gin.Context) { h.ChatCompletions(c) }, errType: "invalid_request_error"},
	}
}

// 白名单拒绝：HTTP 入口返回 404 capability_not_allowed，不选号、不发上游。
func TestOpenAICapabilityAdmission_HTTPNotAllowedReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-6-sol","stream":false,"input":"hello","service_tier":"ultrafast","access_programs":{"cyber":"daybreak_blue"}}`
	for _, entry := range capabilityAdmissionEntries(body) {
		t.Run(entry.name, func(t *testing.T) {
			upstream := &capabilityAdmissionUpstream{}
			h := newCapabilityAdmissionHandler(t, []service.Account{capabilityAdmissionAccount(1, service.AccountTypeOAuth, "blue", capabilityAdmissionBlueManifest)}, upstream)
			c, rec := newCapabilityAdmissionContext(t, entry.path, entry.body, capabilityAdmissionGroup(capabilityAdmissionFastOnly))
			entry.call(h, c)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Equal(t, entry.errType, gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			require.Equal(t, "capability_not_allowed", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
			require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), `"daybreak_blue" is not allowed`)
			require.Zero(t, upstream.calls(), "拒绝不发起上游请求")
			_, selected := c.Get(opsAccountIDKey)
			require.False(t, selected, "拒绝发生在选号之前")
			require.Equal(t, service.OpsClientBusinessLimitedReasonLocalPolicyDenied, service.OpsClientBusinessLimitedReason(c))
			reason, rejected := middleware2.GetIngressRejectReason(c)
			require.True(t, rejected)
			require.Equal(t, middleware2.IngressRejectCapabilityNotAllowed, reason)
		})
	}
}

func TestOpenAICapabilityAdmission_MessagesNotAllowedReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &capabilityAdmissionUpstream{}
	h := newCapabilityAdmissionHandler(t, []service.Account{capabilityAdmissionAccount(1, service.AccountTypeOAuth, "blue", capabilityAdmissionBlueManifest)}, upstream)
	body := `{"model":"gpt-6-sol","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`
	// Messages 路径的 Fast 来自 anthropic-beta fast-mode 头；白名单禁用 priority 时拒绝。
	c, rec := newCapabilityAdmissionContext(t, "/v1/messages", body, capabilityAdmissionGroup(service.GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"ultrafast"}}))
	c.Request.Header.Set("anthropic-beta", claude.BetaFastMode)
	h.Messages(c)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "not_found_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "capability_not_allowed")
	require.Zero(t, upstream.calls())
	reason, rejected := middleware2.GetIngressRejectReason(c)
	require.True(t, rejected)
	require.Equal(t, middleware2.IngressRejectCapabilityNotAllowed, reason)

	// 请求体 service_tier 在 Messages 路径不会到达上游，不构成需求：不被白名单拒绝。
	upstream = &capabilityAdmissionUpstream{}
	h = newCapabilityAdmissionHandler(t, []service.Account{capabilityAdmissionAccount(1, service.AccountTypeOAuth, "blue", capabilityAdmissionBlueManifest)}, upstream)
	c, rec = newCapabilityAdmissionContext(t, "/v1/messages", `{"model":"gpt-6-sol","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"service_tier":"ultrafast"}`, capabilityAdmissionGroup(capabilityAdmissionFastOnly))
	h.Messages(c)
	require.NotEqual(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.GreaterOrEqual(t, upstream.calls(), 1)
	require.False(t, gjson.GetBytes(upstream.bodies[0], "service_tier").Exists(), "转换层不携带请求体 service_tier")
}

// 持久池没有具备能力的账号：HTTP 入口返回 404 capability_not_available。
func TestOpenAICapabilityAdmission_HTTPUnavailableReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-6-sol","stream":false,"input":"hello","access_programs":{"cyber":"daybreak_blue"}}`
	for _, entry := range capabilityAdmissionEntries(body) {
		t.Run(entry.name, func(t *testing.T) {
			upstream := &capabilityAdmissionUpstream{}
			h := newCapabilityAdmissionHandler(t, []service.Account{
				capabilityAdmissionAccount(1, service.AccountTypeOAuth, "standard", capabilityAdmissionStandardManifest),
				capabilityAdmissionAccount(2, service.AccountTypeOAuth, "unprobed", ""),
			}, upstream)
			c, rec := newCapabilityAdmissionContext(t, entry.path, entry.body, capabilityAdmissionGroup(service.GroupCapabilityAllowlist{}))
			entry.call(h, c)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Equal(t, "capability_not_available", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
			require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), `capability "daybreak_blue"`)
			require.Zero(t, upstream.calls())
			require.False(t, isOpsRoutingCapacityLimited(c), "配置类不可用不计入路由容量不足")
		})
	}
}

func TestOpenAICapabilityAdmission_MessagesUnavailableReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &capabilityAdmissionUpstream{}
	// 唯一账号未探测：fast-mode 头构成的 Fast 需求无账号满足。
	h := newCapabilityAdmissionHandler(t, []service.Account{capabilityAdmissionAccount(1, service.AccountTypeOAuth, "unprobed", "")}, upstream)
	body := `{"model":"gpt-6-sol","max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`
	c, rec := newCapabilityAdmissionContext(t, "/v1/messages", body, capabilityAdmissionGroup(service.GroupCapabilityAllowlist{}))
	c.Request.Header.Set("anthropic-beta", claude.BetaFastMode)
	h.Messages(c)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "not_found_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "capability_not_available")
	require.Zero(t, upstream.calls())
}

// 允许的能力正常选号：需求进入调度，只落到 Blue 账号，且 access_programs 原样到达上游。
func TestOpenAICapabilityAdmission_AllowedRequestRoutesToCapableAccountAndKeepsAccessPrograms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	accounts := func() []service.Account {
		return []service.Account{
			capabilityAdmissionAccount(1, service.AccountTypeOAuth, "standard", capabilityAdmissionStandardManifest),
			capabilityAdmissionAccount(2, service.AccountTypeOAuth, "blue", capabilityAdmissionBlueManifest),
		}
	}
	group := capabilityAdmissionGroup(service.GroupCapabilityAllowlist{Enabled: true, CyberPrograms: []string{"daybreak_blue"}, ServiceTiers: []string{"priority", "ultrafast"}})
	body := `{"model":"gpt-6-sol","stream":false,"input":"hello","access_programs":{"cyber":"daybreak_blue"}}`
	for _, entry := range capabilityAdmissionEntries(body) {
		t.Run(entry.name, func(t *testing.T) {
			upstream := &capabilityAdmissionUpstream{}
			h := newCapabilityAdmissionHandler(t, accounts(), upstream)
			c, _ := newCapabilityAdmissionContext(t, entry.path, entry.body, group)
			entry.call(h, c)
			require.GreaterOrEqual(t, upstream.calls(), 1, "允许时正常选号并转发")
			for i, auth := range upstream.auths {
				require.Equal(t, "Bearer blue", auth, "attempt %d must use the Blue account", i)
				require.Equal(t, "daybreak_blue", gjson.GetBytes(upstream.bodies[i], "access_programs.cyber").String(), "attempt %d must keep access_programs", i)
			}
		})
	}

	t.Run("chat completions chat-shape", func(t *testing.T) {
		upstream := &capabilityAdmissionUpstream{}
		h := newCapabilityAdmissionHandler(t, accounts(), upstream)
		c, _ := newCapabilityAdmissionContext(t, "/v1/chat/completions", `{"model":"gpt-6-sol","stream":false,"messages":[{"role":"user","content":"hi"}],"access_programs":{"cyber":"daybreak_blue"}}`, group)
		h.ChatCompletions(c)
		require.GreaterOrEqual(t, upstream.calls(), 1)
		require.Equal(t, "Bearer blue", upstream.auths[0])
		require.Equal(t, "daybreak_blue", gjson.GetBytes(upstream.bodies[0], "access_programs.cyber").String(), "Chat → Responses 重建保留 access_programs")
	})

	t.Run("messages", func(t *testing.T) {
		upstream := &capabilityAdmissionUpstream{}
		h := newCapabilityAdmissionHandler(t, accounts(), upstream)
		c, _ := newCapabilityAdmissionContext(t, "/v1/messages", `{"model":"gpt-6-sol","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"access_programs":{"cyber":"daybreak_blue"}}`, group)
		h.Messages(c)
		require.GreaterOrEqual(t, upstream.calls(), 1)
		require.Equal(t, "Bearer blue", upstream.auths[0])
		require.Equal(t, "daybreak_blue", gjson.GetBytes(upstream.bodies[0], "access_programs.cyber").String(), "Messages → Responses 重建保留 access_programs")
	})
}

// 强制 Fast 分组：白名单允许 Ultrafast 时准入通过，选号按 Fast 折叠（标准账号也可被选中）。
func TestOpenAICapabilityAdmission_ForceFastFoldsRoutingRequirement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &capabilityAdmissionUpstream{}
	h := newCapabilityAdmissionHandler(t, []service.Account{capabilityAdmissionAccount(1, service.AccountTypeOAuth, "standard", capabilityAdmissionStandardManifest)}, upstream)
	group := capabilityAdmissionGroup(service.GroupCapabilityAllowlist{Enabled: true, ServiceTiers: []string{"priority", "ultrafast"}})
	group.ForceOpenAIFast = true
	c, rec := newCapabilityAdmissionContext(t, "/v1/responses", `{"model":"gpt-6-sol","stream":false,"input":"hello","service_tier":"ultrafast"}`, group)
	h.Responses(c)
	require.NotEqual(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.GreaterOrEqual(t, upstream.calls(), 1)
	require.Equal(t, "Bearer standard", upstream.auths[0])
}
