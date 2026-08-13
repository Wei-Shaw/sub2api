package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/tidwall/gjson"
)

// openaiResponsesProbeTimeout 是探测请求的超时时长。
// 探测在后台 goroutine 中异步执行,不阻塞账号创建/更新;留出余量给推理型模型
// 先思考再产出 function_call 的往返。超时则保持 unknown,不下结论。
const openaiResponsesProbeTimeout = 15 * time.Second

// responsesProbeMaxBodyBytes 限制读取探测响应体的字节数,够判定 output 项类型即可。
const responsesProbeMaxBodyBytes = 256 * 1024

// openaiResponsesProbeMaxOutputTokens 是探测请求的输出预算。
// 推理型模型可能把预算全烧在 reasoning 上,还没轮到 function_call 就被截断——
// 那种响应不能用来判定工具能力,见 responsesProbeVerdictIsConclusive。
const openaiResponsesProbeMaxOutputTokens = 512

// openaiResponsesProbePayload 构造探测用的 Responses 请求体。
//
// 关键设计:请求携带一个工具并以 tool_choice=required 强制模型调用它。这样
// 一个真正支持 Responses 工具调用的上游必须在响应里产出 function_call 输出项;
// 而"端点存在、基础补全可用、但工具调用坏掉"的上游(如火山方舟 coding/v3 ×
// kimi-k2.6,只回 reasoning、不产出 function_call)会被这一步暴露出来。
//
// Stream=false 便于一次性读取 output 数组判定;不带 instructions 以免干扰。
func openaiResponsesProbePayload(modelID string) []byte {
	if strings.TrimSpace(modelID) == "" {
		modelID = openai.DefaultTestModel
	}
	body, _ := json.Marshal(map[string]any{
		"model": modelID,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "Call the probe_ping function with ok=true to acknowledge readiness. You must use the tool."},
				},
			},
		},
		"tools": []map[string]any{
			{
				"type":        "function",
				"name":        "probe_ping",
				"description": "Capability probe. Call to acknowledge.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ok": map[string]any{"type": "boolean"},
					},
					"required": []string{"ok"},
				},
			},
		},
		"tool_choice":       "required",
		"max_output_tokens": openaiResponsesProbeMaxOutputTokens,
		"stream":            false,
	})
	return body
}

// selectResponsesProbeModel 选出用于探测的上游模型。
//
// 工具能力探测必须用上游真实存在的模型——用占位模型(DefaultTestModel)打第三方
// 上游只会拿到 400 model-not-found,无从判定工具能力。优先取账号 model_mapping
// 的上游模型(值),按字典序取首个具体(非通配符)模型以保证可复现;无映射时回退
// DefaultTestModel(适配 OpenAI 官方 APIKey 账号)。
func selectResponsesProbeModel(account *Account) string {
	mapping := account.GetModelMapping()
	candidates := make([]string, 0, len(mapping))
	for _, upstream := range mapping {
		upstream = strings.TrimSpace(upstream)
		if upstream == "" || strings.Contains(upstream, "*") {
			continue
		}
		candidates = append(candidates, upstream)
	}
	if len(candidates) == 0 {
		return openai.DefaultTestModel
	}
	sort.Strings(candidates)
	return candidates[0]
}

// ProbeOpenAIAPIKeyResponsesSupport 探测 OpenAI APIKey 账号上游是否支持
// /v1/responses 端点，并将结果持久化到 accounts.extra.openai_responses_supported。
//
// 调用时机：账号创建/更新后，且仅当 platform=openai && type=apikey 时。
//
// 探测策略（参见包文档 internal/pkg/openai_compat）：
//   - 上游 404 / 405 → 端点不存在,写 false
//   - 上游 2xx → 端点存在,进一步看工具能力:响应含 function_call 输出项才写 true;
//     仅 reasoning / 无 function_call(如火山方舟 coding/v3 × kimi-k2.6)写 false
//   - 其他非 2xx（401/422/400/5xx 等）→ 端点存在但无法判定工具能力,保守写 true
//   - 网络层失败（连接错误、超时）→ 不写标记，保持 unknown
//     （后续请求仍按"现状即证据"默认走 Responses）
//
// 该方法是幂等的：重复调用会以最新探测结果覆盖标记。
//
// 关于失败处理：探测本身的失败不应阻塞账号创建——账号能创建/更新成功就够了，
// 探测结果只影响后续路由优化。所有错误都仅记录日志，不向调用方传播。
func (s *AccountTestService) ProbeOpenAIAPIKeyResponsesSupport(ctx context.Context, accountID int64) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		logger.LegacyPrintf("service.openai_probe", "probe_load_account_failed: account_id=%d err=%v", accountID, err)
		return
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		// 仅 OpenAI APIKey 账号需要探测；其他账号类型无能力差异。
		return
	}

	apiKey := account.GetOpenAIApiKey()
	if apiKey == "" {
		logger.LegacyPrintf("service.openai_probe", "probe_skip_no_apikey: account_id=%d", accountID)
		return
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		logger.LegacyPrintf("service.openai_probe", "probe_invalid_baseurl: account_id=%d base_url=%q err=%v", accountID, baseURL, err)
		return
	}

	probeURL := buildOpenAIResponsesURL(normalizedBaseURL)
	probeModel := selectResponsesProbeModel(account)

	probeCtx, cancel := context.WithTimeout(ctx, openaiResponsesProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodPost, probeURL, bytes.NewReader(openaiResponsesProbePayload(probeModel)))
	if err != nil {
		logger.LegacyPrintf("service.openai_probe", "probe_build_request_failed: account_id=%d err=%v", accountID, err)
		return
	}
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAI))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	applyOpenAICodexProbeHeaders(req.Header)

	// 账号级请求头覆写：能力探测与真实转发保持一致的最终头
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, s.tlsFPProfileService.ResolveTLSProfile(account))
	if err != nil {
		// 网络层失败：不写标记，保持 unknown，下次重试或由网关 fallback 处理
		logger.LegacyPrintf("service.openai_probe", "probe_request_failed: account_id=%d url=%s err=%v", accountID, probeURL, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, responsesProbeMaxBodyBytes))
	// 有界排空剩余响应体:既帮助连接复用,又避免行为异常的上游用超大响应体拖住探测。
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, responsesProbeMaxBodyBytes))
	if readErr != nil {
		// 响应体读取失败(部分读取/传输错误):按网络层失败处理,保持 unknown,
		// 不写标记——否则可能给一个 2xx 响应误写 supported=false。
		logger.LegacyPrintf("service.openai_probe", "probe_read_body_failed: account_id=%d url=%s err=%v", accountID, probeURL, readErr)
		return
	}

	// 本次响应不足以下结论时保持 unknown，与网络层失败、响应体读取失败一致：
	// 标记一旦写成 false 就会一直粘住（只有下次账号创建/更新才重探），网关会静默
	// 改走 /v1/chat/completions —— 对 Codex 客户端意味着 prompt 缓存前缀被打散。
	// 宁可不写，让请求继续走既有的 Responses 路径。
	if !responsesProbeVerdictIsConclusive(resp.StatusCode, bodyBytes) {
		logger.LegacyPrintf("service.openai_probe",
			"probe_inconclusive_keep_unknown: account_id=%d base_url=%s probe_model=%s status=%d response_status=%s reason=%s",
			accountID, normalizedBaseURL, probeModel, resp.StatusCode,
			gjson.GetBytes(bodyBytes, "status").String(),
			gjson.GetBytes(bodyBytes, "incomplete_details.reason").String(),
		)
		return
	}

	supported := decideResponsesProbeSupport(resp.StatusCode, bodyBytes)

	if err := s.accountRepo.UpdateExtra(ctx, accountID, map[string]any{
		openai_compat.ExtraKeyResponsesSupported: supported,
	}); err != nil {
		logger.LegacyPrintf("service.openai_probe", "probe_persist_failed: account_id=%d supported=%v err=%v", accountID, supported, err)
		return
	}

	if !supported {
		// 落标为不支持等于把该账号长期钉在 /v1/chat/completions 上，成本与缓存命中率
		// 都会变化，且不会自动恢复。这条必须能被运维看到（#5371）。
		slog.Warn(
			"openai_responses_probe_marked_unsupported",
			"account_id", accountID,
			"account_name", account.Name,
			"base_url", normalizedBaseURL,
			"probe_model", probeModel,
			"upstream_status", resp.StatusCode,
		)
	}

	logger.LegacyPrintf("service.openai_probe",
		"probe_done: account_id=%d base_url=%s probe_model=%s status=%d supported=%v",
		accountID, normalizedBaseURL, probeModel, resp.StatusCode, supported,
	)
}

// responsesProbeVerdictIsConclusive 判断本次探测响应是否足以对「上游是否支持带工具的
// Responses 调用」下结论。
//
// 2xx 分支靠「output 里有没有 function_call」下结论，但这只在响应真的跑完时成立：
//
//   - status=incomplete 且 incomplete_details.reason=max_output_tokens：探测请求自己
//     只给了 openaiResponsesProbeMaxOutputTokens 的预算，推理型模型可能把预算全烧在
//     reasoning 上，还没轮到 function_call 就被截断。此时「没有 function_call」是探测
//     预算不足造成的，不是上游能力缺失。
//   - status=failed：HTTP 200 携带的失败响应（上游瞬时故障）同样不构成能力证据。
//
// 其余 2xx 一律可下结论——尤其 status=completed 却只回 reasoning 的上游（火山方舟
// coding/v3 × kimi-k2.6），仍按原逻辑判为不支持。
//
// 非 2xx 的结论只看状态码、不依赖响应内容，恒可下结论。
// 缺少 status 字段的响应体（含非 JSON）也按可下结论处理，保持既有行为。
func responsesProbeVerdictIsConclusive(status int, body []byte) bool {
	if status < 200 || status >= 300 {
		return true
	}
	switch strings.TrimSpace(gjson.GetBytes(body, "status").String()) {
	case "failed":
		return false
	case "incomplete":
		return strings.TrimSpace(gjson.GetBytes(body, "incomplete_details.reason").String()) != "max_output_tokens"
	default:
		return true
	}
}

// isResponsesEndpointSupportedByStatus 根据探测响应的 HTTP 状态码判定上游
// 是否暴露 /v1/responses 端点。
//
// 关键观察：第三方 OpenAI 兼容上游（DeepSeek/Kimi 等）对未知端点统一返回 404
// 或 405；而 OpenAI 官方/有 Responses 实现的上游会因为请求体最简（缺字段）
// 返回 400/422 等业务错误，但端点本身存在。
//
// 因此：仅 404 和 405 视为"端点不存在"，其他 status 视为"端点存在"。
//
// 5xx 也视为"端点存在"——上游偶发故障不应误判为不支持。
func isResponsesEndpointSupportedByStatus(status int) bool {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return false
	}
	return true
}

// decideResponsesProbeSupport 依据探测响应判定上游 /v1/responses 是否真正可用于
// 携带工具的请求。
//
//   - 404 / 405：端点不存在 → false
//   - 其他非 2xx（401/403/422/5xx 等）：端点存在,但本次无法判定工具能力
//     （鉴权/校验/瞬时故障）→ 保守按 true,保持既有"端点存在即支持"行为
//   - 2xx：探测以 tool_choice=required 强制工具调用,响应必须含 function_call
//     输出项才算真正可用;否则(如火山方舟 coding/v3 × kimi-k2.6 仅回 reasoning)
//     判为 false,使网关改走 /v1/chat/completions 直转路径。
func decideResponsesProbeSupport(status int, body []byte) bool {
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed {
		return false
	}
	if status < 200 || status >= 300 {
		return true
	}
	return responsesProbeBodyHasFunctionCall(body)
}

// responsesProbeBodyHasFunctionCall 判断非流式 Responses 响应体的 output 数组里
// 是否存在 function_call 输出项。
func responsesProbeBodyHasFunctionCall(body []byte) bool {
	output := gjson.GetBytes(body, "output")
	if !output.IsArray() {
		return false
	}
	for _, item := range output.Array() {
		if strings.TrimSpace(item.Get("type").String()) == "function_call" {
			return true
		}
	}
	return false
}
