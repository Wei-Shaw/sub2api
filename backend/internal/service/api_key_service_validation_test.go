//go:build unit

package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCreateAPIKeyRequestNumericLimits(t *testing.T) {
	positiveExpiry := 1
	require.NoError(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{
		Quota: 1e100, RateLimit5h: 1e100, ExpiresInDays: &positiveExpiry,
REDACTED))
	require.NoError(t, validateCreateAPIKeyRequest(CreateAPIKeyRequest{REDACTED))

	invalidExpiry := 0
	tests := []CreateAPIKeyRequest{
		{Quota: -1REDACTED,
		{Quota: math.NaN()REDACTED,
		{Quota: math.Inf(1)REDACTED,
		{RateLimit5h: -1REDACTED,
		{RateLimit1d: math.NaN()REDACTED,
		{RateLimit7d: math.Inf(-1)REDACTED,
		{ExpiresInDays: &invalidExpiryREDACTED,
REDACTED
	for _, req := range tests {
		require.Error(t, validateCreateAPIKeyRequest(req))
REDACTED
REDACTED

func TestValidateUpdateAPIKeyRequestNumericLimits(t *testing.T) {
	zero, large, negative, nan, inf := 0.0, 1e100, -1.0, math.NaN(), math.Inf(1)
	require.NoError(t, validateUpdateAPIKeyRequest(UpdateAPIKeyRequest{Quota: &zero, RateLimit7d: &largeREDACTED))

	for _, req := range []UpdateAPIKeyRequest{
		{Quota: &negativeREDACTED,
		{RateLimit5h: &nanREDACTED,
		{RateLimit1d: &infREDACTED,
		{RateLimit7d: &negativeREDACTED,
REDACTED {
		require.Error(t, validateUpdateAPIKeyRequest(req))
REDACTED
REDACTED
