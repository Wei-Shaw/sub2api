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
	})

	t.Run("accepts valid request stores refs and enqueues once", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)

		got, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.NoError(t, err)
		require.Equal(t, "image.batch", got.Object)
		require.Equal(t, "queued", got.Status)
		require.Equal(t, BatchImageProviderGeminiAPI, got.Provider)
		require.Equal(t, 2, got.ItemCount)
		require.Equal(t, 0.25, got.EstimatedCost)
		require.Len(t, repo.jobs, 1)
		require.Len(t, gemini.submits, 1)
		require.Equal(t, []string{got.ID}, queue.enqueued)
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)
		require.Len(t, billing.reserves, 1)
		require.Equal(t, BatchImageHoldRequestID(got.ID), billing.reserves[0].RequestID)
		require.InDelta(t, 0.3, billing.reserves[0].HoldAmount, 1e-12)
		require.Empty(t, billing.releases)
		authCache := svc.AuthCache.(*fakeBatchImageAuthCacheInvalidator)
		require.Equal(t, []int64{11}, authCache.userIDs)

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
	})

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
			},
		}}
		userRate := 0.5
		svc.UserGroupRateRepo = &publicBatchImageUserGroupRateRepo{rates: map[int64]*float64{groupID: &userRate}}

		got, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupID}, validBatchImageSubmitRequest(), "")
		require.NoError(t, err)
		require.InDelta(t, 0.25, got.EstimatedCost, 1e-12)

		job := repo.jobs[got.ID]
		require.InDelta(t, 0.25, job.BaseUnitPrice, 1e-12)
		require.InDelta(t, 0.5, job.GroupRateMultiplier, 1e-12)
		require.InDelta(t, 1.25, job.AccountRateMultiplier, 1e-12)
		require.InDelta(t, 0.8, job.BatchDiscountMultiplier, 1e-12)
		// 配置的 hold(0.6) < discount(0.8) 属于会导致结算死锁的脏数据，
		// 快照时被钳制为 discount，保证 holdAmount >= 实际成本上限。
		require.InDelta(t, 0.8, job.HoldMultiplier, 1e-12)
		require.InDelta(t, 0.125, job.BillableUnitPrice, 1e-12)
		require.InDelta(t, 0.125, job.HoldUnitPrice, 1e-12)
		require.InDelta(t, 0.25, *job.HoldAmount, 1e-12)
	})

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
			},
		}}

		got, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupID}, validBatchImageSubmitRequest(), "")
		require.NoError(t, err)
		require.InDelta(t, 0.134, got.EstimatedCost, 1e-12)

		job := repo.jobs[got.ID]
		require.InDelta(t, 0.134, job.BaseUnitPrice, 1e-12)
		require.InDelta(t, 0.067, job.BillableUnitPrice, 1e-12)
		require.InDelta(t, 0.0804, job.HoldUnitPrice, 1e-12)
		require.InDelta(t, 0.1608, *job.HoldAmount, 1e-12)
	})

	t.Run("pricing missing rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		svc.Pricing = &fakeBatchImagePricingResolver{err: ErrBatchImageSettlementPricingMissing}

		_, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageSettlementPricingMissing)
		require.Empty(t, repo.jobs)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
	})

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
			},
		}}

		_, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupID}, validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageGroupDisabled)
		require.Empty(t, repo.jobs)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
	})

	t.Run("group pricing load failure rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		groupID := int64(404)

		_, err := svc.Submit(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupID}, validBatchImageSubmitRequest(), "")
		require.ErrorIs(t, err, ErrBatchImageSettlementPricingMissing)
		require.Empty(t, repo.jobs)
		require.Empty(t, queue.enqueued)
		require.Empty(t, gemini.submits)
	})

	t.Run("generates custom ids deterministically", func(t *testing.T) {
		svc, _, _, gemini, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Items[0].CustomID = ""
		req.Items[1].CustomID = ""

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.NoError(t, err)
		require.Len(t, gemini.submits, 1)
		require.Equal(t, "item_000001", gemini.submits[0].Items[0].CustomID)
		require.Equal(t, "item_000002", gemini.submits[0].Items[1].CustomID)
	})

	t.Run("expands output count into separate billable items", func(t *testing.T) {
		svc, repo, _, gemini, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Items = []BatchImageSubmitItem{
			{CustomID: "cover", Prompt: "hero", OutputCount: 3, ReferenceImages: []BatchImageReferenceInput{{MimeType: "image/png", Data: []byte("ref")}}},
		}

		got, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.NoError(t, err)
		require.Equal(t, 3, got.ItemCount)
		require.InDelta(t, 0.375, got.EstimatedCost, 1e-12)
		require.Len(t, gemini.submits, 1)
		require.Len(t, gemini.submits[0].Items, 3)
		require.Equal(t, []string{"cover_01", "cover_02", "cover_03"}, []string{
			gemini.submits[0].Items[0].CustomID,
			gemini.submits[0].Items[1].CustomID,
			gemini.submits[0].Items[2].CustomID,
		})
		require.Len(t, gemini.submits[0].Items[0].ReferenceImages, 1)
		require.Len(t, repo.items[got.ID], 3)
	})

	t.Run("validates request fields", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*BatchImageSubmitRequest)
			want   error
		}{
			{name: "missing_model", mutate: func(r *BatchImageSubmitRequest) { r.Model = "" }, want: ErrBatchImageInvalidModel},
			{name: "empty_items", mutate: func(r *BatchImageSubmitRequest) { r.Items = nil }, want: ErrBatchImageInvalidItems},
			{name: "duplicate_custom_ids", mutate: func(r *BatchImageSubmitRequest) { r.Items[1].CustomID = r.Items[0].CustomID }, want: ErrBatchImageDuplicateCustomIDInRequest},
			{name: "empty_prompt", mutate: func(r *BatchImageSubmitRequest) { r.Items[0].Prompt = " " }, want: ErrBatchImageInvalidItems},
			{name: "prompt_too_long", mutate: func(r *BatchImageSubmitRequest) { r.Items[0].Prompt = strings.Repeat("x", 9) }, want: ErrBatchImagePromptTooLong},
			{name: "unsupported_provider", mutate: func(r *BatchImageSubmitRequest) { r.Provider = "other" }, want: ErrBatchImageUnsupportedProvider},
			{name: "vertex_rejects_2k", mutate: func(r *BatchImageSubmitRequest) { r.Provider = BatchImageProviderVertex; r.ImageSize = "2K" }, want: ErrBatchImageInvalidItems},
			{name: "too_many_outputs_per_item", mutate: func(r *BatchImageSubmitRequest) {
				r.Items[0].OutputCount = 5
			}, want: ErrBatchImageInvalidItems},
			{name: "too_many_reference_images_for_flash", mutate: func(r *BatchImageSubmitRequest) {
				r.Model = "gemini-2.5-flash-image"
				r.Items[0].ReferenceImages = []BatchImageReferenceInput{
					{MimeType: "image/png", Data: []byte("1")},
					{MimeType: "image/png", Data: []byte("2")},
					{MimeType: "image/png", Data: []byte("3")},
					{MimeType: "image/png", Data: []byte("4")},
				}
			}, want: ErrBatchImageTooManyReferenceImages},
			{name: "bad_reference_mime", mutate: func(r *BatchImageSubmitRequest) {
				r.Items[0].ReferenceImages = []BatchImageReferenceInput{{MimeType: "application/octet-stream", Data: []byte("x")}}
			}, want: ErrBatchImageInvalidReferenceImage},
			{name: "reference_requires_data_or_file_uri", mutate: func(r *BatchImageSubmitRequest) {
				r.Items[0].ReferenceImages = []BatchImageReferenceInput{{MimeType: "image/png"}}
			}, want: ErrBatchImageInvalidReferenceImage},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				svc, _, _, _, _ := newTestBatchImagePublicService(true)
				req := validBatchImageSubmitRequest()
				tt.mutate(&req)

				_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
				require.ErrorIs(t, err, tt.want)
			})
		}
	})

	t.Run("rejects too many items", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Items = append(req.Items, BatchImageSubmitItem{CustomID: "too_many", Prompt: "x"})

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageInvalidItems)
	})

	t.Run("rejects too many output images", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Config.BatchImage.MaxOutputImagesPerJob = 3
		req := validBatchImageSubmitRequest()
		req.Items[0].OutputCount = 2
		req.Items[1].OutputCount = 2

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageTooManyOutputImages)
	})

	t.Run("rejects too many reference images across request", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Config.BatchImage.MaxReferenceImagesPerJob = 3
		req := validBatchImageSubmitRequest()
		req.Model = "gemini-2.5-flash-image"
		req.Items[0].ReferenceImages = []BatchImageReferenceInput{
			{MimeType: "image/png", Data: []byte("1")},
			{MimeType: "image/png", Data: []byte("2")},
		}
		req.Items[1].ReferenceImages = []BatchImageReferenceInput{
			{MimeType: "image/png", Data: []byte("3")},
			{MimeType: "image/png", Data: []byte("4")},
		}

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageTooManyReferenceImages)
	})

	t.Run("rejects too much inline reference image data across request", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Config.BatchImage.MaxReferenceImagesPerJob = 10
		svc.Config.BatchImage.MaxReferenceInlineBytesPerJob = 4
		req := validBatchImageSubmitRequest()
		req.Model = "gemini-2.5-flash-image"
		req.Items[0].ReferenceImages = []BatchImageReferenceInput{{MimeType: "image/png", Data: []byte("123")}}
		req.Items[1].ReferenceImages = []BatchImageReferenceInput{{MimeType: "image/png", Data: []byte("456")}}

		_, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.ErrorIs(t, err, ErrBatchImageReferenceImagesTooLarge)
	})

	t.Run("selects requested provider", func(t *testing.T) {
		svc, _, _, gemini, vertex := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		req.Provider = BatchImageProviderVertex

		got, err := svc.Submit(ctx, testBatchImageOwner(), req, "")
		require.NoError(t, err)
		require.Equal(t, BatchImageProviderVertex, got.Provider)
		require.Empty(t, gemini.submits)
		require.Len(t, vertex.submits, 1)
	})

	t.Run("insufficient balance rejects before provider submit", func(t *testing.T) {
		svc, repo, queue, gemini, _ := newTestBatchImagePublicService(true)
		billing := &fakeBatchImageBillingRepo{err: ErrBatchImageInsufficientBalance}
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
		}
	})

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
		}
	})

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
			require.Equal(t, []string{job.BatchID}, queue.enqueued)
		}
	})

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
		}
	})

	t.Run("idempotency returns same batch without provider resubmit", func(t *testing.T) {
		svc, _, queue, gemini, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()

		first, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
		require.NoError(t, err)
		second, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
		require.NoError(t, err)

		require.Equal(t, first.ID, second.ID)
		require.Len(t, gemini.submits, 1)
		require.Equal(t, []string{first.ID}, queue.enqueued)
	})

	t.Run("idempotency conflict rejects changed request", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		req := validBatchImageSubmitRequest()
		first, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
		require.NoError(t, err)

		req.Items[0].Prompt = "diff"
		second, err := svc.Submit(ctx, testBatchImageOwner(), req, "client-key")
		require.Nil(t, second)
		require.ErrorIs(t, err, ErrBatchImageIdempotencyConflict)
		require.NotEmpty(t, first.ID)
	})

	t.Run("public response does not expose internals", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		got, err := svc.Submit(ctx, testBatchImageOwner(), validBatchImageSubmitRequest(), "")
		require.NoError(t, err)

		body, err := json.Marshal(got)
		require.NoError(t, err)
		requireBatchImagePublicJSONHasNoInternals(t, string(body))
	})
}

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
	}
	repo.jobs["hidden-other-key"] = &BatchImageJob{
		BatchID:   "hidden-other-key",
		UserID:    11,
		APIKeyID:  &otherKeyID,
		Status:    BatchImageJobStatusCompleted,
		Provider:  BatchImageProviderVertex,
		Model:     "gemini-3.1-flash-lite-image",
		ItemCount: 1,
		CreatedAt: time.Now(),
	}

	got, err := svc.List(ctx, BatchImageOwner{UserID: 11, APIKeyID: visibleKeyID}, BatchImageJobsQuery{Limit: 20})
	require.NoError(t, err)
	require.Equal(t, "list", got.Object)
	require.Len(t, got.Data, 1)
	require.Equal(t, "visible-1", got.Data[0].ID)
	require.False(t, got.HasMore)
}

func TestBatchImagePublicService_ListModels(t *testing.T) {
	ctx := context.Background()

	t.Run("requires explicit account model mapping", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)

		got, err := svc.ListModels(ctx, testBatchImageOwner())
		require.NoError(t, err)
		require.Equal(t, "list", got.Object)
		require.Empty(t, got.Data)
	})

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
			},
		}}
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts = []Account{testBatchImageMappedAccount(303, AccountTypeAPIKey, map[string]any{
			"gemini-2.5-flash-image": "gemini-2.5-flash-image",
		})}

		got, err := svc.ListModels(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupID})
		require.NoError(t, err)
		require.Equal(t, []BatchImagePublicModel{{
			ID:       "gemini-2.5-flash-image",
			Object:   "image.batch.model",
			Provider: BatchImageProviderGeminiAPI,
		}, {
			ID:       "gemini-2.5-flash-image",
			Object:   "image.batch.model",
			Provider: BatchImageProviderVertex,
		}}, got.Data)
	})

	t.Run("expands wildcard mappings against batch image candidates", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts = []Account{testBatchImageMappedAccount(303, AccountTypeAPIKey, map[string]any{
			"gemini-3.1-*": "gemini-3.1-flash-lite-image",
		})}

		got, err := svc.ListModels(ctx, testBatchImageOwner())
		require.NoError(t, err)
		require.NotEmpty(t, got.Data)
		ids := make([]string, 0, len(got.Data))
		for _, model := range got.Data {
			ids = append(ids, model.ID)
		}
		require.Contains(t, ids, "gemini-3.1-flash-image")
		require.Contains(t, ids, "gemini-3.1-flash-lite-image")
		require.NotContains(t, ids, "gemini-2.5-flash-image")
	})

	t.Run("filters models without batch image pricing", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		svc.Pricing = &fakeBatchImagePricingResolver{
			unitPrice:     0.25,
			missingModels: map[string]bool{"gemini-3.1-flash-lite-image": true},
		}
		accountRepo := svc.AccountRepo.(*publicBatchImageAccountRepo)
		accountRepo.accounts = []Account{testBatchImageMappedAccount(303, AccountTypeAPIKey, map[string]any{
			"gemini-2.5-flash-image":      "gemini-2.5-flash-image",
			"gemini-3.1-flash-lite-image": "gemini-3.1-flash-lite-image",
		})}

		got, err := svc.ListModels(ctx, testBatchImageOwner())
		require.NoError(t, err)
		ids := make([]string, 0, len(got.Data))
		for _, model := range got.Data {
			ids = append(ids, model.ID)
		}
		require.Contains(t, ids, "gemini-2.5-flash-image")
		require.NotContains(t, ids, "gemini-3.1-flash-lite-image")
	})

	t.Run("rejects when group disables batch image", func(t *testing.T) {
		svc, _, _, _, _ := newTestBatchImagePublicService(true)
		groupID := int64(7)
		svc.GroupRepo = &publicBatchImageGroupRepo{groups: map[int64]*Group{
			groupID: {ID: groupID, AllowBatchImageGeneration: false},
		}}

		_, err := svc.ListModels(ctx, BatchImageOwner{UserID: 11, APIKeyID: 22, GroupID: &groupID})
		require.ErrorIs(t, err, ErrBatchImageGroupDisabled)
	})
}

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
		}

		got, err := svc.Get(ctx, testBatchImageOwner(), "imgbatch_status")
		require.NoError(t, err)
		require.Equal(t, "processing_results", got.Status)
		body, err := json.Marshal(got)
		require.NoError(t, err)
		requireBatchImagePublicJSONHasNoInternals(t, string(body))

		_, err = svc.Get(ctx, BatchImageOwner{UserID: 11, APIKeyID: 999}, "imgbatch_status")
		require.ErrorIs(t, err, ErrBatchImageJobNotFound)
	})

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
		}
		sourceObject := "gs://bucket/internal/output.jsonl"
		mime := "image/png"
		ext := "png"
		code := "SAFETY_BLOCKED"
		msg := "blocked in gs://bucket/internal/output.jsonl"
		repo.items["imgbatch_items"] = []CreateBatchImageItemParams{
			{JobID: "imgbatch_items", CustomID: "ok_1", Status: BatchImageItemStatusSuccess, ProviderSourceObject: &sourceObject, MimeType: &mime, FileExtension: &ext, ImageCount: 1},
			{JobID: "imgbatch_items", CustomID: "bad_1", Status: BatchImageItemStatusFailed, ProviderSourceObject: &sourceObject, ErrorCode: &code, ErrorMessage: &msg},
			{JobID: "imgbatch_items", CustomID: "ok_2", Status: BatchImageItemStatusSuccess, MimeType: &mime, FileExtension: &ext, ImageCount: 1},
		}

		page, err := svc.ListItems(ctx, testBatchImageOwner(), "imgbatch_items", BatchImageItemsQuery{Limit: 1})
		require.NoError(t, err)
		require.True(t, page.HasMore)
		require.Len(t, page.Data, 1)
		require.Equal(t, "ok_1", page.Data[0].CustomID)

		filtered, err := svc.ListItems(ctx, testBatchImageOwner(), "imgbatch_items", BatchImageItemsQuery{Status: "failed", Limit: 100})
		require.NoError(t, err)
		require.False(t, filtered.HasMore)
		require.Len(t, filtered.Data, 1)
		require.Equal(t, "failed", filtered.Data[0].Status)
		require.NotNil(t, filtered.Data[0].Error)
		require.Equal(t, "upstream provider operation failed", filtered.Data[0].Error.Message)

		body, err := json.Marshal(filtered)
		require.NoError(t, err)
		requireBatchImagePublicJSONHasNoInternals(t, string(body))
		require.NotContains(t, string(body), "download_url")

		_, err = svc.ListItems(ctx, BatchImageOwner{UserID: 12, APIKeyID: 22}, "imgbatch_items", BatchImageItemsQuery{})
		require.ErrorIs(t, err, ErrBatchImageJobNotFound)
	})

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
		}

		got, err := svc.Cancel(ctx, testBatchImageOwner(), "imgbatch_cancel")
		require.NoError(t, err)
		require.Equal(t, "queued", got.Status)
		require.Equal(t, 1, gemini.cancelCount)
		billing := svc.BillingRepo.(*fakeBatchImageBillingRepo)
		require.Empty(t, billing.releases)
		require.Equal(t, []string{"imgbatch_cancel"}, queue.enqueued)
		require.Equal(t, BatchImageJobStatusSubmitted, repo.jobs["imgbatch_cancel"].Status)
		require.Contains(t, repo.events["imgbatch_cancel"], "job_cancel_requested")
	})

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
		}

		got, err := svc.Cancel(ctx, testBatchImageOwner(), "imgbatch_done")
		require.NoError(t, err)
		require.Equal(t, "completed", got.Status)
		require.Zero(t, gemini.cancelCount)
	})

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
		}

		_, err := svc.Cancel(ctx, testBatchImageOwner(), "imgbatch_cancel_error")
		require.ErrorIs(t, err, ErrBatchImageCancelFailed)
		require.Equal(t, "BATCH_IMAGE_CANCEL_FAILED", infraerrors.Reason(err))
		require.NotContains(t, infraerrors.Message(err), "projects/")
	})
}

func newTestBatchImagePublicService(enabled bool) (*BatchImagePublicService, *fakeBatchImageRepository, *publicBatchImageQueue, *publicBatchImageProvider, *publicBatchImageProvider) {
	repo := newFakeBatchImageRepository()
	queue := &publicBatchImageQueue{}
	gemini := &publicBatchImageProvider{name: BatchImageProviderGeminiAPI}
	vertex := &publicBatchImageProvider{name: BatchImageProviderVertex}
	svc := &BatchImagePublicService{
		Repo:        repo,
		AccountRepo: &publicBatchImageAccountRepo{accounts: []Account{testBatchImageAccount(101, AccountTypeAPIKey), testBatchImageAccount(202, AccountTypeServiceAccount)}},
		Queue:       queue,
		ProviderRegistry: NewBatchImageProviderRegistry(
			gemini,
			vertex,
		),
		Pricing:     &fakeBatchImagePricingResolver{unitPrice: 0.25},
		BillingRepo: &fakeBatchImageBillingRepo{},
		AuthCache:   &fakeBatchImageAuthCacheInvalidator{},
		Config: &config.Config{BatchImage: config.BatchImageConfig{
			Enabled:                 enabled,
			MaxItemsPerJobDefault:   2,
			MaxPromptCharsPerItem:   8,
			DefaultResponseMimeType: "image/png",
			DefaultImageSize:        "1K",
		}},
	}
	return svc, repo, queue, gemini, vertex
}

func testBatchImageOwner() BatchImageOwner {
	return BatchImageOwner{UserID: 11, APIKeyID: 22}
}

type fakeBatchImageAuthCacheInvalidator struct {
	keys     []string
	userIDs  []int64
	groupIDs []int64
}

func (f *fakeBatchImageAuthCacheInvalidator) InvalidateAuthCacheByKey(_ context.Context, key string) {
	f.keys = append(f.keys, key)
}

func (f *fakeBatchImageAuthCacheInvalidator) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	f.userIDs = append(f.userIDs, userID)
}

func (f *fakeBatchImageAuthCacheInvalidator) InvalidateAuthCacheByGroupID(_ context.Context, groupID int64) {
	f.groupIDs = append(f.groupIDs, groupID)
}

func validBatchImageSubmitRequest() BatchImageSubmitRequest {
	return BatchImageSubmitRequest{
		Model:            "gemini-2.5-flash-image",
		Provider:         BatchImageProviderGeminiAPI,
		ResponseMimeType: "image/png",
		AspectRatio:      "1:1",
		ImageSize:        "1K",
		Metadata:         map[string]string{"project": "campaign-a", "secret": strings.Repeat("x", 300)},
		Items: []BatchImageSubmitItem{
			{CustomID: "cover_001", Prompt: "hero"},
			{CustomID: "cover_002", Prompt: "clean"},
		},
	}
}

func testBatchImageAccount(id int64, accountType string) Account {
	return Account{
		ID:            id,
		Platform:      PlatformGemini,
		Type:          accountType,
		Status:        StatusActive,
		Schedulable:   true,
		Priority:      int(id),
		Credentials:   map[string]any{"api_key": "test-secret"},
		Concurrency:   1,
		RateLimitedAt: nil,
	}
}

func testBatchImageMappedAccount(id int64, accountType string, mapping map[string]any) Account {
	account := testBatchImageAccount(id, accountType)
	account.Credentials["model_mapping"] = mapping
	return account
}

func requireBatchImagePublicJSONHasNoInternals(t *testing.T, body string) {
	t.Helper()
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
	} {
		require.NotContains(t, body, forbidden)
	}
}

type publicBatchImageAccountRepo struct {
	accounts []Account
}

func (r *publicBatchImageAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, errors.New("account not found")
}

func (r *publicBatchImageAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]Account, error) {
	out := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r *publicBatchImageAccountRepo) ListSchedulableByGroupIDAndPlatform(ctx context.Context, _ int64, platform string) ([]Account, error) {
	return r.ListSchedulableByPlatform(ctx, platform)
}

type publicBatchImageQueue struct {
	enqueued []string
	err      error
}

func (q *publicBatchImageQueue) Enqueue(_ context.Context, batchID string) error {
	if q.err != nil {
		return q.err
	}
	for _, existing := range q.enqueued {
		if existing == batchID {
			return ErrBatchImageAlreadyQueued
		}
	}
	q.enqueued = append(q.enqueued, batchID)
	return nil
}

func (q *publicBatchImageQueue) Reserve(context.Context, time.Duration) (ReservedBatchImageJob, error) {
	return ReservedBatchImageJob{}, ErrBatchImageQueueEmpty
}

func (q *publicBatchImageQueue) RequeueAfter(context.Context, string, time.Duration) error {
	return nil
}

func (q *publicBatchImageQueue) Ack(context.Context, string) error {
	return nil
}

func (q *publicBatchImageQueue) Heartbeat(context.Context, string) error {
	return nil
}

func (q *publicBatchImageQueue) MoveDueDelayedToReady(context.Context, int) (int, error) {
	return 0, nil
}

func (q *publicBatchImageQueue) RecoverStaleActive(context.Context, time.Duration, int) (int, error) {
	return 0, nil
}

func (q *publicBatchImageQueue) TryAcquireJobLock(context.Context, string, time.Duration) (BatchImageJobLock, bool, error) {
	return nil, false, nil
}

type publicBatchImageProvider struct {
	name           string
	submits        []BatchImageInput
	submitErr      error
	cancelCount    int
	cancelErr      error
	result         string
	cleanupTargets []CleanupTarget
	cleanupErr     error
}

func (p *publicBatchImageProvider) Name() string { return p.name }

func (p *publicBatchImageProvider) SupportsAccount(*Account) bool { return true }

func (p *publicBatchImageProvider) Submit(_ context.Context, _ *BatchImageJob, _ *Account, input BatchImageInput) (*BatchProviderJob, error) {
	p.submits = append(p.submits, input)
	if p.submitErr != nil {
		return nil, p.submitErr
	}
	return &BatchProviderJob{
		ProviderJobName:   "providers/" + p.name + "/job",
		ProviderInputRef:  "files/" + p.name + "/input",
		ProviderOutputRef: "files/" + p.name + "/output",
	}, nil
}

func (p *publicBatchImageProvider) Get(context.Context, *BatchImageJob, *Account) (*BatchProviderStatus, error) {
	return &BatchProviderStatus{InternalState: BatchProviderStateQueued}, nil
}

func (p *publicBatchImageProvider) Cancel(context.Context, *BatchImageJob, *Account) error {
	p.cancelCount++
	return p.cancelErr
}

func (p *publicBatchImageProvider) OpenResult(context.Context, *BatchImageJob, *Account) (io.ReadCloser, string, error) {
	return io.NopCloser(strings.NewReader(p.result)), "application/jsonl", nil
}

func (p *publicBatchImageProvider) Cleanup(_ context.Context, _ *BatchImageJob, _ *Account, target CleanupTarget) error {
	p.cleanupTargets = append(p.cleanupTargets, target)
	return p.cleanupErr
}

var _ BatchImageAccountSelectionRepository = (*publicBatchImageAccountRepo)(nil)
var _ BatchImageQueue = (*publicBatchImageQueue)(nil)
var _ BatchImageProvider = (*publicBatchImageProvider)(nil)

type publicBatchImageGroupRepo struct {
	groups map[int64]*Group
}

func (r *publicBatchImageGroupRepo) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	if r != nil && r.groups != nil {
		if group, ok := r.groups[id]; ok {
			return group, nil
		}
	}
	return nil, ErrGroupNotFound
}

type publicBatchImageUserGroupRateRepo struct {
	rates map[int64]*float64
}

func (r *publicBatchImageUserGroupRateRepo) GetByUserAndGroup(_ context.Context, _ int64, groupID int64) (*float64, error) {
	if r != nil && r.rates != nil {
		return r.rates[groupID], nil
	}
	return nil, nil
}

var _ BatchImageGroupPricingRepository = (*publicBatchImageGroupRepo)(nil)
var _ BatchImageUserGroupRateRepository = (*publicBatchImageUserGroupRateRepo)(nil)
