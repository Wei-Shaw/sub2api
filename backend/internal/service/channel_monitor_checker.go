package service

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/tidwall/gjson"
)

// monitorHTTPClient 共享一个 http.Client，避免每次检测重建 transport。
// 自定义 Transport 在 dial 时强制再次校验 IP，防止 DNS rebinding 绕过 validateEndpoint。
//
// 【可调优】这些不再是包级常量：runner 每次 fire 前通过 ApplyMonitorTuning 注入
// 当前生效参数，monitorHTTPClient() 按参数指纹懒重建 client，因此管理员在后台
// 改完超时/保活参数后无需重启进程即可生效。
var (
	monitorClientMu          sync.Mutex
	monitorClientCache       *http.Client
	monitorPingCache         *http.Client
	monitorClientFingerprint string
)

// monitorTuningState 保存当前生效的探测调优参数（原子读写，供 fire 与 checker 并发访问）。
var monitorTuningState atomic.Value // ChannelMonitorTuning

// ApplyMonitorTuning 更新全局探测调优参数；参数变化会触发下次取 client 时重建 transport。
// 由 ChannelMonitorRunner 在每次 fire 前调用，传零值等价于恢复默认。
func ApplyMonitorTuning(t ChannelMonitorTuning) {
	if t.WorkerConcurrency <= 0 && t.ResponseHeaderTimeoutSeconds <= 0 && t.RequestTimeoutSeconds <= 0 {
		t = DefaultChannelMonitorTuning()
	}
	monitorTuningState.Store(NormalizeChannelMonitorTuning(t))
}

// currentMonitorTuning 返回当前生效参数（未注入过时为默认值）。
func currentMonitorTuning() ChannelMonitorTuning {
	if v, ok := monitorTuningState.Load().(ChannelMonitorTuning); ok {
		return v
	}
	return DefaultChannelMonitorTuning()
}

// monitorHTTPClient 返回当前参数下应使用的检测 client。
// 参数未变则复用缓存；变了则重建（旧 client 的 idle 连接会被 Go 在 GC 时关闭）。
func monitorHTTPClient() *http.Client {
	return monitorClientFor(false)
}

// monitorPingHTTPClient 用于 endpoint origin 的 HEAD ping，超时更短。
func monitorPingHTTPClient() *http.Client {
	return monitorClientFor(true)
}

func monitorClientFor(ping bool) *http.Client {
	t := currentMonitorTuning()
	fp := fmt.Sprintf("%d|%d|%d|%d", t.RequestTimeoutSeconds, t.ResponseHeaderTimeoutSeconds, t.IdleConnTimeoutSeconds, t.MaxIdleConnsPerHost)

	monitorClientMu.Lock()
	defer monitorClientMu.Unlock()

	if monitorClientCache == nil || monitorClientFingerprint != fp {
		monitorClientCache = newSSRFSafeHTTPClientTuned(
			time.Duration(t.RequestTimeoutSeconds)*time.Second,
			time.Duration(t.ResponseHeaderTimeoutSeconds)*time.Second,
			time.Duration(t.IdleConnTimeoutSeconds)*time.Second,
			t.MaxIdleConnsPerHost,
		)
		monitorPingCache = newSSRFSafeHTTPClientTuned(
			monitorPingTimeout,
			time.Duration(t.ResponseHeaderTimeoutSeconds)*time.Second,
			time.Duration(t.IdleConnTimeoutSeconds)*time.Second,
			t.MaxIdleConnsPerHost,
		)
		monitorClientFingerprint = fp
		slog.Info("channel_monitor: http client rebuilt for new tuning",
			"request_timeout_s", t.RequestTimeoutSeconds,
			"response_header_timeout_s", t.ResponseHeaderTimeoutSeconds,
			"idle_conn_timeout_s", t.IdleConnTimeoutSeconds,
			"max_idle_conns_per_host", t.MaxIdleConnsPerHost)
	}
	if ping {
		return monitorPingCache
	}
	return monitorClientCache
}

// newSSRFSafeHTTPClient 保留原签名，供既有调用方/测试继续使用。
// newSSRFSafeHTTPClientTuned 返回一个使用 safeDialContext 的 http.Client。
// 仅供监控模块对外发起请求使用——所有目标都应是公网 endpoint。
//
// 关键：MaxIdleConnsPerHost 必须显式设置。Go 默认只有 2，而channel monitor 的
// 全部渠道通常指向同一个 host，默认值会让绝大多数探测都走新建连接（TCP+TLS+
// HTTP/2 握手 + 上游预热），把冷启动延迟误记成渠道故障。
func newSSRFSafeHTTPClientTuned(timeout, responseHeaderTimeout, idleConnTimeout time.Duration, maxIdlePerHost int) *http.Client {
	if maxIdlePerHost < 1 {
		maxIdlePerHost = ChannelMonitorMaxIdleConnsPerHostDefault
	}
	tr := &http.Transport{
		DialContext:           safeDialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxIdlePerHost * 4,
		MaxIdleConnsPerHost:   maxIdlePerHost,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   monitorTLSHandshakeTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
	return &http.Client{Timeout: timeout, Transport: servertiming.WrapRoundTripper(tr)}
}

// CheckOptions 承载一次检测的自定义入参。
// 所有字段都是可选（零值即等价于"用默认行为"）。
type CheckOptions struct {
	// APIMode 仅对 OpenAI provider 生效；空串等同 chat_completions。
	APIMode string
	// ExtraHeaders 用户自定义 HTTP 头（merge 到 adapter 默认 headers，用户优先）。
	ExtraHeaders map[string]string
	// BodyOverrideMode: off | merge | replace
	BodyOverrideMode string
	// BodyOverride 在 merge 模式下做浅合并（key 命中黑名单时静默丢弃），
	// 在 replace 模式下直接当作完整 body。
	BodyOverride map[string]any
	// UpstreamRequestIDHeader 是直连上游时要读取的响应头名（来自归属账号的
	// extra.upstream_request_id_header）。为空表示这次不抓请求 ID。
	UpstreamRequestIDHeader string
}

// runCheckForModel 对单个 (provider, model) 做一次完整检测。
// 不返回 error：所有失败都包装进 CheckResult.Status=error/failed。
//
// opts 承载模板 / 监控快照带来的自定义配置。nil 等同于 "off + 无 extra headers"。
func runCheckForModel(ctx context.Context, provider, endpoint, apiKey, model string, opts *CheckOptions) *CheckResult {
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusError,
		CheckedAt: time.Now(),
	}

	challenge := generateChallenge()
	mode := bodyOverrideMode(opts)

	start := time.Now()
	call, err := callProvider(ctx, provider, endpoint, apiKey, model, challenge.Prompt, opts)
	latency := time.Since(start)
	latencyMs := int(latency / time.Millisecond)
	res.LatencyMs = &latencyMs
	// 记账所需的原始信息：仅直连上游的记账会读，不参与这里的状态判定。
	res.StatusCode = call.Status
	res.Stream = call.Stream
	res.FirstTokenMs = call.FirstTokenMs
	res.UpstreamRequestID = monitorUpstreamRequestID(opts, call.Headers)
	res.Usage = call.Usage
	// 真实打到的上游协议路径（/v1/chat/completions 等）：探针按模板直连原生端点，
	// 这里记录下来供使用记录页的「上游」列展示。注意取的是 call.Endpoint（adapter
	// 算出来的路径），而不是本函数的 endpoint 参数（那是 base URL）。
	res.UpstreamEndpoint = strings.TrimSpace(call.Endpoint)
	respText := call.Text

	if err != nil {
		res.Status = MonitorStatusError
		res.Message = truncateMessage(sanitizeErrorMessage(err.Error()))
		return res
	}
	if call.Status < 200 || call.Status >= 300 {
		// 错误路径：用 rawBody 而非 respText（gjson textPath 抽取在错误响应里通常为空，
		// 会丢掉真正的上游错误信息，例如 `{"error":{"message":"No available accounts ..."}}`）。
		res.Status = MonitorStatusError
		bodySnippet := truncateForErrorBody(call.RawBody)
		res.Message = truncateMessage(sanitizeErrorMessage(fmt.Sprintf("upstream HTTP %d: %s", call.Status, bodySnippet)))
		return res
	}

	// Replace 模式：跳过 challenge 校验（用户 body 是静态的，challenge 没法嵌入）。
	// 改用「HTTP 2xx + 响应文本（adapter.textPath 抽取）非空」作为 operational 判定。
	// 响应文本为空则降级为 failed（视为上游回了 200 但没实际内容）。
	if mode == MonitorBodyOverrideModeReplace {
		if strings.TrimSpace(respText) == "" {
			res.Status = MonitorStatusFailed
			res.Message = truncateMessage("replace-mode: upstream returned 2xx with empty text")
			return res
		}
		return finalizeOperationalOrDegraded(res, latency, latencyMs)
	}

	if !validateChallenge(respText, challenge.Expected) {
		res.Status = MonitorStatusFailed
		res.Message = truncateMessage(sanitizeErrorMessage(fmt.Sprintf("challenge mismatch (expected %s, got %q)", challenge.Expected, respText)))
		return res
	}

	return finalizeOperationalOrDegraded(res, latency, latencyMs)
}

// finalizeOperationalOrDegraded 负责走到最后一步的 operational/degraded 判定。
// 拆出来是为了让 runCheckForModel 不超过 30 行。
func finalizeOperationalOrDegraded(res *CheckResult, latency time.Duration, latencyMs int) *CheckResult {
	if latency >= monitorDegradedThreshold {
		res.Status = MonitorStatusDegraded
		res.Message = truncateMessage(fmt.Sprintf("slow response: %dms", latencyMs))
		return res
	}
	res.Status = MonitorStatusOperational
	return res
}

// bodyOverrideMode 归一取 opts.BodyOverrideMode，nil opts / 空串都视为 off。
func bodyOverrideMode(opts *CheckOptions) string {
	if opts == nil || opts.BodyOverrideMode == "" {
		return MonitorBodyOverrideModeOff
	}
	return opts.BodyOverrideMode
}

// pingEndpointOrigin 对 endpoint 的 origin (scheme://host) 发起 HEAD 请求，返回耗时。
// 失败时返回 nil（不影响主状态判定）。
func pingEndpointOrigin(ctx context.Context, endpoint string) *int {
	origin, err := extractOrigin(endpoint)
	if err != nil || origin == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, origin, nil)
	if err != nil {
		return nil
	}
	start := time.Now()
	resp, err := monitorPingHTTPClient().Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, monitorPingDiscardMaxBytes))
	ms := int(time.Since(start) / time.Millisecond)
	return &ms
}

// providerAdapter 描述某个 provider 在 challenge 检测中需要的几件事：
//   - 拼出请求路径（含 model 占位）
//   - 序列化请求体
//   - 构造鉴权头
//   - 从响应 JSON 中提取文本（默认按 gjson path；需要时可自定义）
//
// 加新 provider 只需要在 providerAdapters 里增加一个条目，无需触碰 callProvider / validateProvider。
type providerAdapter struct {
	buildPath    func(model string) string
	buildBody    func(model, prompt string) ([]byte, error)
	buildHeaders func(apiKey string) map[string]string
	textPath     string // gjson 提取响应文本的 path
	extractText  func([]byte) string
}

// providerAdapters 全部已支持的 provider。键值即 MonitorProvider* 字符串。
//
//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerAdapters = map[string]providerAdapter{
	MonitorProviderOpenAI: providerOpenAIChatAdapter,
	MonitorProviderGrok:   providerGrokChatAdapter,
	// 国产 3 家（配额模式引入）：均为 OpenAI 兼容 Chat Completions，
	// 仅智谱路径前缀不同（/api/paas/v4/chat/completions）。
	MonitorProviderKimi:     providerKimiChatAdapter,
	MonitorProviderZhipu:    providerZhipuChatAdapter,
	MonitorProviderDeepseek: providerDeepseekChatAdapter,
	MonitorProviderMiniMax:  providerMiniMaxChatAdapter,
	MonitorProviderAnthropic: {
		buildPath: func(string) string { return providerAnthropicPath },
		buildBody: func(model, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"model":      model,
				"messages":   []map[string]string{{"role": "user", "content": prompt}},
				"max_tokens": monitorChallengeMaxTokens,
			})
		},
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{
				"x-api-key":         apiKey,
				"anthropic-version": monitorAnthropicAPIVersion,
			}
		},
		extractText: extractAnthropicMonitorText,
	},
	MonitorProviderGemini: {
		// Gemini 把 model 名写在 URL path 上：/v1beta/models/{model}:generateContent
		buildPath: func(model string) string { return fmt.Sprintf(providerGeminiPathTemplate, model) },
		buildBody: func(_, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"contents": []map[string]any{
					{"role": "user", "parts": []map[string]any{{"text": prompt}}},
				},
				"generationConfig": map[string]any{"maxOutputTokens": monitorChallengeMaxTokens},
			})
		},
		// 使用 x-goog-api-key header 而不是 ?key= query，避免 *url.Error 把 key 回填到错误日志。
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{"x-goog-api-key": apiKey}
		},
		textPath: "candidates.0.content.parts.0.text",
	},
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerOpenAIChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerGrokChatAdapter = newOpenAICompatibleChatAdapter(providerGrokPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerKimiChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerZhipuChatAdapter = newOpenAICompatibleChatAdapter(providerZhipuPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerDeepseekChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerMiniMaxChatAdapter = newOpenAICompatibleChatAdapter(providerOpenAIPath)

func newOpenAICompatibleChatAdapter(path string) providerAdapter {
	return providerAdapter{
		buildPath: func(string) string { return path },
		buildBody: func(model, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"model":      model,
				"messages":   []map[string]string{{"role": "user", "content": prompt}},
				"max_tokens": monitorChallengeMaxTokens,
				"stream":     false,
			})
		},
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{"Authorization": "Bearer " + apiKey}
		},
		textPath: "choices.0.message.content",
	}
}

//nolint:gochecknoglobals // 适配器表是只读静态数据，初始化后不变更。
var providerOpenAIResponsesAdapter = providerAdapter{
	buildPath: func(string) string { return providerOpenAIResponsesPath },
	buildBody: func(model, prompt string) ([]byte, error) {
		return json.Marshal(map[string]any{
			"model":             model,
			"instructions":      "You are a channel health-check endpoint. Answer the arithmetic challenge exactly and briefly.",
			"input":             prompt,
			"max_output_tokens": monitorChallengeMaxTokens,
			"stream":            false,
		})
	},
	buildHeaders: func(apiKey string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + apiKey}
	},
	textPath: "output.0.content.0.text",
}

// providerAdapterFor 按 provider + api_mode 选择具体 adapter。
func providerAdapterFor(provider, apiMode string) (providerAdapter, string, bool) {
	if provider == MonitorProviderOpenAI && defaultAPIMode(apiMode) == MonitorAPIModeResponses {
		return providerOpenAIResponsesAdapter, MonitorAPIModeResponses, true
	}
	adapter, ok := providerAdapters[provider]
	return adapter, MonitorAPIModeChatCompletions, ok
}

// monitorCallResult 是一次上游调用的原始结果。
// 相比旧的四个返回值多了响应头与首字时间——「直连上游探针记账」要用。
type monitorCallResult struct {
	// Text 按适配器抽取出的正文，仅在 2xx 时有意义。
	Text string
	// RawBody 完整响应体（已按 monitorResponseMaxBytes 截断），错误路径用来保留上游原话。
	RawBody string
	// Status HTTP 状态码；0 表示没拿到响应。
	Status int
	// Headers 上游响应头，用于抓上游请求 ID。
	Headers http.Header
	// Stream 本次是否以流式发起。
	Stream bool
	// Endpoint 本次实际请求的**协议路径**（/v1/chat/completions 等），由 adapter 决定。
	// 供记账行填 upstream_endpoint，让使用记录页的「上游」列显示真实协议。
	Endpoint string
	// FirstTokenMs 流式时的首个内容块到达耗时。
	FirstTokenMs *int
	// Usage 从上游响应里解析出的 token 用量（记账用）。
	Usage ProbeUsageTokens
}

// callProvider 通过 providerAdapters 分发到具体实现。
// opts 承载用户的自定义 headers / body 覆盖（可为 nil）。
func callProvider(ctx context.Context, provider, endpoint, apiKey, model, prompt string, opts *CheckOptions) (monitorCallResult, error) {
	requestedAPIMode := checkAPIMode(opts)
	if err := validateAPIMode(provider, requestedAPIMode); err != nil {
		return monitorCallResult{}, err
	}
	adapter, apiMode, ok := providerAdapterFor(provider, requestedAPIMode)
	if !ok {
		return monitorCallResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	body, err := buildRequestBody(adapter, provider, apiMode, model, prompt, opts)
	if err != nil {
		return monitorCallResult{}, err
	}
	headers := mergeHeaders(adapter.buildHeaders(apiKey), opts)
	// 记下真正要打的协议路径（/v1/chat/completions 等）：使用记录页的「上游」列
	// 展示的是**路径**，与网关那侧 upstream_endpoint 的口径一致；填 base URL 会让
	// 那一列显示成域名，管理员看不出这次探针验的是哪条协议。
	//
	// 合并上游后，路径计算改用上游新增的 monitorRequestPath（它替我们处理了
	// provider / endpoint / adapter 的差异），但「把路径写进 Endpoint」这一点保留。
	path := monitorRequestPath(provider, endpoint, adapter, model)
	full := joinURL(endpoint, path)

	// 只有请求体自己声明了 stream=true 才走流式。探针默认 body 恒为 stream=false，
	// 因此除非管理员在「覆盖」模式的 Body 里显式打开，行为与改动前逐字一致。
	if monitorRequestBodyStreams(body) {
		if kind, ok := monitorStreamKind(provider, apiMode); ok {
			result, err := postJSONStream(ctx, full, body, headers, kind)
			result.Endpoint = path
			return result, err
		}
	}

	respBytes, status, respHeaders, err := postRawJSON(ctx, full, body, headers)
	if err != nil {
		return monitorCallResult{Status: status, Headers: respHeaders, Endpoint: path}, err
	}
	result := monitorCallResult{Status: status, Headers: respHeaders, RawBody: string(respBytes), Endpoint: path}
	if provider == MonitorProviderOpenAI && apiMode == MonitorAPIModeResponses {
		result.Text = extractOpenAIResponsesText(respBytes)
	} else {
		result.Text = extractMonitorResponseText(adapter, respBytes)
	}
	// token 用量：非流式响应体的顶层 usage / usageMetadata。
	captureProbeUsage(&result.Usage, parseProbeJSONObject(string(respBytes)))
	return result, nil
}

func extractMonitorResponseText(adapter providerAdapter, respBytes []byte) string {
	if adapter.extractText != nil {
		return adapter.extractText(respBytes)
	}
	return gjson.GetBytes(respBytes, adapter.textPath).String()
}

func extractAnthropicMonitorText(respBytes []byte) string {
	content := gjson.GetBytes(respBytes, "content")
	if !content.IsArray() {
		return ""
	}

	parts := make([]string, 0, 1)
	content.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() != "text" {
			return true
		}
		text := strings.TrimSpace(item.Get("text").String())
		if text != "" {
			parts = append(parts, text)
		}
		return true
	})
	return strings.Join(parts, "\n")
}

// extractOpenAIResponsesText 聚合 Responses API 的最终 assistant 文本。
// Responses 的 output 数组顺序由模型决定：reasoning / tool-call item 可能排在 message 前面，
// 因此不能假设文本永远在 output.0.content.0.text。
func extractOpenAIResponsesText(respBytes []byte) string {
	if text := gjson.GetBytes(respBytes, "output_text").String(); strings.TrimSpace(text) != "" {
		return text
	}

	var texts []string
	outputs := gjson.GetBytes(respBytes, "output")
	if outputs.IsArray() {
		outputs.ForEach(func(_, output gjson.Result) bool {
			outputType := output.Get("type").String()
			if outputType != "" && outputType != "message" {
				return true
			}

			content := output.Get("content")
			if !content.IsArray() {
				return true
			}

			content.ForEach(func(_, block gjson.Result) bool {
				blockType := block.Get("type").String()
				if blockType != "" && blockType != "output_text" {
					return true
				}
				if text := block.Get("text").String(); strings.TrimSpace(text) != "" {
					texts = append(texts, text)
				}
				return true
			})
			return true
		})
	}

	if len(texts) > 0 {
		return strings.Join(texts, "")
	}
	return gjson.GetBytes(respBytes, providerOpenAIResponsesAdapter.textPath).String()
}

// mergeHeaders 把用户自定义 headers 合并到 adapter 默认 headers 上。
// 用户值覆盖默认；命中黑名单（hop-by-hop / 由 http.Client 自管的）的 key 静默丢弃。
func mergeHeaders(base map[string]string, opts *CheckOptions) map[string]string {
	if opts == nil || len(opts.ExtraHeaders) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(opts.ExtraHeaders))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range opts.ExtraHeaders {
		if IsForbiddenHeaderName(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// buildRequestBody 根据 body_override_mode 构造请求 body。
//
//   - off:     adapter 默认 body
//   - merge:   adapter 默认 body 与 BodyOverride 浅合并；BodyOverride 中命中
//     bodyMergeKeyDenyList[provider] 的 key 会被静默丢弃，避免破坏 challenge / model 路由
//   - replace: 直接 marshal BodyOverride 作为完整 body
//
// 任何 mode 返回的 []byte 都已经是合法 JSON，可直接送入 postRawJSON。
func buildRequestBody(adapter providerAdapter, provider, apiMode, model, prompt string, opts *CheckOptions) ([]byte, error) {
	mode := bodyOverrideMode(opts)

	if mode == MonitorBodyOverrideModeReplace {
		if opts == nil || len(opts.BodyOverride) == 0 {
			return nil, fmt.Errorf("replace mode: body_override is empty")
		}
		if err := validateReplaceRequestBody(provider, apiMode, opts.BodyOverride); err != nil {
			return nil, err
		}
		body, err := json.Marshal(opts.BodyOverride)
		if err != nil {
			return nil, fmt.Errorf("marshal body_override (replace): %w", err)
		}
		return body, nil
	}

	defaultBody, err := adapter.buildBody(model, prompt)
	if err != nil {
		return nil, fmt.Errorf("marshal default body: %w", err)
	}
	if mode != MonitorBodyOverrideModeMerge || opts == nil || len(opts.BodyOverride) == 0 {
		return defaultBody, nil
	}

	var defaultMap map[string]any
	if err := json.Unmarshal(defaultBody, &defaultMap); err != nil {
		return nil, fmt.Errorf("unmarshal default body for merge: %w", err)
	}
	deny := bodyMergeKeyDenyList[bodyMergeDenyKey(provider, apiMode)]
	for k, v := range opts.BodyOverride {
		if deny[k] {
			continue
		}
		defaultMap[k] = v
	}
	merged, err := json.Marshal(defaultMap)
	if err != nil {
		return nil, fmt.Errorf("marshal merged body: %w", err)
	}
	return merged, nil
}

// bodyMergeKeyDenyList 在 merge 模式下，禁止用户覆盖这些 provider-specific 的关键字段。
// 思路抄 check-cx 的 EXCLUDED_METADATA_KEYS：保护 challenge / model 路由不被用户误伤。
// 用户想动这些字段就用 replace 模式（已知会跳 challenge 校验）。
//
//nolint:gochecknoglobals // 静态查表，初始化后不变。
var bodyMergeKeyDenyList = map[string]map[string]bool{
	MonitorProviderOpenAI + ":" + MonitorAPIModeChatCompletions: {"model": true, "messages": true, "stream": true},
	MonitorProviderOpenAI + ":" + MonitorAPIModeResponses:       {"model": true, "instructions": true, "input": true, "stream": true},
	MonitorProviderGrok:      {"model": true, "messages": true, "stream": true},
	MonitorProviderAnthropic: {"model": true, "messages": true},
	MonitorProviderGemini:    {"contents": true},
	// 国产 3 家与 OpenAI Chat Completions 同构。
	MonitorProviderKimi:     {"model": true, "messages": true, "stream": true},
	MonitorProviderZhipu:    {"model": true, "messages": true, "stream": true},
	MonitorProviderDeepseek: {"model": true, "messages": true, "stream": true},
	MonitorProviderMiniMax:  {"model": true, "messages": true, "stream": true},
}

func checkAPIMode(opts *CheckOptions) string {
	if opts == nil {
		return MonitorAPIModeChatCompletions
	}
	return defaultAPIMode(opts.APIMode)
}

func bodyMergeDenyKey(provider, apiMode string) string {
	if provider == MonitorProviderOpenAI {
		return provider + ":" + defaultAPIMode(apiMode)
	}
	return provider
}

// isOpenAICompatibleChatProvider 该 provider 的探活请求是否为 OpenAI Chat
// Completions 同构（replace 模式的 body 校验按 messages 必填处理）。
func isOpenAICompatibleChatProvider(provider string) bool {
	switch provider {
	case MonitorProviderOpenAI, MonitorProviderGrok,
		MonitorProviderKimi, MonitorProviderZhipu, MonitorProviderDeepseek, MonitorProviderMiniMax:
		return true
	default:
		return false
	}
}

func validateReplaceRequestBody(provider, apiMode string, body map[string]any) error {
	if !isOpenAICompatibleChatProvider(provider) {
		return nil
	}
	switch defaultAPIMode(apiMode) {
	case MonitorAPIModeResponses:
		if strings.TrimSpace(stringFromAny(body["instructions"])) == "" || !hasNonEmptyBodyValue(body["input"]) {
			return fmt.Errorf("replace mode responses body: instructions and input are required")
		}
	case MonitorAPIModeChatCompletions:
		if !hasNonEmptyBodyValue(body["messages"]) {
			return fmt.Errorf("replace mode chat_completions body: messages are required")
		}
	}
	return nil
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return s
}

func hasNonEmptyBodyValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(val) != ""
	case []any:
		return len(val) > 0
	case []map[string]any:
		return len(val) > 0
	case []map[string]string:
		return len(val) > 0
	default:
		return true
	}
}

// monitorRequestGzipMinBytes 是探针请求体启用 gzip 压缩的最小字节数。
//
// 与网关 gateway.upstream_request_compression.min_bytes 的默认值保持一致（64KiB）。
// 探针刻意**不读那份配置**：它的请求体只有几百字节到几 KB，永远够不到这个门槛，
// 这里保留压缩能力是为了与网关行为一致、并为将来可能出现的大 payload 探针留出通路。
//
// ⚠️ 现状：默认探针（一次简短提问）压不到门槛，因此本机制在当下是休眠的；
// 它不会改变任何现有探针的行为。
const monitorRequestGzipMinBytes = 64 << 10

// monitorRequestPayload 返回真正要发送的请求体，以及因压缩而需要附加的请求头。
//
// 压不小就原样返回：宁可不压，也不要为了一个负收益的改动引入新风险。
func monitorRequestPayload(payload []byte) ([]byte, map[string]string) {
	if len(payload) < monitorRequestGzipMinBytes {
		return payload, nil
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return payload, nil
	}
	if _, err := zw.Write(payload); err != nil {
		_ = zw.Close()
		return payload, nil
	}
	if err := zw.Close(); err != nil {
		return payload, nil
	}
	if buf.Len() >= len(payload) {
		return payload, nil
	}
	return buf.Bytes(), map[string]string{"Content-Encoding": "gzip"}
}

// monitorCompressionRetryable 报告该状态码是否值得「去掉压缩重试一次」。
//
// 与网关同一取舍：400 / 415 是上游对请求体的确定性拒绝，而「上游不认
// Content-Encoding: gzip」正好落在这两个码上，去掉压缩重试一次即可自愈。
func monitorCompressionRetryable(status int) bool {
	return status == http.StatusBadRequest || status == http.StatusUnsupportedMediaType
}

// doMonitorPost 发一次探针 POST，返回原始响应（调用方负责关闭 Body）。
// accept 决定 Accept 头；extraHeaders 用于附加压缩相关头。
func doMonitorPost(
	ctx context.Context,
	fullURL string,
	payload []byte,
	headers map[string]string,
	extraHeaders map[string]string,
	accept string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", accept)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	return monitorHTTPClient().Do(req)
}

// postRawJSON 发送 POST + 已序列化好的 JSON 字节，限制响应体大小，
// 返回响应字节、HTTP status、响应头、错误。
// adapter 自行 marshal 是为了精确控制字段顺序与类型，所以这里直接收 []byte 而不是 any。
func postRawJSON(ctx context.Context, fullURL string, payload []byte, headers map[string]string) ([]byte, int, http.Header, error) {
	body, extraHeaders := monitorRequestPayload(payload)

	resp, err := doMonitorPost(ctx, fullURL, body, headers, extraHeaders, "application/json")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("do request: %w", err)
	}
	if extraHeaders != nil && monitorCompressionRetryable(resp.StatusCode) {
		// 上游不认 gzip：去掉压缩原样重试一次，别把「我们自己压了」变成「请求失败」。
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, monitorResponseMaxBytes))
		_ = resp.Body.Close()
		resp, err = doMonitorPost(ctx, fullURL, payload, headers, nil, "application/json")
		if err != nil {
			return nil, 0, nil, fmt.Errorf("do request: %w", err)
		}
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, monitorResponseMaxBytes))
	if err != nil {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("read body: %w", err)
	}
	return respBody, resp.StatusCode, resp.Header, nil
}

// 流式探针支持的内容形态。
const (
	monitorStreamChat = iota + 1
	monitorStreamResponses
)

// monitorRequestBodyStreams 报告最终请求体是否显式声明了 stream=true。
// 只看请求体、不看协议默认值：探针默认 body 恒为 stream=false，
// 因此「非流式行为与改动前逐字一致」这条不变式靠的就是这里。
func monitorRequestBodyStreams(body []byte) bool {
	var probe struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	return probe.Stream
}

// monitorStreamKind 返回该 (provider, apiMode) 是否支持按 SSE 读取。
// 只支持 OpenAI 系：其它 provider 的适配器没有对应的流式解析，维持整包读取。
func monitorStreamKind(provider, apiMode string) (int, bool) {
	if !isOpenAICompatibleChatProvider(provider) {
		return 0, false
	}
	if provider == MonitorProviderOpenAI && defaultAPIMode(apiMode) == MonitorAPIModeResponses {
		return monitorStreamResponses, true
	}
	return monitorStreamChat, true
}

// monitorUpstreamRequestID 从响应头里取出这次调用的上游请求 ID。
// 头名来自归属账号的 extra.upstream_request_id_header；没配就返回空串。
func monitorUpstreamRequestID(opts *CheckOptions, headers http.Header) string {
	if opts == nil || len(headers) == 0 {
		return ""
	}
	name := strings.TrimSpace(opts.UpstreamRequestIDHeader)
	if name == "" {
		return ""
	}
	return truncateUsageUpstreamRequestID(headers.Get(name))
}

// postJSONStream 以流式发起一次探针，边读边解析 SSE，量出首个内容块到达耗时。
// 非 2xx 时仍整包读取响应体，交给上层做错误信息展示。
func postJSONStream(ctx context.Context, fullURL string, payload []byte, headers map[string]string, kind int) (monitorCallResult, error) {
	start := time.Now()
	body, extraHeaders := monitorRequestPayload(payload)

	resp, err := doMonitorPost(ctx, fullURL, body, headers, extraHeaders, "text/event-stream")
	if err != nil {
		return monitorCallResult{}, fmt.Errorf("do request: %w", err)
	}
	if extraHeaders != nil && monitorCompressionRetryable(resp.StatusCode) {
		// 与 postRawJSON 同样的自愈：上游不认 gzip 时去掉压缩重试一次。
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, monitorResponseMaxBytes))
		_ = resp.Body.Close()
		resp, err = doMonitorPost(ctx, fullURL, payload, headers, nil, "text/event-stream")
		if err != nil {
			return monitorCallResult{}, fmt.Errorf("do request: %w", err)
		}
	}
	defer func() { _ = resp.Body.Close() }()

	result := monitorCallResult{Status: resp.StatusCode, Headers: resp.Header, Stream: true}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, monitorResponseMaxBytes))
		result.RawBody = string(raw)
		return result, nil
	}

	text, firstTokenMs, raw, usage := readMonitorSSE(resp.Body, kind, start)
	result.Text = text
	result.FirstTokenMs = firstTokenMs
	result.RawBody = raw
	result.Usage = usage
	return result, nil
}

// readMonitorSSE 逐行读取 SSE，拼出正文并记录首个内容块到达耗时。
// 读取上限沿用 monitorResponseMaxBytes，避免上游异常流把内存撑爆。
func readMonitorSSE(body io.Reader, kind int, start time.Time) (text string, firstTokenMs *int, raw string, usage ProbeUsageTokens) {
	reader := bufio.NewReader(io.LimitReader(body, monitorResponseMaxBytes))
	var textBuilder strings.Builder
	var rawBuilder strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			_, _ = rawBuilder.WriteString(line)
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))

				// 首字口径必须与网关一致：**收到第一个 SSE 数据块**即计时。
				//
				// 这里曾经是「等到第一段正文才计时」，结果与使用记录页的「上游延迟」
				// （A6 账单口径 = 上游第一个字节）不可比：推理模型会先吐大量 reasoning
				// 事件，正文可能十几秒后才出现，于是「本站首字」比「上游首字」虚高一个
				// 完整的推理时长，两者相减得出的「中转开销」能到几秒甚至十几秒，
				// 而真实的中转开销只有几百毫秒。实测该页监控行「首字后剩余」中位数
				// 不足 300ms，正是这一口径错位的指纹。
				if firstTokenMs == nil && data != "" && data != "[DONE]" {
					ms := int(time.Since(start) / time.Millisecond)
					firstTokenMs = &ms
				}

				if data == "[DONE]" {
					break
				}
				// token 用量：Responses 的 response.completed、Anthropic 的 message_*、
				// Chat 的 usage chunk 都会被同一个解析器接住。
				captureProbeUsage(&usage, parseProbeJSONObject(data))
				if content, done := monitorStreamContent(kind, data); content != "" || done {
					if content != "" {
						_, _ = textBuilder.WriteString(content)
					}
					if done {
						break
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	return textBuilder.String(), firstTokenMs, rawBuilder.String(), usage
}

// monitorStreamContent 从一个 SSE data 载荷里取出增量正文，并报告流是否已结束。
func monitorStreamContent(kind int, data string) (string, bool) {
	if data == "" {
		return "", false
	}
	switch kind {
	case monitorStreamResponses:
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return "", false
		}
		switch event.Type {
		case "response.output_text.delta":
			return event.Delta, false
		case "response.completed", "response.incomplete", "response.failed", "error":
			return "", true
		}
		return "", false
	case monitorStreamChat:
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", false
		}
		if len(chunk.Choices) == 0 {
			return "", false
		}
		return chunk.Choices[0].Delta.Content, false
	}
	return "", false
}

// monitorRequestPath 返回探测请求路径。智谱按 endpoint 区分：
//   - 已带 /paas/v4（含 Coding Plan 的 /api/coding/paas/v4）：只追加 /chat/completions
//   - 官方域名根地址：/api/paas/v4/chat/completions
//   - 中转站 / 本站网关：只暴露 OpenAI 兼容的 /v1/chat/completions，与 Kimi / DeepSeek 一致
func monitorRequestPath(provider, endpoint string, adapter providerAdapter, model string) string {
	if provider != MonitorProviderZhipu {
		return adapter.buildPath(model)
	}
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return providerOpenAIPath
	}
	if strings.Contains(u.EscapedPath(), "/paas/v4") {
		return "/chat/completions"
	}
	if isZhipuOfficialHost(u) {
		return adapter.buildPath(model)
	}
	return providerOpenAIPath
}

func isZhipuOfficialHost(u *url.URL) bool {
	host := strings.ToLower(u.Hostname())
	for _, official := range []string{"bigmodel.cn", "z.ai"} {
		if host == official || strings.HasSuffix(host, "."+official) {
			return true
		}
	}
	return false
}

// joinURL 保留 base 的上游路径前缀，并避免重复追加已有的 API 路径前缀。
// 使用 EscapedPath 匹配完整路径段，避免把 hostname 或编码斜杠当作路径。
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if u, err := url.Parse(base); err == nil {
		basePath := u.EscapedPath()
		for end := strings.LastIndex(path, "/"); end > 0; end = strings.LastIndex(path[:end], "/") {
			if strings.HasSuffix(basePath, path[:end]) {
				return base + path[end:]
			}
		}
	}
	return base + path
}

// extractOrigin 从一个 endpoint URL 中提取 scheme://host[:port] 部分。
func extractOrigin(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("endpoint missing scheme or host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// monitorSensitiveQueryParamRegex 匹配 URL query 中可能泄露凭证的参数：
// key / api_key / api-key / access_token / token / authorization / x-api-key。
// 大小写不敏感，匹配 `?name=value` 或 `&name=value` 形式（value 截到 & 或字符串末尾）。
var monitorSensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|api[_-]?key|access[_-]?token|token|authorization|x-api-key)=)[^&\s"']+`)

// monitorAPIKeyPatterns 匹配常见 provider 的 API key 字面量。
// 顺序敏感：sk-ant- 必须放在 sk- 之前，否则会被通用 sk- 模式先消费。
var monitorAPIKeyPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	// Anthropic（带前缀，必须先匹配）：sk-ant-xxxxxxx
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`), "sk-ant-***REDACTED***"},
	// OpenAI / Anthropic 通用 sk-: sk-xxxxxxx
	{regexp.MustCompile(`sk-[A-Za-z0-9-]{20,}`), "sk-***REDACTED***"},
	// xAI API Key：xai-xxxxxxx
	{regexp.MustCompile(`xai-[A-Za-z0-9_-]{6,}`), "xai-***REDACTED***"},
	// Gemini / Google API Key：固定前缀 + 35 位
	{regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`), "AIza***REDACTED***"},
	// Gemini auth key：AQ.Ab 前缀 + 48 位，长度放宽以覆盖格式变体
	{regexp.MustCompile(`AQ\.Ab[A-Za-z0-9_-]{40,}`), "AQ.***REDACTED***"},
	// JWT 三段式（Bearer 后常出现）：eyJxxx.eyJxxx.signature
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "eyJ***REDACTED.JWT***"},
}

// monitorUTF8Replacement 是替换非法 UTF-8 字节用的替换字符（U+FFFD）。
// 上游响应体可能带着被截断或被污染的字节（Latin-1 错误页、半截 gzip、二进制片段），
// 而 PostgreSQL 的 text 列会直接拒收含非法 UTF-8 的整批 INSERT。
const monitorUTF8Replacement = "\uFFFD"

// sanitizeErrorMessage 把错误/响应文本归一化为合法 UTF-8，并擦除可能泄露的 API key。
// 处理三类来源：
//  1. 上游响应体里的非法 UTF-8 字节（截断的多字节字符、被污染的错误页）
//  2. URL query 中的 ?key= / ?api_key= 等（Go *url.Error 会回填完整 URL）
//  3. 上游 HTTP body 文本里直接出现的 sk-* / xai-* / AIza* / JWT 等密钥碎片
//
// 第 1 条必须放在最前面：后面的脱敏与截断都按字节操作，只有先保证文本合法，
// 后续步骤才不会再次切出半个字符。
//
// 注意：与 gemini_messages_compat_service.go 的 sanitizeUpstreamErrorMessage 关注点类似但参数集更广，
// 监控模块独立维护，避免互相耦合。
// 注意：与 gemini_messages_compat_service.go 的 sanitizeUpstreamErrorMessage 关注点类似但参数集更广，
// 监控模块独立维护，避免互相耦合。
func sanitizeErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	msg = strings.ToValidUTF8(msg, monitorUTF8Replacement)
	msg = monitorSensitiveQueryParamRegex.ReplaceAllString(msg, `${1}REDACTED`)
	for _, p := range monitorAPIKeyPatterns {
		msg = p.pattern.ReplaceAllString(msg, p.replace)
	}
	return msg
}

// truncateMessage 把消息按 monitorMessageMaxBytes 截断，避免 DB 列溢出与日志过长。
//
// 截断点会回退到完整字符边界（沿用 ops_error_logger / easypay 等处的既有写法）：
// 按字节硬切会把多字节字符拦腰截断（中文错误文本尤其常见），产出非法 UTF-8，
// 而 PostgreSQL 会因此拒收整批 INSERT —— 表现为探测点静默丢失。
func truncateMessage(msg string) string {
	if len(msg) <= monitorMessageMaxBytes {
		return msg
	}
	const ellipsis = "...(truncated)"
	cutoff := monitorMessageMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	if cutoff > len(msg) {
		cutoff = len(msg)
	}
	for cutoff > 0 && !utf8.ValidString(msg[:cutoff]) {
		cutoff--
	}
	return msg[:cutoff] + ellipsis
}

// truncateForErrorBody 把上游错误响应 body 压到 monitorErrorBodySnippetMaxBytes 以内，
// 并顺手把连续空白折成一个空格：上游 HTML 错误页常含大量缩进/换行，保留会浪费预算。
// 被 truncateMessage 做最终总截断兜底，所以这里只负责 body 自身的精简。
func truncateForErrorBody(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) <= monitorErrorBodySnippetMaxBytes {
		return body
	}
	const ellipsis = "...(body truncated)"
	cutoff := monitorErrorBodySnippetMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	return body[:cutoff] + ellipsis
}
