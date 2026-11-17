//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONMayEscapePlainASCII(t *testing.T) {
	// U(hex) spells the six-byte JSON escape backslash, 'u', hex.
	U := func(hex string) string { return `\` + "u" + hex }
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"no escapes", `{"input":[{"type":"message","content":"hi"}]}`, false},
		{"ansi control escape from shell output", `{"output":"` + U("001b") + `[31mred` + U("001b") + `[0m"}`, false},
		{"go html-safe escapes", `{"text":"a ` + U("003c") + ` b ` + U("0026") + ` c ` + U("003e") + ` d"}`, false},
		{"quote apostrophe and backslash escapes", `{"text":"` + U("0022") + U("0027") + U("005c") + `"}`, false},
		{"non-ascii and surrogate pair", `{"text":"caf` + U("00e9") + ` ` + U("d83d") + U("de00") + ` ` + U("4e2d") + `"}`, false},
		{"escaped letter hides a keyword", `{"type":"` + U("0063") + `ompaction_trigger"}`, true},
		{"upper-case hex escaped underscore", `{"type":"input` + U("005F") + `image"}`, true},
		{"escaped digit", `{"n":"` + U("0031") + `"}`, true},
		{"escaped backslash before u is literal text", `{"code":"\` + U("0041") + `"}`, false},
		{"escaped backslash in a path", `{"cwd":"C:\\users\\dev","re":"\\u{1F600}"}`, false},
		{"escape after an escaped backslash", `{"t":"\\` + U("0063") + `"}`, true},
		{"truncated escape", `{"t":"` + U("00"), true},
		{"invalid hex", `{"t":"` + U("ZZZZ") + `"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, JSONMayEscapePlainASCII([]byte(tc.body)))
		})
	}
}
