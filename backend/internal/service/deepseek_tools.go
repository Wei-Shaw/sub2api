package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// DeepseekToolsUsage measures successful calls, pages, whole seconds or billing characters.
type DeepseekToolsUsage struct {
	Endpoint string
	Units    int
}

func (u DeepseekToolsUsage) Model() string {
	return "deepseek-tools-" + u.Endpoint
}

// DeepseekToolsForwardResult 是 ForwardDeepseekToolsRequest 的回执：
// 计费量 + 上游状态码/响应头，供 handler 记账与观测使用。
type DeepseekToolsForwardResult struct {
	StatusCode      int
	Usage           DeepseekToolsUsage
	UpstreamHeaders http.Header
}

// A prepared request carries the verified file owner through scheduling and forwarding.
type DeepseekToolsRequest struct {
	endpoint, fileID       string
	userID, ownerAccountID int64
	anthropicFiles         bool
	upload                 deepseekUploadMetadata
	body                   []byte
	contentType            string
}

func (r *DeepseekToolsRequest) SchedulingContext(ctx context.Context) context.Context {
	return withDeepseekFileAccount(ctx, r.ownerAccountID)
}

func (s *OpenAIGatewayService) PrepareDeepseekToolsRequest(ctx context.Context, c *gin.Context, userID int64, endpoint string, body []byte, contentType string) (*DeepseekToolsRequest, error) {
	endpoint, fileID, _ := strings.Cut(endpoint, "/")
	r := &DeepseekToolsRequest{endpoint: endpoint, fileID: fileID, userID: userID, body: body, contentType: contentType,
		anthropicFiles: strings.HasPrefix(endpoint, "files_") && IsDeepseekFilesAnthropicRequest(c)}
	switch endpoint {
	case "files_get", "files_delete":
		id, err := s.DeepseekFileAccount(ctx, userID, []string{fileID})
		if err != nil {
			return nil, err
		}
		r.ownerAccountID = id
	case "files_create":
		if c != nil && c.Request != nil {
			for _, purpose := range c.Request.URL.Query()["purpose"] {
				if err := s.checkDeepseekUploadPurpose(strings.TrimSpace(purpose)); err != nil {
					return nil, err
				}
			}
		}
		meta, err := s.deepseekUploadMetadata(body, contentType)
		if err != nil {
			return nil, err
		}
		if _, err := s.deepseekFileStore(); err != nil || userID <= 0 {
			return nil, errDeepseekFileStore
		}
		r.upload = meta
	case "search", "fetch", "ocr", "asr", "tts":
	default:
		return nil, invalidFileRequest("Unsupported DeepSeek tools endpoint")
	}
	return r, nil
}

func (s *OpenAIGatewayService) ForwardDeepseekToolsRequest(ctx context.Context, c *gin.Context, account *Account, r *DeepseekToolsRequest) (*DeepseekToolsForwardResult, error) {
	if s == nil || account == nil || r == nil {
		return nil, fmt.Errorf("deepseek tools service, account and request are required")
	}
	if account.Platform != PlatformDeepseek {
		return nil, fmt.Errorf("account platform %s is not supported for deepseek tools", account.Platform)
	}
	endpoint, body, contentType := r.endpoint, r.body, r.contentType
	if (endpoint == "files_get" || endpoint == "files_delete") && (r.ownerAccountID <= 0 || account.ID != r.ownerAccountID) {
		return nil, errDeepseekFileNotFound
	}
	targetURL, err := buildDeepseekToolsURL(account, endpoint, r.fileID, r.anthropicFiles, s.validateUpstreamBaseURL)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "api_error", "Invalid DeepSeek upstream URL").WithCause(err)
	}
	if c != nil && c.Request != nil && c.Request.URL.RawQuery != "" {
		if targetURL.RawQuery != "" {
			targetURL.RawQuery += "&"
		}
		targetURL.RawQuery += c.Request.URL.RawQuery
	}
	apiKey, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	method := http.MethodPost
	switch endpoint {
	case "files_get":
		method = http.MethodGet
	case "files_delete":
		method = http.MethodDelete
	}

	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	var reader io.Reader
	if method == http.MethodPost {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(upstreamCtx), method, targetURL.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if r.anthropicFiles {
		// 官方 /anthropic/v1/files 要求透传 anthropic-beta（files-api-2025-04-14），
		// anthropic-version 一并保留以贴近客户端原始请求。
		for _, beta := range c.Request.Header.Values("anthropic-beta") {
			req.Header.Add("anthropic-beta", beta)
		}
		if version := strings.TrimSpace(c.GetHeader("anthropic-version")); version != "" {
			req.Header.Set("anthropic-version", version)
		}
	}
	if method == http.MethodPost {
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/json"
		}
		// multipart 时必须保留原 boundary，body 已由 handler 原样读入。
		req.Header.Set("Content-Type", contentType)
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		// 网络层错误统一走既有 transport error 通道，供 handler 按需 failover。
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	success := resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices
	// TTS 成功响应是音频流：逐头透传 + io.Copy，不做全量缓冲。
	if endpoint == "tts" && success {
		if err := writeDeepseekToolsContentResponse(c, resp); err != nil {
			return nil, err
		}
		return &DeepseekToolsForwardResult{
			StatusCode:      resp.StatusCode,
			Usage:           extractDeepseekToolsUsage(endpoint, body, nil, resp.StatusCode),
			UpstreamHeaders: resp.Header,
		}, nil
	}

	data, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if success && (endpoint == "files_create" || endpoint == "files_delete") {
		store, err := s.deepseekFileStore()
		if err != nil {
			return nil, err
		}
		if endpoint == "files_create" {
			file, err := deepseekUploadedFile(data, account.ID, r.anthropicFiles, r.upload)
			if err != nil {
				return nil, infraerrors.New(http.StatusBadGateway, "api_error", "Invalid upstream file metadata").WithCause(err)
			}
			if err := store.SaveDeepseekFile(upstreamCtx, r.userID, file); err != nil {
				return nil, errDeepseekFileStore.WithCause(err)
			}
		} else {
			if resp.StatusCode != http.StatusNoContent && !deepseekFileDeleted(data, r.fileID, r.anthropicFiles) {
				return nil, infraerrors.New(http.StatusBadGateway, "api_error", "Upstream did not confirm file deletion")
			}
			if err := store.DeleteDeepseekFile(upstreamCtx, r.userID, account.ID, r.fileID); err != nil {
				return nil, errDeepseekFileStore.WithCause(err)
			}
		}
	}
	writeGrokMediaResponse(c, resp, data, s.responseHeaderFilter)
	return &DeepseekToolsForwardResult{
		StatusCode:      resp.StatusCode,
		Usage:           extractDeepseekToolsUsage(endpoint, body, data, resp.StatusCode),
		UpstreamHeaders: resp.Header,
	}, nil
}

// writeDeepseekToolsContentResponse 以流式方式回写上游二进制响应
// （TTS 音频），避免长音频（可达 30 分钟）全量缓冲在内存里。
func writeDeepseekToolsContentResponse(c *gin.Context, resp *http.Response) error {
	if c == nil || resp == nil || resp.Body == nil {
		return fmt.Errorf("deepseek tools content response is incomplete")
	}
	for _, name := range []string{
		"Content-Type",
		"Content-Length",
		"Content-Range",
		"Accept-Ranges",
		"Content-Disposition",
	} {
		if value := strings.TrimSpace(resp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Length")) == "" && resp.ContentLength >= 0 {
		c.Header("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	if strings.TrimSpace(c.Writer.Header().Get("Content-Type")) == "" {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Status(resp.StatusCode)
	MarkResponseCommitted(c)
	_, err := io.Copy(c.Writer, resp.Body)
	return err
}

// Search, fetch and OCR require a completed HTTP 200 result. OCR can return
// an error object after sending keepalive newlines and a 200 status.
func extractDeepseekToolsUsage(endpoint string, reqBody, respBody []byte, status int) DeepseekToolsUsage {
	if status < http.StatusOK || status >= http.StatusMultipleChoices || status == http.StatusNoContent || status == http.StatusResetContent {
		return DeepseekToolsUsage{}
	}
	switch endpoint {
	case "tts":
		text := gjson.GetBytes(reqBody, "text")
		if !text.Exists() {
			text = gjson.GetBytes(reqBody, "input")
		}
		if text.Type == gjson.String {
			return DeepseekToolsUsage{Endpoint: endpoint, Units: countTTSBillableChars(text.Str)}
		}
		return DeepseekToolsUsage{}
	case "search", "fetch", "ocr":
		if status != http.StatusOK {
			return DeepseekToolsUsage{}
		}
	case "asr":
	default:
		return DeepseekToolsUsage{}
	}

	if !gjson.ValidBytes(respBody) || gjson.GetBytes(respBody, "error").Type != gjson.Null {
		return DeepseekToolsUsage{}
	}
	units := 0
	switch endpoint {
	case "search":
		if results := gjson.GetBytes(respBody, "results"); results.IsArray() && results.Get("0").Exists() {
			units = 1
		}
	case "fetch":
		if content := gjson.GetBytes(respBody, "content"); content.Type == gjson.String && strings.TrimSpace(content.Str) != "" {
			units = 1
		}
	case "ocr":
		object := gjson.GetBytes(respBody, "object")
		if object.Exists() && object.String() != "ocr.result" {
			return DeepseekToolsUsage{}
		}
		if !object.Exists() && !gjson.GetBytes(respBody, "pages").IsArray() {
			return DeepseekToolsUsage{}
		}
		if pages := gjson.GetBytes(respBody, "page_count"); pages.Exists() {
			if pages.Type == gjson.Number && pages.Float() == float64(pages.Int()) {
				units = int(pages.Int())
			}
		} else if pages := gjson.GetBytes(respBody, "pages"); pages.IsArray() {
			units = int(pages.Get("#").Int())
		}
	case "asr":
		if ms := gjson.GetBytes(respBody, "duration_ms"); ms.Type == gjson.Number {
			units = int(math.Ceil(ms.Float() / 1000))
		}
	}
	return DeepseekToolsUsage{Endpoint: endpoint, Units: max(0, units)}
}

func countTTSBillableChars(text string) int {
	count := 0
	for _, r := range text {
		count++
		if unicode.Is(unicode.Han, r) {
			count++
		}
	}
	return count
}

// deepseekFilesBetaHeader 是官方 Anthropic 兼容 Files API 的 beta 标志。
const deepseekFilesBetaHeader = "files-api-2025-04-14"

// IsDeepseekFilesAnthropicRequest identifies the Anthropic Files API beta token.
func IsDeepseekFilesAnthropicRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	for _, value := range c.Request.Header.Values("anthropic-beta") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), deepseekFilesBetaHeader) {
				return true
			}
		}
	}
	return false
}
