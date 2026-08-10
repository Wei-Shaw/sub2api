// Package apiz 封装 apiz 上游的异步视频生成协议（任务创建 + 任务查询），
// 并把它适配成与 internal/pkg/fal 相同的语义
// （SubmitResponse / StatusResponse / ResultRaw），
// 以便直接复用 service 层的异步视频执行内核。
//
// apiz 协议（相对账户 credential "base_url"，默认 https://api.apiz.ai）：
//
//	createPOST {base}/api/v3/tasks/create -> { task_id, status, ... }
//	query   POST {base}/api/v3/tasks/querybody={ task_id } -> { status, video_url|outputs, ... }
//
// 注意：query 是 POST 且 task_id 放在 body 里，而执行内核的轮询接口以
// URL 字符串传递（Status(ctx, statusURL)）。因此这里把 task_id 编码进
// statusURL 的 query 参数（{base}/api/v3/tasks/query?task_id=xxx），
// 实际发请求时再取出放进 POST body，对上层保持无感。
//
// 鉴权头：Authorization: Bearer {api_key}
//
// 提交参数（由客户端 payload 透传，不在此处校验）：
//
//	prompt(必填,1-5000) / duration(480P:4-30, 720P:4-29, 默认8)
//	resolution(480P|720P, 默认720P) / aspect_ratio(21:9|16:9|4:3|1:1|3:4|9:16)
//	audio(bool) / image_url(首帧,带图即图生视频) / end_image_url(尾帧,需同时给image_url)
//	reference_image_urls(<=30) / reference_video_urls(<=10) / reference_audio_urls(<=10)
package apiz

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

	"github.com/Wei-Shaw/sub2api/internal/pkg/fal"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyurl"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
)

const (
	proxyDialTimeout               = 10 * time.Second
	proxyTLSHandshakeTimeout       = 10 * time.Second
	defaultClientTimeout           = 120 * time.Second
	bodyLimit                int64 = 32 << 20

	// pathTasksCreate/pathTasksQuery 为上游相对路径（均为 POST）。
	pathTasksCreate = "/api/v3/tasks/create"
	pathTasksQuery  = "/api/v3/tasks/query"

	// queryParamTaskID 是把 task_id 携带在 statusURL 上的 query 参数名。
	queryParamTaskID = "task_id"
)

// terminalSuccessStatuses / terminalFailureStatuses 是 apiz 任务状态的归一化集合。
// 上游状态字面量可能随版本演进，这里对常见同义词做宽容匹配；
// 未命中任何集合的状态一律视为"仍在进行"，由上层按超时策略收敛。
var (
	terminalSuccessStatuses = map[string]struct{}{
		"completed": {}, "complete": {}, "succeeded": {}, "success": {},
		"finished": {}, "done": {},
	}
	terminalFailureStatuses = map[string]struct{}{
		"failed": {}, "failure": {}, "error": {}, "timeout": {},
		"cancelled": {}, "canceled": {}, "rejected": {},
	}
)

// Client 是 apiz 的 HTTP 客户端。
// 对外暴露与 fal.Client 相同签名的方法子集，供 service 层多平台分派。
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

// Config 构造 apiz 客户端所需的配置。
type Config struct {
	APIKey   string // 上游 api_key（必填），鉴权为 Authorization: Bearer {api_key}
	BaseURL  string // 上游 base_url（必填，由账号凭证或平台默认值提供）
	ProxyURL string // 可选代理
	Timeout  time.Duration
}

// NewClient 创建 apiz 客户端。
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("apiz: api key is required")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		return nil, errors.New("apiz: base url is required")
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
			return nil, fmt.Errorf("apiz: configure proxy: %w", err)
		}
		client.Transport = transport
	}

	return &Client{
		httpClient: client,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		baseURL:    base,
	}, nil
}

// taskResponse 是 tasks/create 与 tasks/query 返回的统一结构。
//
// 上游可能把业务字段放在顶层，也可能包在 data 里，两种都兼容。
// 除结构化字段外保留 Raw（完整 JSON map）用于结果透传。
type taskResponse struct {
	TaskID   string `json:"task_id"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	State    string `json:"state"`
	VideoURL string `json:"video_url"`
	Error    string `json:"error"`
	Message  string `json:"message"`
	Msg      string `json:"msg"`

	Data map[string]any `json:"data"`
	Raw  map[string]any `json:"-"`
}

// taskID 返回任务 id（顶层优先，回退 data 内的 task_id / id）。
func (r *taskResponse) taskID() string {
	if id := strings.TrimSpace(r.TaskID); id != "" {
		return id
	}
	if id := strings.TrimSpace(r.ID); id != "" {
		return id
	}
	return strings.TrimSpace(firstStringField(r.Data, "task_id", "id"))
}

// statusValue 返回任务状态（顶层优先，回退 data 内的 status / state）。
func (r *taskResponse) statusValue() string {
	if s := strings.TrimSpace(r.Status); s != "" {
		return s
	}
	if s := strings.TrimSpace(r.State); s != "" {
		return s
	}
	return strings.TrimSpace(firstStringField(r.Data, "status", "state"))
}

// failureReason 返回失败原因（用于错误信息拼接）。
func (r *taskResponse) failureReason() string {
	return firstNonEmpty(
		r.Error, r.Message, r.Msg,
		firstStringField(r.Data, "error", "message", "msg", "fail_reason"),
		r.statusValue(),
	)
}

// SubmitRaw 向 tasks/create 端点提交异步视频任务。
//
// body 为客户端原始 payload（prompt / duration / resolution / aspect_ratio /
// audio / image_url 等），直接透传给上游，不在此处做参数校验。
// 上游返回 { task_id, status, ... }，映射为 fal.SubmitResponse：
//   - RequestID = task_id
//   - Status    = IN_QUEUE
//   - StatusURL / ResponseURL = {base}/api/v3/tasks/query?task_id={task_id}
//
// 注意：model 参数仅用于日志/回退，实际模型以 body 中的字段为准。
func (c *Client) SubmitRaw(ctx context.Context, model string, body any) (*fal.SubmitResponse, error) {
	_ = model
	resp, err := c.doTask(ctx, c.baseURL+pathTasksCreate, body)
	if err != nil {
		return nil, err
	}
	taskID := resp.taskID()
	if taskID == "" {
		return nil, &fal.APIError{StatusCode: http.StatusBadGateway, Body: "apiz: create response missing task_id"}
	}
	queryURL := c.buildQueryURL(taskID)
	return &fal.SubmitResponse{
		RequestID:   taskID,
		Status:      fal.StatusInQueue,
		StatusURL:   queryURL,
		ResponseURL: queryURL,
	}, nil
}

// Status 查询任务状态。apiz 用同一个 tasks/query 端点承载状态与结果。
//
// status 映射：
//   - completed / succeeded / success / finished / done → fal.StatusCompleted
//   - failed / error / timeout / cancelled …→ *fal.APIError（HTTP 400），交由上层退费
//   - 其它（processing / pending / running …）          → fal.StatusInProgress
func (c *Client) Status(ctx context.Context, statusURL string) (*fal.StatusResponse, error) {
	resp, err := c.queryTask(ctx, statusURL)
	if err != nil {
		return nil, err
	}
	status := strings.ToLower(strings.TrimSpace(resp.statusValue()))
	out := &fal.StatusResponse{
		RequestID:   resp.taskID(),
		ResponseURL: statusURL,
	}
	switch {
	case isTerminalSuccess(status):
		out.Status = fal.StatusCompleted
	case isTerminalFailure(status):
		// 终态失败：用 4xx 让上层 pollOnce 走 markFailedAndRefund 分支。
		return nil, &fal.APIError{
			StatusCode: http.StatusBadRequest,
			Body:       fmt.Sprintf("apiz upstream %s: %s", firstNonEmpty(status, "failed"), resp.failureReason()),
		}
	default:
		out.Status = fal.StatusInProgress
	}
	return out, nil
}

// ResultRaw 拉取任务最终结果的原始 payload。
//
// 上游结果可能形如 { video_url: "..." } 或 { outputs: [...] } / { videos: [...] }，
// 也可能嵌在 data 里。为兼容 service 层的 fal.ExtractVideoURLs
// （识别 {video:{url}} / {videos:[{url}]}），这里把抽取到的视频地址
// 统一映射成 fal 风格结构，并保留原始字段透传给客户端。
func (c *Client) ResultRaw(ctx context.Context, responseURL string) (map[string]any, error) {
	resp, err := c.queryTask(ctx, responseURL)
	if err != nil {
		return nil, err
	}
	out := resp.Raw
	if out == nil {
		out = make(map[string]any)
	}
	urls := collectVideoURLs(resp)
	if len(urls) > 0 {
		videos := make([]any, 0, len(urls))
		for _, u := range urls {
			videos = append(videos, map[string]any{"url": u})
		}
		// 第一个作为主 video 对象，全部放入 videos 数组。
		if first, ok := videos[0].(map[string]any); ok {
			out["video"] = first
		}
		out["videos"] = videos
	}
	return out, nil
}

// BuildStatusURL 回退拼接 status url（apiz 状态/结果同一端点）。
func (c *Client) BuildStatusURL(_ /*model*/, requestID string) string {
	return c.buildQueryURL(requestID)
}

// BuildResponseURL 回退拼接 result url（apiz 状态/结果同一端点）。
func (c *Client) BuildResponseURL(_ /*model*/, requestID string) string {
	return c.buildQueryURL(requestID)
}

// BuildCancelURL apiz 无取消端点，返回空串（Cancel 遇空 URL 直接 no-op）。
func (c *Client) BuildCancelURL(_ /*model*/, _ /*requestID*/ string) string {
	return ""
}

// Cancel apiz 暂不支持任务取消：空 URL 时静默返回，不视为错误。
func (c *Client) Cancel(_ context.Context, cancelURL string) error {
	if strings.TrimSpace(cancelURL) == "" {
		return nil
	}
	return nil
}

// buildQueryURL 拼接轮询地址，把 task_id 编码在query 参数上。
// 真正请求时会由 queryTask 取出 task_id 放进 POST body。
func (c *Client) buildQueryURL(taskID string) string {
	id := strings.TrimSpace(taskID)
	if id == "" {
		return ""
	}
	return fmt.Sprintf("%s%s?%s=%s", c.baseURL, pathTasksQuery, queryParamTaskID, url.QueryEscape(id))
}

// queryTask 以 POST 方式查询任务：从 statusURL 中解析出 task_id，
// 去掉 query 串后向 tasks/query 端点提交 { "task_id": ... }。
func (c *Client) queryTask(ctx context.Context, statusURL string) (*taskResponse, error) {
	endpoint, taskID, err := splitQueryURL(statusURL)
	if err != nil {
		return nil, err
	}
	return c.doTask(ctx, endpoint, map[string]any{queryParamTaskID: taskID})
}

// splitQueryURL 把 {base}/api/v3/tasks/query?task_id=xxx 拆成端点与 task_id。
func splitQueryURL(statusURL string) (endpoint, taskID string, err error) {
	trimmed := strings.TrimSpace(statusURL)
	if trimmed == "" {
		return "", "", errors.New("apiz: status url is empty")
	}
	parsed, parseErr := url.Parse(trimmed)
	if parseErr != nil {
		return "", "", fmt.Errorf("apiz: parse status url: %w", parseErr)
	}
	taskID = strings.TrimSpace(parsed.Query().Get(queryParamTaskID))
	if taskID == "" {
		return "", "", errors.New("apiz: status url missing task_id")
	}
	parsed.RawQuery = ""
	return parsed.String(), taskID, nil
}

// doTask 执行一次 POST 请求并解析为 taskResponse（含原始 Raw map）。
func (c *Client) doTask(ctx context.Context, endpoint string, reqBody any) (*taskResponse, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, endpoint, reqBody)
	if err != nil {
		return nil, err
	}
	out := &taskResponse{}
	if len(bytes.TrimSpace(raw)) == 0 {
		out.Raw = make(map[string]any)
		return out, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("apiz: decode response: %w", err)
	}
	rawMap := make(map[string]any)
	if err := json.Unmarshal(raw, &rawMap); err == nil {
		out.Raw = rawMap
	} else {
		out.Raw = make(map[string]any)
	}
	return out, nil
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
			return nil, fmt.Errorf("apiz: marshal request: %w", err)
		}
		rawBody = buf
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("apiz: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	requestID := newRequestID()
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("apiz_http_request",
			"request_id", requestID,
			"method", method,
			"endpoint", endpoint,
			"body_bytes", len(rawBody),
		)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apiz: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("apiz: read response: %w", err)
	}

	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		slog.Debug("apiz_http_response",
			"request_id", requestID,
			"method", method,
			"endpoint", endpoint,
			"status", resp.StatusCode,
			"body_bytes", len(raw),
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &fal.APIError{StatusCode: resp.StatusCode, Body: string(raw), RequestID: requestID}
	}
	return raw, nil
}

// isTerminalSuccess / isTerminalFailure 判定归一化后的状态是否为终态。
func isTerminalSuccess(status string) bool {
	_, ok := terminalSuccessStatuses[status]
	return ok
}

func isTerminalFailure(status string) bool {
	_, ok := terminalFailureStatuses[status]
	return ok
}

// collectVideoURLs 从响应中抽取视频地址，按video_url → data.video_url →
// outputs/videos/urls 数组的顺序合并，去重且保持顺序。
func collectVideoURLs(resp *taskResponse) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	appendURL := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, dup := seen[v]; dup {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	appendURL(resp.VideoURL)
	appendURL(firstStringField(resp.Data, "video_url", "url"))

	arrayKeys := []string{"outputs", "videos", "video_urls", "urls"}
	for _, container := range []map[string]any{resp.Raw, resp.Data} {
		if container == nil {
			continue
		}
		for _, key := range arrayKeys {
			for _, v := range stringsFromAny(container[key]) {
				appendURL(v)
			}
		}
	}
	return out
}

// stringsFromAny 把任意 JSON 值展开成字符串切片：
// 支持字符串、字符串数组、以及 [{url: "..."}] 形式的对象数组。
func stringsFromAny(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return []string{v}
	case map[string]any:
		if s := firstStringField(v, "url", "video_url"); s != "" {
			return []string{s}
		}
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, stringsFromAny(item)...)
		}
		return out
	default:
		return nil
	}
}

// firstStringField 返回 m 中第一个存在且为非空字符串的 key 对应值。
func firstStringField(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// newRequestID 生成本次调用的日志追踪 id。
func newRequestID() string {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		return "apiz-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "apiz-" + hex.EncodeToString(buf[:])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return "unknown error"
}
