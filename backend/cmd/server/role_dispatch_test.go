package main

import (
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/stretchr/testify/require"
)

func TestDispatchRoleRejectsSetupOutsideAll(t *testing.T) {
	err := dispatchRole(runtime.RoleWorker, true, roleLaunchers{})
	require.ErrorContains(t, err, "--setup requires runtime role")
}

func TestDispatchRoleAllUsesLegacySetupOnly(t *testing.T) {
	var events []string
	launchers := roleLaunchers{
		needsSetup:       func() bool { events = append(events, "needs-setup"); return true },
		autoSetupEnabled: func() bool { events = append(events, "auto-setup-enabled"); return false },
		runSetupServer:   func() { events = append(events, "setup-server") },
		runResident:      func(runtime.Role) error { events = append(events, "resident"); return nil },
	}

	require.NoError(t, dispatchRole(runtime.RoleAll, false, launchers))
	require.Equal(t, []string{"needs-setup", "auto-setup-enabled", "setup-server"}, events)
}

func TestDispatchRoleAllRunsResidentAfterAutoSetup(t *testing.T) {
	var events []string
	launchers := roleLaunchers{
		needsSetup:       func() bool { events = append(events, "needs-setup"); return true },
		autoSetupEnabled: func() bool { events = append(events, "auto-setup-enabled"); return true },
		runAutoSetup:     func() error { events = append(events, "auto-setup"); return nil },
		runResident:      func(runtime.Role) error { events = append(events, "resident"); return nil },
	}

	require.NoError(t, dispatchRole(runtime.RoleAll, false, launchers))
	require.Equal(t, []string{"needs-setup", "auto-setup-enabled", "auto-setup", "resident"}, events)
}

func TestDispatchRoleResidentSkipsLegacySetup(t *testing.T) {
	for _, role := range []runtime.Role{runtime.RoleAPI, runtime.RoleWorker, runtime.RoleScheduler} {
		t.Run(string(role), func(t *testing.T) {
			var started runtime.Role
			require.NoError(t, dispatchRole(role, false, roleLaunchers{
				runResident: func(got runtime.Role) error { started = got; return nil },
			}))
			require.Equal(t, role, started)
		})
	}
}

func TestDispatchRoleOneShotDoesNotEnterResidentServing(t *testing.T) {
	for _, role := range []runtime.Role{runtime.RoleMigrate, runtime.RoleBootstrap} {
		t.Run(string(role), func(t *testing.T) {
			var events []string
			launchers := roleLaunchers{
				runResident:  func(runtime.Role) error { events = append(events, "resident"); return nil },
				runMigrate:   func() error { events = append(events, "migrate"); return nil },
				runBootstrap: func() error { events = append(events, "bootstrap"); return nil },
			}
			require.NoError(t, dispatchRole(role, false, launchers))
			require.Equal(t, []string{string(role)}, events)
		})
	}
}

func TestDispatchRoleReturnsLauncherErrors(t *testing.T) {
	want := errors.New("migration failed")
	err := dispatchRole(runtime.RoleMigrate, false, roleLaunchers{runMigrate: func() error { return want }})
	require.ErrorIs(t, err, want)
}
