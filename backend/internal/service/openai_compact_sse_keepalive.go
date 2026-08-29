package service

import (
	"time"

	"github.com/gin-gonic/gin"
)

// openAICompactSSEKeepaliveKey stores the downstream SSE heartbeat used by
// body-signal compact requests and by the post-header passthrough stream.
const openAICompactSSEKeepaliveKey = "openai_compact_sse_keepalive"

// Keep the established private names available to the existing focused tests
// while sharing the writer/timer implementation with normal Responses SSE.
type openAICompactSSEKeepalive = downstreamSSEKeepalive
type openAICompactKeepaliveWriter = downstreamSSEKeepaliveWriter

// StartOpenAICompactSSEKeepalive starts downstream heartbeats for a compact
// request marked as client-streaming. The first heartbeat is delayed by one
// complete interval.
func StartOpenAICompactSSEKeepalive(c *gin.Context, interval time.Duration) func() {
	return startDownstreamSSEKeepalive(
		c,
		openAICompactSSEKeepaliveKey,
		interval,
		[]byte(": keepalive\n\n"),
		openAICompactClientWantsStream(c),
		true,
	)
}

// startOpenAISSEKeepalive is used after passthrough has received upstream SSE
// headers. A pre-header helper, when present, already remains active until the
// first real downstream write and therefore owns this phase as well.
func startOpenAISSEKeepalive(c *gin.Context, interval time.Duration) func() {
	if downstreamSSEKeepaliveForKey(c, openAIPreHeaderSSEKeepaliveKey) != nil {
		return func() {}
	}
	return startDownstreamSSEKeepalive(
		c,
		openAICompactSSEKeepaliveKey,
		interval,
		[]byte(": keepalive\n\n"),
		true,
		true,
	)
}

// StopOpenAICompactSSEKeepaliveCommitted stops both compact/post-header and
// normal Responses pre-header keepalives. Handlers use the combined result to
// decide whether a final error must be emitted as response.failed.
func StopOpenAICompactSSEKeepaliveCommitted(c *gin.Context) bool {
	compactCommitted := stopDownstreamSSEKeepalive(c, openAICompactSSEKeepaliveKey)
	preHeaderCommitted := StopOpenAIPreHeaderSSEKeepaliveCommitted(c)
	return compactCommitted || preHeaderCommitted
}

// OpenAICompactKeepaliveAdjustedWrittenSize excludes every OpenAI SSE
// heartbeat byte from handler failover and semantic-output decisions.
func OpenAICompactKeepaliveAdjustedWrittenSize(c *gin.Context) int {
	size := downstreamSSEKeepaliveAdjustedWrittenSize(
		c,
		openAICompactSSEKeepaliveKey,
		openAIPreHeaderSSEKeepaliveKey,
	)
	if size < 0 || c == nil {
		return size
	}
	streamKeepaliveBytes := 0
	if value, ok := c.Get(openAIStreamKeepaliveBytesKey); ok {
		streamKeepaliveBytes, _ = value.(int)
	}
	if streamKeepaliveBytes <= 0 {
		return size
	}
	if real := size - streamKeepaliveBytes; real > 0 {
		return real
	}
	return -1
}
