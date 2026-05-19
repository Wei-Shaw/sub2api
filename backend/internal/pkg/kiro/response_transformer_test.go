package kiro

import (
	"testing"
)

func collectingCallback() (*StreamCallback, *string, *[]ToolUse, *int, *int, *float64) {
	textBuf := ""
	var tools []ToolUse
	inputTok := 0
	outputTok := 0
	credits := 0.0
	cb := &StreamCallback{
		OnText: func(text string, _ bool) {
			textBuf += text
		},
		OnToolUse: func(tu ToolUse) {
			tools = append(tools, tu)
		},
		OnComplete: func(inT, outT int) {
			inputTok = inT
			outputTok = outT
		},
		OnCredits: func(c float64) {
			credits = c
		},
	}
	return cb, &textBuf, &tools, &inputTok, &outputTok, &credits
}

func TestProcessEvents_TextDeltas(t *testing.T) {
	events := []Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello"}},
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": " world"}},
	}
	cb, text, _, _, _, _ := collectingCallback()
	ProcessEvents(events, nil, cb)
	if *text != "hello world" {
		t.Fatalf("text = %q", *text)
	}
}

func TestProcessEvents_PrefixDeduplication(t *testing.T) {
	// Upstream sometimes re-sends the growing prefix; we should emit only
	// the new tail each time.
	events := []Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello"}},
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "hello world"}},
	}
	cb, text, _, _, _, _ := collectingCallback()
	ProcessEvents(events, nil, cb)
	if *text != "hello world" {
		t.Fatalf("text = %q (expected only one ' world' tail)", *text)
	}
}

func TestProcessEvents_ToolUseBufferedAndStopped(t *testing.T) {
	events := []Event{
		{Type: "toolUseEvent", Payload: map[string]any{
			"toolUseId": "tu_1",
			"name":      "listFiles",
			"input":     `{"path":`,
		}},
		{Type: "toolUseEvent", Payload: map[string]any{
			"toolUseId": "tu_1",
			"name":      "listFiles",
			"input":     `"/tmp"}`,
			"stop":      true,
		}},
	}
	cb, _, tools, _, _, _ := collectingCallback()
	ProcessEvents(events, map[string]string{"listFiles": "list_files"}, cb)

	if len(*tools) != 1 {
		t.Fatalf("expected 1 tool use, got %d", len(*tools))
	}
	tu := (*tools)[0]
	if tu.ToolUseID != "tu_1" {
		t.Fatalf("tool use id = %q", tu.ToolUseID)
	}
	if tu.Name != "list_files" {
		t.Fatalf("tool name should be restored, got %q", tu.Name)
	}
	path, ok := tu.Input["path"].(string)
	if !ok || path != "/tmp" {
		t.Fatalf("tool input = %+v", tu.Input)
	}
}

func TestProcessEvents_ToolUseObjectInputReplacesBuffer(t *testing.T) {
	events := []Event{
		{Type: "toolUseEvent", Payload: map[string]any{
			"toolUseId": "tu_1",
			"name":      "f",
			"input":     map[string]any{"a": 1},
			"stop":      true,
		}},
	}
	cb, _, tools, _, _, _ := collectingCallback()
	ProcessEvents(events, nil, cb)
	if len(*tools) != 1 || (*tools)[0].Input["a"].(float64) != 1 {
		t.Fatalf("unexpected tools: %+v", *tools)
	}
}

func TestProcessEvents_MeteringSumsCredits(t *testing.T) {
	events := []Event{
		{Type: "meteringEvent", Payload: map[string]any{"usage": 0.5}},
		{Type: "meteringEvent", Payload: map[string]any{"usage": 0.25}},
	}
	cb, _, _, _, _, credits := collectingCallback()
	ProcessEvents(events, nil, cb)
	if *credits != 0.75 {
		t.Fatalf("credits = %v", *credits)
	}
}

func TestProcessEvents_TokensFromNestedUsage(t *testing.T) {
	events := []Event{
		{Type: "assistantResponseEvent", Payload: map[string]any{
			"content": "hi",
			"usage": map[string]any{
				"inputTokens":  100.0,
				"outputTokens": 5.0,
			},
		}},
	}
	cb, _, _, inT, outT, _ := collectingCallback()
	ProcessEvents(events, nil, cb)
	if *inT != 100 || *outT != 5 {
		t.Fatalf("tokens = (%d, %d)", *inT, *outT)
	}
}

func TestProcessEvents_TokensFromCachedSplit(t *testing.T) {
	events := []Event{
		{Type: "meteringEvent", Payload: map[string]any{
			"usage": map[string]any{
				"uncachedInputTokens":       10.0,
				"cacheReadInputTokens":      20.0,
				"cacheCreationInputTokens":  5.0,
				"outputTokens":              3.0,
			},
		}},
	}
	cb, _, _, inT, outT, _ := collectingCallback()
	ProcessEvents(events, nil, cb)
	if *inT != 35 {
		t.Fatalf("input tokens = %d (expected 10+20+5)", *inT)
	}
	if *outT != 3 {
		t.Fatalf("output tokens = %d", *outT)
	}
}

func TestProcessEvents_ReasoningEmitsThinkingDelta(t *testing.T) {
	var captured []string
	cb := &StreamCallback{
		OnText: func(text string, isThinking bool) {
			if isThinking {
				captured = append(captured, "T:"+text)
			} else {
				captured = append(captured, "N:"+text)
			}
		},
		OnComplete: func(int, int) {},
	}
	events := []Event{
		{Type: "reasoningContentEvent", Payload: map[string]any{"text": "let me think"}},
		{Type: "assistantResponseEvent", Payload: map[string]any{"content": "answer"}},
	}
	ProcessEvents(events, nil, cb)
	if len(captured) != 2 || captured[0] != "T:let me think" || captured[1] != "N:answer" {
		t.Fatalf("captured = %v", captured)
	}
}

func TestProcessEvents_FlushUnclosedToolUseAtEnd(t *testing.T) {
	// A toolUseEvent without a final stop:true should still flush at
	// stream end so the client gets the tool block.
	events := []Event{
		{Type: "toolUseEvent", Payload: map[string]any{
			"toolUseId": "tu_1",
			"name":      "f",
			"input":     `{"x":1}`,
		}},
	}
	cb, _, tools, _, _, _ := collectingCallback()
	ProcessEvents(events, nil, cb)
	if len(*tools) != 1 {
		t.Fatalf("expected 1 tool flushed at end, got %d", len(*tools))
	}
}

func TestProcessEventsFromCallback_StreamingShape(t *testing.T) {
	// The streaming dispatcher should behave like ProcessEvents when the
	// caller drives it event by event.
	dispatch, finalize := ProcessEventsFromCallback(nil, &StreamCallback{
		OnText:     func(string, bool) {},
		OnComplete: func(int, int) {},
	})
	dispatch(Event{Type: "assistantResponseEvent", Payload: map[string]any{"content": "a"}})
	dispatch(Event{Type: "assistantResponseEvent", Payload: map[string]any{"content": "ab"}})
	finalize()
}
