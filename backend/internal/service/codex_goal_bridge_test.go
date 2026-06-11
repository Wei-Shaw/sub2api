package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestExtractCodexGoalObjectiveOpenAIResponses(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.1-codex",
		"instructions": "Be concise.",
		"input": [
			{"role": "user", "content": [{"type": "input_text", "text": "Refactor the billing path."}]}
		]
	}`)

	objective, model, stream, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if stream {
		t.Fatal("stream = true, want false")
	}
	if model != "gpt-5.1-codex" {
		t.Fatalf("model = %q, want gpt-5.1-codex", model)
	}
	for _, want := range []string{"Instructions:", "Be concise.", "User:", "Refactor the billing path."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveOpenAIResponsesImageOnly(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"input": [
			{"type":"message","role":"user","content":[
				{"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo="}
			]}
		]
	}`)

	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if !strings.Contains(objective, "Image attachment") {
		t.Fatalf("objective %q does not contain image placeholder", objective)
	}
}

func TestExtractCodexGoalObjectiveOpenAIChatFileOnly(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"messages": [
			{"role":"user","content":[
				{"type":"input_file","filename":"notes.txt","file_data":"data:text/plain;base64,aGVsbG8="}
			]}
		]
	}`)

	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIChat,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if !strings.Contains(objective, "File attachment") || !strings.Contains(objective, "notes.txt") {
		t.Fatalf("objective %q does not contain file placeholder", objective)
	}
}

func TestExtractCodexGoalObjectiveOpenAIResponsesWebSocketCreate(t *testing.T) {
	body := []byte(`{
		"type": "response.create",
		"model": "gpt-5.3-codex",
		"input": [
			{"type": "input_text", "text": "Handle the websocket request."}
		]
	}`)

	objective, model, stream, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Endpoint: "/responses",
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if model != "gpt-5.3-codex" {
		t.Fatalf("model = %q, want gpt-5.3-codex", model)
	}
	if stream {
		t.Fatal("stream = true, want false")
	}
	if !strings.Contains(objective, "Handle the websocket request.") {
		t.Fatalf("objective %q does not contain websocket input text", objective)
	}
}

func TestExtractCodexGoalObjectiveOpenAIResponsesNestedWebSocketCreate(t *testing.T) {
	body := []byte(`{
		"type": "response.create",
		"response": {
			"model": "gpt-5.4-codex",
			"instructions": "Use the goal bridge.",
			"input": [
				{"role": "user", "content": [{"type": "input_text", "text": "Handle nested websocket payload."}]}
			]
		}
	}`)

	objective, model, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Endpoint: "/responses",
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if model != "gpt-5.4-codex" {
		t.Fatalf("model = %q, want gpt-5.4-codex", model)
	}
	for _, want := range []string{"Use the goal bridge.", "Handle nested websocket payload."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveOpenAIChat(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "system", "content": "Use the uploaded account."},
			{"role": "user", "content": "Ship the bridge."}
		]
	}`)

	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIChat,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"System:", "Use the uploaded account.", "User:", "Ship the bridge."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveOpenAIChatTools(t *testing.T) {
	objective, _, stream, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIChat,
		Body: []byte(`{
			"stream": true,
			"tools": [{
				"type": "function",
				"function": {
					"name": "lookup",
					"description": "Look up a value.",
					"parameters": {"type":"object","properties":{"q":{"type":"string"}}}
				}
			}],
			"messages": [{"role":"user","content":"call lookup"}]
		}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if !stream {
		t.Fatal("stream = false, want true")
	}
	if len(features.FunctionTools) != 1 || features.FunctionTools[0].Name != "lookup" {
		t.Fatalf("FunctionTools = %#v, want lookup", features.FunctionTools)
	}
	for _, want := range []string{"User:", "call lookup", "Available client function tools:", "lookup", "codex_goal_function_call"} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveOpenAIChatPreservesToolContext(t *testing.T) {
	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIChat,
		Body: []byte(`{
			"messages": [
				{"role":"user","content":"lookup beta"},
				{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"beta\"}"}}]},
				{"role":"tool","tool_call_id":"call_1","content":"lookup result beta"},
				{"role":"user","content":"answer now"}
			]
		}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"Prior assistant function call:", "name: lookup", "call_id: call_1", `{"q":"beta"}`, "Tool result:", "lookup result beta", "answer now"} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveAnthropicMessages(t *testing.T) {
	body := []byte(`{
		"system": "No destructive commands.",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Audit the API routes."}]}
		]
	}`)

	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolAnthropic,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"System:", "No destructive commands.", "User:", "Audit the API routes."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveAnthropicTools(t *testing.T) {
	body := []byte(`{
		"tools": [{
			"name": "lookup_marker",
			"description": "Look up a marker.",
			"input_schema": {
				"type": "object",
				"properties": {"key": {"type": "string"}},
				"required": ["key"]
			}
		}],
		"tool_choice": {"type": "tool", "name": "lookup_marker"},
		"messages": [
			{"role": "assistant", "content": [{"type":"tool_use","id":"toolu_1","name":"lookup_marker","input":{"key":"alpha"}}]},
			{"role": "user", "content": [{"type":"tool_result","tool_use_id":"toolu_1","content":"marker-alpha"}]},
			{"role": "user", "content": [{"type": "text", "text": "Use the tool."}]}
		]
	}`)

	objective, _, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolAnthropic,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if len(features.FunctionTools) != 1 || features.FunctionTools[0].Name != "lookup_marker" {
		t.Fatalf("FunctionTools = %#v, want lookup_marker", features.FunctionTools)
	}
	if !strings.Contains(features.FunctionTools[0].ParametersJSON, `"key"`) {
		t.Fatalf("ParametersJSON = %q, want key schema", features.FunctionTools[0].ParametersJSON)
	}
	for _, want := range []string{
		"Available client function tools:",
		"Tool choice: you must call function lookup_marker.",
		"Prior assistant tool call:",
		"call_id: toolu_1",
		`{"key":"alpha"}`,
		"Tool result:",
		"marker-alpha",
		"Use the tool.",
	} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveGeminiGenerateContent(t *testing.T) {
	body := []byte(`{
		"system_instruction": {"parts": [{"text": "Keep changes scoped."}]},
		"contents": [
			{"role": "user", "parts": [{"text": "Implement the compatibility layer."}]}
		]
	}`)

	objective, model, stream, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolGemini,
		Endpoint: "/gemini-2.5-pro:generateContent",
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if model != "gemini-2.5-pro" {
		t.Fatalf("model = %q, want gemini-2.5-pro", model)
	}
	if stream {
		t.Fatal("stream = true, want false")
	}
	for _, want := range []string{"System:", "Keep changes scoped.", "User:", "Implement the compatibility layer."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveGeminiTools(t *testing.T) {
	body := []byte(`{
		"tools": [{
			"functionDeclarations": [{
				"name": "lookup_marker",
				"description": "Look up a marker.",
				"parameters": {
					"type": "object",
					"properties": {"key": {"type": "string"}},
					"required": ["key"]
				}
			}]
		}, {
			"googleSearch": {}
		}],
		"toolConfig": {
			"functionCallingConfig": {
				"mode": "ANY",
				"allowedFunctionNames": ["lookup_marker"]
			}
		},
		"contents": [
			{"role": "model", "parts": [{"functionCall": {"id": "func_1", "name": "lookup_marker", "args": {"key": "alpha"}}}]},
			{"role": "user", "parts": [{"functionResponse": {"id": "func_1", "name": "lookup_marker", "response": {"content": "marker-alpha"}}}]},
			{"role": "user", "parts": [{"text": "Use the tool."}]}
		]
	}`)

	objective, _, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolGemini,
		Endpoint: "/gemini-2.5-pro:generateContent",
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if !features.EnableWebSearch {
		t.Fatal("EnableWebSearch = false, want true")
	}
	if len(features.FunctionTools) != 1 || features.FunctionTools[0].Name != "lookup_marker" {
		t.Fatalf("FunctionTools = %#v, want lookup_marker", features.FunctionTools)
	}
	if features.ToolChoice.Name != "lookup_marker" {
		t.Fatalf("ToolChoice = %#v, want lookup_marker", features.ToolChoice)
	}
	for _, want := range []string{
		"Available hosted tool:",
		"google_search",
		"Available client function tools:",
		"Tool choice: you must call function lookup_marker.",
		"Prior assistant function call:",
		"call_id: func_1",
		`{"key":"alpha"}`,
		"Tool result:",
		"marker-alpha",
		"Use the tool.",
	} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectivePreservesStreaming(t *testing.T) {
	objective, _, stream, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIChat,
		Body:     []byte(`{"stream": true, "messages": [{"role": "user", "content": "hi"}]}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if !stream {
		t.Fatal("stream = false, want true")
	}
	if !strings.Contains(objective, "hi") {
		t.Fatalf("objective %q does not contain input text", objective)
	}
}

func TestExtractCodexGoalObjectiveAndFeaturesResponsesTools(t *testing.T) {
	body := []byte(`{
		"model": "gpt-5.5",
		"tools": [
			{"type": "web_search"},
			{
				"type": "mcp",
				"server_label": "dmcp",
				"server_description": "Dice roller.",
				"server_url": "https://dmcp-server.example/sse",
				"authorization": "token-123",
				"require_approval": "never",
				"allowed_tools": ["roll"]
			}
		],
		"input": [
			{"type": "input_text", "text": "Roll and check current rules."}
		]
	}`)

	objective, model, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if model != "gpt-5.5" {
		t.Fatalf("model = %q, want gpt-5.5", model)
	}
	if !features.EnableWebSearch {
		t.Fatal("EnableWebSearch = false, want true")
	}
	if len(features.MCPServers) != 1 {
		t.Fatalf("got %d MCP servers, want 1", len(features.MCPServers))
	}
	server := features.MCPServers[0]
	if server.Label != "dmcp" || server.URL != "https://dmcp-server.example/sse" {
		t.Fatalf("server = %#v, want dmcp server URL", server)
	}
	if server.Command != "codex-mcp-sse-proxy" || len(server.Args) < 1 || server.Args[0] != "https://dmcp-server.example/sse" {
		t.Fatalf("server command/args = %q %#v, want SSE proxy URL adapter", server.Command, server.Args)
	}
	if server.StartupTimeout != 60 {
		t.Fatalf("StartupTimeout = %d, want 60", server.StartupTimeout)
	}
	if got := server.HTTPHeaders["Authorization"]; got != "Bearer token-123" {
		t.Fatalf("Authorization header = %q, want Bearer token-123", got)
	}
	if !containsString(server.Args, "Authorization: Bearer token-123") {
		t.Fatalf("server args = %#v, want Authorization header arg", server.Args)
	}
	if len(server.EnabledTools) != 1 || server.EnabledTools[0] != "roll" {
		t.Fatalf("EnabledTools = %#v, want [roll]", server.EnabledTools)
	}
	for _, want := range []string{"Available hosted tool:", "web_search", "Available remote MCP server:", "dmcp", "Roll and check current rules."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveRejectsPreviousResponseID(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	_, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"previous_response_id":"resp_123","input":"continue"}`),
	})
	if err == nil {
		t.Fatal("ExtractCodexGoalObjective returned nil error for previous_response_id")
	}
	if !strings.Contains(err.Error(), "previous_response_id") {
		t.Fatalf("error = %v, want previous_response_id message", err)
	}
}

func TestExtractCodexGoalObjectiveUsesPreviousResponseContext(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	saveCodexGoalStoredResponse(codexGoalStoredResponse{
		ID:        "resp_codex_goal_previous",
		Protocol:  CodexGoalProtocolOpenAIResponses,
		Model:     "gpt-5.5",
		Objective: "User:\nRemember alpha.",
		Text:      "Alpha remembered.",
		CreatedAt: time.Unix(1710000000, 0),
	})
	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"previous_response_id":"resp_codex_goal_previous","input":"What did I ask you to remember?"}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"Previous response context", "Remember alpha.", "Alpha remembered.", "Current request:", "What did I ask you to remember?"} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveSupportsResponsesFunctionTool(t *testing.T) {
	objective, _, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"tools":[{"type":"function","name":"lookup","description":"Look up a value","parameters":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}}],"input":"call lookup"}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if len(features.FunctionTools) != 1 || features.FunctionTools[0].Name != "lookup" {
		t.Fatalf("FunctionTools = %#v, want lookup", features.FunctionTools)
	}
	for _, want := range []string{"Available client function tools:", "lookup", "codex_goal_function_call", `"q":{"type":"string"}`} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveRejectsUnsupportedResponsesTool(t *testing.T) {
	_, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"tools":[{"type":"file_search","vector_store_ids":["vs_123"]}],"input":"search files"}`),
	})
	if err == nil {
		t.Fatal("ExtractCodexGoalObjective returned nil error for unsupported Responses tool")
	}
	if !strings.Contains(err.Error(), "file_search") {
		t.Fatalf("error = %v, want unsupported file_search tool message", err)
	}
}

func TestExtractCodexGoalObjectiveSupportsForcedResponsesHostedToolChoice(t *testing.T) {
	objective, _, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"tools":[{"type":"web_search"}],"tool_choice":"required","input":"must search"}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if !features.EnableWebSearch {
		t.Fatal("EnableWebSearch = false, want true")
	}
	for _, want := range []string{"Tool choice for hosted/MCP tools:", "requires at least one enabled hosted or MCP tool call", "web_search"} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveSupportsForcedResponsesMCPToolChoice(t *testing.T) {
	objective, _, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body: []byte(`{
			"tools":[{"type":"mcp","server_label":"demo","server_url":"https://demo-day.mcp.cloudflare.com/sse","require_approval":"never","allowed_tools":["mcp_demo_day_info"]}],
			"tool_choice":{"type":"mcp","name":"mcp_demo_day_info"},
			"input":"use mcp"
		}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if len(features.MCPServers) != 1 || features.ToolChoice.Name != "mcp_demo_day_info" {
		t.Fatalf("features = %#v", features)
	}
	for _, want := range []string{"Tool choice for hosted/MCP tools:", "explicitly selected tool mcp_demo_day_info", "MCP server demo exposes allowed tools: mcp_demo_day_info"} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestExtractCodexGoalObjectiveSupportsStreamingResponsesFunctionTool(t *testing.T) {
	objective, _, stream, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"stream":true,"tools":[{"type":"function","name":"lookup"}],"input":"call lookup"}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	if !stream {
		t.Fatal("stream = false, want true")
	}
	if !strings.Contains(objective, "codex_goal_function_call") {
		t.Fatalf("objective %q does not contain function-call wrapper instructions", objective)
	}
}

func TestExtractCodexGoalObjectiveResponsesToolChoiceNoneDisablesTools(t *testing.T) {
	objective, _, _, features, err := ExtractCodexGoalObjectiveAndFeatures(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"tools":[{"type":"web_search"},{"type":"function","name":"lookup"}],"tool_choice":"none","input":"answer directly"}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjectiveAndFeatures returned error: %v", err)
	}
	if features.EnableWebSearch || len(features.MCPServers) != 0 || len(features.FunctionTools) != 0 {
		t.Fatalf("features = %#v, want no enabled tools", features)
	}
	if strings.Contains(objective, "Available hosted tool:") {
		t.Fatalf("objective %q should not advertise disabled tools", objective)
	}
}

func TestExtractCodexGoalObjectiveRejectsUnsupportedToolingByProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		body     string
		want     string
	}{
		{
			name:     "gemini unsupported tool",
			protocol: CodexGoalProtocolGemini,
			body:     `{"tools":[{"codeExecution":{}}],"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`,
			want:     "Gemini tool",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
				Protocol: tt.protocol,
				Body:     []byte(tt.body),
			})
			if err == nil {
				t.Fatal("ExtractCodexGoalObjective returned nil error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestParseCodexGoalFunctionCallOutput(t *testing.T) {
	text, calls := parseCodexGoalFunctionCallOutput(
		`<codex_goal_function_call>{"name":"lookup","arguments":{"q":"alpha"}}</codex_goal_function_call>`,
		[]CodexGoalFunctionToolConfig{{Name: "lookup"}},
		time.Unix(1710000000, 0),
	)
	if text != "" {
		t.Fatalf("text = %q, want empty text", text)
	}
	if len(calls) != 1 {
		t.Fatalf("got %d calls, want 1: %#v", len(calls), calls)
	}
	if calls[0].Name != "lookup" || calls[0].Arguments != `{"q":"alpha"}` || calls[0].CallID == "" {
		t.Fatalf("call = %#v", calls[0])
	}
}

func TestInputSequenceTextPreservesPriorToolContext(t *testing.T) {
	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body: []byte(`{
			"input": [
				{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": "{\"q\":\"x\"}"},
				{"type": "function_call_output", "call_id": "call_1", "output": "lookup result"},
				{"role": "user", "content": [{"type": "input_text", "text": "Use that result."}]}
			]
		}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"Prior assistant function call:", "name: lookup", "Tool result:", "output: lookup result", "Use that result."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestBuildCodexGoalAuthJSON(t *testing.T) {
	account := &Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":       "access-token",
			"refresh_token":      "refresh-token",
			"id_token":           "id-token",
			"chatgpt_account_id": "acct_123",
		},
		Extra: map[string]any{
			"import_source": "codex_session",
		},
	}
	if !IsCodexGoalSessionAccount(account) {
		t.Fatal("account should be accepted as a Codex session account")
	}
	data, err := BuildCodexGoalAuthJSON(account)
	if err != nil {
		t.Fatalf("BuildCodexGoalAuthJSON returned error: %v", err)
	}
	var payload struct {
		AuthMode     string  `json:"auth_mode"`
		OpenAIAPIKey *string `json:"OPENAI_API_KEY"`
		Tokens       struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("auth JSON did not decode: %v", err)
	}
	if payload.AuthMode != "chatgpt" {
		t.Fatalf("auth_mode = %q, want chatgpt", payload.AuthMode)
	}
	if payload.OpenAIAPIKey != nil {
		t.Fatalf("OPENAI_API_KEY = %v, want nil", *payload.OpenAIAPIKey)
	}
	if payload.Tokens.AccessToken != "access-token" || payload.Tokens.RefreshToken != "refresh-token" || payload.Tokens.IDToken != "id-token" || payload.Tokens.AccountID != "acct_123" {
		t.Fatalf("tokens = %#v, want imported Codex session tokens", payload.Tokens)
	}
}

func TestBuildCodexGoalAuthJSONAcceptsOpenAIOAuthAccount(t *testing.T) {
	account := &Account{
		ID:          43,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":       "access-token",
			"refresh_token":      "refresh-token",
			"id_token":           "id-token",
			"chatgpt_account_id": "acct_from_oauth",
		},
	}
	if !IsCodexGoalSessionAccount(account) {
		t.Fatal("OpenAI OAuth account with Codex-compatible tokens should be accepted")
	}
	data, err := BuildCodexGoalAuthJSON(account)
	if err != nil {
		t.Fatalf("BuildCodexGoalAuthJSON returned error: %v", err)
	}
	if !strings.Contains(string(data), "acct_from_oauth") {
		t.Fatalf("auth JSON %s does not contain account id", data)
	}
}

func TestBuildCodexGoalAuthJSONRejectsIncompleteOpenAIOAuthAccount(t *testing.T) {
	account := &Account{
		ID:          44,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":       "access-token",
			"chatgpt_account_id": "acct_missing_tokens",
		},
	}
	if IsCodexGoalSessionAccount(account) {
		t.Fatal("OpenAI OAuth account without refresh/id tokens should be rejected")
	}
	if _, err := BuildCodexGoalAuthJSON(account); err == nil {
		t.Fatal("BuildCodexGoalAuthJSON returned nil error for incomplete account")
	}
}

func TestExtractCodexGoalAuthJSONCredentials(t *testing.T) {
	accessToken := codexGoalTestJWT(`{"exp":1710003600}`)
	data, err := json.Marshal(map[string]any{
		"auth_mode":    "chatgpt",
		"last_refresh": "2026-06-11T01:02:03Z",
		"tokens": map[string]any{
			"access_token":  accessToken,
			"refresh_token": "new-refresh-token",
			"id_token":      "new-id-token",
			"account_id":    "acct_refreshed",
		},
	})
	if err != nil {
		t.Fatalf("marshal auth JSON: %v", err)
	}

	credentials, err := extractCodexGoalAuthJSONCredentials(data)
	if err != nil {
		t.Fatalf("extractCodexGoalAuthJSONCredentials returned error: %v", err)
	}
	if credentials["access_token"] != accessToken ||
		credentials["refresh_token"] != "new-refresh-token" ||
		credentials["id_token"] != "new-id-token" ||
		credentials["chatgpt_account_id"] != "acct_refreshed" ||
		credentials["auth_mode"] != "chatgpt" ||
		credentials["last_refresh"] != "2026-06-11T01:02:03Z" {
		t.Fatalf("credentials = %#v", credentials)
	}
	wantExpiresAt := time.Unix(1710003600, 0).UTC().Format(time.RFC3339)
	if credentials["expires_at"] != wantExpiresAt {
		t.Fatalf("expires_at = %#v, want %s", credentials["expires_at"], wantExpiresAt)
	}
}

func TestMergeCodexGoalRefreshedCredentialsDropsStaleExpiryWhenJWTExpiryUnknown(t *testing.T) {
	account := codexGoalBridgeTestAccount()
	account.Credentials["expires_at"] = "2024-01-01T00:00:00Z"

	merged, changed := mergeCodexGoalRefreshedCredentials(&account, map[string]any{
		"access_token": "opaque-new-access-token",
	})
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if merged["access_token"] != "opaque-new-access-token" {
		t.Fatalf("access_token = %#v", merged["access_token"])
	}
	if _, ok := merged["expires_at"]; ok {
		t.Fatalf("expires_at should be removed when access token changes without a new expiry: %#v", merged)
	}
}

func TestCodexGoalBridgeServicePersistsRefreshedCredentials(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	account := codexGoalBridgeTestAccount()
	account.Credentials["custom"] = "keep"
	accessToken := codexGoalTestJWT(`{"exp":1710007200}`)
	repo := &codexGoalRefreshingAccountRepo{
		stubOpenAIAccountRepo: stubOpenAIAccountRepo{
			accounts: []Account{account},
		},
	}
	runner := &fakeCodexGoalStreamingRunner{
		deltas: []string{"refreshed answer"},
		refreshedCredentials: map[string]any{
			"access_token":       accessToken,
			"refresh_token":      "rotated-refresh-token",
			"id_token":           "rotated-id-token",
			"chatgpt_account_id": "acct_rotated",
			"auth_mode":          "chatgpt",
			"expires_at":         time.Unix(1710007200, 0).UTC().Format(time.RFC3339),
		},
	}
	svc := NewCodexGoalBridgeService(repo, cfg)
	svc.SetRunnerForTesting(runner)

	result, err := svc.Handle(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"model":"gpt-5.5","input":"use the codex account"}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if result.Text != "refreshed answer" {
		t.Fatalf("result.Text = %q", result.Text)
	}
	if repo.updateCredentialsCalls != 1 || repo.updatedID != account.ID {
		t.Fatalf("UpdateCredentials calls = %d id = %d", repo.updateCredentialsCalls, repo.updatedID)
	}
	updated := repo.updatedCredentials
	if updated["access_token"] != accessToken ||
		updated["refresh_token"] != "rotated-refresh-token" ||
		updated["id_token"] != "rotated-id-token" ||
		updated["chatgpt_account_id"] != "acct_rotated" ||
		updated["auth_mode"] != "chatgpt" ||
		updated["custom"] != "keep" {
		t.Fatalf("updated credentials = %#v", updated)
	}
	if result.Account.GetOpenAIAccessToken() != accessToken {
		t.Fatalf("result account access token was not updated")
	}
}

func TestCodexGoalBridgeServiceStagesResponsesImageDataURL(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	tmp := t.TempDir()
	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5
	cfg.Gateway.CodexGoalBridge.CWD = tmp

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{"saw attachment"}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	_, err := svc.Handle(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body: []byte(`{
			"model":"gpt-5.5",
			"input":[{"type":"message","role":"user","content":[
				{"type":"input_text","text":"Describe the image."},
				{"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo="}
			]}]
		}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	for _, want := range []string{"Describe the image.", "Attachments available", "path=", "image/png"} {
		if !strings.Contains(runner.input.Objective, want) {
			t.Fatalf("objective %q does not contain %q", runner.input.Objective, want)
		}
	}
	matches, err := filepath.Glob(filepath.Join(tmp, ".codex-goal-bridge", "attachments", "*", "*.png"))
	if err != nil {
		t.Fatalf("glob staged attachments: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("staged png files = %v, want one", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read staged attachment: %v", err)
	}
	wantData := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if string(data) != string(wantData) {
		t.Fatalf("staged data = %v, want %v", data, wantData)
	}
}

func TestCodexGoalBridgeServiceStagesUploadedFileID(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	tmp := t.TempDir()
	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5
	cfg.Gateway.CodexGoalBridge.CWD = tmp

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{"read file"}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	stored, err := svc.StoreUploadedFile(context.Background(), "notes.txt", "text/plain", "assistants", strings.NewReader("hello from upload"))
	if err != nil {
		t.Fatalf("StoreUploadedFile returned error: %v", err)
	}
	if stored.ID == "" || stored.Path == "" {
		t.Fatalf("stored file missing id/path: %#v", stored)
	}

	_, err = svc.Handle(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body: []byte(`{
			"model":"gpt-5.5",
			"input":[{"type":"message","role":"user","content":[
				{"type":"input_text","text":"Summarize the uploaded file."},
				{"type":"input_file","file_id":"` + stored.ID + `"}
			]}]
		}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	for _, want := range []string{stored.ID, stored.Path, "Attachments available", "notes.txt"} {
		if !strings.Contains(runner.input.Objective, want) {
			t.Fatalf("objective %q does not contain %q", runner.input.Objective, want)
		}
	}
}

func TestCodexGoalBridgeServiceLimitsLongObjectiveBeforeRunner(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	tmp := t.TempDir()
	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5
	cfg.Gateway.CodexGoalBridge.CWD = tmp

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{"trimmed"}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	stored, err := svc.StoreUploadedFile(context.Background(), "report.txt", "text/plain", "assistants", strings.NewReader("important file"))
	if err != nil {
		t.Fatalf("StoreUploadedFile returned error: %v", err)
	}
	saveCodexGoalStoredResponse(codexGoalStoredResponse{
		ID:        "resp_long_context",
		Protocol:  CodexGoalProtocolOpenAIResponses,
		Model:     "gpt-5.5",
		Objective: strings.Repeat("old objective context ", 300),
		Text:      strings.Repeat("old assistant answer ", 300),
		CreatedAt: time.Now(),
	})

	_, err = svc.Handle(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body: []byte(`{
			"model":"gpt-5.5",
			"previous_response_id":"resp_long_context",
			"input":[{"type":"message","role":"user","content":[
				{"type":"input_text","text":"Summarize the attached file in one sentence."},
				{"type":"input_file","file_id":"` + stored.ID + `"}
			]}]
		}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if got := runeLenCodexGoal(runner.input.Objective); got > codexGoalObjectiveMaxChars {
		t.Fatalf("runner objective length = %d, want <= %d", got, codexGoalObjectiveMaxChars)
	}
	for _, want := range []string{"shortened", "Current request:", "Summarize the attached file", stored.Path} {
		if !strings.Contains(runner.input.Objective, want) {
			t.Fatalf("objective %q does not contain %q", runner.input.Objective, want)
		}
	}
}

func TestCodexGoalBridgeServiceHandleStreamEmitsRunnerDeltas(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{"hel", "lo"}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	var events []CodexGoalBridgeStreamEvent
	result, err := svc.HandleStream(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"model":"gpt-5.3-codex","stream":true,"tools":[{"type":"web_search"}],"input":"say hello"}`),
	}, func(event CodexGoalBridgeStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("HandleStream returned error: %v", err)
	}
	if result.Text != "hello" {
		t.Fatalf("result.Text = %q, want hello", result.Text)
	}
	if !result.Stream {
		t.Fatal("result.Stream = false, want true")
	}
	if len(events) != 3 {
		t.Fatalf("got %d stream events, want 3: %#v", len(events), events)
	}
	if events[0].Type != CodexGoalBridgeStreamEventStart {
		t.Fatalf("first event type = %q, want start", events[0].Type)
	}
	if events[1].Type != CodexGoalBridgeStreamEventDelta || events[1].Delta != "hel" {
		t.Fatalf("second event = %#v, want hel delta", events[1])
	}
	if events[2].Type != CodexGoalBridgeStreamEventDelta || events[2].Delta != "lo" {
		t.Fatalf("third event = %#v, want lo delta", events[2])
	}
	if runner.input.Model != "gpt-5.3-codex" {
		t.Fatalf("runner model = %q, want gpt-5.3-codex", runner.input.Model)
	}
	if !runner.input.EnableWebSearch {
		t.Fatal("runner input EnableWebSearch = false, want true")
	}
}

func TestCodexGoalBridgeServiceHandleStreamEmitsToolEvents(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{
		deltas: []string{"done"},
		toolEvents: []CodexGoalToolEvent{{
			Type:        "mcp_call",
			ID:          "mcp_1",
			ServerLabel: "dmcp",
			Name:        "roll",
			Arguments:   `{"diceRollExpression":"2d4+1"}`,
			Output:      "7",
		}},
	}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	var events []CodexGoalBridgeStreamEvent
	result, err := svc.HandleStream(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"model":"gpt-5.5","stream":true,"tools":[{"type":"mcp","server_url":"https://dmcp-server.example/sse","server_label":"dmcp"}],"input":"roll"}`),
	}, func(event CodexGoalBridgeStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("HandleStream returned error: %v", err)
	}
	if result.Text != "done" || len(result.ToolEvents) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(events) != 3 {
		t.Fatalf("got %d stream events, want 3: %#v", len(events), events)
	}
	if events[2].Type != CodexGoalBridgeStreamEventToolEvent || events[2].ToolEvent.Name != "roll" || events[2].ToolEventIndex != 0 {
		t.Fatalf("tool stream event = %#v", events[2])
	}
}

func TestCodexGoalBridgeServiceStoresResponsesForPreviousResponseID(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{"stored answer"}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	result, err := svc.Handle(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"model":"gpt-5.5","input":"Remember beta."}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	responseID := codexGoalResponseID(result.CreatedAt)
	stored, ok := loadCodexGoalStoredResponse(responseID)
	if !ok {
		t.Fatalf("stored response %s not found", responseID)
	}
	if stored.Text != "stored answer" || !strings.Contains(stored.Objective, "Remember beta.") {
		t.Fatalf("stored response = %#v", stored)
	}

	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"previous_response_id":"` + responseID + `","input":"Continue."}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"Remember beta.", "stored answer", "Continue."} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestCodexGoalBridgeServicePersistsResponsesForPreviousResponseID(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	tempDir := t.TempDir()
	configureCodexGoalStoredResponsePersistence(tempDir)
	saveCodexGoalStoredResponse(codexGoalStoredResponse{
		ID:        "resp_codex_goal_persisted",
		Protocol:  CodexGoalProtocolOpenAIResponses,
		Model:     "gpt-5.5",
		Objective: "Remember persisted beta.",
		Text:      "persisted answer",
		ToolEvents: []CodexGoalToolEvent{{
			Type:        "mcp_call",
			ID:          "mcp_1",
			ServerLabel: "dmcp",
			Name:        "roll",
			Arguments:   `{"diceRollExpression":"2d4+1"}`,
			Output:      "7",
		}},
		CreatedAt: time.Unix(1710000000, 0),
	})

	resetCodexGoalStoredResponsesForTesting()
	configureCodexGoalStoredResponsePersistence(tempDir)

	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"previous_response_id":"resp_codex_goal_persisted","input":"What was the MCP result?"}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{
		"Remember persisted beta.",
		"persisted answer",
		"Previous MCP tool call:",
		"dmcp",
		"roll",
		`{"diceRollExpression":"2d4+1"}`,
		"7",
		"What was the MCP result?",
	} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestCodexGoalBridgeServiceReturnsFunctionCall(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{`<codex_goal_function_call>{"name":"lookup","arguments":{"q":"beta"}}</codex_goal_function_call>`}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	result, err := svc.Handle(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"model":"gpt-5.5","tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string"}}}}],"input":"call lookup for beta"}`),
	})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if result.Text != "" {
		t.Fatalf("result.Text = %q, want empty text for pure function call", result.Text)
	}
	if len(result.FunctionCalls) != 1 {
		t.Fatalf("got %d function calls, want 1: %#v", len(result.FunctionCalls), result.FunctionCalls)
	}
	call := result.FunctionCalls[0]
	if call.Name != "lookup" || call.Arguments != `{"q":"beta"}` || call.CallID == "" || call.ID == "" {
		t.Fatalf("function call = %#v", call)
	}

	responseID := codexGoalResponseID(result.CreatedAt)
	objective, _, _, err := ExtractCodexGoalObjective(CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"previous_response_id":"` + responseID + `","input":[{"type":"function_call_output","call_id":"` + call.CallID + `","output":"lookup result beta"}]}`),
	})
	if err != nil {
		t.Fatalf("ExtractCodexGoalObjective returned error: %v", err)
	}
	for _, want := range []string{"Previous assistant function call:", "lookup", call.CallID, `{"q":"beta"}`, "Tool result", "lookup result beta"} {
		if !strings.Contains(objective, want) {
			t.Fatalf("objective %q does not contain %q", objective, want)
		}
	}
}

func TestCodexGoalBridgeServiceStreamFunctionCallSuppressesRawDeltas(t *testing.T) {
	resetCodexGoalStoredResponsesForTesting()
	t.Cleanup(resetCodexGoalStoredResponsesForTesting)

	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{`<codex_goal_function_call>{"name":"lookup","arguments":{"q":"gamma"}}</codex_goal_function_call>`}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	var events []CodexGoalBridgeStreamEvent
	result, err := svc.HandleStream(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIResponses,
		Body:     []byte(`{"model":"gpt-5.5","stream":true,"tools":[{"type":"function","name":"lookup"}],"input":"call lookup for gamma"}`),
	}, func(event CodexGoalBridgeStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("HandleStream returned error: %v", err)
	}
	if len(events) != 4 || events[0].Type != CodexGoalBridgeStreamEventStart || !events[0].DeferResponsesOutputItem {
		t.Fatalf("events = %#v, want start plus function-call stream events", events)
	}
	if events[1].Type != CodexGoalBridgeStreamEventFunctionCallStart || events[1].FunctionCall.Name != "lookup" {
		t.Fatalf("function start event = %#v", events[1])
	}
	if events[2].Type != CodexGoalBridgeStreamEventFunctionArgumentsDelta || events[2].Delta != `{"q":"gamma"}` {
		t.Fatalf("arguments delta event = %#v", events[2])
	}
	if events[3].Type != CodexGoalBridgeStreamEventFunctionCallDone || events[3].FunctionCall.Arguments != `{"q":"gamma"}` {
		t.Fatalf("function done event = %#v", events[3])
	}
	for _, event := range events {
		if event.Type == CodexGoalBridgeStreamEventDelta {
			t.Fatalf("events = %#v, should not include raw text deltas", events)
		}
	}
	if len(result.FunctionCalls) != 1 || result.FunctionCalls[0].Arguments != `{"q":"gamma"}` {
		t.Fatalf("FunctionCalls = %#v", result.FunctionCalls)
	}
}

func TestCodexGoalBridgeServiceChatFunctionCall(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{`<codex_goal_function_call>{"name":"lookup","arguments":{"q":"chat"}}</codex_goal_function_call>`}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	var events []CodexGoalBridgeStreamEvent
	result, err := svc.HandleStream(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolOpenAIChat,
		Body:     []byte(`{"model":"gpt-5.5","stream":true,"tools":[{"type":"function","function":{"name":"lookup"}}],"messages":[{"role":"user","content":"call lookup"}]}`),
	}, func(event CodexGoalBridgeStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("HandleStream returned error: %v", err)
	}
	if len(events) != 4 || events[0].Type != CodexGoalBridgeStreamEventStart {
		t.Fatalf("events = %#v, want start plus function-call stream events", events)
	}
	if events[1].Type != CodexGoalBridgeStreamEventFunctionCallStart || events[1].FunctionCall.Name != "lookup" {
		t.Fatalf("function start event = %#v", events[1])
	}
	if events[2].Type != CodexGoalBridgeStreamEventFunctionArgumentsDelta || events[2].Delta != `{"q":"chat"}` {
		t.Fatalf("arguments delta event = %#v", events[2])
	}
	if events[3].Type != CodexGoalBridgeStreamEventFunctionCallDone || events[3].FunctionCall.Arguments != `{"q":"chat"}` {
		t.Fatalf("function done event = %#v", events[3])
	}
	if len(result.FunctionCalls) != 1 || result.FunctionCalls[0].Name != "lookup" || result.FunctionCalls[0].Arguments != `{"q":"chat"}` {
		t.Fatalf("FunctionCalls = %#v", result.FunctionCalls)
	}
}

func TestCodexGoalBridgeServiceGeminiFunctionCall(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.CodexGoalBridge.Enabled = true
	cfg.Gateway.CodexGoalBridge.TimeoutSeconds = 5

	runner := &fakeCodexGoalStreamingRunner{deltas: []string{`<codex_goal_function_call>{"name":"lookup","arguments":{"q":"gemini"}}</codex_goal_function_call>`}}
	svc := NewCodexGoalBridgeService(stubOpenAIAccountRepo{
		accounts: []Account{codexGoalBridgeTestAccount()},
	}, cfg)
	svc.SetRunnerForTesting(runner)

	var events []CodexGoalBridgeStreamEvent
	result, err := svc.HandleStream(context.Background(), CodexGoalBridgeRequest{
		Protocol: CodexGoalProtocolGemini,
		Endpoint: "/gemini-2.5-pro:streamGenerateContent",
		Body:     []byte(`{"tools":[{"functionDeclarations":[{"name":"lookup","parameters":{"type":"object"}}]}],"toolConfig":{"functionCallingConfig":{"mode":"ANY","allowedFunctionNames":["lookup"]}},"contents":[{"role":"user","parts":[{"text":"call lookup"}]}]}`),
	}, func(event CodexGoalBridgeStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("HandleStream returned error: %v", err)
	}
	if len(events) != 4 || events[0].Type != CodexGoalBridgeStreamEventStart || !events[0].DeferResponsesOutputItem {
		t.Fatalf("events = %#v, want start plus function-call stream events", events)
	}
	if events[1].Type != CodexGoalBridgeStreamEventFunctionCallStart || events[1].FunctionCall.Name != "lookup" {
		t.Fatalf("function start event = %#v", events[1])
	}
	if events[2].Type != CodexGoalBridgeStreamEventFunctionArgumentsDelta || events[2].Delta != `{"q":"gemini"}` {
		t.Fatalf("arguments delta event = %#v", events[2])
	}
	if events[3].Type != CodexGoalBridgeStreamEventFunctionCallDone || events[3].FunctionCall.Arguments != `{"q":"gemini"}` {
		t.Fatalf("function done event = %#v", events[3])
	}
	if len(result.FunctionCalls) != 1 || result.FunctionCalls[0].Name != "lookup" || result.FunctionCalls[0].Arguments != `{"q":"gemini"}` {
		t.Fatalf("FunctionCalls = %#v", result.FunctionCalls)
	}
}

func TestCodexGoalConfigTOMLIncludesMCPServers(t *testing.T) {
	toml := codexGoalConfigTOML(CodexGoalRunInput{
		Model: "gpt-5.5",
		MCPServers: []CodexGoalMCPServerConfig{{
			Label:        "dmcp",
			URL:          "https://dmcp-server.example/sse",
			EnabledTools: []string{"roll"},
			ApprovalMode: "approve",
			HTTPHeaders: map[string]string{
				"Authorization": "Bearer token-123",
			},
		}},
	})
	for _, want := range []string{
		`apps = false`,
		`plugins = false`,
		`[mcp_servers."dmcp"]`,
		`url = "https://dmcp-server.example/sse"`,
		`enabled_tools = ["roll"]`,
		`default_tools_approval_mode = "approve"`,
		`http_headers = { "Authorization" = "Bearer token-123" }`,
	} {
		if !strings.Contains(toml, want) {
			t.Fatalf("config TOML %q does not contain %q", toml, want)
		}
	}
}

func TestCodexGoalConfigTOMLIncludesMCPRemoteCommand(t *testing.T) {
	toml := codexGoalConfigTOML(CodexGoalRunInput{
		Model: "gpt-5.5",
		MCPServers: []CodexGoalMCPServerConfig{{
			Label:        "dmcp",
			Command:      "mcp-remote",
			Args:         []string{"https://dmcp-server.example/sse", "--header", "Authorization: Bearer token-123"},
			EnabledTools: []string{"roll"},
			ApprovalMode: "approve",
		}},
	})
	for _, want := range []string{
		`[mcp_servers."dmcp"]`,
		`command = "mcp-remote"`,
		`args = ["https://dmcp-server.example/sse", "--header", "Authorization: Bearer token-123"]`,
		`enabled_tools = ["roll"]`,
		`default_tools_approval_mode = "approve"`,
	} {
		if !strings.Contains(toml, want) {
			t.Fatalf("config TOML %q does not contain %q", toml, want)
		}
	}
	if strings.Contains(toml, `url =`) || strings.Contains(toml, `http_headers =`) {
		t.Fatalf("config TOML %q should use stdio command adapter only", toml)
	}
}

func TestCodexGoalThreadStartSandboxAndConfigDefaultsToReadOnly(t *testing.T) {
	sandbox, config := codexGoalThreadStartSandboxAndConfig(CodexGoalRunInput{
		MCPServers: []CodexGoalMCPServerConfig{{
			Label: "dmcp",
			URL:   "https://dmcp-server.example/mcp",
		}},
	})
	if sandbox != "read-only" {
		t.Fatalf("sandbox = %q, want read-only", sandbox)
	}
	if _, ok := config["sandbox_workspace_write"]; ok {
		t.Fatalf("config = %#v, should not enable workspace-write network for direct URL MCP", config)
	}
	features, ok := config["features"].(map[string]any)
	if !ok || features["goals"] != true {
		t.Fatalf("config = %#v, want goals feature enabled", config)
	}
}

func TestCodexGoalThreadStartSandboxAndConfigEnablesNetworkForMCPAdapter(t *testing.T) {
	sandbox, config := codexGoalThreadStartSandboxAndConfig(CodexGoalRunInput{
		MCPServers: []CodexGoalMCPServerConfig{{
			Label:   "dmcp",
			Command: "mcp-remote",
			Args:    []string{"https://dmcp-server.example/sse", "--transport", "sse-only"},
		}},
	})
	if sandbox != "workspace-write" {
		t.Fatalf("sandbox = %q, want workspace-write", sandbox)
	}
	workspace, ok := config["sandbox_workspace_write"].(map[string]any)
	if !ok || workspace["network_access"] != true {
		t.Fatalf("config = %#v, want sandbox_workspace_write.network_access=true", config)
	}
	features, ok := config["features"].(map[string]any)
	if !ok || features["goals"] != true {
		t.Fatalf("config = %#v, want goals feature enabled", config)
	}
}

func TestAppendCodexGoalCompletedToolEvent(t *testing.T) {
	var events []CodexGoalToolEvent
	allowedMCP := map[string]struct{}{"dmcp": {}}
	appendCodexGoalCompletedToolEvent([]byte(`{
		"item": {
			"type": "webSearch",
			"id": "ws_1",
			"query": "OpenAI docs",
			"action": {"type": "search", "query": "OpenAI docs"}
		}
	}`), &events, true, allowedMCP)
	appendCodexGoalCompletedToolEvent([]byte(`{
		"item": {
			"type": "mcpToolCall",
			"id": "mcp_1",
			"server": "dmcp",
			"tool": "roll",
			"arguments": {"diceRollExpression": "2d4+1"},
			"status": "completed",
			"result": {"content": [{"type": "text", "text": "6"}]}
		}
	}`), &events, true, allowedMCP)
	appendCodexGoalCompletedToolEvent([]byte(`{
		"item": {
			"type": "mcpToolCall",
			"id": "mcp_2",
			"server": "codex_apps",
			"tool": "gmail_get_profile",
			"arguments": {},
			"status": "completed",
			"result": {"content": [{"type": "text", "text": "private"}]}
		}
	}`), &events, true, allowedMCP)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2: %#v", len(events), events)
	}
	if events[0].Type != "web_search_call" || events[0].ID != "ws_1" || !strings.Contains(string(events[0].Action), "OpenAI docs") {
		t.Fatalf("web event = %#v", events[0])
	}
	if events[1].Type != "mcp_call" || events[1].ServerLabel != "dmcp" || events[1].Name != "roll" || events[1].Output != "6" || !strings.Contains(events[1].Arguments, "2d4+1") {
		t.Fatalf("mcp event = %#v", events[1])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fakeCodexGoalStreamingRunner struct {
	deltas               []string
	toolEvents           []CodexGoalToolEvent
	input                CodexGoalRunInput
	refreshedCredentials map[string]any
}

func (r *fakeCodexGoalStreamingRunner) RunGoal(ctx context.Context, input CodexGoalRunInput) (*CodexGoalRunResult, error) {
	return r.RunGoalStream(ctx, input, nil)
}

func (r *fakeCodexGoalStreamingRunner) RunGoalStream(ctx context.Context, input CodexGoalRunInput, onDelta CodexGoalDeltaSink) (*CodexGoalRunResult, error) {
	r.input = input
	var text strings.Builder
	for _, delta := range r.deltas {
		text.WriteString(delta)
		if onDelta != nil {
			if err := onDelta(delta); err != nil {
				return nil, err
			}
		}
	}
	for _, event := range r.toolEvents {
		if input.ToolEventSink != nil {
			if err := input.ToolEventSink(event); err != nil {
				return nil, err
			}
		}
	}
	return &CodexGoalRunResult{
		Text:                 text.String(),
		ToolEvents:           append([]CodexGoalToolEvent(nil), r.toolEvents...),
		RefreshedCredentials: r.refreshedCredentials,
	}, nil
}

type codexGoalRefreshingAccountRepo struct {
	stubOpenAIAccountRepo
	updateCredentialsCalls int
	updatedID              int64
	updatedCredentials     map[string]any
}

func (r *codexGoalRefreshingAccountRepo) UpdateCredentials(ctx context.Context, id int64, credentials map[string]any) error {
	r.updateCredentialsCalls++
	r.updatedID = id
	r.updatedCredentials = cloneCredentials(credentials)
	return nil
}

func codexGoalBridgeTestAccount() Account {
	return Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"access_token":       "access-token",
			"refresh_token":      "refresh-token",
			"id_token":           "id-token",
			"chatgpt_account_id": "acct_123",
		},
	}
}

func codexGoalTestJWT(payload string) string {
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + "."
}
