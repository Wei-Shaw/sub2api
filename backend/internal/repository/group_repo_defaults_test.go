package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestNormalizeGroupFirstOutputFailoverDefaults(t *testing.T) {
	t.Run("fills omitted values", func(t *testing.T) {
		group := &service.Group{}

		normalizeGroupFirstOutputFailoverDefaults(group)

		require.Equal(t, 6, group.FirstOutputFailoverTimeoutSeconds)
		require.Equal(t, 3, group.FirstOutputFailoverMaxSwitches)
	})

	t.Run("preserves configured values", func(t *testing.T) {
		group := &service.Group{
			FirstOutputFailoverTimeoutSeconds: 12,
			FirstOutputFailoverMaxSwitches:    5,
		}

		normalizeGroupFirstOutputFailoverDefaults(group)

		require.Equal(t, 12, group.FirstOutputFailoverTimeoutSeconds)
		require.Equal(t, 5, group.FirstOutputFailoverMaxSwitches)
	})
}
