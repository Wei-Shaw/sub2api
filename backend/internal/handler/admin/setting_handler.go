package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// semverPattern 预编译 semver 格式校验正则
var semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// menuItemIDPattern validates custom menu item IDs: alphanumeric, hyphens, underscores only.
var menuItemIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// generateMenuItemID generates a short random hex ID for a custom menu item.
func generateMenuItemID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate menu item ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func scopesContainOpenID(scopes string) bool {
	for _, scope := range strings.Fields(strings.ToLower(strings.TrimSpace(scopes))) {
		if scope == "openid" {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// SettingHandler 系统设置处理器
type SettingHandler struct {
	settingService           *service.SettingService
	emailService             *service.EmailService
	turnstileService         *service.TurnstileService
	opsService               *service.OpsService
	paymentConfigService     *service.PaymentConfigService
	paymentService           *service.PaymentService
	userAttributeService     *service.UserAttributeService
	notificationEmailService *service.NotificationEmailService
	totpService              *service.TotpService
	userService              *service.UserService
}

// NewSettingHandler 创建系统设置处理器
func NewSettingHandler(settingService *service.SettingService, emailService *service.EmailService, turnstileService *service.TurnstileService, opsService *service.OpsService, paymentConfigService *service.PaymentConfigService, paymentService *service.PaymentService, userAttributeService *service.UserAttributeService) *SettingHandler {
	return &SettingHandler{
		settingService:       settingService,
		emailService:         emailService,
		turnstileService:     turnstileService,
		opsService:           opsService,
		paymentConfigService: paymentConfigService,
		paymentService:       paymentService,
		userAttributeService: userAttributeService,
	}
}

// SetNotificationEmailService attaches the notification template service without changing
// the constructor signature used by existing unit tests.
func (h *SettingHandler) SetNotificationEmailService(notificationEmailService *service.NotificationEmailService) {
	h.notificationEmailService = notificationEmailService
}

// SetStepUpDeps attaches the services backing the step-up switch preconditions
// (enable requires the acting admin to have TOTP enabled; disable is itself a
// step-up gated operation), without changing the constructor signature used by
// existing unit tests.
func (h *SettingHandler) SetStepUpDeps(totpService *service.TotpService, userService *service.UserService) {
	h.totpService = totpService
	h.userService = userService
}

// GetSettings 获取所有系统设置
// GET /api/v1/admin/settings
func (h *SettingHandler) GetSettings(c *gin.Context) {
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	authSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Check if ops monitoring is enabled (respects config.ops.enabled)
	opsEnabled := h.opsService != nil && h.opsService.IsMonitoringEnabled(c.Request.Context())
	defaultSubscriptions := make([]dto.DefaultSubscriptionSetting, 0, len(settings.DefaultSubscriptions))
	for _, sub := range settings.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, dto.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// Load payment config
	var paymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		paymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
	}
	if paymentCfg == nil {
		paymentCfg = &service.PaymentConfig{}
	}

	payload := dto.SystemSettings{
		RegistrationEnabled:                                    settings.RegistrationEnabled,
		EmailVerifyEnabled:                                     settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:                       settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                                       settings.PromoCodeEnabled,
		PasswordResetEnabled:                                   settings.PasswordResetEnabled,
		FrontendURL:                                            settings.FrontendURL,
		InvitationCodeEnabled:                                  settings.InvitationCodeEnabled,
		TotpEnabled:                                            settings.TotpEnabled,
		TotpEncryptionKeyConfigured:                            h.settingService.IsTotpEncryptionKeyConfigured(),
		SessionBindingEnabled:                                  settings.SessionBindingEnabled,
		StepUpEnabled:                                          settings.StepUpEnabled,
		AuditLogRetentionDays:                                  settings.AuditLogRetentionDays,
		LoginAgreementEnabled:                                  settings.LoginAgreementEnabled,
		LoginAgreementMode:                                     settings.LoginAgreementMode,
		LoginAgreementUpdatedAt:                                settings.LoginAgreementUpdatedAt,
		LoginAgreementDocuments:                                loginAgreementDocumentsToDTO(settings.LoginAgreementDocuments),
		SMTPHost:                                               settings.SMTPHost,
		SMTPPort:                                               settings.SMTPPort,
		SMTPUsername:                                           settings.SMTPUsername,
		SMTPPasswordConfigured:                                 settings.SMTPPasswordConfigured,
		SMTPFrom:                                               settings.SMTPFrom,
		SMTPFromName:                                           settings.SMTPFromName,
		SMTPUseTLS:                                             settings.SMTPUseTLS,
		TurnstileEnabled:                                       settings.TurnstileEnabled,
		TurnstileSiteKey:                                       settings.TurnstileSiteKey,
		TurnstileSecretKeyConfigured:                           settings.TurnstileSecretKeyConfigured,
		APIKeyACLTrustForwardedIP:                              settings.APIKeyACLTrustForwardedIP,
		ForwardedClientIPHeaders:                               settings.ForwardedClientIPHeaders,
		LinuxDoConnectEnabled:                                  settings.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:                                 settings.LinuxDoConnectClientID,
		LinuxDoConnectClientSecretConfigured:                   settings.LinuxDoConnectClientSecretConfigured,
		LinuxDoConnectRedirectURL:                              settings.LinuxDoConnectRedirectURL,
		DingTalkConnectEnabled:                                 settings.DingTalkConnectEnabled,
		DingTalkConnectClientID:                                settings.DingTalkConnectClientID,
		DingTalkConnectClientSecretConfigured:                  settings.DingTalkConnectClientSecretConfigured,
		DingTalkConnectRedirectURL:                             settings.DingTalkConnectRedirectURL,
		DingTalkConnectCorpRestrictionPolicy:                   settings.DingTalkConnectCorpRestrictionPolicy,
		DingTalkConnectInternalCorpID:                          settings.DingTalkConnectInternalCorpID,
		DingTalkConnectBypassRegistration:                      settings.DingTalkConnectBypassRegistration,
		DingTalkConnectSyncCorpEmail:                           settings.DingTalkConnectSyncCorpEmail,
		DingTalkConnectSyncDisplayName:                         settings.DingTalkConnectSyncDisplayName,
		DingTalkConnectSyncDept:                                settings.DingTalkConnectSyncDept,
		DingTalkConnectSyncCorpEmailAttrKey:                    settings.DingTalkConnectSyncCorpEmailAttrKey,
		DingTalkConnectSyncDisplayNameAttrKey:                  settings.DingTalkConnectSyncDisplayNameAttrKey,
		DingTalkConnectSyncDeptAttrKey:                         settings.DingTalkConnectSyncDeptAttrKey,
		DingTalkConnectSyncCorpEmailAttrName:                   settings.DingTalkConnectSyncCorpEmailAttrName,
		DingTalkConnectSyncDisplayNameAttrName:                 settings.DingTalkConnectSyncDisplayNameAttrName,
		DingTalkConnectSyncDeptAttrName:                        settings.DingTalkConnectSyncDeptAttrName,
		WeChatConnectEnabled:                                   settings.WeChatConnectEnabled,
		WeChatConnectAppID:                                     settings.WeChatConnectAppID,
		WeChatConnectAppSecretConfigured:                       settings.WeChatConnectAppSecretConfigured,
		WeChatConnectOpenAppID:                                 settings.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecretConfigured:                   settings.WeChatConnectOpenAppSecretConfigured,
		WeChatConnectMPAppID:                                   settings.WeChatConnectMPAppID,
		WeChatConnectMPAppSecretConfigured:                     settings.WeChatConnectMPAppSecretConfigured,
		WeChatConnectMobileAppID:                               settings.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecretConfigured:                 settings.WeChatConnectMobileAppSecretConfigured,
		WeChatConnectOpenEnabled:                               settings.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:                                 settings.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:                             settings.WeChatConnectMobileEnabled,
		WeChatConnectMode:                                      settings.WeChatConnectMode,
		WeChatConnectScopes:                                    settings.WeChatConnectScopes,
		WeChatConnectRedirectURL:                               settings.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL:                       settings.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:                                     settings.OIDCConnectEnabled,
		OIDCConnectProviderName:                                settings.OIDCConnectProviderName,
		OIDCConnectClientID:                                    settings.OIDCConnectClientID,
		OIDCConnectClientSecretConfigured:                      settings.OIDCConnectClientSecretConfigured,
		OIDCConnectIssuerURL:                                   settings.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:                                settings.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:                                settings.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                                    settings.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:                                 settings.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                                     settings.OIDCConnectJWKSURL,
		OIDCConnectScopes:                                      settings.OIDCConnectScopes,
		OIDCConnectRedirectURL:                                 settings.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:                         settings.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:                             settings.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                                     settings.OIDCConnectUsePKCE,
		OIDCConnectValidateIDToken:                             settings.OIDCConnectValidateIDToken,
		OIDCConnectAllowedSigningAlgs:                          settings.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:                            settings.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:                        settings.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:                           settings.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:                              settings.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:                        settings.OIDCConnectUserInfoUsernamePath,
		GitHubOAuthEnabled:                                     settings.GitHubOAuthEnabled,
		GitHubOAuthClientID:                                    settings.GitHubOAuthClientID,
		GitHubOAuthClientSecretConfigured:                      settings.GitHubOAuthClientSecretConfigured,
		GitHubOAuthRedirectURL:                                 settings.GitHubOAuthRedirectURL,
		GitHubOAuthFrontendRedirectURL:                         settings.GitHubOAuthFrontendRedirectURL,
		GoogleOAuthEnabled:                                     settings.GoogleOAuthEnabled,
		GoogleOAuthClientID:                                    settings.GoogleOAuthClientID,
		GoogleOAuthClientSecretConfigured:                      settings.GoogleOAuthClientSecretConfigured,
		GoogleOAuthRedirectURL:                                 settings.GoogleOAuthRedirectURL,
		GoogleOAuthFrontendRedirectURL:                         settings.GoogleOAuthFrontendRedirectURL,
		SiteName:                                               settings.SiteName,
		SiteLogo:                                               settings.SiteLogo,
		SiteSubtitle:                                           settings.SiteSubtitle,
		APIBaseURL:                                             settings.APIBaseURL,
		ContactInfo:                                            settings.ContactInfo,
		DocURL:                                                 settings.DocURL,
		HomeContent:                                            settings.HomeContent,
		HideCcsImportButton:                                    settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:                            settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:                                settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:                                   settings.TableDefaultPageSize,
		TablePageSizeOptions:                                   settings.TablePageSizeOptions,
		CustomMenuItems:                                        dto.ParseCustomMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                                        dto.ParseCustomEndpoints(settings.CustomEndpoints),
		DefaultConcurrency:                                     settings.DefaultConcurrency,
		DefaultBalance:                                         settings.DefaultBalance,
		RiskControlEnabled:                                     settings.RiskControlEnabled,
		CyberSessionBlockEnabled:                               settings.CyberSessionBlockEnabled,
		CyberSessionBlockTTLSeconds:                            settings.CyberSessionBlockTTLSeconds,
		AffiliateRebateRate:                                    settings.AffiliateRebateRate,
		AffiliateRebateFreezeHours:                             settings.AffiliateRebateFreezeHours,
		AffiliateRebateDurationDays:                            settings.AffiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:                           settings.AffiliateRebatePerInviteeCap,
		AdminRechargeRebateEnabled:                             settings.AdminRechargeRebateEnabled,
		DefaultUserRPMLimit:                                    settings.DefaultUserRPMLimit,
		DefaultSubscriptions:                                   defaultSubscriptions,
		EnableModelFallback:                                    settings.EnableModelFallback,
		FallbackModelAnthropic:                                 settings.FallbackModelAnthropic,
		FallbackModelOpenAI:                                    settings.FallbackModelOpenAI,
		FallbackModelGemini:                                    settings.FallbackModelGemini,
		FallbackModelAntigravity:                               settings.FallbackModelAntigravity,
		EnableIdentityPatch:                                    settings.EnableIdentityPatch,
		IdentityPatchPrompt:                                    settings.IdentityPatchPrompt,
		OpsMonitoringEnabled:                                   opsEnabled && settings.OpsMonitoringEnabled,
		OpsRealtimeMonitoringEnabled:                           settings.OpsRealtimeMonitoringEnabled,
		OpsQueryModeDefault:                                    settings.OpsQueryModeDefault,
		OpsMetricsIntervalSeconds:                              settings.OpsMetricsIntervalSeconds,
		MinClaudeCodeVersion:                                   settings.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                                   settings.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:                            settings.AllowUngroupedKeyScheduling,
		BackendModeEnabled:                                     settings.BackendModeEnabled,
		EnableFingerprintUnification:                           settings.EnableFingerprintUnification,
		EnableMetadataPassthrough:                              settings.EnableMetadataPassthrough,
		EnableCCHSigning:                                       settings.EnableCCHSigning,
		EnableClaudeOAuthSystemPromptInjection:                 settings.EnableClaudeOAuthSystemPromptInjection,
		ClaudeOAuthSystemPrompt:                                settings.ClaudeOAuthSystemPrompt,
		ClaudeOAuthSystemPromptBlocks:                          settings.ClaudeOAuthSystemPromptBlocks,
		EnableAnthropicCacheTTL1hInjection:                     settings.EnableAnthropicCacheTTL1hInjection,
		RewriteMessageCacheControl:                             settings.RewriteMessageCacheControl,
		EnableClientDatelineNormalization:                      settings.EnableClientDatelineNormalization,
		AntigravityUserAgentVersion:                            settings.AntigravityUserAgentVersion,
		OpenAICodexUserAgent:                                   settings.OpenAICodexUserAgent,
		MinCodexVersion:                                        settings.MinCodexVersion,
		MaxCodexVersion:                                        settings.MaxCodexVersion,
		CodexCLIOnlyBlacklist:                                  settings.CodexCLIOnlyBlacklist,
		CodexCLIOnlyWhitelist:                                  settings.CodexCLIOnlyWhitelist,
		CodexCLIOnlyAllowAppServerClients:                      settings.CodexCLIOnlyAllowAppServerClients,
		CodexCLIOnlyEngineFingerprintSignals:                   settings.CodexCLIOnlyEngineFingerprintSignals,
		WebSearchEmulationEnabled:                              settings.WebSearchEmulationEnabled,
		PaymentVisibleMethodAlipaySource:                       settings.PaymentVisibleMethodAlipaySource,
		PaymentVisibleMethodWxpaySource:                        settings.PaymentVisibleMethodWxpaySource,
		PaymentVisibleMethodAlipayEnabled:                      settings.PaymentVisibleMethodAlipayEnabled,
		PaymentVisibleMethodWxpayEnabled:                       settings.PaymentVisibleMethodWxpayEnabled,
		OpenAILowUpstreamRatePriorityEnabled:                   settings.OpenAILowUpstreamRatePriorityEnabled,
		OpenAIOAuthSchedulingRateMultiplier:                    settings.OpenAIOAuthSchedulingRateMultiplier,
		OpenAIAdvancedSchedulerEnabled:                         settings.OpenAIAdvancedSchedulerEnabled,
		OpenAIAdvancedSchedulerStickyWeightedEnabled:           settings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled:     settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		OpenAIAdvancedSchedulerLBTopK:                          settings.OpenAIAdvancedSchedulerLBTopK,
		OpenAIAdvancedSchedulerWeightPriority:                  settings.OpenAIAdvancedSchedulerWeightPriority,
		OpenAIAdvancedSchedulerWeightLoad:                      settings.OpenAIAdvancedSchedulerWeightLoad,
		OpenAIAdvancedSchedulerWeightQueue:                     settings.OpenAIAdvancedSchedulerWeightQueue,
		OpenAIAdvancedSchedulerWeightErrorRate:                 settings.OpenAIAdvancedSchedulerWeightErrorRate,
		OpenAIAdvancedSchedulerWeightTTFT:                      settings.OpenAIAdvancedSchedulerWeightTTFT,
		OpenAIAdvancedSchedulerWeightReset:                     settings.OpenAIAdvancedSchedulerWeightReset,
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:             settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerWeightUpstreamCost:              settings.OpenAIAdvancedSchedulerWeightUpstreamCost,
		OpenAIAdvancedSchedulerWeightPreviousResponse:          settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		OpenAIAdvancedSchedulerWeightSessionSticky:             settings.OpenAIAdvancedSchedulerWeightSessionSticky,
		OpenAIAdvancedSchedulerEffectiveLBTopK:                 settings.OpenAIAdvancedSchedulerEffectiveLBTopK,
		OpenAIAdvancedSchedulerEffectiveWeightPriority:         settings.OpenAIAdvancedSchedulerEffectiveWeightPriority,
		OpenAIAdvancedSchedulerEffectiveWeightLoad:             settings.OpenAIAdvancedSchedulerEffectiveWeightLoad,
		OpenAIAdvancedSchedulerEffectiveWeightQueue:            settings.OpenAIAdvancedSchedulerEffectiveWeightQueue,
		OpenAIAdvancedSchedulerEffectiveWeightErrorRate:        settings.OpenAIAdvancedSchedulerEffectiveWeightErrorRate,
		OpenAIAdvancedSchedulerEffectiveWeightTTFT:             settings.OpenAIAdvancedSchedulerEffectiveWeightTTFT,
		OpenAIAdvancedSchedulerEffectiveWeightReset:            settings.OpenAIAdvancedSchedulerEffectiveWeightReset,
		OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom:    settings.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom,
		OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost:     settings.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost,
		OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse: settings.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse,
		OpenAIAdvancedSchedulerEffectiveWeightSessionSticky:    settings.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky,
		BalanceLowNotifyEnabled:                                settings.BalanceLowNotifyEnabled,
		BalanceLowNotifyThreshold:                              settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:                            settings.BalanceLowNotifyRechargeURL,
		SubscriptionExpiryNotifyEnabled:                        settings.SubscriptionExpiryNotifyEnabled,
		AccountQuotaNotifyEnabled:                              settings.AccountQuotaNotifyEnabled,
		AccountQuotaNotifyEmails:                               dto.NotifyEmailEntriesFromService(settings.AccountQuotaNotifyEmails),
		PaymentEnabled:                                         paymentCfg.Enabled,
		PaymentMinAmount:                                       paymentCfg.MinAmount,
		PaymentMaxAmount:                                       paymentCfg.MaxAmount,
		PaymentDailyLimit:                                      paymentCfg.DailyLimit,
		PaymentOrderTimeoutMin:                                 paymentCfg.OrderTimeoutMin,
		PaymentMaxPendingOrders:                                paymentCfg.MaxPendingOrders,
		PaymentEnabledTypes:                                    paymentCfg.EnabledTypes,
		PaymentBalanceDisabled:                                 paymentCfg.BalanceDisabled,
		PaymentBalanceRechargeMultiplier:                       paymentCfg.BalanceRechargeMultiplier,
		PaymentSubscriptionUSDToCNYRate:                        paymentCfg.SubscriptionUSDToCNYRate,
		PaymentRechargeFeeRate:                                 paymentCfg.RechargeFeeRate,
		PaymentLoadBalanceStrat:                                paymentCfg.LoadBalanceStrategy,
		PaymentProductNamePrefix:                               paymentCfg.ProductNamePrefix,
		PaymentProductNameSuffix:                               paymentCfg.ProductNameSuffix,
		PaymentHelpImageURL:                                    paymentCfg.HelpImageURL,
		PaymentHelpText:                                        paymentCfg.HelpText,
		PaymentCancelRateLimitEnabled:                          paymentCfg.CancelRateLimitEnabled,
		PaymentCancelRateLimitMax:                              paymentCfg.CancelRateLimitMax,
		PaymentCancelRateLimitWindow:                           paymentCfg.CancelRateLimitWindow,
		PaymentCancelRateLimitUnit:                             paymentCfg.CancelRateLimitUnit,
		PaymentCancelRateLimitMode:                             paymentCfg.CancelRateLimitMode,
		PaymentAlipayForceQRCode:                               paymentCfg.AlipayForceQRCode,
		PaymentAlipayMobilePrecreateDeepLink:                   paymentCfg.AlipayMobilePrecreateDeepLink,

		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,

		AvailableChannelsEnabled: settings.AvailableChannelsEnabled,

		AffiliateEnabled: settings.AffiliateEnabled,

		AllowUserViewErrorRequests: settings.AllowUserViewErrorRequests,
	}

	// OpenAI fast policy (stored under a dedicated setting key)
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
	} else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
	}

	// Default platform quotas（JSON map）
	if platformQuotas, err := h.settingService.GetDefaultPlatformQuotas(c.Request.Context()); err != nil {
		slog.Error("default_platform_quotas_get_failed", "error", err)
	} else {
		payload.DefaultPlatformQuotas = platformQuotas
	}

	response.Success(c, systemSettingsResponseData(payload, authSourceDefaults))
}

// openaiFastPolicySettingsToDTO converts service -> dto for OpenAI fast policy.
func openaiFastPolicySettingsToDTO(s *service.OpenAIFastPolicySettings) *dto.OpenAIFastPolicySettings {
	if s == nil {
		return nil
	}
	rules := make([]dto.OpenAIFastPolicyRule, len(s.Rules))
	for i, r := range s.Rules {
		rules[i] = dto.OpenAIFastPolicyRule(r)
	}
	return &dto.OpenAIFastPolicySettings{Rules: rules}
}

// openaiFastPolicySettingsFromDTO converts dto -> service for OpenAI fast policy.
//
// 规范化 ServiceTier：在 DTO 进入 service 层之前统一把空字符串归一为
// service.OpenAIFastTierAny ("all")，避免管理员保存时空串与 "all" 同时
// 表达"匹配任意 tier"造成数据库取值的二义性。其它非空值原样透传，由
// service.SetOpenAIFastPolicySettings 负责合法值校验。
func openaiFastPolicySettingsFromDTO(s *dto.OpenAIFastPolicySettings) *service.OpenAIFastPolicySettings {
	if s == nil {
		return nil
	}
	rules := make([]service.OpenAIFastPolicyRule, len(s.Rules))
	for i, r := range s.Rules {
		rules[i] = service.OpenAIFastPolicyRule(r)
		tier := strings.ToLower(strings.TrimSpace(rules[i].ServiceTier))
		if tier == "" {
			tier = service.OpenAIFastTierAny
		}
		rules[i].ServiceTier = tier
	}
	return &service.OpenAIFastPolicySettings{Rules: rules}
}

func loginAgreementDocumentsToDTO(items []service.LoginAgreementDocument) []dto.LoginAgreementDocument {
	result := make([]dto.LoginAgreementDocument, 0, len(items))
	for _, item := range items {
		result = append(result, dto.LoginAgreementDocument{
			ID:        item.ID,
			Title:     item.Title,
			ContentMD: item.ContentMD,
		})
	}
	return result
}

func loginAgreementDocumentsToService(items []dto.LoginAgreementDocument) []service.LoginAgreementDocument {
	result := make([]service.LoginAgreementDocument, 0, len(items))
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		content := strings.TrimSpace(item.ContentMD)
		if title == "" && content == "" {
			continue
		}
		result = append(result, service.LoginAgreementDocument{
			ID:        strings.TrimSpace(item.ID),
			Title:     title,
			ContentMD: content,
		})
	}
	return result
}

func systemSettingsResponseData(settings dto.SystemSettings, authSourceDefaults *service.AuthSourceDefaultSettings) map[string]any {
	data := make(map[string]any)
	raw, err := json.Marshal(settings)
	if err == nil {
		_ = json.Unmarshal(raw, &data)
	}
	if authSourceDefaults == nil {
		authSourceDefaults = &service.AuthSourceDefaultSettings{}
	}

	data["auth_source_default_email_balance"] = authSourceDefaults.Email.Balance
	data["auth_source_default_email_concurrency"] = authSourceDefaults.Email.Concurrency
	data["auth_source_default_email_subscriptions"] = authSourceDefaults.Email.Subscriptions
	data["auth_source_default_email_grant_on_signup"] = authSourceDefaults.Email.GrantOnSignup
	data["auth_source_default_email_grant_on_first_bind"] = authSourceDefaults.Email.GrantOnFirstBind
	data["auth_source_default_linuxdo_balance"] = authSourceDefaults.LinuxDo.Balance
	data["auth_source_default_linuxdo_concurrency"] = authSourceDefaults.LinuxDo.Concurrency
	data["auth_source_default_linuxdo_subscriptions"] = authSourceDefaults.LinuxDo.Subscriptions
	data["auth_source_default_linuxdo_grant_on_signup"] = authSourceDefaults.LinuxDo.GrantOnSignup
	data["auth_source_default_linuxdo_grant_on_first_bind"] = authSourceDefaults.LinuxDo.GrantOnFirstBind
	data["auth_source_default_dingtalk_balance"] = authSourceDefaults.DingTalk.Balance
	data["auth_source_default_dingtalk_concurrency"] = authSourceDefaults.DingTalk.Concurrency
	data["auth_source_default_dingtalk_subscriptions"] = authSourceDefaults.DingTalk.Subscriptions
	data["auth_source_default_dingtalk_grant_on_signup"] = authSourceDefaults.DingTalk.GrantOnSignup
	data["auth_source_default_dingtalk_grant_on_first_bind"] = authSourceDefaults.DingTalk.GrantOnFirstBind
	data["auth_source_default_oidc_balance"] = authSourceDefaults.OIDC.Balance
	data["auth_source_default_oidc_concurrency"] = authSourceDefaults.OIDC.Concurrency
	data["auth_source_default_oidc_subscriptions"] = authSourceDefaults.OIDC.Subscriptions
	data["auth_source_default_oidc_grant_on_signup"] = authSourceDefaults.OIDC.GrantOnSignup
	data["auth_source_default_oidc_grant_on_first_bind"] = authSourceDefaults.OIDC.GrantOnFirstBind
	data["auth_source_default_wechat_balance"] = authSourceDefaults.WeChat.Balance
	data["auth_source_default_wechat_concurrency"] = authSourceDefaults.WeChat.Concurrency
	data["auth_source_default_wechat_subscriptions"] = authSourceDefaults.WeChat.Subscriptions
	data["auth_source_default_wechat_grant_on_signup"] = authSourceDefaults.WeChat.GrantOnSignup
	data["auth_source_default_wechat_grant_on_first_bind"] = authSourceDefaults.WeChat.GrantOnFirstBind
	data["auth_source_default_github_balance"] = authSourceDefaults.GitHub.Balance
	data["auth_source_default_github_concurrency"] = authSourceDefaults.GitHub.Concurrency
	data["auth_source_default_github_subscriptions"] = authSourceDefaults.GitHub.Subscriptions
	data["auth_source_default_github_grant_on_signup"] = authSourceDefaults.GitHub.GrantOnSignup
	data["auth_source_default_github_grant_on_first_bind"] = authSourceDefaults.GitHub.GrantOnFirstBind
	data["auth_source_default_google_balance"] = authSourceDefaults.Google.Balance
	data["auth_source_default_google_concurrency"] = authSourceDefaults.Google.Concurrency
	data["auth_source_default_google_subscriptions"] = authSourceDefaults.Google.Subscriptions
	data["auth_source_default_google_grant_on_signup"] = authSourceDefaults.Google.GrantOnSignup
	data["auth_source_default_google_grant_on_first_bind"] = authSourceDefaults.Google.GrantOnFirstBind
	data["auth_source_default_email_platform_quotas"] = authSourceDefaults.Email.PlatformQuotas
	data["auth_source_default_linuxdo_platform_quotas"] = authSourceDefaults.LinuxDo.PlatformQuotas
	data["auth_source_default_oidc_platform_quotas"] = authSourceDefaults.OIDC.PlatformQuotas
	data["auth_source_default_wechat_platform_quotas"] = authSourceDefaults.WeChat.PlatformQuotas
	data["auth_source_default_github_platform_quotas"] = authSourceDefaults.GitHub.PlatformQuotas
	data["auth_source_default_google_platform_quotas"] = authSourceDefaults.Google.PlatformQuotas
	data["auth_source_default_dingtalk_platform_quotas"] = authSourceDefaults.DingTalk.PlatformQuotas
	data["force_email_on_third_party_signup"] = authSourceDefaults.ForceEmailOnThirdPartySignup

	return data
}
