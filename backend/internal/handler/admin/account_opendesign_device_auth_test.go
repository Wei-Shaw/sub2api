package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// OpenDesign 一键设备授权管理端接口测试。

// openDesignTestPrefix mirrors the production mount point
// (/api/v1/admin + /accounts). Registering the same constants the router uses
// keeps the tests honest about the real URL shape.
const openDesignTestPrefix = "/api/v1/admin/accounts"

func registerOpenDesignRoutes(handler *AccountHandler) *gin.Engine {
	router := gin.New()
	accounts := router.Group(openDesignTestPrefix)
	accounts.POST(OpenDesignDeviceAuthRoute, handler.CreateOpenDesignDeviceAuth)
	accounts.GET(OpenDesignDeviceAuthStatusRoute, handler.GetOpenDesignDeviceAuth)
	accounts.DELETE(OpenDesignDeviceAuthStatusRoute, handler.CancelOpenDesignDeviceAuth)
	accounts.POST(OpenDesignAccountsRoute, handler.CreateOpenDesignAccountFromSession)
	return router
}

type openDesignFakeUpstream struct {
	authorized bool
	createHits int32
	walletHits int32
}

func (f *openDesignFakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/cli/device-authorizations"):
		atomic.AddInt32(&f.createHits, 1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"deviceId":"dev_1","userCode":"GQRSUT","status":"pending",` +
			`"expiresAt":"2099-01-01T00:00:00Z","deviceSecret":"secret-1",` +
			`"activationUrl":"https://open-design.ai/cloud/cli/activate?userCode=GQRSUT","pollIntervalSeconds":2}`))
	case strings.HasSuffix(r.URL.Path, "/token"):
		if f.authorized {
			_, _ = w.Write([]byte(`{"status":"authorized","controlKey":"vela_ctrl_x","runtimeKey":"sk-amr-x",` +
				`"user":{"email":"dev@example.com"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	case strings.HasSuffix(r.URL.Path, "/wallet/balance"):
		atomic.AddInt32(&f.walletHits, 1)
		_, _ = w.Write([]byte(`{"workspaceId":"wstestworkspace01","balanceUsd":"1"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func newOpenDesignTestHandler(t *testing.T, upstream *openDesignFakeUpstream) (*AccountHandler, *stubAdminService) {
	t.Helper()
	server := httptest.NewServer(upstream)
	t.Cleanup(server.Close)
	t.Setenv("OPENDESIGN_AUTH_API_BASE_URL", server.URL)
	t.Setenv("OPENDESIGN_CLI_VERSION", "0.1.6")

	authSvc := service.NewOpenDesignDeviceAuthService()
	t.Cleanup(authSvc.Stop)

	stub := newStubAdminService()
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.adminService = stub
	handler.SetOpenDesignDeviceAuthService(authSvc)
	return handler, stub
}

func authorizeOpenDesignSession(t *testing.T, handler *AccountHandler, upstream *openDesignFakeUpstream) string {
	t.Helper()
	router := registerOpenDesignRoutes(handler)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/device-auth", bytes.NewBufferString(`{"host":"h"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var created struct {
		Data struct {
			SessionID     string `json:"sessionId"`
			UserCode      string `json:"userCode"`
			ActivationURL string `json:"activationUrl"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &created))
	require.NotEmpty(t, created.Data.SessionID)
	require.NotEmpty(t, created.Data.ActivationURL)
	// 创建响应不得包含任何敏感凭据
	body := recorder.Body.String()
	for _, secret := range []string{"secret-1", "deviceSecret", "controlKey", "runtimeKey"} {
		require.NotContains(t, body, secret)
	}

	// 用户在浏览器完成授权
	upstream.authorized = true

	statusRouter := registerOpenDesignRoutes(handler)
	statusReq := httptest.NewRequest(http.MethodGet, openDesignTestPrefix+"/opendesign/device-auth/"+created.Data.SessionID, nil)
	statusRec := httptest.NewRecorder()
	statusRouter.ServeHTTP(statusRec, statusReq)
	require.Equal(t, http.StatusOK, statusRec.Code, statusRec.Body.String())

	var status struct {
		Data struct {
			Status      string `json:"status"`
			Email       string `json:"email"`
			WorkspaceID string `json:"workspaceId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(statusRec.Body.Bytes(), &status))
	require.Equal(t, "authorized", status.Data.Status)
	require.Equal(t, "dev@example.com", status.Data.Email)
	require.Equal(t, "wstestworkspace01", status.Data.WorkspaceID)
	require.NotContains(t, statusRec.Body.String(), "vela_ctrl_x", "controlKey 绝不能下发前端")

	return created.Data.SessionID
}

func TestOpenDesignDeviceAuthCreatesAPIKeyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`","group_ids":[66],"priority":40}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

	require.Len(t, stub.createdAccounts, 1)
	input := stub.createdAccounts[0]
	// §3.4：落库 type 必须是 apikey，即使 UI 上选的是 OAuth
	require.Equal(t, service.AccountTypeAPIKey, input.Type)
	require.Equal(t, service.PlatformOpenDesign, input.Platform)
	require.Equal(t, "sk-amr-x", input.Credentials["api_key"])
	require.Equal(t, "vela_ctrl_x", input.Credentials[service.OpenDesignUsageCredentialsKey])
	require.Equal(t, "wstestworkspace01", input.Credentials[service.OpenDesignUsageWorkspaceKey])
	require.Equal(t, service.DefaultOpenDesignBaseURL, input.Credentials["base_url"])
	require.Equal(t, service.APIProtocolAdaptive, input.Credentials["api_protocol"])
	require.Equal(t, "dev@example.com", input.Credentials["email"])
	require.Equal(t, service.OpenDesignAuthSourceOAuth, input.Extra[service.OpenDesignAuthSourceExtraKey])
	require.Equal(t, []int64{66}, input.GroupIDs)
	require.Equal(t, 40, input.Priority)
	// 邮箱自动生成账号名
	require.Equal(t, "OpenDesign-go-dev@example.com", input.Name)

	// 响应里不得回显 controlKey / runtimeKey
	require.NotContains(t, recorder.Body.String(), "vela_ctrl_x")
	require.NotContains(t, recorder.Body.String(), "sk-amr-x")

	// 会话重放必须被拒
	replayRec := httptest.NewRecorder()
	replayReq := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`"}`,
	))
	replayReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(replayRec, replayReq)
	require.Equal(t, http.StatusConflict, replayRec.Code, replayRec.Body.String())

	var errBody struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(replayRec.Body.Bytes(), &errBody))
	require.Equal(t, "OPENDESIGN_SESSION_CONSUMED", errBody.Reason)
	require.Len(t, stub.createdAccounts, 1, "重放不得再建账号")
}

func TestOpenDesignDeviceAuthUpdatesExistingAccountByEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	stub.accounts = []service.Account{{
		ID:       901,
		Name:     "OpenDesign-go-dev@example.com",
		Platform: service.PlatformOpenDesign,
		Type:     service.AccountTypeAPIKey,
	}}
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
	require.Empty(t, stub.createdAccounts, "同邮箱重复授权应更新而非新建")
	require.EqualValues(t, 1, stub.updateAccountCalls)
	require.NotNil(t, stub.lastUpdateAccountInput)
	// 精确更新三键 + email，不能整份覆盖 credentials
	require.Equal(t, map[string]any{
		"api_key":                             "sk-amr-x",
		service.OpenDesignUsageCredentialsKey: "vela_ctrl_x",
		service.OpenDesignUsageWorkspaceKey:   "wstestworkspace01",
		"email":                               "dev@example.com",
	}, stub.lastUpdateAccountInput.Credentials)
	require.Equal(t, service.AccountTypeAPIKey, stub.lastUpdateAccountInput.Type)
	require.EqualValues(t, 1, stub.updateAccountExtraCalls)
}

func TestOpenDesignNameMatchesEmailRequiresSeparator(t *testing.T) {
	// 本功能之前建的老账号名形如 OpenDesign-go-<邮箱>，没有 credentials.email，
	// 只能靠名字兜底匹配——必须带分隔符校验，避免 a@b.com 误吃 xa@b.com。
	require.True(t, openDesignNameMatchesEmail("OpenDesign-go-dev@example.com", "dev@example.com"))
	require.True(t, openDesignNameMatchesEmail("opendesign-go-DEV@Example.com", "dev@example.com"))
	require.True(t, openDesignNameMatchesEmail("dev@example.com", "dev@example.com"))
	require.False(t, openDesignNameMatchesEmail("OpenDesign-go-xdev@example.com", "dev@example.com"))
	require.False(t, openDesignNameMatchesEmail("OpenDesign-go-other@example.com", "dev@example.com"))
	require.False(t, openDesignNameMatchesEmail("", "dev@example.com"))
}

// 同一授权邮箱命中多个 open_design 账号时必须拒绝，绝不能用「第一条命中」猜目标账号。
func TestOpenDesignDeviceAuthRejectsAmbiguousEmailMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	stub.accounts = []service.Account{
		{
			ID: 910, Name: "od-a", Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"email": "dev@example.com"},
		},
		{
			ID: 911, Name: "od-b", Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{"email": "dev@example.com"},
		},
	}
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var errBody struct {
		Reason   string            `json:"reason"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	require.Equal(t, "OPENDESIGN_ACCOUNT_AMBIGUOUS", errBody.Reason)
	// 候选账号 ID 必须真的带上（只断言字段存在无法发现空值/错值）
	require.Equal(t, "910,911", errBody.Metadata["candidate_account_ids"])
	require.Empty(t, stub.createdAccounts, "歧义场景不得建号")
	require.EqualValues(t, 0, stub.updateAccountCalls, "歧义场景不得更新任何账号")
}

// 建号弹窗里可见的 5 个通用字段必须逐字段落到 CreateAccountInput（非默认值也一致）。
func TestOpenDesignDeviceAuthPassesSharedAccountFieldsOnCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`","proxy_id":42,"rate_multiplier":0.5,"load_factor":20,`+
			`"expires_at":1893456000,"auto_pause_on_expired":false}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, stub.createdAccounts, 1)
	input := stub.createdAccounts[0]
	require.NotNil(t, input.ProxyID)
	require.EqualValues(t, 42, *input.ProxyID)
	require.NotNil(t, input.RateMultiplier)
	require.Equal(t, 0.5, *input.RateMultiplier)
	require.NotNil(t, input.LoadFactor)
	require.Equal(t, 20, *input.LoadFactor)
	require.NotNil(t, input.ExpiresAt)
	require.EqualValues(t, 1893456000, *input.ExpiresAt)
	require.NotNil(t, input.AutoPauseOnExpired)
	require.False(t, *input.AutoPauseOnExpired)
}

// 创建路径的 proxy_id=0 是「无代理」，不能落成外键 0。
func TestOpenDesignDeviceAuthNormalizesZeroProxyOnCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`","proxy_id":0,"rate_multiplier":0}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, stub.createdAccounts, 1)
	input := stub.createdAccounts[0]
	require.Nil(t, input.ProxyID, "proxy_id=0 必须归一为无代理")
	require.NotNil(t, input.RateMultiplier)
	require.Equal(t, 0.0, *input.RateMultiplier, "rate_multiplier=0（免费）不得被吞")
}

// workspaceId 是更强的身份：命中唯一 workspace 时可直接更新，无需邮箱消歧。
func TestOpenDesignDeviceAuthPrefersWorkspaceMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	stub.accounts = []service.Account{
		{
			ID: 920, Name: "od-workspace", Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{service.OpenDesignUsageWorkspaceKey: "wstestworkspace01"},
		},
		{
			ID: 921, Name: "od-other", Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
			Credentials: map[string]any{service.OpenDesignUsageWorkspaceKey: "wsother"},
		},
	}
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.EqualValues(t, 1, stub.updateAccountCalls)
	require.Empty(t, stub.createdAccounts)
}

// 账号已有权威 credentials.email 时，名称后缀不再参与匹配（避免旧名称误吃）。
func TestOpenDesignDeviceAuthNameFallbackOnlyWithoutStoredEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	stub.accounts = []service.Account{{
		ID: 930,
		// 名称后缀命中授权邮箱，但账号自身记录的是另一个邮箱
		Name:        "OpenDesign-go-dev@example.com",
		Platform:    service.PlatformOpenDesign,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"email": "someone-else@example.com"},
	}}
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, stub.createdAccounts, 1, "权威邮箱不匹配时应新建，不得按名称覆盖既有账号")
	require.EqualValues(t, 0, stub.updateAccountCalls)
}

// 更新路径也必须透传混合渠道确认标志，否则确认后仍会重复 409。
func TestOpenDesignDeviceAuthPassesMixedChannelConfirmationOnUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	stub.accounts = []service.Account{{
		ID: 940, Name: "od-single", Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"email": "dev@example.com"},
	}}
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+sessionID+`","group_ids":[66],"confirm_mixed_channel":true}`,
	))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.EqualValues(t, 1, stub.updateAccountCalls)
	require.NotNil(t, stub.lastUpdateAccountInput)
	require.True(t, stub.lastUpdateAccountInput.SkipMixedChannelCheck, "确认标志必须透传到更新路径")
}

func TestOpenDesignDeviceAuthRequiresAuthorizedSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)

	router := registerOpenDesignRoutes(handler)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/device-auth", bytes.NewBufferString(`{"host":"h"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	var created struct {
		Data struct {
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &created))

	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts", bytes.NewBufferString(
		`{"session_id":"`+created.Data.SessionID+`"}`,
	))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRec, createReq)

	require.Equal(t, http.StatusConflict, createRec.Code, createRec.Body.String())
	var errBody struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &errBody))
	require.Equal(t, "OPENDESIGN_SESSION_NOT_AUTHORIZED", errBody.Reason)
	require.Empty(t, stub.createdAccounts)
}

func TestOpenDesignDeviceAuthUnknownSessionAndCancel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, _ := newOpenDesignTestHandler(t, upstream)

	router := registerOpenDesignRoutes(handler)

	// 不存在的会话 → 404（服务重启后前端拿 404，提示重新发起）
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, openDesignTestPrefix+"/opendesign/device-auth/ods_missing", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)

	// 取消后立即失效
	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/device-auth", bytes.NewBufferString(`{}`))
	createReq.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRec, createReq)
	require.Equal(t, http.StatusOK, createRec.Code, createRec.Body.String())

	var created struct {
		Data struct {
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(createRec.Body.Bytes(), &created))

	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, openDesignTestPrefix+"/opendesign/device-auth/"+created.Data.SessionID, nil))
	require.Equal(t, http.StatusOK, delRec.Code)

	afterRec := httptest.NewRecorder()
	router.ServeHTTP(afterRec, httptest.NewRequest(http.MethodGet, openDesignTestPrefix+"/opendesign/device-auth/"+created.Data.SessionID, nil))
	require.Equal(t, http.StatusNotFound, afterRec.Code)
}

// 保证测试编译期就锁死「服务未注入时 fail-closed」的行为。
func TestOpenDesignDeviceAuthRequiresService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := registerOpenDesignRoutes(handler)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/device-auth", nil)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.NotEqual(t, http.StatusOK, rec.Code)
}

// 重新授权只应覆盖凭据相关的键：既有 model_mapping / 协议端点 / 账号模式必须原样保留
// （UpdateAccount 的非敏感键语义是"完全由入参决定"，只传三键会把它们清空）。
func TestOpenDesignReauthPreservesExistingCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &openDesignFakeUpstream{}
	handler, stub := newOpenDesignTestHandler(t, upstream)
	stub.accounts = []service.Account{{
		ID:       901,
		Name:     "OpenDesign-go-dev@example.com",
		Platform: service.PlatformOpenDesign,
		Type:     service.AccountTypeAPIKey,
	}}
	existingCreds := map[string]any{
		"base_url":      service.DefaultOpenDesignBaseURL,
		"api_protocol":  service.APIProtocolAdaptive,
		"account_mode":  service.AccountModePayG,
		"model_mapping": map[string]any{"deepseek-v4-pro": "od-pro"},
		"api_base_urls": map[string]any{service.APIProtocolAnthropic: service.DefaultOpenDesignAnthropicBaseURL},
	}
	stub.getAccountResult = &service.Account{
		ID:          901,
		Name:        "OpenDesign-go-dev@example.com",
		Platform:    service.PlatformOpenDesign,
		Type:        service.AccountTypeAPIKey,
		Credentials: existingCreds,
	}
	sessionID := authorizeOpenDesignSession(t, handler, upstream)

	router := registerOpenDesignRoutes(handler)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, openDesignTestPrefix+"/opendesign/accounts",
		bytes.NewBufferString(`{"session_id":"`+sessionID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())

	require.NotNil(t, stub.lastUpdateAccountInput)
	got := stub.lastUpdateAccountInput.Credentials
	require.Equal(t, "sk-amr-x", got["api_key"])
	require.Equal(t, "vela_ctrl_x", got[service.OpenDesignUsageCredentialsKey])
	require.Equal(t, "wstestworkspace01", got[service.OpenDesignUsageWorkspaceKey])
	for key, want := range existingCreds {
		require.Equal(t, want, got[key], "re-auth must keep %s", key)
	}
}
