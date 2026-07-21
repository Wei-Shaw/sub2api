package securityaudit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditForceDisabled(t *testing.T) {
	manager := NewConfigManager(nil, nil, nil, nil)
	manager.SetForceDisabled(true)

	require.NoError(t, manager.Start(context.Background()))
	require.Equal(t, ModeOff, manager.EffectiveMode())
	require.False(t, manager.Public().Enabled)

	_, err := manager.Save(context.Background(), UpdateConfigRequest{ExpectedConfigVersion: 1, Enabled: true}, 1)
	require.ErrorContains(t, err, "minimal storage mode")
	require.NoError(t, manager.Shutdown(context.Background()))
}
