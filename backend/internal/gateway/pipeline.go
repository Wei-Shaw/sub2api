package gateway

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	defaultMaxFailovers       = 10
	defaultMaxFailoversGemini = 3
)

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
	registry           *ProviderRegistry
	gatewayService     *service.GatewayService
	billingCache       *service.BillingCacheService
	concurrency        *service.ConcurrencyService
	settings           *service.SettingService
	contentInterceptor ContentInterceptor
	maxFailovers       int
	maxFailoversGemini int
}

// SetContentInterceptor wires the content interception hook (optional).
func (p *GatewayPipeline) SetContentInterceptor(ci ContentInterceptor) {
	p.contentInterceptor = ci
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
	maxFailoversGemini := defaultMaxFailoversGemini
	if cfg != nil {
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxFailovers = cfg.Gateway.MaxAccountSwitches
		}
		if cfg.Gateway.MaxAccountSwitchesGemini > 0 {
			maxFailoversGemini = cfg.Gateway.MaxAccountSwitchesGemini
		}
	}
	return &GatewayPipeline{
		registry:           registry,
		gatewayService:     gw,
		billingCache:       billing,
		concurrency:        conc,
		settings:           settings,
		maxFailovers:       maxFailovers,
		maxFailoversGemini: maxFailoversGemini,
	}
}

// Registry returns the provider registry for diagnostics.
func (p *GatewayPipeline) Registry() *ProviderRegistry { return p.registry }

// effectiveMaxFailovers returns the failover limit for the given protocol.
// Gemini uses a lower limit due to stricter API rate limits.
func (p *GatewayPipeline) effectiveMaxFailovers(protocol string) int {
	if protocol == ProtocolGemini {
		return p.maxFailoversGemini
	}
	return p.maxFailovers
}

// Execute runs the full gateway request lifecycle.
func (p *GatewayPipeline) Execute(
	c *gin.Context,
	protocol string,
	forcePlatform string,
	parse ParseRequestFunc,
	record RecordUsageFunc,
) error {
	requestStart := time.Now()

	req, slotRelease, err := p.executePreFlight(c, protocol, forcePlatform, parse)
	if err != nil {
		return err
	}
	defer slotRelease()
	defer req.BillingTicket.Close()

	// Auth latency: time from request start through pre-flight (auth + billing)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())
	routingStart := time.Now()

	result, err := p.selectAndForward(c.Request.Context(), c.Writer, req)
	if err != nil {
		return err
	}

	// Routing latency: time spent in account selection + forwarding
	forwardDurationMs := time.Since(routingStart).Milliseconds()
	service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())

	// Response latency: forward duration minus upstream processing time
	var responseLatencyMs int64
	if result != nil && result.FirstTokenMs != nil {
		upstreamMs := int64(*result.FirstTokenMs)
		if forwardDurationMs > upstreamMs {
			responseLatencyMs = forwardDurationMs - upstreamMs
		}
		service.SetOpsLatencyMs(c, service.OpsTimeToFirstTokenMsKey, upstreamMs)
	}
	service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

	return p.recordUsage(c.Request.Context(), req, result, record)
}
