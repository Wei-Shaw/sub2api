package basispoints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func repairCall(id, summary, code string) object {
	args, _ := json.Marshal(object{"summary": summary, "code": code, "extended_summary": "{}", "destructive": false, "references": []any{}})
	return object{"type": "function_call", "name": "run_officejs", "id": "fc_" + id, "call_id": id, "arguments": string(args), "status": "completed"}
}

func repairResponse(id string, input, output int, items ...any) object {
	return object{"id": id, "model": "gpt-5.6-sol", "status": "completed", "output": items, "usage": object{
		"input_tokens": input, "output_tokens": output, "total_tokens": input + output, "input_tokens_details": object{"cached_tokens": 2}, "output_tokens_details": object{"reasoning_tokens": 1},
	}}
}

func repairBridge(t *testing.T, cache *ReplayCache) (object, *Bridge) {
	t.Helper()
	source := testSource()
	source["tools"] = []any{object{"type": "custom", "name": "functions.exec"}, object{"type": "function", "name": "shell"}}
	return mustPrepare(t, source, "repair-scope", cache)
}

func repairEvents(t *testing.T, body io.ReadCloser) []object {
	t.Helper()
	defer func() { _ = body.Close() }()
	var events []object
	err := readEvents(body, func(_ string, raw []byte) error {
		var event object
		require.NoError(t, decode(raw, &event))
		events = append(events, event)
		return nil
	})
	require.NoError(t, err)
	for i, event := range events {
		require.Equal(t, json.Number(fmt.Sprint(i)), event["sequence_number"])
	}
	return events
}

func repairValue[T any](t *testing.T, value any) T {
	t.Helper()
	result, ok := value.(T)
	require.True(t, ok, "unexpected decoded value type: %T", value)
	return result
}

type repairTrackedBody struct {
	io.Reader
	closed atomic.Int32
}

func (r *repairTrackedBody) Close() error { r.closed.Add(1); return nil }

func TestToolRepairKeepsOneResponseAndAtomicToolBatch(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["stream"] = true
	source["tools"] = []any{object{"type": "custom", "name": "functions.exec"}, object{"type": "function", "name": "shell"}}
	_, bridge := mustPrepare(t, source, "repair-scope", cache)
	code := "const value = 'literal';\ntext(value);"
	valid := nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd"}})
	invalid := repairCall("bad", "Run client tool", code)
	message := object{"type": "message", "id": "msg_original", "role": "assistant", "content": []any{object{"type": "output_text", "text": "Working."}}}
	original := repairResponse("resp_original", 10, 3, message, valid, invalid)
	wire := sse(object{"type": "response.created", "response": object{"id": "resp_original", "output": []any{}}}) + sse(object{"type": "response.output_text.delta", "delta": "Working."}) + sse(object{"type": "response.completed", "response": original})
	upstream := &repairTrackedBody{Reader: strings.NewReader(wire)}
	repairedValid := nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd"}})
	repairedValid["call_id"], repairedValid["id"] = "fixed_shell", "fc_fixed_shell"
	repairedCustom := repairCall("fixed_exec", "codex2api.custom/functions.exec", code)
	calls := 0
	stream := bridge.StreamWithToolRepair(context.Background(), upstream, func(ctx context.Context, failed object, validation error) (object, error) {
		calls++
		if upstream.closed.Load() != 1 || cache.get("repair-scope", "call_native") != nil {
			return nil, errors.New("upstream lease or partial cache retained before correction")
		}
		if validation == nil || text(failed["id"]) != "resp_original" || ctx.Err() != nil {
			return nil, errors.New("incorrect correction context")
		}
		return repairResponse("resp_hidden", 20, 4, object{"type": "message", "content": "Hidden commentary"}, repairedValid, repairedCustom), nil
	})
	events := repairEvents(t, stream)
	require.Equal(t, 1, calls)
	counts := make(map[string]int)
	var indexes []json.Number
	for _, event := range events {
		kind := text(event["type"])
		counts[kind]++
		if kind == "response.output_item.added" {
			indexes = append(indexes, repairValue[json.Number](t, event["output_index"]))
		}
	}
	require.Equal(t, []json.Number{"1", "2"}, indexes)
	require.Equal(t, 1, counts["response.created"])
	require.Equal(t, 1, counts["response.completed"])
	require.Equal(t, 1, counts["response.output_text.delta"])
	require.Zero(t, counts["response.failed"])
	last := repairValue[object](t, events[len(events)-1]["response"])
	require.Equal(t, "resp_original", last["id"])
	output := repairValue[[]any](t, last["output"])
	require.Equal(t, "msg_original", repairValue[object](t, output[0])["id"])
	require.Equal(t, code, repairValue[object](t, output[2])["input"])
	usage := repairValue[object](t, last["usage"])
	require.Equal(t, json.Number("30"), usage["input_tokens"])
	require.Equal(t, json.Number("7"), usage["output_tokens"])
	require.Equal(t, json.Number("37"), usage["total_tokens"])
	require.Equal(t, json.Number("4"), repairValue[object](t, usage["input_tokens_details"])["cached_tokens"])
	require.True(t, reflect.DeepEqual(repairedCustom, cache.get("repair-scope", "fixed_exec")))
	require.Nil(t, cache.get("repair-scope", "bad"))
	require.EqualValues(t, 1, upstream.closed.Load())
}

func TestToolRepairBoundsAttemptsAndPreservesFailureUsage(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprint(success), func(t *testing.T) {
			_, bridge := repairBridge(t, nil)
			initial := repairResponse("resp_initial", 10, 2, repairCall("bad", "Run", "text(1)"))
			calls := 0
			stream := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(_ context.Context, failed object, _ error) (object, error) {
				calls++
				if calls == 2 && failed["id"] != "resp_1" {
					return nil, errors.New("did not continue latest failed response")
				}
				summary := "Run"
				if success && calls == 2 {
					summary = "codex2api.custom/functions.exec"
				}
				return repairResponse(fmt.Sprintf("resp_%d", calls), 20, 3, repairCall(fmt.Sprintf("retry_%d", calls), summary, fmt.Sprintf("text(%d)", calls+1))), nil
			})
			events := repairEvents(t, stream)
			require.Equal(t, 2, calls)
			last := events[len(events)-1]
			response := repairValue[object](t, last["response"])
			if success {
				require.Equal(t, "response.completed", last["type"])
				output := repairValue[[]any](t, response["output"])
				require.Equal(t, "text(1)", repairValue[object](t, output[0])["input"], "retain the initial source across both corrections")
			} else {
				require.Equal(t, "response.failed", last["type"])
				require.Len(t, events, 1)
				require.Contains(t, repairValue[object](t, response["error"])["message"], "after 2 corrections")
				require.Empty(t, response["output"])
			}
			usage := repairValue[object](t, response["usage"])
			require.Equal(t, json.Number("50"), usage["input_tokens"])
			require.Equal(t, json.Number("8"), usage["output_tokens"])
		})
	}
}

func TestToolRepairDoesNotRetryOtherFailures(t *testing.T) {
	for _, name := range []string{"unsupported_native", "undeclared_target", "missing_call_id", "invalid_outer_json", "incomplete", "failed", "missing_terminal_tool", "truncated", "structured"} {
		t.Run(name, func(t *testing.T) {
			_, bridge := repairBridge(t, nil)
			item := repairCall("bad", "Run", "text(1)")
			kind := "response.completed"
			prefix := ""
			switch name {
			case "unsupported_native":
				item["name"] = "unknown"
			case "undeclared_target":
				item = nativeCall(object{"name": "outside_catalog", "arguments": object{}})
			case "missing_call_id":
				delete(item, "call_id")
			case "invalid_outer_json":
				item["arguments"] = "invalid"
			case "incomplete", "failed":
				kind = "response." + name
			case "missing_terminal_tool":
				prefix = sse(object{"type": "response.output_item.done", "item": repairCall("missing", "Run", "text(1)")})
			case "structured":
				source := testSource()
				source["text"] = object{"format": object{"type": "json_object"}}
				_, bridge = mustPrepare(t, source, "", nil)
			}
			wire := prefix + sse(object{"type": kind, "response": repairResponse("resp", 10, 2, item)})
			if name == "truncated" {
				wire = sse(object{"type": "response.output_item.done", "item": item})
			}
			calls := 0
			body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(wire)), func(context.Context, object, error) (object, error) {
				calls++
				return nil, errors.New("unexpected repair")
			})
			out, err := io.ReadAll(body)
			_ = body.Close()
			if name == "truncated" {
				require.ErrorIs(t, err, io.ErrUnexpectedEOF)
			} else {
				require.NoError(t, err)
			}
			require.Zero(t, calls)
			require.NotContains(t, string(out), "response.custom_tool_call_input")
		})
	}
}

func TestToolRepairRestoresOriginalRawPayload(t *testing.T) {
	for _, tc := range []struct {
		name, code   string
		functionCode bool
	}{
		{name: "patch", code: "*** Begin Patch\n*** Update File: sample.tex\n@@\n-($x,y)\n+(x,y)\n*** End Patch"},
		{name: "javascript", code: "text(1)"},
		{name: "raw_json", code: "{\"payload\":1}"},
		{name: "function_code", code: "const price = '$1';\ntext(price);", functionCode: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := tc.code
			cache := new(ReplayCache)
			source := testSource()
			definition := object{"type": "custom", "name": "apply_patch"}
			marker := customTransportPrefix + "apply_patch"
			if tc.functionCode {
				definition = object{"type": "function", "name": "run_script", "parameters": object{"type": "object", "properties": object{"code": object{"type": "string"}, "timeout_ms": object{"type": "integer"}}}}
				marker = functionCodeTransportPrefix + "run_script"
			}
			source["tools"] = []any{definition}
			_, bridge := mustPrepare(t, source, "repair-scope", cache)
			initial := repairResponse("resp_original", 10, 2, repairCall("bad", "Run", code))
			corrected := repairCall("fixed", marker, strings.ReplaceAll(code, "$", "")+"\n")
			if tc.functionCode {
				args := transportArguments(corrected)
				args["extended_summary"] = "{\"timeout_ms\":1500}"
				encoded, err := json.Marshal(args)
				require.NoError(t, err)
				corrected["arguments"] = string(encoded)
			}
			modelArguments := corrected["arguments"]
			body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(context.Context, object, error) (object, error) {
				return repairResponse("resp_fixed", 20, 3, corrected), nil
			})
			events := repairEvents(t, body)
			last := events[len(events)-1]
			require.Equal(t, "response.completed", last["type"])
			response := repairValue[object](t, last["response"])
			output := repairValue[[]any](t, response["output"])
			require.Len(t, output, 1)
			call := repairValue[object](t, output[0])
			if tc.functionCode {
				var args object
				require.NoError(t, decode([]byte(text(call["arguments"])), &args))
				require.Equal(t, code, args["code"])
				require.Equal(t, json.Number("1500"), args["timeout_ms"])
			} else {
				require.Equal(t, code, call["input"])
			}
			require.Equal(t, "fixed", call["call_id"])
			require.Equal(t, code, transportArguments(cache.get("repair-scope", "fixed"))["code"])
			require.Equal(t, modelArguments, corrected["arguments"], "leave the model response untouched")
			require.Nil(t, cache.get("repair-scope", "bad"))
			source["input"] = []any{call, object{"type": text(call["type"]) + "_output", "call_id": "fixed", "output": "done"}}
			prepared, _ := mustPrepare(t, source, "repair-scope", cache)
			found := false
			for _, raw := range repairValue[[]any](t, prepared["input"]) {
				item := repairValue[object](t, raw)
				if text(item["call_id"]) == "fixed" && text(item["type"]) == "function_call" {
					found = true
					require.Equal(t, code, transportArguments(item)["code"])
				}
			}
			require.True(t, found, "next request must replay the dispatched source text")
		})
	}
}

func TestToolRepairRejectsChangedBatchAndOperations(t *testing.T) {
	for _, change := range []string{"extra_tool", "text_only", "unmarked_raw_code", "changed_target", "explicit_target", "oversized_raw_code"} {
		t.Run(change, func(t *testing.T) {
			cache := new(ReplayCache)
			_, bridge := repairBridge(t, cache)
			code := "text(1)"
			original := []any{repairCall("bad", "Run", code)}
			corrected := []any{repairCall("fixed", "codex2api.custom/functions.exec", code)}
			switch change {
			case "extra_tool":
				corrected = append(corrected, repairCall("extra", "codex2api.custom/functions.exec", code))
			case "text_only":
				corrected = nil
			case "unmarked_raw_code":
				corrected = []any{nativeCall(object{"name": "functions.exec", "input": "text(2)"})}
			case "changed_target":
				original = append(original, nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd"}}))
				corrected = append(corrected, repairCall("changed_target", customTransportPrefix+"functions.exec", "text(1)"))
			case "explicit_target":
				original = []any{nativeCall(object{"name": "shell", "arguments": 42})}
			case "oversized_raw_code":
				original = []any{repairCall("bad", "Run", strings.Repeat("x", maxEnvelopeBytes+1))}
			}
			calls := 0
			initial := repairResponse("resp", 10, 2, original...)
			body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(context.Context, object, error) (object, error) {
				calls++
				return repairResponse("resp_fixed", 20, 3, corrected...), nil
			})
			events := repairEvents(t, body)
			require.Equal(t, 1, calls)
			require.Len(t, events, 1)
			require.Equal(t, "response.failed", events[0]["type"])
			require.Nil(t, cache.get("repair-scope", "fixed"))
		})
	}
}

func TestToolRepairDoesNotRetryCallbackFailure(t *testing.T) {
	_, bridge := repairBridge(t, nil)
	initial := repairResponse("resp", 10, 2, repairCall("bad", "Run", "text(1)"))
	calls := 0
	body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(context.Context, object, error) (object, error) {
		calls++
		return repairResponse("resp_failed", 20, 3), io.ErrUnexpectedEOF
	})
	events := repairEvents(t, body)
	require.Len(t, events, 1)
	require.Equal(t, 1, calls)
	require.Equal(t, "response.failed", events[0]["type"])
	response := repairValue[object](t, events[0]["response"])
	require.Equal(t, json.Number("30"), repairValue[object](t, response["usage"])["input_tokens"])
}

func TestToolRepairCloseCancelsInFlightCorrection(t *testing.T) {
	_, bridge := repairBridge(t, nil)
	initial := repairResponse("resp_initial", 10, 2, repairCall("bad", "Run", "text(1)"))
	started, canceled := make(chan struct{}), make(chan struct{})
	body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(ctx context.Context, _ object, _ error) (object, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("correction did not start")
	}
	require.NoError(t, body.Close())
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("correction was not canceled")
	}
}

func TestToolRepairBuilderPreservesNativeHistoryAndSettings(t *testing.T) {
	prepared, _ := repairBridge(t, nil)
	encoded, err := json.Marshal(prepared)
	require.NoError(t, err)
	initialLen := len(repairValue[[]any](t, prepared["input"]))
	for attempt := 0; attempt < 2; attempt++ {
		failed := repairResponse("resp", 10, 2, repairCall(fmt.Sprint(attempt), "Run", "text(1)"))
		encoded, err = BuildToolRepairRequest(encoded, failed, errors.New("missing CUSTOM marker"))
		require.NoError(t, err)
		var request object
		require.NoError(t, decode(encoded, &request))
		for _, field := range []string{"model", "reasoning_effort", "model_selection", "prompt_cache_key", "context_management"} {
			require.Equal(t, prepared[field], request[field])
		}
		require.NotContains(t, request, "tools")
		require.NotContains(t, request, "previous_response_id")
		metadata := repairValue[object](t, request["metadata"])
		require.Equal(t, repairValue[object](t, prepared["metadata"])["task_id"], metadata["task_id"])
		require.Equal(t, repairValue[object](t, prepared["metadata"])["turn_id"], metadata["turn_id"])
		require.Equal(t, fmt.Sprint(attempt+2), metadata["agent_iteration"])
		input := repairValue[[]any](t, request["input"])
		require.Len(t, input, initialLen+(attempt+1)*3)
		require.Equal(t, prepared["input"], input[:initialLen])
		result := repairValue[object](t, input[len(input)-2])
		require.Equal(t, fmt.Sprint(attempt), result["call_id"])
		var feedback object
		require.NoError(t, decode([]byte(text(result["output"])), &feedback))
		require.Equal(t, false, feedback["executed"])
		require.Equal(t, "invalid_client_tool_transport", repairValue[object](t, feedback["error"])["code"])
	}
}

func TestToolRepairReaderRejectsIncompleteOrMissingTools(t *testing.T) {
	item := repairCall("good", "codex2api.custom/functions.exec", "text(1)")
	for _, kind := range []string{"response.completed", "response.failed", "response.incomplete", "missing_tool", "truncated"} {
		t.Run(kind, func(t *testing.T) {
			response := repairResponse("resp", 10, 2, item)
			event := kind
			if kind == "missing_tool" {
				response["output"], event = []any{}, "response.completed"
			}
			wire := sse(object{"type": "response.output_item.done", "item": item})
			if kind != "truncated" {
				wire += sse(object{"type": event, "response": response})
			}
			actual, err := ReadToolRepairResponse(strings.NewReader(wire))
			if kind == "response.completed" {
				require.NoError(t, err)
				require.Equal(t, "resp", actual["id"])
			} else {
				require.Error(t, err)
			}
			if kind != "truncated" {
				require.Equal(t, json.Number("10"), repairValue[object](t, actual["usage"])["input_tokens"])
			}
		})
	}
}

func TestToolRepairBindsOriginalRawCommand(t *testing.T) {
	source := testSource()
	source["tools"] = []any{functionCmdTestTool("exec_command")}
	cache := new(ReplayCache)
	_, bridge := mustPrepare(t, source, "cmd-repair", cache)
	original := "printf '%s\\n' \"literal $value\"\n"
	initial := repairResponse("initial", 3, 1, repairCall("bad", "Run command", original))
	attempts := 0
	body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(context.Context, object, error) (object, error) {
		attempts++
		fixed := functionCmdTestNative(t, "exec_command", "rewritten command", "{\"description\":\"Run command\"}")
		return repairResponse("fixed", 2, 1, fixed), nil
	})
	events := repairEvents(t, body)
	require.Equal(t, 1, attempts)
	var final object
	for _, event := range events {
		if event["type"] == "response.completed" {
			final = repairValue[object](t, event["response"])
		}
	}
	require.NotNil(t, final)
	output := repairValue[[]any](t, final["output"])
	call := repairValue[object](t, output[0])
	require.Equal(t, original, functionCmdTestArguments(t, call)["cmd"])
	cached := cache.get("cmd-repair", "call_code")
	require.Equal(t, original, transportArguments(cached)["code"])
}

func TestToolRepairPreservesAlreadyValidOperations(t *testing.T) {
	for _, kind := range []string{"function", "custom"} {
		t.Run(kind, func(t *testing.T) {
			cache := new(ReplayCache)
			_, bridge := repairBridge(t, cache)
			valid := nativeCall(object{"name": "shell", "arguments": object{"cmd": "pwd", "workdir": "/original"}})
			changed := nativeCall(object{"name": "shell", "arguments": object{"cmd": "rm changed", "workdir": "/changed"}})
			if kind == "custom" {
				valid = repairCall("valid", customTransportPrefix+"functions.exec", "text('original')")
				changed = repairCall("changed", customTransportPrefix+"functions.exec", "text('changed')")
			}
			changed["id"], changed["call_id"] = "fc_changed", "changed"
			initial := repairResponse("resp_original", 10, 2, valid, repairCall("bad", "Run", "text(42)"))
			corrected := repairResponse("resp_correction", 20, 3, changed, repairCall("fixed", customTransportPrefix+"functions.exec", "text(42)"))
			before, err := json.Marshal(corrected)
			require.NoError(t, err)
			calls := 0
			body := bridge.StreamWithToolRepair(context.Background(), io.NopCloser(strings.NewReader(sse(object{"type": "response.completed", "response": initial}))), func(context.Context, object, error) (object, error) {
				calls++
				return corrected, nil
			})
			events := repairEvents(t, body)
			last := events[len(events)-1]
			require.Equal(t, "response.completed", last["type"])
			require.Equal(t, 1, calls)
			response := repairValue[object](t, last["response"])
			require.Equal(t, "resp_original", response["id"])
			output := repairValue[[]any](t, response["output"])
			require.Len(t, output, 2)
			first := repairValue[object](t, output[0])
			if kind == "custom" {
				require.Equal(t, "text('original')", first["input"])
			} else {
				require.JSONEq(t, "{\"cmd\":\"pwd\",\"workdir\":\"/original\"}", text(first["arguments"]))
			}
			cached := cache.get("repair-scope", "changed")
			require.NotNil(t, cached)
			check := *bridge
			check.replay = nil
			replayed, err := check.translateCall(cached)
			require.NoError(t, err)
			require.Equal(t, historyCallFingerprint(first), historyCallFingerprint(replayed))
			after, err := json.Marshal(corrected)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "keep native continuation history immutable")
			require.Equal(t, json.Number("30"), repairValue[object](t, response["usage"])["input_tokens"])
		})
	}
}
