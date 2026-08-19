package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	codexReservedPythonToolName = "python"
	codexPythonToolAlias        = "python__sub2api"
	codexToolNameReverseKey     = "openai_codex_tool_name_reverse"
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
			if toolType != "namespace" {
				appendName(tool, "name")
		REDACTED
			if function, ok := tool["function"].(map[string]any); ok {
				appendName(function, "name")
		REDACTED
			collectTools(tool["tools"])
	REDACTED
REDACTED
	collectTools(reqBody["tools"])
	collectTools(reqBody["functions"])
	if choice, ok := reqBody["tool_choice"].(map[string]any); ok {
		if !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(choice["type"])), "namespace") {
			appendName(choice, "name")
	REDACTED
		if function, ok := choice["function"].(map[string]any); ok {
			appendName(function, "name")
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
			if strings.HasSuffix(typ, "_call") || typ == "tool_call" {
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
	if err := json.Unmarshal(body, &reqBody); err != nil {
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
	copyMap := make(map[string]string, len(reverse))
	for aliased, original := range reverse {
		copyMap[aliased] = original
REDACTED
	c.Set(codexToolNameReverseKey, copyMap)
REDACTED

func codexToolNameReverseFromContext(c *gin.Context) map[string]string {
	if c == nil {
		return nil
REDACTED
	raw, ok := c.Get(codexToolNameReverseKey)
	if !ok {
		return nil
REDACTED
	reverse, _ := raw.(map[string]string)
	return reverse
REDACTED

func restoreCodexToolNamesInJSON(data []byte, reverse map[string]string) []byte {
	if len(data) == 0 || len(reverse) == 0 || !json.Valid(data) {
		return data
REDACTED
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
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
	return restoreCodexToolNamesInJSON(data, codexToolNameReverseFromContext(c))
REDACTED

func restoreCodexToolNameFields(value any, reverse map[string]string) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		if name, ok := typed["name"].(string); ok {
			if original, exists := reverse[name]; exists {
				typed["name"] = original
				changed = true
		REDACTED
	REDACTED
		for _, child := range typed {
			if restoreCodexToolNameFields(child, reverse) {
				changed = true
		REDACTED
	REDACTED
	case []any:
		for _, child := range typed {
			if restoreCodexToolNameFields(child, reverse) {
				changed = true
		REDACTED
	REDACTED
REDACTED
	return changed
REDACTED
