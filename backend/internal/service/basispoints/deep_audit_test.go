package basispoints

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestDeepAuditReplayLossRecoversCompleteClientHistory(t *testing.T) {
	for _, scenario := range []string{"same_account", "changed_account", "restart", "other_clients_evict", "idle_expiry"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now()
			cache := &ReplayCache{now: func() time.Time { return now }}
			source := testSource()
			source["tools"] = []any{object{"type": "function", "name": "shell"}}
			_, bridge := mustPrepare(t, source, "account-1|client-key", cache)
			native := nativeCall(object{"name": "shell", "arguments": object{"command": "pwd"}})
			call, err := bridge.translateCall(native)
			if err != nil {
				t.Fatal(err)
			}
			source["input"] = []any{message("user", "Inspect project"), call, object{"type": "function_call_output", "call_id": call["call_id"], "output": "project"}}
			scope := bridge.scope
			switch scenario {
			case "changed_account":
				scope = "account-2|client-key"
			case "restart":
				cache = new(ReplayCache)
			case "idle_expiry":
				now = now.Add(replayCacheIdleTTL)
			case "other_clients_evict":
				for i := 0; i < 1024; i++ {
					cache.put("unrelated-account|unrelated-key", fmt.Sprint(i), native)
				}
			}
			raw, _ := json.Marshal(source)
			_, _, err = Prepare(raw, scope, cache)
			if err != nil {
				t.Fatalf("complete client history must survive %s: %v", scenario, err)
			}
		})
	}
}

func TestDeepAuditAcceptedToolCanExceedReplayEntryLimit(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "shell"}}
	_, bridge := mustPrepare(t, source, "account|key", cache)
	// Escaping at three levels makes the stored native item larger than the
	// accepted inner JSON envelope, without exceeding the decoder's 1 MiB limit.
	native := nativeCall(object{"name": "shell", "arguments": object{"value": strings.Repeat("\"", 150000)}})
	call, err := bridge.translateCall(native)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(native)
	if len(raw) <= 1<<20 {
		t.Fatalf("fixture too small: %d", len(raw))
	}
	if cache.get(bridge.scope, text(call["call_id"])) != nil {
		t.Fatal("expected observed silent replay-cache rejection")
	}
	source["input"] = []any{message("user", "continue"), call, object{"type": "function_call_output", "call_id": call["call_id"], "output": "done"}}
	raw, _ = json.Marshal(source)
	if _, _, err := Prepare(raw, bridge.scope, cache); err != nil {
		t.Fatalf("a tool too large to cache must remain recoverable: %v", err)
	}
}

func TestDeepAuditDisabledHostedToolAllowsTextRequest(t *testing.T) {
	source := testSource()
	source["tool_choice"] = "none"
	source["tools"] = []any{object{"type": "web_search"}}
	raw, _ := json.Marshal(source)
	_, _, err := Prepare(raw, "account|key", new(ReplayCache))
	if err != nil {
		t.Fatalf("disabled hosted tool must not reject text request: %v", err)
	}
}

func TestDeepAuditRepeatedAdditionalToolsDeduplicatesHistory(t *testing.T) {
	source := testSource()
	decl := object{"type": "function", "name": "shell"}
	source["tools"] = []any{decl}
	source["input"] = []any{message("user", "Continue"), object{"type": "additional_tools", "tools": []any{decl}}}
	raw, _ := json.Marshal(source)
	_, _, err := Prepare(raw, "account|key", new(ReplayCache))
	if err != nil {
		t.Fatalf("matching repeated declarations must be accepted: %v", err)
	}
}

func TestDeepAuditImageInToolResultValidated(t *testing.T) {
	cache := new(ReplayCache)
	source := testSource()
	source["tools"] = []any{object{"type": "function", "name": "view_image"}}
	_, bridge := mustPrepare(t, source, "account|key", cache)
	call, err := bridge.translateCall(nativeCall(object{"name": "view_image", "arguments": object{"path": "screen.png"}}))
	if err != nil {
		t.Fatal(err)
	}
	source["input"] = []any{message("user", "Inspect the screenshot"), call, object{
		"type": "function_call_output", "call_id": call["call_id"], "output": []any{
			object{"type": "input_image", "image_url": "data:image/png;base64,AAAA"},
		},
	}}
	raw, _ := json.Marshal(source)
	_, _, err = Prepare(raw, bridge.scope, cache)
	if err == nil || !strings.Contains(err.Error(), "image support is disabled") {
		t.Fatalf("tool result image must have the same HTTPS requirement as user content: %v", err)
	}
}
