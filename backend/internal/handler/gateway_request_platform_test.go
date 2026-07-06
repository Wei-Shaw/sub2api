package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGatewayRequestPlatformForAPIKey_RouteIntentOverridesPrimaryGroup(t *testing.T) {
	anthropicGroup := service.Group{ID: 10, Platform: service.PlatformAnthropic, Status: service.StatusActive, Hydrated: true}
	openAIGroup := service.Group{ID: 20, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true}
	apiKey := &service.APIKey{
		GroupID:  &openAIGroup.ID,
		GroupIDs: []int64{openAIGroup.ID, anthropicGroup.ID},
		Group:    &openAIGroup,
		Groups:   []service.Group{openAIGroup, anthropicGroup},
	}
	claudePrimaryAPIKey := &service.APIKey{
		GroupID:  &anthropicGroup.ID,
		GroupIDs: []int64{anthropicGroup.ID, openAIGroup.ID},
		Group:    &anthropicGroup,
		Groups:   []service.Group{anthropicGroup, openAIGroup},
	}

	require.Equal(t, service.PlatformAnthropic, gatewayRequestPlatformForAPIKey(apiKey, "", service.PlatformAnthropic))
	require.Equal(t, service.PlatformOpenAI, gatewayRequestPlatformForAPIKey(claudePrimaryAPIKey, "", service.PlatformOpenAI))
	require.Equal(t, service.PlatformOpenAI, gatewayRequestPlatformForAPIKey(apiKey, "", service.PlatformGemini))
	require.Equal(t, service.PlatformAntigravity, gatewayRequestPlatformForAPIKey(apiKey, service.PlatformAntigravity, service.PlatformAnthropic))
}

func TestOpenAICompatibleRequestPlatform_RouteIntentOverridesGroupOrder(t *testing.T) {
	openAIGroup := service.Group{ID: 20, Platform: service.PlatformOpenAI, Status: service.StatusActive, Hydrated: true}
	grokGroup := service.Group{ID: 30, Platform: service.PlatformGrok, Status: service.StatusActive, Hydrated: true}
	apiKey := &service.APIKey{
		GroupID:  &openAIGroup.ID,
		GroupIDs: []int64{openAIGroup.ID, grokGroup.ID},
		Group:    &openAIGroup,
		Groups:   []service.Group{openAIGroup, grokGroup},
	}

	require.Equal(t, service.PlatformGrok, openAICompatibleRequestPlatform(apiKey, service.PlatformGrok))
	require.Equal(t, service.PlatformOpenAI, openAICompatibleRequestPlatform(apiKey, service.PlatformAnthropic))
}
