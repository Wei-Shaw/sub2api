package handler

import (
	"net/http"

	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"go.uber.org/zap"
)

// requestBodyReadFailure maps a request body read error to the response status
// and message. Timeouts (408) and client disconnects (499) are no longer
// reported as malformed requests; other read failures keep the stable generic
// 400 message. Failures other than the configured size limit are logged with
// bounded, payload-free diagnostics (kind, declared length, bytes received and
// read duration) so operators can tell a stalled or slow upload from a broken
// client without logging request content.
func requestBodyReadFailure(reqLog *zap.Logger, req *http.Request, err error) (int, string) {
	failure := pkghttputil.DescribeRequestBodyReadFailure(req, err)
	if failure.Kind == pkghttputil.RequestBodyReadErrorMaxBytes {
		return failure.Status, buildBodyTooLargeMessage(failure.MaxBytesLimit)
	}
	if reqLog != nil {
		reqLog.Warn("read request body failed", failure.LogFields()...)
	}
	return failure.Status, failure.Message
}
