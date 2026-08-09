//go:build unit

package handler

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateAPIKeyCreateRequest(t *testing.T) {
	zero, large, negative, nan, inf := 0.0, 1e100, -1.0, math.NaN(), math.Inf(1)
	positiveDays, zeroDays, negativeDays := 1, 0, -1
	require.NoError(t, validateAPIKeyCreateRequest(CreateAPIKeyRequest{REDACTED))
	require.NoError(t, validateAPIKeyCreateRequest(CreateAPIKeyRequest{Quota: &zero, RateLimit5h: &large, ExpiresInDays: &positiveDaysREDACTED))

	for _, req := range []CreateAPIKeyRequest{
		{Quota: &negativeREDACTED,
		{Quota: &nanREDACTED,
		{RateLimit5h: &infREDACTED,
		{RateLimit1d: &negativeREDACTED,
		{RateLimit7d: &negativeREDACTED,
		{ExpiresInDays: &zeroDaysREDACTED,
		{ExpiresInDays: &negativeDaysREDACTED,
REDACTED {
		require.Error(t, validateAPIKeyCreateRequest(req))
REDACTED
REDACTED

func TestValidateAPIKeyUpdateRequest(t *testing.T) {
	zero, large, negative, nan, inf := 0.0, 1e100, -1.0, math.NaN(), math.Inf(-1)
	require.NoError(t, validateAPIKeyUpdateRequest(UpdateAPIKeyRequest{Quota: &zero, RateLimit7d: &largeREDACTED))

	for _, req := range []UpdateAPIKeyRequest{
		{Quota: &negativeREDACTED,
		{RateLimit5h: &nanREDACTED,
		{RateLimit1d: &infREDACTED,
		{RateLimit7d: &negativeREDACTED,
REDACTED {
		require.Error(t, validateAPIKeyUpdateRequest(req))
REDACTED
REDACTED
