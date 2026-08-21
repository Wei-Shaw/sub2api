package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// sanitizeGrokResponsesModelInput converts replay items from OpenAI and Chat
// shapes into the subset accepted by xAI's ModelInput decoder. Call IDs are
// assigned in a separate pass so an output can safely appear before its call.
func sanitizeGrokResponsesModelInput(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || input.Type == gjson.String {
		return body, nil
REDACTED
	if input.IsObject() {
		var err error
		body, err = sjson.SetRawBytes(body, "input", []byte("["+input.Raw+"]"))
		if err != nil {
			return nil, fmt.Errorf("wrap Grok Responses model input: %w", err)
	REDACTED
		input = gjson.GetBytes(body, "input")
REDACTED
	if !input.IsArray() {
		return body, nil
REDACTED

	var items []any
	decoder := json.NewDecoder(bytes.NewReader([]byte(input.Raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&items); err != nil {
		return nil, fmt.Errorf("parse Grok Responses model input: %w", err)
REDACTED
	callIDs, outputIDs := pairGrokReplayCallIDs(items)
	filtered := make([]any, 0, len(items))
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			if text, ok := rawItem.(string); ok && strings.TrimSpace(text) != "" {
				filtered = append(filtered, map[string]any{"type": "message", "role": "user", "content": textREDACTED)
		REDACTED
			continue
	REDACTED

		itemType := strings.ToLower(strings.TrimSpace(grokStringValue(item["type"])))
		role := strings.ToLower(strings.TrimSpace(grokStringValue(item["role"])))
		if role == "tool" || role == "function" || isGrokReplayOutputType(itemType) {
			output := firstNonNilGrokJSONValue(item["output"], item["content"], item["results"])
			filtered = append(filtered, map[string]any{
				"type":    "function_call_output",
				"call_id": outputIDs[index],
				"output":  grokModelInputString(output, "(empty)"),
		REDACTED)
			continue
	REDACTED

		switch itemType {
		case "text", "input_text", "output_text":
			text := strings.TrimSpace(grokStringValue(item["text"]))
			if text == "" {
				continue
		REDACTED
			if role == "" {
				role = "user"
		REDACTED
			item = map[string]any{"type": "message", "role": role, "content": textREDACTED
			itemType = "message"
		case "custom_tool_call", "tool_search_call":
			originalType := itemType
			item["type"] = "function_call"
			itemType = "function_call"
			if strings.TrimSpace(grokStringValue(item["name"])) == "" {
				if originalType == "tool_search_call" {
					item["name"] = "tool_search"
			REDACTED
		REDACTED
	REDACTED

		if itemType == "" && role != "" {
			itemType = "message"
			item["type"] = itemType
	REDACTED
		if itemType == "message" {
			if role == "" {
				role = "user"
				item["role"] = role
		REDACTED
			content, keep := sanitizeGrokMessageContent(item["content"])
			if !keep {
				continue
		REDACTED
			item["content"] = content
			if role == "assistant" && !grokIsCompleteOutputMessage(item) {
				if text, ok := collapseGrokAssistantOutputText(content); ok {
					item["content"] = text
			REDACTED
				delete(item, "id")
				delete(item, "status")
		REDACTED
	REDACTED else if itemType == "reasoning" {
			delete(item, "status")
			if content, exists := item["content"]; exists && content == nil {
				delete(item, "content")
		REDACTED
	REDACTED else if itemType == "function_call" {
			name := strings.TrimSpace(grokStringValue(item["name"]))
			arguments := item["arguments"]
			if function, ok := item["function"].(map[string]any); ok {
				if name == "" {
					name = strings.TrimSpace(grokStringValue(function["name"]))
			REDACTED
				if arguments == nil {
					arguments = function["arguments"]
			REDACTED
		REDACTED
			if arguments == nil {
				arguments = firstNonNilGrokJSONValue(item["input"], item["query"])
				if _, custom := item["input"]; custom {
					arguments = map[string]any{"input": argumentsREDACTED
			REDACTED
		REDACTED
			if name == "" {
				name = "unknown_tool"
		REDACTED
			item["call_id"] = callIDs[index]
			item["name"] = name
			item["arguments"] = grokModelInputString(arguments, "{REDACTED")
			for _, field := range []string{"id", "status", "tool_call_id", "function", "input", "query", "execution"REDACTED {
				delete(item, field)
		REDACTED
	REDACTED
		if shouldStripOpenAIResponsesNonPairCallID(itemType) {
			delete(item, "call_id")
	REDACTED
		filtered = append(filtered, item)
REDACTED

	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, fmt.Errorf("serialize Grok Responses model input: %w", err)
REDACTED
	updated, err := sjson.SetRawBytes(body, "input", encoded)
	if err != nil {
		return nil, fmt.Errorf("set Grok Responses model input: %w", err)
REDACTED
	return updated, nil
REDACTED

func pairGrokReplayCallIDs(items []any) (map[int]string, map[int]string) {
	callIDs := make(map[int]string)
	outputIDs := make(map[int]string)
	aliases := make(map[string]string)
	conflictingAliases := make(map[string]struct{REDACTED)
	pendingCalls := make([]string, 0)
	nextID := 0
	synthetic := func() string {
		nextID++
		return fmt.Sprintf("grok_replay_call_%d", nextID)
REDACTED
	registerAlias := func(alias, canonical string) {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			return
	REDACTED
		if _, conflict := conflictingAliases[alias]; conflict {
			return
	REDACTED
		if existing, exists := aliases[alias]; exists && existing != canonical {
			delete(aliases, alias)
			conflictingAliases[alias] = struct{REDACTED{REDACTED
			return
	REDACTED
		aliases[alias] = canonical
REDACTED

	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType := strings.ToLower(strings.TrimSpace(grokStringValue(item["type"])))
		if !isGrokReplayCallType(itemType) {
			continue
	REDACTED
		callID := firstNonEmptyGrokString(item["call_id"], item["tool_call_id"])
		if callID == "" {
			callID = synthetic()
	REDACTED
		callIDs[index] = callID
		pendingCalls = append(pendingCalls, callID)
		registerAlias(callID, callID)
		registerAlias(grokStringValue(item["call_id"]), callID)
		registerAlias(grokStringValue(item["tool_call_id"]), callID)
		registerAlias(grokStringValue(item["id"]), callID)
REDACTED

	consumedCalls := make(map[string]struct{REDACTED)
	consumeNextCall := func() string {
		for _, callID := range pendingCalls {
			if _, consumed := consumedCalls[callID]; consumed {
				continue
		REDACTED
			consumedCalls[callID] = struct{REDACTED{REDACTED
			return callID
	REDACTED
		return synthetic()
REDACTED
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType := strings.ToLower(strings.TrimSpace(grokStringValue(item["type"])))
		role := strings.ToLower(strings.TrimSpace(grokStringValue(item["role"])))
		if role != "tool" && role != "function" && !isGrokReplayOutputType(itemType) {
			continue
	REDACTED
		alias := firstNonEmptyGrokString(item["call_id"], item["tool_call_id"], item["id"])
		_, conflict := conflictingAliases[alias]
		if canonical := aliases[alias]; canonical != "" && !conflict {
			outputIDs[index] = canonical
			consumedCalls[canonical] = struct{REDACTED{REDACTED
	REDACTED else if firstNonEmptyGrokString(item["call_id"], item["tool_call_id"]) != "" {
			// Explicit call identifiers remain authoritative even when an invalid
			// transcript reuses the same identifier for more than one call.
			outputIDs[index] = alias
	REDACTED else if conflict {
			// An item ID shared by multiple calls cannot safely identify either
			// one. Keep it out of the call_id namespace and leave the output orphaned.
			outputIDs[index] = synthetic()
	REDACTED else {
			outputIDs[index] = consumeNextCall()
	REDACTED
REDACTED
	return callIDs, outputIDs
REDACTED

func isGrokReplayCallType(itemType string) bool {
	switch itemType {
	case "function_call", "custom_tool_call", "tool_search_call":
		return true
	default:
		return false
REDACTED
REDACTED

func isGrokReplayOutputType(itemType string) bool {
	switch itemType {
	case "function_call_output", "custom_tool_call_output", "tool_search_output", "tool_search_call_output":
		return true
	default:
		return false
REDACTED
REDACTED

func firstNonEmptyGrokString(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(grokStringValue(value)); text != "" {
			return text
	REDACTED
REDACTED
	return ""
REDACTED

func firstNonNilGrokJSONValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
	REDACTED
REDACTED
	return nil
REDACTED

func sanitizeGrokMessageContent(content any) (any, bool) {
	switch value := content.(type) {
	case nil:
		return nil, false
	case string:
		return value, strings.TrimSpace(value) != ""
	case []any:
		filtered := make([]any, 0, len(value))
		for _, rawPart := range value {
			part, ok := rawPart.(map[string]any)
			if !ok {
				if text, isText := rawPart.(string); !isText || strings.TrimSpace(text) != "" {
					filtered = append(filtered, rawPart)
			REDACTED
				continue
		REDACTED
			switch strings.ToLower(strings.TrimSpace(grokStringValue(part["type"]))) {
			case "text", "input_text", "output_text":
				if strings.TrimSpace(grokStringValue(part["text"])) == "" {
					continue
			REDACTED
			case "image_url", "input_image":
				if !grokContentPartHasImageURL(part) {
					continue
			REDACTED
		REDACTED
			filtered = append(filtered, part)
	REDACTED
		return filtered, len(filtered) > 0
	default:
		return content, true
REDACTED
REDACTED

func grokContentPartHasImageURL(part map[string]any) bool {
	for _, field := range []string{"file_id", "file_data"REDACTED {
		if strings.TrimSpace(grokStringValue(part[field])) != "" {
			return true
	REDACTED
REDACTED
	raw := firstNonNilGrokJSONValue(part["image_url"], part["file_url"])
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		return value != "" && !isEmptyBase64DataURI(value)
	case map[string]any:
		url := strings.TrimSpace(grokStringValue(value["url"]))
		return url != "" && !isEmptyBase64DataURI(url)
	default:
		return false
REDACTED
REDACTED

func grokStringValue(value any) string {
	text, _ := value.(string)
	return text
REDACTED

func grokModelInputString(value any, fallback string) string {
	if text, ok := value.(string); ok {
		if strings.TrimSpace(text) == "" {
			return fallback
	REDACTED
		return text
REDACTED
	if value == nil {
		return fallback
REDACTED
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) == 0 || string(encoded) == "null" {
		return fallback
REDACTED
	return string(encoded)
REDACTED

func grokIsCompleteOutputMessage(item map[string]any) bool {
	return strings.TrimSpace(grokStringValue(item["id"])) != "" &&
		strings.TrimSpace(grokStringValue(item["status"])) != ""
REDACTED

func collapseGrokAssistantOutputText(content any) (string, bool) {
	parts, ok := content.([]any)
	if !ok || len(parts) == 0 {
		return "", false
REDACTED
	var text strings.Builder
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok || strings.ToLower(strings.TrimSpace(grokStringValue(part["type"]))) != "output_text" {
			return "", false
	REDACTED
		value, ok := part["text"].(string)
		if !ok {
			return "", false
	REDACTED
		_, _ = text.WriteString(value)
REDACTED
	if strings.TrimSpace(text.String()) == "" {
		return "", false
REDACTED
	return text.String(), true
REDACTED
