// adapter_test.go 验证请求编码 / 会话派生 / 响应解码 / 流控的关键语义。
// 用例集对应 devin2api devin_test.go 的核心场景（proto getter 断言改为
// 直接读 ChatRequestParams 结构字段）。
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

func buildParams(t *testing.T, request llm.RequestMessages) (devin.ChatRequestParams, llm.RequestRepairs) {
	t.Helper()
	params, repairs, err := buildRequestParams(request, "tok", "3000.2.17", "mac", "model", "")
	if err != nil {
		t.Fatal(err)
	}
	return params, repairs
}

func TestBuildRequestMapsLoopMessages(t *testing.T) {
	request := llm.RequestMessages{
		SystemPrompt: "system",
		Messages: []llm.Message{
			llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hello"}}},
			llm.AssistantMessage{Content: []llm.Content{
				llm.ThinkingContent{Thinking: "think", ThinkingSignature: "sig"},
				llm.ToolCall{ID: "call-1", Name: "exec", Arguments: json.RawMessage(`{"command":"ls"}`)},
			}},
			llm.ToolResultMessage{ToolCallID: "call-1", IsError: true, Content: []llm.Content{llm.TextContent{Text: "failed"}}},
		},
		Tools: []llm.ToolDefinition{
			{Name: "exec", Description: "run", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "read", Description: "read file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
		},
	}
	params, _ := buildParams(t, request)
	wantPrompt := "system\n\n# tools descriptions\n<tool name=\"exec\">\n1. run\n</tool>\n<tool name=\"read\">\n1. read file\n</tool>"
	if params.SystemPrompt != wantPrompt {
		t.Fatalf("system prompt = %q, want %q", params.SystemPrompt, wantPrompt)
	}
	if len(params.Prompts) != 3 {
		t.Fatalf("prompt count = %d, want 3", len(params.Prompts))
	}
	assistant := params.Prompts[1]
	if assistant.Source != devin.SourceAssistant || assistant.Text != "" {
		t.Fatalf("assistant prompt = %+v", assistant)
	}
	if assistant.Thinking != "think" || assistant.Signature != "sig" || len(assistant.ToolCalls) != 1 {
		t.Fatalf("assistant prompt = %+v", assistant)
	}
	call := assistant.ToolCalls[0]
	if call.Name != "exec" || call.ArgumentsJSON != `{"command":"ls"}` {
		t.Fatalf("tool call = %+v", call)
	}
	result := params.Prompts[2]
	if result.Source != devin.SourceTool || result.ToolCallID != "call-1" || !result.ToolResultIsErr {
		t.Fatalf("tool result = %+v", result)
	}
	if len(params.Tools) != 2 || params.Tools[0].Name != "exec" || params.Tools[0].Description != "exec" {
		t.Fatalf("tools = %+v", params.Tools)
	}
}

func TestBuildRequestMergesParallelToolCalls(t *testing.T) {
	request := llm.RequestMessages{
		Messages: []llm.Message{
			llm.AssistantMessage{Content: []llm.Content{
				llm.TextContent{Text: "checking"},
				llm.ToolCall{ID: "a", Name: "read", Arguments: json.RawMessage(`{}`)},
				llm.ToolCall{ID: "b", Name: "read", Arguments: json.RawMessage(`{}`)},
			}},
			llm.ToolResultMessage{ToolCallID: "a", Content: []llm.Content{llm.TextContent{Text: "ra"}}},
			llm.ToolResultMessage{ToolCallID: "b", Content: []llm.Content{llm.TextContent{Text: "rb"}}},
		},
	}
	params, _ := buildParams(t, request)
	if len(params.Prompts) != 3 {
		t.Fatalf("prompts = %d, want 3 (merged call + 2 results)", len(params.Prompts))
	}
	if len(params.Prompts[0].ToolCalls) != 2 || params.Prompts[0].Text != "checking" {
		t.Fatalf("merged call prompt = %+v", params.Prompts[0])
	}
	if params.Prompts[1].ToolCallID != "a" || params.Prompts[2].ToolCallID != "b" {
		t.Fatalf("results not interleaved: %+v", params.Prompts)
	}
}

func TestBuildRequestDemotesOrphanToolResult(t *testing.T) {
	request := llm.RequestMessages{
		Messages: []llm.Message{
			llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hi"}}},
			llm.ToolResultMessage{ToolCallID: "lost", Content: []llm.Content{llm.TextContent{Text: "data"}}},
		},
	}
	params, repairs := buildParams(t, request)
	if repairs.DemotedOrphanResults != 1 {
		t.Fatalf("demoted = %d", repairs.DemotedOrphanResults)
	}
	last := params.Prompts[len(params.Prompts)-1]
	if last.Source != devin.SourceUser || !strings.Contains(last.Text, "data") {
		t.Fatalf("orphan not demoted: %+v", last)
	}
}

func TestBuildRequestImagesOnlyOnLatestTurn(t *testing.T) {
	img := llm.ImageContent{Data: "aGk=", MIMEType: "image/png"}
	request := llm.RequestMessages{
		Messages: []llm.Message{
			llm.UserMessage{Content: []llm.Content{img, llm.TextContent{Text: "old"}}},
			llm.AssistantMessage{Content: []llm.Content{llm.TextContent{Text: "seen"}}},
			llm.UserMessage{Content: []llm.Content{img, llm.TextContent{Text: "now"}}},
		},
	}
	params, repairs := buildParams(t, request)
	if len(params.Prompts[0].Images) != 0 || !strings.Contains(params.Prompts[0].Text, "[Image omitted from history]") {
		t.Fatalf("historical image must be demoted to text: %+v", params.Prompts[0])
	}
	if repairs.OmittedHistoryImages != 1 {
		t.Fatalf("omitted = %d", repairs.OmittedHistoryImages)
	}
	last := params.Prompts[len(params.Prompts)-1]
	if len(last.Images) != 1 || last.Images[0].Data != "aGk=" {
		t.Fatalf("latest-turn image missing: %+v", last)
	}
}

func TestBuildRequestToolChoice(t *testing.T) {
	base := llm.RequestMessages{
		Messages: []llm.Message{llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "x"}}}},
		Tools:    []llm.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	}
	none := llm.ToolChoice{Mode: llm.ToolChoiceNone}
	params, _ := buildParams(t, withChoice(base, &none))
	if params.ToolChoiceOption != "none" || params.ToolChoiceTool != "" {
		t.Fatalf("none: %+v", params)
	}
	named := llm.ToolChoice{Mode: llm.ToolChoiceNamed, ToolName: "read"}
	params, _ = buildParams(t, withChoice(base, &named))
	if params.ToolChoiceTool != "read" {
		t.Fatalf("named: %+v", params)
	}
	bad := llm.ToolChoice{Mode: llm.ToolChoiceNamed, ToolName: "ghost"}
	if _, _, err := buildRequestParams(withChoice(base, &bad), "t", "v", "o", "m", ""); err == nil {
		t.Fatalf("named choice for missing tool must fail")
	}
}

func withChoice(r llm.RequestMessages, c *llm.ToolChoice) llm.RequestMessages {
	r.ToolChoice = c
	return r
}

func TestDeriveSessionIDs(t *testing.T) {
	msg := llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hello"}}}
	r1 := llm.RequestMessages{SessionKey: "sess-1", Messages: []llm.Message{msg}}
	r2 := llm.RequestMessages{SessionKey: "sess-1", Messages: []llm.Message{msg, llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "more"}}}}}
	t1, c1 := deriveSessionIDs(r1)
	t2, c2 := deriveSessionIDs(r2)
	if t1 != t2 || c1 != c2 {
		t.Fatalf("same SessionKey must derive same IDs across compaction")
	}
	r3 := llm.RequestMessages{SessionKey: "sess-2", Messages: []llm.Message{msg}}
	t3, _ := deriveSessionIDs(r3)
	if t3 == t1 {
		t.Fatalf("different SessionKey must derive different trajectory")
	}
}

func TestNextStepIndexSequence(t *testing.T) {
	traj := "test-trajectory-seq"
	if got := nextStepIndex(traj); got != 0 {
		t.Fatalf("first step = %d, want 0 (absent on wire)", got)
	}
	if got := nextStepIndex(traj); got != 2 {
		t.Fatalf("second step = %d, want 2", got)
	}
	if got := nextStepIndex(traj); got != 3 {
		t.Fatalf("third step = %d, want 3", got)
	}
}

func decodeFrames(t *testing.T, decoder *responseDecoder, frames ...*devin.ChatResponseFrame) []llm.ResponseEvent {
	t.Helper()
	var events []llm.ResponseEvent
	for _, frame := range frames {
		events = append(events, decoder.decode(frame)...)
	}
	return events
}

func TestResponseDecoderBasicFlow(t *testing.T) {
	decoder := newResponseDecoder("m", nil, nil)
	decoder.start()
	events := decodeFrames(t, decoder,
		&devin.ChatResponseFrame{DeltaThinking: "hmm", DeltaSignature: "s1", DeltaSignatureType: "sealed"},
		&devin.ChatResponseFrame{DeltaText: "Hel"},
		&devin.ChatResponseFrame{DeltaText: "lo", StopReason: devin.StopReasonStopPattern},
		&devin.ChatResponseFrame{Usage: &devin.ChatUsage{Input: 10, Output: 5}},
	)
	events = append(events, decoder.finish(nil)...)
	var hasThink, hasText, hasDone bool
	for _, e := range events {
		switch e.Type {
		case llm.ResponseEventThinkingStart, llm.ResponseEventThinkingDelta:
			hasThink = true
		case llm.ResponseEventTextDelta:
			hasText = true
		case llm.ResponseEventDone:
			hasDone = true
			if e.Message == nil || e.Message.StopReason != llm.StopReasonStop {
				t.Fatalf("done reason = %+v", e.Message)
			}
			if e.Message.Usage.Input != 10 || e.Message.Usage.Output != 5 {
				t.Fatalf("usage = %+v", e.Message.Usage)
			}
		}
	}
	if !hasThink || !hasText || !hasDone {
		t.Fatalf("events = %+v", events)
	}
	// thinking 块应带 signature + type。
	th, ok := decoder.partial.Content[0].(llm.ThinkingContent)
	if !ok || th.ThinkingSignature != "s1" || th.SignatureType != "sealed" {
		t.Fatalf("thinking = %+v", decoder.partial.Content[0])
	}
}

func TestResponseDecoderToolCall(t *testing.T) {
	decoder := newResponseDecoder("m", nil, nil)
	decoder.start()
	events := decodeFrames(t, decoder,
		&devin.ChatResponseFrame{ToolDeltas: []devin.ChatToolDelta{{ID: "c1", Name: "read"}}},
		&devin.ChatResponseFrame{ToolDeltas: []devin.ChatToolDelta{{ID: "c1", Args: `{"pa`, HasArgs: true}}},
		&devin.ChatResponseFrame{ToolDeltas: []devin.ChatToolDelta{{ID: "c1", Args: `th":"x"}`, HasArgs: true}}, StopReason: devin.StopReasonFunctionCall},
	)
	events = append(events, decoder.finish(nil)...)
	var done *llm.ResponseEvent
	for i := range events {
		if events[i].Type == llm.ResponseEventDone {
			done = &events[i]
		}
	}
	if done == nil || done.Message.StopReason != llm.StopReasonToolUse {
		t.Fatalf("done = %+v", done)
	}
	call, ok := done.Message.Content[0].(llm.ToolCall)
	if !ok || call.ID != "c1" || string(call.Arguments) != `{"path":"x"}` {
		t.Fatalf("call = %+v", done.Message.Content[0])
	}
}

func TestResponseDecoderEOFWithoutStopReasonFails(t *testing.T) {
	decoder := newResponseDecoder("m", nil, nil)
	decoder.start()
	decodeFrames(t, decoder, &devin.ChatResponseFrame{DeltaText: "x"})
	events := decoder.finish(nil)
	if len(events) != 1 || events[0].Type != llm.ResponseEventError {
		t.Fatalf("truncated stream must fail: %+v", events)
	}
}

func TestResponseDecoderLocalStopSequence(t *testing.T) {
	decoder := newResponseDecoder("m", []string{"STOP"}, nil)
	decoder.start()
	var deltas []string
	events := decodeFrames(t, decoder,
		&devin.ChatResponseFrame{DeltaText: "abcST"},
		&devin.ChatResponseFrame{DeltaText: "OPtail", StopReason: devin.StopReasonStopPattern},
	)
	events = append(events, decoder.finish(nil)...)
	for _, e := range events {
		if e.Type == llm.ResponseEventTextDelta {
			deltas = append(deltas, e.Delta)
		}
		if e.Type == llm.ResponseEventDone {
			if e.Message.StopReason != llm.StopReasonStopSequence || e.Message.StopSequence != "STOP" {
				t.Fatalf("done = %+v", e.Message)
			}
		}
	}
	if got := strings.Join(deltas, ""); got != "abc" {
		t.Fatalf("deltas = %q, want %q", got, "abc")
	}
}

func TestRepairLeakedXMLArguments(t *testing.T) {
	out, ok := repairLeakedXMLArguments(`<parameter name="path">/a</parameter><parameter name="n">1</parameter>`)
	if !ok {
		t.Fatal("must repair")
	}
	var m map[string]string
	if err := json.Unmarshal(out, &m); err != nil || m["path"] != "/a" || m["n"] != "1" {
		t.Fatalf("repaired = %s", out)
	}
}

// fakeStream 构造预填帧的 responseStream（泵由调用方保证容量足够）。
func fakeStream(frames ...upstreamFrame) *responseStream {
	ch := make(chan upstreamFrame, len(frames)+1)
	for _, f := range frames {
		ch <- f
	}
	close(ch)
	return &responseStream{
		frames:  ch,
		cancel:  func() {},
		decoder: newResponseDecoder("m", nil, nil),
	}
}

func drainStream(t *testing.T, stream *responseStream) []llm.ResponseEvent {
	t.Helper()
	var events []llm.ResponseEvent
	for {
		event, err := stream.Recv(context.Background())
		if err == io.EOF {
			return events
		}
		if err != nil {
			t.Fatalf("recv: %v", err)
		}
		events = append(events, event)
	}
}

func TestResponseStreamYieldsErrorBeforeStart(t *testing.T) {
	stream := fakeStream(upstreamFrame{err: &devin.ConnectError{Code: "invalid_argument", Message: "bad"}})
	events := drainStream(t, stream)
	if len(events) == 0 || events[0].Type != llm.ResponseEventError {
		t.Fatalf("first event must be error, got %+v", events)
	}
}

func TestResponseStreamEmitsHeldStartBeforeContent(t *testing.T) {
	stream := fakeStream(upstreamFrame{response: &devin.ChatResponseFrame{DeltaText: "hi", StopReason: devin.StopReasonStopPattern}})
	events := drainStream(t, stream)
	if len(events) == 0 || events[0].Type != llm.ResponseEventStart {
		t.Fatalf("start must precede content, got %+v", events)
	}
}

func TestResponseStreamReleasesStartOnHoldTimeout(t *testing.T) {
	old := startHoldTimeout
	startHoldTimeout = 20 * time.Millisecond
	defer func() { startHoldTimeout = old }()
	// 永不投递的帧通道：start 扣留到期后应单独下发。
	ch := make(chan upstreamFrame)
	stream := &responseStream{frames: ch, cancel: func() {}, decoder: newResponseDecoder("m", nil, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	event, err := stream.Recv(ctx)
	if err != nil || event.Type != llm.ResponseEventStart {
		t.Fatalf("held start = %v, %v", event, err)
	}
}

func TestResponseStreamStopsOnContextCancel(t *testing.T) {
	ch := make(chan upstreamFrame)
	stream := &responseStream{frames: ch, cancel: func() {}, decoder: newResponseDecoder("m", nil, nil)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	// 首事件为扣留的 start 或直接 error；取消后流必须终止而不是挂死。
	_, _ = stream.Recv(ctx)
	for {
		_, err := stream.Recv(ctx)
		if err != nil {
			return
		}
	}
}

func TestSanitizeRequestCompetitorFingerprint(t *testing.T) {
	request := llm.RequestMessages{
		SystemPrompt: "You are Claude Code, Anthropic's official CLI for Claude.",
		Messages:     []llm.Message{llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hi"}}}},
	}
	out, hits := sanitizeRequest(request)
	if len(hits) == 0 || strings.Contains(out.SystemPrompt, "Claude Code") {
		t.Fatalf("fingerprint not sanitized: %q hits=%v", out.SystemPrompt, hits)
	}
}

func TestSanitizeInjectsEmptySystemWithTools(t *testing.T) {
	request := llm.RequestMessages{
		Messages: []llm.Message{llm.UserMessage{Content: []llm.Content{llm.TextContent{Text: "hi"}}}},
		Tools:    []llm.ToolDefinition{{Name: "t", InputSchema: json.RawMessage(`{}`)}},
	}
	out, hits := sanitizeRequest(request)
	if strings.TrimSpace(out.SystemPrompt) == "" || hits["inject-empty-system"] == 0 {
		t.Fatalf("empty system with tools must inject: %q", out.SystemPrompt)
	}
}

func TestConnectErrorClassification(t *testing.T) {
	if !devin.IsCode(&devin.ConnectError{Code: "resource_exhausted"}, "resource_exhausted") {
		t.Fatal("IsCode failed")
	}
	if devin.IsTransientTransportError(&devin.ConnectError{Code: "unavailable"}) {
		t.Fatal("unavailable is a semantic error, not transient")
	}
	if devin.IsTransientTransportError(errors.New("x")) != true {
		t.Fatal("plain transport error should be transient")
	}
}
