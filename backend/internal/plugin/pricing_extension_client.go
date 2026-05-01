// Package plugin — pricing_extension_client.go
//
// PricingExtensionClient is the host-side proxy that talks to plugins
// implementing the PricingExtension service (P2 skeleton). It owns:
//
//   - Bootstrap List: full snapshot of pricing overrides on plugin start,
//     loaded into PricingOverrideCache before the gateway hot path uses it.
//   - Watch loop: long-lived stream that pushes incremental updates to
//     the cache. Reconnects with exponential backoff (1s..30s) when the
//     stream ends.
//   - Periodic safety re-sync: every reSyncInterval the client invokes
//     ListPricingOverrides again to catch silently-dropped events.
//   - AdjustCost: per-request RPC the BillingService hooks in after it
//     computes the base cost. Plugin response replaces the cost; errors
//     fall back to the host-computed value with a logged warning.
//
// Connection model: the client is bound to a single plugin (one per
// PluginInstance) and uses the existing gRPC connection PluginManager
// already opened to the plugin process. Stop() must be called when the
// plugin is disabled / restarted so the goroutine and stream exit cleanly.
//
// This file deliberately does NOT register itself with the PluginManager
// today — wiring is left to a follow-up phase. See PLUGIN-PRICING design
// doc for the full integration plan.

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"google.golang.org/grpc"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	"github.com/Wei-Shaw/sub2api/plugin-sdk/streamutil"
)

const (
	// pricingListTimeout caps a single ListPricingOverrides RPC. Plugins
	// returning bigger snapshots should chunk in a future revision.
	pricingListTimeout = 30 * time.Second
	// pricingAdjustTimeout caps a single AdjustCost RPC. The host falls
	// back to its own computed cost on timeout so the value is small
	// enough to keep gateway latency predictable.
	pricingAdjustTimeout = 1500 * time.Millisecond
	// pricingAccountStatsTimeout caps a ResolveAccountStatsCost RPC. The
	// host falls back to NULL on timeout (default formula) so this stays
	// small to keep recordUsageCore latency stable.
	pricingAccountStatsTimeout = 1500 * time.Millisecond
	// pricingReSyncInterval is the period at which the client invokes
	// ListPricingOverrides as a safety net against silently dropped Watch
	// events. 5 minutes matches the existing channel cache contract.
	pricingReSyncInterval = 5 * time.Minute

	// pricingWatchBackoffMin / Max bracket the reconnect backoff used by
	// streamutil.Loop when the Watch stream breaks. Same shape as the SDK's
	// jobs / settings / events / log_remote loops; multiplier and jitter
	// match those clients so a single bad plugin doesn't reconnect on a
	// noticeably different cadence than the SDK-side ones.
	pricingWatchBackoffMin  = 1 * time.Second
	pricingWatchBackoffMax  = 30 * time.Second
	pricingWatchMultiplier  = 2.0
	pricingWatchJitterRatio = 0.2
)

// PricingExtensionClient proxies host calls to a single plugin's
// PricingExtension server. It is safe to call AdjustCost concurrently;
// Start / Stop are not — typical lifecycle is one Start per plugin spawn
// and one Stop on shutdown.
type PricingExtensionClient struct {
	pluginName string
	cache      *service.PricingOverrideCache
	logger     *slog.Logger

	// parentCtx anchors the watch / resync loops. Decoupling them from the
	// Start ctx (which is a request-scoped HTTP / spawn ctx that gets
	// cancelled as soon as the caller returns) is what keeps the loops
	// alive past Start. Stop() owns explicit cancellation through the
	// derived loopCancel below.
	//
	// We keep this as a field rather than a Start argument so the
	// constructor caller (PluginManager.tryStartPricingExtension) can pin
	// it once with a long-lived context (today: context.Background(); in
	// the future: a manager-scoped shutdown ctx without losing trace).
	parentCtx context.Context

	// stub is set lazily on Start so a nil client (no plugin registered)
	// is a valid no-op state. AdjustCost short-circuits when stub is nil.
	mu     sync.RWMutex
	stub   pb.PricingExtensionClient
	cancel context.CancelFunc
	loopWG sync.WaitGroup
}

// NewPricingExtensionClient constructs the client. cache must be non-nil
// in production; tests may pass nil to exercise AdjustCost without the
// data layer.
//
// parentCtx is the long-lived ancestor of the watch / resync loops. Pass
// context.Background() when you want loops to run until Stop() is called;
// pass a manager-scoped shutdown ctx if you want a process-wide cancellation
// signal to tear them down for free. Nil is treated as Background.
//
// The caller is expected to invoke Start once a plugin's gRPC connection
// is ready, and Stop on plugin teardown. The constructor itself does not
// dial anything.
func NewPricingExtensionClient(
	parentCtx context.Context,
	pluginName string,
	cache *service.PricingOverrideCache,
	logger *slog.Logger,
) *PricingExtensionClient {
	if logger == nil {
		logger = slog.Default()
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	return &PricingExtensionClient{
		pluginName: pluginName,
		cache:      cache,
		logger:     logger.With("component", "pricing_ext_client", "plugin", pluginName),
		parentCtx:  parentCtx,
	}
}

// Start binds the client to the supplied gRPC connection and runs the
// initial List + background Watch + periodic re-sync. It returns once
// the initial List has completed (or failed); caller should treat a
// non-nil error as "plugin is up but PricingExtension not usable".
func (c *PricingExtensionClient) Start(ctx context.Context, conn grpc.ClientConnInterface) error {
	if c == nil {
		return errors.New("pricing extension client: nil receiver")
	}
	if conn == nil {
		return errors.New("pricing extension client: nil grpc conn")
	}

	stub := pb.NewPricingExtensionClient(conn)

	c.mu.Lock()
	c.stub = stub
	// Derive the loop ctx from parentCtx (set by the constructor), not from
	// the Start ctx. Start ctx is request-scoped and cancels as soon as the
	// caller returns; the loops must outlive that. The trace context (if any
	// is attached to parentCtx) flows through automatically via the gRPC
	// client interceptor — no manual metadata handling here.
	loopCtx, cancel := context.WithCancel(c.parentCtx)
	c.cancel = cancel
	c.mu.Unlock()

	// Initial snapshot. Errors are not fatal — the cache simply stays
	// empty and AdjustCost still works.
	if err := c.fullSync(ctx); err != nil {
		c.logger.Warn("initial ListPricingOverrides failed", "error", err)
	}

	// Background loops. Both check loopCtx and exit on Stop.
	c.loopWG.Add(2)
	go func() {
		defer c.loopWG.Done()
		c.watchLoop(loopCtx)
	}()
	go func() {
		defer c.loopWG.Done()
		c.reSyncLoop(loopCtx)
	}()
	return nil
}

// Stop tears down the watch + resync loops and waits for them to exit.
// Subsequent calls to AdjustCost return modified=false.
func (c *PricingExtensionClient) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.stub = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	c.loopWG.Wait()
}

// stubSnapshot returns the current stub under the read lock. Callers
// short-circuit when nil.
func (c *PricingExtensionClient) stubSnapshot() pb.PricingExtensionClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stub
}

// fullSync invokes ListPricingOverrides and replaces the cache contents.
// Ctx scopes only the RPC; the cache write is non-cancellable.
func (c *PricingExtensionClient) fullSync(ctx context.Context) error {
	stub := c.stubSnapshot()
	if stub == nil {
		return errors.New("pricing extension client: not started")
	}
	rpcCtx, cancel := context.WithTimeout(ctx, pricingListTimeout)
	defer cancel()

	resp, err := stub.ListPricingOverrides(rpcCtx, &pb.ListPricingOverridesRequest{})
	if err != nil {
		return fmt.Errorf("ListPricingOverrides: %w", err)
	}
	if c.cache == nil {
		return nil
	}
	overrides := make([]service.PricingOverride, 0, len(resp.GetOverrides()))
	for _, o := range resp.GetOverrides() {
		overrides = append(overrides, c.protoToOverride(o))
	}
	c.cache.ReplaceAll(overrides, resp.GetVersion())
	c.logger.Info("pricing override cache resynced",
		"count", len(overrides),
		"version", resp.GetVersion(),
	)
	return nil
}

// watchLoop opens WatchPricingOverrides and applies events to the cache.
// Reconnect cadence is delegated to streamutil.Loop so this host-side
// client shares one backoff schedule with the SDK-side jobs / settings /
// events / log_remote loops. Crucially, streamutil.Loop resets backoff to
// pricingWatchBackoffMin whenever runWatchOnce returns nil — that is the
// audit-S10 fix for "stream ran successfully for an hour, lost, reconnect
// waited 30s instead of 1s". Exits when loopCtx is cancelled.
func (c *PricingExtensionClient) watchLoop(loopCtx context.Context) {
	_ = streamutil.Loop(loopCtx, streamutil.Config{
		Name:        "pricing.watch",
		Initial:     pricingWatchBackoffMin,
		Max:         pricingWatchBackoffMax,
		Multiplier:  pricingWatchMultiplier,
		JitterRatio: pricingWatchJitterRatio,
		Logger:      c.logger,
	}, c.runWatchOnce)
}

// runWatchOnce opens a single Watch stream and processes events until
// the stream ends. Returns the terminating error (io.EOF for clean
// close).
func (c *PricingExtensionClient) runWatchOnce(loopCtx context.Context) error {
	stub := c.stubSnapshot()
	if stub == nil {
		return errors.New("pricing extension client: not started")
	}
	since := ""
	if c.cache != nil {
		since = c.cache.Version()
	}
	stream, err := stub.WatchPricingOverrides(loopCtx, &pb.WatchPricingOverridesRequest{
		SinceVersion: since,
	})
	if err != nil {
		return fmt.Errorf("open watch stream: %w", err)
	}
	for {
		evt, err := stream.Recv()
		if err != nil {
			return err
		}
		c.applyEvent(evt)
	}
}

// applyEvent translates a single proto event into a cache mutation.
func (c *PricingExtensionClient) applyEvent(evt *pb.PricingOverrideEvent) {
	if evt == nil || c.cache == nil {
		return
	}
	switch evt.GetOp() {
	case pb.PricingOverrideEvent_UPSERT:
		if o := evt.GetOverride(); o != nil {
			c.cache.Set(c.protoToOverride(o))
		}
	case pb.PricingOverrideEvent_DELETE:
		if k := evt.GetDeletedKey(); k != nil {
			c.cache.Delete(service.PricingOverrideKey{
				GroupID:  k.GetGroupId(),
				Platform: k.GetPlatform(),
				Model:    k.GetModel(),
			})
		}
	}
}

// reSyncLoop periodically full-syncs from the plugin to defend against
// dropped Watch events. Runs until loopCtx is cancelled.
func (c *PricingExtensionClient) reSyncLoop(loopCtx context.Context) {
	t := time.NewTicker(pricingReSyncInterval)
	defer t.Stop()
	for {
		select {
		case <-loopCtx.Done():
			return
		case <-t.C:
			if err := c.fullSync(loopCtx); err != nil {
				c.logger.Warn("pricing override resync failed", "error", err)
			}
		}
	}
}

// AdjustCost invokes PricingExtension.AdjustCost on the plugin. It is
// safe to call from any goroutine. Errors / nil-stub state are surfaced
// as Modified=false; callers must treat a non-nil error as "no
// adjustment, keep host-computed cost" and continue serving the
// request.
//
// Input / result types live in the service package so BillingService can
// invoke this method via service.PricingAdjuster without importing the
// plugin package (which would create an import cycle).
func (c *PricingExtensionClient) AdjustCost(ctx context.Context, in service.AdjustCostInput) (service.AdjustCostResult, error) {
	if c == nil {
		return service.AdjustCostResult{}, nil
	}
	stub := c.stubSnapshot()
	if stub == nil {
		return service.AdjustCostResult{}, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, pricingAdjustTimeout)
	defer cancel()

	req := &pb.AdjustCostRequest{
		Model:       in.Model,
		GroupId:     in.GroupID,
		UserId:      in.UserID,
		Platform:    in.Platform,
		ServiceTier: in.ServiceTier,
		RequestId:   in.RequestID,
		CoreCost: &pb.PricingCostBreakdown{
			Currency:       in.CoreCost.Currency,
			Total:          formatFloatAsDecimalString(in.CoreCost.Total),
			InputCost:      formatFloatAsDecimalString(in.CoreCost.InputCost),
			OutputCost:     formatFloatAsDecimalString(in.CoreCost.OutputCost),
			CacheWriteCost: formatFloatAsDecimalString(in.CoreCost.CacheWriteCost),
			CacheReadCost:  formatFloatAsDecimalString(in.CoreCost.CacheReadCost),
			ImageCost:      formatFloatAsDecimalString(in.CoreCost.ImageCost),
			BillingMode:    in.CoreCost.BillingMode,
		},
		Tokens: &pb.PricingUsageTokens{
			InputTokens:         in.Tokens.InputTokens,
			OutputTokens:        in.Tokens.OutputTokens,
			CacheCreationTokens: in.Tokens.CacheCreationTokens,
			CacheReadTokens:     in.Tokens.CacheReadTokens,
			ImageCount:          in.Tokens.ImageCount,
		},
	}

	resp, err := stub.AdjustCost(rpcCtx, req)
	if err != nil {
		return service.AdjustCostResult{}, err
	}
	if !resp.GetModified() {
		return service.AdjustCostResult{Modified: false, AdjustmentReason: resp.GetAdjustmentReason()}, nil
	}
	final := resp.GetFinalCost()
	if final == nil {
		// Plugin signalled modified=true but did not supply a cost. Treat
		// as a defective response and fall back to host cost.
		c.logger.Warn("adjust cost: modified=true but final_cost missing")
		return service.AdjustCostResult{Modified: false, AdjustmentReason: resp.GetAdjustmentReason()}, nil
	}
	return service.AdjustCostResult{
		Modified: true,
		FinalCost: service.AdjustCostBreakdown{
			Currency:       final.GetCurrency(),
			Total:          c.parseDecimalString(final.GetTotal()),
			InputCost:      c.parseDecimalString(final.GetInputCost()),
			OutputCost:     c.parseDecimalString(final.GetOutputCost()),
			CacheWriteCost: c.parseDecimalString(final.GetCacheWriteCost()),
			CacheReadCost:  c.parseDecimalString(final.GetCacheReadCost()),
			ImageCost:      c.parseDecimalString(final.GetImageCost()),
			BillingMode:    final.GetBillingMode(),
		},
		AdjustmentReason: resp.GetAdjustmentReason(),
	}, nil
}

// ResolveAccountStatsCost invokes PricingExtension.ResolveAccountStatsCost
// on the plugin. Returns HasCost=false on any failure (nil receiver,
// not-yet-started client, RPC error, timeout) so the gateway hot path
// can keep going with the default formula
// (total_cost × account_rate_multiplier).
//
// Input / result types live in the service package so GatewayService
// can invoke this method via service.AccountStatsCostResolver without
// importing the plugin package (avoids the existing import cycle).
func (c *PricingExtensionClient) ResolveAccountStatsCost(ctx context.Context, in service.AccountStatsCostInput) (service.AccountStatsCostResult, error) {
	if c == nil {
		return service.AccountStatsCostResult{}, nil
	}
	stub := c.stubSnapshot()
	if stub == nil {
		return service.AccountStatsCostResult{}, nil
	}

	rpcCtx, cancel := context.WithTimeout(ctx, pricingAccountStatsTimeout)
	defer cancel()

	resp, err := stub.ResolveAccountStatsCost(rpcCtx, &pb.ResolveAccountStatsCostRequest{
		ChannelId:     in.ChannelID,
		AccountId:     in.AccountID,
		GroupId:       in.GroupID,
		UpstreamModel: in.UpstreamModel,
		Tokens: &pb.PricingUsageTokens{
			InputTokens:         in.Tokens.InputTokens,
			OutputTokens:        in.Tokens.OutputTokens,
			CacheCreationTokens: in.Tokens.CacheCreationTokens,
			CacheReadTokens:     in.Tokens.CacheReadTokens,
			ImageCount:          in.Tokens.ImageOutputTokens,
		},
		RequestCount: int32(in.RequestCount),
		TotalCost:    formatFloatAsDecimalString(in.TotalCost),
		RequestId:    in.RequestID,
	})
	if err != nil {
		return service.AccountStatsCostResult{}, err
	}
	if !resp.GetHasCost() {
		return service.AccountStatsCostResult{HasCost: false, ResolutionReason: resp.GetResolutionReason()}, nil
	}
	return service.AccountStatsCostResult{
		HasCost:          true,
		Cost:             c.parseDecimalString(resp.GetCost()),
		ResolutionReason: resp.GetResolutionReason(),
	}, nil
}

// parseDecimalString parses a T24 proto decimal string into float64. Empty
// string maps to 0 (the cache treats zero as "unset"). A parse failure is
// logged and demoted to 0 so the gateway hot path never aborts on a single
// malformed plugin payload.
//
// TODO(plugin-grpc, T24 follow-up): the host's service.PricingOverride still
// stores float64 because BillingService / channel pricing layers consume
// float64. Upgrading those to decimal.Decimal end-to-end is a much larger
// change and intentionally out of scope here. The single bounded loss
// happens at this site only — the proto wire and the plugin both speak
// canonical decimal, and the host cache lives behind a 12-digit
// NUMERIC(20,12) DB schema, so the 53-bit IEEE-754 mantissa still covers
// every price magnitude in production. Switch to a decimal-everywhere
// model only after the corresponding host-side cleanup ships.
func (c *PricingExtensionClient) parseDecimalString(s string) float64 {
	if s == "" {
		return 0
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		c.logger.Warn("pricing override: bad decimal string", "raw", s, "error", err)
		return 0
	}
	return d.InexactFloat64()
}

// protoToOverride converts a proto PricingOverride into the cache value
// type. Callers feed the result into PricingOverrideCache.Set / ReplaceAll.
//
// T24: prices arrive as decimal strings; we parse via shopspring/decimal
// and demote to float64 once at this boundary. See parseDecimalString
// for why the host still uses float64 internally.
func (c *PricingExtensionClient) protoToOverride(o *pb.PricingOverride) service.PricingOverride {
	if o == nil {
		return service.PricingOverride{}
	}
	out := service.PricingOverride{
		BillingMode:      o.GetBillingMode(),
		InputPrice:       c.parseDecimalString(o.GetInputPrice()),
		OutputPrice:      c.parseDecimalString(o.GetOutputPrice()),
		CacheWritePrice:  c.parseDecimalString(o.GetCacheWritePrice()),
		CacheReadPrice:   c.parseDecimalString(o.GetCacheReadPrice()),
		ImageOutputPrice: c.parseDecimalString(o.GetImageOutputPrice()),
		PerRequestPrice:  c.parseDecimalString(o.GetPerRequestPrice()),
		SourcePlugin:     c.pluginName,
	}
	if k := o.GetKey(); k != nil {
		out.Key = service.PricingOverrideKey{
			GroupID:  k.GetGroupId(),
			Platform: k.GetPlatform(),
			Model:    k.GetModel(),
		}
	}
	if intervals := o.GetIntervals(); len(intervals) > 0 {
		out.Intervals = make([]service.PricingOverrideInterval, 0, len(intervals))
		for _, iv := range intervals {
			out.Intervals = append(out.Intervals, service.PricingOverrideInterval{
				MinTokens:        iv.GetMinTokens(),
				MaxTokens:        iv.GetMaxTokens(),
				InputPrice:       c.parseDecimalString(iv.GetInputPrice()),
				OutputPrice:      c.parseDecimalString(iv.GetOutputPrice()),
				CacheWritePrice:  c.parseDecimalString(iv.GetCacheWritePrice()),
				CacheReadPrice:   c.parseDecimalString(iv.GetCacheReadPrice()),
				ImageOutputPrice: c.parseDecimalString(iv.GetImageOutputPrice()),
				PerRequestPrice:  c.parseDecimalString(iv.GetPerRequestPrice()),
			})
		}
	}
	return out
}

// formatFloatAsDecimalString encodes a host-side float64 price as the T24
// canonical decimal string for the proto wire. Zero maps to the empty
// string ("not set") to match the legacy zero-as-unset semantics; any
// other value goes through decimal.NewFromFloat → String() so the wire
// representation stays readable and round-trips through the plugin's
// decimalx.FromProtoString without nasty trailing nines.
//
// The IEEE-754 noise that rides on float64 inputs ends up in the proto
// payload here — that is acceptable because the inputs are themselves
// host-computed float64 (BillingService.computeCost). See protoToOverride's
// TODO for the broader plan.
func formatFloatAsDecimalString(v float64) string {
	if v == 0 {
		return ""
	}
	return decimal.NewFromFloat(v).String()
}
