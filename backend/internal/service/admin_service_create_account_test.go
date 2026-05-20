//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type accountRepoStubForCreateAccount struct {
	accountRepoStub
	created *Account
}

func (s *accountRepoStubForCreateAccount) Create(_ context.Context, account *Account) error {
	copied := *account
	s.created = &copied
	if account.ID == 0 {
		account.ID = 101
	}
	return nil
}

func (s *accountRepoStubForCreateAccount) BindGroups(_ context.Context, _ int64, _ []int64) error {
	return nil
}

func TestAdminServiceCreateAccount_BatchDefaultsSchedulable(t *testing.T) {
	batchID := int64(7)
	repo := &accountRepoStubForCreateAccount{}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "batch-account",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		Credentials:          map[string]any{"token": "x"},
		Concurrency:          3,
		Priority:             50,
		BatchID:              &batchID,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotNil(t, account)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.Schedulable)
	require.True(t, account.Schedulable)
}

func TestAdminServiceCreateAccount_ExplicitSchedulableFalseWins(t *testing.T) {
	batchID := int64(7)
	schedulable := false
	repo := &accountRepoStubForCreateAccount{}
	svc := &adminServiceImpl{accountRepo: repo}

	account, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                 "batch-account-paused",
		Platform:             PlatformOpenAI,
		Type:                 AccountTypeOAuth,
		Credentials:          map[string]any{"token": "x"},
		Concurrency:          3,
		Priority:             50,
		BatchID:              &batchID,
		Schedulable:          &schedulable,
		SkipDefaultGroupBind: true,
	})

	require.NoError(t, err)
	require.NotNil(t, account)
	require.NotNil(t, repo.created)
	require.False(t, repo.created.Schedulable)
	require.False(t, account.Schedulable)
}
