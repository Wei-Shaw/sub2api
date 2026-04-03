package service

import (
	"net/http"
	"strings"
)

// headerWireCasing 定义每个白名单 header 在真实 Claude CLI 抓包中的准确大小写。
// Go 的 HTTP server 解析请求时会将所有 header key 转为 Canonical 形式（如 x-app → X-App），
// 此 map 用于在转发时恢复到真实的 wire format。
//
// 来源：对真实 Claude CLI (claude-cli/2.1.81) 到 api.anthropic.com 的 HTTPS 流量抓包。
var headerWireCasing = map[string]string{
	// Title case
	"accept":     "Accept",
	"user-agent": "User-Agent",

	// X-Stainless-* 保持 SDK 原始大小写
	"x-stainless-retry-count":     "X-Stainless-Retry-Count",
	"x-stainless-timeout":         "X-Stainless-Timeout",
	"x-stainless-lang":            "X-Stainless-Lang",
	"x-stainless-package-version": "X-Stainless-Package-Version",
	"x-stainless-os":              "X-Stainless-OS",
	"x-stainless-arch":            "X-Stainless-Arch",
	"x-stainless-runtime":         "X-Stainless-Runtime",
	"x-stainless-runtime-version": "X-Stainless-Runtime-Version",
	"x-stainless-helper-method":   "x-stainless-helper-method",

	// Anthropic SDK 自身设置的 header，全小写
	"anthropic-dangerous-direct-browser-access": "anthropic-dangerous-direct-browser-access",
	"anthropic-version":                         "anthropic-version",
	"anthropic-beta":                            "anthropic-beta",
	"x-app":                                     "x-app",
	"content-type":                              "content-type",
	"accept-language":                           "accept-language",
	"sec-fetch-mode":                            "sec-fetch-mode",
	"accept-encoding":                           "accept-encoding",
	"authorization":                             "authorization",

	// Claude Code 2.1.87+ 新增 header
	"x-claude-code-session-id": "X-Claude-Code-Session-Id",
	"x-client-request-id":      "x-client-request-id",
	"content-length":           "content-length",
}

// headerWireOrder 定义真实 Claude CLI 发送 header 的顺序（基于抓包）。
// 用于 debug log 按此顺序输出，便于与抓包结果直接对比。
var headerWireOrder = []string{
	"Accept",
	"X-Stainless-Retry-Count",
	"X-Stainless-Timeout",
	"X-Stainless-Lang",
	"X-Stainless-Package-Version",
	"X-Stainless-OS",
	"X-Stainless-Arch",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
	"anthropic-dangerous-direct-browser-access",
	"anthropic-version",
	"authorization",
	"x-app",
	"User-Agent",
	"X-Claude-Code-Session-Id",
	"content-type",
	"anthropic-beta",
	"x-client-request-id",
	"accept-language",
	"sec-fetch-mode",
	"accept-encoding",
	"content-length",
	"x-stainless-helper-method",
}

// headerWireOrderSet 用于快速判断某个 key 是否在 headerWireOrder 中（按 lowercase 匹配）。
var headerWireOrderSet map[string]struct{}

func init() {
	headerWireOrderSet = make(map[string]struct{}, len(headerWireOrder))
	for _, k := range headerWireOrder {
		headerWireOrderSet[strings.ToLower(k)] = struct{}{}
	}
}

// resolveWireCasing 将 Go canonical key（如 X-Stainless-Os）映射为真实 wire casing（如 X-Stainless-OS）。
// 如果 map 中没有对应条目，返回原始 key 不变。
func resolveWireCasing(key string) string {
	if wk, ok := headerWireCasing[strings.ToLower(key)]; ok {
		return wk
	}
	return key
}

// setHeaderRaw sets a header bypassing Go's canonical-case normalization.
// The key is stored exactly as provided, preserving original casing.
//
// It first removes any existing value under the canonical key, the wire casing key,
// and the exact raw key, preventing duplicates from any source.
func setHeaderRaw(h http.Header, key, value string) {
	h.Del(key) // remove canonical form (e.g. "Anthropic-Beta")
	if wk := resolveWireCasing(key); wk != key {
		delete(h, wk) // remove wire casing form if different
	}
	delete(h, key) // remove exact raw key if it differs from canonical
	h[key] = []string{value}
}

// addHeaderRaw appends a header value bypassing Go's canonical-case normalization.
func addHeaderRaw(h http.Header, key, value string) {
	h[key] = append(h[key], value)
}

// getHeaderRaw reads a header value, trying multiple key forms to handle the mismatch
// between Go canonical keys, wire casing keys, and raw keys:
//  1. exact key as provided
//  2. wire casing form (from headerWireCasing)
//  3. Go canonical form (via http.Header.Get)
func getHeaderRaw(h http.Header, key string) string {
	// 1. exact key
	if vals := h[key]; len(vals) > 0 {
		return vals[0]
	}
	// 2. wire casing (e.g. looking up "Anthropic-Dangerous-Direct-Browser-Access" finds "anthropic-dangerous-direct-browser-access")
	if wk := resolveWireCasing(key); wk != key {
		if vals := h[wk]; len(vals) > 0 {
			return vals[0]
		}
	}
	// 3. canonical fallback
	return h.Get(key)
}

// sortHeadersByWireOrder 按照真实 Claude CLI 的 header 顺序返回排序后的 key 列表。
// 在 headerWireOrder 中定义的 key 按其顺序排列，未定义的 key 追加到末尾。
func sortHeadersByWireOrder(h http.Header) []string {
	// 构建 lowercase -> actual map key 的映射
	present := make(map[string]string, len(h))
	for k := range h {
		present[strings.ToLower(k)] = k
	}

	result := make([]string, 0, len(h))
	seen := make(map[string]struct{}, len(h))

	// 先按 wire order 输出
	for _, wk := range headerWireOrder {
		lk := strings.ToLower(wk)
		if actual, ok := present[lk]; ok {
			if _, dup := seen[lk]; !dup {
				result = append(result, actual)
				seen[lk] = struct{}{}
			}
		}
	}

	// 再追加不在 wire order 中的 header
	for k := range h {
		lk := strings.ToLower(k)
		if _, ok := seen[lk]; !ok {
			result = append(result, k)
			seen[lk] = struct{}{}
		}
	}

	return result
}
