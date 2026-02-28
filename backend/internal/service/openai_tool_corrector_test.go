package service

import (
	"encoding/json"
	"testing"
)

func TestMayContainToolCallPayload(t *testing.T) {
	if mayContainToolCallPayload([]byte(`{"type":"response.output_text.delta","delta":"hello"REDACTED`)) {
		t.Fatalf("plain text event should not trigger tool-call parsing")
REDACTED
	if !mayContainToolCallPayload([]byte(`{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTED`)) {
		t.Fatalf("tool_calls event should trigger tool-call parsing")
REDACTED
REDACTED

func TestCorrectToolCallsInSSEData(t *testing.T) {
	corrector := NewCodexToolCorrector()

	tests := []struct {
		name            string
		input           string
		expectCorrected bool
		checkFunc       func(t *testing.T, result string)
REDACTED{
		{
			name:            "empty string",
			input:           "",
			expectCorrected: false,
	REDACTED,
		{
			name:            "newline only",
			input:           "\n",
			expectCorrected: false,
	REDACTED,
		{
			name:            "invalid json",
			input:           "not a json",
			expectCorrected: false,
	REDACTED,
		{
			name:            "correct apply_patch in tool_calls",
			input:           `{"tool_calls":[{"function":{"name":"apply_patch","arguments":"{REDACTED"REDACTEDREDACTED]REDACTED`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
			REDACTED
				toolCalls, ok := payload["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in result")
			REDACTED
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
			REDACTED
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
			REDACTED
				if functionCall["name"] != "edit" {
					t.Errorf("Expected tool name 'edit', got '%v'", functionCall["name"])
			REDACTED
		REDACTED,
	REDACTED,
		{
			name:            "correct update_plan in function_call",
			input:           `{"function_call":{"name":"update_plan","arguments":"{REDACTED"REDACTEDREDACTED`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
			REDACTED
				functionCall, ok := payload["function_call"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function_call format")
			REDACTED
				if functionCall["name"] != "todowrite" {
					t.Errorf("Expected tool name 'todowrite', got '%v'", functionCall["name"])
			REDACTED
		REDACTED,
	REDACTED,
		{
			name:            "correct search_files in delta.tool_calls",
			input:           `{"delta":{"tool_calls":[{"function":{"name":"search_files"REDACTEDREDACTED]REDACTEDREDACTED`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
			REDACTED
				delta, ok := payload["delta"].(map[string]any)
				if !ok {
					t.Fatal("Invalid delta format")
			REDACTED
				toolCalls, ok := delta["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in delta")
			REDACTED
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
			REDACTED
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
			REDACTED
				if functionCall["name"] != "grep" {
					t.Errorf("Expected tool name 'grep', got '%v'", functionCall["name"])
			REDACTED
		REDACTED,
	REDACTED,
		{
			name:            "correct list_files in choices.message.tool_calls",
			input:           `{"choices":[{"message":{"tool_calls":[{"function":{"name":"list_files"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
			REDACTED
				choices, ok := payload["choices"].([]any)
				if !ok || len(choices) == 0 {
					t.Fatal("No choices found in result")
			REDACTED
				choice, ok := choices[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid choice format")
			REDACTED
				message, ok := choice["message"].(map[string]any)
				if !ok {
					t.Fatal("Invalid message format")
			REDACTED
				toolCalls, ok := message["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in message")
			REDACTED
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
			REDACTED
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
			REDACTED
				if functionCall["name"] != "glob" {
					t.Errorf("Expected tool name 'glob', got '%v'", functionCall["name"])
			REDACTED
		REDACTED,
	REDACTED,
		{
			name:            "no correction needed",
			input:           `{"tool_calls":[{"function":{"name":"read","arguments":"{REDACTED"REDACTEDREDACTED]REDACTED`,
			expectCorrected: false,
	REDACTED,
		{
			name:            "correct multiple tool calls",
			input:           `{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED,{"function":{"name":"read_file"REDACTEDREDACTED]REDACTED`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
			REDACTED
				toolCalls, ok := payload["tool_calls"].([]any)
				if !ok || len(toolCalls) < 2 {
					t.Fatal("Expected at least 2 tool_calls")
			REDACTED

				toolCall1, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid first tool_call format")
			REDACTED
				func1, ok := toolCall1["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid first function format")
			REDACTED
				if func1["name"] != "edit" {
					t.Errorf("Expected first tool name 'edit', got '%v'", func1["name"])
			REDACTED

				toolCall2, ok := toolCalls[1].(map[string]any)
				if !ok {
					t.Fatal("Invalid second tool_call format")
			REDACTED
				func2, ok := toolCall2["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid second function format")
			REDACTED
				if func2["name"] != "read" {
					t.Errorf("Expected second tool name 'read', got '%v'", func2["name"])
			REDACTED
		REDACTED,
	REDACTED,
		{
			name:            "camelCase format - applyPatch",
			input:           `{"tool_calls":[{"function":{"name":"applyPatch"REDACTEDREDACTED]REDACTED`,
			expectCorrected: true,
			checkFunc: func(t *testing.T, result string) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(result), &payload); err != nil {
					t.Fatalf("Failed to parse result: %v", err)
			REDACTED
				toolCalls, ok := payload["tool_calls"].([]any)
				if !ok || len(toolCalls) == 0 {
					t.Fatal("No tool_calls found in result")
			REDACTED
				toolCall, ok := toolCalls[0].(map[string]any)
				if !ok {
					t.Fatal("Invalid tool_call format")
			REDACTED
				functionCall, ok := toolCall["function"].(map[string]any)
				if !ok {
					t.Fatal("Invalid function format")
			REDACTED
				if functionCall["name"] != "edit" {
					t.Errorf("Expected tool name 'edit', got '%v'", functionCall["name"])
			REDACTED
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, corrected := corrector.CorrectToolCallsInSSEData(tt.input)

			if corrected != tt.expectCorrected {
				t.Errorf("Expected corrected=%v, got %v", tt.expectCorrected, corrected)
		REDACTED

			if !corrected && result != tt.input {
				t.Errorf("Expected unchanged result when not corrected")
		REDACTED

			if tt.checkFunc != nil {
				tt.checkFunc(t, result)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCorrectToolName(t *testing.T) {
	tests := []struct {
		input     string
		expected  string
		corrected bool
REDACTED{
		{"apply_patch", "edit", trueREDACTED,
		{"applyPatch", "edit", trueREDACTED,
		{"update_plan", "todowrite", trueREDACTED,
		{"updatePlan", "todowrite", trueREDACTED,
		{"read_plan", "todoread", trueREDACTED,
		{"readPlan", "todoread", trueREDACTED,
		{"search_files", "grep", trueREDACTED,
		{"searchFiles", "grep", trueREDACTED,
		{"list_files", "glob", trueREDACTED,
		{"listFiles", "glob", trueREDACTED,
		{"read_file", "read", trueREDACTED,
		{"readFile", "read", trueREDACTED,
		{"write_file", "write", trueREDACTED,
		{"writeFile", "write", trueREDACTED,
		{"execute_bash", "bash", trueREDACTED,
		{"executeBash", "bash", trueREDACTED,
		{"exec_bash", "bash", trueREDACTED,
		{"execBash", "bash", trueREDACTED,
		{"unknown_tool", "unknown_tool", falseREDACTED,
		{"read", "read", falseREDACTED,
		{"edit", "edit", falseREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, corrected := CorrectToolName(tt.input)

			if corrected != tt.corrected {
				t.Errorf("Expected corrected=%v, got %v", tt.corrected, corrected)
		REDACTED

			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestGetToolNameMapping(t *testing.T) {
	mapping := GetToolNameMapping()

	expectedMappings := map[string]string{
		"apply_patch":  "edit",
		"update_plan":  "todowrite",
		"read_plan":    "todoread",
		"search_files": "grep",
		"list_files":   "glob",
REDACTED

	for from, to := range expectedMappings {
		if mapping[from] != to {
			t.Errorf("Expected mapping[%s] = %s, got %s", from, to, mapping[from])
	REDACTED
REDACTED

	mapping["test_tool"] = "test_value"
	newMapping := GetToolNameMapping()
	if _, exists := newMapping["test_tool"]; exists {
		t.Error("Modifications to returned mapping should not affect original")
REDACTED
REDACTED

func TestCorrectorStats(t *testing.T) {
	corrector := NewCodexToolCorrector()

	stats := corrector.GetStats()
	if stats.TotalCorrected != 0 {
		t.Errorf("Expected TotalCorrected=0, got %d", stats.TotalCorrected)
REDACTED
	if len(stats.CorrectionsByTool) != 0 {
		t.Errorf("Expected empty CorrectionsByTool, got length %d", len(stats.CorrectionsByTool))
REDACTED

	corrector.CorrectToolCallsInSSEData(`{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTED`)
	corrector.CorrectToolCallsInSSEData(`{"tool_calls":[{"function":{"name":"apply_patch"REDACTEDREDACTED]REDACTED`)
	corrector.CorrectToolCallsInSSEData(`{"tool_calls":[{"function":{"name":"update_plan"REDACTEDREDACTED]REDACTED`)

	stats = corrector.GetStats()
	if stats.TotalCorrected != 3 {
		t.Errorf("Expected TotalCorrected=3, got %d", stats.TotalCorrected)
REDACTED

	if stats.CorrectionsByTool["apply_patch->edit"] != 2 {
		t.Errorf("Expected apply_patch->edit count=2, got %d", stats.CorrectionsByTool["apply_patch->edit"])
REDACTED

	if stats.CorrectionsByTool["update_plan->todowrite"] != 1 {
		t.Errorf("Expected update_plan->todowrite count=1, got %d", stats.CorrectionsByTool["update_plan->todowrite"])
REDACTED

	corrector.ResetStats()
	stats = corrector.GetStats()
	if stats.TotalCorrected != 0 {
		t.Errorf("Expected TotalCorrected=0 after reset, got %d", stats.TotalCorrected)
REDACTED
	if len(stats.CorrectionsByTool) != 0 {
		t.Errorf("Expected empty CorrectionsByTool after reset, got length %d", len(stats.CorrectionsByTool))
REDACTED
REDACTED

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
								"arguments": "{\"file\":\"test.go\"REDACTED"
						REDACTED
					REDACTED
					]
			REDACTED,
				"finish_reason": null
		REDACTED
		]
REDACTED`

	result, corrected := corrector.CorrectToolCallsInSSEData(input)

	if !corrected {
		t.Error("Expected data to be corrected")
REDACTED

	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
REDACTED

	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		t.Fatal("No choices found in result")
REDACTED
	choice, ok := choices[0].(map[string]any)
	if !ok {
		t.Fatal("Invalid choice format")
REDACTED
	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		t.Fatal("Invalid delta format")
REDACTED
	toolCalls, ok := delta["tool_calls"].([]any)
	if !ok || len(toolCalls) == 0 {
		t.Fatal("No tool_calls found in delta")
REDACTED
	toolCall, ok := toolCalls[0].(map[string]any)
	if !ok {
		t.Fatal("Invalid tool_call format")
REDACTED
	function, ok := toolCall["function"].(map[string]any)
	if !ok {
		t.Fatal("Invalid function format")
REDACTED

	if function["name"] != "edit" {
		t.Errorf("Expected tool name 'edit', got '%v'", function["name"])
REDACTED
REDACTED

// TestCorrectToolParameters 测试工具参数修正
func TestCorrectToolParameters(t *testing.T) {
	corrector := NewCodexToolCorrector()

	tests := []struct {
		name     string
		input    string
		expected map[string]bool // key: 期待存在的参数, value: true表示应该存在
REDACTED{
		{
			name: "rename work_dir to workdir in bash tool",
			input: `{
				"tool_calls": [{
					"function": {
						"name": "bash",
						"arguments": "{\"command\":\"ls\",\"work_dir\":\"/tmp\"REDACTED"
				REDACTED
			REDACTED]
		REDACTED`,
			expected: map[string]bool{
				"command":  true,
				"workdir":  true,
				"work_dir": false,
		REDACTED,
	REDACTED,
		{
			name: "rename snake_case edit params to camelCase",
			input: `{
				"tool_calls": [{
					"function": {
						"name": "apply_patch",
						"arguments": "{\"path\":\"/foo/bar.go\",\"old_string\":\"old\",\"new_string\":\"new\"REDACTED"
				REDACTED
			REDACTED]
		REDACTED`,
			expected: map[string]bool{
				"filePath":   true,
				"path":       false,
				"oldString":  true,
				"old_string": false,
				"newString":  true,
				"new_string": false,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrected, changed := corrector.CorrectToolCallsInSSEData(tt.input)
			if !changed {
				t.Error("expected data to be corrected")
		REDACTED

			// 解析修正后的数据
			var result map[string]any
			if err := json.Unmarshal([]byte(corrected), &result); err != nil {
				t.Fatalf("failed to parse corrected data: %v", err)
		REDACTED

			// 检查工具调用
			toolCalls, ok := result["tool_calls"].([]any)
			if !ok || len(toolCalls) == 0 {
				t.Fatal("no tool_calls found in corrected data")
		REDACTED

			toolCall, ok := toolCalls[0].(map[string]any)
			if !ok {
				t.Fatal("invalid tool_call structure")
		REDACTED

			function, ok := toolCall["function"].(map[string]any)
			if !ok {
				t.Fatal("no function found in tool_call")
		REDACTED

			argumentsStr, ok := function["arguments"].(string)
			if !ok {
				t.Fatal("arguments is not a string")
		REDACTED

			var args map[string]any
			if err := json.Unmarshal([]byte(argumentsStr), &args); err != nil {
				t.Fatalf("failed to parse arguments: %v", err)
		REDACTED

			// 验证期望的参数
			for param, shouldExist := range tt.expected {
				_, exists := args[param]
				if shouldExist && !exists {
					t.Errorf("expected parameter %q to exist, but it doesn't", param)
			REDACTED
				if !shouldExist && exists {
					t.Errorf("expected parameter %q to not exist, but it does", param)
			REDACTED
		REDACTED
	REDACTED)
REDACTED
REDACTED
