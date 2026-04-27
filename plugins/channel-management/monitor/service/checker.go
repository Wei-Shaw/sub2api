package monitorservice

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

	"github.com/tidwall/gjson"
)

// monitorHTTPClient is the lazily-initialised SafeHTTPClient used for
// challenge requests. monitorPingHTTPClient is the shorter-timeout twin
// used for endpoint origin pings. They are constructed via the SDK's
// SafeOutboundHTTP helper so SSRF protection (block lists, DNS rebinding,
// per-host allow lists from the host) live inside the SDK rather than
// the plugin process itself — this is the W4 replacement for the old
// 152-line hand-rolled channel_monitor_ssrf.go.
var (
	monitorHTTPClientOnce  sync.Once
	monitorHTTPClientCache *http.Client
	monitorPingClientOnce  sync.Once
	monitorPingClientCache *http.Client
)

// getMonitorHTTPClient builds (or returns the cached) main outbound client.
// Errors are surfaced via the result error so each request can decide whether
// to fall back to a "skip check" CheckResult; constructing a raw http.Client
// without SafeOutboundHTTP is intentionally NOT a fallback — that would
// regress SSRF protections.
func getMonitorHTTPClient() (*http.Client, error) {
	var buildErr error
	monitorHTTPClientOnce.Do(func() {
		c, err := pluginsdk.NewSafeHTTPClient(pluginsdk.OutboundConfig{
			Timeout:      monitorRequestTimeout,
			MaxBodyBytes: monitorResponseMaxBytes,
		})
		if err != nil {
			buildErr = err
			return
		}
		monitorHTTPClientCache = c
	})
	if buildErr != nil {
		return nil, buildErr
	}
	if monitorHTTPClientCache == nil {
		return nil, stderrors.New("safe http client not initialised")
	}
	return monitorHTTPClientCache, nil
}

// getMonitorPingHTTPClient is the shorter-timeout twin for HEAD pings.
func getMonitorPingHTTPClient() (*http.Client, error) {
	var buildErr error
	monitorPingClientOnce.Do(func() {
		c, err := pluginsdk.NewSafeHTTPClient(pluginsdk.OutboundConfig{
			Timeout:      monitorPingTimeout,
			MaxBodyBytes: monitorPingDiscardMaxBytes,
		})
		if err != nil {
			buildErr = err
			return
		}
		monitorPingClientCache = c
	})
	if buildErr != nil {
		return nil, buildErr
	}
	if monitorPingClientCache == nil {
		return nil, stderrors.New("safe ping client not initialised")
	}
	return monitorPingClientCache, nil
}

// runCheckForModel performs a complete check of a single (provider, model).
// It never returns error: all failures are folded into CheckResult.Status
// (error / failed).
//
// opts carries the per-monitor / per-template snapshot (extra headers, body
// override mode, body override). nil opts behaves as "off + no extra headers".
func runCheckForModel(ctx context.Context, provider, endpoint, apiKey, model string, opts *CheckOptions) *CheckResult {
	res := &CheckResult{
		Model:     model,
		Status:    MonitorStatusError,
		CheckedAt: time.Now(),
	}

	challenge := generateChallenge()
	mode := bodyOverrideMode(opts)

	start := time.Now()
	respText, rawBody, statusCode, err := callProvider(ctx, provider, endpoint, apiKey, model, challenge.Prompt, opts)
	latency := time.Since(start)
	latencyMs := int(latency / time.Millisecond)
	res.LatencyMs = &latencyMs

	if err != nil {
		res.Status = MonitorStatusError
		res.Message = truncateMessage(sanitizeErrorMessage(err.Error()))
		return res
	}
	if statusCode < 200 || statusCode >= 300 {
		res.Status = MonitorStatusError
		bodySnippet := truncateForErrorBody(rawBody)
		res.Message = truncateMessage(sanitizeErrorMessage(fmt.Sprintf("upstream HTTP %d: %s", statusCode, bodySnippet)))
		return res
	}

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

// finalizeOperationalOrDegraded picks operational vs degraded based on latency.
// Extracted to keep runCheckForModel under 30 lines.
func finalizeOperationalOrDegraded(res *CheckResult, latency time.Duration, latencyMs int) *CheckResult {
	if latency >= monitorDegradedThreshold {
		res.Status = MonitorStatusDegraded
		res.Message = truncateMessage(fmt.Sprintf("slow response: %dms", latencyMs))
		return res
	}
	res.Status = MonitorStatusOperational
	return res
}

// bodyOverrideMode normalises opts.BodyOverrideMode. nil opts and empty
// mode both map to off.
func bodyOverrideMode(opts *CheckOptions) string {
	if opts == nil || opts.BodyOverrideMode == "" {
		return MonitorBodyOverrideModeOff
	}
	return opts.BodyOverrideMode
}

// pingEndpointOrigin issues a HEAD request to the endpoint's origin and
// returns the round-trip duration in ms. Returns nil on any failure — ping
// is best-effort, the caller persists nil for ping_latency_ms in that case.
func pingEndpointOrigin(ctx context.Context, endpoint string) *int {
	origin, err := extractOrigin(endpoint)
	if err != nil || origin == "" {
		return nil
	}
	client, err := getMonitorPingHTTPClient()
	if err != nil {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, origin, nil)
	if err != nil {
		return nil
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, monitorPingDiscardMaxBytes))
	ms := int(time.Since(start) / time.Millisecond)
	return &ms
}

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

// isSupportedProvider returns true when p is registered in providerAdapters.
func isSupportedProvider(p string) bool {
	_, ok := providerAdapters[p]
	return ok
}

// isPrivateOrLoopbackHost is delegated to the SDK's SafeOutboundHTTP layer
// and therefore degrades to "always allow" at validation time. The actual
// SSRF check happens at every dial inside SafeHTTPClient — TOCTOU and DNS
// rebinding are caught when the network call is made, not at form-submit
// time. validateEndpoint still uses this hook to emit a host-side "looks
// reachable" message; the real protection lives downstream.
func isPrivateOrLoopbackHost(_ context.Context, _ string) (bool, error) {
	return false, nil
}

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
func buildRequestBody(adapter providerAdapter, provider, model, prompt string, opts *CheckOptions) ([]byte, error) {
	mode := bodyOverrideMode(opts)

	if mode == MonitorBodyOverrideModeReplace {
		if opts == nil || len(opts.BodyOverride) == 0 {
			return nil, fmt.Errorf("replace mode: body_override is empty")
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
	deny := bodyMergeKeyDenyList[provider]
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

// bodyMergeKeyDenyList lists the provider-specific keys that the merge
// body-override mode refuses to overwrite (challenge / model routing).
var bodyMergeKeyDenyList = map[string]map[string]bool{
	MonitorProviderOpenAI:    {"model": true, "messages": true, "stream": true},
	MonitorProviderAnthropic: {"model": true, "messages": true},
	MonitorProviderGemini:    {"contents": true},
}

// postRawJSON issues a POST with already-serialised JSON bytes and returns
// the response body, status code, and error. Response body size is capped
// via the SafeHTTPClient transport.
func postRawJSON(ctx context.Context, fullURL string, payload []byte, headers map[string]string) ([]byte, int, error) {
	client, err := getMonitorHTTPClient()
	if err != nil {
		return nil, 0, fmt.Errorf("init safe client: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := pluginsdk.LimitedReadAll(resp.Body, monitorResponseMaxBytes)
	if err != nil && !stderrors.Is(err, pluginsdk.ErrBodyTooLarge) {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// joinURL stitches base origin + path into a full URL, tolerating trailing
// slashes on base and ensuring path has a leading slash.
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// extractOrigin parses an endpoint URL and returns scheme://host[:port].
func extractOrigin(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", stderrors.New("endpoint missing scheme or host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// monitorSensitiveQueryParamRegex matches credential-bearing URL query params
// (key / api_key / api-key / access_token / token / authorization / x-api-key).
// Case insensitive; matches `?name=value` or `&name=value` shapes.
var monitorSensitiveQueryParamRegex = regexp.MustCompile(`(?i)([?&](?:key|api[_-]?key|access[_-]?token|token|authorization|x-api-key)=)[^&\s"']+`)

// monitorAPIKeyPatterns matches common provider API-key literals. Order
// matters: sk-ant- must come before sk- so it is consumed first.
var monitorAPIKeyPatterns = []struct {
	pattern *regexp.Regexp
	replace string
}{
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{20,}`), "sk-ant-***REDACTED***"},
	{regexp.MustCompile(`sk-[A-Za-z0-9-]{20,}`), "sk-***REDACTED***"},
	{regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`), "AIza***REDACTED***"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), "eyJ***REDACTED.JWT***"},
}

// sanitizeErrorMessage scrubs API-key fragments and credential-bearing query
// params out of error / response strings before they are persisted as the
// monitor history "message" column.
func sanitizeErrorMessage(msg string) string {
	if msg == "" {
		return msg
	}
	msg = monitorSensitiveQueryParamRegex.ReplaceAllString(msg, `${1}REDACTED`)
	for _, p := range monitorAPIKeyPatterns {
		msg = p.pattern.ReplaceAllString(msg, p.replace)
	}
	return msg
}

// truncateMessage caps the message length to monitorMessageMaxBytes to avoid
// DB column overflow + over-long log lines.
func truncateMessage(msg string) string {
	if len(msg) <= monitorMessageMaxBytes {
		return msg
	}
	const ellipsis = "...(truncated)"
	cutoff := monitorMessageMaxBytes - len(ellipsis)
	if cutoff < 0 {
		cutoff = 0
	}
	return msg[:cutoff] + ellipsis
}

// truncateForErrorBody trims an upstream error body to
// monitorErrorBodySnippetMaxBytes, collapsing whitespace runs to a single
// space first so HTML error pages don't waste budget.
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
