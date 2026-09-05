package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/media"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/repository"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/service"
)

type memoryMediaStorage struct {
	objects map[string][]byte
	types   map[string]string
}

type fakeAPIKeyResolver struct {
	secret string
	calls  []int64
	models []string
}

func (r *fakeAPIKeyResolver) Resolve(_ context.Context, _ *http.Request, _ model.CurrentPrincipal, _ string, keyID int64, modelName string) (string, error) {
	r.calls = append(r.calls, keyID)
	r.models = append(r.models, modelName)
	return r.secret, nil
}

func (r *fakeAPIKeyResolver) ResolveAny(_ context.Context, _ *http.Request, _ model.CurrentPrincipal, _ string, keyID int64) (string, error) {
	r.calls = append(r.calls, keyID)
	r.models = append(r.models, "")
	return r.secret, nil
}

func newMemoryMediaStorage() *memoryMediaStorage {
	return &memoryMediaStorage{objects: map[string][]byte{}, types: map[string]string{}}
}

func (s *memoryMediaStorage) Put(_ context.Context, key, contentType string, body io.Reader, _ int64) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.objects[key] = data
	s.types[key] = contentType
	return nil
}

func (s *memoryMediaStorage) Get(_ context.Context, key string) (*media.Object, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, media.ErrNotFound
	}
	return &media.Object{Body: io.NopCloser(bytes.NewReader(data)), ContentType: s.types[key], Size: int64(len(data))}, nil
}

func (s *memoryMediaStorage) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *memoryMediaStorage) PresignGet(_ context.Context, key string, _ time.Duration) (*url.URL, error) {
	return url.Parse("https://minio.example/" + key)
}

func TestGenerationService_OptimizePromptUsesSelectedKeyAndModel(t *testing.T) {
	var providerRequest struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer provider-secret" {
			t.Fatalf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&providerRequest); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A cinematic orange cat portrait, soft window light."}}]}`))
	}))
	defer upstream.Close()

	resolver := &fakeAPIKeyResolver{secret: "provider-secret"}
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), APIKeyResolver: resolver})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	response, err := svc.OptimizePromptWithRequest(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), principal, upstream.URL, OptimizePromptRequest{
		Prompt:   "orange cat",
		APIKeyID: 42,
		Model:    "gpt-5.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Prompt != "A cinematic orange cat portrait, soft window light." || response.Model != "gpt-5.1" {
		t.Fatalf("response = %#v", response)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != 42 || resolver.models[0] != "" {
		t.Fatalf("resolver calls = %#v models=%#v", resolver.calls, resolver.models)
	}
	if providerRequest.Model != "gpt-5.1" || len(providerRequest.Messages) != 2 || !strings.Contains(providerRequest.Messages[1].Content, "orange cat") {
		t.Fatalf("provider request = %#v", providerRequest)
	}
}

func TestGenerationService_GenerateVariantsUsesIndependentPrompts(t *testing.T) {
	var prompts []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		prompts = append(prompts, stringValue(payload["prompt"]))
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer upstream.Close()

	svc := NewGenerationService(nil, GenerationServiceOptions{HTTPClient: upstream.Client()})
	result, err := svc.generateWithProvider(context.Background(), upstream.URL, GenerateRequest{
		Prompt: "character", ProviderAPIKey: "key", Model: "gpt-image-1", Size: "1024x1024", ResponseFormat: "b64_json",
		Variants: []GenerateVariant{
			{Label: "正面", Prompt: "character front view"},
			{Label: "背面", Prompt: "character back view"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(prompts, []string{"character front view", "character back view"}) {
		t.Fatalf("prompts = %#v", prompts)
	}
	images := imageMapsValue(result["images"])
	if len(images) != 2 || images[0]["variant_label"] != "正面" || images[1]["variant_label"] != "背面" {
		t.Fatalf("images = %#v", images)
	}
}

func TestGenerationService_GPTVariantsPersistCompletedImagesWhilePending(t *testing.T) {
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	var releaseOnce sync.Once
	var requestCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := requestCount.Add(1)
		if request == 2 {
			close(secondStarted)
			<-releaseSecond
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer func() {
		releaseOnce.Do(func() { close(releaseSecond) })
		upstream.Close()
	}()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), MediaStorage: newMemoryMediaStorage()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "character", ProviderAPIKey: "provider-key", Model: "gpt-image-1",
		Variants: []GenerateVariant{{Label: "正面", Prompt: "front"}, {Label: "背面", Prompt: "back"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second GPT variant did not start")
	}

	pending, err := svc.Status(context.Background(), principal, upstream.URL, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	images := imageMapsValue(pending.Result["images"])
	if pending.Status != model.HistoryStatusPending || len(images) != 1 || images[0]["variant_label"] != "正面" {
		t.Fatalf("pending record = %#v", pending)
	}

	releaseOnce.Do(func() { close(releaseSecond) })
	deadline := time.Now().Add(time.Second)
	for pending.Status != model.HistoryStatusSucceeded && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		pending, err = svc.Status(context.Background(), principal, upstream.URL, created.JobID)
		if err != nil {
			t.Fatal(err)
		}
	}
	completedImages := imageMapsValue(pending.Result["images"])
	if pending.Status != model.HistoryStatusSucceeded || len(completedImages) != 2 {
		t.Fatalf("completed record = %#v", pending)
	}
	if completedImages[0]["url"] != apiBasePath+"/assets/"+created.JobID+"/result/0" || completedImages[1]["url"] != apiBasePath+"/assets/"+created.JobID+"/result/1" {
		t.Fatalf("completed image URLs = %#v", completedImages)
	}
}

func TestGenerationService_OptimizePromptFallsBackWhenProviderReturnsEmptyContent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer upstream.Close()

	resolver := &fakeAPIKeyResolver{secret: "provider-secret"}
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), APIKeyResolver: resolver})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	response, err := svc.OptimizePromptWithRequest(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), principal, upstream.URL, OptimizePromptRequest{
		Prompt: "一只橙色的猫", APIKeyID: 42, Model: "gpt-5.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Prompt, "一只橙色的猫") || !strings.Contains(response.Prompt, "构图") {
		t.Fatalf("fallback prompt = %q", response.Prompt)
	}
}

func TestExtractOptimizedPromptSupportsContentParts(t *testing.T) {
	prompt := extractOptimizedPrompt(map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"content": []any{
						map[string]any{"type": "text", "text": "Detailed cinematic cat prompt"},
					},
				},
			},
		},
	})
	if prompt != "Detailed cinematic cat prompt" {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestGenerationService_PromptModelsUsesSelectedKeyV1Models(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer provider-secret" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-image-2"},{"id":"gpt-5.1"},{"id":"sora-2"},{"id":"claude-sonnet-4-5"},{"id":"gpt-5.1"}]}`))
	}))
	defer upstream.Close()

	resolver := &fakeAPIKeyResolver{secret: "provider-secret"}
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), APIKeyResolver: resolver})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	response, err := svc.PromptModelsWithRequest(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), principal, upstream.URL, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := response.Models, []string{"gpt-5.1", "claude-sonnet-4-5"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != 42 || resolver.models[0] != "" {
		t.Fatalf("resolver calls = %#v models=%#v", resolver.calls, resolver.models)
	}
}

func TestGenerationService_ArchivesBase64Result(t *testing.T) {
	fixture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			fixture.Set(x, y, color.RGBA{R: 40, G: 100, B: 180, A: 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, fixture); err != nil {
		t.Fatal(err)
	}
	encodedPNG := base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + encodedPNG + `"}]}`))
	}))
	defer upstream.Close()

	storage := newMemoryMediaStorage()
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), MediaStorage: storage})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "cat", ProviderAPIKey: "key", Model: "gpt-image-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	record := waitForHistoryStatus(t, history, principal, created.JobID, model.HistoryStatusSucceeded)
	images := imageMapsValue(record.Result["images"])
	if len(images) != 1 || images[0]["object_key"] == "" {
		t.Fatalf("images = %#v", images)
	}
	if images[0]["b64_json"] != nil && images[0]["b64_json"] != "" {
		t.Fatalf("base64 persisted in history: %#v", images[0])
	}
	key := stringValue(images[0]["object_key"])
	original, _ := base64.StdEncoding.DecodeString(encodedPNG)
	if !bytes.Equal(storage.objects[key], original) {
		t.Fatal("stored original bytes changed")
	}
	previewKey := stringValue(images[0]["preview_object_key"])
	if previewKey == "" || len(storage.objects[previewKey]) == 0 {
		t.Fatalf("preview metadata = %#v", images[0])
	}
	if images[0]["preview_url"] == "" {
		t.Fatalf("preview URL missing: %#v", images[0])
	}
}

func TestGenerationService_RejectsTooManyReferenceImagesBeforeCreatingHistory(t *testing.T) {
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}

	response, err := svc.Generate(context.Background(), principal, "http://provider.example", GenerateRequest{
		Prompt:         "cat",
		ProviderAPIKey: "key",
		Model:          "gpt-image-1",
		ReferenceImages: []ReferenceImage{
			{Name: "first.png", DataURL: "data:image/png;base64,Zmlyc3Q="},
			{Name: "second.png", DataURL: "data:image/png;base64,c2Vjb25k"},
			{Name: "third.png", DataURL: "data:image/png;base64,dGhpcmQ="},
			{Name: "fourth.png", DataURL: "data:image/png;base64,Zm91cnRo"},
			{Name: "fifth.png", DataURL: "data:image/png;base64,ZmlmdGg="},
			{Name: "sixth.png", DataURL: "data:image/png;base64,c2l4dGg="},
			{Name: "seventh.png", DataURL: "data:image/png;base64,c2V2ZW50aA=="},
			{Name: "eighth.png", DataURL: "data:image/png;base64,ZWlnaHRo"},
			{Name: "ninth.png", DataURL: "data:image/png;base64,bmludGg="},
			{Name: "tenth.png", DataURL: "data:image/png;base64,dGVudGg="},
			{Name: "eleventh.png", DataURL: "data:image/png;base64,ZWxldmVudGg="},
			{Name: "twelfth.png", DataURL: "data:image/png;base64,dHdlbGZ0aA=="},
			{Name: "thirteenth.png", DataURL: "data:image/png;base64,dGhpcnRlZW50aA=="},
			{Name: "fourteenth.png", DataURL: "data:image/png;base64,Zm91cnRlZW50aA=="},
			{Name: "fifteenth.png", DataURL: "data:image/png;base64,ZmlmdGVlbnRo"},
			{Name: "sixteenth.png", DataURL: "data:image/png;base64,c2l4dGVlbnRo"},
			{Name: "seventeenth.png", DataURL: "data:image/png;base64,c2V2ZW50ZWVudGg="},
		},
	})
	if response != nil {
		t.Fatalf("response = %#v, want nil", response)
	}
	if !errors.Is(err, ErrTooManyReferenceImages) {
		t.Fatalf("error = %v, want ErrTooManyReferenceImages", err)
	}

	records, listErr := history.List(context.Background(), principal, model.HistoryQuery{Page: 1, PageSize: 20})
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(records) != 0 {
		t.Fatalf("history records = %d, want 0", len(records))
	}
}

func TestGenerationService_PersistsAPIKeyIDWithoutSecret(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer resolved-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"url":"https://cdn.example.com/image.png"}]}`))
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	resolver := &fakeAPIKeyResolver{secret: "resolved-secret"}
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), APIKeyResolver: resolver})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "cat", APIKeyID: 42, Model: "gpt-image-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	record := waitForHistoryStatus(t, history, principal, created.JobID, model.HistoryStatusSucceeded)
	if got := int64(intValue(record.Request["api_key_id"])); got != 42 {
		t.Fatalf("api_key_id = %d, want 42", got)
	}
	if _, exists := record.Request["provider_api_key"]; exists {
		t.Fatalf("history contains provider_api_key: %#v", record.Request)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != 42 {
		t.Fatalf("resolver calls = %#v", resolver.calls)
	}
}

func TestGenerationService_BatchLifecycleResolvesStoredAPIKeyID(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer resolved-secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches":
			_, _ = w.Write([]byte(`{"id":"batch-key-id","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/batch-key-id":
			_, _ = w.Write([]byte(`{"id":"batch-key-id","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/batch-key-id/items":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches/batch-key-id/cancel":
			_, _ = w.Write([]byte(`{"id":"batch-key-id","status":"cancelled"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	resolver := &fakeAPIKeyResolver{secret: "resolved-secret"}
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: server.Client(), APIKeyResolver: resolver})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(ctx, principal, server.URL, GenerateRequest{Prompt: "cat", APIKeyID: 42, Model: "gemini-2.5-flash-image"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Status(ctx, principal, server.URL, created.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(ctx, principal, server.URL, created.JobID); err != nil {
		t.Fatal(err)
	}
	if len(resolver.calls) != 3 {
		t.Fatalf("resolver calls = %#v, want submit/status/cancel", resolver.calls)
	}
}

func TestGenerationService_RetryResolvesStoredAPIKeyID(t *testing.T) {
	ctx := context.Background()
	var payloads []batchSubmitRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload batchSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, payload)
		_, _ = w.Write([]byte(`{"id":"batch-retry-id","status":"queued"}`))
	}))
	defer server.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	resolver := &fakeAPIKeyResolver{secret: "resolved-secret"}
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: server.Client(), APIKeyResolver: resolver})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(ctx, principal, server.URL, GenerateRequest{
		Prompt: "cat", APIKeyID: 42, Model: "gemini-2.5-flash-image", AspectRatio: "21:9", Resolution: "4K",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Retry(ctx, principal, server.URL, created.JobID); err != nil {
		t.Fatal(err)
	}
	if len(resolver.calls) != 2 {
		t.Fatalf("resolver calls = %#v, want submit and retry", resolver.calls)
	}
	if len(payloads) != 2 {
		t.Fatalf("payload count = %d, want 2", len(payloads))
	}
	for _, payload := range payloads {
		if payload.AspectRatio != "21:9" || payload.ImageSize != "4K" {
			t.Fatalf("batch dimensions = %q %q", payload.AspectRatio, payload.ImageSize)
		}
	}
}

func TestGenerationService_GPTCreatesLocalTaskAndReturnsResultFromStatus(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var requestCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			N int `json:"n"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.N != 1 {
			t.Fatalf("generation payload = %#v, err = %v", payload, err)
		}
		requestCount.Add(1)
		once.Do(func() { close(started) })
		<-release
		_, _ = w.Write([]byte(`{"created":1783000000,"data":[{"url":"https://cdn.example.com/gpt.png"}]}`))
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}

	response, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "draw a cat", ProviderAPIKey: "provider-key", Model: "gpt-image-1", OutputCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != model.HistoryStatusPending {
		t.Fatalf("status = %q, want pending", response.Status)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("GPT task did not start")
	}
	close(release)

	var record *model.HistoryRecord
	deadline := time.Now().Add(2 * time.Second)
	for {
		record, err = svc.Status(context.Background(), principal, upstream.URL, response.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status == model.HistoryStatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task status = %q", record.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	images := imageMapsValue(record.Result["images"])
	if requestCount.Load() != 3 || len(images) != 3 || images[0]["url"] != "https://cdn.example.com/gpt.png" {
		t.Fatalf("images = %#v", images)
	}
}

func TestGenerationService_CancelGPTLocalTaskKeepsCanceledStatus(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		close(stopped)
		return nil, r.Context().Err()
	})}

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: client})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(context.Background(), principal, "http://provider.example", GenerateRequest{
		Prompt: "cat", ProviderAPIKey: "provider-key", Model: "gpt-image-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	canceled, err := svc.Cancel(context.Background(), principal, "http://provider.example", created.JobID)
	if err != nil || canceled.Status != model.HistoryStatusCanceled {
		t.Fatalf("Cancel() = %#v, %v", canceled, err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("GPT provider request was not interrupted")
	}
	record, err := svc.Status(context.Background(), principal, "http://provider.example", created.JobID)
	if err != nil || record.Status != model.HistoryStatusCanceled {
		t.Fatalf("Status() = %#v, %v", record, err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestGenerationService_GenerateReferenceImageSubmitsBatch(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/batches" {
			t.Fatalf("request = %s %s, want POST /v1/images/batches", r.Method, r.URL.Path)
		}
		var payload batchSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Items) != 1 || len(payload.Items[0].ReferenceImages) != 1 {
			t.Fatalf("payload items = %#v", payload.Items)
		}
		ref := payload.Items[0].ReferenceImages[0]
		if ref.MimeType != "image/png" || string(ref.Data) != "png-bytes" {
			t.Fatalf("reference = %#v", ref)
		}
		_, _ = w.Write([]byte(`{"id":"imgbatch_reference","status":"queued","model":"gemini-2.5-flash-image"}`))
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}

	resp, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "use this reference", ProviderAPIKey: "provider-key", Model: "gemini-2.5-flash-image",
		ReferenceImages: []ReferenceImage{{
			Name: "reference.png", MimeType: "image/png", DataURL: "data:image/png;base64,cG5nLWJ5dGVz",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != model.HistoryStatusPending {
		t.Fatalf("status = %q, want pending", resp.Status)
	}
}

func TestGenerationService_PersistsReferenceWithoutBase64(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"imgbatch_reference","status":"queued"}`))
	}))
	defer upstream.Close()

	storage := newMemoryMediaStorage()
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), MediaStorage: storage})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "reference", ProviderAPIKey: "key", Model: "gemini-2.5-flash-image",
		ReferenceImages: []ReferenceImage{{Name: "reference.png", MimeType: "image/png", DataURL: "data:image/png;base64,cG5nLWJ5dGVz"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := history.Get(context.Background(), principal, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	references := referenceImagesValue(record.Request["reference_images"])
	if len(references) != 1 || references[0].StorageKey == "" {
		t.Fatalf("references = %#v", references)
	}
	if references[0].DataURL != "" {
		t.Fatal("reference data URL persisted in history")
	}
	if string(storage.objects[references[0].StorageKey]) != "png-bytes" {
		t.Fatalf("stored reference = %q", storage.objects[references[0].StorageKey])
	}
}

func TestGenerationService_UsesPreviouslyUploadedReference(t *testing.T) {
	fixture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var source bytes.Buffer
	if err := png.Encode(&source, fixture); err != nil {
		t.Fatal(err)
	}
	storage := newMemoryMediaStorage()
	storage.objects["image-generation/uploads/7/original.png"] = source.Bytes()
	storage.types["image-generation/uploads/7/original.png"] = "image/png"

	var received []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(maxPersistedImageBytes); err != nil {
			t.Fatal(err)
		}
		file, _, err := r.FormFile("image")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		received, err = io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		encoded := base64.StdEncoding.EncodeToString(source.Bytes())
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + encoded + `"}]}`))
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), MediaStorage: storage})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "edit", ProviderAPIKey: "key", Model: "gpt-image-1",
		ReferenceImages: []ReferenceImage{{
			Name: "reference.png", MimeType: "image/png",
			StorageKey:        "image-generation/uploads/7/original.png",
			PreviewStorageKey: "image-generation/uploads/7/preview.jpg",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForHistoryStatus(t, history, principal, created.JobID, model.HistoryStatusSucceeded)
	if !bytes.Equal(received, source.Bytes()) {
		t.Fatal("provider did not receive the uploaded original")
	}
	record, err := history.Get(context.Background(), principal, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	reference := referenceImagesValue(record.Request["reference_images"])[0]
	if reference.DataURL != "" || reference.StorageKey == "" || reference.PreviewStorageKey == "" {
		t.Fatalf("persisted reference = %#v", reference)
	}
}

func TestGenerationService_ReusesPreviouslyUploadedReference(t *testing.T) {
	fixture := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var source bytes.Buffer
	if err := png.Encode(&source, fixture); err != nil {
		t.Fatal(err)
	}
	storage := newMemoryMediaStorage()
	storage.objects["image-generation/uploads/7/upload-1/original"] = source.Bytes()
	storage.types["image-generation/uploads/7/upload-1/original"] = "image/png"
	storage.objects["image-generation/uploads/7/upload-1/preview"] = source.Bytes()
	storage.types["image-generation/uploads/7/upload-1/preview"] = "image/png"
	encoded := base64.StdEncoding.EncodeToString(source.Bytes())
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"` + encoded + `"}]}`))
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), MediaStorage: storage})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	for attempt := 0; attempt < 2; attempt++ {
		created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
			Prompt: "edit", ProviderAPIKey: "key", Model: "gpt-image-1",
			ReferenceImages: []ReferenceImage{{
				Name: "reference.png", MimeType: "image/png",
				StorageKey:        "image-generation/uploads/7/upload-1/original",
				PreviewStorageKey: "image-generation/uploads/7/upload-1/preview",
			}},
		})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt+1, err)
		}
		waitForHistoryStatus(t, history, principal, created.JobID, model.HistoryStatusSucceeded)
	}
}

func TestGenerationService_RejectsAnotherUsersUploadedReference(t *testing.T) {
	storage := newMemoryMediaStorage()
	storage.objects["image-generation/uploads/8/upload-1/original"] = []byte("private")
	storage.types["image-generation/uploads/8/upload-1/original"] = "image/png"
	svc := NewGenerationService(service.NewHistoryService(repository.NewHistoryRepository()), GenerationServiceOptions{MediaStorage: storage})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}

	_, _, _, err := svc.referenceImageBytes(context.Background(), principal, ReferenceImage{
		StorageKey: "image-generation/uploads/8/upload-1/original",
	})
	if err == nil {
		t.Fatal("another user's upload was accepted")
	}
}

func TestGenerationService_NewEditRequestUsesJSONForRemoteReferenceImage(t *testing.T) {
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{})
	req, err := svc.newEditRequest(context.Background(), "https://provider.example", GenerateRequest{
		Prompt: "restyle this image", Model: "gpt-image-1", Size: "1024x1024", ResponseFormat: "b64_json", OutputCount: 3,
		Quality: "high", OutputFormat: "webp", Background: "transparent", InputFidelity: "low",
		ReferenceImages: []ReferenceImage{{RemoteURL: "https://cdn.example.com/reference.png"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/v1/images/edits" {
		t.Fatalf("path = %q", req.URL.Path)
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q, want application/json", req.Header.Get("Content-Type"))
	}
	var payload struct {
		N             int    `json:"n"`
		Quality       string `json:"quality"`
		OutputFormat  string `json:"output_format"`
		Background    string `json:"background"`
		InputFidelity string `json:"input_fidelity"`
		Images        []struct {
			URL string `json:"image_url"`
		} `json:"images"`
	}
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.N != 3 || len(payload.Images) != 1 || payload.Images[0].URL != "https://cdn.example.com/reference.png" {
		t.Fatalf("images = %#v", payload.Images)
	}
	if payload.Quality != "high" || payload.OutputFormat != "webp" || payload.Background != "transparent" || payload.InputFidelity != "low" {
		t.Fatalf("advanced parameters = %#v", payload)
	}
}

func TestGenerationService_NewGenerationRequestForwardsAdvancedParameters(t *testing.T) {
	compression := 82
	svc := NewGenerationService(nil, GenerationServiceOptions{})
	req, err := svc.newGenerationRequest(context.Background(), "https://provider.example", GenerateRequest{
		Model: "gpt-image-1", Prompt: "cat", Size: "1024x1024", ResponseFormat: "b64_json", OutputCount: 1,
		Quality: "high", OutputFormat: "webp", OutputCompression: &compression, Background: "transparent",
	})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["quality"] != "high" || payload["output_format"] != "webp" || payload["background"] != "transparent" || payload["output_compression"] != float64(82) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestGenerationService_NewEditRequestForwardsMultipartParameters(t *testing.T) {
	compression := 76
	svc := NewGenerationService(nil, GenerationServiceOptions{})
	req, err := svc.newEditRequest(context.Background(), "https://provider.example", GenerateRequest{
		Model: "gpt-image-1", Prompt: "edit", Size: "1024x1024", ResponseFormat: "b64_json", OutputCount: 1,
		Quality: "medium", OutputFormat: "jpeg", OutputCompression: &compression, Background: "opaque", InputFidelity: "low",
		ReferenceImages: []ReferenceImage{{Name: "reference.png", MimeType: "image/png", DataURL: "data:image/png;base64,cG5n"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"quality": "medium", "output_format": "jpeg", "output_compression": "76", "background": "opaque", "input_fidelity": "low"}
	for name, expected := range want {
		if got := req.FormValue(name); got != expected {
			t.Fatalf("%s = %q, want %q", name, got, expected)
		}
	}
}

func TestGenerationService_StoredResultReferenceBecomesMultipartUpload(t *testing.T) {
	storage := newMemoryMediaStorage()
	storage.objects["result-key"] = []byte("stored-png")
	storage.types["result-key"] = "image/png"
	history := service.NewHistoryService(repository.NewHistoryRepository())
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	record, err := history.Create(context.Background(), principal, "source", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	record.Status = model.HistoryStatusSucceeded
	record.Result = map[string]any{"type": "image_generation", "images": []map[string]any{{"object_key": "result-key"}}}
	if err := history.Update(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	svc := NewGenerationService(history, GenerationServiceOptions{MediaStorage: storage})
	image := ReferenceImage{
		Name: "generated.png", MimeType: "image/png",
		DataURL: apiBasePath + "/assets/" + record.ID + "/result/0?token=secret",
	}
	name, contentType, data, err := svc.referenceImageBytes(context.Background(), principal, image)
	if err != nil {
		t.Fatal(err)
	}
	request, err := svc.newEditRequest(context.Background(), "https://provider.example", GenerateRequest{
		Prompt: "edit", Model: "gpt-image-1", Size: "1024x1024", ResponseFormat: "b64_json",
		ReferenceImages: []ReferenceImage{{Name: name, MimeType: contentType, DataURL: "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "multipart/form-data;") {
		t.Fatalf("content type = %q", request.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("stored-png")) {
		t.Fatalf("multipart body does not contain stored image bytes")
	}
}

func TestGenerationService_GenerateWithStoredResultReferenceUploadsMultipart(t *testing.T) {
	storage := newMemoryMediaStorage()
	storage.objects["result-key"] = []byte("stored-png")
	storage.types["result-key"] = "image/png"
	history := service.NewHistoryService(repository.NewHistoryRepository())
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	source, err := history.Create(context.Background(), principal, "source", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	source.Status = model.HistoryStatusSucceeded
	source.Result = map[string]any{"type": "image_generation", "images": []map[string]any{{"object_key": "result-key"}}}
	if err := history.Update(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte("stored-png")) {
			t.Fatal("multipart body does not contain stored image")
		}
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"bmV3LWltYWdl"}]}`))
	}))
	defer upstream.Close()
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client(), MediaStorage: storage})
	created, err := svc.Generate(context.Background(), principal, upstream.URL, GenerateRequest{
		Prompt: "edit", ProviderAPIKey: "key", Model: "gpt-image-1",
		ReferenceImages: []ReferenceImage{{
			Name: "generated.png", MimeType: "image/png",
			DataURL: apiBasePath + "/assets/" + source.ID + "/result/0?token=secret",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForHistoryStatus(t, history, principal, created.JobID, model.HistoryStatusSucceeded)
}

func TestGenerationService_GenerateSubmitsSingleBatch(t *testing.T) {
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/batches" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var payload batchSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Items) != 1 || payload.Items[0].OutputCount != 3 {
			t.Fatalf("payload = %#v", payload)
		}
		_, _ = w.Write([]byte(`{"id":"imgbatch_async","status":"queued","model":"gemini-2.5-flash-image"}`))
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Email: "user@example.com", Plugin: "image-generation"}

	resp, err := svc.Generate(ctx, principal, upstream.URL, GenerateRequest{
		OutputCount: 3,
		Prompt:      "draw a cat", ProviderAPIKey: "api-key", Model: "gemini-2.5-flash-image",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != model.HistoryStatusPending || resp.Result["batch_status"] != "queued" {
		t.Fatalf("response = %#v", resp)
	}
	record, err := history.Get(ctx, principal, resp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Request["batch_id"] != "imgbatch_async" {
		t.Fatalf("batch_id = %#v", record.Request["batch_id"])
	}
	if record.Request["batch_custom_id"] != "plugin-image-"+record.ID {
		t.Fatalf("batch_custom_id = %#v", record.Request["batch_custom_id"])
	}
}

func TestGenerationService_ReconcileCompletedBatch(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches":
			_, _ = w.Write([]byte(`{"id":"imgbatch_done","status":"queued","model":"gemini-2.5-flash-image"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_done":
			_, _ = w.Write([]byte(`{"id":"imgbatch_done","status":"completed","model":"gemini-2.5-flash-image"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_done/items":
			_, _ = w.Write([]byte(`{"data":[{"custom_id":"plugin-image-placeholder","status":"completed","image_count":2}]}`))
		default:
			if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/content") {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write([]byte("png"))
				return
			}
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: server.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(ctx, principal, server.URL, GenerateRequest{Prompt: "cat", ProviderAPIKey: "api-key", Model: "gemini-2.5-flash-image"})
	if err != nil {
		t.Fatal(err)
	}
	record, _ := history.Get(ctx, principal, created.JobID)
	// Make the fake item match the generated stable custom ID without coupling the server to repository internals.
	record.Request["batch_custom_id"] = "plugin-image-placeholder"
	_ = history.Update(ctx, record)
	completed, err := svc.Status(ctx, principal, server.URL, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != model.HistoryStatusSucceeded {
		t.Fatalf("status = %q", completed.Status)
	}
	images := imageMapsValue(completed.Result["images"])
	if len(images) != 2 || images[0]["url"] != "data:image/png;base64,cG5n" {
		t.Fatalf("images = %#v", images)
	}
}

func TestGenerationService_BatchPersistsCompletedItemsWhilePending(t *testing.T) {
	ctx := context.Background()
	var contentCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches":
			_, _ = w.Write([]byte(`{"id":"batch-progress","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/batch-progress":
			_, _ = w.Write([]byte(`{"id":"batch-progress","status":"running"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/batch-progress/items":
			_, _ = w.Write([]byte(`{"data":[{"custom_id":"variant-front","status":"completed","image_count":1},{"custom_id":"variant-back","status":"pending","image_count":0}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/batch-progress/items/variant-front/content":
			contentCalls.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("front"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: upstream.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(ctx, principal, upstream.URL, GenerateRequest{
		Prompt: "character", ProviderAPIKey: "api-key", Model: "gemini-2.5-flash-image",
		Variants: []GenerateVariant{{Label: "正面", Prompt: "front"}, {Label: "背面", Prompt: "back"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := history.Get(ctx, principal, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	record.Request["batch_variant_labels"] = map[string]string{"variant-front": "正面", "variant-back": "背面"}
	if err := history.Update(ctx, record); err != nil {
		t.Fatal(err)
	}

	pending, err := svc.Status(ctx, principal, upstream.URL, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	images := imageMapsValue(pending.Result["images"])
	if pending.Status != model.HistoryStatusPending || len(images) != 1 || images[0]["variant_label"] != "正面" {
		t.Fatalf("pending record = %#v", pending)
	}
	if _, err := svc.Status(ctx, principal, upstream.URL, created.JobID); err != nil {
		t.Fatal(err)
	}
	if contentCalls.Load() != 1 {
		t.Fatalf("completed item content calls = %d, want 1", contentCalls.Load())
	}
}

func TestGenerationService_BatchVariantsKeepSuccessfulImages(t *testing.T) {

	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches":
			_, _ = w.Write([]byte(`{"id":"imgbatch_variants","status":"queued"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_variants":
			_, _ = w.Write([]byte(`{"id":"imgbatch_variants","status":"completed"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_variants/items":
			_, _ = w.Write([]byte(`{"data":[{"custom_id":"variant-front","status":"completed","image_count":1},{"custom_id":"variant-back","status":"failed","image_count":0}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_variants/items/variant-front/content":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("png"))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: server.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(ctx, principal, server.URL, GenerateRequest{
		Prompt: "character", ProviderAPIKey: "api-key", Model: "gemini-2.5-flash-image",
		Variants: []GenerateVariant{{Label: "正面", Prompt: "front"}, {Label: "背面", Prompt: "back"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := history.Get(ctx, principal, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	record.Request["batch_variant_labels"] = map[string]string{"variant-front": "正面", "variant-back": "背面"}
	_ = history.Update(ctx, record)

	completed, err := svc.Status(ctx, principal, server.URL, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != model.HistoryStatusSucceeded {
		t.Fatalf("status = %q", completed.Status)
	}
	images := imageMapsValue(completed.Result["images"])
	if len(images) != 1 || images[0]["variant_label"] != "正面" {
		t.Fatalf("images = %#v", images)
	}
	failed, ok := completed.Result["failed_variants"].([]string)
	if !ok || !reflect.DeepEqual(failed, []string{"背面"}) {
		t.Fatalf("failed_variants = %#v", completed.Result["failed_variants"])
	}
}

func TestGenerationService_CancelBatch(t *testing.T) {
	ctx := context.Background()
	cancelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches":
			_, _ = w.Write([]byte(`{"id":"imgbatch_cancel","status":"queued"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches/imgbatch_cancel/cancel":
			cancelCalls++
			_, _ = w.Write([]byte(`{"id":"imgbatch_cancel","status":"cancelled"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	history := service.NewHistoryService(repository.NewHistoryRepository())
	svc := NewGenerationService(history, GenerationServiceOptions{HTTPClient: server.Client()})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Plugin: "image-generation"}
	created, err := svc.Generate(ctx, principal, server.URL, GenerateRequest{Prompt: "cat", ProviderAPIKey: "api-key", Model: "gemini-2.5-flash-image"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.Cancel(ctx, principal, server.URL, created.JobID)
	if err != nil || cancelled.Status != model.HistoryStatusCanceled || cancelCalls != 1 {
		t.Fatalf("Cancel() = %#v, %v, calls=%d", cancelled, err, cancelCalls)
	}
}

func TestGenerationService_ListCreationsForAdminAndUser(t *testing.T) {
	ctx := context.Background()
	historyRepo := repository.NewHistoryRepository()
	history := service.NewHistoryService(historyRepo)
	svc := NewGenerationService(history, GenerationServiceOptions{})

	userA := model.CurrentPrincipal{UserID: 1, Role: model.RoleUser, Email: "a@example.com", Username: "a", Plugin: "gen"}
	userB := model.CurrentPrincipal{UserID: 2, Role: model.RoleUser, Email: "b@example.com", Username: "b", Plugin: "gen"}
	admin := model.CurrentPrincipal{UserID: 99, Role: model.RoleAdmin, Email: "admin@example.com", Username: "admin", Plugin: "gen"}

	createSucceededImageHistory(t, history, userA, "first image", "https://cdn.example.com/a.png")
	createSucceededImageHistory(t, history, userB, "second image", "https://cdn.example.com/b.png")

	userCreations, err := svc.ListCreations(ctx, userA, model.HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(userCreations) != 1 {
		t.Fatalf("user creation count = %d, want 1", len(userCreations))
	}
	if userCreations[0].UserID != userA.UserID {
		t.Fatalf("user creation user_id = %d, want %d", userCreations[0].UserID, userA.UserID)
	}

	adminCreations, err := svc.ListCreations(ctx, admin, model.HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(adminCreations) != 2 {
		t.Fatalf("admin creation count = %d, want 2", len(adminCreations))
	}
}

func TestGenerationService_RetryUsesStoredPromptWhileHistoryKeepsDisplayPrompt(t *testing.T) {
	ctx := context.Background()
	var prompts []string
	var promptsMu sync.Mutex

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload batchSubmitRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		if len(payload.Items) != 1 {
			t.Fatalf("batch items = %#v", payload.Items)
		}
		promptsMu.Lock()
		prompts = append(prompts, payload.Items[0].Prompt)
		batchNumber := len(prompts)
		promptsMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"imgbatch_retry_` + strconv.Itoa(batchNumber) + `","status":"queued","model":"gemini-2.5-flash-image"}`))
	}))
	defer upstream.Close()

	historyRepo := repository.NewHistoryRepository()
	history := service.NewHistoryService(historyRepo)
	resolver := &fakeAPIKeyResolver{secret: "provider-secret"}
	svc := NewGenerationService(history, GenerationServiceOptions{APIKeyResolver: resolver})

	principal := model.CurrentPrincipal{
		UserID:   7,
		Role:     model.RoleUser,
		Email:    "user@example.com",
		Username: "user",
		Plugin:   "image-generation",
	}

	resp, err := svc.Generate(ctx, principal, upstream.URL, GenerateRequest{
		Prompt:   "Follow the user request.\nUser request: draw a camera",
		APIKeyID: 42,
		Model:    "gemini-2.5-flash-image",
		Inputs: map[string]any{
			"display_prompt": "draw a camera",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	record, err := history.Get(ctx, principal, resp.JobID)
	if err != nil {
		t.Fatal(err)
	}

	if record.Prompt != "draw a camera" {
		t.Fatalf("history prompt = %q, want %q", record.Prompt, "draw a camera")
	}
	if got := record.Request["prompt"]; got != "Follow the user request.\nUser request: draw a camera" {
		t.Fatalf("stored request prompt = %#v", got)
	}
	if record.ConversationID != "" {
		t.Fatalf("conversation id = %q, want empty", record.ConversationID)
	}
	retry, err := svc.Retry(ctx, principal, upstream.URL, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retry.JobID == record.ID {
		t.Fatal("retry reused the original history id")
	}
	retryWithGroup, err := svc.RetryWithRequest(ctx, nil, principal, upstream.URL, record.ID, "retry-group-1")
	if err != nil {
		t.Fatal(err)
	}
	retryRecord, err := history.Get(ctx, principal, retryWithGroup.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if retryRecord.Request["generation_group_id"] != "retry-group-1" {
		t.Fatalf("retry generation group = %#v, want retry-group-1", retryRecord.Request["generation_group_id"])
	}
	if retryRecord.Request["prompt"] != record.Request["prompt"] {
		t.Fatalf("retry prompt = %#v, want %#v", retryRecord.Request["prompt"], record.Request["prompt"])
	}

	promptsMu.Lock()
	defer promptsMu.Unlock()
	if len(prompts) != 3 {
		t.Fatalf("provider prompt count = %d, want 3", len(prompts))
	}
	if prompts[0] != "Follow the user request.\nUser request: draw a camera" {
		t.Fatalf("first provider prompt = %q", prompts[0])
	}
	if prompts[1] != "Follow the user request.\nUser request: draw a camera" {
		t.Fatalf("retry provider prompt = %q", prompts[1])
	}
	if prompts[2] != "Follow the user request.\nUser request: draw a camera" {
		t.Fatalf("grouped retry provider prompt = %q", prompts[2])
	}
}

func TestGenerationService_GenerateStoresConversationID(t *testing.T) {
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"imgbatch_conversation","status":"queued","model":"gemini-2.5-flash-image"}`))
	}))
	defer upstream.Close()

	historyRepo := repository.NewHistoryRepository()
	history := service.NewHistoryService(historyRepo)
	svc := NewGenerationService(history, GenerationServiceOptions{})
	principal := model.CurrentPrincipal{UserID: 7, Role: model.RoleUser, Email: "user@example.com", Username: "user", Plugin: "image-generation"}

	resp, err := svc.Generate(ctx, principal, upstream.URL, GenerateRequest{
		Prompt:         "draw a camera",
		ProviderAPIKey: "provider-secret",
		Model:          "gemini-2.5-flash-image",
		Inputs: map[string]any{
			"conversation_id": "conversation-live-123",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	record, err := history.Get(ctx, principal, resp.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if record.ConversationID != "conversation-live-123" {
		t.Fatalf("conversation id = %q, want %q", record.ConversationID, "conversation-live-123")
	}
}

func TestGenerationServiceGenerateRequiresProviderBaseURL(t *testing.T) {
	ctx := context.Background()
	historyRepo := repository.NewHistoryRepository()
	history := service.NewHistoryService(historyRepo)
	svc := NewGenerationService(history, GenerationServiceOptions{})
	principal := model.CurrentPrincipal{
		UserID:   7,
		Role:     model.RoleUser,
		Email:    "user@example.com",
		Username: "user",
		Plugin:   "image-generation",
	}

	_, err := svc.Generate(ctx, principal, "", GenerateRequest{
		Prompt:         "draw a city",
		ProviderAPIKey: "provider-key",
	})
	if !errors.Is(err, ErrProviderBaseURL) {
		t.Fatalf("Generate() err = %v, want ErrProviderBaseURL", err)
	}
}

func TestGenerationService_GenerateReturnsUpstreamStatusAndMessage(t *testing.T) {
	ctx := context.Background()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`))
	}))
	defer upstream.Close()

	historyRepo := repository.NewHistoryRepository()
	history := service.NewHistoryService(historyRepo)
	svc := NewGenerationService(history, GenerationServiceOptions{})
	principal := model.CurrentPrincipal{
		UserID:   7,
		Role:     model.RoleUser,
		Email:    "user@example.com",
		Username: "user",
		Plugin:   "image-generation",
	}

	resp, err := svc.Generate(ctx, principal, upstream.URL, GenerateRequest{
		Prompt:         "draw a city",
		ProviderAPIKey: "provider-key",
		Model:          "gemini-2.5-flash-image",
	})
	if resp != nil {
		t.Fatalf("Generate() response = %#v, want nil", resp)
	}
	var upstreamErr *UpstreamHTTPError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Generate() err = %v, want upstream 401", err)
	}

	records, listErr := history.List(ctx, principal, model.HistoryQuery{})
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(records) != 1 {
		t.Fatalf("history records = %d, want 1", len(records))
	}
	if records[0].Status != model.HistoryStatusFailed {
		t.Fatalf("history status = %q, want %q", records[0].Status, model.HistoryStatusFailed)
	}
	if records[0].ErrorMessage != "Invalid API key" {
		t.Fatalf("history error message = %q, want %q", records[0].ErrorMessage, "Invalid API key")
	}
}

func createSucceededImageHistory(t *testing.T, history *service.HistoryService, principal model.CurrentPrincipal, prompt, imageURL string) {
	t.Helper()
	record, err := history.Create(context.Background(), principal, prompt, map[string]any{
		"prompt": prompt,
		"model":  "gemini-2.5-flash-image",
		"size":   "1024x1024",
	})
	if err != nil {
		t.Fatal(err)
	}
	record.Status = model.HistoryStatusSucceeded
	record.Result = map[string]any{
		"type":  "image_generation",
		"model": "gemini-2.5-flash-image",
		"size":  "1024x1024",
		"images": []map[string]any{{
			"url": imageURL,
		}},
	}
	if err := history.Update(context.Background(), record); err != nil {
		t.Fatal(err)
	}
}

func waitForHistoryStatus(t *testing.T, history *service.HistoryService, principal model.CurrentPrincipal, id, expected string) *model.HistoryRecord {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		record, err := history.Get(context.Background(), principal, id)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status == expected {
			return record
		}
		if isTerminalHistoryStatus(record.Status) && record.Status != expected {
			t.Fatalf("history status = %q, want %q; error=%q", record.Status, expected, record.ErrorMessage)
		}
		if time.Now().After(deadline) {
			t.Fatalf("history status = %q, want %q", record.Status, expected)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
