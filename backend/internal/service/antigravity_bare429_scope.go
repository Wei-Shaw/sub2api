package service

import (
	"context"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// antigravityBare429CooledAccountKey holds, on the request's gin context, the one
// account this request has cooled down for a bare 429.
const antigravityBare429CooledAccountKey = "antigravity_bare429_cooled_account_id"

// isBareAntigravity429 reports a 429 with no reason and no reset time, only
// "Resource has been exhausted". Google sends it for an account that is
// throttled, but also for a single request it will not serve (2026-11-16: one
// 1.8 MB request got it on four accounts in a row while those accounts kept
// serving other requests). A 429 that names a reason or a reset time is
// Google's statement about the account and is not bare.
func isBareAntigravity429(statusCode int, body []byte) bool {
	if statusCode != http.StatusTooManyRequests {
		return false
	}
	if gjson.GetBytes(body, "error.details").Exists() {
		return false
	}
	return ParseGeminiRateLimitResetTime(body) == nil
}

// claimBare429Cooldown reports whether this request may cool accountID for a
// bare 429. A request cools at most one account this way: when the same bare
// 429 follows the request to another account, the request is the likely cause,
// and cooling every account it reaches takes a shared pool offline for everyone.
// The request still leaves that account and fails over. An account throttled
// for everyone is cooled by the next request that reaches it first.
func claimBare429Cooldown(c *gin.Context, accountID int64) bool {
	if c == nil {
		return true
	}
	if v, ok := c.Get(antigravityBare429CooledAccountKey); ok {
		if id, ok := v.(int64); ok && id != accountID {
			return false
		}
	}
	c.Set(antigravityBare429CooledAccountKey, accountID)
	return true
}

// handleUpstreamErrorAfterRetryLoop applies an error the retry loop handed back
// as a response. A bare 429 there was already decided by the loop: the account
// this request cooled may be cooled again, any other is left alone. Without this
// the compat, Gemini and Claude paths cooled every account the loop had skipped
// (2026-11-19: one 2 MB Codex request cooled #2, #7 and #6 every few minutes).
func (s *AntigravityGatewayService) handleUpstreamErrorAfterRetryLoop(
	ctx context.Context, c *gin.Context, prefix string, account *Account,
	statusCode int, headers http.Header, body []byte,
	requestedModel string,
	groupID int64, sessionHash string, isStickySession bool,
) *handleModelRateLimitResult {
	if isBareAntigravity429(statusCode, body) && !claimBare429Cooldown(c, account.ID) {
		logger.LegacyPrintf("service.antigravity_gateway", "%s status=%d bare_429_cooldown_skipped account=%d (after retry loop)", prefix, statusCode, account.ID)
		return nil
	}
	return s.handleUpstreamError(ctx, prefix, account, statusCode, headers, body, requestedModel, groupID, sessionHash, isStickySession)
}
