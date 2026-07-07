package service

import (
	"context"
	"errors"
	"time"
)

const (
	defaultBatchImageBillingRecoveryStaleAfter = 10 * time.Minute
	defaultBatchImageBillingRecoveryLimit      = 100
)

type BatchImageBillingRecoveryService struct {
	Repo       BatchImageRepository
	Billing    UsageBillingRepository
	AuthCache  APIKeyAuthCacheInvalidator
	StaleAfter time.Duration
	Limit      int
REDACTED

func (s *BatchImageBillingRecoveryService) ReleaseStaleUnsubmittedOnce(ctx context.Context) (int, error) {
	if s == nil || s.Repo == nil || s.Billing == nil {
		return 0, nil
REDACTED
	staleAfter := s.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultBatchImageBillingRecoveryStaleAfter
REDACTED
	limit := s.Limit
	if limit <= 0 {
		limit = defaultBatchImageBillingRecoveryLimit
REDACTED
	jobs, err := s.Repo.ListStaleUnsubmittedBatchImageJobs(ctx, time.Now().Add(-staleAfter), limit)
	if err != nil {
		return 0, err
REDACTED
	released := 0
	for _, job := range jobs {
		if job == nil {
			continue
	REDACTED
		msg := "batch image submission did not reach provider before recovery cutoff"
		if err := s.Repo.TransitionBatchImageJobStatus(ctx, job.BatchID, BatchImageJobStatusFailed, BatchImageTransitionOptions{
			EventType:    "billing_hold_recovery_failed_unsubmitted",
			EventPayload: map[string]any{"batch_id": job.BatchIDREDACTED,
			ErrorCode:    batchImageStringPtr("SUBMIT_STALE_BEFORE_PROVIDER"),
			ErrorMessage: batchImageStringPtr(msg),
	REDACTED); err != nil && !errors.Is(err, ErrBatchImageInvalidTransition) {
			return released, err
	REDACTED
		job.Status = BatchImageJobStatusFailed
		if err := releaseBatchImageBalanceHold(ctx, s.Billing, job, batchImageDerefString(job.RequestHash)); err != nil {
			return released, err
	REDACTED
		if s.AuthCache != nil && job.UserID > 0 {
			s.AuthCache.InvalidateAuthCacheByUserID(ctx, job.UserID)
	REDACTED
		released++
REDACTED
	return released, nil
REDACTED
