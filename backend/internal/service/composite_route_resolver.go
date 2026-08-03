package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type CompositeRouteResolver struct {
	repo CompositeModelRouteRepository
}

func NewCompositeRouteResolver(repo CompositeModelRouteRepository) *CompositeRouteResolver {
	return &CompositeRouteResolver{repo: repo}
}

func (r *CompositeRouteResolver) Resolve(ctx context.Context, groupID int64, model, endpoint string) (CompositeRouteDecision, error) {
	return r.ResolveWithOptions(ctx, groupID, model, endpoint, CompositeRouteResolveOptions{})
}

// ResolveWithOptions resolves a composite route with group-level options applied.
//
// Precedence is: explicit rules first (ranked by matchCompositeRoute, whose
// comparator is intentionally left untouched by endpoint-default routing), then
// the built-in model detector, then the endpoint default when the group opted
// in.
//
// The endpoint default deliberately runs last. It infers a platform from the
// protocol the caller spoke rather than from the model, so letting it outrank
// the detector would send a `gpt-*` request arriving on /v1/messages to
// Anthropic. Running it last also makes the flag purely additive: it can only
// produce a decision where resolution previously failed outright, so enabling
// it never retargets traffic that already worked.
//
// The `images` endpoint has no built-in default on purpose, because image
// models are cross-provider.
func (r *CompositeRouteResolver) ResolveWithOptions(ctx context.Context, groupID int64, model, endpoint string, options CompositeRouteResolveOptions) (CompositeRouteDecision, error) {
	model = strings.TrimSpace(model)
	endpoint = normalizeCompositeRouteEndpoint(endpoint)
	decision := CompositeRouteDecision{
		GroupID:     groupID,
		PublicModel: model,
		Endpoint:    endpoint,
	}
	if model == "" {
		decision.Reason = "model is required"
		return decision, nil
	}

	if r != nil && r.repo != nil && groupID > 0 {
		routes, err := r.repo.ListByGroup(ctx, groupID, false)
		if err != nil {
			return decision, fmt.Errorf("list composite routes: %w", err)
		}
		if route, ok := matchCompositeRoute(routes, model, endpoint); ok {
			upstreamModel := strings.TrimSpace(route.UpstreamModel)
			if upstreamModel == "" {
				upstreamModel = model
			}
			return CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceExplicit,
				GroupID:        groupID,
				PublicModel:    model,
				TargetPlatform: route.TargetPlatform,
				UpstreamModel:  upstreamModel,
				Endpoint:       endpoint,
				Route:          &route,
			}, nil
		}
	}

	if platform, ok := DetectModelPlatform(model); ok {
		return CompositeRouteDecision{
			Matched:        true,
			Source:         CompositeRouteSourceDetector,
			GroupID:        groupID,
			PublicModel:    model,
			TargetPlatform: platform,
			UpstreamModel:  model,
			Endpoint:       endpoint,
		}, nil
	}

	if options.EndpointDefaultRoutingEnabled {
		if platform, ok := compositeEndpointDefaultPlatform(endpoint); ok {
			return CompositeRouteDecision{
				Matched:        true,
				Source:         CompositeRouteSourceEndpointDefault,
				GroupID:        groupID,
				PublicModel:    model,
				TargetPlatform: platform,
				UpstreamModel:  model,
				Endpoint:       endpoint,
			}, nil
		}
	}

	decision.Reason = "no explicit route, built-in detector, or endpoint default match"
	return decision, nil
}

// compositeEndpointDefaultPlatform maps a normalized composite endpoint to the
// provider that serves it natively. `images` and `any` are deliberately absent:
// image models are cross-provider, and `any` means the caller's endpoint could
// not be identified, so both must keep falling through to the detector.
func compositeEndpointDefaultPlatform(endpoint string) (string, bool) {
	switch normalizeCompositeRouteEndpoint(endpoint) {
	case CompositeRouteEndpointMessages, CompositeRouteEndpointCountTokens:
		return PlatformAnthropic, true
	case CompositeRouteEndpointResponses, CompositeRouteEndpointChatCompletions, CompositeRouteEndpointEmbeddings:
		return PlatformOpenAI, true
	case CompositeRouteEndpointGemini:
		return PlatformGemini, true
	default:
		return "", false
	}
}

func matchCompositeRoute(routes []CompositeModelRoute, model, endpoint string) (CompositeModelRoute, bool) {
	if len(routes) == 0 {
		return CompositeModelRoute{}, false
	}

	type candidate struct {
		route          CompositeModelRoute
		matchStrength  int
		endpointWeight int
		prefixLen      int
	}
	candidates := make([]candidate, 0, len(routes))
	for _, route := range routes {
		route.Endpoint = normalizeCompositeRouteEndpoint(route.Endpoint)
		if route.Endpoint != endpoint && route.Endpoint != CompositeRouteEndpointAny {
			continue
		}
		route.MatchType = normalizeCompositeRouteMatchType(route.MatchType)
		publicModel := strings.TrimSpace(route.PublicModel)
		if publicModel == "" {
			continue
		}

		matchStrength := 0
		prefixLen := len(publicModel)
		switch route.MatchType {
		case CompositeRouteMatchExact:
			if publicModel != model {
				continue
			}
			matchStrength = 2
		case CompositeRouteMatchPrefix:
			if !strings.HasPrefix(model, publicModel) {
				continue
			}
			matchStrength = 1
		default:
			continue
		}
		endpointWeight := 0
		if route.Endpoint == endpoint {
			endpointWeight = 1
		}
		candidates = append(candidates, candidate{
			route:          route,
			matchStrength:  matchStrength,
			endpointWeight: endpointWeight,
			prefixLen:      prefixLen,
		})
	}
	if len(candidates) == 0 {
		return CompositeModelRoute{}, false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.matchStrength != b.matchStrength {
			return a.matchStrength > b.matchStrength
		}
		if a.endpointWeight != b.endpointWeight {
			return a.endpointWeight > b.endpointWeight
		}
		if a.prefixLen != b.prefixLen {
			return a.prefixLen > b.prefixLen
		}
		if a.route.Priority != b.route.Priority {
			return a.route.Priority < b.route.Priority
		}
		return a.route.ID < b.route.ID
	})
	return candidates[0].route, true
}
