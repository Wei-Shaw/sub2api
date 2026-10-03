//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type tempUnschedScopeRepoStub struct {
	modelNotFoundAccountRepoStub
	tempReasons []string
}

func (r *tempUnschedScopeRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, reason string) error {
	r.tempCalls++
	r.tempReasons = append(r.tempReasons, reason)
	return nil
}

func newTempUnschedScopeAccount(scope string, extra map[string]any) *Account {
	rule := map[string]any{
		"error_code":       float64(http.StatusServiceUnavailable),
		"keywords":         []any{"unavailable"},
		"duration_minutes": float64(10),
	}
	if scope != "" {
		rule["scope"] = scope
	}
	for k, v := range extra {
		rule[k] = v
	}
	return &Account{
		ID:          202,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules":   []any{rule},
		},
	}
}

var tempUnschedScopeBody = []byte(`{"error":{"message":"upstream unavailable"}}`)

func TestGetTempUnschedulableRules_ParsesScope(t *testing.T) {
	mk := func(keyword string, scope any) map[string]any {
		rule := map[string]any{
			"error_code":       float64(http.StatusServiceUnavailable),
			"keywords":         []any{keyword},
			"duration_minutes": float64(1),
		}
		if scope != nil {
			rule["scope"] = scope
		}
		return rule
	}
	account := &Account{Credentials: map[string]any{
		"temp_unschedulable_rules": []any{
			mk("a", "account"),
			mk("b", " Model "),
			mk("c", "whatever"),
			mk("d", nil),
		},
	}}

	rules := account.GetTempUnschedulableRules()

	require.Len(t, rules, 4)
	require.Equal(t, TempUnschedScopeAccount, rules[0].Scope)
	require.Equal(t, TempUnschedScopeModel, rules[1].Scope, "大小写与首尾空白应被归一化")
	require.Empty(t, rules[2].Scope, "未知取值视为未设置")
	require.Empty(t, rules[3].Scope)
}

func TestTempUnschedRuleUsesModelScope(t *testing.T) {
	cases := []struct {
		name   string
		scope  string
		model  string
		status int
		want   bool
	}{
		{"缺省 模型已知 非401 走模型级", "", "gpt-5.4", http.StatusServiceUnavailable, true},
		{"缺省 401 走账号级", "", "gpt-5.4", http.StatusUnauthorized, false},
		{"缺省 模型未知 走账号级", "", "", http.StatusServiceUnavailable, false},
		{"显式账号级 模型已知 仍账号级", TempUnschedScopeAccount, "gpt-5.4", http.StatusServiceUnavailable, false},
		{"显式模型级 401 仍模型级", TempUnschedScopeModel, "gpt-5.4", http.StatusUnauthorized, true},
		{"显式模型级 模型未知 兜底账号级", TempUnschedScopeModel, "  ", http.StatusServiceUnavailable, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tempUnschedRuleUsesModelScope(TempUnschedulableRule{Scope: tc.scope}, tc.model, tc.status)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestHandleTempUnschedulable_ExplicitAccountScopePausesWholeAccountWithKnownModel(t *testing.T) {
	repo := &tempUnschedScopeRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := newTempUnschedScopeAccount(TempUnschedScopeAccount, nil)

	handled := svc.HandleTempUnschedulable(context.Background(), account, http.StatusServiceUnavailable, tempUnschedScopeBody, "gpt-5.4")

	require.True(t, handled)
	require.Equal(t, 1, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.tempReasons[0]), &state))
	require.Equal(t, TempUnschedScopeAccount, state.Scope)
	require.Empty(t, state.Model)
}

func TestCheckErrorPolicy_ExplicitModelScopeKeeps401ModelScoped(t *testing.T) {
	repo := &tempUnschedScopeRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := newTempUnschedScopeAccount(TempUnschedScopeModel, map[string]any{
		"error_code": float64(http.StatusUnauthorized),
		"keywords":   []any{"unauthorized"},
	})

	result := svc.CheckErrorPolicy(context.Background(), account, http.StatusUnauthorized,
		[]byte(`{"error":{"message":"unauthorized"}}`), "gpt-5.4")

	require.Equal(t, ErrorPolicyTempUnscheduled, result)
	require.Zero(t, repo.tempCalls, "运营人员显式选了模型级，401 也不应连累整个账号")
	require.Len(t, repo.modelRateLimitCalls, 1)
	require.Equal(t, "gpt-5.4", repo.modelRateLimitCalls[0].scope)

	var state TempUnschedState
	require.NoError(t, json.Unmarshal([]byte(repo.modelRateLimitCalls[0].reason), &state))
	require.Equal(t, TempUnschedScopeModel, state.Scope)
	require.Equal(t, "gpt-5.4", state.Model)
	require.Equal(t, http.StatusUnauthorized, state.StatusCode)
}

func TestHandleTempUnschedulable_ExplicitModelScopeFallsBackToAccountWhenModelUnknown(t *testing.T) {
	repo := &tempUnschedScopeRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	account := newTempUnschedScopeAccount(TempUnschedScopeModel, nil)

	handled := svc.HandleTempUnschedulable(context.Background(), account, http.StatusServiceUnavailable, tempUnschedScopeBody)

	require.True(t, handled)
	require.Equal(t, 1, repo.tempCalls)
	require.Empty(t, repo.modelRateLimitCalls)
}
