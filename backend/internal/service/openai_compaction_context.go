package service

import "context"

type openAIForwardModelContextKey struct{}

type openAIForwardModel struct {
	model                  string
	useCompactModelMapping bool
}

// WithOpenAIForwardModel records the model present in the forwarded request
// body after channel mapping and whether the legacy compact-only model mapping
// applies. Channel restriction checks then follow the same model chain used by
// Forward.
func WithOpenAIForwardModel(ctx context.Context, forwardModel string, useCompactModelMapping bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIForwardModelContextKey{}, openAIForwardModel{
		model:                  forwardModel,
		useCompactModelMapping: useCompactModelMapping,
	})
}

func openAIForwardModelFromContext(ctx context.Context) (openAIForwardModel, bool) {
	if ctx == nil {
		return openAIForwardModel{}, false
	}
	forwardModel, ok := ctx.Value(openAIForwardModelContextKey{}).(openAIForwardModel)
	return forwardModel, ok
}
