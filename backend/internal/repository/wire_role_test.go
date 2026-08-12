package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/stretchr/testify/require"
)

func TestEntInitOptionsByRole(t *testing.T) {
	require.Equal(t, EntInitOptions{RunMigrations: true, Bootstrap: true}, entInitOptionsForRole(runtime.RoleAll))
	for _, role := range []runtime.Role{runtime.RoleAPI, runtime.RoleWorker, runtime.RoleScheduler} {
		require.Equal(t, EntInitOptions{}, entInitOptionsForRole(role))
	}
}
