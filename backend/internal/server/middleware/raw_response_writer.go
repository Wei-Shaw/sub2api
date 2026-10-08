package middleware

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// RawResponseWriterHandler records the net/http ResponseWriter in the request
// context before gin wraps it. GPT delivery checks use it to read the real
// network flush error (net/http FlushError) regardless of how many gin writer
// wrappers (ops capture, server timing) sit in between.
func RawResponseWriterHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxkey.RawResponseWriter, w)))
	})
}
