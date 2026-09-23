package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestE2ECursorGatewayChatCompletions exercises the OpenAI-compatible
// Cursor gateway against a live Cursor Pro account.
func TestE2ECursorGatewayChatCompletions(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping gateway e2e")
	}

	machineID, macMachineID := cursorGatewayTelemetryIDs(t)
	account := &Account{
		ID:       1,
		Name:     "cursor-e2e",
		Platform: PlatformCursor,
		Credentials: map[string]any{
			"access_token":   accessToken,
			"machine_id":     machineID,
			"mac_machine_id": macMachineID,
			"client_version": os.Getenv("CURSOR_CLIENT_VERSION"),
		},
	}

	gin.SetMode(gin.TestMode)
	svc := NewCursorGatewayService(nil, nil)
	body := []byte(`{"model":"default","stream":false,"messages":[{"role":"user","content":"Say exactly: Hello from Sub2API Cursor gateway"}]}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
	if err != nil {
		t.Fatalf("ForwardAsChatCompletions: %v", err)
	}
	if result == nil || result.Stream {
		t.Fatalf("unexpected result: %+v", result)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("parse response: %v body=%s", err, rec.Body.String())
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		t.Fatalf("empty completion: %s", rec.Body.String())
	}
	t.Logf("gateway completion: %q usage=%+v", parsed.Choices[0].Message.Content, result.Usage)
}

func TestE2ECursorGatewayVariantModels(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping gateway e2e")
	}

	machineID, macMachineID := cursorGatewayTelemetryIDs(t)
	account := &Account{
		ID:       1,
		Name:     "cursor-e2e",
		Platform: PlatformCursor,
		Credentials: map[string]any{
			"access_token":   accessToken,
			"machine_id":     machineID,
			"mac_machine_id": macMachineID,
			"client_version": os.Getenv("CURSOR_CLIENT_VERSION"),
		},
	}

	gin.SetMode(gin.TestMode)
	svc := NewCursorGatewayService(nil, nil)

	for _, model := range []string{"grok-4.6", "claude-opus-5", "gpt-5.6-sol"} {
		t.Run(model, func(t *testing.T) {
			body := []byte(`{"model":"` + model + `","stream":false,"messages":[{"role":"user","content":"Reply with exactly: 2 + 2 equals 4."}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
			if err != nil {
				t.Fatalf("ForwardAsChatCompletions: %v body=%s", err, rec.Body.String())
			}
			if result == nil {
				t.Fatal("nil result")
			}

			var parsed struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
				Warnings []map[string]string `json:"warnings"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
				t.Fatalf("parse response: %v body=%s", err, rec.Body.String())
			}
			if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
				t.Fatalf("empty completion: %s", rec.Body.String())
			}
			t.Logf("variant=%s warnings=%v content=%q usage=%+v", rec.Header().Get("X-Sub2API-Model-Variant"), parsed.Warnings, parsed.Choices[0].Message.Content, result.Usage)
		})
	}
}

func cursorGatewayTelemetryIDs(t *testing.T) (machineID, macMachineID string) {
	t.Helper()
	machineID = os.Getenv("CURSOR_MACHINE_ID")
	macMachineID = os.Getenv("CURSOR_MAC_MACHINE_ID")

	home, err := os.UserHomeDir()
	if err != nil {
		return machineID, macMachineID
	}
	storagePath := filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "storage.json")
	raw, err := os.ReadFile(storagePath)
	if err != nil {
		return machineID, macMachineID
	}
	var storage map[string]any
	if err := json.Unmarshal(raw, &storage); err != nil {
		return machineID, macMachineID
	}
	telMachine, _ := storage["telemetry.machineId"].(string)
	telMac, _ := storage["telemetry.macMachineId"].(string)
	if strings.Contains(machineID, "-") && telMachine != "" {
		machineID = telMachine
	}
	if macMachineID == "" && telMac != "" {
		macMachineID = telMac
	}
	return machineID, macMachineID
}

// TestE2ECursorGatewayToolLoop drives a full caller-executed tool round-trip
// against a live Cursor Pro account: request with tools -> model calls the
// tool -> caller returns the result -> model answers in natural language.
// This is the acceptance test for the inline exec handshake (request-context
// reply, MCP handoff acknowledgment, turn replay); the wire shapes are
// reverse-engineered, so live validation matters here more than unit coverage.
//
// Required env: CURSOR_ACCESS_TOKEN (skipped otherwise). Optional:
// CURSOR_MACHINE_ID / CURSOR_MAC_MACHINE_ID (read from a local Cursor
// install's storage.json when unset, see cursorGatewayTelemetryIDs).
func TestE2ECursorGatewayToolLoop(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping tool-loop e2e")
	}

	machineID, macMachineID := cursorGatewayTelemetryIDs(t)
	account := &Account{
		ID:       1,
		Name:     "cursor-e2e-tools",
		Platform: PlatformCursor,
		Credentials: map[string]any{
			"access_token":   accessToken,
			"machine_id":     machineID,
			"mac_machine_id": macMachineID,
			"client_version": os.Getenv("CURSOR_CLIENT_VERSION"),
		},
	}

	gin.SetMode(gin.TestMode)
	svc := NewCursorGatewayService(nil, nil)

	toolsJSON := `[
		{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get the current weather for a city. Use this for any question about weather.",
				"parameters": {
					"type": "object",
					"properties": {
						"city": {"type": "string", "description": "City name"}
					},
					"required": ["city"]
				}
			}
		}
	]`

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// --- Round 1: expect a tool call, not an answer ---
	round1 := []byte(`{
		"model": "grok-4.6",
		"stream": false,
		"messages": [{"role": "user", "content": "What is the weather in Paris right now? Use the get_weather tool."}],
		"tools": ` + toolsJSON + `
	}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(round1))

	result, err := svc.ForwardAsChatCompletions(ctx, c, account, round1)
	require.NoError(t, err, "round 1 body: %s", rec.Body.String())

	var round1Resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &round1Resp), "body: %s", rec.Body.String())
	require.NotEmpty(t, round1Resp.Choices, "no choices in round 1: %s", rec.Body.String())

	choice := round1Resp.Choices[0]
	t.Logf("round1 finish=%s content=%q tool_calls=%+v usage=%+v",
		choice.FinishReason, choice.Message.Content, choice.Message.ToolCalls, result.Usage)

	if len(choice.Message.ToolCalls) == 0 {
		// Not fatal: the model may answer directly. But if finish_reason is
		// neither stop nor tool_calls, the handshake likely misbehaved.
		require.Contains(t, []string{"stop", "tool_calls"}, choice.FinishReason,
			"unexpected finish; raw body: %s", rec.Body.String())
		t.Skipf("model answered without a tool call (finish=%s); handshake alive but tool not invoked", choice.FinishReason)
	}

	require.Equal(t, "tool_calls", choice.FinishReason, "raw body: %s", rec.Body.String())
	require.Equal(t, "get_weather", choice.Message.ToolCalls[0].Function.Name)
	callID := choice.Message.ToolCalls[0].ID
	require.NotEmpty(t, callID)
	t.Logf("round1 tool call id=%s args=%s", callID, choice.Message.ToolCalls[0].Function.Arguments)

	// --- Round 2: return the executed result, expect a natural answer ---
	round2 := []byte(`{
		"model": "grok-4.6",
		"stream": false,
		"messages": [
			{"role": "user", "content": "What is the weather in Paris right now? Use the get_weather tool."},
			{"role": "assistant", "content": "", "tool_calls": [
				{"id": "` + callID + `", "type": "function", "function": {"name": "get_weather", "arguments": ` + strconv.Quote(choice.Message.ToolCalls[0].Function.Arguments) + `}}
			]},
			{"role": "tool", "tool_call_id": "` + callID + `", "content": "Sunny, 21 degrees Celsius, wind 8 km/h."}
		],
		"tools": ` + toolsJSON + `
	}`)
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(round2))

	result2, err := svc.ForwardAsChatCompletions(ctx, c2, account, round2)
	require.NoError(t, err, "round 2 body: %s", rec2.Body.String())

	var round2Resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &round2Resp), "body: %s", rec2.Body.String())
	require.NotEmpty(t, round2Resp.Choices, "no choices in round 2: %s", rec2.Body.String())
	require.Equal(t, "stop", round2Resp.Choices[0].FinishReason)
	content := strings.ToLower(round2Resp.Choices[0].Message.Content)
	require.NotEmpty(t, content, "empty final answer: %s", rec2.Body.String())
	require.Contains(t, content, "sun", "answer should reflect the tool result: %s", round2Resp.Choices[0].Message.Content)
	t.Logf("round2 answer=%q usage=%+v", round2Resp.Choices[0].Message.Content, result2.Usage)
	require.True(t, result2.Usage.InputTokens > 0 || result2.Usage.OutputTokens > 0,
		"usage must be recorded for billing")
}
