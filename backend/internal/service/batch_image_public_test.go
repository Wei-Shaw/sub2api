//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestBatchImagePublicService_Submit(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects when disabled", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(false)
		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageDisabled)
REDACTED)

	t.Run("accepts valid request stores refs and enqueues once", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)

		got, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
	REDACTED
		require.Equal(t, "image.batch", got.Object)
		require.Equal(t, "queued", got.Status)
		require.Equal(t, BatchImageProviderGeminiAPI, got.Provider)
		require.Equal(t, 2, got.ItemCount)
		require.Equal(t, 0.25, got.EstimatedCost)
		require.Len(t, repo.jobs, 1)
		require.Len(t, gemini.submits, 1)
		require.Equal(t, []string{got.IDREDACTED, queue.enqueued)
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)
		require.Len(t, billing.reserves, 1)
		require.Equal(t, BatchImageHoldRequestID(got.ID), billing.reserves[0].RequestID)
		require.InDelta(t, 0.3, billing.reserves[0].HoldAmount, 1e-12)
		require.Empty(t, billing.releases)
		authCache := svc.AuthCache.(*fakeBatchImageAuthCacheInvalidator)
		require.Equal(t, []int64{11REDACTED, authCache.userIDs)

		job := repo.jobs[got.ID]
		require.Equal(t, BatchImageJobStatusSubmitted, job.Status)
		require.Equal(t, "providers/gemini_api/job", batchImageDerefString(job.ProviderJobName))
		require.Equal(t, "files/gemini_api/input", batchImageDerefString(job.ProviderInputRef))
		require.Equal(t, "files/gemini_api/output", batchImageDerefString(job.ProviderOutputRef))
		require.NotNil(t, job.AccountID)
		require.Equal(t, int64(202), *job.AccountID)
		require.Equal(t, 1, job.PricingSnapshotVersion)
		require.InDelta(t, 0.25, job.BaseUnitPrice, 1e-12)
		require.InDelta(t, 1.0, job.GroupRateMultiplier, 1e-12)
		require.InDelta(t, 1.0, job.AccountRateMultiplier, 1e-12)
		require.InDelta(t, 0.5, job.BatchDiscountMultiplier, 1e-12)
		require.InDelta(t, 0.6, job.HoldMultiplier, 1e-12)
		require.InDelta(t, 0.125, job.BillableUnitPrice, 1e-12)
		require.InDelta(t, 0.15, job.HoldUnitPrice, 1e-12)
REDACTED)

	t.Run("combines user group image rate account rate discount and hold margin", func(t *testing.T) {
		svc, repo, _, _, _ := newTestBatchImagePublicService(true)
		groupID := int64(7)
		accountMultiplier := 1.25
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts[1].RateMultiplier = &accountMultiplier
		svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{
			groupID: {
				ID:                           groupID,
				Platform:                     PlatformGemini,
				RateMultiplier:               2.0,
				AllowImageGeneration:         true,
				AllowBatchImageGeneration:    true,
				ImageRateIndependent:         false,
				BatchImageDiscountMultiplier: 0.8,
				BatchImageHoldMultiplier:     0.6,
		REDACTED,
	REDACTEDREDACTED
		userRate := 0.5
		svc.UserGroupRateRepo = &publicBatchImageUserGroupRateRepo{rates: map[int64]*float64{groupID: &userRateREDACTEDREDACTED

		got, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupIDREDACTED, validBatchImageSubmitRequest(), "")
	REDACTED
		require.InDelta(t, 0.25, got.EstimatedCost, 1e-12)

		job := repo.jobs[got.ID]
		require.InDelta(t, 0.25, job.BaseUnitPrice, 1e-12)
		require.InDelta(t, 0.5, job.GroupRateMultiplier, 1e-12)
		require.InDelta(t, 1.25, job.AccountRateMultiplier, 1e-12)
		require.InDelta(t, 0.8, job.BatchDiscountMultiplier, 1e-12)
		require.InDelta(t, 0.6, job.HoldMultiplier, 1e-12)
		require.InDelta(t, 0.125, job.BillableUnitPrice, 1e-12)
		require.InDelta(t, 0.09375, job.HoldUnitPrice, 1e-12)
		require.InDelta(t, 0.1875, *job.HoldAmount, 1e-12)
REDACTED)

	t.Run("uses configured group 1k image price for batch image base price", func(t *testing.T) {
		svc, repo, _, _, _ := newTestBatchImagePublicService(true)
		groupID := int64(7)
		imagePrice := 0.134
		svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{
			groupID: {
				ID:                           groupID,
				Platform:                     PlatformGemini,
				RateMultiplier:               1.0,
				AllowImageGeneration:         true,
				AllowBatchImageGeneration:    true,
				ImagePrice1K:                 &imagePrice,
				BatchImageDiscountMultiplier: 0.5,
				BatchImageHoldMultiplier:     0.6,
		REDACTED,
	REDACTEDREDACTED

		got, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupIDREDACTED, validBatchImageSubmitRequest(), "")
	REDACTED
		require.InDelta(t, 0.134, got.EstimatedCost, 1e-12)

		job := repo.jobs[got.ID]
		require.InDelta(t, 0.134, job.BaseUnitPrice, 1e-12)
		require.InDelta(t, 0.067, job.BillableUnitPrice, 1e-12)
		require.InDelta(t, 0.0804, job.HoldUnitPrice, 1e-12)
		require.InDelta(t, 0.1608, *job.HoldAmount, 1e-12)
REDACTED)

	t.Run("pricing missing rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		svc.Pricing = &fakeBatchImagePricingResolver{err: ErrBatchImageSettlementPricingMissingREDACTED

		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageSettlementPricingMissing)
		require.Empty(t, repo.jobs)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
REDACTED)

	t.Run("group batch image disabled rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		groupID := int64(7)
		svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{
			groupID: {
				ID:                           groupID,
				Platform:                     PlatformGemini,
				RateMultiplier:               1,
				AllowBatchImageGeneration:    false,
				BatchImageDiscountMultiplier: 0.5,
				BatchImageHoldMultiplier:     0.6,
		REDACTED,
	REDACTEDREDACTED

		_, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupIDREDACTED, validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageGroupDisabled)
		require.Empty(t, repo.jobs)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
REDACTED)

	t.Run("group pricing load failure rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		groupID := int64(404)

		_, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupIDREDACTED, validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageSettlementPricingMissing)
		require.Empty(t, repo.jobs)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
REDACTED)

	t.Run("generates custom ids deterministically", func(t *testing.T) {
		svc, _, _, gemini, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Items[0].CustomID = ""
		req.Items[1].CustomID = ""

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
	REDACTED
		require.Len(t, gemini.submits, 1)
		require.Equal(t, "item_000001", gemini.submits[0].Items[0].CustomID)
		require.Equal(t, "item_000002", gemini.submits[0].Items[1].CustomID)
REDACTED)

	t.Run("expands output count into separate billable items", func(t *testing.T) {
		svc, repo, _, gemini, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Items = []BatchImageSubmitItem{
			{CustomID: "cover", Prompt: "hero", OutputCount: 3, ReferenceImages: []BatchImageReferenceInput{{MimeType: "image/png", Data: []byte("ref")REDACTEDREDACTEDREDACTED,
	REDACTED

		got, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
	REDACTED
		require.Equal(t, 3, got.ItemCount)
		require.InDelta(t, 0.375, got.EstimatedCost, 1e-12)
		require.Len(t, gemini.submits, 1)
		require.Len(t, gemini.submits[0].Items, 3)
		require.Equal(t, []string{"cover_01", "cover_02", "cover_03"REDACTED, []string{
			gemini.submits[0].Items[0].CustomID,
			gemini.submits[0].Items[1].CustomID,
			gemini.submits[0].Items[2].CustomID,
	REDACTED)
		require.Len(t, gemini.submits[0].Items[0].ReferenceImages, 1)
		require.Len(t, repo.items[got.ID], 3)
REDACTED)

	t.Run("validates request fields", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*BatchImageSubmitRequest)
			want   error
	REDACTED{
			{name: "missing_model", mutate: func(r *BatchImageSubmitRequest) { r.Model = "" REDACTED, want: ErrBatchImageInvalidModelREDACTED,
			{name: "empty_items", mutate: func(r *BatchImageSubmitRequest) { r.Items = nil REDACTED, want: ErrBatchImageInvalidItemsREDACTED,
			{name: "duplicate_custom_ids", mutate: func(r *BatchImageSubmitRequest) { r.Items[1].CustomID = r.Items[0].CustomID REDACTED, want: ErrBatchImageDuplicateCustomIDInRequestREDACTED,
			{name: "empty_prompt", mutate: func(r *BatchImageSubmitRequest) { r.Items[0].Prompt = " " REDACTED, want: ErrBatchImageInvalidItemsREDACTED,
			{name: "prompt_too_long", mutate: func(r *BatchImageSubmitRequest) { r.Items[0].Prompt = strings.Repeat("x", 9) REDACTED, want: ErrBatchImagePromptTooLongREDACTED,
			{name: "unsupported_provider", mutate: func(r *BatchImageSubmitRequest) { r.Provider = "other" REDACTED, want: ErrBatchImageUnsupportedProviderREDACTED,
			{name: "vertex_rejects_2k", mutate: func(r *BatchImageSubmitRequest) { r.Provider = BatchImageProviderVertex; r.ImageSize = "2K" REDACTED, want: ErrBatchImageInvalidItemsREDACTED,
			{name: "too_many_outputs_per_item", mutate: func(r *BatchImageSubmitRequest) {
				r.Items[0].OutputCount = 5
		REDACTED, want: ErrBatchImageInvalidItemsREDACTED,
			{name: "too_many_reference_images_for_flash", mutate: func(r *BatchImageSubmitRequest) {
				r.Model = "gemini-2.5-flash-image"
				r.Items[0].ReferenceImages = []BatchImageReferenceInput{
					{MimeType: "image/png", Data: []byte("1")REDACTED,
					{MimeType: "image/png", Data: []byte("2")REDACTED,
					{MimeType: "image/png", Data: []byte("3")REDACTED,
					{MimeType: "image/png", Data: []byte("4")REDACTED,
			REDACTED
		REDACTED, want: ErrBatchImageTooManyReferenceImagesREDACTED,
			{name: "bad_reference_mime", mutate: func(r *BatchImageSubmitRequest) {
				r.Items[0].ReferenceImages = []BatchImageReferenceInput{{MimeType: "application/octet-stream", Data: []byte("x")REDACTEDREDACTED
		REDACTED, want: ErrBatchImageInvalidReferenceImageREDACTED,
			{name: "reference_requires_data_or_file_uri", mutate: func(r *BatchImageSubmitRequest) {
				r.Items[0].ReferenceImages = []BatchImageReferenceInput{{MimeType: "image/png"REDACTEDREDACTED
		REDACTED, want: ErrBatchImageInvalidReferenceImageREDACTED,
	REDACTED
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, _, _, _, _ := newTestBatchImagePublicService(true)
				req := validBatchImageSubmitRequest()
				tt.mutate(&req)

				_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
				require.ErrorIs(t, err, tt.want)
		REDACTED)
	REDACTED
REDACTED)

	t.Run("rejects too many items", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Items = append(req.Items, BatchImageSubmitItem{CustomID: "too_many", Prompt: "x"REDACTED)

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageInvalidItems)
REDACTED)

	t.Run("rejects too many output images", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Config.BatchImage.MaxOutputImagesPerJob = 3
		req := validBatchImageSubmitRequest()
		req.Items[0].OutputCount = 2
		req.Items[1].OutputCount = 2

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageTooManyOutputImages)
REDACTED)

	t.Run("rejects too many reference images across request", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Config.BatchImage.MaxReferenceImagesPerJob = 3
		req := validBatchImageSubmitRequest()
		req.Model = "gemini-2.5-flash-image"
		req.Items[0].ReferenceImages = []BatchImageReferenceInput{
			{MimeType: "image/png", Data: []byte("1")REDACTED,
			{MimeType: "image/png", Data: []byte("2")REDACTED,
	REDACTED
		req.Items[1].ReferenceImages = []BatchImageReferenceInput{
			{MimeType: "image/png", Data: []byte("3")REDACTED,
			{MimeType: "image/png", Data: []byte("4")REDACTED,
	REDACTED

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageTooManyReferenceImages)
REDACTED)

	t.Run("rejects too much inline reference image data across request", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Config.BatchImage.MaxReferenceImagesPerJob = 10
		svc.Config.BatchImage.MaxReferenceInlineBytesPerJob = 4
		req := validBatchImageSubmitRequest()
		req.Model = "gemini-2.5-flash-image"
		req.Items[0].ReferenceImages = []BatchImageReferenceInput{{MimeType: "image/png", Data: []byte("123")REDACTEDREDACTED
		req.Items[1].ReferenceImages = []BatchImageReferenceInput{{MimeType: "image/png", Data: []byte("456")REDACTEDREDACTED

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageReferenceImagesTooLarge)
REDACTED)

	t.Run("selects requested provider", func(t *testing.T) {
		svc, _, _, gemini, vertex := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Provider = BatchImageProviderVertex

		got, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
	REDACTED
		require.Equal(t, BatchImageProviderVertex, got.Provider)
		require.Empty(t, gemini.submits)
		require.Len(t, vertex.submits, 1)
REDACTED)

	t.Run("insufficient balance rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		billing := &fakeBatchImageBillingRepo{err: ErrBatchImageInsufficientBalanceREDACTED
		svc.BillingRepo = billing

		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageInsufficientBalance)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
		require.Len(t, billing.reserves, 1)
		require.Empty(t, billing.releases)
		require.Len(t, repo.jobs, 1)
		for _, job := range repo.jobs {
			require.Equal(t, BatchImageJobStatusFailed, job.Status)
			require.Equal(t, "INSUFFICIENT_BALANCE", batchImageDerefString(job.LastErrorCode))
			require.NotNil(t, job.UserDeletedAt)
	REDACTED
REDACTED)

	t.Run("provider failure marks failed and does not enqueue", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		gemini.submitErr = errors.New("projects/secret-provider-job failed")
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)

		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageProviderSubmitFailed)
		require.Empty(t, queue.enqueued)
		require.Len(t, billing.reserves, 1)
		require.Len(t, billing.releases, 1)
		require.Equal(t, BatchImageReleaseRequestID(billing.reserves[0].BatchID), billing.releases[0].RequestID)
		require.Len(t, repo.jobs, 1)
		for _, job := range repo.jobs {
			require.Equal(t, BatchImageJobStatusFailed, job.Status)
			require.Equal(t, "PROVIDER_SUBMIT_FAILED", batchImageDerefString(job.LastErrorCode))
			require.Equal(t, "upstream provider operation failed", batchImageDerefString(job.LastErrorMessage))
			require.NotNil(t, job.UserDeletedAt)
	REDACTED
REDACTED)

	t.Run("provider failure with release failure enqueues billing retry", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		gemini.submitErr = errors.New("projects/secret-provider-job failed")
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)
		billing.releaseErr = errors.New("billing database timeout")

		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageBillingHoldFailed)
		require.Len(t, billing.reserves, 1)
		require.Len(t, billing.releases, 1)
		require.Len(t, repo.jobs, 1)
		for _, job := range repo.jobs {
			require.Equal(t, BatchImageJobStatusFailed, job.Status)
			require.Equal(t, "BILLING_RELEASE_FAILED", batchImageDerefString(job.LastErrorCode))
			require.Equal(t, []string{job.BatchIDREDACTED, queue.enqueued)
	REDACTED
REDACTED)

	t.Run("queue failure is recorded after provider submit", func(t *testing.T) {
		svc, repo, queue, _, _ := newTestBatchImagePublicService(true)
		queue.err = errors.New("redis unavailable")
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)

		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageQueueFailed)
		require.Len(t, billing.reserves, 1)
		require.Empty(t, billing.releases)
		require.Len(t, repo.jobs, 1)
		for _, job := range repo.jobs {
			require.Equal(t, BatchImageJobStatusSubmitted, job.Status)
			require.Equal(t, "QUEUE_FAILED", batchImageDerefString(job.LastErrorCode))
			require.Contains(t, repo.events[job.BatchID], "queue_failed")
	REDACTED
REDACTED)

	t.Run("idempotency returns same batch without provider resubmit", func(t *testing.T) {
		svc, _, queue, gemini, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()

		first, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
	REDACTED
		second, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
	REDACTED

		require.Equal(t, first.ID, second.ID)
		require.Len(t, gemini.submits, 1)
		require.Equal(t, []string{first.IDREDACTED, queue.enqueued)
REDACTED)

	t.Run("idempotency conflict rejects changed request", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		first, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
	REDACTED

		req.Items[0].Prompt = "diff"
		second, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
		require.Nil(t, second)
		require.ErrorIs(t, err, ErrBatchImageIdempotencyConflict)
		require.NotEmpty(t, first.ID)
REDACTED)

	t.Run("public response does not expose internals", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		got, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
	REDACTED

		body, err := json.Marshal(got)
	REDACTED
		requireBatchImagePublicJSONHasNoInternals(t, string(body))
REDACTED)
REDACTED

func TestBatchImagePublicService_List(t *testing.T) {
	ctx := context.Background()
	svc, repo, _, _, _ := newTestBatchImagePublicService(true)
	visibleKeyID := int64(22)
	otherKeyID := int64(23)

	repo.jobs["visible-1"] = &BatchImageJob{
		BatchID:   "visible-1",
		UserID:    11,
		APIKeyID:  &visibleKeyID,
		Status:    BatchImageJobStatusCompleted,
		Provider:  BatchImageProviderVertex,
		Model:     "gemini-3.1-flash-lite-image",
		ItemCount: 1,
		CreatedAt: time.Now(),
REDACTED
	repo.jobs["hidden-other-key"] = &BatchImageJob{
		BatchID:   "hidden-other-key",
		UserID:    11,
		APIKeyID:  &otherKeyID,
		Status:    BatchImageJobStatusCompleted,
		Provider:  BatchImageProviderVertex,
		Model:     "gemini-3.1-flash-lite-image",
		ItemCount: 1,
		CreatedAt: time.Now(),
REDACTED

	got, err := svc.List(ctx, BatchImageOwner{UserID: 11, APIKeyID: visibleKeyIDREDACTED, BatchImageJobsQuery{Limit: 20REDACTED)
REDACTED
	require.Equal(t, "list", got.Object)
	require.Len(t, got.Data, 1)
	require.Equal(t, "visible-1", got.Data[0].ID)
	require.False(t, got.HasMore)
REDACTED

func TestBatchImagePublicService_ListModels(t *testing.T) {
	ctx := context.Background()

	t.Run("requires explicit account model mapping", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)

		got, err := svc.ListModels(ctx, testBatchImageOwner())
	REDACTED
		require.Equal(t, "list", got.Object)
		require.Empty(t, got.Data)
REDACTED)

	t.Run("returns priced models from selected account group", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		groupID := int64(7)
		svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{
			groupID: {
				ID:                           groupID,
				Platform:                     PlatformGemini,
				RateMultiplier:               1,
				AllowImageGeneration:         true,
				AllowBatchImageGeneration:    true,
				BatchImageDiscountMultiplier: 0.5,
				BatchImageHoldMultiplier:     0.6,
		REDACTED,
	REDACTEDREDACTED
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts = []Account{testBatchImageMappedAccount(303, AccountTypeAPIKey, map[string]any{
			"gemini-2.5-flash-image": "gemini-2.5-flash-image",
	REDACTED)REDACTED

		got, err := svc.ListModels(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupIDREDACTED)
	REDACTED
		require.Equal(t, []BatchImagePublicModel{{
			ID:       "gemini-2.5-flash-image",
			Object:   "image.batch.model",
			Provider: BatchImageProviderGeminiAPI,
	REDACTED, {
			ID:       "gemini-2.5-flash-image",
			Object:   "image.batch.model",
			Provider: BatchImageProviderVertex,
REDACTED got.Data)
REDACTED)

	t.Run("expands wildcard mappings against batch image candidates", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts = []Account{testBatchImageMappedAccount(303, AccountTypeAPIKey, map[string]any{
			"gemini-3.1-*": "gemini-3.1-flash-lite-image",
	REDACTED)REDACTED

		got, err := svc.ListModels(ctx, testBatchImageOwner())
	REDACTED
		require.NotEmpty(t, got.Data)
		ids := make([]string, 0, len(got.Data))
		for _, model := range got.Data {
			ids = append(ids, model.ID)
	REDACTED
		require.Contains(t, ids, "gemini-3.1-flash-image")
		require.Contains(t, ids, "gemini-3.1-flash-lite-image")
		require.NotContains(t, ids, "gemini-2.5-flash-image")
REDACTED)

	t.Run("filters models without batch image pricing", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Pricing = &fakeBatchImagePricingResolver{
			unitPrice:     0.25,
			missingModels: map[string]bool{"gemini-3.1-flash-lite-image": trueREDACTED,
	REDACTED
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts = []Account{testBatchImageMappedAccount(303, AccountTypeAPIKey, map[string]any{
			"gemini-2.5-flash-image":      "gemini-2.5-flash-image",
			"gemini-3.1-flash-lite-image": "gemini-3.1-flash-lite-image",
	REDACTED)REDACTED

		got, err := svc.ListModels(ctx, testBatchImageOwner())
	REDACTED
		ids := make([]string, 0, len(got.Data))
		for _, model := range got.Data {
			ids = append(ids, model.ID)
	REDACTED
		require.Contains(t, ids, "gemini-2.5-flash-image")
		require.NotContains(t, ids, "gemini-3.1-flash-lite-image")
REDACTED)

	t.Run("rejects when group disables batch image", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		groupID := int64(7)
		svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{
			groupID: {ID: groupID, AllowBatchImageGeneration: falseREDACTED,
	REDACTEDREDACTED

		_, err := svc.ListModels(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupIDREDACTED)
		require.ErrorIs(t, err, ErrBatchImageGroupDisabled)
REDACTED)
REDACTED

func TestBatchImagePublicService_StatusItemsAndCancel(t *testing.T) {
	ctx := context.Background()

	t.Run("status is owner scoped and maps public status", func(t *testing.T) {
		svc, repo, _, _, _ := newTestBatchImagePublicService(true)
		apiKeyID := int64(22)
		accountID := int64(101)
		repo.jobs["imgbatch_status"] = &BatchImageJob{
			BatchID:         "imgbatch_status",
			UserID:          11,
			APIKeyID:        &apiKeyID,
			AccountID:       &accountID,
			Provider:        BatchImageProviderGeminiAPI,
			Model:           "gemini-2.5-flash-image",
			Status:          BatchImageJobStatusIndexing,
			ProviderJobName: batchImageStringPtr("providers/internal/job"),
			CreatedAt:       time.Now(),
	REDACTED

		got, err := svc.Get(ctx, testBatchImageOwner(), "imgbatch_status")
	REDACTED
		require.Equal(t, "processing_results", got.Status)
		body, err := json.Marshal(got)
	REDACTED
		requireBatchImagePublicJSONHasNoInternals(t, string(body))

		_, err = svc.Get(ctx, BatchImageOwner{UserID: 11, APIKeyID: 999REDACTED, "imgbatch_status")
		require.ErrorIs(t, err, ErrBatchImageJobNotFound)
REDACTED)

	t.Run("items are filtered paginated and sanitized", func(t *testing.T) {
		svc, repo, _, _, _ := newTestBatchImagePublicService(true)
		apiKeyID := int64(22)
		repo.jobs["imgbatch_items"] = &BatchImageJob{
			BatchID:   "imgbatch_items",
			UserID:    11,
			APIKeyID:  &apiKeyID,
			Provider:  BatchImageProviderGeminiAPI,
			Model:     "gemini-2.5-flash-image",
			Status:    BatchImageJobStatusCompleted,
			CreatedAt: time.Now(),
	REDACTED
		sourceObject := "gs://bucket/internal/output.jsonl"
		mime := "image/png"
		ext := "png"
		code := "SAFETY_BLOCKED"
		msg := "blocked in gs://bucket/internal/output.jsonl"
		repo.items["imgbatch_items"] = []CreateBatchImageItemParams{
			{JobID: "imgbatch_items", CustomID: "ok_1", Status: BatchImageItemStatusSuccess, ProviderSourceObject: &sourceObject, MimeType: &mime, FileExtension: &ext, ImageCount: 1REDACTED,
			{JobID: "imgbatch_items", CustomID: "bad_1", Status: BatchImageItemStatusFailed, ProviderSourceObject: &sourceObject, ErrorCode: &code, ErrorMessage: &msgREDACTED,
			{JobID: "imgbatch_items", CustomID: "ok_2", Status: BatchImageItemStatusSuccess, MimeType: &mime, FileExtension: &ext, ImageCount: 1REDACTED,
	REDACTED

		page, err := svc.ListItems(ctx, testBatchImageOwner(), "imgbatch_items", BatchImageItemsQuery{Limit: 1REDACTED)
	REDACTED
		require.True(t, page.HasMore)
		require.Len(t, page.Data, 1)
		require.Equal(t, "ok_1", page.Data[0].CustomID)

		filtered, err := svc.ListItems(ctx, testBatchImageOwner(), "imgbatch_items", BatchImageItemsQuery{Status: "failed", Limit: 100REDACTED)
	REDACTED
		require.False(t, filtered.HasMore)
		require.Len(t, filtered.Data, 1)
		require.Equal(t, "failed", filtered.Data[0].Status)
		require.NotNil(t, filtered.Data[0].Error)
		require.Equal(t, "upstream provider operation failed", filtered.Data[0].Error.Message)

		body, err := json.Marshal(filtered)
	REDACTED
		requireBatchImagePublicJSONHasNoInternals(t, string(body))
		require.NotContains(t, string(body), "download_url")

		_, err = svc.ListItems(ctx, BatchImageOwner{UserID: 12, APIKeyID: 22REDACTED, "imgbatch_items", BatchImageItemsQuery{REDACTED)
		require.ErrorIs(t, err, ErrBatchImageJobNotFound)
REDACTED)

	t.Run("cancel active job calls provider and waits for confirmed terminal state", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		apiKeyID := int64(22)
		accountID := int64(101)
		holdAmount := 0.5
		holdID := BatchImageHoldRequestID("imgbatch_cancel")
		repo.jobs["imgbatch_cancel"] = &BatchImageJob{
			BatchID:         "imgbatch_cancel",
			UserID:          11,
			APIKeyID:        &apiKeyID,
			AccountID:       &accountID,
			Provider:        BatchImageProviderGeminiAPI,
			Model:           "gemini-2.5-flash-image",
			Status:          BatchImageJobStatusSubmitted,
			ProviderJobName: batchImageStringPtr("providers/internal/job"),
			EstimatedCost:   holdAmount,
			HoldAmount:      &holdAmount,
			HoldID:          &holdID,
			CreatedAt:       time.Now(),
	REDACTED

		got, err := svc.Cancel(ctx, testBatchImageOwner(), "imgbatch_cancel")
	REDACTED
		require.Equal(t, "queued", got.Status)
		require.Equal(t, 1, gemini.cancelCount)
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)
		require.Empty(t, billing.releases)
		require.Equal(t, []string{"imgbatch_cancel"REDACTED, queue.enqueued)
		require.Equal(t, BatchImageJobStatusSubmitted, repo.jobs["imgbatch_cancel"].Status)
		require.Contains(t, repo.events["imgbatch_cancel"], "job_cancel_requested")
REDACTED)

	t.Run("cancel terminal job is idempotent", func(t *testing.T) {
		svc, repo, _, gemini, _ := newTestBatchImagePublicService(true)
		apiKeyID := int64(22)
		repo.jobs["imgbatch_done"] = &BatchImageJob{
			BatchID:   "imgbatch_done",
			UserID:    11,
			APIKeyID:  &apiKeyID,
			Provider:  BatchImageProviderGeminiAPI,
			Model:     "gemini-2.5-flash-image",
			Status:    BatchImageJobStatusCompleted,
			CreatedAt: time.Now(),
	REDACTED

		got, err := svc.Cancel(ctx, testBatchImageOwner(), "imgbatch_done")
	REDACTED
		require.Equal(t, "completed", got.Status)
		require.Zero(t, gemini.cancelCount)
REDACTED)

	t.Run("cancel hides provider raw errors behind public error", func(t *testing.T) {
		svc, repo, _, gemini, _ := newTestBatchImagePublicService(true)
		gemini.cancelErr = errors.New("projects/secret-provider-job not found")
		apiKeyID := int64(22)
		accountID := int64(101)
		repo.jobs["imgbatch_cancel_error"] = &BatchImageJob{
			BatchID:         "imgbatch_cancel_error",
			UserID:          11,
			APIKeyID:        &apiKeyID,
			AccountID:       &accountID,
			Provider:        BatchImageProviderGeminiAPI,
			Model:           "gemini-2.5-flash-image",
			Status:          BatchImageJobStatusSubmitted,
			ProviderJobName: batchImageStringPtr("providers/internal/job"),
			CreatedAt:       time.Now(),
	REDACTED

		_, err := svc.Cancel(ctx, testBatchImageOwner(), "imgbatch_cancel_error")
		require.ErrorIs(t, err, ErrBatchImageCancelFailed)
		require.Equal(t, "BATCH_IMAGE_CANCEL_FAILED", infraerrors.Reason(err))
		require.NotContains(t, infraerrors.Message(err), "projects/")
REDACTED)
REDACTED

func newTestBatchImagePublicService(enabled bool) (*BatchImagePublicService, *fakeBatchImageRepository, *publicBatchImageQueue, *publicBatchImageProvider, *publicBatchImageProvider) {
	repo := newFakeBatchImageRepository()
	queue := &publicBatchImageQueue{REDACTED
	gemini := &publicBatchImageProvider{name: BatchImageProviderGeminiAPIREDACTED
	vertex := &publicBatchImageProvider{name: BatchImageProviderVertexREDACTED
	svc := &BatchImagePublicService{
		Repo:        repo,
		AccountRepo: &publicBatchImageAccountRepo{accounts: []Account{testBatchImageAccount(101, AccountTypeAPIKey), testBatchImageAccount(202, AccountTypeServiceAccount)REDACTEDREDACTED,
		Queue:       queue,
		ProviderRegistry: NewBatchImageProviderRegistry(
			gemini,
			vertex,
		),
		Pricing:     &fakeBatchImagePricingResolver{unitPrice: 0.25REDACTED,
		BillingRepo: &fakeBatchImageBillingRepo{REDACTED,
		AuthCache:   &fakeBatchImageAuthCacheInvalidator{REDACTED,
		Config: &config.Config{BatchImage: config.BatchImageConfig{
			Enabled:                 enabled,
			MaxItemsPerJobDefault:   2,
			MaxPromptCharsPerItem:   8,
			DefaultResponseMimeType: "image/png",
			DefaultImageSize:        "1K",
REDACTED
REDACTED
	return svc, repo, queue, gemini, vertex
REDACTED

func testBatchImageOwner() BatchImageOwner {
	return BatchImageOwner{UserID: 11, APIKeyID: 22REDACTED
REDACTED

type fakeBatchImageAuthCacheInvalidator struct {
	keys     []string
	userIDs  []int64
	groupIDs []int64
REDACTED

func (f *fakeBatchImageAuthCacheInvalidator) InvalidateAuthCacheByKey(_ context.Context, key string) {
	f.keys = append(f.keys, key)
REDACTED

func (f *fakeBatchImageAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	f.userIDs = append(f.userIDs, userID)
REDACTED

func (f *fakeBatchImageAuthCacheInvalidator) InvalidateAuthCacheByGroupID(_ context.Context, groupID int64) {
	f.groupIDs = append(f.groupIDs, groupID)
REDACTED

func validBatchImageSubmitRequest() BatchImageSubmitRequest {
	return BatchImageSubmitRequest{
		Model:            "gemini-2.5-flash-image",
		Provider:         BatchImageProviderGeminiAPI,
		ResponseMimeType: "image/png",
		AspectRatio:      "1:1",
		ImageSize:        "1K",
		Metadata:         map[string]string{"project": "campaign-a", "secret": strings.Repeat("x", 300)REDACTED,
		Items: []BatchImageSubmitItem{
			{CustomID: "cover_001", Prompt: "hero"REDACTED,
			{CustomID: "cover_002", Prompt: "clean"REDACTED,
	REDACTED,
REDACTED
REDACTED

func testBatchImageAccount(id int64, accountType string) Account {
	return Account{
		ID:            id,
		Platform:      PlatformGemini,
		Type:          accountType,
		Status:        StatusActive,
		Schedulable:   true,
		Priority:      int(id),
		Credentials:   map[string]any{"api_key": "test-secret"REDACTED,
		Concurrency:   1,
		RateLimitedAt: nil,
REDACTED
REDACTED

func testBatchImageMappedAccount(id int64, accountType string, mapping map[string]any) Account {
	account := testBatchImageAccount(id, accountType)
	account.Credentials["model_mapping"] = mapping
	return account
REDACTED

func requireBatchImagePublicJSONHasNoInternals(t *testing.T, body string) {
REDACTED
	for _, forbidden := range []string{
		"provider_job_name",
		"provider_input_ref",
		"provider_output_ref",
		"gcs_input_uri",
		"gcs_output_uri",
		"account_id",
		"service_account",
		"api_key",
		"download_url",
		"providers/",
		"files/",
		"gs://",
REDACTED {
		require.NotContains(t, body, forbidden)
REDACTED
REDACTED

type publicBatchImageAccountRepo struct {
	accounts []Account
REDACTED

func (r *publicBatchImageAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
	REDACTED
REDACTED
	return nil, errors.New("account not found")
REDACTED

func (r *publicBatchImageAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			out = append(out, account)
	REDACTED
REDACTED
	return out, nil
REDACTED

func (r *publicBatchImageAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
REDACTED

type publicBatchImageQueue struct {
	enqueued []string
	err      error
REDACTED

func (q *publicBatchImageQueue) Enqueue(_ context.Context, batchID string) error {
	if q.err != nil {
		return q.err
REDACTED
	for _, existing := range q.enqueued {
		if existing == batchID {
			return ErrBatchImageAlreadyQueued
	REDACTED
REDACTED
	q.enqueued = append(q.enqueued, batchID)
	return nil
REDACTED

func (q *publicBatchImageQueue) Reserve(context.Context, time.Duration) (ReservedBatchImageJob, error) {
	return ReservedBatchImageJob{REDACTED, ErrBatchImageQueueEmpty
REDACTED

func (q *publicBatchImageQueue) RequeueAfter(context.Context, string, time.Duration) error {
	return nil
REDACTED

func (q *publicBatchImageQueue) Ack(context.Context, string) error {
	return nil
REDACTED

func (q *publicBatchImageQueue) Heartbeat(context.Context, string) error {
	return nil
REDACTED

func (q *publicBatchImageQueue) MoveDueDelayedToReady(context.Context, int) (int, error) {
	return 0, nil
REDACTED

func (q *publicBatchImageQueue) RecoverStaleActive(context.Context, time.Duration, int) (int, error) {
	return 0, nil
REDACTED

func (q *publicBatchImageQueue) TryAcquireJobLock(context.Context, string, time.Duration) (BatchImageJobLock, bool, error) {
	return nil, false, nil
REDACTED

type publicBatchImageProvider struct {
	name           string
	submits        []BatchImageInput
	submitErr      error
	cancelCount    int
	cancelErr      error
	result         string
	cleanupTargets []CleanupTarget
	cleanupErr     error
REDACTED

func (p *publicBatchImageProvider) Name() string { return p.name REDACTED

func (p *publicBatchImageProvider) SupportsAccount(*Account) bool { return true REDACTED

func (p *publicBatchImageProvider) Submit(_ context.Context, _ *BatchImageJob, _ *Account, input BatchImageInput) (*BatchProviderJob, error) {
	p.submits = append(p.submits, input)
	if p.submitErr != nil {
		return nil, p.submitErr
REDACTED
	return &BatchProviderJob{
		ProviderJobName:   "providers/" + p.name + "/job",
		ProviderInputRef:  "files/" + p.name + "/input",
		ProviderOutputRef: "files/" + p.name + "/output",
REDACTED, nil
REDACTED

func (p *publicBatchImageProvider) Get(context.Context, *BatchImageJob, *Account) (*BatchProviderStatus, error) {
	return &BatchProviderStatus{InternalState: BatchProviderStateQueuedREDACTED, nil
REDACTED

func (p *publicBatchImageProvider) Cancel(context.Context, *BatchImageJob, *Account) error {
	p.cancelCount++
	return p.cancelErr
REDACTED

func (p *publicBatchImageProvider) OpenResult(context.Context, *BatchImageJob, *Account) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(p.result)), "application/jsonl", nil
REDACTED

func (p *publicBatchImageProvider) Cleanup(_ context.Context, _ *BatchImageJob, _ *Account, target CleanupTarget) error {
	p.cleanupTargets = append(p.cleanupTargets, target)
	return p.cleanupErr
REDACTED

var _ BatchImageAccountSelectionRepository = (*publicBatchImageAccountRepo)(nil)
var _ BatchImageQueue = (*publicBatchImageQueue)(nil)
var _ BatchImageProvider = (*publicBatchImageProvider)(nil)

type publicBatchImageGroupRepo struct {
	groups map[int64]*Group
REDACTED

func (r *publicBatchImageGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if r != nil && r.groups != nil {
		if group, ok := r.groups[id]; ok {
			return group, nil
	REDACTED
REDACTED
	return nil, ErrGroupNotFound
REDACTED

type publicBatchImageUserGroupRateRepo struct {
	rates map[int64]*float64
REDACTED

func (r *publicBatchImageUserGroupRateRepo) GetByUserAndGroup(_ context.Context, _ int64, groupID int64) (*float64, error) {
	if r != nil && r.rates != nil {
		return r.rates[groupID], nil
REDACTED
	return nil, nil
REDACTED

var _ BatchImageGroupPricingRepository = (*publicBatchImageGroupRepo)(nil)
var _ BatchImageUserGroupRateRepository = (*publicBatchImageUserGroupRateRepo)(nil)
