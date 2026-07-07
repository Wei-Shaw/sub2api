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
	}{
		{
			name:   "gemini_inlineData",
			line:   `{"key":"cover_001","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"}}]}}]}}`,
			wantID: "cover_001", wantMime: "image/png", wantExt: "png", wantCount: 1,
		},
		{
			name:   "snake_case_inline_data",
			line:   `{"custom_id":"cover_002","response":{"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/jpeg","data":"` + batchImageTestData + `"}}]}}]}}`,
			wantID: "cover_002", wantMime: "image/jpeg", wantExt: "jpg", wantCount: 1,
		},
		{
			name:   "vertex_top_level_response",
			line:   `{"customId":"cover_003","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/webp","data":"` + batchImageTestData + `"}}]}}]}}`,
			wantID: "cover_003", wantMime: "image/webp", wantExt: "webp", wantCount: 1,
		},
		{
			name:   "top_level_candidates",
			line:   `{"request":{"key":"cover_004"},"candidates":[{"content":{"parts":[{"inline_data":{"mime_type":"image/png","data":"` + batchImageTestData + `"}},{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"}}]}}]}`,
			wantID: "cover_004", wantMime: "image/png", wantExt: "png", wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBatchImageResultLine([]byte(tt.line), 7)
			require.NoError(t, err)
			require.Equal(t, tt.wantID, got.CustomID)
			require.Equal(t, BatchImageParsedStatusSucceeded, got.Status)
			require.Equal(t, tt.wantMime, got.MimeType)
			require.Equal(t, tt.wantExt, got.FileExtension)
			require.Equal(t, tt.wantCount, got.ImageCount)
			require.Equal(t, 7, got.SourceLineNumber)
			require.NotContains(t, fmt.Sprintf("%+v", got), batchImageTestData)
		})
	}
}

func TestParseBatchImageResultLine_FailureShapes(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantCode string
	}{
		{name: "status_row", line: `{"key":"cover_001","status":{"code":3,"message":"invalid argument: bad prompt"}}`, wantCode: "INVALID_ARGUMENT"},
		{name: "error_row", line: `{"key":"cover_002","error":{"code":"SAFETY","message":"blocked by safety policy"}}`, wantCode: "SAFETY_BLOCKED"},
		{name: "quota_row", line: `{"key":"cover_003","error":{"code":"RESOURCE_EXHAUSTED","message":"quota exceeded"}}`, wantCode: "PROVIDER_RATE_LIMITED"},
		{name: "empty_image_output", line: `{"key":"cover_004","response":{"candidates":[{"content":{"parts":[{"text":"no image"}]}}]}}`, wantCode: "EMPTY_IMAGE_OUTPUT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBatchImageResultLine([]byte(tt.line), 1)
			require.NoError(t, err)
			require.Equal(t, BatchImageParsedStatusFailed, got.Status)
			require.Equal(t, tt.wantCode, got.ErrorCode)
		})
	}
}

func TestParseBatchImageResultLine_RejectsMissingCustomIDAndDoesNotLeakData(t *testing.T) {
	_, err := ParseBatchImageResultLine([]byte(`{"response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"`+batchImageTestData+`"}}]}}]}}`), 3)
	require.ErrorIs(t, err, ErrBatchImageIndexParseFailed)
	require.NotContains(t, err.Error(), batchImageTestData)
}

func TestBatchImageResultIndexer_WritesCountsAndReplacesItems(t *testing.T) {
	output := strings.Join([]string{
		`{"key":"ok","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"}}]}}]}}`,
		`{"key":"bad","error":{"code":"SAFETY","message":"blocked by safety policy"}}`,
	}, "\n") + "\n"
	repo := newFakeBatchImageRepository()
	outputRef := "files/output"
	job := &BatchImageJob{BatchID: "imgbatch_index", ProviderOutputRef: &outputRef}
	provider := &fakeProcessorProvider{result: output}

	result, err := (&BatchImageResultIndexer{Repo: repo}).Index(context.Background(), job, provider, &Account{})
	require.NoError(t, err)
	require.True(t, provider.openResultCalled)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 1, result.FailCount)
	require.Equal(t, 2, result.TotalCount)
	require.Equal(t, 1, repo.replaceCalls)
	require.Len(t, repo.items[job.BatchID], 2)
	require.Equal(t, BatchImageItemStatusSuccess, repo.items[job.BatchID][0].Status)
	require.Equal(t, BatchImageItemStatusFailed, repo.items[job.BatchID][1].Status)
	require.Equal(t, BatchImageCounts{SuccessCount: 1, FailCount: 1}, repo.counts[job.BatchID])
	require.NotContains(t, fmt.Sprintf("%+v", repo.items[job.BatchID]), batchImageTestData)

	// 重新索引时与现有 custom_id 集对账：未知的 "ok2" 被丢弃，
	// 输出中缺失的 ok/bad 补为 PROVIDER_RESULT_MISSING 失败记录。
	provider.result = `{"key":"ok2","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/webp","data":"` + batchImageTestData + `"}}]}}]}}` + "\n"
	result, err = (&BatchImageResultIndexer{Repo: repo}).Index(context.Background(), job, provider, &Account{})
	require.NoError(t, err)
	require.Equal(t, 2, result.TotalCount)
	require.Equal(t, 0, result.SuccessCount)
	require.Equal(t, 2, result.FailCount)
	require.Len(t, repo.items[job.BatchID], 2)
	gotIDs := []string{repo.items[job.BatchID][0].CustomID, repo.items[job.BatchID][1].CustomID}
	require.ElementsMatch(t, []string{"ok", "bad"}, gotIDs)
	for _, item := range repo.items[job.BatchID] {
		require.Equal(t, BatchImageItemStatusFailed, item.Status)
		require.Equal(t, "PROVIDER_RESULT_MISSING", batchImageDerefString(item.ErrorCode))
	}
}

func TestBatchImageResultIndexer_ReconcilesMissingAndUnknownCustomIDs(t *testing.T) {
	repo := newFakeBatchImageRepository()
	outputRef := "files/output"
	job := &BatchImageJob{BatchID: "imgbatch_reconcile", ProviderOutputRef: &outputRef, ItemCount: 3}
	// 预创建提交时的 pending 条目（提交流程的行为）。
	require.NoError(t, repo.BulkCreateBatchImageItems(context.Background(), []CreateBatchImageItemParams{
		{JobID: job.BatchID, CustomID: "a", Status: BatchImageItemStatusPending},
		{JobID: job.BatchID, CustomID: "b", Status: BatchImageItemStatusPending},
		{JobID: job.BatchID, CustomID: "c", Status: BatchImageItemStatusPending},
	}))
	// provider 输出：a 成功，b 失败，c 漏掉，多出未知的 x。
	output := strings.Join([]string{
		`{"key":"a","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"}}]}}]}}`,
		`{"key":"b","error":{"code":"SAFETY","message":"blocked"}}`,
		`{"key":"x","error":{"code":"UNKNOWN","message":"not ours"}}`,
	}, "\n") + "\n"
	provider := &fakeProcessorProvider{result: output}

	result, err := (&BatchImageResultIndexer{Repo: repo}).Index(context.Background(), job, provider, &Account{})
	require.NoError(t, err)
	require.Equal(t, 3, result.TotalCount)
	require.Equal(t, 1, result.SuccessCount)
	require.Equal(t, 2, result.FailCount)
	require.Len(t, repo.items[job.BatchID], 3)
	byID := make(map[string]CreateBatchImageItemParams)
	for _, item := range repo.items[job.BatchID] {
		byID[item.CustomID] = item
	}
	require.NotContains(t, byID, "x")
	require.Equal(t, BatchImageItemStatusSuccess, byID["a"].Status)
	require.Equal(t, BatchImageItemStatusFailed, byID["b"].Status)
	require.Equal(t, BatchImageItemStatusFailed, byID["c"].Status)
	require.Equal(t, "PROVIDER_RESULT_MISSING", batchImageDerefString(byID["c"].ErrorCode))
	// 对账后 success+fail == item_count，结算计数校验可通过。
	require.Equal(t, job.ItemCount, result.SuccessCount+result.FailCount)
}

func TestBatchImageResultIndexer_EmptyInvalidAndDuplicateOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{name: "empty", body: "\n", want: ErrBatchImageIndexNoResultLines},
		{name: "invalid_json", body: "{bad-json}\n", want: ErrBatchImageIndexParseFailed},
		{name: "duplicate_custom_id", body: `{"key":"dup","error":{"message":"one"}}` + "\n" + `{"key":"dup","error":{"message":"two"}}` + "\n", want: ErrBatchImageDuplicateCustomID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeBatchImageRepository()
			_, err := (&BatchImageResultIndexer{Repo: repo}).Index(context.Background(), &BatchImageJob{BatchID: "imgbatch_bad"}, &fakeProcessorProvider{result: tt.body}, &Account{})
			require.ErrorIs(t, err, tt.want)
			require.Empty(t, repo.items["imgbatch_bad"])
		})
	}
}

func TestBatchImageProviderProcessor_ValidationAndTerminalCases(t *testing.T) {
	ctx := context.Background()
	accountID := int64(10)
	providerJob := "providers/job"

	t.Run("terminal job returns without provider call", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_done"] = &BatchImageJob{BatchID: "imgbatch_done", Status: BatchImageJobStatusFailed}
		provider := &fakeProcessorProvider{}
		got, err := (&BatchImageProviderProcessor{
			Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(provider), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{}},
		}).Process(ctx, "imgbatch_done")
		require.NoError(t, err)
		require.True(t, got.Terminal)
		require.False(t, provider.getCalled)
	})

	t.Run("missing provider", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_missing_provider"] = &BatchImageJob{BatchID: "imgbatch_missing_provider", Status: BatchImageJobStatusSubmitted, Provider: "missing", AccountID: &accountID, ProviderJobName: &providerJob}
		_, err := (&BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{}}}).Process(ctx, "imgbatch_missing_provider")
		require.ErrorIs(t, err, ErrBatchImageUnsupportedProvider)
	})

	t.Run("missing account id", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_missing_account"] = &BatchImageJob{BatchID: "imgbatch_missing_account", Status: BatchImageJobStatusSubmitted, Provider: "fake", ProviderJobName: &providerJob}
		_, err := (&BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&fakeProcessorProvider{}), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{}}}).Process(ctx, "imgbatch_missing_account")
		require.ErrorIs(t, err, ErrBatchImageMissingAccountID)
	})

	t.Run("missing provider job name", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_missing_name"] = &BatchImageJob{BatchID: "imgbatch_missing_name", Status: BatchImageJobStatusSubmitted, Provider: "fake", AccountID: &accountID}
		_, err := (&BatchImageProviderProcessor{Repo: repo, ProviderRegistry: NewBatchImageProviderRegistry(&fakeProcessorProvider{}), AccountResolver: &fakeBatchImageAccountResolver{account: &Account{}}}).Process(ctx, "imgbatch_missing_name")
		require.ErrorIs(t, err, ErrBatchImageMissingProviderJobName)
	})
}

func TestBatchImageProviderProcessor_StatusFlow(t *testing.T) {
	ctx := context.Background()
	accountID := int64(10)
	providerJob := "providers/job"
	newJob := func(status string) *BatchImageJob {
		return &BatchImageJob{BatchID: "imgbatch_flow", Status: status, Provider: "fake", AccountID: &accountID, ProviderJobName: &providerJob}
	}

	t.Run("running status updates and requeues", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateRunning, RawState: "RUNNING", SuggestedRequeueAfter: 12 * time.Second}}
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
		require.NoError(t, err)
		require.False(t, got.Terminal)
		require.Equal(t, 12*time.Second, got.RequeueAfter)
		require.Equal(t, BatchImageJobStatusRunning, repo.jobs["imgbatch_flow"].Status)
	})

	t.Run("queued status requeues", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateQueued}}
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
		require.NoError(t, err)
		require.False(t, got.Terminal)
		require.Equal(t, defaultBatchImageProcessorRequeue, got.RequeueAfter)
		require.Equal(t, BatchImageJobStatusSubmitted, repo.jobs["imgbatch_flow"].Status)
	})

	t.Run("transient provider get error requeues", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{getErr: errors.New("temporary upstream failure")}
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
		require.NoError(t, err)
		require.False(t, got.Terminal)
		require.Equal(t, time.Minute, got.RequeueAfter)
	})

	t.Run("succeeded indexes and settles from submitted", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusSubmitted)
		provider := &fakeProcessorProvider{
			status: &BatchProviderStatus{InternalState: BatchProviderStateSucceeded, RawState: "SUCCEEDED", ProviderOutputRef: "files/output"},
			result: `{"key":"ok","response":{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + batchImageTestData + `"}}]}}]}}` + "\n",
		}
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
		require.NoError(t, err)
		require.False(t, got.Terminal)
		require.Equal(t, time.Millisecond, got.RequeueAfter)
		require.Equal(t, BatchImageJobStatusSettling, repo.jobs["imgbatch_flow"].Status)
		require.Equal(t, "files/output", batchImageDerefString(repo.jobs["imgbatch_flow"].ProviderOutputRef))
		require.Equal(t, []string{BatchImageJobStatusIndexing, BatchImageJobStatusSettling}, repo.transitions["imgbatch_flow"])
		require.Equal(t, BatchImageCounts{SuccessCount: 1}, repo.counts["imgbatch_flow"])
	})

	t.Run("failed provider marks job failed", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusRunning)
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateFailed, RawState: "FAILED", ErrorCode: "BAD_PROMPT", ErrorMessage: "bad prompt"}}
		got, err := newTestBatchImageProcessor(repo, provider).Process(ctx, "imgbatch_flow")
		require.NoError(t, err)
		require.True(t, got.Terminal)
		require.Equal(t, BatchImageJobStatusFailed, repo.jobs["imgbatch_flow"].Status)
		require.Equal(t, "BAD_PROMPT", batchImageDerefString(repo.jobs["imgbatch_flow"].LastErrorCode))
	})

	t.Run("cancelled provider marks job cancelled", func(t *testing.T) {
		repo := newFakeBatchImageRepository()
		repo.jobs["imgbatch_flow"] = newJob(BatchImageJobStatusRunning)
		apiKeyID := int64(22)
		holdAmount := 0.5
		repo.jobs["imgbatch_flow"].UserID = 11
		repo.jobs["imgbatch_flow"].APIKeyID = &apiKeyID
		repo.jobs["imgbatch_flow"].EstimatedCost = holdAmount
		repo.jobs["imgbatch_flow"].HoldAmount = &holdAmount
		provider := &fakeProcessorProvider{status: &BatchProviderStatus{InternalState: BatchProviderStateCancelled, RawState: "CANCELLED"}}
		processor := newTestBatchImageProcessor(repo, provider)
		billing := &fakeBatchImageBillingRepo{}
		processor.BillingRepo = billing
		got, err := processor.Process(ctx, "imgbatch_flow")
		require.NoError(t, err)
		require.True(t, got.Terminal)
		require.Equal(t, BatchImageJobStatusCancelled, repo.jobs["imgbatch_flow"].Status)
		require.Len(t, billing.releases, 1)
		require.Equal(t, BatchImageReleaseRequestID("imgbatch_flow"), billing.releases[0].RequestID)
	})
}

func TestCanTransitionBatchImageJob_PR5DirectIndexing(t *testing.T) {
	require.True(t, CanTransitionBatchImageJob(BatchImageJobStatusSubmitted, BatchImageJobStatusIndexing))
	require.True(t, CanTransitionBatchImageJob(BatchImageJobStatusSubmitted, BatchImageJobStatusFailed))
	require.True(t, CanTransitionBatchImageJob(BatchImageJobStatusIndexing, BatchImageJobStatusFailed))
}

func newTestBatchImageProcessor(repo *fakeBatchImageRepository, provider *fakeProcessorProvider) *BatchImageProviderProcessor {
	return &BatchImageProviderProcessor{
		Repo:             repo,
		ProviderRegistry: NewBatchImageProviderRegistry(provider),
		AccountResolver:  &fakeBatchImageAccountResolver{account: &Account{}},
		Indexer:          &BatchImageResultIndexer{Repo: repo},
	}
}

type fakeBatchImageAccountResolver struct {
	account *Account
	err     error
}

func (r *fakeBatchImageAccountResolver) ResolveBatchImageAccount(context.Context, int64) (*Account, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.account, nil
}

type fakeProcessorProvider struct {
	status *BatchProviderStatus
	getErr error
	result string

	getCalled        bool
	openResultCalled bool
}

func (p *fakeProcessorProvider) Name() string { return "fake" }
func (p *fakeProcessorProvider) SupportsAccount(*Account) bool {
	return true
}
func (p *fakeProcessorProvider) Submit(context.Context, *BatchImageJob, *Account, BatchImageInput) (*BatchProviderJob, error) {
	panic("Submit must not be called by PR5 processor")
}
func (p *fakeProcessorProvider) Get(context.Context, *BatchImageJob, *Account) (*BatchProviderStatus, error) {
	p.getCalled = true
	if p.getErr != nil {
		return nil, p.getErr
	}
	if p.status == nil {
		return &BatchProviderStatus{InternalState: BatchProviderStateQueued}, nil
	}
	return p.status, nil
}
func (p *fakeProcessorProvider) Cancel(context.Context, *BatchImageJob, *Account) error { return nil }
func (p *fakeProcessorProvider) OpenResult(context.Context, *BatchImageJob, *Account) (io.ReadCloser, string, error) {
	p.openResultCalled = true
	return io.NopCloser(strings.NewReader(p.result)), "application/jsonl", nil
}
func (p *fakeProcessorProvider) Cleanup(context.Context, *BatchImageJob, *Account, CleanupTarget) error {
	return nil
}

type fakeBatchImageRepository struct {
	jobs          map[string]*BatchImageJob
	items         map[string][]CreateBatchImageItemParams
	counts        map[string]BatchImageCounts
	transitions   map[string][]string
	events        map[string][]string
	transitionErr error
	replaceCalls  int
}

func newFakeBatchImageRepository() *fakeBatchImageRepository {
	return &fakeBatchImageRepository{
		jobs:        make(map[string]*BatchImageJob),
		items:       make(map[string][]CreateBatchImageItemParams),
		counts:      make(map[string]BatchImageCounts),
		transitions: make(map[string][]string),
		events:      make(map[string][]string),
	}
}

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
	}
	r.jobs[job.BatchID] = job
	return job, nil
}

func (r *fakeBatchImageRepository) GetBatchImageJobByBatchID(_ context.Context, batchID string) (*BatchImageJob, error) {
	job, ok := r.jobs[batchID]
	if !ok {
		return nil, ErrBatchImageJobNotFound
	}
	return job, nil
}

func (r *fakeBatchImageRepository) GetBatchImageJobByIdempotencyKey(_ context.Context, userID, apiKeyID int64, key string) (*BatchImageJob, error) {
	for _, job := range r.jobs {
		if job.UserID == userID && job.APIKeyID != nil && *job.APIKeyID == apiKeyID && batchImageDerefString(job.IdempotencyKey) == key {
			return job, nil
		}
	}
	return nil, ErrBatchImageJobNotFound
}

func (r *fakeBatchImageRepository) GetBatchImageJobByBatchIDForOwner(_ context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error) {
	job, ok := r.jobs[batchID]
	if !ok || job.UserID != userID || job.APIKeyID == nil || *job.APIKeyID != apiKeyID {
		return nil, ErrBatchImageJobNotFound
	}
	return job, nil
}

func (r *fakeBatchImageRepository) ListBatchImageJobsForOwner(_ context.Context, userID, apiKeyID int64, filter BatchImageJobFilter) ([]*BatchImageJob, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var jobs []*BatchImageJob
	for _, job := range r.jobs {
		if job.UserID != userID || job.APIKeyID == nil || *job.APIKeyID != apiKeyID {
			continue
		}
		if filter.Status != "" && job.Status != filter.Status {
			continue
		}
		if filter.TaskNameLike != "" && !strings.Contains(strings.ToLower(job.TaskName), strings.ToLower(filter.TaskNameLike)) {
			continue
		}
		if filter.ExcludeDeleted && job.UserDeletedAt != nil {
			continue
		}
		if filter.Downloaded != nil {
			downloaded := job.DownloadedAt != nil
			if downloaded != *filter.Downloaded {
				continue
			}
		}
		if filter.CreatedAfter != nil && job.CreatedAt.Before(*filter.CreatedAfter) {
			continue
		}
		if filter.CreatedBefore != nil && !job.CreatedAt.Before(*filter.CreatedBefore) {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
		}
	}
	return jobs, nil
}

func (r *fakeBatchImageRepository) GetBatchImageJobByID(_ context.Context, id int64) (*BatchImageJob, error) {
	for _, job := range r.jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return nil, ErrBatchImageJobNotFound
}

func (r *fakeBatchImageRepository) TransitionBatchImageJobStatus(_ context.Context, batchID, toStatus string, opts BatchImageTransitionOptions) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if !CanTransitionBatchImageJob(job.Status, toStatus) {
		return ErrBatchImageInvalidTransition
	}
	if r.transitionErr != nil {
		return r.transitionErr
	}
	job.Status = toStatus
	job.LastErrorCode = opts.ErrorCode
	job.LastErrorMessage = opts.ErrorMessage
	r.transitions[batchID] = append(r.transitions[batchID], toStatus)
	if opts.EventType != "" {
		r.events[batchID] = append(r.events[batchID], opts.EventType)
	}
	return nil
}

func (r *fakeBatchImageRepository) TouchBatchImageJobSubmitting(_ context.Context, batchID string) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if job.Status == BatchImageJobStatusCreated || job.Status == BatchImageJobStatusUploading {
		job.UpdatedAt = time.Now()
	}
	return nil
}

func (r *fakeBatchImageRepository) FailStaleUnsubmittedBatchImageJob(_ context.Context, batchID string, cutoff time.Time, code, message string) (bool, error) {
	job, ok := r.jobs[batchID]
	if !ok {
		return false, ErrBatchImageJobNotFound
	}
	if job.Status != BatchImageJobStatusCreated && job.Status != BatchImageJobStatusUploading {
		return false, nil
	}
	if batchImageDerefString(job.ProviderJobName) != "" || job.UpdatedAt.After(cutoff) {
		return false, nil
	}
	job.Status = BatchImageJobStatusFailed
	job.LastErrorCode = batchImageStringPtr(code)
	job.LastErrorMessage = batchImageStringPtr(message)
	job.UpdatedAt = time.Now()
	r.transitions[batchID] = append(r.transitions[batchID], BatchImageJobStatusFailed)
	r.events[batchID] = append(r.events[batchID], "billing_hold_recovery_failed_unsubmitted")
	return true, nil
}

func (r *fakeBatchImageRepository) UpdateBatchImageJobProviderOutputRef(_ context.Context, batchID, providerOutputRef string) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	job.ProviderOutputRef = &providerOutputRef
	return nil
}

func (r *fakeBatchImageRepository) UpdateBatchImageJobProviderSubmit(_ context.Context, params UpdateBatchImageJobProviderSubmitParams) error {
	job, ok := r.jobs[params.BatchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if !CanTransitionBatchImageJob(job.Status, BatchImageJobStatusSubmitted) {
		return ErrBatchImageInvalidTransition
	}
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
}

func (r *fakeBatchImageRepository) RecordBatchImageJobSubmitFailure(_ context.Context, batchID, code, message string, markFailed bool) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if markFailed {
		job.Status = BatchImageJobStatusFailed
	}
	job.LastErrorCode = batchImageOptionalStringPtr(code)
	job.LastErrorMessage = batchImageOptionalStringPtr(message)
	eventType := "submit_failed"
	if !markFailed {
		eventType = "queue_failed"
	}
	r.events[batchID] = append(r.events[batchID], eventType)
	return nil
}

func (r *fakeBatchImageRepository) MarkBatchImageJobSettled(_ context.Context, params MarkBatchImageJobSettledParams) error {
	job, ok := r.jobs[params.BatchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if job.Status != BatchImageJobStatusSettling {
		if job.Status == BatchImageJobStatusCompleted {
			return ErrBatchImageAlreadySettled
		}
		return ErrBatchImageSettlementInvalidStatus
	}
	if batchImageDerefString(job.ManifestHash) != "" && batchImageDerefString(job.ManifestHash) != params.ManifestHash {
		return ErrBatchImageSettlementManifestConflict
	}
	now := time.Now()
	job.Status = BatchImageJobStatusCompleted
	job.ActualCost = &params.ActualCost
	job.ManifestHash = &params.ManifestHash
	job.SettledAt = &now
	if job.OutputExpiresAt == nil && params.OutputExpiresAt != nil {
		job.OutputExpiresAt = params.OutputExpiresAt
	}
	r.transitions[params.BatchID] = append(r.transitions[params.BatchID], BatchImageJobStatusCompleted)
	r.events[params.BatchID] = append(r.events[params.BatchID], "settlement_completed")
	return nil
}

func (r *fakeBatchImageRepository) SetBatchImageJobSettlementFailed(_ context.Context, batchID, code, message string) (int, error) {
	job, ok := r.jobs[batchID]
	if !ok {
		return 0, ErrBatchImageJobNotFound
	}
	job.LastErrorCode = batchImageStringPtr(code)
	job.LastErrorMessage = batchImageOptionalStringPtr(message)
	job.RetryCount++
	r.events[batchID] = append(r.events[batchID], "settlement_failed")
	return job.RetryCount, nil
}

func (r *fakeBatchImageRepository) CreateBatchImageItem(_ context.Context, params CreateBatchImageItemParams) (*BatchImageItem, error) {
	r.items[params.JobID] = append(r.items[params.JobID], params)
	return &BatchImageItem{JobID: params.JobID, CustomID: params.CustomID, Status: params.Status}, nil
}

func (r *fakeBatchImageRepository) BulkCreateBatchImageItems(ctx context.Context, params []CreateBatchImageItemParams) error {
	for _, param := range params {
		if _, err := r.CreateBatchImageItem(ctx, param); err != nil {
			return err
		}
	}
	return nil
}

func (r *fakeBatchImageRepository) ReplaceBatchImageItemsForJob(_ context.Context, batchID string, items []CreateBatchImageItemParams, counts BatchImageCounts) error {
	// 与真实实现一致：仅 indexing 状态允许重建 item 表（未注册的 job 保持宽松，
	// 供直接构造 job 的单测使用）。
	if job, ok := r.jobs[batchID]; ok && job.Status != BatchImageJobStatusIndexing {
		return ErrBatchImageIndexStateConflict
	}
	r.replaceCalls++
	copied := append([]CreateBatchImageItemParams(nil), items...)
	for idx := range copied {
		copied[idx].JobID = batchID
	}
	r.items[batchID] = copied
	r.counts[batchID] = counts
	if job, ok := r.jobs[batchID]; ok {
		job.SuccessCount = counts.SuccessCount
		job.FailCount = counts.FailCount
		job.ItemCount = len(copied)
	}
	return nil
}

func (r *fakeBatchImageRepository) ListBatchImageItems(_ context.Context, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	var result []*BatchImageItem
	for _, item := range r.items[batchID] {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if offset > 0 {
			offset--
			continue
		}
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
		})
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (r *fakeBatchImageRepository) ListBatchImageItemsForOwner(ctx context.Context, userID, apiKeyID int64, batchID string, filter BatchImageItemFilter) ([]*BatchImageItem, error) {
	if _, err := r.GetBatchImageJobByBatchIDForOwner(ctx, userID, apiKeyID, batchID); err != nil {
		return nil, err
	}
	return r.ListBatchImageItems(ctx, batchID, filter)
}

func (r *fakeBatchImageRepository) GetBatchImageJobForDownload(ctx context.Context, userID, apiKeyID int64, batchID string) (*BatchImageJob, error) {
	return r.GetBatchImageJobByBatchIDForOwner(ctx, userID, apiKeyID, batchID)
}

func (r *fakeBatchImageRepository) GetBatchImageItemForDownload(_ context.Context, batchID, customID string) (*BatchImageItem, error) {
	for _, item := range r.items[batchID] {
		if item.CustomID != customID {
			continue
		}
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
		}, nil
	}
	return nil, ErrBatchImageItemNotFound
}

func (r *fakeBatchImageRepository) ListBatchImageItemsForDownload(ctx context.Context, batchID string, status string, limit int) ([]*BatchImageItem, error) {
	return r.ListBatchImageItems(ctx, batchID, BatchImageItemFilter{Status: status, Limit: limit})
}

func (r *fakeBatchImageRepository) ListBatchImageJobsDueForInputCleanup(_ context.Context, cutoff time.Time, limit int) ([]*BatchImageJob, error) {
	if limit <= 0 {
		limit = 100
	}
	var jobs []*BatchImageJob
	for _, job := range r.jobs {
		if job.InputDeletedAt != nil || batchImageDerefString(job.ProviderInputRef) == "" || !IsTerminalBatchImageJobStatus(job.Status) {
			continue
		}
		at := job.FinishedAt
		if at == nil {
			at = job.SettledAt
		}
		if at == nil {
			at = &job.UpdatedAt
		}
		if at != nil && at.After(cutoff) {
			continue
		}
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
		}
	}
	return jobs, nil
}

func (r *fakeBatchImageRepository) ListBatchImageJobsDueForOutputCleanup(_ context.Context, now time.Time, limit int) ([]*BatchImageJob, error) {
	if limit <= 0 {
		limit = 100
	}
	var jobs []*BatchImageJob
	for _, job := range r.jobs {
		if job.OutputDeletedAt != nil || batchImageDerefString(job.ProviderOutputRef) == "" || job.Status != BatchImageJobStatusCompleted || job.OutputExpiresAt == nil || job.OutputExpiresAt.After(now) {
			continue
		}
		jobs = append(jobs, job)
		if len(jobs) >= limit {
			break
		}
	}
	return jobs, nil
}

func (r *fakeBatchImageRepository) ListStaleUnsubmittedBatchImageJobs(_ context.Context, cutoff time.Time, limit int) ([]*BatchImageJob, error) {
	if limit <= 0 {
		limit = 100
	}
	jobs := make([]*BatchImageJob, 0, limit)
	for _, job := range r.jobs {
		if len(jobs) >= limit {
			break
		}
		if job.Status != BatchImageJobStatusCreated && job.Status != BatchImageJobStatusUploading {
			continue
		}
		if batchImageDerefString(job.ProviderJobName) != "" {
			continue
		}
		holdAmount := job.EstimatedCost
		if job.HoldAmount != nil {
			holdAmount = *job.HoldAmount
		}
		if holdAmount <= 0 || job.UpdatedAt.After(cutoff) {
			continue
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (r *fakeBatchImageRepository) MarkBatchImageInputDeleted(_ context.Context, batchID string, deletedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if job.InputDeletedAt == nil {
		job.InputDeletedAt = &deletedAt
	}
	r.events[batchID] = append(r.events[batchID], "input_cleanup_completed")
	return nil
}

func (r *fakeBatchImageRepository) MarkBatchImageOutputDeleted(_ context.Context, batchID string, deletedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if job.OutputDeletedAt == nil {
		job.OutputDeletedAt = &deletedAt
	}
	if job.Status == BatchImageJobStatusCompleted {
		job.Status = BatchImageJobStatusOutputDeleted
	}
	r.events[batchID] = append(r.events[batchID], "output_cleanup_completed")
	return nil
}

func (r *fakeBatchImageRepository) MarkBatchImageDownloaded(_ context.Context, batchID string, downloadedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if job.DownloadedAt == nil {
		job.DownloadedAt = &downloadedAt
	}
	r.events[batchID] = append(r.events[batchID], "download_completed")
	return nil
}

func (r *fakeBatchImageRepository) MarkBatchImageJobUserDeleted(_ context.Context, userID, apiKeyID int64, batchID string, deletedAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok || job.UserID != userID || job.APIKeyID == nil || *job.APIKeyID != apiKeyID {
		return ErrBatchImageJobNotFound
	}
	if !isBatchImageProcessorDoneStatus(job.Status) {
		return ErrBatchImageRecordDeleteNotReady
	}
	if job.UserDeletedAt == nil {
		job.UserDeletedAt = &deletedAt
	}
	r.events[batchID] = append(r.events[batchID], "user_record_deleted")
	return nil
}

func (r *fakeBatchImageRepository) SetBatchImageOutputExpiresAt(_ context.Context, batchID string, expiresAt time.Time) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	if job.OutputExpiresAt == nil {
		job.OutputExpiresAt = &expiresAt
	}
	return nil
}

func (r *fakeBatchImageRepository) RecordBatchImageCleanupFailure(_ context.Context, batchID, code, message string) error {
	job, ok := r.jobs[batchID]
	if !ok {
		return ErrBatchImageJobNotFound
	}
	job.LastErrorCode = batchImageStringPtr(code)
	job.LastErrorMessage = batchImageOptionalStringPtr(message)
	r.events[batchID] = append(r.events[batchID], "output_cleanup_failed")
	return nil
}

func (r *fakeBatchImageRepository) AppendBatchImageEvent(_ context.Context, batchID, eventType string, _ any) error {
	r.events[batchID] = append(r.events[batchID], eventType)
	return nil
}

var _ BatchImageRepository = (*fakeBatchImageRepository)(nil)
var _ BatchImageProvider = (*fakeProcessorProvider)(nil)
var _ BatchImageAccountResolver = (*fakeBatchImageAccountResolver)(nil)
var _ = infraerrors.Reason
