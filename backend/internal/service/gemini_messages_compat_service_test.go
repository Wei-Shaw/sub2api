package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConvertClaudeToolsToGeminiTools_CustomType 测试custom类型工具转换
func TestConvertClaudeToolsToGeminiTools_CustomType(t *testing.T) {
	tests := []struct {
		name        string
		tools       any
		expectedLen int
		description string
REDACTED{
		{
			name: "Standard tools",
			tools: []any{
				map[string]any{
					"name":         "get_weather",
					"description":  "Get weather info",
					"input_schema": map[string]any{"type": "object"REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 1,
			description: "标准工具格式应该正常转换",
	REDACTED,
		{
			name: "Custom type tool (MCP format)",
			tools: []any{
				map[string]any{
					"type": "custom",
					"name": "mcp_tool",
					"custom": map[string]any{
						"description":  "MCP tool description",
						"input_schema": map[string]any{"type": "object"REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 1,
			description: "Custom类型工具应该从custom字段读取",
	REDACTED,
		{
			name: "Mixed standard and custom tools",
			tools: []any{
				map[string]any{
					"name":         "standard_tool",
					"description":  "Standard",
					"input_schema": map[string]any{"type": "object"REDACTED,
			REDACTED,
				map[string]any{
					"type": "custom",
					"name": "custom_tool",
					"custom": map[string]any{
						"description":  "Custom",
						"input_schema": map[string]any{"type": "object"REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 1,
			description: "混合工具应该都能正确转换",
	REDACTED,
		{
			name: "Custom tool without custom field",
			tools: []any{
				map[string]any{
					"type": "custom",
					"name": "invalid_custom",
					// 缺少 custom 字段
			REDACTED,
		REDACTED,
			expectedLen: 0, // 应该被跳过
			description: "缺少custom字段的custom工具应该被跳过",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertClaudeToolsToGeminiTools(tt.tools)

			if tt.expectedLen == 0 {
				if result != nil {
					t.Errorf("%s: expected nil result, got %v", tt.description, result)
			REDACTED
				return
		REDACTED

			if result == nil {
				t.Fatalf("%s: expected non-nil result", tt.description)
		REDACTED

			if len(result) != 1 {
				t.Errorf("%s: expected 1 tool declaration, got %d", tt.description, len(result))
				return
		REDACTED

			toolDecl, ok := result[0].(map[string]any)
			if !ok {
				t.Fatalf("%s: result[0] is not map[string]any", tt.description)
		REDACTED

			funcDecls, ok := toolDecl["functionDeclarations"].([]any)
			if !ok {
				t.Fatalf("%s: functionDeclarations is not []any", tt.description)
		REDACTED

			toolsArr, _ := tt.tools.([]any)
			expectedFuncCount := 0
			for _, tool := range toolsArr {
				toolMap, _ := tool.(map[string]any)
				if toolMap["name"] != "" {
					// 检查是否为有效的custom工具
					if toolMap["type"] == "custom" {
						if toolMap["custom"] != nil {
							expectedFuncCount++
					REDACTED
				REDACTED else {
						expectedFuncCount++
				REDACTED
			REDACTED
		REDACTED

			if len(funcDecls) != expectedFuncCount {
				t.Errorf("%s: expected %d function declarations, got %d",
					tt.description, expectedFuncCount, len(funcDecls))
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestConvertClaudeMessagesToGeminiGenerateContent_AddsThoughtSignatureForToolUse(t *testing.T) {
	claudeReq := map[string]any{
		"model":      "REDACTED",
		"max_tokens": 10,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "hi"REDACTED,
			REDACTED,
		REDACTED,
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "text", "text": "ok"REDACTED,
					map[string]any{
						"type":  "tool_use",
						"id":    "toolu_123",
						"name":  "default_api:write_file",
						"input": map[string]any{"path": "a.txt", "content": "x"REDACTED,
						// no signature on purpose
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
		"tools": []any{
			map[string]any{
				"name":        "default_api:write_file",
				"description": "write file",
				"input_schema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"path": map[string]any{"type": "string"REDACTEDREDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	b, _ := json.Marshal(claudeReq)

	out, err := convertClaudeMessagesToGeminiGenerateContent(b)
	if err != nil {
		t.Fatalf("convert failed: %v", err)
REDACTED
	s := string(out)
	if !strings.Contains(s, "\"functionCall\"") {
		t.Fatalf("expected functionCall in output, got: %s", s)
REDACTED
	if !strings.Contains(s, "\"thoughtSignature\":\""+geminiDummyThoughtSignature+"\"") {
		t.Fatalf("expected injected thoughtSignature %q, got: %s", geminiDummyThoughtSignature, s)
REDACTED
REDACTED

func TestEnsureGeminiFunctionCallThoughtSignatures_InsertsWhenMissing(t *testing.T) {
	geminiReq := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"name": "default_api:write_file",
							"args": map[string]any{"path": "a.txt"REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED
	b, _ := json.Marshal(geminiReq)
	out := ensureGeminiFunctionCallThoughtSignatures(b)
	s := string(out)
	if !strings.Contains(s, "\"thoughtSignature\":\""+geminiDummyThoughtSignature+"\"") {
		t.Fatalf("expected injected thoughtSignature %q, got: %s", geminiDummyThoughtSignature, s)
REDACTED
REDACTED

func TestExtractGeminiUsage_ThoughtsTokenCount(t *testing.T) {
	tests := []struct {
		name          string
		resp          map[string]any
		wantInput     int
		wantOutput    int
		wantCacheRead int
		wantNil       bool
REDACTED{
		{
			name: "with thoughtsTokenCount",
			resp: map[string]any{
				"usageMetadata": map[string]any{
					"promptTokenCount":     float64(100),
					"candidatesTokenCount": float64(20),
					"thoughtsTokenCount":   float64(50),
			REDACTED,
		REDACTED,
			wantInput:  100,
			wantOutput: 70,
	REDACTED,
		{
			name: "with thoughtsTokenCount and cache",
			resp: map[string]any{
				"usageMetadata": map[string]any{
					"promptTokenCount":        float64(100),
					"candidatesTokenCount":    float64(20),
					"cachedContentTokenCount": float64(30),
					"thoughtsTokenCount":      float64(50),
			REDACTED,
		REDACTED,
			wantInput:     70,
			wantOutput:    70,
			wantCacheRead: 30,
	REDACTED,
		{
			name: "without thoughtsTokenCount (old model)",
			resp: map[string]any{
				"usageMetadata": map[string]any{
					"promptTokenCount":     float64(100),
					"candidatesTokenCount": float64(20),
			REDACTED,
		REDACTED,
			wantInput:  100,
			wantOutput: 20,
	REDACTED,
		{
			name:    "no usageMetadata",
			resp:    map[string]any{REDACTED,
			wantNil: true,
	REDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := extractGeminiUsage(tt.resp)
			if tt.wantNil {
				require.Nil(t, usage)
				return
		REDACTED
			require.NotNil(t, usage)
			require.Equal(t, tt.wantInput, usage.InputTokens)
			require.Equal(t, tt.wantOutput, usage.OutputTokens)
			require.Equal(t, tt.wantCacheRead, usage.CacheReadInputTokens)
	REDACTED)
REDACTED
REDACTED
