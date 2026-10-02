package apicompat

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This oracle reconstructs only the public Anthropic SSE protocol. It does not
// call any reverse converter or infer expected arguments from product state.
func contextResponseAssemble(events []AnthropicStreamEvent) (map[string]string, error) {
	tools := map[string]string{}
	open := -1
	next := 0
	stopped := false
	var block *AnthropicContentBlock
	var args strings.Builder
	for _, e := range events {
		if stopped {
			return nil, fmt.Errorf("event %s after message_stop", e.Type)
		}
		switch e.Type {
		case "content_block_start":
			if open >= 0 || e.Index == nil || *e.Index != next || e.ContentBlock == nil {
				return nil, fmt.Errorf("invalid block start: index=%v open=%d next=%d", e.Index, open, next)
			}
			open = *e.Index
			block = e.ContentBlock
			args.Reset()
		case "content_block_delta":
			if e.Index == nil || open < 0 || *e.Index != open {
				return nil, fmt.Errorf("delta outside its open block: index=%v open=%d", e.Index, open)
			}
			if e.Delta != nil && e.Delta.Type == "input_json_delta" {
				_, _ = args.WriteString(e.Delta.PartialJSON)
			}
		case "content_block_stop":
			if e.Index == nil || open < 0 || *e.Index != open {
				return nil, fmt.Errorf("stop outside its open block")
			}
			if block.Type == "tool_use" {
				raw := args.String()
				if raw == "" {
					raw = string(block.Input)
				}
				var obj map[string]any
				if err := json.Unmarshal([]byte(raw), &obj); err != nil || obj == nil {
					return nil, fmt.Errorf("tool %s invalid JSON object %q", block.ID, raw)
				}
				canonical, _ := json.Marshal(obj)
				tools[block.ID] = string(canonical)
			}
			open = -1
			next++
		case "message_stop":
			if open >= 0 {
				return nil, fmt.Errorf("message stopped with open block")
			}
			stopped = true
		}
	}
	if open >= 0 {
		return nil, fmt.Errorf("stream left block %d open", open)
	}
	return tools, nil
}

func contextResponseTrace(t *testing.T, input []*ResponsesStreamEvent, output []AnthropicStreamEvent, expected map[string]string) {
	t.Helper()
	dir := os.Getenv("CONTEXT_RESPONSE_TRACE_DIR")
	if dir == "" {
		return
	}
	dir = filepath.Join(dir, strings.ReplaceAll(t.Name(), "/", "_"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var upstream, downstream strings.Builder
	for _, e := range input {
		b, _ := json.Marshal(e)
		fmt.Fprintf(&upstream, "event: %s\ndata: %s\n\n", e.Type, b)
	}
	for _, e := range output {
		b, _ := json.Marshal(e)
		fmt.Fprintf(&downstream, "event: %s\ndata: %s\n\n", e.Type, b)
	}
	for name, data := range map[string]string{"L3.responses.sse": upstream.String(), "L4.anthropic.sse": downstream.String()} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := json.MarshalIndent(expected, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "expected-tools.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
}

func contextResponseConvert(input []*ResponsesStreamEvent, finalize bool) []AnthropicStreamEvent {
	state := NewResponsesEventToAnthropicState()
	var out []AnthropicStreamEvent
	for _, e := range input {
		out = append(out, ResponsesEventToAnthropicEvents(e, state)...)
	}
	if finalize {
		out = append(out, FinalizeResponsesAnthropicStream(state)...)
	}
	return out
}

func contextResponseCreated() *ResponsesStreamEvent {
	return &ResponsesStreamEvent{Type: "response.created", Response: &ResponsesResponse{ID: "resp_context_fixture", Model: "gpt-6.1-sol", Status: "in_progress"}}
}
func contextResponseCompleted() *ResponsesStreamEvent {
	return &ResponsesStreamEvent{Type: "response.completed", Response: &ResponsesResponse{Status: "completed"}}
}
func contextResponseToolAdded(i int, name string) *ResponsesStreamEvent {
	return &ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: i, Item: &ResponsesOutput{Type: "function_call", ID: fmt.Sprintf("fc_%d", i), CallID: fmt.Sprintf("call_%d", i), Name: name}}
}
func contextResponseAssertTools(t *testing.T, input []*ResponsesStreamEvent, expected map[string]string) {
	t.Helper()
	out := contextResponseConvert(input, false)
	contextResponseTrace(t, input, out, expected)
	got, err := contextResponseAssemble(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(expected) {
		t.Fatalf("tools lost: got %v want %v", got, expected)
	}
	for id, want := range expected {
		if got[id] != want {
			t.Errorf("tool %s arguments: got %q want %q", id, got[id], want)
		}
	}
}

func TestContextResponseInterleavedToolArguments(t *testing.T) {
	input := []*ResponsesStreamEvent{contextResponseCreated(), contextResponseToolAdded(0, "Read"), contextResponseToolAdded(1, "Edit"),
		{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: `{"file_path":"src/`},
		{Type: "response.function_call_arguments.delta", OutputIndex: 1, Delta: `{"replacement":"`},
		{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: `sample.txt","pages":""}`},
		{Type: "response.function_call_arguments.done", OutputIndex: 0, Arguments: `{"file_path":"src/sample.txt","pages":""}`},
		{Type: "response.function_call_arguments.delta", OutputIndex: 1, Delta: `updated"}`},
		{Type: "response.function_call_arguments.done", OutputIndex: 1, Arguments: `{"replacement":"updated"}`}, contextResponseCompleted()}
	contextResponseAssertTools(t, input, map[string]string{"call_0": `{"file_path":"src/sample.txt"}`, "call_1": `{"replacement":"updated"}`})
}
func TestContextResponsePackedParallelToolArguments(t *testing.T) {
	input := []*ResponsesStreamEvent{contextResponseCreated()}
	for i := 0; i < 3; i++ {
		input = append(input, contextResponseToolAdded(i, "GetState"))
	}
	for i := 0; i < 3; i++ {
		input = append(input, &ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: i, Arguments: fmt.Sprintf(`{"revision":%d}`, i)})
	}
	input = append(input, contextResponseCompleted())
	contextResponseAssertTools(t, input, map[string]string{"call_0": `{"revision":0}`, "call_1": `{"revision":1}`, "call_2": `{"revision":2}`})
}
func TestContextResponseToolArgumentsFromItemDone(t *testing.T) {
	input := []*ResponsesStreamEvent{contextResponseCreated(), contextResponseToolAdded(0, "GetState"), {Type: "response.output_item.done", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", ID: "fc_0", CallID: "call_0", Name: "GetState", Arguments: `{"revision":9}`}}, contextResponseCompleted()}
	contextResponseAssertTools(t, input, map[string]string{"call_0": `{"revision":9}`})
}
func TestContextResponseTerminalOnlyToolCall(t *testing.T) {
	input := []*ResponsesStreamEvent{contextResponseCreated(), {Type: "response.completed", Response: &ResponsesResponse{Status: "completed", Output: []ResponsesOutput{{Type: "function_call", ID: "fc_0", CallID: "call_0", Name: "GetState", Arguments: `{"revision":9}`}}}}}
	contextResponseAssertTools(t, input, map[string]string{"call_0": `{"revision":9}`})
	out := contextResponseConvert(input, false)
	found := false
	for _, e := range out {
		if e.Type == "message_delta" && e.Delta != nil && e.Delta.StopReason == "tool_use" {
			found = true
		}
	}
	if !found {
		t.Fatal("terminal tool call must finish with tool_use")
	}
}

// Deterministic schedules vary per-tool chunking and cross-tool interleaving,
// while preserving added -> deltas -> done order for each Responses item.
func TestContextResponseToolInterleavings100Seeds(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			input := []*ResponsesStreamEvent{contextResponseCreated()}
			expected := map[string]string{}
			names := []string{"Read", "Edit", "GetState"}
			raw := []string{`{"file_path":"state.json","pages":""}`, `{"replacement":"next"}`, `{"revision":3}`}
			queues := make([][]*ResponsesStreamEvent, 3)
			for i := 0; i < 3; i++ {
				input = append(input, contextResponseToolAdded(i, names[i]))
				expected[fmt.Sprintf("call_%d", i)] = raw[i]
				if i == 0 {
					expected["call_0"] = `{"file_path":"state.json"}`
				}
				for offset := 0; offset < len(raw[i]); {
					n := 1 + rng.Intn(7)
					if offset+n > len(raw[i]) {
						n = len(raw[i]) - offset
					}
					queues[i] = append(queues[i], &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: i, Delta: raw[i][offset : offset+n]})
					offset += n
				}
				queues[i] = append(queues[i], &ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: i, Arguments: raw[i]})
			}
			for {
				candidates := []int{}
				for i, q := range queues {
					if len(q) > 0 {
						candidates = append(candidates, i)
					}
				}
				if len(candidates) == 0 {
					break
				}
				i := candidates[rng.Intn(len(candidates))]
				input = append(input, queues[i][0])
				queues[i] = queues[i][1:]
			}
			input = append(input, contextResponseCompleted())
			contextResponseAssertTools(t, input, expected)
		})
	}
}

func TestContextResponseBufferLimit(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(contextResponseCreated(), state)
	ResponsesEventToAnthropicEvents(contextResponseToolAdded(0, "Edit"), state)
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: strings.Repeat("x", (1<<20)+1)}, state)
	if len(events) != 1 || events[0].Type != "error" || events[0].Error == nil {
		t.Fatal("over-budget tool input must surface an explicit error")
	}
	if len(ResponsesEventToAnthropicEvents(contextResponseCompleted(), state)) != 0 {
		t.Fatal("buffer error must not later advertise message success")
	}
}

func TestContextResponseCompletedToolReleasesArgumentStorage(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(contextResponseCreated(), state)
	item := &ResponsesOutput{Type: "function_call", ID: "fc_0", CallID: "call_0", Name: "Edit", Arguments: `{"replacement":"complete"}`, Status: "completed"}
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: item}, state)
	tools, err := contextResponseAssemble(events)
	if err != nil || tools["call_0"] != item.Arguments {
		t.Fatalf("complete item lost: %v %v", tools, err)
	}
	tool := state.tools[0]
	if !tool.Emitted || tool.Args != "" || tool.Item.Arguments != "" || tool.Item.Input != "" || state.toolBufferBytes != 0 {
		t.Fatal("completed tool must retain identity without unbounded argument copies")
	}
}
