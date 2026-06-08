// Package pricing implements the channel-management plugin's
// PricingExtension gRPC server (P3+P5+P4). The host calls
// ListPricingOverrides on plugin start to bootstrap its in-memory pricing
// cache and keeps a long-lived WatchPricingOverrides stream for incremental
// updates. AdjustCost is invoked per-request after the host computes the
// base cost; the channel-management plugin currently returns Modified=false
// because every pricing decision is already encoded in the override snapshot.
//
// Watch implementation: the server pushes the current snapshot as
// UPSERT events to a freshly-subscribed client, registers a Broker
// subscriber, and forwards every CRUD-driven event onto the stream. The
// Broker is owned by the plugin (constructed once in plugin.go) and shared
// with the ChannelService so its Create/Update/Delete handlers can publish
// after the underlying DB write succeeds. Sub-second freshness lives at
// the Broker boundary; the host's 5-minute periodic re-sync still acts
// as a safety net against silently dropped events (slow subscriber reaped
// by Broker, network partition, etc.).
package pricing

import (
	"context"
	"sync"
	"sync/atomic"

	sdkdecimalx "github.com/Wei-Shaw/sub2api/plugin-sdk/decimalx"
	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// channelLister is the minimal repo surface the server needs. ChannelService
// satisfies it via its embedded repository; tests pass a fake.
type channelLister interface {
	ListAll(ctx context.Context) ([]chService.Channel, error)
	GetGroupPlatforms(ctx context.Context, groupIDs []int64) (map[int64]string, error)
}

// accountStatsResolver lets the gRPC server delegate per-request account
// stats cost resolution to the channel service without import cycles.
// ChannelService satisfies this interface via its ResolveAccountStatsCost
// method.
type accountStatsResolver interface {
	ResolveAccountStatsCost(ctx context.Context, in chService.AccountStatsCostInput) (chService.AccountStatsCostResult, error)
}

// Server implements pluginsdk.PricingExtensionServer for the
// channel-management plugin.
//
// The server is constructed at process startup (before plugin.Init) so it
// can register on the SDK's grpc.Server, but the data dependency is wired
// lazily via SetLister once Init has built the repository. RPCs that arrive
// before SetLister return codes.Unavailable so the host's pricing extension
// client treats the plugin as "not ready yet" and retries via the periodic
// re-sync ticker.
//
// The Broker is wired the same way (NewServer constructs an empty Broker;
// callers may swap it via SetBroker before Init publishes the first event).
// ChannelService.SetBroker shares the same instance so CRUD-driven events
// reach every active Watch stream.
type Server struct {
	pb.UnimplementedPricingExtensionServer

	lister   atomic.Pointer[listerHolder]
	resolver atomic.Pointer[resolverHolder]
	broker   *Broker

	mu      sync.Mutex
	version string
}

// listerHolder wraps the channelLister so atomic.Pointer can store a typed
// nil-safe value without going through reflection.
type listerHolder struct {
	inner channelLister
}

// resolverHolder mirrors listerHolder for accountStatsResolver so a nil
// stored value (no resolver wired) is unambiguous.
type resolverHolder struct {
	inner accountStatsResolver
}

// NewServer returns a Server with no lister wired and a fresh in-process
// Broker. Call SetLister from Plugin.Init once the repository is
// available; share the Broker with ChannelService so CRUD events reach
// every active Watch stream.
func NewServer() *Server {
	return &Server{
		broker: NewBroker(),
	}
}

// Broker exposes the server's broker so ChannelService can publish
// CRUD-driven events on the same fanout WatchPricingOverrides reads
// from. Returns the shared instance — never nil for a value built via
// NewServer.
func (s *Server) Broker() *Broker {
	return s.broker
}

// SetLister wires the data source. Calling SetLister with a nil lister
// detaches the previous one; subsequent RPCs return Unavailable until the
// next non-nil call.
func (s *Server) SetLister(lister channelLister) {
	if lister == nil {
		s.lister.Store(nil)
		return
	}
	s.lister.Store(&listerHolder{inner: lister})
}

// SetAccountStatsResolver wires the per-request account-stats resolver
// (typically ChannelService). Until SetAccountStatsResolver is called
// with a non-nil value, ResolveAccountStatsCost returns has_cost=false
// so the host treats the plugin as "no opinion" and keeps account_stats_cost NULL.
func (s *Server) SetAccountStatsResolver(resolver accountStatsResolver) {
	if resolver == nil {
		s.resolver.Store(nil)
		return
	}
	s.resolver.Store(&resolverHolder{inner: resolver})
}

// loadLister returns the wired lister or nil. Hot path; cheap atomic load.
func (s *Server) loadLister() channelLister {
	h := s.lister.Load()
	if h == nil {
		return nil
	}
	return h.inner
}

// ListPricingOverrides walks every active channel, expands each pricing row
// per attached group, and emits one PricingOverride for each
// (group_id, platform, model) tuple.
func (s *Server) ListPricingOverrides(
	ctx context.Context,
	_ *pb.ListPricingOverridesRequest,
) (*pb.ListPricingOverridesResponse, error) {
	overrides, version, err := s.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.ListPricingOverridesResponse{
		Overrides: overrides,
		Version:   version,
	}, nil
}

// WatchPricingOverrides streams the current snapshot as UPSERT events to
// freshly-connected clients, then forwards every Broker event onto the
// same stream until the client disconnects or the broker reaps the
// subscriber for back-pressure.
//
// since_version is intentionally ignored: we always resend the full
// snapshot first so the host's cache converges even if it missed events
// during a reconnect window. The subsequent live-event tail keeps it
// fresh.
func (s *Server) WatchPricingOverrides(
	req *pb.WatchPricingOverridesRequest,
	stream pb.PricingExtension_WatchPricingOverridesServer,
) error {
	_ = req // since_version intentionally ignored — see godoc above.

	// Subscribe BEFORE we take the snapshot so any CRUD event that lands
	// during the snapshot window is buffered on the subscriber channel
	// and flushed after the initial flood. Worst case: the host receives
	// the same UPSERT twice — applyEvent in the host is idempotent so
	// duplicates are harmless.
	if s.broker == nil {
		return status.Error(codes.Internal,
			"channel-management: pricing broker not wired (NewServer not used)")
	}
	events, unsubscribe := s.broker.Subscribe()
	defer unsubscribe()

	if err := s.sendInitialSnapshot(stream); err != nil {
		return err
	}
	return s.streamLiveEvents(stream, events)
}

// sendInitialSnapshot loads the current override set and pushes every row
// onto the stream as an UPSERT event. Ctx-cancellation / Send errors bubble
// up so the caller exits the Watch RPC and the host reconnects.
func (s *Server) sendInitialSnapshot(stream pb.PricingExtension_WatchPricingOverridesServer) error {
	overrides, version, err := s.snapshot(stream.Context())
	if err != nil {
		return err
	}
	for _, o := range overrides {
		evt := &pb.PricingOverrideEvent{
			Op:       pb.PricingOverrideEvent_UPSERT,
			Override: o,
			Version:  version,
		}
		if err := stream.Send(evt); err != nil {
			return err
		}
	}
	return nil
}

// streamLiveEvents forwards broker events onto the stream until the ctx is
// cancelled, the broker drops us (slow consumer), or Send fails. Returning
// Unavailable on broker-drop signals the host to reconnect with backoff
// and re-sync via ListPricingOverrides.
func (s *Server) streamLiveEvents(
	stream pb.PricingExtension_WatchPricingOverridesServer,
	events <-chan *pb.PricingOverrideEvent,
) error {
	// Live tail. Exit on:
	//   - ctx cancellation (host disconnected / plugin shutdown)
	//   - subscriber chan closed (broker dropped us as slow consumer; the
	//     host will reconnect and trigger a fresh snapshot resend)
	//   - stream.Send error (host RPC error, treat as fatal for this
	//     stream; reconnect path mirrors the broker-drop case)
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case evt, ok := <-events:
			if !ok {
				return status.Error(codes.Unavailable,
					"channel-management: pricing broker dropped subscriber (slow consumer)")
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		}
	}
}

// AdjustCost is currently a typed no-op: returns Modified=false so the host
// keeps its own cost. The RPC channel is wired so future revisions can
// migrate channel-level multipliers from the host into the plugin without
// further proto changes.
func (s *Server) AdjustCost(
	_ context.Context,
	_ *pb.AdjustCostRequest,
) (*pb.AdjustCostResponse, error) {
	return &pb.AdjustCostResponse{
		Modified:         false,
		AdjustmentReason: "channel-management: AdjustCost not implemented yet",
	}, nil
}

// ResolveAccountStatsCost translates the proto request into the channel
// service's AccountStatsCostInput, runs the per-channel custom-rule
// resolver, and packages the result back into the proto response. When
// no resolver is wired (Init still pending) we return has_cost=false so
// the host treats the plugin as "no opinion" rather than failing the
// request.
func (s *Server) ResolveAccountStatsCost(
	ctx context.Context,
	req *pb.ResolveAccountStatsCostRequest,
) (*pb.ResolveAccountStatsCostResponse, error) {
	if req == nil {
		return &pb.ResolveAccountStatsCostResponse{}, nil
	}
	holder := s.resolver.Load()
	if holder == nil || holder.inner == nil {
		return &pb.ResolveAccountStatsCostResponse{
			HasCost:          false,
			ResolutionReason: "channel-management: account stats resolver not yet wired",
		}, nil
	}

	result, err := holder.inner.ResolveAccountStatsCost(ctx, decodeAccountStatsCostInput(req))
	if err != nil {
		// The host treats any error as "no opinion" — surface the cause
		// so operators can debug, but do not fail the gateway hot path.
		return nil, status.Errorf(codes.Internal,
			"channel-management: resolve account stats cost: %v", err)
	}
	if !result.HasCost {
		return &pb.ResolveAccountStatsCostResponse{HasCost: false}, nil
	}
	// T24: cost is now a decimal string. result.Cost is decimal.Decimal so we
	// emit the canonical String() representation; the host's
	// FromProtoString round-trips it back with no IEEE-754 rounding.
	return &pb.ResolveAccountStatsCostResponse{
		HasCost:          true,
		Cost:             sdkdecimalx.DecimalToProtoString(result.Cost),
		ResolutionReason: "channel-management: matched account stats pricing",
	}, nil
}

// decodeAccountStatsCostInput maps the proto request onto the service-layer
// input type. Splitting this out keeps ResolveAccountStatsCost's main body
// linear (validate -> resolve -> encode) without the token-struct plumbing
// obscuring the RPC contract.
//
// total_cost 是 T24 后的 decimal string；无值/解析失败按 0 处理，
// 由后续 ApplyPricingToAccountStats 分支决定是否触发"使用客户成本"路径。
func decodeAccountStatsCostInput(req *pb.ResolveAccountStatsCostRequest) chService.AccountStatsCostInput {
	tokens := chService.UsageTokens{}
	if t := req.GetTokens(); t != nil {
		tokens = chService.UsageTokens{
			InputTokens:         t.GetInputTokens(),
			OutputTokens:        t.GetOutputTokens(),
			CacheCreationTokens: t.GetCacheCreationTokens(),
			CacheReadTokens:     t.GetCacheReadTokens(),
			ImageOutputTokens:   t.GetImageCount(),
		}
	}
	requestCount := int(req.GetRequestCount())
	if requestCount <= 0 {
		requestCount = 1
	}
	return chService.AccountStatsCostInput{
		ChannelID:     req.GetChannelId(),
		AccountID:     req.GetAccountId(),
		GroupID:       req.GetGroupId(),
		UpstreamModel: req.GetUpstreamModel(),
		Tokens:        tokens,
		RequestCount:  requestCount,
		TotalCost:     sdkdecimalx.FromProtoStringOrZero(req.GetTotalCost()),
	}
}

// snapshot loads the current pricing override set and a monotonic version
// string. version is the latest channel UpdatedAt unix-nano; ties are
// resolved by ID so two channels with the same UpdatedAt do not collide.
func (s *Server) snapshot(ctx context.Context) ([]*pb.PricingOverride, string, error) {
	lister := s.loadLister()
	if lister == nil {
		return nil, "", status.Error(codes.Unavailable,
			"channel-management: PricingExtension lister not yet wired (plugin Init pending)")
	}

	channels, err := lister.ListAll(ctx)
	if err != nil {
		return nil, "", err
	}
	platforms, err := s.resolvePlatforms(ctx, lister, channels)
	if err != nil {
		return nil, "", err
	}

	out, maxUpdated := encodeChannelsAsOverrides(channels, platforms)
	return out, s.bumpVersion(maxUpdated), nil
}

// resolvePlatforms batch-loads the per-group platform identifier the snapshot
// encoder needs to filter out model pricing rows whose declared platform
// does not match the group's authoritative platform.
func (s *Server) resolvePlatforms(
	ctx context.Context,
	lister channelLister,
	channels []chService.Channel,
) (map[int64]string, error) {
	allGroups := collectGroupIDs(channels)
	if len(allGroups) == 0 {
		return map[int64]string{}, nil
	}
	return lister.GetGroupPlatforms(ctx, allGroups)
}
