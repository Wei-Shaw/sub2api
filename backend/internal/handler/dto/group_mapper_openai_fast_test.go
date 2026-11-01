package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupMapperExposesForceOpenAIFastOnlyToAdmins(t *testing.T) {
	group := &service.Group{
		ID: 7, Name: "fast", Platform: service.PlatformOpenAI, Status: service.StatusActive, ForceOpenAIFast: true, FreeOpenAIFast: true,
	}

	userJSON, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "force_openai_fast")
	require.NotContains(t, string(userJSON), "force_openai_ultrafast")
	require.NotContains(t, string(userJSON), "free_openai_fast")

	adminJSON, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"force_openai_fast":true`)
	require.Contains(t, string(adminJSON), `"free_openai_fast":true`)
}

func TestGroupMapperExposesForceOpenAIUltrafastOnlyToAdmins(t *testing.T) {
	group := &service.Group{
		ID: 8, Name: "ultrafast", Platform: service.PlatformOpenAI, Status: service.StatusActive, ForceOpenAIUltrafast: true,
	}
	userJSON, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.NotContains(t, string(userJSON), "force_openai_ultrafast")
	adminJSON, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	require.Contains(t, string(adminJSON), `"force_openai_ultrafast":true`)
	require.Contains(t, string(adminJSON), `"force_openai_fast":false`)
}
