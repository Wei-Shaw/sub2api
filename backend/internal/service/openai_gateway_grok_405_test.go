package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldFailoverUpstreamError_405IsFailoverEligible(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED

	assert.True(t, svc.shouldFailoverUpstreamError(http.StatusMethodNotAllowed),
		"405 should trigger failover so sticky sessions can escape to healthy accounts")
REDACTED

func TestShouldFailoverUpstreamError_ExistingCodesStillWork(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED

	failoverCodes := []int{401, 402, 403, 405, 429, 529, 500, 502, 503, 504REDACTED
	for _, code := range failoverCodes {
		assert.True(t, svc.shouldFailoverUpstreamError(code), "status %d should trigger failover", code)
REDACTED

	nonFailoverCodes := []int{200, 201, 400, 404, 408, 422REDACTED
	for _, code := range nonFailoverCodes {
		assert.False(t, svc.shouldFailoverUpstreamError(code), "status %d should NOT trigger failover", code)
REDACTED
REDACTED
