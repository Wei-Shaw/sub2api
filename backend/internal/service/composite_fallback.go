package service

import (
	"context"
	"fmt"
	"strings"
)

type compositeFallbackContextKey struct{}

const compositeFallbackMaxTargets = 2

func compositeFallbackModelClass(platform, model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch platform {
	case PlatformOpenAI:
		switch model {
		case "deepseek-v4.1-flash", "deepseek-v4-flash", "deepseek-flash":
			return "deepseek-flash"
		case "deepseek-v4-pro":
			return "deepseek-pro"
		case "minimax-m2.5", "minimax-m2.7", "minimax-m3", "glm-5.3", "glm-5.3-flash":
			return model
		}
	case PlatformOpenCodeGo:
		switch model {
		case "deepseek-v4.1-flash", "deepseek-v4-flash":
			return "deepseek-flash"
		case "deepseek-v4-pro":
			return "deepseek-pro"
		case "minimax-m2.5", "minimax-m2.7", "minimax-m3", "glm-5.3", "glm-5.3-flash":
			return model
		}
	case PlatformDeepseek:
		switch model {
		case "deepseek-flash", "deepseek-v4-flash":
			return "deepseek-flash"
		case "deepseek-v4-pro":
			return "deepseek-pro"
		}
	case PlatformMiniMax:
		switch model {
		case "minimax-m2.5", "minimax-m2.7", "minimax-m3":
			return model
		}
	case PlatformZhipu:
		switch model {
		case "glm-5.3", "glm-5.3-flash":
			return model
		}
	}
	return ""
}

func compositeFallbackPlatforms(modelClass string) []string {
	switch {
	case strings.HasPrefix(modelClass, "deepseek-"):
		return []string{PlatformOpenCodeGo, PlatformDeepseek, PlatformOpenAI}
	case strings.HasPrefix(modelClass, "minimax-"):
		return []string{PlatformMiniMax, PlatformOpenCodeGo, PlatformOpenAI}
	case strings.HasPrefix(modelClass, "glm-"):
		return []string{PlatformZhipu, PlatformOpenCodeGo, PlatformOpenAI}
	default:
		return nil
	}
}

func validateCompositeFallback(input CompositeRouteInput) error {
	if len(input.FallbackTargets) == 0 {
		return nil
	}
	if input.MatchType != CompositeRouteMatchExact {
		return fmt.Errorf("fallback_targets requires an exact route")
	}
	if len(input.FallbackTargets) > compositeFallbackMaxTargets {
		return fmt.Errorf("fallback_targets supports at most %d ordered backups", compositeFallbackMaxTargets)
	}
	switch input.Endpoint {
	case CompositeRouteEndpointAny, CompositeRouteEndpointMessages, CompositeRouteEndpointResponses, CompositeRouteEndpointChatCompletions:
	default:
		return fmt.Errorf("fallback_targets only supports HTTP text endpoints")
	}
	primaryClass := compositeFallbackModelClass(input.TargetPlatform, input.UpstreamModel)
	allowedPlatforms := compositeFallbackPlatforms(primaryClass)
	if primaryClass == "" || len(allowedPlatforms) == 0 || input.TargetPlatform != allowedPlatforms[0] {
		return fmt.Errorf("primary target is not an allowed paid model/platform for fallback")
	}
	seenPlatforms := map[string]struct{}{input.TargetPlatform: {}}
	for i, target := range input.FallbackTargets {
		if target.TargetPlatform == "" || target.UpstreamModel == "" {
			return fmt.Errorf("fallback_targets[%d] requires platform and model", i)
		}
		if _, duplicate := seenPlatforms[target.TargetPlatform]; duplicate {
			return fmt.Errorf("fallback_targets[%d] duplicates a platform in the route", i)
		}
		if !stringSliceContains(allowedPlatforms, target.TargetPlatform) {
			return fmt.Errorf("fallback_targets[%d] platform %q is not allowed for %s", i, target.TargetPlatform, primaryClass)
		}
		if primaryClass != compositeFallbackModelClass(target.TargetPlatform, target.UpstreamModel) {
			return fmt.Errorf("fallback_targets must use the same paid model tier; Flash, Pro and Free cannot be interchanged")
		}
		if strings.TrimSpace(target.UpstreamModel) != target.UpstreamModel || strings.TrimSpace(target.TargetPlatform) != target.TargetPlatform {
			return fmt.Errorf("fallback target identifiers must not contain surrounding whitespace")
		}
		seenPlatforms[target.TargetPlatform] = struct{}{}
	}
	public := strings.ToLower(input.PublicModel)
	if strings.Contains(public, "free") || strings.Contains(public, "*") {
		return fmt.Errorf("public_model cannot cross free or model tier boundaries")
	}
	if strings.HasPrefix(primaryClass, "deepseek-") &&
		(strings.Contains(public, "flash") && primaryClass != "deepseek-flash" ||
			strings.Contains(public, "pro") && strings.HasPrefix(public, "deepseek") && primaryClass != "deepseek-pro") {
		return fmt.Errorf("public_model cannot cross free or model tier boundaries")
	}
	if strings.HasPrefix(primaryClass, "glm-") &&
		(strings.Contains(public, "flash") != strings.HasSuffix(primaryClass, "-flash") && strings.HasPrefix(public, "glm-")) {
		return fmt.Errorf("public_model cannot cross flash and non-flash tiers")
	}
	return nil
}

func CompositeFallbackDecisionFromContext(ctx context.Context) (CompositeRouteDecision, bool) {
	if ctx == nil {
		return CompositeRouteDecision{}, false
	}
	decision, ok := ctx.Value(compositeFallbackContextKey{}).(CompositeRouteDecision)
	return decision, ok && len(decision.FallbackTargets) > 0
}

func ValidateCompositeFallbackModel(ctx context.Context, platform, model string) error {
	decision, ok := CompositeFallbackDecisionFromContext(ctx)
	if !ok {
		return nil
	}
	if platform != decision.TargetPlatform || !strings.EqualFold(strings.TrimSpace(model), decision.UpstreamModel) {
		return fmt.Errorf("composite fallback target mapping changed the permitted platform or model")
	}
	return nil
}

func compositeFallbackResponseModel(ctx context.Context, model string) string {
	if decision, ok := CompositeFallbackDecisionFromContext(ctx); ok && decision.PublicModel != "" {
		return decision.PublicModel
	}
	return model
}

func compositeFallbackPreviewReason(route CompositeModelRoute) string {
	if len(route.FallbackTargets) == 0 {
		return ""
	}
	reason := "Ordered HTTP text target chain; switch only on unavailable pools or safe pre-output failures. WS, Live and media do not use fallback."
	if route.UpstreamModel == "deepseek-v4.1-flash" {
		reason += " Go v4.1 Flash and official Flash checkpoints are not verified identical."
	}
	return reason
}

func ValidateCompositeFallbackAccount(ctx context.Context, account *Account, model string) error {
	if _, ok := CompositeFallbackDecisionFromContext(ctx); !ok {
		return nil
	}
	if account == nil {
		return fmt.Errorf("composite fallback requires an account")
	}
	if err := ValidateCompositeFallbackModel(ctx, account.Platform, model); err != nil {
		return err
	}
	mapped := account.GetMappedModel(model)
	if err := ValidateCompositeFallbackModel(ctx, account.Platform, mapped); err != nil {
		return err
	}
	if account.Platform == PlatformOpenCodeGo && account.IsOpenCodeZen() {
		return fmt.Errorf("paid composite fallback cannot use a Zen account")
	}
	return nil
}
