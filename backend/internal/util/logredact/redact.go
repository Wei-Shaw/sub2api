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

var defaultSensitiveKeys = map[string]struct{REDACTED{
	"authorization_code": {REDACTED,
	"code":               {REDACTED,
	"code_verifier":      {REDACTED,
	"access_token":       {REDACTED,
	"refresh_token":      {REDACTED,
	"id_token":           {REDACTED,
	"client_secret":      {REDACTED,
	"password":           {REDACTED,
REDACTED

var defaultSensitiveKeyList = []string{
	"authorization_code",
	"code",
	"code_verifier",
	"access_token",
	"refresh_token",
	"id_token",
	"client_secret",
	"password",
REDACTED

type textRedactPatterns struct {
	reJSONLike  *regexp.Regexp
	reQueryLike *regexp.Regexp
	rePlain     *regexp.Regexp
REDACTED

var (
	reGOCSPX = regexp.MustCompile(`GOCSPX-[0-9A-Za-z_-]{24,REDACTED`)
	reAIza   = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35REDACTED`)

	defaultTextRedactPatterns = compileTextRedactPatterns(nil)
	extraTextPatternCache     sync.Map // map[string]*textRedactPatterns
)

func RedactMap(input map[string]any, extraKeys ...string) map[string]any {
	if input == nil {
		return map[string]any{REDACTED
REDACTED
	keys := buildKeySet(extraKeys)
	redacted, ok := redactValueWithDepth(input, keys, 0).(map[string]any)
	if !ok {
		return map[string]any{REDACTED
REDACTED
	return redacted
REDACTED

func RedactJSON(raw []byte, extraKeys ...string) string {
	if len(raw) == 0 {
		return ""
REDACTED
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "<non-json payload redacted>"
REDACTED
	keys := buildKeySet(extraKeys)
	redacted := redactValueWithDepth(value, keys, 0)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return "<redacted>"
REDACTED
	return string(encoded)
REDACTED

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
REDACTED

	raw := []byte(input)
	if json.Valid(raw) {
		return RedactJSON(raw, extraKeys...)
REDACTED

	patterns := getTextRedactPatterns(extraKeys)

	out := input
	out = reGOCSPX.ReplaceAllString(out, "GOCSPX-***")
	out = reAIza.ReplaceAllString(out, "AIza***")
	out = patterns.reJSONLike.ReplaceAllString(out, `$1***$3`)
	out = patterns.reQueryLike.ReplaceAllString(out, `$1=***`)
	out = patterns.rePlain.ReplaceAllString(out, `$1$2***`)
	return out
REDACTED

func compileTextRedactPatterns(extraKeys []string) *textRedactPatterns {
	keyAlt := buildKeyAlternation(extraKeys)
	return &textRedactPatterns{
		// JSON-like: "access_token":"..."
		reJSONLike: regexp.MustCompile(`(?i)("(?:` + keyAlt + `)"\s*:\s*")([^"]*)(")`),
		// Query-like: access_token=...
		reQueryLike: regexp.MustCompile(`(?i)\b((?:` + keyAlt + `))=([^&\s]+)`),
		// Plain: access_token: ... / access_token = ...
		rePlain: regexp.MustCompile(`(?i)\b((?:` + keyAlt + `))\b(\s*[:=]\s*)([^,\s]+)`),
REDACTED
REDACTED

func getTextRedactPatterns(extraKeys []string) *textRedactPatterns {
	normalizedExtraKeys := normalizeAndSortExtraKeys(extraKeys)
	if len(normalizedExtraKeys) == 0 {
		return defaultTextRedactPatterns
REDACTED

	cacheKey := strings.Join(normalizedExtraKeys, ",")
	if cached, ok := extraTextPatternCache.Load(cacheKey); ok {
		if patterns, ok := cached.(*textRedactPatterns); ok {
			return patterns
	REDACTED
REDACTED

	compiled := compileTextRedactPatterns(normalizedExtraKeys)
	actual, _ := extraTextPatternCache.LoadOrStore(cacheKey, compiled)
	if patterns, ok := actual.(*textRedactPatterns); ok {
		return patterns
REDACTED
	return compiled
REDACTED

func normalizeAndSortExtraKeys(extraKeys []string) []string {
	if len(extraKeys) == 0 {
		return nil
REDACTED
	seen := make(map[string]struct{REDACTED, len(extraKeys))
	keys := make([]string, 0, len(extraKeys))
	for _, key := range extraKeys {
		normalized := normalizeKey(key)
		if normalized == "" {
			continue
	REDACTED
		if _, ok := seen[normalized]; ok {
			continue
	REDACTED
		seen[normalized] = struct{REDACTED{REDACTED
		keys = append(keys, normalized)
REDACTED
	sort.Strings(keys)
	return keys
REDACTED

func buildKeyAlternation(extraKeys []string) string {
	seen := make(map[string]struct{REDACTED, len(defaultSensitiveKeyList)+len(extraKeys))
	keys := make([]string, 0, len(defaultSensitiveKeyList)+len(extraKeys))
	for _, k := range defaultSensitiveKeyList {
		seen[k] = struct{REDACTED{REDACTED
		keys = append(keys, regexp.QuoteMeta(k))
REDACTED
	for _, k := range extraKeys {
		n := normalizeKey(k)
		if n == "" {
			continue
	REDACTED
		if _, ok := seen[n]; ok {
			continue
	REDACTED
		seen[n] = struct{REDACTED{REDACTED
		keys = append(keys, regexp.QuoteMeta(n))
REDACTED
	return strings.Join(keys, "|")
REDACTED

func buildKeySet(extraKeys []string) map[string]struct{REDACTED {
	keys := make(map[string]struct{REDACTED, len(defaultSensitiveKeys)+len(extraKeys))
	for k := range defaultSensitiveKeys {
		keys[k] = struct{REDACTED{REDACTED
REDACTED
	for _, key := range extraKeys {
		normalized := normalizeKey(key)
		if normalized == "" {
			continue
	REDACTED
		keys[normalized] = struct{REDACTED{REDACTED
REDACTED
	return keys
REDACTED

func redactValueWithDepth(value any, keys map[string]struct{REDACTED, depth int) any {
	if depth > maxRedactDepth {
		return "<depth limit exceeded>"
REDACTED

	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			if isSensitiveKey(k, keys) {
				out[k] = "***"
				continue
		REDACTED
			out[k] = redactValueWithDepth(val, keys, depth+1)
	REDACTED
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValueWithDepth(item, keys, depth+1)
	REDACTED
		return out
	default:
		return value
REDACTED
REDACTED

func isSensitiveKey(key string, keys map[string]struct{REDACTED) bool {
	_, ok := keys[normalizeKey(key)]
	return ok
REDACTED

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
REDACTED
