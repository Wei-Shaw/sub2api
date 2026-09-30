package basispoints

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestCustomHistoryRebuildUsesCatalogTransport(t *testing.T) {
	for _, namespace := range []string{"", "functions", "nested.tools"} {
		t.Run(namespace, func(t *testing.T) {
			for _, input := range []string{
				"  const result = await tools.exec_command({cmd: \"printf '中文'\"});\r\n\ttext(result);\r\n",
				`{"name":"another_tool","arguments":{"cmd":"do not reinterpret"}}`,
				"",
			} {
				source := testSource()
				decl := object{"type": "custom", "name": "exec"}
				call := object{"type": "custom_tool_call", "id": "ctc_history", "call_id": "call_history", "name": "exec", "input": input}
				name := "exec"
				if namespace != "" {
					decl = object{"type": "namespace", "name": namespace, "tools": []any{decl}}
					call["namespace"] = namespace
					name = namespace + ".exec"
				}
				source["tools"] = []any{decl}
				source["input"] = []any{call, object{"type": "custom_tool_call_output", "call_id": "call_history", "output": "recorded result"}}
				cache := new(ReplayCache)
				var previous object
				for _, replay := range []*ReplayCache{nil, cache, cache, new(ReplayCache)} {
					body, bridge := mustPrepare(t, source, "scope", replay)
					items := mustTestValue[[]any](t, body["input"])
					native := mustTestValue[object](t, items[len(items)-2])
					var outer object
					if err := decode([]byte(text(native["arguments"])), &outer); err != nil {
						t.Fatal(err)
					}
					if outer["summary"] != customTransportPrefix+name || outer["code"] != input {
						t.Fatal("rebuilt custom history must use the declared raw marker and exact input")
					}
					restored, err := bridge.translateCall(native)
					if err != nil || historyCallFingerprint(restored) != historyCallFingerprint(call) {
						t.Fatalf("history transport changed the client call: %v", err)
					}
					output := mustTestValue[object](t, items[len(items)-1])
					if output["output"] != "recorded result" || output["call_id"] != call["call_id"] {
						t.Fatal("rebuilding the call changed its recorded result")
					}
					if previous != nil && !reflect.DeepEqual(previous, body) {
						t.Fatal("custom history changed across cache loss or retries")
					}
					previous = body
				}
			}
		})
	}
}

func TestCustomHistoryRetainsCachedNativeItem(t *testing.T) {
	source := testSource()
	source["tools"] = []any{object{"type": "custom", "name": "exec"}}
	cache := new(ReplayCache)
	_, bridge := mustPrepare(t, source, "scope", cache)
	native := nativeCall(object{"name": "exec", "input": "text('exact input');"})
	native["provider_extension"] = object{"keep": true}
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	source["input"] = []any{call, object{"type": "custom_tool_call_output", "call_id": call["call_id"], "output": "recorded"}}
	body, _ := mustPrepare(t, source, "scope", cache)
	items := mustTestValue[[]any](t, body["input"])
	if !reflect.DeepEqual(items[len(items)-2], native) {
		t.Fatal("canonical reconstruction must not rewrite a cached native item")
	}
}

func TestRawCustomStreamingRequiresExplicitRouting(t *testing.T) {
	const code = "const result = await tools.exec_command({cmd: \"private-fixture\"});\ntext(result);"
	for _, marked := range []bool{false, true} {
		source := testSource()
		source["tools"] = []any{object{"type": "namespace", "name": "functions", "tools": []any{object{"type": "custom", "name": "exec"}}}}
		_, bridge := mustPrepare(t, source, "scope", nil)
		summary := "Run client tool"
		if marked {
			summary = customTransportPrefix + "functions.exec"
		}
		args, err := json.Marshal(object{"summary": summary, "code": code, "extended_summary": "Run client tool", "destructive": false, "references": []any{}})
		if err != nil {
			t.Fatal(err)
		}
		native := object{"type": "function_call", "id": "fc_raw", "call_id": "call_raw", "name": "run_officejs", "arguments": string(args)}
		wire := sse(object{"type": "response.output_item.done", "output_index": 0, "item": native}) +
			sse(object{"type": "response.completed", "response": object{"id": "resp_raw", "output": []any{native}}})
		stream := bridge.Stream(io.NopCloser(strings.NewReader(wire)))
		out, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		calls, completed, failed := 0, 0, 0
		err = readEvents(bytes.NewReader(out), func(_ string, data []byte) error {
			var event object
			if err := decode(data, &event); err != nil {
				return err
			}
			switch event["type"] {
			case "response.output_item.done":
				calls++
				item := mustTestValue[object](t, event["item"])
				if item["type"] != "custom_tool_call" || item["input"] != code || item["namespace"] != "functions" || item["name"] != "exec" {
					t.Fatal("streaming changed custom input or tool identity")
				}
			case "response.completed":
				completed++
			case "response.failed":
				failed++
				if !bytes.Contains(data, []byte("summary=codex2api.custom/CATALOG_NAME")) || bytes.Contains(data, []byte("private-fixture")) {
					t.Fatal("invalid routing must explain the marker without exposing code")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if marked && (calls != 1 || completed != 1 || failed != 0) {
			t.Fatal("marked code must produce one complete custom call")
		}
		if !marked && (calls != 0 || completed != 0 || failed != 1) {
			t.Fatal("unmarked code must fail without guessing or emitting a client call")
		}
	}
}
