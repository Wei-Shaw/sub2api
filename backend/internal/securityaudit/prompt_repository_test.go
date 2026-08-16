package securityaudit

import (
	"strings"
	"testing"
	"time"
)

func TestShouldStorePromptAuditEvent(t *testing.T) {
	tests := []struct {
		name            string
		storePassEvents bool
		decision        EventDecision
		want            bool
	}{
		{name: "pass disabled", storePassEvents: false, decision: EventPass, want: false},
		{name: "flag disabled", storePassEvents: false, decision: EventFlag, want: true},
		{name: "critical disabled", storePassEvents: false, decision: EventCritical, want: true},
		{name: "pass enabled", storePassEvents: true, decision: EventPass, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStorePromptAuditEvent(tt.decision, tt.storePassEvents); got != tt.want {
				t.Fatalf("shouldStorePromptAuditEvent(%q, %t) = %t, want %t", tt.decision, tt.storePassEvents, got, tt.want)
			}
		})
	}
}

func TestBuildEventWhereFiltersAuditSource(t *testing.T) {
	tests := []struct {
		name      string
		auditType string
		clause    string
		args      []any
	}{
		{name: "ai", auditType: EventAuditTypeAI, clause: "COALESCE(e.scanner_backend,'') NOT IN ($1, $2)", args: []any{promptKeywordCategory, promptHashCategory}},
		{name: "keyword", auditType: EventAuditTypeKeyword, clause: "e.scanner_backend=$1", args: []any{promptKeywordCategory}},
		{name: "hash", auditType: EventAuditTypeHash, clause: "e.scanner_backend=$1", args: []any{promptHashCategory}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args := buildEventWhere(EventFilter{AuditType: strings.ToUpper(tt.auditType)}, 1)
			if !strings.Contains(where, tt.clause) {
				t.Fatalf("buildEventWhere() = %q, want clause %q", where, tt.clause)
			}
			if len(args) != len(tt.args) {
				t.Fatalf("buildEventWhere() args = %#v, want %#v", args, tt.args)
			}
			for index, want := range tt.args {
				if args[index] != want {
					t.Fatalf("buildEventWhere() args = %#v, want %#v", args, tt.args)
				}
			}
		})
	}
}

func TestDeleteFilterRejectsUnknownAuditType(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	end := time.Now()
	if err := validateDeleteFilter(EventFilter{AuditType: "other", StartAt: &start, EndAt: &end}); err == nil {
		t.Fatal("validateDeleteFilter() accepted an unknown audit type")
	}
}
