package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func openCodeGoUsageRepositoryAccount() *service.Account {
	previousAttempt := time.Date(2026, time.August, 15, 8, 0, 0, 0, time.UTC)
	return &service.Account{
		ID:          17,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": "https://opencode.ai/zen/go/v1"},
		Extra: map[string]any{
			service.OpenCodeGoUsageAutoRefreshExtraKey: true,
			service.OpenCodeGoUsageSnapshotExtraKey: &service.OpenCodeGoUsageSnapshot{
				Status:        service.OpenCodeGoUsageStatusOK,
				LastAttemptAt: previousAttempt,
				NextRefreshAt: previousAttempt.Add(time.Hour),
			},
		},
	}
}

func TestUpdateOpenCodeGoUsageSnapshotWritesSnapshotOnly(t *testing.T) {
	client, mock := newOllamaCloudUsageRepositoryTestClient(t)
	account := openCodeGoUsageRepositoryAccount()
	previousSnapshotJSON, err := json.Marshal(account.Extra[service.OpenCodeGoUsageSnapshotExtraKey])
	require.NoError(t, err)

	attemptedAt := time.Date(2026, time.August, 15, 9, 0, 0, 0, time.UTC)
	snapshot := &service.OpenCodeGoUsageSnapshot{
		Status:        service.OpenCodeGoUsageStatusOK,
		LastAttemptAt: attemptedAt,
		NextRefreshAt: attemptedAt.Add(time.Hour),
	}
	expectedPayload, err := json.Marshal(map[string]any{
		service.OpenCodeGoUsageSnapshotExtraKey: snapshot,
	})
	require.NoError(t, err)
	require.NotContains(t, string(expectedPayload), service.OpenCodeGoUsageAutoRefreshExtraKey)

	credentials, err := json.Marshal(normalizeJSONMap(account.Credentials))
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
		WithArgs("key", account.ID, account.Platform, account.Type, string(credentials), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "anchor_matches", "auto_refresh", "snapshot"}).
			AddRow(account.ID, true, `true`, string(previousSnapshotJSON)))
	mock.ExpectExec(`(?s)`+regexp.QuoteMeta("UPDATE accounts")).
		WithArgs(string(expectedPayload), "key", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	repo := newAccountRepositoryWithSQL(client, nil, nil)

	err = repo.UpdateOpenCodeGoUsageSnapshot(context.Background(), account, snapshot)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
