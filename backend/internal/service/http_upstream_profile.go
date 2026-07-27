package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/port/upstream"
)

// HTTPUpstreamProfile marks HTTP upstream requests that need provider-specific
// transport policy.
//
// Alias: 契约已迁移至 internal/port/upstream。
type HTTPUpstreamProfile = upstream.HTTPUpstreamProfile

const (
	HTTPUpstreamProfileDefault = upstream.HTTPUpstreamProfileDefault
	HTTPUpstreamProfileOpenAI  = upstream.HTTPUpstreamProfileOpenAI
)

// WithHTTPUpstreamProfile injects an upstream transport profile into ctx.
// Go 无函数别名，转发至 upstream 包。
func WithHTTPUpstreamProfile(ctx context.Context, profile HTTPUpstreamProfile) context.Context {
	return upstream.WithHTTPUpstreamProfile(ctx, profile)
}

// HTTPUpstreamProfileFromContext resolves the upstream transport profile from ctx.
func HTTPUpstreamProfileFromContext(ctx context.Context) HTTPUpstreamProfile {
	return upstream.HTTPUpstreamProfileFromContext(ctx)
}

// WithHTTPUpstreamRedirectsDisabled prevents credential-bearing probes from
// following redirects through the shared upstream client.
func WithHTTPUpstreamRedirectsDisabled(ctx context.Context) context.Context {
	return upstream.WithHTTPUpstreamRedirectsDisabled(ctx)
}

func HTTPUpstreamRedirectsDisabled(ctx context.Context) bool {
	return upstream.HTTPUpstreamRedirectsDisabled(ctx)
}
