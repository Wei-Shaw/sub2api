package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestOpsServiceListSystemLogs_DefaultClampAndSuccess(t *testing.T) {
	var gotFilter *OpsSystemLogFilter
	repo := &opsRepoMock{
		ListSystemLogsFn: func(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error) {
			gotFilter = filter
			return &OpsSystemLogList{
				Logs:     []*OpsSystemLog{{ID: 1, Level: "warn", Message: "x"REDACTEDREDACTED,
				Total:    1,
				Page:     filter.Page,
				PageSize: filter.PageSize,
		REDACTED, nil
	REDACTED,
REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	out, err := svc.ListSystemLogs(context.Background(), &OpsSystemLogFilter{
		Page:     0,
		PageSize: 999,
REDACTED)
	if err != nil {
		t.Fatalf("ListSystemLogs() error: %v", err)
REDACTED
	if gotFilter == nil {
		t.Fatalf("expected repository to receive filter")
REDACTED
	if gotFilter.Page != 1 || gotFilter.PageSize != 200 {
		t.Fatalf("filter normalized unexpectedly: page=%d pageSize=%d", gotFilter.Page, gotFilter.PageSize)
REDACTED
	if out.Total != 1 || len(out.Logs) != 1 {
		t.Fatalf("unexpected result: %+v", out)
REDACTED
REDACTED

func TestOpsServiceListSystemLogs_MonitoringDisabled(t *testing.T) {
	svc := NewOpsService(
		&opsRepoMock{REDACTED,
		nil,
		&config.Config{Ops: config.OpsConfig{Enabled: falseREDACTEDREDACTED,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	_, err := svc.ListSystemLogs(context.Background(), &OpsSystemLogFilter{REDACTED)
	if err == nil {
		t.Fatalf("expected disabled error")
REDACTED
REDACTED

func TestOpsServiceListSystemLogs_NilRepoReturnsEmpty(t *testing.T) {
	svc := NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	out, err := svc.ListSystemLogs(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListSystemLogs() error: %v", err)
REDACTED
	if out == nil || out.Page != 1 || out.PageSize != 50 || out.Total != 0 || len(out.Logs) != 0 {
		t.Fatalf("unexpected nil-repo result: %+v", out)
REDACTED
REDACTED

func TestOpsServiceListSystemLogs_RepoErrorMapped(t *testing.T) {
	repo := &opsRepoMock{
		ListSystemLogsFn: func(ctx context.Context, filter *OpsSystemLogFilter) (*OpsSystemLogList, error) {
			return nil, errors.New("db down")
	REDACTED,
REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.ListSystemLogs(context.Background(), &OpsSystemLogFilter{REDACTED)
	if err == nil {
		t.Fatalf("expected mapped internal error")
REDACTED
	if !strings.Contains(err.Error(), "OPS_SYSTEM_LOG_LIST_FAILED") {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestOpsServiceCleanupSystemLogs_SuccessAndAudit(t *testing.T) {
	var audit *OpsSystemLogCleanupAudit
	repo := &opsRepoMock{
		DeleteSystemLogsFn: func(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
			return 3, nil
	REDACTED,
		InsertSystemLogCleanupAuditFn: func(ctx context.Context, input *OpsSystemLogCleanupAudit) error {
			audit = input
			return nil
	REDACTED,
REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	userID := int64(7)
	apiKeyID := int64(8)
	now := time.Now().UTC()
	filter := &OpsSystemLogCleanupFilter{
		StartTime:       &now,
		Level:           "warn",
		RequestID:       "req-1",
		ClientRequestID: "creq-1",
		UserID:          &userID,
		APIKeyID:        &apiKeyID,
		Query:           "timeout",
REDACTED

	deleted, err := svc.CleanupSystemLogs(context.Background(), filter, 99)
	if err != nil {
		t.Fatalf("CleanupSystemLogs() error: %v", err)
REDACTED
	if deleted != 3 {
		t.Fatalf("deleted=%d, want 3", deleted)
REDACTED
	if audit == nil {
		t.Fatalf("expected cleanup audit")
REDACTED
	if !strings.Contains(audit.Conditions, `"client_request_id":"creq-1"`) {
		t.Fatalf("audit conditions should include client_request_id: %s", audit.Conditions)
REDACTED
	if !strings.Contains(audit.Conditions, `"user_id":7`) {
		t.Fatalf("audit conditions should include user_id: %s", audit.Conditions)
REDACTED
	if !strings.Contains(audit.Conditions, `"api_key_id":8`) {
		t.Fatalf("audit conditions should include api_key_id: %s", audit.Conditions)
REDACTED
REDACTED

func TestOpsServiceCleanupSystemLogs_RepoUnavailableAndInvalidOperator(t *testing.T) {
	svc := NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{RequestID: "r"REDACTED, 1); err == nil {
		t.Fatalf("expected repo unavailable error")
REDACTED

	svc = NewOpsService(&opsRepoMock{REDACTED, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{RequestID: "r"REDACTED, 0); err == nil {
		t.Fatalf("expected invalid operator error")
REDACTED
REDACTED

func TestOpsServiceCleanupSystemLogs_FilterRequired(t *testing.T) {
	repo := &opsRepoMock{
		DeleteSystemLogsFn: func(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
			return 0, errors.New("cleanup requires at least one filter condition")
	REDACTED,
REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{REDACTED, 1)
	if err == nil {
		t.Fatalf("expected filter required error")
REDACTED
	if !strings.Contains(strings.ToLower(err.Error()), "filter") {
		t.Fatalf("unexpected error: %v", err)
REDACTED
REDACTED

func TestOpsServiceCleanupSystemLogs_InvalidRange(t *testing.T) {
	repo := &opsRepoMock{REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	start := time.Now().UTC()
	end := start.Add(-time.Hour)
	_, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{
		StartTime: &start,
		EndTime:   &end,
REDACTED, 1)
	if err == nil {
		t.Fatalf("expected invalid range error")
REDACTED
REDACTED

func TestOpsServiceCleanupSystemLogs_NoRowsAndInternalError(t *testing.T) {
	repo := &opsRepoMock{
		DeleteSystemLogsFn: func(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
			return 0, sql.ErrNoRows
	REDACTED,
REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	deleted, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{
		RequestID: "req-1",
REDACTED, 1)
	if err != nil || deleted != 0 {
		t.Fatalf("expected no rows shortcut, deleted=%d err=%v", deleted, err)
REDACTED

	repo.DeleteSystemLogsFn = func(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
		return 0, errors.New("boom")
REDACTED
	if _, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{
		RequestID: "req-1",
REDACTED, 1); err == nil {
		t.Fatalf("expected internal cleanup error")
REDACTED
REDACTED

func TestOpsServiceCleanupSystemLogs_AuditFailureIgnored(t *testing.T) {
	repo := &opsRepoMock{
		DeleteSystemLogsFn: func(ctx context.Context, filter *OpsSystemLogCleanupFilter) (int64, error) {
			return 5, nil
	REDACTED,
		InsertSystemLogCleanupAuditFn: func(ctx context.Context, input *OpsSystemLogCleanupAudit) error {
			return errors.New("audit down")
	REDACTED,
REDACTED
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	deleted, err := svc.CleanupSystemLogs(context.Background(), &OpsSystemLogCleanupFilter{
		RequestID: "r1",
REDACTED, 1)
	if err != nil || deleted != 5 {
		t.Fatalf("audit failure should not break cleanup, deleted=%d err=%v", deleted, err)
REDACTED
REDACTED

func TestMarshalSystemLogCleanupConditions_NilAndMarshalError(t *testing.T) {
	if got := marshalSystemLogCleanupConditions(nil); got != "{REDACTED" {
		t.Fatalf("nil filter should return {REDACTED, got %s", got)
REDACTED

	now := time.Now().UTC()
	userID := int64(1)
	apiKeyID := int64(2)
	filter := &OpsSystemLogCleanupFilter{
		StartTime: &now,
		EndTime:   &now,
		UserID:    &userID,
		APIKeyID:  &apiKeyID,
REDACTED
	got := marshalSystemLogCleanupConditions(filter)
	if !strings.Contains(got, `"start_time"`) || !strings.Contains(got, `"user_id":1`) || !strings.Contains(got, `"api_key_id":2`) {
		t.Fatalf("unexpected marshal payload: %s", got)
REDACTED
REDACTED

func TestOpsServiceGetSystemLogSinkHealth(t *testing.T) {
	svc := NewOpsService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	health := svc.GetSystemLogSinkHealth()
	if health.QueueCapacity != 0 || health.QueueDepth != 0 {
		t.Fatalf("unexpected health for nil sink: %+v", health)
REDACTED

	sink := NewOpsSystemLogSink(&opsRepoMock{REDACTED)
	svc = NewOpsService(&opsRepoMock{REDACTED, nil, nil, nil, nil, nil, nil, nil, nil, nil, sink)
	health = svc.GetSystemLogSinkHealth()
	if health.QueueCapacity <= 0 {
		t.Fatalf("expected non-zero queue capacity: %+v", health)
REDACTED
REDACTED
