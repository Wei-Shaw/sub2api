package handler

import (
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleRequestBodyReadError handles cancellation and temporary capacity errors
// before callers classify malformed or oversized request bodies.
//
// A read deadline on HTTP/1 also cancels the request context, so a body read
// timeout is left to requestBodyReadFailure (408) instead of being reported as
// a client that went away (499).
func handleRequestBodyReadError(c *gin.Context, reqLog *zap.Logger, err error) bool {
	if err != nil && !pkghttputil.IsRequestBodyReadTimeout(err) && failoverClientGone(c) {
		logRequestBodyReadFailure(reqLog, pkghttputil.DescribeRequestBodyReadFailure(c.Request, err))
		c.Abort()
		return true
	}
	return middleware2.RejectRequestBodyMemoryOverload(c, err)
}

func formatBodyLimit(limit int64) string {
	const mb = 1024 * 1024
	if limit >= mb {
		return fmt.Sprintf("%dMB", limit/mb)
	}
	return fmt.Sprintf("%dB", limit)
}

func buildBodyTooLargeMessage(limit int64) string {
	return fmt.Sprintf("Request body too large, limit is %s", formatBodyLimit(limit))
}

// readLenientJSONRequestBodyWithPrealloc 读取并归一化 JSON 请求体。
//
// jsonValid 表示返回的 body 已经通过严格 JSON 校验，调用方可以据此跳过自己的
// 全量校验（与 gjson.ValidBytes 等价，见 httputil 的快速路径）。
func readLenientJSONRequestBodyWithPrealloc(
	req *http.Request, cfg *config.Config,
) (body []byte, jsonValid bool, err error) {
	return pkghttputil.ReadLenientJSONRequestBodyWithPrealloc(req, gatewayMaxBodySize(cfg))
}

func gatewayMaxBodySize(cfg *config.Config) int64 {
	if cfg == nil {
		return 0
	}
	return cfg.Gateway.MaxBodySize
}
