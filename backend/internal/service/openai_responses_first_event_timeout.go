package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

const (
	openAIResponsesFirstEventTimeoutFeatureKey = "responses_first_event_timeout"
	defaultOpenAIResponsesFirstEventTimeout    = time.Duration(config.DefaultOpenAIResponsesFirstEventTimeoutSeconds) * time.Second
)

// openAIResponsesInitialEventTimeout resolves the process default and the
// channel override used to hold Responses lifecycle SSE events before the
// first semantic output or retryable upstream error.
func (s *OpenAIGatewayService) openAIResponsesInitialEventTimeout(ctx context.Context, c *gin.Context) time.Duration {
	timeout := defaultOpenAIResponsesFirstEventTimeout
	if s != nil && s.cfg != nil {
		seconds := s.cfg.Gateway.OpenAIResponsesFirstEventTimeoutSeconds
		if seconds >= 0 && seconds <= config.MaxOpenAIResponsesFirstEventTimeoutSeconds {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	if s == nil || s.channelService == nil {
		return timeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	groupID := getOpenAIGroupIDFromContext(c)
	if groupID <= 0 {
		return timeout
	}
	channel, err := s.channelService.GetChannelForGroup(ctx, groupID)
	if err != nil {
		slog.Warn("failed to resolve OpenAI Responses first event timeout channel override", "group_id", groupID, "error", err)
		return timeout
	}
	if channel == nil || channel.FeaturesConfig == nil {
		return timeout
	}
	seconds, ok := parseOpenAIResponsesFirstEventTimeoutSeconds(channel.FeaturesConfig[openAIResponsesFirstEventTimeoutFeatureKey])
	if !ok {
		return timeout
	}
	return time.Duration(seconds) * time.Second
}

func parseOpenAIResponsesFirstEventTimeoutSeconds(value any) (int, bool) {
	var seconds int64
	switch value := value.(type) {
	case int:
		seconds = int64(value)
	case int8:
		seconds = int64(value)
	case int16:
		seconds = int64(value)
	case int32:
		seconds = int64(value)
	case int64:
		seconds = value
	case uint:
		if uint64(value) > math.MaxInt64 {
			return 0, false
		}
		seconds = int64(value)
	case uint8:
		seconds = int64(value)
	case uint16:
		seconds = int64(value)
	case uint32:
		seconds = int64(value)
	case uint64:
		if value > math.MaxInt64 {
			return 0, false
		}
		seconds = int64(value)
	case float32:
		if float64(value) != math.Trunc(float64(value)) || value < 0 || value > config.MaxOpenAIResponsesFirstEventTimeoutSeconds {
			return 0, false
		}
		seconds = int64(value)
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) || value < 0 || value > config.MaxOpenAIResponsesFirstEventTimeoutSeconds {
			return 0, false
		}
		seconds = int64(value)
	case json.Number:
		parsed, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil {
			return 0, false
		}
		seconds = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0, false
		}
		seconds = parsed
	default:
		return 0, false
	}
	if seconds < 0 || seconds > config.MaxOpenAIResponsesFirstEventTimeoutSeconds {
		return 0, false
	}
	return int(seconds), true
}
