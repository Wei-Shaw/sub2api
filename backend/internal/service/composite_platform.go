package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// WithResolvedTargetPlatform stores the concrete provider chosen for a request
// made through a composite group.
func WithResolvedTargetPlatform(ctx context.Context, platform string) context.Context {
	platform = strings.TrimSpace(platform)
	if ctx == nil || platform == "" {
		return ctx
REDACTED
	return context.WithValue(ctx, ctxkey.ResolvedTargetPlatform, platform)
REDACTED

// ResolvedTargetPlatformFromContext returns the concrete provider chosen for
// the current request, if one was resolved.
func ResolvedTargetPlatformFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
REDACTED
	platform, ok := ctx.Value(ctxkey.ResolvedTargetPlatform).(string)
	platform = strings.TrimSpace(platform)
	if !ok || platform == "" {
		return "", false
REDACTED
	return platform, true
REDACTED

func WithCompositeRouteDecision(ctx context.Context, decision CompositeRouteDecision) context.Context {
	if ctx == nil || !decision.Matched {
		return ctx
REDACTED
	ctx = WithResolvedTargetPlatform(ctx, decision.TargetPlatform)
	if model := strings.TrimSpace(decision.UpstreamModel); model != "" {
		ctx = context.WithValue(ctx, ctxkey.ResolvedUpstreamModel, model)
REDACTED
	if model := strings.TrimSpace(decision.PublicModel); model != "" {
		ctx = context.WithValue(ctx, ctxkey.RequestedPublicModel, model)
REDACTED
	if source := strings.TrimSpace(decision.Source); source != "" {
		ctx = context.WithValue(ctx, ctxkey.CompositeRouteSource, source)
REDACTED
	return ctx
REDACTED

func ResolvedUpstreamModelFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
REDACTED
	model, ok := ctx.Value(ctxkey.ResolvedUpstreamModel).(string)
	model = strings.TrimSpace(model)
	if !ok || model == "" {
		return "", false
REDACTED
	return model, true
REDACTED

func RequestedPublicModelFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
REDACTED
	model, ok := ctx.Value(ctxkey.RequestedPublicModel).(string)
	model = strings.TrimSpace(model)
	if !ok || model == "" {
		return "", false
REDACTED
	return model, true
REDACTED

func CompositeRouteSourceFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
REDACTED
	source, ok := ctx.Value(ctxkey.CompositeRouteSource).(string)
	source = strings.TrimSpace(source)
	if !ok || source == "" {
		return "", false
REDACTED
	return source, true
REDACTED

// DetectModelPlatform maps common public model IDs to the concrete provider
// platform used by sub2api. It intentionally returns false for ambiguous model
// names so composite groups fail closed instead of guessing.
func DetectModelPlatform(model string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return "", false
REDACTED

	normalized = strings.TrimPrefix(normalized, "models/")
	if slash := strings.IndexByte(normalized, '/'); slash > 0 {
		provider := strings.TrimSpace(normalized[:slash])
		rest := strings.TrimSpace(normalized[slash+1:])
		switch provider {
		case "anthropic", "claude":
			return PlatformAnthropic, true
		case "openai", "chatgpt":
			return PlatformOpenAI, true
		case "google", "google-ai-studio", "gemini":
			return PlatformGemini, true
		case "xai", "x-ai", "grok":
			return PlatformGrok, true
	REDACTED
		if rest != "" {
			normalized = strings.TrimPrefix(rest, "models/")
	REDACTED
REDACTED

	switch {
	case strings.HasPrefix(normalized, "anthropic.claude-"),
		strings.HasPrefix(normalized, "claude-"):
		return PlatformAnthropic, true
	case strings.HasPrefix(normalized, "gpt-"),
		strings.HasPrefix(normalized, "chatgpt-"),
		strings.HasPrefix(normalized, "codex-"),
		strings.HasPrefix(normalized, "text-embedding-"),
		strings.HasPrefix(normalized, "text-moderation-"),
		strings.HasPrefix(normalized, "omni-moderation-"),
		strings.HasPrefix(normalized, "dall-e-"),
		strings.HasPrefix(normalized, "gpt-image-"),
		strings.HasPrefix(normalized, "tts-"),
		strings.HasPrefix(normalized, "whisper-"),
		hasOpenAISeriesPrefix(normalized):
		return PlatformOpenAI, true
	case strings.HasPrefix(normalized, "gemini-"),
		strings.HasPrefix(normalized, "learnlm-"):
		return PlatformGemini, true
	case normalized == "grok" || strings.HasPrefix(normalized, "grok-"):
		return PlatformGrok, true
	default:
		return "", false
REDACTED
REDACTED

func hasOpenAISeriesPrefix(model string) bool {
	for _, prefix := range []string{"o1", "o3", "o4", "o5"REDACTED {
		if model == prefix || strings.HasPrefix(model, prefix+"-") {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (s *GatewayService) resolveCompositeRouteDecision(ctx context.Context, group *Group, requestedModel, endpoint string) (CompositeRouteDecision, bool, error) {
	if group == nil || group.Platform != PlatformComposite {
		return CompositeRouteDecision{REDACTED, false, nil
REDACTED
	if platform, ok := ResolvedTargetPlatformFromContext(ctx); ok {
		upstreamModel := requestedModel
		if resolvedModel, modelOK := ResolvedUpstreamModelFromContext(ctx); modelOK {
			upstreamModel = resolvedModel
	REDACTED
		source := CompositeRouteSourceDetector
		if resolvedSource, sourceOK := CompositeRouteSourceFromContext(ctx); sourceOK {
			source = resolvedSource
	REDACTED
		return CompositeRouteDecision{
			Matched:        true,
			Source:         source,
			GroupID:        group.ID,
			PublicModel:    requestedModel,
			TargetPlatform: platform,
			UpstreamModel:  upstreamModel,
			Endpoint:       normalizeCompositeRouteEndpoint(endpoint),
	REDACTED, true, nil
REDACTED
	decision, err := s.compositeResolver.Resolve(ctx, group.ID, requestedModel, endpoint)
	if err != nil {
		return decision, false, err
REDACTED
	return decision, decision.Matched, nil
REDACTED

func isConcreteRequestPlatform(platform string) bool {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok:
		return true
	default:
		return false
REDACTED
REDACTED
