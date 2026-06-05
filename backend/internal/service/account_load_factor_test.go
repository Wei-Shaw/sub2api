//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func intPtrHelper(v int) *int { return &v }

func TestEffectiveLoadFactor_NilAccount(t *testing.T) {
	var a *Account
	require.Equal(t, 1, a.EffectiveLoadFactor())
}

func TestEffectiveLoadFactor_NilLoadFactor_PositiveConcurrency(t *testing.T) {
	a := &Account{Concurrency: 5}
	require.Equal(t, 5, a.EffectiveLoadFactor())
}

func TestEffectiveLoadFactor_NilLoadFactor_ZeroConcurrency(t *testing.T) {
	a := &Account{Concurrency: 0}
	require.Equal(t, 1, a.EffectiveLoadFactor())
}

func TestEffectiveLoadFactor_PositiveLoadFactor(t *testing.T) {
	a := &Account{Concurrency: 5, LoadFactor: intPtrHelper(20)}
	require.Equal(t, 20, a.EffectiveLoadFactor())
}

func TestEffectiveLoadFactor_ZeroLoadFactor_FallbackToConcurrency(t *testing.T) {
	a := &Account{Concurrency: 5, LoadFactor: intPtrHelper(0)}
	require.Equal(t, 5, a.EffectiveLoadFactor())
}

func TestEffectiveLoadFactor_NegativeLoadFactor_FallbackToConcurrency(t *testing.T) {
	a := &Account{Concurrency: 3, LoadFactor: intPtrHelper(-1)}
	require.Equal(t, 3, a.EffectiveLoadFactor())
}

func TestEffectiveLoadFactor_ZeroLoadFactor_ZeroConcurrency(t *testing.T) {
	a := &Account{Concurrency: 0, LoadFactor: intPtrHelper(0)}
	require.Equal(t, 1, a.EffectiveLoadFactor())
}
