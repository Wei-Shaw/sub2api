package service

// Services 服务集合容器
type Services struct {
	Auth          *AuthService
	User          *UserService
	ApiKey        *ApiKeyService
	Group         *GroupService
	Account       *AccountService
	Proxy         *ProxyService
	Redeem        *RedeemService
	Usage         *UsageService
	Pricing       *PricingService
	Billing       *BillingService
	BillingCache  *BillingCacheService
	Admin         AdminService
	Gateway       *GatewayService
	OpenAIGateway *OpenAIGatewayService
	OAuth         *OAuthService
	OpenAIOAuth   *OpenAIOAuthService
	RateLimit     *RateLimitService
	AccountUsage  *AccountUsageService
	AccountTest   *AccountTestService
	Setting       *SettingService
	Email         *EmailService
	EmailQueue    *EmailQueueService
	Turnstile     *TurnstileService
	Subscription  *SubscriptionService
	Concurrency   *ConcurrencyService
	Identity      *IdentityService
	Update        *UpdateService
	TokenRefresh  *TokenRefreshService
}
