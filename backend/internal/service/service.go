package service

import (
	"sub2api/internal/config"
	"sub2api/internal/repository"

	"github.com/redis/go-redis/v9"
)

// Services 服务集合容器
type Services struct {
	Auth         *AuthService
	User         *UserService
	ApiKey       *ApiKeyService
	Group        *GroupService
	Account      *AccountService
	Proxy        *ProxyService
	Redeem       *RedeemService
	Usage        *UsageService
	Pricing      *PricingService
	Billing      *BillingService
	BillingCache *BillingCacheService
	Admin        AdminService
	Gateway      *GatewayService
	OAuth        *OAuthService
	RateLimit    *RateLimitService
	AccountUsage *AccountUsageService
	AccountTest  *AccountTestService
	Setting      *SettingService
	Email        *EmailService
	EmailQueue   *EmailQueueService
	Turnstile    *TurnstileService
	Subscription *SubscriptionService
	Concurrency  *ConcurrencyService
	Identity     *IdentityService
REDACTED

// NewServices 创建所有服务实例
func NewServices(repos *repository.Repositories, rdb *redis.Client, cfg *config.Config) *Services {
	// 初始化价格服务
	pricingService := NewPricingService(cfg)
	if err := pricingService.Initialize(); err != nil {
		// 价格服务初始化失败不应阻止启动，使用回退价格
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
REDACTED

	// 初始化计费服务（依赖价格服务）
	billingService := NewBillingService(cfg, pricingService)

	// 初始化其他服务
	authService := NewAuthService(repos.User, cfg)
	userService := NewUserService(repos.User, cfg)
	apiKeyService := NewApiKeyService(repos.ApiKey, repos.User, repos.Group, repos.UserSubscription, rdb, cfg)
	groupService := NewGroupService(repos.Group)
	accountService := NewAccountService(repos.Account, repos.Group)
	proxyService := NewProxyService(repos.Proxy)
	usageService := NewUsageService(repos.UsageLog, repos.User)

	// 初始化订阅服务 (RedeemService 依赖)
	subscriptionService := NewSubscriptionService(repos)

	// 初始化兑换服务 (依赖订阅服务)
	redeemService := NewRedeemService(repos.RedeemCode, repos.User, subscriptionService, rdb)

	// 初始化Admin服务
	adminService := NewAdminService(repos)

	// 初始化OAuth服务（GatewayService依赖）
	oauthService := NewOAuthService(repos.Proxy)

	// 初始化限流服务
	rateLimitService := NewRateLimitService(repos, cfg)

	// 初始化计费缓存服务
	billingCacheService := NewBillingCacheService(rdb, repos.User, repos.UserSubscription)

	// 初始化账号使用量服务
	accountUsageService := NewAccountUsageService(repos, oauthService)

	// 初始化账号测试服务
	accountTestService := NewAccountTestService(repos, oauthService)

	// 初始化身份指纹服务
	identityService := NewIdentityService(rdb)

	// 初始化Gateway服务
	gatewayService := NewGatewayService(repos, rdb, cfg, oauthService, billingService, rateLimitService, billingCacheService, identityService)

	// 初始化设置服务
	settingService := NewSettingService(repos.Setting, cfg)
	emailService := NewEmailService(repos.Setting, rdb)

	// 初始化邮件队列服务
	emailQueueService := NewEmailQueueService(emailService, 3)

	// 初始化Turnstile服务
	turnstileService := NewTurnstileService(settingService)

	// 设置Auth服务的依赖（用于注册开关和邮件验证）
	authService.SetSettingService(settingService)
	authService.SetEmailService(emailService)
	authService.SetTurnstileService(turnstileService)
	authService.SetEmailQueueService(emailQueueService)

	// 初始化并发控制服务
	concurrencyService := NewConcurrencyService(rdb)

	// 注入计费缓存服务到需要失效缓存的服务
	redeemService.SetBillingCacheService(billingCacheService)
	subscriptionService.SetBillingCacheService(billingCacheService)
	SetAdminServiceBillingCache(adminService, billingCacheService)

	return &Services{
		Auth:         authService,
		User:         userService,
		ApiKey:       apiKeyService,
		Group:        groupService,
		Account:      accountService,
		Proxy:        proxyService,
		Redeem:       redeemService,
		Usage:        usageService,
		Pricing:      pricingService,
		Billing:      billingService,
		BillingCache: billingCacheService,
		Admin:        adminService,
		Gateway:      gatewayService,
		OAuth:        oauthService,
		RateLimit:    rateLimitService,
		AccountUsage: accountUsageService,
		AccountTest:  accountTestService,
		Setting:      settingService,
		Email:        emailService,
		EmailQueue:   emailQueueService,
		Turnstile:    turnstileService,
		Subscription: subscriptionService,
		Concurrency:  concurrencyService,
		Identity:     identityService,
REDACTED
REDACTED
