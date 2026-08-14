package service

import "context"

type openAIForwardModelContextKey struct{REDACTED

type openAIForwardModel struct {
	model                  string
	useCompactModelMapping bool
REDACTED

// WithOpenAIForwardModel records the model present in the forwarded request
// body after channel mapping and whether the legacy compact-only model mapping
// applies. Channel restriction checks then follow the same model chain used by
// Forward.
func WithOpenAIForwardModel(ctx context.Context, forwardModel string, useCompactModelMapping bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	return context.WithValue(ctx, openAIForwardModelContextKey{REDACTED, openAIForwardModel{
		model:                  forwardModel,
		useCompactModelMapping: useCompactModelMapping,
REDACTED)
REDACTED

func openAIForwardModelFromContext(ctx context.Context) (openAIForwardModel, bool) {
	if ctx == nil {
		return openAIForwardModel{REDACTED, false
REDACTED
	forwardModel, ok := ctx.Value(openAIForwardModelContextKey{REDACTED).(openAIForwardModel)
	return forwardModel, ok
REDACTED
