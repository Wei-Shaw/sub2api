package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

var ErrUpstreamResponseBodyTooLarge = errors.New("upstream response body too large")

const defaultUpstreamResponseReadMaxBytes int64 = 8 * 1024 * 1024

func resolveUpstreamResponseReadLimit(cfg *config.Config) int64 {
	if cfg != nil && cfg.Gateway.UpstreamResponseReadMaxBytes > 0 {
		return cfg.Gateway.UpstreamResponseReadMaxBytes
	}
	return defaultUpstreamResponseReadMaxBytes
}

func readUpstreamResponseBodyLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("response body is nil")
	}
	if maxBytes <= 0 {
		maxBytes = defaultUpstreamResponseReadMaxBytes
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("%w: limit=%d", ErrUpstreamResponseBodyTooLarge, maxBytes)
	}
	return body, nil
}

// TooLargeWriter 在响应超限时向客户端写格式化的错误响应。
type TooLargeWriter func(c *gin.Context)

// ReadUpstreamResponseBody 读取上游非流式响应体。
// 超限时自动记录 ops error 并调用 onTooLarge 向客户端写错误。
func ReadUpstreamResponseBody(reader io.Reader, cfg *config.Config, c *gin.Context, onTooLarge TooLargeWriter) ([]byte, error) {
	maxBytes := resolveUpstreamResponseReadLimit(cfg)
	body, err := readUpstreamResponseBodyLimited(reader, maxBytes)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			setOpsUpstreamError(c, http.StatusBadGateway, "upstream response too large", "")
			if onTooLarge != nil {
				onTooLarge(c)
			}
		}
		return nil, err
	}
	return body, nil
}

// anthropicTooLargeError 以 Anthropic Messages API 格式写入超限错误。
func anthropicTooLargeError(c *gin.Context) {
	c.JSON(http.StatusBadGateway, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    "upstream_error",
			"message": "Upstream response too large",
		},
	})
}

// openAITooLargeError 以 OpenAI / Gemini 格式写入超限错误。
func openAITooLargeError(c *gin.Context) {
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": "Upstream response too large",
		},
	})
}
