package service

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

// JSON permits leading whitespace, so non-stream clients can still decode the
// final body after these bytes keep an otherwise idle connection alive.
const openAIImagesJSONHeartbeatPayload = " \n"

func (s *OpenAIGatewayService) readOpenAIImagesNonStreamingResponseBody(
	reader io.Reader,
	c *gin.Context,
) ([]byte, error) {
	return s.readOpenAIImagesNonStreamingResponseBodyWithInterval(
		reader,
		c,
		s.openAIImageStreamKeepaliveInterval(),
	)
}

func (s *OpenAIGatewayService) readOpenAIImagesNonStreamingResponseBodyWithInterval(
	reader io.Reader,
	c *gin.Context,
	heartbeatInterval time.Duration,
) ([]byte, error) {
	if heartbeatInterval <= 0 || c == nil || c.Writer == nil {
		return ReadUpstreamResponseBody(reader, s.cfg, c, openAITooLargeError)
	}

	maxBytes := resolveUpstreamResponseReadLimit(s.cfg)
	stopHeartbeat := startOpenAIImagesJSONHeartbeat(c.Writer, heartbeatInterval)
	body, err := readUpstreamResponseBodyLimited(reader, maxBytes)
	stopHeartbeat()
	if err != nil {
		handleUpstreamResponseBodyReadError(err, c, openAITooLargeError)
		return nil, err
	}
	return body, nil
}

func (s *OpenAIGatewayService) startOpenAIImagesResponseHeaderHeartbeat(
	c *gin.Context,
	stream bool,
) func() {
	if stream || c == nil || c.Writer == nil {
		return func() {}
	}
	return startOpenAIImagesJSONHeartbeat(c.Writer, s.openAIImageStreamKeepaliveInterval())
}

func startOpenAIImagesJSONHeartbeat(writer http.ResponseWriter, interval time.Duration) func() {
	if writer == nil || interval <= 0 {
		return func() {}
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				if err := writeOpenAIImagesJSONHeartbeat(writer); err != nil {
					logger.LegacyPrintf(
						"service.openai_gateway",
						"[OpenAI] Images non-stream heartbeat disabled: %s",
						sanitizeUpstreamErrorMessage(err.Error()),
					)
					return
				}
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			close(stopCh)
			<-doneCh
		})
	}
}

func writeOpenAIImagesJSONHeartbeat(writer http.ResponseWriter) error {
	rawWriter := unwrapOpenAIImagesResponseWriter(writer)
	if rawWriter == nil {
		return fmt.Errorf("image response writer is unavailable")
	}
	flusher, ok := rawWriter.(http.Flusher)
	if !ok {
		return fmt.Errorf("image response writer does not support flushing")
	}

	header := rawWriter.Header()
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", "application/json; charset=utf-8")
	}
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")

	// The first heartbeat necessarily commits HTTP 200. A later upstream error
	// remains a valid JSON body, but its wire-level status can no longer change.
	if _, err := io.WriteString(rawWriter, openAIImagesJSONHeartbeatPayload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func unwrapOpenAIImagesResponseWriter(writer http.ResponseWriter) http.ResponseWriter {
	for range 8 {
		unwrapper, ok := writer.(interface {
			Unwrap() http.ResponseWriter
		})
		if !ok {
			return writer
		}
		next := unwrapper.Unwrap()
		if next == nil || next == writer {
			return writer
		}
		writer = next
	}
	return writer
}
