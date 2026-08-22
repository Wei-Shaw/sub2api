package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"go.uber.org/zap"
)

// parseFailureSnippetLen bounds the head/tail snippets logged on body parse
// failure. 256 bytes is enough to see the structural context (model field,
// first content block / trailing brace) without dumping user payloads.
const parseFailureSnippetLen = 256

// logRequestBodyParseFailure records the real reason a request body failed
// JSON parsing/validation. The client keeps receiving the generic
// "Failed to parse request body"; the sanitized diagnostics (underlying
// error with byte offset, body length, escaped head/tail snippets) land in
// the server log only, so operators can distinguish genuinely invalid JSON
// from a truncated or partially consumed body.
//
// err may be nil for call sites that validate with gjson.ValidBytes directly;
// the diagnostic error is derived from the body in that case.
func logRequestBodyParseFailure(reqLog *zap.Logger, body []byte, err error) {
	if reqLog == nil {
		return
	}
	if err == nil {
		err = service.DescribeInvalidJSON(body)
	}

	head := body
	var tail []byte
	if len(body) > parseFailureSnippetLen {
		head = body[:parseFailureSnippetLen]
		tail = body[len(body)-parseFailureSnippetLen:]
	}

	fields := []zap.Field{
		zap.Error(err),
		zap.Int("body_len", len(body)),
		zap.String("body_head", sanitizeBodySnippet(head)),
	}
	if len(tail) > 0 {
		fields = append(fields, zap.String("body_tail", sanitizeBodySnippet(tail)))
	}
	reqLog.Warn("parse request body failed", fields...)
}

// sanitizeBodySnippet escapes control characters and invalid UTF-8 so the
// snippet is always a single printable log line.
func sanitizeBodySnippet(b []byte) string {
	return strconv.Quote(string(b))
}
