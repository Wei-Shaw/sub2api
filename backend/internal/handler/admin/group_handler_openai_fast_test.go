package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupRequestsDecodeForceOpenAIFast(t *testing.T) {
	var createReq CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"fast","force_openai_fast":true,"allow_openai_fast_downgrade":true,"free_openai_fast":true}`), &createReq))
	require.True(t, createReq.ForceOpenAIFast)
	require.True(t, createReq.AllowOpenAIFastDowngrade)
	require.True(t, createReq.FreeOpenAIFast)

	var updateReq UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"force_openai_fast":false,"allow_openai_fast_downgrade":false,"free_openai_fast":false}`), &updateReq))
	require.NotNil(t, updateReq.ForceOpenAIFast)
	require.False(t, *updateReq.ForceOpenAIFast)
	require.NotNil(t, updateReq.AllowOpenAIFastDowngrade)
	require.False(t, *updateReq.AllowOpenAIFastDowngrade)
	require.NotNil(t, updateReq.FreeOpenAIFast)
	require.False(t, *updateReq.FreeOpenAIFast)

	var omitted UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{}`), &omitted))
	require.Nil(t, omitted.ForceOpenAIFast)
	require.Nil(t, omitted.AllowOpenAIFastDowngrade)
	require.Nil(t, omitted.FreeOpenAIFast)
}
