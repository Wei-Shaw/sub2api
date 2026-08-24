package kiro

import (
	"strings"
	"testing"
)

// 复现: 当 tool_result.content 是 [{type:input_text,text:...}] (codex 经 responses->anthropic 后的形状),
// Kiro translator 是否能提取到文本。若丢失, 模型将看到空工具结果。
func TestKiroExtractsInputTextToolResult(t *testing.T) {
	// content 用 input_text (codex 形状)
	bodyInputText := `{
	  "model":"gpt-5.6-sol",
	  "messages":[
	    {"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"exec","input":{"cmd":"pwd"}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"input_text","text":"MARKER_INPUT_TEXT_999"}]}]}
	  ]
	}`
	res, err := BuildKiroPayloadWithContext([]byte(bodyInputText), "gpt-5.6-sol", "", "AI_EDITOR", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.Payload), "MARKER_INPUT_TEXT_999") {
		t.Logf("input_text: 存活 ✓")
	} else {
		t.Errorf("input_text: 丢失 ✗ (模型看到空工具结果)")
	}

	// 对照组: content 用 text (标准 Anthropic 形状)
	bodyText := strings.Replace(bodyInputText, `"type":"input_text","text":"MARKER_INPUT_TEXT_999"`, `"type":"text","text":"MARKER_TEXT_888"`, 1)
	res2, _ := BuildKiroPayloadWithContext([]byte(bodyText), "gpt-5.6-sol", "", "AI_EDITOR", nil)
	if strings.Contains(string(res2.Payload), "MARKER_TEXT_888") {
		t.Logf("text(对照): 存活 ✓")
	} else {
		t.Errorf("text(对照): 丢失 ✗")
	}
}
