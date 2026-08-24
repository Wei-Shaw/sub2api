// Package atlascloud 封装 atlascloud 上游的异步媒体生成协议
// （图像/视频生成 + prediction 轮询），并把它适配成与 internal/pkg/fal
// 相同的语义（SubmitResponse / StatusResponse / ResultRaw），
// 以便直接复用 service 层的异步视频/图片执行内核。
//
// atlascloud 协议（相对账户 credential "base_url"）：
//
//	submit video  POST {base}/api/v1/model/generateVideo -> { id, status, ... }
//	submit image  POST {base}/api/v1/model/generateImage -> { id, status, ... }
//	upload media  POST {base}/api/v1/model/uploadMedia
//	pollGET  {base}/api/v1/model/prediction/{id} -> { status, outputs:[...] }
//
// 鉴权头：Authorization: Bearer {api_key}
//
// status 取值：processing / completed / failed / timeout
//   - completed          → 终态成功
//   - failed / timeout   → 终态失败（映射为 *fal.APIError，交由上层退费）
//   - 其它（processing…） → 仍在进行
package atlascloud

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	proxyDialTimeout               = 10 * time.Second
	proxyTLSHandshakeTimeout       = 10 * time.Second
	defaultClientTimeout           = 120 * time.Second
	bodyLimit                int64 = 32 << 20
	promptLogLimitBytes            = 1024

	// pathGenerateVideo/pathGenerateImage/pathPrediction 为上游相对路径。
	pathGenerateVideo = "/api/v1/model/generateVideo"
	pathGenerateImage = "/api/v1/model/generateImage"
	pathUploadMedia   = "/api/v1/model/uploadMedia"
	pathPrediction    = "/api/v1/model/prediction"
)

// atlascloud prediction 状态取值。
const (
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusFailed     = "failed"
	statusTimeout    = "timeout"
)

// Client 是 atlascloud 的 HTTP 客户端。
// 对外暴露与 fal.Client 相同签名的方法子集，供 service 层多平台分派。
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

// Config 构造 atlascloud 客户端所需的配置。
type Config struct {
	APIKey   string // 上游 api_key（必填），鉴权为 Authorization: Bearer {api_key}
	BaseURL  string // 上游 base_url（必填，无默认值）
	ProxyURL string // 可选代理
	Timeout  time.Duration
}

// NewClient 创建 atlascloud 客户端。
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("atlascloud: api key is required")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, errors.New("atlascloud: base url is required")
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	client := &http.Client{Timeout: timeout}

	_, parsed, err := proxyurl.Parse(cfg.ProxyURL)
	if err != nil {
		return nil, err
	}
	if parsed != nil {
		transport := &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: proxyDialTimeout,
			}).DialContext,
			TLSHandshakeTimeout: proxyTLSHandshakeTimeout,
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, fmt.Errorf("atlascloud: configure proxy: %w", err)
		}
		client.Transport = transport
	}

	return &Client{
		httpClient: client,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		baseURL:    base,
	}, nil
}

// predictionResponse 是 GET prediction/{id} 与 generate* 提交返回的统一结构。
//
// 除结构化字段外保留 Raw（完整 JSON map）用于结果透传。
type predictionResponse struct {
	ID      string              `json:"id"`
	Model   string              `json:"model,omitempty"`
	Status  string              `json:"status"`
	Outputs []string            `json:"outputs,omitempty"`
	Error   string              `json:"error,omitempty"`
	Message string              `json:"message,omitempty"`
	Data    *predictionResponse `json:"data,omitempty"`

	Raw map[string]any `json:"-"`
}

// SubmitRaw 向generateVideo 端点提交异步任务。
//
// body 为客户端原始 payload（已含 model 字段），直接透传给上游。
// 上游返回 { id, status, ... }，映射为 fal.SubmitResponse：
//   - RequestID = id
//   - Status    = IN_QUEUE（或按上游 status 归一）
//   - StatusURL / ResponseURL = {base}/api/v1/model/prediction/{id}
//
// 注意：model 参数仅用于日志/回退，实际模型名以 body 中的 model 字段为准。
func (c *Client) SubmitRaw(ctx context.Context, model string, body any) (*fal.SubmitResponse, error) {
	_ = model
	endpoint := c.baseURL + pathGenerateVideo
	resp, err := c.doPrediction(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(resp.ID)
	if id == "" {
		return nil, &fal.APIError{StatusCode: http.StatusBadGateway, Body: "atlascloud: submit response missing id"}
	}
	predictionURL := c.buildPredictionURL(id)
	return &fal.SubmitResponse{
		RequestID:   id,
		Status:      fal.StatusInQueue,
		StatusURL:   predictionURL,
		ResponseURL: predictionURL,
	}, nil
}

// Status 查询任务状态。atlascloud 用同一个 prediction/{id} 端点承载状态与结果。
//
// status 映射：
//   - completed        → fal.StatusCompleted（IsTerminal=true）
//   - failed / timeout → 返回 *fal.APIError（HTTP 502），交由上层退费
//   - 其它             → fal.StatusInProgress（未终结）
func (c *Client) Status(ctx context.Context, statusURL string) (*fal.StatusResponse, error) {
	if strings.TrimSpace(statusURL) == "" {
		return nil, errors.New("atlascloud: status url is empty")
	}
	resp, err := c.doPrediction(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, predictionStatusError(err)
	}
	out := &fal.StatusResponse{
		RequestID:   resp.ID,
		ResponseURL: statusURL,
	}
	switch strings.ToLower(strings.TrimSpace(resp.Status)) {
	case statusCompleted:
		out.Status = fal.StatusCompleted
		if resp.Outputs != nil {
			out.Result = normalizeVideoResult(resp.Outputs)
		}
	case statusFailed, statusTimeout:
		// 终态失败：用 4xx 让上层 pollOnce 走 markFailedAndRefund 分支。
		reason := firstNonEmpty(resp.Error, resp.Message, resp.Status)
		return nil, &fal.APIError{
			StatusCode: http.StatusBadRequest,
			Body:       fmt.Sprintf("atlascloud upstream %s: %s", resp.Status, reason),
		}
	default:
		out.Status = fal.StatusInProgress
	}
	return out, nil
}

// predictionStatusError prevents a failed status response from leaking the
// complete provider payload into task history. AtlasCloud returns the useful
// user-facing reason in data.error, while the rest of data contains internal
// task metadata that is neither useful nor appropriate for error_reason.
func predictionStatusError(err error) error {
	var apiErr *fal.APIError
	if !errors.As(err, &apiErr) {
		return err
	}

	reason := "upstream request failed"
	var response predictionResponse
	if json.Unmarshal([]byte(apiErr.Body), &response) == nil {
		response.applyDataFallback()
		if parsed := strings.TrimSpace(response.Error); parsed != "" {
			reason = parsed
		}
	}
	return &fal.APIError{
		StatusCode: apiErr.StatusCode,
		Body:       reason,
		RequestID:  apiErr.RequestID,
	}
}

// ResultRaw 拉取任务最终结果的原始 payload。
//
// atlascloud 结果结构为 { outputs:["https://.../result.mp4", ...] }。
// 对外收敛为通用视频结构 {video:{url,file_name}, videos:[...]}，不透传
// id/status/model/metrics/时间戳等 atlascloud 私有任务字段。
func (c *Client) ResultRaw(ctx context.Context, responseURL string) (map[string]any, error) {
	if strings.TrimSpace(responseURL) == "" {
		return nil, errors.New("atlascloud: response url is empty")
	}
	resp, err := c.doPrediction(ctx, http.MethodGet, responseURL, nil)
	if err != nil {
		return nil, err
	}
	return normalizeVideoResult(resp.Outputs), nil
}

func normalizeVideoResult(outputs []string) map[string]any {
	out := make(map[string]any, 2)
	videos := make([]any, 0, len(outputs))
	for _, u := range outputs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		video := map[string]any{"url": u}
		if fileName := atlasCloudVideoFileNameFromURL(u); fileName != "" {
			video["file_name"] = fileName
		}
		videos = append(videos, video)
	}
	if len(videos) == 0 {
		return out
	}

	// 第一个作为主 video 对象，全部放入 videos 数组。
	if first, ok := videos[0].(map[string]any); ok {
		out["video"] = first
	}
	out["videos"] = videos
	return out
}

func atlasCloudVideoFileNameFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		return ""
	}
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

// BuildStatusURL 回退拼接 status url（atlascloud 状态/结果同一端点）。
func (c *Client) BuildStatusURL(_ /*model*/, requestID string) string {
	return c.buildPredictionURL(requestID)
}

// BuildResponseURL 回退拼接 result url（atlascloud 状态/结果同一端点）。
func (c *Client) BuildResponseURL(_ /*model*/, requestID string) string {
	return c.buildPredictionURL(requestID)
}

// BuildCancelURL atlascloud 无取消端点，返回空串（Cancel 遇空 URL 直接 no-op）。
func (c *Client) BuildCancelURL(_ /*model*/, _ /*requestID*/ string) string {
	return ""
}

// Cancel atlascloud 暂不支持任务取消：空 URL 时静默返回，不视为错误。
func (c *Client) Cancel(_ context.Context, cancelURL string) error {
	if strings.TrimSpace(cancelURL) == "" {
		return nil
	}
	return nil
}

// buildPredictionURL 拼接 prediction/{id} 轮询地址。
func (c *Client) buildPredictionURL(id string) string {
	return fmt.Sprintf("%s%s/%s", c.baseURL, pathPrediction, url.PathEscape(strings.TrimSpace(id)))
}

// doPrediction 执行一次 HTTP 请求并解析为 predictionResponse（含原始 Raw map）。
func (c *Client) doPrediction(ctx context.Context, method, endpoint string, reqBody any) (*predictionResponse, error) {
	raw, err := c.doJSON(ctx, method, endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	out := &predictionResponse{}
	if len(bytes.TrimSpace(raw)) == 0 {
		out.Raw = make(map[string]any)
		return out, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("atlascloud: decode response: %w", err)
	}
	out.applyDataFallback()
	rawMap := make(map[string]any)
	if err := json.Unmarshal(raw, &rawMap); err == nil {
		out.Raw = rawMap
	} else {
		out.Raw = make(map[string]any)
	}
	return out, nil
}

// applyDataFallback 兼容 atlascloud 将业务字段包在 data 对象中的响应格式。
// 顶层字段仍优先，避免改变旧格式响应的行为。
func (r *predictionResponse) applyDataFallback() {
	if r == nil || r.Data == nil {
		return
	}
	data := r.Data
	if strings.TrimSpace(r.ID) == "" {
		r.ID = data.ID
	}
	if strings.TrimSpace(r.Model) == "" {
		r.Model = data.Model
	}
	if strings.TrimSpace(r.Status) == "" {
		r.Status = data.Status
	}
	if len(r.Outputs) == 0 {
		r.Outputs = data.Outputs
	}
	if strings.TrimSpace(r.Error) == "" {
		r.Error = data.Error
	}
	if strings.TrimSpace(r.Message) == "" {
		r.Message = data.Message
	}
}

// doJSON 执行一次 HTTP 请求，序列化 reqBody（可为 nil）并返回原始响应体。
// 非 2xx 返回 *fal.APIError（复用 fal 的错误类型，让 service 层的
// errors.As(err, &fal.APIError) 分支统一处理退费/脱敏）。
func (c *Client) doJSON(ctx context.Context, method, endpoint string, reqBody any) ([]byte, error) {
	var bodyReader io.Reader
	var rawBody []byte
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("atlascloud: marshal request: %w", err)
		}
		rawBody = buf
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("atlascloud: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	requestID := newRequestID()
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("atlascloud_http_request",
			"request_id", requestID,
			"method", method,
			"endpoint", endpoint,
			"url", req.URL.String(),
			"headers", headersForLog(req.Header),
			"body", requestBodyForLog(rawBody),
			"body_bytes", len(rawBody),
		)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("atlascloud: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("atlascloud: read response: %w", err)
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("atlascloud_http_response",
			"request_id", requestID,
			"method", method,
			"endpoint", endpoint,
			"status", resp.StatusCode,
			"body_bytes", len(raw),
			"body", string(raw),
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &fal.APIError{StatusCode: resp.StatusCode, Body: string(raw), RequestID: requestID}
	}
	return raw, nil
}

func headersForLog(headers http.Header) http.Header {
	logged := headers.Clone()
	for name := range logged {
		if strings.EqualFold(name, "Authorization") {
			logged.Set(name, "[REDACTED]")
		}
	}
	return logged
}

// requestBodyForLog retains the request shape while limiting prompt values.
// It only changes the copy used by logs; the outbound body remains untouched.
func requestBodyForLog(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	truncatePromptValues(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(encoded)
}

func truncatePromptValues(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if strings.EqualFold(key, "prompt") {
				if prompt, ok := child.(string); ok {
					current[key] = truncateUTF8Bytes(prompt, promptLogLimitBytes)
				}
				continue
			}
			truncatePromptValues(child)
		}
	case []any:
		for _, child := range current {
			truncatePromptValues(child)
		}
	}
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}

	const marker = "...(truncated)"
	if limit <= len(marker) {
		return marker[:limit]
	}
	cutoff := limit - len(marker)
	for cutoff > 0 && !utf8.ValidString(value[:cutoff]) {
		cutoff--
	}
	return value[:cutoff] + marker
}

// newRequestID 生成本次调用的日志追踪 id。
func newRequestID() string {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		return "atlas-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "atlas-" + hex.EncodeToString(buf[:])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "unknown error"
}

// UnusedPaths 保留 image/upload 端点常量的引用，供后续图片链路扩展时使用。
// 当前视频链路先不涉及，用一个不导出的占位避免 lint 报未使用常量。
var _ = []string{pathGenerateImage, pathUploadMedia}
