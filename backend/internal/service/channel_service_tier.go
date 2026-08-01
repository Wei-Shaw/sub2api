package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type OpenAICommercialServiceTier string

const (
	OpenAICommercialTierStandard OpenAICommercialServiceTier = "standard"
	OpenAICommercialTierPriority OpenAICommercialServiceTier = "priority"
	OpenAICommercialTierFlex     OpenAICommercialServiceTier = "flex"
)

type ChannelServiceTierOption struct {
	Enabled    bool    `json:"enabled"`
	Multiplier float64 `json:"multiplier"`
}

type ChannelServiceTierConfig struct {
	Standard                  ChannelServiceTierOption `json:"standard"`
	Priority                  ChannelServiceTierOption `json:"priority"`
	Flex                      ChannelServiceTierOption `json:"flex"`
	UseOutboundTierForBilling bool                     `json:"use_outbound_tier_for_billing"`
}

func (c *ChannelServiceTierConfig) UnmarshalJSON(data []byte) error {
	var wire struct {
		Standard                  ChannelServiceTierOption `json:"standard"`
		Priority                  ChannelServiceTierOption `json:"priority"`
		Flex                      ChannelServiceTierOption `json:"flex"`
		UseOutboundTierForBilling *bool                    `json:"use_outbound_tier_for_billing"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*c = ChannelServiceTierConfig{
		Standard:                  wire.Standard,
		Priority:                  wire.Priority,
		Flex:                      wire.Flex,
		UseOutboundTierForBilling: true,
	}
	if wire.UseOutboundTierForBilling != nil {
		c.UseOutboundTierForBilling = *wire.UseOutboundTierForBilling
	}
	return nil
}

type ChannelServiceTierSnapshot struct {
	ChannelID      int64
	GroupID        int64
	ChannelName    string
	Config         ChannelServiceTierConfig
	ConfigRevision string
}

type ChannelServiceTierAuditSnapshot struct {
	Before ChannelServiceTierConfig
	After  ChannelServiceTierConfig
}

func cloneChannelServiceTierSnapshot(snapshot *ChannelServiceTierSnapshot) *ChannelServiceTierSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	return &clone
}

func DefaultChannelServiceTierConfig() ChannelServiceTierConfig {
	return ChannelServiceTierConfig{
		Standard:                  ChannelServiceTierOption{Enabled: true, Multiplier: 1},
		Priority:                  ChannelServiceTierOption{Enabled: true, Multiplier: 2},
		Flex:                      ChannelServiceTierOption{Enabled: true, Multiplier: 0.5},
		UseOutboundTierForBilling: true,
	}
}

func (c ChannelServiceTierConfig) IsZero() bool {
	return c == (ChannelServiceTierConfig{})
}

func (c ChannelServiceTierConfig) Validate() error {
	if !c.Standard.Enabled && !c.Priority.Enabled && !c.Flex.Enabled {
		return fmt.Errorf("at least one OpenAI service tier must be enabled")
	}
	for name, option := range map[string]ChannelServiceTierOption{
		"standard": c.Standard,
		"priority": c.Priority,
		"flex":     c.Flex,
	} {
		if math.IsNaN(option.Multiplier) || math.IsInf(option.Multiplier, 0) || option.Multiplier < 0.01 || option.Multiplier > 100 {
			return fmt.Errorf("%s service tier multiplier must be between 0.01 and 100", name)
		}
	}
	return nil
}

func (c ChannelServiceTierConfig) OptionForTier(tier OpenAICommercialServiceTier) (ChannelServiceTierOption, bool) {
	switch tier {
	case OpenAICommercialTierStandard:
		return c.Standard, true
	case OpenAICommercialTierPriority:
		return c.Priority, true
	case OpenAICommercialTierFlex:
		return c.Flex, true
	default:
		return ChannelServiceTierOption{}, false
	}
}

// NormalizeOpenAICommercialTier maps protocol values to the three commercial
// tiers. The caller must preserve the original protocol value for upstream:
// scale is billed as Standard but is still sent as scale.
func NormalizeOpenAICommercialTier(raw string) (OpenAICommercialServiceTier, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto", "default", "scale", "standard":
		return OpenAICommercialTierStandard, true
	case "fast", "priority":
		return OpenAICommercialTierPriority, true
	case "flex":
		return OpenAICommercialTierFlex, true
	default:
		return "", false
	}
}

func ValidateNativeGrokServiceTier(raw string) *OpenAIFastBlockedError {
	protocolTier := strings.ToLower(strings.TrimSpace(raw))
	switch protocolTier {
	case "", "auto", "default", "standard":
		return nil
	default:
		return &OpenAIFastBlockedError{
			Code:    "SERVICE_TIER_UNSUPPORTED_FOR_PLATFORM",
			Message: fmt.Sprintf("service_tier=%s is not supported by the native Grok platform", protocolTier),
		}
	}
}

func classifyOpenAIServiceTierMismatch(outbound, actual OpenAICommercialServiceTier) string {
	performanceRank := func(tier OpenAICommercialServiceTier) (int, bool) {
		switch tier {
		case OpenAICommercialTierFlex:
			return 0, true
		case OpenAICommercialTierStandard:
			return 1, true
		case OpenAICommercialTierPriority:
			return 2, true
		default:
			return 0, false
		}
	}
	outboundRank, outboundKnown := performanceRank(outbound)
	actualRank, actualKnown := performanceRank(actual)
	if !outboundKnown || !actualKnown || outboundRank == actualRank {
		return "changed"
	}
	if actualRank < outboundRank {
		return "degraded"
	}
	return "upgraded"
}

func applyChannelServiceTierRateMultiplier(
	rateMultiplier float64,
	protocolTier string,
	snapshot *ChannelServiceTierSnapshot,
) (float64, string) {
	if snapshot == nil {
		return rateMultiplier, protocolTier
	}
	tier, ok := NormalizeOpenAICommercialTier(protocolTier)
	if !ok {
		tier = OpenAICommercialTierStandard
	}
	option, ok := snapshot.Config.OptionForTier(tier)
	if !ok {
		return rateMultiplier, ""
	}
	return rateMultiplier * option.Multiplier, ""
}

func (c *Channel) normalizeServiceTierConfig() {
	if c == nil || c.ServiceTierConfigError != "" {
		return
	}
	if c.ServiceTierConfig.IsZero() {
		c.ServiceTierConfig = DefaultChannelServiceTierConfig()
	}
}
