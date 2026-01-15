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
REDACTED

	tests := []struct {
		name     string
		input    []byte
		expected string
		changed  bool
REDACTED{
		{
			name: "correct apply_patch in response body",
			input: []byte(`{
				"choices": [{
					"message": {
						"tool_calls": [{
							"function": {"name": "apply_patch"REDACTED
					REDACTED]
				REDACTED
			REDACTED]
		REDACTED`),
			expected: "edit",
			changed:  true,
	REDACTED,
		{
			name: "correct update_plan in response body",
			input: []byte(`{
				"tool_calls": [{
					"function": {"name": "update_plan"REDACTED
			REDACTED]
		REDACTED`),
			expected: "todowrite",
			changed:  true,
	REDACTED,
		{
			name: "no change for correct tool name",
			input: []byte(`{
				"tool_calls": [{
					"function": {"name": "edit"REDACTED
			REDACTED]
		REDACTED`),
			expected: "edit",
			changed:  false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.correctToolCallsInResponseBody(tt.input)
			resultStr := string(result)

			// 检查是否包含期望的工具名称
			if !strings.Contains(resultStr, tt.expected) {
				t.Errorf("expected result to contain %q, got %q", tt.expected, resultStr)
		REDACTED

			// 对于预期有变化的情况，验证结果与输入不同
			if tt.changed && string(result) == string(tt.input) {
				t.Error("expected result to be different from input, but they are the same")
		REDACTED

			// 对于预期无变化的情况，验证结果与输入相同
			if !tt.changed && string(result) != string(tt.input) {
				t.Error("expected result to be same as input, but they are different")
		REDACTED
	REDACTED)
REDACTED
REDACTED

// TestOpenAIGatewayService_ToolCorrectorInitialization 测试工具修正器是否正确初始化
func TestOpenAIGatewayService_ToolCorrectorInitialization(t *testing.T) {
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
REDACTED

	if service.toolCorrector == nil {
		t.Fatal("toolCorrector should not be nil")
REDACTED

	// 测试修正器可以正常工作
	data := `{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTED`
	corrected, changed := service.toolCorrector.CorrectToolCallsInSSEData(data)

	if !changed {
		t.Error("expected tool call to be corrected")
REDACTED

	if !strings.Contains(corrected, "edit") {
		t.Errorf("expected corrected data to contain 'edit', got %q", corrected)
REDACTED
REDACTED

// TestToolCorrectionStats 测试工具修正统计功能
func TestToolCorrectionStats(t *testing.T) {
	service := &OpenAIGatewayService{
		toolCorrector: NewCodexToolCorrector(),
REDACTED

	// 执行几次修正
	testData := []string{
		`{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTED`,
		`{"tool_calls":[{"function":{"name":"update_plan"REDACTEDREDACTED]REDACTED`,
		`{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTED`,
REDACTED

	for _, data := range testData {
		service.toolCorrector.CorrectToolCallsInSSEData(data)
REDACTED

	stats := service.toolCorrector.GetStats()

	if stats.TotalCorrected != 3 {
		t.Errorf("expected 3 corrections, got %d", stats.TotalCorrected)
REDACTED

	if stats.CorrectionsByTool["apply_patch->edit"] != 2 {
		t.Errorf("expected 2 apply_patch->edit corrections, got %d", stats.CorrectionsByTool["apply_patch->edit"])
REDACTED

	if stats.CorrectionsByTool["update_plan->todowrite"] != 1 {
		t.Errorf("expected 1 update_plan->todowrite correction, got %d", stats.CorrectionsByTool["update_plan->todowrite"])
REDACTED
REDACTED
