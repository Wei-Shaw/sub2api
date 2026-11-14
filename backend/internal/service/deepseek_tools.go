package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

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

// ForwardDeepseekToolsRequest 把 DeepSeek 独立工具接口的九个端点统一透传到上游：
// search/fetch/ocr/asr/tts/files_create 为 POST（含 JSON 或 multipart body），
// files_list/files_get 为 GET、files_delete 为 DELETE（无 body）。
// 响应回写：TTS 成功响应为二进制音频，走流式回写避免长音频全量缓冲；
// 其余端点（含上游错误体）经缓冲后按状态码 + Content-Type + 过滤响应头原样透传。
func (s *OpenAIGatewayService) ForwardDeepseekToolsRequest(ctx context.Context, c *gin.Context, account *Account, endpoint string, body []byte, contentType string) (*DeepseekToolsForwardResult, error) {
	if s == nil || account == nil {
		return nil, fmt.Errorf("deepseek tools service/account is required")
	}
	if account.Platform != PlatformDeepseek {
		return nil, fmt.Errorf("account platform %s is not supported for deepseek tools", account.Platform)
	}
	endpoint, fileID := splitDeepseekToolsEndpoint(endpoint)
	targetURL, err := buildDeepseekToolsURL(account, endpoint, fileID)
	if err != nil {
		return nil, err
	}
	apiKey, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	method := http.MethodPost
	switch endpoint {
	case "files_list", "files_get":
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
	req, err := http.NewRequestWithContext(upstreamCtx, method, targetURL, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
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
	writeGrokMediaResponse(c, resp, data, s.responseHeaderFilter)
	return &DeepseekToolsForwardResult{
		StatusCode:      resp.StatusCode,
		Usage:           extractDeepseekToolsUsage(endpoint, body, data, success),
		UpstreamHeaders: resp.Header,
	}, nil
}

// splitDeepseekToolsEndpoint 拆出基础 endpoint 与可选的 file ID 后缀，
// 例如 "files_get/file-abc" → ("files_get", "file-abc")。file ID 的合法性
// 由 buildDeepseekToolsURL 的白名单校验兜底。
func splitDeepseekToolsEndpoint(endpoint string) (string, string) {
	endpoint = strings.Trim(strings.TrimSpace(endpoint), "/")
	base, fileID, hasID := strings.Cut(endpoint, "/")
	if hasID {
		fileID = strings.Trim(fileID, "/")
	}
	return base, fileID
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
