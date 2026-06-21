package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestTypeCyberBlocked(t *testing.T) {
	require.True(t, RequestTypeCyberBlocked.IsValid())
	require.Equal(t, "cyber", RequestTypeCyberBlocked.String())

	rt, err := ParseUsageRequestType("cyber")
	require.NoError(t, err)
	require.Equal(t, RequestTypeCyberBlocked, rt)

	// 显式 cyber 被 EffectiveRequestType 保留（不被 legacy 推导覆盖）
	u := &UsageLog{RequestType: RequestTypeCyberBlocked, Stream: true}
	require.Equal(t, RequestTypeCyberBlocked, u.EffectiveRequestType())

	// Sync 保留 cyber 且不覆盖真实 stream
	u.SyncRequestTypeAndLegacyFields()
	require.Equal(t, RequestTypeCyberBlocked, u.RequestType)
	require.True(t, u.Stream, "cyber 不应覆盖真实 stream 字段")
}
