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
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "search", body, "application/json")
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
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "ocr", []byte(rawBody), multipartType)
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
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "files_get/file-abc_1", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, gotMethod)
	require.Equal(t, "/v1/files/file-abc_1", gotPath)
	require.Empty(t, gotBody)
	require.Empty(t, result.Usage.Kind)
	require.InDelta(t, 0, result.Usage.Count, 1e-9)

	c, _ = newDeepseekToolsTestContext(http.MethodGet, "/v1/files", nil)
	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "files_list", nil, "")
	require.NoError(t, err)
	require.Equal(t, http.MethodGet, gotMethod)
	require.Equal(t, "/v1/files", gotPath)
	require.Empty(t, gotBody)

	c, _ = newDeepseekToolsTestContext(http.MethodDelete, "/v1/files/file-abc_1", nil)
	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "files_delete/file-abc_1", nil, "")
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
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "tts", body, "application/json")
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
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "asr", []byte(`{}`), "application/json")
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
	result, err := svc.ForwardDeepseekToolsRequest(context.Background(), c, newDeepseekToolsTestAccount(server.URL), "asr", []byte(`{}`), "application/json")
	require.NoError(t, err, "error body is passed through, not turned into a transport error")
	require.Equal(t, http.StatusBadRequest, result.StatusCode)
	require.Equal(t, `{"error":{"message":"bad audio"}}`, rec.Body.String())
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, result.Usage.Kind, "billing must stay zero on upstream error")
	require.InDelta(t, 0, result.Usage.Count, 1e-9)
}

func TestForwardDeepseekToolsRequest_RejectsBadInput(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: &deepseekToolsHTTPUpstream{}}

	_, err := svc.ForwardDeepseekToolsRequest(context.Background(), nil, &Account{Platform: PlatformOpenAI}, "search", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")

	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), nil, &Account{Platform: PlatformDeepseek}, "unknown", nil, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")

	_, err = svc.ForwardDeepseekToolsRequest(context.Background(), nil, &Account{Platform: PlatformDeepseek}, "files_get/../etc", nil, "")
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
