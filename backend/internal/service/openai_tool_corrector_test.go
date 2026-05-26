package service

import (
	"encoding/json"
	"testing"
)

func TestMayContainToolCallPayload(t *testing.T) {
	if mayContainToolCallPayload([]byte(`{"type":"response.output_text.delta","delta":"hello"}`)) {
		t.Fatalf("plain text event should not trigger tool-call parsing")
	}
	if !mayContainToolCallPayload([]byte(`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`)) {
		t.Fatalf("tool_calls event should trigger tool-call parsing")
	}
}

func TestCorrectToolCallsInSSEData(t *testing.T) {
	corrector := NewCodexToolCorrector()

	tests := []struct {
		name            string
		input           string
		expectCorrected bool
		checkFunc       func(t *testing.T, result string)
	}{
		{
			name:            "empty string",
			input:           "",
			expectCorrected: false,
		},
		{
			name:            "newline only",
			input:           "\n",
			expectCorrected: false,
		},
		{
			name:            "invalid json",
			input:           "not a json",
			expectCorrected: false,
		},
		{
			name:            "correct apply_patch in tool_calls",
			input:           `{"tool_calls":[{"function":{"name":"apply_patch","arguments":"{}"}}]}`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				toolCalls, ok := payload["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in result")
				}
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
				}
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
				}
				if functionCall["name"] != "edit" {
					t.Errorf("Expected tool name 'edit', got '%v'", functionCall["name"])
				}
			},
		},
		{
			name:            "correct update_plan in function_call",
			input:           `{"function_call":{"name":"update_plan","arguments":"{}"}}`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				functionCall, ok := payload["function_call"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function_call format")
				}
				if functionCall["name"] != "todowrite" {
					t.Errorf("Expected tool name 'todowrite', got '%v'", functionCall["name"])
				}
			},
		},
		{
			name:            "correct search_files in delta.tool_calls",
			input:           `{"delta":{"tool_calls":[{"function":{"name":"search_files"}}]}}`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				delta, ok := payload["delta"].(map[string]any)
				if !ok {
					t.Fatal("Invalid delta format")
				}
				toolCalls, ok := delta["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in delta")
				}
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
				}
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
				}
				if functionCall["name"] != "grep" {
					t.Errorf("Expected tool name 'grep', got '%v'", functionCall["name"])
				}
			},
		},
		{
			name:            "correct list_files in choices.message.tool_calls",
			input:           `{"choices":[{"message":{"tool_calls":[{"function":{"name":"list_files"}}]}}]}`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				choices, ok := payload["choices"].([]any)
				if !ok || len(choices) == 0 {
					t.Fatal("No choices found in result")
				}
				choice, ok := choices[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid choice format")
				}
				message, ok := choice["message"].(map[string]any)
				if !ok {
					t.Fatal("Invalid message format")
				}
				toolCalls, ok := message["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in message")
				}
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
				}
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
				}
				if functionCall["name"] != "glob" {
					t.Errorf("Expected tool name 'glob', got '%v'", functionCall["name"])
				}
			},
		},
		{
			name:            "no correction needed",
			input:           `{"tool_calls":[{"function":{"name":"read","arguments":"{}"}}]}`,
			expectCorrected: false,
		},
		{
			name:            "correct multiple tool calls",
			input:           `{"tool_calls":[{"function":{"name":"apply_patch"}},{"function":{"name":"read_file"}}]}`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				toolCalls, ok := payload["tool_calls"].([]any)
				if !ok || len(toolCalls) < 2 {
					t.Fatal("Expected at least 2 tool_calls")
				}

				toolCall1, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid first tool_call format")
				}
				func1, ok := toolCall1["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid first function format")
				}
				if func1["name"] != "edit" {
					t.Errorf("Expected first tool name 'edit', got '%v'", func1["name"])
				}

				toolCall2, ok := toolCalls[1].(map[string]any)
				if !ok {
					t.Fatal("Invalid second tool_call format")
				}
				func2, ok := toolCall2["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid second function format")
				}
				if func2["name"] != "read" {
					t.Errorf("Expected second tool name 'read', got '%v'", func2["name"])
				}
			},
		},
		{
			name:            "camelCase format - applyPatch",
			input:           `{"tool_calls":[{"function":{"name":"applyPatch"}}]}`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
				}
				toolCalls, ok := payload["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in result")
				}
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
				}
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
				}
				if functionCall["name"] != "edit" {
					t.Errorf("Expected tool name 'edit', got '%v'", functionCall["name"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, corrected := corrector.CorrectToolCallsInSSEData(tt.input)

			if corrected != tt.expectCorrected {
				t.Errorf("Expected corrected=%v, got %v", tt.expectCorrected, corrected)
			}

			if !corrected && result != tt.input {
				t.Errorf("Expected unchanged result when not corrected")
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
			}
		})
	}
}

func TestCorrectToolName(t *testing.T) {
	tests := []struct {
		input     string
		expected  string
		corrected bool
	}{
		{"apply_patch", "edit", true},
		{"applyPatch", "edit", true},
		{"update_plan", "todowrite", true},
		{"updatePlan", "todowrite", true},
		{"read_plan", "todoread", true},
		{"readPlan", "todoread", true},
		{"search_files", "grep", true},
		{"searchFiles", "grep", true},
		{"list_files", "glob", true},
		{"listFiles", "glob", true},
		{"read_file", "read", true},
		{"readFile", "read", true},
		{"write_file", "write", true},
		{"writeFile", "write", true},
		{"execute_bash", "bash", true},
		{"executeBash", "bash", true},
		{"exec_bash", "bash", true},
		{"execBash", "bash", true},
		{"unknown_tool", "unknown_tool", false},
		{"read", "read", false},
		{"edit", "edit", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, corrected := CorrectToolName(tt.input)

			if corrected != tt.corrected {
				t.Errorf("Expected corrected=%v, got %v", tt.corrected, corrected)
			}

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestGetToolNameMapping(t *testing.T) {
	mapping := GetToolNameMapping()

	expectedMappings := map[string]string{
		"apply_patch":  "edit",
		"update_plan":  "todowrite",
		"read_plan":    "todoread",
		"search_files": "grep",
		"list_files":   "glob",
	}

	for from, to := range expectedMappings {
		if mapping[from] != to {
			t.Errorf("Expected mapping[%s] = %s, got %s", from, to, mapping[from])
		}
	}

	mapping["test_tool"] = "test_value"
	newMapping := GetToolNameMapping()
	if _, exists := newMapping["test_tool"]; exists {
		t.Error("Modifications to returned mapping should not affect original")
	}
}

func TestCorrectorStats(t *testing.T) {
	corrector := NewCodexToolCorrector()

	stats := corrector.GetStats()
	if stats.TotalCorrected != 0 {
		t.Errorf("Expected TotalCorrected=0, got %d", stats.TotalCorrected)
	}
	if len(stats.CorrectionsByTool) != 0 {
		t.Errorf("Expected empty CorrectionsByTool, got length %d", len(stats.CorrectionsByTool))
	}

	corrector.CorrectToolCallsInSSEData(`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`)
	corrector.CorrectToolCallsInSSEData(`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`)
	corrector.CorrectToolCallsInSSEData(`{"tool_calls":[{"function":{"name":"update_plan"}}]}`)

	stats = corrector.GetStats()
	if stats.TotalCorrected != 3 {
		t.Errorf("Expected TotalCorrected=3, got %d", stats.TotalCorrected)
	}

	if stats.CorrectionsByTool["apply_patch->edit"] != 2 {
		t.Errorf("Expected apply_patch->edit count=2, got %d", stats.CorrectionsByTool["apply_patch->edit"])
	}

	if stats.CorrectionsByTool["update_plan->todowrite"] != 1 {
		t.Errorf("Expected update_plan->todowrite count=1, got %d", stats.CorrectionsByTool["update_plan->todowrite"])
	}

	corrector.ResetStats()
	stats = corrector.GetStats()
	if stats.TotalCorrected != 0 {
		t.Errorf("Expected TotalCorrected=0 after reset, got %d", stats.TotalCorrected)
	}
	if len(stats.CorrectionsByTool) != 0 {
		t.Errorf("Expected empty CorrectionsByTool after reset, got length %d", len(stats.CorrectionsByTool))
	}
}

func TestComplexSSEData(t *testing.T) {
	corrector := NewCodexToolCorrector()

	input := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1234567890,
		"model": "gpt-5.1-codex",
		"choices": [
			{
				"index": 0,
				"delta": {
					"tool_calls": [
						{
							"index": 0,
							"function": {
								"name": "apply_patch",
								"arguments": "{\"file\":\"test.go\"}"
							}
						}
					]
				},
				"finish_reason": null
			}
		]
	}`

	result, corrected := corrector.CorrectToolCallsInSSEData(input)

	if !corrected {
		t.Error("Expected data to be corrected")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatal("No choices found in result")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		t.Fatal("Invalid choice format")
	}
	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		t.Fatal("Invalid delta format")
	}
	toolCalls, ok := delta["tool_calls"].([]any)
	if !ok || len(toolCalls) == 0 {
		t.Fatal("No tool_calls found in delta")
	}
	toolCall, ok := toolCalls[0].(map[string]any)
	if !ok {
		t.Fatal("Invalid tool_call format")
	}
	function, ok := toolCall["function"].(map[string]any)
	if !ok {
		t.Fatal("Invalid function format")
	}

	if function["name"] != "edit" {
		t.Errorf("Expected tool name 'edit', got '%v'", function["name"])
	}
}

// TestCorrectToolParameters 测试工具参数修正
func TestCorrectToolParameters(t *testing.T) {
	corrector := NewCodexToolCorrector()

	tests := []struct {
		name     string
		input    string
		expected map[string]bool // key: 期待存在的参数, value: true表示应该存在
	}{
		{
			name: "rename work_dir to workdir in bash tool",
			input: `{
				"tool_calls": [{
					"function": {
						"name": "bash",
						"arguments": "{\"command\":\"ls\",\"work_dir\":\"/tmp\"}"
					}
				}]
			}`,
			expected: map[string]bool{
				"command":  true,
				"workdir":  true,
				"work_dir": false,
			},
		},
		{
			name: "rename snake_case edit params to camelCase",
			input: `{
				"tool_calls": [{
					"function": {
						"name": "apply_patch",
						"arguments": "{\"path\":\"/foo/bar.go\",\"old_string\":\"old\",\"new_string\":\"new\"}"
					}
				}]
			}`,
			expected: map[string]bool{
				"filePath":   true,
				"path":       false,
				"oldString":  true,
				"old_string": false,
				"newString":  true,
				"new_string": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrected, changed := corrector.CorrectToolCallsInSSEData(tt.input)
			if !changed {
				t.Error("expected data to be corrected")
			}

			// 解析修正后的数据
			var result map[string]any
			if err := json.Unmarshal([]byte(corrected), &result); err != nil {
				t.Fatalf("failed to parse corrected data: %v", err)
			}

			// 检查工具调用
			toolCalls, ok := result["tool_calls"].([]any)
			if !ok || len(toolCalls) == 0 {
				t.Fatal("no tool_calls found in corrected data")
			}

			toolCall, ok := toolCalls[0].(map[string]any)
			if !ok {
				t.Fatal("invalid tool_call structure")
			}

			function, ok := toolCall["function"].(map[string]any)
			if !ok {
				t.Fatal("no function found in tool_call")
			}

			argumentsStr, ok := function["arguments"].(string)
			if !ok {
				t.Fatal("arguments is not a string")
			}

			var args map[string]any
			if err := json.Unmarshal([]byte(argumentsStr), &args); err != nil {
				t.Fatalf("failed to parse arguments: %v", err)
			}

			// 验证期望的参数
			for param, shouldExist := range tt.expected {
				_, exists := args[param]
				if shouldExist && !exists {
					t.Errorf("expected parameter %q to exist, but it doesn't", param)
				}
				if !shouldExist && exists {
					t.Errorf("expected parameter %q to not exist, but it does", param)
				}
			}
		})
	}
}
