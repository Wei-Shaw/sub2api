package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaudeCodeFingerprint_JavaScriptUTF16(t *testing.T) {
	const version = "2.1.283"
	// Fixed outputs from Node.js crypto, independent of the Go implementation:
	// createHash('sha256').update('59cf53e54c78' +
	//   [4, 7, 20].map(i => text[i] || '0').join('') + '2.1.283')
	//   .digest('hex').slice(0, 3)
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty", text: "", want: "49b"},
		{name: "short_ascii", text: "hi", want: "49b"},
		{name: "ascii", text: "abcdefghijklmnopqrstuvwxyz", want: "2e9"},
		{name: "chinese", text: "这是一个用于验证中文指纹计算的测试消息，请保持一致。", want: "c7e"},
		{name: "short_chinese", text: "你好世界呀", want: "4ed"},
		{name: "mixed", text: "abcd中fg文ijklmnopqrst界", want: "183"},
		{name: "emoji_before_indices", text: "😀abcdefghijklmnopqrstuvwxyz", want: "c16"},
		{name: "isolated_high_surrogate", text: "abcd😀fghijklmnopqrstuvwxyz", want: "d96"},
		{name: "isolated_low_surrogate", text: "abc😀efghijklmnopqrstuvwxyz", want: "d96"},
		{name: "sampled_surrogates_form_pair", text: "abcd😀😀efghijklmnopqrstuvwxyz", want: "0e9"},
		{name: "nul_is_not_missing", text: "abcd\x00fg\x00hijklmnopqrs\x00", want: "03e"},
	}
	for _, tt := range tests {
		for _, format := range []string{"string", "text_block"} {
			t.Run(tt.name+"/"+format, func(t *testing.T) {
				var content any = tt.text
				if format == "text_block" {
					content = []map[string]any{{"type": "text", "text": tt.text}}
				}
				body, err := json.Marshal(map[string]any{
					"messages": []map[string]any{{"role": "user", "content": content}},
				})
				require.NoError(t, err)
				require.Equal(t, tt.want, computeClaudeCodeFingerprint(body, version))

				billing, err := buildBillingAttributionText(body, version)
				require.NoError(t, err)
				require.Equal(t, "x-anthropic-billing-header: cc_version="+version+"."+tt.want+"; cc_entrypoint=cli;", billing)
			})
		}
	}
}
