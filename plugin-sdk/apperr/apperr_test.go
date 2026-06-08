package apperr

import (
	"errors"
	"strings"
	"testing"
)

// TestSanitizeCauseForLog_RedactsSQLDetail 模拟 lib/pq 风格的错误文本，
// 验证 DETAIL/HINT/QUERY 等字段后的内容被截断。
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

// ---------------------------------------------------------------------------
// ClassifyDBError / IsPQCode tests
// ---------------------------------------------------------------------------

// TestClassifyDBError_UniqueViolation verifies that a gRPC error carrying the
// structured pq[23505,...] format is classified as a 409 Conflict.
func TestClassifyDBError_UniqueViolation(t *testing.T) {
	err := errors.New(`plugin tx exec: pq[23505,constraint=channels_name_key,table=channels]: duplicate key value violates unique constraint "channels_name_key"`)
	appErr := ClassifyDBError(err)
	if appErr == nil {
		t.Fatal("expected non-nil ApplicationError for unique_violation")
	}
	if appErr.Code != 409 {
		t.Errorf("expected 409, got %d", appErr.Code)
	}
	if appErr.Reason != "UNIQUE_VIOLATION" {
		t.Errorf("expected reason UNIQUE_VIOLATION, got %q", appErr.Reason)
	}
	if !strings.Contains(appErr.Message, "channels_name_key") {
		t.Errorf("expected constraint name in message, got %q", appErr.Message)
	}
}

// TestClassifyDBError_NotNullViolation verifies 23502 → 400.
func TestClassifyDBError_NotNullViolation(t *testing.T) {
	err := errors.New(`plugin tx exec: pq[23502,column=name,table=channels]: null value in column "name" violates not-null constraint`)
	appErr := ClassifyDBError(err)
	if appErr == nil {
		t.Fatal("expected non-nil ApplicationError for not_null_violation")
	}
	if appErr.Code != 400 {
		t.Errorf("expected 400, got %d", appErr.Code)
	}
	if !strings.Contains(appErr.Message, "name") {
		t.Errorf("expected column name in message, got %q", appErr.Message)
	}
}

// TestClassifyDBError_CheckViolation verifies 23514 → 400.
func TestClassifyDBError_CheckViolation(t *testing.T) {
	err := errors.New(`plugin sql exec: pq[23514,constraint=channels_status_check]: new row violates check constraint`)
	appErr := ClassifyDBError(err)
	if appErr == nil {
		t.Fatal("expected non-nil ApplicationError for check_violation")
	}
	if appErr.Code != 400 {
		t.Errorf("expected 400, got %d", appErr.Code)
	}
	if appErr.Reason != "CHECK_VIOLATION" {
		t.Errorf("expected reason CHECK_VIOLATION, got %q", appErr.Reason)
	}
}

// TestClassifyDBError_StringTruncation verifies 22001 → 400.
func TestClassifyDBError_StringTruncation(t *testing.T) {
	err := errors.New(`plugin tx exec: pq[22001]: value too long for type character varying(255)`)
	appErr := ClassifyDBError(err)
	if appErr == nil {
		t.Fatal("expected non-nil ApplicationError for string_data_right_truncation")
	}
	if appErr.Code != 400 {
		t.Errorf("expected 400, got %d", appErr.Code)
	}
	if appErr.Reason != "VALUE_TOO_LONG" {
		t.Errorf("expected reason VALUE_TOO_LONG, got %q", appErr.Reason)
	}
}

// TestClassifyDBError_UndefinedColumn verifies 42703 → 500.
func TestClassifyDBError_UndefinedColumn(t *testing.T) {
	err := errors.New(`plugin tx exec: pq[42703,column=deleted_at]: column "deleted_at" does not exist`)
	appErr := ClassifyDBError(err)
	if appErr == nil {
		t.Fatal("expected non-nil ApplicationError for undefined_column")
	}
	if appErr.Code != 500 {
		t.Errorf("expected 500, got %d", appErr.Code)
	}
	if appErr.Reason != "SCHEMA_MISMATCH" {
		t.Errorf("expected reason SCHEMA_MISMATCH, got %q", appErr.Reason)
	}
}

// TestClassifyDBError_UnknownPQCode returns nil for unrecognised codes.
func TestClassifyDBError_UnknownPQCode(t *testing.T) {
	err := errors.New(`plugin tx exec: pq[99999]: some weird error`)
	if appErr := ClassifyDBError(err); appErr != nil {
		t.Errorf("expected nil for unknown pq code, got %v", appErr)
	}
}

// TestClassifyDBError_NonPQError returns nil for non-pq errors.
func TestClassifyDBError_NonPQError(t *testing.T) {
	err := errors.New("connection refused")
	if appErr := ClassifyDBError(err); appErr != nil {
		t.Errorf("expected nil for non-pq error, got %v", appErr)
	}
}

// TestClassifyDBError_Nil returns nil for nil.
func TestClassifyDBError_Nil(t *testing.T) {
	if appErr := ClassifyDBError(nil); appErr != nil {
		t.Errorf("expected nil for nil error, got %v", appErr)
	}
}

// TestIsPQCode verifies the helper for repo-layer use.
func TestIsPQCode(t *testing.T) {
	err := errors.New(`plugin tx exec: pq[23505,constraint=channels_name_key]: duplicate key`)
	if !IsPQCode(err, "23505") {
		t.Error("expected IsPQCode to return true for 23505")
	}
	if IsPQCode(err, "23502") {
		t.Error("expected IsPQCode to return false for non-matching code")
	}
	if IsPQCode(errors.New("not a pq error"), "23505") {
		t.Error("expected IsPQCode to return false for non-pq error")
	}
}
