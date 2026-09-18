package service

import (
	mathrand "math/rand"
	"time"
)

// projectedAccountPressure estimates the normalized pressure after admitting
// one more request. LoadFactor is scheduling capacity only; Concurrency remains
// the hard admission limit. Queue depth is included explicitly here so it is
// not hidden inside LoadRate.
func projectedAccountPressure(candidate accountWithLoad) float64 {
	if candidate.account == nil {
		return 1e9
	}
	capacity := candidate.account.EffectiveLoadFactor()
	if capacity <= 0 {
		capacity = 1
	}
	current, waiting, loadRate := 0, 0, 0
	if candidate.loadInfo != nil {
		current = candidate.loadInfo.CurrentConcurrency
		waiting = candidate.loadInfo.WaitingCount
		loadRate = candidate.loadInfo.LoadRate
	}
	activePressure := float64(current) / float64(capacity)
	if reported := float64(loadRate) / 100.0; reported > activePressure {
		activePressure = reported
	}
	return activePressure + float64(waiting+1)/float64(capacity)
}

func accountHasHardCapacity(candidate accountWithLoad) bool {
	if candidate.account == nil {
		return false
	}
	if candidate.account.Concurrency <= 0 || candidate.loadInfo == nil {
		return true
	}
	return candidate.loadInfo.CurrentConcurrency < candidate.account.Concurrency
}

func accountWithLoadBetter(left, right accountWithLoad, preferOAuth bool) bool {
	leftPressure := projectedAccountPressure(left)
	rightPressure := projectedAccountPressure(right)
	if leftPressure != rightPressure {
		return leftPressure < rightPressure
	}
	if left.loadInfo != nil && right.loadInfo != nil && left.loadInfo.WaitingCount != right.loadInfo.WaitingCount {
		return left.loadInfo.WaitingCount < right.loadInfo.WaitingCount
	}
	if preferOAuth && left.account.Type != right.account.Type {
		return left.account.Type == AccountTypeOAuth
	}
	switch {
	case left.account.LastUsedAt == nil && right.account.LastUsedAt != nil:
		return true
	case left.account.LastUsedAt != nil && right.account.LastUsedAt == nil:
		return false
	case left.account.LastUsedAt != nil && right.account.LastUsedAt != nil && !left.account.LastUsedAt.Equal(*right.account.LastUsedAt):
		return left.account.LastUsedAt.Before(*right.account.LastUsedAt)
	default:
		return mathrand.Intn(2) == 0
	}
}

// selectByPowerOfTwoChoices samples two candidates instead of making every
// request chase the globally lowest load snapshot. This reduces herd behavior
// while retaining near least-request quality under stale distributed state.
func selectByPowerOfTwoChoices(candidates []accountWithLoad, preferOAuth bool) *accountWithLoad {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		selected := candidates[0]
		return &selected
	}
	leftIndex := mathrand.Intn(len(candidates))
	rightIndex := mathrand.Intn(len(candidates) - 1)
	if rightIndex >= leftIndex {
		rightIndex++
	}
	selected := candidates[rightIndex]
	if accountWithLoadBetter(candidates[leftIndex], selected, preferOAuth) {
		selected = candidates[leftIndex]
	}
	return &selected
}

// buildPowerOfTwoSelectionOrder preserves hard priority tiers and optional
// use-it-or-lose-it reset ordering, but distributes probes inside the active
// tier with P2C. Lower tiers are considered only after the active tier cannot
// acquire a hard slot.
func buildPowerOfTwoSelectionOrder(available []accountWithLoad, preferOAuth, preferSoonestReset bool) []accountWithLoad {
	remaining := append([]accountWithLoad(nil), available...)
	order := make([]accountWithLoad, 0, len(remaining))
	for len(remaining) > 0 {
		pool := filterByMinPriority(remaining)
		if preferSoonestReset {
			pool = filterBySoonestReset(pool)
		}
		selected := selectByPowerOfTwoChoices(pool, preferOAuth)
		if selected == nil || selected.account == nil {
			break
		}
		order = append(order, *selected)
		selectedID := selected.account.ID
		next := remaining[:0]
		for _, candidate := range remaining {
			if candidate.account == nil || candidate.account.ID != selectedID {
				next = append(next, candidate)
			}
		}
		remaining = next
	}
	return order
}

func soonestResetAt(account *Account) *time.Time {
	if account == nil || account.SessionWindowEnd == nil || !time.Now().Before(*account.SessionWindowEnd) {
		return nil
	}
	return account.SessionWindowEnd
}
