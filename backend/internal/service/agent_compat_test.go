package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFlattenAgentRequestExtractsSystemUserAndTools(t *testing.T) {
	req := &apicompat.ResponsesRequest{
		Model:        "claude-4.6-opus-high",
		Instructions: "be brief",
		Input:        json.RawMessage(`[{"role":"user","content":[{"type":"input_text","text":"hello world"}]}]`),
		Tools: []apicompat.ResponsesTool{{
			Type:        "function",
			Name:        "lookup",
			Description: "look something up",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	}

	got, err := flattenAgentRequest(req)
	require.NoError(t, err)
	require.Equal(t, "claude-4.6-opus-high", got.Model)
	require.Equal(t, "be brief", got.System)
	require.Equal(t, "hello world", got.UserText)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "lookup", got.Tools[0].Name)
	require.Equal(t, `{"type":"object"}`, got.Tools[0].SchemaJSON)
	require.NotEmpty(t, got.Conversation)
}

func TestFlattenAgentRequestAcceptsStringInput(t *testing.T) {
	got, err := flattenAgentRequest(&apicompat.ResponsesRequest{
		Model: "swe-1-6",
		Input: json.RawMessage(`"plain prompt"`),
	})
	require.NoError(t, err)
	require.Equal(t, "plain prompt", got.UserText)
	require.Equal(t, "plain prompt", got.Messages[0].Prompt)
}

func TestAgentResponsesEmitterFansOutTextAndTools(t *testing.T) {
	emitter := newAgentResponsesEmitter("claude-4.6-opus-high")
	var types []string
	for _, ev := range []agentEvent{
		{Kind: "thinking", Text: "plan"},
		{Kind: "text", Text: "hello"},
		{Kind: "tool_start", CallID: "call_1", ToolName: "lookup"},
		{Kind: "tool_delta", CallID: "call_1", Arguments: `{"q":`},
		{Kind: "tool_delta", CallID: "call_1", Arguments: `"x"}`},
		{Kind: "tool_end", CallID: "call_1", ToolName: "lookup"},
		{Kind: "usage", Input: 3, Output: 2},
		{Kind: "done"},
	} {
		for _, evt := range emitter.apply(ev) {
			types = append(types, evt.Type)
		}
	}
	require.Contains(t, types, "response.created")
	require.Contains(t, types, "response.reasoning_summary_text.delta")
	require.Contains(t, types, "response.output_text.delta")
	require.Contains(t, types, "response.function_call_arguments.delta")
	require.Contains(t, types, "response.completed")
	require.Equal(t, 3, emitter.usage.InputTokens)
	require.Equal(t, 2, emitter.usage.OutputTokens)
}

func TestShouldUseAgentCompat(t *testing.T) {
	require.False(t, ShouldUseAgentCompat(nil))
	require.False(t, ShouldUseAgentCompat(&Account{Platform: PlatformGrok}))
	require.True(t, ShouldUseAgentCompat(&Account{Platform: PlatformCursor}))
	require.True(t, ShouldUseAgentCompat(&Account{Platform: PlatformDevin}))
}

func TestAgentCompatForwardFansOutProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &AgentCompatGatewayService{
		stream: func(_ context.Context, _ *Account, req agentRequest, emit func(agentEvent) error) error {
			require.Equal(t, "claude-4.6-opus-high", req.Model)
			require.Equal(t, "hello", req.UserText)
			require.NoError(t, emit(agentEvent{Kind: "text", Text: "hi there"}))
			require.NoError(t, emit(agentEvent{Kind: "usage", Input: 4, Output: 2}))
			require.NoError(t, emit(agentEvent{Kind: "done"}))
			return nil
		},
	}
	account := &Account{ID: 7, Platform: PlatformCursor, Type: AccountTypeOAuth}

	t.Run("messages stream", func(t *testing.T) {
		rec, c := agentCompatContext(`{"model":"claude-4.6-opus-high","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
		result, err := svc.Forward(context.Background(), c, account, []byte(c.GetString("body")), nil)
		require.NoError(t, err)
		require.True(t, result.Stream)
		require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
		require.Contains(t, rec.Body.String(), "content_block_delta")
		require.Contains(t, rec.Body.String(), "hi there")
	})

	t.Run("responses stream", func(t *testing.T) {
		rec, c := agentCompatContext(`{"model":"claude-4.6-opus-high","input":"hello","stream":true}`)
		result, err := svc.ForwardAsResponses(context.Background(), c, account, []byte(c.GetString("body")), nil)
		require.NoError(t, err)
		require.True(t, result.Stream)
		require.Contains(t, rec.Body.String(), "response.output_text.delta")
		require.Contains(t, rec.Body.String(), "hi there")
	})

	t.Run("chat completions stream", func(t *testing.T) {
		rec, c := agentCompatContext(`{"model":"claude-4.6-opus-high","messages":[{"role":"user","content":"hello"}],"stream":true}`)
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, []byte(c.GetString("body")), nil)
		require.NoError(t, err)
		require.True(t, result.Stream)
		require.Contains(t, rec.Body.String(), "chat.completion.chunk")
		require.Contains(t, rec.Body.String(), "hi there")
		require.Contains(t, rec.Body.String(), "data: [DONE]")
	})

	t.Run("messages json", func(t *testing.T) {
		rec, c := agentCompatContext(`{"model":"claude-4.6-opus-high","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`)
		result, err := svc.Forward(context.Background(), c, account, []byte(c.GetString("body")), nil)
		require.NoError(t, err)
		require.False(t, result.Stream)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "hi there")
		require.Contains(t, rec.Body.String(), `"type":"message"`)
	})
}

func TestAgentCompatForwardModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mapping := range []struct {
		name        string
		platform    string
		pattern     string
		upstream    string
		publicRoute bool
	}{
		{name: "exact after channel mapping", platform: PlatformCursor, pattern: "channel-alias", upstream: "claude-4.6-opus-high"},
		{name: "wildcard after composite and channel mapping", platform: PlatformDevin, pattern: "channel-*", upstream: "swe-1-6", publicRoute: true},
	} {
		t.Run(mapping.name, func(t *testing.T) {
			const publicModel = "client-alias"
			account := &Account{
				ID: 7, Platform: mapping.platform, Type: AccountTypeOAuth,
				Credentials: map[string]any{"model_mapping": map[string]any{
					mapping.pattern:  mapping.upstream,
					mapping.upstream: "must-not-map-twice",
					publicModel:      "must-not-map-before-channel",
				}},
			}
			svc := &AgentCompatGatewayService{
				stream: func(_ context.Context, _ *Account, req agentRequest, emit func(agentEvent) error) error {
					require.Equal(t, mapping.upstream, req.Model)
					require.Equal(t, "hello", req.UserText)
					return emit(agentEvent{Kind: "text", Text: "hi there"})
				},
			}
			for _, protocol := range []struct {
				name    string
				body    string
				forward func(context.Context, *gin.Context, *Account, []byte, *ParsedRequest) (*ForwardResult, error)
			}{
				{"messages", `{"model":"channel-alias","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, svc.Forward},
				{"responses", `{"model":"channel-alias","input":"hello","stream":%t}`, svc.ForwardAsResponses},
				{"chat completions", `{"model":"channel-alias","messages":[{"role":"user","content":"hello"}],"stream":%t}`, svc.ForwardAsChatCompletions},
			} {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/stream=%t", protocol.name, stream), func(t *testing.T) {
						body := fmt.Sprintf(protocol.body, stream)
						rec, c := agentCompatContext(body)
						ctx := context.Background()
						parsed := &ParsedRequest{Model: publicModel}
						if mapping.publicRoute {
							ctx = WithCompositeRouteDecision(ctx, CompositeRouteDecision{
								Matched: true, PublicModel: publicModel, TargetPlatform: mapping.platform, UpstreamModel: "route-alias",
							})
							parsed.Model = "route-alias"
						}
						result, err := protocol.forward(ctx, c, account, []byte(body), parsed)
						require.NoError(t, err)
						require.Equal(t, publicModel, result.Model)
						require.Equal(t, mapping.upstream, result.UpstreamModel)
						require.Equal(t, stream, result.Stream)
						require.Equal(t, http.StatusOK, rec.Code)
						require.Contains(t, rec.Body.String(), "hi there")
						payloads := []string{rec.Body.String()}
						if stream {
							payloads = nil
							for _, line := range strings.Split(rec.Body.String(), "\n") {
								if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
									payloads = append(payloads, strings.TrimPrefix(line, "data: "))
								}
							}
						}
						var visibleModels []string
						for _, payload := range payloads {
							var event struct {
								Model   string `json:"model"`
								Message struct {
									Model string `json:"model"`
								} `json:"message"`
								Response struct {
									Model string `json:"model"`
								} `json:"response"`
							}
							require.NoError(t, json.Unmarshal([]byte(payload), &event))
							for _, model := range []string{event.Model, event.Message.Model, event.Response.Model} {
								if model != "" {
									require.Equal(t, publicModel, model)
									visibleModels = append(visibleModels, model)
								}
							}
						}
						require.Contains(t, visibleModels, publicModel)
					})
				}
			}
		})
	}
}

func agentCompatContext(body string) (*httptest.ResponseRecorder, *gin.Context) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("body", body)
	return rec, c
}
