package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayStallsIdleDeadlineTracksLastRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
		MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 1,
	}}}
	pr, pw := io.Pipe()
	defer func() { _ = pr.Close() }()
	defer func() { _ = pw.Close() }()
	go func() {
		time.Sleep(250 * time.Millisecond)
		_, _ = io.WriteString(pw, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n")
	}()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	start := time.Now()
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, start, "model", "model")
	elapsed := time.Since(start)
	t.Logf("idle timeout after %s (last upstream read at approximately 250ms)", elapsed)
	require.Error(t, err)
	require.GreaterOrEqual(t, elapsed, 1200*time.Millisecond, "do not shorten the configured idle interval")
	require.Less(t, elapsed, 1800*time.Millisecond, "do not wait for the next full polling interval")
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover), "an idle attempt with no model output should remain failover-safe")
	require.Empty(t, rec.Body.String())
}

func TestRelayStallsIdleFailoverBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, output := range []bool{false, true} {
		name := "before_output"
		if output {
			name = "after_output"
		}
		t.Run(name, func(t *testing.T) {
			svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{
				MaxLineSize: defaultMaxLineSize, StreamDataIntervalTimeout: 1,
			}}}
			pr, pw := io.Pipe()
			defer func() { _ = pr.Close() }()
			defer func() { _ = pw.Close() }()
			go func() {
				frame := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n"
				if output {
					frame += "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n"
				}
				_, _ = io.WriteString(pw, frame)
			}()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "model", "model")
			require.Error(t, err)
			var failover *UpstreamFailoverError
			if output {
				require.False(t, errors.As(err, &failover), "partial output must not be replayed")
				require.Contains(t, rec.Body.String(), "hello")
				require.Contains(t, rec.Body.String(), "stream_timeout")
			} else {
				require.ErrorAs(t, err, &failover)
				require.Empty(t, rec.Body.String())
			}
		})
	}
}
