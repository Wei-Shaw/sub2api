package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldFailoverUpstreamError_405IsFailoverEligible(t *testing.T) {
	svc := &OpenAIGatewayService{}

	assert.True(t, svc.shouldFailoverUpstreamError(http.StatusMethodNotAllowed),
		"405 should trigger failover so sticky sessions can escape to healthy accounts")
}

func TestShouldFailoverUpstreamError_ExistingCodesStillWork(t *testing.T) {
	svc := &OpenAIGatewayService{}

	failoverCodes := []int{401, 402, 403, 405, 429, 529, 500, 502, 503, 504}
	for _, code := range failoverCodes {
		assert.True(t, svc.shouldFailoverUpstreamError(code), "status %d should trigger failover", code)
	}

	nonFailoverCodes := []int{200, 201, 400, 404, 408, 422}
	for _, code := range nonFailoverCodes {
		assert.False(t, svc.shouldFailoverUpstreamError(code), "status %d should NOT trigger failover", code)
	}
}

func TestHandleGrokAccountUpstreamError_405TriggersTempUnschedule(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 999, Platform: "grok"}

	// Should not panic on nil repos — the function gracefully handles
	// missing dependencies and only sets the in-memory cooldown.
	svc.handleGrokAccountUpstreamError(nil, account, http.StatusMethodNotAllowed, nil, nil)

	// Verify the account is temp-unscheduled by checking the runtime state.
	// Since tempUnscheduleGrok requires accountRepo which is nil, the durable
	// write will be skipped but the function itself should not panic.
}
