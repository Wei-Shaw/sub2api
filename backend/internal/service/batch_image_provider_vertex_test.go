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
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestBatchImageProviderRegistry_ReturnsVertex(t *testing.T) {
	registry := NewDefaultBatchImageProviderRegistry()
	provider, ok := registry.Get(BatchImageProviderVertex)
	require.True(t, ok)
	require.Equal(t, BatchImageProviderVertex, provider.Name())
}

func TestVertexProvider_SupportsOnlyGeminiServiceAccount(t *testing.T) {
	provider := newTestVertexProvider(&fakeVertexBatchClient{}, &fakeVertexObjectStore{})

	require.True(t, provider.SupportsAccount(vertexServiceAccount()))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}}))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"}}))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount, Credentials: vertexServiceAccount().Credentials}))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{}}))
}

func TestVertexProvider_MissingServiceAccountRejected(t *testing.T) {
	provider := newTestVertexProvider(&fakeVertexBatchClient{}, &fakeVertexObjectStore{})
	_, err := provider.Submit(context.Background(), nil, &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{}}, validVertexBatchInput())
	require.ErrorIs(t, err, ErrBatchImageProviderMissingServiceAccount)
}

func TestVertexProvider_MissingManagedGCSBucketRejected(t *testing.T) {
	provider := NewVertexBatchImageProvider(VertexBatchImageProviderOptions{ProjectID: "proj", Environment: "test"}, &fakeVertexBatchClient{}, &fakeVertexObjectStore{}, &fakeGeminiTokenCache{token: "token"})
	_, err := provider.Submit(context.Background(), nil, vertexServiceAccount(), validVertexBatchInput())
	require.Error(t, err)
	require.Equal(t, "VERTEX_MANAGED_GCS_BUCKET_MISSING", infraerrors.Reason(err))
}

func TestBuildVertexBatchJSONL_WritesValidLinesAndPreservesCustomID(t *testing.T) {
	input := validVertexBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_002", Prompt: "Second prompt"})

	jsonl, err := BuildVertexBatchJSONL(input)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	require.Len(t, lines, 2)
	requireVertexJSONLLine(t, lines[0], "cover_001", "A clean product hero image")
	requireVertexJSONLLine(t, lines[1], "cover_002", "Second prompt")
}

func TestBuildVertexBatchJSONL_RejectsDuplicateCustomIDs(t *testing.T) {
	input := validVertexBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_001", Prompt: "Duplicate"})
	_, err := BuildVertexBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
}

func TestBuildVertexBatchJSONL_RejectsEmptyPrompt(t *testing.T) {
	input := validVertexBatchInput()
	input.Items[0].Prompt = " "
	_, err := BuildVertexBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
}

func TestBuildVertexBatchJSONL_WritesReferenceImages(t *testing.T) {
	input := validVertexBatchInput()
	input.Items[0].ReferenceImages = []BatchImageReference{
		{MimeType: "image/png", Data: []byte("png-bytes")},
		{MimeType: "image/jpeg", FileURI: "gs://bucket/refs/style.jpg"},
	}

	jsonl, err := BuildVertexBatchJSONL(input)
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
	require.Equal(t, "image/png", inlineData["mimeType"])
	require.Equal(t, "cG5nLWJ5dGVz", inlineData["data"])
	fileData := parts[2].(map[string]any)["fileData"].(map[string]any)
	require.Equal(t, "image/jpeg", fileData["mimeType"])
	require.Equal(t, "gs://bucket/refs/style.jpg", fileData["fileUri"])
}

func TestNormalizeVertexBatchModelPath(t *testing.T) {
	require.Equal(t, "publishers/google/models/gemini-3.1-flash-image", NormalizeVertexBatchModelPath("gemini-3.1-flash-image"))
	require.Equal(t, "publishers/google/models/gemini-2.5-flash-image", NormalizeVertexBatchModelPath("publishers/google/models/gemini-2.5-flash-image"))
	require.Equal(t, "projects/p/locations/global/models/m", NormalizeVertexBatchModelPath("projects/p/locations/global/models/m"))
}

func TestBuildVertexBatchPredictionJobsEndpoint(t *testing.T) {
	global, err := BuildVertexBatchPredictionJobsEndpoint("", "my-project", "global")
	require.NoError(t, err)
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/batchPredictionJobs", global)

	regional, err := BuildVertexBatchPredictionJobsEndpoint("", "my-project", "asia-northeast1")
	require.NoError(t, err)
	require.Equal(t, "https://asia-northeast1-aiplatform.googleapis.com/v1/projects/my-project/locations/asia-northeast1/batchPredictionJobs", regional)
}

func TestVertexProvider_SubmitUploadsJSONLAndCreatesBatchPredictionJob(t *testing.T) {
	vertexClient := &fakeVertexBatchClient{created: &VertexBatchPredictionJob{Name: "projects/proj/locations/global/batchPredictionJobs/job-1", State: "JOB_STATE_PENDING"}}
	store := &fakeVertexObjectStore{}
	provider := newTestVertexProvider(vertexClient, store)

	got, err := provider.Submit(context.Background(), &BatchImageJob{BatchID: "imgbatch_abc123", Model: "gemini-3.1-flash-image"}, vertexServiceAccount(), validVertexBatchInput())
	require.NoError(t, err)

	require.Equal(t, "gs://managed-bucket/batch-image/test/imgbatch_abc123/input/requests.jsonl", store.uploadURI)
	require.Equal(t, "projects/proj/locations/global/batchPredictionJobs/job-1", got.ProviderJobName)
	require.Equal(t, store.uploadURI, got.ProviderInputRef)
	require.Equal(t, "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/", got.ProviderOutputRef)
	require.Equal(t, "jsonl", vertexClient.createdReq.InputConfig.InstancesFormat)
	require.Equal(t, "jsonl", vertexClient.createdReq.OutputConfig.PredictionsFormat)
	require.Equal(t, got.ProviderOutputRef, vertexClient.createdReq.OutputConfig.GCSDestination.OutputURIPrefix)
	require.Equal(t, "key", vertexClient.createdReq.InstanceConfig.KeyField)
	require.NotContains(t, string(vertexClient.createdPayloadForAssert(t)), "serviceAccount")
	require.NotContains(t, string(vertexClient.createdPayloadForAssert(t)), "encryptionSpec")
	require.NotContains(t, got.ProviderInputRef+got.ProviderOutputRef+got.ProviderJobName, "A clean product hero image")
	require.NotContains(t, string(store.uploadedJSONL), "private_key")
}

func TestVertexProvider_GetMapsStates(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		err       *VertexBatchJobError
		wantState BatchProviderInternalState
		wantDone  bool
		wantCode  string
	}{
		{name: "pending", state: "JOB_STATE_PENDING", wantState: BatchProviderStateQueued},
		{name: "queued", state: "JOB_STATE_QUEUED", wantState: BatchProviderStateQueued},
		{name: "running", state: "JOB_STATE_RUNNING", wantState: BatchProviderStateRunning},
		{name: "succeeded", state: "JOB_STATE_SUCCEEDED", wantState: BatchProviderStateSucceeded, wantDone: true},
		{name: "failed", state: "JOB_STATE_FAILED", err: &VertexBatchJobError{Status: "INVALID_ARGUMENT", Message: "bad request"}, wantState: BatchProviderStateFailed, wantDone: true, wantCode: "INVALID_ARGUMENT"},
		{name: "cancelled", state: "JOB_STATE_CANCELLED", wantState: BatchProviderStateCancelled, wantDone: true, wantCode: "VERTEX_BATCH_CANCELLED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
			provider := newTestVertexProvider(&fakeVertexBatchClient{got: &VertexBatchPredictionJob{
				Name:         "projects/proj/locations/global/batchPredictionJobs/job-1",
				State:        tt.state,
				Error:        tt.err,
				OutputConfig: VertexBatchOutputConfig{GCSDestination: VertexBatchGCSDestination{OutputURIPrefix: output}},
			}}, &fakeVertexObjectStore{})
			got, err := provider.Get(context.Background(), vertexJobWithName("projects/proj/locations/global/batchPredictionJobs/job-1"), vertexServiceAccount())
			require.NoError(t, err)
			require.Equal(t, tt.wantState, got.InternalState)
			require.Equal(t, tt.wantDone, got.Done)
			require.Equal(t, output, got.ProviderOutputRef)
			require.Equal(t, tt.wantCode, got.ErrorCode)
		})
	}
}

func TestVertexProvider_OpenResultReturnsCombinedJSONLStream(t *testing.T) {
	output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
	store := &fakeVertexObjectStore{
		listed: []string{
			output + "predictions_2.jsonl",
			output + "predictions_1.jsonl",
		},
		objects: map[string]string{
			output + "predictions_1.jsonl": `{"key":"1"}` + "\n",
			output + "predictions_2.jsonl": `{"key":"2"}` + "\n",
		},
	}
	provider := newTestVertexProvider(&fakeVertexBatchClient{}, store)
	r, contentType, err := provider.OpenResult(context.Background(), &BatchImageJob{ProviderOutputRef: &output}, vertexServiceAccount())
	require.NoError(t, err)
	defer r.Close()

	body, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, "application/jsonl", contentType)
	require.Equal(t, "{\"key\":\"1\"}\n\n{\"key\":\"2\"}\n", string(body))
}

func TestVertexProvider_OpenResultMissingObjectsReturnsTypedError(t *testing.T) {
	output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
	provider := newTestVertexProvider(&fakeVertexBatchClient{}, &fakeVertexObjectStore{})
	_, _, err := provider.OpenResult(context.Background(), &BatchImageJob{ProviderOutputRef: &output}, vertexServiceAccount())
	require.Error(t, err)
	require.Equal(t, "VERTEX_RESULT_OBJECTS_MISSING", infraerrors.Reason(err))
}

func TestVertexProvider_CancelCallsClient(t *testing.T) {
	vertexClient := &fakeVertexBatchClient{}
	provider := newTestVertexProvider(vertexClient, &fakeVertexObjectStore{})

	err := provider.Cancel(context.Background(), vertexJobWithName("projects/proj/locations/global/batchPredictionJobs/job-1"), vertexServiceAccount())
	require.NoError(t, err)
	require.Equal(t, "projects/proj/locations/global/batchPredictionJobs/job-1", vertexClient.cancelledName)
}

func TestVertexProvider_CleanupDeletesOnlyManagedPaths(t *testing.T) {
	input := "gs://managed-bucket/batch-image/test/imgbatch_abc123/input/requests.jsonl"
	output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
	store := &fakeVertexObjectStore{}
	provider := newTestVertexProvider(&fakeVertexBatchClient{}, store)

	err := provider.Cleanup(context.Background(), &BatchImageJob{BatchID: "imgbatch_abc123", ProviderInputRef: &input, ProviderOutputRef: &output}, vertexServiceAccount(), CleanupTargetAll)
	require.NoError(t, err)
	require.Equal(t, []string{input}, store.deletedObjects)
	require.Equal(t, []string{output}, store.deletedPrefixes)
}

func TestVertexProvider_CleanupRejectsUnsafePath(t *testing.T) {
	input := "gs://other-bucket/batch-image/test/imgbatch_abc123/input/requests.jsonl"
	provider := newTestVertexProvider(&fakeVertexBatchClient{}, &fakeVertexObjectStore{})

	err := provider.Cleanup(context.Background(), &BatchImageJob{BatchID: "imgbatch_abc123", ProviderInputRef: &input}, vertexServiceAccount(), CleanupTargetInput)
	require.ErrorIs(t, err, ErrBatchImageProviderUnsafeCleanupPath)
}

func TestVertexProvider_ErrorsDoNotExposeServiceAccountSecrets(t *testing.T) {
	privateKey := "-----BEGIN PRIVATE KEY-----secret-----END PRIVATE KEY-----"
	account := vertexServiceAccount()
	account.Credentials["service_account_json"] = map[string]any{
		"type":         "service_account",
		"project_id":   "proj",
		"private_key":  privateKey,
		"client_email": "svc@proj.iam.gserviceaccount.com",
	}
	provider := newTestVertexProvider(&fakeVertexBatchClient{createErr: &VertexAPIError{StatusCode: 403, Message: "do not expose " + privateKey}}, &fakeVertexObjectStore{})

	_, err := provider.Submit(context.Background(), nil, account, validVertexBatchInput())
	require.Error(t, err)
	require.Equal(t, "VERTEX_PERMISSION_DENIED", infraerrors.Reason(err))
	require.NotContains(t, err.Error(), privateKey)
	require.NotContains(t, err.Error(), "svc@proj")
}

func TestVertexProvider_MetadataDoesNotStoreImageBytesOrBase64(t *testing.T) {
	vertexClient := &fakeVertexBatchClient{created: &VertexBatchPredictionJob{Name: "projects/proj/locations/global/batchPredictionJobs/job-1", State: "JOB_STATE_PENDING"}}
	provider := newTestVertexProvider(vertexClient, &fakeVertexObjectStore{})

	got, err := provider.Submit(context.Background(), nil, vertexServiceAccount(), validVertexBatchInput())
	require.NoError(t, err)
	metadata := got.ProviderJobName + got.ProviderInputRef + got.ProviderOutputRef
	require.NotContains(t, metadata, "iVBOR")
	require.NotContains(t, metadata, "base64")
	require.NotContains(t, metadata, "A clean product hero image")
}

func validVertexBatchInput() BatchImageInput {
	return BatchImageInput{
		BatchID:     "imgbatch_abc123",
		Model:       "gemini-3.1-flash-image",
		DisplayName: "test vertex batch",
		Items: []BatchImageInputItem{{
			CustomID: "cover_001",
			Prompt:   "A clean product hero image",
		}},
	}
}

func requireVertexJSONLLine(t *testing.T, line, wantKey, wantPrompt string) {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &got))
	require.Equal(t, wantKey, got["key"])
	request := got["request"].(map[string]any)
	contents := request["contents"].([]any)
	require.Equal(t, "user", contents[0].(map[string]any)["role"])
	parts := contents[0].(map[string]any)["parts"].([]any)
	require.Equal(t, wantPrompt, parts[0].(map[string]any)["text"])
	config := request["generationConfig"].(map[string]any)
	require.Equal(t, []any{"TEXT", "IMAGE"}, config["responseModalities"])
}

func newTestVertexProvider(client *fakeVertexBatchClient, store *fakeVertexObjectStore) *VertexBatchImageProvider {
	return NewVertexBatchImageProvider(VertexBatchImageProviderOptions{
		ProjectID:        "proj",
		Location:         "global",
		ManagedGCSBucket: "managed-bucket",
		ManagedGCSPrefix: "batch-image/{env}/{batch_id}",
		Environment:      "test",
	}, client, store, &fakeGeminiTokenCache{token: "ya29.test-token"})
}

func vertexServiceAccount() *Account {
	return &Account{
		Platform: PlatformGemini,
		Type:     AccountTypeServiceAccount,
		Credentials: map[string]any{
			"service_account_json": map[string]any{
				"type":         "service_account",
				"project_id":   "proj",
				"private_key":  "-----BEGIN PRIVATE KEY-----\nabc\n-----END PRIVATE KEY-----\n",
				"client_email": "svc@proj.iam.gserviceaccount.com",
			},
		},
	}
}

func vertexJobWithName(name string) *BatchImageJob {
	return &BatchImageJob{ProviderJobName: &name}
}

type fakeVertexBatchClient struct {
	created       *VertexBatchPredictionJob
	got           *VertexBatchPredictionJob
	createErr     error
	getErr        error
	cancelErr     error
	createdReq    VertexCreateBatchPredictionJobRequest
	cancelledName string
}

func (f *fakeVertexBatchClient) CreateBatchPredictionJob(_ context.Context, accessToken string, req VertexCreateBatchPredictionJobRequest) (*VertexBatchPredictionJob, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("missing token")
	}
	f.createdReq = req
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.created != nil {
		return f.created, nil
	}
	return &VertexBatchPredictionJob{Name: "projects/proj/locations/global/batchPredictionJobs/job-1", State: "JOB_STATE_PENDING"}, nil
}

func (f *fakeVertexBatchClient) GetBatchPredictionJob(_ context.Context, _ string, _ string) (*VertexBatchPredictionJob, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.got, nil
}

func (f *fakeVertexBatchClient) CancelBatchPredictionJob(_ context.Context, _ string, name string) error {
	f.cancelledName = name
	return f.cancelErr
}

func (f *fakeVertexBatchClient) createdPayloadForAssert(t *testing.T) []byte {
	t.Helper()
	b, err := json.Marshal(f.createdReq)
	require.NoError(t, err)
	return b
}

type fakeVertexObjectStore struct {
	uploadURI       string
	uploadedJSONL   []byte
	uploadErr       error
	listed          []string
	objects         map[string]string
	listErr         error
	openErr         error
	deleteErr       error
	deletedObjects  []string
	deletedPrefixes []string
}

func (f *fakeVertexObjectStore) UploadJSONL(_ context.Context, _ string, uri string, r io.Reader) error {
	f.uploadURI = uri
	f.uploadedJSONL, _ = io.ReadAll(r)
	return f.uploadErr
}

func (f *fakeVertexObjectStore) ListJSONLObjects(_ context.Context, _ string, _ string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]string, 0, len(f.listed))
	for _, item := range f.listed {
		if strings.HasSuffix(item, ".jsonl") {
			out = append(out, item)
		}
	}
	return out, nil
}

func (f *fakeVertexObjectStore) OpenObject(_ context.Context, _ string, uri string) (io.ReadCloser, string, error) {
	if f.openErr != nil {
		return nil, "", f.openErr
	}
	return io.NopCloser(bytes.NewBufferString(f.objects[uri])), "application/jsonl", nil
}

func (f *fakeVertexObjectStore) DeleteObject(_ context.Context, _ string, uri string) error {
	f.deletedObjects = append(f.deletedObjects, uri)
	return f.deleteErr
}

func (f *fakeVertexObjectStore) DeletePrefix(_ context.Context, _ string, uri string) error {
	f.deletedPrefixes = append(f.deletedPrefixes, uri)
	return f.deleteErr
}

type fakeGeminiTokenCache struct {
	token string
}

func (f *fakeGeminiTokenCache) GetAccessToken(context.Context, string) (string, error) {
	if strings.TrimSpace(f.token) == "" {
		return "", errors.New("missing token")
	}
	return f.token, nil
}

func (f *fakeGeminiTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (f *fakeGeminiTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
}

func (f *fakeGeminiTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return false, nil
}

func (f *fakeGeminiTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
}
