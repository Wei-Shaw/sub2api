package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/stretchr/testify/require"
)

func TestAdmitResidentRoleRequiresBootstrapForSplitRoles(t *testing.T) {
	for _, role := range []runtime.Role{runtime.RoleAPI, runtime.RoleWorker, runtime.RoleScheduler} {
		t.Run(string(role), func(t *testing.T) {
			err := admitResidentRole(role, func(context.Context) error { return errors.New("bootstrap has not completed") })
			require.ErrorContains(t, err, "bootstrap has not completed")
		})
	}
}

func TestAdmitResidentRolePreservesAllStandaloneInitialization(t *testing.T) {
	called := false
	err := admitResidentRole(runtime.RoleAll, func(context.Context) error {
		called = true
		return errors.New("should not be called")
	})
	require.NoError(t, err)
	require.False(t, called)
}
