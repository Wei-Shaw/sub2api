package service

import (
	"io"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
)

// Pings written to a streaming client while the gateway waits for the upstream
// response headers, one per client protocol: Codex treats an unknown event as
// fatal, so Responses clients get an SSE comment; Anthropic clients get the
// ping event the Messages stream already uses.
const (
	openAIResponsesUpstreamWaitPing = ":\n\n"
	anthropicUpstreamWaitPing       = "event: ping\ndata: {\"type\":\"ping\"}\n\n"
)

// The stream handlers keep the client alive only after the upstream response
// headers arrive. An upstream that thinks before answering (a long-context
// Gemini relay, say) otherwise leaves the client connection silent, and
// Cloudflare closes a connection that receives nothing for 120 s with a 524.
//
// The pings commit the SSE headers and are counted in
// openAIStreamKeepaliveBytesKey, which OpenAICompactKeepaliveAdjustedWrittenSize
// subtracts. Both handlers compare that adjusted size across Forward, so a
// failure before any model output still fails over to another account, and
// once the pings have committed the response every later error is written as
// an SSE event.

// doOpenAIUpstreamWithClientKeepalive sends the upstream request and, for a
// streaming client, writes ping every StreamKeepaliveInterval until the upstream
// response headers arrive.
func (s *OpenAIGatewayService) doOpenAIUpstreamWithClientKeepalive(
	c *gin.Context,
	clientStream bool,
	ping string,
	request *http.Request,
	proxyURL string,
	account *Account,
) (*http.Response, error) {
	stop := func() {}
	if clientStream && s.cfg != nil {
		stop = startUpstreamWaitKeepalive(c, time.Duration(s.cfg.Gateway.StreamKeepaliveInterval)*time.Second, ping)
	}
	resp, err := s.doOpenAIUpstream(request, proxyURL, account)
	stop()
	return resp, err
}

// doUpstreamWithClientKeepalive sends an Anthropic-gateway upstream request
// and, for a streaming client, writes an Anthropic ping event every
// StreamKeepaliveInterval until the upstream response headers arrive.
func (s *GatewayService) doUpstreamWithClientKeepalive(
	c *gin.Context,
	clientStream bool,
	request *http.Request,
	proxyURL string,
	account *Account,
	tlsProfile *tlsfingerprint.Profile,
) (*http.Response, error) {
	stop := func() {}
	if clientStream && s.cfg != nil {
		stop = startUpstreamWaitKeepalive(c, time.Duration(s.cfg.Gateway.StreamKeepaliveInterval)*time.Second, anthropicUpstreamWaitPing)
	}
	resp, err := s.httpUpstream.DoWithTLS(request, proxyURL, account.ID, account.Concurrency, tlsProfile)
	stop()
	return resp, err
}

// startUpstreamWaitKeepalive starts the ping goroutine and returns a stop
// function that returns only after the goroutine has exited, so the caller owns
// the ResponseWriter again once stop returns. The request goroutine is blocked
// in the upstream call meanwhile, so the goroutine is the only writer.
func startUpstreamWaitKeepalive(c *gin.Context, interval time.Duration, ping string) func() {
	if c == nil || c.Writer == nil || interval <= 0 {
		return func() {}
	}
	// The compact heartbeat already pings through its own writer wrapper; a
	// write through that wrapper would stop it.
	if _, ok := c.Get(openAICompactSSEKeepaliveKey); ok {
		return func() {}
	}
	writer := c.Writer
	var requestDone <-chan struct{}
	if c.Request != nil {
		requestDone = c.Request.Context().Done()
	}
	stopCh := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-requestDone:
				return
			case <-ticker.C:
			}
			if !writer.Written() {
				header := writer.Header()
				header.Set("Content-Type", "text/event-stream")
				header.Set("Cache-Control", "no-cache")
				header.Set("Connection", "keep-alive")
				header.Set("X-Accel-Buffering", "no")
				writer.WriteHeader(http.StatusOK)
			}
			n, err := io.WriteString(writer, ping)
			recordOpenAIStreamKeepaliveBytes(c, n)
			if err != nil {
				return
			}
			writer.Flush()
		}
	}()
	return func() {
		close(stopCh)
		<-exited
	}
}
