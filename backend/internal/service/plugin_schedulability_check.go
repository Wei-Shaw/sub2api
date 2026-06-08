package service

import (
	"context"
)

// PluginSchedulabilityChecker is the minimal interface the GatewayService
// uses to delegate schedulability checks to a plugin's CheckSchedulability
// RPC. The host calls Check during scheduling for plugin-owned platforms;
// if the plugin returns an error (including codes.Unimplemented), the host
// treats the account as schedulable (default pass).
//
// Wired via GatewayService.SetPluginSchedulabilityChecker from the plugin
// manager; nil means no plugin check available.
type PluginSchedulabilityChecker interface {
	// Check asks the plugin owning the given platform whether the account
	// is schedulable. Returns (schedulable, reason, error). An error signals
	// that the plugin does not implement the check or is unavailable, and
	// the caller should treat the account as schedulable.
	Check(ctx context.Context, account *Account, requestedModel, gatewayProtocol string) (schedulable bool, reason string, err error)
}
