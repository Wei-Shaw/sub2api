package traffic

import "context"

// originalRequestBodyKey carries the unencoded length of a request body that
// the sender compressed before writing it. Without it, a content-coded request
// only exposes actual wire bytes: the original length cannot be recovered from
// a compressed stream after the fact.
type originalRequestBodyKey struct{}

// WithOriginalRequestBodyBytes annotates an outgoing request with the size of
// the body before content coding. It is only meaningful together with a
// non-identity Content-Encoding; identity senders must not set it because their
// actual bytes already measure the original representation.
func WithOriginalRequestBodyBytes(ctx context.Context, n int64) context.Context {
	if ctx == nil || n < 0 {
		return ctx
	}
	return context.WithValue(ctx, originalRequestBodyKey{}, n)
}

// OriginalRequestBodyBytes returns the pre-coding body size recorded by the
// sender, if any.
func OriginalRequestBodyBytes(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	n, ok := ctx.Value(originalRequestBodyKey{}).(int64)
	return n, ok
}
