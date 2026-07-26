package service

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/port/billing"
)

// Usage billing write-path types, errors, and the repository contract live in
// the domain / port layers; this file re-exports them as aliases so existing
// call sites and test stubs keep compiling. The aliases preserve type
// identity, so methods declared on the domain types (Normalize, etc.) remain
// reachable through the service package.

var ErrUsageBillingRequestIDRequired = domain.ErrUsageBillingRequestIDRequired
var ErrUsageBillingRequestConflict = domain.ErrUsageBillingRequestConflict

type UsageBillingCommand = domain.UsageBillingCommand
type AccountQuotaState = domain.AccountQuotaState
type UsageBillingApplyResult = domain.UsageBillingApplyResult
type BatchImageBalanceHoldCommand = domain.BatchImageBalanceHoldCommand
type BatchImageBalanceHoldResult = domain.BatchImageBalanceHoldResult

type UsageBillingRepository = billing.UsageBillingRepository

// HashUsageRequestPayload hashes a raw request payload for dedup/fingerprint
// purposes. It is a pure helper invoked by gateway handlers and is not part
// of the UsageBillingRepository contract, so it remains in the service package.
func HashUsageRequestPayload(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
