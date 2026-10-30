package service

import (
	"sort"
	"time"
)

// ReconcileAccountSchedulingSlots is the deterministic state transition used by
// the serialized repository transaction. Live request counts include every
// process. Draining identities retain capacity until their last request ends.
func ReconcileAccountSchedulingSlots(candidates []AccountSchedulingCandidate, previous []AccountSchedulingSlot, live map[string]int, limit int, now time.Time) ([]AccountSchedulingSlot, map[int64]bool) {
	byIdentity := map[string][]AccountSchedulingCandidate{}
	for _, candidate := range candidates {
		byIdentity[candidate.Identity] = append(byIdentity[candidate.Identity], candidate)
	}
	old := map[string]AccountSchedulingSlot{}
	for _, slot := range previous {
		old[slot.Identity] = slot
	}
	slots := map[string]AccountSchedulingSlot{}
	representative := map[string]AccountSchedulingCandidate{}
	allowed := map[int64]bool{}
	for identity, aliases := range byIdentity {
		sort.Slice(aliases, func(i, j int) bool {
			if aliases[i].Position != aliases[j].Position {
				return aliases[i].Position < aliases[j].Position
			}
			return aliases[i].AccountID < aliases[j].AccountID
		})
		candidate := aliases[0]
		for _, alias := range aliases {
			allowed[alias.AccountID] = false
			if alias.ExitGroup != candidate.ExitGroup {
				candidate.State = AccountSchedulingStandby
				candidate.Reason = "identity_exit_conflict"
			}
			if alias.State == AccountSchedulingReauth || alias.Reason == "quota_exhausted" {
				candidate.State = alias.State
				candidate.Reason = alias.Reason
			}
		}
		representative[identity] = candidate
		slot, exists := old[identity]
		if !exists {
			slot = AccountSchedulingSlot{Identity: identity, AccountID: candidate.AccountID, ExitGroup: candidate.ExitGroup, State: AccountSchedulingStandby, Reason: "ready", QueuedAt: now, UpdatedAt: now}
		}
		was := slot
		ready := candidate.Reason == "ready"
		if slot.ExitGroup != candidate.ExitGroup {
			if live[identity] > 0 {
				slot.State = AccountSchedulingDraining
				slot.Reason = "exit_changed"
				slots[identity] = slot
				continue
			}
			slot.ExitGroup = candidate.ExitGroup
			slot.State = AccountSchedulingStandby
			slot.QueuedAt = now
		}
		if !ready {
			if candidate.Reason == "temporary_cooldown" && slot.State == AccountSchedulingActive {
				slot.Reason = candidate.Reason
				slots[identity] = slot
				continue
			}
			if live[identity] > 0 {
				slot.State = AccountSchedulingDraining
			} else {
				slot.State = candidate.State
			}
			slot.Reason = candidate.Reason
			if candidate.Reason == "authentication_failed" && was.State != AccountSchedulingActive && was.State != AccountSchedulingDraining && was.Reason != "authentication_failed" {
				slot.Reason = "requires_reauthentication"
			}
		} else if slot.State != AccountSchedulingActive {
			if live[identity] > 0 {
				slot.State = AccountSchedulingDraining
			} else {
				if slot.State != AccountSchedulingStandby || slot.Reason != "ready" {
					slot.QueuedAt = now
				}
				slot.State = AccountSchedulingStandby
				slot.Reason = "ready"
			}
		} else {
			slot.Reason = "ready"
		}
		slot.AccountID = candidate.AccountID
		if slot.State != was.State || slot.Reason != was.Reason || slot.ExitGroup != was.ExitGroup {
			slot.UpdatedAt = now
		}
		slots[identity] = slot
	}
	// Deleted or newly paid accounts with running requests retain the old budget
	// until release. They are never selected for a new governed request.
	for identity, slot := range old {
		if _, exists := slots[identity]; !exists && live[identity] > 0 {
			slot.State = AccountSchedulingDraining
			slot.Reason = "account_removed"
			slots[identity] = slot
		}
	}
	ordered := make([]AccountSchedulingSlot, 0, len(slots))
	for _, slot := range slots {
		ordered = append(ordered, slot)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].QueuedAt.Equal(ordered[j].QueuedAt) {
			return ordered[i].QueuedAt.Before(ordered[j].QueuedAt)
		}
		a, b := representative[ordered[i].Identity], representative[ordered[j].Identity]
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return ordered[i].Identity < ordered[j].Identity
	})
	// A burst of permanent account errors is evidence against cycling through
	// every reserve on the same exit. Do not activate replacements for 10 minutes.
	authFailures := map[string]int{}
	for _, slot := range ordered {
		if slot.State == AccountSchedulingReauth && slot.Reason == "authentication_failed" && now.Sub(slot.UpdatedAt) < 10*time.Minute {
			authFailures[slot.ExitGroup]++
		}
	}
	occupied := map[string]int{}
	for _, slot := range ordered {
		if slot.State == AccountSchedulingDraining {
			occupied[slot.ExitGroup]++
		}
	}
	for i := range ordered {
		slot := &ordered[i]
		if slot.State == AccountSchedulingDraining {
			continue
		}
		if slot.State == AccountSchedulingActive {
			if occupied[slot.ExitGroup] >= limit {
				if live[slot.Identity] > 0 {
					slot.State = AccountSchedulingDraining
					occupied[slot.ExitGroup]++
				} else {
					slot.State = AccountSchedulingStandby
				}
				slot.Reason = "limit_reduced"
				slot.UpdatedAt = now
			} else {
				occupied[slot.ExitGroup]++
			}
		}
	}
	for i := range ordered {
		slot := &ordered[i]
		if slot.State == AccountSchedulingStandby && slot.Reason == "ready" {
			if authFailures[slot.ExitGroup] >= 3 {
				slot.Reason = "exit_auth_circuit_open"
				continue
			}
			if occupied[slot.ExitGroup] < limit {
				slot.State = AccountSchedulingActive
				slot.UpdatedAt = now
				occupied[slot.ExitGroup]++
			}
		}
		if slot.State == AccountSchedulingActive {
			for _, alias := range byIdentity[slot.Identity] {
				if alias.Reason == "ready" && alias.ExitGroup == slot.ExitGroup {
					allowed[alias.AccountID] = true
				}
			}
		}
	}
	return ordered, allowed
}
