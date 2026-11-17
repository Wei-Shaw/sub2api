package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// deepseekToolsHTTPUpstream 把请求真实转发到 httptest 假上游，
// 模拟 s.httpUpstream.Do 的仓库既有构造方式。
type deepseekToolsHTTPUpstream struct{}

func (u *deepseekToolsHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req)
}

func (u *deepseekToolsHTTPUpstream) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return http.DefaultTransport.RoundTrip(req)
}

func newDeepseekToolsTestAccount(baseURL string) *Account {
	return &Account{
		ID:          901,
		Name:        "deepseek-tools",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-dstest",
			"base_url": baseURL,
		},
	}
}

func newDeepseekToolsTestContext(method, path string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, path, bytes.NewReader(body))
	return c, rec
}

func TestForwardDeepseekToolsRequest_SearchPassthrough(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":[{"title":"doc"}]}`))
	}))
	t.Cleanup(server.Close)

	body := []byte(`{"query":"deepseek docs"}`)
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/search", body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "search", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-dstest", gotAuth)
	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/v1/tools/search", gotPath)
	require.Equal(t, body, gotBody)
	require.Equal(t, `{"result":[{"title":"doc"}]}`, rec.Body.String())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Equal(t, DeepseekToolsUsagePerRequest, result.Usage.Kind)
	require.InDelta(t, 1, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_MultipartBoundaryPreserved(t *testing.T) {
	var gotContentType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[{"text":"a"},{"text":"b"},{"text":"c"}],"usage":{"pages":99}}`))
	}))
	t.Cleanup(server.Close)

	rawBody := "--dstestboundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nPNGDATA\r\n--dstestboundary--\r\n"
	multipartType := "multipart/form-data; boundary=dstestboundary"
	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/ocr", []byte(rawBody))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "ocr", []byte(rawBody), multipartType)
	require.NoError(t, err)
	require.Equal(t, multipartType, gotContentType, "boundary must reach upstream intact")
	require.Equal(t, rawBody, string(gotBody))
	// 页数以 pages 数组长度为准，usage 仅参考。
	require.Equal(t, DeepseekToolsUsageOCRPages, result.Usage.Kind)
	require.InDelta(t, 3, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_FilesGetDeleteNoBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-abc_1"}`))
	}))
	t.Cleanup(server.Close)

	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}

	c, _ := newDeepseekToolsTestContext(http.MethodGet, "/v1/files/file-abc_1", nil)
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "files_get/file-abc_1", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, gotMethod)
	require.Equal(t, "/v1/files/file-abc_1", gotPath)
	require.Empty(t, gotBody)
	require.Empty(t, result.Usage.Kind)
	require.InDelta(t, 0, result.Usage.Count, 1e-9)

	c, _ = newDeepseekToolsTestContext(http.MethodGet, "/v1/files", nil)
	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "files_list", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, gotMethod)
	require.Equal(t, "/v1/files", gotPath)
	require.Empty(t, gotBody)

	c, _ = newDeepseekToolsTestContext(http.MethodDelete, "/v1/files/file-abc_1", nil)
	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "files_delete/file-abc_1", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.MethodDelete, gotMethod)
	require.Equal(t, "/v1/files/file-abc_1", gotPath)
	require.Empty(t, gotBody)
}

func TestForwardDeepseekToolsRequest_TTSStreamsBinary(t *testing.T) {
	audio := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0xFF}, 512)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
		_, _ = w.Write(audio)
	}))
	t.Cleanup(server.Close)

	body := []byte(`{"text":"你好 world"}`)
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/tts", body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "tts", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, audio, rec.Body.Bytes(), "streamed audio bytes must match upstream")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
	require.Equal(t, DeepseekToolsUsageTTSChars, result.Usage.Kind)
	// 你好 = 2 汉字 × 2 + 空格 1 + world 5 = 10
	require.InDelta(t, 10, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_ASRDurationToSeconds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hi","duration_ms":9500}`))
	}))
	t.Cleanup(server.Close)

	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", []byte(`{}`))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "asr", []byte(`{}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, DeepseekToolsUsageASRSeconds, result.Usage.Kind)
	require.InDelta(t, 9.5, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_UpstreamErrorPassthroughZeroUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad audio"}}`))
	}))
	t.Cleanup(server.Close)

	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}

	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", []byte(`{}`))
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 0, "asr", []byte(`{}`), "application/json")
	require.NoError(t, err, "error body is passed through, not turned into a transport error")
	require.Equal(t, http.StatusBadRequest, result.StatusCode)
	require.Equal(t, `{"error":{"message":"bad audio"}}`, rec.Body.String())
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, result.Usage.Kind, "billing must stay zero on upstream error")
	require.InDelta(t, 0, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_RejectsBadInput(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}

	_, err := svc.ForwardDeepseekToolsRequest(context.Background(), nil, &Account{Platform: PlatformOpenAI}, nil, 0, "search", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")

	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), nil, &Account{Platform: PlatformDeepseek}, nil, 0, "unknown", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")

	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), nil, &Account{Platform: PlatformDeepseek}, nil, 0, "files_get/../etc", nil, "")
	require.Error(t, err)
}

func TestCountTTSBillableChars(t *testing.T) {
	require.Equal(t, 0, countTTSBillableChars(""))
	// ASCII 算 1。
	require.Equal(t, 5, countTTSBillableChars("hello"))
	// 汉字算 2。
	require.Equal(t, 4, countTTSBillableChars("你好"))
	// 假名算 2。
	require.Equal(t, 10, countTTSBillableChars("こんにちは"))
	// 全角标点算 2。
	require.Equal(t, 4, countTTSBillableChars("。！"))
	// 混合：你好 = 4，" hi" = 3。
	require.Equal(t, 7, countTTSBillableChars("你好 hi"))
}

func newDeepseekToolsAnthropicFilesAccount(baseURL string) *Account {
	return &Account{
		ID:          902,
		Name:        "deepseek-anthropic-files",
		Platform:    PlatformDeepseek,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":      "sk-dstest",
			"api_protocol": "adaptive",
			"base_url":     "https://api.deepseek.com",
			"api_base_urls": map[string]any{
				"anthropic": baseURL,
			},
		},
	}
}

func TestForwardDeepseekToolsRequest_AnthropicFilesPassthrough(t *testing.T) {
	var gotPath, gotBeta, gotVersion, gotAuth string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBeta = r.Header.Get("anthropic-beta")
		gotVersion = r.Header.Get("anthropic-version")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-api-1","type":"file","size_bytes":70,"created_at":"2026-11-17T04:20:28.705+00:00","filename":"a.png","mime_type":"image/png"}`))
	}))
	t.Cleanup(server.Close)

	body := []byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nPNG\r\n--b--\r\n")
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/files", body)
	c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	cache := &fakeAnthropicFileOwnerCache{}
	writtenBytesAtBind := -1
	cache.onSet = func() { writtenBytesAtBind = rec.Body.Len() }
	account := newDeepseekToolsAnthropicFilesAccount(server.URL + "/anthropic")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}, cache: cache}
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, account, nil, 7, "files_create", body, "multipart/form-data; boundary=b")
	require.NoError(t, err)
	require.Equal(t, "/anthropic/v1/files", gotPath, "anthropic-beta requests must go to the Anthropic compatible Files endpoint")
	require.Equal(t, "files-api-2025-04-14", gotBeta)
	require.Equal(t, "2023-06-01", gotVersion)
	require.Equal(t, "Bearer sk-dstest", gotAuth)
	require.Equal(t, body, gotBody)
	require.Equal(t, `{"id":"file-api-1","type":"file","size_bytes":70,"created_at":"2026-11-17T04:20:28.705+00:00","filename":"a.png","mime_type":"image/png"}`, rec.Body.String(), "Anthropic shaped response must pass through unchanged")
	require.Empty(t, result.Usage.Kind, "files endpoints are not billed")

	// 归属必须在上传响应 flush 之前写入，否则紧随其后的 /v1/messages 可能查不到。
	require.Equal(t, 0, writtenBytesAtBind, "owner must be recorded before the upload response is written")
	require.Equal(t, fakeAnthropicFileOwnerEntry{ownerUserID: 7, accountID: account.ID, platform: PlatformDeepseek}, cache.owners["file-api-1"])
	require.Equal(t, anthropicFileOwnerTTL, cache.ttls["file-api-1"])

	// 归属按用户生效：上传用户解析命中；其他用户引用同一 file_id 被判为他人的文件。
	fileBody := []byte(`{"messages":[{"role":"user","content":[{"type":"document","source":{"type":"file","file_id":"file-api-1"}}]}]}`)
	ownerID, platform, foreign, ok := svc.ResolveAnthropicFileOwner(context.Background(), 7, fileBody)
	require.True(t, ok)
	require.False(t, foreign)
	require.Equal(t, account.ID, ownerID)
	require.Equal(t, PlatformDeepseek, platform)
	_, _, foreign, ok = svc.ResolveAnthropicFileOwner(context.Background(), 8, fileBody)
	require.True(t, foreign)
	require.False(t, ok)
}

func TestForwardDeepseekToolsRequest_FilesWithoutBetaStaysOpenAI(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-api-2","object":"file","bytes":70}`))
	}))
	t.Cleanup(server.Close)

	body := []byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\n\r\nPNG\r\n--b--\r\n")
	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/files", body)
	cache := &fakeAnthropicFileOwnerCache{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}, cache: cache}
	_, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), nil, 7, "files_create", body, "multipart/form-data; boundary=b")
	require.NoError(t, err)
	require.Equal(t, "/v1/files", gotPath, "requests without the Files beta header keep the OpenAI compatible path")
	require.Zero(t, cache.setCall, "OpenAI shaped file responses must not create Anthropic owner bindings")
	require.Empty(t, cache.owners)
}
