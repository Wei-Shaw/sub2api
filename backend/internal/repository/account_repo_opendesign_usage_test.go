package repository

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCompareAndSwapOpenDesignUsageDoesNotOverwriteChangedBinding(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	account := &service.Account{
		ID: 32, Platform: service.PlatformOpenDesign, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture"},
		Extra:       map[string]any{service.OpenDesignUsageExtraKey: json.RawMessage(`{"ciphertext":"old"}`)},
	}
	mock.ExpectExec(`(?s)UPDATE accounts .*credentials = \$3::jsonb.*AND parent_account_id IS NULL AND \(extra -> \$2::text\) IS NOT DISTINCT FROM \$5::jsonb`).
		WithArgs(int64(32), service.OpenDesignUsageExtraKey, `{"api_key":"fixture"}`, nil, `{"ciphertext":"old"}`, nil, service.PlatformOpenDesign).
		WillReturnResult(sqlmock.NewResult(0, 0))
	repo := newAccountRepositoryWithSQL(client, nil, nil)
	applied, err := repo.CompareAndSwapOpenDesignUsage(context.Background(), account, nil)
	require.NoError(t, err)
	require.False(t, applied)
	require.NoError(t, mock.ExpectationsWereMet())
}
