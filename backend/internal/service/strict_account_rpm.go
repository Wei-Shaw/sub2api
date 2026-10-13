package service

import (
	"context"
	"errors"
	"fmt"
)

// RPMReservationCache admits requests atomically in a rolling minute. Rejected
// attempts must not consume a reservation or extend the window.
type RPMReservationCache interface {
	ReserveRPM(context.Context, int64, int) (bool, error)
}

// MiniMax plan limits are hard provider limits, including sticky requests.
// Keep the existing soft/sticky RPM policy for the other providers.
func selectWithMiniMaxRPMReservation(ctx context.Context, cache RPMCache, excluded map[int64]struct{}, selectOnce func(map[int64]struct{}) (*AccountSelectionResult, error)) (*AccountSelectionResult, error) {
	exclusions := make(map[int64]struct{}, len(excluded))
	for id := range excluded {
		exclusions[id] = struct{}{}
	}
	blocked := 0
	for {
		selection, err := selectOnce(exclusions)
		if err != nil {
			if blocked > 0 && (errors.Is(err, ErrNoAvailableAccounts) || errors.Is(err, ErrNoAvailableCompactAccounts)) {
				return nil, fmt.Errorf("%w (filtered: rpm_rate_limited=%d)", err, blocked)
			}
			return selection, err
		}
		if selection == nil || selection.Account == nil || selection.Account.Platform != PlatformMiniMax || selection.Account.GetBaseRPM() <= 0 {
			return selection, nil
		}
		account := selection.Account
		reservation, ok := cache.(RPMReservationCache)
		if !ok {
			releaseRPMSelection(selection)
			return nil, fmt.Errorf("MiniMax RPM admission requires an atomic reservation cache")
		}
		allowed, err := reservation.ReserveRPM(ctx, account.ID, account.GetBaseRPM())
		if err != nil {
			releaseRPMSelection(selection)
			return nil, fmt.Errorf("MiniMax RPM admission cache unavailable: %w", err)
		}
		if allowed {
			return selection, nil
		}
		releaseRPMSelection(selection)
		if _, repeated := exclusions[account.ID]; repeated {
			return nil, fmt.Errorf("%w (filtered: rpm_rate_limited=%d)", ErrNoAvailableAccounts, blocked+1)
		}
		exclusions[account.ID] = struct{}{}
		blocked++
	}
}

func releaseRPMSelection(selection *AccountSelectionResult) {
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}
