package repository

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestLockAndMergeAccountProbeExtraKeepsLatestGrokManagedKeys(t *testing.T) {
	freshModels := []byte(`{"fetched_at":"2026-10-25T12:00:00Z","models":["fresh-model"]}`)
	freshMetadata := []byte(`{"models":{"fresh-model":{"id":"fresh-model"}}}`)
	failedModels := []byte(`{"fetched_at":"2026-10-25T12:00:00Z","last_attempt":{"error_code":"transport","finished_at":"2026-10-25T13:00:01Z","started_at":"2026-10-25T13:00:00Z","status":"failed"},"models":["fresh-model"]}`)

	tests := []struct {
		name             string
		accountType      string
		credentials      map[string]any
		inputExtra       map[string]any
		databaseModels   []byte
		databaseMetadata []byte
		wantModels       map[string]any
		wantMetadata     map[string]any
	}{
		{
			name:        "old client snapshot and metadata lose to the locked row",
			accountType: service.AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-keep"},
			inputExtra: map[string]any{
				"note": "from-form",
				"grok_observed_models": map[string]any{
					"models": []any{"old-model"}, "fetched_at": "2026-10-24T12:00:00Z",
				},
				service.UpstreamModelMetadataExtraKey: map[string]any{
					"models": map[string]any{"old-model": map[string]any{"id": "old-model"}},
				},
			},
			databaseModels:   freshModels,
			databaseMetadata: freshMetadata,
			wantModels: map[string]any{
				"fetched_at": "2026-10-25T12:00:00Z",
				"models":     []any{"fresh-model"},
			},
			wantMetadata: map[string]any{
				"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
			},
		},
		{
			name:             "apikey payload that omits the keys still restores them",
			accountType:      service.AccountTypeAPIKey,
			credentials:      map[string]any{"api_key": "sk-keep"},
			inputExtra:       map[string]any{"note": "from-form"},
			databaseModels:   freshModels,
			databaseMetadata: freshMetadata,
			wantModels: map[string]any{
				"fetched_at": "2026-10-25T12:00:00Z",
				"models":     []any{"fresh-model"},
			},
			wantMetadata: map[string]any{
				"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
			},
		},
		{
			name:        "injection does not create keys missing from the locked row",
			accountType: service.AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-keep"},
			inputExtra: map[string]any{
				"note": "from-form",
				"grok_observed_models": map[string]any{
					"models": []any{"injected"}, "fetched_at": "2026-10-25T12:00:00Z",
				},
				service.UpstreamModelMetadataExtraKey: map[string]any{"models": map[string]any{}},
			},
		},
		{
			name:        "failed attempt is kept when the client sends an older success",
			accountType: service.AccountTypeAPIKey,
			credentials: map[string]any{"api_key": "sk-keep"},
			inputExtra: map[string]any{
				"note": "from-form",
				"grok_observed_models": map[string]any{
					"models": []any{"old-model"}, "fetched_at": "2026-10-24T12:00:00Z",
				},
			},
			databaseModels:   failedModels,
			databaseMetadata: freshMetadata,
			wantModels: map[string]any{
				"fetched_at": "2026-10-25T12:00:00Z",
				"models":     []any{"fresh-model"},
				"last_attempt": map[string]any{
					"error_code":  "transport",
					"finished_at": "2026-10-25T13:00:01Z",
					"started_at":  "2026-10-25T13:00:00Z",
					"status":      "failed",
				},
			},
			wantMetadata: map[string]any{
				"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
			},
		},
		{
			name:        "credential identity change still keeps the locked success",
			accountType: service.AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-new", "base_url": "https://new.example.test/v1"},
			inputExtra: map[string]any{
				"note":                 "from-form",
				"grok_observed_models": nil,
			},
			databaseModels:   freshModels,
			databaseMetadata: freshMetadata,
			wantModels: map[string]any{
				"fetched_at": "2026-10-25T12:00:00Z",
				"models":     []any{"fresh-model"},
			},
			wantMetadata: map[string]any{
				"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })

			credentials, err := json.Marshal(normalizeJSONMap(tt.credentials))
			require.NoError(t, err)
			mock.ExpectQuery(`(?s)`+regexp.QuoteMeta("SELECT")+`.*`+regexp.QuoteMeta("FOR NO KEY UPDATE")).
				WithArgs(int64(41), service.PlatformGrok, tt.accountType, string(credentials), nil).
				WillReturnRows(sqlmock.NewRows([]string{
					"identity_unchanged", "ollama_group_unchanged", "ollama_proxy_unchanged",
					"enabled", "rate_sync_enabled", "snapshot",
					"ollama_session", "ollama_auto", "ollama_snapshot",
					"opencode_group_unchanged", "opencode_auto", "opencode_snapshot",
					"grok_observed_models", "upstream_model_metadata",
				}).AddRow(false, false, true, nil, nil, nil, nil, nil, nil, false, nil, nil, tt.databaseModels, tt.databaseMetadata))

			account := &service.Account{
				ID:          41,
				Platform:    service.PlatformGrok,
				Type:        tt.accountType,
				Credentials: tt.credentials,
				Extra:       tt.inputExtra,
			}
			got, err := lockAndMergeAccountProbeExtra(context.Background(), client, account, nil, nil)
			require.NoError(t, err)
			require.Equal(t, "from-form", got["note"])
			if tt.wantModels == nil {
				require.NotContains(t, got, "grok_observed_models")
			} else {
				require.Equal(t, tt.wantModels, got["grok_observed_models"])
			}
			if tt.wantMetadata == nil {
				require.NotContains(t, got, service.UpstreamModelMetadataExtraKey)
			} else {
				require.Equal(t, tt.wantMetadata, got[service.UpstreamModelMetadataExtraKey])
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
