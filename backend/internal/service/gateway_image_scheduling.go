package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

func (s *GatewayService) SelectFalAccountInGroup(ctx context.Context, groupID *int64, sessionHash, requestedModel string, excludedIDs map[int64]struct{}, api string) (*Account, error) {
	state := APIKeyRoutingStateFromContext(ctx)
	if state == nil || len(state.Candidates(groupID)) == 0 {
		return s.selectFalAccountInGroupSingle(ctx, groupID, sessionHash, requestedModel, excludedIDs, api)
	}
	start := state.EffectiveIndex()
	var lastErr error
	for {
		index, err := state.EnsureEligibleFrom(ctx, start)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}
		candidateModel := s.apiKeyFallbackCandidateModel(ctx, state.apiKey.GroupID, requestedModel)
		account, err := s.selectFalAccountInGroupSingle(ctx, state.apiKey.GroupID, sessionHash, candidateModel, excludedIDs, api)
		if err == nil {
			state.Commit(index)
			return account, nil
		}
		if !IsAPIKeyFallbackSelectionError(err) {
			return nil, err
		}
		lastErr = err
		start = index + 1
	}
}

// selectFalAccountInGroupSingle 视频异步任务的混合分组选号入口。
// 视频链路统一走 /api/v1/model 门面，分组内可能同时存在 fal / atlascloud / apiz 平台账号；
// 这里按“哪个平台的账号支持该模型”做混合选号，选中后转发到对应平台的账号，
// 而不是硬编码只取 fal 平台账号。
func (s *GatewayService) selectFalAccountInGroupSingle(ctx context.Context, groupID *int64, sessionHash, requestedModel string, excludedIDs map[int64]struct{}, api string) (*Account, error) {
	accounts, err := s.listSchedulableVideoAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		s.logVideoSelectionFailure(ctx, groupID, requestedModel, api, excludedIDs, accounts)
		return nil, ErrNoAvailableAccounts
	}

	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)
	eligible := func(account *Account) bool {
		if account == nil {
			return false
		}
		if _, excluded := excludedIDs[account.ID]; excluded {
			return false
		}
		if !s.isAccountSchedulableForSelection(account) {
			return false
		}
		if !s.videoAccountSupportsRequest(ctx, account, requestedModel, api) {
			return false
		}
		return s.isAccountSchedulableForModelSelection(ctx, account, requestedModel) &&
			s.isAccountSchedulableForQuota(account) &&
			s.isAccountSchedulableForWindowCost(ctx, account, false) &&
			s.isAccountSchedulableForRPM(ctx, account, false)
	}

	if sessionHash != "" && s.cache != nil {
		if accountID, cacheErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); cacheErr == nil && accountID > 0 {
			for i := range accounts {
				account := &accounts[i]
				if account.ID == accountID && eligible(account) {
					return s.hydrateSelectedAccount(ctx, account)
				}
			}
		}
	}

	var selected *Account
	for i := range accounts {
		account := &accounts[i]
		if !eligible(account) {
			continue
		}
		if selected == nil || imageAccountPreferred(account, selected) {
			selected = account
		}
	}
	if selected == nil {
		s.logVideoSelectionFailure(ctx, groupID, requestedModel, api, excludedIDs, accounts)
		return nil, ErrNoAvailableAccounts
	}
	if sessionHash != "" && s.cache != nil {
		if bindErr := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTLForGroup(s.groupFromContext(ctx, derefGroupID(groupID)))); bindErr != nil {
			logger.LegacyPrintf("service.gateway", "set video session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, bindErr)
		}
	}
	return s.hydrateSelectedAccount(ctx, selected)
}

type videoSelectionDiagnostic struct {
	AccountID            int64
	AccountName          string
	Platform             string
	Status               string
	ManualSchedulable    bool
	GenericSchedulable   bool
	VideoEnabled         bool
	MappingSupported     bool
	WhitelistSupported   bool
	RequestSupported     bool
	ModelSchedulable     bool
	QuotaOK              bool
	WindowCostOK         bool
	RPMOK                bool
	Excluded             bool
	SchedulerCandidate   bool
	MappingKeys          []string
	OverloadRemaining    time.Duration
	RateLimitRemaining   time.Duration
	TempUnschedRemaining time.Duration
	ModelLimitRemaining  time.Duration
	TempUnschedReason    string
}

func (s *GatewayService) buildVideoSelectionDiagnostic(
	ctx context.Context,
	account *Account,
	requestedModel string,
	api string,
	excludedIDs map[int64]struct{},
	schedulerCandidate bool,
) videoSelectionDiagnostic {
	diagnostic := videoSelectionDiagnostic{}
	if account == nil {
		return diagnostic
	}
	now := time.Now()
	diagnostic.AccountID = account.ID
	diagnostic.AccountName = account.Name
	diagnostic.Platform = account.Platform
	diagnostic.Status = account.Status
	diagnostic.ManualSchedulable = account.Schedulable
	diagnostic.GenericSchedulable = account.IsSchedulable()
	diagnostic.VideoEnabled = domain.IsVideoModelsEnabled(account.Extra)
	diagnostic.MappingSupported, diagnostic.WhitelistSupported = videoMappingSupportsRequest(account, requestedModel)
	diagnostic.RequestSupported = s.videoAccountSupportsRequest(ctx, account, requestedModel, api)
	diagnostic.ModelSchedulable = s.isAccountSchedulableForModelSelection(ctx, account, requestedModel)
	diagnostic.QuotaOK = s.isAccountSchedulableForQuota(account)
	diagnostic.WindowCostOK = s.isAccountSchedulableForWindowCost(ctx, account, false)
	diagnostic.RPMOK = s.isAccountSchedulableForRPM(ctx, account, false)
	_, diagnostic.Excluded = excludedIDs[account.ID]
	diagnostic.SchedulerCandidate = schedulerCandidate
	if account.OverloadUntil != nil && now.Before(*account.OverloadUntil) {
		diagnostic.OverloadRemaining = time.Until(*account.OverloadUntil).Truncate(time.Second)
	}
	if account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt) {
		diagnostic.RateLimitRemaining = time.Until(*account.RateLimitResetAt).Truncate(time.Second)
	}
	if account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil) {
		diagnostic.TempUnschedRemaining = time.Until(*account.TempUnschedulableUntil).Truncate(time.Second)
	}
	diagnostic.ModelLimitRemaining = account.GetRateLimitRemainingTimeWithContext(ctx, requestedModel).Truncate(time.Second)
	diagnostic.TempUnschedReason = account.TempUnschedulableReason
	for key := range account.GetModelMapping() {
		diagnostic.MappingKeys = append(diagnostic.MappingKeys, key)
	}
	sort.Strings(diagnostic.MappingKeys)
	if len(diagnostic.MappingKeys) > 10 {
		diagnostic.MappingKeys = diagnostic.MappingKeys[:10]
	}
	return diagnostic
}

func (s *GatewayService) logVideoSelectionFailure(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	api string,
	excludedIDs map[int64]struct{},
	schedulableAccounts []Account,
) {
	platformCounts := map[string]int{
		PlatformFal:        0,
		PlatformAtlasCloud: 0,
		PlatformApiz:       0,
	}
	for i := range schedulableAccounts {
		platformCounts[schedulableAccounts[i].Platform]++
	}

	boundAccounts := make([]Account, 0, len(schedulableAccounts))
	schedulerCandidateIDs := make(map[int64]struct{}, len(schedulableAccounts))
	for i := range schedulableAccounts {
		schedulerCandidateIDs[schedulableAccounts[i].ID] = struct{}{}
	}
	queryErrors := make([]string, 0, 3)
	if s == nil || s.accountRepo == nil {
		queryErrors = append(queryErrors, "account repository unavailable")
	}
	for _, platform := range []string{PlatformFal, PlatformAtlasCloud, PlatformApiz} {
		if s == nil || s.accountRepo == nil {
			break
		}
		accounts, err := s.accountRepo.ListByPlatform(ctx, platform)
		if err != nil {
			queryErrors = append(queryErrors, platform+":"+err.Error())
			continue
		}
		for i := range accounts {
			if s.isAccountInGroup(&accounts[i], groupID) {
				boundAccounts = append(boundAccounts, accounts[i])
			}
		}
	}

	logger.LegacyPrintf(
		"service.gateway",
		"[VideoSelectionFailure] group_id=%d model=%q api=%q schedulable_candidates=%d fal_candidates=%d atlascloud_candidates=%d apiz_candidates=%d bound_video_accounts=%d query_errors=%q",
		derefGroupID(groupID),
		requestedModel,
		api,
		len(schedulableAccounts),
		platformCounts[PlatformFal],
		platformCounts[PlatformAtlasCloud],
		platformCounts[PlatformApiz],
		len(boundAccounts),
		queryErrors,
	)
	logCandidate := func(source string, account *Account, schedulerCandidate bool) {
		diagnostic := s.buildVideoSelectionDiagnostic(ctx, account, requestedModel, api, excludedIDs, schedulerCandidate)
		logger.LegacyPrintf(
			"service.gateway",
			"[VideoSelectionCandidate] source=%s group_id=%d model=%q account_id=%d account_name=%q platform=%s scheduler_candidate=%t status=%s manual_schedulable=%t generic_schedulable=%t video_enabled=%t mapping_supported=%t whitelist_supported=%t request_supported=%t model_schedulable=%t quota_ok=%t window_cost_ok=%t rpm_ok=%t excluded=%t overload_remaining=%s rate_limit_remaining=%s temp_unsched_remaining=%s temp_unsched_reason=%q model_limit_remaining=%s mapping_keys=%q",
			source,
			derefGroupID(groupID),
			requestedModel,
			diagnostic.AccountID,
			diagnostic.AccountName,
			diagnostic.Platform,
			diagnostic.SchedulerCandidate,
			diagnostic.Status,
			diagnostic.ManualSchedulable,
			diagnostic.GenericSchedulable,
			diagnostic.VideoEnabled,
			diagnostic.MappingSupported,
			diagnostic.WhitelistSupported,
			diagnostic.RequestSupported,
			diagnostic.ModelSchedulable,
			diagnostic.QuotaOK,
			diagnostic.WindowCostOK,
			diagnostic.RPMOK,
			diagnostic.Excluded,
			diagnostic.OverloadRemaining,
			diagnostic.RateLimitRemaining,
			diagnostic.TempUnschedRemaining,
			diagnostic.TempUnschedReason,
			diagnostic.ModelLimitRemaining,
			diagnostic.MappingKeys,
		)
	}
	for i := range schedulableAccounts {
		logCandidate("scheduler", &schedulableAccounts[i], true)
	}
	for i := range boundAccounts {
		_, schedulerCandidate := schedulerCandidateIDs[boundAccounts[i].ID]
		logCandidate("database", &boundAccounts[i], schedulerCandidate)
	}
}

// listSchedulableVideoAccounts 合并可参与视频调度的多平台账号（fal + atlascloud + apiz）。
func (s *GatewayService) listSchedulableVideoAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	falAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformFal, false)
	if err != nil {
		return nil, fmt.Errorf("query fal accounts failed: %w", err)
	}
	atlasAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformAtlasCloud, false)
	if err != nil {
		return nil, fmt.Errorf("query atlascloud accounts failed: %w", err)
	}
	apizAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformApiz, false)
	if err != nil {
		return nil, fmt.Errorf("query apiz accounts failed: %w", err)
	}
	merged := make([]Account, 0, len(falAccounts)+len(atlasAccounts)+len(apizAccounts))
	merged = append(merged, falAccounts...)
	merged = append(merged, atlasAccounts...)
	merged = append(merged, apizAccounts...)
	return merged, nil
}

// videoAccountSupportsRequest 判断某平台账号是否支持指定视频模型。
// 三个平台统一先匹配公开模型映射 key；未命中时再按各平台的原生模型白名单回退。
func (s *GatewayService) videoAccountSupportsRequest(_ context.Context, account *Account, requestedModel, _ string) bool {
	if account == nil {
		return false
	}
	switch account.Platform {
	case PlatformFal, PlatformAtlasCloud, PlatformApiz:
		if !domain.IsVideoModelsEnabled(account.Extra) {
			return false
		}
		mappingSupported, whitelistSupported := videoMappingSupportsRequest(account, requestedModel)
		return mappingSupported || whitelistSupported
	default:
		return false
	}
}

func videoMappingSupportsRequest(account *Account, requestedModel string) (mappingSupported, whitelistSupported bool) {
	if account == nil {
		return false, false
	}
	mapping := account.GetModelMapping()
	if mappingSupportsRequestedModel(mapping, requestedModel) {
		mappingSupported = true
	}

	if domain.VideoPlatformWhitelistSupports(account.Platform, requestedModel) {
		return mappingSupported, true
	}
	// fal 的 mapping value 是其原生 endpoint，同时也是账号实际支持的
	// endpoint 白名单。atlascloud/apiz 的 value 是内部模型标识，不能用于
	// 判断公开请求模型是否在白名单中。
	if account.Platform != PlatformFal {
		return mappingSupported, false
	}
	requestedSlug := domain.NormalizeFalVideoModelEndpoint(requestedModel)
	if requestedSlug == "" {
		return mappingSupported, false
	}
	for _, upstreamModel := range mapping {
		if strings.EqualFold(domain.NormalizeFalVideoModelEndpoint(upstreamModel), requestedSlug) {
			return mappingSupported, true
		}
	}
	return mappingSupported, false
}

func (s *GatewayService) SelectImageAccountMixed(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	imageCapability OpenAIImagesCapability,
	falAPI string,
	preferPlatform string,
) (*Account, error) {
	state := APIKeyRoutingStateFromContext(ctx)
	if state == nil || len(state.Candidates(groupID)) == 0 {
		return s.selectImageAccountMixedSingle(ctx, groupID, sessionHash, requestedModel, excludedIDs, imageCapability, falAPI, preferPlatform)
	}
	start := state.EffectiveIndex()
	var lastErr error
	for {
		index, err := state.EnsureEligibleFrom(ctx, start)
		if err != nil {
			if lastErr != nil {
				return nil, lastErr
			}
			return nil, err
		}
		candidateModel := s.apiKeyFallbackCandidateModel(ctx, state.apiKey.GroupID, requestedModel)
		account, err := s.selectImageAccountMixedSingle(ctx, state.apiKey.GroupID, sessionHash, candidateModel, excludedIDs, imageCapability, falAPI, preferPlatform)
		if err == nil {
			state.Commit(index)
			return account, nil
		}
		if !IsAPIKeyFallbackSelectionError(err) {
			return nil, err
		}
		lastErr = err
		start = index + 1
	}
}

func (s *GatewayService) selectImageAccountMixedSingle(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	imageCapability OpenAIImagesCapability,
	falAPI string,
	preferPlatform string,
) (*Account, error) {
	accounts, err := s.listSchedulableImageAccounts(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccounts
	}

	ctx = s.withWindowCostPrefetch(ctx, accounts)
	ctx = s.withRPMPrefetch(ctx, accounts)
	diagnostics := make(map[int64]imageSelectionDiagnostic, len(accounts))
	eligibleCount := 0
	eligibleByPlatform := make(map[string]int, 2)
	for i := range accounts {
		diagnostic := s.buildImageSelectionDiagnostic(ctx, &accounts[i], requestedModel, imageCapability, falAPI, groupID, excludedIDs)
		diagnostics[accounts[i].ID] = diagnostic
		if diagnostic.Eligible {
			eligibleCount++
			eligibleByPlatform[accounts[i].Platform]++
		}
	}

	selectionPolicy := "priority_last_used"
	if strings.EqualFold(strings.TrimSpace(preferPlatform), PlatformFal) {
		selectionPolicy = "fal_first_then_priority_last_used"
	} else if strings.EqualFold(strings.TrimSpace(preferPlatform), PlatformLeonardo) {
		selectionPolicy = "leonardo_first_then_priority_last_used"
	}
	reqLog := logger.FromContext(ctx)
	if reqLog.Core().Enabled(zap.DebugLevel) {
		reqLog.Debug("image.account_selection_started",
			zap.Int64("group_id", derefGroupID(groupID)),
			zap.String("model", requestedModel),
			zap.String("required_capability", string(imageCapability)),
			zap.String("fal_api", falAPI),
			zap.String("prefer_platform", strings.ToLower(strings.TrimSpace(preferPlatform))),
			zap.String("selection_policy", selectionPolicy),
			zap.Int("candidate_count", len(accounts)),
			zap.Int("eligible_candidate_count", eligibleCount),
			zap.Int("eligible_openai_count", eligibleByPlatform[PlatformOpenAI]),
			zap.Int("eligible_fal_count", eligibleByPlatform[PlatformFal]),
			zap.Int("excluded_account_count", len(excludedIDs)),
			zap.Bool("session_sticky_enabled", sessionHash != ""),
		)
	}
	eligible := func(account *Account) bool {
		if account == nil {
			return false
		}
		return diagnostics[account.ID].Eligible
	}

	selectionReason := "priority_last_used"
	if sessionHash != "" && s.cache != nil {
		if accountID, cacheErr := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), sessionHash); cacheErr == nil && accountID > 0 {
			for i := range accounts {
				account := &accounts[i]
				if account.ID == accountID && eligible(account) {
					s.logImageSelectionDiagnostics(ctx, groupID, requestedModel, selectionPolicy, "sticky_session", account, accounts, diagnostics)
					return s.hydrateSelectedAccount(ctx, account)
				}
			}
			reqLog.Debug("image.account_selection_sticky_miss",
				zap.Int64("group_id", derefGroupID(groupID)),
				zap.String("model", requestedModel),
				zap.Int64("sticky_account_id", accountID),
				zap.String("reason", "account_missing_or_ineligible"),
			)
		} else if cacheErr != nil {
			reqLog.Debug("image.account_selection_sticky_miss",
				zap.Int64("group_id", derefGroupID(groupID)),
				zap.String("model", requestedModel),
				zap.String("reason", "cache_lookup_failed"),
				zap.Error(cacheErr),
			)
		}
	}

	pick := func(platform string) *Account {
		var selected *Account
		for i := range accounts {
			account := &accounts[i]
			if platform != "" && account.Platform != platform {
				continue
			}
			if !eligible(account) {
				continue
			}
			if selected == nil || imageAccountPreferred(account, selected) {
				selected = account
			}
		}
		return selected
	}

	var selected *Account
	if strings.EqualFold(strings.TrimSpace(preferPlatform), PlatformFal) {
		selected = pick(PlatformFal)
		if selected == nil {
			selected = pick(PlatformOpenAI)
			selectionReason = "fal_pool_empty_fallback"
		} else {
			selectionReason = "preferred_fal_pool"
		}
	} else if strings.EqualFold(strings.TrimSpace(preferPlatform), PlatformLeonardo) {
		selected = pick(PlatformLeonardo)
		if selected == nil {
			selectionReason = "leonardo_pool_empty"
		} else {
			selectionReason = "preferred_leonardo_pool"
		}
	} else {
		selected = pick("")
	}
	if selected == nil {
		s.logImageSelectionDiagnostics(ctx, groupID, requestedModel, selectionPolicy, "no_eligible_account", nil, accounts, diagnostics)
		return nil, ErrNoAvailableAccounts
	}
	s.logImageSelectionDiagnostics(ctx, groupID, requestedModel, selectionPolicy, selectionReason, selected, accounts, diagnostics)
	if sessionHash != "" && s.cache != nil {
		if bindErr := s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), sessionHash, selected.ID, stickySessionTTLForGroup(s.groupFromContext(ctx, derefGroupID(groupID)))); bindErr != nil {
			logger.LegacyPrintf("service.gateway", "set image session account failed: session=%s account_id=%d err=%v", sessionHash, selected.ID, bindErr)
		}
	}
	return s.hydrateSelectedAccount(ctx, selected)
}

type imageSelectionDiagnostic struct {
	Excluded            bool
	Schedulable         bool
	ModelSupported      bool
	CapabilitySupported bool
	RequestSupported    bool
	PricingConfigured   bool
	ModelSchedulable    bool
	QuotaOK             bool
	WindowCostOK        bool
	RPMOK               bool
	Eligible            bool
	RejectionReason     string
	UpstreamModel       string
}

func (s *GatewayService) buildImageSelectionDiagnostic(
	ctx context.Context,
	account *Account,
	requestedModel string,
	imageCapability OpenAIImagesCapability,
	falAPI string,
	groupID *int64,
	excludedIDs map[int64]struct{},
) imageSelectionDiagnostic {
	diagnostic := imageSelectionDiagnostic{}
	if account == nil {
		diagnostic.RejectionReason = "nil_account"
		return diagnostic
	}
	_, diagnostic.Excluded = excludedIDs[account.ID]
	diagnostic.Schedulable = s.isAccountSchedulableForSelection(account)
	switch account.Platform {
	case PlatformOpenAI:
		diagnostic.ModelSupported = requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, "")
		diagnostic.CapabilitySupported = account.SupportsOpenAIImageCapability(imageCapability)
	case PlatformFal:
		diagnostic.ModelSupported = s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, falAPI)
		diagnostic.CapabilitySupported = true
	case PlatformLeonardo:
		diagnostic.ModelSupported = s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, "")
		diagnostic.CapabilitySupported = !strings.EqualFold(falAPI, FalAPIEdit)
	}
	diagnostic.RequestSupported = s.imageAccountSupportsRequest(ctx, account, requestedModel, imageCapability, falAPI)
	diagnostic.PricingConfigured = s.falAccountPricingConfigured(ctx, account, requestedModel, falAPI, groupID)
	diagnostic.ModelSchedulable = s.isAccountSchedulableForModelSelection(ctx, account, requestedModel)
	diagnostic.QuotaOK = s.isAccountSchedulableForQuota(account)
	diagnostic.WindowCostOK = s.isAccountSchedulableForWindowCost(ctx, account, false)
	diagnostic.RPMOK = s.isAccountSchedulableForRPM(ctx, account, false)
	switch account.Platform {
	case PlatformFal:
		diagnostic.UpstreamModel = resolveFalUpstreamModel(account, requestedModel, falAPI == FalAPIEdit)
	case PlatformLeonardo:
		diagnostic.UpstreamModel = strings.TrimSpace(account.GetModelMapping()[requestedModel])
		if diagnostic.UpstreamModel == "" {
			diagnostic.UpstreamModel = "gpt-image-2"
		}
	default:
		diagnostic.UpstreamModel = account.GetMappedModel(requestedModel)
	}

	switch {
	case diagnostic.Excluded:
		diagnostic.RejectionReason = "excluded_after_failure"
	case !diagnostic.Schedulable:
		diagnostic.RejectionReason = imageAccountUnschedulableReason(account)
	case !diagnostic.ModelSupported:
		diagnostic.RejectionReason = "model_or_api_unsupported"
	case !diagnostic.CapabilitySupported:
		diagnostic.RejectionReason = "image_capability_unsupported"
	case !diagnostic.RequestSupported:
		diagnostic.RejectionReason = "request_unsupported"
	case !diagnostic.PricingConfigured:
		diagnostic.RejectionReason = "fal_pricing_not_configured"
	case !diagnostic.ModelSchedulable:
		diagnostic.RejectionReason = "model_temporarily_limited"
	case !diagnostic.QuotaOK:
		diagnostic.RejectionReason = "quota_exceeded"
	case !diagnostic.WindowCostOK:
		diagnostic.RejectionReason = "window_cost_limit"
	case !diagnostic.RPMOK:
		diagnostic.RejectionReason = "rpm_limit"
	default:
		diagnostic.Eligible = true
	}
	return diagnostic
}

func imageAccountUnschedulableReason(account *Account) string {
	if account == nil {
		return "nil_account"
	}
	now := time.Now()
	switch {
	case !account.IsActive():
		return "status_not_active"
	case !account.Schedulable:
		return "manually_unschedulable"
	case account.AutoPauseOnExpired && account.ExpiresAt != nil && !now.Before(*account.ExpiresAt):
		return "expired"
	case account.OverloadUntil != nil && now.Before(*account.OverloadUntil):
		return "overloaded"
	case account.RateLimitResetAt != nil && now.Before(*account.RateLimitResetAt):
		return "rate_limited"
	case account.TempUnschedulableUntil != nil && now.Before(*account.TempUnschedulableUntil):
		return "temporarily_unschedulable"
	case account.IsAPIKeyOrBedrock() && account.IsQuotaExceeded():
		return "quota_exceeded"
	default:
		return "runtime_unschedulable"
	}
}

func (s *GatewayService) logImageSelectionDiagnostics(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	selectionPolicy string,
	selectionReason string,
	selected *Account,
	accounts []Account,
	diagnostics map[int64]imageSelectionDiagnostic,
) {
	reqLog := logger.FromContext(ctx)
	if !reqLog.Core().Enabled(zap.DebugLevel) {
		return
	}
	selectedID := int64(0)
	if selected != nil {
		selectedID = selected.ID
	}
	for i := range accounts {
		account := &accounts[i]
		diagnostic := diagnostics[account.ID]
		outcome := "rejected"
		reason := diagnostic.RejectionReason
		switch {
		case account.ID == selectedID:
			outcome = "selected"
			reason = selectionReason
		case diagnostic.Eligible && selected != nil && selectionReason == "sticky_session":
			outcome = "not_selected"
			reason = "sticky_session_selected"
		case diagnostic.Eligible && selected != nil && selectionPolicy == "fal_first_then_priority_last_used" && selected.Platform == PlatformFal && account.Platform != PlatformFal:
			outcome = "not_selected"
			reason = "fal_platform_preferred"
		case diagnostic.Eligible:
			outcome = "not_selected"
			reason = imageAccountRankLossReason(account, selected)
		}

		reqLog.Debug("image.account_selection_candidate",
			zap.Int64("group_id", derefGroupID(groupID)),
			zap.String("model", requestedModel),
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("platform", account.Platform),
			zap.Int("priority", account.Priority),
			zap.Timep("last_used_at", account.LastUsedAt),
			zap.String("status", account.Status),
			zap.Bool("manual_schedulable", account.Schedulable),
			zap.Bool("schedulable", diagnostic.Schedulable),
			zap.Bool("model_supported", diagnostic.ModelSupported),
			zap.Bool("capability_supported", diagnostic.CapabilitySupported),
			zap.Bool("request_supported", diagnostic.RequestSupported),
			zap.Bool("pricing_configured", diagnostic.PricingConfigured),
			zap.Bool("model_schedulable", diagnostic.ModelSchedulable),
			zap.Bool("quota_ok", diagnostic.QuotaOK),
			zap.Bool("window_cost_ok", diagnostic.WindowCostOK),
			zap.Bool("rpm_ok", diagnostic.RPMOK),
			zap.Bool("excluded", diagnostic.Excluded),
			zap.Bool("eligible", diagnostic.Eligible),
			zap.String("upstream_model", diagnostic.UpstreamModel),
			zap.String("outcome", outcome),
			zap.String("reason", reason),
			zap.Timep("overload_until", account.OverloadUntil),
			zap.Timep("rate_limit_reset_at", account.RateLimitResetAt),
			zap.Timep("temp_unschedulable_until", account.TempUnschedulableUntil),
			zap.String("temp_unschedulable_reason", account.TempUnschedulableReason),
		)
	}

	if selected == nil {
		reqLog.Debug("image.account_selection_failed",
			zap.Int64("group_id", derefGroupID(groupID)),
			zap.String("model", requestedModel),
			zap.String("selection_policy", selectionPolicy),
			zap.String("reason", selectionReason),
			zap.Int("candidate_count", len(accounts)),
		)
		return
	}
	reqLog.Debug("image.account_selection_selected",
		zap.Int64("group_id", derefGroupID(groupID)),
		zap.String("model", requestedModel),
		zap.String("selection_policy", selectionPolicy),
		zap.String("selection_reason", selectionReason),
		zap.Int64("account_id", selected.ID),
		zap.String("account_name", selected.Name),
		zap.String("platform", selected.Platform),
		zap.Int("priority", selected.Priority),
		zap.Timep("last_used_at", selected.LastUsedAt),
	)
}

func imageAccountRankLossReason(candidate, selected *Account) string {
	if candidate == nil || selected == nil {
		return "not_selected"
	}
	if candidate.Priority > selected.Priority {
		return "lower_priority"
	}
	if candidate.Priority < selected.Priority {
		return "platform_pool_order"
	}
	switch {
	case candidate.LastUsedAt == nil && selected.LastUsedAt == nil:
		return "stable_order_tie"
	case candidate.LastUsedAt != nil && selected.LastUsedAt == nil:
		return "selected_account_never_used"
	case candidate.LastUsedAt == nil:
		return "platform_pool_order"
	case candidate.LastUsedAt.After(*selected.LastUsedAt):
		return "more_recently_used"
	case candidate.LastUsedAt.Equal(*selected.LastUsedAt):
		return "stable_order_tie"
	default:
		return "platform_pool_order"
	}
}

func imageAccountPreferred(candidate, selected *Account) bool {
	if candidate.Priority != selected.Priority {
		return candidate.Priority < selected.Priority
	}
	switch {
	case candidate.LastUsedAt == nil && selected.LastUsedAt != nil:
		return true
	case candidate.LastUsedAt != nil && selected.LastUsedAt == nil:
		return false
	case candidate.LastUsedAt == nil:
		return false
	default:
		return candidate.LastUsedAt.Before(*selected.LastUsedAt)
	}
}

func (s *GatewayService) listSchedulableImageAccounts(ctx context.Context, groupID *int64) ([]Account, error) {
	openAIAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformOpenAI, false)
	if err != nil {
		return nil, fmt.Errorf("query openai accounts failed: %w", err)
	}
	falAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformFal, false)
	if err != nil {
		return nil, fmt.Errorf("query fal accounts failed: %w", err)
	}
	leonardoAccounts, _, err := s.listSchedulableAccounts(ctx, groupID, PlatformLeonardo, false)
	if err != nil {
		return nil, fmt.Errorf("query leonardo accounts failed: %w", err)
	}
	accounts := append(openAIAccounts, falAccounts...)
	return append(accounts, leonardoAccounts...), nil
}

func (s *GatewayService) imageAccountSupportsRequest(ctx context.Context, account *Account, requestedModel string, capability OpenAIImagesCapability, falAPI string) bool {
	switch account.Platform {
	case PlatformOpenAI:
		return (requestedModel == "" || s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, "")) && account.SupportsOpenAIImageCapability(capability)
	case PlatformFal:
		return s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, falAPI)
	case PlatformLeonardo:
		return !strings.EqualFold(falAPI, FalAPIEdit) &&
			s.isModelSupportedByAccountWithContext(ctx, account, requestedModel, "")
	default:
		return false
	}
}

func (s *GatewayService) falAccountPricingConfigured(ctx context.Context, account *Account, requestedModel, falAPI string, groupID *int64) bool {
	if account == nil || (account.Platform != PlatformFal && account.Platform != PlatformLeonardo) {
		return true
	}
	if s.resolver == nil {
		return false
	}
	upstreamModel := resolveFalUpstreamModel(account, requestedModel, falAPI == FalAPIEdit)
	if account.Platform == PlatformLeonardo {
		upstreamModel = strings.TrimSpace(account.GetModelMapping()[requestedModel])
		if upstreamModel == "" {
			upstreamModel = "gpt-image-2"
		}
	}
	resolved := s.resolver.Resolve(ctx, PricingInput{Model: upstreamModel, GroupID: groupID})
	if resolved != nil && resolved.Source == PricingSourceChannel {
		return (resolved.Mode == BillingModeImage || resolved.Mode == BillingModePerRequest) &&
			(len(resolved.RequestTiers) > 0 || resolved.DefaultPerRequestPrice > 0)
	}
	if groupID == nil {
		return false
	}
	group := s.groupFromContext(ctx, *groupID)
	if group == nil && s.groupRepo != nil {
		group, _ = s.groupRepo.GetByIDLite(ctx, *groupID)
	}
	if group == nil {
		return false
	}
	for _, row := range group.ImagePricingMatrix {
		for _, price := range row {
			if price > 0 {
				return true
			}
		}
	}
	return false
}
