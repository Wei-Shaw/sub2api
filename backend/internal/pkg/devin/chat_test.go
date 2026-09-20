// chat_test.go 验证 GetChatMessage 请求的结构形态与响应帧解码。
package devin

import (
	"testing"
)

// collectTopFields 收集顶层字段号序列。
func collectTopFields(buf []byte) []int {
	var nums []int
	iterFields(buf, func(f protoField) bool {
		nums = append(nums, f.num)
		return true
	})
	return nums
}

func fieldBytes(buf []byte, num int) []byte {
	var out []byte
	iterFields(buf, func(f protoField) bool {
		if f.num == num && f.wire == wireBytes {
			out = f.vbytes
			return false
		}
		return true
	})
	return out
}

func TestMarshalGetChatMessageShape(t *testing.T) {
	body := MarshalGetChatMessage(ChatRequestParams{
		Token:         "devin-session-token$jwt",
		ClientVersion: "3000.2.17",
		OS:            "mac",
		SystemPrompt:  "you are helpful",
		Prompts: []ChatPrompt{
			{Source: SourceUser, Text: "hello"},
			{Source: SourceAssistant, Text: "hi", ToolCalls: []ChatToolCall{{ID: "c1", Name: "read", ArgumentsJSON: `{"path":"x"}`}}},
			{Source: SourceTool, Text: "content", ToolCallID: "c1"},
		},
		Tools:            []ChatToolDef{{Name: "read", Description: "d", SchemaJSON: `{"type":"object"}`}},
		ToolChoiceOption: "required",
		ModelUID:         "swe-2-high",
		TrajectoryID:     "11111111-2222-3333-4444-555555555555",
		CascadeID:        "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		StepIndex:        0,
	})
	nums := collectTopFields(body)
	want := []int{1, 2, 3, 3, 3, 7, 8, 10, 12, 15, 16, 20, 21}
	if len(nums) != len(want) {
		t.Fatalf("top-level fields = %v, want %v", nums, want)
	}
	for i := range want {
		if nums[i] != want[i] {
			t.Fatalf("top-level fields = %v, want %v", nums, want)
		}
	}
	// f7 REQUEST_CASCADE=5, f20 PLANNER_DEFAULT=1
	if got := fieldInt(body, 7); got != RequestTypeCascade {
		t.Fatalf("f7 = %d, want %d", got, RequestTypeCascade)
	}
	if got := fieldInt(body, 20); got != PlannerModeDefault {
		t.Fatalf("f20 = %d, want %d", got, PlannerModeDefault)
	}
	if got := fieldStr(body, 21); got != "swe-2-high" {
		t.Fatalf("f21 = %q", got)
	}
	// f15 trajectory: {1:id, 3:4, 4:14} — step_index=0 时 f2 缺席
	traj := fieldBytes(body, 15)
	if got := fieldStr(traj, 1); got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("trajectory id = %q", got)
	}
	if hasField(traj, 2) {
		t.Fatalf("step_index=0 must be absent in trajectory")
	}
	if got := fieldInt(traj, 3); got != TrajectoryCascade {
		t.Fatalf("trajectory type = %d", got)
	}
	if got := fieldInt(traj, 4); got != StepTypeUserInput {
		t.Fatalf("step type = %d", got)
	}
	// f8 completion 默认值：num=1 max_tokens=128000 newlines=400 temp=1.0 topk=40
	comp := fieldBytes(body, 8)
	if fieldInt(comp, 1) != 1 || fieldInt(comp, 2) != 128000 || fieldInt(comp, 3) != 400 || fieldInt(comp, 7) != 40 {
		t.Fatalf("completion defaults wrong: %v", collectTopFields(comp))
	}
	// f3 prompts：验证 source/toolCallId 编码
	var prompts [][]byte
	iterFields(body, func(f protoField) bool {
		if f.num == 3 {
			prompts = append(prompts, f.vbytes)
		}
		return true
	})
	if len(prompts) != 3 {
		t.Fatalf("prompts = %d", len(prompts))
	}
	if fieldInt(prompts[0], 2) != SourceUser || fieldStr(prompts[0], 3) != "hello" {
		t.Fatalf("user prompt malformed")
	}
	if fieldInt(prompts[1], 2) != SourceAssistant {
		t.Fatalf("assistant prompt malformed")
	}
	// assistant 的 toolCalls f6 → {1:id, 2:name, 3:arguments_json}
	tc := fieldBytes(prompts[1], 6)
	if fieldStr(tc, 1) != "c1" || fieldStr(tc, 2) != "read" || fieldStr(tc, 3) != `{"path":"x"}` {
		t.Fatalf("tool call malformed: %v", collectTopFields(tc))
	}
	if fieldInt(prompts[2], 2) != SourceTool || fieldStr(prompts[2], 7) != "c1" {
		t.Fatalf("tool result malformed")
	}
	// f12 tool_choice → {1: "required"}
	choice := fieldBytes(body, 12)
	if fieldStr(choice, 1) != "required" {
		t.Fatalf("tool_choice malformed")
	}
}

func TestMarshalGetChatMessageStepIndexPresent(t *testing.T) {
	body := MarshalGetChatMessage(ChatRequestParams{
		Token: "tok", TrajectoryID: "t", StepIndex: 2,
	})
	traj := fieldBytes(body, 15)
	if got := fieldInt(traj, 2); got != 2 {
		t.Fatalf("step_index = %d, want 2", got)
	}
}

func TestCustomToolCallEncoding(t *testing.T) {
	body := MarshalGetChatMessage(ChatRequestParams{
		Token: "tok",
		Prompts: []ChatPrompt{{
			Source:    SourceAssistant,
			ToolCalls: []ChatToolCall{{ID: "c1", Name: "apply_patch", ArgumentsJSON: "*** patch", Custom: true}},
		}},
	})
	var prompt []byte
	iterFields(body, func(f protoField) bool {
		if f.num == 3 {
			prompt = f.vbytes
		}
		return true
	})
	tc := fieldBytes(prompt, 6)
	// custom 调用走 f4 invalid_json_str + f6 is_custom_tool_call=true
	if fieldStr(tc, 4) != "*** patch" || !fieldBool(tc, 6) {
		t.Fatalf("custom tool call malformed")
	}
	if hasField(tc, 3) {
		t.Fatalf("custom tool call must not set arguments_json")
	}
}

func TestDecodeChatResponseFrame(t *testing.T) {
	var frame []byte
	frame = appendString(frame, 1, "msg-1")
	frame = appendString(frame, 3, "Hello")
	frame = appendVarintField(frame, 5, 10) // FUNCTION_CALL
	frame = appendMessage(frame, 6, func() []byte {
		var tc []byte
		tc = appendString(tc, 1, "call_1")
		tc = appendString(tc, 2, "read")
		tc = appendString(tc, 3, `{"path":`)
		return tc
	}())
	frame = appendMessage(frame, 7, func() []byte {
		var u []byte
		u = appendVarintField(u, 2, 100)
		u = appendVarintField(u, 3, 50)
		u = appendVarintField(u, 4, 10)
		u = appendVarintField(u, 5, 20)
		u = appendString(u, 9, "swe-2-high")
		return u
	}())
	got := DecodeChatResponseFrame(frame)
	if got.MessageID != "msg-1" || got.DeltaText != "Hello" {
		t.Fatalf("frame decode: %+v", got)
	}
	if got.StopReason != StopReasonFunctionCall {
		t.Fatalf("stop reason = %d", got.StopReason)
	}
	if MapStopReason(got.StopReason) != "toolUse" {
		t.Fatalf("MapStopReason(10) = %q", MapStopReason(got.StopReason))
	}
	if len(got.ToolDeltas) != 1 || got.ToolDeltas[0].ID != "call_1" || !got.ToolDeltas[0].HasArgs {
		t.Fatalf("tool deltas: %+v", got.ToolDeltas)
	}
	if got.Usage == nil || got.Usage.Input != 100 || got.Usage.Output != 50 ||
		got.Usage.CacheWrite != 10 || got.Usage.CacheRead != 20 || got.Usage.ModelUID != "swe-2-high" {
		t.Fatalf("usage: %+v", got.Usage)
	}
}

// 测试辅助：取顶层 varint/string 字段值。
func fieldInt(buf []byte, num int) uint64 {
	var out uint64
	iterFields(buf, func(f protoField) bool {
		if f.num == num && f.wire == wireVarint {
			out = f.vuint
			return false
		}
		return true
	})
	return out
}

func fieldStr(buf []byte, num int) string {
	var out string
	iterFields(buf, func(f protoField) bool {
		if f.num == num && f.wire == wireBytes {
			out = f.string()
			return false
		}
		return true
	})
	return out
}

func fieldBool(buf []byte, num int) bool { return fieldInt(buf, num) != 0 }

func hasField(buf []byte, num int) bool {
	found := false
	iterFields(buf, func(f protoField) bool {
		if f.num == num {
			found = true
			return false
		}
		return true
	})
	return found
}
