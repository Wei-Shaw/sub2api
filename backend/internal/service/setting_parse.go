package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// InitializeDefaultSettings 初始化默认设置
func (s *SettingService) InitializeDefaultSettings(ctx context.Context) error {
	// 检查是否已有设置
	_, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err == nil {
		// 已有设置，不需要初始化
		return nil
REDACTED
	if !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("check existing settings: %w", err)
REDACTED

	oidcUsePKCEDefault := true
	oidcValidateIDTokenDefault := true
	if s != nil && s.cfg != nil {
		if s.cfg.OIDC.UsePKCEExplicit {
			oidcUsePKCEDefault = s.cfg.OIDC.UsePKCE
	REDACTED
		if s.cfg.OIDC.ValidateIDTokenExplicit {
			oidcValidateIDTokenDefault = s.cfg.OIDC.ValidateIDToken
	REDACTED
REDACTED
	loginAgreementDocumentsJSON, err := marshalLoginAgreementDocuments(defaultLoginAgreementDocuments())
	if err != nil {
		return err
REDACTED

	// 初始化默认设置
	defaults := map[string]string{
		SettingKeyRegistrationEnabled:                       "true",
		SettingKeyEmailVerifyEnabled:                        "false",
		SettingKeyRegistrationEmailSuffixWhitelist:          "[]",
		SettingKeyPromoCodeEnabled:                          "true", // 默认启用优惠码功能
		SettingKeyLoginAgreementEnabled:                     "false",
		SettingKeyLoginAgreementMode:                        defaultLoginAgreementMode,
		SettingKeyLoginAgreementUpdatedAt:                   defaultLoginAgreementDate,
		SettingKeyLoginAgreementDocuments:                   loginAgreementDocumentsJSON,
		SettingKeyAPIKeyACLTrustForwardedIP:                 "false",
		SettingKeySiteName:                                  "Sub2API",
		SettingKeySiteLogo:                                  "",
		SettingKeyPurchaseSubscriptionEnabled:               "false",
		SettingKeyPurchaseSubscriptionURL:                   "",
		SettingKeyTableDefaultPageSize:                      "20",
		SettingKeyTablePageSizeOptions:                      "[10,20,50,100]",
		SettingKeyCustomMenuItems:                           "[]",
		SettingKeyCustomEndpoints:                           "[]",
		SettingKeyWeChatConnectEnabled:                      "false",
		SettingKeyWeChatConnectAppID:                        "",
		SettingKeyWeChatConnectAppSecret:                    "",
		SettingKeyWeChatConnectOpenAppID:                    "",
		SettingKeyWeChatConnectOpenAppSecret:                "",
		SettingKeyWeChatConnectMPAppID:                      "",
		SettingKeyWeChatConnectMPAppSecret:                  "",
		SettingKeyWeChatConnectMobileAppID:                  "",
		SettingKeyWeChatConnectMobileAppSecret:              "",
		SettingKeyWeChatConnectOpenEnabled:                  "false",
		SettingKeyWeChatConnectMPEnabled:                    "false",
		SettingKeyWeChatConnectMobileEnabled:                "false",
		SettingKeyWeChatConnectMode:                         "open",
		SettingKeyWeChatConnectScopes:                       "snsapi_login",
		SettingKeyWeChatConnectRedirectURL:                  "",
		SettingKeyWeChatConnectFrontendRedirectURL:          defaultWeChatConnectFrontend,
		SettingKeyGitHubOAuthEnabled:                        "false",
		SettingKeyGitHubOAuthClientID:                       "",
		SettingKeyGitHubOAuthClientSecret:                   "",
		SettingKeyGitHubOAuthRedirectURL:                    "",
		SettingKeyGitHubOAuthFrontendRedirectURL:            defaultGitHubOAuthFrontend,
		SettingKeyGoogleOAuthEnabled:                        "false",
		SettingKeyGoogleOAuthClientID:                       "",
		SettingKeyGoogleOAuthClientSecret:                   "",
		SettingKeyGoogleOAuthRedirectURL:                    "",
		SettingKeyGoogleOAuthFrontendRedirectURL:            defaultGoogleOAuthFrontend,
		SettingKeyOIDCConnectEnabled:                        "false",
		SettingKeyOIDCConnectProviderName:                   "OIDC",
		SettingKeyOIDCConnectClientID:                       "",
		SettingKeyOIDCConnectClientSecret:                   "",
		SettingKeyOIDCConnectIssuerURL:                      "",
		SettingKeyOIDCConnectDiscoveryURL:                   "",
		SettingKeyOIDCConnectAuthorizeURL:                   "",
		SettingKeyOIDCConnectTokenURL:                       "",
		SettingKeyOIDCConnectUserInfoURL:                    "",
		SettingKeyOIDCConnectJWKSURL:                        "",
		SettingKeyOIDCConnectScopes:                         "openid email profile",
		SettingKeyOIDCConnectRedirectURL:                    "",
		SettingKeyOIDCConnectFrontendRedirectURL:            "/auth/oidc/callback",
		SettingKeyOIDCConnectTokenAuthMethod:                "client_secret_post",
		SettingKeyOIDCConnectUsePKCE:                        strconv.FormatBool(oidcUsePKCEDefault),
		SettingKeyOIDCConnectValidateIDToken:                strconv.FormatBool(oidcValidateIDTokenDefault),
		SettingKeyOIDCConnectAllowedSigningAlgs:             "RS256,ES256,PS256",
		SettingKeyOIDCConnectClockSkewSeconds:               "120",
		SettingKeyOIDCConnectRequireEmailVerified:           "false",
		SettingKeyOIDCConnectUserInfoEmailPath:              "",
		SettingKeyOIDCConnectUserInfoIDPath:                 "",
		SettingKeyOIDCConnectUserInfoUsernamePath:           "",
		SettingKeyDefaultConcurrency:                        strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:                            strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeyAffiliateRebateRate:                       strconv.FormatFloat(AffiliateRebateRateDefault, 'f', 8, 64),
		SettingKeyAffiliateRebateFreezeHours:                strconv.Itoa(AffiliateRebateFreezeHoursDefault),
		SettingKeyAffiliateRebateDurationDays:               strconv.Itoa(AffiliateRebateDurationDaysDefault),
		SettingKeyAffiliateRebatePerInviteeCap:              strconv.FormatFloat(AffiliateRebatePerInviteeCapDefault, 'f', 2, 64),
		SettingKeyDefaultUserRPMLimit:                       "0",
		SettingKeyDefaultSubscriptions:                      "[]",
		SettingKeyAuthSourceDefaultEmailBalance:             "0",
		SettingKeyAuthSourceDefaultEmailConcurrency:         "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions:       "[]",
		SettingKeyAuthSourceDefaultEmailGrantOnSignup:       "false",
		SettingKeyAuthSourceDefaultEmailGrantOnFirstBind:    "false",
		SettingKeyAuthSourceDefaultLinuxDoBalance:           "0",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:       "5",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions:     "[]",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup:     "false",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind:  "false",
		SettingKeyAuthSourceDefaultOIDCBalance:              "0",
		SettingKeyAuthSourceDefaultOIDCConcurrency:          "5",
		SettingKeyAuthSourceDefaultOIDCSubscriptions:        "[]",
		SettingKeyAuthSourceDefaultOIDCGrantOnSignup:        "false",
		SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind:     "false",
		SettingKeyAuthSourceDefaultWeChatBalance:            "0",
		SettingKeyAuthSourceDefaultWeChatConcurrency:        "5",
		SettingKeyAuthSourceDefaultWeChatSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultWeChatGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultGitHubBalance:            "0",
		SettingKeyAuthSourceDefaultGitHubConcurrency:        "5",
		SettingKeyAuthSourceDefaultGitHubSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultGitHubGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultGoogleBalance:            "0",
		SettingKeyAuthSourceDefaultGoogleConcurrency:        "5",
		SettingKeyAuthSourceDefaultGoogleSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultGoogleGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultDingTalkBalance:          "0",
		SettingKeyAuthSourceDefaultDingTalkConcurrency:      "5",
		SettingKeyAuthSourceDefaultDingTalkSubscriptions:    "[]",
		SettingKeyAuthSourceDefaultDingTalkGrantOnSignup:    "false",
		SettingKeyAuthSourceDefaultDingTalkGrantOnFirstBind: "false",
		SettingKeyForceEmailOnThirdPartySignup:              "false",
		SettingKeySMTPPort:                                  "587",
		SettingKeySMTPUseTLS:                                "false",
		// Model fallback defaults
		SettingKeyEnableModelFallback:      "false",
		SettingKeyFallbackModelAnthropic:   "claude-3-5-sonnet-20241022",
		SettingKeyFallbackModelOpenAI:      "gpt-4o",
		SettingKeyFallbackModelGemini:      "gemini-2.5-pro",
		SettingKeyFallbackModelAntigravity: "gemini-2.5-pro",
		// Identity patch defaults
		SettingKeyEnableIdentityPatch: "true",
		SettingKeyIdentityPatchPrompt: "",

		// Ops monitoring defaults (vNext)
		SettingKeyOpsMonitoringEnabled:         "true",
		SettingKeyOpsRealtimeMonitoringEnabled: "true",
		SettingKeyOpsQueryModeDefault:          "auto",
		SettingKeyOpsMetricsIntervalSeconds:    "60",

		// Channel monitor defaults (enabled, 60s)
		SettingKeyChannelMonitorEnabled:                "true",
		SettingKeyChannelMonitorDefaultIntervalSeconds: "60",

		// Available channels feature (default disabled; opt-in)
		SettingKeyAvailableChannelsEnabled: "false",

		// Affiliate (邀请返利) feature (default disabled; opt-in)
		SettingKeyAffiliateEnabled: "false",

		// 风控中心功能（默认关闭，显式启用）
		SettingKeyRiskControlEnabled: "false",

		// cyber 会话屏蔽（默认关闭，TTL 默认 3600s）
		SettingKeyCyberSessionBlockEnabled:    "false",
		SettingKeyCyberSessionBlockTTLSeconds: "3600",

		// Claude Code version check (default: empty = disabled)
		SettingKeyMinClaudeCodeVersion: "",
		SettingKeyMaxClaudeCodeVersion: "",

		// codex_cli_only 加固（默认：版本不检查、名单空、默认种子指纹信号）
		SettingKeyMinCodexVersion:                      "",
		SettingKeyMaxCodexVersion:                      "",
		SettingKeyCodexCLIOnlyBlacklist:                "",
		SettingKeyCodexCLIOnlyWhitelist:                "",
		SettingKeyCodexCLIOnlyAllowAppServerClients:    "false",
		SettingKeyCodexCLIOnlyEngineFingerprintSignals: openai.DefaultEngineFingerprintSignalsJSON(),

		// 分组隔离（默认不允许未分组 Key 调度）
		SettingKeyAllowUngroupedKeyScheduling:                        "false",
		SettingKeyEnableAnthropicCacheTTL1hInjection:                 "false",
		SettingKeyRewriteMessageCacheControl:                         strconv.FormatBool(s.defaultRewriteMessageCacheControl()),
		SettingKeyEnableClientDatelineNormalization:                  "true",
		SettingKeyAntigravityUserAgentVersion:                        "",
		SettingKeyOpenAICodexUserAgent:                               "",
		SettingPaymentVisibleMethodAlipaySource:                      "",
		SettingPaymentVisibleMethodWxpaySource:                       "",
		SettingPaymentVisibleMethodAlipayEnabled:                     "false",
		SettingPaymentVisibleMethodWxpayEnabled:                      "false",
		openAIAdvancedSchedulerSettingKey:                            "false",
		SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled:       "false",
		SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled: "false",
		SettingKeyOpenAIAdvancedSchedulerLBTopK:                      "",
		SettingKeyOpenAIAdvancedSchedulerWeightPriority:              "",
		SettingKeyOpenAIAdvancedSchedulerWeightLoad:                  "",
		SettingKeyOpenAIAdvancedSchedulerWeightQueue:                 "",
		SettingKeyOpenAIAdvancedSchedulerWeightErrorRate:             "",
		SettingKeyOpenAIAdvancedSchedulerWeightTTFT:                  "",
		SettingKeyOpenAIAdvancedSchedulerWeightReset:                 "",
		SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom:         "",
		SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse:      "",
		SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky:         "",

		SettingKeyAllowUserViewErrorRequests: "false",
REDACTED

	return s.settingRepo.SetMultiple(ctx, defaults)
REDACTED

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	loginAgreementDocuments := parseLoginAgreementDocuments(settings[SettingKeyLoginAgreementDocuments])
	loginAgreementUpdatedAt := strings.TrimSpace(settings[SettingKeyLoginAgreementUpdatedAt])
	if loginAgreementUpdatedAt == "" {
		loginAgreementUpdatedAt = defaultLoginAgreementDate
REDACTED
	apiKeyACLTrustForwardedIP := false
	if value, ok := settings[SettingKeyAPIKeyACLTrustForwardedIP]; ok {
		apiKeyACLTrustForwardedIP = value == "true"
REDACTED else if s != nil && s.cfg != nil {
		apiKeyACLTrustForwardedIP = s.cfg.Security.TrustForwardedIPForAPIKeyACL
REDACTED
	result := &SystemSettings{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: ParseRegistrationEmailSuffixWhitelist(settings[SettingKeyRegistrationEmailSuffixWhitelist]),
		PromoCodeEnabled:                 settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:             emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		FrontendURL:                      settings[SettingKeyFrontendURL],
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		LoginAgreementEnabled:            settings[SettingKeyLoginAgreementEnabled] == "true",
		LoginAgreementMode:               normalizeLoginAgreementMode(settings[SettingKeyLoginAgreementMode]),
		LoginAgreementUpdatedAt:          loginAgreementUpdatedAt,
		LoginAgreementDocuments:          loginAgreementDocuments,
		SMTPHost:                         settings[SettingKeySMTPHost],
		SMTPUsername:                     settings[SettingKeySMTPUsername],
		SMTPFrom:                         settings[SettingKeySMTPFrom],
		SMTPFromName:                     settings[SettingKeySMTPFromName],
		SMTPUseTLS:                       settings[SettingKeySMTPUseTLS] == "true",
		SMTPPasswordConfigured:           settings[SettingKeySMTPPassword] != "",
		TurnstileEnabled:                 settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                 settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured:     settings[SettingKeyTurnstileSecretKey] != "",
		APIKeyACLTrustForwardedIP:        apiKeyACLTrustForwardedIP,
		SiteName:                         s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:                         settings[SettingKeySiteLogo],
		SiteSubtitle:                     s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                       settings[SettingKeyAPIBaseURL],
		ContactInfo:                      settings[SettingKeyContactInfo],
		DocURL:                           settings[SettingKeyDocURL],
		HomeContent:                      settings[SettingKeyHomeContent],
		HideCcsImportButton:              settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:      settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:          strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		CustomMenuItems:                  settings[SettingKeyCustomMenuItems],
		CustomEndpoints:                  settings[SettingKeyCustomEndpoints],
		BackendModeEnabled:               settings[SettingKeyBackendModeEnabled] == "true",
REDACTED
	result.TableDefaultPageSize, result.TablePageSizeOptions = parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)

	// 解析整数类型
	if port, err := strconv.Atoi(settings[SettingKeySMTPPort]); err == nil {
		result.SMTPPort = port
REDACTED else {
		result.SMTPPort = 587
REDACTED

	if concurrency, err := strconv.Atoi(settings[SettingKeyDefaultConcurrency]); err == nil {
		result.DefaultConcurrency = concurrency
REDACTED else {
		result.DefaultConcurrency = s.cfg.Default.UserConcurrency
REDACTED

	if rpm, err := strconv.Atoi(settings[SettingKeyDefaultUserRPMLimit]); err == nil && rpm >= 0 {
		result.DefaultUserRPMLimit = rpm
REDACTED

	// 解析浮点数类型
	if balance, err := strconv.ParseFloat(settings[SettingKeyDefaultBalance], 64); err == nil {
		result.DefaultBalance = balance
REDACTED else {
		result.DefaultBalance = s.cfg.Default.UserBalance
REDACTED
	if rebateRate, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebateRate], 64); err == nil {
		result.AffiliateRebateRate = clampAffiliateRebateRate(rebateRate)
REDACTED else {
		result.AffiliateRebateRate = AffiliateRebateRateDefault
REDACTED
	if freezeHours, err := strconv.Atoi(settings[SettingKeyAffiliateRebateFreezeHours]); err == nil && freezeHours >= 0 {
		if freezeHours > AffiliateRebateFreezeHoursMax {
			freezeHours = AffiliateRebateFreezeHoursMax
	REDACTED
		result.AffiliateRebateFreezeHours = freezeHours
REDACTED
	if durationDays, err := strconv.Atoi(settings[SettingKeyAffiliateRebateDurationDays]); err == nil && durationDays >= 0 {
		if durationDays > AffiliateRebateDurationDaysMax {
			durationDays = AffiliateRebateDurationDaysMax
	REDACTED
		result.AffiliateRebateDurationDays = durationDays
REDACTED
	if perInviteeCap, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebatePerInviteeCap], 64); err == nil && perInviteeCap >= 0 {
		result.AffiliateRebatePerInviteeCap = perInviteeCap
REDACTED
	result.DefaultSubscriptions = parseDefaultSubscriptions(settings[SettingKeyDefaultSubscriptions])

	// 敏感信息直接返回，方便测试连接时使用
	result.SMTPPassword = settings[SettingKeySMTPPassword]
	result.TurnstileSecretKey = settings[SettingKeyTurnstileSecretKey]

	// LinuxDo Connect 设置：
	// - 兼容 config.yaml/env（避免老部署因为未迁移到数据库设置而被意外关闭）
	// - 支持在后台“系统设置”中覆盖并持久化（存储于 DB）
	linuxDoBase := config.LinuxDoConnectConfig{REDACTED
	if s.cfg != nil {
		linuxDoBase = s.cfg.LinuxDo
REDACTED

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		result.LinuxDoConnectEnabled = raw == "true"
REDACTED else {
		result.LinuxDoConnectEnabled = linuxDoBase.Enabled
REDACTED

	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectClientID = strings.TrimSpace(v)
REDACTED else {
		result.LinuxDoConnectClientID = linuxDoBase.ClientID
REDACTED

	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.LinuxDoConnectRedirectURL = strings.TrimSpace(v)
REDACTED else {
		result.LinuxDoConnectRedirectURL = linuxDoBase.RedirectURL
REDACTED

	result.LinuxDoConnectClientSecret = strings.TrimSpace(settings[SettingKeyLinuxDoConnectClientSecret])
	if result.LinuxDoConnectClientSecret == "" {
		result.LinuxDoConnectClientSecret = strings.TrimSpace(linuxDoBase.ClientSecret)
REDACTED
	result.LinuxDoConnectClientSecretConfigured = result.LinuxDoConnectClientSecret != ""

	// DingTalk Connect 设置：
	// - 兼容 config.yaml/env
	// - 支持后台系统设置覆盖并持久化（存储于 DB）
	dingTalkBase := config.DingTalkConnectConfig{REDACTED
	if s.cfg != nil {
		dingTalkBase = s.cfg.DingTalk
REDACTED

	if raw, ok := settings[SettingKeyDingTalkConnectEnabled]; ok {
		result.DingTalkConnectEnabled = raw == "true"
REDACTED else {
		result.DingTalkConnectEnabled = dingTalkBase.Enabled
REDACTED

	if v, ok := settings[SettingKeyDingTalkConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectClientID = strings.TrimSpace(v)
REDACTED else {
		result.DingTalkConnectClientID = dingTalkBase.ClientID
REDACTED

	if v, ok := settings[SettingKeyDingTalkConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectRedirectURL = strings.TrimSpace(v)
REDACTED else {
		result.DingTalkConnectRedirectURL = dingTalkBase.RedirectURL
REDACTED

	result.DingTalkConnectClientSecret = strings.TrimSpace(settings[SettingKeyDingTalkConnectClientSecret])
	if result.DingTalkConnectClientSecret == "" {
		result.DingTalkConnectClientSecret = strings.TrimSpace(dingTalkBase.ClientSecret)
REDACTED
	result.DingTalkConnectClientSecretConfigured = result.DingTalkConnectClientSecret != ""

	if v, ok := settings[SettingKeyDingTalkConnectCorpRestrictionPolicy]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectCorpRestrictionPolicy = strings.TrimSpace(v)
REDACTED else {
		result.DingTalkConnectCorpRestrictionPolicy = dingTalkBase.CorpRestrictionPolicy
REDACTED
	result.DingTalkConnectCorpRestrictionPolicy = coerceDeprecatedDingTalkCorpPolicy(result.DingTalkConnectCorpRestrictionPolicy)

	if v, ok := settings[SettingKeyDingTalkConnectInternalCorpID]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectInternalCorpID = strings.TrimSpace(v)
REDACTED else {
		result.DingTalkConnectInternalCorpID = dingTalkBase.InternalCorpID
REDACTED

	if v, ok := settings[SettingKeyDingTalkConnectBypassRegistration]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectBypassRegistration = strings.EqualFold(strings.TrimSpace(v), "true")
REDACTED else {
		result.DingTalkConnectBypassRegistration = dingTalkBase.BypassRegistration
REDACTED
	// bypass_registration 仅在 internal_only 模式下有意义；其它策略下强制 false，
	// 以保证加载出的 effective config 永远是一致状态。
	if result.DingTalkConnectCorpRestrictionPolicy != "internal_only" {
		result.DingTalkConnectBypassRegistration = false
REDACTED

	if v, ok := settings[SettingKeyDingTalkConnectSyncCorpEmail]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectSyncCorpEmail = strings.EqualFold(strings.TrimSpace(v), "true")
REDACTED else {
		result.DingTalkConnectSyncCorpEmail = dingTalkBase.SyncCorpEmail
REDACTED
	if v, ok := settings[SettingKeyDingTalkConnectSyncDisplayName]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectSyncDisplayName = strings.EqualFold(strings.TrimSpace(v), "true")
REDACTED else {
		result.DingTalkConnectSyncDisplayName = dingTalkBase.SyncDisplayName
REDACTED
	if v, ok := settings[SettingKeyDingTalkConnectSyncDept]; ok && strings.TrimSpace(v) != "" {
		result.DingTalkConnectSyncDept = strings.EqualFold(strings.TrimSpace(v), "true")
REDACTED else {
		result.DingTalkConnectSyncDept = dingTalkBase.SyncDept
REDACTED
	// 身份同步三开关仅在 internal_only 模式下有意义；其它策略强制 false。
	if result.DingTalkConnectCorpRestrictionPolicy != "internal_only" {
		result.DingTalkConnectSyncCorpEmail = false
		result.DingTalkConnectSyncDisplayName = false
		result.DingTalkConnectSyncDept = false
REDACTED

	// 身份同步目标 attr key（DB 空 → fallback 默认值）
	result.DingTalkConnectSyncCorpEmailAttrKey = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncCorpEmailAttrKey])
	if result.DingTalkConnectSyncCorpEmailAttrKey == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncCorpEmailAttrKey); v != "" {
			result.DingTalkConnectSyncCorpEmailAttrKey = v
	REDACTED else {
			result.DingTalkConnectSyncCorpEmailAttrKey = "dingtalk_email"
	REDACTED
REDACTED
	result.DingTalkConnectSyncDisplayNameAttrKey = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDisplayNameAttrKey])
	if result.DingTalkConnectSyncDisplayNameAttrKey == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDisplayNameAttrKey); v != "" {
			result.DingTalkConnectSyncDisplayNameAttrKey = v
	REDACTED else {
			result.DingTalkConnectSyncDisplayNameAttrKey = "dingtalk_name"
	REDACTED
REDACTED
	result.DingTalkConnectSyncDeptAttrKey = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDeptAttrKey])
	if result.DingTalkConnectSyncDeptAttrKey == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDeptAttrKey); v != "" {
			result.DingTalkConnectSyncDeptAttrKey = v
	REDACTED else {
			result.DingTalkConnectSyncDeptAttrKey = "dingtalk_department"
	REDACTED
REDACTED

	// 身份同步目标 attr 显示名称（DB 空 → fallback 默认中文）
	result.DingTalkConnectSyncCorpEmailAttrName = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncCorpEmailAttrName])
	if result.DingTalkConnectSyncCorpEmailAttrName == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncCorpEmailAttrName); v != "" {
			result.DingTalkConnectSyncCorpEmailAttrName = v
	REDACTED else {
			result.DingTalkConnectSyncCorpEmailAttrName = "钉钉企业邮箱"
	REDACTED
REDACTED
	result.DingTalkConnectSyncDisplayNameAttrName = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDisplayNameAttrName])
	if result.DingTalkConnectSyncDisplayNameAttrName == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDisplayNameAttrName); v != "" {
			result.DingTalkConnectSyncDisplayNameAttrName = v
	REDACTED else {
			result.DingTalkConnectSyncDisplayNameAttrName = "钉钉姓名"
	REDACTED
REDACTED
	result.DingTalkConnectSyncDeptAttrName = strings.TrimSpace(settings[SettingKeyDingTalkConnectSyncDeptAttrName])
	if result.DingTalkConnectSyncDeptAttrName == "" {
		if v := strings.TrimSpace(dingTalkBase.SyncDeptAttrName); v != "" {
			result.DingTalkConnectSyncDeptAttrName = v
	REDACTED else {
			result.DingTalkConnectSyncDeptAttrName = "钉钉部门"
	REDACTED
REDACTED

	// Generic OIDC 设置：
	// - 兼容 config.yaml/env
	// - 支持后台系统设置覆盖并持久化（存储于 DB）
	oidcBase := config.OIDCConnectConfig{REDACTED
	if s.cfg != nil {
		oidcBase = s.cfg.OIDC
REDACTED

	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		result.OIDCConnectEnabled = raw == "true"
REDACTED else {
		result.OIDCConnectEnabled = oidcBase.Enabled
REDACTED

	if v, ok := settings[SettingKeyOIDCConnectProviderName]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectProviderName = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectProviderName = strings.TrimSpace(oidcBase.ProviderName)
REDACTED
	if result.OIDCConnectProviderName == "" {
		result.OIDCConnectProviderName = "OIDC"
REDACTED

	if v, ok := settings[SettingKeyOIDCConnectClientID]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectClientID = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectClientID = strings.TrimSpace(oidcBase.ClientID)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectIssuerURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectIssuerURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectIssuerURL = strings.TrimSpace(oidcBase.IssuerURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectDiscoveryURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectDiscoveryURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectDiscoveryURL = strings.TrimSpace(oidcBase.DiscoveryURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectAuthorizeURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectAuthorizeURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectAuthorizeURL = strings.TrimSpace(oidcBase.AuthorizeURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectTokenURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectTokenURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectTokenURL = strings.TrimSpace(oidcBase.TokenURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectUserInfoURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectUserInfoURL = strings.TrimSpace(oidcBase.UserInfoURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectJWKSURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectJWKSURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectJWKSURL = strings.TrimSpace(oidcBase.JWKSURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectScopes]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectScopes = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectScopes = strings.TrimSpace(oidcBase.Scopes)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectRedirectURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectRedirectURL = strings.TrimSpace(oidcBase.RedirectURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectFrontendRedirectURL]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectFrontendRedirectURL = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectFrontendRedirectURL = strings.TrimSpace(oidcBase.FrontendRedirectURL)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectTokenAuthMethod]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(v))
REDACTED else {
		result.OIDCConnectTokenAuthMethod = strings.ToLower(strings.TrimSpace(oidcBase.TokenAuthMethod))
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectUsePKCE]; ok {
		result.OIDCConnectUsePKCE = raw == "true"
REDACTED else {
		result.OIDCConnectUsePKCE = oidcUsePKCECompatibilityDefault(oidcBase)
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectValidateIDToken]; ok {
		result.OIDCConnectValidateIDToken = raw == "true"
REDACTED else {
		result.OIDCConnectValidateIDToken = oidcValidateIDTokenCompatibilityDefault(oidcBase)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectAllowedSigningAlgs]; ok && strings.TrimSpace(v) != "" {
		result.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectAllowedSigningAlgs = strings.TrimSpace(oidcBase.AllowedSigningAlgs)
REDACTED
	clockSkewSet := false
	if raw, ok := settings[SettingKeyOIDCConnectClockSkewSeconds]; ok && strings.TrimSpace(raw) != "" {
		if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			result.OIDCConnectClockSkewSeconds = parsed
			clockSkewSet = true
	REDACTED
REDACTED
	if !clockSkewSet {
		result.OIDCConnectClockSkewSeconds = oidcBase.ClockSkewSeconds
REDACTED
	if !clockSkewSet && result.OIDCConnectClockSkewSeconds == 0 {
		result.OIDCConnectClockSkewSeconds = 120
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectRequireEmailVerified]; ok {
		result.OIDCConnectRequireEmailVerified = raw == "true"
REDACTED else {
		result.OIDCConnectRequireEmailVerified = oidcBase.RequireEmailVerified
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoEmailPath]; ok {
		result.OIDCConnectUserInfoEmailPath = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectUserInfoEmailPath = strings.TrimSpace(oidcBase.UserInfoEmailPath)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoIDPath]; ok {
		result.OIDCConnectUserInfoIDPath = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectUserInfoIDPath = strings.TrimSpace(oidcBase.UserInfoIDPath)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoUsernamePath]; ok {
		result.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(v)
REDACTED else {
		result.OIDCConnectUserInfoUsernamePath = strings.TrimSpace(oidcBase.UserInfoUsernamePath)
REDACTED
	result.OIDCConnectClientSecret = strings.TrimSpace(settings[SettingKeyOIDCConnectClientSecret])
	if result.OIDCConnectClientSecret == "" {
		result.OIDCConnectClientSecret = strings.TrimSpace(oidcBase.ClientSecret)
REDACTED
	result.OIDCConnectClientSecretConfigured = result.OIDCConnectClientSecret != ""

	gitHubEffective := s.effectiveEmailOAuthConfig(settings, "github")
	result.GitHubOAuthEnabled = gitHubEffective.Enabled
	result.GitHubOAuthClientID = strings.TrimSpace(gitHubEffective.ClientID)
	result.GitHubOAuthClientSecret = strings.TrimSpace(gitHubEffective.ClientSecret)
	result.GitHubOAuthClientSecretConfigured = result.GitHubOAuthClientSecret != ""
	result.GitHubOAuthRedirectURL = strings.TrimSpace(gitHubEffective.RedirectURL)
	result.GitHubOAuthFrontendRedirectURL = strings.TrimSpace(gitHubEffective.FrontendRedirectURL)

	googleEffective := s.effectiveEmailOAuthConfig(settings, "google")
	result.GoogleOAuthEnabled = googleEffective.Enabled
	result.GoogleOAuthClientID = strings.TrimSpace(googleEffective.ClientID)
	result.GoogleOAuthClientSecret = strings.TrimSpace(googleEffective.ClientSecret)
	result.GoogleOAuthClientSecretConfigured = result.GoogleOAuthClientSecret != ""
	result.GoogleOAuthRedirectURL = strings.TrimSpace(googleEffective.RedirectURL)
	result.GoogleOAuthFrontendRedirectURL = strings.TrimSpace(googleEffective.FrontendRedirectURL)

	// WeChat Connect 设置：
	// - 优先读取 DB 系统设置
	// - 缺失时回退到 config/env，保持升级兼容
	weChatEffective := s.effectiveWeChatConnectOAuthConfig(settings)
	result.WeChatConnectEnabled = weChatEffective.Enabled
	result.WeChatConnectAppID = weChatEffective.LegacyAppID
	result.WeChatConnectAppSecret = weChatEffective.LegacyAppSecret
	result.WeChatConnectAppSecretConfigured = weChatEffective.LegacyAppSecret != ""
	result.WeChatConnectOpenAppID = weChatEffective.OpenAppID
	result.WeChatConnectOpenAppSecret = weChatEffective.OpenAppSecret
	result.WeChatConnectOpenAppSecretConfigured = weChatEffective.OpenAppSecret != ""
	result.WeChatConnectMPAppID = weChatEffective.MPAppID
	result.WeChatConnectMPAppSecret = weChatEffective.MPAppSecret
	result.WeChatConnectMPAppSecretConfigured = weChatEffective.MPAppSecret != ""
	result.WeChatConnectMobileAppID = weChatEffective.MobileAppID
	result.WeChatConnectMobileAppSecret = weChatEffective.MobileAppSecret
	result.WeChatConnectMobileAppSecretConfigured = weChatEffective.MobileAppSecret != ""
	result.WeChatConnectOpenEnabled = weChatEffective.OpenEnabled
	result.WeChatConnectMPEnabled = weChatEffective.MPEnabled
	result.WeChatConnectMobileEnabled = weChatEffective.MobileEnabled
	result.WeChatConnectMode = weChatEffective.Mode
	result.WeChatConnectScopes = weChatEffective.Scopes
	result.WeChatConnectRedirectURL = weChatEffective.RedirectURL
	result.WeChatConnectFrontendRedirectURL = weChatEffective.FrontendRedirectURL

	// Model fallback settings
	result.EnableModelFallback = settings[SettingKeyEnableModelFallback] == "true"
	result.FallbackModelAnthropic = s.getStringOrDefault(settings, SettingKeyFallbackModelAnthropic, "claude-3-5-sonnet-20241022")
	result.FallbackModelOpenAI = s.getStringOrDefault(settings, SettingKeyFallbackModelOpenAI, "gpt-4o")
	result.FallbackModelGemini = s.getStringOrDefault(settings, SettingKeyFallbackModelGemini, "gemini-2.5-pro")
	result.FallbackModelAntigravity = s.getStringOrDefault(settings, SettingKeyFallbackModelAntigravity, "gemini-2.5-pro")

	// Identity patch settings (default: enabled, to preserve existing behavior)
	if v, ok := settings[SettingKeyEnableIdentityPatch]; ok && v != "" {
		result.EnableIdentityPatch = v == "true"
REDACTED else {
		result.EnableIdentityPatch = true
REDACTED
	result.IdentityPatchPrompt = settings[SettingKeyIdentityPatchPrompt]

	// Ops monitoring settings (default: enabled, fail-open)
	result.OpsMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsMonitoringEnabled])
	result.OpsRealtimeMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsRealtimeMonitoringEnabled])
	result.OpsQueryModeDefault = string(ParseOpsQueryMode(settings[SettingKeyOpsQueryModeDefault]))
	result.OpsMetricsIntervalSeconds = 60
	if raw := strings.TrimSpace(settings[SettingKeyOpsMetricsIntervalSeconds]); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v < 60 {
				v = 60
		REDACTED
			if v > 3600 {
				v = 3600
		REDACTED
			result.OpsMetricsIntervalSeconds = v
	REDACTED
REDACTED

	// Channel monitor feature (default: enabled, 60s)
	result.ChannelMonitorEnabled = !isFalseSettingValue(settings[SettingKeyChannelMonitorEnabled])
	result.ChannelMonitorDefaultIntervalSeconds = parseChannelMonitorInterval(
		settings[SettingKeyChannelMonitorDefaultIntervalSeconds],
	)

	// Available channels feature (default: disabled; strict true)
	result.AvailableChannelsEnabled = settings[SettingKeyAvailableChannelsEnabled] == "true"

	// Affiliate (邀请返利) feature (default: disabled; strict true)
	result.AffiliateEnabled = settings[SettingKeyAffiliateEnabled] == "true"

	// 风控中心功能（默认关闭，严格 true 才启用）
	result.RiskControlEnabled = settings[SettingKeyRiskControlEnabled] == "true"

	// cyber 会话屏蔽（默认关闭，TTL 默认 3600s）
	result.CyberSessionBlockEnabled = settings[SettingKeyCyberSessionBlockEnabled] == "true"
	if v, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyCyberSessionBlockTTLSeconds])); err == nil && v > 0 {
		result.CyberSessionBlockTTLSeconds = v
REDACTED else {
		result.CyberSessionBlockTTLSeconds = 3600
REDACTED

	// Claude Code version check
	result.MinClaudeCodeVersion = settings[SettingKeyMinClaudeCodeVersion]
	result.MaxClaudeCodeVersion = settings[SettingKeyMaxClaudeCodeVersion]

	// 分组隔离
	result.AllowUngroupedKeyScheduling = settings[SettingKeyAllowUngroupedKeyScheduling] == "true"

	// Gateway forwarding behavior (defaults: fingerprint=true, metadata_passthrough=false,
	// cch_signing=false, claude_oauth_system_prompt_injection=true)
	if v, ok := settings[SettingKeyEnableFingerprintUnification]; ok && v != "" {
		result.EnableFingerprintUnification = v == "true"
REDACTED else {
		result.EnableFingerprintUnification = true // default: enabled (current behavior)
REDACTED
	result.EnableMetadataPassthrough = settings[SettingKeyEnableMetadataPassthrough] == "true"
	result.EnableCCHSigning = settings[SettingKeyEnableCCHSigning] == "true"
	if v, ok := settings[SettingKeyEnableClaudeOAuthSystemPromptInjection]; ok && v != "" {
		result.EnableClaudeOAuthSystemPromptInjection = v == "true"
REDACTED else {
		result.EnableClaudeOAuthSystemPromptInjection = true
REDACTED
	result.ClaudeOAuthSystemPrompt = settings[SettingKeyClaudeOAuthSystemPrompt]
	result.ClaudeOAuthSystemPromptBlocks = settings[SettingKeyClaudeOAuthSystemPromptBlocks]
	result.EnableAnthropicCacheTTL1hInjection = settings[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"
	if v, ok := settings[SettingKeyRewriteMessageCacheControl]; ok && v != "" {
		result.RewriteMessageCacheControl = v == "true"
REDACTED else {
		result.RewriteMessageCacheControl = s.defaultRewriteMessageCacheControl()
REDACTED
	if v, ok := settings[SettingKeyEnableClientDatelineNormalization]; ok && v != "" {
		result.EnableClientDatelineNormalization = v == "true"
REDACTED else {
		result.EnableClientDatelineNormalization = true
REDACTED
	result.AntigravityUserAgentVersion = antigravity.NormalizeUserAgentVersion(settings[SettingKeyAntigravityUserAgentVersion])
	result.OpenAICodexUserAgent = strings.TrimSpace(settings[SettingKeyOpenAICodexUserAgent])
	// codex_cli_only 加固
	result.MinCodexVersion = settings[SettingKeyMinCodexVersion]
	result.MaxCodexVersion = settings[SettingKeyMaxCodexVersion]
	result.CodexCLIOnlyBlacklist = settings[SettingKeyCodexCLIOnlyBlacklist]
	result.CodexCLIOnlyWhitelist = settings[SettingKeyCodexCLIOnlyWhitelist]
	result.CodexCLIOnlyAllowAppServerClients = settings[SettingKeyCodexCLIOnlyAllowAppServerClients] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyCodexCLIOnlyEngineFingerprintSignals]); raw != "" {
		result.CodexCLIOnlyEngineFingerprintSignals = raw
REDACTED else {
		result.CodexCLIOnlyEngineFingerprintSignals = openai.DefaultEngineFingerprintSignalsJSON() // 缺失/空 → 展示默认种子
REDACTED

	// Web search emulation: quick enabled check from the JSON config
	if raw := settings[SettingKeyWebSearchEmulationConfig]; raw != "" {
		var wsCfg WebSearchEmulationConfig
		if err := json.Unmarshal([]byte(raw), &wsCfg); err == nil {
			result.WebSearchEmulationEnabled = wsCfg.Enabled && len(wsCfg.Providers) > 0
	REDACTED
REDACTED
	result.PaymentVisibleMethodAlipaySource = NormalizeVisibleMethodSource("alipay", settings[SettingPaymentVisibleMethodAlipaySource])
	result.PaymentVisibleMethodWxpaySource = NormalizeVisibleMethodSource("wxpay", settings[SettingPaymentVisibleMethodWxpaySource])
	result.PaymentVisibleMethodAlipayEnabled = settings[SettingPaymentVisibleMethodAlipayEnabled] == "true"
	result.PaymentVisibleMethodWxpayEnabled = settings[SettingPaymentVisibleMethodWxpayEnabled] == "true"
	result.OpenAIAdvancedSchedulerEnabled = settings[openAIAdvancedSchedulerSettingKey] == "true"
	result.OpenAIAdvancedSchedulerStickyWeightedEnabled = settings[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled] == "true"
	result.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled = settings[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled] == "true"
	result.OpenAIAdvancedSchedulerLBTopK = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerLBTopK])
	result.OpenAIAdvancedSchedulerWeightPriority = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightPriority])
	result.OpenAIAdvancedSchedulerWeightLoad = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightLoad])
	result.OpenAIAdvancedSchedulerWeightQueue = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightQueue])
	result.OpenAIAdvancedSchedulerWeightErrorRate = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate])
	result.OpenAIAdvancedSchedulerWeightTTFT = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightTTFT])
	result.OpenAIAdvancedSchedulerWeightReset = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightReset])
	result.OpenAIAdvancedSchedulerWeightQuotaHeadroom = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom])
	result.OpenAIAdvancedSchedulerWeightPreviousResponse = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse])
	result.OpenAIAdvancedSchedulerWeightSessionSticky = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky])
	result.OpenAIAdvancedSchedulerEffectiveLBTopK = s.openAIAdvancedSchedulerEffectiveLBTopK()
	effectiveWeights := s.openAIAdvancedSchedulerEffectiveWeights()
	result.OpenAIAdvancedSchedulerEffectiveWeightPriority = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Priority)
	result.OpenAIAdvancedSchedulerEffectiveWeightLoad = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Load)
	result.OpenAIAdvancedSchedulerEffectiveWeightQueue = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Queue)
	result.OpenAIAdvancedSchedulerEffectiveWeightErrorRate = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.ErrorRate)
	result.OpenAIAdvancedSchedulerEffectiveWeightTTFT = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.TTFT)
	result.OpenAIAdvancedSchedulerEffectiveWeightReset = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Reset)
	result.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.QuotaHeadroom)
	result.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.PreviousResponse)
	result.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.SessionSticky)

	// 余额、订阅到期与账号限额通知
	result.BalanceLowNotifyEnabled = settings[SettingKeyBalanceLowNotifyEnabled] == "true"
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		result.BalanceLowNotifyThreshold = v
REDACTED
	result.BalanceLowNotifyRechargeURL = settings[SettingKeyBalanceLowNotifyRechargeURL]
	result.SubscriptionExpiryNotifyEnabled = !isFalseSettingValue(settings[SettingKeySubscriptionExpiryNotifyEnabled])

	// 账号限额通知
	result.AccountQuotaNotifyEnabled = settings[SettingKeyAccountQuotaNotifyEnabled] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyAccountQuotaNotifyEmails]); raw != "" {
		result.AccountQuotaNotifyEmails = ParseNotifyEmails(raw)
REDACTED
	if result.AccountQuotaNotifyEmails == nil {
		result.AccountQuotaNotifyEmails = []NotifyEmailEntry{REDACTED
REDACTED

	// 系统层默认 platform quota（修复 Bug B：parseSettings 不填充导致回显恒为 nil）
	if raw := settings[SettingKeyDefaultPlatformQuotas]; raw != "" {
		parsed := map[string]*DefaultPlatformQuotaSetting{REDACTED
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			slog.Warn("[Setting] parseSettings: unmarshal default_platform_quotas failed", "error", err)
	REDACTED else {
			result.DefaultPlatformQuotas = parsed
	REDACTED
REDACTED

	result.AllowUserViewErrorRequests = settings[SettingKeyAllowUserViewErrorRequests] == "true" // default false

	return result
REDACTED

func clampAffiliateRebateRate(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return AffiliateRebateRateDefault
REDACTED
	if value < AffiliateRebateRateMin {
		return AffiliateRebateRateMin
REDACTED
	if value > AffiliateRebateRateMax {
		return AffiliateRebateRateMax
REDACTED
	return value
REDACTED

func isFalseSettingValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return true
	default:
		return false
REDACTED
REDACTED

func normalizeVisibleMethodSettingSource(method, source string, enabled bool) (string, error) {
	_ = enabled
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
REDACTED

	normalized := NormalizeVisibleMethodSource(method, source)
	if normalized == "" {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
REDACTED
	return normalized, nil
REDACTED

func (s *SettingService) openAIAdvancedSchedulerEffectiveLBTopK() string {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return strconv.Itoa(s.cfg.Gateway.OpenAIWS.LBTopK)
REDACTED
	return "7"
REDACTED

func (s *SettingService) openAIAdvancedSchedulerEffectiveWeights() config.GatewayOpenAIWSSchedulerScoreWeights {
	defaults := config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         1.0,
		Load:             1.0,
		Queue:            0.7,
		ErrorRate:        0.8,
		TTFT:             0.5,
		Reset:            0.0,
		QuotaHeadroom:    0.0,
		PreviousResponse: 5.0,
		SessionSticky:    3.0,
REDACTED
	if s == nil || s.cfg == nil {
		return defaults
REDACTED

	weights := s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights
	baseSum := weights.Priority + weights.Load + weights.Queue + weights.ErrorRate + weights.TTFT + weights.QuotaHeadroom
	if baseSum <= 0 {
		return defaults
REDACTED
	return weights
REDACTED

func formatOpenAIAdvancedSchedulerFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
REDACTED

func (s *SettingService) normalizeOpenAIAdvancedSchedulerOverrides(settings *SystemSettings) error {
	lbTopK, err := normalizeOptionalPositiveIntString(settings.OpenAIAdvancedSchedulerLBTopK)
	if err != nil {
		return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_LB_TOP_K", "openai advanced scheduler TopK must be a positive integer or empty")
REDACTED
	settings.OpenAIAdvancedSchedulerLBTopK = lbTopK

	weights := []*string{
		&settings.OpenAIAdvancedSchedulerWeightPriority,
		&settings.OpenAIAdvancedSchedulerWeightLoad,
		&settings.OpenAIAdvancedSchedulerWeightQueue,
		&settings.OpenAIAdvancedSchedulerWeightErrorRate,
		&settings.OpenAIAdvancedSchedulerWeightTTFT,
		&settings.OpenAIAdvancedSchedulerWeightReset,
		&settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		&settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		&settings.OpenAIAdvancedSchedulerWeightSessionSticky,
REDACTED
	for _, target := range weights {
		normalized, err := normalizeOptionalNonNegativeFloatString(*target)
		if err != nil {
			return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler weights must be non-negative numbers or empty")
	REDACTED
		*target = normalized
REDACTED

	// 与 config.Validate 的 "scheduler_score_weights must not all be zero" 保持一致：
	// 覆盖值（空则回退到生效的配置值）叠加后的基础权重和不允许为 0，
	// 否则调度会静默退化为 TopK 内均匀随机。
	effective := s.openAIAdvancedSchedulerEffectiveWeights()
	baseSum := resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightPriority, effective.Priority) +
		resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightLoad, effective.Load) +
		resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightQueue, effective.Queue) +
		resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightErrorRate, effective.ErrorRate) +
		resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightTTFT, effective.TTFT) +
		resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom, effective.QuotaHeadroom)
	if baseSum <= 0 {
		return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler base weights must not all be zero")
REDACTED
	return nil
REDACTED

// resolveOpenAIAdvancedSchedulerWeight 返回覆盖值（已归一化的非空字符串），空则回退默认值。
func resolveOpenAIAdvancedSchedulerWeight(normalized string, fallback float64) float64 {
	if normalized == "" {
		return fallback
REDACTED
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return fallback
REDACTED
	return value
REDACTED

func normalizeOptionalPositiveIntString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
REDACTED
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return "", fmt.Errorf("invalid positive integer")
REDACTED
	return strconv.Itoa(value), nil
REDACTED

func normalizeOptionalNonNegativeFloatString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
REDACTED
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("invalid non-negative float")
REDACTED
	return strconv.FormatFloat(value, 'f', -1, 64), nil
REDACTED

func parseDefaultSubscriptions(raw string) []DefaultSubscriptionSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
REDACTED

	var items []DefaultSubscriptionSetting
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
REDACTED

	normalized := make([]DefaultSubscriptionSetting, 0, len(items))
	for _, item := range items {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
	REDACTED
		if item.ValidityDays > MaxValidityDays {
			item.ValidityDays = MaxValidityDays
	REDACTED
		normalized = append(normalized, item)
REDACTED

	return normalized
REDACTED

func parseProviderDefaultGrantSettings(settings map[string]string, keys authSourceDefaultKeySet) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          defaultAuthSourceBalance,
		Concurrency:      defaultAuthSourceConcurrency,
		Subscriptions:    []DefaultSubscriptionSetting{REDACTED,
		GrantOnSignup:    false,
		GrantOnFirstBind: false,
REDACTED

	if v, err := strconv.ParseFloat(strings.TrimSpace(settings[keys.balance]), 64); err == nil {
		result.Balance = v
REDACTED
	if v, err := strconv.Atoi(strings.TrimSpace(settings[keys.concurrency])); err == nil {
		result.Concurrency = v
REDACTED
	if items := parseDefaultSubscriptions(settings[keys.subscriptions]); items != nil {
		result.Subscriptions = items
REDACTED
	if raw, ok := settings[keys.grantOnSignup]; ok {
		result.GrantOnSignup = raw == "true"
REDACTED
	if raw, ok := settings[keys.grantOnFirstBind]; ok {
		result.GrantOnFirstBind = raw == "true"
REDACTED

	if raw := settings[keys.platformQuotas]; raw != "" {
		parsed := map[string]*DefaultPlatformQuotaSetting{REDACTED
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			slog.Warn("[Setting] parseProviderDefaultGrantSettings: unmarshal auth source platform quotas failed", "source", keys.source, "error", err)
	REDACTED else {
			result.PlatformQuotas = parsed
	REDACTED
REDACTED

	return result
REDACTED

func writeProviderDefaultGrantUpdates(updates map[string]string, keys authSourceDefaultKeySet, settings ProviderDefaultGrantSettings) {
	updates[keys.balance] = strconv.FormatFloat(settings.Balance, 'f', 8, 64)
	updates[keys.concurrency] = strconv.Itoa(settings.Concurrency)

	subscriptions := settings.Subscriptions
	if subscriptions == nil {
		subscriptions = []DefaultSubscriptionSetting{REDACTED
REDACTED
	raw, err := json.Marshal(subscriptions)
	if err != nil {
		raw = []byte("[]")
REDACTED
	updates[keys.subscriptions] = string(raw)
	updates[keys.grantOnSignup] = strconv.FormatBool(settings.GrantOnSignup)
	updates[keys.grantOnFirstBind] = strconv.FormatBool(settings.GrantOnFirstBind)

	// auth source platform quota：整体替换语义。
	// nil = 请求未携带该字段，跳过写入以保留既有配置（与系统层 buildSystemSettingsUpdates 的
	// DefaultPlatformQuotas nil 守卫一致）；非 nil（含空 map）才整体替换。二者语义不可混同。
	if keys.platformQuotas != "" && settings.PlatformQuotas != nil {
		blob, err := json.Marshal(settings.PlatformQuotas)
		if err != nil {
			blob = []byte("{REDACTED")
	REDACTED
		updates[keys.platformQuotas] = string(blob)
REDACTED
REDACTED

func mergeProviderDefaultGrantSettings(globalDefaults ProviderDefaultGrantSettings, providerDefaults ProviderDefaultGrantSettings) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          globalDefaults.Balance,
		Concurrency:      globalDefaults.Concurrency,
		Subscriptions:    append([]DefaultSubscriptionSetting(nil), globalDefaults.Subscriptions...),
		GrantOnSignup:    providerDefaults.GrantOnSignup,
		GrantOnFirstBind: providerDefaults.GrantOnFirstBind,
REDACTED

	// 注意：不能把 parse 默认值 (defaultAuthSourceBalance / defaultAuthSourceConcurrency)
	// 当作"未配置"哨兵——admin 完全有权显式设成相同的值，那时仍应覆盖 globalDefaults。
	// 旧实现的 `!= defaultAuthSourceConcurrency` 会把 admin 设的 5 与 fallback 5 混淆，
	// 导致渠道发放退回到全局默认（如 1），表现为"管理员设 5、新用户实际拿 1"。
	if providerDefaults.Balance >= 0 {
		result.Balance = providerDefaults.Balance
REDACTED
	if providerDefaults.Concurrency > 0 {
		result.Concurrency = providerDefaults.Concurrency
REDACTED
	if len(providerDefaults.Subscriptions) > 0 {
		result.Subscriptions = append([]DefaultSubscriptionSetting(nil), providerDefaults.Subscriptions...)
REDACTED

	return result
REDACTED

func parseTablePreferences(defaultPageSizeRaw, optionsRaw string) (int, []int) {
	defaultPageSize := 20
	if v, err := strconv.Atoi(strings.TrimSpace(defaultPageSizeRaw)); err == nil {
		defaultPageSize = v
REDACTED

	var options []int
	if strings.TrimSpace(optionsRaw) != "" {
		_ = json.Unmarshal([]byte(optionsRaw), &options)
REDACTED

	return normalizeTablePreferences(defaultPageSize, options)
REDACTED

func normalizeTablePreferences(defaultPageSize int, options []int) (int, []int) {
	const minPageSize = 5
	const maxPageSize = 1000
	const fallbackPageSize = 20

	seen := make(map[int]struct{REDACTED, len(options))
	normalizedOptions := make([]int, 0, len(options))
	for _, option := range options {
		if option < minPageSize || option > maxPageSize {
			continue
	REDACTED
		if _, ok := seen[option]; ok {
			continue
	REDACTED
		seen[option] = struct{REDACTED{REDACTED
		normalizedOptions = append(normalizedOptions, option)
REDACTED
	sort.Ints(normalizedOptions)

	if defaultPageSize < minPageSize || defaultPageSize > maxPageSize {
		defaultPageSize = fallbackPageSize
REDACTED

	if len(normalizedOptions) == 0 {
		normalizedOptions = []int{10, 20, 50REDACTED
REDACTED

	return defaultPageSize, normalizedOptions
REDACTED
