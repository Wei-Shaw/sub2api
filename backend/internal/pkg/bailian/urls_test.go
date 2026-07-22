//go:build unit

package bailian

import "testing"

func TestNormalizeAPIRoot(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare host", "https://dashscope.aliyuncs.com", "https://dashscope.aliyuncs.com"},
		{"trailing slash", "https://dashscope.aliyuncs.com/", "https://dashscope.aliyuncs.com"},
		{"compatible-mode v1 suffix", "https://dashscope.aliyuncs.com/compatible-mode/v1", "https://dashscope.aliyuncs.com"},
		{"compatible-mode suffix", "https://dashscope.aliyuncs.com/compatible-mode/", "https://dashscope.aliyuncs.com"},
		{"api v1 suffix", "https://dashscope.aliyuncs.com/api/v1", "https://dashscope.aliyuncs.com"},
		{"workspace domain", "https://ws-123.cn-beijing.maas.aliyuncs.com", "https://ws-123.cn-beijing.maas.aliyuncs.com"},
		{"empty falls back to default", "  ", DefaultBaseURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeAPIRoot(tt.in); got != tt.want {
				t.Fatalf("NormalizeAPIRoot(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEndpointURLs(t *testing.T) {
	root := "https://dashscope.aliyuncs.com"
	if got := VideoSynthesisURL(root); got != root+"/api/v1/services/aigc/video-generation/video-synthesis" {
		t.Fatalf("VideoSynthesisURL = %q", got)
	}
	if got := TaskURL(root, "task/../123"); got != root+"/api/v1/tasks/task%2F..%2F123" {
		t.Fatalf("TaskURL should path-escape the task id, got %q", got)
	}
	if got := ChatCompletionsURL(root + "/compatible-mode/v1"); got != root+"/compatible-mode/v1/chat/completions" {
		t.Fatalf("ChatCompletionsURL = %q", got)
	}
}
