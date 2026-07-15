package service

import "context"

// HTTPUpstreamProfile marks HTTP upstream requests that need provider-specific
// transport policy.
type HTTPUpstreamProfile string

const (
	HTTPUpstreamProfileDefault HTTPUpstreamProfile = ""
	HTTPUpstreamProfileOpenAI  HTTPUpstreamProfile = "openai"
)

type httpUpstreamProfileContextKey struct{REDACTED
type httpUpstreamDisableRedirectsContextKey struct{REDACTED

// WithHTTPUpstreamProfile injects an upstream transport profile into ctx.
func WithHTTPUpstreamProfile(ctx context.Context, profile HTTPUpstreamProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if profile == HTTPUpstreamProfileDefault {
		return ctx
REDACTED
	return context.WithValue(ctx, httpUpstreamProfileContextKey{REDACTED, profile)
REDACTED

// HTTPUpstreamProfileFromContext resolves the upstream transport profile from ctx.
func HTTPUpstreamProfileFromContext(ctx context.Context) HTTPUpstreamProfile {
	if ctx == nil {
		return HTTPUpstreamProfileDefault
REDACTED
	profile, ok := ctx.Value(httpUpstreamProfileContextKey{REDACTED).(HTTPUpstreamProfile)
	if !ok {
		return HTTPUpstreamProfileDefault
REDACTED
	switch profile {
	case HTTPUpstreamProfileOpenAI:
		return profile
	default:
		return HTTPUpstreamProfileDefault
REDACTED
REDACTED

// WithHTTPUpstreamRedirectsDisabled prevents credential-bearing probes from
// following redirects through the shared upstream client.
func WithHTTPUpstreamRedirectsDisabled(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return context.WithValue(ctx, httpUpstreamDisableRedirectsContextKey{REDACTED, true)
REDACTED

func HTTPUpstreamRedirectsDisabled(ctx context.Context) bool {
	return ctx != nil && ctx.Value(httpUpstreamDisableRedirectsContextKey{REDACTED) == true
REDACTED
