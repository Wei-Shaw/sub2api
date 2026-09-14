//go:build unit

package service

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// withStubbedAdobeTestClient 把账号测试用的 Adobe 客户端换成假传输，避免打真网络。
// 用 t.Cleanup 还原，防止污染同包其它测试。
func withStubbedAdobeTestClient(t *testing.T, client *adobe.Client) {
	t.Helper()
	prev := adobeTestClients
	adobeTestClients = &adobeClientCache{
		newClient: func(string) *adobe.Client { return client },
	}
	t.Cleanup(func() { adobeTestClients = prev })
}

func adobeTestConnAccount(credentials map[string]any) *Account {
	if credentials == nil {
		credentials = map[string]any{
			"cookie":       "aux_sid=abc",
			"access_token": "adobe-access-token",
		}
	}
	return &Account{
		ID:          21,
		Platform:    domain.PlatformAdobe,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: credentials,
	}
}

func runAdobeTestConn(t *testing.T, account *Account, modelID, prompt string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	svc := &AccountTestService{accountRepo: repo}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/21/test", nil)

	err := svc.TestAccountConnection(c, account.ID, modelID, prompt, AccountTestModeDefault)
	return rec, err
}

// Adobe 账号必须走真出图路径并回传图片，而不是落到 Claude 兜底去打 api.anthropic.com。
func TestTestAccountConnectionAdobeGeneratesImage(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("PNGDATA"))
	withStubbedAdobeTestClient(t, client)

	rec, err := runAdobeTestConn(t, adobeTestConnAccount(nil), "gpt-image-2", "")
	require.NoError(t, err)

	body := rec.Body.String()
	require.Contains(t, body, `"type":"test_start"`)
	require.Contains(t, body, `"model":"gpt-image-2"`)
	require.Contains(t, body, `"type":"image"`)
	require.Contains(t, body, "data:image/png;base64,")
	require.Contains(t, body, `"type":"test_complete"`)
	require.Contains(t, body, `"success":true`)

	// 改前 Adobe 会落到 testClaudeAccountConnection —— 这条钉死回归。
	require.NotContains(t, body, "anthropic")
	require.NotContains(t, body, "claude")
}

// status 事件必须暴露映射结果：运维要能直接看到「我选的 2.5-flare 实际打到哪个
// upstream modelVersion」。这是排查静默降级的手段（Step 7 那个 bug 的复发探针）。
func TestTestAccountConnectionAdobeSurfacesUpstreamModelVersion(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	withStubbedAdobeTestClient(t, client)

	rec, err := runAdobeTestConn(t, adobeTestConnAccount(nil), "gpt-image-2.5-flare", "")
	require.NoError(t, err)

	body := rec.Body.String()
	require.Contains(t, body, "gpt-image-2.5-flare",
		"status 事件应显示真实的 upstream modelVersion，降级成 2 时这条会失败")
	require.Contains(t, body, "firefly-gpt-image-2-5-flare")
}

// 模型留空时取 ImageModelIDs 的第一项（与前端默认选中一致）。
func TestTestAccountConnectionAdobeDefaultsToFirstModel(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	withStubbedAdobeTestClient(t, client)

	rec, err := runAdobeTestConn(t, adobeTestConnAccount(nil), "", "")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), `"model":"`+adobe.ImageModelIDs()[0]+`"`)
}

// access_token 在账号表单上是可选的（文案：留空则首次刷新时用 cookie 自动换取）。
// 刚建好的账号必然没有 token，测试必须当场用 cookie 换一个，而不是让运维干等后台刷新器。
func TestTestAccountConnectionAdobeExchangesTokenFromCookie(t *testing.T) {
	imageBytes := []byte("PNGDATA")
	refreshed := adobeTestJWT(t, time.Now().Add(12*time.Hour))

	var refreshCalls int
	api := &adobeFakeTransport{}
	api.handler = func(req *adobe.Request, _ int) (*adobe.Response, error) {
		// IMS 换 token
		if strings.Contains(req.URL, "ims/check/v6/token") {
			refreshCalls++
			return adobeJSONResponse(t, 200, map[string]any{
				"access_token": refreshed,
				"expires_in":   86400,
				"token_type":   "bearer",
			}, nil), nil
		}
		// 提交
		if strings.Contains(req.URL, "generate-async") {
			return adobeJSONResponse(t, 200, map[string]any{},
				map[string]string{"x-override-status-link": "https://poll/x"}), nil
		}
		// 轮询
		return adobeJSONResponse(t, 200, map[string]any{
			"status":  "COMPLETED",
			"outputs": []any{map[string]any{"image": map[string]any{"presignedUrl": "https://cdn/img.png"}}},
		}, nil), nil
	}
	download := &adobeFakeTransport{handler: func(*adobe.Request, int) (*adobe.Response, error) {
		return &adobe.Response{StatusCode: 200, Headers: map[string]string{}, Body: imageBytes}, nil
	}}
	withStubbedAdobeTestClient(t, adobe.NewClient(adobe.ClientConfig{Transport: api, DownloadTransport: download}))

	account := adobeTestConnAccount(map[string]any{"cookie": "aux_sid=abc", "access_token": ""})
	rec, err := runAdobeTestConn(t, account, "gpt-image-2", "")
	require.NoError(t, err)

	body := rec.Body.String()
	require.Equal(t, 1, refreshCalls, "应当场用 cookie 换一次 token")
	require.Contains(t, body, "exchanging a fresh one from the cookie")
	require.Contains(t, body, `"type":"image"`)
	require.Contains(t, body, `"success":true`)

	// 换到的 token 必须落库，下次测试与生产请求直接复用。
	require.Equal(t, refreshed, account.GetCredential("access_token"))
	// cookie 不能被刷新流程抹掉（MergeCredentials 的契约）。
	require.Equal(t, "aux_sid=abc", account.GetCredential("cookie"))
}

// token 与 cookie 都没有才是真的没救——此时短路报错，不该打上游。
func TestTestAccountConnectionAdobeNoCredentialsShortCircuits(t *testing.T) {
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	withStubbedAdobeTestClient(t, client)

	account := adobeTestConnAccount(map[string]any{"cookie": "", "access_token": ""})
	rec, err := runAdobeTestConn(t, account, "gpt-image-2", "")
	require.Error(t, err)
	require.Contains(t, rec.Body.String(), `"type":"error"`)
	require.Contains(t, rec.Body.String(), "cookie")
	require.Empty(t, api.calls, "无凭据时不该发起上游请求")
}

// 已有未过期 token 时直接复用，不该白刷一次 IMS。
func TestTestAccountConnectionAdobeReusesLiveToken(t *testing.T) {
	live := adobeTestJWT(t, time.Now().Add(12*time.Hour))
	api := &adobeFakeTransport{}
	client := adobeSubmitPollDownload(t, api, []byte("X"))
	withStubbedAdobeTestClient(t, client)

	account := adobeTestConnAccount(map[string]any{"cookie": "aux_sid=abc", "access_token": live})
	rec, err := runAdobeTestConn(t, account, "gpt-image-2", "")
	require.NoError(t, err)

	for _, call := range api.calls {
		require.NotContains(t, call.URL, "ims/check/v6/token", "未过期的 token 不该触发刷新")
	}
	require.Contains(t, rec.Body.String(), `"success":true`)
}

// adobeTestJWT 造一个带 exp 的最小 JWT，供过期判定使用。
func adobeTestJWT(t *testing.T, exp time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(fmt.Sprintf(`{"user_id":"u","exp":%d}`, exp.Unix())))
	return header + "." + payload + ".sig"
}

// 类型化错误要翻译成运维看得懂的处置建议，而不是把上游原文直接甩出去。
func TestFormatAdobeTestError(t *testing.T) {
	authMsg := formatAdobeTestError(adobe.NewAuthError("expired", http.StatusUnauthorized))
	require.Contains(t, authMsg, "cookie")

	quotaMsg := formatAdobeTestError(adobe.NewQuotaExhaustedError("taste_exhausted", http.StatusForbidden))
	require.Contains(t, quotaMsg, "credits")

	tempMsg := formatAdobeTestError(
		adobe.NewUpstreamTemporaryError("bad gateway", http.StatusBadGateway, adobe.ErrorTypeStatus))
	require.Contains(t, tempMsg, "temporarily")

	rejectedMsg := formatAdobeTestError(adobe.NewContentRejectedError(
		`poll failed: 451 {"error_code":"image_unsafe"}`, http.StatusUnavailableForLegalReasons, ""))
	require.Contains(t, rejectedMsg, "content")
	require.NotContains(t, rejectedMsg, "temporarily")

	entitledMsg := formatAdobeTestError(adobe.NewNotEntitledError(
		`submit failed: 403 {"error_code":"model_not_entitled"}`, http.StatusForbidden, ""))
	require.Contains(t, entitledMsg, "not entitled")
	require.NotContains(t, strings.ToLower(entitledMsg), "cookie")

	// 未知错误原样透出，不吞。
	require.Equal(t, "boom", formatAdobeTestError(adobe.NewRequestError("boom")))
}

func TestResolveAdobeTestPrompt(t *testing.T) {
	require.Equal(t, adobeTestDefaultPrompt, resolveAdobeTestPrompt(""))
	require.Equal(t, adobeTestDefaultPrompt, resolveAdobeTestPrompt("   "))
	require.Equal(t, "custom", resolveAdobeTestPrompt("  custom  "))
}
