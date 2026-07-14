package service

type AccountRuntimeStateCleaner interface {
	DeleteAccountRuntimeState(accountID int64)
}

type compositeAccountRuntimeStateCleaner struct {
	usageCache               *UsageCache
	openAIGatewayService     *OpenAIGatewayService
	antigravityTokenProvider *AntigravityTokenProvider
	rateLimitService         *RateLimitService
}

func ProvideAccountRuntimeStateCleaner(
	usageCache *UsageCache,
	openAIGatewayService *OpenAIGatewayService,
	antigravityTokenProvider *AntigravityTokenProvider,
	rateLimitService *RateLimitService,
) AccountRuntimeStateCleaner {
	return &compositeAccountRuntimeStateCleaner{
		usageCache:               usageCache,
		openAIGatewayService:     openAIGatewayService,
		antigravityTokenProvider: antigravityTokenProvider,
		rateLimitService:         rateLimitService,
	}
}

func (c *compositeAccountRuntimeStateCleaner) DeleteAccountRuntimeState(accountID int64) {
	if c == nil || accountID <= 0 {
		return
	}
	if c.usageCache != nil {
		c.usageCache.DeleteAccount(accountID)
	}
	if c.openAIGatewayService != nil {
		c.openAIGatewayService.DeleteAccountRuntimeState(accountID)
	}
	if c.antigravityTokenProvider != nil {
		c.antigravityTokenProvider.DeleteAccountRuntimeState(accountID)
	}
	if c.rateLimitService != nil {
		c.rateLimitService.DeleteAccountRuntimeState(accountID)
	}
}
