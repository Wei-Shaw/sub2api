//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// dbFallbackRepoStub extends errorPolicyRepoStub with a configurable DB account
// returned by GetByID, simulating cache miss + DB fallback.
type dbFallbackRepoStub struct {
	errorPolicyRepoStub
	dbAccount *Account // returned by GetByID when non-nil
}

func (r *dbFallbackRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	if r.dbAccount != nil && r.dbAccount.ID == id {
		return r.dbAccount, nil
	}
	return nil, nil // not found, no error
}

func TestCheckErrorPolicy_401_DBFallback_Escalates(t *testing.T) {
	// Scenario: cache account has empty TempUnschedulableReason (cache miss),
	// but DB account has a previous 401 record.
	// Non-Antigravity: should escalate to ErrorPolicyNone (second 401 = permanent error).
	// Antigravity: skips escalation logic (401 handled by applyErrorPolicy rules).
	t.Run("gemini_escalates", func(t *testing.T) {
		repo := &dbFallbackRepoStub{
			dbAccount: &Account{
				ID:                      20,
				TempUnschedulableReason: `{"status_code":401,"until_unix":1735689600}`,
			},
		}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

		account := &Account{
			ID:                      20,
			Type:                    AccountTypeOAuth,
			Platform:                PlatformGemini,
			TempUnschedulableReason: "",
			Credentials: map[string]any{
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(401),
						"keywords":         []any{"unauthorized"},
						"duration_minutes": float64(10),
					},
				},
			},
		}

		result := svc.CheckErrorPolicy(context.Background(), account, http.StatusUnauthorized, []byte(`unauthorized`))
		require.Equal(t, ErrorPolicyNone, result, "gemini 401 with DB fallback showing previous 401 should escalate")
	})

	t.Run("antigravity_stays_temp", func(t *testing.T) {
		repo := &dbFallbackRepoStub{
			dbAccount: &Account{
				ID:                      20,
				TempUnschedulableReason: `{"status_code":401,"until_unix":1735689600}`,
			},
		}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

		account := &Account{
			ID:                      20,
			Type:                    AccountTypeOAuth,
			Platform:                PlatformAntigravity,
			TempUnschedulableReason: "",
			Credentials: map[string]any{
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(401),
						"keywords":         []any{"unauthorized"},
						"duration_minutes": float64(10),
					},
				},
			},
		}

		result := svc.CheckErrorPolicy(context.Background(), account, http.StatusUnauthorized, []byte(`unauthorized`))
		require.Equal(t, ErrorPolicyTempUnscheduled, result, "antigravity 401 skips escalation, stays temp-unscheduled")
	})
}

func TestCheckErrorPolicy_401_DBFallback_NoDBRecord_FirstHit(t *testing.T) {
	// Scenario: cache account has empty TempUnschedulableReason,
	// DB also has no previous 401 record → should NOT escalate (first hit → temp unscheduled).
	repo := &dbFallbackRepoStub{
		dbAccount: &Account{
			ID:                      21,
			TempUnschedulableReason: "", // DB also empty
		},
	}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	account := &Account{
		ID:                      21,
		Type:                    AccountTypeOAuth,
		Platform:                PlatformAntigravity,
		TempUnschedulableReason: "",
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(401),
					"keywords":         []any{"unauthorized"},
					"duration_minutes": float64(10),
				},
			},
		},
	}

	result := svc.CheckErrorPolicy(context.Background(), account, http.StatusUnauthorized, []byte(`unauthorized`))
	require.Equal(t, ErrorPolicyTempUnscheduled, result, "401 first hit with no DB record should temp-unschedule")
}

func TestCheckErrorPolicy_401_DBFallback_DBError_FirstHit(t *testing.T) {
	// Scenario: cache account has empty TempUnschedulableReason,
	// DB lookup returns nil (not found) → should treat as first hit → temp unscheduled.
	repo := &dbFallbackRepoStub{
		dbAccount: nil, // GetByID returns nil, nil
	}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)

	account := &Account{
		ID:                      22,
		Type:                    AccountTypeOAuth,
		Platform:                PlatformAntigravity,
		TempUnschedulableReason: "",
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{
				map[string]any{
					"error_code":       float64(401),
					"keywords":         []any{"unauthorized"},
					"duration_minutes": float64(10),
				},
			},
		},
	}

	result := svc.CheckErrorPolicy(context.Background(), account, http.StatusUnauthorized, []byte(`unauthorized`))
	require.Equal(t, ErrorPolicyTempUnscheduled, result, "401 first hit with DB not found should temp-unschedule")
}
