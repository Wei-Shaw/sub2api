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

func TestApplyAntigravityPrivacyMode_PreservedBySubscriptionResult(t *testing.T) {
	account := &Account{
REDACTED
			"access_token": "token",
	REDACTED,
		Extra: map[string]any{
			"existing": "value",
	REDACTED,
REDACTED
	applyAntigravityPrivacyMode(account, AntigravityPrivacySet)

	_, extra := applyAntigravitySubscriptionResult(account, AntigravitySubscriptionResult{
		PlanType: "Pro",
REDACTED)

	if got := extra["privacy_mode"]; got != AntigravityPrivacySet {
		t.Fatalf("expected subscription writeback to keep privacy_mode %q, got %v", AntigravityPrivacySet, got)
REDACTED
	if got := extra["existing"]; got != "value" {
		t.Fatalf("expected existing extra fields to be preserved, got %v", got)
REDACTED
REDACTED
