//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchImageSettlementService_SettlesAndChargesSuccessfulImagesOnly(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_settle")
	job.SuccessCount = 3
	job.FailCount = 2
	job.ItemCount = 5
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{REDACTED
	svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED

	result, err := svc.Settle(context.Background(), job.BatchID)
REDACTED
	require.Equal(t, 0.75, result.ActualCost)
	require.Equal(t, "batch_image_settlement:"+job.BatchID, result.RequestID)
	require.False(t, result.AlreadySettled)
	require.Equal(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	require.NotNil(t, repo.jobs[job.BatchID].ActualCost)
	require.Equal(t, 0.75, *repo.jobs[job.BatchID].ActualCost)
	require.NotEmpty(t, batchImageDerefString(repo.jobs[job.BatchID].ManifestHash))
	require.NotNil(t, repo.jobs[job.BatchID].SettledAt)
	require.Len(t, billing.commands, 1)
	require.Equal(t, int64(321), billing.commands[0].APIKeyID)
	require.Equal(t, job.UserID, billing.commands[0].UserID)
	require.Equal(t, int64(654), billing.commands[0].AccountID)
	require.Equal(t, job.Model, billing.commands[0].Model)
	require.Equal(t, 3, billing.commands[0].ImageCount)
	require.Equal(t, 0.75, billing.commands[0].BalanceCost)
	require.Equal(t, "image", billing.commands[0].MediaType)
	require.NotContains(t, fmt.Sprintf("%+v", billing.commands[0]), batchImageTestData)
	require.NotContains(t, fmt.Sprintf("%+v", billing.commands[0]), "gs://")
	require.NotContains(t, fmt.Sprintf("%+v", billing.commands[0]), "prompt")
REDACTED

func TestBatchImageSettlementService_ZeroSuccessCanComplete(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_zero")
	job.SuccessCount = 0
	job.FailCount = 4
	job.ItemCount = 4
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{REDACTED
	svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED

	result, err := svc.Settle(context.Background(), job.BatchID)
REDACTED
	require.Equal(t, 0.0, result.ActualCost)
	require.Equal(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	require.Len(t, billing.commands, 1)
	require.Equal(t, 0.0, billing.commands[0].BalanceCost)
REDACTED

func TestBatchImageSettlementService_CompletedJobReturnsAlreadySettledWithoutBilling(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_done")
	job.Status = BatchImageJobStatusCompleted
	cost := 0.5
	job.ActualCost = &cost
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{REDACTED
	svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED

	result, err := svc.Settle(context.Background(), job.BatchID)
REDACTED
	require.True(t, result.AlreadySettled)
	require.Equal(t, 0.5, result.ActualCost)
	require.Empty(t, billing.commands)
REDACTED

func TestBatchImageSettlementService_IdempotentAfterBillingCrash(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_crash")
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{alreadyApplied: map[string]bool{BatchImageSettlementRequestID(job.BatchID): trueREDACTEDREDACTED
	svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED

	result, err := svc.Settle(context.Background(), job.BatchID)
REDACTED
	require.Equal(t, 0.5, result.ActualCost)
	require.Equal(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	require.Len(t, billing.commands, 1)
REDACTED

func TestBatchImageSettlementService_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*BatchImageJob)
		pricing BatchImagePricingResolver
		want    error
REDACTED{
		{name: "invalid_status", mutate: func(j *BatchImageJob) { j.Status = BatchImageJobStatusRunning REDACTED, want: ErrBatchImageSettlementInvalidStatusREDACTED,
		{name: "negative_success_count", mutate: func(j *BatchImageJob) { j.SuccessCount = -1 REDACTED, want: ErrBatchImageSettlementInvalidCountsREDACTED,
		{name: "negative_fail_count", mutate: func(j *BatchImageJob) { j.FailCount = -1 REDACTED, want: ErrBatchImageSettlementInvalidCountsREDACTED,
		{name: "missing_api_key", mutate: func(j *BatchImageJob) { j.APIKeyID = nil REDACTED, want: ErrBatchImageSettlementMissingAPIKeyIDREDACTED,
		{name: "missing_account", mutate: func(j *BatchImageJob) { j.AccountID = nil REDACTED, want: ErrBatchImageSettlementMissingAccountIDREDACTED,
		{name: "pricing_missing", pricing: &fakeBatchImagePricingResolver{err: ErrBatchImageSettlementPricingMissingREDACTED, want: ErrBatchImageSettlementPricingMissingREDACTED,
		{name: "manifest_conflict", mutate: func(j *BatchImageJob) { v := "different"; j.ManifestHash = &v REDACTED, want: ErrBatchImageSettlementManifestConflictREDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeBatchImageRepository()
			job := testSettlingBatchImageJob("imgbatch_" + tt.name)
			if tt.mutate != nil {
				tt.mutate(job)
		REDACTED
			repo.jobs[job.BatchID] = job
			pricing := tt.pricing
			if pricing == nil {
				pricing = &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTED
		REDACTED
			billing := &fakeBatchImageBillingRepo{REDACTED
			svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: pricingREDACTED

			_, err := svc.Settle(context.Background(), job.BatchID)
			require.ErrorIs(t, err, tt.want)
			require.Empty(t, billing.commands)
			require.NotEqual(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	REDACTED)
REDACTED
REDACTED

func TestBatchImageSettlementService_BillingFailureLeavesSettlingAndRecordsError(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_billing_fail")
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{err: errors.New("temporary billing timeout with gs://hidden-output")REDACTED
	svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.ErrorIs(t, err, ErrBatchImageSettlementBillingFailed)
	require.Equal(t, BatchImageJobStatusSettling, repo.jobs[job.BatchID].Status)
	require.Equal(t, "SETTLEMENT_BILLING_FAILED", batchImageDerefString(repo.jobs[job.BatchID].LastErrorCode))
	require.Contains(t, batchImageDerefString(repo.jobs[job.BatchID].LastErrorMessage), "temporary billing timeout")
	require.NotNil(t, billing.commands[0])
REDACTED

func TestBatchImagePipelineProcessor_SettlesQueuedSettlingJob(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_pipeline")
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{REDACTED
	settlement := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED
	processor := &BatchImagePipelineProcessor{
		ProviderProcessor: &BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&fakeProcessorProvider{REDACTED), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTEDREDACTED,
		SettlementService: settlement,
REDACTED

	result, err := processor.Process(context.Background(), job.BatchID)
REDACTED
	require.True(t, result.Terminal)
	require.Equal(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	require.Len(t, billing.commands, 1)
REDACTED

func TestBatchImagePipelineProcessor_RequeuesTransientSettlementFailure(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_pipeline_retry")
	repo.jobs[job.BatchID] = job
	settlement := &BatchImageSettlementService{Repo: repo, BillingRepo: &fakeBatchImageBillingRepo{err: errors.New("temporary")REDACTED, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED
	processor := &BatchImagePipelineProcessor{
		ProviderProcessor: &BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&fakeProcessorProvider{REDACTED), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTEDREDACTED,
		SettlementService: settlement,
REDACTED

	result, err := processor.Process(context.Background(), job.BatchID)
REDACTED
	require.False(t, result.Terminal)
	require.Equal(t, batchImageSettlementRetryDelay, result.RequeueAfter)
	require.Equal(t, BatchImageJobStatusSettling, repo.jobs[job.BatchID].Status)
REDACTED

func TestBatchImageSettlementManifestHash(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_hash")
	first := BuildBatchImageSettlementManifestHash(job)
	job.CreatedAt = job.CreatedAt.AddDate(0, 0, 1)
	job.UpdatedAt = job.UpdatedAt.AddDate(0, 0, 1)
	require.Equal(t, first, BuildBatchImageSettlementManifestHash(job))

	job.SuccessCount++
	require.NotEqual(t, first, BuildBatchImageSettlementManifestHash(job))

	job.SuccessCount--
	promptOrBase64 := first + " prompt " + batchImageTestData
	require.NotContains(t, BuildBatchImageSettlementManifestHash(job), promptOrBase64)
REDACTED

func TestBatchImageSettlementBillingRequestIDs(t *testing.T) {
	repo := newFakeBatchImageRepository()
	first := testSettlingBatchImageJob("imgbatch_unique_1")
	second := testSettlingBatchImageJob("imgbatch_unique_2")
	repo.jobs[first.BatchID] = first
	repo.jobs[second.BatchID] = second
	billing := &fakeBatchImageBillingRepo{REDACTED
	svc := &BatchImageSettlementService{Repo: repo, BillingRepo: billing, Pricing: &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTEDREDACTED

	_, err := svc.Settle(context.Background(), first.BatchID)
REDACTED
	_, err = svc.Settle(context.Background(), first.BatchID)
REDACTED
	_, err = svc.Settle(context.Background(), second.BatchID)
REDACTED

	require.Len(t, billing.commands, 2)
	require.Equal(t, "batch_image_settlement:"+first.BatchID, billing.commands[0].RequestID)
	require.Equal(t, "batch_image_settlement:"+second.BatchID, billing.commands[1].RequestID)
	require.NotEqual(t, billing.commands[0].RequestID, billing.commands[1].RequestID)
	require.Len(t, billing.seen, 2)
REDACTED

func testSettlingBatchImageJob(batchID string) *BatchImageJob {
	apiKeyID := int64(321)
	accountID := int64(654)
	providerJobName := "providers/job"
	outputRef := "files/output"
	return &BatchImageJob{
		BatchID:           batchID,
		UserID:            123,
		APIKeyID:          &apiKeyID,
		AccountID:         &accountID,
		Provider:          BatchImageProviderGeminiAPI,
		Model:             "gemini-image",
		Status:            BatchImageJobStatusSettling,
		ProviderJobName:   &providerJobName,
		ProviderOutputRef: &outputRef,
		ItemCount:         3,
		SuccessCount:      2,
		FailCount:         1,
REDACTED
REDACTED

type fakeBatchImagePricingResolver struct {
	unitPrice float64
	err       error
REDACTED

func (r *fakeBatchImagePricingResolver) BatchImageUnitPrice(context.Context, *BatchImageJob) (float64, error) {
	if r.err != nil {
		return 0, r.err
REDACTED
	return r.unitPrice, nil
REDACTED

type fakeBatchImageBillingRepo struct {
	commands       []*UsageBillingCommand
	seen           map[string]struct{REDACTED
	alreadyApplied map[string]bool
	err            error
REDACTED

func (r *fakeBatchImageBillingRepo) Apply(_ context.Context, cmd *UsageBillingCommand) (*UsageBillingApplyResult, error) {
	if r.seen == nil {
		r.seen = make(map[string]struct{REDACTED)
REDACTED
	if r.err != nil {
		r.commands = append(r.commands, cmd)
		return nil, r.err
REDACTED
	if cmd != nil {
		cmd.Normalize()
		if _, ok := r.seen[cmd.RequestID]; ok || r.alreadyApplied[cmd.RequestID] {
			r.commands = append(r.commands, cmd)
			return &UsageBillingApplyResult{Applied: falseREDACTED, nil
	REDACTED
		r.seen[cmd.RequestID] = struct{REDACTED{REDACTED
REDACTED
	r.commands = append(r.commands, cmd)
	return &UsageBillingApplyResult{Applied: trueREDACTED, nil
REDACTED

var _ UsageBillingRepository = (*fakeBatchImageBillingRepo)(nil)
var _ BatchImagePricingResolver = (*fakeBatchImagePricingResolver)(nil)
var _ = strings.TrimSpace
