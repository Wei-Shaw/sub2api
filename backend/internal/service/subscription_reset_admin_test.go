package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type resetAdminRepoStub struct {
	SubscriptionResetObserverRepository
	saved int
}

func (r *resetAdminRepoStub) SavePolicy(_ context.Context, p *SubscriptionResetPolicy) (*SubscriptionResetPolicy, error) {
	r.saved++
	return p, nil
}

type resetAdminGroupsStub struct {
	GroupRepository
	group *Group
}

func (g *resetAdminGroupsStub) GetByIDLite(context.Context, int64) (*Group, error) {
	return g.group, nil
}

type resetAdminAccountsStub struct {
	AccountRepository
	accounts []*Account
}

func (a *resetAdminAccountsStub) GetByIDs(context.Context, []int64) ([]*Account, error) {
	return a.accounts, nil
}

func TestSubscriptionResetAdminRejectsUnrelatedReferences(t *testing.T) {
	for _, account := range []*Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, GroupIDs: []int64{2}},
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, GroupIDs: []int64{1}},
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, QuotaDimension: "spark", GroupIDs: []int64{1}},
	} {
		r := &resetAdminRepoStub{}
		s := NewAdminSubscriptionResetService(r, &resetAdminGroupsStub{group: &Group{ID: 1, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeSubscription}}, &resetAdminAccountsStub{accounts: []*Account{account}})
		p := DefaultSubscriptionResetPolicy(1)
		p.Mode = "observe"
		p.AccountIDs = []int64{1}
		_, err := s.SavePolicy(context.Background(), p)
		require.Error(t, err)
		require.Zero(t, r.saved)
	}
}

func TestSubscriptionResetAdminDoesNotExcludeExhaustedAccounts(t *testing.T) {
	r := &resetAdminRepoStub{}
	s := NewAdminSubscriptionResetService(r, &resetAdminGroupsStub{group: &Group{ID: 1, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeSubscription}}, &resetAdminAccountsStub{accounts: []*Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false, GroupIDs: []int64{1}}}})
	p := DefaultSubscriptionResetPolicy(1)
	p.Mode = "observe"
	p.AccountIDs = []int64{1}
	_, err := s.SavePolicy(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, 1, r.saved)
}
