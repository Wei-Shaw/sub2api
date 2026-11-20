//go:build unit

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

// Exercise the forwarded HTTP request without opening a network listener.
type deepseekToolsHTTPUpstream struct{ handler http.Handler }

func (u *deepseekToolsHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if req.Body == nil {
		req.Body = http.NoBody
	}
	u.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func (u *deepseekToolsHTTPUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
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

func forwardDeepseekToolsTest(svc *OpenAIGatewayService, c *gin.Context, account *Account, userID int64, endpoint string, body []byte, contentType string) (*DeepseekToolsForwardResult, error) {
	ctx := context.Background()
	request, err := svc.PrepareDeepseekToolsRequest(ctx, c, userID, endpoint, body, contentType)
	if err != nil {
		return nil, err
	}
	return svc.ForwardDeepseekToolsRequest(request.SchedulingContext(ctx), c, account, request)
}

func TestForwardDeepseekToolsRequest_SearchPassthrough(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":[{"title":"doc"}]}`))
	})

	body := []byte(`{"query":"deepseek docs"}`)
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/search", body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "search", body, "application/json")
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pages":[{"text":"a"},{"text":"b"},{"text":"c"}],"usage":{"pages":99}}`))
	})

	rawBody := "--dstestboundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nPNGDATA\r\n--dstestboundary--\r\n"
	multipartType := "multipart/form-data; boundary=dstestboundary"
	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/ocr", []byte(rawBody))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "ocr", []byte(rawBody), multipartType)
	require.NoError(t, err)
	require.Equal(t, multipartType, gotContentType, "boundary must reach upstream intact")
	require.Equal(t, rawBody, string(gotBody))
	// 页数以 pages 数组长度为准，usage 仅参考。
	require.Equal(t, DeepseekToolsUsageOCRPages, result.Usage.Kind)
	require.InDelta(t, 3, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_OwnedGetDelete(t *testing.T) {
	var paths []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.RequestURI())
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Empty(t, body)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			if r.Header.Get("anthropic-beta") != "" {
				_, _ = w.Write([]byte(`{"id":"file-abc_1","type":"file_deleted"}`))
			} else {
				_, _ = w.Write([]byte(`{"id":"file-abc_1","object":"file","deleted":true}`))
			}
			return
		}
		_, _ = w.Write([]byte(`{"id":"file-abc_1"}`))
	})
	account := newDeepseekToolsTestAccount("https://relay.test")
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}, cache: cache}
	const query = "?tag=one&tag=two&q=some%20text"
	for _, format := range []string{"openai", "anthropic"} {
		require.NoError(t, cache.SaveDeepseekFile(context.Background(), 1, DeepseekFile{ID: "file-abc_1", AccountID: account.ID}))
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			endpoint := "files_get/file-abc_1"
			if method == http.MethodDelete {
				endpoint = "files_delete/file-abc_1"
			}
			c, _ := newDeepseekToolsTestContext(method, "/v1/files/file-abc_1"+query, nil)
			path := "/v1/files/file-abc_1" + query
			if format == "anthropic" {
				c.Request.Header.Set("anthropic-beta", deepseekFilesBetaHeader)
				path = "/anthropic" + path
			}
			count := len(paths)
			_, err := forwardDeepseekToolsTest(svc, c, account, 2, endpoint, nil, "")
			require.ErrorIs(t, err, errDeepseekFileNotFound)
			wrong := *account
			wrong.ID++
			_, err = forwardDeepseekToolsTest(svc, c, &wrong, 1, endpoint, nil, "")
			require.ErrorIs(t, err, errDeepseekFileNotFound)
			require.Len(t, paths, count)
			result, err := forwardDeepseekToolsTest(svc, c, account, 1, endpoint, nil, "")
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, result.StatusCode)
			require.Equal(t, method+" "+path, paths[count])
		}
		require.Empty(t, cache.files[1])
	}
}

func TestForwardDeepseekToolsRequest_TTSStreamsBinary(t *testing.T) {
	audio := bytes.Repeat([]byte{0x01, 0x02, 0x03, 0xFF}, 512)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
		_, _ = w.Write(audio)
	})

	body := []byte(`{"text":"你好 world"}`)
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/tts", body)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "tts", body, "application/json")
	require.NoError(t, err)
	require.Equal(t, audio, rec.Body.Bytes(), "streamed audio bytes must match upstream")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "audio/wav", rec.Header().Get("Content-Type"))
	require.Equal(t, DeepseekToolsUsageTTSChars, result.Usage.Kind)
	// 你好 = 2 汉字 × 2 + 空格 1 + world 5 = 10
	require.InDelta(t, 10, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_ASRDurationToSeconds(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hi","duration_ms":9500}`))
	})

	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", []byte(`{}`))
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "asr", []byte(`{}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, DeepseekToolsUsageASRSeconds, result.Usage.Kind)
	require.InDelta(t, 9.5, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_UpstreamErrorPassthroughZeroUsage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad audio"}}`))
	})

	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}}

	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", []byte(`{}`))
	result, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 0, "asr", []byte(`{}`), "application/json")
	require.NoError(t, err, "error body is passed through, not turned into a transport error")
	require.Equal(t, http.StatusBadRequest, result.StatusCode)
	require.Equal(t, `{"error":{"message":"bad audio"}}`, rec.Body.String())
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, result.Usage.Kind, "billing must stay zero on upstream error")
	require.InDelta(t, 0, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_RejectsBadInput(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}

	_, err := forwardDeepseekToolsTest(svc, nil, &Account{Platform: PlatformOpenAI}, 0, "search", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")

	_, err = forwardDeepseekToolsTest(svc, nil, &Account{Platform: PlatformDeepseek}, 0, "unknown", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unsupported")

	_, err = forwardDeepseekToolsTest(svc, nil, &Account{Platform: PlatformDeepseek}, 0, "files_get/../etc", nil, "")
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBeta = r.Header.Get("anthropic-beta")
		gotVersion = r.Header.Get("anthropic-version")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-api-1","type":"file","size_bytes":70,"created_at":"2026-11-17T04:20:28.705+00:00","filename":"a.png","mime_type":"image/png"}`))
	})

	body := []byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\nContent-Type: image/png\r\n\r\nPNG\r\n--b--\r\n")
	c, rec := newDeepseekToolsTestContext(http.MethodPost, "/v1/files", body)
	c.Request.Header.Set("anthropic-beta", "files-api-2025-04-14")
	c.Request.Header.Set("anthropic-version", "2023-06-01")
	cache := &testDeepseekFileCache{}
	writtenBytesAtBind := -1
	cache.onSave = func() { writtenBytesAtBind = rec.Body.Len() }
	account := newDeepseekToolsAnthropicFilesAccount("https://relay.test/anthropic")
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}, cache: cache}
	result, err := forwardDeepseekToolsTest(svc, c, account, 7, "files_create", body, "multipart/form-data; boundary=b")
	require.NoError(t, err)
	require.Equal(t, "/anthropic/v1/files", gotPath, "anthropic-beta requests must go to the Anthropic compatible Files endpoint")
	require.Equal(t, "files-api-2025-04-14", gotBeta)
	require.Equal(t, "2023-06-01", gotVersion)
	require.Equal(t, "Bearer sk-dstest", gotAuth)
	require.Equal(t, body, gotBody)
	require.Equal(t, `{"id":"file-api-1","type":"file","size_bytes":70,"created_at":"2026-11-17T04:20:28.705+00:00","filename":"a.png","mime_type":"image/png"}`, rec.Body.String(), "Anthropic shaped response must pass through unchanged")
	require.Empty(t, result.Usage.Kind, "files endpoints are not billed")

	require.Zero(t, writtenBytesAtBind)
	file := cache.files[7]["file-api-1"]
	require.Equal(t, account.ID, file.AccountID)
	require.Equal(t, "user_data", file.Purpose)
	require.Equal(t, "image/png", file.MIMEType)
	require.Equal(t, "anthropic", file.Format)
	require.Equal(t, int64(70), file.Bytes)
	require.Zero(t, file.ExpiresAt)
}

func TestForwardDeepseekToolsRequest_FilesWithoutBetaStaysOpenAI(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"file-api-2","object":"file","bytes":70,"created_at":1700000000,"expires_at":1700086400,"purpose":"user_data"}`))
	})

	body := []byte("--b\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.png\"\r\n\r\nPNG\r\n--b--\r\n")
	c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/files", body)
	cache := &testDeepseekFileCache{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{handler: handler}, cache: cache}
	_, err := forwardDeepseekToolsTest(svc, c, newDeepseekToolsTestAccount("https://relay.test"), 7, "files_create", body, "multipart/form-data; boundary=b")
	require.NoError(t, err)
	require.Equal(t, "/v1/files", gotPath, "requests without the Files beta header keep the OpenAI compatible path")
	file := cache.files[7]["file-api-2"]
	require.Equal(t, int64(901), file.AccountID)
	require.Equal(t, int64(1700086400), file.ExpiresAt)
	require.Equal(t, "openai", file.Format)
}
