package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildOpsSystemLogsWhere_WithClientRequestIDAndUserID(t *testing.T) {
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	userID := int64(12)
	apiKeyID := int64(56)
	accountID := int64(34)

	filter := &service.OpsSystemLogFilter{
		StartTime:       &start,
		EndTime:         &end,
		Host:            "api-node-1",
		Level:           "warn",
		Component:       "http.access",
		RequestID:       "req-1",
		ClientRequestID: "creq-1",
		UserID:          &userID,
		APIKeyID:        &apiKeyID,
		AccountID:       &accountID,
		Platform:        "openai",
		Model:           "gpt-5",
		Query:           "timeout",
	}

	where, args, hasConstraint := buildOpsSystemLogsWhere(filter)
	if !hasConstraint {
		t.Fatalf("expected hasConstraint=true")
	}
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 13 {
		t.Fatalf("args len = %d, want 13", len(args))
	}
	if !contains(where, "l.host = $") {
		t.Fatalf("where should include host condition: %s", where)
	}
	if !contains(where, "COALESCE(l.client_request_id,'') = $") {
		t.Fatalf("where should include client_request_id condition: %s", where)
	}
	if !contains(where, "l.user_id = $") {
		t.Fatalf("where should include user_id condition: %s", where)
	}
	if !contains(where, "l.api_key_id = $") {
		t.Fatalf("where should include api_key_id condition: %s", where)
	}
}

func TestBuildOpsSystemLogsCleanupWhere_RequireConstraint(t *testing.T) {
	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(&service.OpsSystemLogCleanupFilter{})
	if hasConstraint {
		t.Fatalf("expected hasConstraint=false")
	}
	if where == "" {
		t.Fatalf("where should not be empty")
	}
	if len(args) != 0 {
		t.Fatalf("args len = %d, want 0", len(args))
	}
}

func TestBuildOpsSystemLogsCleanupWhere_WithClientRequestIDAndUserID(t *testing.T) {
	userID := int64(9)
	apiKeyID := int64(10)
	filter := &service.OpsSystemLogCleanupFilter{
		Host:            "api-node-2",
		ClientRequestID: "creq-9",
		UserID:          &userID,
		APIKeyID:        &apiKeyID,
	}

	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(filter)
	if !hasConstraint {
		t.Fatalf("expected hasConstraint=true")
	}
	if len(args) != 4 {
		t.Fatalf("args len = %d, want 4", len(args))
	}
	if !contains(where, "l.host = $") {
		t.Fatalf("where should include host condition: %s", where)
	}
	if !contains(where, "COALESCE(l.client_request_id,'') = $") {
		t.Fatalf("where should include client_request_id condition: %s", where)
	}
	if !contains(where, "l.user_id = $") {
		t.Fatalf("where should include user_id condition: %s", where)
	}
	if !contains(where, "l.api_key_id = $") {
		t.Fatalf("where should include api_key_id condition: %s", where)
	}
}

func contains(s string, sub string) bool {
	return strings.Contains(s, sub)
}
