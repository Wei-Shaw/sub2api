package service

import (
	"reflect"
	"testing"
)

func TestModelReasoningEfforts(t *testing.T) {
	cases := []struct {
		name  string
		model string
		want  []string
	}{
		{"deepseek 原生四档", "deepseek-v4-flash", []string{"low", "medium", "high", "max"}},
		{"deepseek 大写", "DeepSeek-V4-Pro", []string{"low", "medium", "high", "max"}},
		{"glm 只报 z.ai 原生两档", "glm-5.2", []string{"high", "max"}},
		{"gpt 常规", "gpt-5.4", []string{"low", "medium", "high", "xhigh"}},
		{"gpt 5.6 加 max", "gpt-5.6-luna", []string{"low", "medium", "high", "xhigh", "max"}},
		{"codex", "codex-auto-review", []string{"low", "medium", "high", "xhigh"}},
		{"o 系列", "o3", []string{"low", "medium", "high", "xhigh"}},
		{"o 前缀但非型号", "o-foo", nil},
		{"grok 名单", "grok-4.5", []string{"low", "medium", "high"}},
		{"长尾模型不支持", "longcat-2.0-free", nil},
		{"空串", "", nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := ModelReasoningEfforts(tt.model)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ModelReasoningEfforts(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestModelSupportsAttachments(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4-flash", false},
		{"glm-5.2", false},
		{"longcat-2.0-free", false},
		{"gpt-5.6", true},
		{"claude-sonnet-4-6", true},
		{"gemini-2.5-pro", true},
		{"grok-4.5", true},
		{"o3", true},
	}
	for _, tt := range cases {
		if got := ModelSupportsAttachments(tt.model); got != tt.want {
			t.Errorf("ModelSupportsAttachments(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestCapModelReasoningEfforts(t *testing.T) {
	efforts := []string{"low", "medium", "high", "max"}
	cases := []struct {
		name      string
		maxEffort string
		want      []string
	}{
		{"空上限不截断", "", efforts},
		{"max 上限不截断", "max", efforts},
		{"high 上限截掉 max", "high", []string{"low", "medium", "high"}},
		{"未知档位不截断", "banana", efforts},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := CapModelReasoningEfforts(append([]string(nil), efforts...), tt.maxEffort)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CapModelReasoningEfforts(%v, %q) = %v, want %v", efforts, tt.maxEffort, got, tt.want)
			}
		})
	}
}
