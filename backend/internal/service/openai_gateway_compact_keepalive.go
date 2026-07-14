package service

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const openAICompactNonstreamKeepaliveStateKey = "openai_compact_nonstream_keepalive_state"

type openAICompactNonstreamKeepaliveState struct {
	committed atomic.Bool
}

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

func (s *OpenAIGatewayService) logOpenAICompactNonstreamKeepaliveBootstrap() {
	interval := s.compactNonstreamKeepaliveInterval()
	if interval <= 0 {
		return
	}
	logger.L().With(
		zap.String("component", "service.openai_gateway"),
		zap.Int("interval_seconds", int(interval.Seconds())),
	).Info("OpenAI compact non-stream keepalive enabled")
}

func (s *OpenAIGatewayService) startCompactNonstreamKeepalive(ctx context.Context, c *gin.Context) func() {
	if s == nil || c == nil || c.Writer == nil || !isOpenAIResponsesCompactPath(c) || openAICompactClientWantsStream(c) {
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
	path := ""
	if c.Request != nil && c.Request.URL != nil {
		path = strings.TrimSpace(c.Request.URL.Path)
	}
	log := logger.FromContext(ctx).With(
		zap.String("component", "service.openai_gateway"),
		zap.String("request_path", path),
		zap.Int("interval_seconds", int(interval.Seconds())),
	)
	log.Info("OpenAI compact non-stream keepalive started")

	headers := c.Writer.Header()
	headers.Set("Content-Type", "application/json")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("X-Accel-Buffering", "no")
	headers.Del("Content-Length")

	state := &openAICompactNonstreamKeepaliveState{}
	c.Set(openAICompactNonstreamKeepaliveStateKey, state)
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		flushedLogged := false
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				n, err := c.Writer.Write([]byte("\n"))
				if n > 0 || c.Writer.Written() {
					state.committed.Store(true)
				}
				if err != nil {
					log.Warn("OpenAI compact non-stream keepalive write failed", zap.Error(err))
					return
				}
				flusher.Flush()
				if !flushedLogged {
					log.Info("OpenAI compact non-stream keepalive flushed")
					flushedLogged = true
				}
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

func compactStopFunc(stops ...func()) func() {
	if len(stops) == 0 || stops[0] == nil {
		return func() {}
	}
	return stops[0]
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
	if c == nil {
		return false
	}
	value, ok := c.Get(openAICompactNonstreamKeepaliveStateKey)
	if !ok {
		return false
	}
	state, ok := value.(*openAICompactNonstreamKeepaliveState)
	return ok && state != nil && state.committed.Load()
}

func writeOpenAICommittedTransportError(c *gin.Context) {
	if c == nil || c.Writer == nil {
		return
	}
	c.Data(http.StatusBadGateway, "application/json; charset=utf-8", openAITransportFailoverBody)
}
