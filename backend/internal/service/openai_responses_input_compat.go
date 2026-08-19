package service

import (
	"bytes"
	"strings"
)

const openAIResponsesInputTextMaxChars = 10000000

// sanitizeOpenAIResponsesOrphanToolOutputs removes tool-output items that have
// no matching call or item reference anywhere in the current input.
func sanitizeOpenAIResponsesOrphanToolOutputs(reqBody map[string]any, input []any, hasPreviousResponseID bool) bool {
	if len(input) == 0 || hasPreviousResponseID {
		return false
REDACTED

	toolCallIDs := make(map[string]struct{REDACTED, len(input))
	referenceIDs := make(map[string]struct{REDACTED, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if itemType == "item_reference" {
			if id := strings.TrimSpace(firstNonEmptyString(item["id"])); id != "" {
				referenceIDs[id] = struct{REDACTED{REDACTED
		REDACTED
			continue
	REDACTED
		if !isCodexToolCallContextItemType(itemType) {
			continue
	REDACTED
		if id := strings.TrimSpace(firstNonEmptyString(item["call_id"], item["id"])); id != "" {
			toolCallIDs[id] = struct{REDACTED{REDACTED
	REDACTED
REDACTED

	modified := false
	normalized := make([]any, 0, len(input))
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || !isCodexToolCallOutputItemType(strings.TrimSpace(firstNonEmptyString(item["type"]))) {
			normalized = append(normalized, rawItem)
			continue
	REDACTED

		callID := strings.TrimSpace(firstNonEmptyString(item["call_id"]))
		_, hasToolCall := toolCallIDs[callID]
		_, hasReference := referenceIDs[callID]
		if callID != "" && (hasToolCall || hasReference) {
			normalized = append(normalized, rawItem)
			continue
	REDACTED

		modified = true
REDACTED
	if !modified {
		return false
REDACTED
	reqBody["input"] = normalized
	return true
REDACTED

func truncateOpenAIResponsesInputText(reqBody map[string]any) bool {
	input, ok := reqBody["input"].([]any)
	if !ok || len(input) == 0 {
		return false
REDACTED
	modified := false
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
	REDACTED
		itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
		if isCodexToolCallOutputItemType(itemType) {
			if output, ok := item["output"].(string); ok {
				if truncated, changed := truncateOpenAIResponsesInputString(output); changed {
					item["output"] = truncated
					modified = true
			REDACTED
		REDACTED
	REDACTED
		if truncateOpenAIResponsesMessageText(item) {
			modified = true
	REDACTED
REDACTED
	return modified
REDACTED

func openAIResponsesInputMayNeedTruncation(body []byte) bool {
	if len(body) <= openAIResponsesInputTextMaxChars {
		return false
REDACTED
	if bytes.Contains(body, []byte(`"text"`)) && bytes.Contains(body, []byte(`"content"`)) {
		return true
REDACTED
	for _, itemType := range []string{
		"function_call_output",
		"tool_search_output",
		"custom_tool_call_output",
		"mcp_tool_call_output",
REDACTED {
		if bytes.Contains(body, []byte(itemType)) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func truncateOpenAIResponsesMessageText(item map[string]any) bool {
	itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
	role := strings.TrimSpace(firstNonEmptyString(item["role"]))
	if itemType != "message" && role == "" {
		return false
REDACTED
	parts, ok := item["content"].([]any)
	if !ok {
		return false
REDACTED
	modified := false
	for _, rawPart := range parts {
		part, ok := rawPart.(map[string]any)
		if !ok {
			continue
	REDACTED
		text, ok := part["text"].(string)
		if !ok {
			continue
	REDACTED
		if truncated, changed := truncateOpenAIResponsesInputString(text); changed {
			part["text"] = truncated
			modified = true
	REDACTED
REDACTED
	return modified
REDACTED

func truncateOpenAIResponsesInputString(value string) (string, bool) {
	if len(value) <= openAIResponsesInputTextMaxChars {
		return value, false
REDACTED
	chars := 0
	for index := range value {
		if chars == openAIResponsesInputTextMaxChars {
			return value[:index], true
	REDACTED
		chars++
REDACTED
	return value, false
REDACTED
