package monitorservice

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
)

// =============================================================
// Provider adapters: OpenAI / Anthropic / Gemini 三家厂商的请求构造与
// 响应文本提取。新增 provider 只需要往 providerAdapters map 里加一个
// adapter，主流程 (callProvider / runCheckForModel) 不需要改动 —— 这是
// "抽象 (Abstract)" 原则在 monitor 层的落地点。
// =============================================================

// providerAdapter describes the four hooks each provider needs:
//   - request path (with model placeholder)
//   - request body marshaller
//   - auth header builder
//   - gjson path that extracts the response text
type providerAdapter struct {
	buildPath    func(model string) string
	buildBody    func(model, prompt string) ([]byte, error)
	buildHeaders func(apiKey string) map[string]string
	textPath     string
}

// providerAdapters maps provider strings to their adapters.
var providerAdapters = map[string]providerAdapter{
	MonitorProviderOpenAI: {
		buildPath: func(string) string { return providerOpenAIPath },
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
	},
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
		textPath: "content.0.text",
	},
	MonitorProviderGemini: {
		buildPath: func(model string) string { return fmt.Sprintf(providerGeminiPathTemplate, model) },
		buildBody: func(_, prompt string) ([]byte, error) {
			return json.Marshal(map[string]any{
				"contents": []map[string]any{
					{"parts": []map[string]any{{"text": prompt}}},
				},
				"generationConfig": map[string]any{"maxOutputTokens": monitorChallengeMaxTokens},
			})
		},
		buildHeaders: func(apiKey string) map[string]string {
			return map[string]string{"x-goog-api-key": apiKey}
		},
		textPath: "candidates.0.content.parts.0.text",
	},
}

// bodyMergeKeyDenyList lists the provider-specific keys that the merge
// body-override mode refuses to overwrite (challenge / model routing).
var bodyMergeKeyDenyList = map[string]map[string]bool{
	MonitorProviderOpenAI:    {"model": true, "messages": true, "stream": true},
	MonitorProviderAnthropic: {"model": true, "messages": true},
	MonitorProviderGemini:    {"contents": true},
}

// isSupportedProvider returns true when p is registered in providerAdapters.
func isSupportedProvider(p string) bool {
	_, ok := providerAdapters[p]
	return ok
}

// 历史上这里曾有一个 isPrivateOrLoopbackHost(ctx, hostname) hook 给
// validateEndpoint 做 SSRF 预检，但因为它永远返回 (false, nil)（实际防护
// 已下沉到 SDK SafeOutboundHTTP 层），保留只会让代码读起来误以为这里有检查。
// T13 一并移除，避免误导。

// callProvider dispatches into providerAdapters and returns the extracted
// text plus the raw response body.
func callProvider(ctx context.Context, provider, endpoint, apiKey, model, prompt string, opts *CheckOptions) (extractedText, rawBody string, status int, err error) {
	adapter, ok := providerAdapters[provider]
	if !ok {
		return "", "", 0, fmt.Errorf("unsupported provider %q", provider)
	}
	body, err := buildRequestBody(adapter, provider, model, prompt, opts)
	if err != nil {
		return "", "", 0, err
	}
	headers := mergeHeaders(adapter.buildHeaders(apiKey), opts)
	full := joinURL(endpoint, adapter.buildPath(model))
	respBytes, status, err := postRawJSON(ctx, full, body, headers)
	if err != nil {
		return "", "", status, err
	}
	return gjson.GetBytes(respBytes, adapter.textPath).String(), string(respBytes), status, nil
}

// mergeHeaders merges the user-supplied extra headers onto the adapter's
// defaults. User values win; forbidden hop-by-hop / HTTP-client-managed
// keys are dropped silently.
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

// buildRequestBody constructs the request body according to body_override_mode.
//
//   - off:     adapter default body
//   - merge:   adapter default body shallow-merged with BodyOverride; keys in
//     bodyMergeKeyDenyList[provider] are dropped to protect challenge / model routing
//   - replace: BodyOverride is marshalled directly as the entire body
//
// 主流程拆出 buildReplaceBody / buildMergedBody 两个 helper 让本函数 ≤ 30 行。
func buildRequestBody(adapter providerAdapter, provider, model, prompt string, opts *CheckOptions) ([]byte, error) {
	mode := bodyOverrideMode(opts)

	if mode == MonitorBodyOverrideModeReplace {
		return buildReplaceBody(opts)
	}

	defaultBody, err := adapter.buildBody(model, prompt)
	if err != nil {
		return nil, fmt.Errorf("marshal default body: %w", err)
	}
	if mode != MonitorBodyOverrideModeMerge || opts == nil || len(opts.BodyOverride) == 0 {
		return defaultBody, nil
	}
	return buildMergedBody(defaultBody, provider, opts.BodyOverride)
}

// buildReplaceBody marshals opts.BodyOverride directly as the full request
// body. Returns an error when BodyOverride is empty (caller's mode says
// replace but no override given — almost certainly a config bug).
func buildReplaceBody(opts *CheckOptions) ([]byte, error) {
	if opts == nil || len(opts.BodyOverride) == 0 {
		return nil, fmt.Errorf("replace mode: body_override is empty")
	}
	body, err := json.Marshal(opts.BodyOverride)
	if err != nil {
		return nil, fmt.Errorf("marshal body_override (replace): %w", err)
	}
	return body, nil
}

// buildMergedBody shallow-merges override onto defaultBody, skipping any
// key listed in bodyMergeKeyDenyList[provider] (those guard the challenge
// prompt / model routing invariants).
func buildMergedBody(defaultBody []byte, provider string, override map[string]any) ([]byte, error) {
	var defaultMap map[string]any
	if err := json.Unmarshal(defaultBody, &defaultMap); err != nil {
		return nil, fmt.Errorf("unmarshal default body for merge: %w", err)
	}
	deny := bodyMergeKeyDenyList[provider]
	for k, v := range override {
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
