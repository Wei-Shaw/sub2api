// Package service — plugin_events.go
//
// Plugin Hook Phase B — service-side abstraction for fanning typed host
// events out to subscribed plugins.
//
// Why this lives in service rather than plugin:
//   - The plugin package already imports service (PluginSettingsService,
//     etc.). Pointing services at plugin.EventPublisher would close the
//     dependency cycle.
//   - The interface only needs to know about typed proto payloads, which
//     live in plugin-sdk/proto/pluginsdk — a leaf package both sides can
//     import freely.
//
// The plugin.EventPublisher type satisfies PluginEventPublisher; wire code
// passes the same instance to every business service that needs to emit
// events. Implementations MUST be non-blocking — host main paths cannot
// wait on plugin delivery.
//
// Per-receiver publish helpers (publishGatewayModelInvoked,
// publishAuthUserRegistered, publishAccountRateLimitTriggered) live next
// to their owning service file (antigravity_gateway_service.go,
// auth_service.go, ratelimit_service.go) so each service owns its own
// event surface and this file stays a pure interface declaration.
// PaymentService publish helpers were already migrated to plugins/payment/.
package service

import pb "github.com/Wei-Shaw/sub2api/plugin-sdk/proto/pluginsdk"

// PluginEventPublisher is the minimal surface a host service uses to emit
// Phase B events. A nil PluginEventPublisher (or a service whose publisher
// has not yet been wired) MUST be a safe no-op for every call site, so
// wiring order between PluginManager and the various services does not
// dictate a strict construction sequence.
type PluginEventPublisher interface {
	PublishPaymentOrderCreated(*pb.PaymentOrderCreated)
	PublishPaymentOrderFulfilled(*pb.PaymentOrderFulfilled)
	PublishGatewayModelInvoked(*pb.GatewayModelInvoked)
	PublishAuthUserRegistered(*pb.AuthUserRegistered)
	PublishAccountRateLimitTriggered(*pb.AccountRateLimitTriggered)
}
