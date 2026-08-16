package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type accountWindowUsageAccountRepoStub struct {
	AccountRepository
	account *Account
	err     error
}

func (r *accountWindowUsageAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.account, nil
}

type accountWindowUsageReaderStub struct {
	UsageLogRepository
	queries    []AccountWindowUsageQuery
	aggregates []AccountWindowUsageAggregate
	err        error
}

func (r *accountWindowUsageReaderStub) GetAccountWindowUsage(
	_ context.Context,
	_ int64,
	queries []AccountWindowUsageQuery,
) ([]AccountWindowUsageAggregate, error) {
	r.queries = append([]AccountWindowUsageQuery(nil), queries...)
	if r.err != nil {
		return nil, r.err
	}
	return r.aggregates, nil
}

func TestValidateAccountWindowUsageTargets(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 11, 10, 30, 0, 0, time.UTC)
	valid := func(key, period string, start, end time.Time) AccountWindowUsageTarget {
		return AccountWindowUsageTarget{
			WindowKey: key,
			Period:    period,
			StartTime: start.Format(time.RFC3339),
			EndTime:   end.Format(time.RFC3339),
		}
	}
	base := valid(AccountWindowKeyFiveHour, AccountWindowPeriodCurrent, now.Add(-2*time.Hour), now.Add(-time.Hour))

	tests := []struct {
		name    string
		targets []AccountWindowUsageTarget
	}{
		{name: "empty", targets: nil},
		{name: "too many", targets: make([]AccountWindowUsageTarget, 7)},
		{name: "duplicate", targets: []AccountWindowUsageTarget{base, base}},
		{name: "invalid key", targets: []AccountWindowUsageTarget{{WindowKey: "rolling", Period: base.Period, StartTime: base.StartTime, EndTime: base.EndTime}}},
		{name: "invalid period", targets: []AccountWindowUsageTarget{{WindowKey: base.WindowKey, Period: "next", StartTime: base.StartTime, EndTime: base.EndTime}}},
		{name: "invalid start", targets: []AccountWindowUsageTarget{{WindowKey: base.WindowKey, Period: base.Period, StartTime: "yesterday", EndTime: base.EndTime}}},
		{name: "reverse", targets: []AccountWindowUsageTarget{valid(base.WindowKey, base.Period, now, now.Add(-time.Hour))}},
		{name: "too long", targets: []AccountWindowUsageTarget{valid(base.WindowKey, base.Period, now.Add(-33*24*time.Hour), now)}},
		{name: "too old", targets: []AccountWindowUsageTarget{valid(base.WindowKey, base.Period, now.Add(-66*24*time.Hour), now.Add(-65*24*time.Hour))}},
		{name: "future", targets: []AccountWindowUsageTarget{valid(base.WindowKey, base.Period, now, now.Add(2*time.Minute))}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateAccountWindowUsageTargets(test.targets, now)
			if err == nil || infraerrors.Code(err) != 400 {
				t.Fatalf("validateAccountWindowUsageTargets() error = %v, want HTTP 400", err)
			}
		})
	}

	allSix := []AccountWindowUsageTarget{
		valid(AccountWindowKeyFiveHour, AccountWindowPeriodCurrent, now.Add(-time.Hour), now),
		valid(AccountWindowKeyFiveHour, AccountWindowPeriodPrevious, now.Add(-2*time.Hour), now.Add(-time.Hour)),
		valid(AccountWindowKeySevenDay, AccountWindowPeriodCurrent, now.Add(-time.Hour), now),
		valid(AccountWindowKeySevenDay, AccountWindowPeriodPrevious, now.Add(-2*time.Hour), now.Add(-time.Hour)),
		valid(AccountWindowKeyThirtyDay, AccountWindowPeriodCurrent, now.Add(-time.Hour), now),
		valid(AccountWindowKeyThirtyDay, AccountWindowPeriodPrevious, now.Add(-2*time.Hour), now.Add(-time.Hour)),
	}
	queries, err := validateAccountWindowUsageTargets(allSix, now)
	if err != nil || len(queries) != 6 {
		t.Fatalf("six valid targets = (%d, %v), want (6, nil)", len(queries), err)
	}
}

func TestAccountUsageServiceGetWindowUsageComputesCoverageAndRates(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	targets := []AccountWindowUsageTarget{
		{
			WindowKey: AccountWindowKeyFiveHour, Period: AccountWindowPeriodCurrent,
			StartTime: now.Add(-2 * time.Hour).Format(time.RFC3339), EndTime: now.Add(-time.Hour).Format(time.RFC3339),
		},
		{
			WindowKey: AccountWindowKeyFiveHour, Period: AccountWindowPeriodPrevious,
			StartTime: now.Add(-3 * time.Hour).Format(time.RFC3339), EndTime: now.Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}
	reader := &accountWindowUsageReaderStub{aggregates: []AccountWindowUsageAggregate{
		{
			WindowKey: AccountWindowKeyFiveHour, Period: AccountWindowPeriodPrevious,
			StartTime: now.Add(-3 * time.Hour), EndTime: now.Add(-2 * time.Hour),
		},
		{
			WindowKey: AccountWindowKeyFiveHour, Period: AccountWindowPeriodCurrent,
			StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour),
			SuccessCalls: 8, FailureCalls: 2, TotalTokens: 1000, AccountCost: 1.25, StandardCost: 1, UserCost: 1.5,
		},
	}}
	service := &AccountUsageService{
		accountRepo:  &accountWindowUsageAccountRepoStub{account: &Account{ID: 7}},
		usageLogRepo: reader,
	}

	result, err := service.GetWindowUsage(context.Background(), 7, targets, AccountWindowUsageCoverage{MonitoringEnabled: true})
	if err != nil {
		t.Fatalf("GetWindowUsage() error = %v", err)
	}
	if len(reader.queries) != 2 || len(result.Items) != 2 {
		t.Fatalf("query/item count = %d/%d, want 2/2", len(reader.queries), len(result.Items))
	}
	current := result.Items[0]
	if current.TotalRequests != 10 || current.SuccessRate == nil || *current.SuccessRate != 80 {
		t.Fatalf("current item = %#v, want 10 requests and 80%%", current)
	}
	if current.SuccessRateStatus != SuccessRateStatusAvailable || !current.Matched {
		t.Fatalf("current status/matched = %s/%v", current.SuccessRateStatus, current.Matched)
	}
	if result.Items[1].SuccessRateStatus != SuccessRateStatusNoData || result.Items[1].SuccessRate != nil {
		t.Fatalf("empty item = %#v, want no_data with nil rate", result.Items[1])
	}
	if result.Items[0].Period != AccountWindowPeriodCurrent || result.Items[1].Period != AccountWindowPeriodPrevious {
		t.Fatalf("items did not preserve request order: %#v", result.Items)
	}
}

func TestAccountUsageServiceGetWindowUsageCoveragePrecedence(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	target := AccountWindowUsageTarget{
		WindowKey: AccountWindowKeySevenDay, Period: AccountWindowPeriodCurrent,
		StartTime: now.Add(-2 * time.Hour).Format(time.RFC3339), EndTime: now.Add(-time.Hour).Format(time.RFC3339),
	}
	aggregate := AccountWindowUsageAggregate{
		WindowKey: target.WindowKey, Period: target.Period,
		StartTime: now.Add(-2 * time.Hour), EndTime: now.Add(-time.Hour), FailureCalls: 3,
	}
	newService := func() *AccountUsageService {
		return &AccountUsageService{
			accountRepo:  &accountWindowUsageAccountRepoStub{account: &Account{ID: 9}},
			usageLogRepo: &accountWindowUsageReaderStub{aggregates: []AccountWindowUsageAggregate{aggregate}},
		}
	}

	disabled, err := newService().GetWindowUsage(context.Background(), 9, []AccountWindowUsageTarget{target}, AccountWindowUsageCoverage{})
	if err != nil || disabled.Items[0].SuccessRateStatus != SuccessRateStatusMonitoringDisabled {
		t.Fatalf("monitoring disabled result = %#v, %v", disabled, err)
	}

	cutoff := now.Add(-90 * time.Minute)
	limited, err := newService().GetWindowUsage(context.Background(), 9, []AccountWindowUsageTarget{target}, AccountWindowUsageCoverage{
		MonitoringEnabled: true, ErrorLogRetentionCutoff: &cutoff,
	})
	if err != nil || limited.Items[0].SuccessRateStatus != SuccessRateStatusRetentionLimited {
		t.Fatalf("retention-limited result = %#v, %v", limited, err)
	}

	available, err := newService().GetWindowUsage(context.Background(), 9, []AccountWindowUsageTarget{target}, AccountWindowUsageCoverage{MonitoringEnabled: true})
	if err != nil || available.Items[0].SuccessRate == nil || *available.Items[0].SuccessRate != 0 {
		t.Fatalf("failure-only result = %#v, %v, want available 0%%", available, err)
	}
}

func TestAccountUsageServiceGetWindowUsageErrors(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	target := AccountWindowUsageTarget{
		WindowKey: AccountWindowKeyFiveHour, Period: AccountWindowPeriodCurrent,
		StartTime: now.Add(-2 * time.Hour).Format(time.RFC3339), EndTime: now.Add(-time.Hour).Format(time.RFC3339),
	}

	missing := &AccountUsageService{
		accountRepo:  &accountWindowUsageAccountRepoStub{err: ErrAccountNotFound},
		usageLogRepo: &accountWindowUsageReaderStub{},
	}
	if _, err := missing.GetWindowUsage(context.Background(), 88, []AccountWindowUsageTarget{target}, AccountWindowUsageCoverage{}); !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("missing account error = %v", err)
	}

	repoErr := errors.New("query failed")
	failing := &AccountUsageService{
		accountRepo:  &accountWindowUsageAccountRepoStub{account: &Account{ID: 7}},
		usageLogRepo: &accountWindowUsageReaderStub{err: repoErr},
	}
	if _, err := failing.GetWindowUsage(context.Background(), 7, []AccountWindowUsageTarget{target}, AccountWindowUsageCoverage{}); !errors.Is(err, repoErr) {
		t.Fatalf("repository error = %v", err)
	}

	mismatchedRange := &AccountUsageService{
		accountRepo: &accountWindowUsageAccountRepoStub{account: &Account{ID: 7}},
		usageLogRepo: &accountWindowUsageReaderStub{aggregates: []AccountWindowUsageAggregate{{
			WindowKey: target.WindowKey,
			Period:    target.Period,
			StartTime: now.Add(-3 * time.Hour),
			EndTime:   now.Add(-time.Hour),
		}}},
	}
	if _, err := mismatchedRange.GetWindowUsage(context.Background(), 7, []AccountWindowUsageTarget{target}, AccountWindowUsageCoverage{}); err == nil {
		t.Fatal("mismatched aggregate range error = nil")
	}
}
