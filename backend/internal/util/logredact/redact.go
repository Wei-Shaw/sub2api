package logredact

import (
	"encoding/json"
	"strings"
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
