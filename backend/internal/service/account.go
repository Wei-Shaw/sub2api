// Package service provides business logic and domain services for the application.
//
// Account BC alias shim (Phase 3): the Account aggregate, its value types, its
// errors, and its pure methods now live in internal/domain. The aliases below
// keep every existing service/handler/repo/test call site compiling unchanged.
// Two former methods could not move to domain because they depend on non-leaf /
// impure packages (openai_compat, the grok billing/free-tier probes); they are
// re-exposed here as free functions AccountSupportsOpenAIEndpointCapability and
// AccountGrokMediaGenerationEligibility.
package service

import (
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
	portaccount "github.com/Wei-Shaw/sub2api/internal/port/account"
)

// --- Type aliases (entity, value types, port contracts) ---

type Account = domain.Account
type AccountRepository = portaccount.Repository
type AccountDuplicateRepository = portaccount.DuplicateRepository
type AdminAccountRepository = portaccount.AdminRepository
type AccountBulkUpdate = portaccount.AccountBulkUpdate
type OAuthRefreshPageOptions = portaccount.OAuthRefreshPageOptions
type OAuthRefreshCandidatePage = portaccount.OAuthRefreshCandidatePage
type OAuthRefreshCandidatePager = portaccount.OAuthRefreshCandidatePager

type OpenAIEndpointCapability = domain.OpenAIEndpointCapability
type TempUnschedulableRule = domain.TempUnschedulableRule
type WindowCostSchedulability = domain.WindowCostSchedulability

// Note: AccountGroup is aliased in group.go (its natural home); OpenAIImagesCapability
// (+ consts) is aliased in openai_images.go. Not redeclared here to avoid duplicates.

// --- Exported const re-exports ---

const (
	OpenAIEndpointCapabilityChatCompletions     = domain.OpenAIEndpointCapabilityChatCompletions
	OpenAIEndpointCapabilityEmbeddings          = domain.OpenAIEndpointCapabilityEmbeddings
	OpenAIEndpointCapabilityAlphaSearch         = domain.OpenAIEndpointCapabilityAlphaSearch
	OpenAIEndpointCapabilityGrokMediaGeneration = domain.OpenAIEndpointCapabilityGrokMediaGeneration
	OpenAIEndpointCapabilityResponses           = domain.OpenAIEndpointCapabilityResponses

	OpenAIAuthModePersonalAccessToken = domain.OpenAIAuthModePersonalAccessToken

	OpenAICompactModeAuto     = domain.OpenAICompactModeAuto
	OpenAICompactModeForceOn  = domain.OpenAICompactModeForceOn
	OpenAICompactModeForceOff = domain.OpenAICompactModeForceOff

	OpenAIWSIngressModeOff         = domain.OpenAIWSIngressModeOff
	OpenAIWSIngressModeShared      = domain.OpenAIWSIngressModeShared
	OpenAIWSIngressModeDedicated   = domain.OpenAIWSIngressModeDedicated
	OpenAIWSIngressModeCtxPool     = domain.OpenAIWSIngressModeCtxPool
	OpenAIWSIngressModePassthrough = domain.OpenAIWSIngressModePassthrough
	OpenAIWSIngressModeHTTPBridge  = domain.OpenAIWSIngressModeHTTPBridge

	WebSearchModeDefault  = domain.WebSearchModeDefault
	WebSearchModeEnabled  = domain.WebSearchModeEnabled
	WebSearchModeDisabled = domain.WebSearchModeDisabled

	GrokMediaEligibleExtraKey = domain.GrokMediaEligibleExtraKey

	WindowCostSchedulable    = domain.WindowCostSchedulable
	WindowCostStickyOnly     = domain.WindowCostStickyOnly
	WindowCostNotSchedulable = domain.WindowCostNotSchedulable
)

// --- Error re-exports ---

var (
	ErrAccountNotFound      = domain.ErrAccountNotFound
	ErrAccountNilInput      = domain.ErrAccountNilInput
	ErrAccountNotInFallback = domain.ErrAccountNotInFallback
)

// --- Unexported const + helper re-exports ---
//
// These identifiers moved to domain with the Account entity but are still
// referenced (under their original unexported names) by other service files
// and by service _test.go files. Re-export each so those callers compile
// unchanged; the canonical definition lives in domain.

const (
	openAILongContextBillingEnabledKey = domain.OpenAILongContextBillingEnabledKey
	openAIAuthModeCredentialKey        = domain.OpenAIAuthModeCredentialKey
	openAIAuthModeLegacyCredentialKey  = domain.OpenAIAuthModeLegacyCredentialKey

	defaultPoolModeRetryCount = domain.DefaultPoolModeRetryCount
	maxPoolModeRetryCount     = domain.MaxPoolModeRetryCount
)

var (
	parseExtraFloat64                   = domain.ParseExtraFloat64
	parseExtraTime                      = domain.ParseExtraTime
	parseExtraInt                       = domain.ParseExtraInt
	ParseExtraInt                       = domain.ParseExtraInt
	normalizeAccountNotes               = domain.NormalizeAccountNotes
	isOpenAIPersonalAccessTokenAuthMode = domain.IsOpenAIPersonalAccessTokenAuthMode
	matchWildcard                       = domain.MatchWildcard
	stringMappingFromRaw                = domain.StringMappingFromRaw
	nextFixedDailyReset                 = domain.NextFixedDailyReset
	lastFixedDailyReset                 = domain.LastFixedDailyReset
	nextFixedWeeklyReset                = domain.NextFixedWeeklyReset
	lastFixedWeeklyReset                = domain.LastFixedWeeklyReset
	normalizeOpenAIWSIngressDefaultMode = domain.NormalizeOpenAIWSIngressDefaultMode
	normalizeOpenAIWSIngressMode        = domain.NormalizeOpenAIWSIngressMode
	normalizeOpenAICompactMode          = domain.NormalizeOpenAICompactMode
	isOpenAIOAuthServableModel          = domain.IsOpenAIOAuthServableModel
	matchWildcardMappingResult          = domain.MatchWildcardMappingResult
)

// --- Pure quota-reset free functions (stay in service; limit blast radius) ---
//
// These touch extra maps but do not depend on the Account entity's moved
// methods, so they were intentionally left here rather than relocated.

// ComputeQuotaResetAt 根据当前配置计算并填充 extra 中的 quota_daily_reset_at / quota_weekly_reset_at
// 在保存账号配置时调用
func ComputeQuotaResetAt(extra map[string]any) {
	now := time.Now()
	tzName, _ := extra["quota_reset_timezone"].(string)
	if tzName == "" {
		tzName = "UTC"
	}
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		tz = time.UTC
	}

	// 日配额固定重置时间
	if mode, _ := extra["quota_daily_reset_mode"].(string); mode == "fixed" {
		hour := int(parseExtraFloat64(extra["quota_daily_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		resetAt := nextFixedDailyReset(hour, tz, now)
		extra["quota_daily_reset_at"] = resetAt.UTC().Format(time.RFC3339)
	} else {
		delete(extra, "quota_daily_reset_at")
	}

	// 周配额固定重置时间
	if mode, _ := extra["quota_weekly_reset_mode"].(string); mode == "fixed" {
		day := 1 // 默认周一
		if d, ok := extra["quota_weekly_reset_day"]; ok {
			day = int(parseExtraFloat64(d))
		}
		if day < 0 || day > 6 {
			day = 1
		}
		hour := int(parseExtraFloat64(extra["quota_weekly_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		resetAt := nextFixedWeeklyReset(day, hour, tz, now)
		extra["quota_weekly_reset_at"] = resetAt.UTC().Format(time.RFC3339)
	} else {
		delete(extra, "quota_weekly_reset_at")
	}
}

// NormalizeFixedQuotaWindows aligns preserved quota usage with the active fixed reset window.
//
// Editing an existing account can switch a daily/weekly quota from rolling to fixed reset
// while preserving quota_*_used and quota_*_start. If the preserved start belongs to the
// old rolling window, response mapping treats the usage as expired and the dashboard shows
// 0 until the next reset. Normalize those stale starts before persisting the edited account.
func NormalizeFixedQuotaWindows(extra map[string]any) {
	if extra == nil {
		return
	}
	now := time.Now()
	tzName, _ := extra["quota_reset_timezone"].(string)
	if tzName == "" {
		tzName = "UTC"
	}
	tz, err := time.LoadLocation(tzName)
	if err != nil {
		tz = time.UTC
	}

	if mode, _ := extra["quota_daily_reset_mode"].(string); mode == "fixed" && parseExtraFloat64(extra["quota_daily_limit"]) > 0 {
		hour := int(parseExtraFloat64(extra["quota_daily_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		lastReset := lastFixedDailyReset(hour, tz, now)
		start := parseExtraTime(extra["quota_daily_start"])
		if start.IsZero() || start.Before(lastReset) {
			extra["quota_daily_used"] = 0.0
			extra["quota_daily_start"] = lastReset.UTC().Format(time.RFC3339)
		}
	}

	if mode, _ := extra["quota_weekly_reset_mode"].(string); mode == "fixed" && parseExtraFloat64(extra["quota_weekly_limit"]) > 0 {
		day := 1
		if rawDay, ok := extra["quota_weekly_reset_day"]; ok {
			day = int(parseExtraFloat64(rawDay))
		}
		if day < 0 || day > 6 {
			day = 1
		}
		hour := int(parseExtraFloat64(extra["quota_weekly_reset_hour"]))
		if hour < 0 || hour > 23 {
			hour = 0
		}
		lastReset := lastFixedWeeklyReset(day, hour, tz, now)
		start := parseExtraTime(extra["quota_weekly_start"])
		if start.IsZero() || start.Before(lastReset) {
			extra["quota_weekly_used"] = 0.0
			extra["quota_weekly_start"] = lastReset.UTC().Format(time.RFC3339)
		}
	}
}

// ValidateQuotaResetConfig 校验配额固定重置时间配置的合法性
func ValidateQuotaResetConfig(extra map[string]any) error {
	if extra == nil {
		return nil
	}
	// 校验时区
	if tz, ok := extra["quota_reset_timezone"].(string); ok && tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return errors.New("invalid quota_reset_timezone: must be a valid IANA timezone name")
		}
	}
	// 日配额重置模式
	if mode, ok := extra["quota_daily_reset_mode"].(string); ok {
		if mode != "rolling" && mode != "fixed" {
			return errors.New("quota_daily_reset_mode must be 'rolling' or 'fixed'")
		}
	}
	// 日配额重置小时
	if v, ok := extra["quota_daily_reset_hour"]; ok {
		hour := int(parseExtraFloat64(v))
		if hour < 0 || hour > 23 {
			return errors.New("quota_daily_reset_hour must be between 0 and 23")
		}
	}
	// 周配额重置模式
	if mode, ok := extra["quota_weekly_reset_mode"].(string); ok {
		if mode != "rolling" && mode != "fixed" {
			return errors.New("quota_weekly_reset_mode must be 'rolling' or 'fixed'")
		}
	}
	// 周配额重置星期几
	if v, ok := extra["quota_weekly_reset_day"]; ok {
		day := int(parseExtraFloat64(v))
		if day < 0 || day > 6 {
			return errors.New("quota_weekly_reset_day must be between 0 (Sunday) and 6 (Saturday)")
		}
	}
	// 周配额重置小时
	if v, ok := extra["quota_weekly_reset_hour"]; ok {
		hour := int(parseExtraFloat64(v))
		if hour < 0 || hour > 23 {
			return errors.New("quota_weekly_reset_hour must be between 0 and 23")
		}
	}
	return nil
}

// --- Free-function gates (impure deps; could not move to domain) ---

// AccountSupportsOpenAIEndpointCapability is the free-function form of the
// former (a *Account) SupportsOpenAIEndpointCapability method. It stays in the
// service layer because it depends on internal/pkg/openai_compat (non-leaf) and
// on the impure AccountGrokMediaGenerationEligibility gate.
func AccountSupportsOpenAIEndpointCapability(a *Account, capability OpenAIEndpointCapability) bool {
	if a == nil {
		return false
	}
	if capability == "" {
		return true
	}
	if !a.IsOpenAICompatible() {
		return false
	}
	if a.IsGrok() {
		switch capability {
		case OpenAIEndpointCapabilityChatCompletions:
			return true
		case OpenAIEndpointCapabilityGrokMediaGeneration:
			eligible, reason := AccountGrokMediaGenerationEligibility(a)
			// Unobserved OAuth accounts remain scheduler candidates only so the
			// request path can run the billing probe before forwarding. The
			// forwarding gate itself fails closed if that probe is unavailable or
			// cannot produce positive paid-entitlement evidence.
			return eligible || reason == "billing_unobserved"
		default:
			return false
		}
	}
	switch capability {
	case OpenAIEndpointCapabilityChatCompletions:
	case OpenAIEndpointCapabilityResponses:
		// Responses 支持状态由 accounts.extra 的自动探测标记决定，而非
		// credentials 能力集。已探测确认不支持 /v1/responses 的 APIKey 上游
		// 必须排除——否则会在 forward 阶段被静默降级为 Chat Completions，
		// 无法完成生图（#4417）。未探测/OAuth 账号保留旧行为（不排除）。
		if a.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(a.Extra) {
			return false
		}
		// 支持 Responses 的上游同样需具备 chat 能力：复用下方 chat_completions
		// 配置集校验。
		capability = OpenAIEndpointCapabilityChatCompletions
	case OpenAIEndpointCapabilityAlphaSearch:
		// alpha/search 的转发按账号类型分流：OAuth/PAT 走
		// chatgpt.com/backend-api/codex/alpha/search，API key 走
		// {base_url}/v1/alpha/search（见 openAIAlphaSearchURL），两类账号
		// 都可承接独立搜索请求。上游不支持该端点时由转发层 failover 兜底。
		if a.Type != AccountTypeOAuth && a.Type != AccountTypeAPIKey {
			return false
		}
	case OpenAIEndpointCapabilityEmbeddings:
		if a.Type != AccountTypeAPIKey {
			return false
		}
	default:
		return false
	}

	configured, found := a.OpenAIEndpointCapabilitySet()
	if !found {
		return true
	}
	if capability == OpenAIEndpointCapabilityAlphaSearch && configured[string(OpenAIEndpointCapabilityChatCompletions)] {
		return true
	}
	return configured[string(capability)]
}

// AccountGrokMediaGenerationEligibility is the free-function form of the former
// (a *Account) GrokMediaGenerationEligibility method. It stays in the service
// layer because it depends on impure helpers (grokBillingSnapshotFromExtra,
// isKnownGrokFreeAccount) that consult probe/net-http-derived state. The pure
// override check (domain.GrokMediaEligibilityOverride) moved to domain.
func AccountGrokMediaGenerationEligibility(a *Account) (bool, string) {
	if a == nil || !a.IsGrok() {
		return false, "not_grok"
	}
	if override, ok := domain.GrokMediaEligibilityOverride(a.Extra); ok {
		if override {
			return true, "override_enabled"
		}
		return false, "override_disabled"
	}
	if a.Type != AccountTypeOAuth {
		return true, "non_oauth"
	}

	billing, err := grokBillingSnapshotFromExtra(a.Extra)
	if err != nil || billing == nil {
		return false, "billing_unobserved"
	}
	if billing.StatusCode == 403 || billing.WeeklyStatusCode == 403 || billing.MonthlyStatusCode == 403 {
		return false, "billing_forbidden"
	}
	if isKnownGrokFreeAccount(a) {
		return false, "billing_free_tier"
	}
	if !grokBillingHasAuthoritativeQuota(billing) {
		return false, "billing_inconclusive"
	}
	return true, "eligible"
}
