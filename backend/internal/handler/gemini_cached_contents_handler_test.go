//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type geminiCachedContentHandlerRepoStub struct {
	records []*service.GeminiCachedContent
}

func (r *geminiCachedContentHandlerRepoStub) Create(_ context.Context, record *service.GeminiCachedContent) error {
	record.ID = int64(len(r.records) + 1)
	r.records = append(r.records, record)
	return nil
}

func (r *geminiCachedContentHandlerRepoStub) GetForOwner(_ context.Context, apiKeyID int64, publicID string) (*service.GeminiCachedContent, error) {
	for _, record := range r.records {
		if record.PublicID == publicID && record.APIKeyID == apiKeyID {
			return record, nil
		}
	}
	return nil, service.ErrGeminiCachedContentNotFound
}

func (r *geminiCachedContentHandlerRepoStub) ListForOwner(_ context.Context, apiKeyID, groupID int64, activeAt time.Time, beforeID int64, limit int) ([]*service.GeminiCachedContent, error) {
	var out []*service.GeminiCachedContent
	for i := len(r.records) - 1; i >= 0; i-- {
		record := r.records[i]
		if record.APIKeyID == apiKeyID && record.GroupID == groupID && record.ExpireTime.After(activeAt) && (beforeID <= 0 || record.ID < beforeID) {
			out = append(out, record)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (r *geminiCachedContentHandlerRepoStub) UpdateExpireTime(context.Context, int64, time.Time) (time.Time, error) {
	return time.Time{}, nil
}

func (r *geminiCachedContentHandlerRepoStub) SoftDelete(context.Context, int64) error {
	return nil
}

func newGeminiCachedContentHandlerForTest(repo *geminiCachedContentHandlerRepoStub) *GatewayHandler {
	return &GatewayHandler{geminiCachedContentService: service.NewGeminiCachedContentService(repo, nil, &config.Config{})}
}

func geminiCachedContentTestAPIKey(platform string) *service.APIKey {
	groupID := int64(42)
	return &service.APIKey{ID: 7, UserID: 3, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: platform}}
}

func newGeminiCachedContentTestContext(method, target string, apiKey *service.APIKey) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 10})
	return c, rec
}

func seedGeminiCachedContent(repo *geminiCachedContentHandlerRepoStub, publicID string, apiKeyID int64, expire time.Time) *service.GeminiCachedContent {
	record := &service.GeminiCachedContent{
		PublicID:        publicID,
		UserID:          3,
		APIKeyID:        apiKeyID,
		GroupID:         42,
		AccountID:       900,
		UpstreamName:    "cachedContents/upstream" + publicID,
		Model:           "gemini-3.8-flash",
		UpstreamModel:   "models/gemini-3.8-flash",
		TotalTokenCount: 6273,
		ExpireTime:      expire,
		CreatedAt:       time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC),
	}
	_ = repo.Create(context.Background(), record)
	return record
}

func TestResolveGeminiCachedContentBinding(t *testing.T) {
	repo := &geminiCachedContentHandlerRepoStub{}
	seedGeminiCachedContent(repo, "abc", 7, time.Now().Add(time.Hour))
	seedGeminiCachedContent(repo, "foreign", 8, time.Now().Add(time.Hour))
	h := newGeminiCachedContentHandlerForTest(repo)
	apiKey := geminiCachedContentTestAPIKey(service.PlatformGemini)

	t.Run("request without cache passes through", func(t *testing.T) {
		c, _ := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
		body := []byte(`{"contents":[]}`)
		record, out, ok := h.resolveGeminiCachedContentBinding(c, zap.NewNop(), apiKey, "gemini-3.8-flash", body)
		require.True(t, ok)
		require.Nil(t, record)
		require.Equal(t, body, out)
	})

	t.Run("owned cache rewrites reference and binds account", func(t *testing.T) {
		c, _ := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
		record, out, ok := h.resolveGeminiCachedContentBinding(c, zap.NewNop(), apiKey, "gemini-3.8-flash",
			[]byte(`{"cachedContent":"cachedContents/abc","contents":[]}`))
		require.True(t, ok)
		require.Equal(t, int64(900), record.AccountID)
		require.JSONEq(t, `{"cachedContent":"cachedContents/upstreamabc","contents":[]}`, string(out))
	})

	t.Run("cache owned by another key looks missing", func(t *testing.T) {
		c, rec := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
		_, _, ok := h.resolveGeminiCachedContentBinding(c, zap.NewNop(), apiKey, "gemini-3.8-flash",
			[]byte(`{"cachedContent":"cachedContents/foreign","contents":[]}`))
		require.False(t, ok)
		require.Equal(t, http.StatusForbidden, rec.Code)
		require.Contains(t, rec.Body.String(), service.GeminiCachedContentNotFoundMessage)
	})

	t.Run("model must match the cache literally", func(t *testing.T) {
		c, rec := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-flash-latest:generateContent", apiKey)
		_, _, ok := h.resolveGeminiCachedContentBinding(c, zap.NewNop(), apiKey, "gemini-flash-latest",
			[]byte(`{"cachedContent":"cachedContents/abc","contents":[]}`))
		require.False(t, ok)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), "has to be the same")
	})

	t.Run("composite groups are rejected", func(t *testing.T) {
		composite := geminiCachedContentTestAPIKey(service.PlatformComposite)
		c, rec := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", composite)
		_, _, ok := h.resolveGeminiCachedContentBinding(c, zap.NewNop(), composite, "gemini-3.8-flash",
			[]byte(`{"cachedContent":"cachedContents/abc","contents":[]}`))
		require.False(t, ok)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestGeminiCachedContentsListAndGet(t *testing.T) {
	repo := &geminiCachedContentHandlerRepoStub{}
	seedGeminiCachedContent(repo, "first", 7, time.Now().Add(time.Hour))
	seedGeminiCachedContent(repo, "second", 7, time.Now().Add(time.Hour))
	seedGeminiCachedContent(repo, "expired", 7, time.Now().Add(-time.Minute))
	seedGeminiCachedContent(repo, "foreign", 8, time.Now().Add(time.Hour))
	h := newGeminiCachedContentHandlerForTest(repo)
	apiKey := geminiCachedContentTestAPIKey(service.PlatformGemini)

	c, rec := newGeminiCachedContentTestContext(http.MethodGet, "/v1beta/cachedContents?pageSize=1", apiKey)
	h.GeminiCachedContentsList(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var page struct {
		CachedContents []map[string]any `json:"cachedContents"`
		NextPageToken  string           `json:"nextPageToken"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.CachedContents, 1)
	require.Equal(t, "cachedContents/second", page.CachedContents[0]["name"])
	require.NotEmpty(t, page.NextPageToken)

	c, rec = newGeminiCachedContentTestContext(http.MethodGet, "/v1beta/cachedContents?pageSize=1&pageToken="+page.NextPageToken, apiKey)
	h.GeminiCachedContentsList(c)
	require.Equal(t, http.StatusOK, rec.Code)
	page.NextPageToken = ""
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &page))
	require.Len(t, page.CachedContents, 1)
	require.Equal(t, "cachedContents/first", page.CachedContents[0]["name"])
	require.Empty(t, page.NextPageToken, "过期与其他 Key 的缓存不出现在列表里")

	c, rec = newGeminiCachedContentTestContext(http.MethodGet, "/v1beta/cachedContents/first", apiKey)
	c.Params = gin.Params{{Key: "cacheID", Value: "first"}}
	h.GeminiCachedContentsGet(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var view map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Equal(t, "cachedContents/first", view["name"])
	require.Equal(t, "models/gemini-3.8-flash", view["model"])
	require.NotContains(t, rec.Body.String(), "upstreamfirst", "上游资源名不暴露给客户端")

	c, rec = newGeminiCachedContentTestContext(http.MethodGet, "/v1beta/cachedContents/foreign", apiKey)
	c.Params = gin.Params{{Key: "cacheID", Value: "foreign"}}
	h.GeminiCachedContentsGet(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.True(t, strings.Contains(rec.Body.String(), service.GeminiCachedContentNotFoundMessage))
}

func TestHandleGeminiCachedContentFailoverExhausted(t *testing.T) {
	h := &GatewayHandler{}
	apiKey := geminiCachedContentTestAPIKey(service.PlatformGemini)
	unavailable := `{"error":{"code":503,"message":"The account holding this cached content is temporarily unavailable","status":"UNAVAILABLE"}}`

	c, rec := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
	h.handleGeminiCachedContentFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, ResponseBody: []byte(unavailable), BoundUpstreamPassthrough: true})
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "绑定缓存的请求无账号可换，上游网关的 503 原样返回")
	require.JSONEq(t, unavailable, rec.Body.String())

	c, rec = newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
	h.handleGeminiCachedContentFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseBody: []byte(`{"error":{"code":429,"message":"Too many pending requests, please retry later","status":"RESOURCE_EXHAUSTED"}}`), BoundUpstreamPassthrough: true})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), "Too many pending requests")

	officialQuota := `{"error":{"code":429,"message":"You exceeded your current quota","status":"RESOURCE_EXHAUSTED","details":[{"@type":"type.googleapis.com/google.rpc.QuotaFailure","violations":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}]}}`
	c, rec = newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
	h.handleGeminiCachedContentFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, ResponseBody: []byte(officialQuota)})
	require.Equal(t, http.StatusTooManyRequests, rec.Code, "官方账号的限流按普通 Gemini 路径映射")
	require.NotContains(t, rec.Body.String(), "FreeTier", "不向客户端暴露上游账号的配额明细")
	require.Contains(t, rec.Body.String(), "Upstream rate limit exceeded")

	c, rec = newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/models/gemini-3.8-flash:generateContent", apiKey)
	h.handleGeminiCachedContentFailoverExhausted(c, &service.UpstreamFailoverError{StatusCode: http.StatusUnauthorized, ResponseBody: []byte(`{"error":{"code":401,"message":"bad key"}}`)})
	require.Equal(t, http.StatusBadGateway, rec.Code, "账号凭据类错误仍按网关映射，不透传给客户端")
	require.NotContains(t, rec.Body.String(), "bad key")
}

func TestWriteGeminiCachedContentUpstreamError_RateLimitMappedUnlessUpstreamGateway(t *testing.T) {
	h := &GatewayHandler{}
	apiKey := geminiCachedContentTestAPIKey(service.PlatformGemini)
	quota := `{"error":{"code":429,"message":"You exceeded your current quota","status":"RESOURCE_EXHAUSTED","details":[{"quotaId":"GenerateRequestsPerDayPerProjectPerModel-FreeTier"}]}}`

	c, rec := newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/cachedContents", apiKey)
	h.writeGeminiCachedContentUpstreamError(c, &service.GeminiCachedContentUpstreamError{StatusCode: http.StatusTooManyRequests, Body: []byte(quota)})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.NotContains(t, rec.Body.String(), "FreeTier", "官方账号的限流不透传配额明细")

	c, rec = newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/cachedContents", apiKey)
	h.writeGeminiCachedContentUpstreamError(c, &service.GeminiCachedContentUpstreamError{StatusCode: http.StatusTooManyRequests, Body: []byte(quota), UpstreamGateway: true})
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.JSONEq(t, quota, rec.Body.String(), "上游网关账号的限流反映持有缓存的上游账号状态，原样回写")

	c, rec = newGeminiCachedContentTestContext(http.MethodPost, "/v1beta/cachedContents", apiKey)
	h.writeGeminiCachedContentUpstreamError(c, &service.GeminiCachedContentUpstreamError{StatusCode: http.StatusBadRequest, Body: []byte(`{"error":{"code":400,"message":"Cached content is too small","status":"INVALID_ARGUMENT"}}`)})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "too small", "请求校验类错误仍原样回写")
}
