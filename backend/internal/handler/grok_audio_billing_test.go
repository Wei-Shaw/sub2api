//go:build unit

package handler

import (
	"testing"
	"time"

	coderws "github.com/coder/websocket"
)

func TestIsExpectedGrokRealtimeClose(t *testing.T) {
	for _, status := range []coderws.StatusCode{
		coderws.StatusNormalClosure,
		coderws.StatusGoingAway,
		coderws.StatusNoStatusRcvd,
		coderws.StatusAbnormalClosure,
REDACTED {
		if !isExpectedGrokRealtimeClose(coderws.CloseError{Code: statusREDACTED) {
			t.Fatalf("status %v should be treated as an expected session close", status)
	REDACTED
REDACTED
	if isExpectedGrokRealtimeClose(coderws.CloseError{Code: coderws.StatusPolicyViolationREDACTED) {
		t.Fatal("policy violations must not be treated as billable normal closes")
REDACTED
REDACTED

func TestGrokRealtimeBillingResultRequiresObservedAudio(t *testing.T) {
	if grokRealtimeBillingResult("grok-voice-latest", time.Second, false) != nil {
		t.Fatal("a session without observed audio must not be billed")
REDACTED
	if grokRealtimeBillingResult("grok-voice-latest", 0, true) != nil {
		t.Fatal("zero-duration sessions must not be billed")
REDACTED
REDACTED

func TestGrokRealtimeBillingResultUsesForcedUniqueID(t *testing.T) {
	first := grokRealtimeBillingResult("grok-voice-latest", 90*time.Second, true)
	second := grokRealtimeBillingResult("grok-voice-latest", 90*time.Second, true)
	if first == nil || second == nil {
		t.Fatal("observed audio sessions should be billable")
REDACTED
	if first.RequestID == "" {
		t.Fatalf("unexpected billing request ID %q", first.RequestID)
REDACTED
	if first.RequestID == second.RequestID {
		t.Fatal("independent realtime connections must not share a billing request ID")
REDACTED
	if first.AudioUsage == nil || first.AudioUsage.Mode != "realtime" || first.AudioUsage.DurationOrUnits != 1.5 {
		t.Fatalf("unexpected audio usage: %#v", first.AudioUsage)
REDACTED
REDACTED
