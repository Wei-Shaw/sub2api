package service

import (
	"context"
)

// PluginModelSupportChecker is the minimal interface the GatewayService
// uses to delegate model-support checks to a plugin's IsModelSupported
// RPC. The host calls Check during scheduling; if the plugin returns an
// error (including codes.Unimplemented), the host falls back to the
// static account.IsModelSupported() check.
//
// Wired via GatewayService.SetPluginModelSupportChecker from the plugin
// manager; nil means no plugin check available.
type PluginModelSupportChecker interface {
	// Check asks the plugin owning the given platform whether the account
	// supports the requested model. Returns (supported, error). An error
	// signals that the plugin does not implement the check or is
	// unavailable, and the caller should fall back.
	Check(ctx context.Context, account *Account, requestedModel string) (supported bool, err error)
}
