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

// --- Helpers per receiver ---

// publishPaymentOrderCreated emits payment.order.created. Safe when the
// publisher has not been wired (s.eventPublisher == nil).
func (s *PaymentService) publishPaymentOrderCreated(payload *pb.PaymentOrderCreated) {
	if s == nil || s.eventPublisher == nil {
		return
	}
	s.eventPublisher.PublishPaymentOrderCreated(payload)
}

// publishPaymentOrderFulfilled emits payment.order.fulfilled.
func (s *PaymentService) publishPaymentOrderFulfilled(payload *pb.PaymentOrderFulfilled) {
	if s == nil || s.eventPublisher == nil {
		return
	}
	s.eventPublisher.PublishPaymentOrderFulfilled(payload)
}

// publishGatewayModelInvoked emits gateway.model.invoked.
func (s *AntigravityGatewayService) publishGatewayModelInvoked(payload *pb.GatewayModelInvoked) {
	if s == nil || s.eventPublisher == nil {
		return
	}
	s.eventPublisher.PublishGatewayModelInvoked(payload)
}

// publishAuthUserRegistered emits auth.user.registered.
func (s *AuthService) publishAuthUserRegistered(payload *pb.AuthUserRegistered) {
	if s == nil || s.eventPublisher == nil {
		return
	}
	s.eventPublisher.PublishAuthUserRegistered(payload)
}

// publishAccountRateLimitTriggered emits account.rate_limit.triggered.
func (s *RateLimitService) publishAccountRateLimitTriggered(payload *pb.AccountRateLimitTriggered) {
	if s == nil || s.eventPublisher == nil {
		return
	}
	s.eventPublisher.PublishAccountRateLimitTriggered(payload)
}
