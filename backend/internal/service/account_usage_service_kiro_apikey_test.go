//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountUsageService_GetUsage_KiroAPIKeyUnsupported(t *testing.T) {
	account := &Account{
		ID:       9101,
		Platform: PlatformKiro,
		Type:     AccountTypeAPIKey,
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	usage, err := svc.GetUsage(context.Background(), account.ID)
	require.Nil(t, usage)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not support usage query")
}

func TestAccountUsageService_GetPassiveUsage_KiroAPIKeyUnsupported(t *testing.T) {
	account := &Account{
		ID:       9102,
		Platform: PlatformKiro,
		Type:     AccountTypeAPIKey,
	}
	repo := &mockAccountRepoForGemini{accountsByID: map[int64]*Account{account.ID: account}}
	svc := NewAccountUsageService(repo, nil, nil, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	usage, err := svc.GetPassiveUsage(context.Background(), account.ID)
	require.Nil(t, usage)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Kiro OAuth")
}
