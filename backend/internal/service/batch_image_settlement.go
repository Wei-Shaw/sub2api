package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const (
	batchImageSettlementRequestPrefix = "batch_image_settlement:"
	batchImageSettlementRetryDelay    = time.Minute
)

type BatchImagePricingResolver interface {
	BatchImageUnitPrice(ctx context.Context, job *BatchImageJob) (float64, error)
REDACTED

type BatchImageModelPricingResolver struct {
	Resolver *ModelPricingResolver
REDACTED

func (r *BatchImageModelPricingResolver) BatchImageUnitPrice(ctx context.Context, job *BatchImageJob) (float64, error) {
	if r == nil || r.Resolver == nil || job == nil || strings.TrimSpace(job.Model) == "" {
		return 0, ErrBatchImageSettlementPricingMissing
REDACTED
	resolved := r.Resolver.Resolve(ctx, PricingInput{Model: job.ModelREDACTED)
	if resolved == nil {
		return 0, ErrBatchImageSettlementPricingMissing
REDACTED
	switch resolved.Mode {
	case BillingModeImage, BillingModePerRequest:
		if resolved.DefaultPerRequestPrice > 0 {
			return resolved.DefaultPerRequestPrice, nil
	REDACTED
		if len(resolved.RequestTiers) == 1 && resolved.RequestTiers[0].PerRequestPrice != nil && *resolved.RequestTiers[0].PerRequestPrice >= 0 {
			return *resolved.RequestTiers[0].PerRequestPrice, nil
	REDACTED
	case BillingModeToken:
		if resolved.BasePricing != nil && (resolved.BasePricing.ImageOutputPriceExplicit || resolved.BasePricing.ImageOutputPricePerToken > 0) {
			return resolved.BasePricing.ImageOutputPricePerToken, nil
	REDACTED
REDACTED
	return 0, ErrBatchImageSettlementPricingMissing
REDACTED

type BatchImageSettlementService struct {
	Repo        BatchImageRepository
	BillingRepo UsageBillingRepository
	Pricing     BatchImagePricingResolver
	Config      *config.Config
REDACTED

type BatchImageSettlementResult struct {
	BatchID        string
	SuccessCount   int
	FailCount      int
	ActualCost     float64
	ManifestHash   string
	RequestID      string
	AlreadySettled bool
REDACTED

func (s *BatchImageSettlementService) Settle(ctx context.Context, batchID string) (*BatchImageSettlementResult, error) {
	if s == nil || s.Repo == nil || s.BillingRepo == nil || s.Pricing == nil {
		return nil, ErrBatchImageSettlementBillingFailed.WithCause(errors.New("batch image settlement service is not configured"))
REDACTED
	job, err := s.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return nil, err
REDACTED

	manifestHash := BuildBatchImageSettlementManifestHash(job)
	result := &BatchImageSettlementResult{
		BatchID:      job.BatchID,
		SuccessCount: job.SuccessCount,
		FailCount:    job.FailCount,
		ManifestHash: manifestHash,
		RequestID:    BatchImageSettlementRequestID(job.BatchID),
REDACTED
	if job.ActualCost != nil {
		result.ActualCost = *job.ActualCost
REDACTED
	if job.Status == BatchImageJobStatusCompleted {
		result.AlreadySettled = true
		return result, nil
REDACTED
	if job.Status != BatchImageJobStatusSettling {
		return nil, ErrBatchImageSettlementInvalidStatus
REDACTED
	if job.SuccessCount < 0 || job.FailCount < 0 || job.ItemCount < 0 {
		return nil, ErrBatchImageSettlementInvalidCounts
REDACTED
	if strings.TrimSpace(batchImageDerefString(job.ManifestHash)) != "" && batchImageDerefString(job.ManifestHash) != manifestHash {
		return nil, ErrBatchImageSettlementManifestConflict
REDACTED
	if job.APIKeyID == nil || *job.APIKeyID <= 0 {
		return nil, ErrBatchImageSettlementMissingAPIKeyID
REDACTED
	if job.AccountID == nil || *job.AccountID <= 0 {
		return nil, ErrBatchImageSettlementMissingAccountID
REDACTED

	unitPrice, err := s.Pricing.BatchImageUnitPrice(ctx, job)
	if err != nil {
		return nil, err
REDACTED
	if unitPrice < 0 {
		return nil, ErrBatchImageSettlementPricingMissing
REDACTED
	actualCost := float64(job.SuccessCount) * unitPrice
	result.ActualCost = actualCost

	cmd := &UsageBillingCommand{
		RequestID:          result.RequestID,
		APIKeyID:           *job.APIKeyID,
		RequestPayloadHash: manifestHash,
		UserID:             job.UserID,
		AccountID:          *job.AccountID,
		Model:              job.Model,
		BillingType:        BillingTypeBalance,
		ImageCount:         job.SuccessCount,
		MediaType:          "image",
		BalanceCost:        actualCost,
REDACTED
	if _, err := s.BillingRepo.Apply(ctx, cmd); err != nil {
		msg := truncateBatchImageMessage(err.Error(), batchImageMaxErrorMessageLength)
		_ = s.Repo.SetBatchImageJobSettlementFailed(ctx, job.BatchID, "SETTLEMENT_BILLING_FAILED", msg)
		return nil, ErrBatchImageSettlementBillingFailed.WithCause(err)
REDACTED

	now := time.Now()
	outputExpiresAt := now.Add(s.outputRetentionAfterTerminal())
	if err := s.Repo.MarkBatchImageJobSettled(ctx, MarkBatchImageJobSettledParams{
		BatchID:         job.BatchID,
		ActualCost:      actualCost,
		ManifestHash:    manifestHash,
		Now:             &now,
		OutputExpiresAt: &outputExpiresAt,
		EventPayload: map[string]any{
			"batch_id":      job.BatchID,
			"request_id":    result.RequestID,
			"success_count": job.SuccessCount,
			"fail_count":    job.FailCount,
			"actual_cost":   actualCost,
			"manifest_hash": manifestHash,
	REDACTED,
REDACTED); err != nil {
		return nil, err
REDACTED

	return result, nil
REDACTED

func (s *BatchImageSettlementService) outputRetentionAfterTerminal() time.Duration {
	if s != nil && s.Config != nil && s.Config.BatchImage.OutputRetentionAfterTerminalHours > 0 {
		return time.Duration(s.Config.BatchImage.OutputRetentionAfterTerminalHours) * time.Hour
REDACTED
	return 72 * time.Hour
REDACTED

func BatchImageSettlementRequestID(batchID string) string {
	return batchImageSettlementRequestPrefix + strings.TrimSpace(batchID)
REDACTED

func BuildBatchImageSettlementManifestHash(job *BatchImageJob) string {
	if job == nil {
		return ""
REDACTED
	parts := []string{
		strings.TrimSpace(job.BatchID),
		strings.TrimSpace(job.Provider),
		strings.TrimSpace(job.Model),
		batchImageDerefString(job.ProviderJobName),
		batchImageDerefString(job.ProviderOutputRef),
		strconv.Itoa(job.SuccessCount),
		strconv.Itoa(job.FailCount),
		strconv.Itoa(job.ItemCount),
REDACTED
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
REDACTED

type BatchImagePipelineProcessor struct {
	ProviderProcessor *BatchImageProviderProcessor
	SettlementService *BatchImageSettlementService
	RetryDelay        time.Duration
REDACTED

func (p *BatchImagePipelineProcessor) Process(ctx context.Context, batchID string) (BatchImageProcessResult, error) {
	if p == nil || p.ProviderProcessor == nil {
		return BatchImageProcessResult{REDACTED, errors.New("batch image pipeline processor is not configured")
REDACTED
	job, err := p.ProviderProcessor.Repo.GetBatchImageJobByBatchID(ctx, batchID)
	if err != nil {
		return BatchImageProcessResult{REDACTED, err
REDACTED
	if job.Status == BatchImageJobStatusSettling {
		if p.SettlementService == nil {
			return BatchImageProcessResult{Terminal: trueREDACTED, nil
	REDACTED
		_, err := p.SettlementService.Settle(ctx, batchID)
		if err != nil {
			if errors.Is(err, ErrBatchImageSettlementBillingFailed) {
				delay := p.RetryDelay
				if delay <= 0 {
					delay = batchImageSettlementRetryDelay
			REDACTED
				return BatchImageProcessResult{RequeueAfter: delayREDACTED, nil
		REDACTED
			return BatchImageProcessResult{REDACTED, err
	REDACTED
		return BatchImageProcessResult{Terminal: trueREDACTED, nil
REDACTED
	return p.ProviderProcessor.Process(ctx, batchID)
REDACTED

func (r *BatchImageSettlementResult) String() string {
	if r == nil {
		return ""
REDACTED
	return fmt.Sprintf("batch_id=%s success=%d fail=%d actual_cost=%0.10f already_settled=%t",
		r.BatchID, r.SuccessCount, r.FailCount, r.ActualCost, r.AlreadySettled)
REDACTED
