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
	"io"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"

	"github.com/Wei-Shaw/sub2api/internal/service"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
)

const (
	// pricingListTimeout caps a single ListPricingOverrides RPC. Plugins
	// returning bigger snapshots should chunk in a future revision.
	pricingListTimeout = 30 * time.Second
	// pricingAdjustTimeout caps a single AdjustCost RPC. The host falls
	// back to its own computed cost on timeout so the value is small
	// enough to keep gateway latency predictable.
	pricingAdjustTimeout = 1500 * time.Millisecond
	// pricingReSyncInterval is the period at which the client invokes
	// ListPricingOverrides as a safety net against silently dropped Watch
	// events. 5 minutes matches the existing channel cache contract.
	pricingReSyncInterval = 5 * time.Minute

	// pricingWatchBackoffMin / Max bracket the exponential backoff used
	// when the Watch stream ends. Same shape as EventsExtension.
	pricingWatchBackoffMin = 1 * time.Second
	pricingWatchBackoffMax = 30 * time.Second
)

// PricingExtensionClient proxies host calls to a single plugin's
// PricingExtension server. It is safe to call AdjustCost concurrently;
// Start / Stop are not — typical lifecycle is one Start per plugin spawn
// and one Stop on shutdown.
type PricingExtensionClient struct {
	pluginName string
	cache      *service.PricingOverrideCache
	logger     *slog.Logger

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
// The caller is expected to invoke Start once a plugin's gRPC connection
// is ready, and Stop on plugin teardown. The constructor itself does not
// dial anything.
func NewPricingExtensionClient(
	pluginName string,
	cache *service.PricingOverrideCache,
	logger *slog.Logger,
) *PricingExtensionClient {
	if logger == nil {
		logger = slog.Default()
	}
	return &PricingExtensionClient{
		pluginName: pluginName,
		cache:      cache,
		logger:     logger.With("component", "pricing_ext_client", "plugin", pluginName),
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
	loopCtx, cancel := context.WithCancel(context.Background())
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
// On stream end it backs off exponentially and reconnects. Exits when
// loopCtx is cancelled.
func (c *PricingExtensionClient) watchLoop(loopCtx context.Context) {
	backoff := pricingWatchBackoffMin
	for {
		if loopCtx.Err() != nil {
			return
		}
		err := c.runWatchOnce(loopCtx)
		if loopCtx.Err() != nil {
			return
		}
		if err != nil && !errors.Is(err, io.EOF) {
			c.logger.Warn("watch pricing overrides ended", "error", err, "backoff", backoff)
		}
		select {
		case <-loopCtx.Done():
			return
		case <-time.After(backoff):
		}
		// Exponential backoff capped at pricingWatchBackoffMax.
		backoff *= 2
		if backoff > pricingWatchBackoffMax {
			backoff = pricingWatchBackoffMax
		}
	}
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
			Total:          in.CoreCost.Total,
			InputCost:      in.CoreCost.InputCost,
			OutputCost:     in.CoreCost.OutputCost,
			CacheWriteCost: in.CoreCost.CacheWriteCost,
			CacheReadCost:  in.CoreCost.CacheReadCost,
			ImageCost:      in.CoreCost.ImageCost,
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
			Total:          final.GetTotal(),
			InputCost:      final.GetInputCost(),
			OutputCost:     final.GetOutputCost(),
			CacheWriteCost: final.GetCacheWriteCost(),
			CacheReadCost:  final.GetCacheReadCost(),
			ImageCost:      final.GetImageCost(),
			BillingMode:    final.GetBillingMode(),
		},
		AdjustmentReason: resp.GetAdjustmentReason(),
	}, nil
}

// protoToOverride converts a proto PricingOverride into the cache value
// type. Callers feed the result into PricingOverrideCache.Set / ReplaceAll.
func (c *PricingExtensionClient) protoToOverride(o *pb.PricingOverride) service.PricingOverride {
	if o == nil {
		return service.PricingOverride{}
	}
	out := service.PricingOverride{
		BillingMode:      o.GetBillingMode(),
		InputPrice:       o.GetInputPrice(),
		OutputPrice:      o.GetOutputPrice(),
		CacheWritePrice:  o.GetCacheWritePrice(),
		CacheReadPrice:   o.GetCacheReadPrice(),
		ImageOutputPrice: o.GetImageOutputPrice(),
		PerRequestPrice:  o.GetPerRequestPrice(),
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
				InputPrice:       iv.GetInputPrice(),
				OutputPrice:      iv.GetOutputPrice(),
				CacheWritePrice:  iv.GetCacheWritePrice(),
				CacheReadPrice:   iv.GetCacheReadPrice(),
				ImageOutputPrice: iv.GetImageOutputPrice(),
				PerRequestPrice:  iv.GetPerRequestPrice(),
			})
		}
	}
	return out
}
