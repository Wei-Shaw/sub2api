package service

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (s *OpenAIGatewayService) compactNonstreamKeepaliveInterval() time.Duration {
	if s == nil || s.cfg == nil {
		return 0
	}
	seconds := s.cfg.Gateway.OpenAICompactNonstreamKeepaliveInterval
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (s *OpenAIGatewayService) startCompactNonstreamKeepalive(ctx context.Context, c *gin.Context) func() {
	if s == nil || c == nil || c.Writer == nil || !isOpenAIResponsesCompactPath(c) {
		return func() {}
	}
	interval := s.compactNonstreamKeepaliveInterval()
	if interval <= 0 {
		return func() {}
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	headers := c.Writer.Header()
	headers.Set("Content-Type", "application/json")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("X-Accel-Buffering", "no")
	headers.Del("Content-Length")

	stopCh := make(chan struct{})
	var stopOnce sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = c.Writer.Write([]byte("\n"))
				flusher.Flush()
			}
		}
	}()

	return func() {
		stopOnce.Do(func() {
			close(stopCh)
		})
		wg.Wait()
	}
}

func logOpenAICompactKeepaliveCommitted(ctx context.Context, c *gin.Context, account *Account, resp *http.Response) {
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	accountName := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
	}
	statusCode := 0
	upstreamRequestID := ""
	if resp != nil {
		statusCode = resp.StatusCode
		upstreamRequestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
	}
	requestPath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		requestPath = strings.TrimSpace(c.Request.URL.Path)
	}
	logger.FromContext(ctx).With(
		zap.String("component", "service.openai_gateway"),
		zap.Bool("compact_keepalive_committed", true),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.Int("upstream_status", statusCode),
		zap.String("upstream_request_id", upstreamRequestID),
		zap.String("request_path", requestPath),
	).Warn("OpenAI compact non-stream keepalive committed response before upstream error; proxying error without failover")
}

func openAICompactKeepaliveCommitted(c *gin.Context) bool {
	return c != nil && c.Writer != nil && c.Writer.Written() && isOpenAIResponsesCompactPath(c)
}

func writeOpenAICommittedTransportError(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	c.Data(http.StatusBadGateway, "application/json; charset=utf-8", openAITransportFailoverBody)
}
