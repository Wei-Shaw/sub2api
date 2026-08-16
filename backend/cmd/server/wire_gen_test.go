package main

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/runtime"
	"github.com/stretchr/testify/require"
)

func TestProvideServiceBuildInfo(t *testing.T) {
	in := handler.BuildInfo{
		Version:   "v-test",
		BuildType: "release",
	}
	out := provideServiceBuildInfo(in)
	require.Equal(t, in.Version, out.Version)
	require.Equal(t, in.BuildType, out.BuildType)
}

func TestProvideCleanupWithNilInfrastructure(t *testing.T) {
	cleanup := provideCleanup(nil, nil)
	require.NotPanics(t, cleanup)
}

func TestInitializeApplicationForRoleSelectsOnlyRequestedGraph(t *testing.T) {
	buildInfo := handler.BuildInfo{Version: "v-test"}
	selected := ""
	initializers := applicationInitializers{
		all:       func(handler.BuildInfo) (*Application, error) { selected = "all"; return &Application{}, nil },
		api:       func(handler.BuildInfo) (*Application, error) { selected = "api"; return &Application{}, nil },
		worker:    func(handler.BuildInfo) (*Application, error) { selected = "worker"; return &Application{}, nil },
		scheduler: func(handler.BuildInfo) (*Application, error) { selected = "scheduler"; return &Application{}, nil },
	}

	_, err := initializeApplicationForRole(runtime.RoleWorker, buildInfo, initializers)

	require.NoError(t, err)
	require.Equal(t, "worker", selected)
}

func TestInitializeApplicationForRoleRejectsOneShotRoles(t *testing.T) {
	_, err := initializeApplicationForRole(runtime.RoleMigrate, handler.BuildInfo{}, applicationInitializers{})

	require.ErrorContains(t, err, "does not have a resident application graph")
}
