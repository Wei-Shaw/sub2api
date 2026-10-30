package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/minimax"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaReceiptRepo struct {
	service.MiniMaxMediaRepository
	records        int
	settles        int
	status         string
	bill           *service.MiniMaxMediaBill
	settleErr      error
	recordFailures int
}

func (r *mediaReceiptRepo) Record(_ context.Context, _ string, status string, bill *service.MiniMaxMediaBill, _ json.RawMessage) error {
	r.records++
	if r.recordFailures > 0 {
		r.recordFailures--
		return errors.New("receipt storage unavailable")
	}
	r.status = status
	r.bill = bill
	return nil
}
func (r *mediaReceiptRepo) Settle(context.Context, string) (bool, error) {
	r.settles++
	return r.settleErr == nil, r.settleErr
}
func handlerMediaTask() *service.MiniMaxMediaTask {
	price, mult := 100.0, 1.0
	return &service.MiniMaxMediaTask{ID: "mm_receipt", Model: "speech-2.8-hd", Kind: "speech", Pricing: service.MiniMaxMediaPricing{Retail: service.MiniMaxMediaRates{CharactersPerMillion: &price}, Cost: service.MiniMaxMediaRates{CharactersPerMillion: &price}, CostCurrency: "USD", Multiplier: &mult}}
}
func mediaStreamHarness(t *testing.T, repo *mediaReceiptRepo, stream io.Reader, clientCtx context.Context) (*httptest.ResponseRecorder, *service.MiniMaxMediaTask) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/t2a_v2", nil).WithContext(clientCtx)
	svc := service.NewMiniMaxMediaService(repo, nil, nil)
	h := &OpenAIGatewayHandler{minimaxMedia: svc}
	task := handlerMediaTask()
	h.miniMaxSpeechStream(c, context.Background(), task, stream)
	return w, task
}

const mediaFinalSSE = `data: {"base_resp":{"status_code":0},"data":{"status":2,"audio":"AABB"},"extra_info":{"usage_characters":12,"word_count":6}}` + "\n\n"

func TestMiniMaxMediaStreamMetersOnceAndAvoidsAggregatedAudio(t *testing.T) {
	repo := &mediaReceiptRepo{}
	stream := "data: {\"base_resp\":{\"status_code\":0},\"data\":{\"status\":1,\"audio\":\"AA\"}}\n\n" + mediaFinalSSE + mediaFinalSSE
	w, _ := mediaStreamHarness(t, repo, strings.NewReader(stream), context.Background())
	require.Equal(t, 1, repo.records)
	require.Equal(t, 1, repo.settles)
	require.EqualValues(t, 12, repo.bill.Usage.Characters)
	require.NotContains(t, w.Body.String(), `"audio":"AABB"`)
	require.Contains(t, w.Body.String(), `"audio":"AA"`)
}
func TestMiniMaxMediaStreamDrainsAfterClientCancellation(t *testing.T) {
	repo := &mediaReceiptRepo{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = mediaStreamHarness(t, repo, strings.NewReader(mediaFinalSSE), ctx)
	require.Equal(t, 1, repo.records)
	require.Equal(t, "succeeded", repo.status)
	require.Equal(t, 1, repo.settles)
}
func TestMiniMaxMediaIncompleteStreamRetainsUnknown(t *testing.T) {
	repo := &mediaReceiptRepo{}
	_, _ = mediaStreamHarness(t, repo, strings.NewReader("data: {\"base_resp\":{\"status_code\":0},\"data\":{\"status\":1,\"audio\":\"AA\"}}\n\n"), context.Background())
	require.Equal(t, "unknown", repo.status)
	require.Nil(t, repo.bill)
	require.Zero(t, repo.settles)
}
func TestMiniMaxMediaLedgerFailurePreservesReceipt(t *testing.T) {
	repo := &mediaReceiptRepo{settleErr: errors.New("ledger unavailable")}
	_, _ = mediaStreamHarness(t, repo, strings.NewReader(mediaFinalSSE), context.Background())
	require.Equal(t, "succeeded", repo.status)
	require.NotNil(t, repo.bill)
	require.Equal(t, 1, repo.settles)
}
func TestMiniMaxMediaReceiptStorageFailureRetainsUnknownHold(t *testing.T) {
	repo := &mediaReceiptRepo{recordFailures: 3}
	_, _ = mediaStreamHarness(t, repo, strings.NewReader(mediaFinalSSE), context.Background())
	require.Equal(t, 4, repo.records)
	require.Equal(t, "unknown", repo.status)
	require.Nil(t, repo.bill)
	require.Zero(t, repo.settles)
}
func TestMiniMaxMediaBusinessErrorAndCostRedaction(t *testing.T) {
	require.True(t, miniMaxSpeechRejected([]byte(`{"base_resp":{"status_code":1004},"data":null}`)))
	require.False(t, miniMaxSpeechRejected([]byte(`{"base_resp":{"status_code":1004},"data":{"audio":"AA"}}`)))
	require.False(t, miniMaxSpeechRejected([]byte(`{"base_resp":{"status_code":1004},"extra_info":{"usage_voice_count":1}}`)))
	task := handlerMediaTask()
	task.UpstreamID = "private-upstream"
	task.AccountID = 999
	task.Bill = &service.MiniMaxMediaBill{UpstreamCost: 123, UpstreamUSD: 123}
	data, err := json.Marshal(mediaPublicTask(task))
	require.NoError(t, err)
	require.NotContains(t, string(data), "upstream")
	require.NotContains(t, string(data), "account_id")
	require.NotContains(t, string(data), "cost_currency")
}

func TestMiniMaxMediaAuditsAllGenerationTextBeforeBilling(t *testing.T) {
	for _, tc := range []struct {
		kind, body string
		texts      []string
	}{
		{"speech", `{"model":"speech-2.8-hd","text":"speech text","voice_setting":{"voice_id":"system"}}`, []string{"speech text"}},
		{"image", `{"model":"image-01","prompt":"image prompt"}`, []string{"image prompt"}},
		{"voice_design", `{"prompt":"voice description","preview_text":"design preview"}`, []string{"voice description", "design preview"}},
		{"voice_clone", `{"file_id":123,"voice_id":"CloneVoice001","text":"clone preview","model":"speech-2.8-turbo","clone_prompt":{"prompt_audio":456,"prompt_text":"sample transcript"}}`, []string{"clone preview", "sample transcript"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			engine := blockingHandlerPromptEngine()
			h := &OpenAIGatewayHandler{minimaxMedia: service.NewMiniMaxMediaService(nil, nil, nil), securityAuditCoordinator: securityaudit.NewCoordinator(nil, engine)}
			router := gin.New()
			router.Use(func(c *gin.Context) {
				groupID := int64(3)
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7, User: &service.User{ID: 7}, GroupID: &groupID, Group: &service.Group{ID: groupID, Platform: service.PlatformMiniMax, AllowImageGeneration: true, RateMultiplier: 1}})
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 7})
				c.Next()
			})
			router.POST("/media", func(c *gin.Context) { h.MiniMaxMedia(c, tc.kind) })
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/media", strings.NewReader(tc.body))
			require.NotPanics(t, func() { router.ServeHTTP(w, req) }, "billing and upstream dependencies must not be reached after a block")
			require.Equal(t, http.StatusForbidden, w.Code)
			_, _, requests := engine.snapshot()
			require.Len(t, requests, 1)
			moderated := service.ExtractContentModerationText(requests[0].Protocol, requests[0].Body)
			snapshot, err := securityaudit.ExtractPromptSnapshot(requests[0])
			require.NoError(t, err)
			for _, text := range tc.texts {
				require.Contains(t, moderated, text)
				require.Contains(t, snapshot.FullPrompt, text)
			}
		})
	}
}

func TestMiniMaxMediaGenerationPermission(t *testing.T) {
	for _, tc := range []struct{ kind, body string }{
		{"image", `{"model":"image-01","prompt":"bird"}`},
		{"video", `{"model":"MiniMax-H3","resolution":"768P","duration":4,"ratio":"16:9","content":[{"type":"text","text":"bird"}]}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/media", strings.NewReader(tc.body))
			c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7, Group: &service.Group{Platform: service.PlatformMiniMax}})
			h := &OpenAIGatewayHandler{minimaxMedia: service.NewMiniMaxMediaService(nil, nil, nil)}
			require.NotPanics(t, func() { h.MiniMaxMedia(c, tc.kind) })
			require.Equal(t, http.StatusForbidden, w.Code)
		})
	}
}

type miniMaxHandlerReplayRepo struct {
	service.MiniMaxMediaRepository
	task *service.MiniMaxMediaTask
}

func (r *miniMaxHandlerReplayRepo) FindByIdempotencyKey(context.Context, int64, string) (*service.MiniMaxMediaTask, error) {
	return r.task, nil
}

func TestMiniMaxMediaReplayPrecedesCurrentAdmission(t *testing.T) {
	body := []byte(`{"model":"old-public-alias"}`)
	for _, kind := range []string{"speech", "video", "image", "voice_clone", "voice_design"} {
		t.Run(kind, func(t *testing.T) {
			task := &service.MiniMaxMediaTask{ID: "mm_original", Kind: kind, APIKeyID: 9, UserID: 7, RequestHash: service.HashUsageRequestPayload(body)}
			h := &OpenAIGatewayHandler{minimaxMedia: service.NewMiniMaxMediaService(&miniMaxHandlerReplayRepo{task: task}, nil, nil)}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/media", strings.NewReader(string(body)))
			c.Request.Header.Set("Idempotency-Key", "original")
			c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 9, UserID: 7, Group: &service.Group{Platform: service.PlatformComposite, ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"different-model"}}}})
			require.NotPanics(t, func() { h.MiniMaxMedia(c, kind) }, "replay must not need billing, pricing or an available upstream account")
			status := http.StatusConflict
			if kind == "video" {
				status = http.StatusOK
			}
			require.Equal(t, status, w.Code)
			require.Equal(t, task.ID, w.Header().Get("X-Sub2API-Task-ID"))
			require.Contains(t, w.Body.String(), task.ID)
		})
	}
}

func TestMiniMaxPublicReceiptDisclosesMeterSource(t *testing.T) {
	task := handlerMediaTask()
	task.Bill = &service.MiniMaxMediaBill{MeterSource: "request_characters", ReconciliationRequired: true, Usage: minimax.Usage{Characters: 3}}
	data, err := json.Marshal(mediaPublicTask(task))
	require.NoError(t, err)
	require.Contains(t, string(data), `"meter_source":"request_characters"`)
	require.Contains(t, string(data), `"reconciliation_required":true`)
}
