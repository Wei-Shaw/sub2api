package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCodexSharedDeviceConcurrencyPolicy(t *testing.T) {
	for _, limit := range []int{0, 1, 2, 5, MaxCodexSlotConcurrency} {
		require.NoError(t, validateCodexSessionPolicy(CodexSessionPolicySpec{
			Mode: CodexSessionDeviceShared, MaxActiveConversationsPerSlot: limit,
			DisableCrossKeyContinuation: true,
		}))
	}
	for _, limit := range []int{-1, MaxCodexSlotConcurrency + 1} {
		require.Error(t, validateCodexSessionPolicy(CodexSessionPolicySpec{
			Mode: CodexSessionDeviceShared, MaxActiveConversationsPerSlot: limit,
			DisableCrossKeyContinuation: true,
		}))
	}
	require.Error(t, validateCodexSessionPolicy(CodexSessionPolicySpec{Mode: CodexSessionDeviceShared}))
}
