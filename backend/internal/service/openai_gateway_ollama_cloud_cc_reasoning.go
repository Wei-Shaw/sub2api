package service

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Ollama Cloud 的 OpenAI 兼容 /v1/chat/completions 把思维放在 reasoning / thinking，
// 而 DeepSeek/OpenAI 客户端只认 reasoning_content。仅在 raw CC 直转路径上做 wire JSON
// 双向补齐，不改 CC↔Responses / Anthropic / Grok 桥。

func isOllamaCloudRawChatCompletionsAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
		return false
REDACTED
	mode, _ := account.Extra[openai_compat.ExtraKeyResponsesMode].(string)
	if openai_compat.NormalizeResponsesSupportMode(mode) != openai_compat.ResponsesSupportModeForceChatCompletions {
		return false
REDACTED
	if accountHasOllamaCloudUsageExtra(account) {
		return true
REDACTED
	if account.Credentials == nil {
		return false
REDACTED
	baseURL, _ := account.Credentials["base_url"].(string)
	return isOllamaCloudBaseURL(baseURL)
REDACTED

func accountHasOllamaCloudUsageExtra(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
REDACTED
	for _, key := range []string{
		OllamaCloudUsageSessionExtraKey,
		OllamaCloudUsageAutoRefreshExtraKey,
		OllamaCloudUsageSnapshotExtraKey,
REDACTED {
		if _, ok := account.Extra[key]; ok {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func applyOllamaCloudRawChatCompletionsRequest(account *Account, body []byte) []byte {
	if !isOllamaCloudRawChatCompletionsAccount(account) || len(body) == 0 {
		return body
REDACTED
	return normalizeOllamaCloudChatCompletionsRequest(body)
REDACTED

func applyOllamaCloudRawChatCompletionsResponse(account *Account, body []byte) []byte {
	if !isOllamaCloudRawChatCompletionsAccount(account) || len(body) == 0 {
		return body
REDACTED
	return normalizeOllamaCloudChatCompletionsResponseJSON(body)
REDACTED

func applyOllamaCloudRawChatCompletionsSSELine(account *Account, line string) string {
	if !isOllamaCloudRawChatCompletionsAccount(account) || line == "" {
		return line
REDACTED
	return normalizeOllamaCloudChatCompletionsSSELine(line)
REDACTED

func normalizeOllamaCloudChatCompletionsRequest(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
REDACTED
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
REDACTED
	updated := body
	changed := false
	for i, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
	REDACTED
		reasoningContent, ok := jsonNonEmptyString(msg.Get("reasoning_content"))
		if !ok {
			continue
	REDACTED
		if _, has := jsonNonEmptyString(msg.Get("reasoning")); has {
			continue
	REDACTED
		if _, has := jsonNonEmptyString(msg.Get("thinking")); has {
			continue
	REDACTED
		next, err := sjson.SetBytes(updated, "messages."+strconv.Itoa(i)+".reasoning", reasoningContent)
		if err != nil {
			return body
	REDACTED
		updated = next
		changed = true
REDACTED
	if !changed {
		return body
REDACTED
	return updated
REDACTED

func normalizeOllamaCloudChatCompletionsResponseJSON(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
REDACTED
	choices := gjson.GetBytes(body, "choices")
	if !choices.IsArray() {
		return body
REDACTED
	updated := body
	changed := false
	for i, choice := range choices.Array() {
		for _, container := range []string{"message", "delta"REDACTED {
			obj := choice.Get(container)
			if !obj.Exists() || !obj.IsObject() {
				continue
		REDACTED
			if obj.Get("reasoning_content").Exists() {
				continue
		REDACTED
			src, ok := jsonNonEmptyString(obj.Get("reasoning"))
			if !ok {
				src, ok = jsonNonEmptyString(obj.Get("thinking"))
		REDACTED
			if !ok {
				continue
		REDACTED
			next, err := sjson.SetBytes(updated, "choices."+strconv.Itoa(i)+"."+container+".reasoning_content", src)
			if err != nil {
				return body
		REDACTED
			updated = next
			changed = true
	REDACTED
REDACTED
	if !changed {
		return body
REDACTED
	return updated
REDACTED

func normalizeOllamaCloudChatCompletionsSSELine(line string) string {
	payload, ok := extractOpenAISSEDataLine(line)
	if !ok {
		return line
REDACTED
	trimmed := strings.TrimSpace(payload)
	if trimmed == "" || trimmed == "[DONE]" {
		return line
REDACTED
	rewritten := normalizeOllamaCloudChatCompletionsResponseJSON([]byte(payload))
	if string(rewritten) == payload {
		return line
REDACTED
	prefixLen := len(line) - len(payload)
	if prefixLen < 0 {
		return line
REDACTED
	return line[:prefixLen] + string(rewritten)
REDACTED

func jsonNonEmptyString(v gjson.Result) (string, bool) {
	if v.Type != gjson.String || v.Str == "" {
		return "", false
REDACTED
	return v.Str, true
REDACTED
