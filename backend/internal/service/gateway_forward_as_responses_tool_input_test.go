package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Anthropic 的 content_block_start 会带空对象占位（"input": {}），真实参数随后由
// input_json_delta 分片流出。buffered 路径把整个 ContentBlock 追加进
// finalResp.Content，Input 因此已是 "{}"；若把占位当普通分片往后拼，就会产出
// `{}{"command":"ls"}` 这种非法 JSON 并原样出到客户端（Input 是 json.RawMessage，
// 不做校验）。
func TestAppendRawJSON_DropsContentBlockStartPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		existing json.RawMessage
		fragment string
		want     string
	}{
		{
			name:     "占位空对象被第一个真实分片替换",
			existing: json.RawMessage(`{}`),
			fragment: `{"command":"ls -la /tmp"}`,
			want:     `{"command":"ls -la /tmp"}`,
		},
		{
			name:     "带空白的占位同样被替换",
			existing: json.RawMessage(` { } `),
			fragment: `{"command":"ls"}`,
			want:     `{"command":"ls"}`,
		},
		{
			name:     "空值走原有替换分支",
			existing: nil,
			fragment: `{"command":"ls"}`,
			want:     `{"command":"ls"}`,
		},
		{
			name:     "不闭合前缀必须继续拼接，不能被误判为占位",
			existing: json.RawMessage(`{`),
			fragment: `"command":"ls"}`,
			want:     `{"command":"ls"}`,
		},
		{
			name:     "上游把空对象拆成两片发时不受干扰",
			existing: json.RawMessage(`{`),
			fragment: `}`,
			want:     `{}`,
		},
		{
			name:     "已有真实内容时正常追加",
			existing: json.RawMessage(`{"command":"ls`),
			fragment: ` -la"}`,
			want:     `{"command":"ls -la"}`,
		},
		{
			name:     "占位后收到完整空对象分片，结果仍是合法空对象",
			existing: json.RawMessage(`{}`),
			fragment: `{}`,
			want:     `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendRawJSON(tt.existing, tt.fragment)
			require.Equal(t, tt.want, string(got))
			require.True(t, json.Valid(got), "累积结果必须是合法 JSON：%s", got)
		})
	}
}

// 无参工具调用：上游只发占位、不发任何 delta。此时必须保留 "{}"，不能退化成空串，
// 否则 Input 的 omitempty 会让 input 字段整个消失（对齐 kiro 侧
// normalizeStreamingToolInput 的空输入语义）。
func TestAppendRawJSON_KeepsPlaceholderWhenNoDeltaArrives(t *testing.T) {
	existing := json.RawMessage(`{}`)
	require.True(t, isEmptyJSONObject(existing))
	require.True(t, json.Valid(existing))

	encoded, err := json.Marshal(map[string]any{"input": existing})
	require.NoError(t, err)
	require.JSONEq(t, `{"input":{}}`, string(encoded), "占位保留下来才能序列化出 input 字段")
}
