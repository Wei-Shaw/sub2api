package service

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// The stream reader cannot keep the downstream alive until Do returns response
// headers. Keep all Gin writes on the calling goroutine while the HTTP request
// runs separately; late responses must be closed if the client has left.
type antigravityHeaderKeepaliveUpstream struct {
	HTTPUpstream
	c        *gin.Context
	interval time.Duration
}

type antigravityHeaderResult struct {
	response *http.Response
	err      error
}

func (u *antigravityHeaderKeepaliveUpstream) Do(req *http.Request, proxyURL string, accountID int64, concurrency int) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	completed := make(chan antigravityHeaderResult)
	abandoned := make(chan struct{})
	defer close(abandoned)
	go func() {
		resp, err := u.HTTPUpstream.Do(req.WithContext(ctx), proxyURL, accountID, concurrency)
		select {
		case completed <- antigravityHeaderResult{resp, err}:
		case <-abandoned:
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		}
	}()
	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()
	for {
		select {
		case result := <-completed:
			if result.err != nil || result.response == nil || result.response.Body == nil {
				cancel()
			} else {
				// Cancellation must outlive Do: the response body still uses ctx.
				result.response.Body = &antigravityHeaderResponseBody{ReadCloser: result.response.Body, cancel: cancel}
			}
			return result.response, result.err
		case <-ctx.Done():
			cancel()
			return nil, ctx.Err()
		case <-u.c.Request.Context().Done():
			cancel()
			return nil, u.c.Request.Context().Err()
		case <-ticker.C:
			if !u.c.Writer.Written() {
				u.c.Header("Content-Type", "text/event-stream")
				u.c.Header("Cache-Control", "no-cache")
				u.c.Header("Connection", "keep-alive")
				u.c.Header("X-Accel-Buffering", "no")
				u.c.Status(http.StatusOK)
			}
			MarkResponseCommitted(u.c)
			if _, err := u.c.Writer.Write([]byte(": ping\n\n")); err != nil {
				cancel()
				return nil, err
			}
			u.c.Writer.Flush()
		}
	}
}

type antigravityHeaderResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *antigravityHeaderResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}
