//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

const batchImageTestData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ"

func TestParseBatchImageResultLine_SuccessShapes(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantID    string
		wantMime  string
		wantExt   string
		wantCount int
REDACTED{
		{
			name:   "gemini_inlineData",
			line:   `{"key":"cover_001","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED`,
			wantID: "cover_001", wantMime: "image/png", wantExt: "png", wantCount: 1,
	REDACTED,
		{
			name:   "snake_case_inline_data",
			line:   `{"custom_id":"cover_002","response":{"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/jpeg","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED`,
			wantID: "cover_002", wantMime: "image/jpeg", wantExt: "jpg", wantCount: 1,
	REDACTED,
		{
			name:   "vertex_top_level_response",
			line:   `{"customId":"cover_003","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/webp","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED`,
			wantID: "cover_003", wantMime: "image/webp", wantExt: "webp", wantCount: 1,
	REDACTED,
		{
			name:   "top_level_candidates",
			line:   `{"request":{"key":"cover_004"REDACTED,"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/png","data":"` + batchImageTestData + `"REDACTEDREDACTED,{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTED`,
			wantID: "cover_004", wantMime: "image/png", wantExt: "png", wantCount: 2,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBatchImageResultLine([]byte(tt.line), 7)
		REDACTED
			require.Equal(t, tt.wantID, got.CustomID)
			require.Equal(t, BatchImageParsedStatusSucceeded, got.Status)
			require.Equal(t, tt.wantMime, got.MimeType)
			require.Equal(t, tt.wantExt, got.FileExtension)
			require.Equal(t, tt.wantCount, got.ImageCount)
			require.Equal(t, 7, got.SourceLineNumber)
			require.NotContains(t, fmt.Sprintf("%+v", got), batchImageTestData)
	REDACTED)
REDACTED
REDACTED

func TestParseBatchImageResultLine_FailureShapes(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantCode string
REDACTED{
		{name: "status_row", line: `{"key":"cover_001","status":{"code":3,"message":"invalid argument: bad prompt"REDACTEDREDACTED`, wantCode: "INVALID_ARGUMENT"REDACTED,
		{name: "error_row", line: `{"key":"cover_002","error":{"code":"SAFETY","message":"blocked by safety policy"REDACTEDREDACTED`, wantCode: "SAFETY_BLOCKED"REDACTED,
		{name: "quota_row", line: `{"key":"cover_003","error":{"code":"RESOURCE_EXHAUSTED","message":"quota exceeded"REDACTEDREDACTED`, wantCode: "PROVIDER_RATE_LIMITED"REDACTED,
		{name: "empty_image_output", line: `{"key":"cover_004","response":{"candidates":[{"content":{"parts":[{"text":"no image"REDACTED]REDACTEDREDACTED]REDACTEDREDACTED`, wantCode: "EMPTY_IMAGE_OUTPUT"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBatchImageResultLine([]byte(tt.line), 1)
		REDACTED
			require.Equal(t, BatchImageParsedStatusFailed, got.Status)
			require.Equal(t, tt.wantCode, got.ErrorCode)
	REDACTED)
REDACTED
REDACTED

func TestParseBatchImageResultLine_RejectsMissingCustomIDAndDoesNotLeakData(t *testing.T) {
	_, err := ParseBatchImageResultLine([]byte(`{"response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"`+batchImageTestData+`"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED`), 3)
	require.ErrorIs(t, err, ErrBatchImageIndexParseFailed)
	require.NotContains(t, err.Error(), batchImageTestData)
REDACTED

func TestBatchImageResultIndexer_WritesCountsAndReplacesItems(t *testing.T) {
	output := strings.Join([]string{
		`{"key":"ok","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED`,
		`{"key":"bad","error":{"code":"SAFETY","message":"blocked by safety policy"REDACTEDREDACTED`,
REDACTED, "\n") + "\n"
	repo := newFakeBatchImageRepository()
	outputRef := "files/output"
	job := &BatchImageJob{BatchID: "imgbatch_index", ProviderOutputRef: &outputRefREDACTED
	provider := &fakeProcessorProvider{result: outputREDACTED

	result, err := (&BatchImageResultIndexer{Repo: repoREDACTED).Index(context.Background(), job, provider, &Account{REDACTED)
REDACTED
	require.True(t, provider.openResultCalled)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 1, result.FailCount)
	require.Equal(t, 2, result.TotalCount)
	require.Equal(t, 1, repo.replaceCalls)
	require.Len(t, repo.items[job.BatchID], 2)
	require.Equal(t, BatchImageItemStatusSuccess, repo.items[job.BatchID][0].Status)
	require.Equal(t, BatchImageItemStatusFailed, repo.items[job.BatchID][1].Status)
	require.Equal(t, BatchImageCounts{SuccessCount: 1, FailCount: 1REDACTED, repo.counts[job.BatchID])
	require.NotContains(t, fmt.Sprintf("%+v", repo.items[job.BatchID]), batchImageTestData)

	provider.result = `{"key":"ok2","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/webp","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED` + "\n"
	result, err = (&BatchImageResultIndexer{Repo: repoREDACTED).Index(context.Background(), job, provider, &Account{REDACTED)
REDACTED
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, repo.items[job.BatchID], 1)
	require.Equal(t, "ok2", repo.items[job.BatchID][0].CustomID)
REDACTED

func TestBatchImageResultIndexer_EmptyInvalidAndDuplicateOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
REDACTED{
		{name: "empty", body: "\n", want: ErrBatchImageIndexNoResultLinesREDACTED,
		{name: "invalid_json", body: "{bad-jsonREDACTED\n", want: ErrBatchImageIndexParseFailedREDACTED,
		{name: "duplicate_custom_id", body: `{"key":"dup","error":{"message":"one"REDACTEDREDACTED` + "\n" + `{"key":"dup","error":{"message":"two"REDACTEDREDACTED` + "\n", want: ErrBatchImageDuplicateCustomIDREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeBatchImageRepository()
			_, err := (&BatchImageResultIndexer{Repo: repoREDACTED).Index(context.Background(), &BatchImageJob{BatchID: "imgbatch_bad"REDACTED, &fakeProcessorProvider{result: tt.bodyREDACTED, &Account{REDACTED)
			require.ErrorIs(t, err, tt.want)
			require.Empty(t, repo.items["imgbatch_bad"])
	REDACTED)
REDACTED
REDACTED

func TestBatchImageProviderProcessor_ValidationAndTerminalCases(t *testing.T) {
	ctx := context.Background()
	accountID := int64(10)
	providerJob := "providers/job"

	t.Run("terminal job returns without provider call", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_done"] = &BatchImageJob{BatchID: "imgbatch_done", Status: BatchImageJobStatusFailedREDACTED
		provider := &fakeProcessorProvider{REDACTED
		got, err := (&BatchImageProviderProcessor{
			Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(provider), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTED,
	REDACTED).Process(ctx, "imgbatch_done")
	REDACTED
		require.True(t, got.Terminal)
		require.False(t, provider.getCalled)
REDACTED)

	t.Run("missing provider", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_missing_provider"] = &BatchImageJob{BatchID: "imgbatch_missing_provider", Status: BatchImageJobStatusSubmitted, Provider: "missing", AccountID: &accountID, ProviderJobName: &providerJobREDACTED
		_, err := (&BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTEDREDACTED).Process(ctx, "imgbatch_missing_provider")
		require.ErrorIs(t, err, ErrBatchImageUnsupportedProvider)
REDACTED)

	t.Run("missing account id", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_missing_account"] = &BatchImageJob{BatchID: "imgbatch_missing_account", Status: BatchImageJobStatusSubmitted, Provider: "fake", ProviderJobName: &providerJobREDACTED
		_, err := (&BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&fakeProcessorProvider{REDACTED), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTEDREDACTED).Process(ctx, "imgbatch_missing_account")
		require.ErrorIs(t, err, ErrBatchImageMissingAccountID)
REDACTED)

	t.Run("missing provider job name", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_missing_name"] = &BatchImageJob{BatchID: "imgbatch_missing_name", Status: BatchImageJobStatusSubmitted, Provider: "fake", AccountID: &accountIDREDACTED
		_, err := (&BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&fakeProcessorProvider{REDACTED), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTEDREDACTED).Process(ctx, "imgbatch_missing_name")
		require.ErrorIs(t, err, ErrBatchImageMissingProviderJobName)
REDACTED)
REDACTED

func TestBatchImageProviderProcessor_StatusFlow(t *testing.T) {
	ctx := context.Background()
	accountID := int64(10)
	providerJob := "providers/job"
	newJob := func(status string) *BatchImageJob {
		return &BatchImageJob{BatchID: "imgbatch_flow", Status: status, Provider: "fake", AccountID: &accountID, ProviderJobName: &providerJobREDACTED
REDACTED

	t.Run("running status updates and requeues", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateRunning, RawState: "RUNNING", SuggestedRequeueAfter: 12 * time.SecondREDACTEDREDACTED
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
	REDACTED
		require.False(t, got.Terminal)
		require.Equal(t, 12*time.Second, got.RequeueAfter)
		require.Equal(t, BatchImageJobStatusRunning, repo.jobs["imgbatch_flow"].Status)
REDACTED)

	t.Run("queued status requeues", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateQueuedREDACTEDREDACTED
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
	REDACTED
		require.False(t, got.Terminal)
		require.Equal(t, defaultBatchImageProcessorRequeue, got.RequeueAfter)
		require.Equal(t, BatchImageJobStatusSubmitted, repo.jobs["imgbatch_flow"].Status)
REDACTED)

	t.Run("transient provider get error requeues", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{getErr: errors.New("temporary upstream failure")REDACTED
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
	REDACTED
		require.False(t, got.Terminal)
		require.Equal(t, time.Minute, got.RequeueAfter)
REDACTED)

	t.Run("succeeded indexes and settles from submitted", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{
			status: &BatchProviderStatus{InternalState: BatchProviderStateSucceeded, RawState: "SUCCEEDED", ProviderOutputRef: "files/output"REDACTED,
			result: `{"key":"ok","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"REDACTEDREDACTED]REDACTEDREDACTED]REDACTEDREDACTED` + "\n",
	REDACTED
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
	REDACTED
		require.False(t, got.Terminal)
		require.Equal(t, time.Millisecond, got.RequeueAfter)
		require.Equal(t, BatchImageJobStatusSettling, repo.jobs["imgbatch_flow"].Status)
		require.Equal(t, "files/output", batchImageDerefString(repo.jobs["imgbatch_flow"].ProviderOutputRef))
		require.Equal(t, []string{BatchImageJobStatusIndexing, BatchImageJobStatusSettlingREDACTED, repo.transitions["imgbatch_flow"])
		require.Equal(t, BatchImageCounts{SuccessCount: 1REDACTED, repo.counts["imgbatch_flow"])
REDACTED)

	t.Run("failed provider marks job failed", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusRunning)
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateFailed, RawState: "FAILED", ErrorCode: "BAD_PROMPT", ErrorMessage: "bad prompt"REDACTEDREDACTED
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
	REDACTED
		require.True(t, got.Terminal)
		require.Equal(t, BatchImageJobStatusFailed, repo.jobs["imgbatch_flow"].Status)
		require.Equal(t, "BAD_PROMPT", batchImageDerefString(repo.jobs["imgbatch_flow"].LastErrorCode))
REDACTED)

	t.Run("cancelled provider marks job cancelled", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusRunning)
		apiKeyID := int64(22)
		holdAmount := 0.5
		repo.jobs["imgbatch_flow"].UserID = 11
		repo.jobs["imgbatch_flow"].APIKeyID = &apiKeyID
		repo.jobs["imgbatch_flow"].EstimatedCost = holdAmount
		repo.jobs["imgbatch_flow"].HoldAmount = &holdAmount
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateCancelled, RawState: "CANCELLED"REDACTEDREDACTED
		processor := newTestBatchImageProcessor(repo, provider)
		billing := &fakeBatchImageBillingRepo{REDACTED
		processor.BillingRepo = billing
		got, err := processor.Process(ctx, "imgbatch_flow")
	REDACTED
		require.True(t, got.Terminal)
		require.Equal(t, BatchImageJobStatusCancelled, repo.jobs["imgbatch_flow"].Status)
		require.Len(t, billing.releases, 1)
		require.Equal(t, BatchImageReleaseRequestID("imgbatch_flow"), billing.releases[0].RequestID)
REDACTED)
REDACTED

func TestCanTransitionBatchImageJob_PR5DirectIndexing(t *testing.T) {
	require.True(t, CanTransitionBatchImageJob(BatchImageJobStatusSubmitted, BatchImageJobStatusIndexing))
	require.True(t, CanTransitionBatchImageJob(BatchImageJobStatusSubmitted, BatchImageJobStatusFailed))
	require.True(t, CanTransitionBatchImageJob(BatchImageJobStatusIndexing, BatchImageJobStatusFailed))
REDACTED

func newTestBatchImageProcessor(repo *fakeBatchImageRepository, provider *fakeProcessorProvider) *BatchImageProviderProcessor {
	return &BatchImageProviderProcessor{
		Repo:             repo,
		ProviderRegistry: NewBatchImageProviderRegistry(provider),
		AccountResolver:  &fakeBatchImageAccountResolver{account: &Account{REDACTEDREDACTED,
		Indexer:          &BatchImageResultIndexer{Repo: repoREDACTED,
REDACTED
REDACTED

type fakeBatchImageAccountResolver struct {
	account *Account
	err     error
REDACTED

func (r *fakeBatchImageAccountResolver) ResolveBatchImageAccount(context.Context, int64) (*Account, error) {
	if r.err != nil {
		return nil, r.err
REDACTED
	return r.account, nil
REDACTED

type fakeProcessorProvider struct {
	status *BatchProviderStatus
	getErr error
	result string

	getCalled        bool
	openResultCalled bool
REDACTED

func (p *fakeProcessorProvider) Name() string { return "fake" REDACTED
func (p *fakeProcessorProvider) SupportsAccount(*Account) bool {
	return true
REDACTED
func (p *fakeProcessorProvider) Submit(context.Context, *BatchImageJob, *Account, BatchImageInput) (*BatchProviderJob, error) {
	panic("Submit must not be called by PR5 processor")
REDACTED
func (p *fakeProcessorProvider) Get(context.Context, *BatchImageJob, *Account) (*BatchProviderStatus, error) {
	p.getCalled = true
	if p.getErr != nil {
		return nil, p.getErr
REDACTED
	if p.status == nil {
		return &BatchProviderStatus{InternalState: BatchProviderStateQueuedREDACTED, nil
REDACTED
	return p.status, nil
REDACTED
func (p *fakeProcessorProvider) Cancel(context.Context, *BatchImageJob, *Account) error { return nil REDACTED
func (p *fakeProcessorProvider) OpenResult(context.Context, *BatchImageJob, *Account) (io.ReadCloser, string, error) {
	p.openResultCalled = true
	return io.NopCloser(strings.NewReader(p.result)), "application/jsonl", nil
REDACTED
func (p *fakeProcessorProvider) Cleanup(context.Context, *BatchImageJob, *Account, CleanupTarget) error {
	return nil
REDACTED

type fakeBatchImageRepository struct {
	jobs         map[string]*BatchImageJob
	items        map[string][]CreateBatchImageItemParams
	counts       map[string]BatchImageCounts
	transitions  map[string][]string
	events       map[string][]string
	replaceCalls int
REDACTED

func newFakeBatchImageRepository() *fakeBatchImageRepository {
	return &fakeBatchImageRepository{
		jobs:        make(map[string]*BatchImageJob),
		items:       make(map[string][]CreateBatchImageItemParams),
		counts:      make(map[string]BatchImageCounts),
		transitions: make(map[string][]string),
		events:      make(map[string][]string),
REDACTED
REDACTED

func (r *fakeBatchImageRepository) CreateBatchImageJob(_ context.Context, params CreateBatchImageJobParams) (*BatchImageJob, error) {
	job := &BatchImageJob{
		BatchID:                 params.BatchID,
		UserID:                  params.UserID,
		APIKeyID:                params.APIKeyID,
		AccountID:               params.AccountID,
		Status:                  params.Status,
		Provider:                params.Provider,
		Model:                   params.Model,
		TaskName:                params.TaskName,
		ProviderJobName:         params.ProviderJobName,
		ItemCount:               params.ItemCount,
		EstimatedCost:           params.EstimatedCost,
		HoldAmount:              params.HoldAmount,
		HoldID:                  params.HoldID,
		BaseUnitPrice:           params.BaseUnitPrice,
		GroupRateMultiplier:     params.GroupRateMultiplier,
		AccountRateMultiplier:   params.AccountRateMultiplier,
		BatchDiscountMultiplier: params.BatchDiscountMultiplier,
		HoldMultiplier:          params.HoldMultiplier,
		BillableUnitPrice:       params.BillableUnitPrice,
		HoldUnitPrice:           params.HoldUnitPrice,
		PricingSnapshotVersion:  params.PricingSnapshotVersion,
		Currency:                params.Currency,
		IdempotencyKey:          params.IdempotencyKey,
		RequestHash:             params.RequestHash,
		CreatedAt:               time.Now(),
REDACTED
	r.jobs[job.BatchID] = job
	return job, nil
REDACTED

func (r *fakeBatchImageRepository) GetBatchImageJobByBatchID(_ context.Context, batchID string) (*BatchImageJob, error) {
	job, ok := r.jobs[batchID]
	if !ok {
		return nil, ErrBatchImageJobNotFound
REDACTED
	return job, nil
REDACTED

func (r *fakeBatchImageRepository) GetBatchImageJobByIdempotencyKey(_ context.Context, userID, apiKeyID int64, key string) (*BatchImageJob, error) {
	for _, job := range r.jobs {
		if job.UserID == userID && job.APIKeyID != nil && *job.APIKeyID == apiKeyID && batchImageDerefString(job.IdempotencyKey) == key {
			return job, nil
	REDACTED
REDACTED
	return nil, ErrBatchImageJobNotFound
REDACTED

func (r *fakeBatchImageRepository) GetBatchImageJobByBatchIDForOwner(_ context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error) {
	job, ok := r.jobs[batchID]
	if !ok || job.UserID != userID || job.APIKeyID == nil || *job.APIKeyID != apiKeyID {
		return nil, ErrBatchImageJobNotFound
REDACTED
	return job, nil
REDACTED

func (r *fakeBatchImageRepository) ListBatchImageJobsForOwner(_ context.Context, userID, apiKeyID int64, filter BatchImageJobFilter) ([]*BatchImageJob, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
REDACTED
	offset := filter.Offset
	if offset < 0 {
		offset = 0
REDACTED
	var jobs []*BatchImageJob
	for _, job := range r.jobs {
		if job.UserID != userID || job.APIKeyID == nil || *job.APIKeyID != apiKeyID {
			continue
	REDACTED
		if filter.Status != "" && job.Status != filter.Status {
			continue
	REDACTED
		if filter.TaskNameLike != "" && !strings.Contains(strings.ToLower(job.TaskName), strings.ToLower(filter.TaskNameLike)) {
			continue
	REDACTED
		if filter.ExcludeDeleted && job.UserDeletedAt != nil {
			continue
	REDACTED
		if filter.Downloaded != nil {
			downloaded := job.DownloadedAt != nil
			if downloaded != *filter.Downloaded {
				continue
		REDACTED
	REDACTED
		if filter.CreatedAfter != nil && job.CreatedAt.Before(*filter.CreatedAfter) {
			continue
	REDACTED
		if filter.CreatedBefore != nil && !job.CreatedAt.Before(*filter.CreatedBefore) {
			continue
	REDACTED
		if offset > 0 {
			offset--
			continue
	REDACTED
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
	REDACTED
REDACTED
	return jobs, nil
REDACTED

func (r *fakeBatchImageRepository) GetBatchImageJobByID(_ context.Context, id int64) (*BatchImageJob, error) {
	for _, job := range r.jobs {
		if job.ID == id {
			return job, nil
	REDACTED
REDACTED
	return nil, ErrBatchImageJobNotFound
REDACTED

func (r *fakeBatchImageRepository) TransitionBatchImageJobStatus(_ context.Context, batchID, toStatus string, opts BatchImageTransitionOptions) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if !CanTransitionBatchImageJob(job.Status, toStatus) {
		return ErrBatchImageInvalidTransition
REDACTED
	job.Status = toStatus
	job.LastErrorCode = opts.ErrorCode
	job.LastErrorMessage = opts.ErrorMessage
	r.transitions[batchID] = append(r.transitions[batchID], toStatus)
	if opts.EventType != "" {
		r.events[batchID] = append(r.events[batchID], opts.EventType)
REDACTED
	return nil
REDACTED

func (r *fakeBatchImageRepository) UpdateBatchImageJobProviderOutputRef(_ context.Context, batchID, providerOutputRef string) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	job.ProviderOutputRef = &providerOutputRef
	return nil
REDACTED

func (r *fakeBatchImageRepository) UpdateBatchImageJobProviderSubmit(_ context.Context, params UpdateBatchImageJobProviderSubmitParams) error {
	job, ok := r.jobs[params.BatchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if !CanTransitionBatchImageJob(job.Status, BatchImageJobStatusSubmitted) {
		return ErrBatchImageInvalidTransition
REDACTED
	job.Status = BatchImageJobStatusSubmitted
	job.ProviderJobName = batchImageOptionalStringPtr(params.ProviderJobName)
	job.ProviderInputRef = batchImageOptionalStringPtr(params.ProviderInputRef)
	job.ProviderOutputRef = batchImageOptionalStringPtr(params.ProviderOutputRef)
	job.GCSInputURI = batchImageOptionalStringPtr(params.GCSInputURI)
	job.GCSOutputURI = batchImageOptionalStringPtr(params.GCSOutputURI)
	now := time.Now()
	job.SubmittedAt = &now
	r.transitions[params.BatchID] = append(r.transitions[params.BatchID], BatchImageJobStatusSubmitted)
	r.events[params.BatchID] = append(r.events[params.BatchID], "provider_submitted")
	return nil
REDACTED

func (r *fakeBatchImageRepository) RecordBatchImageJobSubmitFailure(_ context.Context, batchID, code, message string, markFailed bool) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if markFailed {
		job.Status = BatchImageJobStatusFailed
REDACTED
	job.LastErrorCode = batchImageOptionalStringPtr(code)
	job.LastErrorMessage = batchImageOptionalStringPtr(message)
	eventType := "submit_failed"
	if !markFailed {
		eventType = "queue_failed"
REDACTED
	r.events[batchID] = append(r.events[batchID], eventType)
	return nil
REDACTED

func (r *fakeBatchImageRepository) MarkBatchImageJobSettled(_ context.Context, params MarkBatchImageJobSettledParams) error {
	job, ok := r.jobs[params.BatchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if job.Status != BatchImageJobStatusSettling {
		if job.Status == BatchImageJobStatusCompleted {
			return ErrBatchImageAlreadySettled
	REDACTED
		return ErrBatchImageSettlementInvalidStatus
REDACTED
	if batchImageDerefString(job.ManifestHash) != "" && batchImageDerefString(job.ManifestHash) != params.ManifestHash {
		return ErrBatchImageSettlementManifestConflict
REDACTED
	now := time.Now()
	job.Status = BatchImageJobStatusCompleted
	job.ActualCost = &params.ActualCost
	job.ManifestHash = &params.ManifestHash
	job.SettledAt = &now
	if job.OutputExpiresAt == nil && params.OutputExpiresAt != nil {
		job.OutputExpiresAt = params.OutputExpiresAt
REDACTED
	r.transitions[params.BatchID] = append(r.transitions[params.BatchID], BatchImageJobStatusCompleted)
	r.events[params.BatchID] = append(r.events[params.BatchID], "settlement_completed")
	return nil
REDACTED

func (r *fakeBatchImageRepository) SetBatchImageJobSettlementFailed(_ context.Context, batchID, code, message string) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	job.LastErrorCode = batchImageStringPtr(code)
	job.LastErrorMessage = batchImageOptionalStringPtr(message)
	r.events[batchID] = append(r.events[batchID], "settlement_failed")
	return nil
REDACTED

func (r *fakeBatchImageRepository) CreateBatchImageItem(_ context.Context, params CreateBatchImageItemParams) (*BatchImageItem, error) {
	r.items[params.JobID] = append(r.items[params.JobID], params)
	return &BatchImageItem{JobID: params.JobID, CustomID: params.CustomID, Status: params.StatusREDACTED, nil
REDACTED

func (r *fakeBatchImageRepository) BulkCreateBatchImageItems(ctx context.Context, params []CreateBatchImageItemParams) error {
	for _, param := range params {
		if _, err := r.CreateBatchImageItem(ctx, param); err != nil {
			return err
	REDACTED
REDACTED
	return nil
REDACTED

func (r *fakeBatchImageRepository) ReplaceBatchImageItemsForJob(_ context.Context, batchID string, items []CreateBatchImageItemParams, counts BatchImageCounts) error {
	r.replaceCalls++
	copied := append([]CreateBatchImageItemParams(nil), items...)
	for idx := range copied {
		copied[idx].JobID = batchID
REDACTED
	r.items[batchID] = copied
	r.counts[batchID] = counts
	if job, ok := r.jobs[batchID]; ok {
		job.SuccessCount = counts.SuccessCount
		job.FailCount = counts.FailCount
		job.ItemCount = len(copied)
REDACTED
	return nil
REDACTED

func (r *fakeBatchImageRepository) ListBatchImageItems(_ context.Context, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
REDACTED
	offset := filter.Offset
	if offset < 0 {
		offset = 0
REDACTED
	var result []*BatchImageItem
	for _, item := range r.items[batchID] {
		if filter.Status != "" && item.Status != filter.Status {
			continue
	REDACTED
		if offset > 0 {
			offset--
			continue
	REDACTED
		result = append(result, &BatchImageItem{
			JobID:                item.JobID,
			CustomID:             item.CustomID,
			Status:               item.Status,
			RequestHash:          item.RequestHash,
			PromptPreview:        item.PromptPreview,
			ProviderSourceObject: item.ProviderSourceObject,
			SourceLineNumber:     item.SourceLineNumber,
			SourceByteOffset:     item.SourceByteOffset,
			SourceByteLength:     item.SourceByteLength,
			MimeType:             item.MimeType,
			FileExtension:        item.FileExtension,
			ImageCount:           item.ImageCount,
			ErrorCode:            item.ErrorCode,
			ErrorMessage:         item.ErrorMessage,
			BilledAmount:         item.BilledAmount,
			IndexedAt:            item.IndexedAt,
	REDACTED)
		if len(result) >= limit {
			break
	REDACTED
REDACTED
	return result, nil
REDACTED

func (r *fakeBatchImageRepository) ListBatchImageItemsForOwner(ctx context.Context, userID, apiKeyID int64, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error) {
	if _, err := r.GetBatchImageJobByBatchIDForOwner(ctx, userID, apiKeyID, batchID); err != nil {
		return nil, err
REDACTED
	return r.ListBatchImageItems(ctx, batchID, filter)
REDACTED

func (r *fakeBatchImageRepository) GetBatchImageJobForDownload(ctx context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error) {
	return r.GetBatchImageJobByBatchIDForOwner(ctx, userID, apiKeyID, batchID)
REDACTED

func (r *fakeBatchImageRepository) GetBatchImageItemForDownload(_ context.Context, batchID, customID string) (*BatchImageItem, error) {
	for _, item := range r.items[batchID] {
		if item.CustomID != customID {
			continue
	REDACTED
		return &BatchImageItem{
			JobID:                item.JobID,
			CustomID:             item.CustomID,
			Status:               item.Status,
			RequestHash:          item.RequestHash,
			PromptPreview:        item.PromptPreview,
			ProviderSourceObject: item.ProviderSourceObject,
			SourceLineNumber:     item.SourceLineNumber,
			SourceByteOffset:     item.SourceByteOffset,
			SourceByteLength:     item.SourceByteLength,
			MimeType:             item.MimeType,
			FileExtension:        item.FileExtension,
			ImageCount:           item.ImageCount,
			ErrorCode:            item.ErrorCode,
			ErrorMessage:         item.ErrorMessage,
			BilledAmount:         item.BilledAmount,
			IndexedAt:            item.IndexedAt,
	REDACTED, nil
REDACTED
	return nil, ErrBatchImageItemNotFound
REDACTED

func (r *fakeBatchImageRepository) ListBatchImageItemsForDownload(ctx context.Context, batchID string, status string, limit int) ([]*BatchImageItem, error) {
	return r.ListBatchImageItems(ctx, batchID, BatchImageItemFilter{Status: status, Limit: limitREDACTED)
REDACTED

func (r *fakeBatchImageRepository) ListBatchImageJobsDueForInputCleanup(_ context.Context, cutoff time.Time, limit int) ([]*BatchImageJob, error) {
	if limit <= 0 {
		limit = 100
REDACTED
	var jobs []*BatchImageJob
	for _, job := range r.jobs {
		if job.InputDeletedAt != nil || batchImageDerefString(job.ProviderInputRef) == "" || !IsTerminalBatchImageJobStatus(job.Status) {
			continue
	REDACTED
		at := job.FinishedAt
		if at == nil {
			at = job.SettledAt
	REDACTED
		if at == nil {
			at = &job.UpdatedAt
	REDACTED
		if at != nil && at.After(cutoff) {
			continue
	REDACTED
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
	REDACTED
REDACTED
	return jobs, nil
REDACTED

func (r *fakeBatchImageRepository) ListBatchImageJobsDueForOutputCleanup(_ context.Context, now time.Time, limit int) ([]*BatchImageJob, error) {
	if limit <= 0 {
		limit = 100
REDACTED
	var jobs []*BatchImageJob
	for _, job := range r.jobs {
		if job.OutputDeletedAt != nil || batchImageDerefString(job.ProviderOutputRef) == "" || job.Status != BatchImageJobStatusCompleted || job.OutputExpiresAt == nil || job.OutputExpiresAt.After(now) {
			continue
	REDACTED
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
	REDACTED
REDACTED
	return jobs, nil
REDACTED

func (r *fakeBatchImageRepository) ListStaleUnsubmittedBatchImageJobs(_ context.Context, cutoff time.Time, limit int) ([]*BatchImageJob, error) {
	if limit <= 0 {
		limit = 100
REDACTED
	jobs := make([]*BatchImageJob, 0, limit)
	for _, job := range r.jobs {
		if len(jobs) >= limit {
			break
	REDACTED
		if job.Status != BatchImageJobStatusCreated && job.Status != BatchImageJobStatusUploading {
			continue
	REDACTED
		if batchImageDerefString(job.ProviderJobName) != "" {
			continue
	REDACTED
		holdAmount := job.EstimatedCost
		if job.HoldAmount != nil {
			holdAmount = *job.HoldAmount
	REDACTED
		if holdAmount <= 0 || job.UpdatedAt.After(cutoff) {
			continue
	REDACTED
		jobs = append(jobs, job)
REDACTED
	return jobs, nil
REDACTED

func (r *fakeBatchImageRepository) MarkBatchImageInputDeleted(_ context.Context, batchID string, deletedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if job.InputDeletedAt == nil {
		job.InputDeletedAt = &deletedAt
REDACTED
	r.events[batchID] = append(r.events[batchID], "input_cleanup_completed")
	return nil
REDACTED

func (r *fakeBatchImageRepository) MarkBatchImageOutputDeleted(_ context.Context, batchID string, deletedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if job.OutputDeletedAt == nil {
		job.OutputDeletedAt = &deletedAt
REDACTED
	if job.Status == BatchImageJobStatusCompleted {
		job.Status = BatchImageJobStatusOutputDeleted
REDACTED
	r.events[batchID] = append(r.events[batchID], "output_cleanup_completed")
	return nil
REDACTED

func (r *fakeBatchImageRepository) MarkBatchImageDownloaded(_ context.Context, batchID string, downloadedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if job.DownloadedAt == nil {
		job.DownloadedAt = &downloadedAt
REDACTED
	r.events[batchID] = append(r.events[batchID], "download_completed")
	return nil
REDACTED

func (r *fakeBatchImageRepository) MarkBatchImageJobUserDeleted(_ context.Context, userID, apiKeyID int64, batchID string, deletedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok || job.UserID != userID || job.APIKeyID == nil || *job.APIKeyID != apiKeyID {
		return ErrBatchImageJobNotFound
REDACTED
	if !isBatchImageProcessorDoneStatus(job.Status) {
		return ErrBatchImageRecordDeleteNotReady
REDACTED
	if job.UserDeletedAt == nil {
		job.UserDeletedAt = &deletedAt
REDACTED
	r.events[batchID] = append(r.events[batchID], "user_record_deleted")
	return nil
REDACTED

func (r *fakeBatchImageRepository) SetBatchImageOutputExpiresAt(_ context.Context, batchID string, expiresAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	if job.OutputExpiresAt == nil {
		job.OutputExpiresAt = &expiresAt
REDACTED
	return nil
REDACTED

func (r *fakeBatchImageRepository) RecordBatchImageCleanupFailure(_ context.Context, batchID, code, message string) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
REDACTED
	job.LastErrorCode = batchImageStringPtr(code)
	job.LastErrorMessage = batchImageOptionalStringPtr(message)
	r.events[batchID] = append(r.events[batchID], "output_cleanup_failed")
	return nil
REDACTED

func (r *fakeBatchImageRepository) AppendBatchImageEvent(_ context.Context, batchID, eventType string, _ any) error {
	r.events[batchID] = append(r.events[batchID], eventType)
	return nil
REDACTED

var _ BatchImageRepository = (*fakeBatchImageRepository)(nil)
var _ BatchImageProvider = (*fakeProcessorProvider)(nil)
var _ BatchImageAccountResolver = (*fakeBatchImageAccountResolver)(nil)
var _ = infraerrors.Reason
