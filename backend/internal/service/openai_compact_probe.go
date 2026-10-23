package service

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

type openAICompactProbeResult uint8

const (
	openAICompactProbeUnknown openAICompactProbeResult = iota
	openAICompactProbeSupported
	openAICompactProbeUnsupported
)

const (
	// AccountTestModeDefault drives the standard /responses connection test.
	AccountTestModeDefault = "default"
	// AccountTestModeCompact drives the remote-compaction probe test
	// (native v2: streaming /responses with a compaction_trigger input item).
	AccountTestModeCompact = "compact"
)

func normalizeAccountTestMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccountTestModeCompact:
		return AccountTestModeCompact
	default:
		return AccountTestModeDefault
	}
}

// createOpenAICompactProbePayload 构造原生 remote compaction v2 探测载荷：
// 流式 /responses + input 末尾 {"type":"compaction_trigger"}。上游已下线
// legacy unary /responses/compact（v1 形态恒 404，#5598/#5624），现行 codex
// 默认协议即 v2（RemoteCompactionV2 Stable + default_enabled）。
func createOpenAICompactProbePayload(model string, isOAuth bool) map[string]any {
	payload := map[string]any{
		"model":        strings.TrimSpace(model),
		"instructions": "You are a helpful coding assistant.",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "Respond with OK.",
			},
			map[string]any{"type": "compaction_trigger"},
		},
		"stream": true,
	}
	// ChatGPT internal API 要求 store: false，与真实转发一致。
	if isOAuth {
		payload["store"] = false
	}
	return payload
}

// openAICompactProbeFoundCompactionItem 判定响应是否满足 Codex 的 v2 消费
// 契约：SSE 中恰好一个 response.output_item.done compaction，并等到成功终态。
// output_item.added、仅终态 response.output[]、整体 JSON 都不能证明真实客户端
// 可消费该 v2 响应。
func openAICompactProbeFoundCompactionItem(body []byte) bool {
	return classifyOpenAICompactProbeSuccess(body) == openAICompactProbeSupported
}

func classifyOpenAICompactProbeSuccess(body []byte) openAICompactProbeResult {
	if len(body) == 0 {
		return openAICompactProbeUnknown
	}
	if bodyHasSSEFraming(body) {
		doneCount := 0
		completed := false
		failed := false
		forEachOpenAISSEFrame(string(body), func(eventType string, data []byte) {
			switch eventType {
			case "response.output_item.done":
				item := gjson.GetBytes(data, "item")
				if item.IsObject() && isResponsesCompactionItemType(item.Get("type").String()) && strings.TrimSpace(item.Get("encrypted_content").String()) != "" {
					doneCount++
				}
			case "response.completed":
				status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(data, "response.status").String()))
				// response.completed is itself the success terminal event. Some
				// compatible relays omit the nested status field, while the Codex
				// client still observes a completed turn from this event type.
				if status == "" || status == "completed" {
					completed = true
				}
			case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
				failed = true
			}
		})
		if failed {
			return openAICompactProbeUnknown
		}
		if !completed {
			return openAICompactProbeUnknown
		}
		if doneCount == 1 {
			return openAICompactProbeSupported
		}
		return openAICompactProbeUnsupported
	}
	if !json.Valid(body) {
		return openAICompactProbeUnknown
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status != "completed" {
		return openAICompactProbeUnknown
	}
	// A completed unary JSON response is not the streaming v2 wire. It may be a
	// legacy/compat response, but it is negative evidence for native v2.
	return openAICompactProbeUnsupported
}

func shouldMarkOpenAICompactUnsupported(status int, body []byte) bool {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return true
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity:
		lower := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body) + " " + string(body)))
		if strings.Contains(lower, "compact") {
			for _, keyword := range []string{
				"unsupported",
				"not support",
				"does not support",
				"not available",
				"disabled",
			} {
				if strings.Contains(lower, keyword) {
					return true
				}
			}
		}
	}
	return false
}

func mergeExtraUpdates(base map[string]any, more map[string]any) map[string]any {
	if len(base) == 0 && len(more) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(more))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range more {
		out[key] = value
	}
	return out
}

// compactProbeSessionID 返回探测请求使用的会话标识。真实 Codex 的
// session-id / thread-id 恒为 UUID（codex-protocol ThreadId 是 UUIDv7），
// 探测既然与真实流量走同一个 /responses 端点，标识形态就必须同构——
// 否则上游能凭 "probe_compact_5" 这类字面量一眼区分出探测流量。
// 账号级稳定派生：重复探测复用同一会话，而不是每次新开一个。
func compactProbeSessionID(accountID int64) string {
	if accountID <= 0 {
		return deriveStableUUIDv4("sub2api:codex-compact-probe:v1:anonymous")
	}
	return deriveStableUUIDv4("sub2api:codex-compact-probe:v1:" + strconv.FormatInt(accountID, 10))
}
