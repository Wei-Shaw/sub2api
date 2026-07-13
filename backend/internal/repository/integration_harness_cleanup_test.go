//go:build integration

package repository

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

func TestRunIntegrationCleanupsRunsInReverseAndFailsOnCleanupError(t *testing.T) {
	order := make([]string, 0, 2)
	code := runIntegrationCleanups(0, []integrationCleanup{
		{
			name: "first",
			cleanup: func() error {
				order = append(order, "first")
				return nil
			},
		},
		{
			name: "second",
			cleanup: func() error {
				order = append(order, "second")
				return errors.New("cleanup failed")
			},
		},
	})

	require.Equal(t, 1, code)
	require.Equal(t, []string{"second", "first"}, order)
	require.Equal(t, 7, runIntegrationCleanups(7, nil))
}

func TestIntegrationContainerOptionsLabelRunAndUseTmpfs(t *testing.T) {
	t.Setenv(integrationTestRunIDEnv, "issue8-run-123")
	request := &testcontainers.GenericContainerRequest{}
	for _, option := range integrationContainerOptions(postgresTestDataPath) {
		require.NoError(t, option.Customize(request))
	}

	require.Equal(t, "issue8-run-123", request.Labels[integrationTestRunLabel])
	require.Equal(t, "rw", request.Tmpfs[postgresTestDataPath])
}
