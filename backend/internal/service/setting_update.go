package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
	}

	err = s.settingRepo.SetMultiple(ctx, updates)
	if err == nil {
		s.refreshCachedSettings(settings)
	}
	return err
}

// UpdateSettingsWithAuthSourceDefaults persists system settings and auth-source defaults in a single write.
func (s *SettingService) UpdateSettingsWithAuthSourceDefaults(ctx context.Context, settings *SystemSettings, authDefaults *AuthSourceDefaultSettings) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
	}

	authSourceUpdates, err := s.buildAuthSourceDefaultUpdates(ctx, authDefaults)
	if err != nil {
		return err
	}
	for key, value := range authSourceUpdates {
		updates[key] = value
	}

	err = s.settingRepo.SetMultiple(ctx, updates)
	if err == nil {
		s.refreshCachedSettings(settings)
	}
	return err
}

func (s *SettingService) buildSystemSettingsUpdates(ctx context.Context, settings *SystemSettings) (map[string]string, error) {
	if err := s.validateDefaultSubscriptionGroups(ctx, settings.DefaultSubscriptions); err != nil {
		return nil, err
	}
	normalizedWhitelist, err := NormalizeRegistrationEmailSuffixWhitelist(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", err.Error())
	}
	if normalizedWhitelist == nil {
		normalizedWhitelist = []string{}
	}
	settings.RegistrationEmailSuffixWhitelist = normalizedWhitelist
	alipaySource, err := normalizeVisibleMethodSettingSource("alipay", settings.PaymentVisibleMethodAlipaySource, settings.PaymentVisibleMethodAlipayEnabled)
	if err != nil {
		return nil, err
	}
	wxpaySource, err := normalizeVisibleMethodSettingSource("wxpay", settings.PaymentVisibleMethodWxpaySource, settings.PaymentVisibleMethodWxpayEnabled)
	if err != nil {
		return nil, err
	}
	if err := s.normalizeOpenAIAdvancedSchedulerOverrides(settings); err != nil {
		return nil, err
	}
	settings.PaymentVisibleMethodAlipaySource = alipaySource
	settings.PaymentVisibleMethodWxpaySource = wxpaySource
	settings.WeChatConnectAppID = strings.TrimSpace(settings.WeChatConnectAppID)
	settings.WeChatConnectAppSecret = strings.TrimSpace(settings.WeChatConnectAppSecret)
	settings.WeChatConnectOpenAppID = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectOpenAppID, settings.WeChatConnectAppID))
	settings.WeChatConnectOpenAppSecret = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectOpenAppSecret, settings.WeChatConnectAppSecret))
	settings.WeChatConnectMPAppID = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMPAppID, settings.WeChatConnectAppID))
	settings.WeChatConnectMPAppSecret = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMPAppSecret, settings.WeChatConnectAppSecret))
	settings.WeChatConnectMobileAppID = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMobileAppID, settings.WeChatConnectAppID))
	settings.WeChatConnectMobileAppSecret = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMobileAppSecret, settings.WeChatConnectAppSecret))
	settings.WeChatConnectMode = normalizeWeChatConnectStoredMode(
		settings.WeChatConnectOpenEnabled,
		settings.WeChatConnectMPEnabled,
		settings.WeChatConnectMobileEnabled,
		settings.WeChatConnectMode,
	)
	settings.WeChatConnectScopes = normalizeWeChatConnectScopeSetting(settings.WeChatConnectScopes, settings.WeChatConnectMode)
	settings.WeChatConnectRedirectURL = strings.TrimSpace(settings.WeChatConnectRedirectURL)
	settings.WeChatConnectFrontendRedirectURL = strings.TrimSpace(settings.WeChatConnectFrontendRedirectURL)
	if settings.WeChatConnectFrontendRedirectURL == "" {
		settings.WeChatConnectFrontendRedirectURL = defaultWeChatConnectFrontend
	}
	settings.GitHubOAuthRedirectURL = strings.TrimSpace(settings.GitHubOAuthRedirectURL)
	settings.GitHubOAuthFrontendRedirectURL = strings.TrimSpace(settings.GitHubOAuthFrontendRedirectURL)
	if settings.GitHubOAuthFrontendRedirectURL == "" {
		settings.GitHubOAuthFrontendRedirectURL = defaultGitHubOAuthFrontend
	}
	settings.GoogleOAuthRedirectURL = strings.TrimSpace(settings.GoogleOAuthRedirectURL)
	settings.GoogleOAuthFrontendRedirectURL = strings.TrimSpace(settings.GoogleOAuthFrontendRedirectURL)
	if settings.GoogleOAuthFrontendRedirectURL == "" {
		settings.GoogleOAuthFrontendRedirectURL = defaultGoogleOAuthFrontend
	}

	updates := make(map[string]string)

	// 注册设置
	updates[SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)
	registrationEmailSuffixWhitelistJSON, err := json.Marshal(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return nil, fmt.Errorf("marshal registration email suffix whitelist: %w", err)
	}
	updates[SettingKeyRegistrationEmailSuffixWhitelist] = string(registrationEmailSuffixWhitelistJSON)
	updates[SettingKeyPromoCodeEnabled] = strconv.FormatBool(settings.PromoCodeEnabled)
	updates[SettingKeyPasswordResetEnabled] = strconv.FormatBool(settings.PasswordResetEnabled)
	updates[SettingKeyFrontendURL] = settings.FrontendURL
	updates[SettingKeyInvitationCodeEnabled] = strconv.FormatBool(settings.InvitationCodeEnabled)
	updates[SettingKeyTotpEnabled] = strconv.FormatBool(settings.TotpEnabled)
	settings.LoginAgreementMode = normalizeLoginAgreementMode(settings.LoginAgreementMode)
	settings.LoginAgreementUpdatedAt = strings.TrimSpace(settings.LoginAgreementUpdatedAt)
	if settings.LoginAgreementUpdatedAt == "" {
		settings.LoginAgreementUpdatedAt = defaultLoginAgreementDate
	}
	loginAgreementDocumentsJSON, err := marshalLoginAgreementDocuments(settings.LoginAgreementDocuments)
	if err != nil {
		return nil, err
	}
	updates[SettingKeyLoginAgreementEnabled] = strconv.FormatBool(settings.LoginAgreementEnabled)
	updates[SettingKeyLoginAgreementMode] = settings.LoginAgreementMode
	updates[SettingKeyLoginAgreementUpdatedAt] = settings.LoginAgreementUpdatedAt
	updates[SettingKeyLoginAgreementDocuments] = loginAgreementDocumentsJSON

	// 邮件服务设置（只有非空才更新密码）
	updates[SettingKeySMTPHost] = settings.SMTPHost
	updates[SettingKeySMTPPort] = strconv.Itoa(settings.SMTPPort)
	updates[SettingKeySMTPUsername] = settings.SMTPUsername
	if settings.SMTPPassword != "" {
		updates[SettingKeySMTPPassword] = settings.SMTPPassword
	}
	updates[SettingKeySMTPFrom] = settings.SMTPFrom
	updates[SettingKeySMTPFromName] = settings.SMTPFromName
	updates[SettingKeySMTPUseTLS] = strconv.FormatBool(settings.SMTPUseTLS)

	// Cloudflare Turnstile 设置（只有非空才更新密钥）
	updates[SettingKeyTurnstileEnabled] = strconv.FormatBool(settings.TurnstileEnabled)
	updates[SettingKeyTurnstileSiteKey] = settings.TurnstileSiteKey
	if settings.TurnstileSecretKey != "" {
		updates[SettingKeyTurnstileSecretKey] = settings.TurnstileSecretKey
	}
	updates[SettingKeyAPIKeyACLTrustForwardedIP] = strconv.FormatBool(settings.APIKeyACLTrustForwardedIP)

	// LinuxDo Connect OAuth 登录
	updates[SettingKeyLinuxDoConnectEnabled] = strconv.FormatBool(settings.LinuxDoConnectEnabled)
	updates[SettingKeyLinuxDoConnectClientID] = settings.LinuxDoConnectClientID
	updates[SettingKeyLinuxDoConnectRedirectURL] = settings.LinuxDoConnectRedirectURL
	if settings.LinuxDoConnectClientSecret != "" {
		updates[SettingKeyLinuxDoConnectClientSecret] = settings.LinuxDoConnectClientSecret
	}

	// DingTalk Connect OAuth 登录
	updates[SettingKeyDingTalkConnectEnabled] = strconv.FormatBool(settings.DingTalkConnectEnabled)
	updates[SettingKeyDingTalkConnectClientID] = settings.DingTalkConnectClientID
	updates[SettingKeyDingTalkConnectRedirectURL] = settings.DingTalkConnectRedirectURL
	if settings.DingTalkConnectClientSecret != "" {
		updates[SettingKeyDingTalkConnectClientSecret] = settings.DingTalkConnectClientSecret
	}
	updates[SettingKeyDingTalkConnectCorpRestrictionPolicy] = settings.DingTalkConnectCorpRestrictionPolicy
	updates[SettingKeyDingTalkConnectInternalCorpID] = settings.DingTalkConnectInternalCorpID
	updates[SettingKeyDingTalkConnectBypassRegistration] = strconv.FormatBool(settings.DingTalkConnectBypassRegistration)
	updates[SettingKeyDingTalkConnectSyncCorpEmail] = strconv.FormatBool(settings.DingTalkConnectSyncCorpEmail)
	updates[SettingKeyDingTalkConnectSyncDisplayName] = strconv.FormatBool(settings.DingTalkConnectSyncDisplayName)
	updates[SettingKeyDingTalkConnectSyncDept] = strconv.FormatBool(settings.DingTalkConnectSyncDept)
	updates[SettingKeyDingTalkConnectSyncCorpEmailAttrKey] = settings.DingTalkConnectSyncCorpEmailAttrKey
	updates[SettingKeyDingTalkConnectSyncDisplayNameAttrKey] = settings.DingTalkConnectSyncDisplayNameAttrKey
	updates[SettingKeyDingTalkConnectSyncDeptAttrKey] = settings.DingTalkConnectSyncDeptAttrKey
	updates[SettingKeyDingTalkConnectSyncCorpEmailAttrName] = settings.DingTalkConnectSyncCorpEmailAttrName
	updates[SettingKeyDingTalkConnectSyncDisplayNameAttrName] = settings.DingTalkConnectSyncDisplayNameAttrName
	updates[SettingKeyDingTalkConnectSyncDeptAttrName] = settings.DingTalkConnectSyncDeptAttrName

	// Generic OIDC OAuth 登录
	updates[SettingKeyOIDCConnectEnabled] = strconv.FormatBool(settings.OIDCConnectEnabled)
	updates[SettingKeyOIDCConnectProviderName] = settings.OIDCConnectProviderName
	updates[SettingKeyOIDCConnectClientID] = settings.OIDCConnectClientID
	updates[SettingKeyOIDCConnectIssuerURL] = settings.OIDCConnectIssuerURL
	updates[SettingKeyOIDCConnectDiscoveryURL] = settings.OIDCConnectDiscoveryURL
	updates[SettingKeyOIDCConnectAuthorizeURL] = settings.OIDCConnectAuthorizeURL
	updates[SettingKeyOIDCConnectTokenURL] = settings.OIDCConnectTokenURL
	updates[SettingKeyOIDCConnectUserInfoURL] = settings.OIDCConnectUserInfoURL
	updates[SettingKeyOIDCConnectJWKSURL] = settings.OIDCConnectJWKSURL
	updates[SettingKeyOIDCConnectScopes] = settings.OIDCConnectScopes
	updates[SettingKeyOIDCConnectRedirectURL] = settings.OIDCConnectRedirectURL
	updates[SettingKeyOIDCConnectFrontendRedirectURL] = settings.OIDCConnectFrontendRedirectURL
	updates[SettingKeyOIDCConnectTokenAuthMethod] = settings.OIDCConnectTokenAuthMethod
	updates[SettingKeyOIDCConnectUsePKCE] = strconv.FormatBool(settings.OIDCConnectUsePKCE)
	updates[SettingKeyOIDCConnectValidateIDToken] = strconv.FormatBool(settings.OIDCConnectValidateIDToken)
	updates[SettingKeyOIDCConnectAllowedSigningAlgs] = settings.OIDCConnectAllowedSigningAlgs
	updates[SettingKeyOIDCConnectClockSkewSeconds] = strconv.Itoa(settings.OIDCConnectClockSkewSeconds)
	updates[SettingKeyOIDCConnectRequireEmailVerified] = strconv.FormatBool(settings.OIDCConnectRequireEmailVerified)
	updates[SettingKeyOIDCConnectUserInfoEmailPath] = settings.OIDCConnectUserInfoEmailPath
	updates[SettingKeyOIDCConnectUserInfoIDPath] = settings.OIDCConnectUserInfoIDPath
	updates[SettingKeyOIDCConnectUserInfoUsernamePath] = settings.OIDCConnectUserInfoUsernamePath
	if settings.OIDCConnectClientSecret != "" {
		updates[SettingKeyOIDCConnectClientSecret] = settings.OIDCConnectClientSecret
	}

	// GitHub / Google 邮箱快捷登录
	updates[SettingKeyGitHubOAuthEnabled] = strconv.FormatBool(settings.GitHubOAuthEnabled)
	updates[SettingKeyGitHubOAuthClientID] = strings.TrimSpace(settings.GitHubOAuthClientID)
	updates[SettingKeyGitHubOAuthRedirectURL] = settings.GitHubOAuthRedirectURL
	updates[SettingKeyGitHubOAuthFrontendRedirectURL] = settings.GitHubOAuthFrontendRedirectURL
	if settings.GitHubOAuthClientSecret != "" {
		updates[SettingKeyGitHubOAuthClientSecret] = strings.TrimSpace(settings.GitHubOAuthClientSecret)
	}
	updates[SettingKeyGoogleOAuthEnabled] = strconv.FormatBool(settings.GoogleOAuthEnabled)
	updates[SettingKeyGoogleOAuthClientID] = strings.TrimSpace(settings.GoogleOAuthClientID)
	updates[SettingKeyGoogleOAuthRedirectURL] = settings.GoogleOAuthRedirectURL
	updates[SettingKeyGoogleOAuthFrontendRedirectURL] = settings.GoogleOAuthFrontendRedirectURL
	if settings.GoogleOAuthClientSecret != "" {
		updates[SettingKeyGoogleOAuthClientSecret] = strings.TrimSpace(settings.GoogleOAuthClientSecret)
	}

	// WeChat Connect OAuth 登录
	updates[SettingKeyWeChatConnectEnabled] = strconv.FormatBool(settings.WeChatConnectEnabled)
	updates[SettingKeyWeChatConnectAppID] = settings.WeChatConnectAppID
	updates[SettingKeyWeChatConnectOpenAppID] = settings.WeChatConnectOpenAppID
	updates[SettingKeyWeChatConnectMPAppID] = settings.WeChatConnectMPAppID
	updates[SettingKeyWeChatConnectMobileAppID] = settings.WeChatConnectMobileAppID
	updates[SettingKeyWeChatConnectOpenEnabled] = strconv.FormatBool(settings.WeChatConnectOpenEnabled)
	updates[SettingKeyWeChatConnectMPEnabled] = strconv.FormatBool(settings.WeChatConnectMPEnabled)
	updates[SettingKeyWeChatConnectMobileEnabled] = strconv.FormatBool(settings.WeChatConnectMobileEnabled)
	updates[SettingKeyWeChatConnectMode] = settings.WeChatConnectMode
	updates[SettingKeyWeChatConnectScopes] = settings.WeChatConnectScopes
	updates[SettingKeyWeChatConnectRedirectURL] = settings.WeChatConnectRedirectURL
	updates[SettingKeyWeChatConnectFrontendRedirectURL] = settings.WeChatConnectFrontendRedirectURL
	if settings.WeChatConnectAppSecret != "" {
		updates[SettingKeyWeChatConnectAppSecret] = settings.WeChatConnectAppSecret
	}
	if settings.WeChatConnectOpenAppSecret != "" {
		updates[SettingKeyWeChatConnectOpenAppSecret] = settings.WeChatConnectOpenAppSecret
	}
	if settings.WeChatConnectMPAppSecret != "" {
		updates[SettingKeyWeChatConnectMPAppSecret] = settings.WeChatConnectMPAppSecret
	}
	if settings.WeChatConnectMobileAppSecret != "" {
		updates[SettingKeyWeChatConnectMobileAppSecret] = settings.WeChatConnectMobileAppSecret
	}

	// OEM设置
	updates[SettingKeySiteName] = settings.SiteName
	updates[SettingKeySiteLogo] = settings.SiteLogo
	updates[SettingKeySiteSubtitle] = settings.SiteSubtitle
	updates[SettingKeyAPIBaseURL] = settings.APIBaseURL
	updates[SettingKeyContactInfo] = settings.ContactInfo
	updates[SettingKeyDocURL] = settings.DocURL
	updates[SettingKeyHomeContent] = settings.HomeContent
	updates[SettingKeyHideCcsImportButton] = strconv.FormatBool(settings.HideCcsImportButton)
	updates[SettingKeyPurchaseSubscriptionEnabled] = strconv.FormatBool(settings.PurchaseSubscriptionEnabled)
	updates[SettingKeyPurchaseSubscriptionURL] = strings.TrimSpace(settings.PurchaseSubscriptionURL)
	tableDefaultPageSize, tablePageSizeOptions := normalizeTablePreferences(
		settings.TableDefaultPageSize,
		settings.TablePageSizeOptions,
	)
	updates[SettingKeyTableDefaultPageSize] = strconv.Itoa(tableDefaultPageSize)
	tablePageSizeOptionsJSON, err := json.Marshal(tablePageSizeOptions)
	if err != nil {
		return nil, fmt.Errorf("marshal table page size options: %w", err)
	}
	updates[SettingKeyTablePageSizeOptions] = string(tablePageSizeOptionsJSON)
	updates[SettingKeyCustomMenuItems] = settings.CustomMenuItems
	updates[SettingKeyCustomEndpoints] = settings.CustomEndpoints

	// 默认配置
	updates[SettingKeyDefaultConcurrency] = strconv.Itoa(settings.DefaultConcurrency)
	updates[SettingKeyDefaultBalance] = strconv.FormatFloat(settings.DefaultBalance, 'f', 8, 64)
	settings.AffiliateRebateRate = clampAffiliateRebateRate(settings.AffiliateRebateRate)
	updates[SettingKeyAffiliateRebateRate] = strconv.FormatFloat(settings.AffiliateRebateRate, 'f', 8, 64)
	if settings.AffiliateRebateFreezeHours < 0 {
		settings.AffiliateRebateFreezeHours = AffiliateRebateFreezeHoursDefault
	}
	if settings.AffiliateRebateFreezeHours > AffiliateRebateFreezeHoursMax {
		settings.AffiliateRebateFreezeHours = AffiliateRebateFreezeHoursMax
	}
	updates[SettingKeyAffiliateRebateFreezeHours] = strconv.Itoa(settings.AffiliateRebateFreezeHours)
	if settings.AffiliateRebateDurationDays < 0 {
		settings.AffiliateRebateDurationDays = AffiliateRebateDurationDaysDefault
	}
	if settings.AffiliateRebateDurationDays > AffiliateRebateDurationDaysMax {
		settings.AffiliateRebateDurationDays = AffiliateRebateDurationDaysMax
	}
	updates[SettingKeyAffiliateRebateDurationDays] = strconv.Itoa(settings.AffiliateRebateDurationDays)
	if settings.AffiliateRebatePerInviteeCap < 0 {
		settings.AffiliateRebatePerInviteeCap = AffiliateRebatePerInviteeCapDefault
	}
	updates[SettingKeyAffiliateRebatePerInviteeCap] = strconv.FormatFloat(settings.AffiliateRebatePerInviteeCap, 'f', 8, 64)
	updates[SettingKeyAffiliateAdminRechargeEnabled] = strconv.FormatBool(settings.AdminRechargeRebateEnabled)
	updates[SettingKeyDefaultUserRPMLimit] = strconv.Itoa(settings.DefaultUserRPMLimit)
	defaultSubsJSON, err := json.Marshal(settings.DefaultSubscriptions)
	if err != nil {
		return nil, fmt.Errorf("marshal default subscriptions: %w", err)
	}
	updates[SettingKeyDefaultSubscriptions] = string(defaultSubsJSON)

	// Model fallback configuration
	updates[SettingKeyEnableModelFallback] = strconv.FormatBool(settings.EnableModelFallback)
	updates[SettingKeyFallbackModelAnthropic] = settings.FallbackModelAnthropic
	updates[SettingKeyFallbackModelOpenAI] = settings.FallbackModelOpenAI
	updates[SettingKeyFallbackModelGemini] = settings.FallbackModelGemini
	updates[SettingKeyFallbackModelAntigravity] = settings.FallbackModelAntigravity

	// Identity patch configuration (Claude -> Gemini)
	updates[SettingKeyEnableIdentityPatch] = strconv.FormatBool(settings.EnableIdentityPatch)
	updates[SettingKeyIdentityPatchPrompt] = settings.IdentityPatchPrompt

	// Ops monitoring (vNext)
	updates[SettingKeyOpsMonitoringEnabled] = strconv.FormatBool(settings.OpsMonitoringEnabled)
	updates[SettingKeyOpsRealtimeMonitoringEnabled] = strconv.FormatBool(settings.OpsRealtimeMonitoringEnabled)
	updates[SettingKeyOpsQueryModeDefault] = string(ParseOpsQueryMode(settings.OpsQueryModeDefault))
	if settings.OpsMetricsIntervalSeconds > 0 {
		updates[SettingKeyOpsMetricsIntervalSeconds] = strconv.Itoa(settings.OpsMetricsIntervalSeconds)
	}

	// Channel monitor feature switch
	updates[SettingKeyChannelMonitorEnabled] = strconv.FormatBool(settings.ChannelMonitorEnabled)
	if v := clampChannelMonitorInterval(settings.ChannelMonitorDefaultIntervalSeconds); v > 0 {
		updates[SettingKeyChannelMonitorDefaultIntervalSeconds] = strconv.Itoa(v)
	}

	// Available channels feature switch
	updates[SettingKeyAvailableChannelsEnabled] = strconv.FormatBool(settings.AvailableChannelsEnabled)

	// Affiliate (邀请返利) feature switch
	updates[SettingKeyAffiliateEnabled] = strconv.FormatBool(settings.AffiliateEnabled)

	// 风控中心功能开关
	updates[SettingKeyRiskControlEnabled] = strconv.FormatBool(settings.RiskControlEnabled)

	// cyber 会话屏蔽开关 + TTL
	updates[SettingKeyCyberSessionBlockEnabled] = strconv.FormatBool(settings.CyberSessionBlockEnabled)
	if settings.CyberSessionBlockTTLSeconds > 0 {
		updates[SettingKeyCyberSessionBlockTTLSeconds] = strconv.Itoa(settings.CyberSessionBlockTTLSeconds)
	}

	// Claude Code version check
	updates[SettingKeyMinClaudeCodeVersion] = settings.MinClaudeCodeVersion
	updates[SettingKeyMaxClaudeCodeVersion] = settings.MaxClaudeCodeVersion

	// 分组隔离
	updates[SettingKeyAllowUngroupedKeyScheduling] = strconv.FormatBool(settings.AllowUngroupedKeyScheduling)

	// Backend Mode
	updates[SettingKeyBackendModeEnabled] = strconv.FormatBool(settings.BackendModeEnabled)

	// Gateway forwarding behavior
	updates[SettingKeyEnableFingerprintUnification] = strconv.FormatBool(settings.EnableFingerprintUnification)
	updates[SettingKeyEnableMetadataPassthrough] = strconv.FormatBool(settings.EnableMetadataPassthrough)
	updates[SettingKeyEnableCCHSigning] = strconv.FormatBool(settings.EnableCCHSigning)
	updates[SettingKeyEnableClaudeOAuthSystemPromptInjection] = strconv.FormatBool(settings.EnableClaudeOAuthSystemPromptInjection)
	updates[SettingKeyClaudeOAuthSystemPrompt] = settings.ClaudeOAuthSystemPrompt
	if err := ValidateClaudeOAuthSystemPromptBlocksConfig(settings.ClaudeOAuthSystemPromptBlocks); err != nil {
		return nil, err
	}
	updates[SettingKeyClaudeOAuthSystemPromptBlocks] = settings.ClaudeOAuthSystemPromptBlocks
	updates[SettingKeyEnableAnthropicCacheTTL1hInjection] = strconv.FormatBool(settings.EnableAnthropicCacheTTL1hInjection)
	updates[SettingKeyRewriteMessageCacheControl] = strconv.FormatBool(settings.RewriteMessageCacheControl)
	updates[SettingKeyEnableClientDatelineNormalization] = strconv.FormatBool(settings.EnableClientDatelineNormalization)
	updates[SettingKeyAntigravityUserAgentVersion] = antigravity.NormalizeUserAgentVersion(settings.AntigravityUserAgentVersion)
	updates[SettingKeyOpenAICodexUserAgent] = strings.TrimSpace(settings.OpenAICodexUserAgent)
	// codex_cli_only 加固
	updates[SettingKeyMinCodexVersion] = strings.TrimSpace(settings.MinCodexVersion)
	updates[SettingKeyMaxCodexVersion] = strings.TrimSpace(settings.MaxCodexVersion)
	updates[SettingKeyCodexCLIOnlyBlacklist] = strings.TrimSpace(settings.CodexCLIOnlyBlacklist)
	updates[SettingKeyCodexCLIOnlyWhitelist] = strings.TrimSpace(settings.CodexCLIOnlyWhitelist)
	updates[SettingKeyCodexCLIOnlyAllowAppServerClients] = strconv.FormatBool(settings.CodexCLIOnlyAllowAppServerClients)
	updates[SettingKeyCodexCLIOnlyEngineFingerprintSignals] = strings.TrimSpace(settings.CodexCLIOnlyEngineFingerprintSignals)
	updates[SettingPaymentVisibleMethodAlipaySource] = settings.PaymentVisibleMethodAlipaySource
	updates[SettingPaymentVisibleMethodWxpaySource] = settings.PaymentVisibleMethodWxpaySource
	updates[SettingPaymentVisibleMethodAlipayEnabled] = strconv.FormatBool(settings.PaymentVisibleMethodAlipayEnabled)
	updates[SettingPaymentVisibleMethodWxpayEnabled] = strconv.FormatBool(settings.PaymentVisibleMethodWxpayEnabled)
	updates[openAIAdvancedSchedulerSettingKey] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerStickyWeightedEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerLBTopK] = settings.OpenAIAdvancedSchedulerLBTopK
	updates[SettingKeyOpenAIAdvancedSchedulerWeightPriority] = settings.OpenAIAdvancedSchedulerWeightPriority
	updates[SettingKeyOpenAIAdvancedSchedulerWeightLoad] = settings.OpenAIAdvancedSchedulerWeightLoad
	updates[SettingKeyOpenAIAdvancedSchedulerWeightQueue] = settings.OpenAIAdvancedSchedulerWeightQueue
	updates[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate] = settings.OpenAIAdvancedSchedulerWeightErrorRate
	updates[SettingKeyOpenAIAdvancedSchedulerWeightTTFT] = settings.OpenAIAdvancedSchedulerWeightTTFT
	updates[SettingKeyOpenAIAdvancedSchedulerWeightReset] = settings.OpenAIAdvancedSchedulerWeightReset
	updates[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom] = settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom
	updates[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse] = settings.OpenAIAdvancedSchedulerWeightPreviousResponse
	updates[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky] = settings.OpenAIAdvancedSchedulerWeightSessionSticky

	// 余额、订阅到期与账号限额通知
	updates[SettingKeyBalanceLowNotifyEnabled] = strconv.FormatBool(settings.BalanceLowNotifyEnabled)
	updates[SettingKeyBalanceLowNotifyThreshold] = strconv.FormatFloat(settings.BalanceLowNotifyThreshold, 'f', 8, 64)
	updates[SettingKeyBalanceLowNotifyRechargeURL] = settings.BalanceLowNotifyRechargeURL
	updates[SettingKeySubscriptionExpiryNotifyEnabled] = strconv.FormatBool(settings.SubscriptionExpiryNotifyEnabled)
	updates[SettingKeyAccountQuotaNotifyEnabled] = strconv.FormatBool(settings.AccountQuotaNotifyEnabled)
	updates[SettingKeyAccountQuotaNotifyEmails] = MarshalNotifyEmails(settings.AccountQuotaNotifyEmails)

	// 系统全局 platform quota：整体替换语义（null/缺省 = 不限制）。
	if settings.DefaultPlatformQuotas != nil {
		if err := validateDefaultPlatformQuotaMap(settings.DefaultPlatformQuotas); err != nil {
			return nil, err
		}
		blob, err := json.Marshal(settings.DefaultPlatformQuotas)
		if err != nil {
			return nil, fmt.Errorf("marshal default platform quotas: %w", err)
		}
		updates[SettingKeyDefaultPlatformQuotas] = string(blob)
	}

	updates[SettingKeyAllowUserViewErrorRequests] = strconv.FormatBool(settings.AllowUserViewErrorRequests)

	return updates, nil
}

// validateDefaultPlatformQuotaMap 校验 platform quota map 的合法性：
// 平台名须在 AllowedQuotaPlatforms 白名单内，每个非 nil 上限须 finite 且 >= 0。
// 系统层和 auth-source 层共用此 helper。
func validateDefaultPlatformQuotaMap(m map[string]*DefaultPlatformQuotaSetting) error {
	for platform, pq := range m {
		if !IsAllowedQuotaPlatform(platform) {
			return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", fmt.Sprintf("unknown platform %q", platform))
		}
		if pq == nil {
			continue
		}
		for _, v := range []*float64{pq.DailyLimitUSD, pq.WeeklyLimitUSD, pq.MonthlyLimitUSD} {
			if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
				return infraerrors.BadRequest("INVALID_DEFAULT_PLATFORM_QUOTA", "platform quota limit must be a finite non-negative number")
			}
		}
	}
	return nil
}

func (s *SettingService) buildAuthSourceDefaultUpdates(ctx context.Context, settings *AuthSourceDefaultSettings) (map[string]string, error) {
	if settings == nil {
		return nil, nil
	}

	for _, subscriptions := range [][]DefaultSubscriptionSetting{
		settings.Email.Subscriptions,
		settings.LinuxDo.Subscriptions,
		settings.OIDC.Subscriptions,
		settings.WeChat.Subscriptions,
		settings.GitHub.Subscriptions,
		settings.Google.Subscriptions,
		settings.DingTalk.Subscriptions,
	} {
		if err := s.validateDefaultSubscriptionGroups(ctx, subscriptions); err != nil {
			return nil, err
		}
	}

	// 校验各 auth source 的 platform quota map（改动 C：对等系统层校验）
	for _, pgs := range []struct {
		name string
		pq   map[string]*DefaultPlatformQuotaSetting
	}{
		{"email", settings.Email.PlatformQuotas},
		{"linuxdo", settings.LinuxDo.PlatformQuotas},
		{"oidc", settings.OIDC.PlatformQuotas},
		{"wechat", settings.WeChat.PlatformQuotas},
		{"github", settings.GitHub.PlatformQuotas},
		{"google", settings.Google.PlatformQuotas},
		{"dingtalk", settings.DingTalk.PlatformQuotas},
	} {
		if pgs.pq != nil {
			if err := validateDefaultPlatformQuotaMap(pgs.pq); err != nil {
				return nil, err
			}
		}
	}

	updates := make(map[string]string, 36)
	writeProviderDefaultGrantUpdates(updates, emailAuthSourceDefaultKeys, settings.Email)
	writeProviderDefaultGrantUpdates(updates, linuxDoAuthSourceDefaultKeys, settings.LinuxDo)
	writeProviderDefaultGrantUpdates(updates, oidcAuthSourceDefaultKeys, settings.OIDC)
	writeProviderDefaultGrantUpdates(updates, weChatAuthSourceDefaultKeys, settings.WeChat)
	writeProviderDefaultGrantUpdates(updates, gitHubAuthSourceDefaultKeys, settings.GitHub)
	writeProviderDefaultGrantUpdates(updates, googleAuthSourceDefaultKeys, settings.Google)
	writeProviderDefaultGrantUpdates(updates, dingTalkAuthSourceDefaultKeys, settings.DingTalk)
	updates[SettingKeyForceEmailOnThirdPartySignup] = strconv.FormatBool(settings.ForceEmailOnThirdPartySignup)
	return updates, nil
}

func (s *SettingService) refreshCachedSettings(settings *SystemSettings) {
	if settings == nil {
		return
	}

	// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口
	versionBoundsSF.Forget("version_bounds")
	versionBoundsCache.Store(&cachedVersionBounds{
		min:       settings.MinClaudeCodeVersion,
		max:       settings.MaxClaudeCodeVersion,
		expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
	})
	backendModeSF.Forget("backend_mode")
	backendModeCache.Store(&cachedBackendMode{
		value:     settings.BackendModeEnabled,
		expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
	})
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification:           settings.EnableFingerprintUnification,
		metadataPassthrough:              settings.EnableMetadataPassthrough,
		cchSigning:                       settings.EnableCCHSigning,
		claudeOAuthSystemPromptInjection: settings.EnableClaudeOAuthSystemPromptInjection,
		claudeOAuthSystemPrompt:          settings.ClaudeOAuthSystemPrompt,
		claudeOAuthSystemPromptBlocks:    settings.ClaudeOAuthSystemPromptBlocks,
		anthropicCacheTTL1hInjection:     settings.EnableAnthropicCacheTTL1hInjection,
		rewriteMessageCacheControl:       settings.RewriteMessageCacheControl,
		clientDatelineNormalization:      settings.EnableClientDatelineNormalization,
		expiresAt:                        time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
	})
	s.antigravityUAVersionSF.Forget("antigravity_user_agent_version")
	antigravityUserAgentVersion := antigravity.NormalizeUserAgentVersion(settings.AntigravityUserAgentVersion)
	if antigravityUserAgentVersion == "" {
		antigravityUserAgentVersion = antigravity.GetDefaultUserAgentVersion()
	}
	s.antigravityUAVersionCache.Store(&cachedAntigravityUserAgentVersion{
		version:   antigravityUserAgentVersion,
		expiresAt: time.Now().Add(antigravityUserAgentVersionCacheTTL).UnixNano(),
	})
	s.openAICodexUASF.Forget("openai_codex_user_agent")
	codexUA := strings.TrimSpace(settings.OpenAICodexUserAgent)
	if codexUA == "" {
		codexUA = DefaultOpenAICodexUserAgent
	}
	s.openAICodexUACache.Store(&cachedOpenAICodexUserAgent{
		value:     codexUA,
		expiresAt: time.Now().Add(openAICodexUserAgentCacheTTL).UnixNano(),
	})
	openAIAdvancedSchedulerSettingSF.Forget(openAIAdvancedSchedulerSettingKey)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		enabled:                     settings.OpenAIAdvancedSchedulerEnabled,
		stickyWeightedEnabled:       settings.OpenAIAdvancedSchedulerStickyWeightedEnabled,
		subscriptionPriorityEnabled: settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled,
		lbTopKOverride:              parsePositiveIntOverride(settings.OpenAIAdvancedSchedulerLBTopK),
		weightOverrides: parseOpenAIAdvancedSchedulerWeightOverrides(map[string]string{
			SettingKeyOpenAIAdvancedSchedulerWeightPriority:         settings.OpenAIAdvancedSchedulerWeightPriority,
			SettingKeyOpenAIAdvancedSchedulerWeightLoad:             settings.OpenAIAdvancedSchedulerWeightLoad,
			SettingKeyOpenAIAdvancedSchedulerWeightQueue:            settings.OpenAIAdvancedSchedulerWeightQueue,
			SettingKeyOpenAIAdvancedSchedulerWeightErrorRate:        settings.OpenAIAdvancedSchedulerWeightErrorRate,
			SettingKeyOpenAIAdvancedSchedulerWeightTTFT:             settings.OpenAIAdvancedSchedulerWeightTTFT,
			SettingKeyOpenAIAdvancedSchedulerWeightReset:            settings.OpenAIAdvancedSchedulerWeightReset,
			SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom:    settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
			SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse: settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
			SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky:    settings.OpenAIAdvancedSchedulerWeightSessionSticky,
		}),
		expiresAt: time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
	})
	// Invalidate the quota auto-pause cache and let the next read trigger a fresh load.
	// We can't know from here whether ops_advanced_settings was also touched, so be
	// defensive: store an expired entry — GetOpenAIQuotaAutoPauseSettings will serve
	// stale and kick off an async refresh, never blocking the request that follows.
	s.openAIQuotaAutoPauseSettingsSF.Forget(openAIQuotaAutoPauseSettingsRefreshKey)
	if cached, _ := s.openAIQuotaAutoPauseSettingsCache.Load().(*cachedOpenAIQuotaAutoPauseSettings); cached != nil {
		s.openAIQuotaAutoPauseSettingsCache.Store(&cachedOpenAIQuotaAutoPauseSettings{
			settings:  cached.settings,
			expiresAt: 0,
		})
	}
	if s.cfg != nil {
		s.cfg.SetTrustForwardedIPForAPIKeyACL(settings.APIKeyACLTrustForwardedIP)
	}
	// codex_cli_only 加固策略缓存：设置更新后强制下次重载（涉及 4 个键 + JSON 解析，直接置过期）。
	s.codexRestrictionPolicySF.Forget("codex_restriction_policy")
	s.codexRestrictionPolicyCache.Store(&cachedCodexRestrictionPolicy{expiresAt: 0})
	if s.onUpdate != nil {
		s.onUpdate() // Invalidate cache after settings update
	}
}

func (s *SettingService) defaultRewriteMessageCacheControl() bool {
	return false
}

func (s *SettingService) validateDefaultSubscriptionGroups(ctx context.Context, items []DefaultSubscriptionSetting) error {
	if len(items) == 0 {
		return nil
	}

	checked := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if item.GroupID <= 0 {
			continue
		}
		if _, ok := checked[item.GroupID]; ok {
			return ErrDefaultSubGroupDuplicate.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
		checked[item.GroupID] = struct{}{}
		if s.defaultSubGroupReader == nil {
			continue
		}

		group, err := s.defaultSubGroupReader.GetByID(ctx, item.GroupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(item.GroupID, 10),
				})
			}
			return fmt.Errorf("get default subscription group %d: %w", item.GroupID, err)
		}
		if !group.IsSubscriptionType() {
			return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
			})
		}
	}

	return nil
}
