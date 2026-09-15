// 本文件验证 OpenAI Responses 请求能保留中间模型需要的上下文语义。
package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin/llm"
)

// TestDecodeRequestBuildsConversationContext 验证系统提示词、多模态输入、工具和工具结果的保留。
func TestDecodeRequestBuildsConversationContext(t *testing.T) {
	data := []byte(`{
  "model": "gpt-test",
  "instructions": "你是一个谨慎的助手。",
  "input": [
    {"type":"message","role":"user","content":[
      {"type":"input_text","text":"读取这个文件"},
      {"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo="}
    ]},
    {"type":"function_call","call_id":"call-1","name":"read_file","arguments":"{\"path\":\"a.txt\"}"},
    {"type":"function_call_output","call_id":"call-1","output":"内容"}
  ],
  "tools": [{"type":"function","name":"read_file","description":"读取文件","parameters":{"type":"object"}}]
}`)

	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if request.Context.Model != "gpt-test" {
		t.Fatalf("Model = %q, want gpt-test", request.Context.Model)
	}
	if request.Context.SystemPrompt != "你是一个谨慎的助手。" {
		t.Fatalf("SystemPrompt = %q", request.Context.SystemPrompt)
	}
	if len(request.Context.Messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(request.Context.Messages))
	}
	if _, ok := request.Context.Messages[0].(llm.UserMessage); !ok {
		t.Fatalf("message[0] type = %T, want llm.UserMessage", request.Context.Messages[0])
	}
	if _, ok := request.Context.Messages[1].(llm.AssistantMessage); !ok {
		t.Fatalf("message[1] type = %T, want llm.AssistantMessage", request.Context.Messages[1])
	}
	if _, ok := request.Context.Messages[2].(llm.ToolResultMessage); !ok {
		t.Fatalf("message[2] type = %T, want llm.ToolResultMessage", request.Context.Messages[2])
	}
	if len(request.Context.Tools) != 1 || request.Context.Tools[0].Name != "read_file" {
		t.Fatalf("tools = %#v", request.Context.Tools)
	}
	if err := request.Context.Validate(); err != nil {
		t.Fatalf("context validation error = %v", err)
	}
}

// TestDecodeRequestAcceptsStringInput 验证紧凑字符串输入会转换为用户文字消息。
func TestDecodeRequestAcceptsStringInput(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"model":"gpt-test","input":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(request.Context.Messages))
	}
	message, ok := request.Context.Messages[0].(llm.UserMessage)
	if !ok {
		t.Fatalf("message[0] type = %T", request.Context.Messages[0])
	}
	content, ok := message.Content[0].(llm.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T", message.Content[0])
	}
	if content.Text != "hello" {
		t.Fatalf("text = %q, want hello", content.Text)
	}
}

// TestDecodeRequestAcceptsImageURLObject 验证 IDE 常见的 image_url 对象形态可解码。
func TestDecodeRequestAcceptsImageURLObject(t *testing.T) {
	data := []byte(`{
  "model":"gpt-test",
  "input":[{"role":"user","content":[
    {"type":"input_text","text":"see"},
    {"type":"input_image","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}
  ]}]
}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	user, ok := request.Context.Messages[0].(llm.UserMessage)
	if !ok {
		t.Fatalf("message[0] type = %T", request.Context.Messages[0])
	}
	if len(user.Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(user.Content))
	}
	img, ok := user.Content[1].(llm.ImageContent)
	if !ok {
		t.Fatalf("content[1] = %T, want ImageContent", user.Content[1])
	}
	if img.MIMEType != "image/png" || img.Data == "" || strings.HasPrefix(img.Data, "data:") {
		t.Fatalf("image = %#v", img)
	}
}

// TestDecodeRequestAcceptsChatCompletionsImagePart 验证 type=image_url 的 Chat 风格 part。
func TestDecodeRequestAcceptsChatCompletionsImagePart(t *testing.T) {
	data := []byte(`{
  "model":"gpt-test",
  "input":[{"role":"user","content":[
    {"type":"text","text":"see"},
    {"type":"image_url","image_url":{"url":"data:image/png;base64,iVBORw0KGgo="}}
  ]}]
}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	user, _ := request.Context.Messages[0].(llm.UserMessage)
	if _, ok := user.Content[1].(llm.ImageContent); !ok {
		t.Fatalf("content[1] = %#v, want ImageContent", user.Content[1])
	}
}

// TestDecodeRequestRejectsInvalidToolArguments 验证适配器调用前会拒绝非对象工具参数。
func TestDecodeRequestRejectsInvalidToolArguments(t *testing.T) {
	data := []byte(`{"model":"gpt-test","input":[{"type":"function_call","call_id":"call-1","name":"tool","arguments":"[]"}]}`)
	if _, err := DecodeRequest(data); err == nil {
		t.Fatal("DecodeRequest() error = nil, want invalid arguments error")
	}
}

// TestDecodeRequestRetainsRawSchema 验证工具 schema 会以原始 JSON 保留。
func TestDecodeRequestRetainsRawSchema(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"model":"gpt-test","tools":[{"type":"function","name":"tool","parameters":{"type":"object","additionalProperties":false}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(request.Context.Tools[0].InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema = %#v", schema)
	}
}

// TestDecodeRequestCustomToolDeclaration 验证 type:"custom" 工具被包装成
// 单 input 参数的 function 声明并带 Custom 标记；format.definition 是模型
// 可见的唯一语法规范，随 description 注入；未知工具类型记 Dropped。
func TestDecodeRequestCustomToolDeclaration(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"model":"gpt-test","tools":[
		{"type":"custom","name":"apply_patch","description":"Patch files","format":{"syntax":"lark","definition":"patch_grammar"}},
		{"type":"custom","name":"no_grammar"},
		{"type":"mystery","name":"dropped_tool"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Tools) != 2 {
		t.Fatalf("tools = %#v, want 2 decoded", request.Context.Tools)
	}
	tool := request.Context.Tools[0]
	if !tool.Custom || tool.Name != "apply_patch" {
		t.Fatalf("tool = %#v, want custom apply_patch", tool)
	}
	if string(tool.InputSchema) != string(customToolInputSchema) {
		t.Fatalf("wrapped schema = %s, want %s", tool.InputSchema, customToolInputSchema)
	}
	if tool.Description != "Patch files\n\nInput grammar (lark):\npatch_grammar" {
		t.Fatalf("description = %q, want grammar appended", tool.Description)
	}
	if plain := request.Context.Tools[1]; plain.Description != "" {
		t.Fatalf("no-format custom tool description = %q, want empty", plain.Description)
	}
	found := false
	for _, d := range request.Context.Dropped {
		if d == "tool:mystery" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dropped = %#v, want tool:mystery", request.Context.Dropped)
	}
}

// TestDecodeRequestAcceptsMessageWithoutType 的测试动机是覆盖 OpenAI 官方示例和 SDK 发送的 role 加 content 简写。
func TestDecodeRequestAcceptsMessageWithoutType(t *testing.T) {
	data := []byte(`{
  "model": "glm-5.2",
  "input": [{
    "role": "user",
    "content": [{"type":"input_text","text":"hello"}]
  }],
  "stream": true
}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(request.Context.Messages))
	}
	message, ok := request.Context.Messages[0].(llm.UserMessage)
	if !ok {
		t.Fatalf("message type = %T, want llm.UserMessage", request.Context.Messages[0])
	}
	text, ok := message.Content[0].(llm.TextContent)
	if !ok || text.Text != "hello" {
		t.Fatalf("message content = %#v", message.Content)
	}
}

// TestDecodeRequestAttachesReasoningSummary 验证 reasoning item 的 summary
// 文本挂到紧随其后的 assistant 产出上（ThinkingContent 前置块）。
func TestDecodeRequestAttachesReasoningSummary(t *testing.T) {
	data := []byte(`{
  "model": "gpt-test",
  "input": [
    {"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
    {"type":"reasoning","summary":[{"type":"summary_text","text":"计划：先读文件再改"}]},
    {"type":"message","role":"assistant","content":[{"type":"output_text","text":"好的"}]},
    {"type":"reasoning","summary":[{"type":"summary_text","text":"需要调用 read_file"}],"encrypted_content":"sealed.v1.xyz"},
    {"type":"function_call","call_id":"call-1","name":"read_file","arguments":"{}"},
    {"type":"function_call_output","call_id":"call-1","output":"内容"}
  ]
}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	// 同一回合的 assistant message 与 function_call 合并为一条消息：
	// thinking/text 按输入序保留在合并后的内容块里。
	if len(request.Context.Messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(request.Context.Messages))
	}
	assistant, _ := request.Context.Messages[1].(llm.AssistantMessage)
	thinking, ok := assistant.Content[0].(llm.ThinkingContent)
	if !ok || thinking.Thinking != "计划：先读文件再改" {
		t.Fatalf("assistant content[0] = %#v, want ThinkingContent", assistant.Content[0])
	}
	thinking, ok = assistant.Content[2].(llm.ThinkingContent)
	if !ok || thinking.Thinking != "需要调用 read_file" {
		t.Fatalf("assistant content[2] = %#v, want ThinkingContent", assistant.Content[2])
	}
	if thinking.ThinkingSignature != "sealed.v1.xyz" {
		t.Fatalf("thinking signature = %q, want sealed.v1.xyz replay", thinking.ThinkingSignature)
	}
	if _, ok := assistant.Content[3].(llm.ToolCall); !ok {
		t.Fatalf("assistant content[3] = %#v, want ToolCall", assistant.Content[3])
	}
}

// TestDecodeRequestMergesAssistantTurnItems 验证同一回合铺平的多个 input
// item（assistant message / reasoning / function_call）合并为一条
// AssistantMessage——逐 item 成消息会让 wire 上出现假回合边界，抬高
// premature end_turn 概率（issue #2）。负例：被 function_call_output
// 分隔的 assistant 产出属不同回合，不得合并。
func TestDecodeRequestMergesAssistantTurnItems(t *testing.T) {
	data := []byte(`{
	  "model": "gpt-test",
	  "input": [
	    {"type":"message","role":"user","content":[{"type":"input_text","text":"看看项目结构"}]},
	    {"type":"reasoning","summary":[{"type":"summary_text","text":"先看 README"}]},
	    {"type":"message","role":"assistant","id":"msg_1","content":[{"type":"output_text","text":"我先读 README"}]},
	    {"type":"reasoning","summary":[{"type":"summary_text","text":"需要 read_file"}],"encrypted_content":"sealed.v1.sig"},
	    {"type":"function_call","call_id":"c1","name":"read_file","arguments":"{\"path\":\"README.md\"}"},
	    {"type":"function_call_output","call_id":"c1","output":"readme 内容"},
	    {"type":"function_call","call_id":"c2","name":"list_dir","arguments":"{}"},
	    {"type":"function_call_output","call_id":"c2","output":"file list"},
	    {"type":"message","role":"user","content":[{"type":"input_text","text":"继续"}]}
	  ]
	}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Messages) != 6 {
		t.Fatalf("message count = %d, want 6", len(request.Context.Messages))
	}
	assistant, ok := request.Context.Messages[1].(llm.AssistantMessage)
	if !ok {
		t.Fatalf("message[1] type = %T, want AssistantMessage", request.Context.Messages[1])
	}
	if len(assistant.Content) != 4 {
		t.Fatalf("merged content = %#v, want 4 blocks", assistant.Content)
	}
	if thinking, ok := assistant.Content[0].(llm.ThinkingContent); !ok || thinking.Thinking != "先看 README" {
		t.Fatalf("content[0] = %#v, want turn reasoning", assistant.Content[0])
	}
	if text, ok := assistant.Content[1].(llm.TextContent); !ok || text.Text != "我先读 README" {
		t.Fatalf("content[1] = %#v, want announcement text", assistant.Content[1])
	}
	if thinking, ok := assistant.Content[2].(llm.ThinkingContent); !ok || thinking.ThinkingSignature != "sealed.v1.sig" {
		t.Fatalf("content[2] = %#v, want signed reasoning", assistant.Content[2])
	}
	call, ok := assistant.Content[3].(llm.ToolCall)
	if !ok || call.ID != "c1" || call.Name != "read_file" {
		t.Fatalf("content[3] = %#v, want read_file ToolCall", assistant.Content[3])
	}
	if assistant.StopReason != llm.StopReasonToolUse {
		t.Fatalf("StopReason = %q, want toolUse for merged turn with call", assistant.StopReason)
	}
	if assistant.OutputID != "msg_1" {
		t.Fatalf("OutputID = %q, want last non-empty msg_1", assistant.OutputID)
	}
	if _, ok := request.Context.Messages[2].(llm.ToolResultMessage); !ok {
		t.Fatalf("message[2] type = %T, want ToolResultMessage", request.Context.Messages[2])
	}
	// 负例：output 分隔后的 function_call 属下一回合，必须独立成条。
	next, ok := request.Context.Messages[3].(llm.AssistantMessage)
	if !ok || len(next.Content) != 1 {
		t.Fatalf("message[3] = %#v, want separate AssistantMessage", request.Context.Messages[3])
	}
	if call, ok := next.Content[0].(llm.ToolCall); !ok || call.ID != "c2" {
		t.Fatalf("message[3] content = %#v, want list_dir ToolCall", next.Content)
	}

	// 同回合内两条 assistant message 的文本用 "\n" 分隔拼接，
	// OutputID 取 run 内最后一个非空。
	request, err = DecodeRequest([]byte(`{"model":"m","input":[
		{"type":"message","role":"assistant","id":"msg_a","content":[{"type":"output_text","text":"第一段"}]},
		{"type":"message","role":"assistant","id":"msg_b","content":[{"type":"output_text","text":"第二段"}]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Messages) != 1 {
		t.Fatalf("message count = %d, want 1 merged", len(request.Context.Messages))
	}
	joined, _ := request.Context.Messages[0].(llm.AssistantMessage)
	texts := make([]string, 0, len(joined.Content))
	for _, block := range joined.Content {
		text, ok := block.(llm.TextContent)
		if !ok {
			t.Fatalf("content = %#v, want text blocks only", joined.Content)
		}
		texts = append(texts, text.Text)
	}
	if strings.Join(texts, "") != "第一段\n第二段" {
		t.Fatalf("joined text = %q, want newline-separated", strings.Join(texts, ""))
	}
	if joined.OutputID != "msg_b" {
		t.Fatalf("OutputID = %q, want last non-empty msg_b", joined.OutputID)
	}
}

// TestDecodeRequestDropsOrphanReasoning 验证 reasoning 后无 assistant 产出时
// 缓冲不会挂到后续 user 消息上。
func TestDecodeRequestDropsOrphanReasoning(t *testing.T) {
	data := []byte(`{
  "model": "gpt-test",
  "input": [
    {"type":"reasoning","summary":[{"type":"summary_text","text":"orphan"}]},
    {"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
  ]
}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	user, _ := request.Context.Messages[0].(llm.UserMessage)
	if len(user.Content) != 1 {
		t.Fatalf("user content = %#v, want single text block", user.Content)
	}
}

// TestDecodeRequestToleratesOrphanToolOutput 验证压缩丢失 function_call 后
// 孤立的 function_call_output 用兜底名放行，不整请求失败。
func TestDecodeRequestToleratesOrphanToolOutput(t *testing.T) {
	data := []byte(`{
  "model": "gpt-test",
  "input": [
    {"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
    {"type":"function_call_output","call_id":"call-gone","output":"残留结果"}
  ]
}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	tool, ok := request.Context.Messages[1].(llm.ToolResultMessage)
	if !ok {
		t.Fatalf("message[1] type = %T, want ToolResultMessage", request.Context.Messages[1])
	}
	if tool.ToolCallID != "call-gone" || tool.ToolName != "tool" {
		t.Fatalf("tool result = %#v", tool)
	}
}

// TestDecodeRequestIgnoresUnsupportedExtensions 的测试动机是确保上游新增字段和类型不会阻断可识别的对话内容。
func TestDecodeRequestIgnoresUnsupportedExtensions(t *testing.T) {
	request, err := DecodeRequest([]byte(`{
  "model":"model",
  "client_metadata":{"client":"codex"},
  "input":[
    {"type":"additional_tools","role":"developer","tools":[{"name":"unknown"}]},
    {"content":"untyped extension"},
    {"type":"message","role":"future_role","content":"ignored"},
    {"type":"message","role":"user","content":[
      {"type":"future_content","value":"ignored"},
      {"type":"input_text","text":"hello"}
    ]}
  ],
  "tools":[
    {"type":"web_search_preview"},
    {"type":"function","name":"known","parameters":{"type":"object"}}
  ]
}`))
	if err != nil {
		t.Fatal(err)
	}
	// 未知 item 降级为 USER 文本保住内容：additional_tools 与无 type 项
	// 各产生一条降级消息，future_role 消息被丢弃，可识别内容不受影响。
	if len(request.Context.Messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(request.Context.Messages))
	}
	for index := 0; index < 2; index++ {
		message, ok := request.Context.Messages[index].(llm.UserMessage)
		if !ok {
			t.Fatalf("message %d type = %T, want llm.UserMessage", index, request.Context.Messages[index])
		}
		text := mustText(t, message.Content)
		if !strings.HasPrefix(text, "[input item type=") {
			t.Fatalf("demoted message %d = %q", index, text)
		}
	}
	message, ok := request.Context.Messages[2].(llm.UserMessage)
	if !ok {
		t.Fatalf("message type = %T, want llm.UserMessage", request.Context.Messages[2])
	}
	if len(message.Content) != 1 || mustText(t, message.Content) != "hello" {
		t.Fatalf("message content = %#v", message.Content)
	}
	if len(request.Context.Tools) != 1 || request.Context.Tools[0].Name != "known" {
		t.Fatalf("tools = %#v", request.Context.Tools)
	}
}

// TestDecodeRequestAcceptsCallIDVariants 验证 function_call_output 的调用
// ID 四种字段名都被接受：call_id 是规范，其余来自 Chat 习惯/驼峰/id 直用。
func TestDecodeRequestAcceptsCallIDVariants(t *testing.T) {
	for _, field := range []string{"call_id", "tool_call_id", "callId", "id"} {
		data := []byte(`{"model":"m","input":[
			{"type":"function_call","call_id":"c1","name":"t","arguments":"{}"},
			{"type":"function_call_output","` + field + `":"c1","output":"ok"}
		]}`)
		request, err := DecodeRequest(data)
		if err != nil {
			t.Fatalf("%s: %v", field, err)
		}
		result, ok := request.Context.Messages[1].(llm.ToolResultMessage)
		if !ok || result.ToolCallID != "c1" {
			t.Fatalf("%s: message[1] = %#v", field, request.Context.Messages[1])
		}
	}
}

// TestDecodeRequestReplaysOpenAIReasoningSignature 验证 openai 型签名
// （序列化 reasoning item 数组）从 encrypted_content 原样回放为
// signature+signature_type——这是 Responses 多轮 reasoning 的回放通道。
func TestDecodeRequestReplaysOpenAIReasoningSignature(t *testing.T) {
	blob := `[{"id":"rs_9","type":"reasoning","encrypted_content":"gAAA","summary":[],"content":[],"status":""}]`
	data := []byte(`{"model":"m","input":[
		{"type":"reasoning","id":"rs_9","summary":[],"encrypted_content":` + "`" + blob + "`" + `},
		{"type":"message","role":"assistant","id":"msg_7","content":[{"type":"output_text","text":"done"}]}
	]}`)
	data = []byte(strings.ReplaceAll(string(data), "`"+blob+"`", `"`+strings.ReplaceAll(blob, `"`, `\"`)+`"`))
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	assistant, ok := request.Context.Messages[0].(llm.AssistantMessage)
	if !ok {
		t.Fatalf("message[0] = %T", request.Context.Messages[0])
	}
	thinking, ok := assistant.Content[0].(llm.ThinkingContent)
	if !ok || thinking.SignatureType != "openai" || thinking.ThinkingSignature != blob {
		t.Fatalf("thinking block = %#v", assistant.Content[0])
	}
	if !thinking.Redacted {
		t.Fatal("signature-only reasoning must be marked redacted")
	}
	if assistant.OutputID != "msg_7" {
		t.Fatalf("OutputID = %q, want msg_7", assistant.OutputID)
	}
}

// TestDecodeRequestDropsForeignReasoningPayload 验证外来不透明
// encrypted_content（既非 sealed.* 也非 reasoning item JSON）不透传。
func TestDecodeRequestDropsForeignReasoningPayload(t *testing.T) {
	data := []byte(`{"model":"m","input":[
		{"type":"reasoning","summary":[],"encrypted_content":"gAAAAB-foreign"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}
	]}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	assistant, _ := request.Context.Messages[0].(llm.AssistantMessage)
	for _, block := range assistant.Content {
		if thinking, ok := block.(llm.ThinkingContent); ok && thinking.ThinkingSignature != "" {
			t.Fatalf("foreign signature must be dropped, got %#v", thinking)
		}
	}
	found := false
	for _, d := range request.Context.Dropped {
		if d == "reasoning:encrypted_content" {
			found = true
		}
	}
	if !found {
		t.Fatalf("dropped = %#v, want reasoning:encrypted_content", request.Context.Dropped)
	}
}

// TestDecodeRequestCustomToolCall 验证 custom_tool_call item 的 input 原文
// 走 Custom 通道——freeform 参数体不是 JSON。
func TestDecodeRequestCustomToolCall(t *testing.T) {
	data := []byte(`{"model":"m","input":[
		{"type":"custom_tool_call","call_id":"c1","name":"apply_patch","input":"*** Begin Patch\n+x"},
		{"type":"custom_tool_call_output","call_id":"c1","output":"patched"}
	]}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	assistant, _ := request.Context.Messages[0].(llm.AssistantMessage)
	call, ok := assistant.Content[0].(llm.ToolCall)
	if !ok || !call.Custom || string(call.Arguments) != "*** Begin Patch\n+x" {
		t.Fatalf("custom tool call = %#v", assistant.Content[0])
	}
	result, _ := request.Context.Messages[1].(llm.ToolResultMessage)
	if result.ToolCallID != "c1" {
		t.Fatalf("custom_tool_call_output = %#v", result)
	}
}

// TestDecodeRequestToolOutputPartArray 验证 function_call_output 的 output
// part 数组（含 input_image）解码为内容块而不是字面 JSON 文本。
func TestDecodeRequestToolOutputPartArray(t *testing.T) {
	data := []byte(`{"model":"m","input":[
		{"type":"function_call","call_id":"c1","name":"shot","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":[
			{"type":"input_text","text":"see"},
			{"type":"input_image","image_url":"data:image/png;base64,iVBORw0KGgo="}
		]}
	]}`)
	request, err := DecodeRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := request.Context.Messages[1].(llm.ToolResultMessage)
	if len(result.Content) != 2 {
		t.Fatalf("tool result content = %#v", result.Content)
	}
	if _, ok := result.Content[1].(llm.ImageContent); !ok {
		t.Fatalf("content[1] = %T, want ImageContent", result.Content[1])
	}
}

// mustText 断言 content[0] 为 TextContent 并返回正文。
func mustText(t *testing.T, contents []llm.Content) string {
	t.Helper()
	if len(contents) == 0 {
		t.Fatal("empty content")
	}
	text, ok := contents[0].(llm.TextContent)
	if !ok {
		t.Fatalf("content[0] type = %T, want TextContent", contents[0])
	}
	return text.Text
}
