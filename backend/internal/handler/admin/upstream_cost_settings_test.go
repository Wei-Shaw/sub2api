//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// upstreamCostSettingsStateStub 是一个内存版的状态表。
//
// 用真实现而不是 mock 框架：这个测试要验证的核心是「令牌加密后落库、
// 读出来能解密」，mock 掉存储就等于把要验的那一环换成了自己的假设。
type upstreamCostSettingsStateStub struct {
	values map[string]string
}

func newUpstreamCostSettingsStateStub() *upstreamCostSettingsStateStub {
	return &upstreamCostSettingsStateStub{values: map[string]string{}}
}

func (s *upstreamCostSettingsStateStub) Get(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *upstreamCostSettingsStateStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *upstreamCostSettingsStateStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			out[key] = v
		}
	}
	return out, nil
}

func setupUpstreamCostSettingsRouter(t *testing.T, encryptor service.SecretEncryptor) (*gin.Engine, *upstreamCostSettingsStateStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	state := newUpstreamCostSettingsStateStub()
	svc := service.NewReconciliationA6SettingsService(
		state,
		encryptor,
		service.ReconciliationA6Config{},
		7.1,
	)
	h := NewUpstreamCostSettingsHandler(svc)

	router := gin.New()
	router.GET("/upstream-cost/settings", h.Settings)
	router.PUT("/upstream-cost/settings", h.UpdateSettings)
	return router, state
}

// doRawJSON 发一段**原样**的 JSON 字符串。
//
// 刻意不复用同包里的 doJSON（它收 map[string]any）：本文件需要能发出
// 语法错误的请求体，来验证「坏 JSON 回 400 而不是 500」——用 map 构造不出坏 JSON。
func doRawJSON(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeSettingsData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	return payload.Data
}

// TestUpstreamCostSettingsHandlerNeverReturnsPlaintextToken 是本文件最重要的一条。
//
// 这个接口的响应体直接进浏览器，一旦把令牌原文序列化出去，等于把上游账号
// 交给任何一个能打开这个页面的管理员抓包带走。所以断言不只看「有没有 mask 字段」，
// 而是拿**整段响应体**去搜令牌原文——字段名以后怎么改都拦不住这条断言。
func TestUpstreamCostSettingsHandlerNeverReturnsPlaintextToken(t *testing.T) {
	router, state := setupUpstreamCostSettingsRouter(t, nil)

	const token = "sk-upstream-a6-secret-token-0123456789"
	// 事先把一个「已配置」的覆盖值写进去，走的是服务自己的加密路径。
	svc := service.NewReconciliationA6SettingsService(
		state,
		nil,
		service.ReconciliationA6Config{},
		7.1,
	)
	// 没有加密器时保存令牌必须**报错而不是落明文**。
	_, err := svc.Update(context.Background(), service.ReconciliationA6SettingsInput{
		BaseURL:     ptrTo("https://a6.example.com"),
		UserID:      ptrTo("42"),
		AccessToken: ptrTo(token),
	})
	require.ErrorIs(t, err, service.ErrReconciliationSecretEncryptorUnavailable)

	// 存储里不能留下明文。
	raw, _ := state.Get(context.Background(), service.ReconciliationStateKeyA6AccessTokenOverride)
	require.NotContains(t, raw, token, "加密器缺席时必须拒绝保存，绝不能落明文")

	rec := doRawJSON(t, router, http.MethodGet, "/upstream-cost/settings", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), token)
	require.NotContains(t, rec.Body.String(), "secret")

	data := decodeSettingsData(t, rec)
	require.Equal(t, "https://a6.example.com", data["a6_base_url"])
	require.Equal(t, false, data["a6_token_configured"])
}

// TestUpstreamCostSettingsHandlerSavesAndMasks 验证正常保存后，只有脱敏提示外露。
func TestUpstreamCostSettingsHandlerSavesAndMasks(t *testing.T) {
	router, _ := setupUpstreamCostSettingsRouter(t, stubUpstreamCostEncryptor{})

	const token = "sk-upstream-a6-secret-token-0123456789"
	rec := doRawJSON(t, router, http.MethodPut, "/upstream-cost/settings",
		`{"a6_base_url":"https://a6.example.com","a6_user_id":"42","a6_access_token":"`+token+`"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), token, "保存后的回包也不能带明文")

	data := decodeSettingsData(t, rec)
	require.Equal(t, true, data["a6_token_configured"])
	require.NotEmpty(t, data["a6_token_mask"])
	// override_keys 前端直接遍历，绝不能是 null。
	keys, ok := data["override_keys"].([]any)
	require.True(t, ok, "override_keys 必须是数组，null 会让前端崩掉")
	require.Contains(t, keys, "a6_access_token")

	// 再读一次：令牌不回显，但「已配置」状态要保持。
	getRec := doRawJSON(t, router, http.MethodGet, "/upstream-cost/settings", "")
	require.Equal(t, http.StatusOK, getRec.Code)
	require.NotContains(t, getRec.Body.String(), token)
	require.Equal(t, true, decodeSettingsData(t, getRec)["a6_token_configured"])
}

// TestUpstreamCostSettingsHandlerLeavesTokenUntouchedWhenOmitted
// 守住「留空 = 不改」这条语义：如果哪天把缺省字段当成空串处理，
// 人每次来改个地址就会顺手把令牌清掉，成本会整整一天取不到。
func TestUpstreamCostSettingsHandlerLeavesTokenUntouchedWhenOmitted(t *testing.T) {
	router, _ := setupUpstreamCostSettingsRouter(t, stubUpstreamCostEncryptor{})

	const token = "sk-upstream-a6-secret-token-0123456789"
	first := doRawJSON(t, router, http.MethodPut, "/upstream-cost/settings",
		`{"a6_base_url":"https://a6.example.com","a6_user_id":"42","a6_access_token":"`+token+`"}`)
	require.Equal(t, http.StatusOK, first.Code)

	// 只改地址，不带令牌字段。
	second := doRawJSON(t, router, http.MethodPut, "/upstream-cost/settings",
		`{"a6_base_url":"https://a6-2.example.com","a6_user_id":"42"}`)
	require.Equal(t, http.StatusOK, second.Code)
	data := decodeSettingsData(t, second)
	require.Equal(t, "https://a6-2.example.com", data["a6_base_url"])
	require.Equal(t, true, data["a6_token_configured"], "没传令牌时不能把已配置的令牌弄丢")
	require.NotContains(t, second.Body.String(), token)
}

// TestUpstreamCostSettingsHandlerRejectsInvalidBaseURL 验证 URL 校验被真正执行。
//
// 面板上填错地址是最常见的人为失误，而失败形态是「成本一直取不到」——
// 沉默、且指向完全无关的地方。所以必须在保存那一刻就挡下来。
func TestUpstreamCostSettingsHandlerRejectsInvalidBaseURL(t *testing.T) {
	router, state := setupUpstreamCostSettingsRouter(t, stubUpstreamCostEncryptor{})

	rec := doRawJSON(t, router, http.MethodPut, "/upstream-cost/settings",
		`{"a6_base_url":"a6.example.com"}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "UPSTREAM_COST_INVALID_BASE_URL")
	// 校验失败不能留下半截写入。
	require.Empty(t, state.values)
}

// TestUpstreamCostSettingsHandlerRejectsMalformedJSON 验证坏请求体是 400 而不是 500。
func TestUpstreamCostSettingsHandlerRejectsMalformedJSON(t *testing.T) {
	router, _ := setupUpstreamCostSettingsRouter(t, stubUpstreamCostEncryptor{})
	rec := doRawJSON(t, router, http.MethodPut, "/upstream-cost/settings", `{"a6_base_url":`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "UPSTREAM_COST_BAD_REQUEST")
}

// stubUpstreamCostEncryptor 是只做可逆标记的假加密器。
//
// 刻意让密文**不等于**明文（加前缀），这样「有没有加密」这件事是可断言的；
// 真加密算法的强度不属于本测试的范围。
type stubUpstreamCostEncryptor struct{}

func (stubUpstreamCostEncryptor) Encrypt(plaintext string) (string, error) {
	return "enc::" + plaintext, nil
}

func (stubUpstreamCostEncryptor) Decrypt(ciphertext string) (string, error) {
	return strings.TrimPrefix(ciphertext, "enc::"), nil
}

func ptrTo[T any](v T) *T { return &v }
