package service

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseOpenAIResponsesFirstEventTimeoutSeconds(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  int
		ok    bool
	}{
		{name: "json number", value: json.Number("3"), want: 3, ok: true},
		{name: "float from json", value: float64(4), want: 4, ok: true},
		{name: "integer", value: 5, want: 5, ok: true},
		{name: "string", value: "6", want: 6, ok: true},
		{name: "zero", value: 0, want: 0, ok: true},
		{name: "fraction", value: 1.5, ok: false},
		{name: "negative", value: -1, ok: false},
		{name: "too large", value: 601, ok: false},
		{name: "nan", value: math.NaN(), ok: false},
		{name: "unsupported", value: true, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseOpenAIResponsesFirstEventTimeoutSeconds(tc.value)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.Equal(t, tc.want, got)
			}
		})
	}
}

func TestOpenAIResponsesInitialEventTimeoutUsesGlobalAndChannelOverrides(t *testing.T) {
	groupID := int64(42)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{GroupID: &groupID})

	channelService := NewChannelService(nil, nil, nil, nil)
	channelService.cache.Store(populateChannelCache([]Channel{{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{groupID},
		FeaturesConfig: map[string]any{openAIResponsesFirstEventTimeoutFeatureKey: float64(4)},
	}}, map[int64]string{groupID: PlatformOpenAI}))

	svc := &OpenAIGatewayService{
		cfg:            &config.Config{Gateway: config.GatewayConfig{OpenAIResponsesFirstEventTimeoutSeconds: 1}},
		channelService: channelService,
	}
	require.Equal(t, 4*time.Second, svc.openAIResponsesInitialEventTimeout(context.Background(), c))

	channelService.cache.Store(populateChannelCache([]Channel{{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{groupID},
		FeaturesConfig: map[string]any{openAIResponsesFirstEventTimeoutFeatureKey: "bad"},
	}}, map[int64]string{groupID: PlatformOpenAI}))
	require.Equal(t, time.Second, svc.openAIResponsesInitialEventTimeout(context.Background(), c))

	channelService.cache.Store(populateChannelCache([]Channel{{
		ID:             1,
		Status:         StatusActive,
		GroupIDs:       []int64{groupID},
		FeaturesConfig: map[string]any{openAIResponsesFirstEventTimeoutFeatureKey: 0},
	}}, map[int64]string{groupID: PlatformOpenAI}))
	require.Zero(t, svc.openAIResponsesInitialEventTimeout(context.Background(), c))
}

func TestOpenAIResponsesInitialEventTimeoutFallsBackToDefault(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.Equal(t, defaultOpenAIResponsesFirstEventTimeout, svc.openAIResponsesInitialEventTimeout(context.Background(), nil))
}
