package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsReservedEmail_DingTalkDomain(t *testing.T) {
	require.True(t, isReservedEmail("dingtalk-123@dingtalk-connect.invalid"))
	require.True(t, isReservedEmail("DINGTALK-456@DINGTALK-CONNECT.INVALID")) // case-insensitive
	require.False(t, isReservedEmail("real@dingtalk.com"))
}
