//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ==================== Stub: SoraGenerationRepository ====================

var _ service.SoraGenerationRepository = (*stubSoraGenRepo)(nil)

type stubSoraGenRepo struct {
	gens       map[int64]*service.SoraGeneration
	nextID     int64
	createErr  error
	getErr     error
	updateErr  error
	deleteErr  error
	listErr    error
	countErr   error
	countValue int64

	// 条件性 Update 失败：前 updateFailAfterN 次成功，之后失败
	updateCallCount  *int32
	updateFailAfterN int32

	// 条件性 GetByID 状态覆盖：前 getByIDOverrideAfterN 次正常返回，之后返回 overrideStatus
	getByIDCallCount      int32
	getByIDOverrideAfterN int32 // 0 = 不覆盖
	getByIDOverrideStatus string
}

func newStubSoraGenRepo() *stubSoraGenRepo {
	return &stubSoraGenRepo{gens: make(map[int64]*service.SoraGeneration), nextID: 1}
}

func (r *stubSoraGenRepo) Create(_ context.Context, gen *service.SoraGeneration) error {
	if r.createErr != nil {
		return r.createErr
	}
	gen.ID = r.nextID
	r.nextID++
	r.gens[gen.ID] = gen
	return nil
}
func (r *stubSoraGenRepo) GetByID(_ context.Context, id int64) (*service.SoraGeneration, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	gen, ok := r.gens[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	// 条件性状态覆盖：模拟外部取消等场景
	if r.getByIDOverrideAfterN > 0 {
		n := atomic.AddInt32(&r.getByIDCallCount, 1)
		if n > r.getByIDOverrideAfterN {
			cp := *gen
			cp.Status = r.getByIDOverrideStatus
			return &cp, nil
		}
	}
	return gen, nil
}
func (r *stubSoraGenRepo) Update(_ context.Context, gen *service.SoraGeneration) error {
	// 条件性失败：前 N 次成功，之后失败
	if r.updateCallCount != nil {
		n := atomic.AddInt32(r.updateCallCount, 1)
		if n > r.updateFailAfterN {
			return fmt.Errorf("conditional update error (call #%d)", n)
		}
	}
	if r.updateErr != nil {
		return r.updateErr
	}
	r.gens[gen.ID] = gen
	return nil
}
func (r *stubSoraGenRepo) Delete(_ context.Context, id int64) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.gens, id)
	return nil
}
func (r *stubSoraGenRepo) List(_ context.Context, params service.SoraGenerationListParams) ([]*service.SoraGeneration, int64, error) {
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	var result []*service.SoraGeneration
	for _, gen := range r.gens {
		if gen.UserID != params.UserID {
			continue
		}
		result = append(result, gen)
	}
	return result, int64(len(result)), nil
}
func (r *stubSoraGenRepo) CountByUserAndStatus(_ context.Context, _ int64, _ []string) (int64, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	return r.countValue, nil
}

// ==================== 辅助函数 ====================

func newTestSoraClientHandler(repo *stubSoraGenRepo) *SoraClientHandler {
	genService := service.NewSoraGenerationService(repo, nil, nil)
	return &SoraClientHandler{genService: genService}
}

func makeGinContext(method, path, body string, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	if body != "" {
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
	} else {
		c.Request = httptest.NewRequest(method, path, nil)
	}
	if userID > 0 {
		c.Set("user_id", userID)
	}
	return c, rec
}

func parseResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// ==================== 纯函数测试: buildAsyncRequestBody ====================

func TestBuildAsyncRequestBody(t *testing.T) {
	body := buildAsyncRequestBody("sora2-landscape-10s", "一只猫在跳舞", "", 1)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "sora2-landscape-10s", parsed["model"])
	require.Equal(t, false, parsed["stream"])

	msgs := parsed["messages"].([]any)
	require.Len(t, msgs, 1)
	msg := msgs[0].(map[string]any)
	require.Equal(t, "user", msg["role"])
	require.Equal(t, "一只猫在跳舞", msg["content"])
}

func TestBuildAsyncRequestBody_EmptyPrompt(t *testing.T) {
	body := buildAsyncRequestBody("gpt-image", "", "", 1)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "gpt-image", parsed["model"])
	msgs := parsed["messages"].([]any)
	msg := msgs[0].(map[string]any)
	require.Equal(t, "", msg["content"])
}

func TestBuildAsyncRequestBody_WithImageInput(t *testing.T) {
	body := buildAsyncRequestBody("gpt-image", "一只猫", "https://example.com/ref.png", 1)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, "https://example.com/ref.png", parsed["image_input"])
}

func TestBuildAsyncRequestBody_WithVideoCount(t *testing.T) {
	body := buildAsyncRequestBody("sora2-landscape-10s", "一只猫在跳舞", "", 3)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed))
	require.Equal(t, float64(3), parsed["video_count"])
}

func TestNormalizeVideoCount(t *testing.T) {
	require.Equal(t, 1, normalizeVideoCount("video", 0))
	require.Equal(t, 2, normalizeVideoCount("video", 2))
	require.Equal(t, 3, normalizeVideoCount("video", 5))
	require.Equal(t, 1, normalizeVideoCount("image", 3))
}

// ==================== 纯函数测试: parseMediaURLsFromBody ====================

func TestParseMediaURLsFromBody_MediaURLs(t *testing.T) {
	urls := parseMediaURLsFromBody([]byte(`{"media_urls":["https://a.com/1.mp4","https://a.com/2.mp4"]}`))
	require.Equal(t, []string{"https://a.com/1.mp4", "https://a.com/2.mp4"}, urls)
}

func TestParseMediaURLsFromBody_SingleMediaURL(t *testing.T) {
	urls := parseMediaURLsFromBody([]byte(`{"media_url":"https://a.com/video.mp4"}`))
	require.Equal(t, []string{"https://a.com/video.mp4"}, urls)
}

func TestParseMediaURLsFromBody_EmptyBody(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody(nil))
	require.Nil(t, parseMediaURLsFromBody([]byte{}))
}

func TestParseMediaURLsFromBody_InvalidJSON(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody([]byte("not json")))
}

func TestParseMediaURLsFromBody_NoMediaFields(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody([]byte(`{"data":"something"}`)))
}

func TestParseMediaURLsFromBody_EmptyMediaURL(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody([]byte(`{"media_url":""}`)))
}

func TestParseMediaURLsFromBody_EmptyMediaURLs(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody([]byte(`{"media_urls":[]}`)))
}

func TestParseMediaURLsFromBody_MediaURLsPriority(t *testing.T) {
	body := `{"media_url":"https://single.com/1.mp4","media_urls":["https://multi.com/a.mp4","https://multi.com/b.mp4"]}`
	urls := parseMediaURLsFromBody([]byte(body))
	require.Len(t, urls, 2)
	require.Equal(t, "https://multi.com/a.mp4", urls[0])
}

func TestParseMediaURLsFromBody_FilterEmpty(t *testing.T) {
	urls := parseMediaURLsFromBody([]byte(`{"media_urls":["https://a.com/1.mp4","","https://a.com/2.mp4"]}`))
	require.Equal(t, []string{"https://a.com/1.mp4", "https://a.com/2.mp4"}, urls)
}

func TestParseMediaURLsFromBody_AllEmpty(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody([]byte(`{"media_urls":["",""]}`)))
}

func TestParseMediaURLsFromBody_NonStringArray(t *testing.T) {
	// media_urls 不是 string 数组
	require.Nil(t, parseMediaURLsFromBody([]byte(`{"media_urls":"not-array"}`)))
}

func TestParseMediaURLsFromBody_MediaURLNotString(t *testing.T) {
	require.Nil(t, parseMediaURLsFromBody([]byte(`{"media_url":123}`)))
}

// ==================== 纯函数测试: extractMediaURLsFromResult ====================

func TestExtractMediaURLsFromResult_OAuthPath(t *testing.T) {
	result := &service.ForwardResult{MediaURL: "https://oauth.com/video.mp4"}
	recorder := httptest.NewRecorder()
	url, urls := extractMediaURLsFromResult(result, recorder)
	require.Equal(t, "https://oauth.com/video.mp4", url)
	require.Equal(t, []string{"https://oauth.com/video.mp4"}, urls)
}

func TestExtractMediaURLsFromResult_OAuthWithBody(t *testing.T) {
	result := &service.ForwardResult{MediaURL: "https://oauth.com/video.mp4"}
	recorder := httptest.NewRecorder()
	_, _ = recorder.Write([]byte(`{"media_urls":["https://body.com/1.mp4","https://body.com/2.mp4"]}`))
	url, urls := extractMediaURLsFromResult(result, recorder)
	require.Equal(t, "https://body.com/1.mp4", url)
	require.Len(t, urls, 2)
}

func TestExtractMediaURLsFromResult_APIKeyPath(t *testing.T) {
	recorder := httptest.NewRecorder()
	_, _ = recorder.Write([]byte(`{"media_url":"https://upstream.com/video.mp4"}`))
	url, urls := extractMediaURLsFromResult(nil, recorder)
	require.Equal(t, "https://upstream.com/video.mp4", url)
	require.Equal(t, []string{"https://upstream.com/video.mp4"}, urls)
}

func TestExtractMediaURLsFromResult_NilResultEmptyBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	url, urls := extractMediaURLsFromResult(nil, recorder)
	require.Empty(t, url)
	require.Nil(t, urls)
}

func TestExtractMediaURLsFromResult_EmptyMediaURL(t *testing.T) {
	result := &service.ForwardResult{MediaURL: ""}
	recorder := httptest.NewRecorder()
	url, urls := extractMediaURLsFromResult(result, recorder)
	require.Empty(t, url)
	require.Nil(t, urls)
}

// ==================== getUserIDFromContext ====================

func TestGetUserIDFromContext_Int64(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("user_id", int64(42))
	require.Equal(t, int64(42), getUserIDFromContext(c))
}

func TestGetUserIDFromContext_AuthSubject(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 777})
	require.Equal(t, int64(777), getUserIDFromContext(c))
}

func TestGetUserIDFromContext_Float64(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("user_id", float64(99))
	require.Equal(t, int64(99), getUserIDFromContext(c))
}

func TestGetUserIDFromContext_String(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("user_id", "123")
	require.Equal(t, int64(123), getUserIDFromContext(c))
}

func TestGetUserIDFromContext_UserIDFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("userID", int64(55))
	require.Equal(t, int64(55), getUserIDFromContext(c))
}

func TestGetUserIDFromContext_NoID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	require.Equal(t, int64(0), getUserIDFromContext(c))
}

func TestGetUserIDFromContext_InvalidString(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Set("user_id", "not-a-number")
	require.Equal(t, int64(0), getUserIDFromContext(c))
}

// ==================== Handler: Generate ====================

func TestGenerate_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 0)
	h.Generate(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGenerate_BadRequest_MissingModel(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGenerate_BadRequest_MissingPrompt(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGenerate_BadRequest_InvalidJSON(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{invalid`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGenerate_TooManyRequests(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.countValue = 3
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestGenerate_CountError(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.countErr = fmt.Errorf("db error")
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestGenerate_Success(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"测试生成"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.NotZero(t, data["generation_id"])
	require.Equal(t, "pending", data["status"])
}

func TestGenerate_DefaultMediaType(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "video", repo.gens[1].MediaType)
}

func TestGenerate_ImageMediaType(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"gpt-image","prompt":"test","media_type":"image"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image", repo.gens[1].MediaType)
}

func TestGenerate_CreatePendingError(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.createErr = fmt.Errorf("create failed")
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestGenerate_NilQuotaServiceSkipsCheck(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerate_APIKeyInContext(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	c.Set("api_key_id", int64(42))
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.gens[1].APIKeyID)
	require.Equal(t, int64(42), *repo.gens[1].APIKeyID)
}

func TestGenerate_NoAPIKeyInContext(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.gens[1].APIKeyID)
}

func TestGenerate_ConcurrencyBoundary(t *testing.T) {
	// activeCount == 2 应该允许
	repo := newStubSoraGenRepo()
	repo.countValue = 2
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
}

// ==================== Handler: ListGenerations ====================

func TestListGenerations_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/generations", "", 0)
	h.ListGenerations(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListGenerations_Success(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Model: "sora2-landscape-10s", Status: "completed", StorageType: "upstream"}
	repo.gens[2] = &service.SoraGeneration{ID: 2, UserID: 1, Model: "gpt-image", Status: "pending", StorageType: "none"}
	repo.nextID = 3
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("GET", "/api/v1/sora/generations?page=1&page_size=10", "", 1)
	h.ListGenerations(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	items := data["data"].([]any)
	require.Len(t, items, 2)
	require.Equal(t, float64(2), data["total"])
}

func TestListGenerations_ListError(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.listErr = fmt.Errorf("db error")
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("GET", "/api/v1/sora/generations", "", 1)
	h.ListGenerations(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestListGenerations_DefaultPagination(t *testing.T) {
	repo := newStubSoraGenRepo()
	h := newTestSoraClientHandler(repo)
	// 不传分页参数，应默认 page=1 page_size=20
	c, rec := makeGinContext("GET", "/api/v1/sora/generations", "", 1)
	h.ListGenerations(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, float64(1), data["page"])
}

// ==================== Handler: GetGeneration ====================

func TestGetGeneration_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/generations/1", "", 0)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.GetGeneration(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetGeneration_InvalidID(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/generations/abc", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	h.GetGeneration(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetGeneration_NotFound(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/generations/999", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	h.GetGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetGeneration_WrongUser(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 2, Status: "completed"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("GET", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.GetGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetGeneration_Success(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Model: "sora2-landscape-10s", Status: "completed", StorageType: "upstream", MediaURL: "https://example.com/video.mp4"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("GET", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.GetGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, float64(1), data["id"])
}

// ==================== Handler: DeleteGeneration ====================

func TestDeleteGeneration_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 0)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestDeleteGeneration_InvalidID(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/abc", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDeleteGeneration_NotFound(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/999", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteGeneration_WrongUser(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 2, Status: "completed"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteGeneration_Success(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
	_, exists := repo.gens[1]
	require.False(t, exists)
}

// ==================== Handler: CancelGeneration ====================

func TestCancelGeneration_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 0)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCancelGeneration_InvalidID(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/abc/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCancelGeneration_NotFound(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/999/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCancelGeneration_WrongUser(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 2, Status: "pending"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestCancelGeneration_Pending(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "cancelled", repo.gens[1].Status)
}

func TestCancelGeneration_Generating(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "generating"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "cancelled", repo.gens[1].Status)
}

func TestCancelGeneration_Completed(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestCancelGeneration_Failed(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "failed"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestCancelGeneration_Cancelled(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "cancelled"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusConflict, rec.Code)
}

// ==================== Handler: GetQuota ====================

func TestGetQuota_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/quota", "", 0)
	h.GetQuota(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestGetQuota_NilQuotaService(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/quota", "", 1)
	h.GetQuota(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, "unlimited", data["source"])
}

// ==================== Handler: GetModels ====================

func TestGetModels(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/models", "", 0)
	h.GetModels(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].([]any)
	require.Len(t, data, 4)
	// 验证类型分布
	videoCount, imageCount := 0, 0
	for _, item := range data {
		m := item.(map[string]any)
		if m["type"] == "video" {
			videoCount++
		} else if m["type"] == "image" {
			imageCount++
		}
	}
	require.Equal(t, 3, videoCount)
	require.Equal(t, 1, imageCount)
}

// ==================== Handler: GetStorageStatus ====================

func TestGetStorageStatus_NilS3(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("GET", "/api/v1/sora/storage-status", "", 0)
	h.GetStorageStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, false, data["s3_enabled"])
	require.Equal(t, false, data["s3_healthy"])
	require.Equal(t, false, data["local_enabled"])
}

func TestGetStorageStatus_LocalEnabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sora-storage-status-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: tmpDir,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)
	h := &SoraClientHandler{mediaStorage: mediaStorage}

	c, rec := makeGinContext("GET", "/api/v1/sora/storage-status", "", 0)
	h.GetStorageStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, false, data["s3_enabled"])
	require.Equal(t, false, data["s3_healthy"])
	require.Equal(t, true, data["local_enabled"])
}

// ==================== Handler: SaveToStorage ====================

func TestSaveToStorage_Unauthorized(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 0)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSaveToStorage_InvalidID(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/abc/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveToStorage_NotFound(t *testing.T) {
	h := newTestSoraClientHandler(newStubSoraGenRepo())
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/999/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "999"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSaveToStorage_NotUpstream(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed", StorageType: "s3", MediaURL: "https://example.com/v.mp4"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveToStorage_EmptyMediaURL(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed", StorageType: "upstream", MediaURL: ""}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSaveToStorage_S3Nil(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed", StorageType: "upstream", MediaURL: "https://example.com/video.mp4"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, fmt.Sprint(resp["message"]), "云存储")
}

func TestSaveToStorage_WrongUser(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 2, Status: "completed", StorageType: "upstream", MediaURL: "https://example.com/video.mp4"}
	h := newTestSoraClientHandler(repo)
	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ==================== storeMediaWithDegradation — nil guard 路径 ====================

func TestStoreMediaWithDegradation_NilS3NilMedia(t *testing.T) {
	h := &SoraClientHandler{}
	url, urls, storageType, keys, size := h.storeMediaWithDegradation(
		context.Background(), 1, "video", "https://upstream.com/v.mp4", nil,
	)
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
	require.Equal(t, "https://upstream.com/v.mp4", url)
	require.Equal(t, []string{"https://upstream.com/v.mp4"}, urls)
	require.Nil(t, keys)
	require.Equal(t, int64(0), size)
}

func TestStoreMediaWithDegradation_NilGuardsMultiURL(t *testing.T) {
	h := &SoraClientHandler{}
	url, urls, storageType, keys, size := h.storeMediaWithDegradation(
		context.Background(), 1, "video", "https://upstream.com/v.mp4", []string{"https://a.com/1.mp4", "https://a.com/2.mp4"},
	)
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
	require.Equal(t, "https://a.com/1.mp4", url)
	require.Equal(t, []string{"https://a.com/1.mp4", "https://a.com/2.mp4"}, urls)
	require.Nil(t, keys)
	require.Equal(t, int64(0), size)
}

func TestStoreMediaWithDegradation_EmptyMediaURLsFallback(t *testing.T) {
	h := &SoraClientHandler{}
	url, _, storageType, _, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", "https://upstream.com/v.mp4", []string{},
	)
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
	require.Equal(t, "https://upstream.com/v.mp4", url)
}

// ==================== Stub: UserRepository (用于 SoraQuotaService) ====================

var _ service.UserRepository = (*stubUserRepoForHandler)(nil)

type stubUserRepoForHandler struct {
	users     map[int64]*service.User
	updateErr error
}

func newStubUserRepoForHandler() *stubUserRepoForHandler {
	return &stubUserRepoForHandler{users: make(map[int64]*service.User)}
}

func (r *stubUserRepoForHandler) GetByID(_ context.Context, id int64) (*service.User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("user not found")
}
func (r *stubUserRepoForHandler) Update(_ context.Context, user *service.User) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.users[user.ID] = user
	return nil
}
func (r *stubUserRepoForHandler) Create(context.Context, *service.User) error { return nil }
func (r *stubUserRepoForHandler) GetByEmail(context.Context, string) (*service.User, error) {
	return nil, nil
}
func (r *stubUserRepoForHandler) GetFirstAdmin(context.Context) (*service.User, error) {
	return nil, nil
}
func (r *stubUserRepoForHandler) Delete(context.Context, int64) error { return nil }
func (r *stubUserRepoForHandler) List(context.Context, pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubUserRepoForHandler) ListWithFilters(context.Context, pagination.PaginationParams, service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubUserRepoForHandler) UpdateBalance(context.Context, int64, float64) error { return nil }
func (r *stubUserRepoForHandler) DeductBalance(context.Context, int64, float64) error { return nil }
func (r *stubUserRepoForHandler) UpdateConcurrency(context.Context, int64, int) error { return nil }
func (r *stubUserRepoForHandler) ExistsByEmail(context.Context, string) (bool, error) {
	return false, nil
}
func (r *stubUserRepoForHandler) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *stubUserRepoForHandler) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	return nil
}
func (r *stubUserRepoForHandler) UpdateTotpSecret(context.Context, int64, *string) error { return nil }
func (r *stubUserRepoForHandler) EnableTotp(context.Context, int64) error                { return nil }
func (r *stubUserRepoForHandler) DisableTotp(context.Context, int64) error               { return nil }
func (r *stubUserRepoForHandler) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	return nil
}

// ==================== NewSoraClientHandler ====================

func TestNewSoraClientHandler(t *testing.T) {
	h := NewSoraClientHandler(nil, nil, nil, nil, nil, nil, nil)
	require.NotNil(t, h)
}

func TestNewSoraClientHandler_WithAPIKeyService(t *testing.T) {
	h := NewSoraClientHandler(nil, nil, nil, nil, nil, nil, nil)
	require.NotNil(t, h)
	require.Nil(t, h.apiKeyService)
}

// ==================== Stub: APIKeyRepository (用于 API Key 校验测试) ====================

var _ service.APIKeyRepository = (*stubAPIKeyRepoForHandler)(nil)

type stubAPIKeyRepoForHandler struct {
	keys   map[int64]*service.APIKey
	getErr error
}

func newStubAPIKeyRepoForHandler() *stubAPIKeyRepoForHandler {
	return &stubAPIKeyRepoForHandler{keys: make(map[int64]*service.APIKey)}
}

func (r *stubAPIKeyRepoForHandler) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if k, ok := r.keys[id]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("api key not found: %d", id)
}
func (r *stubAPIKeyRepoForHandler) Create(context.Context, *service.APIKey) error { return nil }
func (r *stubAPIKeyRepoForHandler) GetKeyAndOwnerID(_ context.Context, _ int64) (string, int64, error) {
	return "", 0, nil
}
func (r *stubAPIKeyRepoForHandler) GetByKey(context.Context, string) (*service.APIKey, error) {
	return nil, nil
}
func (r *stubAPIKeyRepoForHandler) GetByKeyForAuth(context.Context, string) (*service.APIKey, error) {
	return nil, nil
}
func (r *stubAPIKeyRepoForHandler) Update(context.Context, *service.APIKey) error { return nil }
func (r *stubAPIKeyRepoForHandler) Delete(context.Context, int64) error           { return nil }
func (r *stubAPIKeyRepoForHandler) ListByUserID(_ context.Context, _ int64, _ pagination.PaginationParams, _ service.APIKeyListFilters) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubAPIKeyRepoForHandler) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	return nil, nil
}
func (r *stubAPIKeyRepoForHandler) CountByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *stubAPIKeyRepoForHandler) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}
func (r *stubAPIKeyRepoForHandler) ListByGroupID(_ context.Context, _ int64, _ pagination.PaginationParams) ([]service.APIKey, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubAPIKeyRepoForHandler) SearchAPIKeys(context.Context, int64, string, int) ([]service.APIKey, error) {
	return nil, nil
}
func (r *stubAPIKeyRepoForHandler) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *stubAPIKeyRepoForHandler) UpdateGroupIDByUserAndGroup(_ context.Context, userID, oldGroupID, newGroupID int64) (int64, error) {
	var updated int64
	for id, key := range r.keys {
		if key.UserID != userID || key.GroupID == nil || *key.GroupID != oldGroupID {
			continue
		}
		clone := *key
		gid := newGroupID
		clone.GroupID = &gid
		r.keys[id] = &clone
		updated++
	}
	return updated, nil
}
func (r *stubAPIKeyRepoForHandler) CountByGroupID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (r *stubAPIKeyRepoForHandler) ListKeysByUserID(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (r *stubAPIKeyRepoForHandler) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	return nil, nil
}
func (r *stubAPIKeyRepoForHandler) IncrementQuotaUsed(_ context.Context, _ int64, _ float64) (float64, error) {
	return 0, nil
}
func (r *stubAPIKeyRepoForHandler) UpdateLastUsed(context.Context, int64, time.Time) error {
	return nil
}
func (r *stubAPIKeyRepoForHandler) IncrementRateLimitUsage(context.Context, int64, float64) error {
	return nil
}
func (r *stubAPIKeyRepoForHandler) ResetRateLimitWindows(context.Context, int64) error {
	return nil
}
func (r *stubAPIKeyRepoForHandler) GetRateLimitData(context.Context, int64) (*service.APIKeyRateLimitData, error) {
	return nil, nil
}

// newTestAPIKeyService 创建测试用的 APIKeyService
func newTestAPIKeyService(repo *stubAPIKeyRepoForHandler) *service.APIKeyService {
	return service.NewAPIKeyService(repo, nil, nil, nil, nil, nil, &config.Config{})
}

// ==================== Generate: API Key 校验（前端传递 api_key_id）====================

func TestGenerate_WithAPIKeyID_Success(t *testing.T) {
	// 前端传递 api_key_id，校验通过 → 成功生成，记录关联 api_key_id
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	groupID := int64(5)
	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:      42,
		UserID:  1,
		Status:  service.StatusAPIKeyActive,
		GroupID: &groupID,
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.NotZero(t, data["generation_id"])

	// 验证 api_key_id 已关联到生成记录
	gen := repo.gens[1]
	require.NotNil(t, gen.APIKeyID)
	require.Equal(t, int64(42), *gen.APIKeyID)
}

func TestGenerate_WithAPIKeyID_NotFound(t *testing.T) {
	// 前端传递不存在的 api_key_id → 400
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":999}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, fmt.Sprint(resp["message"]), "不存在")
}

func TestGenerate_WithAPIKeyID_WrongUser(t *testing.T) {
	// 前端传递别人的 api_key_id → 403
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:     42,
		UserID: 999, // 属于 user 999
		Status: service.StatusAPIKeyActive,
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, fmt.Sprint(resp["message"]), "不属于")
}

func TestGenerate_WithAPIKeyID_Disabled(t *testing.T) {
	// 前端传递已禁用的 api_key_id → 403
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:     42,
		UserID: 1,
		Status: service.StatusAPIKeyDisabled,
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, fmt.Sprint(resp["message"]), "不可用")
}

func TestGenerate_WithAPIKeyID_QuotaExhausted(t *testing.T) {
	// 前端传递配额耗尽的 api_key_id → 403
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:     42,
		UserID: 1,
		Status: service.StatusAPIKeyQuotaExhausted,
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGenerate_WithAPIKeyID_Expired(t *testing.T) {
	// 前端传递已过期的 api_key_id → 403
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:     42,
		UserID: 1,
		Status: service.StatusAPIKeyExpired,
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGenerate_WithAPIKeyID_NilAPIKeyService(t *testing.T) {
	// apiKeyService 为 nil 时忽略 api_key_id → 正常生成但不记录 api_key_id
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	h := &SoraClientHandler{genService: genService} // apiKeyService = nil
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	// apiKeyService 为 nil → 跳过校验 → api_key_id 不记录
	require.Nil(t, repo.gens[1].APIKeyID)
}

func TestGenerate_WithAPIKeyID_NilGroupID(t *testing.T) {
	// api_key 有效但 GroupID 为 nil → 成功，groupID 为 nil
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:      42,
		UserID:  1,
		Status:  service.StatusAPIKeyActive,
		GroupID: nil, // 无分组
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, repo.gens[1].APIKeyID)
	require.Equal(t, int64(42), *repo.gens[1].APIKeyID)
}

func TestGenerate_NoAPIKeyID_NoContext_NilResult(t *testing.T) {
	// 既无 api_key_id 字段也无 context 中的 api_key_id → api_key_id 为 nil
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Nil(t, repo.gens[1].APIKeyID)
}

func TestGenerate_WithAPIKeyIDInBody_OverridesContext(t *testing.T) {
	// 同时有 body api_key_id 和 context api_key_id → 优先使用 body 的
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	groupID := int64(10)
	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyRepo.keys[42] = &service.APIKey{
		ID:      42,
		UserID:  1,
		Status:  service.StatusAPIKeyActive,
		GroupID: &groupID,
	}
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":42}`, 1)
	c.Set("api_key_id", int64(99)) // context 中有另一个 api_key_id
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	// 应使用 body 中的 api_key_id=42，而不是 context 中的 99
	require.NotNil(t, repo.gens[1].APIKeyID)
	require.Equal(t, int64(42), *repo.gens[1].APIKeyID)
}

func TestGenerate_WithContextAPIKeyID_FallbackPath(t *testing.T) {
	// 无 body api_key_id，但 context 有 → 使用 context 中的（兼容网关路由）
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	c.Set("api_key_id", int64(99))
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
	// 应使用 context 中的 api_key_id=99
	require.NotNil(t, repo.gens[1].APIKeyID)
	require.Equal(t, int64(99), *repo.gens[1].APIKeyID)
}

func TestGenerate_APIKeyID_Zero_IgnoredInJSON(t *testing.T) {
	// JSON 中 api_key_id=0 被视为 omitempty → 仍然为指针值 0，需要传 nil 检查
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	apiKeyRepo := newStubAPIKeyRepoForHandler()
	apiKeyService := newTestAPIKeyService(apiKeyRepo)

	h := &SoraClientHandler{genService: genService, apiKeyService: apiKeyService}
	// JSON 中传了 api_key_id: 0 → 解析后 *int64(0)，会触发校验
	// api_key_id=0 不存在 → 400
	c, rec := makeGinContext("POST", "/api/v1/sora/generate",
		`{"model":"sora2-landscape-10s","prompt":"test","api_key_id":0}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// ==================== processGeneration: groupID 传递与 ForcePlatform ====================

func TestProcessGeneration_WithGroupID_NoForcePlatform(t *testing.T) {
	// groupID 不为 nil → 不设置 ForcePlatform
	// gatewayService 为 nil → MarkFailed → 检查错误消息不包含 ForcePlatform 相关
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	gid := int64(5)
	h.processGeneration(1, 1, &gid, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "gatewayService")
}

func TestProcessGeneration_NilGroupID_SetsForcePlatform(t *testing.T) {
	// groupID 为 nil → 设置 ForcePlatform → gatewayService 为 nil → MarkFailed
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "gatewayService")
}

func TestProcessGeneration_MarkGeneratingStateConflict(t *testing.T) {
	// 任务状态已变化（如已取消）→ MarkGenerating 返回 ErrSoraGenerationStateConflict → 跳过
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "cancelled"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	// 状态为 cancelled 时 MarkGenerating 不符合状态转换规则 → 应保持 cancelled
	require.Equal(t, "cancelled", repo.gens[1].Status)
}

// ==================== GenerateRequest JSON 解析 ====================

func TestGenerateRequest_WithAPIKeyID_JSONParsing(t *testing.T) {
	// 验证 api_key_id 在 JSON 中正确解析为 *int64
	var req GenerateRequest
	err := json.Unmarshal([]byte(`{"model":"sora2","prompt":"test","api_key_id":42}`), &req)
	require.NoError(t, err)
	require.NotNil(t, req.APIKeyID)
	require.Equal(t, int64(42), *req.APIKeyID)
}

func TestGenerateRequest_WithoutAPIKeyID_JSONParsing(t *testing.T) {
	// 不传 api_key_id → 解析后为 nil
	var req GenerateRequest
	err := json.Unmarshal([]byte(`{"model":"sora2","prompt":"test"}`), &req)
	require.NoError(t, err)
	require.Nil(t, req.APIKeyID)
}

func TestGenerateRequest_NullAPIKeyID_JSONParsing(t *testing.T) {
	// api_key_id: null → 解析后为 nil
	var req GenerateRequest
	err := json.Unmarshal([]byte(`{"model":"sora2","prompt":"test","api_key_id":null}`), &req)
	require.NoError(t, err)
	require.Nil(t, req.APIKeyID)
}

func TestGenerateRequest_FullFields_JSONParsing(t *testing.T) {
	// 全字段解析
	var req GenerateRequest
	err := json.Unmarshal([]byte(`{
		"model":"sora2-landscape-10s",
		"prompt":"test prompt",
		"media_type":"video",
		"video_count":2,
		"image_input":"data:image/png;base64,abc",
		"api_key_id":100
	}`), &req)
	require.NoError(t, err)
	require.Equal(t, "sora2-landscape-10s", req.Model)
	require.Equal(t, "test prompt", req.Prompt)
	require.Equal(t, "video", req.MediaType)
	require.Equal(t, 2, req.VideoCount)
	require.Equal(t, "data:image/png;base64,abc", req.ImageInput)
	require.NotNil(t, req.APIKeyID)
	require.Equal(t, int64(100), *req.APIKeyID)
}

func TestGenerateRequest_JSONSerialize_OmitsNilAPIKeyID(t *testing.T) {
	// api_key_id 为 nil 时 JSON 序列化应省略
	req := GenerateRequest{Model: "sora2", Prompt: "test"}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	_, hasAPIKeyID := parsed["api_key_id"]
	require.False(t, hasAPIKeyID, "api_key_id 为 nil 时应省略")
}

func TestGenerateRequest_JSONSerialize_IncludesAPIKeyID(t *testing.T) {
	// api_key_id 不为 nil 时 JSON 序列化应包含
	id := int64(42)
	req := GenerateRequest{Model: "sora2", Prompt: "test", APIKeyID: &id}
	b, err := json.Marshal(req)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(b, &parsed))
	require.Equal(t, float64(42), parsed["api_key_id"])
}

// ==================== GetQuota: 有配额服务 ====================

func TestGetQuota_WithQuotaService_Success(t *testing.T) {
	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID:                    1,
		SoraStorageQuotaBytes: 10 * 1024 * 1024,
		SoraStorageUsedBytes:  3 * 1024 * 1024,
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)

	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{
		genService:   genService,
		quotaService: quotaService,
	}

	c, rec := makeGinContext("GET", "/api/v1/sora/quota", "", 1)
	h.GetQuota(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, "user", data["source"])
	require.Equal(t, float64(10*1024*1024), data["quota_bytes"])
	require.Equal(t, float64(3*1024*1024), data["used_bytes"])
}

func TestGetQuota_WithQuotaService_Error(t *testing.T) {
	// 用户不存在时 GetQuota 返回错误
	userRepo := newStubUserRepoForHandler()
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)

	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{
		genService:   genService,
		quotaService: quotaService,
	}

	c, rec := makeGinContext("GET", "/api/v1/sora/quota", "", 999)
	h.GetQuota(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ==================== Generate: 配额检查 ====================

func TestGenerate_QuotaCheckFailed(t *testing.T) {
	// 配额超限时返回 429
	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID:                    1,
		SoraStorageQuotaBytes: 1024,
		SoraStorageUsedBytes:  1025, // 已超限
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)

	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{
		genService:   genService,
		quotaService: quotaService,
	}

	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

func TestGenerate_QuotaCheckPassed(t *testing.T) {
	// 配额充足时允许生成
	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID:                    1,
		SoraStorageQuotaBytes: 10 * 1024 * 1024,
		SoraStorageUsedBytes:  0,
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)

	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{
		genService:   genService,
		quotaService: quotaService,
	}

	c, rec := makeGinContext("POST", "/api/v1/sora/generate", `{"model":"sora2-landscape-10s","prompt":"test"}`, 1)
	h.Generate(c)
	require.Equal(t, http.StatusOK, rec.Code)
}

// ==================== Stub: SettingRepository (用于 S3 存储测试) ====================

var _ service.SettingRepository = (*stubSettingRepoForHandler)(nil)

type stubSettingRepoForHandler struct {
	values map[string]string
}

func newStubSettingRepoForHandler(values map[string]string) *stubSettingRepoForHandler {
	if values == nil {
		values = make(map[string]string)
	}
	return &stubSettingRepoForHandler{values: values}
}

func (r *stubSettingRepoForHandler) Get(_ context.Context, key string) (*service.Setting, error) {
	if v, ok := r.values[key]; ok {
		return &service.Setting{Key: key, Value: v}, nil
	}
	return nil, service.ErrSettingNotFound
}
func (r *stubSettingRepoForHandler) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := r.values[key]; ok {
		return v, nil
	}
	return "", service.ErrSettingNotFound
}
func (r *stubSettingRepoForHandler) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}
func (r *stubSettingRepoForHandler) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			result[k] = v
		}
	}
	return result, nil
}
func (r *stubSettingRepoForHandler) SetMultiple(_ context.Context, settings map[string]string) error {
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}
func (r *stubSettingRepoForHandler) GetAll(_ context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *stubSettingRepoForHandler) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

// ==================== S3 / MediaStorage 辅助函数 ====================

// newS3StorageForHandler 创建指向指定 endpoint 的 S3Storage（用于测试）。
func newS3StorageForHandler(endpoint string) *service.SoraS3Storage {
	settingRepo := newStubSettingRepoForHandler(map[string]string{
		"sora_s3_enabled":           "true",
		"sora_s3_endpoint":          endpoint,
		"sora_s3_region":            "us-east-1",
		"sora_s3_bucket":            "test-bucket",
		"sora_s3_access_key_id":     "AKIATEST",
		"sora_s3_secret_access_key": "test-secret",
		"sora_s3_prefix":            "sora",
		"sora_s3_force_path_style":  "true",
	})
	settingService := service.NewSettingService(settingRepo, &config.Config{})
	return service.NewSoraS3Storage(settingService)
}

// newFakeSourceServer 创建返回固定内容的 HTTP 服务器（模拟上游媒体文件）。
func newFakeSourceServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fake video data for test"))
	}))
}

// newFakeS3Server 创建模拟 S3 的 HTTP 服务器。
// mode: "ok" 接受所有请求，"fail" 返回 403，"fail-second" 第一次成功第二次失败。
func newFakeS3Server(mode string) *httptest.Server {
	var counter atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()

		switch mode {
		case "ok":
			w.Header().Set("ETag", `"test-etag"`)
			w.WriteHeader(http.StatusOK)
		case "fail":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`))
		case "fail-second":
			n := counter.Add(1)
			if n <= 1 {
				w.Header().Set("ETag", `"test-etag"`)
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`))
			}
		}
	}))
}

// ==================== processGeneration 直接调用测试 ====================

func TestProcessGeneration_MarkGeneratingFails(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	repo.updateErr = fmt.Errorf("db error")
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	// 直接调用（非 goroutine），MarkGenerating 失败 → 早退
	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	// MarkGenerating 在调用 repo.Update 前已修改内存对象为 "generating"
	// repo.Update 返回错误 → processGeneration 早退，不会继续到 MarkFailed
	// 因此 ErrorMessage 为空（证明未调用 MarkFailed）
	require.Equal(t, "generating", repo.gens[1].Status)
	require.Empty(t, repo.gens[1].ErrorMessage)
}

func TestProcessGeneration_GatewayServiceNil(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}
	// gatewayService 未设置 → MarkFailed

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "gatewayService")
}

// ==================== storeMediaWithDegradation: S3 路径 ====================

func TestStoreMediaWithDegradation_S3SuccessSingleURL(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	storedURL, storedURLs, storageType, s3Keys, fileSize := h.storeMediaWithDegradation(
		context.Background(), 1, "video", sourceServer.URL+"/v.mp4", nil,
	)
	require.Equal(t, service.SoraStorageTypeS3, storageType)
	require.Len(t, s3Keys, 1)
	require.NotEmpty(t, s3Keys[0])
	require.Len(t, storedURLs, 1)
	require.Equal(t, storedURL, storedURLs[0])
	require.Contains(t, storedURL, fakeS3.URL)
	require.Contains(t, storedURL, "/test-bucket/")
	require.Greater(t, fileSize, int64(0))
}

func TestStoreMediaWithDegradation_S3SuccessMultiURL(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	urls := []string{sourceServer.URL + "/a.mp4", sourceServer.URL + "/b.mp4"}
	storedURL, storedURLs, storageType, s3Keys, fileSize := h.storeMediaWithDegradation(
		context.Background(), 1, "video", sourceServer.URL+"/a.mp4", urls,
	)
	require.Equal(t, service.SoraStorageTypeS3, storageType)
	require.Len(t, s3Keys, 2)
	require.Len(t, storedURLs, 2)
	require.Equal(t, storedURL, storedURLs[0])
	require.Contains(t, storedURLs[0], fakeS3.URL)
	require.Contains(t, storedURLs[1], fakeS3.URL)
	require.Greater(t, fileSize, int64(0))
}

func TestStoreMediaWithDegradation_S3DownloadFails(t *testing.T) {
	// 上游返回 404 → 下载失败 → S3 上传不会开始
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()
	badSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer badSource.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	_, _, storageType, _, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", badSource.URL+"/missing.mp4", nil,
	)
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
}

func TestStoreMediaWithDegradation_S3FailsSingleURL(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("fail")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	_, _, storageType, s3Keys, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", sourceServer.URL+"/v.mp4", nil,
	)
	// S3 失败，降级到 upstream
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
	require.Nil(t, s3Keys)
}

func TestStoreMediaWithDegradation_S3PartialFailureCleanup(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("fail-second")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	urls := []string{sourceServer.URL + "/a.mp4", sourceServer.URL + "/b.mp4"}
	_, _, storageType, s3Keys, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", sourceServer.URL+"/a.mp4", urls,
	)
	// 第二个 URL 上传失败 → 清理已上传 → 降级到 upstream
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
	require.Nil(t, s3Keys)
}

// ==================== storeMediaWithDegradation: 本地存储路径 ====================

func TestStoreMediaWithDegradation_LocalStorageFails(t *testing.T) {
	// 使用无效路径，EnsureLocalDirs 失败 → StoreFromURLs 返回 error
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: "/dev/null/invalid_dir",
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)
	h := &SoraClientHandler{mediaStorage: mediaStorage}

	_, _, storageType, _, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", "https://upstream.com/v.mp4", nil,
	)
	// 本地存储失败，降级到 upstream
	require.Equal(t, service.SoraStorageTypeUpstream, storageType)
}

func TestStoreMediaWithDegradation_LocalStorageSuccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sora-handler-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:                   "local",
				LocalPath:              tmpDir,
				DownloadTimeoutSeconds: 5,
				MaxDownloadBytes:       10 * 1024 * 1024,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)
	h := &SoraClientHandler{mediaStorage: mediaStorage}

	_, _, storageType, s3Keys, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", sourceServer.URL+"/v.mp4", nil,
	)
	require.Equal(t, service.SoraStorageTypeLocal, storageType)
	require.Nil(t, s3Keys) // 本地存储不返回 S3 keys
}

func TestStoreMediaWithDegradation_S3FailsFallbackToLocal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sora-handler-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("fail")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:                   "local",
				LocalPath:              tmpDir,
				DownloadTimeoutSeconds: 5,
				MaxDownloadBytes:       10 * 1024 * 1024,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)
	h := &SoraClientHandler{
		s3Storage:    s3Storage,
		mediaStorage: mediaStorage,
	}

	_, _, storageType, _, _ := h.storeMediaWithDegradation(
		context.Background(), 1, "video", sourceServer.URL+"/v.mp4", nil,
	)
	// S3 失败 → 本地存储成功
	require.Equal(t, service.SoraStorageTypeLocal, storageType)
}

// ==================== SaveToStorage: S3 路径 ====================

func TestSaveToStorage_S3EnabledButUploadFails(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("fail")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, resp["message"], "S3")
}

func TestSaveToStorage_UpstreamURLExpired(t *testing.T) {
	expiredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer expiredServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    expiredServer.URL + "/v.mp4",
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusGone, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, fmt.Sprint(resp["message"]), "过期")
}

func TestSaveToStorage_S3EnabledUploadSuccess(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Contains(t, data["message"], "S3")
	require.NotEmpty(t, data["object_key"])
	// 验证记录已更新为 S3 存储
	require.Equal(t, service.SoraStorageTypeS3, repo.gens[1].StorageType)
}

func TestSaveToStorage_S3EnabledUploadSuccess_MultiMediaURLs(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v1.mp4",
		MediaURLs: []string{
			sourceServer.URL + "/v1.mp4",
			sourceServer.URL + "/v2.mp4",
		},
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Len(t, data["object_keys"].([]any), 2)
	require.Equal(t, service.SoraStorageTypeS3, repo.gens[1].StorageType)
	require.Len(t, repo.gens[1].S3ObjectKeys, 2)
	require.Len(t, repo.gens[1].MediaURLs, 2)
}

func TestSaveToStorage_S3EnabledUploadSuccessWithQuota(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)

	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID:                    1,
		SoraStorageQuotaBytes: 100 * 1024 * 1024,
		SoraStorageUsedBytes:  0,
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage, quotaService: quotaService}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusOK, rec.Code)
	// 验证配额已累加
	require.Greater(t, userRepo.users[1].SoraStorageUsedBytes, int64(0))
}

func TestSaveToStorage_S3UploadSuccessMarkCompletedFails(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	// S3 上传成功后，MarkCompleted 会调用 repo.Update → 失败
	repo.updateErr = fmt.Errorf("db error")
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ==================== GetStorageStatus: S3 路径 ====================

func TestGetStorageStatus_S3EnabledNotHealthy(t *testing.T) {
	// S3 启用但 TestConnection 失败（fake 端点不响应 HeadBucket）
	fakeS3 := newFakeS3Server("fail")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	c, rec := makeGinContext("GET", "/api/v1/sora/storage-status", "", 0)
	h.GetStorageStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, true, data["s3_enabled"])
	require.Equal(t, false, data["s3_healthy"])
}

func TestGetStorageStatus_S3EnabledHealthy(t *testing.T) {
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	c, rec := makeGinContext("GET", "/api/v1/sora/storage-status", "", 0)
	h.GetStorageStatus(c)
	require.Equal(t, http.StatusOK, rec.Code)
	resp := parseResponse(t, rec)
	data := resp["data"].(map[string]any)
	require.Equal(t, true, data["s3_enabled"])
	require.Equal(t, true, data["s3_healthy"])
}

// ==================== Stub: AccountRepository (用于 GatewayService) ====================

var _ service.AccountRepository = (*stubAccountRepoForHandler)(nil)

type stubAccountRepoForHandler struct {
	accounts []service.Account
}

func (r *stubAccountRepoForHandler) Create(context.Context, *service.Account) error { return nil }
func (r *stubAccountRepoForHandler) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, fmt.Errorf("account not found")
}
func (r *stubAccountRepoForHandler) GetByIDs(context.Context, []int64) ([]*service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) ExistsByID(context.Context, int64) (bool, error) {
	return false, nil
}
func (r *stubAccountRepoForHandler) GetByCRSAccountID(context.Context, string) (*service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) FindByExtraField(context.Context, string, any) ([]service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) ListCRSAccountIDs(context.Context) (map[string]int64, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) Update(context.Context, *service.Account) error { return nil }
func (r *stubAccountRepoForHandler) Delete(context.Context, int64) error            { return nil }
func (r *stubAccountRepoForHandler) List(context.Context, pagination.PaginationParams) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubAccountRepoForHandler) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, string, int64, string) ([]service.Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *stubAccountRepoForHandler) ListByGroup(context.Context, int64) ([]service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) ListActive(context.Context) ([]service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) ListByPlatform(context.Context, string) ([]service.Account, error) {
	return nil, nil
}
func (r *stubAccountRepoForHandler) UpdateLastUsed(context.Context, int64) error { return nil }
func (r *stubAccountRepoForHandler) BatchUpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (r *stubAccountRepoForHandler) SetError(context.Context, int64, string) error { return nil }
func (r *stubAccountRepoForHandler) ClearError(context.Context, int64) error       { return nil }
func (r *stubAccountRepoForHandler) SetSchedulable(context.Context, int64, bool) error {
	return nil
}
func (r *stubAccountRepoForHandler) AutoPauseExpiredAccounts(context.Context, time.Time) (int64, error) {
	return 0, nil
}
func (r *stubAccountRepoForHandler) BindGroups(context.Context, int64, []int64) error { return nil }
func (r *stubAccountRepoForHandler) ListSchedulable(context.Context) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableByGroupID(context.Context, int64) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableByPlatform(_ context.Context, _ string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableByPlatforms(context.Context, []string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableUngroupedByPlatform(_ context.Context, _ string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) ListSchedulableUngroupedByPlatforms(_ context.Context, _ []string) ([]service.Account, error) {
	return r.accounts, nil
}
func (r *stubAccountRepoForHandler) SetRateLimited(context.Context, int64, time.Time) error {
	return nil
}
func (r *stubAccountRepoForHandler) SetModelRateLimit(context.Context, int64, string, time.Time) error {
	return nil
}
func (r *stubAccountRepoForHandler) SetOverloaded(context.Context, int64, time.Time) error {
	return nil
}
func (r *stubAccountRepoForHandler) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	return nil
}
func (r *stubAccountRepoForHandler) ClearTempUnschedulable(context.Context, int64) error { return nil }
func (r *stubAccountRepoForHandler) ClearRateLimit(context.Context, int64) error         { return nil }
func (r *stubAccountRepoForHandler) ClearAntigravityQuotaScopes(context.Context, int64) error {
	return nil
}
func (r *stubAccountRepoForHandler) ClearModelRateLimits(context.Context, int64) error { return nil }
func (r *stubAccountRepoForHandler) UpdateSessionWindow(context.Context, int64, *time.Time, *time.Time, string) error {
	return nil
}
func (r *stubAccountRepoForHandler) UpdateExtra(context.Context, int64, map[string]any) error {
	return nil
}
func (r *stubAccountRepoForHandler) BulkUpdate(context.Context, []int64, service.AccountBulkUpdate) (int64, error) {
	return 0, nil
}

func (r *stubAccountRepoForHandler) IncrementQuotaUsed(context.Context, int64, float64) error {
	return nil
}

func (r *stubAccountRepoForHandler) ResetQuotaUsed(context.Context, int64) error {
	return nil
}

// ==================== Stub: SoraClient (用于 SoraGatewayService) ====================

var _ service.SoraClient = (*stubSoraClientForHandler)(nil)

type stubSoraClientForHandler struct {
	videoStatus *service.SoraVideoTaskStatus
}

func (s *stubSoraClientForHandler) Enabled() bool { return true }
func (s *stubSoraClientForHandler) UploadImage(context.Context, *service.Account, []byte, string) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) CreateImageTask(context.Context, *service.Account, service.SoraImageRequest) (string, error) {
	return "task-image", nil
}
func (s *stubSoraClientForHandler) CreateVideoTask(context.Context, *service.Account, service.SoraVideoRequest) (string, error) {
	return "task-video", nil
}
func (s *stubSoraClientForHandler) CreateStoryboardTask(context.Context, *service.Account, service.SoraStoryboardRequest) (string, error) {
	return "task-video", nil
}
func (s *stubSoraClientForHandler) UploadCharacterVideo(context.Context, *service.Account, []byte) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) GetCameoStatus(context.Context, *service.Account, string) (*service.SoraCameoStatus, error) {
	return nil, nil
}
func (s *stubSoraClientForHandler) DownloadCharacterImage(context.Context, *service.Account, string) ([]byte, error) {
	return nil, nil
}
func (s *stubSoraClientForHandler) UploadCharacterImage(context.Context, *service.Account, []byte) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) FinalizeCharacter(context.Context, *service.Account, service.SoraCharacterFinalizeRequest) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) SetCharacterPublic(context.Context, *service.Account, string) error {
	return nil
}
func (s *stubSoraClientForHandler) DeleteCharacter(context.Context, *service.Account, string) error {
	return nil
}
func (s *stubSoraClientForHandler) PostVideoForWatermarkFree(context.Context, *service.Account, string) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) DeletePost(context.Context, *service.Account, string) error {
	return nil
}
func (s *stubSoraClientForHandler) GetWatermarkFreeURLCustom(context.Context, *service.Account, string, string, string) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) EnhancePrompt(context.Context, *service.Account, string, string, int) (string, error) {
	return "", nil
}
func (s *stubSoraClientForHandler) GetImageTask(context.Context, *service.Account, string) (*service.SoraImageTaskStatus, error) {
	return nil, nil
}
func (s *stubSoraClientForHandler) GetVideoTask(_ context.Context, _ *service.Account, _ string) (*service.SoraVideoTaskStatus, error) {
	return s.videoStatus, nil
}

// ==================== 辅助：创建最小 GatewayService 和 SoraGatewayService ====================

// newMinimalGatewayService 创建仅包含 accountRepo 的最小 GatewayService（用于测试 SelectAccountForModel）。
func newMinimalGatewayService(accountRepo service.AccountRepository) *service.GatewayService {
	return service.NewGatewayService(
		accountRepo, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		nil, nil,
	)
}

// newMinimalSoraGatewayService 创建最小 SoraGatewayService（用于测试 Forward）。
func newMinimalSoraGatewayService(soraClient service.SoraClient) *service.SoraGatewayService {
	cfg := &config.Config{
		Sora: config.SoraConfig{
			Client: config.SoraClientConfig{
				PollIntervalSeconds: 1,
				MaxPollAttempts:     1,
			},
		},
	}
	return service.NewSoraGatewayService(soraClient, nil, nil, cfg)
}

// ==================== processGeneration: 更多路径测试 ====================

func TestProcessGeneration_SelectAccountError(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	// accountRepo 返回空列表 → SelectAccountForModel 返回 "no available accounts"
	accountRepo := &stubAccountRepoForHandler{accounts: nil}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{genService: genService, gatewayService: gatewayService}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "选择账号失败")
}

func TestProcessGeneration_SoraGatewayServiceNil(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora processGeneration 集成测试，待流程稳定后恢复")
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	// 提供可用账号使 SelectAccountForModel 成功
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	// soraGatewayService 为 nil
	h := &SoraClientHandler{genService: genService, gatewayService: gatewayService}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "soraGatewayService")
}

func TestProcessGeneration_ForwardError(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora processGeneration 集成测试，待流程稳定后恢复")
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	// SoraClient 返回视频任务失败
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{
			Status:   "failed",
			ErrorMsg: "content policy violation",
		},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test prompt", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "生成失败")
}

func TestProcessGeneration_ForwardErrorCancelled(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	// MarkGenerating 内部调用 GetByID（第 1 次），Forward 失败后 processGeneration
	// 调用 GetByID（第 2 次）。模拟外部在 Forward 期间取消了任务。
	repo.getByIDOverrideAfterN = 1
	repo.getByIDOverrideStatus = "cancelled"
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{Status: "failed", ErrorMsg: "reject"},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	// Forward 失败后检测到外部取消，不应调用 MarkFailed（状态保持 generating）
	require.Equal(t, "generating", repo.gens[1].Status)
}

func TestProcessGeneration_ForwardSuccessNoMediaURL(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora processGeneration 集成测试，待流程稳定后恢复")
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	// SoraClient 返回 completed 但无 URL
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{
			Status: "completed",
			URLs:   nil, // 无 URL
		},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "未获取到媒体 URL")
}

func TestProcessGeneration_ForwardSuccessCancelledBeforeStore(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	// MarkGenerating 调用 GetByID（第 1 次），之后 processGeneration 行 176 调用 GetByID（第 2 次）
	// 第 2 次返回 "cancelled" 状态，模拟外部取消
	repo.getByIDOverrideAfterN = 1
	repo.getByIDOverrideStatus = "cancelled"
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{
			Status: "completed",
			URLs:   []string{"https://example.com/video.mp4"},
		},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	// Forward 成功后检测到外部取消，不应调用存储和 MarkCompleted（状态保持 generating）
	require.Equal(t, "generating", repo.gens[1].Status)
}

func TestProcessGeneration_FullSuccessUpstream(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora processGeneration 集成测试，待流程稳定后恢复")
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{
			Status: "completed",
			URLs:   []string{"https://example.com/video.mp4"},
		},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	// 无 S3 和本地存储，降级到 upstream
	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test prompt", "video", "", 1)
	require.Equal(t, "completed", repo.gens[1].Status)
	require.Equal(t, service.SoraStorageTypeUpstream, repo.gens[1].StorageType)
	require.NotEmpty(t, repo.gens[1].MediaURL)
}

func TestProcessGeneration_FullSuccessWithS3(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora processGeneration 集成测试，待流程稳定后恢复")
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{
			Status: "completed",
			URLs:   []string{sourceServer.URL + "/video.mp4"},
		},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	s3Storage := newS3StorageForHandler(fakeS3.URL)

	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID: 1, SoraStorageQuotaBytes: 100 * 1024 * 1024,
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)

	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
		s3Storage:          s3Storage,
		quotaService:       quotaService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test prompt", "video", "", 1)
	require.Equal(t, "completed", repo.gens[1].Status)
	require.Equal(t, service.SoraStorageTypeS3, repo.gens[1].StorageType)
	require.NotEmpty(t, repo.gens[1].S3ObjectKeys)
	require.Greater(t, repo.gens[1].FileSizeBytes, int64(0))
	// 验证配额已累加
	require.Greater(t, userRepo.users[1].SoraStorageUsedBytes, int64(0))
}

func TestProcessGeneration_MarkCompletedFails(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora processGeneration 集成测试，待流程稳定后恢复")
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	// 第 1 次 Update（MarkGenerating）成功，第 2 次（MarkCompleted）失败
	repo.updateCallCount = new(int32)
	repo.updateFailAfterN = 1
	genService := service.NewSoraGenerationService(repo, nil, nil)
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	soraClient := &stubSoraClientForHandler{
		videoStatus: &service.SoraVideoTaskStatus{
			Status: "completed",
			URLs:   []string{"https://example.com/video.mp4"},
		},
	}
	soraGatewayService := newMinimalSoraGatewayService(soraClient)
	h := &SoraClientHandler{
		genService:         genService,
		gatewayService:     gatewayService,
		soraGatewayService: soraGatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test prompt", "video", "", 1)
	// MarkCompleted 内部先修改内存对象状态为 completed，然后 Update 失败。
	// 由于 stub 存储的是指针，内存中的状态已被修改为 completed。
	// 此测试验证 processGeneration 在 MarkCompleted 失败后提前返回（不调用 AddUsage）。
	require.Equal(t, "completed", repo.gens[1].Status)
}

// ==================== cleanupStoredMedia 直接测试 ====================

func TestCleanupStoredMedia_S3Path(t *testing.T) {
	// S3 清理路径：s3Storage 为 nil 时不 panic
	h := &SoraClientHandler{}
	// 不应 panic
	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeS3, []string{"key1"}, nil)
}

func TestCleanupStoredMedia_LocalPath(t *testing.T) {
	// 本地清理路径：mediaStorage 为 nil 时不 panic
	h := &SoraClientHandler{}
	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeLocal, nil, []string{"/tmp/test.mp4"})
}

func TestCleanupStoredMedia_UpstreamPath(t *testing.T) {
	// upstream 类型不清理
	h := &SoraClientHandler{}
	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeUpstream, nil, nil)
}

func TestCleanupStoredMedia_EmptyKeys(t *testing.T) {
	// 空 keys 不触发清理
	h := &SoraClientHandler{}
	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeS3, nil, nil)
	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeLocal, nil, nil)
}

// ==================== DeleteGeneration: 本地存储清理路径 ====================

func TestDeleteGeneration_LocalStorageCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sora-delete-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: tmpDir,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID:          1,
		UserID:      1,
		Status:      "completed",
		StorageType: service.SoraStorageTypeLocal,
		MediaURL:    "video/test.mp4",
		MediaURLs:   []string{"video/test.mp4"},
	}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, mediaStorage: mediaStorage}

	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
	_, exists := repo.gens[1]
	require.False(t, exists)
}

func TestDeleteGeneration_LocalStorageCleanup_MediaURLFallback(t *testing.T) {
	// MediaURLs 为空，使用 MediaURL 作为清理路径
	tmpDir, err := os.MkdirTemp("", "sora-delete-fallback-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: tmpDir,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID:          1,
		UserID:      1,
		Status:      "completed",
		StorageType: service.SoraStorageTypeLocal,
		MediaURL:    "video/test.mp4",
		MediaURLs:   nil, // 空
	}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, mediaStorage: mediaStorage}

	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestDeleteGeneration_NonLocalStorage_SkipCleanup(t *testing.T) {
	// 非本地存储类型 → 跳过清理
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID:          1,
		UserID:      1,
		Status:      "completed",
		StorageType: service.SoraStorageTypeUpstream,
		MediaURL:    "https://upstream.com/v.mp4",
	}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestDeleteGeneration_DeleteError(t *testing.T) {
	// repo.Delete 出错
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed", StorageType: "upstream"}
	repo.deleteErr = fmt.Errorf("delete failed")
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ==================== fetchUpstreamModels 测试 ====================

func TestFetchUpstreamModels_NilGateway(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	h := &SoraClientHandler{}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "gatewayService 未初始化")
}

func TestFetchUpstreamModels_NoAccounts(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	accountRepo := &stubAccountRepoForHandler{accounts: nil}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "选择 Sora 账号失败")
}

func TestFetchUpstreamModels_NonAPIKeyAccount(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: "oauth", Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "不支持模型同步")
}

func TestFetchUpstreamModels_MissingAPIKey(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"base_url": "https://sora.test"}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "api_key")
}

func TestFetchUpstreamModels_MissingBaseURL_FallsBackToDefault(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	// GetBaseURL() 在缺少 base_url 时返回默认值 "https://api.anthropic.com"
	// 因此不会触发 "账号缺少 base_url" 错误，而是会尝试请求默认 URL 并失败
	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test"}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
}

func TestFetchUpstreamModels_UpstreamReturns500(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": ts.URL}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "状态码 500")
}

func TestFetchUpstreamModels_UpstreamReturnsInvalidJSON(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts.Close()

	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": ts.URL}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "解析响应失败")
}

func TestFetchUpstreamModels_UpstreamReturnsEmptyList(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer ts.Close()

	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": ts.URL}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "空模型列表")
}

func TestFetchUpstreamModels_Success(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求头
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		require.True(t, strings.HasSuffix(r.URL.Path, "/sora/v1/models"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"sora2-landscape-10s"},{"id":"sora2-portrait-10s"},{"id":"sora2-landscape-15s"},{"id":"gpt-image"}]}`))
	}))
	defer ts.Close()

	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": ts.URL}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	families, err := h.fetchUpstreamModels(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, families)
}

func TestFetchUpstreamModels_UnrecognizedModels(t *testing.T) {
	t.Skip("TODO: 临时屏蔽 Sora 上游模型同步相关测试，待账号选择逻辑稳定后恢复")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"unknown-model-1"},{"id":"unknown-model-2"}]}`))
	}))
	defer ts.Close()

	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": ts.URL}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}
	_, err := h.fetchUpstreamModels(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "未能从上游模型列表中识别")
}

// ==================== getModelFamilies 缓存测试 ====================

func TestGetModelFamilies_CachesLocalConfig(t *testing.T) {
	// gatewayService 为 nil → fetchUpstreamModels 失败 → 降级到本地配置
	h := &SoraClientHandler{}
	families := h.getModelFamilies(context.Background())
	require.NotEmpty(t, families)

	// 第二次调用应命中缓存（modelCacheUpstream=false → 使用短 TTL）
	families2 := h.getModelFamilies(context.Background())
	require.Equal(t, families, families2)
	require.False(t, h.modelCacheUpstream)
}

func TestGetModelFamilies_CachesUpstreamResult(t *testing.T) {
	t.Skip("TODO: 临时屏蔽依赖 Sora 上游模型同步的缓存测试，待账号选择逻辑稳定后恢复")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"sora2-landscape-10s"},{"id":"gpt-image"}]}`))
	}))
	defer ts.Close()

	accountRepo := &stubAccountRepoForHandler{
		accounts: []service.Account{
			{ID: 1, Type: service.AccountTypeAPIKey, Platform: service.PlatformSora, Status: service.StatusActive, Schedulable: true,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": ts.URL}},
		},
	}
	gatewayService := newMinimalGatewayService(accountRepo)
	h := &SoraClientHandler{gatewayService: gatewayService}

	families := h.getModelFamilies(context.Background())
	require.NotEmpty(t, families)
	require.True(t, h.modelCacheUpstream)

	// 第二次调用命中缓存
	families2 := h.getModelFamilies(context.Background())
	require.Equal(t, families, families2)
}

func TestGetModelFamilies_ExpiredCacheRefreshes(t *testing.T) {
	// 预设过期的缓存（modelCacheUpstream=false → 短 TTL）
	h := &SoraClientHandler{
		cachedFamilies:     []service.SoraModelFamily{{ID: "old"}},
		modelCacheTime:     time.Now().Add(-10 * time.Minute), // 已过期
		modelCacheUpstream: false,
	}
	// gatewayService 为 nil → fetchUpstreamModels 失败 → 使用本地配置刷新缓存
	families := h.getModelFamilies(context.Background())
	require.NotEmpty(t, families)
	// 缓存已刷新，不再是 "old"
	found := false
	for _, f := range families {
		if f.ID == "old" {
			found = true
		}
	}
	require.False(t, found, "过期缓存应被刷新")
}

// ==================== processGeneration: groupID 与 ForcePlatform ====================

func TestProcessGeneration_NilGroupID_WithGateway_SelectAccountFails(t *testing.T) {
	// groupID 为 nil → 设置 ForcePlatform=sora → 无可用 sora 账号 → MarkFailed
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "pending"}
	genService := service.NewSoraGenerationService(repo, nil, nil)

	// 空账号列表 → SelectAccountForModel 失败
	accountRepo := &stubAccountRepoForHandler{accounts: nil}
	gatewayService := newMinimalGatewayService(accountRepo)

	h := &SoraClientHandler{
		genService:     genService,
		gatewayService: gatewayService,
	}

	h.processGeneration(1, 1, nil, "sora2-landscape-10s", "test", "video", "", 1)
	require.Equal(t, "failed", repo.gens[1].Status)
	require.Contains(t, repo.gens[1].ErrorMessage, "选择账号失败")
}

// ==================== Generate: 配额检查非 QuotaExceeded 错误 ====================

func TestGenerate_CheckQuotaNonQuotaError(t *testing.T) {
	// quotaService.CheckQuota 返回非 QuotaExceededError → 返回 403
	repo := newStubSoraGenRepo()
	genService := service.NewSoraGenerationService(repo, nil, nil)

	// 用户不存在 → GetByID 失败 → CheckQuota 返回普通 error
	userRepo := newStubUserRepoForHandler()
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)

	h := NewSoraClientHandler(genService, quotaService, nil, nil, nil, nil, nil)

	body := `{"model":"sora2-landscape-10s","prompt":"test"}`
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", body, 1)
	h.Generate(c)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

// ==================== Generate: CreatePending 并发限制错误 ====================

// stubSoraGenRepoWithAtomicCreate 实现 soraGenerationRepoAtomicCreator 接口
type stubSoraGenRepoWithAtomicCreate struct {
	stubSoraGenRepo
	limitErr error
}

func (r *stubSoraGenRepoWithAtomicCreate) CreatePendingWithLimit(_ context.Context, gen *service.SoraGeneration, _ []string, _ int64) error {
	if r.limitErr != nil {
		return r.limitErr
	}
	return r.stubSoraGenRepo.Create(context.Background(), gen)
}

func TestGenerate_CreatePendingConcurrencyLimit(t *testing.T) {
	repo := &stubSoraGenRepoWithAtomicCreate{
		stubSoraGenRepo: *newStubSoraGenRepo(),
		limitErr:        service.ErrSoraGenerationConcurrencyLimit,
	}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := NewSoraClientHandler(genService, nil, nil, nil, nil, nil, nil)

	body := `{"model":"sora2-landscape-10s","prompt":"test"}`
	c, rec := makeGinContext("POST", "/api/v1/sora/generate", body, 1)
	h.Generate(c)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, resp["message"], "3")
}

// ==================== SaveToStorage: 配额超限 ====================

func TestSaveToStorage_QuotaExceeded(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)

	// 用户配额已满
	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID:                    1,
		SoraStorageQuotaBytes: 10,
		SoraStorageUsedBytes:  10,
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage, quotaService: quotaService}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// ==================== SaveToStorage: 配额非 QuotaExceeded 错误 ====================

func TestSaveToStorage_QuotaNonQuotaError(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)

	// 用户不存在 → GetByID 失败 → AddUsage 返回普通 error
	userRepo := newStubUserRepoForHandler()
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage, quotaService: quotaService}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ==================== SaveToStorage: MediaURLs 全为空 ====================

func TestSaveToStorage_EmptyMediaURLs(t *testing.T) {
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    "",
		MediaURLs:   []string{},
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	resp := parseResponse(t, rec)
	require.Contains(t, resp["message"], "已过期")
}

// ==================== SaveToStorage: S3 上传失败时已有已上传文件需清理 ====================

func TestSaveToStorage_MultiURL_SecondUploadFails(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("fail-second")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v1.mp4",
		MediaURLs:   []string{sourceServer.URL + "/v1.mp4", sourceServer.URL + "/v2.mp4"},
	}
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ==================== SaveToStorage: UpdateStorageForCompleted 失败（含配额回滚） ====================

func TestSaveToStorage_MarkCompletedFailsWithQuotaRollback(t *testing.T) {
	sourceServer := newFakeSourceServer()
	defer sourceServer.Close()
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: "upstream",
		MediaURL:    sourceServer.URL + "/v.mp4",
	}
	repo.updateErr = fmt.Errorf("db error")
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	genService := service.NewSoraGenerationService(repo, nil, nil)

	userRepo := newStubUserRepoForHandler()
	userRepo.users[1] = &service.User{
		ID:                    1,
		SoraStorageQuotaBytes: 100 * 1024 * 1024,
		SoraStorageUsedBytes:  0,
	}
	quotaService := service.NewSoraQuotaService(userRepo, nil, nil)
	h := &SoraClientHandler{genService: genService, s3Storage: s3Storage, quotaService: quotaService}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/save", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.SaveToStorage(c)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ==================== cleanupStoredMedia: 实际 S3 删除路径 ====================

func TestCleanupStoredMedia_WithS3Storage_ActualDelete(t *testing.T) {
	fakeS3 := newFakeS3Server("ok")
	defer fakeS3.Close()
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeS3, []string{"key1", "key2"}, nil)
}

func TestCleanupStoredMedia_S3DeleteFails_LogOnly(t *testing.T) {
	fakeS3 := newFakeS3Server("fail")
	defer fakeS3.Close()
	s3Storage := newS3StorageForHandler(fakeS3.URL)
	h := &SoraClientHandler{s3Storage: s3Storage}

	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeS3, []string{"key1"}, nil)
}

func TestCleanupStoredMedia_LocalDeleteFails_LogOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sora-cleanup-fail-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: tmpDir,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)
	h := &SoraClientHandler{mediaStorage: mediaStorage}

	h.cleanupStoredMedia(context.Background(), service.SoraStorageTypeLocal, nil, []string{"nonexistent/file.mp4"})
}

// ==================== DeleteGeneration: 本地文件删除失败（仅日志） ====================

func TestDeleteGeneration_LocalStorageDeleteFails_LogOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sora-del-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:      "local",
				LocalPath: tmpDir,
			},
		},
	}
	mediaStorage := service.NewSoraMediaStorage(cfg)

	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{
		ID: 1, UserID: 1, Status: "completed",
		StorageType: service.SoraStorageTypeLocal,
		MediaURL:    "nonexistent/video.mp4",
		MediaURLs:   []string{"nonexistent/video.mp4"},
	}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService, mediaStorage: mediaStorage}

	c, rec := makeGinContext("DELETE", "/api/v1/sora/generations/1", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.DeleteGeneration(c)
	require.Equal(t, http.StatusOK, rec.Code)
}

// ==================== CancelGeneration: 任务已结束冲突 ====================

func TestCancelGeneration_AlreadyCompleted(t *testing.T) {
	repo := newStubSoraGenRepo()
	repo.gens[1] = &service.SoraGeneration{ID: 1, UserID: 1, Status: "completed"}
	genService := service.NewSoraGenerationService(repo, nil, nil)
	h := &SoraClientHandler{genService: genService}

	c, rec := makeGinContext("POST", "/api/v1/sora/generations/1/cancel", "", 1)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	h.CancelGeneration(c)
	require.Equal(t, http.StatusConflict, rec.Code)
}
