package service

import (
	"context"
)

// PluginSchedulingHintsProvider is the minimal interface the GatewayService
// uses to delegate dynamic scheduling-weight adjustments to plugins.
// The host calls GetHints during scheduling with a batch of candidate
// accounts for a given platform; the plugin returns per-account hints
// (priority modifier, temporary unavailability).
//
// Wired via GatewayService.SetPluginSchedulingHintsProvider from the
// plugin manager; nil means no plugin hints available.
type PluginSchedulingHintsProvider interface {
	// GetHints asks the plugin owning the given platform for dynamic
	// scheduling hints on the candidate accounts. Returns a map of
	// account ID → hint. An error (including codes.Unimplemented)
	// signals that the plugin does not implement this, and the caller
	// should silently skip hint application.
	GetHints(ctx context.Context, platform string, accounts []*Account, gatewayProtocol string, requestedModel string) (map[int64]SchedulingHintResult, error)
}

// SchedulingHintResult holds the per-account scheduling hint returned
// by a plugin.
type SchedulingHintResult struct {
	PriorityModifier       int32
	TemporarilyUnavailable bool
	Reason                 string
}
