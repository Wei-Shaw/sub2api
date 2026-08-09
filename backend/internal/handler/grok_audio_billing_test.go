//go:build unit

package handler

import (
	"testing"

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
