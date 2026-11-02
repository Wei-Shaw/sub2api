//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGrokManagedExtraSurvivesOrdinaryUpdate(t *testing.T) {
	freshModels := map[string]any{
		"models":     []any{"fresh-model"},
		"fetched_at": "2026-10-25T12:00:00Z",
	}
	freshMetadata := map[string]any{
		"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
	}
	failedModels := map[string]any{
		"models":     []any{"fresh-model"},
		"fetched_at": "2026-10-25T12:00:00Z",
		"last_attempt": map[string]any{
			"status":      "failed",
			"error_code":  "transport",
			"started_at":  "2026-10-25T13:00:00Z",
			"finished_at": "2026-10-25T13:00:01Z",
		},
	}
	oldModels := map[string]any{
		"models":     []any{"old-model"},
		"fetched_at": "2026-10-24T12:00:00Z",
	}
	oldMetadata := map[string]any{
		"models": map[string]any{"old-model": map[string]any{"id": "old-model"}},
	}

	tests := []struct {
		name         string
		accountType  string
		credentials  map[string]any
		storedExtra  map[string]any
		updateExtra  map[string]any
		updateCreds  map[string]any
		wantModels   map[string]any
		wantMetadata map[string]any
		wantBaseURL  string
	}{
		{
			name:        "oauth old form",
			accountType: service.AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-keep"},
			storedExtra: map[string]any{
				"unrelated":                           "keep",
				"grok_observed_models":                freshModels,
				service.UpstreamModelMetadataExtraKey: freshMetadata,
			},
			updateExtra: map[string]any{
				"unrelated":                           "keep",
				"note":                                "from-form",
				"grok_observed_models":                oldModels,
				service.UpstreamModelMetadataExtraKey: oldMetadata,
			},
			wantModels:   freshModels,
			wantMetadata: freshMetadata,
		},
		{
			name:        "apikey missing keys",
			accountType: service.AccountTypeAPIKey,
			credentials: map[string]any{"api_key": "sk-keep", "base_url": "https://api.example.test/v1"},
			storedExtra: map[string]any{
				"unrelated":                           "keep",
				"grok_observed_models":                freshModels,
				service.UpstreamModelMetadataExtraKey: freshMetadata,
			},
			updateExtra: map[string]any{
				"unrelated": "keep",
				"note":      "from-form",
			},
			wantModels:   freshModels,
			wantMetadata: freshMetadata,
		},
		{
			name:        "injection when absent",
			accountType: service.AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-keep"},
			storedExtra: map[string]any{"unrelated": "keep"},
			updateExtra: map[string]any{
				"unrelated":                           "keep",
				"note":                                "from-form",
				"grok_observed_models":                oldModels,
				service.UpstreamModelMetadataExtraKey: oldMetadata,
			},
		},
		{
			name:        "failed attempt",
			accountType: service.AccountTypeAPIKey,
			credentials: map[string]any{"api_key": "sk-keep"},
			storedExtra: map[string]any{
				"unrelated":                           "keep",
				"grok_observed_models":                failedModels,
				service.UpstreamModelMetadataExtraKey: freshMetadata,
			},
			updateExtra: map[string]any{
				"unrelated":                           "keep",
				"note":                                "from-form",
				"grok_observed_models":                oldModels,
				service.UpstreamModelMetadataExtraKey: oldMetadata,
			},
			wantModels:   failedModels,
			wantMetadata: freshMetadata,
		},
		{
			name:        "source identity change keeps success",
			accountType: service.AccountTypeOAuth,
			credentials: map[string]any{
				"access_token": "token-keep",
				"base_url":     "https://old.example.test/v1",
			},
			storedExtra: map[string]any{
				"unrelated":                           "keep",
				"grok_observed_models":                freshModels,
				service.UpstreamModelMetadataExtraKey: freshMetadata,
			},
			updateExtra: map[string]any{
				"unrelated":            "keep",
				"note":                 "from-form",
				"grok_observed_models": nil,
			},
			updateCreds: map[string]any{
				"access_token": "token-keep",
				"base_url":     "https://new.example.test/v1",
			},
			wantModels:   freshModels,
			wantMetadata: freshMetadata,
			wantBaseURL:  "https://new.example.test/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := testEntTx(t)
			repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
			account := mustCreateAccount(t, tx.Client(), &service.Account{
				Name:        "grok-managed-" + tt.name,
				Platform:    service.PlatformGrok,
				Type:        tt.accountType,
				Credentials: tt.credentials,
				Extra:       tt.storedExtra,
			})
			account.Name = "renamed"
			account.Extra = tt.updateExtra
			if tt.updateCreds != nil {
				account.Credentials = tt.updateCreds
			}

			require.NoError(t, repo.Update(context.Background(), account))

			got := readGrokManagedExtra(t, tx, account.ID)
			require.Equal(t, "renamed", got.name)
			require.Equal(t, "from-form", got.note)
			require.Equal(t, "keep", got.unrelated)
			require.Equal(t, tt.wantModels, got.models)
			require.Equal(t, tt.wantMetadata, got.metadata)
			if tt.wantBaseURL != "" {
				require.Equal(t, tt.wantBaseURL, got.baseURL)
			}
		})
	}
}

func TestGrokManagedExtraUpdateOverlapsCatalogCAS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	account := mustCreateGrokCatalogAccount(t, testEntClient(t), "grok-managed-overlap")
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM accounts WHERE id = $1", account.ID)
	})

	freshModels := `{"models":["fresh-model"],"fetched_at":"2026-10-25T12:00:00Z","last_attempt":{"status":"failed","error_code":"transport","started_at":"2026-10-25T13:00:00Z","finished_at":"2026-10-25T13:00:01Z"}}`
	freshMetadata := `{"models":{"fresh-model":{"id":"fresh-model"}}}`

	tx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	locked := make(chan struct{})
	release := make(chan struct{})
	holderDone := make(chan struct{})
	var holderErr error
	go func() {
		defer close(holderDone)
		rows, queryErr := tx.QueryContext(ctx, `SELECT id FROM accounts WHERE id = $1 FOR NO KEY UPDATE`, account.ID)
		if queryErr != nil {
			holderErr = queryErr
			return
		}
		if !rows.Next() {
			holderErr = rows.Err()
			if holderErr == nil {
				holderErr = errors.New("catalog account row was not locked")
			}
			_ = rows.Close()
			return
		}
		var id int64
		if scanErr := rows.Scan(&id); scanErr != nil {
			holderErr = scanErr
			_ = rows.Close()
			return
		}
		if closeErr := rows.Close(); closeErr != nil {
			holderErr = closeErr
			return
		}
		close(locked)
		select {
		case <-release:
		case <-ctx.Done():
			holderErr = ctx.Err()
			return
		}
		_, execErr := tx.ExecContext(ctx, `
			UPDATE accounts
			SET extra = jsonb_set(
				jsonb_set(COALESCE(extra, '{}'::jsonb), '{grok_observed_models}', $2::jsonb, true),
				'{upstream_model_metadata}', $3::jsonb, true)
			WHERE id = $1`, account.ID, freshModels, freshMetadata)
		if execErr != nil {
			holderErr = execErr
			return
		}
		holderErr = tx.Commit()
	}()

	select {
	case <-locked:
	case <-holderDone:
		t.Fatalf("catalog lock holder finished early: %v", holderErr)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the catalog row lock")
	}

	updateDone := make(chan error, 1)
	go func() {
		repo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
		account.Name = "saved-during-cas"
		account.Extra = map[string]any{
			"unrelated": "keep",
			"note":      "from-form",
			"grok_observed_models": map[string]any{
				"models": []any{"old-model"}, "fetched_at": "2026-10-24T12:00:00Z",
			},
			service.UpstreamModelMetadataExtraKey: map[string]any{
				"models": map[string]any{"old-model": map[string]any{"id": "old-model"}},
			},
		}
		updateDone <- repo.Update(ctx, account)
	}()
	close(release)

	select {
	case err = <-updateDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for the overlapping account update")
	}
	<-holderDone
	require.NoError(t, holderErr)

	got := readGrokManagedExtra(t, integrationDB, account.ID)
	require.Equal(t, "saved-during-cas", got.name)
	require.Equal(t, "from-form", got.note)
	require.Equal(t, "keep", got.unrelated)
	require.Equal(t, []any{"fresh-model"}, got.models["models"])
	require.Equal(t, "2026-10-25T12:00:00Z", got.models["fetched_at"])
	attempt, ok := got.models["last_attempt"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "failed", attempt["status"])
	require.Equal(t, "transport", attempt["error_code"])
	require.Equal(t, map[string]any{
		"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
	}, got.metadata)
}

type grokManagedExtraRow struct {
	name      string
	note      string
	unrelated string
	baseURL   string
	models    map[string]any
	metadata  map[string]any
}

func readGrokManagedExtra(t *testing.T, q grokCatalogSQL, id int64) grokManagedExtraRow {
	t.Helper()
	rows, err := q.QueryContext(context.Background(), `
		SELECT name,
		       COALESCE(extra->>'note', ''),
		       COALESCE(extra->>'unrelated', ''),
		       COALESCE(credentials->>'base_url', ''),
		       COALESCE((extra->'grok_observed_models')::text, 'null'),
		       COALESCE((extra->'upstream_model_metadata')::text, 'null')
		FROM accounts
		WHERE id = $1`, id)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var row grokManagedExtraRow
	var modelsRaw, metadataRaw string
	require.NoError(t, rows.Scan(&row.name, &row.note, &row.unrelated, &row.baseURL, &modelsRaw, &metadataRaw))
	require.NoError(t, rows.Err())
	if modelsRaw != "null" {
		require.NoError(t, json.Unmarshal([]byte(modelsRaw), &row.models))
	}
	if metadataRaw != "null" {
		require.NoError(t, json.Unmarshal([]byte(metadataRaw), &row.metadata))
	}
	return row
}
