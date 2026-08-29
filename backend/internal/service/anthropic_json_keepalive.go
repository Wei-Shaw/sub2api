package service

import (
	"time"

	"github.com/gin-gonic/gin"
)

const anthropicJSONKeepaliveKey = "anthropic_json_keepalive"

// StartAnthropicJSONKeepalive starts leading-whitespace heartbeats for a
// non-streaming Anthropic Messages response. The first beat commits HTTP 200.
func StartAnthropicJSONKeepalive(c *gin.Context, interval time.Duration) func() {
	return startDownstreamJSONKeepalive(c, anthropicJSONKeepaliveKey, interval)
}

func StopAnthropicJSONKeepaliveCommitted(c *gin.Context) bool {
	return stopDownstreamJSONKeepalive(c, anthropicJSONKeepaliveKey)
}

func AnthropicJSONKeepalivePresent(c *gin.Context) bool {
	return downstreamJSONKeepalivePresent(c, anthropicJSONKeepaliveKey)
}

func AnthropicJSONKeepaliveCommitted(c *gin.Context) bool {
	return downstreamJSONKeepaliveCommitted(c, anthropicJSONKeepaliveKey)
}

func AnthropicJSONKeepaliveBytes(c *gin.Context) int {
	return downstreamJSONKeepaliveBytes(c, anthropicJSONKeepaliveKey)
}

func AnthropicJSONKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	return downstreamJSONKeepaliveAdjustedWrittenSize(c, anthropicJSONKeepaliveKey)
}

// AnthropicDownstreamAdjustedWrittenSize excludes either SSE or JSON
// heartbeat bytes from failover response-write checks.
func AnthropicDownstreamAdjustedWrittenSize(c *gin.Context) int {
	if AnthropicJSONKeepalivePresent(c) {
		return AnthropicJSONKeepaliveAdjustedWrittenSize(c)
	}
	return AnthropicPreHeaderKeepaliveAdjustedWrittenSize(c)
}
