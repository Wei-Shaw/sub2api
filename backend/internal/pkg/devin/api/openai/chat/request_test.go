// 本文件验证 OpenAI Chat Completions 请求能保留系统提示、多模态输入、工具和工具结果。
package chat

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
  "messages": [
    {"role": "system", "content": "你是一个谨慎的助手。"},
    {"role": "user", "content": [
      {"type": "text", "text": "读取这个文件"},
      {"type": "image_url", "image_url": {"url": "data:image/png;base64,iVBORw0KGgo="}}
    ]},
    {"role": "assistant", "content": null, "tool_calls": [{"id": "call-1", "type": "function", "function": {"name": "read_file", "arguments": "{\"path\":\"a.txt\"}"}}]},
    {"role": "tool", "tool_call_id": "call-1", "content": "内容"}
  ],
  "tools": [{"type": "function", "function": {"name": "read_file", "description": "读取文件", "parameters": {"type": "object"}}}]
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
	user := request.Context.Messages[0].(llm.UserMessage)
	if _, ok := user.Content[1].(llm.ImageContent); !ok {
		t.Fatalf("user content[1] = %T, want ImageContent", user.Content[1])
	}
	if err := request.Context.Validate(); err != nil {
		t.Fatalf("context validation error = %v", err)
	}
}

// TestDecodeRequestAcceptsPlainString 验证简短字符串输入会转换为用户文字消息。
func TestDecodeRequestAcceptsPlainString(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(request.Context.Messages))
	}
	message := request.Context.Messages[0].(llm.UserMessage)
	if message.Content[0].(llm.TextContent).Text != "hello" {
		t.Fatalf("message content = %#v", message.Content)
	}
}

// TestDecodeRequestAcceptsFunctionCallArguments 验证工具调用参数按 JSON 对象保留。
func TestDecodeRequestAcceptsFunctionCallArguments(t *testing.T) {
	request, err := DecodeRequest([]byte(`{"model":"gpt-test","messages":[{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	assistant := request.Context.Messages[0].(llm.AssistantMessage)
	call := assistant.Content[0].(llm.ToolCall)
	if call.Name != "read_file" {
		t.Fatalf("tool call name = %q", call.Name)
	}
	if !json.Valid(call.Arguments) || !strings.Contains(string(call.Arguments), `"path"`) {
		t.Fatalf("tool call arguments = %q", call.Arguments)
	}
}
