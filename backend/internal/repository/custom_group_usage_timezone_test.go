package repository

import (
	"testing"

	appTimezone "github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

func useGroupUsageRepositoryTestTimezone(t *testing.T, name string) {
	t.Helper()

	previousName := appTimezone.Name()
	require.NoError(t, appTimezone.Init(name))
	t.Cleanup(func() { require.NoError(t, appTimezone.Init(previousName)) })
}
