package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// DeepSeek 独立工具接口的计费量种类。
const (
	// DeepseekToolsUsagePerRequest 按次计费（search/fetch），Count 恒为 1。
	DeepseekToolsUsagePerRequest = "per_request"
	// DeepseekToolsUsageOCRPages OCR 页数，Count = 响应 pages 数组长度。
	DeepseekToolsUsageOCRPages = "ocr_pages"
	// DeepseekToolsUsageASRSeconds ASR 音频秒数，Count = duration_ms / 1000。
	DeepseekToolsUsageASRSeconds = "asr_seconds"
	// DeepseekToolsUsageTTSChars TTS 计费字符数（汉字等全宽字符算 2，其余算 1）。
	DeepseekToolsUsageTTSChars = "tts_chars"
)

// DeepseekToolsUsage 是一次 DeepSeek 工具调用提取出的计费量。
// service 层只负责提取，落账由 handler 完成；上游失败时 Kind 为空、Count 为 0。
type DeepseekToolsUsage struct {
	Kind  string  `json:"kind"`
	Count float64 `json:"count"`
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

	success := resp.StatusCode >= 200 && resp.StatusCode < 400
	// TTS 成功响应是音频流：逐头透传 + io.Copy，不做全量缓冲。
	if endpoint == "tts" && success {
		if err := writeDeepseekToolsContentResponse(c, resp); err != nil {
			return nil, err
		}
		return &DeepseekToolsForwardResult{
			StatusCode:      resp.StatusCode,
			Usage:           extractDeepseekToolsUsage(endpoint, body, nil, true),
			UpstreamHeaders: resp.Header,
		}, nil
	}

	data, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && (endpoint == "files_create" || endpoint == "files_delete") {
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
		Usage:           extractDeepseekToolsUsage(endpoint, body, data, success),
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

// extractDeepseekToolsUsage 从请求/响应中提取计费量，只提取、不落账。
// success=false 时（上游错误）计费量安全置零。
func extractDeepseekToolsUsage(endpoint string, reqBody, respBody []byte, success bool) DeepseekToolsUsage {
	if !success {
		return DeepseekToolsUsage{}
	}
	switch endpoint {
	case "search", "fetch":
		return DeepseekToolsUsage{Kind: DeepseekToolsUsagePerRequest, Count: 1}
	case "ocr":
		// 官方声明 usage 仅供参考，页数以 pages 数组长度为准。
		return DeepseekToolsUsage{
			Kind:  DeepseekToolsUsageOCRPages,
			Count: float64(gjson.GetBytes(respBody, "pages.#").Int()),
		}
	case "asr":
		return DeepseekToolsUsage{
			Kind:  DeepseekToolsUsageASRSeconds,
			Count: gjson.GetBytes(respBody, "duration_ms").Float() / 1000.0,
		}
	case "tts":
		text := ""
		for _, key := range []string{"text", "input"} {
			if v := strings.TrimSpace(gjson.GetBytes(reqBody, key).String()); v != "" {
				text = v
				break
			}
		}
		return DeepseekToolsUsage{
			Kind:  DeepseekToolsUsageTTSChars,
			Count: float64(countTTSBillableChars(text)),
		}
	default:
		// files_* 不计费。
		return DeepseekToolsUsage{}
	}
}

// countTTSBillableChars 按 DeepSeek TTS 官方计费规则统计字符数：
// 汉字等全宽字符算 2，其余字符算 1。
func countTTSBillableChars(text string) int {
	count := 0
	for _, r := range text {
		if isDeepseekWideRune(r) {
			count += 2
		} else {
			count++
		}
	}
	return count
}

// isDeepseekWideRune 判断是否为全宽字符（CJK 表意文字、假名、谚文、
// 全角符号等），这类字符在 TTS 计费中算 2 个字符。
func isDeepseekWideRune(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303F,   // CJK 部首、康熙部首、CJK 符号与标点
		r >= 0x3040 && r <= 0x9FFF,   // 假名、CJK 统一表意文字
		r >= 0xA000 && r <= 0xA4CF,   // 彝文
		r >= 0xAC00 && r <= 0xD7A3,   // Hangul 音节
		r >= 0xF900 && r <= 0xFAFF,   // CJK 兼容表意文字
		r >= 0xFE30 && r <= 0xFE4F,   // CJK 兼容形式
		r >= 0xFF00 && r <= 0xFF60,   // 全角 ASCII 与标点
		r >= 0xFFE0 && r <= 0xFFE6,   // 全角符号
		r >= 0x20000 && r <= 0x2FFFD, // CJK 扩展 B-F
		r >= 0x30000 && r <= 0x3FFFD: // CJK 扩展 G-
		return true
	default:
		return false
	}
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
