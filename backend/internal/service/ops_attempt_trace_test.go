package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func traceStrPtr(s string) *string { return &s }

func TestBuildUpstreamAttemptRecordsFailoverChain(t *testing.T) {
	entry := &OpsInsertErrorLogInput{
		RequestID:      "req-1",
		Platform:       PlatformAnthropic,
		Model:          "claude-sonnet-4-6",
		RequestedModel: "claude-sonnet-4-6",
		UpstreamModel:  "claude-sonnet-4-6-upstream",
		UpstreamErrors: []*OpsUpstreamErrorEvent{
			{
				Platform:             PlatformAnthropic,
				AccountID:            7,
				UpstreamStatusCode:   401,
				UpstreamURL:          "https://api.anthropic.com/v1/messages",
				UpstreamResponseBody: `{"type":"error","error":{"type":"authentication_error"}}`,
				Kind:                 "http_error",
				Message:              "invalid x-api-key",
			},
			{
				Platform:           PlatformAnthropic,
				AccountID:          9,
				UpstreamStatusCode: 200,
				Kind:               "failover",
				Message:            "switched account",
			},
		},
	}

	records := buildUpstreamAttemptRecords(
		[]*OpsInsertErrorLogInput{entry},
		time.Unix(1000, 0),
		func(id int64) *string { return traceStrPtr("fp" + string(rune('0'+id))) },
	)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}

	first, second := records[0], records[1]
	if first.AttemptNo != 1 || second.AttemptNo != 2 {
		t.Fatalf("attempt numbers = %d,%d", first.AttemptNo, second.AttemptNo)
	}
	if first.RetryFrom != nil {
		t.Fatalf("first attempt must not have retry_from, got %v", *first.RetryFrom)
	}
	if second.RetryFrom == nil || *second.RetryFrom != 7 {
		t.Fatalf("second attempt retry_from = %v, want 7", second.RetryFrom)
	}
	if first.AccountID == nil || *first.AccountID != 7 {
		t.Fatalf("first account = %v", first.AccountID)
	}
	// 401 on Claude → auth; this drives the same enum the live scheduler uses.
	if first.FailureKind != "auth" {
		t.Fatalf("first failure_kind = %q, want auth", first.FailureKind)
	}
	if second.FailureKind != "none" {
		t.Fatalf("second failure_kind = %q, want none", second.FailureKind)
	}
	if first.ErrorCode == nil || *first.ErrorCode != "authentication_error" {
		t.Fatalf("error code = %v", first.ErrorCode)
	}
	if first.CredentialFP == nil || *first.CredentialFP != "fp7" {
		t.Fatalf("credential fp = %v", first.CredentialFP)
	}
	if !first.CreatedAt.Equal(time.Unix(1000, 0)) {
		t.Fatalf("created_at = %v", first.CreatedAt)
	}
}

func TestBuildUpstreamAttemptRecordsSkipsIncomplete(t *testing.T) {
	cases := []struct {
		name  string
		entry *OpsInsertErrorLogInput
	}{
		{"nil entry", nil},
		{"no request id", &OpsInsertErrorLogInput{UpstreamErrors: []*OpsUpstreamErrorEvent{{AccountID: 1}}}},
		{"no events", &OpsInsertErrorLogInput{RequestID: "r"}},
		{"nil events only", &OpsInsertErrorLogInput{RequestID: "r", UpstreamErrors: []*OpsUpstreamErrorEvent{nil}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildUpstreamAttemptRecords([]*OpsInsertErrorLogInput{tc.entry}, time.Now(), nil); len(got) != 0 {
				t.Fatalf("expected no records, got %d", len(got))
			}
		})
	}
}

func TestClassifyTraceFailureUsesPlatformSemantics(t *testing.T) {
	cases := []struct {
		name     string
		platform string
		status   int
		body     string
		want     string
	}{
		{"openai quota exhaustion on 429", PlatformOpenAI, 429, `{"error":{"code":"insufficient_quota"}}`, "auth"},
		{"openai plain rate limit", PlatformOpenAI, 429, `{"error":{"code":"rate_limit_exceeded"}}`, "rate_limit"},
		{"anthropic credit exhausted on 400", PlatformAnthropic, 400, `{"error":{"message":"Your credit balance is too low"}}`, "auth"},
		{"anthropic overloaded", PlatformAnthropic, 529, `{}`, "overload"},
		{"antigravity capacity", PlatformAntigravity, 429, `{"error":{"details":[{"reason":"MODEL_CAPACITY_EXHAUSTED"}]}}`, "overload"},
		{"unknown platform falls back to generic", "gemini", 401, `{}`, "auth"},
		{"no status means transient", PlatformAnthropic, 0, ``, "transient"},
		{"success", PlatformAnthropic, 200, `{}`, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyTraceFailure(tc.platform, tc.status, []byte(tc.body)).String()
			if got != tc.want {
				t.Fatalf("kind = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractTraceErrorCode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"error":{"code":"insufficient_quota"}}`, "insufficient_quota"},
		{`{"error":{"type":"rate_limit_error"}}`, "rate_limit_error"},
		{`{"error":{"status":"RESOURCE_EXHAUSTED"}}`, "RESOURCE_EXHAUSTED"},
		{`plain text without code`, ""},
		{``, ""},
	}
	for _, tc := range cases {
		got := extractTraceErrorCode(tc.in)
		if tc.want == "" {
			if got != nil {
				t.Fatalf("%q: expected nil, got %q", tc.in, *got)
			}
			continue
		}
		if got == nil || *got != tc.want {
			t.Fatalf("%q: got %v, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTruncateTraceMsg(t *testing.T) {
	long := strings.Repeat("中", 600)
	got := truncateTraceMsg(long)
	if n := len([]rune(got)); n != 512 {
		t.Fatalf("rune length = %d, want 512", n)
	}
	if strings.Contains(got, "\n") {
		t.Fatal("newlines must be flattened")
	}
	if got := truncateTraceMsg("line1\nline2\r\nline3"); got != "line1 line2  line3" {
		t.Fatalf("newline flattening = %q", got)
	}
}

func TestCredentialFingerprint(t *testing.T) {
	withToken := &Account{Credentials: map[string]any{"access_token": "tok-1", "api_key": "key-1"}}
	fp := credentialFingerprint(withToken)
	if fp == nil || len(*fp) != 16 {
		t.Fatalf("fingerprint = %v", fp)
	}
	if again := credentialFingerprint(withToken); again == nil || *again != *fp {
		t.Fatal("fingerprint must be deterministic")
	}
	// access_token wins over api_key, so rotating either changes the fp.
	rotated := &Account{Credentials: map[string]any{"access_token": "tok-2", "api_key": "key-1"}}
	if fp2 := credentialFingerprint(rotated); fp2 == nil || *fp2 == *fp {
		t.Fatal("rotating the access token must change the fingerprint")
	}
	if credentialFingerprint(&Account{}) != nil {
		t.Fatal("account without credentials must yield nil")
	}
	if credentialFingerprint(nil) != nil {
		t.Fatal("nil account must yield nil")
	}
}

func TestAccountStatusEventFromBlocking(t *testing.T) {
	account := &Account{ID: 42, Status: StatusActive}

	// 零值 until → 摘号
	ev := accountStatusEventFromBlocking(context.Background(), account, time.Time{}, "auth_error", "classify")
	if ev == nil || ev.ToState != "invalid" {
		t.Fatalf("zero until must map to invalid, got %+v", ev)
	}
	if ev.CoolingUntil != nil {
		t.Fatal("invalid event must not carry cooling_until")
	}
	if ev.FromState == nil || *ev.FromState != StatusActive {
		t.Fatalf("from_state = %v", ev.FromState)
	}

	// 非零 until → 冷却
	until := time.Unix(2000, 0)
	ev = accountStatusEventFromBlocking(context.Background(), account, until, "429", "classify")
	if ev.ToState != "cooling" {
		t.Fatalf("to_state = %q", ev.ToState)
	}
	if ev.CoolingUntil == nil || !ev.CoolingUntil.Equal(until) {
		t.Fatalf("cooling_until = %v", ev.CoolingUntil)
	}
	if ev.Reason == nil || *ev.Reason != "429" {
		t.Fatalf("reason = %v", ev.Reason)
	}

	if accountStatusEventFromBlocking(context.Background(), nil, until, "429", "classify") != nil {
		t.Fatal("nil account must yield nil event")
	}
	if ev := accountStatusEventFromBlocking(context.Background(), &Account{}, until, "429", "classify"); ev != nil {
		t.Fatal("account without id must yield nil event")
	}
}

func TestEmitAccountStatusEventWithoutSinkIsNoop(t *testing.T) {
	SetAccountStatusEventSink(nil)
	// 不应 panic，也不应阻塞
	EmitAccountStatusEvent(accountStatusEventFromBlocking(context.Background(), &Account{ID: 1}, time.Time{}, "x", "classify"))
	EmitAccountStatusEvent(nil)
}

func TestRecordUpstreamAttemptsWritesThroughRepo(t *testing.T) {
	repo := &opsRepoMock{}
	svc := &OpsService{opsRepo: repo}
	svc.recordUpstreamAttempts(context.Background(), []*OpsInsertErrorLogInput{
		{
			RequestID: "req-wire",
			Platform:  PlatformAnthropic,
			UpstreamErrors: []*OpsUpstreamErrorEvent{
				{AccountID: 3, UpstreamStatusCode: 429, Kind: "http_error", Message: "rate limited"},
			},
		},
		{RequestID: "", UpstreamErrors: []*OpsUpstreamErrorEvent{{AccountID: 4}}}, // 应被跳过
	})
	if len(repo.RecordedAttempts) != 1 {
		t.Fatalf("recorded = %d, want 1", len(repo.RecordedAttempts))
	}
	rec := repo.RecordedAttempts[0]
	if rec.RequestID != "req-wire" || rec.FailureKind != "rate_limit" || rec.AccountID == nil || *rec.AccountID != 3 {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func TestRecordUpstreamAttemptsSurvivesRepoFailure(t *testing.T) {
	repo := &opsRepoMock{
		InsertUpstreamAttemptsFn: func(ctx context.Context, records []*OpsUpstreamAttemptRecord) (int, error) {
			return 0, errors.New("boom")
		},
	}
	svc := &OpsService{opsRepo: repo}
	// 不应 panic：写失败只记日志，不能打断错误日志链路。
	svc.recordUpstreamAttempts(context.Background(), []*OpsInsertErrorLogInput{
		{RequestID: "r2", UpstreamErrors: []*OpsUpstreamErrorEvent{{AccountID: 1, UpstreamStatusCode: 500}}},
	})
}

func TestAccountStatusEventCarriesRequestID(t *testing.T) {
	account := &Account{ID: 55, Status: StatusActive}
	until := time.Unix(3000, 0)

	// 有请求上下文 → 事件带上 request_id，可回指到触发它的那次请求
	ctx := context.WithValue(context.Background(), ctxkey.RequestID, "  req-abc  ")
	ev := accountStatusEventFromBlocking(ctx, account, until, "429", "classify")
	if ev == nil || ev.RequestID == nil || *ev.RequestID != "req-abc" {
		t.Fatalf("request_id = %v, want trimmed req-abc", ev)
	}

	// 后台任务（无 ctx）→ 不写 request_id，但事件照常产生
	ev = accountStatusEventFromBlocking(context.Background(), account, time.Time{}, "auth_error", "refresh")
	if ev == nil {
		t.Fatal("background event must still be produced")
	}
	if ev.RequestID != nil {
		t.Fatalf("background event must not carry request_id, got %v", *ev.RequestID)
	}

	// 空字符串的 request_id 也不应落成空指针值
	blank := context.WithValue(context.Background(), ctxkey.RequestID, "   ")
	if ev := accountStatusEventFromBlocking(blank, account, until, "429", "classify"); ev.RequestID != nil {
		t.Fatalf("blank request_id must be dropped, got %v", *ev.RequestID)
	}
}

// prepareErrorLogInput 会把 UpstreamErrors 置空，只剩 JSON。这里锁住回读路径，
// 否则线上表现为「JSONB 有数据、归一化表永远为空」。
func TestBuildUpstreamAttemptRecordsFromJSONFallback(t *testing.T) {
	raw := `[{"kind":"failover","detail":"{\"error\":{\"message\":\"invalid x-api-key\",\"type\":\"authentication_error\"}}","message":"invalid x-api-key","platform":"anthropic","account_id":1,"upstream_status_code":401}]`
	entry := &OpsInsertErrorLogInput{
		RequestID:          "req-json",
		Platform:           PlatformAnthropic,
		Model:              "claude-sonnet-4-6",
		UpstreamErrorsJSON: &raw,
		// 切片已按 prepare 的逻辑被清空
		UpstreamErrors: nil,
	}

	records := buildUpstreamAttemptRecords([]*OpsInsertErrorLogInput{entry}, time.Unix(4000, 0), nil)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1 (JSON fallback must work)", len(records))
	}
	rec := records[0]
	if rec.RequestID != "req-json" || rec.AttemptNo != 1 || rec.Platform != "anthropic" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if rec.AccountID == nil || *rec.AccountID != 1 {
		t.Fatalf("account = %v", rec.AccountID)
	}
	if rec.UpstreamStatus == nil || *rec.UpstreamStatus != 401 {
		t.Fatalf("status = %v", rec.UpstreamStatus)
	}
	if rec.FailureKind != "auth" {
		t.Fatalf("failure_kind = %q, want auth", rec.FailureKind)
	}
	if rec.ErrorMsg == nil || *rec.ErrorMsg != "invalid x-api-key" {
		t.Fatalf("error_msg = %v", rec.ErrorMsg)
	}
	// Detail 里带着上游错误体时，错误码必须从中提取，而不是退回成 Kind 值
	if rec.ErrorCode == nil || *rec.ErrorCode != "authentication_error" {
		t.Fatalf("error_code = %v, want authentication_error (from detail)", rec.ErrorCode)
	}

	// 空 JSON / 非法 JSON 不应产生记录，也不应 panic
	empty := ""
	bad := "{not-json"
	for _, raw := range []string{empty, bad} {
		e := &OpsInsertErrorLogInput{RequestID: "x", UpstreamErrorsJSON: &raw}
		if got := buildUpstreamAttemptRecords([]*OpsInsertErrorLogInput{e}, time.Now(), nil); len(got) != 0 {
			t.Fatalf("raw %q must yield no records, got %d", raw, len(got))
		}
	}
}
