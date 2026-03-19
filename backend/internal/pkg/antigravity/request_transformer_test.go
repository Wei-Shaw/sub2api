package antigravity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildParts_ThinkingBlockWithoutSignature 测试thinking block无signature时的处理
func TestBuildParts_ThinkingBlockWithoutSignature(t *testing.T) {
	tests := []struct {
		name              string
		content           string
		allowDummyThought bool
		expectedParts     int
		description       string
REDACTED{
		{
			name: "Claude model - downgrade thinking to text without signature",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "Let me think...", "signature": ""REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			allowDummyThought: false,
			expectedParts:     3, // thinking 内容降级为普通 text part
			description:       "Claude模型缺少signature时应将thinking降级为text，并在上层禁用thinking mode",
	REDACTED,
		{
			name: "Claude model - preserve thinking block with signature",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "Let me think...", "signature": "sig_real_123"REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			allowDummyThought: false,
			expectedParts:     3,
			description:       "Claude模型应透传带 signature 的 thinking block（用于 Vertex 签名链路）",
	REDACTED,
		{
			name: "Gemini model - use dummy signature",
			content: `[
				{"type": "text", "text": "Hello"REDACTED,
				{"type": "thinking", "thinking": "Let me think...", "signature": ""REDACTED,
				{"type": "text", "text": "World"REDACTED
			]`,
			allowDummyThought: true,
			expectedParts:     3, // 三个block都保留，thinking使用dummy signature
			description:       "Gemini模型应该为无signature的thinking block使用dummy signature",
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolIDToName := make(map[string]string)
			parts, _, err := buildParts(json.RawMessage(tt.content), toolIDToName, tt.allowDummyThought)

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
			case "Claude model - downgrade thinking to text without signature":
				if len(parts) != 3 {
					t.Fatalf("expected 3 parts, got %d", len(parts))
			REDACTED
				if parts[1].Thought {
					t.Fatalf("expected downgraded text part, got thought=%v signature=%q",
						parts[1].Thought, parts[1].ThoughtSignature)
			REDACTED
				if parts[1].Text != "Let me think..." {
					t.Fatalf("expected downgraded text %q, got %q", "Let me think...", parts[1].Text)
			REDACTED
			case "Gemini model - use dummy signature":
				if len(parts) != 3 {
					t.Fatalf("expected 3 parts, got %d", len(parts))
			REDACTED
				if !parts[1].Thought || parts[1].ThoughtSignature != DummyThoughtSignature {
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

	t.Run("Gemini preserves provided tool_use signature", func(t *testing.T) {
		toolIDToName := make(map[string]string)
		parts, _, err := buildParts(json.RawMessage(content), toolIDToName, true)
		if err != nil {
			t.Fatalf("buildParts() error = %v", err)
	REDACTED
		if len(parts) != 1 || parts[0].FunctionCall == nil {
			t.Fatalf("expected 1 functionCall part, got %+v", parts)
	REDACTED
		if parts[0].ThoughtSignature != "sig_tool_abc" {
			t.Fatalf("expected preserved tool signature %q, got %q", "sig_tool_abc", parts[0].ThoughtSignature)
	REDACTED
REDACTED)

	t.Run("Gemini falls back to dummy tool_use signature when missing", func(t *testing.T) {
		contentNoSig := `[
			{"type": "tool_use", "id": "t1", "name": "Bash", "input": {"command": "ls"REDACTEDREDACTED
		]`
		toolIDToName := make(map[string]string)
		parts, _, err := buildParts(json.RawMessage(contentNoSig), toolIDToName, true)
		if err != nil {
			t.Fatalf("buildParts() error = %v", err)
	REDACTED
		if len(parts) != 1 || parts[0].FunctionCall == nil {
			t.Fatalf("expected 1 functionCall part, got %+v", parts)
	REDACTED
		if parts[0].ThoughtSignature != DummyThoughtSignature {
			t.Fatalf("expected dummy tool signature %q, got %q", DummyThoughtSignature, parts[0].ThoughtSignature)
	REDACTED
REDACTED)

	t.Run("Claude model - preserve valid signature for tool_use", func(t *testing.T) {
		toolIDToName := make(map[string]string)
		parts, _, err := buildParts(json.RawMessage(content), toolIDToName, false)
		if err != nil {
			t.Fatalf("buildParts() error = %v", err)
	REDACTED
		if len(parts) != 1 || parts[0].FunctionCall == nil {
			t.Fatalf("expected 1 functionCall part, got %+v", parts)
	REDACTED
		// Claude 模型应透传有效的 signature（Vertex/Google 需要完整签名链路）
		if parts[0].ThoughtSignature != "sig_tool_abc" {
			t.Fatalf("expected preserved tool signature %q, got %q", "sig_tool_abc", parts[0].ThoughtSignature)
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

func TestBuildGenerationConfig_ThinkingDynamicBudget(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		thinking    *ThinkingConfig
		wantBudget  int
		wantPresent bool
REDACTED{
		{
			name:        "enabled without budget defaults to dynamic (-1)",
			model:       "claude-opus-4-6-thinking",
			thinking:    &ThinkingConfig{Type: "enabled"REDACTED,
			wantBudget:  -1,
			wantPresent: true,
	REDACTED,
		{
			name:        "enabled with budget uses the provided value",
			model:       "claude-opus-4-6-thinking",
			thinking:    &ThinkingConfig{Type: "enabled", BudgetTokens: 1024REDACTED,
			wantBudget:  1024,
			wantPresent: true,
	REDACTED,
		{
			name:        "enabled with -1 budget uses dynamic (-1)",
			model:       "claude-opus-4-6-thinking",
			thinking:    &ThinkingConfig{Type: "enabled", BudgetTokens: -1REDACTED,
			wantBudget:  -1,
			wantPresent: true,
	REDACTED,
		{
			name:        "adaptive on opus4.6 maps to high budget (24576)",
			model:       "claude-opus-4-6-thinking",
			thinking:    &ThinkingConfig{Type: "adaptive", BudgetTokens: 20000REDACTED,
			wantBudget:  ClaudeAdaptiveHighThinkingBudgetTokens,
			wantPresent: true,
	REDACTED,
		{
			name:        "adaptive on non-opus model keeps default dynamic (-1)",
			model:       "claude-sonnet-4-5-thinking",
			thinking:    &ThinkingConfig{Type: "adaptive"REDACTED,
			wantBudget:  -1,
			wantPresent: true,
	REDACTED,
		{
			name:        "disabled does not emit thinkingConfig",
			model:       "claude-opus-4-6-thinking",
			thinking:    &ThinkingConfig{Type: "disabled", BudgetTokens: 1024REDACTED,
			wantBudget:  0,
			wantPresent: false,
	REDACTED,
		{
			name:        "nil thinking does not emit thinkingConfig",
			model:       "claude-opus-4-6-thinking",
			thinking:    nil,
			wantBudget:  0,
			wantPresent: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ClaudeRequest{
				Model:    tt.model,
				Thinking: tt.thinking,
		REDACTED
			cfg := buildGenerationConfig(req)
			if cfg == nil {
				t.Fatalf("expected non-nil generationConfig")
		REDACTED

			if tt.wantPresent {
				if cfg.ThinkingConfig == nil {
					t.Fatalf("expected thinkingConfig to be present")
			REDACTED
				if !cfg.ThinkingConfig.IncludeThoughts {
					t.Fatalf("expected includeThoughts=true")
			REDACTED
				if cfg.ThinkingConfig.ThinkingBudget != tt.wantBudget {
					t.Fatalf("expected thinkingBudget=%d, got %d", tt.wantBudget, cfg.ThinkingConfig.ThinkingBudget)
			REDACTED
				return
		REDACTED

			if cfg.ThinkingConfig != nil {
				t.Fatalf("expected thinkingConfig to be nil, got %+v", cfg.ThinkingConfig)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestTransformClaudeToGeminiWithOptions_PreservesBillingHeaderSystemBlock(t *testing.T) {
	tests := []struct {
		name   string
		system json.RawMessage
REDACTED{
		{
			name:   "system array",
			system: json.RawMessage(`[{"type":"text","text":"x-anthropic-billing-header keep"REDACTED]`),
	REDACTED,
		{
			name:   "system string",
			system: json.RawMessage(`"x-anthropic-billing-header keep"`),
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claudeReq := &ClaudeRequest{
				Model:  "claude-3-5-sonnet-latest",
				System: tt.system,
				Messages: []ClaudeMessage{
					{
						Role:    "user",
						Content: json.RawMessage(`[{"type":"text","text":"hello"REDACTED]`),
				REDACTED,
			REDACTED,
		REDACTED

			body, err := TransformClaudeToGeminiWithOptions(claudeReq, "project-1", "gemini-2.5-flash", DefaultTransformOptions())
		REDACTED

			var req V1InternalRequest
			require.NoError(t, json.Unmarshal(body, &req))
			require.NotNil(t, req.Request.SystemInstruction)

			found := false
			for _, part := range req.Request.SystemInstruction.Parts {
				if strings.Contains(part.Text, "x-anthropic-billing-header keep") {
					found = true
					break
			REDACTED
		REDACTED

			require.True(t, found, "转换后的 systemInstruction 应保留 x-anthropic-billing-header 内容")
	REDACTED)
REDACTED
REDACTED
