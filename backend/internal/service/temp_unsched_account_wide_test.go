//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tempUnschedAccountWideRepoStub embeds accountRepoStub (which panics on any
// unexpected call) and records which persistence path triggerTempUnschedulable
// took: model-scoped SetModelRateLimit or account-wide SetTempUnschedulable.
type tempUnschedAccountWideRepoStub struct {
	accountRepoStub
	modelRateLimitScopes map[int64][]string
	tempUnschedUntil     map[int64]time.Time
	tempUnschedReasons   map[int64]string
}

func newTempUnschedAccountWideRepoStub() *tempUnschedAccountWideRepoStub {
	return &tempUnschedAccountWideRepoStub{
		modelRateLimitScopes: make(map[int64][]string),
		tempUnschedUntil:     make(map[int64]time.Time),
		tempUnschedReasons:   make(map[int64]string),
	}
}

func (s *tempUnschedAccountWideRepoStub) SetModelRateLimit(_ context.Context, id int64, scope string, _ time.Time, _ ...string) error {
	s.modelRateLimitScopes[id] = append(s.modelRateLimitScopes[id], scope)
	return nil
}

func (s *tempUnschedAccountWideRepoStub) SetTempUnschedulable(_ context.Context, id int64, until time.Time, reason string) error {
	if existing, ok := s.tempUnschedUntil[id]; !ok || until.After(existing) {
		s.tempUnschedUntil[id] = until
	}
	s.tempUnschedReasons[id] = reason
	return nil
}

func accountWideTestService(repo *tempUnschedAccountWideRepoStub) *RateLimitService {
	s := &RateLimitService{}
	s.accountRepo = repo
	return s
}

func accountWideTestAccount(rules []any) *Account {
	return &Account{
		ID:     42,
		Status: StatusActive,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules":   rules,
		},
	}
}

func TestGetTempUnschedulableRulesParsesAccountWide(t *testing.T) {
	account := accountWideTestAccount([]any{
		map[string]any{
			"error_code":       float64(402),
			"keywords":         []any{"payment required"},
			"duration_minutes": float64(43200),
			"account_wide":     true,
		},
		map[string]any{
			"error_code":       float64(429),
			"keywords":         []any{"overloaded"},
			"duration_minutes": float64(5),
			// account_wide omitted → must default to false
		},
		map[string]any{
			"error_code":       float64(500),
			"keywords":         []any{"internal"},
			"duration_minutes": float64(10),
			"account_wide":     "true", // wrong type → must stay false
		},
	})

	rules := account.GetTempUnschedulableRules()
	require.Len(t, rules, 3)
	require.True(t, rules[0].AccountWide, "explicit account_wide=true must parse")
	require.False(t, rules[1].AccountWide, "omitted account_wide must default to false")
	require.False(t, rules[2].AccountWide, "non-bool account_wide must parse as false")
}

func TestTriggerTempUnschedulableAccountWideBlocksWholeAccount(t *testing.T) {
	repo := newTempUnschedAccountWideRepoStub()
	s := accountWideTestService(repo)
	account := accountWideTestAccount([]any{
		map[string]any{
			"error_code":       float64(402),
			"keywords":         []any{"payment"},
			"duration_minutes": float64(43200),
			"account_wide":     true,
		},
	})
	body := []byte(`{"error":{"type":"upstream_error","message":"402 payment required"}}`)

	// 402 on a known model must NOT take the model-scoped path when the rule is
	// account_wide: providers with account-shared quota drain every model at once.
	handled := s.HandleTempUnschedulable(context.Background(), account, 402, body, "zai-org/GLM-5.3-Flash-BF16")
	require.True(t, handled, "rule should match and fail over")
	require.Empty(t, repo.modelRateLimitScopes, "account-wide rule must not write model rate limits")
	require.Contains(t, repo.tempUnschedUntil, int64(42), "account-wide rule must block the account")

	// The persisted reason must record account_wide so the status modal can
	// distinguish rule-triggered account blocks from 401 fallback blocks.
	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.tempUnschedReasons[42]), &state))
	require.True(t, state.AccountWide, "persisted reason must record account_wide=true")
}

func TestTriggerTempUnschedulableDefaultStaysModelScoped(t *testing.T) {
	repo := newTempUnschedAccountWideRepoStub()
	s := accountWideTestService(repo)
	account := accountWideTestAccount([]any{
		map[string]any{
			"error_code":       float64(402),
			"keywords":         []any{"payment"},
			"duration_minutes": float64(43200),
			// no account_wide: legacy behavior
		},
	})
	body := []byte(`{"error":{"type":"upstream_error","message":"402 payment required"}}`)

	handled := s.HandleTempUnschedulable(context.Background(), account, 402, body, "zai-org/GLM-5.3-Flash-BF16")
	require.True(t, handled, "rule should match and fail over")
	require.Equal(t, map[int64][]string{42: {"zai-org/GLM-5.3-Flash-BF16"}}, repo.modelRateLimitScopes,
		"default rule must keep the (account, model) scoped behavior")
	require.Empty(t, repo.tempUnschedUntil, "default rule must not block the whole account")
}

func TestTempUnschedRuleAccountWideFieldDefaults(t *testing.T) {
	// Struct-level check: account_wide must serialize with omitempty when
	// false and parse back cleanly, so credentials written by older
	// frontends stay forward/backward compatible.
	rule := TempUnschedulableRule{
		ErrorCode:       402,
		Keywords:        []string{"payment"},
		DurationMinutes: 43200,
		Description:     "shared quota",
	}
	data, err := json.Marshal(rule)
	require.NoError(t, err)
	require.NotContains(t, string(data), "account_wide", "false must be omitted from wire format")

	rule.AccountWide = true
	data, err = json.Marshal(rule)
	require.NoError(t, err)
	require.Contains(t, string(data), `"account_wide":true`)

	var decoded TempUnschedulableRule
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.True(t, decoded.AccountWide)
}
