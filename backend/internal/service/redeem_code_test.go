package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedeemCodeExpiry(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name        string
		code        RedeemCode
		wantExpired bool
		wantCanUse  bool
REDACTED{
		{
			name:        "unused without expiry can be used",
			code:        RedeemCode{Status: StatusUnusedREDACTED,
			wantExpired: false,
			wantCanUse:  true,
	REDACTED,
		{
			name:        "unused before expiry can be used",
			code:        RedeemCode{Status: StatusUnused, ExpiresAt: &futureREDACTED,
			wantExpired: false,
			wantCanUse:  true,
	REDACTED,
		{
			name:        "unused after expiry cannot be used",
			code:        RedeemCode{Status: StatusUnused, ExpiresAt: &pastREDACTED,
			wantExpired: true,
			wantCanUse:  false,
	REDACTED,
		{
			name:        "explicit expired status is expired",
			code:        RedeemCode{Status: StatusExpiredREDACTED,
			wantExpired: true,
			wantCanUse:  false,
	REDACTED,
		{
			name:        "used code remains used even after expiry time",
			code:        RedeemCode{Status: StatusUsed, ExpiresAt: &pastREDACTED,
			wantExpired: false,
			wantCanUse:  false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantExpired, tt.code.IsExpiredAt(now))
			require.Equal(t, tt.wantCanUse, tt.code.CanUse())
	REDACTED)
REDACTED
REDACTED
