package service

import (
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

// TestCorrectToolParameters 测试工具参数修正
