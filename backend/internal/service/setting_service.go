package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/imroc/req/v3"
	"golang.org/x/sync/singleflight"
)

var (
	ErrRegistrationDisabled   = infraerrors.Forbidden("REGISTRATION_DISABLED", "registration is currently disabled")
	ErrSettingNotFound        = infraerrors.NotFound("SETTING_NOT_FOUND", "setting not found")
	ErrDefaultSubGroupInvalid = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_INVALID",
		"default subscription group must exist and be subscription type",
	)
	ErrDefaultSubGroupDuplicate = infraerrors.BadRequest(
		"DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE",
		"default subscription group cannot be duplicated",
	)
)

type SettingRepository interface {
	Get(ctx context.Context, key string) (*Setting, error)
	GetValue(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetMultiple(ctx context.Context, keys []string) (map[string]string, error)
	SetMultiple(ctx context.Context, settings map[string]string) error
	GetAll(ctx context.Context) (map[string]string, error)
	Delete(ctx context.Context, key string) error
REDACTED

// cachedVersionBounds 缓存 Claude Code 版本号上下限（进程内缓存，60s TTL）
type cachedVersionBounds struct {
	min       string // 空字符串 = 不检查
	max       string // 空字符串 = 不检查
	expiresAt int64  // unix nano
REDACTED

// versionBoundsCache 版本号上下限进程内缓存
var versionBoundsCache atomic.Value // *cachedVersionBounds

// versionBoundsSF 防止缓存过期时 thundering herd
var versionBoundsSF singleflight.Group

// versionBoundsCacheTTL 缓存有效期
const versionBoundsCacheTTL = 60 * time.Second

// versionBoundsErrorTTL DB 错误时的短缓存，快速重试
const versionBoundsErrorTTL = 5 * time.Second

// versionBoundsDBTimeout singleflight 内 DB 查询超时，独立于请求 context
const versionBoundsDBTimeout = 5 * time.Second

// cachedBackendMode Backend Mode cache (in-process, 60s TTL)
type cachedBackendMode struct {
	value     bool
	expiresAt int64 // unix nano
REDACTED

var backendModeCache atomic.Value // *cachedBackendMode
var backendModeSF singleflight.Group

const backendModeCacheTTL = 60 * time.Second
const backendModeErrorTTL = 5 * time.Second
const backendModeDBTimeout = 5 * time.Second

// cachedGatewayForwardingSettings 缓存网关转发行为设置（进程内缓存，60s TTL）
type cachedGatewayForwardingSettings struct {
	fingerprintUnification       bool
	metadataPassthrough          bool
	cchSigning                   bool
	anthropicCacheTTL1hInjection bool
	expiresAt                    int64 // unix nano
REDACTED

var gatewayForwardingCache atomic.Value // *cachedGatewayForwardingSettings
var gatewayForwardingSF singleflight.Group

const gatewayForwardingCacheTTL = 60 * time.Second
const gatewayForwardingErrorTTL = 5 * time.Second
const gatewayForwardingDBTimeout = 5 * time.Second

// DefaultSubscriptionGroupReader validates group references used by default subscriptions.
type DefaultSubscriptionGroupReader interface {
	GetByID(ctx context.Context, id int64) (*Group, error)
REDACTED

// WebSearchManagerBuilder creates a websearch.Manager from config (injected by infra layer).
// proxyURLs maps proxy ID to resolved URL for provider-level proxy support.
type WebSearchManagerBuilder func(cfg *WebSearchEmulationConfig, proxyURLs map[int64]string)

// SettingService 系统设置服务
type SettingService struct {
	settingRepo             SettingRepository
	defaultSubGroupReader   DefaultSubscriptionGroupReader
	proxyRepo               ProxyRepository // for resolving websearch provider proxy URLs
	cfg                     *config.Config
	onUpdate                func() // Callback when settings are updated (for cache invalidation)
	version                 string // Application version
	webSearchManagerBuilder WebSearchManagerBuilder
REDACTED

type ProviderDefaultGrantSettings struct {
	Balance          float64
	Concurrency      int
	Subscriptions    []DefaultSubscriptionSetting
	GrantOnSignup    bool
	GrantOnFirstBind bool
REDACTED

type AuthSourceDefaultSettings struct {
	Email                        ProviderDefaultGrantSettings
	LinuxDo                      ProviderDefaultGrantSettings
	OIDC                         ProviderDefaultGrantSettings
	WeChat                       ProviderDefaultGrantSettings
	GitHub                       ProviderDefaultGrantSettings
	Google                       ProviderDefaultGrantSettings
	ForceEmailOnThirdPartySignup bool
REDACTED

type authSourceDefaultKeySet struct {
	balance          string
	concurrency      string
	subscriptions    string
	grantOnSignup    string
	grantOnFirstBind string
REDACTED

var (
	emailAuthSourceDefaultKeys = authSourceDefaultKeySet{
		balance:          SettingKeyAuthSourceDefaultEmailBalance,
		concurrency:      SettingKeyAuthSourceDefaultEmailConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultEmailSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultEmailGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultEmailGrantOnFirstBind,
REDACTED
	linuxDoAuthSourceDefaultKeys = authSourceDefaultKeySet{
		balance:          SettingKeyAuthSourceDefaultLinuxDoBalance,
		concurrency:      SettingKeyAuthSourceDefaultLinuxDoConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultLinuxDoSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind,
REDACTED
	oidcAuthSourceDefaultKeys = authSourceDefaultKeySet{
		balance:          SettingKeyAuthSourceDefaultOIDCBalance,
		concurrency:      SettingKeyAuthSourceDefaultOIDCConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultOIDCSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultOIDCGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind,
REDACTED
	weChatAuthSourceDefaultKeys = authSourceDefaultKeySet{
		balance:          SettingKeyAuthSourceDefaultWeChatBalance,
		concurrency:      SettingKeyAuthSourceDefaultWeChatConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultWeChatSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultWeChatGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind,
REDACTED
	gitHubAuthSourceDefaultKeys = authSourceDefaultKeySet{
		balance:          SettingKeyAuthSourceDefaultGitHubBalance,
		concurrency:      SettingKeyAuthSourceDefaultGitHubConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultGitHubSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultGitHubGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind,
REDACTED
	googleAuthSourceDefaultKeys = authSourceDefaultKeySet{
		balance:          SettingKeyAuthSourceDefaultGoogleBalance,
		concurrency:      SettingKeyAuthSourceDefaultGoogleConcurrency,
		subscriptions:    SettingKeyAuthSourceDefaultGoogleSubscriptions,
		grantOnSignup:    SettingKeyAuthSourceDefaultGoogleGrantOnSignup,
		grantOnFirstBind: SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind,
REDACTED
)

const (
	defaultAuthSourceBalance     = 0
	defaultAuthSourceConcurrency = 5
	defaultWeChatConnectMode     = "open"
	defaultWeChatConnectScopes   = "snsapi_login"
	defaultWeChatConnectFrontend = "/auth/wechat/callback"
	defaultGitHubOAuthAuthorize  = "https://github.com/login/oauth/authorize"
	defaultGitHubOAuthToken      = "https://github.com/login/oauth/access_token"
	defaultGitHubOAuthUserInfo   = "https://api.github.com/user"
	defaultGitHubOAuthEmails     = "https://api.github.com/user/emails"
	defaultGitHubOAuthScopes     = "read:user user:email"
	defaultGitHubOAuthFrontend   = "/auth/oauth/callback"
	defaultGoogleOAuthAuthorize  = "https://accounts.google.com/o/oauth2/v2/auth"
	defaultGoogleOAuthToken      = "https://oauth2.googleapis.com/token"
	defaultGoogleOAuthUserInfo   = "https://openidconnect.googleapis.com/v1/userinfo"
	defaultGoogleOAuthScopes     = "openid email profile"
	defaultGoogleOAuthFrontend   = "/auth/oauth/callback"
)

func normalizeWeChatConnectModeSetting(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "mp":
		return "mp"
	case "mobile":
		return "mobile"
	default:
		return "open"
REDACTED
REDACTED

func defaultWeChatConnectScopeForMode(mode string) string {
	switch normalizeWeChatConnectModeSetting(mode) {
	case "mp":
		return "snsapi_userinfo"
	case "mobile":
		return ""
REDACTED
	return defaultWeChatConnectScopes
REDACTED

func normalizeWeChatConnectScopeSetting(raw, mode string) string {
	switch normalizeWeChatConnectModeSetting(mode) {
	case "mp":
		switch strings.TrimSpace(raw) {
		case "snsapi_base":
			return "snsapi_base"
		case "snsapi_userinfo":
			return "snsapi_userinfo"
		default:
			return defaultWeChatConnectScopeForMode(mode)
	REDACTED
	case "mobile":
		return ""
	default:
		return defaultWeChatConnectScopes
REDACTED
REDACTED

func parseWeChatConnectCapabilitySettings(settings map[string]string, enabled bool, mode string) (bool, bool, bool) {
	mode = normalizeWeChatConnectModeSetting(mode)
	rawOpen, hasOpen := settings[SettingKeyWeChatConnectOpenEnabled]
	rawMP, hasMP := settings[SettingKeyWeChatConnectMPEnabled]
	rawMobile, hasMobile := settings[SettingKeyWeChatConnectMobileEnabled]
	openConfigured := hasOpen && strings.TrimSpace(rawOpen) != ""
	mpConfigured := hasMP && strings.TrimSpace(rawMP) != ""
	mobileConfigured := hasMobile && strings.TrimSpace(rawMobile) != ""

	if openConfigured || mpConfigured || mobileConfigured {
		openEnabled := strings.TrimSpace(rawOpen) == "true"
		mpEnabled := strings.TrimSpace(rawMP) == "true"
		mobileEnabled := strings.TrimSpace(rawMobile) == "true"
		return openEnabled, mpEnabled, mobileEnabled
REDACTED

	if !enabled {
		return false, false, false
REDACTED
	if mode == "mp" {
		return false, true, false
REDACTED
	if mode == "mobile" {
		return false, false, true
REDACTED
	return true, false, false
REDACTED

func normalizeWeChatConnectStoredMode(openEnabled, mpEnabled, mobileEnabled bool, mode string) string {
	mode = normalizeWeChatConnectModeSetting(mode)
	switch mode {
	case "open":
		if openEnabled {
			return "open"
	REDACTED
	case "mp":
		if mpEnabled {
			return "mp"
	REDACTED
	case "mobile":
		if mobileEnabled {
			return "mobile"
	REDACTED
REDACTED
	switch {
	case openEnabled:
		return "open"
	case mpEnabled:
		return "mp"
	case mobileEnabled:
		return "mobile"
	default:
		return mode
REDACTED
REDACTED

func mergeWeChatConnectCapabilitySettings(settings map[string]string, base config.WeChatConnectConfig, enabled bool, mode string) (bool, bool, bool) {
	mode = normalizeWeChatConnectModeSetting(firstNonEmpty(mode, base.Mode))
	rawOpen, hasOpen := settings[SettingKeyWeChatConnectOpenEnabled]
	rawMP, hasMP := settings[SettingKeyWeChatConnectMPEnabled]
	rawMobile, hasMobile := settings[SettingKeyWeChatConnectMobileEnabled]
	openConfigured := hasOpen && strings.TrimSpace(rawOpen) != ""
	mpConfigured := hasMP && strings.TrimSpace(rawMP) != ""
	mobileConfigured := hasMobile && strings.TrimSpace(rawMobile) != ""

	if openConfigured || mpConfigured || mobileConfigured {
		openEnabled := strings.TrimSpace(rawOpen) == "true"
		mpEnabled := strings.TrimSpace(rawMP) == "true"
		mobileEnabled := strings.TrimSpace(rawMobile) == "true"
		_, enabledConfigured := settings[SettingKeyWeChatConnectEnabled]
		if !enabledConfigured &&
			enabled &&
			!openEnabled &&
			!mpEnabled &&
			!mobileEnabled &&
			(base.OpenEnabled || base.MPEnabled || base.MobileEnabled) {
			return base.OpenEnabled, base.MPEnabled, base.MobileEnabled
	REDACTED
		return openEnabled, mpEnabled, mobileEnabled
REDACTED
	if !enabled {
		return false, false, false
REDACTED
	if base.OpenEnabled || base.MPEnabled || base.MobileEnabled {
		return base.OpenEnabled, base.MPEnabled, base.MobileEnabled
REDACTED
	return parseWeChatConnectCapabilitySettings(settings, enabled, mode)
REDACTED

func (s *SettingService) effectiveWeChatConnectOAuthConfig(settings map[string]string) WeChatConnectOAuthConfig {
	base := config.WeChatConnectConfig{REDACTED
	if s != nil && s.cfg != nil {
		base = s.cfg.WeChat
REDACTED

	enabled := base.Enabled
	if raw, ok := settings[SettingKeyWeChatConnectEnabled]; ok {
		enabled = strings.TrimSpace(raw) == "true"
REDACTED

	legacyAppID := strings.TrimSpace(firstNonEmpty(
		settings[SettingKeyWeChatConnectAppID],
		base.AppID,
		base.OpenAppID,
		base.MPAppID,
		base.MobileAppID,
	))
	legacyAppSecret := strings.TrimSpace(firstNonEmpty(
		settings[SettingKeyWeChatConnectAppSecret],
		base.AppSecret,
		base.OpenAppSecret,
		base.MPAppSecret,
		base.MobileAppSecret,
	))
	openAppID := strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectOpenAppID], base.OpenAppID, legacyAppID))
	openAppSecret := strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectOpenAppSecret], base.OpenAppSecret, legacyAppSecret))
	mpAppID := strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectMPAppID], base.MPAppID, legacyAppID))
	mpAppSecret := strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectMPAppSecret], base.MPAppSecret, legacyAppSecret))
	mobileAppID := strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectMobileAppID], base.MobileAppID, legacyAppID))
	mobileAppSecret := strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectMobileAppSecret], base.MobileAppSecret, legacyAppSecret))

	modeRaw := firstNonEmpty(settings[SettingKeyWeChatConnectMode], base.Mode)
	openEnabled, mpEnabled, mobileEnabled := mergeWeChatConnectCapabilitySettings(settings, base, enabled, modeRaw)
	mode := normalizeWeChatConnectStoredMode(openEnabled, mpEnabled, mobileEnabled, modeRaw)

	return WeChatConnectOAuthConfig{
		Enabled:             enabled,
		LegacyAppID:         legacyAppID,
		LegacyAppSecret:     legacyAppSecret,
		OpenAppID:           openAppID,
		OpenAppSecret:       openAppSecret,
		MPAppID:             mpAppID,
		MPAppSecret:         mpAppSecret,
		MobileAppID:         mobileAppID,
		MobileAppSecret:     mobileAppSecret,
		OpenEnabled:         openEnabled,
		MPEnabled:           mpEnabled,
		MobileEnabled:       mobileEnabled,
		Mode:                mode,
		Scopes:              normalizeWeChatConnectScopeSetting(firstNonEmpty(settings[SettingKeyWeChatConnectScopes], base.Scopes), mode),
		RedirectURL:         strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectRedirectURL], base.RedirectURL)),
		FrontendRedirectURL: strings.TrimSpace(firstNonEmpty(settings[SettingKeyWeChatConnectFrontendRedirectURL], base.FrontendRedirectURL, defaultWeChatConnectFrontend)),
REDACTED
REDACTED

// NewSettingService 创建系统设置服务实例
func NewSettingService(settingRepo SettingRepository, cfg *config.Config) *SettingService {
	return &SettingService{
		settingRepo: settingRepo,
		cfg:         cfg,
REDACTED
REDACTED

// SetDefaultSubscriptionGroupReader injects an optional group reader for default subscription validation.
func (s *SettingService) SetDefaultSubscriptionGroupReader(reader DefaultSubscriptionGroupReader) {
	s.defaultSubGroupReader = reader
REDACTED

// SetProxyRepository injects a proxy repo for resolving websearch provider proxy URLs.
func (s *SettingService) SetProxyRepository(repo ProxyRepository) {
	s.proxyRepo = repo
REDACTED

// GetAllSettings 获取所有系统设置
func (s *SettingService) GetAllSettings(ctx context.Context) (*SystemSettings, error) {
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("get all settings: %w", err)
REDACTED

	return s.parseSettings(settings), nil
REDACTED

// GetFrontendURL 获取前端基础URL（数据库优先，fallback 到配置文件）
func (s *SettingService) GetFrontendURL(ctx context.Context) string {
	val, err := s.settingRepo.GetValue(ctx, SettingKeyFrontendURL)
	if err == nil && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
REDACTED
	return s.cfg.Server.FrontendURL
REDACTED

// GetPublicSettings 获取公开设置（无需登录）
func (s *SettingService) GetPublicSettings(ctx context.Context) (*PublicSettings, error) {
	keys := []string{
		SettingKeyRegistrationEnabled,
		SettingKeyEmailVerifyEnabled,
		SettingKeyForceEmailOnThirdPartySignup,
		SettingKeyRegistrationEmailSuffixWhitelist,
		SettingKeyPromoCodeEnabled,
		SettingKeyPasswordResetEnabled,
		SettingKeyInvitationCodeEnabled,
		SettingKeyTotpEnabled,
		SettingKeyTurnstileEnabled,
		SettingKeyTurnstileSiteKey,
		SettingKeySiteName,
		SettingKeySiteLogo,
		SettingKeySiteSubtitle,
		SettingKeyAPIBaseURL,
		SettingKeyContactInfo,
		SettingKeyDocURL,
		SettingKeyHomeContent,
		SettingKeyHideCcsImportButton,
		SettingKeyPurchaseSubscriptionEnabled,
		SettingKeyPurchaseSubscriptionURL,
		SettingKeyTableDefaultPageSize,
		SettingKeyTablePageSizeOptions,
		SettingKeyCustomMenuItems,
		SettingKeyCustomEndpoints,
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyWeChatConnectEnabled,
		SettingKeyWeChatConnectAppID,
		SettingKeyWeChatConnectAppSecret,
		SettingKeyWeChatConnectOpenAppID,
		SettingKeyWeChatConnectOpenAppSecret,
		SettingKeyWeChatConnectMPAppID,
		SettingKeyWeChatConnectMPAppSecret,
		SettingKeyWeChatConnectMobileAppID,
		SettingKeyWeChatConnectMobileAppSecret,
		SettingKeyWeChatConnectOpenEnabled,
		SettingKeyWeChatConnectMPEnabled,
		SettingKeyWeChatConnectMobileEnabled,
		SettingKeyWeChatConnectMode,
		SettingKeyWeChatConnectScopes,
		SettingKeyWeChatConnectRedirectURL,
		SettingKeyWeChatConnectFrontendRedirectURL,
		SettingKeyBackendModeEnabled,
		SettingPaymentEnabled,
		SettingKeyOIDCConnectEnabled,
		SettingKeyOIDCConnectProviderName,
		SettingKeyGitHubOAuthEnabled,
		SettingKeyGitHubOAuthClientID,
		SettingKeyGitHubOAuthClientSecret,
		SettingKeyGoogleOAuthEnabled,
		SettingKeyGoogleOAuthClientID,
		SettingKeyGoogleOAuthClientSecret,
		SettingKeyBalanceLowNotifyEnabled,
		SettingKeyBalanceLowNotifyThreshold,
		SettingKeyBalanceLowNotifyRechargeURL,
		SettingKeyAccountQuotaNotifyEnabled,
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorDefaultIntervalSeconds,
		SettingKeyAvailableChannelsEnabled,
		SettingKeyAffiliateEnabled,
REDACTED

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get public settings: %w", err)
REDACTED

	linuxDoEnabled := false
	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		linuxDoEnabled = raw == "true"
REDACTED else {
		linuxDoEnabled = s.cfg != nil && s.cfg.LinuxDo.Enabled
REDACTED
	oidcEnabled := false
	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		oidcEnabled = raw == "true"
REDACTED else {
		oidcEnabled = s.cfg != nil && s.cfg.OIDC.Enabled
REDACTED
	oidcProviderName := strings.TrimSpace(settings[SettingKeyOIDCConnectProviderName])
	if oidcProviderName == "" && s.cfg != nil {
		oidcProviderName = strings.TrimSpace(s.cfg.OIDC.ProviderName)
REDACTED
	if oidcProviderName == "" {
		oidcProviderName = "OIDC"
REDACTED
	gitHubEnabled := s.emailOAuthPublicEnabled(settings, "github")
	googleEnabled := s.emailOAuthPublicEnabled(settings, "google")
	weChatEnabled, weChatOpenEnabled, weChatMPEnabled, weChatMobileEnabled := s.weChatOAuthCapabilitiesFromSettings(settings)

	// Password reset requires email verification to be enabled
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	passwordResetEnabled := emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true"
	registrationEmailSuffixWhitelist := ParseRegistrationEmailSuffixWhitelist(
		settings[SettingKeyRegistrationEmailSuffixWhitelist],
	)
	tableDefaultPageSize, tablePageSizeOptions := parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)

	var balanceLowNotifyThreshold float64
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		balanceLowNotifyThreshold = v
REDACTED

	return &PublicSettings{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		ForceEmailOnThirdPartySignup:     settings[SettingKeyForceEmailOnThirdPartySignup] == "true",
		RegistrationEmailSuffixWhitelist: registrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:             passwordResetEnabled,
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		TurnstileEnabled:                 settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                 settings[SettingKeyTurnstileSiteKey],
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
		TableDefaultPageSize:             tableDefaultPageSize,
		TablePageSizeOptions:             tablePageSizeOptions,
		CustomMenuItems:                  settings[SettingKeyCustomMenuItems],
		CustomEndpoints:                  settings[SettingKeyCustomEndpoints],
		LinuxDoOAuthEnabled:              linuxDoEnabled,
		WeChatOAuthEnabled:               weChatEnabled,
		WeChatOAuthOpenEnabled:           weChatOpenEnabled,
		WeChatOAuthMPEnabled:             weChatMPEnabled,
		WeChatOAuthMobileEnabled:         weChatMobileEnabled,
		BackendModeEnabled:               settings[SettingKeyBackendModeEnabled] == "true",
		PaymentEnabled:                   settings[SettingPaymentEnabled] == "true",
		OIDCOAuthEnabled:                 oidcEnabled,
		OIDCOAuthProviderName:            oidcProviderName,
		GitHubOAuthEnabled:               gitHubEnabled,
		GoogleOAuthEnabled:               googleEnabled,
		BalanceLowNotifyEnabled:          settings[SettingKeyBalanceLowNotifyEnabled] == "true",
		AccountQuotaNotifyEnabled:        settings[SettingKeyAccountQuotaNotifyEnabled] == "true",
		BalanceLowNotifyThreshold:        balanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings[SettingKeyBalanceLowNotifyRechargeURL],

		ChannelMonitorEnabled:                !isFalseSettingValue(settings[SettingKeyChannelMonitorEnabled]),
		ChannelMonitorDefaultIntervalSeconds: parseChannelMonitorInterval(settings[SettingKeyChannelMonitorDefaultIntervalSeconds]),

		AvailableChannelsEnabled: settings[SettingKeyAvailableChannelsEnabled] == "true",

		AffiliateEnabled: settings[SettingKeyAffiliateEnabled] == "true",
REDACTED, nil
REDACTED

// channelMonitorIntervalMin / channelMonitorIntervalMax bound the default interval
// (mirrors the monitor-level constraint but lives here so setting_service stays decoupled).
const (
	channelMonitorIntervalMin      = 15
	channelMonitorIntervalMax      = 3600
	channelMonitorIntervalFallback = 60
)

// parseChannelMonitorInterval parses the stored string and clamps to [15, 3600].
// Empty / invalid input falls back to channelMonitorIntervalFallback.
func parseChannelMonitorInterval(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return channelMonitorIntervalFallback
REDACTED
	return clampChannelMonitorInterval(v)
REDACTED

// clampChannelMonitorInterval clamps v to the allowed range. 0 means "not provided".
func clampChannelMonitorInterval(v int) int {
	if v <= 0 {
		return 0
REDACTED
	if v < channelMonitorIntervalMin {
		return channelMonitorIntervalMin
REDACTED
	if v > channelMonitorIntervalMax {
		return channelMonitorIntervalMax
REDACTED
	return v
REDACTED

// ChannelMonitorRuntime is the lightweight view of the channel monitor feature
// consumed by the runner and user-facing handlers.
type ChannelMonitorRuntime struct {
	Enabled                bool
	DefaultIntervalSeconds int
REDACTED

// GetChannelMonitorRuntime reads the channel monitor feature flags directly from
// the settings store. Fail-open: on error returns Enabled=true with the default interval.
func (s *SettingService) GetChannelMonitorRuntime(ctx context.Context) ChannelMonitorRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyChannelMonitorEnabled,
		SettingKeyChannelMonitorDefaultIntervalSeconds,
REDACTED)
	if err != nil {
		return ChannelMonitorRuntime{Enabled: true, DefaultIntervalSeconds: channelMonitorIntervalFallbackREDACTED
REDACTED
	return ChannelMonitorRuntime{
		Enabled:                !isFalseSettingValue(vals[SettingKeyChannelMonitorEnabled]),
		DefaultIntervalSeconds: parseChannelMonitorInterval(vals[SettingKeyChannelMonitorDefaultIntervalSeconds]),
REDACTED
REDACTED

// AvailableChannelsRuntime is the lightweight view of the available-channels feature
// switch consumed by the user-facing handler.
type AvailableChannelsRuntime struct {
	Enabled bool
REDACTED

// GetAvailableChannelsRuntime reads the available-channels feature switch directly
// from the settings store. Fail-closed: on error returns Enabled=false, matching
// the opt-in default (unknown ↔ disabled).
func (s *SettingService) GetAvailableChannelsRuntime(ctx context.Context) AvailableChannelsRuntime {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeyAvailableChannelsEnabledREDACTED)
	if err != nil {
		return AvailableChannelsRuntime{Enabled: falseREDACTED
REDACTED
	return AvailableChannelsRuntime{
		Enabled: vals[SettingKeyAvailableChannelsEnabled] == "true",
REDACTED
REDACTED

// SetOnUpdateCallback sets a callback function to be called when settings are updated
// This is used for cache invalidation (e.g., HTML cache in frontend server)
func (s *SettingService) SetOnUpdateCallback(callback func()) {
	s.onUpdate = callback
REDACTED

// SetVersion sets the application version for injection into public settings
func (s *SettingService) SetVersion(version string) {
	s.version = version
REDACTED

// PublicSettingsInjectionPayload is the JSON shape embedded into HTML as
// `window.__APP_CONFIG__` so the frontend can hydrate feature flags & site
// config before the first XHR finishes.
//
// INVARIANT: every `json` tag here MUST also exist on handler/dto.PublicSettings.
// If you forget a feature-flag field here, the frontend's
// `cachedPublicSettings.xxx_enabled` will be `undefined` on refresh until the
// async `/api/v1/settings/public` call returns — which causes opt-in menus
// (strict `=== true`) to flicker off/on. See
// frontend/src/utils/featureFlags.ts for the matching registry.
//
// A unit test diffs this struct's JSON keys against dto.PublicSettings to catch
// drift automatically (see setting_service_injection_test.go).
type PublicSettingsInjectionPayload struct {
	RegistrationEnabled              bool            `json:"registration_enabled"`
	EmailVerifyEnabled               bool            `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string        `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool            `json:"promo_code_enabled"`
	PasswordResetEnabled             bool            `json:"password_reset_enabled"`
	InvitationCodeEnabled            bool            `json:"invitation_code_enabled"`
	TotpEnabled                      bool            `json:"totp_enabled"`
	TurnstileEnabled                 bool            `json:"turnstile_enabled"`
	TurnstileSiteKey                 string          `json:"turnstile_site_key"`
	SiteName                         string          `json:"site_name"`
	SiteLogo                         string          `json:"site_logo"`
	SiteSubtitle                     string          `json:"site_subtitle"`
	APIBaseURL                       string          `json:"api_base_url"`
	ContactInfo                      string          `json:"contact_info"`
	DocURL                           string          `json:"doc_url"`
	HomeContent                      string          `json:"home_content"`
	HideCcsImportButton              bool            `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled      bool            `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL          string          `json:"purchase_subscription_url"`
	TableDefaultPageSize             int             `json:"table_default_page_size"`
	TablePageSizeOptions             []int           `json:"table_page_size_options"`
	CustomMenuItems                  json.RawMessage `json:"custom_menu_items"`
	CustomEndpoints                  json.RawMessage `json:"custom_endpoints"`
	LinuxDoOAuthEnabled              bool            `json:"linuxdo_oauth_enabled"`
	WeChatOAuthEnabled               bool            `json:"wechat_oauth_enabled"`
	WeChatOAuthOpenEnabled           bool            `json:"wechat_oauth_open_enabled"`
	WeChatOAuthMPEnabled             bool            `json:"wechat_oauth_mp_enabled"`
	WeChatOAuthMobileEnabled         bool            `json:"wechat_oauth_mobile_enabled"`
	OIDCOAuthEnabled                 bool            `json:"oidc_oauth_enabled"`
	OIDCOAuthProviderName            string          `json:"oidc_oauth_provider_name"`
	GitHubOAuthEnabled               bool            `json:"github_oauth_enabled"`
	GoogleOAuthEnabled               bool            `json:"google_oauth_enabled"`
	BackendModeEnabled               bool            `json:"backend_mode_enabled"`
	PaymentEnabled                   bool            `json:"payment_enabled"`
	Version                          string          `json:"version"`
	BalanceLowNotifyEnabled          bool            `json:"balance_low_notify_enabled"`
	AccountQuotaNotifyEnabled        bool            `json:"account_quota_notify_enabled"`
	BalanceLowNotifyThreshold        float64         `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL      string          `json:"balance_low_notify_recharge_url"`

	// Feature flags — MUST match the opt-in/opt-out registry in
	// frontend/src/utils/featureFlags.ts. Missing a field here is the bug
	// that hid the "可用渠道" menu on page refresh.
	ChannelMonitorEnabled                bool `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds int  `json:"channel_monitor_default_interval_seconds"`
	AvailableChannelsEnabled             bool `json:"available_channels_enabled"`
	AffiliateEnabled                     bool `json:"affiliate_enabled"`
REDACTED

// GetPublicSettingsForInjection returns public settings in a format suitable for HTML injection.
// This implements the web.PublicSettingsProvider interface.
func (s *SettingService) GetPublicSettingsForInjection(ctx context.Context) (any, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
REDACTED

	return &PublicSettingsInjectionPayload{
		RegistrationEnabled:              settings.RegistrationEnabled,
		EmailVerifyEnabled:               settings.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings.PromoCodeEnabled,
		PasswordResetEnabled:             settings.PasswordResetEnabled,
		InvitationCodeEnabled:            settings.InvitationCodeEnabled,
		TotpEnabled:                      settings.TotpEnabled,
		TurnstileEnabled:                 settings.TurnstileEnabled,
		TurnstileSiteKey:                 settings.TurnstileSiteKey,
		SiteName:                         settings.SiteName,
		SiteLogo:                         settings.SiteLogo,
		SiteSubtitle:                     settings.SiteSubtitle,
		APIBaseURL:                       settings.APIBaseURL,
		ContactInfo:                      settings.ContactInfo,
		DocURL:                           settings.DocURL,
		HomeContent:                      settings.HomeContent,
		HideCcsImportButton:              settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:          settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:             settings.TableDefaultPageSize,
		TablePageSizeOptions:             settings.TablePageSizeOptions,
		CustomMenuItems:                  filterUserVisibleMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                  safeRawJSONArray(settings.CustomEndpoints),
		LinuxDoOAuthEnabled:              settings.LinuxDoOAuthEnabled,
		WeChatOAuthEnabled:               settings.WeChatOAuthEnabled,
		WeChatOAuthOpenEnabled:           settings.WeChatOAuthOpenEnabled,
		WeChatOAuthMPEnabled:             settings.WeChatOAuthMPEnabled,
		WeChatOAuthMobileEnabled:         settings.WeChatOAuthMobileEnabled,
		OIDCOAuthEnabled:                 settings.OIDCOAuthEnabled,
		OIDCOAuthProviderName:            settings.OIDCOAuthProviderName,
		GitHubOAuthEnabled:               settings.GitHubOAuthEnabled,
		GoogleOAuthEnabled:               settings.GoogleOAuthEnabled,
		BackendModeEnabled:               settings.BackendModeEnabled,
		PaymentEnabled:                   settings.PaymentEnabled,
		Version:                          s.version,
		BalanceLowNotifyEnabled:          settings.BalanceLowNotifyEnabled,
		AccountQuotaNotifyEnabled:        settings.AccountQuotaNotifyEnabled,
		BalanceLowNotifyThreshold:        settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings.BalanceLowNotifyRechargeURL,

		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,
		AvailableChannelsEnabled:             settings.AvailableChannelsEnabled,
		AffiliateEnabled:                     settings.AffiliateEnabled,
REDACTED, nil
REDACTED

func DefaultWeChatConnectScopesForMode(mode string) string {
	return defaultWeChatConnectScopeForMode(mode)
REDACTED

func (s *SettingService) parseWeChatConnectOAuthConfig(settings map[string]string) (WeChatConnectOAuthConfig, error) {
	cfg := s.effectiveWeChatConnectOAuthConfig(settings)

	if !cfg.Enabled || (!cfg.OpenEnabled && !cfg.MPEnabled) {
		return WeChatConnectOAuthConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "wechat oauth is disabled")
REDACTED
	if cfg.OpenEnabled {
		if cfg.AppIDForMode("open") == "" {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth pc app id not configured")
	REDACTED
		if cfg.AppSecretForMode("open") == "" {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth pc app secret not configured")
	REDACTED
REDACTED
	if cfg.MPEnabled {
		if cfg.AppIDForMode("mp") == "" {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth official account app id not configured")
	REDACTED
		if cfg.AppSecretForMode("mp") == "" {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth official account app secret not configured")
	REDACTED
REDACTED
	if cfg.MobileEnabled {
		if cfg.AppIDForMode("mobile") == "" {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth mobile app id not configured")
	REDACTED
		if cfg.AppSecretForMode("mobile") == "" {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth mobile app secret not configured")
	REDACTED
REDACTED
	if v := strings.TrimSpace(cfg.RedirectURL); v != "" {
		if err := config.ValidateAbsoluteHTTPURL(v); err != nil {
			return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth redirect url invalid")
	REDACTED
REDACTED
	if err := config.ValidateFrontendRedirectURL(cfg.FrontendRedirectURL); err != nil {
		return WeChatConnectOAuthConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "wechat oauth frontend redirect url invalid")
REDACTED
	return cfg, nil
REDACTED

func (s *SettingService) weChatOAuthCapabilitiesFromSettings(settings map[string]string) (bool, bool, bool, bool) {
	cfg := s.effectiveWeChatConnectOAuthConfig(settings)
	if !cfg.Enabled {
		return false, false, false, false
REDACTED

	openReady := cfg.OpenEnabled && cfg.AppIDForMode("open") != "" && cfg.AppSecretForMode("open") != ""
	mpReady := cfg.MPEnabled && cfg.AppIDForMode("mp") != "" && cfg.AppSecretForMode("mp") != ""
	mobileReady := cfg.MobileEnabled && cfg.AppIDForMode("mobile") != "" && cfg.AppSecretForMode("mobile") != ""

	return openReady || mpReady, openReady, mpReady, mobileReady
REDACTED

func (s *SettingService) emailOAuthBaseConfig(provider string) config.EmailOAuthProviderConfig {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		cfg := config.EmailOAuthProviderConfig{
			AuthorizeURL:        defaultGitHubOAuthAuthorize,
			TokenURL:            defaultGitHubOAuthToken,
			UserInfoURL:         defaultGitHubOAuthUserInfo,
			EmailsURL:           defaultGitHubOAuthEmails,
			Scopes:              defaultGitHubOAuthScopes,
			FrontendRedirectURL: defaultGitHubOAuthFrontend,
	REDACTED
		if s != nil && s.cfg != nil {
			cfg = mergeEmailOAuthBaseConfig(cfg, s.cfg.GitHubOAuth)
	REDACTED
		return cfg
	case "google":
		cfg := config.EmailOAuthProviderConfig{
			AuthorizeURL:        defaultGoogleOAuthAuthorize,
			TokenURL:            defaultGoogleOAuthToken,
			UserInfoURL:         defaultGoogleOAuthUserInfo,
			Scopes:              defaultGoogleOAuthScopes,
			FrontendRedirectURL: defaultGoogleOAuthFrontend,
	REDACTED
		if s != nil && s.cfg != nil {
			cfg = mergeEmailOAuthBaseConfig(cfg, s.cfg.GoogleOAuth)
	REDACTED
		return cfg
	default:
		return config.EmailOAuthProviderConfig{REDACTED
REDACTED
REDACTED

func mergeEmailOAuthBaseConfig(base, override config.EmailOAuthProviderConfig) config.EmailOAuthProviderConfig {
	base.Enabled = override.Enabled
	if strings.TrimSpace(override.ClientID) != "" {
		base.ClientID = strings.TrimSpace(override.ClientID)
REDACTED
	if strings.TrimSpace(override.ClientSecret) != "" {
		base.ClientSecret = strings.TrimSpace(override.ClientSecret)
REDACTED
	if strings.TrimSpace(override.AuthorizeURL) != "" {
		base.AuthorizeURL = strings.TrimSpace(override.AuthorizeURL)
REDACTED
	if strings.TrimSpace(override.TokenURL) != "" {
		base.TokenURL = strings.TrimSpace(override.TokenURL)
REDACTED
	if strings.TrimSpace(override.UserInfoURL) != "" {
		base.UserInfoURL = strings.TrimSpace(override.UserInfoURL)
REDACTED
	if strings.TrimSpace(override.EmailsURL) != "" {
		base.EmailsURL = strings.TrimSpace(override.EmailsURL)
REDACTED
	if strings.TrimSpace(override.Scopes) != "" {
		base.Scopes = strings.TrimSpace(override.Scopes)
REDACTED
	if strings.TrimSpace(override.RedirectURL) != "" {
		base.RedirectURL = strings.TrimSpace(override.RedirectURL)
REDACTED
	if strings.TrimSpace(override.FrontendRedirectURL) != "" {
		base.FrontendRedirectURL = strings.TrimSpace(override.FrontendRedirectURL)
REDACTED
	return base
REDACTED

func (s *SettingService) emailOAuthPublicEnabled(settings map[string]string, provider string) bool {
	cfg := s.effectiveEmailOAuthConfig(settings, provider)
	return cfg.Enabled && strings.TrimSpace(cfg.ClientID) != "" && strings.TrimSpace(cfg.ClientSecret) != ""
REDACTED

func (s *SettingService) effectiveEmailOAuthConfig(settings map[string]string, provider string) config.EmailOAuthProviderConfig {
	cfg := s.emailOAuthBaseConfig(provider)
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "github":
		if raw, ok := settings[SettingKeyGitHubOAuthEnabled]; ok {
			cfg.Enabled = raw == "true"
	REDACTED
		cfg.ClientID = firstNonEmpty(settings[SettingKeyGitHubOAuthClientID], cfg.ClientID)
		cfg.ClientSecret = firstNonEmpty(settings[SettingKeyGitHubOAuthClientSecret], cfg.ClientSecret)
		cfg.RedirectURL = firstNonEmpty(settings[SettingKeyGitHubOAuthRedirectURL], cfg.RedirectURL)
		cfg.FrontendRedirectURL = firstNonEmpty(settings[SettingKeyGitHubOAuthFrontendRedirectURL], cfg.FrontendRedirectURL, defaultGitHubOAuthFrontend)
	case "google":
		if raw, ok := settings[SettingKeyGoogleOAuthEnabled]; ok {
			cfg.Enabled = raw == "true"
	REDACTED
		cfg.ClientID = firstNonEmpty(settings[SettingKeyGoogleOAuthClientID], cfg.ClientID)
		cfg.ClientSecret = firstNonEmpty(settings[SettingKeyGoogleOAuthClientSecret], cfg.ClientSecret)
		cfg.RedirectURL = firstNonEmpty(settings[SettingKeyGoogleOAuthRedirectURL], cfg.RedirectURL)
		cfg.FrontendRedirectURL = firstNonEmpty(settings[SettingKeyGoogleOAuthFrontendRedirectURL], cfg.FrontendRedirectURL, defaultGoogleOAuthFrontend)
REDACTED
	return cfg
REDACTED

// filterUserVisibleMenuItems filters out admin-only menu items from a raw JSON
// array string, returning only items with visibility != "admin".
func filterUserVisibleMenuItems(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return json.RawMessage("[]")
REDACTED
	var items []struct {
		Visibility string `json:"visibility"`
REDACTED
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return json.RawMessage("[]")
REDACTED

	// Parse full items to preserve all fields
	var fullItems []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fullItems); err != nil {
		return json.RawMessage("[]")
REDACTED

	var filtered []json.RawMessage
	for i, item := range items {
		if item.Visibility != "admin" {
			filtered = append(filtered, fullItems[i])
	REDACTED
REDACTED
	if len(filtered) == 0 {
		return json.RawMessage("[]")
REDACTED
	result, err := json.Marshal(filtered)
	if err != nil {
		return json.RawMessage("[]")
REDACTED
	return result
REDACTED

// safeRawJSONArray returns raw as json.RawMessage if it's valid JSON, otherwise "[]".
func safeRawJSONArray(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return json.RawMessage("[]")
REDACTED
	if json.Valid([]byte(raw)) {
		return json.RawMessage(raw)
REDACTED
	return json.RawMessage("[]")
REDACTED

// GetFrameSrcOrigins returns deduplicated http(s) origins from home_content URL,
// purchase_subscription_url, and all custom_menu_items URLs. Used by the router layer for CSP frame-src injection.
func (s *SettingService) GetFrameSrcOrigins(ctx context.Context) ([]string, error) {
	settings, err := s.GetPublicSettings(ctx)
	if err != nil {
		return nil, err
REDACTED

	seen := make(map[string]struct{REDACTED)
	var origins []string

	addOrigin := func(rawURL string) {
		if origin := extractOriginFromURL(rawURL); origin != "" {
			if _, ok := seen[origin]; !ok {
				seen[origin] = struct{REDACTED{REDACTED
				origins = append(origins, origin)
		REDACTED
	REDACTED
REDACTED

	// home content URL (when home_content is set to a URL for iframe embedding)
	addOrigin(settings.HomeContent)

	// purchase subscription URL
	if settings.PurchaseSubscriptionEnabled {
		addOrigin(settings.PurchaseSubscriptionURL)
REDACTED

	// all custom menu items (including admin-only, since CSP must allow all iframes)
	for _, item := range parseCustomMenuItemURLs(settings.CustomMenuItems) {
		addOrigin(item)
REDACTED

	return origins, nil
REDACTED

// extractOriginFromURL returns the scheme+host origin from rawURL.
// Only http and https schemes are accepted.
func extractOriginFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
REDACTED
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
REDACTED
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
REDACTED
	return u.Scheme + "://" + u.Host
REDACTED

// parseCustomMenuItemURLs extracts URLs from a raw JSON array of custom menu items.
func parseCustomMenuItemURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
REDACTED
	var items []struct {
		URL string `json:"url"`
REDACTED
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
REDACTED
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.URL != "" {
			urls = append(urls, item.URL)
	REDACTED
REDACTED
	return urls
REDACTED

func oidcUsePKCECompatibilityDefault(base config.OIDCConnectConfig) bool {
	if base.UsePKCEExplicit {
		return base.UsePKCE
REDACTED
	return true
REDACTED

func oidcValidateIDTokenCompatibilityDefault(base config.OIDCConnectConfig) bool {
	if base.ValidateIDTokenExplicit {
		return base.ValidateIDToken
REDACTED
	return true
REDACTED

func oidcCompatibilityWriteDefault(base config.OIDCConnectConfig, configured bool, raw string, explicit bool, explicitValue bool) bool {
	if configured {
		return strings.TrimSpace(raw) == "true"
REDACTED
	if explicit {
		return explicitValue
REDACTED
	return false
REDACTED

// UpdateSettings 更新系统设置
func (s *SettingService) UpdateSettings(ctx context.Context, settings *SystemSettings) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
REDACTED

	err = s.settingRepo.SetMultiple(ctx, updates)
	if err == nil {
		s.refreshCachedSettings(settings)
REDACTED
	return err
REDACTED

func (s *SettingService) OIDCSecurityWriteDefaults(ctx context.Context) (bool, bool, error) {
	rawSettings, err := s.settingRepo.GetMultiple(ctx, []string{
		SettingKeyOIDCConnectUsePKCE,
		SettingKeyOIDCConnectValidateIDToken,
REDACTED)
	if err != nil {
		return false, false, fmt.Errorf("get oidc security write defaults: %w", err)
REDACTED

	base := config.OIDCConnectConfig{REDACTED
	if s != nil && s.cfg != nil {
		base = s.cfg.OIDC
REDACTED

	rawUsePKCE, hasUsePKCE := rawSettings[SettingKeyOIDCConnectUsePKCE]
	rawValidateIDToken, hasValidateIDToken := rawSettings[SettingKeyOIDCConnectValidateIDToken]

	return oidcCompatibilityWriteDefault(base, hasUsePKCE, rawUsePKCE, base.UsePKCEExplicit, base.UsePKCE),
		oidcCompatibilityWriteDefault(base, hasValidateIDToken, rawValidateIDToken, base.ValidateIDTokenExplicit, base.ValidateIDToken),
		nil
REDACTED

// UpdateSettingsWithAuthSourceDefaults persists system settings and auth-source defaults in a single write.
func (s *SettingService) UpdateSettingsWithAuthSourceDefaults(ctx context.Context, settings *SystemSettings, authDefaults *AuthSourceDefaultSettings) error {
	updates, err := s.buildSystemSettingsUpdates(ctx, settings)
	if err != nil {
		return err
REDACTED

	authSourceUpdates, err := s.buildAuthSourceDefaultUpdates(ctx, authDefaults)
	if err != nil {
		return err
REDACTED
	for key, value := range authSourceUpdates {
		updates[key] = value
REDACTED

	err = s.settingRepo.SetMultiple(ctx, updates)
	if err == nil {
		s.refreshCachedSettings(settings)
REDACTED
	return err
REDACTED

func (s *SettingService) buildSystemSettingsUpdates(ctx context.Context, settings *SystemSettings) (map[string]string, error) {
	if err := s.validateDefaultSubscriptionGroups(ctx, settings.DefaultSubscriptions); err != nil {
		return nil, err
REDACTED
	normalizedWhitelist, err := NormalizeRegistrationEmailSuffixWhitelist(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", err.Error())
REDACTED
	if normalizedWhitelist == nil {
		normalizedWhitelist = []string{REDACTED
REDACTED
	settings.RegistrationEmailSuffixWhitelist = normalizedWhitelist
	alipaySource, err := normalizeVisibleMethodSettingSource("alipay", settings.PaymentVisibleMethodAlipaySource, settings.PaymentVisibleMethodAlipayEnabled)
	if err != nil {
		return nil, err
REDACTED
	wxpaySource, err := normalizeVisibleMethodSettingSource("wxpay", settings.PaymentVisibleMethodWxpaySource, settings.PaymentVisibleMethodWxpayEnabled)
	if err != nil {
		return nil, err
REDACTED
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
REDACTED
	settings.GitHubOAuthRedirectURL = strings.TrimSpace(settings.GitHubOAuthRedirectURL)
	settings.GitHubOAuthFrontendRedirectURL = strings.TrimSpace(settings.GitHubOAuthFrontendRedirectURL)
	if settings.GitHubOAuthFrontendRedirectURL == "" {
		settings.GitHubOAuthFrontendRedirectURL = defaultGitHubOAuthFrontend
REDACTED
	settings.GoogleOAuthRedirectURL = strings.TrimSpace(settings.GoogleOAuthRedirectURL)
	settings.GoogleOAuthFrontendRedirectURL = strings.TrimSpace(settings.GoogleOAuthFrontendRedirectURL)
	if settings.GoogleOAuthFrontendRedirectURL == "" {
		settings.GoogleOAuthFrontendRedirectURL = defaultGoogleOAuthFrontend
REDACTED

	updates := make(map[string]string)

	// 注册设置
	updates[SettingKeyRegistrationEnabled] = strconv.FormatBool(settings.RegistrationEnabled)
	updates[SettingKeyEmailVerifyEnabled] = strconv.FormatBool(settings.EmailVerifyEnabled)
	registrationEmailSuffixWhitelistJSON, err := json.Marshal(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return nil, fmt.Errorf("marshal registration email suffix whitelist: %w", err)
REDACTED
	updates[SettingKeyRegistrationEmailSuffixWhitelist] = string(registrationEmailSuffixWhitelistJSON)
	updates[SettingKeyPromoCodeEnabled] = strconv.FormatBool(settings.PromoCodeEnabled)
	updates[SettingKeyPasswordResetEnabled] = strconv.FormatBool(settings.PasswordResetEnabled)
	updates[SettingKeyFrontendURL] = settings.FrontendURL
	updates[SettingKeyInvitationCodeEnabled] = strconv.FormatBool(settings.InvitationCodeEnabled)
	updates[SettingKeyTotpEnabled] = strconv.FormatBool(settings.TotpEnabled)

	// 邮件服务设置（只有非空才更新密码）
	updates[SettingKeySMTPHost] = settings.SMTPHost
	updates[SettingKeySMTPPort] = strconv.Itoa(settings.SMTPPort)
	updates[SettingKeySMTPUsername] = settings.SMTPUsername
	if settings.SMTPPassword != "" {
		updates[SettingKeySMTPPassword] = settings.SMTPPassword
REDACTED
	updates[SettingKeySMTPFrom] = settings.SMTPFrom
	updates[SettingKeySMTPFromName] = settings.SMTPFromName
	updates[SettingKeySMTPUseTLS] = strconv.FormatBool(settings.SMTPUseTLS)

	// Cloudflare Turnstile 设置（只有非空才更新密钥）
	updates[SettingKeyTurnstileEnabled] = strconv.FormatBool(settings.TurnstileEnabled)
	updates[SettingKeyTurnstileSiteKey] = settings.TurnstileSiteKey
	if settings.TurnstileSecretKey != "" {
		updates[SettingKeyTurnstileSecretKey] = settings.TurnstileSecretKey
REDACTED

	// LinuxDo Connect OAuth 登录
	updates[SettingKeyLinuxDoConnectEnabled] = strconv.FormatBool(settings.LinuxDoConnectEnabled)
	updates[SettingKeyLinuxDoConnectClientID] = settings.LinuxDoConnectClientID
	updates[SettingKeyLinuxDoConnectRedirectURL] = settings.LinuxDoConnectRedirectURL
	if settings.LinuxDoConnectClientSecret != "" {
		updates[SettingKeyLinuxDoConnectClientSecret] = settings.LinuxDoConnectClientSecret
REDACTED

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
REDACTED

	// GitHub / Google 邮箱快捷登录
	updates[SettingKeyGitHubOAuthEnabled] = strconv.FormatBool(settings.GitHubOAuthEnabled)
	updates[SettingKeyGitHubOAuthClientID] = strings.TrimSpace(settings.GitHubOAuthClientID)
	updates[SettingKeyGitHubOAuthRedirectURL] = settings.GitHubOAuthRedirectURL
	updates[SettingKeyGitHubOAuthFrontendRedirectURL] = settings.GitHubOAuthFrontendRedirectURL
	if settings.GitHubOAuthClientSecret != "" {
		updates[SettingKeyGitHubOAuthClientSecret] = strings.TrimSpace(settings.GitHubOAuthClientSecret)
REDACTED
	updates[SettingKeyGoogleOAuthEnabled] = strconv.FormatBool(settings.GoogleOAuthEnabled)
	updates[SettingKeyGoogleOAuthClientID] = strings.TrimSpace(settings.GoogleOAuthClientID)
	updates[SettingKeyGoogleOAuthRedirectURL] = settings.GoogleOAuthRedirectURL
	updates[SettingKeyGoogleOAuthFrontendRedirectURL] = settings.GoogleOAuthFrontendRedirectURL
	if settings.GoogleOAuthClientSecret != "" {
		updates[SettingKeyGoogleOAuthClientSecret] = strings.TrimSpace(settings.GoogleOAuthClientSecret)
REDACTED

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
REDACTED
	if settings.WeChatConnectOpenAppSecret != "" {
		updates[SettingKeyWeChatConnectOpenAppSecret] = settings.WeChatConnectOpenAppSecret
REDACTED
	if settings.WeChatConnectMPAppSecret != "" {
		updates[SettingKeyWeChatConnectMPAppSecret] = settings.WeChatConnectMPAppSecret
REDACTED
	if settings.WeChatConnectMobileAppSecret != "" {
		updates[SettingKeyWeChatConnectMobileAppSecret] = settings.WeChatConnectMobileAppSecret
REDACTED

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
REDACTED
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
REDACTED
	if settings.AffiliateRebateFreezeHours > AffiliateRebateFreezeHoursMax {
		settings.AffiliateRebateFreezeHours = AffiliateRebateFreezeHoursMax
REDACTED
	updates[SettingKeyAffiliateRebateFreezeHours] = strconv.Itoa(settings.AffiliateRebateFreezeHours)
	if settings.AffiliateRebateDurationDays < 0 {
		settings.AffiliateRebateDurationDays = AffiliateRebateDurationDaysDefault
REDACTED
	if settings.AffiliateRebateDurationDays > AffiliateRebateDurationDaysMax {
		settings.AffiliateRebateDurationDays = AffiliateRebateDurationDaysMax
REDACTED
	updates[SettingKeyAffiliateRebateDurationDays] = strconv.Itoa(settings.AffiliateRebateDurationDays)
	if settings.AffiliateRebatePerInviteeCap < 0 {
		settings.AffiliateRebatePerInviteeCap = AffiliateRebatePerInviteeCapDefault
REDACTED
	updates[SettingKeyAffiliateRebatePerInviteeCap] = strconv.FormatFloat(settings.AffiliateRebatePerInviteeCap, 'f', 8, 64)
	updates[SettingKeyDefaultUserRPMLimit] = strconv.Itoa(settings.DefaultUserRPMLimit)
	defaultSubsJSON, err := json.Marshal(settings.DefaultSubscriptions)
	if err != nil {
		return nil, fmt.Errorf("marshal default subscriptions: %w", err)
REDACTED
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
REDACTED

	// Channel monitor feature switch
	updates[SettingKeyChannelMonitorEnabled] = strconv.FormatBool(settings.ChannelMonitorEnabled)
	if v := clampChannelMonitorInterval(settings.ChannelMonitorDefaultIntervalSeconds); v > 0 {
		updates[SettingKeyChannelMonitorDefaultIntervalSeconds] = strconv.Itoa(v)
REDACTED

	// Available channels feature switch
	updates[SettingKeyAvailableChannelsEnabled] = strconv.FormatBool(settings.AvailableChannelsEnabled)

	// Affiliate (邀请返利) feature switch
	updates[SettingKeyAffiliateEnabled] = strconv.FormatBool(settings.AffiliateEnabled)

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
	updates[SettingKeyEnableAnthropicCacheTTL1hInjection] = strconv.FormatBool(settings.EnableAnthropicCacheTTL1hInjection)
	updates[SettingPaymentVisibleMethodAlipaySource] = settings.PaymentVisibleMethodAlipaySource
	updates[SettingPaymentVisibleMethodWxpaySource] = settings.PaymentVisibleMethodWxpaySource
	updates[SettingPaymentVisibleMethodAlipayEnabled] = strconv.FormatBool(settings.PaymentVisibleMethodAlipayEnabled)
	updates[SettingPaymentVisibleMethodWxpayEnabled] = strconv.FormatBool(settings.PaymentVisibleMethodWxpayEnabled)
	updates[openAIAdvancedSchedulerSettingKey] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerEnabled)

	// Balance low notification
	updates[SettingKeyBalanceLowNotifyEnabled] = strconv.FormatBool(settings.BalanceLowNotifyEnabled)
	updates[SettingKeyBalanceLowNotifyThreshold] = strconv.FormatFloat(settings.BalanceLowNotifyThreshold, 'f', 8, 64)
	updates[SettingKeyBalanceLowNotifyRechargeURL] = settings.BalanceLowNotifyRechargeURL
	updates[SettingKeyAccountQuotaNotifyEnabled] = strconv.FormatBool(settings.AccountQuotaNotifyEnabled)
	updates[SettingKeyAccountQuotaNotifyEmails] = MarshalNotifyEmails(settings.AccountQuotaNotifyEmails)

	return updates, nil
REDACTED

func (s *SettingService) buildAuthSourceDefaultUpdates(ctx context.Context, settings *AuthSourceDefaultSettings) (map[string]string, error) {
	if settings == nil {
		return nil, nil
REDACTED

	for _, subscriptions := range [][]DefaultSubscriptionSetting{
		settings.Email.Subscriptions,
		settings.LinuxDo.Subscriptions,
		settings.OIDC.Subscriptions,
		settings.WeChat.Subscriptions,
		settings.GitHub.Subscriptions,
		settings.Google.Subscriptions,
REDACTED {
		if err := s.validateDefaultSubscriptionGroups(ctx, subscriptions); err != nil {
			return nil, err
	REDACTED
REDACTED

	updates := make(map[string]string, 31)
	writeProviderDefaultGrantUpdates(updates, emailAuthSourceDefaultKeys, settings.Email)
	writeProviderDefaultGrantUpdates(updates, linuxDoAuthSourceDefaultKeys, settings.LinuxDo)
	writeProviderDefaultGrantUpdates(updates, oidcAuthSourceDefaultKeys, settings.OIDC)
	writeProviderDefaultGrantUpdates(updates, weChatAuthSourceDefaultKeys, settings.WeChat)
	writeProviderDefaultGrantUpdates(updates, gitHubAuthSourceDefaultKeys, settings.GitHub)
	writeProviderDefaultGrantUpdates(updates, googleAuthSourceDefaultKeys, settings.Google)
	updates[SettingKeyForceEmailOnThirdPartySignup] = strconv.FormatBool(settings.ForceEmailOnThirdPartySignup)
	return updates, nil
REDACTED

func (s *SettingService) refreshCachedSettings(settings *SystemSettings) {
	if settings == nil {
		return
REDACTED

	// 先使 inflight singleflight 失效，再刷新缓存，缩小旧值覆盖新值的竞态窗口
	versionBoundsSF.Forget("version_bounds")
	versionBoundsCache.Store(&cachedVersionBounds{
		min:       settings.MinClaudeCodeVersion,
		max:       settings.MaxClaudeCodeVersion,
		expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
REDACTED)
	backendModeSF.Forget("backend_mode")
	backendModeCache.Store(&cachedBackendMode{
		value:     settings.BackendModeEnabled,
		expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
REDACTED)
	gatewayForwardingSF.Forget("gateway_forwarding")
	gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
		fingerprintUnification:       settings.EnableFingerprintUnification,
		metadataPassthrough:          settings.EnableMetadataPassthrough,
		cchSigning:                   settings.EnableCCHSigning,
		anthropicCacheTTL1hInjection: settings.EnableAnthropicCacheTTL1hInjection,
		expiresAt:                    time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
REDACTED)
	openAIAdvancedSchedulerSettingSF.Forget(openAIAdvancedSchedulerSettingKey)
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		enabled:   settings.OpenAIAdvancedSchedulerEnabled,
		expiresAt: time.Now().Add(openAIAdvancedSchedulerSettingCacheTTL).UnixNano(),
REDACTED)
	if s.onUpdate != nil {
		s.onUpdate() // Invalidate cache after settings update
REDACTED
REDACTED

func (s *SettingService) validateDefaultSubscriptionGroups(ctx context.Context, items []DefaultSubscriptionSetting) error {
	if len(items) == 0 {
		return nil
REDACTED

	checked := make(map[int64]struct{REDACTED, len(items))
	for _, item := range items {
		if item.GroupID <= 0 {
			continue
	REDACTED
		if _, ok := checked[item.GroupID]; ok {
			return ErrDefaultSubGroupDuplicate.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
		REDACTED)
	REDACTED
		checked[item.GroupID] = struct{REDACTED{REDACTED
		if s.defaultSubGroupReader == nil {
			continue
	REDACTED

		group, err := s.defaultSubGroupReader.GetByID(ctx, item.GroupID)
		if err != nil {
			if errors.Is(err, ErrGroupNotFound) {
				return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
					"group_id": strconv.FormatInt(item.GroupID, 10),
			REDACTED)
		REDACTED
			return fmt.Errorf("get default subscription group %d: %w", item.GroupID, err)
	REDACTED
		if !group.IsSubscriptionType() {
			return ErrDefaultSubGroupInvalid.WithMetadata(map[string]string{
				"group_id": strconv.FormatInt(item.GroupID, 10),
		REDACTED)
	REDACTED
REDACTED

	return nil
REDACTED

func (s *SettingService) GetEmailOAuthProviderConfig(ctx context.Context, provider string) (config.EmailOAuthProviderConfig, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "github" && provider != "google" {
		return config.EmailOAuthProviderConfig{REDACTED, infraerrors.NotFound("OAUTH_PROVIDER_NOT_FOUND", "oauth provider not found")
REDACTED
	keys := []string{
		SettingKeyGitHubOAuthEnabled,
		SettingKeyGitHubOAuthClientID,
		SettingKeyGitHubOAuthClientSecret,
		SettingKeyGitHubOAuthRedirectURL,
		SettingKeyGitHubOAuthFrontendRedirectURL,
		SettingKeyGoogleOAuthEnabled,
		SettingKeyGoogleOAuthClientID,
		SettingKeyGoogleOAuthClientSecret,
		SettingKeyGoogleOAuthRedirectURL,
		SettingKeyGoogleOAuthFrontendRedirectURL,
REDACTED
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config.EmailOAuthProviderConfig{REDACTED, fmt.Errorf("get email oauth settings: %w", err)
REDACTED
	cfg := s.effectiveEmailOAuthConfig(settings, provider)
	if !cfg.Enabled {
		return config.EmailOAuthProviderConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
REDACTED
	if strings.TrimSpace(cfg.ClientID) == "" {
		return config.EmailOAuthProviderConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client id not configured")
REDACTED
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return config.EmailOAuthProviderConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client secret not configured")
REDACTED
	for label, rawURL := range map[string]string{
		"authorize": cfg.AuthorizeURL,
		"token":     cfg.TokenURL,
		"userinfo":  cfg.UserInfoURL,
		"redirect":  cfg.RedirectURL,
REDACTED {
		if strings.TrimSpace(rawURL) == "" {
			return config.EmailOAuthProviderConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth "+label+" url not configured")
	REDACTED
		if err := config.ValidateAbsoluteHTTPURL(rawURL); err != nil {
			return config.EmailOAuthProviderConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth "+label+" url invalid")
	REDACTED
REDACTED
	if strings.TrimSpace(cfg.EmailsURL) != "" {
		if err := config.ValidateAbsoluteHTTPURL(cfg.EmailsURL); err != nil {
			return config.EmailOAuthProviderConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth emails url invalid")
	REDACTED
REDACTED
	if err := config.ValidateFrontendRedirectURL(cfg.FrontendRedirectURL); err != nil {
		return config.EmailOAuthProviderConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url invalid")
REDACTED
	return cfg, nil
REDACTED

// IsRegistrationEnabled 检查是否开放注册
func (s *SettingService) IsRegistrationEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err != nil {
		// 安全默认：如果设置不存在或查询出错，默认关闭注册
		return false
REDACTED
	return value == "true"
REDACTED

// IsBackendModeEnabled checks if backend mode is enabled
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path
func (s *SettingService) IsBackendModeEnabled(ctx context.Context) bool {
	if cached, ok := backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.value
	REDACTED
REDACTED
	result, _, _ := backendModeSF.Do("backend_mode", func() (any, error) {
		if cached, ok := backendModeCache.Load().(*cachedBackendMode); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return cached.value, nil
		REDACTED
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), backendModeDBTimeout)
		defer cancel()
		value, err := s.settingRepo.GetValue(dbCtx, SettingKeyBackendModeEnabled)
		if err != nil {
			if errors.Is(err, ErrSettingNotFound) {
				// Setting not yet created (fresh install) - default to disabled with full TTL
				backendModeCache.Store(&cachedBackendMode{
					value:     false,
					expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
			REDACTED)
				return false, nil
		REDACTED
			slog.Warn("failed to get backend_mode_enabled setting", "error", err)
			backendModeCache.Store(&cachedBackendMode{
				value:     false,
				expiresAt: time.Now().Add(backendModeErrorTTL).UnixNano(),
		REDACTED)
			return false, nil
	REDACTED
		enabled := value == "true"
		backendModeCache.Store(&cachedBackendMode{
			value:     enabled,
			expiresAt: time.Now().Add(backendModeCacheTTL).UnixNano(),
	REDACTED)
		return enabled, nil
REDACTED)
	if val, ok := result.(bool); ok {
		return val
REDACTED
	return false
REDACTED

type gatewayForwardingSettingsResult struct {
	fp, mp, cch, cacheTTL1h bool
REDACTED

func (s *SettingService) getGatewayForwardingSettingsCached(ctx context.Context) gatewayForwardingSettingsResult {
	if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
		if time.Now().UnixNano() < cached.expiresAt {
			return gatewayForwardingSettingsResult{
				fp:         cached.fingerprintUnification,
				mp:         cached.metadataPassthrough,
				cch:        cached.cchSigning,
				cacheTTL1h: cached.anthropicCacheTTL1hInjection,
		REDACTED
	REDACTED
REDACTED
	val, _, _ := gatewayForwardingSF.Do("gateway_forwarding", func() (any, error) {
		if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil {
			if time.Now().UnixNano() < cached.expiresAt {
				return gatewayForwardingSettingsResult{
					fp:         cached.fingerprintUnification,
					mp:         cached.metadataPassthrough,
					cch:        cached.cchSigning,
					cacheTTL1h: cached.anthropicCacheTTL1hInjection,
			REDACTED, nil
		REDACTED
	REDACTED
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayForwardingDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyEnableFingerprintUnification,
			SettingKeyEnableMetadataPassthrough,
			SettingKeyEnableCCHSigning,
			SettingKeyEnableAnthropicCacheTTL1hInjection,
	REDACTED)
		if err != nil {
			slog.Warn("failed to get gateway forwarding settings", "error", err)
			gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
				fingerprintUnification:       true,
				metadataPassthrough:          false,
				cchSigning:                   false,
				anthropicCacheTTL1hInjection: false,
				expiresAt:                    time.Now().Add(gatewayForwardingErrorTTL).UnixNano(),
		REDACTED)
			return gatewayForwardingSettingsResult{fp: trueREDACTED, nil
	REDACTED
		fp := true
		if v, ok := values[SettingKeyEnableFingerprintUnification]; ok && v != "" {
			fp = v == "true"
	REDACTED
		mp := values[SettingKeyEnableMetadataPassthrough] == "true"
		cch := values[SettingKeyEnableCCHSigning] == "true"
		cacheTTL1h := values[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"
		gatewayForwardingCache.Store(&cachedGatewayForwardingSettings{
			fingerprintUnification:       fp,
			metadataPassthrough:          mp,
			cchSigning:                   cch,
			anthropicCacheTTL1hInjection: cacheTTL1h,
			expiresAt:                    time.Now().Add(gatewayForwardingCacheTTL).UnixNano(),
	REDACTED)
		return gatewayForwardingSettingsResult{fp: fp, mp: mp, cch: cch, cacheTTL1h: cacheTTL1hREDACTED, nil
REDACTED)
	if r, ok := val.(gatewayForwardingSettingsResult); ok {
		return r
REDACTED
	return gatewayForwardingSettingsResult{fp: trueREDACTED
REDACTED

// GetGatewayForwardingSettings returns cached gateway forwarding settings.
// Uses in-process atomic.Value cache with 60s TTL, zero-lock hot path.
// Returns (fingerprintUnification, metadataPassthrough, cchSigning).
func (s *SettingService) GetGatewayForwardingSettings(ctx context.Context) (fingerprintUnification, metadataPassthrough, cchSigning bool) {
	result := s.getGatewayForwardingSettingsCached(ctx)
	return result.fp, result.mp, result.cch
REDACTED

// IsAnthropicCacheTTL1hInjectionEnabled 检查是否对 Anthropic OAuth/SetupToken 请求体注入 1h cache_control ttl。
func (s *SettingService) IsAnthropicCacheTTL1hInjectionEnabled(ctx context.Context) bool {
	return s.getGatewayForwardingSettingsCached(ctx).cacheTTL1h
REDACTED

// IsEmailVerifyEnabled 检查是否开启邮件验证
func (s *SettingService) IsEmailVerifyEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEmailVerifyEnabled)
	if err != nil {
		return false
REDACTED
	return value == "true"
REDACTED

// GetRegistrationEmailSuffixWhitelist returns normalized registration email suffix whitelist.
func (s *SettingService) GetRegistrationEmailSuffixWhitelist(ctx context.Context) []string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEmailSuffixWhitelist)
	if err != nil {
		return []string{REDACTED
REDACTED
	return ParseRegistrationEmailSuffixWhitelist(value)
REDACTED

// IsPromoCodeEnabled 检查是否启用优惠码功能
func (s *SettingService) IsPromoCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPromoCodeEnabled)
	if err != nil {
		return true // 默认启用
REDACTED
	return value != "false"
REDACTED

// IsInvitationCodeEnabled 检查是否启用邀请码注册功能
func (s *SettingService) IsInvitationCodeEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyInvitationCodeEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// IsAffiliateEnabled 检查是否启用邀请返利功能（总开关）
func (s *SettingService) IsAffiliateEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// GetAffiliateRebateRatePercent 读取并 clamp 全局返利比例。
// 解析失败、缺失或越界都回退到 AffiliateRebateRateDefault — 该比例从不抛错，
// 调用方只关心一个可用的数值。
func (s *SettingService) GetAffiliateRebateRatePercent(ctx context.Context) float64 {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateRate)
	if err != nil {
		return AffiliateRebateRateDefault
REDACTED
	rate, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return AffiliateRebateRateDefault
REDACTED
	return clampAffiliateRebateRate(rate)
REDACTED

// GetAffiliateRebateFreezeHours 返回返利冻结期（小时）。
// 返回 0 表示不冻结（向后兼容）。
func (s *SettingService) GetAffiliateRebateFreezeHours(ctx context.Context) int {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateFreezeHours)
	if err != nil {
		return AffiliateRebateFreezeHoursDefault
REDACTED
	hours, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || hours < 0 {
		return AffiliateRebateFreezeHoursDefault
REDACTED
	if hours > AffiliateRebateFreezeHoursMax {
		return AffiliateRebateFreezeHoursMax
REDACTED
	return hours
REDACTED

// GetAffiliateRebateDurationDays 返回返利有效期（天）。
// 返回 0 表示永久有效。
func (s *SettingService) GetAffiliateRebateDurationDays(ctx context.Context) int {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebateDurationDays)
	if err != nil {
		return AffiliateRebateDurationDaysDefault
REDACTED
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || days < 0 {
		return AffiliateRebateDurationDaysDefault
REDACTED
	if days > AffiliateRebateDurationDaysMax {
		return AffiliateRebateDurationDaysMax
REDACTED
	return days
REDACTED

// GetAffiliateRebatePerInviteeCap 返回单人返利上限。
// 返回 0 表示无上限。
func (s *SettingService) GetAffiliateRebatePerInviteeCap(ctx context.Context) float64 {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAffiliateRebatePerInviteeCap)
	if err != nil {
		return AffiliateRebatePerInviteeCapDefault
REDACTED
	cap, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || cap < 0 || math.IsNaN(cap) || math.IsInf(cap, 0) {
		return AffiliateRebatePerInviteeCapDefault
REDACTED
	return cap
REDACTED

// IsPasswordResetEnabled 检查是否启用密码重置功能
// 要求：必须同时开启邮件验证
func (s *SettingService) IsPasswordResetEnabled(ctx context.Context) bool {
	// Password reset requires email verification to be enabled
	if !s.IsEmailVerifyEnabled(ctx) {
		return false
REDACTED
	value, err := s.settingRepo.GetValue(ctx, SettingKeyPasswordResetEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// IsTotpEnabled 检查是否启用 TOTP 双因素认证功能
func (s *SettingService) IsTotpEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTotpEnabled)
	if err != nil {
		return false // 默认关闭
REDACTED
	return value == "true"
REDACTED

// IsTotpEncryptionKeyConfigured 检查 TOTP 加密密钥是否已手动配置
// 只有手动配置了密钥才允许在管理后台启用 TOTP 功能
func (s *SettingService) IsTotpEncryptionKeyConfigured() bool {
	return s.cfg.Totp.EncryptionKeyConfigured
REDACTED

// GetSiteName 获取网站名称
func (s *SettingService) GetSiteName(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeySiteName)
	if err != nil || value == "" {
		return "Sub2API"
REDACTED
	return value
REDACTED

// GetDefaultConcurrency 获取默认并发量
func (s *SettingService) GetDefaultConcurrency(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultConcurrency)
	if err != nil {
		return s.cfg.Default.UserConcurrency
REDACTED
	if v, err := strconv.Atoi(value); err == nil && v > 0 {
		return v
REDACTED
	return s.cfg.Default.UserConcurrency
REDACTED

// GetDefaultBalance 获取默认余额
func (s *SettingService) GetDefaultBalance(ctx context.Context) float64 {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultBalance)
	if err != nil {
		return s.cfg.Default.UserBalance
REDACTED
	if v, err := strconv.ParseFloat(value, 64); err == nil && v >= 0 {
		return v
REDACTED
	return s.cfg.Default.UserBalance
REDACTED

// GetDefaultUserRPMLimit 获取新用户默认 RPM 限制（0 = 不限制）。未配置则返回 0。
func (s *SettingService) GetDefaultUserRPMLimit(ctx context.Context) int {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultUserRPMLimit)
	if err != nil || value == "" {
		return 0
REDACTED
	if v, err := strconv.Atoi(value); err == nil && v >= 0 {
		return v
REDACTED
	return 0
REDACTED

// GetDefaultSubscriptions 获取新用户默认订阅配置列表。
func (s *SettingService) GetDefaultSubscriptions(ctx context.Context) []DefaultSubscriptionSetting {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyDefaultSubscriptions)
	if err != nil {
		return nil
REDACTED
	return parseDefaultSubscriptions(value)
REDACTED

func (s *SettingService) GetAuthSourceDefaultSettings(ctx context.Context) (*AuthSourceDefaultSettings, error) {
	keys := []string{
		SettingKeyAuthSourceDefaultEmailBalance,
		SettingKeyAuthSourceDefaultEmailConcurrency,
		SettingKeyAuthSourceDefaultEmailSubscriptions,
		SettingKeyAuthSourceDefaultEmailGrantOnSignup,
		SettingKeyAuthSourceDefaultEmailGrantOnFirstBind,
		SettingKeyAuthSourceDefaultLinuxDoBalance,
		SettingKeyAuthSourceDefaultLinuxDoConcurrency,
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions,
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup,
		SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind,
		SettingKeyAuthSourceDefaultOIDCBalance,
		SettingKeyAuthSourceDefaultOIDCConcurrency,
		SettingKeyAuthSourceDefaultOIDCSubscriptions,
		SettingKeyAuthSourceDefaultOIDCGrantOnSignup,
		SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind,
		SettingKeyAuthSourceDefaultWeChatBalance,
		SettingKeyAuthSourceDefaultWeChatConcurrency,
		SettingKeyAuthSourceDefaultWeChatSubscriptions,
		SettingKeyAuthSourceDefaultWeChatGrantOnSignup,
		SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind,
		SettingKeyAuthSourceDefaultGitHubBalance,
		SettingKeyAuthSourceDefaultGitHubConcurrency,
		SettingKeyAuthSourceDefaultGitHubSubscriptions,
		SettingKeyAuthSourceDefaultGitHubGrantOnSignup,
		SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind,
		SettingKeyAuthSourceDefaultGoogleBalance,
		SettingKeyAuthSourceDefaultGoogleConcurrency,
		SettingKeyAuthSourceDefaultGoogleSubscriptions,
		SettingKeyAuthSourceDefaultGoogleGrantOnSignup,
		SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind,
		SettingKeyForceEmailOnThirdPartySignup,
REDACTED

	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("get auth source default settings: %w", err)
REDACTED

	return &AuthSourceDefaultSettings{
		Email:                        parseProviderDefaultGrantSettings(settings, emailAuthSourceDefaultKeys),
		LinuxDo:                      parseProviderDefaultGrantSettings(settings, linuxDoAuthSourceDefaultKeys),
		OIDC:                         parseProviderDefaultGrantSettings(settings, oidcAuthSourceDefaultKeys),
		WeChat:                       parseProviderDefaultGrantSettings(settings, weChatAuthSourceDefaultKeys),
		GitHub:                       parseProviderDefaultGrantSettings(settings, gitHubAuthSourceDefaultKeys),
		Google:                       parseProviderDefaultGrantSettings(settings, googleAuthSourceDefaultKeys),
		ForceEmailOnThirdPartySignup: settings[SettingKeyForceEmailOnThirdPartySignup] == "true",
REDACTED, nil
REDACTED

func (s *SettingService) ResolveAuthSourceGrantSettings(ctx context.Context, signupSource string, firstBind bool) (ProviderDefaultGrantSettings, bool, error) {
	result := ProviderDefaultGrantSettings{
		Balance:       s.GetDefaultBalance(ctx),
		Concurrency:   s.GetDefaultConcurrency(ctx),
		Subscriptions: s.GetDefaultSubscriptions(ctx),
REDACTED

	defaults, err := s.GetAuthSourceDefaultSettings(ctx)
	if err != nil {
		return result, false, err
REDACTED

	providerDefaults, ok := authSourceSignupSettings(defaults, signupSource)
	if !ok {
		return result, false, nil
REDACTED

	enabled := providerDefaults.GrantOnSignup
	if firstBind {
		enabled = providerDefaults.GrantOnFirstBind
REDACTED
	if !enabled {
		return result, false, nil
REDACTED

	return mergeProviderDefaultGrantSettings(result, providerDefaults), true, nil
REDACTED

func (s *SettingService) UpdateAuthSourceDefaultSettings(ctx context.Context, settings *AuthSourceDefaultSettings) error {
	updates, err := s.buildAuthSourceDefaultUpdates(ctx, settings)
	if err != nil {
		return err
REDACTED
	if len(updates) == 0 {
		return nil
REDACTED

	if err := s.settingRepo.SetMultiple(ctx, updates); err != nil {
		return fmt.Errorf("update auth source default settings: %w", err)
REDACTED
	return nil
REDACTED

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

	// 初始化默认设置
	defaults := map[string]string{
		SettingKeyRegistrationEnabled:                      "true",
		SettingKeyEmailVerifyEnabled:                       "false",
		SettingKeyRegistrationEmailSuffixWhitelist:         "[]",
		SettingKeyPromoCodeEnabled:                         "true", // 默认启用优惠码功能
		SettingKeySiteName:                                 "Sub2API",
		SettingKeySiteLogo:                                 "",
		SettingKeyPurchaseSubscriptionEnabled:              "false",
		SettingKeyPurchaseSubscriptionURL:                  "",
		SettingKeyTableDefaultPageSize:                     "20",
		SettingKeyTablePageSizeOptions:                     "[10,20,50,100]",
		SettingKeyCustomMenuItems:                          "[]",
		SettingKeyCustomEndpoints:                          "[]",
		SettingKeyWeChatConnectEnabled:                     "false",
		SettingKeyWeChatConnectAppID:                       "",
		SettingKeyWeChatConnectAppSecret:                   "",
		SettingKeyWeChatConnectOpenAppID:                   "",
		SettingKeyWeChatConnectOpenAppSecret:               "",
		SettingKeyWeChatConnectMPAppID:                     "",
		SettingKeyWeChatConnectMPAppSecret:                 "",
		SettingKeyWeChatConnectMobileAppID:                 "",
		SettingKeyWeChatConnectMobileAppSecret:             "",
		SettingKeyWeChatConnectOpenEnabled:                 "false",
		SettingKeyWeChatConnectMPEnabled:                   "false",
		SettingKeyWeChatConnectMobileEnabled:               "false",
		SettingKeyWeChatConnectMode:                        "open",
		SettingKeyWeChatConnectScopes:                      "snsapi_login",
		SettingKeyWeChatConnectRedirectURL:                 "",
		SettingKeyWeChatConnectFrontendRedirectURL:         defaultWeChatConnectFrontend,
		SettingKeyGitHubOAuthEnabled:                       "false",
		SettingKeyGitHubOAuthClientID:                      "",
		SettingKeyGitHubOAuthClientSecret:                  "",
		SettingKeyGitHubOAuthRedirectURL:                   "",
		SettingKeyGitHubOAuthFrontendRedirectURL:           defaultGitHubOAuthFrontend,
		SettingKeyGoogleOAuthEnabled:                       "false",
		SettingKeyGoogleOAuthClientID:                      "",
		SettingKeyGoogleOAuthClientSecret:                  "",
		SettingKeyGoogleOAuthRedirectURL:                   "",
		SettingKeyGoogleOAuthFrontendRedirectURL:           defaultGoogleOAuthFrontend,
		SettingKeyOIDCConnectEnabled:                       "false",
		SettingKeyOIDCConnectProviderName:                  "OIDC",
		SettingKeyOIDCConnectClientID:                      "",
		SettingKeyOIDCConnectClientSecret:                  "",
		SettingKeyOIDCConnectIssuerURL:                     "",
		SettingKeyOIDCConnectDiscoveryURL:                  "",
		SettingKeyOIDCConnectAuthorizeURL:                  "",
		SettingKeyOIDCConnectTokenURL:                      "",
		SettingKeyOIDCConnectUserInfoURL:                   "",
		SettingKeyOIDCConnectJWKSURL:                       "",
		SettingKeyOIDCConnectScopes:                        "openid email profile",
		SettingKeyOIDCConnectRedirectURL:                   "",
		SettingKeyOIDCConnectFrontendRedirectURL:           "/auth/oidc/callback",
		SettingKeyOIDCConnectTokenAuthMethod:               "client_secret_post",
		SettingKeyOIDCConnectUsePKCE:                       strconv.FormatBool(oidcUsePKCEDefault),
		SettingKeyOIDCConnectValidateIDToken:               strconv.FormatBool(oidcValidateIDTokenDefault),
		SettingKeyOIDCConnectAllowedSigningAlgs:            "RS256,ES256,PS256",
		SettingKeyOIDCConnectClockSkewSeconds:              "120",
		SettingKeyOIDCConnectRequireEmailVerified:          "false",
		SettingKeyOIDCConnectUserInfoEmailPath:             "",
		SettingKeyOIDCConnectUserInfoIDPath:                "",
		SettingKeyOIDCConnectUserInfoUsernamePath:          "",
		SettingKeyDefaultConcurrency:                       strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:                           strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeyAffiliateRebateRate:                      strconv.FormatFloat(AffiliateRebateRateDefault, 'f', 8, 64),
		SettingKeyAffiliateRebateFreezeHours:               strconv.Itoa(AffiliateRebateFreezeHoursDefault),
		SettingKeyAffiliateRebateDurationDays:              strconv.Itoa(AffiliateRebateDurationDaysDefault),
		SettingKeyAffiliateRebatePerInviteeCap:             strconv.FormatFloat(AffiliateRebatePerInviteeCapDefault, 'f', 2, 64),
		SettingKeyDefaultUserRPMLimit:                      "0",
		SettingKeyDefaultSubscriptions:                     "[]",
		SettingKeyAuthSourceDefaultEmailBalance:            "0",
		SettingKeyAuthSourceDefaultEmailConcurrency:        "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultEmailGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultEmailGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultLinuxDoBalance:          "0",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:      "5",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions:    "[]",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup:    "false",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind: "false",
		SettingKeyAuthSourceDefaultOIDCBalance:             "0",
		SettingKeyAuthSourceDefaultOIDCConcurrency:         "5",
		SettingKeyAuthSourceDefaultOIDCSubscriptions:       "[]",
		SettingKeyAuthSourceDefaultOIDCGrantOnSignup:       "false",
		SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind:    "false",
		SettingKeyAuthSourceDefaultWeChatBalance:           "0",
		SettingKeyAuthSourceDefaultWeChatConcurrency:       "5",
		SettingKeyAuthSourceDefaultWeChatSubscriptions:     "[]",
		SettingKeyAuthSourceDefaultWeChatGrantOnSignup:     "false",
		SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind:  "false",
		SettingKeyAuthSourceDefaultGitHubBalance:           "0",
		SettingKeyAuthSourceDefaultGitHubConcurrency:       "5",
		SettingKeyAuthSourceDefaultGitHubSubscriptions:     "[]",
		SettingKeyAuthSourceDefaultGitHubGrantOnSignup:     "false",
		SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind:  "false",
		SettingKeyAuthSourceDefaultGoogleBalance:           "0",
		SettingKeyAuthSourceDefaultGoogleConcurrency:       "5",
		SettingKeyAuthSourceDefaultGoogleSubscriptions:     "[]",
		SettingKeyAuthSourceDefaultGoogleGrantOnSignup:     "false",
		SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind:  "false",
		SettingKeyForceEmailOnThirdPartySignup:             "false",
		SettingKeySMTPPort:                                 "587",
		SettingKeySMTPUseTLS:                               "false",
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

		// Claude Code version check (default: empty = disabled)
		SettingKeyMinClaudeCodeVersion: "",
		SettingKeyMaxClaudeCodeVersion: "",

		// 分组隔离（默认不允许未分组 Key 调度）
		SettingKeyAllowUngroupedKeyScheduling:        "false",
		SettingKeyEnableAnthropicCacheTTL1hInjection: "false",
		SettingPaymentVisibleMethodAlipaySource:      "",
		SettingPaymentVisibleMethodWxpaySource:       "",
		SettingPaymentVisibleMethodAlipayEnabled:     "false",
		SettingPaymentVisibleMethodWxpayEnabled:      "false",
		openAIAdvancedSchedulerSettingKey:            "false",
REDACTED

	return s.settingRepo.SetMultiple(ctx, defaults)
REDACTED

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	result := &SystemSettings{
		RegistrationEnabled:              settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:               emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: ParseRegistrationEmailSuffixWhitelist(settings[SettingKeyRegistrationEmailSuffixWhitelist]),
		PromoCodeEnabled:                 settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:             emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		FrontendURL:                      settings[SettingKeyFrontendURL],
		InvitationCodeEnabled:            settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                      settings[SettingKeyTotpEnabled] == "true",
		SMTPHost:                         settings[SettingKeySMTPHost],
		SMTPUsername:                     settings[SettingKeySMTPUsername],
		SMTPFrom:                         settings[SettingKeySMTPFrom],
		SMTPFromName:                     settings[SettingKeySMTPFromName],
		SMTPUseTLS:                       settings[SettingKeySMTPUseTLS] == "true",
		SMTPPasswordConfigured:           settings[SettingKeySMTPPassword] != "",
		TurnstileEnabled:                 settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                 settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured:     settings[SettingKeyTurnstileSecretKey] != "",
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

	// Claude Code version check
	result.MinClaudeCodeVersion = settings[SettingKeyMinClaudeCodeVersion]
	result.MaxClaudeCodeVersion = settings[SettingKeyMaxClaudeCodeVersion]

	// 分组隔离
	result.AllowUngroupedKeyScheduling = settings[SettingKeyAllowUngroupedKeyScheduling] == "true"

	// Gateway forwarding behavior (defaults: fingerprint=true, metadata_passthrough=false, cch_signing=false)
	if v, ok := settings[SettingKeyEnableFingerprintUnification]; ok && v != "" {
		result.EnableFingerprintUnification = v == "true"
REDACTED else {
		result.EnableFingerprintUnification = true // default: enabled (current behavior)
REDACTED
	result.EnableMetadataPassthrough = settings[SettingKeyEnableMetadataPassthrough] == "true"
	result.EnableCCHSigning = settings[SettingKeyEnableCCHSigning] == "true"
	result.EnableAnthropicCacheTTL1hInjection = settings[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"

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

	// Balance low notification
	result.BalanceLowNotifyEnabled = settings[SettingKeyBalanceLowNotifyEnabled] == "true"
	if v, err := strconv.ParseFloat(settings[SettingKeyBalanceLowNotifyThreshold], 64); err == nil && v >= 0 {
		result.BalanceLowNotifyThreshold = v
REDACTED
	result.BalanceLowNotifyRechargeURL = settings[SettingKeyBalanceLowNotifyRechargeURL]

	// Account quota notification
	result.AccountQuotaNotifyEnabled = settings[SettingKeyAccountQuotaNotifyEnabled] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyAccountQuotaNotifyEmails]); raw != "" {
		result.AccountQuotaNotifyEmails = ParseNotifyEmails(raw)
REDACTED
	if result.AccountQuotaNotifyEmails == nil {
		result.AccountQuotaNotifyEmails = []NotifyEmailEntry{REDACTED
REDACTED

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
REDACTED

func mergeProviderDefaultGrantSettings(globalDefaults ProviderDefaultGrantSettings, providerDefaults ProviderDefaultGrantSettings) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          globalDefaults.Balance,
		Concurrency:      globalDefaults.Concurrency,
		Subscriptions:    append([]DefaultSubscriptionSetting(nil), globalDefaults.Subscriptions...),
		GrantOnSignup:    providerDefaults.GrantOnSignup,
		GrantOnFirstBind: providerDefaults.GrantOnFirstBind,
REDACTED

	if providerDefaults.Balance != defaultAuthSourceBalance {
		result.Balance = providerDefaults.Balance
REDACTED
	if providerDefaults.Concurrency > 0 && providerDefaults.Concurrency != defaultAuthSourceConcurrency {
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

// getStringOrDefault 获取字符串值或默认值
func (s *SettingService) getStringOrDefault(settings map[string]string, key, defaultValue string) string {
	if value, ok := settings[key]; ok && value != "" {
		return value
REDACTED
	return defaultValue
REDACTED

// IsTurnstileEnabled 检查是否启用 Turnstile 验证
func (s *SettingService) IsTurnstileEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileEnabled)
	if err != nil {
		return false
REDACTED
	return value == "true"
REDACTED

// GetTurnstileSecretKey 获取 Turnstile Secret Key
func (s *SettingService) GetTurnstileSecretKey(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyTurnstileSecretKey)
	if err != nil {
		return ""
REDACTED
	return value
REDACTED

// IsIdentityPatchEnabled 检查是否启用身份补丁（Claude -> Gemini systemInstruction 注入）
func (s *SettingService) IsIdentityPatchEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableIdentityPatch)
	if err != nil {
		// 默认开启，保持兼容
		return true
REDACTED
	return value == "true"
REDACTED

// GetIdentityPatchPrompt 获取自定义身份补丁提示词（为空表示使用内置默认模板）
func (s *SettingService) GetIdentityPatchPrompt(ctx context.Context) string {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyIdentityPatchPrompt)
	if err != nil {
		return ""
REDACTED
	return value
REDACTED

// GenerateAdminAPIKey 生成新的管理员 API Key
func (s *SettingService) GenerateAdminAPIKey(ctx context.Context) (string, error) {
	// 生成 32 字节随机数 = 64 位十六进制字符
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
REDACTED

	key := AdminAPIKeyPrefix + hex.EncodeToString(bytes)

	// 存储到 settings 表
	if err := s.settingRepo.Set(ctx, SettingKeyAdminAPIKey, key); err != nil {
		return "", fmt.Errorf("save admin api key: %w", err)
REDACTED

	return key, nil
REDACTED

// GetAdminAPIKeyStatus 获取管理员 API Key 状态
// 返回脱敏的 key、是否存在、错误
func (s *SettingService) GetAdminAPIKeyStatus(ctx context.Context) (maskedKey string, exists bool, err error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", false, nil
	REDACTED
		return "", false, err
REDACTED
	if key == "" {
		return "", false, nil
REDACTED

	// 脱敏：显示前 10 位和后 4 位
	if len(key) > 14 {
		maskedKey = key[:10] + "..." + key[len(key)-4:]
REDACTED else {
		maskedKey = key
REDACTED

	return maskedKey, true, nil
REDACTED

// GetAdminAPIKey 获取完整的管理员 API Key（仅供内部验证使用）
// 如果未配置返回空字符串和 nil 错误，只有数据库错误时才返回 error
func (s *SettingService) GetAdminAPIKey(ctx context.Context) (string, error) {
	key, err := s.settingRepo.GetValue(ctx, SettingKeyAdminAPIKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return "", nil // 未配置，返回空字符串
	REDACTED
		return "", err // 数据库错误
REDACTED
	return key, nil
REDACTED

// DeleteAdminAPIKey 删除管理员 API Key
func (s *SettingService) DeleteAdminAPIKey(ctx context.Context) error {
	return s.settingRepo.Delete(ctx, SettingKeyAdminAPIKey)
REDACTED

// IsModelFallbackEnabled 检查是否启用模型兜底机制
func (s *SettingService) IsModelFallbackEnabled(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyEnableModelFallback)
	if err != nil {
		return false // Default: disabled
REDACTED
	return value == "true"
REDACTED

// GetFallbackModel 获取指定平台的兜底模型
func (s *SettingService) GetFallbackModel(ctx context.Context, platform string) string {
	var key string
	var defaultModel string

	switch platform {
	case PlatformAnthropic:
		key = SettingKeyFallbackModelAnthropic
		defaultModel = "claude-3-5-sonnet-20241022"
	case PlatformOpenAI:
		key = SettingKeyFallbackModelOpenAI
		defaultModel = "gpt-4o"
	case PlatformGemini:
		key = SettingKeyFallbackModelGemini
		defaultModel = "gemini-2.5-pro"
	case PlatformAntigravity:
		key = SettingKeyFallbackModelAntigravity
		defaultModel = "gemini-2.5-pro"
	default:
		return ""
REDACTED

	value, err := s.settingRepo.GetValue(ctx, key)
	if err != nil || value == "" {
		return defaultModel
REDACTED
	return value
REDACTED

// GetLinuxDoConnectOAuthConfig 返回用于登录的"最终生效" LinuxDo Connect 配置。
//
// 优先级：
// - 若对应系统设置键存在，则覆盖 config.yaml/env 的值
// - 否则回退到 config.yaml/env 的值
func (s *SettingService) GetLinuxDoConnectOAuthConfig(ctx context.Context) (config.LinuxDoConnectConfig, error) {
	if s == nil || s.cfg == nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
REDACTED

	effective := s.cfg.LinuxDo

	keys := []string{
		SettingKeyLinuxDoConnectEnabled,
		SettingKeyLinuxDoConnectClientID,
		SettingKeyLinuxDoConnectClientSecret,
		SettingKeyLinuxDoConnectRedirectURL,
REDACTED
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config.LinuxDoConnectConfig{REDACTED, fmt.Errorf("get linuxdo connect settings: %w", err)
REDACTED

	if raw, ok := settings[SettingKeyLinuxDoConnectEnabled]; ok {
		effective.Enabled = raw == "true"
REDACTED
	if v, ok := settings[SettingKeyLinuxDoConnectClientID]; ok && strings.TrimSpace(v) != "" {
		effective.ClientID = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyLinuxDoConnectClientSecret]; ok && strings.TrimSpace(v) != "" {
		effective.ClientSecret = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyLinuxDoConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.RedirectURL = strings.TrimSpace(v)
REDACTED
	if !effective.Enabled {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
REDACTED

	// 基础健壮性校验（避免把用户重定向到一个必然失败或不安全的 OAuth 流程里）。
	if strings.TrimSpace(effective.ClientID) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client id not configured")
REDACTED
	if strings.TrimSpace(effective.AuthorizeURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url not configured")
REDACTED
	if strings.TrimSpace(effective.TokenURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url not configured")
REDACTED
	if strings.TrimSpace(effective.UserInfoURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url not configured")
REDACTED
	if strings.TrimSpace(effective.RedirectURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured")
REDACTED
	if strings.TrimSpace(effective.FrontendRedirectURL) == "" {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url not configured")
REDACTED

	if err := config.ValidateAbsoluteHTTPURL(effective.AuthorizeURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.TokenURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.UserInfoURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.RedirectURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url invalid")
REDACTED
	if err := config.ValidateFrontendRedirectURL(effective.FrontendRedirectURL); err != nil {
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url invalid")
REDACTED

	method := strings.ToLower(strings.TrimSpace(effective.TokenAuthMethod))
	switch method {
	case "", "client_secret_post", "client_secret_basic":
		if strings.TrimSpace(effective.ClientSecret) == "" {
			return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client secret not configured")
	REDACTED
	case "none":
	default:
		return config.LinuxDoConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token_auth_method invalid")
REDACTED

	return effective, nil
REDACTED

// GetWeChatConnectOAuthConfig 返回用于登录的最终生效 WeChat Connect 配置。
//
// WeChat Connect 已回归 DB 系统设置模型，不再回退到 config/env。
func (s *SettingService) GetWeChatConnectOAuthConfig(ctx context.Context) (WeChatConnectOAuthConfig, error) {
	keys := []string{
		SettingKeyWeChatConnectEnabled,
		SettingKeyWeChatConnectAppID,
		SettingKeyWeChatConnectAppSecret,
		SettingKeyWeChatConnectOpenAppID,
		SettingKeyWeChatConnectOpenAppSecret,
		SettingKeyWeChatConnectMPAppID,
		SettingKeyWeChatConnectMPAppSecret,
		SettingKeyWeChatConnectMobileAppID,
		SettingKeyWeChatConnectMobileAppSecret,
		SettingKeyWeChatConnectOpenEnabled,
		SettingKeyWeChatConnectMPEnabled,
		SettingKeyWeChatConnectMobileEnabled,
		SettingKeyWeChatConnectMode,
		SettingKeyWeChatConnectScopes,
		SettingKeyWeChatConnectRedirectURL,
		SettingKeyWeChatConnectFrontendRedirectURL,
REDACTED
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return WeChatConnectOAuthConfig{REDACTED, fmt.Errorf("get wechat connect settings: %w", err)
REDACTED
	return s.parseWeChatConnectOAuthConfig(settings)
REDACTED

// GetOverloadCooldownSettings 获取529过载冷却配置
func (s *SettingService) GetOverloadCooldownSettings(ctx context.Context) (*OverloadCooldownSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOverloadCooldownSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultOverloadCooldownSettings(), nil
	REDACTED
		return nil, fmt.Errorf("get overload cooldown settings: %w", err)
REDACTED
	if value == "" {
		return DefaultOverloadCooldownSettings(), nil
REDACTED

	var settings OverloadCooldownSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultOverloadCooldownSettings(), nil
REDACTED

	// 修正配置值范围
	if settings.CooldownMinutes < 1 {
		settings.CooldownMinutes = 1
REDACTED
	if settings.CooldownMinutes > 120 {
		settings.CooldownMinutes = 120
REDACTED

	return &settings, nil
REDACTED

// SetOverloadCooldownSettings 设置529过载冷却配置
func (s *SettingService) SetOverloadCooldownSettings(ctx context.Context, settings *OverloadCooldownSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	// 禁用时修正为合法值即可，不拒绝请求
	if settings.CooldownMinutes < 1 || settings.CooldownMinutes > 120 {
		if settings.Enabled {
			return fmt.Errorf("cooldown_minutes must be between 1-120")
	REDACTED
		settings.CooldownMinutes = 10 // 禁用状态下归一化为默认值
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal overload cooldown settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyOverloadCooldownSettings, string(data))
REDACTED

// GetRateLimit429CooldownSettings 获取429默认回避配置
func (s *SettingService) GetRateLimit429CooldownSettings(ctx context.Context) (*RateLimit429CooldownSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRateLimit429CooldownSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultRateLimit429CooldownSettings(), nil
	REDACTED
		return nil, fmt.Errorf("get 429 cooldown settings: %w", err)
REDACTED
	if value == "" {
		return DefaultRateLimit429CooldownSettings(), nil
REDACTED

	var settings RateLimit429CooldownSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultRateLimit429CooldownSettings(), nil
REDACTED

	if settings.CooldownSeconds < 1 {
		settings.CooldownSeconds = 1
REDACTED
	if settings.CooldownSeconds > 7200 {
		settings.CooldownSeconds = 7200
REDACTED

	return &settings, nil
REDACTED

// SetRateLimit429CooldownSettings 设置429默认回避配置
func (s *SettingService) SetRateLimit429CooldownSettings(ctx context.Context, settings *RateLimit429CooldownSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	if settings.CooldownSeconds < 1 || settings.CooldownSeconds > 7200 {
		if settings.Enabled {
			return fmt.Errorf("cooldown_seconds must be between 1-7200")
	REDACTED
		settings.CooldownSeconds = 5
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal 429 cooldown settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyRateLimit429CooldownSettings, string(data))
REDACTED

// GetOIDCConnectOAuthConfig 返回用于登录的“最终生效” OIDC 配置。
//
// 优先级：
// - 若对应系统设置键存在，则覆盖 config.yaml/env 的值
// - 否则回退到 config.yaml/env 的值
func (s *SettingService) GetOIDCConnectOAuthConfig(ctx context.Context) (config.OIDCConnectConfig, error) {
	if s == nil || s.cfg == nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.ServiceUnavailable("CONFIG_NOT_READY", "config not loaded")
REDACTED

	effective := s.cfg.OIDC

	keys := []string{
		SettingKeyOIDCConnectEnabled,
		SettingKeyOIDCConnectProviderName,
		SettingKeyOIDCConnectClientID,
		SettingKeyOIDCConnectClientSecret,
		SettingKeyOIDCConnectIssuerURL,
		SettingKeyOIDCConnectDiscoveryURL,
		SettingKeyOIDCConnectAuthorizeURL,
		SettingKeyOIDCConnectTokenURL,
		SettingKeyOIDCConnectUserInfoURL,
		SettingKeyOIDCConnectJWKSURL,
		SettingKeyOIDCConnectScopes,
		SettingKeyOIDCConnectRedirectURL,
		SettingKeyOIDCConnectFrontendRedirectURL,
		SettingKeyOIDCConnectTokenAuthMethod,
		SettingKeyOIDCConnectUsePKCE,
		SettingKeyOIDCConnectValidateIDToken,
		SettingKeyOIDCConnectAllowedSigningAlgs,
		SettingKeyOIDCConnectClockSkewSeconds,
		SettingKeyOIDCConnectRequireEmailVerified,
		SettingKeyOIDCConnectUserInfoEmailPath,
		SettingKeyOIDCConnectUserInfoIDPath,
		SettingKeyOIDCConnectUserInfoUsernamePath,
REDACTED
	settings, err := s.settingRepo.GetMultiple(ctx, keys)
	if err != nil {
		return config.OIDCConnectConfig{REDACTED, fmt.Errorf("get oidc connect settings: %w", err)
REDACTED

	if raw, ok := settings[SettingKeyOIDCConnectEnabled]; ok {
		effective.Enabled = raw == "true"
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectProviderName]; ok && strings.TrimSpace(v) != "" {
		effective.ProviderName = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectClientID]; ok && strings.TrimSpace(v) != "" {
		effective.ClientID = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectClientSecret]; ok && strings.TrimSpace(v) != "" {
		effective.ClientSecret = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectIssuerURL]; ok && strings.TrimSpace(v) != "" {
		effective.IssuerURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectDiscoveryURL]; ok && strings.TrimSpace(v) != "" {
		effective.DiscoveryURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectAuthorizeURL]; ok && strings.TrimSpace(v) != "" {
		effective.AuthorizeURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectTokenURL]; ok && strings.TrimSpace(v) != "" {
		effective.TokenURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoURL]; ok && strings.TrimSpace(v) != "" {
		effective.UserInfoURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectJWKSURL]; ok && strings.TrimSpace(v) != "" {
		effective.JWKSURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectScopes]; ok && strings.TrimSpace(v) != "" {
		effective.Scopes = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.RedirectURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectFrontendRedirectURL]; ok && strings.TrimSpace(v) != "" {
		effective.FrontendRedirectURL = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectTokenAuthMethod]; ok && strings.TrimSpace(v) != "" {
		effective.TokenAuthMethod = strings.ToLower(strings.TrimSpace(v))
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectUsePKCE]; ok {
		effective.UsePKCE = raw == "true"
REDACTED else {
		effective.UsePKCE = oidcUsePKCECompatibilityDefault(effective)
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectValidateIDToken]; ok {
		effective.ValidateIDToken = raw == "true"
REDACTED else {
		effective.ValidateIDToken = oidcValidateIDTokenCompatibilityDefault(effective)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectAllowedSigningAlgs]; ok && strings.TrimSpace(v) != "" {
		effective.AllowedSigningAlgs = strings.TrimSpace(v)
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectClockSkewSeconds]; ok && strings.TrimSpace(raw) != "" {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(raw)); parseErr == nil {
			effective.ClockSkewSeconds = parsed
	REDACTED
REDACTED
	if raw, ok := settings[SettingKeyOIDCConnectRequireEmailVerified]; ok {
		effective.RequireEmailVerified = raw == "true"
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoEmailPath]; ok {
		effective.UserInfoEmailPath = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoIDPath]; ok {
		effective.UserInfoIDPath = strings.TrimSpace(v)
REDACTED
	if v, ok := settings[SettingKeyOIDCConnectUserInfoUsernamePath]; ok {
		effective.UserInfoUsernamePath = strings.TrimSpace(v)
REDACTED

	if !effective.Enabled {
		return config.OIDCConnectConfig{REDACTED, infraerrors.NotFound("OAUTH_DISABLED", "oauth login is disabled")
REDACTED
	if strings.TrimSpace(effective.ProviderName) == "" {
		effective.ProviderName = "OIDC"
REDACTED
	if strings.TrimSpace(effective.ClientID) == "" {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client id not configured")
REDACTED
	if strings.TrimSpace(effective.IssuerURL) == "" {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth issuer url not configured")
REDACTED
	if strings.TrimSpace(effective.RedirectURL) == "" {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url not configured")
REDACTED
	if strings.TrimSpace(effective.FrontendRedirectURL) == "" {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url not configured")
REDACTED
	if !scopesContainOpenID(effective.Scopes) {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth scopes must contain openid")
REDACTED
	if effective.ClockSkewSeconds < 0 || effective.ClockSkewSeconds > 600 {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth clock skew must be between 0 and 600")
REDACTED

	if err := config.ValidateAbsoluteHTTPURL(effective.IssuerURL); err != nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth issuer url invalid")
REDACTED

	discoveryURL := strings.TrimSpace(effective.DiscoveryURL)
	if discoveryURL == "" {
		discoveryURL = oidcDefaultDiscoveryURL(effective.IssuerURL)
		effective.DiscoveryURL = discoveryURL
REDACTED
	if discoveryURL != "" {
		if err := config.ValidateAbsoluteHTTPURL(discoveryURL); err != nil {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth discovery url invalid")
	REDACTED
REDACTED

	needsDiscovery := strings.TrimSpace(effective.AuthorizeURL) == "" ||
		strings.TrimSpace(effective.TokenURL) == "" ||
		(effective.ValidateIDToken && strings.TrimSpace(effective.JWKSURL) == "")
	if needsDiscovery && discoveryURL != "" {
		metadata, resolveErr := oidcResolveProviderMetadata(ctx, discoveryURL)
		if resolveErr != nil {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth discovery resolve failed").WithCause(resolveErr)
	REDACTED
		if strings.TrimSpace(effective.AuthorizeURL) == "" {
			effective.AuthorizeURL = strings.TrimSpace(metadata.AuthorizationEndpoint)
	REDACTED
		if strings.TrimSpace(effective.TokenURL) == "" {
			effective.TokenURL = strings.TrimSpace(metadata.TokenEndpoint)
	REDACTED
		if strings.TrimSpace(effective.UserInfoURL) == "" {
			effective.UserInfoURL = strings.TrimSpace(metadata.UserInfoEndpoint)
	REDACTED
		if strings.TrimSpace(effective.JWKSURL) == "" {
			effective.JWKSURL = strings.TrimSpace(metadata.JWKSURI)
	REDACTED
REDACTED

	if strings.TrimSpace(effective.AuthorizeURL) == "" {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url not configured")
REDACTED
	if strings.TrimSpace(effective.TokenURL) == "" {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url not configured")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.AuthorizeURL); err != nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth authorize url invalid")
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.TokenURL); err != nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token url invalid")
REDACTED
	if v := strings.TrimSpace(effective.UserInfoURL); v != "" {
		if err := config.ValidateAbsoluteHTTPURL(v); err != nil {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth userinfo url invalid")
	REDACTED
REDACTED
	if effective.ValidateIDToken {
		if strings.TrimSpace(effective.JWKSURL) == "" {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth jwks url not configured")
	REDACTED
		if strings.TrimSpace(effective.AllowedSigningAlgs) == "" {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth signing algs not configured")
	REDACTED
REDACTED
	if v := strings.TrimSpace(effective.JWKSURL); v != "" {
		if err := config.ValidateAbsoluteHTTPURL(v); err != nil {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth jwks url invalid")
	REDACTED
REDACTED
	if err := config.ValidateAbsoluteHTTPURL(effective.RedirectURL); err != nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth redirect url invalid")
REDACTED
	if err := config.ValidateFrontendRedirectURL(effective.FrontendRedirectURL); err != nil {
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth frontend redirect url invalid")
REDACTED

	method := strings.ToLower(strings.TrimSpace(effective.TokenAuthMethod))
	switch method {
	case "", "client_secret_post", "client_secret_basic":
		if strings.TrimSpace(effective.ClientSecret) == "" {
			return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth client secret not configured")
	REDACTED
	case "none":
	default:
		return config.OIDCConnectConfig{REDACTED, infraerrors.InternalServer("OAUTH_CONFIG_INVALID", "oauth token_auth_method invalid")
REDACTED

	return effective, nil
REDACTED

func scopesContainOpenID(scopes string) bool {
	for _, scope := range strings.Fields(strings.ToLower(strings.TrimSpace(scopes))) {
		if scope == "openid" {
			return true
	REDACTED
REDACTED
	return false
REDACTED

type oidcProviderMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
REDACTED

func oidcDefaultDiscoveryURL(issuerURL string) string {
	issuerURL = strings.TrimSpace(issuerURL)
	if issuerURL == "" {
		return ""
REDACTED
	return strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
REDACTED

func oidcResolveProviderMetadata(ctx context.Context, discoveryURL string) (*oidcProviderMetadata, error) {
	discoveryURL = strings.TrimSpace(discoveryURL)
	if discoveryURL == "" {
		return nil, fmt.Errorf("discovery url is empty")
REDACTED

	resp, err := req.C().
		SetTimeout(15*time.Second).
		R().
		SetContext(ctx).
		SetHeader("Accept", "application/json").
		Get(discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("request discovery document: %w", err)
REDACTED
	if !resp.IsSuccessState() {
		return nil, fmt.Errorf("discovery request failed: status=%d", resp.StatusCode)
REDACTED

	metadata := &oidcProviderMetadata{REDACTED
	if err := json.Unmarshal(resp.Bytes(), metadata); err != nil {
		return nil, fmt.Errorf("parse discovery document: %w", err)
REDACTED
	return metadata, nil
REDACTED

// GetStreamTimeoutSettings 获取流超时处理配置
func (s *SettingService) GetStreamTimeoutSettings(ctx context.Context) (*StreamTimeoutSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyStreamTimeoutSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultStreamTimeoutSettings(), nil
	REDACTED
		return nil, fmt.Errorf("get stream timeout settings: %w", err)
REDACTED
	if value == "" {
		return DefaultStreamTimeoutSettings(), nil
REDACTED

	var settings StreamTimeoutSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultStreamTimeoutSettings(), nil
REDACTED

	// 验证并修正配置值
	if settings.TempUnschedMinutes < 1 {
		settings.TempUnschedMinutes = 1
REDACTED
	if settings.TempUnschedMinutes > 60 {
		settings.TempUnschedMinutes = 60
REDACTED
	if settings.ThresholdCount < 1 {
		settings.ThresholdCount = 1
REDACTED
	if settings.ThresholdCount > 10 {
		settings.ThresholdCount = 10
REDACTED
	if settings.ThresholdWindowMinutes < 1 {
		settings.ThresholdWindowMinutes = 1
REDACTED
	if settings.ThresholdWindowMinutes > 60 {
		settings.ThresholdWindowMinutes = 60
REDACTED

	// 验证 action
	switch settings.Action {
	case StreamTimeoutActionTempUnsched, StreamTimeoutActionError, StreamTimeoutActionNone:
		// valid
	default:
		settings.Action = StreamTimeoutActionTempUnsched
REDACTED

	return &settings, nil
REDACTED

// IsUngroupedKeySchedulingAllowed 查询是否允许未分组 Key 调度
func (s *SettingService) IsUngroupedKeySchedulingAllowed(ctx context.Context) bool {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyAllowUngroupedKeyScheduling)
	if err != nil {
		return false // fail-closed: 查询失败时默认不允许
REDACTED
	return value == "true"
REDACTED

// GetClaudeCodeVersionBounds 获取 Claude Code 版本号上下限要求
// 使用进程内 atomic.Value 缓存，60 秒 TTL，热路径零锁开销
// singleflight 防止缓存过期时 thundering herd
// 返回空字符串表示不做对应方向的版本检查
func (s *SettingService) GetClaudeCodeVersionBounds(ctx context.Context) (min, max string) {
	if cached, ok := versionBoundsCache.Load().(*cachedVersionBounds); ok {
		if time.Now().UnixNano() < cached.expiresAt {
			return cached.min, cached.max
	REDACTED
REDACTED
	// singleflight: 同一时刻只有一个 goroutine 查询 DB，其余复用结果
	type bounds struct{ min, max string REDACTED
	result, err, _ := versionBoundsSF.Do("version_bounds", func() (any, error) {
		// 二次检查，避免排队的 goroutine 重复查询
		if cached, ok := versionBoundsCache.Load().(*cachedVersionBounds); ok {
			if time.Now().UnixNano() < cached.expiresAt {
				return bounds{cached.min, cached.maxREDACTED, nil
		REDACTED
	REDACTED
		// 使用独立 context：断开请求取消链，避免客户端断连导致空值被长期缓存
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), versionBoundsDBTimeout)
		defer cancel()
		values, err := s.settingRepo.GetMultiple(dbCtx, []string{
			SettingKeyMinClaudeCodeVersion,
			SettingKeyMaxClaudeCodeVersion,
	REDACTED)
		if err != nil {
			// fail-open: DB 错误时不阻塞请求，但记录日志并使用短 TTL 快速重试
			slog.Warn("failed to get claude code version bounds setting, skipping version check", "error", err)
			versionBoundsCache.Store(&cachedVersionBounds{
				min:       "",
				max:       "",
				expiresAt: time.Now().Add(versionBoundsErrorTTL).UnixNano(),
		REDACTED)
			return bounds{"", ""REDACTED, nil
	REDACTED
		b := bounds{
			min: values[SettingKeyMinClaudeCodeVersion],
			max: values[SettingKeyMaxClaudeCodeVersion],
	REDACTED
		versionBoundsCache.Store(&cachedVersionBounds{
			min:       b.min,
			max:       b.max,
			expiresAt: time.Now().Add(versionBoundsCacheTTL).UnixNano(),
	REDACTED)
		return b, nil
REDACTED)
	if err != nil {
		return "", ""
REDACTED
	b, ok := result.(bounds)
	if !ok {
		return "", ""
REDACTED
	return b.min, b.max
REDACTED

// GetRectifierSettings 获取请求整流器配置
func (s *SettingService) GetRectifierSettings(ctx context.Context) (*RectifierSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyRectifierSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultRectifierSettings(), nil
	REDACTED
		return nil, fmt.Errorf("get rectifier settings: %w", err)
REDACTED
	if value == "" {
		return DefaultRectifierSettings(), nil
REDACTED

	var settings RectifierSettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultRectifierSettings(), nil
REDACTED

	return &settings, nil
REDACTED

// SetRectifierSettings 设置请求整流器配置
func (s *SettingService) SetRectifierSettings(ctx context.Context, settings *RectifierSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal rectifier settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyRectifierSettings, string(data))
REDACTED

// IsSignatureRectifierEnabled 判断签名整流是否启用（总开关 && 签名子开关）
func (s *SettingService) IsSignatureRectifierEnabled(ctx context.Context) bool {
	settings, err := s.GetRectifierSettings(ctx)
	if err != nil {
		return true // fail-open: 查询失败时默认启用
REDACTED
	return settings.Enabled && settings.ThinkingSignatureEnabled
REDACTED

// IsBudgetRectifierEnabled 判断 Budget 整流是否启用（总开关 && Budget 子开关）
func (s *SettingService) IsBudgetRectifierEnabled(ctx context.Context) bool {
	settings, err := s.GetRectifierSettings(ctx)
	if err != nil {
		return true // fail-open: 查询失败时默认启用
REDACTED
	return settings.Enabled && settings.ThinkingBudgetEnabled
REDACTED

// GetBetaPolicySettings 获取 Beta 策略配置
func (s *SettingService) GetBetaPolicySettings(ctx context.Context) (*BetaPolicySettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyBetaPolicySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultBetaPolicySettings(), nil
	REDACTED
		return nil, fmt.Errorf("get beta policy settings: %w", err)
REDACTED
	if value == "" {
		return DefaultBetaPolicySettings(), nil
REDACTED

	var settings BetaPolicySettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		return DefaultBetaPolicySettings(), nil
REDACTED

	return &settings, nil
REDACTED

// SetBetaPolicySettings 设置 Beta 策略配置
func (s *SettingService) SetBetaPolicySettings(ctx context.Context, settings *BetaPolicySettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	validActions := map[string]bool{
		BetaPolicyActionPass: true, BetaPolicyActionFilter: true, BetaPolicyActionBlock: true,
REDACTED
	validScopes := map[string]bool{
		BetaPolicyScopeAll: true, BetaPolicyScopeOAuth: true, BetaPolicyScopeAPIKey: true, BetaPolicyScopeBedrock: true,
REDACTED

	for i, rule := range settings.Rules {
		if rule.BetaToken == "" {
			return fmt.Errorf("rule[%d]: beta_token cannot be empty", i)
	REDACTED
		if !validActions[rule.Action] {
			return fmt.Errorf("rule[%d]: invalid action %q", i, rule.Action)
	REDACTED
		if !validScopes[rule.Scope] {
			return fmt.Errorf("rule[%d]: invalid scope %q", i, rule.Scope)
	REDACTED
		// Validate model_whitelist patterns
		for j, pattern := range rule.ModelWhitelist {
			trimmed := strings.TrimSpace(pattern)
			if trimmed == "" {
				return fmt.Errorf("rule[%d]: model_whitelist[%d] cannot be empty", i, j)
		REDACTED
			settings.Rules[i].ModelWhitelist[j] = trimmed
	REDACTED
		// Validate fallback_action
		if rule.FallbackAction != "" && !validActions[rule.FallbackAction] {
			return fmt.Errorf("rule[%d]: invalid fallback_action %q", i, rule.FallbackAction)
	REDACTED
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal beta policy settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyBetaPolicySettings, string(data))
REDACTED

// GetOpenAIFastPolicySettings 获取 OpenAI fast 策略配置
func (s *SettingService) GetOpenAIFastPolicySettings(ctx context.Context) (*OpenAIFastPolicySettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAIFastPolicySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return DefaultOpenAIFastPolicySettings(), nil
	REDACTED
		return nil, fmt.Errorf("get openai fast policy settings: %w", err)
REDACTED
	if value == "" {
		return DefaultOpenAIFastPolicySettings(), nil
REDACTED

	var settings OpenAIFastPolicySettings
	if err := json.Unmarshal([]byte(value), &settings); err != nil {
		// JSON 损坏时静默 fallback 到默认配置会让策略意外失效（管理员配
		// 置的 block/filter 规则被忽略）。记录 Warn 让运维能在出现异常
		// 行为时定位到 settings 表里的脏数据。
		slog.Warn("failed to unmarshal openai fast policy settings, falling back to defaults",
			"error", err,
			"key", SettingKeyOpenAIFastPolicySettings)
		return DefaultOpenAIFastPolicySettings(), nil
REDACTED

	return &settings, nil
REDACTED

// SetOpenAIFastPolicySettings 设置 OpenAI fast 策略配置
func (s *SettingService) SetOpenAIFastPolicySettings(ctx context.Context, settings *OpenAIFastPolicySettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	validActions := map[string]bool{
		BetaPolicyActionPass: true, BetaPolicyActionFilter: true, BetaPolicyActionBlock: true,
REDACTED
	validScopes := map[string]bool{
		BetaPolicyScopeAll: true, BetaPolicyScopeOAuth: true, BetaPolicyScopeAPIKey: true, BetaPolicyScopeBedrock: true,
REDACTED
	validTiers := map[string]bool{
		OpenAIFastTierAny: true, OpenAIFastTierPriority: true, OpenAIFastTierFlex: true,
REDACTED

	for i, rule := range settings.Rules {
		tier := strings.ToLower(strings.TrimSpace(rule.ServiceTier))
		if tier == "" {
			tier = OpenAIFastTierAny
	REDACTED
		if !validTiers[tier] {
			return fmt.Errorf("rule[%d]: invalid service_tier %q", i, rule.ServiceTier)
	REDACTED
		settings.Rules[i].ServiceTier = tier
		if !validActions[rule.Action] {
			return fmt.Errorf("rule[%d]: invalid action %q", i, rule.Action)
	REDACTED
		if !validScopes[rule.Scope] {
			return fmt.Errorf("rule[%d]: invalid scope %q", i, rule.Scope)
	REDACTED
		for j, pattern := range rule.ModelWhitelist {
			trimmed := strings.TrimSpace(pattern)
			if trimmed == "" {
				return fmt.Errorf("rule[%d]: model_whitelist[%d] cannot be empty", i, j)
		REDACTED
			settings.Rules[i].ModelWhitelist[j] = trimmed
	REDACTED
		if rule.FallbackAction != "" && !validActions[rule.FallbackAction] {
			return fmt.Errorf("rule[%d]: invalid fallback_action %q", i, rule.FallbackAction)
	REDACTED
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal openai fast policy settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyOpenAIFastPolicySettings, string(data))
REDACTED

// SetStreamTimeoutSettings 设置流超时处理配置
func (s *SettingService) SetStreamTimeoutSettings(ctx context.Context, settings *StreamTimeoutSettings) error {
	if settings == nil {
		return fmt.Errorf("settings cannot be nil")
REDACTED

	// 验证配置值
	if settings.TempUnschedMinutes < 1 || settings.TempUnschedMinutes > 60 {
		return fmt.Errorf("temp_unsched_minutes must be between 1-60")
REDACTED
	if settings.ThresholdCount < 1 || settings.ThresholdCount > 10 {
		return fmt.Errorf("threshold_count must be between 1-10")
REDACTED
	if settings.ThresholdWindowMinutes < 1 || settings.ThresholdWindowMinutes > 60 {
		return fmt.Errorf("threshold_window_minutes must be between 1-60")
REDACTED

	switch settings.Action {
	case StreamTimeoutActionTempUnsched, StreamTimeoutActionError, StreamTimeoutActionNone:
		// valid
	default:
		return fmt.Errorf("invalid action: %s", settings.Action)
REDACTED

	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("marshal stream timeout settings: %w", err)
REDACTED

	return s.settingRepo.Set(ctx, SettingKeyStreamTimeoutSettings, string(data))
REDACTED
