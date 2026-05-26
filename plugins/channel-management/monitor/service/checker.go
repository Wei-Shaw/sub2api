package monitorservice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// =============================================================
// Channel monitor check 主路径：
//   - runCheckForModel：单 (provider, model) 一次端到端检查 — 调用 provider
//     adapter 发请求 → 校验 challenge / 时延 → 折叠成 CheckResult。
//   - pingEndpointOrigin：HEAD 探测 endpoint origin 测量 RTT。
//
// HTTP 客户端 / sanitize / truncate 等横切助手挪到 checker_http.go；
// provider-specific request body / 响应解析挪到 provider_adapters.go。
// 本文件只保留 monitor 主流程，便于后续平台扩展时一眼看清调度顺序。
// =============================================================

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
