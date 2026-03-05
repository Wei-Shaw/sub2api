package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveLoadFactor_NilAccount(t *testing.T) {
	var a *Account
	require.Equal(t, 1, a.EffectiveLoadFactor())
REDACTED

func TestEffectiveLoadFactor_NilLoadFactor_PositiveConcurrency(t *testing.T) {
	a := &Account{Concurrency: 5REDACTED
	require.Equal(t, 5, a.EffectiveLoadFactor())
REDACTED

func TestEffectiveLoadFactor_NilLoadFactor_ZeroConcurrency(t *testing.T) {
	a := &Account{Concurrency: 0REDACTED
	require.Equal(t, 1, a.EffectiveLoadFactor())
REDACTED

func TestEffectiveLoadFactor_PositiveLoadFactor(t *testing.T) {
	a := &Account{Concurrency: 5, LoadFactor: intPtr(20)REDACTED
	require.Equal(t, 20, a.EffectiveLoadFactor())
REDACTED

func TestEffectiveLoadFactor_ZeroLoadFactor_FallbackToConcurrency(t *testing.T) {
	a := &Account{Concurrency: 5, LoadFactor: intPtr(0)REDACTED
	require.Equal(t, 5, a.EffectiveLoadFactor())
REDACTED

func TestEffectiveLoadFactor_NegativeLoadFactor_FallbackToConcurrency(t *testing.T) {
	a := &Account{Concurrency: 3, LoadFactor: intPtr(-1)REDACTED
	require.Equal(t, 3, a.EffectiveLoadFactor())
REDACTED

func TestEffectiveLoadFactor_ZeroLoadFactor_ZeroConcurrency(t *testing.T) {
	a := &Account{Concurrency: 0, LoadFactor: intPtr(0)REDACTED
	require.Equal(t, 1, a.EffectiveLoadFactor())
REDACTED
