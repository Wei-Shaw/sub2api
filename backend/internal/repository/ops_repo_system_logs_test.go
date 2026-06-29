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
REDACTED

	where, args, hasConstraint := buildOpsSystemLogsWhere(filter)
	if !hasConstraint {
		t.Fatalf("expected hasConstraint=true")
REDACTED
	if where == "" {
		t.Fatalf("where should not be empty")
REDACTED
	if len(args) != 12 {
		t.Fatalf("args len = %d, want 12", len(args))
REDACTED
	if !contains(where, "COALESCE(l.client_request_id,'') = $") {
		t.Fatalf("where should include client_request_id condition: %s", where)
REDACTED
	if !contains(where, "l.user_id = $") {
		t.Fatalf("where should include user_id condition: %s", where)
REDACTED
	if !contains(where, "l.api_key_id = $") {
		t.Fatalf("where should include api_key_id condition: %s", where)
REDACTED
REDACTED

func TestBuildOpsSystemLogsCleanupWhere_RequireConstraint(t *testing.T) {
	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(&service.OpsSystemLogCleanupFilter{REDACTED)
	if hasConstraint {
		t.Fatalf("expected hasConstraint=false")
REDACTED
	if where == "" {
		t.Fatalf("where should not be empty")
REDACTED
	if len(args) != 0 {
		t.Fatalf("args len = %d, want 0", len(args))
REDACTED
REDACTED

func TestBuildOpsSystemLogsCleanupWhere_WithClientRequestIDAndUserID(t *testing.T) {
	userID := int64(9)
	apiKeyID := int64(10)
	filter := &service.OpsSystemLogCleanupFilter{
		ClientRequestID: "creq-9",
		UserID:          &userID,
		APIKeyID:        &apiKeyID,
REDACTED

	where, args, hasConstraint := buildOpsSystemLogsCleanupWhere(filter)
	if !hasConstraint {
		t.Fatalf("expected hasConstraint=true")
REDACTED
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
REDACTED
	if !contains(where, "COALESCE(l.client_request_id,'') = $") {
		t.Fatalf("where should include client_request_id condition: %s", where)
REDACTED
	if !contains(where, "l.user_id = $") {
		t.Fatalf("where should include user_id condition: %s", where)
REDACTED
	if !contains(where, "l.api_key_id = $") {
		t.Fatalf("where should include api_key_id condition: %s", where)
REDACTED
REDACTED

func contains(s string, sub string) bool {
	return strings.Contains(s, sub)
REDACTED
