package service

import infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"

// Creation uses explicit settings, never probe snapshots copied through extra.
// Defaults belong to the personal UI; omitted API fields preserve old behavior.
func applyCreationBillingSettings(account *Account, input *CreateAccountInput) error {
	probe := input.ProbeEnabled != nil && *input.ProbeEnabled
	sync := input.RateSyncEnabled != nil && *input.RateSyncEnabled
	if sync {
		if input.ProbeEnabled != nil && !*input.ProbeEnabled {
			return infraerrors.BadRequest("UPSTREAM_BILLING_RATE_SYNC_REQUIRES_PROBE", "upstream billing rate sync requires upstream billing probe")
		}
		probe = true
	}
	if !probe {
		return nil
	}
	if !isUpstreamBillingProbeAccount(account) {
		return ErrUpstreamBillingProbeAccountInvalid
	}
	if account.Extra == nil {
		account.Extra = make(map[string]any)
	}
	account.Extra[UpstreamBillingProbeEnabledExtraKey] = true
	account.Extra[UpstreamBillingRateSyncEnabledExtraKey] = sync
	return nil
}
