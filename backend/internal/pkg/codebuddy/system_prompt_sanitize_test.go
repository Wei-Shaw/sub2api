package codebuddy

import (
	"encoding/json"
	"strings"
	"testing"
)

type testMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type testBody struct {
	Messages []testMsg `json:"messages"`
}

func systemContents(t *testing.T, raw []byte) []string {
	t.Helper()
	var b testBody
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var out []string
	for _, m := range b.Messages {
		if m.Role != "system" {
			continue
		}
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			out = append(out, s)
		}
	}
	return out
}

func errorPromptBody(t *testing.T, line string) []byte {
	t.Helper()
	content, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("marshal line: %v", err)
	}
	body, err := json.Marshal(testBody{Messages: []testMsg{{Role: "system", Content: content}}})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return body
}

// sanitizeCase 是一条 "错误 prompt → 预期改写后剩余内容" 的映射。
// 取自 backend/error-prompt.txt（每行一个 system text）/ failN.json，
// 对照 succN.json 中对应 system prompt 人工核对的预期输出。
// 策略：仅删除/改写会触发上游 content_filter 的具体触发句，其余内容原样保留。
// 后续遇到新的触发样本，直接往这里加一条即可。
//
// removed 为必须被删掉的触发句（断言输出中不再出现）；
// kept 为应被原样保留的内容片段（断言输出中仍然存在）。
var sanitizeCases = []struct {
	name    string
	input   string
	removed []string
	kept    []string
}{
	{
		name:  "claude_identity",
		input: "You are Claude Code, Anthropic's official CLI for Claude.\n\n\nYou are an interactive agent that helps users with software engineering tasks.\n\nIMPORTANT: Assist with authorized security testing.",
		removed: []string{
			"You are Claude Code, Anthropic's official CLI for Claude.",
		},
		kept: []string{
			"You are an interactive agent that helps users with software engineering tasks.",
			"IMPORTANT: Assist with authorized security testing.",
		},
	},
	{
		name:  "git_branch_prs",
		input: "and will not update during the conversation.\n\nCurrent branch: feat/codebuddy_workbuddy\n\nMain branch (you will usually use this for PRs): main\n\nGit",
		removed: []string{
			"PRs",
		},
		kept: []string{
			"Main branch (you will usually use this for PR): main",
			"Current branch: feat/codebuddy_workbuddy",
		},
	},
	{
		name:  "codex_cli_attribution",
		input: "You are a coding agent running in the Codex CLI, a terminal-based coding assistant. Codex CLI is an open source project led by OpenAI. You are expected to be precise, safe, and helpful.",
		removed: []string{
			"Codex CLI is an open source project led by OpenAI.",
			"Codex CLI",
		},
		kept: []string{
			"You are a coding agent running in the workbuddy, a terminal-based coding assistant.",
			"You are expected to be precise, safe, and helpful.",
		},
	},
}

func TestSanitizeErrorPromptCases(t *testing.T) {
	for _, tc := range sanitizeCases {
		t.Run(tc.name, func(t *testing.T) {
			body := errorPromptBody(t, tc.input)
			got, changed, err := SanitizeForContentFilter(body, nil)
			if err != nil {
				t.Fatalf("sanitize: %v", err)
			}
			if !changed {
				t.Fatalf("expected change for case %q", tc.name)
			}
			gotSys := systemContents(t, got)
			if len(gotSys) != 1 {
				t.Fatalf("system count = %d, want 1", len(gotSys))
			}
			out := gotSys[0]
			for _, r := range tc.removed {
				if strings.Contains(out, r) {
					t.Errorf("case %q: trigger phrase still present after sanitize: %q", tc.name, r)
				}
			}
			for _, k := range tc.kept {
				if !strings.Contains(out, k) {
					t.Errorf("case %q: expected content lost during sanitize: %q\n--- got ---\n%s", tc.name, k, out)
				}
			}
		})
	}
}

func TestSanitizeNoMatchUnchanged(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":"You are a helpful assistant with no brand names."},{"role":"user","content":"hi"}]}`)
	out, changed, err := SanitizeForContentFilter(body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatalf("expected no change, got: %s", out)
	}
}
