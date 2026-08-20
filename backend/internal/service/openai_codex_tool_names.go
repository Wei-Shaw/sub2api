package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	codexReservedPythonToolName = "python"
	codexPythonToolAlias        = "python__sub2api"
	codexToolNameReverseKey     = "openai_codex_tool_name_reverse"
	codexToolNameSessionKey     = "openai_codex_tool_name_session_reverse"
)

type codexToolNameField struct {
	object map[string]any
	key    string
	name   string
REDACTED

// aliasOpenAIOAuthReservedToolNames avoids names reserved by the ChatGPT
// Codex backend. It validates every declaration/reference before mutating so
// collisions cannot leave a partially rewritten request.
func aliasOpenAIOAuthReservedToolNames(reqBody map[string]any) (map[string]string, bool, error) {
	if reqBody == nil {
		return nil, false, nil
REDACTED

	fields := collectOpenAIResponsesToolNameFields(reqBody)
	owners := make(map[string]string)
	reverse := make(map[string]string)
	for _, field := range fields {
		normalized := aliasOpenAIOAuthReservedToolName(field.name)
		original := field.name
		if normalized != field.name {
			original = strings.TrimSpace(field.name)
	REDACTED
		if previous, exists := owners[normalized]; exists && previous != original {
			return nil, false, fmt.Errorf("tool names %q and %q both normalize to %q", previous, original, normalized)
	REDACTED
		owners[normalized] = original
		if normalized != field.name {
			reverse[normalized] = original
	REDACTED
REDACTED
	if len(reverse) == 0 {
		return nil, false, nil
REDACTED
	for _, field := range fields {
		if aliased := aliasOpenAIOAuthReservedToolName(field.name); aliased != field.name {
			field.object[field.key] = aliased
	REDACTED
REDACTED
	return reverse, true, nil
REDACTED

func aliasOpenAIOAuthReservedToolName(name string) string {
	trimmed := strings.TrimSpace(name)
	if strings.EqualFold(trimmed, codexReservedPythonToolName) {
		return codexPythonToolAlias
REDACTED
	return name
REDACTED

func collectOpenAIResponsesToolNameFields(reqBody map[string]any) []codexToolNameField {
	fields := make([]codexToolNameField, 0, 8)
	appendName := func(object map[string]any, key string) {
		if object == nil {
			return
	REDACTED
		name, ok := object[key].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return
	REDACTED
		fields = append(fields, codexToolNameField{object: object, key: key, name: nameREDACTED)
REDACTED
	var collectTools func(any)
	collectTools = func(rawTools any) {
		tools, ok := rawTools.([]any)
		if !ok {
			return
	REDACTED
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok {
				continue
		REDACTED
			toolType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(tool["type"])))
			if toolType == "function" {
				appendName(tool, "name")
				if function, ok := tool["function"].(map[string]any); ok {
					appendName(function, "name")
			REDACTED
		REDACTED
			if toolType == "namespace" {
				collectTools(tool["tools"])
		REDACTED
	REDACTED
REDACTED
	collectTools(reqBody["tools"])
	if functions, ok := reqBody["functions"].([]any); ok {
		for _, raw := range functions {
			function, _ := raw.(map[string]any)
			appendName(function, "name")
	REDACTED
REDACTED
	if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(reqBody["type"])), "session.update") {
		if session, ok := reqBody["session"].(map[string]any); ok {
			collectTools(session["tools"])
	REDACTED
REDACTED
	if choice, ok := reqBody["tool_choice"].(map[string]any); ok {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(choice["type"])), "function") {
			appendName(choice, "name")
			if function, ok := choice["function"].(map[string]any); ok {
				appendName(function, "name")
		REDACTED
	REDACTED
REDACTED
	if input, ok := reqBody["input"].([]any); ok {
		for _, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
		REDACTED
			typ := strings.ToLower(strings.TrimSpace(firstNonEmptyString(item["type"])))
			if typ == "additional_tools" {
				collectTools(item["tools"])
		REDACTED
			if typ == "function_call" {
				appendName(item, "name")
				if function, ok := item["function"].(map[string]any); ok {
					appendName(function, "name")
			REDACTED
		REDACTED
	REDACTED
REDACTED
	return fields
REDACTED

func aliasOpenAIOAuthReservedToolNamesBody(body []byte) ([]byte, map[string]string, bool, error) {
	if len(body) == 0 || !containsASCIIFold(body, []byte(codexReservedPythonToolName)) {
		return body, nil, false, nil
REDACTED
	var reqBody map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &reqBody); err != nil {
		return body, nil, false, fmt.Errorf("decode OAuth reserved tool names: %w", err)
REDACTED
	reverse, changed, err := aliasOpenAIOAuthReservedToolNames(reqBody)
	if err != nil || !changed {
		return body, reverse, false, err
REDACTED
	normalized, err := json.Marshal(reqBody)
	if err != nil {
		return body, nil, false, fmt.Errorf("encode OAuth reserved tool names: %w", err)
REDACTED
	return normalized, reverse, true, nil
REDACTED

func containsASCIIFold(haystack, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return false
REDACTED
	for i := 0; i <= len(haystack)-len(needle); i++ {
		matched := true
		for j := range needle {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
		REDACTED
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
		REDACTED
			if a != b {
				matched = false
				break
		REDACTED
	REDACTED
		if matched {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func setCodexToolNameReverse(c *gin.Context, reverse map[string]string) {
	if c == nil {
		return
REDACTED
	storeCodexToolNameReverse(c, codexToolNameReverseKey, reverse)
	storeCodexToolNameReverse(c, codexToolNameSessionKey, nil)
REDACTED

func storeCodexToolNameReverse(c *gin.Context, key string, reverse map[string]string) {
	if c == nil {
		return
REDACTED
	copyMap := make(map[string]string, len(reverse))
	for aliased, original := range reverse {
		copyMap[aliased] = original
REDACTED
	c.Set(key, copyMap)
REDACTED

func mergeCodexToolNameReverse(c *gin.Context, reverse map[string]string) {
	if c == nil || len(reverse) == 0 {
		return
REDACTED
	merged := make(map[string]string, len(reverse)+len(codexToolNameReverseFromContext(c)))
	for aliased, original := range codexToolNameReverseFromContext(c) {
		merged[aliased] = original
REDACTED
	for aliased, original := range reverse {
		merged[aliased] = original
REDACTED
	storeCodexToolNameReverse(c, codexToolNameReverseKey, merged)
REDACTED

func codexToolNameReverseFromContext(c *gin.Context) map[string]string {
	return codexToolNameReverseForKey(c, codexToolNameReverseKey)
REDACTED

func codexToolNameReverseForKey(c *gin.Context, key string) map[string]string {
	if c == nil {
		return nil
REDACTED
	raw, ok := c.Get(key)
	if !ok {
		return nil
REDACTED
	reverse, _ := raw.(map[string]string)
	return reverse
REDACTED

// updateCodexToolNameReverseForWSFrame keeps the active turn isolated from
// session updates that may arrive while that turn is still streaming.
func updateCodexToolNameReverseForWSFrame(c *gin.Context, frame []byte, reverse map[string]string) {
	if c == nil {
		return
REDACTED
	eventType := strings.TrimSpace(gjson.GetBytes(frame, "type").String())
	switch eventType {
	case "session.update":
		if gjson.GetBytes(frame, "session.tools").Exists() {
			storeCodexToolNameReverse(c, codexToolNameSessionKey, reverse)
	REDACTED
	case "response.create", "":
		active := reverse
		if !openAIWSFrameHasExplicitToolDeclarations(frame) {
			active = mergeCodexToolNameReverseMaps(
				codexToolNameReverseForKey(c, codexToolNameSessionKey),
				reverse,
			)
	REDACTED
		storeCodexToolNameReverse(c, codexToolNameReverseKey, active)
REDACTED
REDACTED

func openAIWSFrameHasExplicitToolDeclarations(frame []byte) bool {
	if gjson.GetBytes(frame, "tools").Exists() {
		return true
REDACTED
	for _, item := range gjson.GetBytes(frame, "input").Array() {
		if strings.EqualFold(strings.TrimSpace(item.Get("type").String()), "additional_tools") && item.Get("tools").Exists() {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func mergeCodexToolNameReverseMaps(base, overlay map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overlay))
	for aliased, original := range base {
		merged[aliased] = original
REDACTED
	for aliased, original := range overlay {
		merged[aliased] = original
REDACTED
	return merged
REDACTED

func restoreCodexToolNamesInJSON(data []byte, reverse map[string]string) []byte {
	if len(data) == 0 || len(reverse) == 0 || !json.Valid(data) {
		return data
REDACTED
	var decoded any
	if err := decodeOpenAIJSONUseNumber(data, &decoded); err != nil {
		return data
REDACTED
	if !restoreCodexToolNameFields(decoded, reverse) {
		return data
REDACTED
	restored, err := json.Marshal(decoded)
	if err != nil {
		return data
REDACTED
	return restored
REDACTED

func restoreCodexToolNamesFromContext(c *gin.Context, data []byte) []byte {
	reverse := codexToolNameReverseFromContext(c)
	switch strings.TrimSpace(gjson.GetBytes(data, "type").String()) {
	case "session.created", "session.updated":
		reverse = codexToolNameReverseForKey(c, codexToolNameSessionKey)
REDACTED
	return restoreCodexToolNamesInJSON(data, reverse)
REDACTED

func restoreCodexToolNamesFromSSEContext(c *gin.Context, data []byte, eventType string) []byte {
	if strings.TrimSpace(gjson.GetBytes(data, "type").String()) != "" || strings.TrimSpace(eventType) == "" {
		return restoreCodexToolNamesFromContext(c, data)
REDACTED
	compat := []byte(openAICompatPayloadWithEventType(string(data), eventType))
	restored := restoreCodexToolNamesFromContext(c, compat)
	if string(restored) == string(compat) {
		return data
REDACTED
	withoutSyntheticType, err := sjson.DeleteBytes(restored, "type")
	if err != nil {
		return restored
REDACTED
	return withoutSyntheticType
REDACTED

func restoreCodexToolNameFields(value any, reverse map[string]string) bool {
	root, ok := value.(map[string]any)
	if !ok {
		return false
REDACTED
	changed := false
	restoreItem := func(raw any) {
		item, ok := raw.(map[string]any)
		if !ok || !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(item["type"])), "function_call") {
			return
	REDACTED
		name, _ := item["name"].(string)
		if original, exists := reverse[name]; exists {
			item["name"] = original
			changed = true
	REDACTED
REDACTED
	restoreOutput := func(raw any) {
		output, _ := raw.([]any)
		for _, item := range output {
			restoreItem(item)
	REDACTED
REDACTED
	restoreResponse := func(raw any) {
		response, ok := raw.(map[string]any)
		if ok {
			restoreOutput(response["output"])
	REDACTED
REDACTED
	restoreFunction := func(raw any) {
		function, ok := raw.(map[string]any)
		if !ok {
			return
	REDACTED
		name, _ := function["name"].(string)
		if original, exists := reverse[name]; exists {
			function["name"] = original
			changed = true
	REDACTED
REDACTED
	restoreChatToolCalls := func(raw any) {
		toolCalls, _ := raw.([]any)
		for _, rawCall := range toolCalls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
		REDACTED
			callType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(call["type"])))
			if callType == "" || callType == "function" {
				restoreFunction(call["function"])
		REDACTED
	REDACTED
REDACTED
	restoreMessageContent := func(raw any) {
		content, _ := raw.([]any)
		for _, rawBlock := range content {
			block, ok := rawBlock.(map[string]any)
			if !ok || !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(block["type"])), "tool_use") {
				continue
		REDACTED
			name, _ := block["name"].(string)
			if original, exists := reverse[name]; exists {
				block["name"] = original
				changed = true
		REDACTED
	REDACTED
REDACTED
	var restoreTools func(any)
	restoreTools = func(raw any) {
		tools, _ := raw.([]any)
		for _, rawTool := range tools {
			tool, ok := rawTool.(map[string]any)
			if !ok {
				continue
		REDACTED
			toolType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(tool["type"])))
			if toolType == "function" {
				name, _ := tool["name"].(string)
				if original, exists := reverse[name]; exists {
					tool["name"] = original
					changed = true
			REDACTED
		REDACTED
			if toolType == "namespace" {
				restoreTools(tool["tools"])
		REDACTED
	REDACTED
REDACTED

	eventType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(root["type"])))
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		restoreItem(root["item"])
	case "response.created", "response.in_progress", "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		restoreResponse(root["response"])
	case "session.created", "session.updated":
		if session, ok := root["session"].(map[string]any); ok {
			restoreTools(session["tools"])
	REDACTED
REDACTED
	if _, hasOutput := root["output"]; hasOutput {
		restoreOutput(root["output"])
REDACTED
	if choices, ok := root["choices"].([]any); ok {
		for _, rawChoice := range choices {
			choice, ok := rawChoice.(map[string]any)
			if !ok {
				continue
		REDACTED
			for _, key := range []string{"message", "delta"REDACTED {
				if message, ok := choice[key].(map[string]any); ok {
					restoreChatToolCalls(message["tool_calls"])
			REDACTED
		REDACTED
	REDACTED
REDACTED
	restoreMessageContent(root["content"])
	if block, ok := root["content_block"].(map[string]any); ok && strings.EqualFold(strings.TrimSpace(firstNonEmptyString(block["type"])), "tool_use") {
		name, _ := block["name"].(string)
		if original, exists := reverse[name]; exists {
			block["name"] = original
			changed = true
	REDACTED
REDACTED
	return changed
REDACTED
