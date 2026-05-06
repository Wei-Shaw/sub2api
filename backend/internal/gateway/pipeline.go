package gateway

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const defaultMaxFailovers = 10

// ParseRequestFunc extracts a ForwardRequest from the raw body.
type ParseRequestFunc func(body []byte) (*ForwardRequest, error)

// RecordUsageFunc persists usage after a successful forward.
type RecordUsageFunc func(ctx context.Context, account *service.Account, result *ForwardResult) error

// GatewayPipeline orchestrates the common request lifecycle:
//
//	parse -> channel mapping -> user slot -> billing check ->
//	session hash -> LOOP { select account -> account slot ->
//	billing consume -> provider.Forward } -> record usage -> close
type GatewayPipeline struct {
	registry       *ProviderRegistry
	gatewayService *service.GatewayService
	billingCache   *service.BillingCacheService
	concurrency    *service.ConcurrencyService
	settings       *service.SettingService
	maxFailovers   int
}

func NewGatewayPipeline(
	registry *ProviderRegistry,
	gw *service.GatewayService,
	billing *service.BillingCacheService,
	conc *service.ConcurrencyService,
	settings *service.SettingService,
	cfg *config.Config,
) *GatewayPipeline {
	maxFailovers := defaultMaxFailovers
	if cfg != nil && cfg.Gateway.MaxAccountSwitches > 0 {
		maxFailovers = cfg.Gateway.MaxAccountSwitches
	}
	return &GatewayPipeline{
		registry:       registry,
		gatewayService: gw,
		billingCache:   billing,
		concurrency:    conc,
		settings:       settings,
		maxFailovers:   maxFailovers,
	}
}

// Registry returns the provider registry for diagnostics.
func (p *GatewayPipeline) Registry() *ProviderRegistry { return p.registry }

// Execute runs the full gateway request lifecycle.
func (p *GatewayPipeline) Execute(
	c *gin.Context,
	protocol string,
	forcePlatform string,
	parse ParseRequestFunc,
	record RecordUsageFunc,
) error {
	req, slotRelease, err := p.executePreFlight(c, protocol, forcePlatform, parse)
	if err != nil {
		return err
	}
	defer slotRelease()
	defer req.BillingTicket.Close()

	result, err := p.selectAndForward(c.Request.Context(), c.Writer, req)
	if err != nil {
		return err
	}
	return p.recordUsage(c.Request.Context(), req, result, record)
}
