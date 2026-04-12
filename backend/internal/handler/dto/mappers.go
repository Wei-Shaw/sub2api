// Package dto provides data transfer objects for HTTP handlers.
package dto

import (
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func UserFromServiceShallow(u *service.User) *User {
	if u == nil {
		return nil
	}
	return &User{
		ID:                         u.ID,
		Email:                      u.Email,
		Username:                   u.Username,
		Role:                       u.Role,
		Balance:                    u.Balance,
		Concurrency:                u.Concurrency,
		Status:                     u.Status,
		AllowedGroups:              u.AllowedGroups,
		CreatedAt:                  u.CreatedAt,
		UpdatedAt:                  u.UpdatedAt,
		BalanceNotifyEnabled:       u.BalanceNotifyEnabled,
		BalanceNotifyThresholdType: u.BalanceNotifyThresholdType,
		BalanceNotifyThreshold:     u.BalanceNotifyThreshold,
		BalanceNotifyExtraEmails:   NotifyEmailEntriesFromService(u.BalanceNotifyExtraEmails),
		TotalRecharged:             u.TotalRecharged,
	}
}

func UserFromService(u *service.User) *User {
	if u == nil {
		return nil
	}
	out := UserFromServiceShallow(u)
	if len(u.APIKeys) > 0 {
		out.APIKeys = make([]APIKey, 0, len(u.APIKeys))
		for i := range u.APIKeys {
			k := u.APIKeys[i]
			out.APIKeys = append(out.APIKeys, *APIKeyFromService(&k))
		}
	}
	if len(u.Subscriptions) > 0 {
		out.Subscriptions = make([]UserSubscription, 0, len(u.Subscriptions))
		for i := range u.Subscriptions {
			s := u.Subscriptions[i]
			out.Subscriptions = append(out.Subscriptions, *UserSubscriptionFromService(&s))
		}
	}
	return out
}

// UserFromServiceAdmin converts a service User to DTO for admin users.
// It includes notes - user-facing endpoints must not use this.
func UserFromServiceAdmin(u *service.User) *AdminUser {
	if u == nil {
		return nil
	}
	base := UserFromService(u)
	if base == nil {
		return nil
	}
	return &AdminUser{
		User:       *base,
		Notes:      u.Notes,
		GroupRates: u.GroupRates,
	}
}

func APIKeyFromService(k *service.APIKey) *APIKey {
	if k == nil {
		return nil
	}
	out := &APIKey{
		ID:            k.ID,
		UserID:        k.UserID,
		Key:           k.Key,
		Name:          k.Name,
		GroupID:       k.GroupID,
		Status:        k.Status,
		IPWhitelist:   k.IPWhitelist,
		IPBlacklist:   k.IPBlacklist,
		LastUsedAt:    k.LastUsedAt,
		Quota:         k.Quota,
		QuotaUsed:     k.QuotaUsed,
		ExpiresAt:     k.ExpiresAt,
		CreatedAt:     k.CreatedAt,
		UpdatedAt:     k.UpdatedAt,
		RateLimit5h:   k.RateLimit5h,
		RateLimit1d:   k.RateLimit1d,
		RateLimit7d:   k.RateLimit7d,
		Usage5h:       k.EffectiveUsage5h(),
		Usage1d:       k.EffectiveUsage1d(),
		Usage7d:       k.EffectiveUsage7d(),
		Window5hStart: k.Window5hStart,
		Window1dStart: k.Window1dStart,
		Window7dStart: k.Window7dStart,
		User:          UserFromServiceShallow(k.User),
		Group:         GroupFromServiceShallow(k.Group),
	}
	if k.Window5hStart != nil && !service.IsWindowExpired(k.Window5hStart, service.RateLimitWindow5h) {
		t := k.Window5hStart.Add(service.RateLimitWindow5h)
		out.Reset5hAt = &t
	}
	if k.Window1dStart != nil && !service.IsWindowExpired(k.Window1dStart, service.RateLimitWindow1d) {
		t := k.Window1dStart.Add(service.RateLimitWindow1d)
		out.Reset1dAt = &t
	}
	if k.Window7dStart != nil && !service.IsWindowExpired(k.Window7dStart, service.RateLimitWindow7d) {
		t := k.Window7dStart.Add(service.RateLimitWindow7d)
		out.Reset7dAt = &t
	}
	return out
}

func GroupFromServiceShallow(g *service.Group) *Group {
	if g == nil {
		return nil
	}
	out := groupFromServiceBase(g)
	return &out
}

func GroupFromService(g *service.Group) *Group {
	if g == nil {
		return nil
	}
	return GroupFromServiceShallow(g)
}

// GroupFromServiceAdmin converts a service Group to DTO for admin users.
// It includes internal fields like model_routing and account_count.
func GroupFromServiceAdmin(g *service.Group) *AdminGroup {
	if g == nil {
		return nil
	}
	out := &AdminGroup{
		Group:                       groupFromServiceBase(g),
		ModelRouting:                g.ModelRouting,
		ModelRoutingEnabled:         g.ModelRoutingEnabled,
		MCPXMLInject:                g.MCPXMLInject,
		DefaultMappedModel:          g.DefaultMappedModel,
		MessagesDispatchModelConfig: g.MessagesDispatchModelConfig,
		SupportedModelScopes:        g.SupportedModelScopes,
		AccountCount:                g.AccountCount,
		ActiveAccountCount:          g.ActiveAccountCount,
		RateLimitedAccountCount:     g.RateLimitedAccountCount,
		SortOrder:                   g.SortOrder,
	}
	if len(g.AccountGroups) > 0 {
		out.AccountGroups = make([]AccountGroup, 0, len(g.AccountGroups))
		for i := range g.AccountGroups {
			ag := g.AccountGroups[i]
			out.AccountGroups = append(out.AccountGroups, *AccountGroupFromService(&ag))
		}
	}
	return out
}

func groupFromServiceBase(g *service.Group) Group {
	return Group{
		ID:                              g.ID,
		Name:                            g.Name,
		Description:                     g.Description,
		Platform:                        g.Platform,
		RateMultiplier:                  g.RateMultiplier,
		IsExclusive:                     g.IsExclusive,
		Status:                          g.Status,
		SubscriptionType:                g.SubscriptionType,
		DailyLimitUSD:                   g.DailyLimitUSD,
		WeeklyLimitUSD:                  g.WeeklyLimitUSD,
		MonthlyLimitUSD:                 g.MonthlyLimitUSD,
		ImagePrice1K:                    g.ImagePrice1K,
		ImagePrice2K:                    g.ImagePrice2K,
		ImagePrice4K:                    g.ImagePrice4K,
		ClaudeCodeOnly:                  g.ClaudeCodeOnly,
		FallbackGroupID:                 g.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: g.FallbackGroupIDOnInvalidRequest,
		AllowMessagesDispatch:           g.AllowMessagesDispatch,
		RequireOAuthOnly:                g.RequireOAuthOnly,
		RequirePrivacySet:               g.RequirePrivacySet,
		CreatedAt:                       g.CreatedAt,
		UpdatedAt:                       g.UpdatedAt,
	}
}

func AccountFromServiceShallow(a *service.Account) *Account {
	if a == nil {
		return nil
	}
	out := &Account{
		ID:                      a.ID,
		Name:                    a.Name,
		Notes:                   a.Notes,
		Platform:                a.Platform,
		Type:                    a.Type,
		Credentials:             a.Credentials,
		Extra:                   a.Extra,
		ProxyID:                 a.ProxyID,
		Concurrency:             a.Concurrency,
		LoadFactor:              a.LoadFactor,
		Priority:                a.Priority,
		RateMultiplier:          a.BillingRateMultiplier(),
		Status:                  a.Status,
		ErrorMessage:            a.ErrorMessage,
		LastUsedAt:              a.LastUsedAt,
		ExpiresAt:               timeToUnixSeconds(a.ExpiresAt),
		AutoPauseOnExpired:      a.AutoPauseOnExpired,
		CreatedAt:               a.CreatedAt,
		UpdatedAt:               a.UpdatedAt,
		Schedulable:             a.Schedulable,
		RateLimitedAt:           a.RateLimitedAt,
		RateLimitResetAt:        a.RateLimitResetAt,
		OverloadUntil:           a.OverloadUntil,
		TempUnschedulableUntil:  a.TempUnschedulableUntil,
		TempUnschedulableReason: a.TempUnschedulableReason,
		SessionWindowStart:      a.SessionWindowStart,
		SessionWindowEnd:        a.SessionWindowEnd,
		SessionWindowStatus:     a.SessionWindowStatus,
		GroupIDs:                a.GroupIDs,
	}

	// 提取 5h 窗口费用控制和会话数量控制配置（仅 Anthropic OAuth/SetupToken 账号有效）
	if a.IsAnthropicOAuthOrSetupToken() {
		if limit := a.GetWindowCostLimit(); limit > 0 {
			out.WindowCostLimit = &limit
		}
		if reserve := a.GetWindowCostStickyReserve(); reserve > 0 {
			out.WindowCostStickyReserve = &reserve
		}
		if maxSessions := a.GetMaxSessions(); maxSessions > 0 {
			out.MaxSessions = &maxSessions
		}
		if idleTimeout := a.GetSessionIdleTimeoutMinutes(); idleTimeout > 0 {
			out.SessionIdleTimeoutMin = &idleTimeout
		}
		if rpm := a.GetBaseRPM(); rpm > 0 {
			out.BaseRPM = &rpm
			strategy := a.GetRPMStrategy()
			out.RPMStrategy = &strategy
			buffer := a.GetRPMStickyBuffer()
			out.RPMStickyBuffer = &buffer
		}
		// 用户消息队列模式
		if mode := a.GetUserMsgQueueMode(); mode != "" {
			out.UserMsgQueueMode = &mode
		}
		// TLS指纹伪装开关
		if a.IsTLSFingerprintEnabled() {
			enabled := true
			out.EnableTLSFingerprint = &enabled
		}
		// TLS指纹模板ID
		if profileID := a.GetTLSFingerprintProfileID(); profileID > 0 {
			out.TLSFingerprintProfileID = &profileID
		}
		// 会话ID伪装开关
		if a.IsSessionIDMaskingEnabled() {
			enabled := true
			out.EnableSessionIDMasking = &enabled
		}
		// 缓存 TTL 强制替换
		if a.IsCacheTTLOverrideEnabled() {
			enabled := true
			out.CacheTTLOverrideEnabled = &enabled
			target := a.GetCacheTTLOverrideTarget()
			out.CacheTTLOverrideTarget = &target
		}
		// 自定义 Base URL 中继转发
		if a.IsCustomBaseURLEnabled() {
			enabled := true
			out.CustomBaseURLEnabled = &enabled
			if customURL := a.GetCustomBaseURL(); customURL != "" {
				out.CustomBaseURL = &customURL
			}
		}
	}

	// 提取账号配额限制（apikey / bedrock 类型有效）
	if a.IsAPIKeyOrBedrock() {
		if limit := a.GetQuotaLimit(); limit > 0 {
			out.QuotaLimit = &limit
			used := a.GetQuotaUsed()
			out.QuotaUsed = &used
		}
		if limit := a.GetQuotaDailyLimit(); limit > 0 {
			out.QuotaDailyLimit = &limit
			used := a.GetQuotaDailyUsed()
			if a.IsDailyQuotaPeriodExpired() {
				used = 0
			}
			out.QuotaDailyUsed = &used
		}
		if limit := a.GetQuotaWeeklyLimit(); limit > 0 {
			out.QuotaWeeklyLimit = &limit
			used := a.GetQuotaWeeklyUsed()
			if a.IsWeeklyQuotaPeriodExpired() {
				used = 0
			}
			out.QuotaWeeklyUsed = &used
		}
		// 固定时间重置配置
		if mode := a.GetQuotaDailyResetMode(); mode == "fixed" {
			out.QuotaDailyResetMode = &mode
			hour := a.GetQuotaDailyResetHour()
			out.QuotaDailyResetHour = &hour
		}
		if mode := a.GetQuotaWeeklyResetMode(); mode == "fixed" {
			out.QuotaWeeklyResetMode = &mode
			day := a.GetQuotaWeeklyResetDay()
			out.QuotaWeeklyResetDay = &day
			hour := a.GetQuotaWeeklyResetHour()
			out.QuotaWeeklyResetHour = &hour
		}
		if a.GetQuotaDailyResetMode() == "fixed" || a.GetQuotaWeeklyResetMode() == "fixed" {
			tz := a.GetQuotaResetTimezone()
			out.QuotaResetTimezone = &tz
		}
		if a.Extra != nil {
			if v, ok := a.Extra["quota_daily_reset_at"].(string); ok && v != "" {
				out.QuotaDailyResetAt = &v
			}
			if v, ok := a.Extra["quota_weekly_reset_at"].(string); ok && v != "" {
				out.QuotaWeeklyResetAt = &v
			}
		}

		// 配额通知配置
		if enabled := a.GetQuotaNotifyDailyEnabled(); enabled {
			out.QuotaNotifyDailyEnabled = &enabled
		}
		if threshold := a.GetQuotaNotifyDailyThreshold(); threshold > 0 {
			out.QuotaNotifyDailyThreshold = &threshold
		}
		if enabled := a.GetQuotaNotifyWeeklyEnabled(); enabled {
			out.QuotaNotifyWeeklyEnabled = &enabled
		}
		if threshold := a.GetQuotaNotifyWeeklyThreshold(); threshold > 0 {
			out.QuotaNotifyWeeklyThreshold = &threshold
		}
		if enabled := a.GetQuotaNotifyTotalEnabled(); enabled {
			out.QuotaNotifyTotalEnabled = &enabled
		}
		if threshold := a.GetQuotaNotifyTotalThreshold(); threshold > 0 {
			out.QuotaNotifyTotalThreshold = &threshold
		}
	}

	return out
}

func AccountFromService(a *service.Account) *Account {
	if a == nil {
		return nil
	}
	out := AccountFromServiceShallow(a)
	out.Proxy = ProxyFromService(a.Proxy)
	if len(a.AccountGroups) > 0 {
		out.AccountGroups = make([]AccountGroup, 0, len(a.AccountGroups))
		for i := range a.AccountGroups {
			ag := a.AccountGroups[i]
			out.AccountGroups = append(out.AccountGroups, *AccountGroupFromService(&ag))
		}
	}
	if len(a.Groups) > 0 {
		out.Groups = make([]*Group, 0, len(a.Groups))
		for _, g := range a.Groups {
			out.Groups = append(out.Groups, GroupFromServiceShallow(g))
		}
	}
	return out
}

func timeToUnixSeconds(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	ts := value.Unix()
	return &ts
}

func AccountGroupFromService(ag *service.AccountGroup) *AccountGroup {
	if ag == nil {
		return nil
	}
	return &AccountGroup{
		AccountID: ag.AccountID,
		GroupID:   ag.GroupID,
		Priority:  ag.Priority,
		CreatedAt: ag.CreatedAt,
		Account:   AccountFromServiceShallow(ag.Account),
		Group:     GroupFromServiceShallow(ag.Group),
	}
}

func ProxyFromService(p *service.Proxy) *Proxy {
	if p == nil {
		return nil
	}
	return &Proxy{
		ID:        p.ID,
		Name:      p.Name,
		Protocol:  p.Protocol,
		Host:      p.Host,
		Port:      p.Port,
		Username:  p.Username,
		Status:    p.Status,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

func ProxyWithAccountCountFromService(p *service.ProxyWithAccountCount) *ProxyWithAccountCount {
	if p == nil {
		return nil
	}
	return &ProxyWithAccountCount{
		Proxy:          *ProxyFromService(&p.Proxy),
		AccountCount:   p.AccountCount,
		LatencyMs:      p.LatencyMs,
		LatencyStatus:  p.LatencyStatus,
		LatencyMessage: p.LatencyMessage,
		IPAddress:      p.IPAddress,
		Country:        p.Country,
		CountryCode:    p.CountryCode,
		Region:         p.Region,
		City:           p.City,
		QualityStatus:  p.QualityStatus,
		QualityScore:   p.QualityScore,
		QualityGrade:   p.QualityGrade,
		QualitySummary: p.QualitySummary,
		QualityChecked: p.QualityChecked,
	}
}

// ProxyFromServiceAdmin converts a service Proxy to AdminProxy DTO for admin users.
// It includes the password field - user-facing endpoints must not use this.
func ProxyFromServiceAdmin(p *service.Proxy) *AdminProxy {
	if p == nil {
		return nil
	}
	base := ProxyFromService(p)
	if base == nil {
		return nil
	}
	return &AdminProxy{
		Proxy:    *base,
		Password: p.Password,
	}
}

// ProxyWithAccountCountFromServiceAdmin converts a service ProxyWithAccountCount to AdminProxyWithAccountCount DTO.
// It includes the password field - user-facing endpoints must not use this.
func ProxyWithAccountCountFromServiceAdmin(p *service.ProxyWithAccountCount) *AdminProxyWithAccountCount {
	if p == nil {
		return nil
	}
	admin := ProxyFromServiceAdmin(&p.Proxy)
	if admin == nil {
		return nil
	}
	return &AdminProxyWithAccountCount{
		AdminProxy:     *admin,
		AccountCount:   p.AccountCount,
		LatencyMs:      p.LatencyMs,
		LatencyStatus:  p.LatencyStatus,
		LatencyMessage: p.LatencyMessage,
		IPAddress:      p.IPAddress,
		Country:        p.Country,
		CountryCode:    p.CountryCode,
		Region:         p.Region,
		City:           p.City,
		QualityStatus:  p.QualityStatus,
		QualityScore:   p.QualityScore,
		QualityGrade:   p.QualityGrade,
		QualitySummary: p.QualitySummary,
		QualityChecked: p.QualityChecked,
	}
}

func ProxyAccountSummaryFromService(a *service.ProxyAccountSummary) *ProxyAccountSummary {
	if a == nil {
		return nil
	}
	return &ProxyAccountSummary{
		ID:       a.ID,
		Name:     a.Name,
		Platform: a.Platform,
		Type:     a.Type,
		Notes:    a.Notes,
	}
}

func RedeemCodeFromService(rc *service.RedeemCode) *RedeemCode {
	if rc == nil {
		return nil
	}
	out := redeemCodeFromServiceBase(rc)
	return &out
}

// RedeemCodeFromServiceAdmin converts a service RedeemCode to DTO for admin users.
// It includes notes - user-facing endpoints must not use this.
func RedeemCodeFromServiceAdmin(rc *service.RedeemCode) *AdminRedeemCode {
	if rc == nil {
		return nil
	}
	return &AdminRedeemCode{
		RedeemCode: redeemCodeFromServiceBase(rc),
		Notes:      rc.Notes,
	}
}

func redeemCodeFromServiceBase(rc *service.RedeemCode) RedeemCode {
	out := RedeemCode{
		ID:           rc.ID,
		Code:         rc.Code,
		Type:         rc.Type,
		Value:        rc.Value,
		Status:       rc.Status,
		UsedBy:       rc.UsedBy,
		UsedAt:       rc.UsedAt,
		CreatedAt:    rc.CreatedAt,
		GroupID:      rc.GroupID,
		ValidityDays: rc.ValidityDays,
		User:         UserFromServiceShallow(rc.User),
		Group:        GroupFromServiceShallow(rc.Group),
	}

	// For admin_balance/admin_concurrency types, include notes so users can see
	// why they were charged or credited by admin
	if (rc.Type == "admin_balance" || rc.Type == "admin_concurrency") && rc.Notes != "" {
		out.Notes = &rc.Notes
	}

	return out
}

// AccountSummaryFromService returns a minimal AccountSummary for usage log display.
// Only includes ID and Name - no sensitive fields like Credentials, Proxy, etc.
func AccountSummaryFromService(a *service.Account) *AccountSummary {
	if a == nil {
		return nil
	}
	return &AccountSummary{
		ID:   a.ID,
		Name: a.Name,
	}
}

func usageLogFromServiceUser(l *service.UsageLog) UsageLog {
	// 普通用户 DTO：严禁包含管理员字段（例如 account_rate_multiplier、ip_address、account）。
	requestType := l.EffectiveRequestType()
	stream, openAIWSMode := service.ApplyLegacyRequestFields(requestType, l.Stream, l.OpenAIWSMode)
	requestedModel := l.RequestedModel
	if requestedModel == "" {
		requestedModel = l.Model
	}
	return UsageLog{
		ID:                    l.ID,
		UserID:                l.UserID,
		APIKeyID:              l.APIKeyID,
		AccountID:             l.AccountID,
		RequestID:             l.RequestID,
		Model:                 requestedModel,
		ServiceTier:           l.ServiceTier,
		ReasoningEffort:       l.ReasoningEffort,
		InboundEndpoint:       l.InboundEndpoint,
		UpstreamEndpoint:      l.UpstreamEndpoint,
		GroupID:               l.GroupID,
		SubscriptionID:        l.SubscriptionID,
		InputTokens:           l.InputTokens,
		OutputTokens:          l.OutputTokens,
		CacheCreationTokens:   l.CacheCreationTokens,
		CacheReadTokens:       l.CacheReadTokens,
		CacheCreation5mTokens: l.CacheCreation5mTokens,
		CacheCreation1hTokens: l.CacheCreation1hTokens,
		InputCost:             l.InputCost,
		OutputCost:            l.OutputCost,
		CacheCreationCost:     l.CacheCreationCost,
		CacheReadCost:         l.CacheReadCost,
		TotalCost:             l.TotalCost,
		ActualCost:            l.ActualCost,
		RateMultiplier:        l.RateMultiplier,
		BillingType:           l.BillingType,
		RequestType:           requestType.String(),
		Stream:                stream,
		OpenAIWSMode:          openAIWSMode,
		DurationMs:            l.DurationMs,
		FirstTokenMs:          l.FirstTokenMs,
		ImageCount:            l.ImageCount,
		ImageSize:             l.ImageSize,
		MediaType:             l.MediaType,
		UserAgent:             l.UserAgent,
		CacheTTLOverridden:    l.CacheTTLOverridden,
		BillingMode:           l.BillingMode,
		CreatedAt:             l.CreatedAt,
		User:                  UserFromServiceShallow(l.User),
		APIKey:                APIKeyFromService(l.APIKey),
		Group:                 GroupFromServiceShallow(l.Group),
		Subscription:          UserSubscriptionFromService(l.Subscription),
	}
}

// UsageLogFromService converts a service UsageLog to DTO for regular users.
// It excludes Account details and IP address - users should not see these.
func UsageLogFromService(l *service.UsageLog) *UsageLog {
	if l == nil {
		return nil
	}
	u := usageLogFromServiceUser(l)
	return &u
}

// UsageLogFromServiceAdmin converts a service UsageLog to DTO for admin users.
// It includes minimal Account info (ID, Name only) and IP address.
func UsageLogFromServiceAdmin(l *service.UsageLog) *AdminUsageLog {
	if l == nil {
		return nil
	}
	return &AdminUsageLog{
		UsageLog:              usageLogFromServiceUser(l),
		UpstreamModel:         l.UpstreamModel,
		ChannelID:             l.ChannelID,
		ModelMappingChain:     l.ModelMappingChain,
		BillingTier:           l.BillingTier,
		AccountRateMultiplier: l.AccountRateMultiplier,
		IPAddress:             l.IPAddress,
		Account:               AccountSummaryFromService(l.Account),
	}
}

func UsageCleanupTaskFromService(task *service.UsageCleanupTask) *UsageCleanupTask {
	if task == nil {
		return nil
	}
	return &UsageCleanupTask{
		ID:     task.ID,
		Status: task.Status,
		Filters: UsageCleanupFilters{
			StartTime:   task.Filters.StartTime,
			EndTime:     task.Filters.EndTime,
			UserID:      task.Filters.UserID,
			APIKeyID:    task.Filters.APIKeyID,
			AccountID:   task.Filters.AccountID,
			GroupID:     task.Filters.GroupID,
			Model:       task.Filters.Model,
			RequestType: requestTypeStringPtr(task.Filters.RequestType),
			Stream:      task.Filters.Stream,
			BillingType: task.Filters.BillingType,
		},
		CreatedBy:    task.CreatedBy,
		DeletedRows:  task.DeletedRows,
		ErrorMessage: task.ErrorMsg,
		CanceledBy:   task.CanceledBy,
		CanceledAt:   task.CanceledAt,
		StartedAt:    task.StartedAt,
		FinishedAt:   task.FinishedAt,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}

func requestTypeStringPtr(requestType *int16) *string {
	if requestType == nil {
		return nil
	}
	value := service.RequestTypeFromInt16(*requestType).String()
	return &value
}

func SettingFromService(s *service.Setting) *Setting {
	if s == nil {
		return nil
	}
	return &Setting{
		ID:        s.ID,
		Key:       s.Key,
		Value:     s.Value,
		UpdatedAt: s.UpdatedAt,
	}
}

func UserSubscriptionFromService(sub *service.UserSubscription) *UserSubscription {
	if sub == nil {
		return nil
	}
	out := userSubscriptionFromServiceBase(sub)
	return &out
}

// UserSubscriptionFromServiceAdmin converts a service UserSubscription to DTO for admin users.
// It includes assignment metadata and notes.
func UserSubscriptionFromServiceAdmin(sub *service.UserSubscription) *AdminUserSubscription {
	if sub == nil {
		return nil
	}
	return &AdminUserSubscription{
		UserSubscription: userSubscriptionFromServiceBase(sub),
		AssignedBy:       sub.AssignedBy,
		AssignedAt:       sub.AssignedAt,
		Notes:            sub.Notes,
		AssignedByUser:   UserFromServiceShallow(sub.AssignedByUser),
	}
}

func userSubscriptionFromServiceBase(sub *service.UserSubscription) UserSubscription {
	return UserSubscription{
		ID:                 sub.ID,
		UserID:             sub.UserID,
		GroupID:            sub.GroupID,
		StartsAt:           sub.StartsAt,
		ExpiresAt:          sub.ExpiresAt,
		Status:             sub.Status,
		DailyWindowStart:   sub.DailyWindowStart,
		WeeklyWindowStart:  sub.WeeklyWindowStart,
		MonthlyWindowStart: sub.MonthlyWindowStart,
		DailyUsageUSD:      sub.DailyUsageUSD,
		WeeklyUsageUSD:     sub.WeeklyUsageUSD,
		MonthlyUsageUSD:    sub.MonthlyUsageUSD,
		CreatedAt:          sub.CreatedAt,
		UpdatedAt:          sub.UpdatedAt,
		User:               UserFromServiceShallow(sub.User),
		Group:              GroupFromServiceShallow(sub.Group),
	}
}

func BulkAssignResultFromService(r *service.BulkAssignResult) *BulkAssignResult {
	if r == nil {
		return nil
	}
	subs := make([]AdminUserSubscription, 0, len(r.Subscriptions))
	for i := range r.Subscriptions {
		subs = append(subs, *UserSubscriptionFromServiceAdmin(&r.Subscriptions[i]))
	}
	statuses := make(map[string]string, len(r.Statuses))
	for userID, status := range r.Statuses {
		statuses[strconv.FormatInt(userID, 10)] = status
	}
	return &BulkAssignResult{
		SuccessCount:  r.SuccessCount,
		CreatedCount:  r.CreatedCount,
		ReusedCount:   r.ReusedCount,
		FailedCount:   r.FailedCount,
		Subscriptions: subs,
		Errors:        r.Errors,
		Statuses:      statuses,
	}
}

func PromoCodeFromService(pc *service.PromoCode) *PromoCode {
	if pc == nil {
		return nil
	}
	return &PromoCode{
		ID:          pc.ID,
		Code:        pc.Code,
		BonusAmount: pc.BonusAmount,
		MaxUses:     pc.MaxUses,
		UsedCount:   pc.UsedCount,
		Status:      pc.Status,
		ExpiresAt:   pc.ExpiresAt,
		Notes:       pc.Notes,
		CreatedAt:   pc.CreatedAt,
		UpdatedAt:   pc.UpdatedAt,
	}
}

func PromoCodeUsageFromService(u *service.PromoCodeUsage) *PromoCodeUsage {
	if u == nil {
		return nil
	}
	return &PromoCodeUsage{
		ID:          u.ID,
		PromoCodeID: u.PromoCodeID,
		UserID:      u.UserID,
		BonusAmount: u.BonusAmount,
		UsedAt:      u.UsedAt,
		User:        UserFromServiceShallow(u.User),
	}
}
