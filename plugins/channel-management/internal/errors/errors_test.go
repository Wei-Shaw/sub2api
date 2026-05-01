package errors

import (
	"errors"
	"strings"
	"testing"
)

// TestSanitizeCauseForLog_RedactsSQLDetail 是 T14 Fix 4 的回归测试。
// 模拟 lib/pq 风格的错误文本，验证 DETAIL/HINT/QUERY 等字段后的内容被截断。
func TestSanitizeCauseForLog_RedactsSQLDetail(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		mustHave   string // 必须保留的前缀
		mustNotHas string // 必须被脱敏掉的敏感子串
	}{
		{
			name:       "lib/pq DETAIL with user input",
			input:      `pq: duplicate key value violates unique constraint "users_email_key" DETAIL: Key (email)=(victim@example.com) already exists.`,
			mustHave:   `pq: duplicate key`,
			mustNotHas: `victim@example.com`,
		},
		{
			name:       "QUERY with bind values",
			input:      `pq: invalid input syntax for type integer QUERY: SELECT * FROM users WHERE id = 'attacker_input'`,
			mustHave:   `pq: invalid input`,
			mustNotHas: `attacker_input`,
		},
		{
			name:       "HINT with column hint",
			input:      `pq: column "secret_token" does not exist HINT: Perhaps you meant "tokens.secret_token_value".`,
			mustHave:   `pq: column "secret_token"`,
			mustNotHas: `secret_token_value`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeCauseForLog(errors.New(tc.input))
			if !strings.Contains(got, tc.mustHave) {
				t.Errorf("expected output to contain %q, got %q", tc.mustHave, got)
			}
			if strings.Contains(got, tc.mustNotHas) {
				t.Errorf("expected output to NOT contain redacted segment %q, got %q", tc.mustNotHas, got)
			}
			if !strings.Contains(got, "redacted") {
				t.Errorf("expected redaction marker in output, got %q", got)
			}
		})
	}
}

// TestSanitizeCauseForLog_TruncatesLongErrors 验证超长错误被截断到上限。
func TestSanitizeCauseForLog_TruncatesLongErrors(t *testing.T) {
	long := strings.Repeat("a", causeLogMaxLen+200)
	got := SanitizeCauseForLog(errors.New(long))
	if len(got) > causeLogMaxLen+5 { // +5 是 "..." 后缀的余量
		t.Errorf("expected truncation to ~%d bytes, got %d", causeLogMaxLen, len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected truncated output to end with '...', got %q", got[max(0, len(got)-10):])
	}
}

// TestSanitizeCauseForLog_NilSafe 验证 nil 输入安全。
func TestSanitizeCauseForLog_NilSafe(t *testing.T) {
	if got := SanitizeCauseForLog(nil); got != "" {
		t.Errorf("expected empty string for nil cause, got %q", got)
	}
}
