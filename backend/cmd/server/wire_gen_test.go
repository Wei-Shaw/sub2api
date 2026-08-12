package main

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
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
