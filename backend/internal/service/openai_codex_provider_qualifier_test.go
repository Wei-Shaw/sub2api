package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// #6999: provider-qualified IDs must keep family reasoning descriptors.
func TestStripProviderQualifier(t *testing.T) {
	require.True(t, isDeepSeekCodexModel("deepseek/deepseek-v4.1-flash"))
	require.True(t, isDeepSeekCodexModel("deepseek-v4-flash"))
	require.True(t, isMimoCodexModel("xiaomi/mimo-v2.5"))
	require.True(t, isMimoCodexModel("mimo-v2.5"))
	require.False(t, isDeepSeekCodexModel("gpt-5"))
	require.False(t, isMimoCodexModel("gpt-5"))
}
