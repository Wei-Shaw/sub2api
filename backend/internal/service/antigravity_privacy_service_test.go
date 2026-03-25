//go:build unit

package service

import "testing"

func TestApplyAntigravityPrivacyMode_SetsInMemoryExtra(t *testing.T) {
	account := &Account{REDACTED

	applyAntigravityPrivacyMode(account, AntigravityPrivacySet)

	if account.Extra == nil {
		t.Fatal("expected account.Extra to be initialized")
REDACTED
	if got := account.Extra["privacy_mode"]; got != AntigravityPrivacySet {
		t.Fatalf("expected privacy_mode %q, got %v", AntigravityPrivacySet, got)
REDACTED
REDACTED
