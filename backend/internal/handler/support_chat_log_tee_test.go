package handler

import "testing"

// TestExtractSSEDeltaContent 覆盖 tee 累积的核心解析函数（add-support-chat-transcript-log）：
// 从一行 SSE data 帧里抽 choices[0].delta.content，非预期输入 silent skip 返回空串。
func TestExtractSSEDeltaContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "normal delta",
			in:   `data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			want: "Hello",
		},
		{
			name: "empty delta content",
			in:   `data: {"choices":[{"delta":{}}]}`,
			want: "",
		},
		{
			name: "no choices",
			in:   `data: {"choices":[]}`,
			want: "",
		},
		{
			name: "done sentinel is not content",
			in:   `data: [DONE]`,
			want: "",
		},
		{
			name: "malformed json skipped",
			in:   `data: {not json`,
			want: "",
		},
		{
			name: "non-data line",
			in:   `event: error`,
			want: "",
		},
		{
			name: "empty payload",
			in:   `data: `,
			want: "",
		},
		{
			name: "multi-choice takes first",
			in:   `data: {"choices":[{"delta":{"content":"A"}},{"delta":{"content":"B"}}]}`,
			want: "A",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractSSEDeltaContent([]byte(tc.in))
			if got != tc.want {
				t.Fatalf("extractSSEDeltaContent(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
