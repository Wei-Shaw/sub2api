package antigravity

import (
	"encoding/json"
	"testing"
)

// TestBuildParts_ThinkingBlockWithoutSignature 测试thinking block无signature时的处理
func TestBuildParts_ThinkingBlockWithoutSignature(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		thoughtMode   thoughtSignatureMode
		expectedParts int
		description   string
REDACTED{
		{
			name: "Claude model - skip thinking block without signature",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "Let me think...", "signature": ""REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			thoughtMode:   thoughtSignatureModePreserve,
			expectedParts: 2, // 只有两个text block
			description:   "Claude模型应该跳过无signature的thinking block",
	REDACTED,
		{
			name: "Claude model - preserve thinking block with signature",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "Let me think...", "signature": "sig_real_123"REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			thoughtMode:   thoughtSignatureModePreserve,
			expectedParts: 3,
			description:   "Claude模型应透传带 signature 的 thinking block（用于 Vertex 签名链路）",
	REDACTED,
		{
			name: "Gemini model - use dummy signature",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "Let me think...", "signature": ""REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			thoughtMode:   thoughtSignatureModeDummy,
			expectedParts: 3, // 三个block都保留，thinking使用dummy signature
			description:   "Gemini模型应该为无signature的thinking block使用dummy signature",
	REDACTED,
		{
			name: "Claude model - signature-only thinking block becomes signature-only part",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "", "signature": "sig_only_456"REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			thoughtMode:   thoughtSignatureModePreserve,
			expectedParts: 3,
			description:   "Claude模型应将空 thinking + signature 映射为 signature-only part，便于 roundtrip",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolIDToName := make(map[string]string)
			parts, err := buildParts(json.RawMessage(tt.content), toolIDToName, tt.thoughtMode)

			if err != nil {
				t.Fatalf("buildParts() error = %v", err)
		REDACTED

			if len(parts) != tt.expectedParts {
				t.Errorf("%s: got %d parts, want %d parts", tt.description, len(parts), tt.expectedParts)
		REDACTED

			switch tt.name {
			case "Claude model - preserve thinking block with signature":
				if len(parts) != 3 {
					t.Fatalf("expected 3 parts, got %d", len(parts))
			REDACTED
				if !parts[1].Thought || parts[1].ThoughtSignature != "sig_real_123" {
					t.Fatalf("expected thought part with signature sig_real_123, got thought=%v signature=%q",
						parts[1].Thought, parts[1].ThoughtSignature)
			REDACTED
			case "Claude model - signature-only thinking block becomes signature-only part":
				if len(parts) != 3 {
					t.Fatalf("expected 3 parts, got %d", len(parts))
			REDACTED
				if parts[1].Thought || parts[1].Text != "" || parts[1].ThoughtSignature != "sig_only_456" {
					t.Fatalf("expected signature-only part, got thought=%v text=%q signature=%q",
						parts[1].Thought, parts[1].Text, parts[1].ThoughtSignature)
			REDACTED
			case "Gemini model - use dummy signature":
				if len(parts) != 3 {
					t.Fatalf("expected 3 parts, got %d", len(parts))
			REDACTED
				if !parts[1].Thought || parts[1].ThoughtSignature != dummyThoughtSignature {
					t.Fatalf("expected dummy thought signature, got thought=%v signature=%q",
						parts[1].Thought, parts[1].ThoughtSignature)
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestBuildParts_ToolUseSignatureHandling(t *testing.T) {
	content := `[
		{"type": "tool_use", "id": "t1", "name": "Bash", "input": {"command": "ls"REDACTED, "signature": "sig_tool_abc"REDACTED
	]`

	t.Run("Claude preserve tool_use signature", func(t *testing.T) {
		toolIDToName := make(map[string]string)
		parts, err := buildParts(json.RawMessage(content), toolIDToName, thoughtSignatureModePreserve)
		if err != nil {
			t.Fatalf("buildParts() error = %v", err)
	REDACTED
		if len(parts) != 1 || parts[0].FunctionCall == nil {
			t.Fatalf("expected 1 functionCall part, got %+v", parts)
	REDACTED
		if parts[0].ThoughtSignature != "sig_tool_abc" {
			t.Fatalf("expected tool signature sig_tool_abc, got %q", parts[0].ThoughtSignature)
	REDACTED
REDACTED)

	t.Run("Gemini uses dummy tool_use signature", func(t *testing.T) {
		toolIDToName := make(map[string]string)
		parts, err := buildParts(json.RawMessage(content), toolIDToName, thoughtSignatureModeDummy)
		if err != nil {
			t.Fatalf("buildParts() error = %v", err)
	REDACTED
		if len(parts) != 1 || parts[0].FunctionCall == nil {
			t.Fatalf("expected 1 functionCall part, got %+v", parts)
	REDACTED
		if parts[0].ThoughtSignature != dummyThoughtSignature {
			t.Fatalf("expected dummy tool signature %q, got %q", dummyThoughtSignature, parts[0].ThoughtSignature)
	REDACTED
REDACTED)
REDACTED

// TestBuildTools_CustomTypeTools 测试custom类型工具转换
func TestBuildTools_CustomTypeTools(t *testing.T) {
	tests := []struct {
		name        string
		tools       []ClaudeTool
		expectedLen int
		description string
REDACTED{
		{
			name: "Standard tool format",
			tools: []ClaudeTool{
				{
					Name:        "get_weather",
					Description: "Get weather information",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"location": map[string]any{"type": "string"REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 1,
			description: "标准工具格式应该正常转换",
	REDACTED,
		{
			name: "Custom type tool (MCP format)",
			tools: []ClaudeTool{
				{
					Type: "custom",
					Name: "mcp_tool",
					Custom: &ClaudeCustomToolSpec{
						Description: "MCP tool description",
						InputSchema: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"param": map[string]any{"type": "string"REDACTED,
						REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 1,
			description: "Custom类型工具应该从Custom字段读取description和input_schema",
	REDACTED,
		{
			name: "Mixed standard and custom tools",
			tools: []ClaudeTool{
				{
					Name:        "standard_tool",
					Description: "Standard tool",
					InputSchema: map[string]any{"type": "object"REDACTED,
			REDACTED,
				{
					Type: "custom",
					Name: "custom_tool",
					Custom: &ClaudeCustomToolSpec{
						Description: "Custom tool",
						InputSchema: map[string]any{"type": "object"REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 1, // 返回一个GeminiToolDeclaration，包含2个function declarations
			description: "混合标准和custom工具应该都能正确转换",
	REDACTED,
		{
			name: "Invalid custom tool - nil Custom field",
			tools: []ClaudeTool{
				{
					Type: "custom",
					Name: "invalid_custom",
					// Custom 为 nil
			REDACTED,
		REDACTED,
			expectedLen: 0, // 应该被跳过
			description: "Custom字段为nil的custom工具应该被跳过",
	REDACTED,
		{
			name: "Invalid custom tool - nil InputSchema",
			tools: []ClaudeTool{
				{
					Type: "custom",
					Name: "invalid_custom",
					Custom: &ClaudeCustomToolSpec{
						Description: "Invalid",
						// InputSchema 为 nil
				REDACTED,
			REDACTED,
		REDACTED,
			expectedLen: 0, // 应该被跳过
			description: "InputSchema为nil的custom工具应该被跳过",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildTools(tt.tools)

			if len(result) != tt.expectedLen {
				t.Errorf("%s: got %d tool declarations, want %d", tt.description, len(result), tt.expectedLen)
		REDACTED

			// 验证function declarations存在
			if len(result) > 0 && result[0].FunctionDeclarations != nil {
				if len(result[0].FunctionDeclarations) != len(tt.tools) {
					t.Errorf("%s: got %d function declarations, want %d",
						tt.description, len(result[0].FunctionDeclarations), len(tt.tools))
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED
