package basispoints

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamFailureClassificationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		event  object
		status int
		code   string
	}{
		{"flat error", object{"type": "error", "code": "rate_limit_exceeded", "message": "PRIVATE"}, 429, "rate_limit_exceeded"},
		{"nested status", object{"type": "error", "error": object{"status": 401, "code": "PRIVATE", "type": "invalid_request_error"}}, 401, "basispoints_upstream_error"},
		{"explicit first", object{"type": "error", "status": 429, "error": object{"status": 400, "code": "invalid_value"}}, 429, "invalid_value"},
		{"invalid success", object{"type": "error", "status": 200, "error": object{"code": "rate_limit_exceeded"}}, 429, "rate_limit_exceeded"},
		{"invalid range", object{"type": "error", "status": 600}, 502, "basispoints_upstream_error"},
		{"invalid fraction", object{"type": "error", "status": 429.5}, 502, "basispoints_upstream_error"},
		{"invalid string", object{"type": "error", "status": "429 PRIVATE"}, 502, "basispoints_upstream_error"},
		{"type only", object{"type": "response.failed", "response": object{"error": object{"type": "rate_limit_error"}}}, 429, "basispoints_upstream_error"},
		{"no error", object{"type": "response.cancelled"}, 502, "basispoints_upstream_cancelled"},
		{"not failure", object{"type": "response.completed", "error": object{"code": "rate_limit_exceeded"}}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.event)
			require.NoError(t, err)
			failure := ParseUpstreamFailure(raw)
			if tc.status == 0 {
				require.Nil(t, failure)
				return
			}
			require.NotNil(t, failure)
			require.Equal(t, tc.status, failure.Status)
			require.Equal(t, tc.code, failure.Code)
			require.NotContains(t, failure.Error(), "PRIVATE")
		})
	}
	require.Nil(t, ParseUpstreamFailure([]byte("invalid JSON")))
}

func TestUpstreamFailureStopsReadersWithoutEOF(t *testing.T) {
	for _, mode := range []string{"bridge", "tool correction", "unknown tool correction", "compaction"} {
		t.Run(mode, func(t *testing.T) {
			_, bridge := mustPrepare(t, testSource(), "", nil)
			reader, writer := io.Pipe()
			t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
			wire := sse(object{"type": "response.cancelled", "response": object{
				"id": "resp_cancelled", "status": "cancelled", "output": []any{},
				"error": object{"code": "rate_limit_exceeded", "message": "PRIVATE"},
				"usage": object{"input_tokens": 7},
			}})
			go func() { _, _ = io.WriteString(writer, wire) }()
			done := make(chan error, 1)
			go func() {
				switch mode {
				case "bridge":
					body := bridge.Stream(reader)
					defer func() { _ = body.Close() }()
					raw, err := io.ReadAll(body)
					if err == nil && (!strings.Contains(string(raw), "rate_limit_exceeded") || strings.Contains(string(raw), "PRIVATE")) {
						err = io.ErrUnexpectedEOF
					}
					done <- err
				case "tool correction":
					_, err := ReadToolRepairResponse(reader)
					done <- err
				case "unknown tool correction":
					_, err := readRepairResponse(reader)
					done <- err
				case "compaction":
					_, err := ReadImageCompaction(reader, nil)
					done <- err
				}
			}()
			select {
			case err := <-done:
				if mode == "bridge" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.NotContains(t, err.Error(), "PRIVATE")
					if mode != "compaction" {
						var failure *UpstreamFailure
						require.ErrorAs(t, err, &failure)
						require.Equal(t, 429, failure.Status)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("reader waited for EOF after cancellation terminal")
			}
		})
	}
}

func TestUpstreamFailureWithholdsPendingToolsAndNeverRepairs(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	_, bridge := mustPrepare(t, source, "", nil)
	tool := nativeCall(object{"name": "shell", "arguments": object{"cmd": "PRIVATE_TOOL_ARGUMENTS"}})
	wire := sse(object{"type": "response.output_item.done", "item": tool, "output_index": 0}) +
		sse(object{"type": "response.failed", "response": object{
			"status": "failed", "output": []any{tool}, "error": object{"code": "invalid_value", "message": "PRIVATE"},
		}})
	corrections := 0
	body := bridge.StreamWithRepairs(context.Background(), io.NopCloser(strings.NewReader(wire)),
		func(context.Context, object, error) (object, error) { corrections++; return nil, nil },
		func(context.Context) (io.ReadCloser, error) { corrections++; return nil, nil })
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Zero(t, corrections)
	require.NotContains(t, string(raw), "PRIVATE")
	require.NotContains(t, string(raw), "function_call")
	require.Contains(t, string(raw), "invalid_value")
}

func TestCompactedCancellationRetainsWindowAndUsage(t *testing.T) {
	wire := sse(object{"type": "response.cancelled", "response": object{
		"id": "resp_cancelled", "status": "cancelled", "output": []any{},
		"usage": object{"input_tokens": 7, "output_tokens": 2},
		"error": object{"code": "rate_limit_exceeded"},
	}})
	window := []any{object{"type": "compaction", "id": "compaction_test"}}
	body := WithCompactedWindow(context.Background(), io.NopCloser(strings.NewReader(wire)), window, object{"input_tokens": 5, "output_tokens": 1})
	defer func() { _ = body.Close() }()
	events := repairEvents(t, body)
	last := events[len(events)-1]
	require.Equal(t, "response.cancelled", last["type"])
	response := mustTestValue[object](t, last["response"])
	require.Equal(t, window, response["output"])
	usage := mustTestValue[object](t, response["usage"])
	require.Equal(t, json.Number("12"), usage["input_tokens"])
	require.Equal(t, json.Number("3"), usage["output_tokens"])
}
