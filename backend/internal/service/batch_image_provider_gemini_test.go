//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestBatchImageProviderRegistry_ReturnsGeminiAPI(t *testing.T) {
	registry := NewDefaultBatchImageProviderRegistry()
	provider, ok := registry.Get(BatchImageProviderGeminiAPI)
	require.True(t, ok)
	require.Equal(t, BatchImageProviderGeminiAPI, provider.Name())

	must, err := registry.MustGet(BatchImageProviderGeminiAPI)
	require.NoError(t, err)
	require.Same(t, provider, must)

	_, err = registry.MustGet("unknown_provider")
	require.ErrorIs(t, err, ErrBatchImageInvalidProvider)
}

func TestGeminiProvider_SupportsOnlyGeminiAPIKeyWithSecret(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{})

	require.True(t, provider.SupportsAccount(geminiAPIKeyAccount("sk-gemini")))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{}}))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"api_key": "sk"}}))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}))
	require.False(t, provider.SupportsAccount(nil))
}

func TestGeminiProvider_MissingAPIKeyRejected(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{})
	_, err := provider.Submit(context.Background(), nil, &Account{Platform: PlatformGemini, Type: AccountTypeAPIKey}, validGeminiBatchInput())
	require.ErrorIs(t, err, ErrBatchImageProviderMissingAPIKey)
}

func TestBuildGeminiBatchJSONL_WritesValidLinesAndPreservesCustomID(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_002", Prompt: "Second prompt"})

	jsonl, err := BuildGeminiBatchJSONL(input)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	require.Len(t, lines, 2)
	requireJSONLLine(t, lines[0], "cover_001", "A clean product hero image")
	requireJSONLLine(t, lines[1], "cover_002", "Second prompt")
}

func TestBuildGeminiBatchJSONL_RejectsDuplicateCustomIDs(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_001", Prompt: "Duplicate"})

	_, err := BuildGeminiBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
}

func TestBuildGeminiBatchJSONL_RejectsEmptyPrompt(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items[0].Prompt = " "

	_, err := BuildGeminiBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
}

func TestBuildGeminiBatchJSONL_WritesReferenceImages(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items[0].ReferenceImages = []BatchImageReference{
		{MimeType: "image/webp", Data: []byte("webp-bytes")},
		{MimeType: "image/jpeg", FileURI: "gs://bucket/refs/style.jpg"},
	}

	jsonl, err := BuildGeminiBatchJSONL(input)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	require.Len(t, lines, 1)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &got))
	request := got["request"].(map[string]any)
	contents := request["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	require.Len(t, parts, 3)
	require.Equal(t, "A clean product hero image", parts[0].(map[string]any)["text"])
	inlineData := parts[1].(map[string]any)["inlineData"].(map[string]any)
	require.Equal(t, "image/webp", inlineData["mimeType"])
	require.Equal(t, "d2VicC1ieXRlcw==", inlineData["data"])
	fileData := parts[2].(map[string]any)["fileData"].(map[string]any)
	require.Equal(t, "image/jpeg", fileData["mimeType"])
	require.Equal(t, "gs://bucket/refs/style.jpg", fileData["fileUri"])
}

func TestGeminiProvider_SubmitUploadsJSONLThenCreatesBatch(t *testing.T) {
	client := &fakeGeminiBatchClient{
		uploaded: &GeminiUploadedFile{Name: "files/input-jsonl"},
		created:  &GeminiBatchJob{Name: "batches/job-123", State: "JOB_STATE_PENDING"},
	}
	provider := NewGeminiAPIBatchImageProvider(client)

	got, err := provider.Submit(context.Background(), &BatchImageJob{BatchID: "imgbatch_123", Model: "gemini-3.1-flash-image"}, geminiAPIKeyAccount("sk-secret"), validGeminiBatchInput())
	require.NoError(t, err)
	require.Equal(t, []string{"upload", "create"}, client.calls)
	require.Equal(t, "files/input-jsonl", got.ProviderInputRef)
	require.Equal(t, "batches/job-123", got.ProviderJobName)
	require.Empty(t, got.ProviderOutputRef)
	require.NotContains(t, got.ProviderInputRef, "A clean product hero image")
	require.NotContains(t, string(client.uploadedJSONL), "sk-secret")
}

func TestGeminiProvider_GetMapsStates(t *testing.T) {
	tests := []struct {
		name      string
		job       *GeminiBatchJob
		wantState BatchProviderInternalState
		wantDone  bool
		wantRef   string
		wantCode  string
	}{
		{name: "running", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_RUNNING"}, wantState: BatchProviderStateRunning},
		{name: "succeeded_dest_fileName", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_SUCCEEDED", Dest: &GeminiBatchDest{FileName: "files/out"}}, wantState: BatchProviderStateSucceeded, wantDone: true, wantRef: "files/out"},
		{name: "failed", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_FAILED", Error: &GeminiBatchError{Code: "BAD_PROMPT", Message: "bad prompt"}}, wantState: BatchProviderStateFailed, wantDone: true, wantCode: "BAD_PROMPT"},
		{name: "cancelled", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_CANCELLED"}, wantState: BatchProviderStateCancelled, wantDone: true, wantCode: "GEMINI_BATCH_CANCELLED"},
		{name: "expired", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_EXPIRED"}, wantState: BatchProviderStateExpired, wantDone: true, wantCode: "GEMINI_BATCH_EXPIRED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{got: tt.job})
			got, err := provider.Get(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret"))
			require.NoError(t, err)
			require.Equal(t, tt.wantState, got.InternalState)
			require.Equal(t, tt.wantDone, got.Done)
			require.Equal(t, tt.wantRef, got.ProviderOutputRef)
			require.Equal(t, tt.wantCode, got.ErrorCode)
			require.NotContains(t, got.ErrorMessage, "sk-secret")
		})
	}
}

func TestGeminiProvider_GetExtractsResponsesFileReference(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{
		got: &GeminiBatchJob{
			Name:     "batches/1",
			State:    "JOB_STATE_SUCCEEDED",
			Response: &GeminiBatchResponse{ResponsesFile: "files/responses-jsonl"},
		},
	})

	got, err := provider.Get(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret"))
	require.NoError(t, err)
	require.Equal(t, BatchProviderStateSucceeded, got.InternalState)
	require.Equal(t, "files/responses-jsonl", got.ProviderOutputRef)
}

func TestGeminiProvider_GetRejectsInlineResultShape(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{
		got: &GeminiBatchJob{
			Name:     "batches/1",
			State:    "JOB_STATE_SUCCEEDED",
			Response: &GeminiBatchResponse{InlinedResponses: []any{map[string]any{"response": "large"}}},
		},
	})

	_, err := provider.Get(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret"))
	require.ErrorIs(t, err, ErrBatchImageProviderInlineResultUnsupported)
}

func TestGeminiProvider_OpenResultStreamsResultFile(t *testing.T) {
	client := &fakeGeminiBatchClient{downloadBody: "line1\n", downloadContentType: "application/jsonl"}
	provider := NewGeminiAPIBatchImageProvider(client)

	outputRef := "files/output-jsonl"
	r, contentType, err := provider.OpenResult(context.Background(), &BatchImageJob{ProviderOutputRef: &outputRef}, geminiAPIKeyAccount("sk-secret"))
	require.NoError(t, err)
	defer r.Close()

	body, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "line1\n", string(body))
	require.Equal(t, "application/jsonl", contentType)
	require.Equal(t, "files/output-jsonl", client.downloadedFile)
}

func TestGeminiProvider_CancelCallsClient(t *testing.T) {
	client := &fakeGeminiBatchClient{}
	provider := NewGeminiAPIBatchImageProvider(client)

	require.NoError(t, provider.Cancel(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret")))
	require.Equal(t, "batches/1", client.cancelledBatch)
}

func TestGeminiProvider_CleanupDeletesRefsOnlyWhenPresent(t *testing.T) {
	inputRef := "files/input"
	outputRef := "files/output"
	client := &fakeGeminiBatchClient{}
	provider := NewGeminiAPIBatchImageProvider(client)

	err := provider.Cleanup(context.Background(), &BatchImageJob{ProviderInputRef: &inputRef, ProviderOutputRef: &outputRef}, geminiAPIKeyAccount("sk-secret"), CleanupTargetAll)
	require.NoError(t, err)
	require.Equal(t, []string{"files/input", "files/output"}, client.deletedFiles)

	err = provider.Cleanup(context.Background(), &BatchImageJob{}, geminiAPIKeyAccount("sk-secret"), CleanupTargetAll)
	require.NoError(t, err)
	require.Equal(t, []string{"files/input", "files/output"}, client.deletedFiles)
}

func TestGeminiProvider_ErrorsDoNotExposeAPIKey(t *testing.T) {
	apiKey := "sk-top-secret"
	client := &fakeGeminiBatchClient{uploadErr: &GeminiAPIError{StatusCode: 401, Message: "upstream body should be hidden " + apiKey}}
	provider := NewGeminiAPIBatchImageProvider(client)

	_, err := provider.Submit(context.Background(), nil, geminiAPIKeyAccount(apiKey), validGeminiBatchInput())
	require.Error(t, err)
	require.Equal(t, "GEMINI_AUTH_FAILED", infraerrors.Reason(err))
	require.NotContains(t, err.Error(), apiKey)
}

func TestGeminiProvider_MetadataDoesNotStoreImageBytesOrBase64(t *testing.T) {
	client := &fakeGeminiBatchClient{
		uploaded: &GeminiUploadedFile{Name: "files/input-jsonl"},
		created:  &GeminiBatchJob{Name: "batches/job-123", State: "JOB_STATE_PENDING"},
	}
	provider := NewGeminiAPIBatchImageProvider(client)

	got, err := provider.Submit(context.Background(), nil, geminiAPIKeyAccount("sk-secret"), validGeminiBatchInput())
	require.NoError(t, err)
	require.NotContains(t, got.ProviderJobName, "base64")
	require.NotContains(t, got.ProviderInputRef, "base64")
	require.NotContains(t, got.ProviderOutputRef, "base64")
	require.NotContains(t, got.ProviderJobName+got.ProviderInputRef+got.ProviderOutputRef, "iVBOR")
	require.NotContains(t, got.ProviderJobName+got.ProviderInputRef+got.ProviderOutputRef, "A clean product hero image")
}

func requireJSONLLine(t *testing.T, line, wantKey, wantPrompt string) {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got))
	require.Equal(t, wantKey, got["key"])
	request := got["request"].(map[string]any)
	config := request["generationConfig"].(map[string]any)
	require.Equal(t, []any{"TEXT", "IMAGE"}, config["responseModalities"])
	contents := request["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	require.Equal(t, wantPrompt, parts[0].(map[string]any)["text"])
}

func validGeminiBatchInput() BatchImageInput {
	return BatchImageInput{
		BatchID:     "imgbatch_123",
		Model:       "gemini-3.1-flash-image",
		DisplayName: "test batch",
		Items: []BatchImageInputItem{{
			CustomID: "cover_001",
			Prompt:   "A clean product hero image",
		}},
	}
}

func geminiAPIKeyAccount(apiKey string) *Account {
	return &Account{
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": apiKey},
	}
}

func jobWithProviderName(name string) *BatchImageJob {
	return &BatchImageJob{ProviderJobName: &name}
}

type fakeGeminiBatchClient struct {
	calls               []string
	uploaded            *GeminiUploadedFile
	created             *GeminiBatchJob
	got                 *GeminiBatchJob
	uploadErr           error
	createErr           error
	getErr              error
	cancelErr           error
	downloadErr         error
	deleteErr           error
	uploadedJSONL       []byte
	createdFile         string
	cancelledBatch      string
	downloadedFile      string
	downloadBody        string
	downloadContentType string
	deletedFiles        []string
}

func (f *fakeGeminiBatchClient) UploadJSONL(_ context.Context, apiKey string, _ string, r io.Reader) (*GeminiUploadedFile, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("missing api key")
	}
	f.calls = append(f.calls, "upload")
	f.uploadedJSONL, _ = io.ReadAll(r)
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	if f.uploaded != nil {
		return f.uploaded, nil
	}
	return &GeminiUploadedFile{Name: "files/input-jsonl"}, nil
}

func (f *fakeGeminiBatchClient) CreateBatch(_ context.Context, _ string, _ string, fileName string, _ string) (*GeminiBatchJob, error) {
	f.calls = append(f.calls, "create")
	f.createdFile = fileName
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.created != nil {
		return f.created, nil
	}
	return &GeminiBatchJob{Name: "batches/job-123", State: "JOB_STATE_PENDING"}, nil
}

func (f *fakeGeminiBatchClient) GetBatch(_ context.Context, _ string, _ string) (*GeminiBatchJob, error) {
	f.calls = append(f.calls, "get")
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.got, nil
}

func (f *fakeGeminiBatchClient) CancelBatch(_ context.Context, _ string, batchName string) error {
	f.calls = append(f.calls, "cancel")
	f.cancelledBatch = batchName
	return f.cancelErr
}

func (f *fakeGeminiBatchClient) DownloadFile(_ context.Context, _ string, fileName string) (io.ReadCloser, string, error) {
	f.calls = append(f.calls, "download")
	f.downloadedFile = fileName
	if f.downloadErr != nil {
		return nil, "", f.downloadErr
	}
	contentType := f.downloadContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return io.NopCloser(bytes.NewBufferString(f.downloadBody)), contentType, nil
}

func (f *fakeGeminiBatchClient) DeleteFile(_ context.Context, _ string, fileName string) error {
	f.calls = append(f.calls, "delete")
	f.deletedFiles = append(f.deletedFiles, fileName)
	return f.deleteErr
}
