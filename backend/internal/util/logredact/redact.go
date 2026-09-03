package logredact

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// maxRedactDepth 限制递归深度以防止栈溢出
const maxRedactDepth = 32

// defaultSensitiveKeyList 是默认脱敏的字段名。
//
// 匹配语义（两处必须一起理解，否则容易误判覆盖面）：
//   - RedactMap / RedactJSON 走 isSensitiveKey，是「整键精确匹配」，
//     仅做 ToLower+TrimSpace，不做子串匹配、也不做 camelCase→snake_case 归一化。
//     所以 camelCase 配置键（wxpay 的 apiV3Key/privateKey）必须以其小写形式
//     单独列出（apiv3key/privatekey），否则不会命中。
//   - RedactText 走 buildKeyAlternation 拼出的正则，两侧都有 \b 词边界。
//     Go regexp 里 `_` 属于词字符，因此 \bapi_key\b 不会命中 api_key_id、
//     \bapikey\b 不会命中 apikey_id —— 业务 ID 字段仍然可见，不会被过度脱敏。
//
// 新增条目请只放"值本身即凭据"的字段名，绝不要放裸 key/id/name 这类通用词。
var defaultSensitiveKeyList = []string{
	// OAuth 簇
	"authorization_code",
	"code",
	"code_verifier",
	"access_token",
	"refresh_token",
	"id_token",
	"client_secret",
	"password",
	// 通用密钥簇：snake_case 与 camelCase 小写形式都要列，见上文匹配语义说明
	"secret_key",
	"secretkey",
	"api_key",
	"apikey",
	"private_key",
	"privatekey",
	"api_v3_key",
	"apiv3key",
	"session_key",
	"sessionkey",
	// 会话凭据：Cookie / Set-Cookie 的值等价于一次完整的身份接管
	"cookie",
	"set-cookie",
}

// defaultSensitiveKeys 由 defaultSensitiveKeyList 派生，避免两份清单各改各的而漂移。
var defaultSensitiveKeys = func() map[string]struct{} {
	keys := make(map[string]struct{}, len(defaultSensitiveKeyList))
	for _, k := range defaultSensitiveKeyList {
		keys[k] = struct{}{}
	}
	return keys
}()

type textRedactPatterns struct {
	reJSONLike  *regexp.Regexp
	reQueryLike *regexp.Regexp
	rePlain     *regexp.Regexp
}

var (
	reGOCSPX = regexp.MustCompile(`GOCSPX-[0-9A-Za-z_-]{24,}`)
	reAIza   = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)

	defaultTextRedactPatterns = compileTextRedactPatterns(nil)
	extraTextPatternCache     sync.Map // map[string]*textRedactPatterns
)

func RedactMap(input map[string]any, extraKeys ...string) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	keys := buildKeySet(extraKeys)
	redacted, ok := redactValueWithDepth(input, keys, 0).(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return redacted
}

func RedactJSON(raw []byte, extraKeys ...string) string {
	if len(raw) == 0 {
		return ""
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "<non-json payload redacted>"
	}
	keys := buildKeySet(extraKeys)
	redacted := redactValueWithDepth(value, keys, 0)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "<redacted>"
	}
	return string(encoded)
}

// RedactText 对非结构化文本做轻量脱敏。
//
// 规则：
// - 如果文本本身是 JSON，则按 RedactJSON 处理。
// - 否则尝试对常见 key=value / key:"value" 片段做脱敏。
//
// 注意：该函数用于日志/错误信息兜底，不保证覆盖所有格式。
func RedactText(input string, extraKeys ...string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	raw := []byte(input)
	if json.Valid(raw) {
		return RedactJSON(raw, extraKeys...)
	}

	patterns := getTextRedactPatterns(extraKeys)

	out := input
	out = reGOCSPX.ReplaceAllString(out, "GOCSPX-***")
	out = reAIza.ReplaceAllString(out, "AIza***")
	out = patterns.reJSONLike.ReplaceAllString(out, `$1***$3`)
	out = patterns.reQueryLike.ReplaceAllString(out, `$1=***`)
	out = patterns.rePlain.ReplaceAllString(out, `$1$2***`)
	return out
}

func compileTextRedactPatterns(extraKeys []string) *textRedactPatterns {
	keyAlt := buildKeyAlternation(extraKeys)
	return &textRedactPatterns{
		// JSON-like: "access_token":"..."
		reJSONLike: regexp.MustCompile(`(?i)("(?:` + keyAlt + `)"\s*:\s*")([^"]*)(")`),
		// Query-like: access_token=...
		reQueryLike: regexp.MustCompile(`(?i)\b((?:` + keyAlt + `))=([^&\s]+)`),
		// Plain: access_token: ... / access_token = ...
		rePlain: regexp.MustCompile(`(?i)\b((?:` + keyAlt + `))\b(\s*[:=]\s*)([^,\s]+)`),
	}
}

func getTextRedactPatterns(extraKeys []string) *textRedactPatterns {
	normalizedExtraKeys := normalizeAndSortExtraKeys(extraKeys)
	if len(normalizedExtraKeys) == 0 {
		return defaultTextRedactPatterns
	}

	cacheKey := strings.Join(normalizedExtraKeys, ",")
	if cached, ok := extraTextPatternCache.Load(cacheKey); ok {
		if patterns, ok := cached.(*textRedactPatterns); ok {
			return patterns
		}
	}

	compiled := compileTextRedactPatterns(normalizedExtraKeys)
	actual, _ := extraTextPatternCache.LoadOrStore(cacheKey, compiled)
	if patterns, ok := actual.(*textRedactPatterns); ok {
		return patterns
	}
	return compiled
}

func normalizeAndSortExtraKeys(extraKeys []string) []string {
	if len(extraKeys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(extraKeys))
	keys := make([]string, 0, len(extraKeys))
	for _, key := range extraKeys {
		normalized := normalizeKey(key)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		keys = append(keys, normalized)
	}
	sort.Strings(keys)
	return keys
}

func buildKeyAlternation(extraKeys []string) string {
	seen := make(map[string]struct{}, len(defaultSensitiveKeyList)+len(extraKeys))
	keys := make([]string, 0, len(defaultSensitiveKeyList)+len(extraKeys))
	for _, k := range defaultSensitiveKeyList {
		seen[k] = struct{}{}
		keys = append(keys, regexp.QuoteMeta(k))
	}
	for _, k := range extraKeys {
		n := normalizeKey(k)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		keys = append(keys, regexp.QuoteMeta(n))
	}
	return strings.Join(keys, "|")
}

func buildKeySet(extraKeys []string) map[string]struct{} {
	keys := make(map[string]struct{}, len(defaultSensitiveKeys)+len(extraKeys))
	for k := range defaultSensitiveKeys {
		keys[k] = struct{}{}
	}
	for _, key := range extraKeys {
		normalized := normalizeKey(key)
		if normalized == "" {
			continue
		}
		keys[normalized] = struct{}{}
	}
	return keys
}

func redactValueWithDepth(value any, keys map[string]struct{}, depth int) any {
	if depth > maxRedactDepth {
		return "<depth limit exceeded>"
	}

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			if isSensitiveKey(k, keys) {
				out[k] = "***"
				continue
			}
			out[k] = redactValueWithDepth(val, keys, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValueWithDepth(item, keys, depth+1)
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string, keys map[string]struct{}) bool {
	_, ok := keys[normalizeKey(key)]
	return ok
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
