package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestVideoGatewayCreateTaskContentArrayContract(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "")
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeReferenceToVideo,
		Prompt:            "make a product demo",
		Content: []VideoTaskContentItem{
			{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/ref-a.png"},
			{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/ref-b.png"},
			{Type: "video_url", Role: "reference_video", URL: "https://assets.example.com/ref.mp4"},
			{Type: "audio_url", Role: "reference_audio", URL: "https://assets.example.com/ref.mp3"},
		},
		Duration:   5,
		Resolution: "720p",
		CreatedBy:  7,
	})
	if err != nil {
		t.Fatalf("create content-array task: %v", err)
	}
	if !task.HasVideoInput {
		t.Fatalf("expected has_video_input=true for video_url content")
	}
	if len(task.Content) != 5 {
		t.Fatalf("expected text + 4 media content items, got %#v", task.Content)
	}
	if task.Content[0].Type != "text" || task.Content[0].Text != "make a product demo" {
		t.Fatalf("expected prompt to be stored as first text content item, got %#v", task.Content[0])
	}
}

func TestVideoGatewayCreateTaskLegacyReferenceImageURLCompat(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "")
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	task, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID: providerID,
		TaskType:          VideoTaskTypeImageToVideo,
		Prompt:            "animate this image",
		ReferenceImageURL: "https://assets.example.com/first.png",
		Duration:          5,
		Resolution:        "720p",
		CreatedBy:         7,
	})
	if err != nil {
		t.Fatalf("create legacy image task: %v", err)
	}
	if task.ReferenceImageURL != "https://assets.example.com/first.png" {
		t.Fatalf("legacy reference_image_url changed: %q", task.ReferenceImageURL)
	}
	if task.HasVideoInput {
		t.Fatalf("image-only legacy task must not set has_video_input")
	}
	if len(task.Content) != 2 || task.Content[1].Role != "first_frame" {
		t.Fatalf("legacy image_to_video must map reference_image_url to first_frame content, got %#v", task.Content)
	}
}

func TestVideoGatewayCreateTaskRejectsInvalidContentContracts(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "")
	repo := newMemoryVideoGatewayRepo()
	providerID := repo.seedMockProvider()
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	cases := []struct {
		name    string
		params  VideoTaskCreateParams
		wantErr string
	}{
		{
			name: "mixed image modes",
			params: VideoTaskCreateParams{
				TaskType: VideoTaskTypeReferenceToVideo,
				Prompt:   "mixed",
				Content: []VideoTaskContentItem{
					{Type: "image_url", Role: "first_frame", URL: "https://assets.example.com/first.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/ref.png"},
				},
			},
			wantErr: "image modes",
		},
		{
			name: "audio only",
			params: VideoTaskCreateParams{
				TaskType: VideoTaskTypeReferenceToVideo,
				Prompt:   "audio",
				Content: []VideoTaskContentItem{
					{Type: "audio_url", Role: "reference_audio", URL: "https://assets.example.com/ref.mp3"},
				},
			},
			wantErr: "audio",
		},
		{
			name: "too many images",
			params: VideoTaskCreateParams{
				TaskType: VideoTaskTypeReferenceToVideo,
				Prompt:   "many",
				Content: []VideoTaskContentItem{
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/1.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/2.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/3.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/4.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/5.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/6.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/7.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/8.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/9.png"},
					{Type: "image_url", Role: "reference_image", URL: "https://assets.example.com/10.png"},
				},
			},
			wantErr: "9",
		},
		{
			name: "task type mismatch",
			params: VideoTaskCreateParams{
				TaskType: VideoTaskTypeTextToVideo,
				Prompt:   "mismatch",
				Content: []VideoTaskContentItem{
					{Type: "video_url", Role: "reference_video", URL: "https://assets.example.com/ref.mp4"},
				},
			},
			wantErr: "task_type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.params.ProviderAccountID = providerID
			tc.params.Duration = 5
			tc.params.Resolution = "720p"
			tc.params.CreatedBy = 7
			_, err := svc.CreateTask(context.Background(), tc.params)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestVideoGatewayCreateTaskValidatesSeedanceDurationAndResolution(t *testing.T) {
	repo := newMemoryVideoGatewayRepo()
	accountID := seedSmokeAuthorizedSeedanceProvider(repo, "seedance-test-key-1234567890", "https://ark.example.com/api/v3")
	svc := NewVideoGatewayService(repo, noopVideoKeyEncryptor{}, nil)

	if _, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID: accountID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             "doubao-seedance-2-0-fast",
		Prompt:            "fast 1080p should fail",
		Duration:          5,
		Resolution:        "1080p",
		CreatedBy:         7,
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "1080p") {
		t.Fatalf("expected fast 1080p validation error, got %v", err)
	}

	if _, err := svc.CreateTask(context.Background(), VideoTaskCreateParams{
		ProviderAccountID: accountID,
		TaskType:          VideoTaskTypeTextToVideo,
		Model:             "doubao-seedance-2-0-260128",
		Prompt:            "duration too long",
		Duration:          16,
		Resolution:        "720p",
		CreatedBy:         7,
	}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "duration") {
		t.Fatalf("expected duration validation error, got %v", err)
	}
}

func TestSeedanceCreateMapsContentArrayWithRoles(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "")
	var captured map[string]any
	adapter, acc := newSmokeGatedSeedanceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"t","status":"queued"}`))
	})

	generateAudio := false
	returnLastFrame := true
	task := &VideoTask{
		Model:           "doubao-seedance-2-0-260128",
		Prompt:          "make a product demo",
		Duration:        5,
		Resolution:      "720p",
		GenerateAudio:   &generateAudio,
		ReturnLastFrame: &returnLastFrame,
		Content: []VideoTaskContentItem{
			{Type: "text", Text: "make a product demo"},
			{Type: "image_url", Role: "reference_image", URL: "https://ref.cn-beijing.volces.com/ref-a.png"},
			{Type: "image_url", Role: "reference_image", URL: "https://ref.cn-beijing.volces.com/ref-b.png"},
			{Type: "video_url", Role: "reference_video", URL: "https://ref.cn-beijing.volces.com/ref.mp4"},
			{Type: "audio_url", Role: "reference_audio", URL: "https://ref.cn-beijing.volces.com/ref.mp3"},
		},
	}
	if _, err := adapter.CreateTask(context.Background(), acc, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	content, ok := captured["content"].([]any)
	if !ok || len(content) != 5 {
		t.Fatalf("expected 5 content items, got %#v", captured["content"])
	}
	if captured["generate_audio"] != false || captured["return_last_frame"] != true {
		t.Fatalf("top-level bool options not mapped: %+v", captured)
	}
	var sawAudio, sawVideo bool
	for _, raw := range content {
		item, _ := raw.(map[string]any)
		switch item["type"] {
		case "video_url":
			if item["role"] != "reference_video" {
				t.Fatalf("video role = %v", item["role"])
			}
			sawVideo = true
		case "audio_url":
			if item["role"] != "reference_audio" {
				t.Fatalf("audio role = %v", item["role"])
			}
			sawAudio = true
		}
	}
	if !sawVideo || !sawAudio {
		t.Fatalf("expected video and audio content, got %#v", content)
	}
}

func TestSeedanceCreatePayloadSnapshotMatchesArkContract(t *testing.T) {
	t.Setenv("SUB2API_VIDEO_URL_ALLOWLIST", "volces.com")

	generateAudio := false
	adapter := &seedanceVideoAdapter{}
	payload := adapter.BuildCreatePayload(&VideoProviderAccount{
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
	}, &VideoTask{
		Model:         "doubao-seedance-2-0-260128",
		Prompt:        "animate the product hero shot",
		TaskType:      VideoTaskTypeImageToVideo,
		AspectRatio:   "portrait",
		Duration:      10,
		Resolution:    "1080p",
		GenerateAudio: &generateAudio,
		Content: []VideoTaskContentItem{
			{Type: VideoContentTypeText, Text: "animate the product hero shot"},
			{Type: VideoContentTypeImageURL, Role: VideoContentRoleFirstFrame, URL: "https://assets.volces.com/frame-first.png"},
			{Type: VideoContentTypeImageURL, Role: VideoContentRoleLastFrame, URL: "https://assets.volces.com/frame-last.png"},
		},
	})

	if payload["duration"] != 10 {
		t.Fatalf("duration = %#v, want 10; payload=%#v", payload["duration"], payload)
	}
	if payload["resolution"] != "1080p" {
		t.Fatalf("resolution = %#v, want 1080p; payload=%#v", payload["resolution"], payload)
	}
	if payload["ratio"] != "9:16" {
		t.Fatalf("ratio = %#v, want 9:16; payload=%#v", payload["ratio"], payload)
	}
	if _, ok := payload["aspect_ratio"]; ok {
		t.Fatalf("payload must not send legacy aspect_ratio: %#v", payload)
	}
	if payload["generate_audio"] != false {
		t.Fatalf("generate_audio = %#v, want false; payload=%#v", payload["generate_audio"], payload)
	}

	content, ok := payload["content"].([]map[string]any)
	if !ok {
		t.Fatalf("content has type %T, want []map[string]any", payload["content"])
	}
	if len(content) != 3 {
		t.Fatalf("content len = %d, want 3; content=%#v", len(content), content)
	}
	if content[0]["type"] != VideoContentTypeText || content[0]["text"] != "animate the product hero shot" {
		t.Fatalf("text content mismatch: %#v", content[0])
	}
	if content[1]["type"] != VideoContentTypeImageURL || content[1]["role"] != VideoContentRoleFirstFrame {
		t.Fatalf("first frame content mismatch: %#v", content[1])
	}
	if content[2]["type"] != VideoContentTypeImageURL || content[2]["role"] != VideoContentRoleLastFrame {
		t.Fatalf("last frame content mismatch: %#v", content[2])
	}
}
