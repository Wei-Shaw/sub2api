package service

import (
	"time"

	"github.com/gin-gonic/gin"
)

const openAIImagesJSONKeepaliveKey = "openai_images_json_keepalive"

// Compatibility aliases keep the existing Images tests and package-private
// helpers working while the implementation is shared with other JSON routes.
type openAIImagesJSONKeepalive = downstreamJSONKeepalive
type openAIImagesJSONKeepaliveWriter = downstreamJSONKeepaliveWriter

// StartOpenAIImagesJSONKeepalive starts whitespace heartbeats for a
// non-streaming Images request. A non-positive interval disables the feature.
func StartOpenAIImagesJSONKeepalive(c *gin.Context, interval time.Duration) func() {
	return startDownstreamJSONKeepalive(c, openAIImagesJSONKeepaliveKey, interval)
}

// StopOpenAIImagesJSONKeepaliveCommitted stops heartbeats and reports whether
// they already committed a 200 response.
func StopOpenAIImagesJSONKeepaliveCommitted(c *gin.Context) bool {
	return stopDownstreamJSONKeepalive(c, openAIImagesJSONKeepaliveKey)
}

// OpenAIImagesJSONKeepalivePresent reports whether the response writer belongs
// to an Images JSON request, including fast responses before the first beat.
func OpenAIImagesJSONKeepalivePresent(c *gin.Context) bool {
	return downstreamJSONKeepalivePresent(c, openAIImagesJSONKeepaliveKey)
}

// OpenAIImagesJSONKeepaliveAdjustedWrittenSize excludes heartbeat whitespace
// from response-size checks so account retry and failover remain available.
func OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	return downstreamJSONKeepaliveAdjustedWrittenSize(c, openAIImagesJSONKeepaliveKey)
}

func openAIImagesJSONKeepaliveFromContext(c *gin.Context) *openAIImagesJSONKeepalive {
	return downstreamJSONKeepaliveForKey(c, openAIImagesJSONKeepaliveKey)
}
