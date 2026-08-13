package main

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/stretchr/testify/require"
)

func TestNewRoleLifecycleStartsOnlyOwnedComponents(t *testing.T) {
	started := []string{}
	component := func(name string, role runtime.Role) runtime.Component {
		return runtime.Component{
			Name:  name,
			Roles: []runtime.Role{role},
			Start: func(context.Context) error { started = append(started, name); return nil },
			Stop:  func(context.Context) error { return nil },
		}
	}

	lifecycle, err := newRoleLifecycle(runtime.RoleWorker, []runtime.Component{
		component("worker", runtime.RoleWorker),
	})
	require.NoError(t, err)
	require.NoError(t, lifecycle.Start(context.Background(), runtime.RoleWorker))
	require.Equal(t, []string{"worker"}, started)
}
