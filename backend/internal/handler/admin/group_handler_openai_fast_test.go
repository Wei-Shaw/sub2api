package admin

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupRequestsDecodeForceOpenAIFast(t *testing.T) {
	var createReq CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"fast","forced_service_tier":"priority","free_openai_fast":true}`), &createReq))
	require.Equal(t, service.OpenAIFastTierPriority, createReq.ForcedServiceTier)
	require.True(t, createReq.FreeOpenAIFast)

	var updateReq UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"forced_service_tier":"","free_openai_fast":false}`), &updateReq))
	require.NotNil(t, updateReq.ForcedServiceTier)
	require.Empty(t, *updateReq.ForcedServiceTier)
	require.NotNil(t, updateReq.FreeOpenAIFast)
	require.False(t, *updateReq.FreeOpenAIFast)

	var legacy CreateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"fast","force_openai_fast":true}`), &legacy))
	require.NotNil(t, legacy.ForceOpenAIFast)
	require.True(t, *legacy.ForceOpenAIFast)
	require.Equal(t, service.OpenAIFastTierPriority, resolveCreateForcedServiceTier(legacy.ForcedServiceTier, legacy.ForceOpenAIFast))
	require.Equal(t, service.OpenAIFastTierDefault, resolveCreateForcedServiceTier(service.OpenAIFastTierDefault, legacy.ForceOpenAIFast))

	var legacyUpdate UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"force_openai_fast":false}`), &legacyUpdate))
	resolved := resolveUpdateForcedServiceTier(legacyUpdate.ForcedServiceTier, legacyUpdate.ForceOpenAIFast)
	require.NotNil(t, resolved)
	require.Empty(t, *resolved)
	require.Nil(t, resolveUpdateForcedServiceTier(nil, nil))
}
