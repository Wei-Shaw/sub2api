//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func result(status string, finishedAt time.Time) *ScheduledTestResult {
	return &ScheduledTestResult{Status: status, FinishedAt: finishedAt}
}

func TestEvaluateHealth_Priority(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Hour) // 未过期

	tests := []struct {
		name   string
		acc    *Account
		latest *ScheduledTestResult
		want   string
	}{
		{
			name: "1-disabled status -> paused",
			acc:  &Account{Status: StatusDisabled, Schedulable: true},
			want: HealthPaused,
		},
		{
			name: "1-schedulable false -> paused (active)",
			acc:  &Account{Status: StatusActive, Schedulable: false},
			want: HealthPaused,
		},
		{
			name:   "1-paused wins over success result",
			acc:    &Account{Status: StatusActive, Schedulable: false},
			latest: result("success", fresh),
			want:   HealthPaused,
		},
		{
			name: "2-error status -> error",
			acc:  &Account{Status: StatusError, Schedulable: true},
			want: HealthError,
		},
		{
			name:   "2-error status wins over success result",
			acc:    &Account{Status: StatusError, Schedulable: true},
			latest: result("success", fresh),
			want:   HealthError,
		},
		{
			name: "3-rate limited -> limited",
			acc:  &Account{Status: StatusActive, Schedulable: true, RateLimitResetAt: ptrTime(now.Add(time.Hour))},
			want: HealthLimited,
		},
		{
			name: "3-overloaded -> limited",
			acc:  &Account{Status: StatusActive, Schedulable: true, OverloadUntil: ptrTime(now.Add(time.Hour))},
			want: HealthLimited,
		},
		{
			name: "3-temp unschedulable -> limited",
			acc:  &Account{Status: StatusActive, Schedulable: true, TempUnschedulableUntil: ptrTime(now.Add(time.Hour))},
			want: HealthLimited,
		},
		{
			name:   "3-limited wins over success result",
			acc:    &Account{Status: StatusActive, Schedulable: true, OverloadUntil: ptrTime(now.Add(time.Hour))},
			latest: result("success", fresh),
			want:   HealthLimited,
		},
		{
			name:   "4-failed result not expired -> error",
			acc:    &Account{Status: StatusActive, Schedulable: true},
			latest: result("failed", fresh),
			want:   HealthError,
		},
		{
			name:   "5-success result not expired -> healthy",
			acc:    &Account{Status: StatusActive, Schedulable: true},
			latest: result("success", fresh),
			want:   HealthHealthy,
		},
		{
			name: "6-no result -> untested",
			acc:  &Account{Status: StatusActive, Schedulable: true},
			want: HealthUntested,
		},
		{
			name:   "6-zero finished_at -> untested",
			acc:    &Account{Status: StatusActive, Schedulable: true},
			latest: result("success", time.Time{}),
			want:   HealthUntested,
		},
		{
			name: "nil account -> untested",
			acc:  nil,
			want: HealthUntested,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, EvaluateHealth(tc.acc, tc.latest, now))
		})
	}
}

func TestEvaluateHealth_TTLBoundary(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	acc := &Account{Status: StatusActive, Schedulable: true}

	// 刚好 24h:now - finishedAt == TTL,边界内(<=),仍视为有效。
	exactly24h := now.Add(-HealthResultTTL)
	require.Equal(t, HealthHealthy, EvaluateHealth(acc, result("success", exactly24h), now),
		"finished exactly TTL ago should still count")
	require.Equal(t, HealthError, EvaluateHealth(acc, result("failed", exactly24h), now))

	// 超过 24h:过期,降级为 untested。
	over24h := now.Add(-HealthResultTTL - time.Second)
	require.Equal(t, HealthUntested, EvaluateHealth(acc, result("success", over24h), now),
		"expired success result should be untested")
	require.Equal(t, HealthUntested, EvaluateHealth(acc, result("failed", over24h), now),
		"expired failed result should be untested")
}
