package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"golang.org/x/net/http/httpguts"
)

const openAICustomHeadersExtraKey = "openai_custom_headers"
const openAICustomAuthorizationTokenPlaceholder = "__sub2api_openai_custom_authorization__"

var openAICustomHeaderForbiddenNames = map[string]struct{}{
	"accept":                   {},
	"chatgpt-account-id":       {},
	"connection":               {},
	"content-length":           {},
	"content-type":             {},
	"conversation_id":          {},
	"cookie":                   {},
	"host":                     {},
	"openai-beta":              {},
	"originator":               {},
	"proxy-authorization":      {},
	"sec-websocket-extensions": {},
	"sec-websocket-key":        {},
	"sec-websocket-protocol":   {},
	"sec-websocket-version":    {},
	"session_id":               {},
	"set-cookie":               {},
	"transfer-encoding":        {},
	"upgrade":                  {},
	"user-agent":               {},
	"version":                  {},
	"x-api-key":                {},
	"x-codex-turn-metadata":    {},
	"x-codex-turn-state":       {},
	"x-goog-api-key":           {},
}

// GetOpenAICustomHeaders returns account-level custom request headers configured in Extra.
// The canonical storage format is an object: {"Header-Name": "value"}. A row-array format is
// also accepted for import/UI compatibility: [{"name":"Header-Name","value":"value"}].
func (a *Account) GetOpenAICustomHeaders() http.Header {
	if a == nil || !a.IsOpenAI() || len(a.Extra) == 0 {
		return nil
	}
	raw, ok := a.Extra[openAICustomHeadersExtraKey]
	if !ok || raw == nil {
		return nil
	}

	headers := make(http.Header)
	switch value := raw.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			addOpenAICustomHeader(headers, key, value[key])
		}
	case map[string]string:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			addOpenAICustomHeader(headers, key, value[key])
		}
	case []any:
		for _, row := range value {
			addOpenAICustomHeaderRow(headers, row)
		}
	case []map[string]any:
		for _, row := range value {
			addOpenAICustomHeaderRow(headers, row)
		}
	case []map[string]string:
		for _, row := range value {
			addOpenAICustomHeaderRow(headers, row)
		}
	}

	if len(headers) == 0 {
		return nil
	}
	return headers
}

func applyOpenAIAccountCustomHeaders(dst http.Header, account *Account) {
	if dst == nil {
		return
	}
	for key, values := range account.GetOpenAICustomHeaders() {
		if len(values) == 0 {
			continue
		}
		dst.Set(key, values[0])
	}
}

func getOpenAICustomAuthorization(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetOpenAICustomHeaders().Get("Authorization"))
}

func hasOpenAICustomAuthorization(account *Account) bool {
	return getOpenAICustomAuthorization(account) != ""
}

func addOpenAICustomHeaderRow(dst http.Header, raw any) {
	switch row := raw.(type) {
	case map[string]any:
		name := firstOpenAICustomHeaderRowString(row, "name", "key", "header")
		addOpenAICustomHeader(dst, name, row["value"])
	case map[string]string:
		name := firstOpenAICustomHeaderRowString(row, "name", "key", "header")
		addOpenAICustomHeader(dst, name, row["value"])
	}
}

func firstOpenAICustomHeaderRowString[T any](row map[string]T, keys ...string) string {
	for _, key := range keys {
		if raw, ok := row[key]; ok {
			return strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	return ""
}

func addOpenAICustomHeader(dst http.Header, rawName string, rawValue any) {
	name := strings.TrimSpace(rawName)
	value, ok := openAICustomHeaderValueString(rawValue)
	if !ok {
		return
	}
	if !isAllowedOpenAICustomHeader(name, value) {
		return
	}
	dst.Set(http.CanonicalHeaderKey(name), value)
}

func openAICustomHeaderValueString(raw any) (string, bool) {
	if raw == nil {
		return "", false
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v), true
	case json.Number:
		return strings.TrimSpace(v.String()), true
	case fmt.Stringer:
		return strings.TrimSpace(v.String()), true
	case bool, int, int64, float64:
		return strings.TrimSpace(fmt.Sprint(v)), true
	default:
		return "", false
	}
}

func isAllowedOpenAICustomHeader(name string, value string) bool {
	if name == "" || value == "" {
		return false
	}
	if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
		return false
	}
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	_, forbidden := openAICustomHeaderForbiddenNames[strings.ToLower(name)]
	return !forbidden
}
