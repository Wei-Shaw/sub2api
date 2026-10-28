package service

import "testing"

// 算法必须与真机 CLI 一致：sha256("59cf53e54c78" + 首条 user 文本的码元 4/7/20 + 版本)[:3]。
// 期望值由真机抓包反推：prompt "say OK" → 9cf；prompt "0123…89" → d81。
func TestComputeClaudeCodeFingerprintVectors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"short prompt pads with zeros", `{"messages":[{"role":"user","content":"say OK"}]}`, "9cf"},
		{"digits at sample offsets", `{"messages":[{"role":"user","content":"0123456789012345678901234567890123456789"}]}`, "d81"},
		{"meta message skipped", `{"messages":[{"role":"user","isMeta":true,"content":"zzzzzzzzzzzzzzzzzzzzzzz"},{"role":"user","content":"say OK"}]}`, "9cf"},
		{"no user message", `{"messages":[]}`, "972"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := computeClaudeCodeFingerprint([]byte(c.body), "2.1.311"); got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}
