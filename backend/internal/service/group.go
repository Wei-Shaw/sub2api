package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	portgroup "github.com/Wei-Shaw/sub2api/internal/port/group"
)

// Type aliases keep existing service call sites compiling while the group BC
// owns its domain types. Mirror of proxy/redeem/promo/announcement.
type Group = domain.Group
type GroupSortOrderUpdate = portgroup.SortOrderUpdate
type GroupRepository = portgroup.Repository
type GroupDuplicateRepository = portgroup.DuplicateRepository
type AdminGroupRepository = portgroup.AdminRepository

// OpenAIMessagesDispatchModelConfig / GroupModelsListConfig / ReasoningEffortMapping
// already lived in domain and were re-exported; keep the aliases.
type OpenAIMessagesDispatchModelConfig = domain.OpenAIMessagesDispatchModelConfig
type GroupModelsListConfig = domain.GroupModelsListConfig
type ReasoningEffortMapping = domain.ReasoningEffortMapping

// Video billing helpers re-exported from domain.
const (
	VideoBillingResolution480P         = domain.VideoBillingResolution480P
	VideoBillingResolution720P         = domain.VideoBillingResolution720P
	VideoBillingResolution1080P        = domain.VideoBillingResolution1080P
	VideoBillingMinDurationSeconds     = domain.VideoBillingMinDurationSeconds
	VideoBillingMaxDurationSeconds     = domain.VideoBillingMaxDurationSeconds
	VideoBillingDefaultDurationSeconds = domain.VideoBillingDefaultDurationSeconds
)

func NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds int) int {
	return domain.NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
}

func NormalizeVideoBillingResolutionOrDefault(resolution string) string {
	return domain.NormalizeVideoBillingResolutionOrDefault(resolution)
}

// Peak-rate helpers re-exported from domain.
func ValidatePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) error {
	return domain.ValidatePeakRateConfig(subscriptionType, enabled, start, end, multiplier)
}

func NormalizePeakRateConfig(subscriptionType string, enabled bool, start, end string, multiplier float64) (bool, string, string, float64) {
	return domain.NormalizePeakRateConfig(subscriptionType, enabled, start, end, multiplier)
}

func IsGroupContextValid(group *Group) bool {
	return domain.IsGroupContextValid(group)
}

// AccountGroup is the account↔group membership edge with nested aggregate
// pointers. Canonical home is now domain (it nests *domain.Account + *domain.Group,
// both lifted in Phase 3); this alias keeps existing service call sites compiling.
// The ToLink method moved to domain with the type.
type AccountGroup = domain.AccountGroup

// AccountGroupFromLink rebuilds an application AccountGroup from a domain link.
func AccountGroupFromLink(link domain.AccountGroupLink) AccountGroup {
	return AccountGroup{
		AccountID: link.AccountID,
		GroupID:   link.GroupID,
		Priority:  link.Priority,
		CreatedAt: link.CreatedAt,
	}
}

// AccountGroupsFromLinks converts domain links to application AccountGroups.
func AccountGroupsFromLinks(links []domain.AccountGroupLink) []AccountGroup {
	if len(links) == 0 {
		return nil
	}
	out := make([]AccountGroup, 0, len(links))
	for _, link := range links {
		out = append(out, AccountGroupFromLink(link))
	}
	return out
}

// AccountGroupLinksFromService converts application AccountGroups to domain links.
func AccountGroupLinksFromService(groups []AccountGroup) []domain.AccountGroupLink {
	if len(groups) == 0 {
		return nil
	}
	out := make([]domain.AccountGroupLink, 0, len(groups))
	for _, ag := range groups {
		out = append(out, ag.ToLink())
	}
	return out
}

// computePeakAwareMultipliers splits base token multiplier into final text and
// image multipliers. Shared by gateway usage paths; peak factor only multiplies
// text. Covered by group_peak_rate_test.
func computePeakAwareMultipliers(apiKey *APIKey, base float64, now time.Time) (text, image float64) {
	image = resolveImageRateMultiplier(apiKey, base)
	peak := 1.0
	if apiKey != nil && apiKey.Group != nil {
		peak = apiKey.Group.PeakMultiplierAt(now)
	}
	text = base * peak
	return
}
