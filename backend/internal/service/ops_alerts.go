package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func (s *OpsService) ListAlertRules(ctx context.Context) ([]*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return []*OpsAlertRule{REDACTED, nil
REDACTED
	return s.opsRepo.ListAlertRules(ctx)
REDACTED

func (s *OpsService) CreateAlertRule(ctx context.Context, rule *OpsAlertRule) (*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if rule == nil {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid rule")
REDACTED

	created, err := s.opsRepo.CreateAlertRule(ctx, rule)
	if err != nil {
		return nil, err
REDACTED
	return created, nil
REDACTED

func (s *OpsService) UpdateAlertRule(ctx context.Context, rule *OpsAlertRule) (*OpsAlertRule, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if rule == nil || rule.ID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE", "invalid rule")
REDACTED

	updated, err := s.opsRepo.UpdateAlertRule(ctx, rule)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
	REDACTED
		return nil, err
REDACTED
	return updated, nil
REDACTED

func (s *OpsService) DeleteAlertRule(ctx context.Context, id int64) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
REDACTED
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if id <= 0 {
		return infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
REDACTED
	if err := s.opsRepo.DeleteAlertRule(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return infraerrors.NotFound("OPS_ALERT_RULE_NOT_FOUND", "alert rule not found")
	REDACTED
		return err
REDACTED
	return nil
REDACTED

func (s *OpsService) ListAlertEvents(ctx context.Context, filter *OpsAlertEventFilter) ([]*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return []*OpsAlertEvent{REDACTED, nil
REDACTED
	return s.opsRepo.ListAlertEvents(ctx, filter)
REDACTED

func (s *OpsService) GetAlertEventByID(ctx context.Context, eventID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if eventID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
REDACTED
	ev, err := s.opsRepo.GetAlertEventByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, infraerrors.NotFound("OPS_ALERT_EVENT_NOT_FOUND", "alert event not found")
	REDACTED
		return nil, err
REDACTED
	if ev == nil {
		return nil, infraerrors.NotFound("OPS_ALERT_EVENT_NOT_FOUND", "alert event not found")
REDACTED
	return ev, nil
REDACTED

func (s *OpsService) GetActiveAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
REDACTED
	return s.opsRepo.GetActiveAlertEvent(ctx, ruleID)
REDACTED

func (s *OpsService) CreateAlertSilence(ctx context.Context, input *OpsAlertSilence) (*OpsAlertSilence, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_SILENCE", "invalid silence")
REDACTED
	if input.RuleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
REDACTED
	if strings.TrimSpace(input.Platform) == "" {
		return nil, infraerrors.BadRequest("INVALID_PLATFORM", "invalid platform")
REDACTED
	if input.Until.IsZero() {
		return nil, infraerrors.BadRequest("INVALID_UNTIL", "invalid until")
REDACTED

	created, err := s.opsRepo.CreateAlertSilence(ctx, input)
	if err != nil {
		return nil, err
REDACTED
	return created, nil
REDACTED

func (s *OpsService) IsAlertSilenced(ctx context.Context, ruleID int64, platform string, groupID *int64, region *string, now time.Time) (bool, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return false, err
REDACTED
	if s.opsRepo == nil {
		return false, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if ruleID <= 0 {
		return false, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
REDACTED
	if strings.TrimSpace(platform) == "" {
		return false, nil
REDACTED
	return s.opsRepo.IsAlertSilenced(ctx, ruleID, platform, groupID, region, now)
REDACTED

func (s *OpsService) GetLatestAlertEvent(ctx context.Context, ruleID int64) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if ruleID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_RULE_ID", "invalid rule id")
REDACTED
	return s.opsRepo.GetLatestAlertEvent(ctx, ruleID)
REDACTED

func (s *OpsService) CreateAlertEvent(ctx context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return nil, err
REDACTED
	if s.opsRepo == nil {
		return nil, infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if event == nil {
		return nil, infraerrors.BadRequest("INVALID_EVENT", "invalid event")
REDACTED

	created, err := s.opsRepo.CreateAlertEvent(ctx, event)
	if err != nil {
		return nil, err
REDACTED
	return created, nil
REDACTED

func (s *OpsService) UpdateAlertEventStatus(ctx context.Context, eventID int64, status string, resolvedAt *time.Time) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
REDACTED
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if eventID <= 0 {
		return infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
REDACTED
	status = strings.TrimSpace(status)
	if status == "" {
		return infraerrors.BadRequest("INVALID_STATUS", "invalid status")
REDACTED
	if status != OpsAlertStatusResolved && status != OpsAlertStatusManualResolved {
		return infraerrors.BadRequest("INVALID_STATUS", "invalid status")
REDACTED
	return s.opsRepo.UpdateAlertEventStatus(ctx, eventID, status, resolvedAt)
REDACTED

func (s *OpsService) UpdateAlertEventEmailSent(ctx context.Context, eventID int64, emailSent bool) error {
	if err := s.RequireMonitoringEnabled(ctx); err != nil {
		return err
REDACTED
	if s.opsRepo == nil {
		return infraerrors.ServiceUnavailable("OPS_REPO_UNAVAILABLE", "Ops repository not available")
REDACTED
	if eventID <= 0 {
		return infraerrors.BadRequest("INVALID_EVENT_ID", "invalid event id")
REDACTED
	return s.opsRepo.UpdateAlertEventEmailSent(ctx, eventID, emailSent)
REDACTED
