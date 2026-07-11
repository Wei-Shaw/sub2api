package handler

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestD2ApiKeyVideoContractMatchesSnapshotV1(t *testing.T) {
	rawSnapshot, err := os.ReadFile("testdata/api_key_video_task_contract_v1.json")
	if err != nil {
		t.Fatalf("read contract snapshot: %v", err)
	}
	var snapshot struct {
		Request  apiKeyVideoTaskCreateRequest `json:"request"`
		Response map[string]any               `json:"response"`
	}
	if err := json.Unmarshal(rawSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal contract snapshot: %v", err)
	}

	reqRaw, err := json.Marshal(snapshot.Request)
	if err != nil {
		t.Fatalf("marshal snapshot request: %v", err)
	}
	var bound apiKeyVideoTaskCreateRequest
	if err := json.Unmarshal(reqRaw, &bound); err != nil {
		t.Fatalf("snapshot request must bind to apiKeyVideoTaskCreateRequest: %v", err)
	}
	if bound.TaskType != service.VideoTaskTypeReferenceToVideo ||
		bound.Model != "mock-video-v1" ||
		bound.ReferenceImageURL != "https://example.invalid/ref.png" ||
		bound.AspectRatio != "16:9" ||
		bound.Duration != 5 ||
		bound.Resolution != "720p" {
		t.Fatalf("snapshot request did not bind expected top-level fields: %+v", bound)
	}
	if len(bound.Content) != 4 {
		t.Fatalf("snapshot request content length = %d, want 4", len(bound.Content))
	}
	if bound.Content[1].Type != "image_url" ||
		bound.Content[1].Role != "reference_image" ||
		bound.Content[1].URL != "https://example.invalid/ref-a.png" {
		t.Fatalf("snapshot request did not bind expected content item: %+v", bound.Content[1])
	}
	if bound.Content[2].Type != "video_url" ||
		bound.Content[2].Role != "reference_video" ||
		bound.Content[2].URL != "https://example.invalid/ref.mp4" {
		t.Fatalf("snapshot request did not bind expected video content item: %+v", bound.Content[2])
	}
	if bound.GenerateAudio == nil || *bound.GenerateAudio ||
		bound.Watermark == nil || *bound.Watermark ||
		bound.CameraFixed == nil || !*bound.CameraFixed ||
		bound.ReturnLastFrame == nil || !*bound.ReturnLastFrame {
		t.Fatalf("snapshot request did not bind expected boolean options: %+v", bound)
	}

	tokens := int64(321)
	actualDuration := 5
	generateAudio := false
	watermark := false
	cameraFixed := true
	returnLastFrame := true
	task := &service.VideoTask{
		ID:                12345,
		Provider:          service.VideoProviderMock,
		Model:             bound.Model,
		TaskType:          bound.TaskType,
		Prompt:            bound.Prompt,
		ReferenceImageURL: bound.ReferenceImageURL,
		Content:           bound.Content,
		HasVideoInput:     true,
		AspectRatio:       bound.AspectRatio,
		Duration:          bound.Duration,
		Resolution:        bound.Resolution,
		GenerateAudio:     &generateAudio,
		Watermark:         &watermark,
		CameraFixed:       &cameraFixed,
		ReturnLastFrame:   &returnLastFrame,
		Status:            service.VideoStatusSucceeded,
		ResultURL:         "/api/v1/video/mock-assets/12345.svg",
		UsageTotalTokens:  &tokens,
		ActualResolution:  "720p",
		ActualDuration:    &actualDuration,
		LastFrameURL:      "/api/v1/video/mock-assets/12345-last-frame.png",
	}
	rawResponse, err := json.Marshal(apiKeyVideoTaskToResponse(task, nil))
	if err != nil {
		t.Fatalf("marshal api-key video response: %v", err)
	}
	var actual map[string]any
	if err := json.Unmarshal(rawResponse, &actual); err != nil {
		t.Fatalf("unmarshal api-key video response: %v", err)
	}
	for key, want := range snapshot.Response {
		got, ok := actual[key]
		if !ok {
			t.Fatalf("response missing snapshot key %q; actual keys=%v", key, actual)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("response[%s]=%v (%T), want %v (%T)", key, got, got, want, want)
		}
	}
	if _, ok := actual["ResultURL"]; ok {
		t.Fatalf("api-key response must not carry PascalCase ResultURL; actual keys=%v", actual)
	}
}

// TestC1ApiKeyVideoResponseMatchesQCanvasContract is the C1 skeleton contract guard.
//
// The end-to-end skeleton is QCanvas → Sub2API (mock provider) → result → candidate.
// The Sub2API service→mock→succeeded chain is already proven dynamically by the
// service-package tests (TestFormASeedanceHappyPathInMemory drives create→worker→poll
// →succeeded against an in-memory repo). The QCanvas side maps the Sub2API response in
// apps/hono-api/.../sub2api.video-mock-gateway.service.ts#normalizeTaskRecord, which
// reads these keys: taskId←(task_id|taskId|id), status, resultUrl←(result_url|resultUrl
// |url), errorMessage←(error_message|errorMessage|message), provider.
//
// The one link NOT covered by either side's tests is the HTTP RESPONSE SERIALIZATION:
// that the api-key video response Sub2API actually emits carries those exact JSON keys.
// This test marshals the real response builder for a representative succeeded MOCK task
// (the shape the always-on mock adapter produces) and asserts the wire keys QCanvas
// consumes are present and correct. If a json tag drifts, this fails before QCanvas's
// mapping silently degrades to "sub2api-mock-task-missing-id".
func TestC1ApiKeyVideoResponseMatchesQCanvasContract(t *testing.T) {
	// Mirror what the mock adapter yields on success (provider=mock, succeeded, a
	// playable mock URL, a mock upstream id).
	task := &service.VideoTask{
		ID:             12345,
		Provider:       service.VideoProviderMock,
		Model:          "mock-video",
		TaskType:       service.VideoTaskTypeTextToVideo,
		Prompt:         "qcanvas C1 skeleton clip",
		Status:         service.VideoStatusSucceeded,
		UpstreamTaskID: "mock-video-12345",
		ResultURL:      "/api/v1/video/mock-assets/12345.svg",
	}

	raw, err := json.Marshal(apiKeyVideoTaskToResponse(task, nil))
	if err != nil {
		t.Fatalf("marshal api-key video response: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// --- keys QCanvas normalizeTaskRecord reads (the create/poll/result contract) ---
	if id, ok := m["id"].(float64); !ok || int64(id) != 12345 {
		t.Fatalf("response must carry numeric `id` (QCanvas taskId source), got %v (%T)", m["id"], m["id"])
	}
	if m["status"] != "succeeded" {
		t.Fatalf("response `status` = %v, want succeeded (QCanvas status source)", m["status"])
	}
	if m["result_url"] != "/api/v1/video/mock-assets/12345.svg" {
		t.Fatalf("response `result_url` = %v (QCanvas resultUrl source)", m["result_url"])
	}
	if _, ok := m["ResultURL"]; ok {
		t.Fatal("response must not carry duplicate PascalCase `ResultURL`")
	}
	if _, ok := m["error_message"]; !ok {
		t.Fatal("response must carry `error_message` (QCanvas errorMessage source)")
	}
	if m["provider"] != "mock" {
		t.Fatalf("response `provider` = %v, want mock", m["provider"])
	}

	// --- mock-only boundary fields QCanvas diagnostics relies on ---
	if m["mock_only"] != true {
		t.Fatalf("mock task must report mock_only=true, got %v", m["mock_only"])
	}
	if cnt, ok := m["real_provider_dispatch_count"].(float64); !ok || cnt != 0 {
		t.Fatalf("mock-only response must report real_provider_dispatch_count=0, got %v", m["real_provider_dispatch_count"])
	}
	if m["provider_boundary"] != "api-key-video-mock-only" {
		t.Fatalf("mock-only boundary = %v, want api-key-video-mock-only", m["provider_boundary"])
	}
}

// TestC1ApiKeyVideoCreateRequestAcceptsQCanvasBody proves the inbound side: the JSON
// body QCanvas's createSub2ApiVideoMockTask sends ({provider, task_type, prompt, model,
// ...}) binds onto Sub2API's apiKeyVideoTaskCreateRequest with provider=mock accepted.
func TestC1ApiKeyVideoCreateRequestAcceptsQCanvasBody(t *testing.T) {
	// The exact shape QCanvas POSTs to /v1/video/tasks (mock path).
	body := []byte(`{"provider":"mock","task_type":"text_to_video","prompt":"hello","model":"mock-video","metadata":{"nodeId":"n1"}}`)
	var req apiKeyVideoTaskCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("QCanvas body must unmarshal onto apiKeyVideoTaskCreateRequest: %v", err)
	}
	if req.Provider != "mock" {
		t.Fatalf("provider = %q, want mock", req.Provider)
	}
	if req.TaskType != "text_to_video" {
		t.Fatalf("task_type = %q, want text_to_video", req.TaskType)
	}
	if req.Prompt != "hello" {
		t.Fatalf("prompt = %q, want hello", req.Prompt)
	}
}

func TestVideoTaskDeliveryLifecycleMapper(t *testing.T) {
	tests := []struct {
		name         string
		task         *service.VideoTask
		wantDelivery string
		wantNext     string
		wantStatus   string
	}{
		{
			name:         "non terminal remains processing",
			task:         &service.VideoTask{Status: service.VideoStatusRunning},
			wantDelivery: videoDeliveryStatusProcessing,
			wantNext:     videoNextActionPoll,
			wantStatus:   service.VideoStatusRunning,
		},
		{
			name:         "succeeded archive pending is archiving",
			task:         &service.VideoTask{Status: service.VideoStatusSucceeded, ArchiveStatus: service.VideoSideEffectStatusPending, ResultURL: "https://example.invalid/result.mp4"},
			wantDelivery: videoDeliveryStatusArchiving,
			wantNext:     videoNextActionArchive,
			wantStatus:   service.VideoStatusSucceeded,
		},
		{
			name:         "local asset takes priority",
			task:         &service.VideoTask{Status: service.VideoStatusSucceeded, ArchiveStatus: "failed", LocalAssetPath: "assets/video/7/result.mp4"},
			wantDelivery: videoDeliveryStatusDeliverable,
			wantNext:     videoNextActionDownload,
			wantStatus:   service.VideoStatusSucceeded,
		},
		{
			name:         "archive dead without asset is delivery failed",
			task:         &service.VideoTask{Status: service.VideoStatusSucceeded, ArchiveStatus: "failed"},
			wantDelivery: videoDeliveryStatusFailed,
			wantNext:     videoNextActionReviewDelivery,
			wantStatus:   service.VideoStatusSucceeded,
		},
		{
			name: "archive dead with retained expired remote URL is delivery failed",
			task: &service.VideoTask{
				Status:        service.VideoStatusSucceeded,
				ArchiveStatus: "failed",
				ResultURL:     "https://example.invalid/result.mp4",
				CompletedAt:   timePointer(time.Now().UTC().Add(-48 * time.Hour)),
			},
			wantDelivery: videoDeliveryStatusFailed,
			wantNext:     videoNextActionReviewDelivery,
			wantStatus:   service.VideoStatusSucceeded,
		},
		{
			name:         "unknown dispatch requires reconciliation",
			task:         &service.VideoTask{Status: service.VideoStatusSubmitted, DispatchState: service.VideoDispatchStateUnknown},
			wantDelivery: videoDeliveryStatusProcessing,
			wantNext:     videoNextActionReconcileDispatch,
			wantStatus:   service.VideoStatusSubmitted,
		},
		{
			name:         "settlement error does not change generation status",
			task:         &service.VideoTask{Status: service.VideoStatusSucceeded, SettlementStatus: "error", ResultURL: "https://example.invalid/result.mp4", ArchiveStatus: "succeeded"},
			wantDelivery: videoDeliveryStatusDeliverable,
			wantNext:     videoNextActionReviewSettlement,
			wantStatus:   service.VideoStatusSucceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := videoTaskToResponse(tt.task, nil)
			if got.DeliveryStatus != tt.wantDelivery || got.NextAction != tt.wantNext {
				t.Fatalf("delivery=%q next_action=%q, want %q/%q", got.DeliveryStatus, got.NextAction, tt.wantDelivery, tt.wantNext)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("generation status=%q, want %q", got.Status, tt.wantStatus)
			}
		})
	}
}

func TestAPIKeyVideoTaskResponseDistinguishesSeedanceBoundaries(t *testing.T) {
	task := &service.VideoTask{Provider: service.VideoProviderSeedance}
	production := apiKeyVideoTaskToResponse(task, nil)
	if production.ProviderBoundary != "api-key-video-seedance-production" {
		t.Fatalf("production seedance boundary = %q", production.ProviderBoundary)
	}
	if production.TrialMode != "" {
		t.Fatalf("production response must not report a trial mode: %q", production.TrialMode)
	}

	tinyEvents := []*service.VideoTaskEvent{{
		EventType: "trial_gate",
		Payload: map[string]any{
			"trial_mode":  "tiny_real",
			"gate_result": "passed",
		},
	}}
	tiny := apiKeyVideoTaskToResponse(task, tinyEvents)
	if tiny.ProviderBoundary != "api-key-video-seedance-tiny-trial" {
		t.Fatalf("tiny trial boundary = %q", tiny.ProviderBoundary)
	}
	if tiny.TrialMode != "tiny_real" {
		t.Fatalf("tiny trial mode = %q", tiny.TrialMode)
	}

	cancelledTiny := apiKeyVideoTaskToResponse(&service.VideoTask{
		Provider: service.VideoProviderSeedance,
		Status:   service.VideoStatusCancelled,
	}, tinyEvents)
	if cancelledTiny.ProviderBoundary != "api-key-video-seedance-tiny-trial" {
		t.Fatalf("cancelled tiny trial boundary = %q", cancelledTiny.ProviderBoundary)
	}

	// Cancel must never fall back to nil events for Seedance tasks: that mislabels tiny trial as production.
	lostProvenance := apiKeyVideoTaskToResponse(task, nil)
	if lostProvenance.ProviderBoundary != "api-key-video-seedance-production" {
		t.Fatalf("nil-events Seedance response = %q, mapper defaults to production; cancel path must supply provenance events", lostProvenance.ProviderBoundary)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
