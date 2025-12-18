package service

import (
	"sub2api/internal/config"

	"github.com/google/wire"
)

// ProvidePricingService creates and initializes PricingService
func ProvidePricingService(cfg *config.Config) (*PricingService, error) {
	svc := NewPricingService(cfg)
	if err := svc.Initialize(); err != nil {
		// 价格服务初始化失败不应阻止启动,使用回退价格
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
REDACTED
	return svc, nil
REDACTED

// ProvideEmailQueueService creates EmailQueueService with default worker count
func ProvideEmailQueueService(emailService *EmailService) *EmailQueueService {
	return NewEmailQueueService(emailService, 3)
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
	ProvidePricingService,
	NewBillingService,
	NewBillingCacheService,
	NewAdminService,
	NewGatewayService,
	NewOAuthService,
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

	// Provide the Services container struct
	wire.Struct(new(Services), "*"),
)
