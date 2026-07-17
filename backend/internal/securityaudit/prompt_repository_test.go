package securityaudit

import "testing"

func TestShouldStorePromptAuditEvent(t *testing.T) {
	tests := []struct {
		name            string
		storePassEvents bool
		decision        EventDecision
		want            bool
REDACTED{
		{name: "pass disabled", storePassEvents: false, decision: EventPass, want: falseREDACTED,
		{name: "flag disabled", storePassEvents: false, decision: EventFlag, want: trueREDACTED,
		{name: "critical disabled", storePassEvents: false, decision: EventCritical, want: trueREDACTED,
		{name: "pass enabled", storePassEvents: true, decision: EventPass, want: trueREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStorePromptAuditEvent(tt.decision, tt.storePassEvents); got != tt.want {
				t.Fatalf("shouldStorePromptAuditEvent(%q, %t) = %t, want %t", tt.decision, tt.storePassEvents, got, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED
