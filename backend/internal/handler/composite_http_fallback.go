package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type compositeHTTPFallback struct {
	decision service.CompositeRouteDecision
	used     bool
	baseline int
}

func newCompositeHTTPFallback(c *gin.Context, body []byte, disabled bool) *compositeHTTPFallback {
	if c == nil || c.Request == nil || disabled {
		return nil
	}
	decision, ok := service.CompositeFallbackDecisionFromContext(c.Request.Context())
	if !ok || len(decision.FallbackTargets) != 1 {
		return nil
	}
	for _, field := range []string{"previous_response_id", "conversation", "cached_content", "cachedContent"} {
		if value := gjson.GetBytes(body, field); value.Exists() && value.Type != gjson.Null && value.String() != "" {
			return nil
		}
	}
	return &compositeHTTPFallback{decision: decision, baseline: service.OpenAIStreamKeepaliveAdjustedWrittenSize(c)}
}

func safeCompositeFallbackFailure(err *service.UpstreamFailoverError) bool {
	if err == nil {
		return true
	}
	if !err.ShouldRetryNextAccount() || err.BoundUpstreamPassthrough || err.Scope == service.GatewayFailureScopeRequest {
		return false
	}
	for _, path := range []string{"error.type", "error.code", "response.error.type", "response.error.code"} {
		switch strings.ToLower(gjson.GetBytes(err.ResponseBody, path).String()) {
		case "content_policy_violation", "content_filter", "safety", "permission_error", "invalid_request_error", "model_not_found", "openai_silent_refusal":
			return false
		}
	}
	return err.StatusCode == 0 || err.StatusCode == http.StatusRequestTimeout || err.StatusCode == http.StatusTooManyRequests || err.StatusCode >= 500 && err.StatusCode <= 599 || err.StatusCode == http.StatusUnauthorized && err.Scope != service.GatewayFailureScopeProvider || err.IsCredentialFailure() && err.StatusCode == http.StatusForbidden
}

func (f *compositeHTTPFallback) advance(c *gin.Context, selectionErr error, last *service.UpstreamFailoverError) bool {
	if f == nil || f.used || failoverClientGone(c) || service.OpenAIStreamKeepaliveAdjustedWrittenSize(c) > f.baseline || !safeCompositeFallbackFailure(last) {
		return false
	}
	if selectionErr != nil {
		if !errors.Is(selectionErr, service.ErrNoAvailableAccounts) {
			return false
		}
		message := strings.ToLower(selectionErr.Error())
		for _, denied := range []string{"channel pricing restriction", "channel_upstream_restricted", "composite_fallback_model_restricted"} {
			if strings.Contains(message, denied) {
				return false
			}
		}
	}
	f.used = true
	target := f.decision.FallbackTargets[0]
	decision := f.decision
	decision.TargetPlatform, decision.UpstreamModel = target.TargetPlatform, target.UpstreamModel
	c.Request = c.Request.WithContext(service.WithCompositeRouteDecision(c.Request.Context(), decision))
	return true
}

func (h *OpenAIGatewayHandler) prepareCompositeHTTPTarget(c *gin.Context, apiKey *service.APIKey, subscription *service.UserSubscription, body []byte) (string, service.ChannelMappingResult, []byte, bool, error) {
	decision, active := service.CompositeFallbackDecisionFromContext(c.Request.Context())
	if !active {
		return "", service.ChannelMappingResult{}, body, false, nil
	}
	if err := h.billingCacheService.RecheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		return "", service.ChannelMappingResult{}, body, true, err
	}
	mapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, decision.UpstreamModel)
	model := openAIChannelForwardModel(mapping, decision.UpstreamModel)
	if err := service.ValidateCompositeFallbackModel(c.Request.Context(), decision.TargetPlatform, model); err != nil {
		return "", mapping, body, true, fmt.Errorf("invalid fallback channel mapping: %w", err)
	}
	return model, mapping, h.gatewayService.ReplaceModelInBody(body, model), true, nil
}

func estimateCompositeInflightReservation(ctx context.Context, estimator inflightReservationEstimator, apiKey *service.APIKey, req service.InflightEstimateRequest) (float64, bool) {
	estimate, priced := estimator.EstimateInflightReservation(ctx, apiKey, req)
	if decision, ok := service.CompositeFallbackDecisionFromContext(ctx); ok && req.Kind == service.InflightEstimateToken {
		for _, target := range decision.FallbackTargets {
			targetDecision := decision
			targetDecision.TargetPlatform, targetDecision.UpstreamModel = target.TargetPlatform, target.UpstreamModel
			targetReq := req
			targetReq.Model = target.UpstreamModel
			targetEstimate, targetPriced := estimator.EstimateInflightReservation(service.WithCompositeRouteDecision(ctx, targetDecision), apiKey, targetReq)
			estimate = max(estimate, targetEstimate)
			priced = priced && targetPriced
		}
	}
	return estimate, priced
}
