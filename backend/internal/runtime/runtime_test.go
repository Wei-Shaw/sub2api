package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		cliSet    bool
		cliValue  string
		envValue  string
		envSet    bool
		want      Role
		wantError string
	}{
		{name: "defaults to all", want: RoleAll},
		{name: "uses environment role", envSet: true, envValue: "worker", want: RoleWorker},
		{name: "uses command line role", cliSet: true, cliValue: "api", want: RoleAPI},
		{name: "accepts matching explicit values", cliSet: true, cliValue: "scheduler", envSet: true, envValue: "scheduler", want: RoleScheduler},
		{name: "command line wins over absent environment", cliSet: true, cliValue: "migrate", want: RoleMigrate},
		{name: "rejects unknown command line role", cliSet: true, cliValue: "gateway", wantError: "unknown runtime role"},
		{name: "rejects unknown environment role", envSet: true, envValue: "gateway", wantError: "unknown runtime role"},
		{name: "rejects blank explicit command line role", cliSet: true, wantError: "runtime role cannot be empty"},
		{name: "rejects blank explicit environment role", envSet: true, wantError: "runtime role cannot be empty"},
		{name: "rejects conflicting explicit values", cliSet: true, cliValue: "api", envSet: true, envValue: "worker", wantError: "conflicting runtime roles"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookup := func(string) (string, bool) {
				return test.envValue, test.envSet
			}

			got, err := ResolveRole(test.cliSet, test.cliValue, lookup)
			if test.wantError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestSupportedRolesAreExact(t *testing.T) {
	t.Parallel()

	require.Equal(t, []Role{RoleAll, RoleAPI, RoleWorker, RoleScheduler, RoleMigrate, RoleBootstrap}, SupportedRoles())
}

func TestLifecycleStartsOnlyRoleOwnedComponentsAndStopsInReverseOrder(t *testing.T) {
	t.Parallel()

	var events []string
	lifecycle, err := NewLifecycle([]Component{
		recordingComponent("api", []Role{RoleAPI}, &events),
		recordingComponent("shared", []Role{RoleAPI, RoleWorker}, &events),
		recordingComponent("worker", []Role{RoleWorker}, &events),
	})
	require.NoError(t, err)

	require.NoError(t, lifecycle.Start(context.Background(), RoleAPI))
	require.NoError(t, lifecycle.Stop(context.Background()))

	require.Equal(t, []string{"start:api", "start:shared", "stop:shared", "stop:api"}, events)
}

func TestLifecycleAllStartsCompleteManifest(t *testing.T) {
	t.Parallel()

	var events []string
	lifecycle, err := NewLifecycle([]Component{
		recordingComponent("api", []Role{RoleAPI}, &events),
		recordingComponent("worker", []Role{RoleWorker}, &events),
		recordingComponent("scheduler", []Role{RoleScheduler}, &events),
	})
	require.NoError(t, err)

	require.NoError(t, lifecycle.Start(context.Background(), RoleAll))
	require.NoError(t, lifecycle.Stop(context.Background()))

	require.Equal(t, []string{"start:api", "start:worker", "start:scheduler", "stop:scheduler", "stop:worker", "stop:api"}, events)
}

func TestLifecycleRollsBackOnlyStartedComponentsAfterStartFailure(t *testing.T) {
	t.Parallel()

	var events []string
	failure := errors.New("cannot start")
	lifecycle, err := NewLifecycle([]Component{
		recordingComponent("first", []Role{RoleWorker}, &events),
		{
			Name:  "failing",
			Roles: []Role{RoleWorker},
			Start: func(context.Context) error {
				events = append(events, "start:failing")
				return failure
			},
			Stop: func(context.Context) error {
				events = append(events, "stop:failing")
				return nil
			},
		},
		recordingComponent("never-started", []Role{RoleWorker}, &events),
	})
	require.NoError(t, err)

	err = lifecycle.Start(context.Background(), RoleWorker)
	require.ErrorIs(t, err, failure)
	require.Equal(t, []string{"start:first", "start:failing", "stop:first"}, events)

	require.NoError(t, lifecycle.Stop(context.Background()))
	require.Equal(t, []string{"start:first", "start:failing", "stop:first"}, events)
}

func TestNewLifecycleRejectsInvalidManifest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		manifest []Component
	}{
		{name: "empty name", manifest: []Component{{Roles: []Role{RoleAPI}, Start: func(context.Context) error { return nil }, Stop: func(context.Context) error { return nil }}}},
		{name: "duplicate name", manifest: []Component{recordingComponent("same", []Role{RoleAPI}, new([]string)), recordingComponent("same", []Role{RoleWorker}, new([]string))}},
		{name: "no role", manifest: []Component{recordingComponent("missing-role", nil, new([]string))}},
		{name: "job role is not resident component role", manifest: []Component{recordingComponent("migrate", []Role{RoleMigrate}, new([]string))}},
		{name: "missing start", manifest: []Component{{Name: "missing-start", Roles: []Role{RoleAPI}, Stop: func(context.Context) error { return nil }}}},
		{name: "missing stop", manifest: []Component{{Name: "missing-stop", Roles: []Role{RoleAPI}, Start: func(context.Context) error { return nil }}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewLifecycle(test.manifest)
			require.Error(t, err)
		})
	}
}

func recordingComponent(name string, roles []Role, events *[]string) Component {
	return Component{
		Name:  name,
		Roles: roles,
		Start: func(context.Context) error {
			*events = append(*events, "start:"+name)
			return nil
		},
		Stop: func(context.Context) error {
			*events = append(*events, "stop:"+name)
			return nil
		},
	}
}

func TestRoleIsValid(t *testing.T) {
	t.Parallel()

	for _, role := range SupportedRoles() {
		require.True(t, role.Valid())
	}
	require.False(t, Role("invalid").Valid())
	require.False(t, Role("").Valid())
}
