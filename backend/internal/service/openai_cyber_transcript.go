package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

type openAICyberTranscriptBlockKeys struct {
	lookupKeys       []string
	preLatestUserKey string
REDACTED

// deriveOpenAICyberTranscriptBlockKeys returns cumulative semantic-history
// hashes plus the context key immediately before the latest user turn. The
// context key requires model-generated history so shared first-turn templates
// cannot block unrelated conversations.
func deriveOpenAICyberTranscriptBlockKeys(apiKeyID int64, body []byte) openAICyberTranscriptBlockKeys {
	if len(body) == 0 {
		return openAICyberTranscriptBlockKeys{REDACTED
REDACTED
	root := openAIRequestPayloadView(body)
	if !root.Exists() || !root.IsObject() {
		return openAICyberTranscriptBlockKeys{REDACTED
REDACTED

	h := sha256.New()
	_, _ = h.Write([]byte("cyber-transcript:v3|api_key="))
	_, _ = h.Write([]byte(strconv.FormatInt(apiKeyID, 10)))
	// Model and tool definitions are request configuration rather than history.
	// Root instructions remain model-visible context and participate in identity.
	for _, field := range []string{"instructions"REDACTED {
		v := root.Get(field)
		if !v.Exists() || (v.Type == gjson.String && strings.TrimSpace(v.String()) == "") {
			continue
	REDACTED
		canonical := v.Raw
		if v.Type == gjson.String {
			canonical = v.String()
	REDACTED else {
			canonical = normalizeCompatSeedJSON(json.RawMessage(v.Raw))
	REDACTED
		_, _ = h.Write([]byte("|"))
		_, _ = h.Write([]byte(field))
		_, _ = h.Write([]byte("="))
		_, _ = h.Write([]byte(canonical))
REDACTED

	appendSequence := func(sequence gjson.Result) openAICyberTranscriptBlockKeys {
		if !sequence.Exists() || !sequence.IsArray() {
			return openAICyberTranscriptBlockKeys{REDACTED
	REDACTED
		result := openAICyberTranscriptBlockKeys{
			lookupKeys: make([]string, 0, int(sequence.Get("#").Int())),
	REDACTED
		// This is an entropy heuristic, not provenance proof: authenticated
		// server-side history would be required to distinguish fixed few-shot
		// assistant items perfectly.
		hasModelGeneratedItem := false
		sequence.ForEach(func(_, item gjson.Result) bool {
			canonical := item.Raw
			if item.Type == gjson.String {
				encoded, _ := json.Marshal(item.String())
				canonical = string(encoded)
		REDACTED else if item.Type == gjson.JSON {
				canonical = normalizeCompatSeedJSON(json.RawMessage(item.Raw))
		REDACTED
			if strings.TrimSpace(canonical) == "" {
				return true
		REDACTED
			if openAICyberTranscriptItemStartsUserTurn(item) && hasModelGeneratedItem && len(result.lookupKeys) > 0 {
				result.preLatestUserKey = result.lookupKeys[len(result.lookupKeys)-1]
		REDACTED
			_, _ = h.Write([]byte("|item="))
			_, _ = h.Write([]byte(canonical))
			result.lookupKeys = append(result.lookupKeys, hex.EncodeToString(h.Sum(nil)))
			if openAICyberTranscriptItemIsModelGenerated(item) {
				hasModelGeneratedItem = true
		REDACTED
			return true
	REDACTED)
		return result
REDACTED

	if messages := root.Get("messages"); messages.Exists() {
		return appendSequence(messages)
REDACTED
	input := root.Get("input")
	if input.IsArray() {
		return appendSequence(input)
REDACTED
	if input.Type == gjson.String && strings.TrimSpace(input.String()) != "" {
		encoded, _ := json.Marshal(input.String())
		_, _ = h.Write([]byte("|item="))
		_, _ = h.Write(encoded)
		return openAICyberTranscriptBlockKeys{lookupKeys: []string{hex.EncodeToString(h.Sum(nil))REDACTEDREDACTED
REDACTED
	return openAICyberTranscriptBlockKeys{REDACTED
REDACTED

func openAICyberTranscriptItemStartsUserTurn(item gjson.Result) bool {
	if strings.EqualFold(strings.TrimSpace(item.Get("role").String()), "user") {
		content := item.Get("content")
		if content.IsArray() {
			hasUserContent := false
			content.ForEach(func(_, block gjson.Result) bool {
				switch strings.ToLower(strings.TrimSpace(block.Get("type").String())) {
				case "tool_result", "function_call_output", "custom_tool_call_output", "computer_call_output":
				default:
					hasUserContent = true
			REDACTED
				return !hasUserContent
		REDACTED)
			return hasUserContent
	REDACTED
		return content.Exists()
REDACTED
	return strings.EqualFold(strings.TrimSpace(item.Get("type").String()), "input_text")
REDACTED

func openAICyberTranscriptItemIsModelGenerated(item gjson.Result) bool {
	switch strings.ToLower(strings.TrimSpace(item.Get("role").String())) {
	case "assistant", "model":
		return true
REDACTED
	switch strings.ToLower(strings.TrimSpace(item.Get("type").String())) {
	case "output_text", "function_call", "tool_call", "custom_tool_call", "computer_call":
		return true
	default:
		return false
REDACTED
REDACTED
