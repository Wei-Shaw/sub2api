package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpdateAccountKeepsGrokCatalogManagedByDatabase(t *testing.T) {
	freshModels := map[string]any{
		"models":     []string{"fresh-model"},
		"fetched_at": "2026-10-25T12:00:00Z",
	}
	freshMetadata := map[string]any{
		"models": map[string]any{"fresh-model": map[string]any{"id": "fresh-model"}},
	}
	failedSnapshot := map[string]any{
		"models":     []string{"fresh-model"},
		"fetched_at": "2026-10-25T12:00:00Z",
		"last_attempt": map[string]any{
			"started_at":  "2026-10-25T13:00:00Z",
			"finished_at": "2026-10-25T13:00:01Z",
			"status":      "failed",
			"error_code":  "transport",
		},
	}
	oldModels := map[string]any{
		"models":     []string{"old-model"},
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
		inputExtra   map[string]any
		inputCreds   map[string]any
		wantModels   any
		wantMetadata any
		wantBaseURL  string
	}{
		{
			name:        "oauth stale form does not replace fresh snapshot or metadata",
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-keep"},
			storedExtra: map[string]any{
				grokObservedModelsExtraKey:    freshModels,
				UpstreamModelMetadataExtraKey: freshMetadata,
			},
			inputExtra: map[string]any{
				"grok_client_tool_cache_enabled": true,
				grokObservedModelsExtraKey:       oldModels,
				UpstreamModelMetadataExtraKey:    oldMetadata,
			},
			wantModels:   freshModels,
			wantMetadata: freshMetadata,
		},
		{
			name:        "apikey form opened without keys does not delete them",
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"api_key": "sk-keep", "base_url": "https://api.example.test/v1"},
			storedExtra: map[string]any{
				grokObservedModelsExtraKey:    freshModels,
				UpstreamModelMetadataExtraKey: freshMetadata,
			},
			inputExtra: map[string]any{
				"grok_client_tool_cache_enabled": true,
			},
			wantModels:   freshModels,
			wantMetadata: freshMetadata,
		},
		{
			name:        "injected catalog is ignored when the account has none",
			accountType: AccountTypeOAuth,
			credentials: map[string]any{"access_token": "token-keep"},
			storedExtra: map[string]any{"note": "keep"},
			inputExtra: map[string]any{
				"note":                        "from-form",
				grokObservedModelsExtraKey:    oldModels,
				UpstreamModelMetadataExtraKey: oldMetadata,
			},
		},
		{
			name:        "failed attempt is not rolled back to an older success",
			accountType: AccountTypeAPIKey,
			credentials: map[string]any{"api_key": "sk-keep"},
			storedExtra: map[string]any{
				grokObservedModelsExtraKey:    failedSnapshot,
				UpstreamModelMetadataExtraKey: freshMetadata,
			},
			inputExtra: map[string]any{
				grokObservedModelsExtraKey:    oldModels,
				UpstreamModelMetadataExtraKey: oldMetadata,
			},
			wantModels:   failedSnapshot,
			wantMetadata: freshMetadata,
		},
		{
			name:        "changing the saved endpoint keeps the old success",
			accountType: AccountTypeOAuth,
			credentials: map[string]any{
				"access_token": "token-keep",
				"base_url":     "https://old.example.test/v1",
			},
			storedExtra: map[string]any{
				grokObservedModelsExtraKey:    freshModels,
				UpstreamModelMetadataExtraKey: freshMetadata,
			},
			inputExtra: map[string]any{
				grokObservedModelsExtraKey: nil,
			},
			inputCreds: map[string]any{
				"base_url": "https://new.example.test/v1",
			},
			wantModels:   freshModels,
			wantMetadata: freshMetadata,
			wantBaseURL:  "https://new.example.test/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const accountID = int64(25)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
				accountID: {
					ID:          accountID,
					Name:        "grok",
					Platform:    PlatformGrok,
					Type:        tt.accountType,
					Status:      StatusActive,
					Credentials: tt.credentials,
					Extra:       tt.storedExtra,
				},
			}}

			got, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
				Extra:       tt.inputExtra,
				Credentials: tt.inputCreds,
			})
			require.NoError(t, err)
			if tt.wantModels == nil {
				require.NotContains(t, got.Extra, grokObservedModelsExtraKey)
			} else {
				require.Equal(t, tt.wantModels, got.Extra[grokObservedModelsExtraKey])
			}
			if tt.wantMetadata == nil {
				require.NotContains(t, got.Extra, UpstreamModelMetadataExtraKey)
			} else {
				require.Equal(t, tt.wantMetadata, got.Extra[UpstreamModelMetadataExtraKey])
			}
			if _, ok := tt.inputExtra["grok_client_tool_cache_enabled"]; ok {
				require.Equal(t, true, got.Extra["grok_client_tool_cache_enabled"])
			}
			if tt.wantBaseURL != "" {
				require.Equal(t, tt.wantBaseURL, got.GetCredential("base_url"))
				require.Equal(t, "token-keep", got.GetCredential("access_token"))
				oldScope, scopeErr := NewGrokCatalogScope(accountID, tt.accountType, "https://old.example.test/v1/models")
				require.NoError(t, scopeErr)
				current, scopeErr := NewGrokCatalogScope(accountID, tt.accountType, "https://new.example.test/v1/models")
				require.NoError(t, scopeErr)
				snapshot := parseGrokObservedModels(got.Extra)
				require.NotNil(t, snapshot)
				snapshot.Scope = oldScope
				diag := EvaluateGrokModelDiagnostics(got, current, snapshot, time.Date(2026, 10, 25, 18, 0, 0, 0, time.UTC))
				require.Equal(t, "source_changed", diag.State)
				require.Equal(t, []string{"fresh-model"}, diag.Models)
			}
		})
	}
}
