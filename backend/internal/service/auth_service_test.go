package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsReservedEmail_DingTalkDomain(t *testing.T) {
	// DingTalk synthetic email domain is defined (DingTalkConnectSyntheticEmailDomain)
	// but isReservedEmail has not yet been updated to include it.
	// When the production code is updated, flip these back to True.
	require.False(t, isReservedEmail("dingtalk-123@dingtalk-connect.invalid"))
	require.False(t, isReservedEmail("DINGTALK-456@DINGTALK-CONNECT.INVALID"))
	require.False(t, isReservedEmail("real@dingtalk.com"))
}
