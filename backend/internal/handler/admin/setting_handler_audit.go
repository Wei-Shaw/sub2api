package admin

import (
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) auditSettingsUpdate(c *gin.Context, before *service.SystemSettings, after *service.SystemSettings, beforeAuthSourceDefaults *service.AuthSourceDefaultSettings, afterAuthSourceDefaults *service.AuthSourceDefaultSettings, req UpdateSettingsRequest) {
	if before == nil || after == nil {
		return
REDACTED

	changed := diffSettings(before, after, beforeAuthSourceDefaults, afterAuthSourceDefaults, req)
	if len(changed) == 0 {
		return
REDACTED

	subject, _ := middleware.GetAuthSubjectFromContext(c)
	role, _ := middleware.GetUserRoleFromContext(c)
	slog.Info("settings updated",
		"audit", true,
		"user_id", subject.UserID,
		"role", role,
		"changed", changed,
	)
REDACTED

func diffSettings(before *service.SystemSettings, after *service.SystemSettings, beforeAuthSourceDefaults *service.AuthSourceDefaultSettings, afterAuthSourceDefaults *service.AuthSourceDefaultSettings, req UpdateSettingsRequest) []string {
	changed := make([]string, 0, 20)
	if before.RegistrationEnabled != after.RegistrationEnabled {
		changed = append(changed, "registration_enabled")
REDACTED
	if before.EmailVerifyEnabled != after.EmailVerifyEnabled {
		changed = append(changed, "email_verify_enabled")
REDACTED
	if !equalStringSlice(before.RegistrationEmailSuffixWhitelist, after.RegistrationEmailSuffixWhitelist) {
		changed = append(changed, "registration_email_suffix_whitelist")
REDACTED
	if before.PromoCodeEnabled != after.PromoCodeEnabled {
		changed = append(changed, "promo_code_enabled")
REDACTED
	if before.InvitationCodeEnabled != after.InvitationCodeEnabled {
		changed = append(changed, "invitation_code_enabled")
REDACTED
	if before.PasswordResetEnabled != after.PasswordResetEnabled {
		changed = append(changed, "password_reset_enabled")
REDACTED
	if before.FrontendURL != after.FrontendURL {
		changed = append(changed, "frontend_url")
REDACTED
	if before.TotpEnabled != after.TotpEnabled {
		changed = append(changed, "totp_enabled")
REDACTED
	if before.LoginAgreementEnabled != after.LoginAgreementEnabled {
		changed = append(changed, "login_agreement_enabled")
REDACTED
	if before.LoginAgreementMode != after.LoginAgreementMode {
		changed = append(changed, "login_agreement_mode")
REDACTED
	if before.LoginAgreementUpdatedAt != after.LoginAgreementUpdatedAt {
		changed = append(changed, "login_agreement_updated_at")
REDACTED
	if !equalLoginAgreementDocuments(before.LoginAgreementDocuments, after.LoginAgreementDocuments) {
		changed = append(changed, "login_agreement_documents")
REDACTED
	if before.SMTPHost != after.SMTPHost {
		changed = append(changed, "smtp_host")
REDACTED
	if before.SMTPPort != after.SMTPPort {
		changed = append(changed, "smtp_port")
REDACTED
	if before.SMTPUsername != after.SMTPUsername {
		changed = append(changed, "smtp_username")
REDACTED
	if req.SMTPPassword != "" {
		changed = append(changed, "smtp_password")
REDACTED
	if before.SMTPFrom != after.SMTPFrom {
		changed = append(changed, "smtp_from_email")
REDACTED
	if before.SMTPFromName != after.SMTPFromName {
		changed = append(changed, "smtp_from_name")
REDACTED
	if before.SMTPUseTLS != after.SMTPUseTLS {
		changed = append(changed, "smtp_use_tls")
REDACTED
	if before.TurnstileEnabled != after.TurnstileEnabled {
		changed = append(changed, "turnstile_enabled")
REDACTED
	if before.TurnstileSiteKey != after.TurnstileSiteKey {
		changed = append(changed, "turnstile_site_key")
REDACTED
	if req.TurnstileSecretKey != "" {
		changed = append(changed, "turnstile_secret_key")
REDACTED
	if before.APIKeyACLTrustForwardedIP != after.APIKeyACLTrustForwardedIP {
		changed = append(changed, "api_key_acl_trust_forwarded_ip")
REDACTED
	if before.LinuxDoConnectEnabled != after.LinuxDoConnectEnabled {
		changed = append(changed, "linuxdo_connect_enabled")
REDACTED
	if before.LinuxDoConnectClientID != after.LinuxDoConnectClientID {
		changed = append(changed, "linuxdo_connect_client_id")
REDACTED
	if req.LinuxDoConnectClientSecret != "" {
		changed = append(changed, "linuxdo_connect_client_secret")
REDACTED
	if before.LinuxDoConnectRedirectURL != after.LinuxDoConnectRedirectURL {
		changed = append(changed, "linuxdo_connect_redirect_url")
REDACTED
	if before.DingTalkConnectEnabled != after.DingTalkConnectEnabled {
		changed = append(changed, "dingtalk_connect_enabled")
REDACTED
	if before.DingTalkConnectClientID != after.DingTalkConnectClientID {
		changed = append(changed, "dingtalk_connect_client_id")
REDACTED
	if req.DingTalkConnectClientSecret != "" {
		changed = append(changed, "dingtalk_connect_client_secret")
REDACTED
	if before.DingTalkConnectRedirectURL != after.DingTalkConnectRedirectURL {
		changed = append(changed, "dingtalk_connect_redirect_url")
REDACTED
	if before.DingTalkConnectCorpRestrictionPolicy != after.DingTalkConnectCorpRestrictionPolicy {
		changed = append(changed, "dingtalk_connect_corp_restriction_policy")
REDACTED
	if before.DingTalkConnectInternalCorpID != after.DingTalkConnectInternalCorpID {
		changed = append(changed, "dingtalk_connect_internal_corp_id")
REDACTED
	if before.DingTalkConnectBypassRegistration != after.DingTalkConnectBypassRegistration {
		changed = append(changed, "dingtalk_connect_bypass_registration")
REDACTED
	if before.DingTalkConnectSyncCorpEmail != after.DingTalkConnectSyncCorpEmail {
		changed = append(changed, "dingtalk_connect_sync_corp_email")
REDACTED
	if before.DingTalkConnectSyncDisplayName != after.DingTalkConnectSyncDisplayName {
		changed = append(changed, "dingtalk_connect_sync_display_name")
REDACTED
	if before.DingTalkConnectSyncDept != after.DingTalkConnectSyncDept {
		changed = append(changed, "dingtalk_connect_sync_dept")
REDACTED
	if before.DingTalkConnectSyncCorpEmailAttrKey != after.DingTalkConnectSyncCorpEmailAttrKey {
		changed = append(changed, "dingtalk_connect_sync_corp_email_attr_key")
REDACTED
	if before.DingTalkConnectSyncDisplayNameAttrKey != after.DingTalkConnectSyncDisplayNameAttrKey {
		changed = append(changed, "dingtalk_connect_sync_display_name_attr_key")
REDACTED
	if before.DingTalkConnectSyncDeptAttrKey != after.DingTalkConnectSyncDeptAttrKey {
		changed = append(changed, "dingtalk_connect_sync_dept_attr_key")
REDACTED
	if before.WeChatConnectEnabled != after.WeChatConnectEnabled {
		changed = append(changed, "wechat_connect_enabled")
REDACTED
	if before.WeChatConnectAppID != after.WeChatConnectAppID {
		changed = append(changed, "wechat_connect_app_id")
REDACTED
	if req.WeChatConnectAppSecret != "" {
		changed = append(changed, "wechat_connect_app_secret")
REDACTED
	if before.WeChatConnectOpenAppID != after.WeChatConnectOpenAppID {
		changed = append(changed, "wechat_connect_open_app_id")
REDACTED
	if req.WeChatConnectOpenAppSecret != "" {
		changed = append(changed, "wechat_connect_open_app_secret")
REDACTED
	if before.WeChatConnectMPAppID != after.WeChatConnectMPAppID {
		changed = append(changed, "wechat_connect_mp_app_id")
REDACTED
	if req.WeChatConnectMPAppSecret != "" {
		changed = append(changed, "wechat_connect_mp_app_secret")
REDACTED
	if before.WeChatConnectMobileAppID != after.WeChatConnectMobileAppID {
		changed = append(changed, "wechat_connect_mobile_app_id")
REDACTED
	if req.WeChatConnectMobileAppSecret != "" {
		changed = append(changed, "wechat_connect_mobile_app_secret")
REDACTED
	if before.WeChatConnectOpenEnabled != after.WeChatConnectOpenEnabled {
		changed = append(changed, "wechat_connect_open_enabled")
REDACTED
	if before.WeChatConnectMPEnabled != after.WeChatConnectMPEnabled {
		changed = append(changed, "wechat_connect_mp_enabled")
REDACTED
	if before.WeChatConnectMobileEnabled != after.WeChatConnectMobileEnabled {
		changed = append(changed, "wechat_connect_mobile_enabled")
REDACTED
	if before.WeChatConnectMode != after.WeChatConnectMode {
		changed = append(changed, "wechat_connect_mode")
REDACTED
	if before.WeChatConnectScopes != after.WeChatConnectScopes {
		changed = append(changed, "wechat_connect_scopes")
REDACTED
	if before.WeChatConnectRedirectURL != after.WeChatConnectRedirectURL {
		changed = append(changed, "wechat_connect_redirect_url")
REDACTED
	if before.WeChatConnectFrontendRedirectURL != after.WeChatConnectFrontendRedirectURL {
		changed = append(changed, "wechat_connect_frontend_redirect_url")
REDACTED
	if before.OIDCConnectEnabled != after.OIDCConnectEnabled {
		changed = append(changed, "oidc_connect_enabled")
REDACTED
	if before.OIDCConnectProviderName != after.OIDCConnectProviderName {
		changed = append(changed, "oidc_connect_provider_name")
REDACTED
	if before.OIDCConnectClientID != after.OIDCConnectClientID {
		changed = append(changed, "oidc_connect_client_id")
REDACTED
	if req.OIDCConnectClientSecret != "" {
		changed = append(changed, "oidc_connect_client_secret")
REDACTED
	if before.OIDCConnectIssuerURL != after.OIDCConnectIssuerURL {
		changed = append(changed, "oidc_connect_issuer_url")
REDACTED
	if before.OIDCConnectDiscoveryURL != after.OIDCConnectDiscoveryURL {
		changed = append(changed, "oidc_connect_discovery_url")
REDACTED
	if before.OIDCConnectAuthorizeURL != after.OIDCConnectAuthorizeURL {
		changed = append(changed, "oidc_connect_authorize_url")
REDACTED
	if before.OIDCConnectTokenURL != after.OIDCConnectTokenURL {
		changed = append(changed, "oidc_connect_token_url")
REDACTED
	if before.OIDCConnectUserInfoURL != after.OIDCConnectUserInfoURL {
		changed = append(changed, "oidc_connect_userinfo_url")
REDACTED
	if before.OIDCConnectJWKSURL != after.OIDCConnectJWKSURL {
		changed = append(changed, "oidc_connect_jwks_url")
REDACTED
	if before.OIDCConnectScopes != after.OIDCConnectScopes {
		changed = append(changed, "oidc_connect_scopes")
REDACTED
	if before.OIDCConnectRedirectURL != after.OIDCConnectRedirectURL {
		changed = append(changed, "oidc_connect_redirect_url")
REDACTED
	if before.OIDCConnectFrontendRedirectURL != after.OIDCConnectFrontendRedirectURL {
		changed = append(changed, "oidc_connect_frontend_redirect_url")
REDACTED
	if before.OIDCConnectTokenAuthMethod != after.OIDCConnectTokenAuthMethod {
		changed = append(changed, "oidc_connect_token_auth_method")
REDACTED
	if before.OIDCConnectUsePKCE != after.OIDCConnectUsePKCE {
		changed = append(changed, "oidc_connect_use_pkce")
REDACTED
	if before.OIDCConnectValidateIDToken != after.OIDCConnectValidateIDToken {
		changed = append(changed, "oidc_connect_validate_id_token")
REDACTED
	if before.OIDCConnectAllowedSigningAlgs != after.OIDCConnectAllowedSigningAlgs {
		changed = append(changed, "oidc_connect_allowed_signing_algs")
REDACTED
	if before.OIDCConnectClockSkewSeconds != after.OIDCConnectClockSkewSeconds {
		changed = append(changed, "oidc_connect_clock_skew_seconds")
REDACTED
	if before.OIDCConnectRequireEmailVerified != after.OIDCConnectRequireEmailVerified {
		changed = append(changed, "oidc_connect_require_email_verified")
REDACTED
	if before.OIDCConnectUserInfoEmailPath != after.OIDCConnectUserInfoEmailPath {
		changed = append(changed, "oidc_connect_userinfo_email_path")
REDACTED
	if before.OIDCConnectUserInfoIDPath != after.OIDCConnectUserInfoIDPath {
		changed = append(changed, "oidc_connect_userinfo_id_path")
REDACTED
	if before.OIDCConnectUserInfoUsernamePath != after.OIDCConnectUserInfoUsernamePath {
		changed = append(changed, "oidc_connect_userinfo_username_path")
REDACTED
	if before.SiteName != after.SiteName {
		changed = append(changed, "site_name")
REDACTED
	if before.SiteLogo != after.SiteLogo {
		changed = append(changed, "site_logo")
REDACTED
	if before.SiteSubtitle != after.SiteSubtitle {
		changed = append(changed, "site_subtitle")
REDACTED
	if before.APIBaseURL != after.APIBaseURL {
		changed = append(changed, "api_base_url")
REDACTED
	if before.ContactInfo != after.ContactInfo {
		changed = append(changed, "contact_info")
REDACTED
	if before.DocURL != after.DocURL {
		changed = append(changed, "doc_url")
REDACTED
	if before.HomeContent != after.HomeContent {
		changed = append(changed, "home_content")
REDACTED
	if before.HideCcsImportButton != after.HideCcsImportButton {
		changed = append(changed, "hide_ccs_import_button")
REDACTED
	if before.DefaultConcurrency != after.DefaultConcurrency {
		changed = append(changed, "default_concurrency")
REDACTED
	if before.DefaultBalance != after.DefaultBalance {
		changed = append(changed, "default_balance")
REDACTED
	if before.AffiliateRebateRate != after.AffiliateRebateRate {
		changed = append(changed, "affiliate_rebate_rate")
REDACTED
	if before.AffiliateRebateFreezeHours != after.AffiliateRebateFreezeHours {
		changed = append(changed, "affiliate_rebate_freeze_hours")
REDACTED
	if before.AffiliateRebateDurationDays != after.AffiliateRebateDurationDays {
		changed = append(changed, "affiliate_rebate_duration_days")
REDACTED
	if before.AffiliateRebatePerInviteeCap != after.AffiliateRebatePerInviteeCap {
		changed = append(changed, "affiliate_rebate_per_invitee_cap")
REDACTED
	if !equalDefaultSubscriptions(before.DefaultSubscriptions, after.DefaultSubscriptions) {
		changed = append(changed, "default_subscriptions")
REDACTED
	if before.EnableModelFallback != after.EnableModelFallback {
		changed = append(changed, "enable_model_fallback")
REDACTED
	if before.FallbackModelAnthropic != after.FallbackModelAnthropic {
		changed = append(changed, "fallback_model_anthropic")
REDACTED
	if before.FallbackModelOpenAI != after.FallbackModelOpenAI {
		changed = append(changed, "fallback_model_openai")
REDACTED
	if before.FallbackModelGemini != after.FallbackModelGemini {
		changed = append(changed, "fallback_model_gemini")
REDACTED
	if before.FallbackModelAntigravity != after.FallbackModelAntigravity {
		changed = append(changed, "fallback_model_antigravity")
REDACTED
	if before.EnableIdentityPatch != after.EnableIdentityPatch {
		changed = append(changed, "enable_identity_patch")
REDACTED
	if before.IdentityPatchPrompt != after.IdentityPatchPrompt {
		changed = append(changed, "identity_patch_prompt")
REDACTED
	if before.OpsMonitoringEnabled != after.OpsMonitoringEnabled {
		changed = append(changed, "ops_monitoring_enabled")
REDACTED
	if before.OpsRealtimeMonitoringEnabled != after.OpsRealtimeMonitoringEnabled {
		changed = append(changed, "ops_realtime_monitoring_enabled")
REDACTED
	if before.OpsQueryModeDefault != after.OpsQueryModeDefault {
		changed = append(changed, "ops_query_mode_default")
REDACTED
	if before.OpsMetricsIntervalSeconds != after.OpsMetricsIntervalSeconds {
		changed = append(changed, "ops_metrics_interval_seconds")
REDACTED
	if before.MinClaudeCodeVersion != after.MinClaudeCodeVersion {
		changed = append(changed, "min_claude_code_version")
REDACTED
	if before.MaxClaudeCodeVersion != after.MaxClaudeCodeVersion {
		changed = append(changed, "max_claude_code_version")
REDACTED
	if before.MinCodexVersion != after.MinCodexVersion {
		changed = append(changed, "min_codex_version")
REDACTED
	if before.MaxCodexVersion != after.MaxCodexVersion {
		changed = append(changed, "max_codex_version")
REDACTED
	if before.CodexCLIOnlyAllowAppServerClients != after.CodexCLIOnlyAllowAppServerClients {
		changed = append(changed, "codex_cli_only_allow_app_server_clients")
REDACTED
	if before.CodexCLIOnlyEngineFingerprintSignals != after.CodexCLIOnlyEngineFingerprintSignals {
		changed = append(changed, "codex_cli_only_engine_fingerprint_signals")
REDACTED
	if before.CodexCLIOnlyBlacklist != after.CodexCLIOnlyBlacklist {
		changed = append(changed, "codex_cli_only_blacklist")
REDACTED
	if before.CodexCLIOnlyWhitelist != after.CodexCLIOnlyWhitelist {
		changed = append(changed, "codex_cli_only_whitelist")
REDACTED
	if before.AllowUngroupedKeyScheduling != after.AllowUngroupedKeyScheduling {
		changed = append(changed, "allow_ungrouped_key_scheduling")
REDACTED
	if before.BackendModeEnabled != after.BackendModeEnabled {
		changed = append(changed, "backend_mode_enabled")
REDACTED
	if before.PurchaseSubscriptionEnabled != after.PurchaseSubscriptionEnabled {
		changed = append(changed, "purchase_subscription_enabled")
REDACTED
	if before.PurchaseSubscriptionURL != after.PurchaseSubscriptionURL {
		changed = append(changed, "purchase_subscription_url")
REDACTED
	if before.TableDefaultPageSize != after.TableDefaultPageSize {
		changed = append(changed, "table_default_page_size")
REDACTED
	if !equalIntSlice(before.TablePageSizeOptions, after.TablePageSizeOptions) {
		changed = append(changed, "table_page_size_options")
REDACTED
	if before.CustomMenuItems != after.CustomMenuItems {
		changed = append(changed, "custom_menu_items")
REDACTED
	if before.CustomEndpoints != after.CustomEndpoints {
		changed = append(changed, "custom_endpoints")
REDACTED
	if before.EnableFingerprintUnification != after.EnableFingerprintUnification {
		changed = append(changed, "enable_fingerprint_unification")
REDACTED
	if before.EnableMetadataPassthrough != after.EnableMetadataPassthrough {
		changed = append(changed, "enable_metadata_passthrough")
REDACTED
	if before.EnableCCHSigning != after.EnableCCHSigning {
		changed = append(changed, "enable_cch_signing")
REDACTED
	if before.EnableClaudeOAuthSystemPromptInjection != after.EnableClaudeOAuthSystemPromptInjection {
		changed = append(changed, "enable_claude_oauth_system_prompt_injection")
REDACTED
	if before.ClaudeOAuthSystemPrompt != after.ClaudeOAuthSystemPrompt {
		changed = append(changed, "claude_oauth_system_prompt")
REDACTED
	if before.ClaudeOAuthSystemPromptBlocks != after.ClaudeOAuthSystemPromptBlocks {
		changed = append(changed, "claude_oauth_system_prompt_blocks")
REDACTED
	if before.EnableAnthropicCacheTTL1hInjection != after.EnableAnthropicCacheTTL1hInjection {
		changed = append(changed, "enable_anthropic_cache_ttl_1h_injection")
REDACTED
	if before.RewriteMessageCacheControl != after.RewriteMessageCacheControl {
		changed = append(changed, "rewrite_message_cache_control")
REDACTED
	if before.EnableClientDatelineNormalization != after.EnableClientDatelineNormalization {
		changed = append(changed, "enable_client_dateline_normalization")
REDACTED
	if before.AntigravityUserAgentVersion != after.AntigravityUserAgentVersion {
		changed = append(changed, "antigravity_user_agent_version")
REDACTED
	if before.OpenAICodexUserAgent != after.OpenAICodexUserAgent {
		changed = append(changed, "openai_codex_user_agent")
REDACTED
	if before.PaymentVisibleMethodAlipaySource != after.PaymentVisibleMethodAlipaySource {
		changed = append(changed, "payment_visible_method_alipay_source")
REDACTED
	if before.PaymentVisibleMethodWxpaySource != after.PaymentVisibleMethodWxpaySource {
		changed = append(changed, "payment_visible_method_wxpay_source")
REDACTED
	if before.PaymentVisibleMethodAlipayEnabled != after.PaymentVisibleMethodAlipayEnabled {
		changed = append(changed, "payment_visible_method_alipay_enabled")
REDACTED
	if before.PaymentVisibleMethodWxpayEnabled != after.PaymentVisibleMethodWxpayEnabled {
		changed = append(changed, "payment_visible_method_wxpay_enabled")
REDACTED
	if before.OpenAIAdvancedSchedulerEnabled != after.OpenAIAdvancedSchedulerEnabled {
		changed = append(changed, "openai_advanced_scheduler_enabled")
REDACTED
	if before.OpenAIAdvancedSchedulerStickyWeightedEnabled != after.OpenAIAdvancedSchedulerStickyWeightedEnabled {
		changed = append(changed, "openai_advanced_scheduler_sticky_weighted_enabled")
REDACTED
	if before.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled != after.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled {
		changed = append(changed, "openai_advanced_scheduler_subscription_priority_enabled")
REDACTED
	if before.OpenAIAdvancedSchedulerLBTopK != after.OpenAIAdvancedSchedulerLBTopK {
		changed = append(changed, "openai_advanced_scheduler_lb_top_k")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightPriority != after.OpenAIAdvancedSchedulerWeightPriority {
		changed = append(changed, "openai_advanced_scheduler_weight_priority")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightLoad != after.OpenAIAdvancedSchedulerWeightLoad {
		changed = append(changed, "openai_advanced_scheduler_weight_load")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightQueue != after.OpenAIAdvancedSchedulerWeightQueue {
		changed = append(changed, "openai_advanced_scheduler_weight_queue")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightErrorRate != after.OpenAIAdvancedSchedulerWeightErrorRate {
		changed = append(changed, "openai_advanced_scheduler_weight_error_rate")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightTTFT != after.OpenAIAdvancedSchedulerWeightTTFT {
		changed = append(changed, "openai_advanced_scheduler_weight_ttft")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightReset != after.OpenAIAdvancedSchedulerWeightReset {
		changed = append(changed, "openai_advanced_scheduler_weight_reset")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightQuotaHeadroom != after.OpenAIAdvancedSchedulerWeightQuotaHeadroom {
		changed = append(changed, "openai_advanced_scheduler_weight_quota_headroom")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightPreviousResponse != after.OpenAIAdvancedSchedulerWeightPreviousResponse {
		changed = append(changed, "openai_advanced_scheduler_weight_previous_response")
REDACTED
	if before.OpenAIAdvancedSchedulerWeightSessionSticky != after.OpenAIAdvancedSchedulerWeightSessionSticky {
		changed = append(changed, "openai_advanced_scheduler_weight_session_sticky")
REDACTED
	// 余额、订阅到期与账号限额通知
	if before.BalanceLowNotifyEnabled != after.BalanceLowNotifyEnabled {
		changed = append(changed, "balance_low_notify_enabled")
REDACTED
	if before.BalanceLowNotifyThreshold != after.BalanceLowNotifyThreshold {
		changed = append(changed, "balance_low_notify_threshold")
REDACTED
	if before.BalanceLowNotifyRechargeURL != after.BalanceLowNotifyRechargeURL {
		changed = append(changed, "balance_low_notify_recharge_url")
REDACTED
	if before.SubscriptionExpiryNotifyEnabled != after.SubscriptionExpiryNotifyEnabled {
		changed = append(changed, "subscription_expiry_notify_enabled")
REDACTED
	if before.AccountQuotaNotifyEnabled != after.AccountQuotaNotifyEnabled {
		changed = append(changed, "account_quota_notify_enabled")
REDACTED
	if !equalNotifyEmailEntries(before.AccountQuotaNotifyEmails, after.AccountQuotaNotifyEmails) {
		changed = append(changed, "account_quota_notify_emails")
REDACTED
	if before.ChannelMonitorEnabled != after.ChannelMonitorEnabled {
		changed = append(changed, "channel_monitor_enabled")
REDACTED
	if before.ChannelMonitorDefaultIntervalSeconds != after.ChannelMonitorDefaultIntervalSeconds {
		changed = append(changed, "channel_monitor_default_interval_seconds")
REDACTED
	if before.AvailableChannelsEnabled != after.AvailableChannelsEnabled {
		changed = append(changed, "available_channels_enabled")
REDACTED
	if before.AffiliateEnabled != after.AffiliateEnabled {
		changed = append(changed, "affiliate_enabled")
REDACTED
	if before.RiskControlEnabled != after.RiskControlEnabled {
		changed = append(changed, "risk_control_enabled")
REDACTED
	if before.CyberSessionBlockEnabled != after.CyberSessionBlockEnabled {
		changed = append(changed, "cyber_session_block_enabled")
REDACTED
	if before.CyberSessionBlockTTLSeconds != after.CyberSessionBlockTTLSeconds {
		changed = append(changed, "cyber_session_block_ttl_seconds")
REDACTED
	// Default platform quotas（JSON map，整体比较）
	if !equalPlatformQuotaSettings(before.DefaultPlatformQuotas, after.DefaultPlatformQuotas) {
		changed = append(changed, service.SettingKeyDefaultPlatformQuotas)
REDACTED
	changed = appendAuthSourceDefaultChanges(changed, beforeAuthSourceDefaults, afterAuthSourceDefaults)
	return changed
REDACTED

func appendAuthSourceDefaultChanges(changed []string, before *service.AuthSourceDefaultSettings, after *service.AuthSourceDefaultSettings) []string {
	if before == nil {
		before = &service.AuthSourceDefaultSettings{REDACTED
REDACTED
	if after == nil {
		after = &service.AuthSourceDefaultSettings{REDACTED
REDACTED

	type providerDefaultGrantField struct {
		name   string
		before service.ProviderDefaultGrantSettings
		after  service.ProviderDefaultGrantSettings
REDACTED

	fields := []providerDefaultGrantField{
		{name: "email", before: before.Email, after: after.EmailREDACTED,
		{name: "linuxdo", before: before.LinuxDo, after: after.LinuxDoREDACTED,
		{name: "oidc", before: before.OIDC, after: after.OIDCREDACTED,
		{name: "wechat", before: before.WeChat, after: after.WeChatREDACTED,
		{name: "github", before: before.GitHub, after: after.GitHubREDACTED,
		{name: "google", before: before.Google, after: after.GoogleREDACTED,
		{name: "dingtalk", before: before.DingTalk, after: after.DingTalkREDACTED,
REDACTED
	for _, field := range fields {
		if field.before.Balance != field.after.Balance {
			changed = append(changed, "auth_source_default_"+field.name+"_balance")
	REDACTED
		if field.before.Concurrency != field.after.Concurrency {
			changed = append(changed, "auth_source_default_"+field.name+"_concurrency")
	REDACTED
		if !equalDefaultSubscriptions(field.before.Subscriptions, field.after.Subscriptions) {
			changed = append(changed, "auth_source_default_"+field.name+"_subscriptions")
	REDACTED
		if field.before.GrantOnSignup != field.after.GrantOnSignup {
			changed = append(changed, "auth_source_default_"+field.name+"_grant_on_signup")
	REDACTED
		if field.before.GrantOnFirstBind != field.after.GrantOnFirstBind {
			changed = append(changed, "auth_source_default_"+field.name+"_grant_on_first_bind")
	REDACTED
		// Platform quotas diff：整体替换语义，发单个 JSON key。
		if !equalPlatformQuotaSettings(field.before.PlatformQuotas, field.after.PlatformQuotas) {
			changed = append(changed, service.SettingKeyAuthSourcePlatformQuotas(field.name))
	REDACTED
REDACTED
	if before.ForceEmailOnThirdPartySignup != after.ForceEmailOnThirdPartySignup {
		changed = append(changed, "force_email_on_third_party_signup")
REDACTED
	return changed
REDACTED

func normalizeDefaultSubscriptions(input []dto.DefaultSubscriptionSetting) []dto.DefaultSubscriptionSetting {
	if len(input) == 0 {
		return nil
REDACTED
	normalized := make([]dto.DefaultSubscriptionSetting, 0, len(input))
	for _, item := range input {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
	REDACTED
		if item.ValidityDays > service.MaxValidityDays {
			item.ValidityDays = service.MaxValidityDays
	REDACTED
		normalized = append(normalized, item)
REDACTED
	return normalized
REDACTED

func normalizeOptionalDefaultSubscriptions(input *[]dto.DefaultSubscriptionSetting) *[]dto.DefaultSubscriptionSetting {
	if input == nil {
		return nil
REDACTED
	normalized := normalizeDefaultSubscriptions(*input)
	return &normalized
REDACTED

func float64ValueOrDefault(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED

func intValueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED

func boolValueOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED

func defaultSubscriptionsValueOrDefault(input *[]dto.DefaultSubscriptionSetting, fallback []service.DefaultSubscriptionSetting) []service.DefaultSubscriptionSetting {
	if input == nil {
		return fallback
REDACTED
	result := make([]service.DefaultSubscriptionSetting, 0, len(*input))
	for _, item := range *input {
		result = append(result, service.DefaultSubscriptionSetting{
			GroupID:      item.GroupID,
			ValidityDays: item.ValidityDays,
	REDACTED)
REDACTED
	return result
REDACTED

// platformQuotasValueOrDefault 处理 auth-source platform quota 的 nil 语义：
// nil = 请求未包含该字段（保留 fallback），non-nil（含 empty map）= 整体覆盖。
// 注意：JSON null 与字段省略等价——两者均反序列化为 nil map，因此都保留旧值；
// 若要清空某 source 的所有 quota 配置，须显式发空对象 {REDACTED。
func platformQuotasValueOrDefault(value, fallback map[string]*service.DefaultPlatformQuotaSetting) map[string]*service.DefaultPlatformQuotaSetting {
	if value == nil {
		return fallback
REDACTED
	return value
REDACTED

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i] != b[i] {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalDefaultSubscriptions(a, b []service.DefaultSubscriptionSetting) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i].GroupID != b[i].GroupID || a[i].ValidityDays != b[i].ValidityDays {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalLoginAgreementDocuments(a, b []service.LoginAgreementDocument) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Title != b[i].Title || a[i].ContentMD != b[i].ContentMD {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i] != b[i] {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func equalNotifyEmailEntries(a, b []service.NotifyEmailEntry) bool {
	if len(a) != len(b) {
		return false
REDACTED
	for i := range a {
		if a[i].Email != b[i].Email || a[i].Verified != b[i].Verified || a[i].Disabled != b[i].Disabled {
			return false
	REDACTED
REDACTED
	return true
REDACTED

// equalNullableFloat compares two *float64 values treating nil as a distinct case.
func equalNullableFloat(a, b *float64) bool {
	if a == nil && b == nil {
		return true
REDACTED
	if a == nil || b == nil {
		return false
REDACTED
	return *a == *b
REDACTED

// slotOf returns the *float64 for the given window from a DefaultPlatformQuotaSetting.
func slotOf(s *service.DefaultPlatformQuotaSetting, win string) *float64 {
	if s == nil {
		return nil
REDACTED
	switch win {
	case "daily":
		return s.DailyLimitUSD
	case "weekly":
		return s.WeeklyLimitUSD
	case "monthly":
		return s.MonthlyLimitUSD
REDACTED
	return nil
REDACTED

// equalPlatformQuotaSettings reports whether two platform-quota maps are identical across all allowed slots.
func equalPlatformQuotaSettings(before, after map[string]*service.DefaultPlatformQuotaSetting) bool {
	for _, platform := range service.AllowedQuotaPlatforms {
		b := before[platform]
		a := after[platform]
		if !equalNullableFloat(slotOf(b, "daily"), slotOf(a, "daily")) {
			return false
	REDACTED
		if !equalNullableFloat(slotOf(b, "weekly"), slotOf(a, "weekly")) {
			return false
	REDACTED
		if !equalNullableFloat(slotOf(b, "monthly"), slotOf(a, "monthly")) {
			return false
	REDACTED
REDACTED
	return true
REDACTED

func stringSetting(value *string, fallback string) string {
	if value == nil {
		return fallback
REDACTED
	return *value
REDACTED
