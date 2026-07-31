package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIServiceTierSnapshotContextKey = "openai_service_tier_snapshot"
	openAIServiceTierStateContextKey    = "openai_service_tier_state"
)

type openAIServiceTierSnapshotLookup struct {
	Snapshot *ChannelServiceTierSnapshot
	Found    bool
}

type OpenAIServiceTierRequestState struct {
	Snapshot              *ChannelServiceTierSnapshot
	RequestedProtocolTier string
	OutboundProtocolTier  string
	RequestedTier         OpenAICommercialServiceTier
	OutboundTier          OpenAICommercialServiceTier
}

type openAIServiceTierBillingEvidence string

const (
	openAIServiceTierEvidenceUnavailable                openAIServiceTierBillingEvidence = "unavailable"
	openAIServiceTierEvidenceAPIResponseAuthoritative   openAIServiceTierBillingEvidence = "api_response_authoritative"
	openAIServiceTierEvidenceOAuthOutboundAuthoritative openAIServiceTierBillingEvidence = "oauth_outbound_authoritative"
)

type openAIServiceTierBillingDecision struct {
	ProtocolTier       string
	CommercialTier     OpenAICommercialServiceTier
	Snapshot           *ChannelServiceTierSnapshot
	Evidence           openAIServiceTierBillingEvidence
	ActualWasObserved  bool
	ActualWasUsed      bool
	ActualWasUnknown   bool
	ActualProtocolTier string
}

func cloneOpenAIServiceTierRequestState(state *OpenAIServiceTierRequestState) *OpenAIServiceTierRequestState {
	if state == nil {
		return nil
	}
	clone := *state
	clone.Snapshot = cloneChannelServiceTierSnapshot(state.Snapshot)
	return &clone
}

func OpenAIServiceTierStateFromContext(c interface{ Get(string) (any, bool) }) *OpenAIServiceTierRequestState {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIServiceTierStateContextKey)
	if !ok {
		return nil
	}
	state, _ := value.(*OpenAIServiceTierRequestState)
	return cloneOpenAIServiceTierRequestState(state)
}

func optionalOpenAIProtocolTier(raw string) *string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func resolveOpenAIServiceTierBillingDecision(
	result *OpenAIForwardResult,
	state *OpenAIServiceTierRequestState,
	account *Account,
) openAIServiceTierBillingDecision {
	decision := openAIServiceTierBillingDecision{
		CommercialTier: OpenAICommercialTierStandard,
		Evidence:       openAIServiceTierEvidenceUnavailable,
	}
	hasOutboundEvidence := false
	if state != nil {
		hasOutboundEvidence = true
		decision.Snapshot = cloneChannelServiceTierSnapshot(state.Snapshot)
		decision.ProtocolTier = state.OutboundProtocolTier
		decision.CommercialTier = state.OutboundTier
		if result != nil {
			if result.RequestedServiceTier == nil {
				result.RequestedServiceTier = optionalOpenAIProtocolTier(state.RequestedProtocolTier)
			}
			if result.OutboundServiceTier == nil {
				result.OutboundServiceTier = optionalOpenAIProtocolTier(state.OutboundProtocolTier)
			}
		}
	}

	if result == nil {
		if hasOutboundEvidence && account != nil && account.IsOpenAIOAuth() {
			decision.Evidence = openAIServiceTierEvidenceOAuthOutboundAuthoritative
		}
		return decision
	}
	if result.OutboundServiceTier != nil && state == nil {
		hasOutboundEvidence = true
		decision.ProtocolTier = strings.TrimSpace(*result.OutboundServiceTier)
		if tier, ok := NormalizeOpenAICommercialTier(decision.ProtocolTier); ok {
			decision.CommercialTier = tier
		}
	} else if state == nil && result.ServiceTier != nil {
		hasOutboundEvidence = true
		decision.ProtocolTier = strings.TrimSpace(*result.ServiceTier)
		if tier, ok := NormalizeOpenAICommercialTier(decision.ProtocolTier); ok {
			decision.CommercialTier = tier
		}
	}

	if result.ActualServiceTier != nil {
		actual := strings.ToLower(strings.TrimSpace(*result.ActualServiceTier))
		decision.ActualProtocolTier = actual
		decision.ActualWasObserved = actual != ""
		if _, ok := NormalizeOpenAICommercialTier(actual); !ok && actual != "" {
			decision.ActualWasUnknown = true
		}

		// ChatGPT OAuth's response.service_tier describes its internal routing and
		// commonly reports default even when Codex Fast sent priority. The outbound
		// request is therefore the authoritative billing evidence for OAuth.
		if hasOutboundEvidence && account != nil && account.IsOpenAIOAuth() {
			decision.Evidence = openAIServiceTierEvidenceOAuthOutboundAuthoritative
		} else if tier, ok := NormalizeOpenAICommercialTier(actual); ok {
			decision.ProtocolTier = actual
			decision.CommercialTier = tier
			decision.ActualWasUsed = true
			decision.Evidence = openAIServiceTierEvidenceAPIResponseAuthoritative
		}
	} else if hasOutboundEvidence && account != nil && account.IsOpenAIOAuth() {
		decision.Evidence = openAIServiceTierEvidenceOAuthOutboundAuthoritative
	}
	billingTier := string(decision.CommercialTier)
	result.BillingServiceTier = &billingTier
	return decision
}

func (s *OpenAIGatewayService) openAIServiceTierSnapshotForRequest(ctx context.Context, c *gin.Context) (*ChannelServiceTierSnapshot, error) {
	if c == nil || s == nil || s.channelService == nil {
		return nil, nil
	}
	if value, ok := c.Get(openAIServiceTierSnapshotContextKey); ok {
		lookup, _ := value.(openAIServiceTierSnapshotLookup)
		return lookup.Snapshot, nil
	}
	return s.refreshOpenAIServiceTierSnapshot(ctx, c)
}

// refreshOpenAIServiceTierSnapshot starts a new request/turn snapshot. HTTP
// requests use the cached wrapper above; each WebSocket response.create is a
// separate billable turn and must observe the channel cache's freshness rules.
func (s *OpenAIGatewayService) refreshOpenAIServiceTierSnapshot(ctx context.Context, c *gin.Context) (*ChannelServiceTierSnapshot, error) {
	if c == nil || s == nil || s.channelService == nil {
		return nil, nil
	}
	groupID := getOpenAIGroupIDFromContext(c)
	if groupID <= 0 {
		c.Set(openAIServiceTierSnapshotContextKey, openAIServiceTierSnapshotLookup{})
		return nil, nil
	}
	snapshot, found, err := s.channelService.GetServiceTierSnapshotForGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	c.Set(openAIServiceTierSnapshotContextKey, openAIServiceTierSnapshotLookup{Snapshot: snapshot, Found: found})
	return snapshot, nil
}

func openAIProtocolTier(body []byte) string {
	return strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "service_tier").String()))
}

func enforceOpenAIServiceTierForAccount(
	snapshot *ChannelServiceTierSnapshot,
	requestedBody []byte,
	outboundBody []byte,
	account *Account,
) (*OpenAIServiceTierRequestState, error) {
	return enforceOpenAIServiceTierValues(snapshot, openAIProtocolTier(requestedBody), openAIProtocolTier(outboundBody), account)
}

func enforceOpenAIServiceTierValues(
	snapshot *ChannelServiceTierSnapshot,
	requestedProtocol string,
	outboundProtocol string,
	account *Account,
) (*OpenAIServiceTierRequestState, error) {
	requestedProtocol = strings.ToLower(strings.TrimSpace(requestedProtocol))
	outboundProtocol = strings.ToLower(strings.TrimSpace(outboundProtocol))
	requestedTier, requestedKnown := NormalizeOpenAICommercialTier(requestedProtocol)
	if !requestedKnown {
		requestedTier = OpenAICommercialTierStandard
	}
	outboundTier, outboundKnown := NormalizeOpenAICommercialTier(outboundProtocol)
	if !outboundKnown {
		outboundTier = OpenAICommercialTierStandard
	}

	state := &OpenAIServiceTierRequestState{
		Snapshot:              snapshot,
		RequestedProtocolTier: requestedProtocol,
		OutboundProtocolTier:  outboundProtocol,
		RequestedTier:         requestedTier,
		OutboundTier:          outboundTier,
	}

	if snapshot != nil {
		option, ok := snapshot.Config.OptionForTier(outboundTier)
		if !ok {
			return state, fmt.Errorf("unknown OpenAI commercial service tier %q", outboundTier)
		}
		if !option.Enabled {
			return state, &OpenAIFastBlockedError{
				Code:    "CHANNEL_SERVICE_TIER_NOT_ALLOWED",
				Message: fmt.Sprintf("channel %s does not allow OpenAI service tier %s", snapshot.ChannelName, outboundTier),
			}
		}
	}
	if account != nil && account.Platform == PlatformGrok {
		if blocked := ValidateNativeGrokServiceTier(outboundProtocol); blocked != nil {
			return state, blocked
		}
	}
	return state, nil
}

func (s *OpenAIGatewayService) applyOpenAIFastAndChannelPolicyToBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	model string,
	body []byte,
) ([]byte, error) {
	snapshot, err := s.openAIServiceTierSnapshotForRequest(ctx, c)
	if err != nil {
		return body, fmt.Errorf("load channel service tier configuration: %w", err)
	}
	updated, err := s.applyOpenAIFastPolicyToBody(ctx, account, model, body)
	if err != nil {
		return body, err
	}
	state, err := enforceOpenAIServiceTierForAccount(snapshot, body, updated, account)
	if state != nil && c != nil {
		c.Set(openAIServiceTierStateContextKey, state)
	}
	if err != nil {
		return body, err
	}
	return updated, nil
}

func (s *OpenAIGatewayService) applyOpenAIChannelPolicyToFinalBody(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	requestedBody []byte,
	outboundBody []byte,
) error {
	snapshot, err := s.openAIServiceTierSnapshotForRequest(ctx, c)
	if err != nil {
		return fmt.Errorf("load channel service tier configuration: %w", err)
	}
	state, err := enforceOpenAIServiceTierForAccount(snapshot, requestedBody, outboundBody, account)
	if state != nil && c != nil {
		c.Set(openAIServiceTierStateContextKey, state)
	}
	return err
}

func (s *OpenAIGatewayService) applyOpenAIFastAndChannelPolicyToWSResponseCreate(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	model string,
	frame []byte,
) ([]byte, *OpenAIFastBlockedError, error) {
	isResponseCreate := strings.TrimSpace(gjson.GetBytes(frame, "type").String()) == "response.create"
	if isResponseCreate && c != nil {
		c.Set(openAIServiceTierStateContextKey, (*OpenAIServiceTierRequestState)(nil))
	}
	updated, blocked, err := s.applyOpenAIFastPolicyToWSResponseCreate(ctx, account, model, frame)
	if err != nil || blocked != nil || !isResponseCreate {
		return updated, blocked, err
	}
	snapshot, err := s.refreshOpenAIServiceTierSnapshot(ctx, c)
	if err != nil {
		return frame, nil, fmt.Errorf("load channel service tier configuration: %w", err)
	}
	state, tierErr := enforceOpenAIServiceTierForAccount(snapshot, frame, updated, account)
	if state != nil && c != nil {
		c.Set(openAIServiceTierStateContextKey, state)
	}
	if tierErr != nil {
		var tierBlocked *OpenAIFastBlockedError
		if errors.As(tierErr, &tierBlocked) {
			return frame, tierBlocked, nil
		}
		return frame, nil, tierErr
	}
	return updated, nil, nil
}

func (s *OpenAIGatewayService) applyOpenAIChannelAndPlatformPolicyToTier(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	protocolTier string,
) error {
	snapshot, err := s.openAIServiceTierSnapshotForRequest(ctx, c)
	if err != nil {
		return fmt.Errorf("load channel service tier configuration: %w", err)
	}
	state, err := enforceOpenAIServiceTierValues(snapshot, protocolTier, protocolTier, account)
	if state != nil && c != nil {
		c.Set(openAIServiceTierStateContextKey, state)
	}
	return err
}
