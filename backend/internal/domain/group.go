package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// Group-domain errors used by persistence and application layers.
var (
	ErrGroupNotFound = infraerrors.NotFound("GROUP_NOT_FOUND", "group not found")
	ErrGroupExists   = infraerrors.Conflict("GROUP_EXISTS", "group name already exists")
)

// AccountGroupLink is the account↔group membership edge without nested aggregates.
// Full Account/Group nesting remains application-layer concern until those BCs land.
type AccountGroupLink struct {
	AccountID int64
	GroupID   int64
	Priority  int
	CreatedAt time.Time
}

// Group is a routing/billing group aggregate.
type Group struct {
	ID             int64
	Name           string
	Description    string
	Platform       string
	RateMultiplier float64
	// Peak-rate: when PeakRateEnabled and current time is in [PeakStart, PeakEnd),
	// token billing multiplies by PeakRateMultiplier. See PeakMultiplierAt.
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsExclusive        bool
	Status             string
	// Hydrated indicates the group was loaded from a trusted repository source.
	Hydrated bool
	// DuplicateOperationID is internal persistence metadata used only to recover
	// an already committed one-click copy. It must never be mapped to API DTOs.
	DuplicateOperationID string

	SubscriptionType    string
	DailyLimitUSD       *float64
	WeeklyLimitUSD      *float64
	MonthlyLimitUSD     *float64
	DefaultValidityDays int

	// Image generation pricing (antigravity / gemini platforms).
	AllowImageGeneration         bool
	AllowBatchImageGeneration    bool
	ImageRateIndependent         bool
	ImageRateMultiplier          float64
	ImagePrice1K                 *float64
	ImagePrice2K                 *float64
	ImagePrice4K                 *float64
	BatchImageDiscountMultiplier float64
	BatchImageHoldMultiplier     float64
	VideoRateIndependent         bool
	VideoRateMultiplier          float64
	VideoPrice480P               *float64
	VideoPrice720P               *float64
	VideoPrice1080P              *float64
	// Codex alpha/search web-search unit price (USD/call, openai only).
	// nil means defaultWebSearchPricePerCall ($10/1000).
	WebSearchPricePerCall *float64

	// Claude Code client restriction.
	ClaudeCodeOnly  bool
	FallbackGroupID *int64
	// Invalid-request fallback group (anthropic only).
	FallbackGroupIDOnInvalidRequest *int64

	// Model routing: pattern (supports trailing *) -> preferred account IDs.
	ModelRouting        map[string][]int64
	ModelRoutingEnabled bool

	// MCP XML protocol injection (antigravity only).
	MCPXMLInject bool

	// Supported model families (antigravity only): claude, gemini_text, gemini_image.
	SupportedModelScopes []string

	// Display sort order.
	SortOrder int

	// OpenAI Messages dispatch config (openai only).
	AllowMessagesDispatch       bool
	RequireOAuthOnly            bool // only non-apikey accounts (OpenAI/Antigravity/Anthropic/Gemini)
	RequirePrivacySet           bool // only privacy-configured accounts
	DefaultMappedModel          string
	MessagesDispatchModelConfig OpenAIMessagesDispatchModelConfig
	ModelsListConfig            GroupModelsListConfig

	// RPMLimit is the group-level requests-per-minute cap (0 = unlimited).
	// When set it owns group user throttling (overrides user rpm_limit) and can
	// itself be overridden by user-group rpm_override.
	RPMLimit int

	// MaxReasoningEffort limits effective OpenAI/Codex reasoning effort.
	// Empty means unlimited; values: minimal/low/medium/high/xhigh/max.
	MaxReasoningEffort string
	// ReasoningEffortMappings rewrites explicit request values before the ceiling.
	ReasoningEffortMappings []ReasoningEffortMapping

	CreatedAt time.Time
	UpdatedAt time.Time

	// Optional projections populated by some list/detail paths.
	AccountGroups           []AccountGroupLink
	AccountCount            int64
	ActiveAccountCount      int64
	RateLimitedAccountCount int64
}

func (g *Group) IsActive() bool {
	return g != nil && g.Status == StatusActive
}

func (g *Group) IsSubscriptionType() bool {
	return g != nil && g.SubscriptionType == SubscriptionTypeSubscription
}

func (g *Group) HasDailyLimit() bool {
	return g != nil && g.DailyLimitUSD != nil && *g.DailyLimitUSD > 0
}

func (g *Group) HasWeeklyLimit() bool {
	return g != nil && g.WeeklyLimitUSD != nil && *g.WeeklyLimitUSD > 0
}

func (g *Group) HasMonthlyLimit() bool {
	return g != nil && g.MonthlyLimitUSD != nil && *g.MonthlyLimitUSD > 0
}

// GetImagePrice returns the configured image price for image_size, or nil for default.
func (g *Group) GetImagePrice(imageSize string) *float64 {
	if g == nil {
		return nil
	}
	switch imageSize {
	case "1K":
		return g.ImagePrice1K
	case "2K":
		return g.ImagePrice2K
	case "4K":
		return g.ImagePrice4K
	default:
		// Unknown size bills as 2K.
		return g.ImagePrice2K
	}
}

// GetVideoPrice returns the configured video price for resolution, or nil for default.
func (g *Group) GetVideoPrice(resolution string) *float64 {
	if g == nil {
		return nil
	}
	switch NormalizeVideoBillingResolutionOrDefault(resolution) {
	case VideoBillingResolution480P:
		return g.VideoPrice480P
	case VideoBillingResolution720P:
		return g.VideoPrice720P
	case VideoBillingResolution1080P:
		return g.VideoPrice1080P
	default:
		return g.VideoPrice480P
	}
}

// IsGroupContextValid reports whether a group from context has fields required for routing.
func IsGroupContextValid(group *Group) bool {
	if group == nil {
		return false
	}
	if group.ID <= 0 {
		return false
	}
	if !group.Hydrated {
		return false
	}
	if group.Platform == "" || group.Status == "" {
		return false
	}
	return true
}

// GetRoutingAccountIDs returns preferred account IDs for requestedModel, or nil.
func (g *Group) GetRoutingAccountIDs(requestedModel string) []int64 {
	if g == nil || !g.ModelRoutingEnabled || len(g.ModelRouting) == 0 || requestedModel == "" {
		return nil
	}

	// 1. Exact match first.
	if accountIDs, ok := g.ModelRouting[requestedModel]; ok && len(accountIDs) > 0 {
		return accountIDs
	}

	// 2. Trailing-wildcard match.
	for pattern, accountIDs := range g.ModelRouting {
		if MatchModelPattern(pattern, requestedModel) && len(accountIDs) > 0 {
			return accountIDs
		}
	}

	return nil
}

// MatchModelPattern supports trailing * wildcards (e.g. "claude-opus-*").
func MatchModelPattern(pattern, model string) bool {
	if pattern == model {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(model, prefix)
	}
	return false
}

// ParseMinutes parses "HH:MM" into minutes-of-day (0..1439).
// Hand-rolled (not time.Parse) for the per-request billing hot path.
// Accepts the same set as time.Parse("15:04", s).
func ParseMinutes(hhmm string) (int, bool) {
	colon := strings.IndexByte(hhmm, ':')
	if (colon != 1 && colon != 2) || len(hhmm)-colon-1 != 2 {
		return 0, false
	}
	h := 0
	for i := 0; i < colon; i++ {
		d := hhmm[i] - '0'
		if d > 9 {
			return 0, false
		}
		h = h*10 + int(d)
	}
	m1, m2 := hhmm[colon+1]-'0', hhmm[colon+2]-'0'
	if m1 > 9 || m2 > 9 {
		return 0, false
	}
	m := int(m1)*10 + int(m2)
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// PeakMultiplierAt returns the peak-rate factor for now.
// Disabled / misconfigured / off-peak → 1.0. Interval is half-open [start, end)
// within a single day (no overnight ranges).
func (g *Group) PeakMultiplierAt(now time.Time) float64 {
	if g == nil || !g.IsSubscriptionType() || !g.PeakRateEnabled || g.PeakStart == "" || g.PeakEnd == "" {
		return 1.0
	}
	start, ok1 := ParseMinutes(g.PeakStart)
	end, ok2 := ParseMinutes(g.PeakEnd)
	if !ok1 || !ok2 || start >= end {
		return 1.0
	}
	t := now.In(timezone.Location())
	cur := t.Hour()*60 + t.Minute()
	if cur >= start && cur < end {
		return g.PeakRateMultiplier
	}
	return 1.0
}

// ValidatePeakRateConfig is the single peak-rate validator for handlers and services.
// enabled=true requires subscription type, valid start/end with end>start, multiplier>=0.
func ValidatePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) error {
	if !enabled {
		return nil
	}
	if subscriptionType != SubscriptionTypeSubscription {
		return errors.New("高峰时段倍率仅支持订阅类型分组")
	}
	if start == "" || end == "" {
		return errors.New("peak_rate_enabled 为 true 时 peak_start 与 peak_end 必填")
	}
	st, okStart := ParseMinutes(start)
	if !okStart {
		return fmt.Errorf("peak_start 格式应为 HH:MM，got %q", start)
	}
	en, okEnd := ParseMinutes(end)
	if !okEnd {
		return fmt.Errorf("peak_end 格式应为 HH:MM，got %q", end)
	}
	if st >= en {
		return errors.New("peak_end 必须大于 peak_start（不支持跨天区间，如 22:00-02:00）")
	}
	if multiplier < 0 {
		return errors.New("peak_rate_multiplier 不能为负")
	}
	return nil
}

// NormalizePeakRateConfig normalizes peak-rate fields for persistence.
// Non-subscription groups always clear peak config. See original service docs.
func NormalizePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) (bool, string, string, float64) {
	if subscriptionType != SubscriptionTypeSubscription {
		return false, "", "", 1.0
	}
	if !enabled {
		if _, ok := ParseMinutes(start); !ok {
			start = ""
		}
		if _, ok := ParseMinutes(end); !ok {
			end = ""
		}
		if multiplier < 0 {
			multiplier = 1.0
		}
	}
	return enabled, start, end, multiplier
}

// NormalizeGroupModelsListConfig deduplicates and trims models list config.
func NormalizeGroupModelsListConfig(cfg GroupModelsListConfig) GroupModelsListConfig {
	out := GroupModelsListConfig{Enabled: cfg.Enabled}
	if len(cfg.Models) == 0 {
		return out
	}

	seen := make(map[string]struct{}, len(cfg.Models))
	out.Models = make([]string, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out.Models = append(out.Models, model)
	}
	if len(out.Models) == 0 {
		out.Models = nil
	}
	return out
}

// CustomModelsListEnabled reports whether a custom models list is active.
func (g *Group) CustomModelsListEnabled() bool {
	return g != nil && g.ModelsListConfig.Enabled && len(g.ModelsListConfig.Models) > 0
}
