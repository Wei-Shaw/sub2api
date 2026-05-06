package gateway

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ParseRequestFunc extracts a ForwardRequest from the raw body.
// Injected by the handler; each protocol has its own parse logic.
type ParseRequestFunc func(body []byte) (*ForwardRequest, error)

// RecordUsageFunc persists usage after a successful forward.
// Injected by the handler; each protocol maps ForwardResult to its
// native usage struct before writing.
type RecordUsageFunc func(ctx context.Context, account *service.Account, result *ForwardResult) error

// GatewayPipeline orchestrates the common request lifecycle:
//
//	parse -> channel mapping -> user slot -> billing check ->
//	session hash -> LOOP { select account -> account slot ->
//	billing consume -> provider.Forward } -> record usage -> close
//
// Platform differences are expressed through GatewayProvider (Forward /
// ShouldFailover) and the two handler-injected callbacks (parse / record).
type GatewayPipeline struct {
	registry        *ProviderRegistry
	gatewayService  *service.GatewayService
	billingCache    *service.BillingCacheService
	concurrency     *service.ConcurrencyService
	settings        *service.SettingService
}

// NewGatewayPipeline creates a pipeline with all required dependencies.
func NewGatewayPipeline(
	registry *ProviderRegistry,
	gw *service.GatewayService,
	billing *service.BillingCacheService,
	conc *service.ConcurrencyService,
	settings *service.SettingService,
) *GatewayPipeline {
	return &GatewayPipeline{
		registry:       registry,
		gatewayService: gw,
		billingCache:   billing,
		concurrency:    conc,
		settings:       settings,
	}
}

// Execute runs the full gateway request lifecycle. It is the single
// entry point that replaces the per-platform if/else chains in the
// gateway handler.
//
// protocol: input wire protocol ("anthropic" / "openai" / "gemini").
// forcePlatform: empty = scheduler picks; non-empty = pin to platform.
// parse: protocol-specific body parser.
// record: protocol-specific usage recorder.
func (p *GatewayPipeline) Execute(
	c *gin.Context,
	protocol string,
	forcePlatform string,
	parse ParseRequestFunc,
	record RecordUsageFunc,
) error {
	// TODO [M2]: read raw body from c.Request.Body

	// TODO [M2]: parse(body) -> ForwardRequest; set Protocol, ForcePlatform, RequestID

	// TODO [M2]: gatewayService.ResolveChannelMappingAndRestrict

	// TODO [M2]: concurrency.AcquireUserSlot (+ wait counter)

	// TODO [M2]: billingCache.PrepareBillingCheckForRequest

	// TODO [M2]: gatewayService.GenerateSessionHash + GetCachedSessionAccountID

	// TODO [M2]: enter retry loop (selectAndForward)

	// TODO [M2]: record(ctx, account, result)

	// TODO [M2]: billingTicket.Close (via defer)

	return errors.New("gateway: pipeline not yet implemented")
}

// selectAndForward runs the select-account -> forward -> failover loop.
// Extracted from Execute to keep each function focused.
func (p *GatewayPipeline) selectAndForward(
	ctx context.Context,
	w http.ResponseWriter,
	req *ForwardRequest,
) (*ForwardResult, error) {
	// TODO [M2]: loop up to failover limit:
	//   1. gatewayService.SelectAccount(groupID, protocol, forcePlatform, excludeIDs)
	//   2. concurrency.AcquireAccountSlot
	//   3. billingTicket.Consume(channelID, accountID)
	//   4. provider := registry.Get(account.Platform)
	//   5. result, err := provider.Forward(ctx, w, req)
	//   6. if err && provider.ShouldFailover: excludeIDs = append(...); continue
	//   7. if err: return nil, err (provider already wrote error response)
	//   8. break → return result, nil

	return nil, errors.New("gateway: selectAndForward not yet implemented")
}
