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
//
// adaptSubmitParams 会把 fal 系的字段名映射成上面这套（generate_audio→audio、
// {image,video,audio}_urls→reference_{image,video,audio}_urls），并把
// duration/aspect_ratio 的 "auto" 换成具体值。直接按 apiz 原生名传参也支持。
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

// adaptSubmitParams 把调用方（fal 兼容协议）传入的 params 转换为 apiz 上游
// 期望的参数命名与取值。当前处理的差异：
//   - generate_audio -> audio（bool 值直接搬运）：apiz 侧字段名是 audio；
//     若调用方已显式传 audio，尊重 audio 并丢弃 generate_audio；
//   - resolution 大小写：apiz 只接受大写 P（480P / 720P），
//     而客户端 / 内部通常用小写 p（480p / 720p / 1080p），
//     这里把末尾的小写 p 统一改成大写 P。非 480/720 也一并处理，
//     保持向前兼容（apiz 若新增分辨率将来只需在其侧新增校验）。
//   - duration=auto：apiz 不接受 "auto"，收到会 422；这里替换为 apiz 兜底
//     秒数（8s），与业务侧“auto 预扣兜底时长”的口径一致。其他字符串数字
//     （"5" / "10"）与数字类型直接透传。
//   - aspect_ratio=auto：apiz 不接受 "auto"，这里兜底替换为 "16:9"（视频
//     业务最常用的横屏比例）。其他显式比例（"1:1" / "9:16" 等）直接透传。
//
// 处理原则：拷贝一层新 map，避免污染 handler 里保存到 DB 的原始 payload。
// body 不是 map[string]any（例如 nil / 结构体）时原样返回。
func adaptSubmitParams(body any) any {
	src, ok := body.(map[string]any)
	if !ok {
		return body
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	// generate_audio → audio：fal 系用 generate_audio，apiz 用 audio。
	renameKeyIfAbsent(out, "generate_audio", "audio")
	if v, exists := out["resolution"]; exists {
		if s, isStr := v.(string); isStr {
			out["resolution"] = normalizeApizResolution(s)
		}
	}
	if v, exists := out["duration"]; exists {
		if s, isStr := v.(string); isStr && strings.EqualFold(strings.TrimSpace(s), "auto") {
			out["duration"] = apizAutoDurationFallback
		}
	}
	if v, exists := out["aspect_ratio"]; exists {
		if s, isStr := v.(string); isStr && strings.EqualFold(strings.TrimSpace(s), "auto") {
			out["aspect_ratio"] = apizAutoAspectRatioFallback
		}
	}
	// 参考素材数组的字段名转换：其他 fal 兼容上游用 {image,video,audio}_urls，
	// apiz 侧要求 reference_{image,video,audio}_urls（见本文件头部的参数说明）。
	// 不在此处校验数量上限（<=30/10/10），保持"只做名字映射、由上游判定业务规则"
	// 的既有原则，避免网关与上游规则各写一份而漂移。
	for _, m := range apizReferenceURLFieldRenames {
		renameKeyIfAbsent(out, m.from, m.to)
	}
	return out
}

// apizReferenceURLFieldRenames 是参考素材数组字段的重命名表。
// 用表驱动而不是三段重复的 if：将来 apiz 再加 reference_xxx_urls 只需加一行。
var apizReferenceURLFieldRenames = []struct{ from, to string }{
	{from: "image_urls", to: "reference_image_urls"},
	{from: "video_urls", to: "reference_video_urls"},
	{from: "audio_urls", to: "reference_audio_urls"},
}

// renameKeyIfAbsent 把 m[from] 搬到 m[to]，并删除 from。
//
// 语义与既有的 generate_audio → audio 转换保持一致：
//   - from 不存在：什么都不做（不会凭空造出一个 to 键）
//   - to 已被调用方显式指定：尊重调用方的 to，仅丢弃 from，避免同键冲突
//     （调用方直接按 apiz 原生字段名传参时就是这种情况）
func renameKeyIfAbsent(m map[string]any, from, to string) {
	v, exists := m[from]
	if !exists {
		return
	}
	if _, hasTarget := m[to]; !hasTarget {
		m[to] = v
	}
	delete(m, from)
}

// apizAutoDurationFallback 是当客户端传 duration="auto" 时，转发给 apiz 上游
// 的兜底秒数。apiz 不接受 "auto"，收到会返回 422。选 8s 与后端异步视频服务
// 里 defaultAutoDurationSeconds 的口径保持一致，方便预扣费与实际计费对账。
const apizAutoDurationFallback = 8

// apizAutoAspectRatioFallback 是当客户端传 aspect_ratio="auto" 时，转发给
// apiz 上游的兜底比例。apiz 不接受 "auto"，选 16:9（视频业务最常用的横屏
// 比例）作为默认值，避免 422。
const apizAutoAspectRatioFallback = "16:9"

// normalizeApizResolution 把 "480p"/"720p" 之类的分辨率字符串规范化为 apiz
// 上游要求的大写形式（"480P"/"720P"）。规则：去两端空白，若末尾字符是 'p'
// 就替换为 'P'；其它形式（如已经是大写 P、或非 p 结尾的自定义分辨率）原样返回。
func normalizeApizResolution(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return s
	}
	if last := trimmed[len(trimmed)-1]; last == 'p' {
		return trimmed[:len(trimmed)-1] + "P"
	}
	return trimmed
}

// SubmitRaw 向 tasks/create 端点提交异步视频任务。
//
// body 为客户端原始 payload（prompt / duration / resolution / aspect_ratio /
// audio / image_url 等）。apiz 上游要求请求体结构与其他 fal-兼容上游不同：
// 上游模型名放在顶层 model 字段，其余业务参数放在顶层 params 对象里，
// 即：{ "model": "<upstream_model>", "params": { ...原始 payload... } }。
// 因此这里把调用方透传下来的 body 包一层，再发给上游。
//
// 参数命名差异（apiz vs 其他 fal 兼容上游），统一由 adaptSubmitParams 处理：
//   - generate_audio (bool) → audio (bool)
//     其他 fal 兼容上游用 generate_audio 控制是否生成音轨；apiz 侧字段名是 audio。
//   - image_urls → reference_image_urls（<=30）
//   - video_urls → reference_video_urls（<=10）
//   - audio_urls → reference_audio_urls（<=10）
//     参考素材数组：其他上游用 {kind}_urls，apiz 要求 reference_{kind}_urls。
//     数量上限交给上游校验，网关只做名字映射，避免两边规则各写一份而漂移。
//
// 以上转换的共同语义：源字段不存在则不动；若调用方同时显式传了目标字段，
// 尊重目标字段、仅丢弃别名，避免同键冲突（直接按 apiz 原生名传参即属此例）。
//
// 另外 duration / aspect_ratio 传 "auto" 时会被替换为具体值（apiz 不接受 "auto"，
// 收到会 422），resolution 的 "720p" 会规范化为 "720P"。
//
// 上游返回 { task_id, status, ... }，映射为 fal.SubmitResponse：
//   - RequestID = task_id
//   - Status    = IN_QUEUE
//   - StatusURL / ResponseURL = {base}/api/v3/tasks/query?task_id={task_id}
func (c *Client) SubmitRaw(ctx context.Context, model string, body any) (*fal.SubmitResponse, error) {
	envelope := map[string]any{
		"model":  model,
		"params": adaptSubmitParams(body),
	}
	resp, err := c.doTask(ctx, c.baseURL+pathTasksCreate, envelope)
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
// 为了让下游客户端拿到一致的"通用视频回包"结构（`{"video": {"url": "...", "file_name": "..."}}`，
// 与 fal 平台一致），apiz 的返回需要做**收敛**：
//   - 剥掉 apiz 私有字段（params/output/result/data/task_id/updated_at/created_at/
//     completed_at/channel/model/price 等），避免把无用甚至暴露内部字段的信息透传到客户端；
//   - 抽取到的 video URL 统一映射成 `video` / `videos` 两个字段，
//     供 service.ExtractVideoURLs 识别。
//
// 另外，apiz 顶层的 `price` 是**本次任务的真实上游成本（分）**，除以 100 即为美元。
// 通过内部约定字段 _apiz_upstream_cost_usd 附加到返回 map（下划线前缀表示内部），
// 由 async_video_executor 读取并从 payload 中删除，再写入 task.UpstreamCost。
// 客户端最终看到的仍是干净的 `{video, videos}`。
func (c *Client) ResultRaw(ctx context.Context, responseURL string) (map[string]any, error) {
	resp, err := c.queryTask(ctx, responseURL)
	if err != nil {
		return nil, err
	}

	// 构造干净的通用回包，而不是继承 apiz 私有字段。
	out := make(map[string]any, 4)

	// 提取上游侧关键元信息，用来一起塞进 video 对象；这样：
	//   - executor 的 fal.ExtractVideoURLs 能在 out["video"].url 抽到 url；
	//   - executor 的 ExtractActualDurationSeconds 能在 out["video"].duration 抽到时长
	//     （否则 duration="auto" 场景无法按上游实际时长重算 finalCost）。
	duration, contentType, width, height := collectVideoMeta(resp)

	urls := collectVideoURLs(resp)
	if len(urls) > 0 {
		videos := make([]any, 0, len(urls))
		for _, u := range urls {
			entry := map[string]any{
				"url":       u,
				"file_name": videoFileNameFromURL(u),
			}
			if duration > 0 {
				entry["duration"] = duration
			}
			if contentType != "" {
				entry["content_type"] = contentType
			}
			if width > 0 {
				entry["width"] = width
			}
			if height > 0 {
				entry["height"] = height
			}
			videos = append(videos, entry)
		}
		// 第一个作为主 video 对象，全部放入 videos 数组。
		if first, ok := videos[0].(map[string]any); ok {
			out["video"] = first
		}
		out["videos"] = videos
	}

	// 上游真实成本（USD）= apiz price / 100。用内部约定字段传给 executor。
	// 只在能解析到正数时附带；解析失败或为 0 时不设，避免污染 payload。
	if cost := parseApizPriceUSD(resp); cost > 0 {
		out[UpstreamCostFieldKey] = cost
	}

	return out, nil
}

// UpstreamCostFieldKey 是 apiz.ResultRaw 附加在返回 map 上的**内部约定字段名**，
// 用来把上游真实成本（USD）传给 executor。executor 读到后应从 map 中删除，
// 避免透传给客户端。使用下划线前缀表示"内部字段"，与 fal 平台的公开字段区分。
const UpstreamCostFieldKey = "_apiz_upstream_cost_usd"

// parseApizPriceUSD 从 apiz taskResponse 顶层或 data 中读取 price 字段（单位：分），
// 返回美元金额。字段缺失或格式异常时返回 0。
//
// 兼容 JSON 中 price 可能被解析为 float64（默认）或 json.Number 的两种情况。
func parseApizPriceUSD(resp *taskResponse) float64 {
	if resp == nil {
		return 0
	}
	// 顶层 price 优先，其次 data.price。
	for _, container := range []map[string]any{resp.Raw, resp.Data} {
		if container == nil {
			continue
		}
		if v, ok := container["price"]; ok {
			if cents := toFloat64(v); cents > 0 {
				return cents / 100.0
			}
		}
	}
	return 0
}

// toFloat64 尝试把任意 JSON 数值（float64 / int / int64 / json.Number / string）
// 转成 float64。无法识别或负数返回 0。
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0
		}
		return n
	case float32:
		if n < 0 {
			return 0
		}
		return float64(n)
	case int:
		if n < 0 {
			return 0
		}
		return float64(n)
	case int64:
		if n < 0 {
			return 0
		}
		return float64(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil || f < 0 {
			return 0
		}
		return f
	case string:
		// 少数上游会把 price 当字符串返回；宽容处理。
		s := strings.TrimSpace(n)
		if s == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
			return f
		}
		return 0
	default:
		return 0
	}
}

// collectVideoMeta 从 apiz 响应中提取视频元信息（duration/content_type/width/height）。
//
// apiz 的实际结构是把业务字段包在 data 下面，元信息通常出现在 data.output / data.result 里，
// 典型回包片段：
//
//	{ "code":200,
//	  "data":{ "output":{duration:8, resolution:"720P", width:1248, height:704, content_type:"video/mp4"},
//	          "result":{...同 output...}, "status":"completed", "task_id":"...", "price":800 } }
//
// 但也兼容顶层扁平（老回包）与 data 直接放 duration 之类的形态。候选容器优先级：
//
//	data.output → data.result → resp.Data → resp.Raw["output"] → resp.Raw["result"] → resp.Raw
//
// 未识别到时返回零值；上层按需忽略缺失字段。
func collectVideoMeta(resp *taskResponse) (duration int, contentType string, width int, height int) {
	if resp == nil {
		return 0, "", 0, 0
	}
	// 从多个候选容器里按优先级尝试；找到第一个"看起来是视频元信息"的对象即可。
	candidates := []map[string]any{
		mapField(resp.Data, "output"),
		mapField(resp.Data, "result"),
		resp.Data,
		mapField(resp.Raw, "output"),
		mapField(resp.Raw, "result"),
		resp.Raw,
	}
	for _, c := range candidates {
		if c == nil {
			continue
		}
		if duration == 0 {
			duration = firstIntFieldFlex(c, "duration", "duration_seconds", "num_seconds")
		}
		if contentType == "" {
			contentType = firstStringField(c, "content_type", "mime_type")
		}
		if width == 0 {
			width = firstIntFieldFlex(c, "width")
		}
		if height == 0 {
			height = firstIntFieldFlex(c, "height")
		}
	}
	return duration, contentType, width, height
}

// mapField 从 map 中安全取一个 map[string]any 子字段。
func mapField(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return nil
}

// firstIntFieldFlex 与 firstStringField 类似但用于整型字段，兼容 float64 / int / json.Number / string。
func firstIntFieldFlex(m map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch typed := v.(type) {
		case float64:
			if typed > 0 {
				return int(typed)
			}
		case int:
			if typed > 0 {
				return typed
			}
		case int64:
			if typed > 0 {
				return int(typed)
			}
		case json.Number:
			if n, err := typed.Int64(); err == nil && n > 0 {
				return int(n)
			}
		case string:
			s := strings.TrimSpace(typed)
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// videoFileNameFromURL 从 URL 中截取最后一段路径作为 file_name（如
// "https://cdn/xxx/2026/08/10/abc.mp4" -> "abc.mp4"）。无法解析时返回空串。
func videoFileNameFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	p := u.Path
	if p == "" {
		return ""
	}
	// 去掉尾部斜杠，取最后一段。
	p = strings.TrimRight(p, "/")
	if idx := strings.LastIndex(p, "/"); idx >= 0 && idx < len(p)-1 {
		return p[idx+1:]
	}
	return p
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
	return c.doTaskWithLogEvents(ctx, endpoint, map[string]any{queryParamTaskID: taskID}, "apiz_status_poll_request", "apiz_status_poll_response", true)
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
	return c.doTaskWithLogEvents(ctx, endpoint, reqBody, "apiz_http_request", "apiz_http_response", false)
}

func (c *Client) doTaskWithLogEvents(ctx context.Context, endpoint string, reqBody any, requestEvent, responseEvent string, info bool) (*taskResponse, error) {
	raw, err := c.doJSON(ctx, http.MethodPost, endpoint, reqBody, requestEvent, responseEvent, info)
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
func (c *Client) doJSON(ctx context.Context, method, endpoint string, reqBody any, requestEvent, responseEvent string, info bool) ([]byte, error) {
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
	if info || slog.Default().Enabled(ctx, slog.LevelDebug) {
		// 完整 request body 分块打印，便于线上排障（如 apiz 422 时看真实提交内容）。
		// 分块长度参照 async_video_executor 里对 upstream error dump 的做法，
		// 保证单条 log 记录不会撑爆日志系统。
		logBodyChunks(ctx, requestEvent, requestID, method, endpoint, 0, rawBody, info)
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

	if info || slog.Default().Enabled(ctx, slog.LevelDebug) {
		// 完整 response body 分块打印。status 一并附带便于筛选 4xx/5xx。
		logBodyChunks(ctx, responseEvent, requestID, method, endpoint, resp.StatusCode, raw, info)
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

// collectVideoURLs 从 apiz 响应中抽取视频地址。
//
// apiz 实际结构把业务字段包在 data 下：video_url 通常在 data.output.video_url
// 或 data.result.video_url，而非顶层。为保持鲁棒性，这里按以下顺序合并、去重：
//
//  1. resp.VideoURL（顶层 video_url，兼容老回包/扁平回包）
//  2. data.video_url / data.url
//  3. data.output.video_url / data.output.url
//  4. data.result.video_url / data.result.url
//  5. Raw / Data / Raw.output / Raw.result / Data.output / Data.result 里的
//     outputs / videos / video_urls / urls 数组
//
// 抽取失败会导致上层 executor 判定 payload 为空、进入 markFailedAndRefund 分支；
// 因此必须覆盖 apiz 的真实结构。
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

	// 单值字段：顶层 → data → data.output → data.result。
	appendURL(resp.VideoURL)
	appendURL(firstStringField(resp.Data, "video_url", "url"))
	appendURL(firstStringField(mapField(resp.Data, "output"), "video_url", "url"))
	appendURL(firstStringField(mapField(resp.Data, "result"), "video_url", "url"))

	// 数组字段：在多个候选容器中都试一遍。apiz 目前的实际结构不用数组，
	// 但保留对 outputs/videos/video_urls/urls 的兼容，避免上游未来一改就抽不到。
	arrayKeys := []string{"outputs", "videos", "video_urls", "urls"}
	containers := []map[string]any{
		resp.Data,
		mapField(resp.Data, "output"),
		mapField(resp.Data, "result"),
		resp.Raw,
		mapField(resp.Raw, "output"),
		mapField(resp.Raw, "result"),
	}
	for _, container := range containers {
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

// apizLogBodyChunkSize 单条 log 里 body_chunk 字段的最大字符数。
// 大 body（尤其是 apiz 422 时的 detail 数组）拆多条打，避免被日志系统截断。
const apizLogBodyChunkSize = 4000

// logBodyChunks 以 Debug 级别把 HTTP body 拆多条 slog 记录打印。
//
//   - event      : "apiz_http_request" / "apiz_http_response"
//   - requestID  : 请求跟踪 id，与 apiz_http_request/response 现有字段一致
//   - method/endpoint : 便于跨条 log 聚合
//   - status     : 响应侧填 HTTP 状态码；请求侧传 0，函数内自动省略该字段
//   - body       : 完整字节数组；为空时输出一条 body_bytes=0 的空 body 记录
//
// 每条 log 附带 chunk_index / chunk_total 便于串起来重组原始 body。
func logBodyChunks(ctx context.Context, event, requestID, method, endpoint string, status int, body []byte, info bool) {
	total := len(body)
	// baseAttrs 组装每条 log 的公共字段。
	// 使用 []any 而非 struct 是为了走 slog 变参 API，避免额外的分配。
	baseAttrs := func(chunkIdx, chunkTotal int, chunk string) []any {
		attrs := []any{
			"request_id", requestID,
			"method", method,
			"endpoint", endpoint,
		}
		if status > 0 {
			attrs = append(attrs, "status", status)
		}
		attrs = append(attrs,
			"body_bytes", total,
			"chunk_index", chunkIdx,
			"chunk_total", chunkTotal,
			"body_chunk", chunk,
		)
		return attrs
	}
	if total == 0 {
		logApizBodyChunk(ctx, info, event, baseAttrs(0, 1, "")...)
		return
	}
	// 按字节切分即可；apiz 提交与返回体均为 JSON（ASCII/UTF-8 混合），
	// 单条 4000 字符切出的边界即便断在多字节字符中间也仅影响日志可读性，
	// 不影响原始 body 语义（真正入库/入错误路径的 raw body 走 apiErr.Body）。
	chunks := (total + apizLogBodyChunkSize - 1) / apizLogBodyChunkSize
	for i := 0; i < chunks; i++ {
		start := i * apizLogBodyChunkSize
		end := start + apizLogBodyChunkSize
		if end > total {
			end = total
		}
		logApizBodyChunk(ctx, info, event, baseAttrs(i, chunks, string(body[start:end]))...)
	}
}

func logApizBodyChunk(ctx context.Context, info bool, event string, attrs ...any) {
	if info {
		slog.InfoContext(ctx, event, attrs...)
		return
	}
	slog.DebugContext(ctx, event, attrs...)
}
