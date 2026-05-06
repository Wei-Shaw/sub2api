package gateway

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ProvideProviderRegistry creates a ProviderRegistry and registers
// the three host-internal platform adapters (Anthropic, OpenAI,
// Antigravity). Phase 2+ will add dynamic registration from plugins.
func ProvideProviderRegistry(
	gw *service.GatewayService,
	openai *service.OpenAIGatewayService,
	antigravity *service.AntigravityGatewayService,
) *ProviderRegistry {
	reg := NewProviderRegistry()
	reg.Register(NewAnthropicProvider(gw))
	reg.Register(NewOpenAIProvider(openai))
	reg.Register(NewAntigravityProvider(antigravity))
	return reg
}

// ProvideGatewayPipeline creates a GatewayPipeline with all required
// service dependencies and the provider registry.
func ProvideGatewayPipeline(
	registry *ProviderRegistry,
	gw *service.GatewayService,
	billing *service.BillingCacheService,
	conc *service.ConcurrencyService,
	settings *service.SettingService,
	cfg *config.Config,
) *GatewayPipeline {
	return NewGatewayPipeline(registry, gw, billing, conc, settings, cfg)
}
