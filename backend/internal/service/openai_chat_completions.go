package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ConvertChatCompletionsToResponses converts an OpenAI Chat Completions request to a Responses request.
func ConvertChatCompletionsToResponses(req map[string]any) (map[string]any, error) {
	if req == nil {
		return nil, errors.New("request is nil")
REDACTED

	model := strings.TrimSpace(getString(req["model"]))
	if model == "" {
		return nil, errors.New("model is required")
REDACTED

	messagesRaw, ok := req["messages"]
	if !ok {
		return nil, errors.New("messages is required")
REDACTED
	messages, ok := messagesRaw.([]any)
	if !ok {
		return nil, errors.New("messages must be an array")
REDACTED

	input, err := convertChatMessagesToResponsesInput(messages)
	if err != nil {
		return nil, err
REDACTED

	out := make(map[string]any, len(req)+1)
	for key, value := range req {
		switch key {
		case "messages", "max_tokens", "max_completion_tokens", "stream_options", "functions", "function_call":
			continue
		default:
			out[key] = value
	REDACTED
REDACTED

	out["model"] = model
	out["input"] = input

	if _, ok := out["max_output_tokens"]; !ok {
		if v, ok := req["max_tokens"]; ok {
			out["max_output_tokens"] = v
	REDACTED else if v, ok := req["max_completion_tokens"]; ok {
			out["max_output_tokens"] = v
	REDACTED
REDACTED

	if _, ok := out["tools"]; !ok {
		if functions, ok := req["functions"].([]any); ok && len(functions) > 0 {
			tools := make([]any, 0, len(functions))
			for _, fn := range functions {
				if fnMap, ok := fn.(map[string]any); ok {
					tools = append(tools, map[string]any{
						"type":     "function",
						"function": fnMap,
				REDACTED)
			REDACTED
		REDACTED
			if len(tools) > 0 {
				out["tools"] = tools
		REDACTED
	REDACTED
REDACTED

	if _, ok := out["tool_choice"]; !ok {
		if functionCall, ok := req["function_call"]; ok {
			out["tool_choice"] = functionCall
	REDACTED
REDACTED

	return out, nil
REDACTED

// ConvertResponsesToChatCompletion converts an OpenAI Responses response body to Chat Completions format.
func ConvertResponsesToChatCompletion(body []byte) ([]byte, error) {
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
REDACTED

	id := strings.TrimSpace(getString(resp["id"]))
	if id == "" {
		id = "chatcmpl-" + safeRandomHex(12)
REDACTED
	model := strings.TrimSpace(getString(resp["model"]))

	created := getInt64(resp["created_at"])
	if created == 0 {
		created = getInt64(resp["created"])
REDACTED
	if created == 0 {
		created = time.Now().Unix()
REDACTED

	text, toolCalls := extractResponseTextAndToolCalls(resp)
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
REDACTED

	message := map[string]any{
		"role":    "assistant",
		"content": text,
REDACTED
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
REDACTED

	chatResp := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
		REDACTED,
	REDACTED,
REDACTED

	if usage := extractResponseUsage(resp); usage != nil {
		chatResp["usage"] = usage
REDACTED
	if fingerprint := strings.TrimSpace(getString(resp["system_fingerprint"])); fingerprint != "" {
		chatResp["system_fingerprint"] = fingerprint
REDACTED

	return json.Marshal(chatResp)
REDACTED

func convertChatMessagesToResponsesInput(messages []any) ([]any, error) {
	input := make([]any, 0, len(messages))
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			return nil, errors.New("message must be an object")
	REDACTED
		role := strings.TrimSpace(getString(msgMap["role"]))
		if role == "" {
			return nil, errors.New("message role is required")
	REDACTED

		switch role {
		case "tool":
			callID := strings.TrimSpace(getString(msgMap["tool_call_id"]))
			if callID == "" {
				callID = strings.TrimSpace(getString(msgMap["id"]))
		REDACTED
			output := extractMessageContentText(msgMap["content"])
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
		REDACTED)
		case "function":
			callID := strings.TrimSpace(getString(msgMap["name"]))
			output := extractMessageContentText(msgMap["content"])
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
		REDACTED)
		default:
			convertedContent := convertChatContent(msgMap["content"])
			toolCalls := []any(nil)
			if role == "assistant" {
				toolCalls = extractToolCallsFromMessage(msgMap)
		REDACTED
			skipAssistantMessage := role == "assistant" && len(toolCalls) > 0 && isEmptyContent(convertedContent)
			if !skipAssistantMessage {
				msgItem := map[string]any{
					"role":    role,
					"content": convertedContent,
			REDACTED
				if name := strings.TrimSpace(getString(msgMap["name"])); name != "" {
					msgItem["name"] = name
			REDACTED
				input = append(input, msgItem)
		REDACTED
			if role == "assistant" && len(toolCalls) > 0 {
				input = append(input, toolCalls...)
		REDACTED
	REDACTED
REDACTED
	return input, nil
REDACTED

func convertChatContent(content any) any {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []any:
		converted := make([]any, 0, len(v))
		for _, part := range v {
			partMap, ok := part.(map[string]any)
			if !ok {
				converted = append(converted, part)
				continue
		REDACTED
			partType := strings.TrimSpace(getString(partMap["type"]))
			switch partType {
			case "text":
				text := getString(partMap["text"])
				if text != "" {
					converted = append(converted, map[string]any{
						"type": "input_text",
						"text": text,
				REDACTED)
					continue
			REDACTED
			case "image_url":
				imageURL := ""
				if imageObj, ok := partMap["image_url"].(map[string]any); ok {
					imageURL = getString(imageObj["url"])
			REDACTED else {
					imageURL = getString(partMap["image_url"])
			REDACTED
				if imageURL != "" {
					converted = append(converted, map[string]any{
						"type":      "input_image",
						"image_url": imageURL,
				REDACTED)
					continue
			REDACTED
			case "input_text", "input_image":
				converted = append(converted, partMap)
				continue
		REDACTED
			converted = append(converted, partMap)
	REDACTED
		return converted
	default:
		return v
REDACTED
REDACTED

func extractToolCallsFromMessage(msg map[string]any) []any {
	var out []any
	if toolCalls, ok := msg["tool_calls"].([]any); ok {
		for _, call := range toolCalls {
			callMap, ok := call.(map[string]any)
			if !ok {
				continue
		REDACTED
			callID := strings.TrimSpace(getString(callMap["id"]))
			if callID == "" {
				callID = strings.TrimSpace(getString(callMap["call_id"]))
		REDACTED
			name := ""
			args := ""
			if fn, ok := callMap["function"].(map[string]any); ok {
				name = strings.TrimSpace(getString(fn["name"]))
				args = getString(fn["arguments"])
		REDACTED
			if name == "" && args == "" {
				continue
		REDACTED
			item := map[string]any{
				"type": "tool_call",
		REDACTED
			if callID != "" {
				item["call_id"] = callID
		REDACTED
			if name != "" {
				item["name"] = name
		REDACTED
			if args != "" {
				item["arguments"] = args
		REDACTED
			out = append(out, item)
	REDACTED
REDACTED

	if fnCall, ok := msg["function_call"].(map[string]any); ok {
		name := strings.TrimSpace(getString(fnCall["name"]))
		args := getString(fnCall["arguments"])
		if name != "" || args != "" {
			callID := strings.TrimSpace(getString(msg["tool_call_id"]))
			if callID == "" {
				callID = name
		REDACTED
			item := map[string]any{
				"type": "function_call",
		REDACTED
			if callID != "" {
				item["call_id"] = callID
		REDACTED
			if name != "" {
				item["name"] = name
		REDACTED
			if args != "" {
				item["arguments"] = args
		REDACTED
			out = append(out, item)
	REDACTED
REDACTED

	return out
REDACTED

func extractMessageContentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, part := range v {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
		REDACTED
			partType := strings.TrimSpace(getString(partMap["type"]))
			if partType == "" || partType == "text" || partType == "output_text" || partType == "input_text" {
				text := getString(partMap["text"])
				if text != "" {
					parts = append(parts, text)
			REDACTED
		REDACTED
	REDACTED
		return strings.Join(parts, "")
	default:
		return ""
REDACTED
REDACTED

func isEmptyContent(content any) bool {
	switch v := content.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []any:
		return len(v) == 0
	default:
		return false
REDACTED
REDACTED

func extractResponseTextAndToolCalls(resp map[string]any) (string, []any) {
	output, ok := resp["output"].([]any)
	if !ok {
		if text, ok := resp["output_text"].(string); ok {
			return text, nil
	REDACTED
		return "", nil
REDACTED

	textParts := make([]string, 0)
	toolCalls := make([]any, 0)

	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType := strings.TrimSpace(getString(itemMap["type"]))

		if itemType == "tool_call" || itemType == "function_call" {
			if tc := responseItemToChatToolCall(itemMap); tc != nil {
				toolCalls = append(toolCalls, tc)
		REDACTED
			continue
	REDACTED

		content := itemMap["content"]
		switch v := content.(type) {
		case string:
			if v != "" {
				textParts = append(textParts, v)
		REDACTED
		case []any:
			for _, part := range v {
				partMap, ok := part.(map[string]any)
				if !ok {
					continue
			REDACTED
				partType := strings.TrimSpace(getString(partMap["type"]))
				switch partType {
				case "output_text", "text", "input_text":
					text := getString(partMap["text"])
					if text != "" {
						textParts = append(textParts, text)
				REDACTED
				case "tool_call", "function_call":
					if tc := responseItemToChatToolCall(partMap); tc != nil {
						toolCalls = append(toolCalls, tc)
				REDACTED
			REDACTED
		REDACTED
	REDACTED
REDACTED

	return strings.Join(textParts, ""), toolCalls
REDACTED

func responseItemToChatToolCall(item map[string]any) map[string]any {
	callID := strings.TrimSpace(getString(item["call_id"]))
	if callID == "" {
		callID = strings.TrimSpace(getString(item["id"]))
REDACTED
	name := strings.TrimSpace(getString(item["name"]))
	arguments := getString(item["arguments"])
	if fn, ok := item["function"].(map[string]any); ok {
		if name == "" {
			name = strings.TrimSpace(getString(fn["name"]))
	REDACTED
		if arguments == "" {
			arguments = getString(fn["arguments"])
	REDACTED
REDACTED

	if name == "" && arguments == "" && callID == "" {
		return nil
REDACTED

	if callID == "" {
		callID = "call_" + safeRandomHex(6)
REDACTED

	return map[string]any{
		"id":   callID,
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": arguments,
	REDACTED,
REDACTED
REDACTED

func extractResponseUsage(resp map[string]any) map[string]any {
	usage, ok := resp["usage"].(map[string]any)
	if !ok {
		return nil
REDACTED
	promptTokens := int(getNumber(usage["input_tokens"]))
	completionTokens := int(getNumber(usage["output_tokens"]))
	if promptTokens == 0 && completionTokens == 0 {
		return nil
REDACTED

	return map[string]any{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      promptTokens + completionTokens,
REDACTED
REDACTED

func getString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case json.Number:
		return v.String()
	default:
		return ""
REDACTED
REDACTED

func getNumber(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
REDACTED
REDACTED

func getInt64(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	default:
		return 0
REDACTED
REDACTED

func safeRandomHex(byteLength int) string {
	value, err := randomHexString(byteLength)
	if err != nil || value == "" {
		return "000000"
REDACTED
	return value
REDACTED
