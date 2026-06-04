package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func groupCtx(g *Group) context.Context {
	return context.WithValue(context.Background(), ctxkey.Group, g)
}

func validGroup(profile string, conc, rpm, cooldown int) *Group {
	return &Group{
		ID: 1, Hydrated: true, Platform: PlatformAnthropic, Status: "active",
		DefaultPassthroughProfile: profile,
		DefaultAccountConcurrency: conc,
		DefaultAccountRPM:         rpm,
		Default429CooldownSec:     cooldown,
	}
}

func TestGroupFromContext(t *testing.T) {
	require.Nil(t, GroupFromContext(nil))
	require.Nil(t, GroupFromContext(context.Background()))
	// invalid group (not hydrated) is rejected
	require.Nil(t, GroupFromContext(groupCtx(&Group{ID: 1})))
	g := validGroup("", 0, 0, 0)
	require.Equal(t, g, GroupFromContext(groupCtx(g)))
}

func TestEffectiveAccountConcurrencyFromCfgCtx_Precedence(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.AccountDefaultConcurrency = 3 // system fleet default

	t.Run("group default beats fleet default", func(t *testing.T) {
		acc := &Account{Concurrency: 0}
		got := EffectiveAccountConcurrencyFromCfgCtx(groupCtx(validGroup("", 7, 0, 0)), cfg, acc)
		require.Equal(t, 7, got)
	})
	t.Run("account beats group", func(t *testing.T) {
		acc := &Account{Concurrency: 11}
		got := EffectiveAccountConcurrencyFromCfgCtx(groupCtx(validGroup("", 7, 0, 0)), cfg, acc)
		require.Equal(t, 11, got)
	})
	t.Run("no group → fleet default", func(t *testing.T) {
		acc := &Account{Concurrency: 0}
		got := EffectiveAccountConcurrencyFromCfgCtx(context.Background(), cfg, acc)
		require.Equal(t, 3, got)
	})
	t.Run("group=0 → falls through to fleet", func(t *testing.T) {
		acc := &Account{Concurrency: 0}
		got := EffectiveAccountConcurrencyFromCfgCtx(groupCtx(validGroup("", 0, 0, 0)), cfg, acc)
		require.Equal(t, 3, got)
	})
}

func TestEffectiveConcurrencyAndRPMWithGroup_Precedence(t *testing.T) {
	// account > group > system, zero = "not set at this level"
	require.Equal(t, 5, (&Account{Concurrency: 5}).EffectiveConcurrencyWithGroup(9, 3)) // account wins
	require.Equal(t, 9, (&Account{Concurrency: 0}).EffectiveConcurrencyWithGroup(9, 3)) // group wins
	require.Equal(t, 3, (&Account{Concurrency: 0}).EffectiveConcurrencyWithGroup(0, 3)) // system

	rpmAcc := &Account{Extra: map[string]any{"base_rpm": 50}}
	require.Equal(t, 50, rpmAcc.EffectiveBaseRPMWithGroup(20, 10)) // account wins
	require.Equal(t, 20, (&Account{}).EffectiveBaseRPMWithGroup(20, 10))
	require.Equal(t, 10, (&Account{}).EffectiveBaseRPMWithGroup(0, 10))
}

func TestAPIKeyAuthGroupSnapshot_RoundTripPreservesM2Fields(t *testing.T) {
	snap := APIKeyAuthGroupSnapshot{
		ID: 1, Name: "g", Platform: PlatformAnthropic, Status: "active",
		DefaultPassthroughProfile: "transparent",
		DefaultAccountConcurrency: 7,
		DefaultAccountRPM:         33,
		Default429CooldownSec:     45,
	}
	b, err := json.Marshal(snap)
	require.NoError(t, err)

	var got APIKeyAuthGroupSnapshot
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "transparent", got.DefaultPassthroughProfile)
	require.Equal(t, 7, got.DefaultAccountConcurrency)
	require.Equal(t, 33, got.DefaultAccountRPM)
	require.Equal(t, 45, got.Default429CooldownSec)

	// Backward compat: old cache entries lack these keys → zero values (fall through to system).
	var legacy APIKeyAuthGroupSnapshot
	require.NoError(t, json.Unmarshal([]byte(`{"id":1,"name":"g","status":"active"}`), &legacy))
	require.Equal(t, "", legacy.DefaultPassthroughProfile)
	require.Equal(t, 0, legacy.DefaultAccountConcurrency)
	require.Equal(t, 0, legacy.DefaultAccountRPM)
	require.Equal(t, 0, legacy.Default429CooldownSec)
}
