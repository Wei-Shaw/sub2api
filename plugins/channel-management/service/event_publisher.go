// event_publisher.go — service-side interface for plugin pricing events.
//
// ChannelService publishes "channel changed" notifications after every
// successful CRUD commit. The interface keeps the service decoupled from
// the proto/gRPC machinery: the implementation lives in
// plugins/channel-management/internal/pricing and converts each Channel
// snapshot into the per-(group, platform, model) PricingOverrideEvent
// the host's PricingExtension Watch loop consumes.
//
// All methods are non-blocking and best-effort: implementations must
// neither block CRUD nor return errors, since the host's 5-minute
// re-sync is the safety net for missed events.

package service

// PricingEventPublisher is the broker interface ChannelService uses to
// announce channel mutations after a successful DB commit.
//
// Method semantics:
//
//   - PublishChannelUpsert: emits one UPSERT event per active
//     (group, platform, model) tuple. Inactive channels degrade to
//     PublishChannelDelete-equivalent semantics so listeners observe
//     the cache-empty end state.
//   - PublishChannelDelete: emits one DELETE event per
//     (group, platform, model) tuple in the supplied snapshot,
//     irrespective of channel status. Caller is responsible for
//     supplying the pre-delete snapshot when removing a channel.
//
// Both methods accept the channel's group→platform map so the publisher
// can resolve which platform each group lives in. Pass an empty map
// when the lookup is unavailable; the publisher will skip groups whose
// platform is unknown (cache will fall back to the host's 5-minute
// re-sync).
//
// A nil PricingEventPublisher is a valid no-op: ChannelService.SetEventPublisher(nil)
// disables broadcasting (useful for unit tests and for the dev/plugin-
// embedded-in-host mode where the broker is not constructed).
type PricingEventPublisher interface {
	PublishChannelUpsert(channel *Channel, groupPlatforms map[int64]string)
	PublishChannelDelete(channel *Channel, groupPlatforms map[int64]string)
}
