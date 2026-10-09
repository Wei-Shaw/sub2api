//go:build unit

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeploymentBuildType(t *testing.T) {
	for _, tc := range []struct{ build, mode, want string }{
		{"release", "managed", "managed"},
		{"source", " MANAGED ", "managed"},
		{"managed", "", "managed"},
		{"release", "", "release"},
		{"source", "", "source"},
		{"release", "unknown", "release"},
	} {
		t.Run(tc.build+"/"+tc.mode, func(t *testing.T) {
			require.Equal(t, tc.want, deploymentBuildType(tc.build, tc.mode))
		})
	}
}
