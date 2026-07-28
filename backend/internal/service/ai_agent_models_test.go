package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIAgentModelProtocols(t *testing.T) {
	tests := []struct {
		name         string
		protocol     string
		thinkingMode string
		path         string
		response     string
		assertBody   func(*testing.T, map[string]any)
	}{
		{
			name: "chat completions", protocol: agentProtocolChatCompletions, thinkingMode: "xhigh", path: "/v1/chat/completions",
			response: `{"choices":[{"message":{"role":"assistant","content":"chat ok"}}]}`,
			assertBody: func(t *testing.T, body map[string]any) {
				if body["reasoning_effort"] != "xhigh" {
					t.Errorf("reasoning_effort = %#v", body["reasoning_effort"])
				}
			},
		},
		{
			name: "responses", protocol: agentProtocolResponses, thinkingMode: "xhigh", path: "/v1/responses",
			response: `{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"responses ok"}]}],"usage":{"input_tokens":2048,"input_tokens_details":{"cached_tokens":1024}}}`,
			assertBody: func(t *testing.T, body map[string]any) {
				reasoning, _ := body["reasoning"].(map[string]any)
				if reasoning["effort"] != "xhigh" {
					t.Errorf("reasoning.effort = %#v", reasoning["effort"])
				}
				if key, _ := body["prompt_cache_key"].(string); !strings.HasPrefix(key, "sub2api-agent-") {
					t.Errorf("prompt_cache_key = %#v", body["prompt_cache_key"])
				}
			},
		},
		{
			name: "messages", protocol: agentProtocolMessages, thinkingMode: "4096", path: "/v1/messages",
			response: `{"content":[{"type":"text","text":"messages ok"}]}`,
			assertBody: func(t *testing.T, body map[string]any) {
				thinking, _ := body["thinking"].(map[string]any)
				if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(4096) {
					t.Errorf("thinking = %#v", thinking)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					t.Errorf("path = %q, want %q", request.URL.Path, test.path)
				}
				if test.protocol == agentProtocolMessages {
					if request.Header.Get("x-api-key") != "model-key" || request.Header.Get("anthropic-version") == "" {
						t.Errorf("missing Messages authentication headers")
					}
					if request.Header.Get("Authorization") != "" {
						t.Errorf("Messages request must not use Bearer authentication")
					}
				} else if request.Header.Get("Authorization") != "Bearer model-key" {
					t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				test.assertBody(t, body)
				if test.protocol != agentProtocolResponses {
					if _, exists := body["prompt_cache_key"]; exists {
						t.Errorf("%s request unexpectedly enabled Responses cache", test.protocol)
					}
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()

			service := &AIAgentService{client: server.Client()}
			message, err := service.complete(context.Background(), AIAgentConfig{
				BaseURL: server.URL, Model: "test-model", Protocol: test.protocol, ThinkingMode: test.thinkingMode,
			}, "model-key", []agentModelMessage{{Role: "user", Content: "hello"}})
			if err != nil {
				t.Fatalf("complete() error = %v", err)
			}
			if text := modelMessageText(message.Content); !strings.Contains(text, "ok") {
				t.Fatalf("content = %q", text)
			}
			if test.protocol == agentProtocolResponses && (message.InputTokens != 2048 || message.CachedInputTokens != 1024) {
				t.Fatalf("Responses cache usage = input:%d cached:%d", message.InputTokens, message.CachedInputTokens)
			}
		})
	}
}

func TestAIAgentModelProtocolsStreamText(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		path     string
		stream   string
	}{
		{
			name: "chat completions", protocol: agentProtocolChatCompletions, path: "/v1/chat/completions",
			stream: ": keep-alive\r\nevent: chat.completion.chunk\r\ndata: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hidden\"}}]}\r\n\r\ndata: not-json\r\n\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"chat \"}}]}\r\n\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\r\n\r\ndata: [DONE]\r\n\r\n",
		},
		{
			name: "responses", protocol: agentProtocolResponses, path: "/v1/responses",
			stream: ": heartbeat\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"responses \"}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"reasoning\",\"id\":\"rs_compat\",\"encrypted_content\":\"opaque\"},{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"responses ok\"}]}],\"usage\":{\"input_tokens\":12,\"input_tokens_details\":{\"cached_tokens\":4}}}}\n\n",
		},
		{
			name: "messages", protocol: agentProtocolMessages, path: "/v1/messages",
			stream: "event: ping\ndata: {\"type\":\"ping\"}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"messages \"}}\n\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path || request.Header.Get("Accept") != "text/event-stream" {
					t.Errorf("stream request path=%q accept=%q", request.URL.Path, request.Header.Get("Accept"))
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body["stream"] != true {
					t.Errorf("stream request body=%#v err=%v", body, err)
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = writer.Write([]byte(test.stream))
			}))
			defer server.Close()
			service := &AIAgentService{client: server.Client()}
			var deltas strings.Builder
			message, err := service.complete(context.Background(), AIAgentConfig{BaseURL: server.URL, Model: "test-model", Protocol: test.protocol}, "model-key", []agentModelMessage{{Role: "user", Content: "hello"}}, func(delta string) {
				_, _ = deltas.WriteString(delta)
			})
			if err != nil {
				t.Fatalf("complete stream: %v", err)
			}
			if !strings.Contains(deltas.String(), "ok") || !strings.Contains(modelMessageText(message.Content), "ok") {
				t.Fatalf("stream deltas=%q message=%q", deltas.String(), modelMessageText(message.Content))
			}
		})
	}
}

func TestResponsesPromptCacheKeyIsStableAndModelScoped(t *testing.T) {
	first := agentResponsesPromptCacheKey("gpt-5.6")
	second := agentResponsesPromptCacheKey("gpt-5.6")
	otherModel := agentResponsesPromptCacheKey("gpt-5.6-luna")
	if first == "" || first != second || first == otherModel {
		t.Fatalf("cache keys first=%q second=%q other=%q", first, second, otherModel)
	}
	if strings.Contains(first, "gpt") || len(first) > 64 {
		t.Fatalf("cache key exposes model or is unbounded: %q", first)
	}
}

func TestResponsesPreservesReasoningItemsAcrossToolCalls(t *testing.T) {
	reasoning := json.RawMessage(`{"type":"reasoning","id":"rs_1","encrypted_content":"opaque"}`)
	functionCall := json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"search_admin_operations","arguments":"{\"query\":\"users\"}"}`)
	history := []agentModelMessage{
		{Role: "assistant", ResponsesOutput: []json.RawMessage{reasoning, functionCall}},
		{Role: "tool", ToolCallID: "call_1", Content: `{"status":"success"}`},
	}
	encoded, err := json.Marshal(responsesInput(history))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	payload := string(encoded)
	for _, expected := range []string{`"encrypted_content":"opaque"`, `"type":"function_call"`, `"type":"function_call_output"`} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("responses input does not preserve %s: %s", expected, payload)
		}
	}
}

func TestMessagesPreservesSignedThinkingBlocksAcrossToolCalls(t *testing.T) {
	thinking := json.RawMessage(`{"type":"thinking","thinking":"private chain","signature":"signed-value"}`)
	toolUse := json.RawMessage(`{"type":"tool_use","id":"tool_1","name":"search_admin_operations","input":{"query":"users"}}`)
	history := []agentModelMessage{
		{Role: "assistant", AnthropicContent: []json.RawMessage{thinking, toolUse}},
		{Role: "tool", ToolCallID: "tool_1", Content: `{"status":"success"}`},
	}
	encoded, err := json.Marshal(anthropicMessages(history))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	payload := string(encoded)
	for _, expected := range []string{`"signature":"signed-value"`, `"type":"thinking"`, `"type":"tool_result"`} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("Messages input does not preserve %s: %s", expected, payload)
		}
	}
}
