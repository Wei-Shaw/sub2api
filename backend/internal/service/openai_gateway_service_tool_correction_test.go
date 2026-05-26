package service

import (
	"strings"
	"testing"
)

// TestOpenAIGatewayService_ToolCorrection 测试 OpenAIGatewayService 中的工具修正集成
func TestOpenAIGatewayService_ToolCorrection(t *testing.T) {
	// 创建一个简单的 service 实例来测试工具修正
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
	}

	tests := []struct {
		name     string
		input    []byte
		expected string
		changed  bool
	}{
		{
			name: "correct apply_patch in response body",
			input: []byte(`{
				"choices": [{
					"message": {
						"tool_calls": [{
							"function": {"name": "apply_patch"}
						}]
					}
				}]
			}`),
			expected: "edit",
			changed:  true,
		},
		{
			name: "correct update_plan in response body",
			input: []byte(`{
				"tool_calls": [{
					"function": {"name": "update_plan"}
				}]
			}`),
			expected: "todowrite",
			changed:  true,
		},
		{
			name: "no change for correct tool name",
			input: []byte(`{
				"tool_calls": [{
					"function": {"name": "edit"}
				}]
			}`),
			expected: "edit",
			changed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.correctToolCallsInResponseBody(tt.input)
			resultStr := string(result)

			// 检查是否包含期望的工具名称
			if !strings.Contains(resultStr, tt.expected) {
				t.Errorf("expected result to contain %q, got %q", tt.expected, resultStr)
			}

			// 对于预期有变化的情况，验证结果与输入不同
			if tt.changed && string(result) == string(tt.input) {
				t.Error("expected result to be different from input, but they are the same")
			}

			// 对于预期无变化的情况，验证结果与输入相同
			if !tt.changed && string(result) != string(tt.input) {
				t.Error("expected result to be same as input, but they are different")
			}
		})
	}
}

// TestOpenAIGatewayService_ToolCorrectorInitialization 测试工具修正器是否正确初始化
func TestOpenAIGatewayService_ToolCorrectorInitialization(t *testing.T) {
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
	}

	if service.toolCorrector == nil {
		t.Fatal("toolCorrector should not be nil")
	}

	// 测试修正器可以正常工作
	data := `{"tool_calls":[{"function":{"name":"apply_patch"}}]}`
	corrected, changed := service.toolCorrector.CorrectToolCallsInSSEData(data)

	if !changed {
		t.Error("expected tool call to be corrected")
	}

	if !strings.Contains(corrected, "edit") {
		t.Errorf("expected corrected data to contain 'edit', got %q", corrected)
	}
}

// TestToolCorrectionStats 测试工具修正统计功能
func TestToolCorrectionStats(t *testing.T) {
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
	}

	// 执行几次修正
	testData := []string{
		`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`,
		`{"tool_calls":[{"function":{"name":"update_plan"}}]}`,
		`{"tool_calls":[{"function":{"name":"apply_patch"}}]}`,
	}

	for _, data := range testData {
		service.toolCorrector.CorrectToolCallsInSSEData(data)
	}

	stats := service.toolCorrector.GetStats()

	if stats.TotalCorrected != 3 {
		t.Errorf("expected 3 corrections, got %d", stats.TotalCorrected)
	}

	if stats.CorrectionsByTool["apply_patch->edit"] != 2 {
		t.Errorf("expected 2 apply_patch->edit corrections, got %d", stats.CorrectionsByTool["apply_patch->edit"])
	}

	if stats.CorrectionsByTool["update_plan->todowrite"] != 1 {
		t.Errorf("expected 1 update_plan->todowrite correction, got %d", stats.CorrectionsByTool["update_plan->todowrite"])
	}
}
