package service

import (
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// headerWireCasing map + resolveWireCasing moved to domain (Account BC hybrid)
// so ApplyHeaderOverrides can call them. Re-export resolveWireCasing under the
// original unexported name; the map itself is no longer needed here (nothing
// ranges it — only resolveWireCasing did).

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

// resolveWireCasing re-exports domain.ResolveWireCasing under the original
// unexported name so setHeaderRaw/getHeaderRaw/deleteHeaderAllForms compile.
func resolveWireCasing(key string) string {
	return domain.ResolveWireCasing(key)
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

// deleteHeaderAllForms removes a header in all common key forms (raw, wire casing,
// canonical) so subsequent setHeaderRaw will not coexist with a passthrough value
// written under a different casing.
func deleteHeaderAllForms(h http.Header, key string) {
	if h == nil || key == "" {
		return
	}
	h.Del(key) // canonical
	delete(h, key)
	if wk := resolveWireCasing(key); wk != key {
		delete(h, wk)
	}
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
