// publisher.go — Broker → service.PricingEventPublisher adapter.
//
// The adapter sits between ChannelService (which announces "channel
// committed") and the in-process Broker (which fans events out to active
// WatchPricingOverrides streams). It owns the encoding from a Channel
// snapshot to one PricingOverrideEvent per (group, platform, model)
// tuple, mirroring the Server.snapshot expansion exactly so cache state
// converges whether it was bootstrapped via List or live-updated via
// Watch.
//
// Event format:
//
//   - UPSERT: Op=UPSERT, override populated, version=now-unix-nano
//     (string-encoded) for monotonic ordering across publish calls.
//   - DELETE: Op=DELETE, deleted_key populated, override left nil.
//
// version is monotonic only at the granularity of a single broker
// instance (not across plugin restarts). The host reconciles via a
// fresh ListPricingOverrides on reconnect anyway, so cross-restart
// monotonicity is not required.

package pricing

import (
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"
	chService "github.com/Wei-Shaw/sub2api/plugins/channel-management/service"
)

// Publisher adapts a *Broker to service.PricingEventPublisher. Construct
// via NewPublisher or Server.Publisher() — both share the underlying
// Broker so events published from CRUD reach every active Watch stream.
type Publisher struct {
	broker *Broker

	// versionCounter ensures successive Publish calls within the same
	// nanosecond emit strictly-increasing version strings. The host's
	// PricingOverrideCache stores version per ReplaceAll only, so the
	// per-event value is purely advisory; we still keep it monotonic for
	// debugging and future "resume since version" support.
	versionCounter atomic.Int64
}

// NewPublisher returns a Publisher backed by broker. broker must be
// non-nil; pass Server.Broker() in plugin wiring.
func NewPublisher(broker *Broker) *Publisher {
	return &Publisher{broker: broker}
}

// Publisher returns a service.PricingEventPublisher backed by the
// server's broker. Plugin wiring calls this once and hands the result
// to ChannelService.SetEventPublisher.
func (s *Server) Publisher() *Publisher {
	return NewPublisher(s.broker)
}

// PublishChannelUpsert fans UPSERT events out for every active
// (group, platform, model) tuple in channel. Inactive channels are
// translated to DELETE events for the same tuple set so cache state
// matches the post-commit DB.
//
// groupPlatforms maps group_id → platform; groups missing from the map
// are skipped and the host falls back to the 5-minute re-sync. Passing
// nil disables fanout entirely.
func (p *Publisher) PublishChannelUpsert(channel *chService.Channel, groupPlatforms map[int64]string) {
	if p == nil || p.broker == nil || channel == nil {
		return
	}
	if !channel.IsActive() {
		p.PublishChannelDelete(channel, groupPlatforms)
		return
	}
	version := p.nextVersion()
	for _, gid := range channel.GroupIDs {
		platform := groupPlatforms[gid]
		if platform == "" {
			continue
		}
		for j := range channel.ModelPricing {
			row := &channel.ModelPricing[j]
			if !platformMatches(platform, row.Platform) {
				continue
			}
			for _, model := range row.Models {
				modelLower := strings.ToLower(strings.TrimSpace(model))
				if modelLower == "" {
					continue
				}
				p.broker.Publish(&pb.PricingOverrideEvent{
					Op:       pb.PricingOverrideEvent_UPSERT,
					Override: encodeOverride(gid, platform, modelLower, row),
					Version:  version,
				})
			}
		}
	}
}

// PublishChannelDelete fans DELETE events out for every
// (group, platform, model) tuple captured in channel — irrespective of
// channel status. Caller passes the pre-mutation snapshot so the
// listener can fully evict the cache rows the channel previously owned.
//
// groupPlatforms / nil semantics match PublishChannelUpsert.
func (p *Publisher) PublishChannelDelete(channel *chService.Channel, groupPlatforms map[int64]string) {
	if p == nil || p.broker == nil || channel == nil {
		return
	}
	version := p.nextVersion()
	for _, gid := range channel.GroupIDs {
		platform := groupPlatforms[gid]
		if platform == "" {
			continue
		}
		for j := range channel.ModelPricing {
			row := &channel.ModelPricing[j]
			if !platformMatches(platform, row.Platform) {
				continue
			}
			for _, model := range row.Models {
				modelLower := strings.ToLower(strings.TrimSpace(model))
				if modelLower == "" {
					continue
				}
				p.broker.Publish(&pb.PricingOverrideEvent{
					Op: pb.PricingOverrideEvent_DELETE,
					DeletedKey: &pb.PricingOverrideKey{
						GroupId:  gid,
						Platform: platform,
						Model:    modelLower,
					},
					Version: version,
				})
			}
		}
	}
}

// nextVersion returns a strictly-increasing decimal-string version. Ties
// at nanosecond resolution are broken by an in-process counter so two
// adjacent publishes never share a version.
func (p *Publisher) nextVersion() string {
	now := time.Now().UnixNano()
	// Counter ensures monotonicity even if time.Now() collides on
	// Windows' 100ns clock granularity (rare but observed).
	c := p.versionCounter.Add(1)
	if c > now {
		now = c
	} else {
		p.versionCounter.Store(now)
	}
	return formatVersion(now)
}
