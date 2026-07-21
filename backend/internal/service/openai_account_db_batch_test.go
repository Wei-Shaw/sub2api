package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type openAIAccountDBBatchRepo struct {
	AccountRepository
	accounts   map[int64]*Account
	batchCalls [][]int64
	errOnCall  int
}

func (r *openAIAccountDBBatchRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.batchCalls = append(r.batchCalls, append([]int64(nil), ids...))
	if r.errOnCall == len(r.batchCalls) {
		return nil, errors.New("db unavailable")
	}
	accounts := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account := r.accounts[id]; account != nil {
			copy := *account
			accounts = append(accounts, &copy)
		}
	}
	return accounts, nil
}

func TestLoadOpenAIAccountDBBatchLoadsParentsOnce(t *testing.T) {
	parentID := int64(10)
	repo := &openAIAccountDBBatchRepo{accounts: map[int64]*Account{
		1:  {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ParentAccountID: &parentID},
		2:  {ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ParentAccountID: &parentID},
		10: {ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
	}}
	service := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: &SchedulerSnapshotService{}}
	before := GetSchedulerOptimizationMetricsSnapshot()

	batch := service.loadOpenAIAccountDBBatch(context.Background(), []*Account{{ID: 1}, {ID: 2}, {ID: 1}})
	require.NotNil(t, batch)
	require.Equal(t, [][]int64{{1, 2}, {10}}, repo.batchCalls)
	require.NotNil(t, batch.accounts[1])
	require.NotNil(t, batch.parentLookup(10))
	after := GetSchedulerOptimizationMetricsSnapshot()
	require.Equal(t, before.Database.BatchTotal+1, after.Database.BatchTotal)
	require.Equal(t, before.Database.QueryTotal+2, after.Database.QueryTotal)
	require.Equal(t, before.Database.CandidateIDTotal+2, after.Database.CandidateIDTotal)
	require.Equal(t, before.Database.ParentIDTotal+1, after.Database.ParentIDTotal)
	require.Equal(t, before.Database.ReturnedAccountTotal+3, after.Database.ReturnedAccountTotal)
}

func TestLoadOpenAIAccountDBBatchParentFailureFailsClosed(t *testing.T) {
	parentID := int64(10)
	repo := &openAIAccountDBBatchRepo{
		accounts: map[int64]*Account{
			1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, ParentAccountID: &parentID},
		},
		errOnCall: 2,
	}
	service := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: &SchedulerSnapshotService{}}
	before := GetSchedulerOptimizationMetricsSnapshot()
	batch := service.loadOpenAIAccountDBBatch(context.Background(), []*Account{{ID: 1}})

	require.True(t, batch.failed)
	require.Nil(t, service.recheckSelectedOpenAIAccountFromDBBatch(
		context.Background(),
		&Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		nil,
		PlatformOpenAI,
		"",
		false,
		OpenAIEndpointCapabilityChatCompletions,
		batch,
	))
	after := GetSchedulerOptimizationMetricsSnapshot()
	require.Equal(t, before.Database.ErrorTotal+1, after.Database.ErrorTotal)
}

func TestLoadOpenAIAccountDBBatchBoundsCandidateQueries(t *testing.T) {
	accounts := make(map[int64]*Account, openAIAccountSelectionProbeLimit+1)
	candidates := make([]*Account, 0, len(accounts))
	for id := int64(1); id <= int64(openAIAccountSelectionProbeLimit+1); id++ {
		account := &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		accounts[id] = account
		candidates = append(candidates, account)
	}
	repo := &openAIAccountDBBatchRepo{accounts: accounts}
	service := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: &SchedulerSnapshotService{}}

	batch := service.loadOpenAIAccountDBBatch(context.Background(), candidates)
	require.NotNil(t, batch)
	require.Len(t, repo.batchCalls, 2)
	require.Len(t, repo.batchCalls[0], openAIAccountSelectionProbeLimit)
	require.Len(t, repo.batchCalls[1], 1)
	for _, candidate := range candidates {
		account, covered := batch.account(candidate.ID)
		require.True(t, covered)
		require.NotNil(t, account)
	}
}
