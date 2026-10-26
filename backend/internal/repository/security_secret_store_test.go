package repository

import (
	"context"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountRepositoryCompactionSummarySecretPersistsAcrossConcurrentCreate(t *testing.T) {
	client := newSecuritySecretTestClient(t)
	repo := &accountRepository{client: client}
	ctx := context.Background()

	value, found, err := repo.GetSecuritySecret(ctx, "compaction_summary_key_v1")
	require.NoError(t, err)
	require.False(t, found)
	require.Empty(t, value)

	const workers = 8
	values := make([]string, workers)
	createdFlags := make([]bool, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			values[index], createdFlags[index], errs[index] = repo.GetOrCreateGeneratedSecuritySecret(ctx, "compaction_summary_key_v1", 32)
		}(i)
	}
	wg.Wait()

	createdCount := 0
	for i := range values {
		require.NoError(t, errs[i])
		require.Equal(t, values[0], values[i])
		if createdFlags[i] {
			createdCount++
		}
	}
	require.Equal(t, 1, createdCount)

	raw, err := hex.DecodeString(values[0])
	require.NoError(t, err)
	require.Len(t, raw, 32)

	stored, found, err := repo.GetSecuritySecret(ctx, "compaction_summary_key_v1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, values[0], stored)
}
