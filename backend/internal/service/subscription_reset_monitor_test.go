package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type resetMonitorRepoStub struct {
	SubscriptionResetObserverRepository
	policies []*SubscriptionResetPolicy
	samples  map[int64][]SubscriptionResetSample
}

func (r *resetMonitorRepoStub) ListEnabled(context.Context) ([]*SubscriptionResetPolicy, error) {
	return r.policies, nil
}
func (r *resetMonitorRepoStub) RecordSample(_ context.Context, groupID, _ int64, sample *SubscriptionResetSample) error {
	r.samples[groupID] = append(r.samples[groupID], *sample)
	return nil
}

type resetMonitorAccountsStub struct {
	AccountRepository
	account *Account
	calls   int
}

func (a *resetMonitorAccountsStub) GetByID(context.Context, int64) (*Account, error) {
	a.calls++
	return a.account, nil
}

type resetMonitorQuotaStub struct {
	usage *OpenAIQuotaUsage
	err   error
	calls int
}

func (q *resetMonitorQuotaStub) QueryUsageSnapshot(context.Context, int64) (*OpenAIQuotaUsage, error) {
	q.calls++
	return q.usage, q.err
}

func resetMonitorFixture(t *testing.T) (*SubscriptionResetMonitor, *resetMonitorRepoStub, *resetMonitorQuotaStub) {
	t.Helper()
	p1, p2 := DefaultSubscriptionResetPolicy(1), DefaultSubscriptionResetPolicy(2)
	p1.Mode, p2.Mode = "observe", "observe"
	p1.AccountIDs, p2.AccountIDs = []int64{10}, []int64{10}
	r := &resetMonitorRepoStub{policies: []*SubscriptionResetPolicy{p1, p2}, samples: map[int64][]SubscriptionResetSample{}}
	account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, GroupIDs: []int64{1, 2}}
	var usage OpenAIQuotaUsage
	require.NoError(t, json.Unmarshal([]byte(`{"user_id":"member","account_id":"workspace","plan_type":"pro","rate_limit":{"secondary_window":{"used_percent":1,"limit_window_seconds":604800,"reset_at":1800000000}}}`), &usage))
	q := &resetMonitorQuotaStub{usage: &usage}
	m := NewSubscriptionResetMonitor(r, &resetMonitorAccountsStub{account: account}, q, nil, nil)
	m.now = func() time.Time { return time.Unix(1799999000, 0) }
	return m, r, q
}

func TestSubscriptionResetMonitorSharesQueryAndKeepsLocalMarker(t *testing.T) {
	m, r, q := resetMonitorFixture(t)
	defer m.Stop()
	marker := m.now().Add(-time.Minute)
	accounts, ok := m.accounts.(*resetMonitorAccountsStub)
	require.True(t, ok)
	accounts.account.Extra = map[string]any{"codex_history_reset_at": marker.Format(time.RFC3339)}
	m.runOnce(context.Background())
	require.Equal(t, 1, q.calls)
	for _, groupID := range []int64{1, 2} {
		require.Len(t, r.samples[groupID], 1)
		sample := r.samples[groupID][0]
		require.Empty(t, sample.Error)
		require.Equal(t, 1.0, *sample.UsedPercent)
		require.True(t, marker.Equal(*sample.LocalResetAt))
	}
	m.runOnce(context.Background())
	require.Equal(t, 1, q.calls, "a fresh account must respect the polling interval")
}

func TestSubscriptionResetMonitorFailuresRemainObservations(t *testing.T) {
	m, r, q := resetMonitorFixture(t)
	defer m.Stop()
	q.err = errors.New("sensitive upstream response must not enter state")
	m.runOnce(context.Background())
	for _, groupID := range []int64{1, 2} {
		sample := r.samples[groupID][0]
		require.Equal(t, "quota_query_failed", sample.Error)
		require.Nil(t, sample.UsedPercent)
	}
	require.Equal(t, 1, m.schedule[10].failures)
}

func TestSubscriptionResetMonitorRemovedMemberCannotVote(t *testing.T) {
	m, r, _ := resetMonitorFixture(t)
	defer m.Stop()
	accounts, ok := m.accounts.(*resetMonitorAccountsStub)
	require.True(t, ok)
	accounts.account.GroupIDs = []int64{1}
	m.runOnce(context.Background())
	require.Empty(t, r.samples[1][0].Error)
	require.Equal(t, "account_not_in_group", r.samples[2][0].Error)
}

func TestSubscriptionResetMonitorUnknownAndAmbiguousWindows(t *testing.T) {
	for _, body := range []string{
		`{"user_id":"u","account_id":"w","rate_limit":{"secondary_window":{"limit_window_seconds":604800,"reset_at":1800000000}}}`,
		`{"user_id":"u","account_id":"w","rate_limit":{"secondary_window":{"used_percent":0,"limit_window_seconds":2592000,"reset_at":1800000000}}}`,
		`{"account_id":"w","rate_limit":{"secondary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":1800000000}}}`,
		`{"user_id":"u","account_id":"w","rate_limit":{"primary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":1800000000},"secondary_window":{"used_percent":0,"limit_window_seconds":604800,"reset_at":1800000000}}}`,
	} {
		var usage OpenAIQuotaUsage
		require.NoError(t, json.Unmarshal([]byte(body), &usage))
		sample := &SubscriptionResetSample{}
		populateSubscriptionResetSample(sample, &usage)
		require.NotEmpty(t, sample.Error)
		require.Nil(t, sample.UsedPercent)
	}
}

func TestSubscriptionResetMonitorStopBeforeStart(t *testing.T) {
	m, _, _ := resetMonitorFixture(t)
	m.Stop()
	m.Start()
	m.Stop()
	require.False(t, m.started)
}
