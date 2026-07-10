package service

import (
	"context"
	"fmt"
	"math"
)

// StaticBudgetGuard is the phase-2A interception primitive for the VA1 budget gate.
// It enforces a fixed per-call budget cap WITHOUT depending on any external balance
// backend: CheckBudget compares the gateway's duration-based cost estimate against
// PerCallBudget and rejects (fail-closed) when the estimate exceeds the cap.
//
// Scope boundary (deliberate):
//   - This guard is the "brake": it proves the threshold-interception logic and gives
//     phase-2 B a concrete guard to inject (set PerCallBudget to a real number) as an
//     interim cap before the per-user-balance backend lands.
//   - It is NOT wired into production DI. The gateway leaves budget=nil by default, so
//     installing this type changes no production behaviour; activation is an explicit
//     SetBudgetGuard call (phase-2 B, author-authorized).
//   - Charge is a no-op here: real per-user-balance deduction (via UsageBillingRepository)
//     is phase-2 B scope. This guard only gates the pre-call affordability check.
type StaticBudgetGuard struct {
	// PerCallBudget is the maximum estimated cost a single create may incur. It MUST be
	// set to a positive value to arm the gate; a non-positive cap rejects every request
	// (fail-closed) so an unconfigured guard can never silently pass-through.
	PerCallBudget float64
}

// NewStaticBudgetGuard builds a per-call budget cap guard. perCallBudget is the maximum
// estimated cost allowed for one create; pass a positive value to arm the gate.
func NewStaticBudgetGuard(perCallBudget float64) *StaticBudgetGuard {
	return &StaticBudgetGuard{PerCallBudget: perCallBudget}
}

// CheckBudget is the FAIL-CLOSED pre-call gate. It rejects when the per-call cap is not
// configured (<= 0), when either the cap or the estimated cost is non-finite (NaN/±Inf —
// which would otherwise slip past a naive `>` comparison and fail OPEN), or when the
// estimated cost exceeds the cap; it returns nil only when the cost is finite and within
// budget. Returning an error here makes the gateway reject the create with no task row
// persisted and no provider dispatch.
func (g *StaticBudgetGuard) CheckBudget(_ context.Context, _ int64, estimatedCost float64) error {
	if g == nil || g.PerCallBudget <= 0 || math.IsNaN(g.PerCallBudget) || math.IsInf(g.PerCallBudget, 0) {
		return fmt.Errorf("INSUFFICIENT_BUDGET: no valid per-call budget configured (fail-closed)")
	}
	if math.IsNaN(estimatedCost) || math.IsInf(estimatedCost, 0) {
		return fmt.Errorf("INSUFFICIENT_BUDGET: estimated cost is not finite (fail-closed)")
	}
	if estimatedCost > g.PerCallBudget {
		return fmt.Errorf("INSUFFICIENT_BUDGET: estimated cost %.4f exceeds per-call cap %.4f", estimatedCost, g.PerCallBudget)
	}
	return nil
}

// Charge is a deliberate no-op in phase-2A. Real per-user-balance deduction (wiring a
// UsageBillingRepository-backed implementation) is phase-2 B scope; this guard installs
// only the pre-call brake, never moving money.
func (g *StaticBudgetGuard) Charge(_ context.Context, _ int64, _ float64, _ int64) error {
	return nil
}
