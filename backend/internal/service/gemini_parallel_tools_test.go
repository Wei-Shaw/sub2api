package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiParallelToolStreams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		frames [][]string
		want   []string
	}{
		{"same_name_distinct_ids_one_event", [][]string{{`{"id":"a","name":"read_file","args":{"path":"a"}}`, `{"id":"b","name":"read_file","args":{"path":"b"}}`}}, []string{`{"path":"a"}`, `{"path":"b"}`}},
		{"same_name_distinct_ids_separate_events", [][]string{{`{"id":"a","name":"read_file","args":{"path":"a"}}`}, {`{"id":"b","name":"read_file","args":{"path":"b"}}`}}, []string{`{"path":"a"}`, `{"path":"b"}`}},
		{"same_name_identical_arguments_distinct_ids", [][]string{{`{"id":"a","name":"read_file","args":{}}`, `{"id":"b","name":"read_file","args":{}}`}}, []string{`{}`, `{}`}},
		{"same_name_no_ids_one_event", [][]string{{`{"name":"read_file","args":{"path":"a"}}`, `{"name":"read_file","args":{"path":"b"}}`}}, []string{`{"path":"a"}`, `{"path":"b"}`}},
		{"same_name_no_ids_identical_separate_events", [][]string{{`{"name":"read_file","args":{}}`}, {`{"name":"read_file","args":{}}`}}, []string{`{}`, `{}`}},
		{"same_id_incremental_strings", [][]string{{`{"id":"a","name":"read_file","args":"{\"path\":"}`}, {`{"id":"a","name":"read_file","args":"\"a\"}"}`}}, []string{`{"path":"a"}`}},
		{"no_id_incremental_strings", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"\"a\"}"}`}}, []string{`{"path":"a"}`}},
		{"same_id_cumulative_and_duplicate", [][]string{{`{"id":"a","name":"read_file","args":"{\"path\":"}`}, {`{"id":"a","name":"read_file","args":"{\"path\":\"a\"}"}`}, {`{"id":"a","name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`}},
		{"anonymous_incremental_then_final_snapshot", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"\"a\"}"}`}, {`{"name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`}},
		{"id_first_chunk_only_then_final_snapshot", [][]string{{`{"id":"a","name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"\"a\"}"}`}, {`{"name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`}},
		{"id_learned_on_second_chunk_then_duplicate", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"id":"a","name":"read_file","args":"{\"path\":\"a\"}"}`}, {`{"id":"a","name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`}},
		{"same_name_no_ids_identical_one_event", [][]string{{`{"name":"read_file","args":{"path":"a"}}`, `{"name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`, `{"path":"a"}`}},
		{"anonymous_fragments_snapshot_then_identical_call", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"\"a\"}"}`}, {`{"name":"read_file","args":{"path":"a"}}`}, {`{"name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`, `{"path":"a"}`}},
		{"anonymous_fragments_then_distinct_complete_call", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"\"a\"}"}`}, {`{"name":"read_file","args":{"path":"b"}}`}}, []string{`{"path":"a"}`, `{"path":"b"}`}},
		{"late_id_then_distinct_same_arguments_call", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"\"a\"}","id":"a"}`}, {`{"name":"read_file","args":{"path":"a"},"id":"b"}`}}, []string{`{"path":"a"}`, `{"path":"a"}`}},
		{"anonymous_cumulative_final_string_then_snapshot", [][]string{{`{"name":"read_file","args":"{\"path\":"}`}, {`{"name":"read_file","args":"{\"path\":\"a\"}"}`}, {`{"name":"read_file","args":"{\"path\":\"a\"}"}`}, {`{"name":"read_file","args":{"path":"a"}}`}}, []string{`{"path":"a"}`}},
		{"explicit_id_change_overrides_fragment_snapshot", [][]string{{`{"name":"read_file","args":"{\"path\":","id":"a"}`}, {`{"name":"read_file","args":"\"a\"}","id":"a"}`}, {`{"name":"read_file","args":{"path":"a"},"id":"b"}`}}, []string{`{"path":"a"}`, `{"path":"a"}`}},
		{"different_names", [][]string{{`{"name":"read_file","args":{}}`, `{"name":"read_other","args":{}}`}}, []string{`{}`, `{}`}},
	}
	for _, tt := range tests {
		for _, protocol := range []string{"chat", "messages"} {
			t.Run(tt.name+"/"+protocol, func(t *testing.T) {
				var upstream strings.Builder
				for _, frame := range tt.frames {
					parts := []any{}
					for _, fc := range frame {
						var call map[string]any
						require.NoError(t, json.Unmarshal([]byte(fc), &call))
						parts = append(parts, map[string]any{"functionCall": call})
					}
					b, err := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": parts}}}})
					require.NoError(t, err)
					_, _ = upstream.WriteString("data: " + string(b) + "\n\n")
				}
				_, _ = upstream.WriteString(`data: {"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"cachedContentTokenCount":3}}` + "\n\ndata: [DONE]\n\n")
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				svc := &GeminiMessagesCompatService{}
				resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream.String()))}
				var result *geminiStreamResult
				var err error
				if protocol == "chat" {
					result, err = svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-test", false, true)
				} else {
					result, err = svc.handleStreamingResponse(c, resp, time.Now(), "gemini-test")
				}
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, 3, result.usage.CacheReadInputTokens)
				calls, terminal, open := collectParallelToolCalls(t, rec.Body.String(), protocol)
				require.True(t, terminal)
				require.Equal(t, -1, open)
				require.Len(t, calls, len(tt.want))
				ids := map[string]bool{}
				for i, want := range tt.want {
					a := calls[i]
					require.NotNil(t, a)
					require.NotEmpty(t, a.id)
					require.NotEmpty(t, a.name)
					require.False(t, ids[a.id])
					ids[a.id] = true
					require.JSONEq(t, want, a.args)
				}
			})
		}
	}
}

// collectParallelToolCalls checks the public SSE representation, including
// per-call metadata and the Anthropic content block lifecycle.
func collectParallelToolCalls(t *testing.T, body, protocol string) (map[int]*parallelToolCall, bool, int) {
	t.Helper()
	calls := map[int]*parallelToolCall{}
	terminal, open := false, -1
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if raw == "[DONE]" {
			continue
		}
		var e struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Delta        struct {
					ToolCalls []struct {
						Index    *int   `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &e))
		if protocol == "chat" {
			for _, choice := range e.Choices {
				if choice.FinishReason == "tool_calls" {
					terminal = true
				}
				for _, tc := range choice.Delta.ToolCalls {
					require.NotNil(t, tc.Index)
					idx := *tc.Index
					a := calls[idx]
					if a == nil {
						require.NotEmpty(t, tc.ID, "first delta must identify its call")
						require.NotEmpty(t, tc.Function.Name, "first delta must name its function")
						a = &parallelToolCall{}
						calls[idx] = a
					}
					a.id += tc.ID
					a.name += tc.Function.Name
					a.args += tc.Function.Arguments
				}
			}
			continue
		}
		switch e.Type {
		case "content_block_start":
			require.Equal(t, -1, open, "previous block must close before the next starts")
			require.Equal(t, "tool_use", e.ContentBlock.Type)
			open = e.Index
			_, exists := calls[e.Index]
			require.False(t, exists, "tool block indexes must be unique")
			calls[e.Index] = &parallelToolCall{id: e.ContentBlock.ID, name: e.ContentBlock.Name}
		case "content_block_delta":
			require.Equal(t, open, e.Index)
			require.Equal(t, "input_json_delta", e.Delta.Type)
			require.NotNil(t, calls[e.Index])
			calls[e.Index].args += e.Delta.PartialJSON
		case "content_block_stop":
			require.Equal(t, open, e.Index)
			open = -1
		case "message_delta":
			terminal = e.Delta.StopReason == "tool_use"
		}
	}
	return calls, terminal, open
}

type parallelToolCall struct{ id, name, args string }
