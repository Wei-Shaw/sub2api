package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeCapabilityAccountLister struct {
	accounts []Account
	err      error
}

func (f *fakeCapabilityAccountLister) ListActive(context.Context) ([]Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]Account(nil), f.accounts...), nil
}

type storedCapabilitySnapshot struct {
	id     int64
	models []string
}

type fakeUpstreamModelStore struct {
	models map[int64][]string
	errs   map[int64]error
	stored []storedCapabilitySnapshot
}

func (f *fakeUpstreamModelStore) FetchUpstreamSupportedModels(_ context.Context, account *Account) ([]string, error) {
	if account == nil {
		return nil, errors.New("account is required")
	}
	if err := f.errs[account.ID]; err != nil {
		return nil, err
	}
	return append([]string(nil), f.models[account.ID]...), nil
}

func (f *fakeUpstreamModelStore) StoreUpstreamModelSnapshot(_ context.Context, account *Account, models []string) error {
	if account == nil {
		return errors.New("account is required")
	}
	f.stored = append(f.stored, storedCapabilitySnapshot{
		id:     account.ID,
		models: append([]string(nil), models...),
	})
	return nil
}

func TestSelectDueUpstreamCapabilityAccounts_PrefersUnsyncedAndStale(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	parentID := int64(99)
	accounts := []Account{
		{ID: 1, Platform: PlatformAntigravity, Extra: ApplyUpstreamModelSnapshot(nil, []string{"fresh"}, now.Add(-10*time.Minute))},
		{ID: 2, Platform: PlatformAntigravity, Extra: ApplyUpstreamModelSnapshot(nil, []string{"stale"}, now.Add(-2*time.Hour))},
		{ID: 3, Platform: PlatformAntigravity},
		{ID: 4, Platform: PlatformAntigravity, ParentAccountID: &parentID},
		{ID: 5, Platform: "unknown"},
	}

	due := selectDueUpstreamCapabilityAccounts(accounts, now, time.Hour, 10)
	require.Equal(t, []int64{3, 2}, dueIDs(due))

	due = selectDueUpstreamCapabilityAccounts(accounts, now, time.Hour, 1)
	require.Equal(t, []int64{3}, dueIDs(due))
}

func TestUpstreamCapabilitySyncService_RunDueStoresSuccessAndKeepsFailedSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeUpstreamModelStore{
		models: map[int64][]string{
			1: {"gemini-3.7-flash"},
		},
		errs: map[int64]error{
			2: errors.New("upstream timeout"),
		},
	}
	svc := NewUpstreamCapabilitySyncService(&fakeCapabilityAccountLister{
		accounts: []Account{
			{ID: 1, Platform: PlatformAntigravity},
			{ID: 2, Platform: PlatformOpenAI, Extra: ApplyUpstreamModelSnapshot(nil, []string{"old-model"}, now.Add(-3*time.Hour))},
		},
	}, store)
	svc.now = func() time.Time { return now }
	svc.interval = time.Hour
	svc.maxPerCycle = 10

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, []storedCapabilitySnapshot{
		{id: 1, models: []string{"gemini-3.7-flash"}},
	}, store.stored)
}

func TestUpstreamCapabilitySyncService_RunDueSkipsFreshSnapshots(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeUpstreamModelStore{
		models: map[int64][]string{1: {"gemini-3.7-flash"}},
	}
	svc := NewUpstreamCapabilitySyncService(&fakeCapabilityAccountLister{
		accounts: []Account{
			{ID: 1, Platform: PlatformAntigravity, Extra: ApplyUpstreamModelSnapshot(nil, []string{"fresh"}, now.Add(-5*time.Minute))},
		},
	}, store)
	svc.now = func() time.Time { return now }
	svc.interval = time.Hour

	require.NoError(t, svc.RunDue(context.Background()))
	require.Empty(t, store.stored)
}

func dueIDs(accounts []Account) []int64 {
	out := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		out = append(out, account.ID)
	}
	return out
}
