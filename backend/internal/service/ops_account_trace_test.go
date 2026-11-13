package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGetAccountTraceRejectsInvalidID(t *testing.T) {
	svc := &OpsService{opsRepo: &opsRepoMock{}}
	if _, err := svc.GetAccountTrace(context.Background(), 0, 24, 50); err == nil {
		t.Fatal("expected error for account id 0")
	}
	if _, err := svc.GetAccountTrace(context.Background(), -3, 24, 50); err == nil {
		t.Fatal("expected error for negative account id")
	}
}

func TestGetAccountTraceClampsWindowAndLimit(t *testing.T) {
	var gotSince time.Time
	var gotLimit int
	repo := &opsRepoMock{
		GetAccountTraceFn: func(ctx context.Context, accountID int64, since time.Time, limit int) (*OpsAccountTrace, error) {
			gotSince, gotLimit = since, limit
			return &OpsAccountTrace{}, nil
		},
	}
	svc := &OpsService{opsRepo: repo}

	before := time.Now()
	trace, err := svc.GetAccountTrace(context.Background(), 7, 10_000, 10_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != accountTraceMaxLimit {
		t.Fatalf("limit = %d, want %d", gotLimit, accountTraceMaxLimit)
	}
	window := before.Sub(gotSince)
	if window < time.Duration(accountTraceMaxHours)*time.Hour-time.Minute ||
		window > time.Duration(accountTraceMaxHours)*time.Hour+time.Minute {
		t.Fatalf("window = %v, want ~%dh", window, accountTraceMaxHours)
	}
	// 空结果必须是空数组而不是 null，前端/脚本依赖这一点。
	if trace.AccountID != 7 || trace.Attempts == nil || trace.StatusEvents == nil || trace.LifecycleEvents == nil {
		t.Fatalf("expected non-nil empty slices and account id, got %+v", trace)
	}

	if _, err := svc.GetAccountTrace(context.Background(), 7, 0, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLimit != accountTraceDefaultLimit {
		t.Fatalf("default limit = %d, want %d", gotLimit, accountTraceDefaultLimit)
	}
}

func TestCountAccountsNeedingRenewal(t *testing.T) {
	now := time.Now()
	items := []*OpsExpiringAccount{
		{ID: 1, ExpiresAt: now.Add(24 * time.Hour), Status: StatusActive, Schedulable: true}, // 即将到期
		{ID: 2, ExpiresAt: now.Add(-time.Hour), Status: StatusActive, Schedulable: true},     // 已到期但仍在调度
		{ID: 3, ExpiresAt: now.Add(-time.Hour), Status: StatusActive, Schedulable: false},    // 已到期已暂停：不计
		{ID: 4, ExpiresAt: now.Add(-time.Hour), Status: StatusDisabled, Schedulable: true},   // 已禁用：不计
		nil,
	}
	if got := countAccountsNeedingRenewal(items, now); got != 2 {
		t.Fatalf("countAccountsNeedingRenewal = %d, want 2", got)
	}
}

func TestAccountFailureRateMetricAndDescription(t *testing.T) {
	repo := &opsRepoMock{
		GetAccountAttemptStatsFn: func(ctx context.Context, since time.Time, minAttempts int) ([]*AccountAttemptStat, error) {
			if minAttempts != accountFailureRateMinAttempts {
				t.Fatalf("minAttempts = %d, want %d", minAttempts, accountFailureRateMinAttempts)
			}
			return []*AccountAttemptStat{
				{AccountID: 42, Attempts: 10, Failures: 8, FailurePct: 80, TopKind: "auth"},
				{AccountID: 7, Attempts: 20, Failures: 2, FailurePct: 10, TopKind: "rate_limit"},
			}, nil
		},
	}
	s := &OpsAlertEvaluatorService{opsRepo: repo}
	rule := &OpsAlertRule{ID: 9, MetricType: opsMetricAccountFailureRate, Operator: ">", Threshold: 50}

	now := time.Now()
	v, ok := s.computeRuleMetric(context.Background(), rule, nil, now.Add(-time.Hour), now, "", nil)
	if !ok || v != 80 {
		t.Fatalf("metric = %v ok=%v, want 80 true", v, ok)
	}
	desc := s.appendWorstAccountDescription(rule, "base")
	for _, want := range []string{"worst_account=42", "attempts=10", "failures=8", "top_kind=auth"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("description %q missing %q", desc, want)
		}
	}

	// 没有失败账号：指标为 0 且仍然有效（让告警能自动恢复），描述不再带旧账号。
	repo.GetAccountAttemptStatsFn = func(ctx context.Context, since time.Time, minAttempts int) ([]*AccountAttemptStat, error) {
		return nil, nil
	}
	v, ok = s.computeRuleMetric(context.Background(), rule, nil, now.Add(-time.Hour), now, "", nil)
	if !ok || v != 0 {
		t.Fatalf("metric = %v ok=%v, want 0 true", v, ok)
	}
	if got := s.appendWorstAccountDescription(rule, "base"); got != "base" {
		t.Fatalf("stale worst account leaked into description: %q", got)
	}

	// 其它指标不受影响。
	other := &OpsAlertRule{ID: 9, MetricType: "error_rate"}
	if got := s.appendWorstAccountDescription(other, "base"); got != "base" {
		t.Fatalf("non-account metric description changed: %q", got)
	}
}

func TestNotifyAccountSchedulingBlockedEmitsWithoutRuntimeBlocker(t *testing.T) {
	sink := &captureStatusSink{ch: make(chan *OpsAccountStatusEventRecord, 1)}
	SetAccountStatusEventSink(sink)
	defer SetAccountStatusEventSink(nil)

	svc := &RateLimitService{} // runtimeBlocker 为 nil 时也必须留痕
	svc.notifyAccountSchedulingBlocked(context.Background(), &Account{ID: 5, Status: StatusActive}, time.Time{}, "auth_error")

	select {
	case ev := <-sink.ch:
		if ev.AccountID != 5 || ev.ToState != "invalid" || ev.Source != "classify" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("status event was not emitted")
	}
}

type captureStatusSink struct {
	ch chan *OpsAccountStatusEventRecord
}

func (c *captureStatusSink) RecordAccountStatusEvents(ctx context.Context, events []*OpsAccountStatusEventRecord) error {
	for _, ev := range events {
		c.ch <- ev
	}
	return nil
}
