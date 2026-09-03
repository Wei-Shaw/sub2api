package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountRuntimeSchedulingProtection(t *testing.T) {
	future := time.Now().Add(time.Hour)
	account := &Account{
		Status:                 StatusError,
		Schedulable:            true,
		TempUnschedulableUntil: &future,
		Credentials: map[string]any{
			"disable_runtime_error_handling": true,
		},
	}
	require.True(t, account.IsSchedulable())

	account.Schedulable = false
	require.True(t, account.IsSchedulable())

	account.Status = StatusActive
	require.False(t, account.IsSchedulable())

	account.Schedulable = true
	account.Status = StatusDisabled
	require.False(t, account.IsSchedulable())
}

func TestAccountTempUnschedulableDisabled(t *testing.T) {
	account := &Account{Credentials: map[string]any{"disable_temp_unschedulable": true}}
	require.True(t, account.IsTempUnschedulableDisabled())
	require.False(t, account.IsRuntimeSchedulingProtectionDisabled())
}
