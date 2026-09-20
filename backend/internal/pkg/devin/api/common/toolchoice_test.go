// 本文件验证 OpenAI / Anthropic tool_choice 到中间模型的解析。
package common

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

func TestParseOpenAIToolChoice(t *testing.T) {
	cases := []struct {
		raw      string
		wantMode llm.ToolChoiceMode
		wantName string
		wantNil  bool
	}{
		{`"auto"`, "", "", true},
		{`"none"`, llm.ToolChoiceNone, "", false},
		{`"required"`, llm.ToolChoiceRequired, "", false},
		{`{"type":"function","function":{"name":"read_file"}}`, llm.ToolChoiceNamed, "read_file", false},
		// Responses API 的扁平形态。
		{`{"type":"function","name":"exec"}`, llm.ToolChoiceNamed, "exec", false},
		{`null`, "", "", true},
	}
	for _, c := range cases {
		choice, err := ParseOpenAIToolChoice(json.RawMessage(c.raw))
		if err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if c.wantNil {
			if choice != nil {
				t.Fatalf("%s: got %#v, want nil", c.raw, choice)
			}
			continue
		}
		if choice == nil || choice.Mode != c.wantMode || choice.ToolName != c.wantName {
			t.Fatalf("%s: got %#v, want mode=%q name=%q", c.raw, choice, c.wantMode, c.wantName)
		}
	}
	if _, err := ParseOpenAIToolChoice(json.RawMessage(`"bogus"`)); err == nil {
		t.Fatal("bogus option should error")
	}
}

func TestParseAnthropicToolChoice(t *testing.T) {
	// "any" 归一为 required（上游 option_name 不接受 "any"）。
	choice, disable, err := ParseAnthropicToolChoice(json.RawMessage(`{"type":"any","disable_parallel_tool_calls":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if choice == nil || choice.Mode != llm.ToolChoiceRequired || !disable {
		t.Fatalf("any => %#v disable=%v, want required + disable", choice, disable)
	}
	choice, _, err = ParseAnthropicToolChoice(json.RawMessage(`{"type":"tool","name":"Bash"}`))
	if err != nil {
		t.Fatal(err)
	}
	if choice == nil || choice.Mode != llm.ToolChoiceNamed || choice.ToolName != "Bash" {
		t.Fatalf("tool => %#v, want named Bash", choice)
	}
	choice, _, err = ParseAnthropicToolChoice(json.RawMessage(`{"type":"none"}`))
	if err != nil || choice == nil || choice.Mode != llm.ToolChoiceNone {
		t.Fatalf("none => %#v err=%v", choice, err)
	}
	choice, _, err = ParseAnthropicToolChoice(json.RawMessage(`{"type":"auto"}`))
	if err != nil || choice != nil {
		t.Fatalf("auto => %#v err=%v, want nil", choice, err)
	}
	if _, _, err = ParseAnthropicToolChoice(json.RawMessage(`{"type":"bogus"}`)); err == nil {
		t.Fatal("bogus type should error")
	}
	if _, _, err = ParseAnthropicToolChoice(json.RawMessage(`{"type":"tool"}`)); err == nil {
		t.Fatal("tool without name should error")
	}
}
