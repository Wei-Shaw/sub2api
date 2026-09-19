package service

import (
	"context"
	"slices"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

// WithResolvedTargetPlatform stores the concrete provider chosen for a request
// made through a composite group.
func WithResolvedTargetPlatform(ctx context.Context, platform string) context.Context {
	platform = strings.TrimSpace(platform)
	if ctx == nil || platform == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.ResolvedTargetPlatform, platform)
}

// ResolvedTargetPlatformFromContext returns the concrete provider chosen for
// the current request, if one was resolved.
func ResolvedTargetPlatformFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	platform, ok := ctx.Value(ctxkey.ResolvedTargetPlatform).(string)
	platform = strings.TrimSpace(platform)
	if !ok || platform == "" {
		return "", false
	}
	return platform, true
}

// normalizeCompositeCandidatePlatforms 归一化候选平台集合：去空白、去空项、
// 去重并按升序排列；输入为空时返回 nil。池集合在写入 ctx / decision 前必须
// 经过该函数，保证「稳定候选契约」:同一候选集在请求生命周期内表现一致。
func normalizeCompositeCandidatePlatforms(platforms []string) []string {
	normalized := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		platform = strings.TrimSpace(platform)
		if platform == "" {
			continue
		}
		if slices.Contains(normalized, platform) {
			continue
		}
		normalized = append(normalized, platform)
	}
	if len(normalized) == 0 {
		return nil
	}
	slices.Sort(normalized)
	return normalized
}

// WithCompositeCandidatePlatforms 在 ctx 上携带 composite 多平台候选池。输入
// 会被归一化（去重升序），存入 ctx 的是内部副本；空集合不写入。
func WithCompositeCandidatePlatforms(ctx context.Context, platforms []string) context.Context {
	if ctx == nil {
		return ctx
	}
	normalized := normalizeCompositeCandidatePlatforms(platforms)
	if len(normalized) == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.CompositeCandidatePlatforms, normalized)
}

// CompositeCandidatePlatformsFromContext 返回当前请求的候选池快照（去重升序）。
// 返回值为副本，调用方修改不会影响 ctx；未携带池时返回 nil。
func CompositeCandidatePlatformsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	platforms, ok := ctx.Value(ctxkey.CompositeCandidatePlatforms).([]string)
	if !ok || len(platforms) == 0 {
		return nil
	}
	return slices.Clone(platforms)
}

func WithCompositeRouteDecision(ctx context.Context, decision CompositeRouteDecision) context.Context {
	if ctx == nil || !decision.Matched {
		return ctx
	}
	if isCompositePoolDecision(decision) {
		// pool 决策：只携带候选集合与公开模型/来源，不写 ResolvedTargetPlatform /
		// ResolvedUpstreamModel——最终平台由 selector 选定，选号/重试期间不得被
		// 单一账号平台覆盖，池请求也不改写 body 模型。
		ctx = WithCompositeCandidatePlatforms(ctx, decision.CandidatePlatforms)
		if model := strings.TrimSpace(decision.PublicModel); model != "" {
			ctx = context.WithValue(ctx, ctxkey.RequestedPublicModel, model)
		}
		if source := strings.TrimSpace(decision.Source); source != "" {
			ctx = context.WithValue(ctx, ctxkey.CompositeRouteSource, source)
		}
		return ctx
	}
	ctx = WithResolvedTargetPlatform(ctx, decision.TargetPlatform)
	if model := strings.TrimSpace(decision.UpstreamModel); model != "" {
		ctx = context.WithValue(ctx, ctxkey.ResolvedUpstreamModel, model)
	}
	if model := strings.TrimSpace(decision.PublicModel); model != "" {
		ctx = context.WithValue(ctx, ctxkey.RequestedPublicModel, model)
	}
	if source := strings.TrimSpace(decision.Source); source != "" {
		ctx = context.WithValue(ctx, ctxkey.CompositeRouteSource, source)
	}
	return ctx
}

func ResolvedUpstreamModelFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	model, ok := ctx.Value(ctxkey.ResolvedUpstreamModel).(string)
	model = strings.TrimSpace(model)
	if !ok || model == "" {
		return "", false
	}
	return model, true
}

func RequestedPublicModelFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	model, ok := ctx.Value(ctxkey.RequestedPublicModel).(string)
	model = strings.TrimSpace(model)
	if !ok || model == "" {
		return "", false
	}
	return model, true
}

func CompositeRouteSourceFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	source, ok := ctx.Value(ctxkey.CompositeRouteSource).(string)
	source = strings.TrimSpace(source)
	if !ok || source == "" {
		return "", false
	}
	return source, true
}

// DetectModelPlatform maps common public model IDs to the concrete provider
// platform used by sub2api. It intentionally returns false for ambiguous model
// names so composite groups fail closed instead of guessing.
//
// ollama_cloud（与 opencode_go 同理）刻意不在本表：本函数只有模型名入参、没有
// host 入参，而 ollama 转售的模型家族（gpt-oss / kimi-k2 / glm / deepseek /
// qwen）与上方前缀规则直接冲突（如 gpt- 已映射 openai）——在这里加 ollama
// 分支等于按名字前缀静默错投。ollama_cloud 进入 composite 只走显式 route 与
// ownership resolver（账号目录），不要给本函数加 ollama 分支。
func DetectModelPlatform(model string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return "", false
	}

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
		case "kimi", "moonshot":
			return PlatformKimi, true
		case "zhipu", "glm", "bigmodel":
			return PlatformZhipu, true
		case "deepseek":
			return PlatformDeepseek, true
		case "minimax":
			return PlatformMiniMax, true
		}
		if rest != "" {
			normalized = strings.TrimPrefix(rest, "models/")
		}
	}

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
	case normalized == "k3",
		normalized == "k3-256k",
		strings.HasPrefix(normalized, "kimi-"),
		strings.HasPrefix(normalized, "moonshot-"):
		return PlatformKimi, true
	case strings.HasPrefix(normalized, "glm-"):
		return PlatformZhipu, true
	case strings.HasPrefix(normalized, "deepseek-"):
		return PlatformDeepseek, true
	case strings.HasPrefix(normalized, "minimax-"),
		strings.HasPrefix(normalized, "abab5"),
		strings.HasPrefix(normalized, "abab6"),
		strings.HasPrefix(normalized, "abab7"):
		return PlatformMiniMax, true
	default:
		return "", false
	}
}

func hasOpenAISeriesPrefix(model string) bool {
	for _, prefix := range []string{"o1", "o3", "o4", "o5"} {
		if model == prefix || strings.HasPrefix(model, prefix+"-") {
			return true
		}
	}
	return false
}

func (s *GatewayService) resolveCompositeRouteDecision(ctx context.Context, group *Group, requestedModel, endpoint string) (CompositeRouteDecision, bool, error) {
	if group == nil || group.Platform != PlatformComposite {
		return CompositeRouteDecision{}, false, nil
	}
	if platform, ok := ResolvedTargetPlatformFromContext(ctx); ok {
		upstreamModel := requestedModel
		if resolvedModel, modelOK := ResolvedUpstreamModelFromContext(ctx); modelOK {
			upstreamModel = resolvedModel
		}
		source := CompositeRouteSourceDetector
		if resolvedSource, sourceOK := CompositeRouteSourceFromContext(ctx); sourceOK {
			source = resolvedSource
		}
		return CompositeRouteDecision{
			Matched:        true,
			Source:         source,
			GroupID:        group.ID,
			PublicModel:    requestedModel,
			TargetPlatform: platform,
			UpstreamModel:  upstreamModel,
			Endpoint:       normalizeCompositeRouteEndpoint(endpoint),
		}, true, nil
	}
	// 已解析的池决策优先于重新解析：避免因 TargetPlatform 为空被误报 unknown
	// 或折叠成 detector 单平台。显式 pin（ResolvedTargetPlatform）仍优先于池，
	// 保持既有 single 行为（如 gemini 端点 fallback pin）。
	if candidates := CompositeCandidatePlatformsFromContext(ctx); len(candidates) > 0 {
		source := CompositeRouteSourceAccountPool
		if resolvedSource, sourceOK := CompositeRouteSourceFromContext(ctx); sourceOK && resolvedSource != "" {
			source = resolvedSource
		}
		return CompositeRouteDecision{
			Matched:            true,
			Source:             source,
			GroupID:            group.ID,
			PublicModel:        requestedModel,
			CandidatePlatforms: candidates,
			Endpoint:           normalizeCompositeRouteEndpoint(endpoint),
		}, true, nil
	}
	decision, err := s.compositeResolver.Resolve(ctx, group.ID, requestedModel, endpoint)
	if err != nil {
		return decision, false, err
	}
	return decision, decision.Matched, nil
}

func isConcreteRequestPlatform(platform string) bool {
	switch platform {
	case PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok,
		PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo, PlatformOllamaCloud:
		return true
	default:
		return false
	}
}
