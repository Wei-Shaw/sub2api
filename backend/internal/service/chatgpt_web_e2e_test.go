//go:build e2e

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 真实链路 e2e：需要 ChatGPT 账号 access_token。
//
//	CGPT_WEB_E2E_ACCESS_TOKEN=... CGPT_WEB_E2E_PROXY=http://127.0.0.1:7890 \
//	  go test -tags=e2e -timeout 600s -run TestChatGPTWebE2E ./internal/service/
type chatGPTWebE2ETransport struct {
	proxyURL string
	clients  map[string]*http.Client
}

func newChatGPTWebE2ETransport(proxyURL string) *chatGPTWebE2ETransport {
	return &chatGPTWebE2ETransport{proxyURL: strings.TrimSpace(proxyURL), clients: map[string]*http.Client{}}
}

func (t *chatGPTWebE2ETransport) clientFor(profile *tlsfingerprint.Profile) (*http.Client, error) {
	cacheKey := "default"
	if profile != nil {
		cacheKey = profile.Name
	}
	if cached, ok := t.clients[cacheKey]; ok {
		return cached, nil
	}
	var dialTLS func(ctx context.Context, network, addr string) (net.Conn, error)
	if t.proxyURL != "" {
		parsed, err := url.Parse(t.proxyURL)
		if err != nil {
			return nil, err
		}
		dialTLS = tlsfingerprint.NewHTTPProxyDialer(profile, parsed).DialTLSContext
	} else {
		base := func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
		}
		dialTLS = tlsfingerprint.NewDialer(profile, base).DialTLSContext
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext:  dialTLS,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			MaxIdleConns:    8,
			IdleConnTimeout: 90 * time.Second,
		},
		Timeout: 300 * time.Second,
	}
	client.Transport = chatgptnetwork.Wrap(client.Transport.(*http.Transport), t.proxyURL != "")
	t.clients[cacheKey] = client
	return client, nil
}

func (t *chatGPTWebE2ETransport) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	client, err := t.clientFor(nil)
	if err != nil {
		return nil, err
	}
	if os.Getenv("CGPT_WEB_E2E_TRACE") == "1" {
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			fmt.Printf("web-e2e remote=%s\n", info.Conn.RemoteAddr())
		}}))
	}
	resp, err := client.Do(req)
	if os.Getenv("CGPT_WEB_E2E_TRACE") == "1" {
		status := 0
		challenge := false
		if resp != nil {
			status = resp.StatusCode
			challenge = resp.Header.Get("Cf-Mitigated") == "challenge"
		}
		fmt.Printf("web-e2e method=%s path=%s status=%d cloudflare_challenge=%t\n", req.Method, req.URL.Path, status, challenge)
	}
	return resp, err
}

func (t *chatGPTWebE2ETransport) DoWithTLS(
	req *http.Request,
	_ string,
	_ int64,
	_ int,
	profile *tlsfingerprint.Profile,
) (*http.Response, error) {
	client, err := t.clientFor(profile)
	if err != nil {
		return nil, err
	}
	if os.Getenv("CGPT_WEB_E2E_TRACE") == "1" {
		req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
			fmt.Printf("web-e2e remote=%s\n", info.Conn.RemoteAddr())
		}}))
	}
	resp, err := client.Do(req)
	// Never log challenge payloads or proofs.
	if err == nil && resp != nil && req.URL.Path == "/backend-api/sentinel/chat-requirements/prepare" && os.Getenv("CGPT_WEB_E2E_TRACE") == "1" {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		if readErr != nil {
			return nil, readErr
		}
		fmt.Printf("web-e2e sentinel vm_required=%t vm_payload_present=%t pow_required=%t\n", gjson.GetBytes(body, "turnstile.required").Bool(), gjson.GetBytes(body, "turnstile.dx").String() != "", gjson.GetBytes(body, "proofofwork.required").Bool())
	}
	if os.Getenv("CGPT_WEB_E2E_TRACE") == "1" {
		status := 0
		challenge := false
		if resp != nil {
			status = resp.StatusCode
			challenge = resp.Header.Get("Cf-Mitigated") == "challenge"
		}
		fmt.Printf("web-e2e method=%s path=%s status=%d cloudflare_challenge=%t\n", req.Method, req.URL.Path, status, challenge)
	}
	return resp, err
}

// chatGPTWebE2EResolveToken 支持直接给 access_token，或用浏览器分区 cookie 现场换一个。
func chatGPTWebE2EResolveToken(t *testing.T, transport *chatGPTWebE2ETransport) string {
	t.Helper()
	if token := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_ACCESS_TOKEN")); token != "" {
		return token
	}
	cookie := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_COOKIE"))
	if cookie == "" {
		t.Skip("CGPT_WEB_E2E_ACCESS_TOKEN / CGPT_WEB_E2E_COOKIE are not set")
	}
	session := newChatGPTWebSession(transport, nil, "", chatGPTWebDevice{})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	response, err := session.doChatGPTURLRequest(ctx, http.MethodGet, chatGPTWebBaseURL+"/api/auth/session", "/api/auth/session", nil,
		map[string]string{"Cookie": cookie, "Accept": "application/json"})
	require.NoError(t, err)
	body, err := readChatGPTWebBody(response)
	require.NoError(t, err)
	token := strings.TrimSpace(gjson.GetBytes(body, "accessToken").String())
	require.NotEmpty(t, token, "session cookie did not yield an access token")
	return token
}

func chatGPTWebE2ESession(t *testing.T) *chatGPTWebSession {
	t.Helper()
	proxyURL := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_PROXY"))
	transport := newChatGPTWebE2ETransport(proxyURL)
	return newChatGPTWebSession(transport, nil, chatGPTWebE2EResolveToken(t, transport), chatGPTWebDevice{})
}

// imageQuota reads the web image_gen allowance for the real-chain diagnostic.
func (s *chatGPTWebSession) imageQuota(ctx context.Context) (int, error) {
	path := "/backend-api/conversation/init"
	response, err := s.doJSON(ctx, http.MethodPost, path, map[string]any{
		"conversation_id":         nil,
		"gizmo_id":                nil,
		"requested_default_model": nil,
	}, nil)
	if err != nil {
		return 0, err
	}
	body, err := readChatGPTWebBody(response)
	if err != nil {
		return 0, err
	}
	for _, item := range gjson.GetBytes(body, "limits_progress").Array() {
		if item.Get("feature_name").String() == "image_gen" {
			return int(item.Get("remaining").Int()), nil
		}
	}
	return 0, nil
}

func isChatGPTWebImageBytes(raw []byte) bool {
	switch {
	case len(raw) > 8 && string(raw[:8]) == "\x89PNG\r\n\x1a\n":
		return true
	case len(raw) > 3 && raw[0] == 0xFF && raw[1] == 0xD8 && raw[2] == 0xFF:
		return true
	case len(raw) > 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP":
		return true
	default:
		return false
	}
}

func TestChatGPTWebE2ESentinelAndImageQuota(t *testing.T) {
	session := chatGPTWebE2ESession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	sentinel, err := session.fetchSentinel(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, sentinel.Token)
	require.NotEmpty(t, sentinel.ProofToken)

	remaining, err := session.imageQuota(ctx)
	require.NoError(t, err)
	t.Logf("image_gen remaining=%d vm_proof_present=%t", remaining, sentinel.TurnstileToken != "")
}

// 复用已生成的会话与指针做下载诊断，避免每次重新生图。
func TestChatGPTWebE2EDownloadExisting(t *testing.T) {
	conversationID := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_CONV_ID"))
	pointer := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_POINTER"))
	if conversationID == "" || pointer == "" {
		t.Skip("CGPT_WEB_E2E_CONV_ID / CGPT_WEB_E2E_POINTER are not set")
	}
	session := chatGPTWebE2ESession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	raw, err := session.downloadPointer(ctx, conversationID, pointer)
	require.NoError(t, err)
	require.True(t, isChatGPTWebImageBytes(raw))
	t.Logf("downloaded bytes=%d", len(raw))
	require.NoError(t, os.WriteFile("/tmp/c2a/e2e_generated.png", raw, 0o600))
}

// 参考图编辑：上传本地图片后让网页端按提示修改。
func TestChatGPTWebE2EEditImage(t *testing.T) {
	referencePath := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_REF_IMAGE"))
	if referencePath == "" {
		t.Skip("CGPT_WEB_E2E_REF_IMAGE is not set")
	}
	data, err := os.ReadFile(referencePath)
	require.NoError(t, err)
	session := chatGPTWebE2ESession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 420*time.Second)
	defer cancel()

	parsed := &OpenAIImagesRequest{
		Prompt: "change the apple to a green pear, keep the same framing and lighting",
		Uploads: []OpenAIImagesUpload{
			{FieldName: "image", FileName: "reference.png", ContentType: "image/png", Data: data},
		},
	}
	references, err := session.collectReferences(ctx, parsed, nil)
	require.NoError(t, err)
	require.Len(t, references, 1)
	t.Logf("uploaded reference file=%s", references[0].FileID)

	outcome, err := session.GenerateImage(ctx, parsed.Prompt, "gpt-image-2", "auto", references)
	require.NoError(t, err)
	defer session.cleanupImageConversation(ctx, outcome.ConversationID)
	require.NotEmpty(t, outcome.Pointers)
	t.Logf("edit conversation=%s pointers=%v", outcome.ConversationID, outcome.Pointers)

	raw, err := session.downloadPointer(ctx, outcome.ConversationID, outcome.Pointers[0])
	require.NoError(t, err)
	require.True(t, isChatGPTWebImageBytes(raw))
	t.Logf("edited image bytes=%d", len(raw))
	require.NoError(t, os.WriteFile("/tmp/c2a/e2e_edited.png", raw, 0o600))
}

// 最接近 HTTP 入口的一层：直接调转发函数，验证合成 Responses 后写出的 OpenAI 图片响应。
func chatGPTWebE2EForwardService(t *testing.T) *OpenAIGatewayService {
	t.Helper()
	proxyURL := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_PROXY"))
	return &OpenAIGatewayService{
		httpUpstream: newChatGPTWebE2ETransport(proxyURL),
		cfg:          &config.Config{},
	}
}

func chatGPTWebE2EAccount(token string) *Account {
	return &Account{
		ID:          9001,
		Name:        "chatgpt-web-e2e",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Credentials: map[string]any{"access_token": token},
		Extra:       map[string]any{featureKeyChatGPTWebImageGeneration: true},
	}
}

func chatGPTWebE2EGinContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	return context, recorder
}

func chatGPTWebE2EAssertImageResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := recorder.Body.Bytes()
	require.NotEmpty(t, payload)
	require.Equal(t, gjson.String, gjson.GetBytes(payload, "data.0.b64_json").Type)
	encoded := gjson.GetBytes(payload, "data.0.b64_json").String()
	require.NotEmpty(t, encoded)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	require.True(t, isChatGPTWebImageBytes(raw))
	t.Logf("response image bytes=%d", len(raw))
}

func TestChatGPTWebE2EForwardImagesGenerations(t *testing.T) {
	session := chatGPTWebE2ESession(t)
	token := session.accessToken
	service := chatGPTWebE2EForwardService(t)
	account := chatGPTWebE2EAccount(token)
	context, recorder := chatGPTWebE2EGinContext(t)

	parsed := &OpenAIImagesRequest{
		Endpoint:       "/v1/images/generations",
		Model:          "gpt-image-2",
		Prompt:         "a green ceramic mug on a wooden table, product photo",
		N:              1,
		Size:           "1024x1024",
		SizeTier:       "1024",
		ResponseFormat: "b64_json",
	}
	result, err := service.forwardOpenAIImagesChatGPTWeb(context.Request.Context(), context, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	chatGPTWebE2EAssertImageResponse(t, recorder)
}

// n>1：并发 n 次独立会话，响应里应当拿到 n 张图。
func TestChatGPTWebE2EForwardImagesGenerationsMultiple(t *testing.T) {
	session := chatGPTWebE2ESession(t)
	service := chatGPTWebE2EForwardService(t)
	account := chatGPTWebE2EAccount(session.accessToken)
	context, recorder := chatGPTWebE2EGinContext(t)

	parsed := &OpenAIImagesRequest{
		Endpoint:       "/v1/images/generations",
		Model:          "gpt-image-2",
		Prompt:         "a small wooden toy boat on a white table, product photo",
		N:              2,
		Size:           "1024x1024",
		SizeTier:       "1024",
		ResponseFormat: "b64_json",
	}
	result, err := service.forwardOpenAIImagesChatGPTWeb(context.Request.Context(), context, account, parsed, "")
	require.NoError(t, err)
	require.GreaterOrEqual(t, result.ImageCount, 2)

	payload := recorder.Body.Bytes()
	count := len(gjson.GetBytes(payload, "data").Array())
	require.GreaterOrEqual(t, count, 2)
	for index := 0; index < count; index++ {
		encoded := gjson.GetBytes(payload, fmt.Sprintf("data.%d.b64_json", index)).String()
		raw, err := base64.StdEncoding.DecodeString(encoded)
		require.NoError(t, err)
		require.True(t, isChatGPTWebImageBytes(raw))
	}
	t.Logf("n=2 produced %d images", count)
}

// 新账号类型：type=web 直接走网页通道，不依赖 extra 开关。
func TestChatGPTWebE2EWebAccountTypeGenerations(t *testing.T) {
	session := chatGPTWebE2ESession(t)
	service := chatGPTWebE2EForwardService(t)
	account := &Account{
		ID:          9003,
		Name:        "chatgpt-web-account-type",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeWebImage,
		Status:      StatusActive,
		Credentials: map[string]any{"access_token": session.accessToken, "chatgpt_account_id": os.Getenv("CGPT_WEB_E2E_ACCOUNT_ID")},
	}
	context, recorder := chatGPTWebE2EGinContext(t)

	parsed := &OpenAIImagesRequest{
		Endpoint:       "/v1/images/generations",
		Model:          "gpt-image-2",
		Prompt:         "a yellow rubber duck on a white table, product photo",
		N:              1,
		Size:           "1024x1024",
		SizeTier:       "1024",
		ResponseFormat: "b64_json",
	}
	result, err := service.ForwardImages(context.Request.Context(), context, account, nil, parsed, "")
	require.NoError(t, err)
	// The web upstream can return multiple assets for one accepted turn.
	require.GreaterOrEqual(t, result.ImageCount, 1)
	t.Logf("returned images=%d", result.ImageCount)
	chatGPTWebE2EAssertImageResponse(t, recorder)
	if dir := os.Getenv("CGPT_WEB_E2E_OUTPUT_DIR"); dir != "" {
		raw, decodeErr := base64.StdEncoding.DecodeString(gjson.GetBytes(recorder.Body.Bytes(), "data.0.b64_json").String())
		require.NoError(t, decodeErr)
		cfg, format, decodeErr := image.DecodeConfig(bytes.NewReader(raw))
		require.NoError(t, decodeErr)
		require.NoError(t, os.MkdirAll(dir, 0700))
		name := filepath.Join(dir, "web-image-aligned.png")
		require.NoError(t, os.WriteFile(name, raw, 0600))
		proof, _ := json.MarshalIndent(map[string]any{"account_type": "web-image", "endpoint": "/backend-api/f/conversation", "bytes": len(raw), "width": cfg.Width, "height": cfg.Height, "format": format, "sha256": fmt.Sprintf("%x", sha256.Sum256(raw))}, "", "  ")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "verification.json"), proof, 0600))
	}

}

// 面板"测试账号连接"的网页通道分支：用真实账号跑一次，断言 SSE 里带回了图片。
func TestChatGPTWebE2EAccountTestWebChannel(t *testing.T) {
	session := chatGPTWebE2ESession(t)
	proxyURL := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_PROXY"))
	service := &AccountTestService{httpUpstream: newChatGPTWebE2ETransport(proxyURL)}
	account := &Account{
		ID:          9004,
		Name:        "chatgpt-web-account-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeWebImage,
		Status:      StatusActive,
		Credentials: map[string]any{"access_token": session.accessToken},
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/api/admin/accounts/9004/test", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 420*time.Second)
	defer cancel()
	err := service.testOpenAIImageChatGPTWeb(ginContext, ctx, account, "gpt-image-2.5-sunburst", "a small blue robot toy on a white table")
	require.NoError(t, err)

	body := recorder.Body.String()
	require.Contains(t, body, `"type":"test_complete"`)
	require.Contains(t, body, `"type":"image"`)
	require.Contains(t, body, "data:image/")
	t.Logf("sse bytes=%d", len(body))
}

func TestChatGPTWebE2EForwardImagesEdits(t *testing.T) {
	referencePath := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_REF_IMAGE"))
	if referencePath == "" {
		t.Skip("CGPT_WEB_E2E_REF_IMAGE is not set")
	}
	data, err := os.ReadFile(referencePath)
	require.NoError(t, err)
	session := chatGPTWebE2ESession(t)
	service := chatGPTWebE2EForwardService(t)
	account := chatGPTWebE2EAccount(session.accessToken)
	context, recorder := chatGPTWebE2EGinContext(t)

	parsed := &OpenAIImagesRequest{
		Endpoint:       "/v1/images/edits",
		Model:          "gpt-image-2",
		Prompt:         "make the apple blue, keep the same framing and lighting",
		N:              1,
		Size:           "1024x1024",
		SizeTier:       "1024",
		ResponseFormat: "b64_json",
		Uploads: []OpenAIImagesUpload{
			{FieldName: "image", FileName: "reference.png", ContentType: "image/png", Data: data},
		},
	}
	result, err := service.forwardOpenAIImagesChatGPTWeb(context.Request.Context(), context, account, parsed, "")
	require.NoError(t, err)
	require.Equal(t, 1, result.ImageCount)
	chatGPTWebE2EAssertImageResponse(t, recorder)
}

func TestChatGPTWebE2EGenerateImage(t *testing.T) {
	session := chatGPTWebE2ESession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 420*time.Second)
	defer cancel()

	outcome, err := session.GenerateImage(ctx, "a single red apple on a white table, product photo", "gpt-image-2", "auto", nil)
	require.NoError(t, err)
	defer session.cleanupImageConversation(ctx, outcome.ConversationID)
	require.NotEmpty(t, outcome.ConversationID)
	require.NotEmpty(t, outcome.Pointers)
	t.Logf("conversation=%s pointers=%v", outcome.ConversationID, outcome.Pointers)

	raw, err := session.downloadPointer(ctx, outcome.ConversationID, outcome.Pointers[0])
	require.NoError(t, err)
	require.True(t, isChatGPTWebImageBytes(raw), "downloaded bytes are not a known image format")
	t.Logf("image bytes=%d", len(raw))
	require.NoError(t, os.WriteFile("/tmp/c2a/e2e_generated.png", raw, 0o600))
}

// Read one explicitly supplied test conversation without submitting a new turn.
func TestChatGPTWebE2EInspectConversation(t *testing.T) {
	id := strings.TrimSpace(os.Getenv("CGPT_WEB_E2E_CONV_ID"))
	if id == "" {
		t.Skip("CGPT_WEB_E2E_CONV_ID is not set")
	}
	session := chatGPTWebE2ESession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := session.doJSON(ctx, http.MethodGet, "/backend-api/conversations/"+url.PathEscape(id)+"?num_turns=10&include_has_versions=true", nil, map[string]string{"Accept": "application/json"})
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	require.NoError(t, err)
	t.Logf("conversation read status=%d retry_after=%q error_code=%q", resp.StatusCode, resp.Header.Get("Retry-After"), gjson.GetBytes(body, "detail.code").String())
}
