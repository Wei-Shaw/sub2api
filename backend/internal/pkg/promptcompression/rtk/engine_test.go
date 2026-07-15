package rtk

import (
	"context"
	"strings"
	"testing"
)

func testFilter() []Filter {
	return []Filter{{
		ID:          "drop-noise",
		Commands:    []string{"bash"},
		Drop:        []string{`^noise:`},
		Collapse:    true,
		Deduplicate: true,
		OnEmpty:     "keep-original",
	}}
}

func testBody() []byte {
	return []byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"bash","input":{"command":"ls"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"summary: files\nnoise: one\nnoise: two\nnoise: three\nnoise: four\nnoise: five\nnoise: six\nnoise: seven\nnoise: eight\nnoise: nine\nnoise: ten\nnoise: eleven\nnoise: twelve\nnoise: thirteen\nnoise: fourteen\nnoise: fifteen\nnoise: sixteen\nnoise: seventeen\nnoise: eighteen\nnoise: nineteen\nnoise: twenty"}]}]}`)
}

func newTestEngine(t *testing.T, mode Mode) *Engine {
	t.Helper()
	e, err := NewEngineWithTokenCounter(Config{Mode: mode, MinCandidateBytes: 1, MinCandidateTokens: 1, MinSavedTokens: 1, MaxBodyBytes: 1 << 20, MaxResultBytes: 1 << 20}, testFilter(), StaticTokenCounter{})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestPrepareEnforcePreservesRequestShape(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	original := testBody()
	result := e.Prepare(context.Background(), original, Options{Protocol: ProtocolAnthropic})
	if !result.Applied || result.Outcome != "applied" {
		t.Fatalf("expected applied result: %+v", result)
	}
	if strings.Contains(string(result.Body), "noise:") {
		t.Fatalf("noise was not removed: %s", result.Body)
	}
	if !strings.Contains(string(result.Body), "summary: files") {
		t.Fatal("summary was not preserved")
	}
	if string(original) == string(result.Body) {
		t.Fatal("body was not changed")
	}
	if result.BeforeTokens <= result.AfterTokens {
		t.Fatalf("token guard not reflected in result: %+v", result)
	}
}

func TestPrepareObserveNeverChangesBody(t *testing.T) {
	e := newTestEngine(t, ModeObserve)
	original := testBody()
	result := e.Prepare(context.Background(), original, Options{Protocol: ProtocolAnthropic})
	if result.Applied || result.Outcome != "observed" {
		t.Fatalf("expected observe result: %+v", result)
	}
	if string(result.Body) != string(original) {
		t.Fatal("observe mode returned effective body")
	}
	if result.AfterBytes >= result.BeforeBytes {
		t.Fatal("observe did not report candidate savings")
	}
}

func TestPrepareSkipsOrphanAndNonShell(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	body := []byte(`{"messages":[{"role":"tool","tool_call_id":"orphan","content":"lots of output"},{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"Read","input":{"command":"cat file"}}]},{"role":"tool","tool_call_id":"call-1","content":"lots of output"}]}`)
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolChat})
	if result.Applied {
		t.Fatal("ineligible tool output was compressed")
	}
	if result.Outcome != "skipped" {
		t.Fatalf("unexpected outcome: %+v", result)
	}
}

func TestPrepareInvalidJSONFailOpen(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	body := []byte(`{"messages":`)
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolAnthropic})
	if result.Applied || string(result.Body) != string(body) {
		t.Fatalf("invalid JSON was not fail-open: %+v", result)
	}
}

func TestPrepareCacheProtected(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"bash","input":{"command":"ls"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","cache_control":{"type":"ephemeral"},"content":"noise: one\nnoise: two\nnoise: three\nnoise: four"}]}]}`)
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolAnthropic})
	if result.Applied || string(result.Body) != string(body) {
		t.Fatal("cache-protected output was modified")
	}
}

func TestDefaultFilterCatalogLoadsEmbeddedRules(t *testing.T) {
	filters := DefaultFilters()
	if len(filters) < 40 {
		t.Fatalf("expected migrated filter catalog, got %d filters", len(filters))
	}
	if _, err := NewEngine(Config{Mode: ModeObserve}, nil); err != nil {
		t.Fatalf("embedded filter snapshot failed to compile: %v", err)
	}
}

func TestResponsesFunctionCallAssociation(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	body := []byte(`{"input":[{"type":"function_call","call_id":"c1","name":"bash","arguments":"{\"command\":\"ls\"}"},{"type":"function_call_output","call_id":"c1","output":"summary: files\nnoise: one\nnoise: two\nnoise: three\nnoise: four"}]}`)
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolResponses})
	if !result.Applied || !strings.Contains(string(result.Body), `"type":"function_call_output"`) {
		t.Fatalf("responses tool output was not compressed safely: %+v", result)
	}
	if strings.Contains(string(result.Body), "noise:") {
		t.Fatal("responses noise remained")
	}
}

func TestProtectedLinesAreNeverTransformed(t *testing.T) {
	filters := []Filter{{ID: "protected", Commands: []string{"bash"}, StripANSI: true, Replace: []ReplaceRule{{Pattern: "secret", With: "redacted"}}, Drop: []string{`^noise`}, Preserve: []string{`^secret`}}}
	e, err := NewEngineWithTokenCounter(Config{Mode: ModeEnforce, MinCandidateBytes: 1, MinCandidateTokens: 1, MinSavedTokens: 1, MaxBodyBytes: 1 << 20, MaxResultBytes: 1 << 20}, filters, StaticTokenCounter{})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c1","name":"bash","input":{"command":"ls"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"secret: \u001b[31mkeep\u001b[0m\nnoise: remove\nsummary: keep"}]}]}`)
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolAnthropic})
	if !result.Applied {
		t.Fatalf("expected unprotected noise to be removed: %+v", result)
	}
	if !strings.Contains(string(result.Body), "secret: \\u001b[31mkeep\\u001b[0m") {
		t.Fatal("protected line was transformed")
	}
	if strings.Contains(string(result.Body), "noise: remove") {
		t.Fatal("unprotected line was not removed")
	}
}

func TestInvalidPerRequestFilterFailsOpen(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	body := testBody()
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolAnthropic, Filters: []Filter{{ID: "bad", Drop: []string{"["}}}})
	if result.Applied || string(result.Body) != string(body) || result.SkipReason != "invalid_filter_pack" {
		t.Fatalf("invalid filter did not fail-open: %+v", result)
	}
}

func TestGeminiTextFieldOnly(t *testing.T) {
	e := newTestEngine(t, ModeEnforce)
	body := []byte(`{"contents":[{"parts":[{"functionCall":{"name":"bash","args":{"command":"ls"}}},{"functionResponse":{"name":"bash","response":{"text":"summary: files\nnoise: one\nnoise: two\nnoise: three"}}}]}]}`)
	result := e.Prepare(context.Background(), body, Options{Protocol: ProtocolGemini})
	if !result.Applied || strings.Contains(string(result.Body), "noise:") {
		t.Fatalf("Gemini textual response was not compressed: %+v", result)
	}
	structured := []byte(`{"contents":[{"parts":[{"functionCall":{"name":"bash","args":{"command":"ls"}}},{"functionResponse":{"name":"bash","response":{"items":["noise: one","noise: two"]}}}]}]}`)
	structuredResult := e.Prepare(context.Background(), structured, Options{Protocol: ProtocolGemini})
	if structuredResult.Applied || string(structuredResult.Body) != string(structured) {
		t.Fatal("Gemini structured response was stringified or modified")
	}
}

func TestEmbeddedFilterCatalog(t *testing.T) {
	filters := embeddedFilters()
	if len(filters) < 50 {
		t.Fatalf("embedded OmniRoute catalog unexpectedly small: got %d filters", len(filters))
	}
	if _, err := NewEngine(Config{Mode: ModeOff}, nil); err != nil {
		t.Fatalf("embedded filter snapshot failed to compile: %v", err)
	}
}

func TestCompositeShellCommandMatching(t *testing.T) {
	if !matchesCommand([]string{"git"}, "bash", `echo "a && b" && git status | cat`) {
		t.Fatal("quoted composite command did not match git segment")
	}
	if matchesCommand([]string{"git"}, "bash", `echo "git status"`) {
		t.Fatal("quoted text was incorrectly treated as command")
	}
}
