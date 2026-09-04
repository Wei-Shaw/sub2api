package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// noAvailableAccountsClientMessage is what end users see when the account pool
// cannot serve a request right now (no schedulable account, no free concurrency
// slot, profit control vetoed every candidate).
//
// It deliberately says nothing about accounts. The previous wording,
// "No available accounts: <internal reason>", was forwarded verbatim by
// downstream gateways into end-user terminals and disclosed how the service is
// built (an account pool) together with scheduler internals (rate-limit
// counters, profit control). The OpenAI gateway already answers these cases with
// the neutral classifier fallback; this constant brings the other entrypoints in
// line. The internal reason is still recorded for operators, see
// recordNoAvailableAccountsReasonForOps.
const noAvailableAccountsClientMessage = "Service temporarily unavailable, please retry later"

// noAvailableAccountsOpsPrefix is the phrase the ops pipeline keys on: the
// IgnoreNoAvailableAccounts filter (isOpsNoAvailableAccountMessage), the channel
// monitor taxonomy ("account_pool_unavailable") and its SQL aggregation all
// match "no available accounts" in the recorded text.
const noAvailableAccountsOpsPrefix = "No available accounts"

// noAvailableAccountsReasonNoSlot is recorded when the selected account has no
// free concurrency slot and the group has no wait plan.
const noAvailableAccountsReasonNoSlot = "account concurrency slots exhausted and no wait plan"

// noAvailableAccountsReasonNoAccount is recorded when scheduling returned no
// account at all for the request.
const noAvailableAccountsReasonNoAccount = "no schedulable account for this request"

// recordNoAvailableAccountsReasonForOps keeps the internal reason visible to the
// ops error log without putting it on the wire. The reason lands in the entry's
// upstream error message, which is exactly where handleFailoverExhausted already
// stores the real upstream text while clients get a generic message.
//
// The upstream status is left unset: the request never reached upstream. Ops
// attribution (phase "routing", business-limited) is decided by
// markOpsRoutingCapacityLimited at the call site, which takes precedence over
// the upstream-error context in classifyOpsErrorLog.
func recordNoAvailableAccountsReasonForOps(c *gin.Context, reason string) {
	msg := noAvailableAccountsOpsPrefix
	if r := strings.TrimSpace(reason); r != "" {
		msg += ": " + r
	}
	service.SetOpsUpstreamError(c, 0, msg, "")
}

// recordNoAvailableAccountsErrorForOps is the account-selection variant. Only
// genuine no-available-account errors are recorded, mirroring
// markOpsRoutingCapacityLimitedIfNoAvailable; any other selection failure keeps
// its server-log trail (the call sites already log zap.Error) and is not
// attributed to the account pool.
func recordNoAvailableAccountsErrorForOps(c *gin.Context, err error) {
	if !isOpsNoAvailableAccountError(err) {
		return
	}
	recordNoAvailableAccountsReasonForOps(c, err.Error())
}
