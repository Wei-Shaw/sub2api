//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAddMessageCacheBreakpoints_JSONStringEscaping(t *testing.T) {
	var controls strings.Builder
	for r := rune(0); r <= 0x9f; r++ {
		controls.WriteRune(r)
	}
	tests := []struct{ name, text string }{
		{"delete_character", "before\x7fafter"},
		{"bell_and_vertical_tab", "before\a\vafter"},
		{"nul_and_escape", "before\x00\x1bafter"},
		{"all_ascii_and_c1_controls", controls.String()},
		{"supplementary_nonprinting_rune", "before\U000e0067after"},
		{"unicode_and_json_metacharacters", "中文 😀 <>& \"quote\" \\slash\n\r\t\b\f"},
		{"literal_backslash_sequences", `literal \x7f \a \v \U000e0067`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Both the final message and the second-to-last user turn are promoted
			// from strings to text blocks by the cache rewrite.
			body, err := json.Marshal(map[string]any{
				"messages": []map[string]any{
					{"role": "user", "content": tt.text},
					{"role": "assistant", "content": "ack"},
					{"role": "user", "content": "next"},
					{"role": "assistant", "content": tt.text},
				},
			})
			require.NoError(t, err)
			out := addMessageCacheBreakpoints(body)
			// gjson accepts some malformed input, so validate with encoding/json
			// before inspecting the rewritten content and cache metadata.
			var decoded any
			require.NoError(t, json.Unmarshal(out, &decoded))
			for _, path := range []string{"messages.0.content", "messages.3.content"} {
				content := gjson.GetBytes(out, path)
				require.True(t, content.IsArray())
				require.Len(t, content.Array(), 1)
				block := content.Array()[0]
				require.Equal(t, "text", block.Get("type").String())
				require.Equal(t, tt.text, block.Get("text").String())
				require.Equal(t, "ephemeral", block.Get("cache_control.type").String())
				require.Equal(t, "5m", block.Get("cache_control.ttl").String())
			}
			require.JSONEq(t, string(out), string(addMessageCacheBreakpoints(out)))
		})
	}
}
