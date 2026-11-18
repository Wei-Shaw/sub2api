//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicConverterOutputCacheFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name           string
		chatUsage      string
		responsesUsage string
		creation       int64
		read           int64
	}{
		{"missing usage", "", "", 0, 0},
		{"null usage", `,"usage":null`, `,"usage":null`, 0, 0},
		{"empty usage", `,"usage":{}`, `,"usage":{}`, 0, 0},
		{"read only", `,"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":3}}`, `,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":3}}`, 0, 3},
		{"creation only", `,"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cache_write_tokens":4}}`, `,"usage":{"input_tokens":10,"cache_creation_input_tokens":4}`, 4, 0},
		{"zero write overrides creation alias", `,"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":0,"cache_creation_tokens":4}}`, `,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":0,"cache_creation_tokens":4}}`, 0, 3},
		{"null Chat write overrides creation alias", `,"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":null,"cache_creation_tokens":4}}`, `,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":0,"cache_creation_tokens":4}}`, 0, 3},
		{"negative write overrides creation alias", `,"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":-2,"cache_creation_tokens":4}}`, `,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":-2,"cache_creation_tokens":4}}`, 0, 3},
		{"positive write overrides creation alias", `,"usage":{"prompt_tokens":10,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":2,"cache_creation_tokens":4}}`, `,"usage":{"input_tokens":10,"input_tokens_details":{"cached_tokens":3,"cache_write_tokens":2,"cache_creation_tokens":4}}`, 2, 3},
	} {
		for _, route := range []string{"chat buffered", "chat terminal", "chat eof", "responses buffered", "responses terminal", "responses eof"} {
			if route == "responses eof" && test.name != "missing usage" {
				continue
			}
			t.Run(test.name+"/"+route, func(t *testing.T) {
				payload := `{"id":"chat_cache","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]` + test.chatUsage + `}`
				if strings.HasPrefix(route, "responses") {
					payload = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_cache\",\"model\":\"gpt-5.4\"}}\n\n"
					if !strings.HasSuffix(route, "eof") {
						payload += `data: {"type":"response.completed","response":{"id":"resp_cache","status":"completed","output":[]` + test.responsesUsage + `}}` + "\n\n"
					}
				} else if !strings.HasSuffix(route, "buffered") {
					payload = `data: {"id":"chat_cache","choices":[{"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]` + test.chatUsage + `}` + "\n\n"
					if strings.HasSuffix(route, "terminal") {
						payload += "data: [DONE]\n\n"
					}
				}
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
				response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}
				service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
				var result *OpenAIForwardResult
				var err error
				switch route {
				case "chat buffered":
					result, err = service.bufferChatCompletionsAsAnthropic(ctx, response, "gpt-5.4", "gpt-5.4", "gpt-5.4", nil, nil, time.Now())
				case "chat terminal", "chat eof":
					result, err = service.streamChatCompletionsAsAnthropic(ctx, response, "gpt-5.4", "gpt-5.4", "gpt-5.4", nil, nil, time.Now())
				case "responses buffered":
					result, err = service.handleAnthropicBufferedStreamingResponse(response, ctx, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
				default:
					result, err = service.handleAnthropicStreamingResponse(response, ctx, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
				}
				if route == "responses eof" {
					require.ErrorContains(t, err, "missing terminal event")
				} else {
					require.NoError(t, err)
				}
				require.NotNil(t, result)
				require.EqualValues(t, test.creation, result.Usage.CacheCreationInputTokens)
				require.EqualValues(t, test.read, result.Usage.CacheReadInputTokens)
				require.EqualValues(t, test.creation, result.Usage.billingUsageTokens().CacheCreationTokens)
				require.Equal(t, http.StatusOK, recorder.Code)
				assertUsage := func(usage gjson.Result, creation, read int64) {
					require.False(t, usage.Get("cache_creation").Exists())
					for field, want := range map[string]int64{"cache_creation_input_tokens": creation, "cache_read_input_tokens": read} {
						tokens := usage.Get(field)
						require.True(t, tokens.Exists(), field)
						require.Equal(t, gjson.Number, tokens.Type, field)
						require.Equal(t, want, tokens.Int(), field)
					}
				}
				if strings.HasSuffix(route, "buffered") {
					assertUsage(gjson.Get(recorder.Body.String(), "usage"), test.creation, test.read)
					return
				}
				starts, deltas, stops := 0, 0, 0
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					event := gjson.Parse(strings.TrimPrefix(line, "data: "))
					switch event.Get("type").String() {
					case "message_start":
						assertUsage(event.Get("message.usage"), 0, 0)
						starts++
					case "message_delta":
						assertUsage(event.Get("usage"), test.creation, test.read)
						deltas++
					case "message_stop":
						stops++
					}
				}
				require.Equal(t, 1, starts)
				if route == "responses eof" {
					require.Zero(t, deltas)
					require.Zero(t, stops)
				} else {
					require.Equal(t, 1, deltas)
					require.Equal(t, 1, stops)
				}
			})
		}
	}
}

func TestResponsesTerminalUsageMatchesAnthropicOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		name := "buffered"
		if stream {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			payload := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_cache\",\"model\":\"gpt-5.4\"}}\n\n" +
				`data: {"type":"response.completed","usage":{"input_tokens":20,"output_tokens":4,"cache_creation_input_tokens":4,"input_tokens_details":{"cached_tokens":5}},"response":{"id":"resp_cache","status":"completed","output":[],"usage":{"input_tokens":10,"output_tokens":2,"cache_creation_input_tokens":1,"input_tokens_details":{"cached_tokens":2}}}}` + "\n\n"
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(payload))}
			service := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig()}
			var result *OpenAIForwardResult
			var err error
			if stream {
				result, err = service.handleAnthropicStreamingResponse(response, ctx, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
			} else {
				result, err = service.handleAnthropicBufferedStreamingResponse(response, ctx, rawChatCompletionsTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.Equal(t, 1, result.Usage.CacheCreationInputTokens)
			require.Equal(t, 2, result.Usage.CacheReadInputTokens)
			tokens := result.Usage.billingUsageTokens()
			require.Equal(t, 7, tokens.InputTokens)
			require.Equal(t, 1, tokens.CacheCreationTokens)
			outputUsage := gjson.Get(recorder.Body.String(), "usage")
			if stream {
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if strings.HasPrefix(line, "data: ") {
						event := gjson.Parse(strings.TrimPrefix(line, "data: "))
						if event.Get("type").String() == "message_delta" {
							outputUsage = event.Get("usage")
						}
					}
				}
			}
			require.True(t, outputUsage.Exists())
			require.EqualValues(t, tokens.InputTokens, outputUsage.Get("input_tokens").Int())
			require.EqualValues(t, tokens.OutputTokens, outputUsage.Get("output_tokens").Int())
			require.EqualValues(t, tokens.CacheCreationTokens, outputUsage.Get("cache_creation_input_tokens").Int())
			require.EqualValues(t, tokens.CacheReadTokens, outputUsage.Get("cache_read_input_tokens").Int())
			require.False(t, outputUsage.Get("cache_creation").Exists())
		})
	}
}

func TestAnthropicZeroCacheDeltaPromptReconciliation(t *testing.T) {
	for _, parser := range []string{"raw", "typed", "generic", "antigravity"} {
		t.Run(parser, func(t *testing.T) {
			usage := &ClaudeUsage{}
			for index, snapshot := range []string{
				`{"input_tokens":100,"prompt_tokens":100,"cache_read_input_tokens":30,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_5m_input_tokens":20}}`,
				`{"input_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":10}}`,
				`{"input_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"cache_creation":{"ephemeral_1h_input_tokens":10}}`,
			} {
				event := `{"type":"message_delta","usage":` + snapshot + `}`
				if index == 0 {
					event = `{"type":"message_start","message":{"usage":` + snapshot + `}}`
				}
				switch parser {
				case "raw":
					parseSSEUsagePassthrough(event, usage)
				case "typed":
					var source apicompat.AnthropicUsage
					require.NoError(t, source.UnmarshalJSON([]byte(snapshot)))
					mergeAnthropicUsage(usage, source)
				case "generic":
					(&GatewayService{}).parseSSEUsage(event, usage)
				case "antigravity":
					(&AntigravityGatewayService{}).extractSSEUsage("data: "+event, usage)
				}
				creation := 20
				if index > 0 {
					creation = 30
				}
				require.Equal(t, 30, usage.CacheReadInputTokens)
				if parser == "antigravity" {
					require.Equal(t, 100, usage.InputTokens)
				} else {
					require.Equal(t, creation, usage.CacheCreationInputTokens)
					require.Equal(t, 100-30-creation, usage.InputTokens)
				}
				cache5m, cache1h := normalizeCacheCreationBreakdown(usage.billingUsageTokens())
				require.Equal(t, creation, cache5m+cache1h)
			}
		})
	}
}

func TestCacheCreationZeroAggregateBillingProjection(t *testing.T) {
	claude := ClaudeUsage{
		InputTokens: 40, CacheReadInputTokens: 10,
		CacheCreation5mTokens: 20, CacheCreation1hTokens: 30,
		cacheCreationInputTokensPresent: true,
	}
	openAI := OpenAIUsage{
		InputTokens: 100, CacheReadInputTokens: 10,
		CacheCreation5mTokens: 20, CacheCreation1hTokens: 30,
		cacheCreationInputTokensPresent: true,
	}
	for _, tokens := range []UsageTokens{claude.billingUsageTokens(), openAI.billingUsageTokens()} {
		require.Equal(t, 40, tokens.InputTokens)
		require.Equal(t, 50, tokens.CacheCreationTokens)
		require.Equal(t, 10, tokens.CacheReadTokens)
		cache5m, cache1h := normalizeCacheCreationBreakdown(tokens)
		require.Equal(t, 20, cache5m)
		require.Equal(t, 30, cache1h)
		require.InDelta(t, 0.1, (&BillingService{}).computeCacheCreationCost(&ModelPricing{}, tokens, 0.002, 1), 1e-12)
	}
	require.Zero(t, claude.CacheCreationInputTokens)
	require.Zero(t, openAI.CacheCreationInputTokens)
}
