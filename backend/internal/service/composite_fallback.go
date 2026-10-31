package service

import (
	"context"
	"fmt"
	"strings"
)

type compositeFallbackContextKey struct{}

func compositeFallbackModelClass(platform, model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch platform {
	case PlatformOpenCodeGo:
		switch model {
		case "deepseek-v4.1-flash", "deepseek-v4-flash":
			return "deepseek-flash"
		case "deepseek-v4-pro":
			return "deepseek-pro"
		case "minimax-m2.5", "minimax-m2.7", "minimax-m3":
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
	}
	return ""
}

func validateCompositeFallback(input CompositeRouteInput) error {
	if len(input.FallbackTargets) == 0 {
		return nil
	}
	if len(input.FallbackTargets) != 1 || input.MatchType != CompositeRouteMatchExact {
		return fmt.Errorf("fallback_targets requires an exact route and at most one backup")
	}
	switch input.Endpoint {
	case CompositeRouteEndpointAny, CompositeRouteEndpointMessages, CompositeRouteEndpointResponses, CompositeRouteEndpointChatCompletions:
	default:
		return fmt.Errorf("fallback_targets only supports HTTP text endpoints")
	}
	primaryClass := compositeFallbackModelClass(input.TargetPlatform, input.UpstreamModel)
	backup := input.FallbackTargets[0]
	if primaryClass == "" || primaryClass != compositeFallbackModelClass(backup.TargetPlatform, backup.UpstreamModel) {
		return fmt.Errorf("fallback_targets must use an allowed equivalent paid model; Flash, Pro and Free cannot be interchanged")
	}
	validDirection := input.TargetPlatform == PlatformOpenCodeGo && backup.TargetPlatform == PlatformDeepseek && strings.HasPrefix(primaryClass, "deepseek-") ||
		input.TargetPlatform == PlatformMiniMax && backup.TargetPlatform == PlatformOpenCodeGo && strings.HasPrefix(primaryClass, "minimax-")
	if !validDirection {
		return fmt.Errorf("fallback_targets only allows paid DeepSeek Go to official DeepSeek, or official MiniMax to Go")
	}
	public := strings.ToLower(input.PublicModel)
	if strings.Contains(public, "free") || strings.Contains(public, "*") || strings.Contains(public, "flash") && primaryClass != "deepseek-flash" || strings.Contains(public, "pro") && strings.HasPrefix(public, "deepseek") && primaryClass != "deepseek-pro" {
		return fmt.Errorf("public_model cannot cross free or model tier boundaries")
	}
	if strings.TrimSpace(backup.UpstreamModel) != backup.UpstreamModel || strings.TrimSpace(backup.TargetPlatform) != backup.TargetPlatform {
		return fmt.Errorf("fallback target identifiers must not contain surrounding whitespace")
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
	reason := "Ordered HTTP text targets; switch only on unavailable pools or safe pre-output failures. WS, Live and media do not use fallback."
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
