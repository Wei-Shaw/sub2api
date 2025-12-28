package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/wire"
)

// BuildInfo contains build information
type BuildInfo struct {
	Version   string
	BuildType string
REDACTED

// ProvidePricingService creates and initializes PricingService
func ProvidePricingService(cfg *config.Config, remoteClient PricingRemoteClient) (*PricingService, error) {
	svc := NewPricingService(cfg, remoteClient)
	if err := svc.Initialize(); err != nil {
		// Pricing service initialization failure should not block startup, use fallback prices
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
REDACTED
	return svc, nil
REDACTED

// ProvideUpdateService creates UpdateService with BuildInfo
func ProvideUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, buildInfo BuildInfo) *UpdateService {
	return NewUpdateService(cache, githubClient, buildInfo.Version, buildInfo.BuildType)
REDACTED

// ProvideEmailQueueService creates EmailQueueService with default worker count
func ProvideEmailQueueService(emailService *EmailService) *EmailQueueService {
	return NewEmailQueueService(emailService, 3)
REDACTED

// ProvideTokenRefreshService creates and starts TokenRefreshService
func ProvideTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	cfg *config.Config,
) *TokenRefreshService {
	svc := NewTokenRefreshService(accountRepo, oauthService, openaiOAuthService, geminiOAuthService, cfg)
	svc.Start()
	return svc
REDACTED

// ProvideTimingWheelService creates and starts TimingWheelService
func ProvideTimingWheelService() *TimingWheelService {
	svc := NewTimingWheelService()
	svc.Start()
	return svc
REDACTED

// ProvideDeferredService creates and starts DeferredService
func ProvideDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService) *DeferredService {
	svc := NewDeferredService(accountRepo, timingWheel, 10*time.Second)
	svc.Start()
	return svc
REDACTED

// ProviderSet is the Wire provider set for all services
var ProviderSet = wire.NewSet(
	// Core services
	NewAuthService,
	NewUserService,
	NewApiKeyService,
	NewGroupService,
	NewAccountService,
	NewProxyService,
	NewRedeemService,
	NewUsageService,
	NewDashboardService,
	ProvidePricingService,
	NewBillingService,
	NewBillingCacheService,
	NewAdminService,
	NewGatewayService,
	NewOpenAIGatewayService,
	NewOAuthService,
	NewOpenAIOAuthService,
	NewGeminiOAuthService,
	NewAntigravityOAuthService,
	NewGeminiTokenProvider,
	NewGeminiMessagesCompatService,
	NewRateLimitService,
	NewAccountUsageService,
	NewAccountTestService,
	NewSettingService,
	NewEmailService,
	ProvideEmailQueueService,
	NewTurnstileService,
	NewSubscriptionService,
	NewConcurrencyService,
	NewIdentityService,
	NewCRSSyncService,
	ProvideUpdateService,
	ProvideTokenRefreshService,
	ProvideTimingWheelService,
	ProvideDeferredService,
)
