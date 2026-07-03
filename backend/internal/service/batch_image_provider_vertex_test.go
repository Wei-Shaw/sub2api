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
REDACTED

func TestVertexProvider_SupportsOnlyGeminiServiceAccount(t *testing.T) {
	provider := newTestVertexProvider(&fakeVertexBatchClient{REDACTED, &fakeVertexObjectStore{REDACTED)

	require.True(t, provider.SupportsAccount(vertexServiceAccount()))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"REDACTEDREDACTED))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"REDACTEDREDACTED))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount, Credentials: vertexServiceAccount().CredentialsREDACTED))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{REDACTEDREDACTED))
REDACTED

func TestVertexProvider_MissingServiceAccountRejected(t *testing.T) {
	provider := newTestVertexProvider(&fakeVertexBatchClient{REDACTED, &fakeVertexObjectStore{REDACTED)
	_, err := provider.Submit(context.Background(), nil, &Account{Platform: PlatformGemini, Type: AccountTypeServiceAccount, Credentials: map[string]any{REDACTEDREDACTED, validVertexBatchInput())
	require.ErrorIs(t, err, ErrBatchImageProviderMissingServiceAccount)
REDACTED

func TestVertexProvider_MissingManagedGCSBucketRejected(t *testing.T) {
	provider := NewVertexBatchImageProvider(VertexBatchImageProviderOptions{ProjectID: "proj", Environment: "test"REDACTED, &fakeVertexBatchClient{REDACTED, &fakeVertexObjectStore{REDACTED, &fakeGeminiTokenCache{token: "token"REDACTED)
	_, err := provider.Submit(context.Background(), nil, vertexServiceAccount(), validVertexBatchInput())
REDACTED
	require.Equal(t, "VERTEX_MANAGED_GCS_BUCKET_MISSING", infraerrors.Reason(err))
REDACTED

func TestBuildVertexBatchJSONL_WritesValidLinesAndPreservesCustomID(t *testing.T) {
	input := validVertexBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_002", Prompt: "Second prompt"REDACTED)

	jsonl, err := BuildVertexBatchJSONL(input)
REDACTED
	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	require.Len(t, lines, 2)
	requireVertexJSONLLine(t, lines[0], "cover_001", "A clean product hero image")
	requireVertexJSONLLine(t, lines[1], "cover_002", "Second prompt")
REDACTED

func TestBuildVertexBatchJSONL_RejectsDuplicateCustomIDs(t *testing.T) {
	input := validVertexBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_001", Prompt: "Duplicate"REDACTED)
	_, err := BuildVertexBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
REDACTED

func TestBuildVertexBatchJSONL_RejectsEmptyPrompt(t *testing.T) {
	input := validVertexBatchInput()
	input.Items[0].Prompt = " "
	_, err := BuildVertexBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
REDACTED

func TestNormalizeVertexBatchModelPath(t *testing.T) {
	require.Equal(t, "publishers/google/models/gemini-3.1-flash-image", NormalizeVertexBatchModelPath("gemini-3.1-flash-image"))
	require.Equal(t, "publishers/google/models/gemini-2.5-flash-image", NormalizeVertexBatchModelPath("publishers/google/models/gemini-2.5-flash-image"))
	require.Equal(t, "projects/p/locations/global/models/m", NormalizeVertexBatchModelPath("projects/p/locations/global/models/m"))
REDACTED

func TestBuildVertexBatchPredictionJobsEndpoint(t *testing.T) {
	global, err := BuildVertexBatchPredictionJobsEndpoint("", "my-project", "global")
REDACTED
	require.Equal(t, "https://aiplatform.googleapis.com/v1/projects/my-project/locations/global/batchPredictionJobs", global)

	regional, err := BuildVertexBatchPredictionJobsEndpoint("", "my-project", "asia-northeast1")
REDACTED
	require.Equal(t, "https://asia-northeast1-aiplatform.googleapis.com/v1/projects/my-project/locations/asia-northeast1/batchPredictionJobs", regional)
REDACTED

func TestVertexProvider_SubmitUploadsJSONLAndCreatesBatchPredictionJob(t *testing.T) {
REDACTED
	store := &fakeVertexObjectStore{REDACTED
	provider := newTestVertexProvider(vertexClient, store)

	got, err := provider.Submit(context.Background(), &BatchImageJob{BatchID: "imgbatch_abc123", Model: "gemini-3.1-flash-image"REDACTED, vertexServiceAccount(), validVertexBatchInput())
REDACTED

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
REDACTED

func TestVertexProvider_GetMapsStates(t *testing.T) {
	tests := []struct {
		name      string
		state     string
		err       *VertexBatchJobError
		wantState BatchProviderInternalState
		wantDone  bool
		wantCode  string
REDACTED{
		{name: "pending", state: "JOB_STATE_PENDING", wantState: BatchProviderStateQueuedREDACTED,
		{name: "queued", state: "JOB_STATE_QUEUED", wantState: BatchProviderStateQueuedREDACTED,
		{name: "running", state: "JOB_STATE_RUNNING", wantState: BatchProviderStateRunningREDACTED,
		{name: "succeeded", state: "JOB_STATE_SUCCEEDED", wantState: BatchProviderStateSucceeded, wantDone: trueREDACTED,
		{name: "failed", state: "JOB_STATE_FAILED", err: &VertexBatchJobError{Status: "INVALID_ARGUMENT", Message: "bad request"REDACTED, wantState: BatchProviderStateFailed, wantDone: true, wantCode: "INVALID_ARGUMENT"REDACTED,
		{name: "cancelled", state: "JOB_STATE_CANCELLED", wantState: BatchProviderStateCancelled, wantDone: true, wantCode: "VERTEX_BATCH_CANCELLED"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
			provider := newTestVertexProvider(&fakeVertexBatchClient{got: &VertexBatchPredictionJob{
				Name:         "projects/proj/locations/global/batchPredictionJobs/job-1",
				State:        tt.state,
				Error:        tt.err,
				OutputConfig: VertexBatchOutputConfig{GCSDestination: VertexBatchGCSDestination{OutputURIPrefix: outputREDACTEDREDACTED,
	REDACTED &fakeVertexObjectStore{REDACTED)
			got, err := provider.Get(context.Background(), vertexJobWithName("projects/proj/locations/global/batchPredictionJobs/job-1"), vertexServiceAccount())
		REDACTED
			require.Equal(t, tt.wantState, got.InternalState)
			require.Equal(t, tt.wantDone, got.Done)
			require.Equal(t, output, got.ProviderOutputRef)
			require.Equal(t, tt.wantCode, got.ErrorCode)
	REDACTED)
REDACTED
REDACTED

func TestVertexProvider_OpenResultReturnsCombinedJSONLStream(t *testing.T) {
	output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
	store := &fakeVertexObjectStore{
		listed: []string{
			output + "predictions_2.jsonl",
			output + "predictions_1.jsonl",
	REDACTED,
		objects: map[string]string{
			output + "predictions_1.jsonl": `{"key":"1"REDACTED` + "\n",
			output + "predictions_2.jsonl": `{"key":"2"REDACTED` + "\n",
	REDACTED,
REDACTED
	provider := newTestVertexProvider(&fakeVertexBatchClient{REDACTED, store)
	r, contentType, err := provider.OpenResult(context.Background(), &BatchImageJob{ProviderOutputRef: &outputREDACTED, vertexServiceAccount())
REDACTED
	defer r.Close()

	body, err := io.ReadAll(r)
REDACTED
	require.Equal(t, "application/jsonl", contentType)
	require.Equal(t, "{\"key\":\"1\"REDACTED\n\n{\"key\":\"2\"REDACTED\n", string(body))
REDACTED

func TestVertexProvider_OpenResultMissingObjectsReturnsTypedError(t *testing.T) {
	output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
	provider := newTestVertexProvider(&fakeVertexBatchClient{REDACTED, &fakeVertexObjectStore{REDACTED)
	_, _, err := provider.OpenResult(context.Background(), &BatchImageJob{ProviderOutputRef: &outputREDACTED, vertexServiceAccount())
REDACTED
	require.Equal(t, "VERTEX_RESULT_OBJECTS_MISSING", infraerrors.Reason(err))
REDACTED

func TestVertexProvider_CancelCallsClient(t *testing.T) {
	vertexClient := &fakeVertexBatchClient{REDACTED
REDACTED

	err := provider.Cancel(context.Background(), vertexJobWithName("projects/proj/locations/global/batchPredictionJobs/job-1"), vertexServiceAccount())
REDACTED
	require.Equal(t, "projects/proj/locations/global/batchPredictionJobs/job-1", vertexClient.cancelledName)
REDACTED

func TestVertexProvider_CleanupDeletesOnlyManagedPaths(t *testing.T) {
	input := "gs://managed-bucket/batch-image/test/imgbatch_abc123/input/requests.jsonl"
	output := "gs://managed-bucket/batch-image/test/imgbatch_abc123/output/"
	store := &fakeVertexObjectStore{REDACTED
	provider := newTestVertexProvider(&fakeVertexBatchClient{REDACTED, store)

	err := provider.Cleanup(context.Background(), &BatchImageJob{BatchID: "imgbatch_abc123", ProviderInputRef: &input, ProviderOutputRef: &outputREDACTED, vertexServiceAccount(), CleanupTargetAll)
REDACTED
	require.Equal(t, []string{inputREDACTED, store.deletedObjects)
	require.Equal(t, []string{outputREDACTED, store.deletedPrefixes)
REDACTED

func TestVertexProvider_CleanupRejectsUnsafePath(t *testing.T) {
	input := "gs://other-bucket/batch-image/test/imgbatch_abc123/input/requests.jsonl"
	provider := newTestVertexProvider(&fakeVertexBatchClient{REDACTED, &fakeVertexObjectStore{REDACTED)

	err := provider.Cleanup(context.Background(), &BatchImageJob{BatchID: "imgbatch_abc123", ProviderInputRef: &inputREDACTED, vertexServiceAccount(), CleanupTargetInput)
	require.ErrorIs(t, err, ErrBatchImageProviderUnsafeCleanupPath)
REDACTED

func TestVertexProvider_ErrorsDoNotExposeServiceAccountSecrets(t *testing.T) {
	privateKey := "REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED

REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED

REDACTED
REDACTED
REDACTED

REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED

REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED

REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED

REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED, client, store, &fakeGeminiTokenCache{token: "ya29.test-token"REDACTED)
REDACTED

REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED\nabc\n-----END PRIVATE KEY-----\n",
		REDACTED
		REDACTED,
	REDACTED,
REDACTED
REDACTED

func vertexJobWithName(name string) *BatchImageJob {
	return &BatchImageJob{ProviderJobName: &nameREDACTED
REDACTED

type fakeVertexBatchClient struct {
	created       *VertexBatchPredictionJob
	got           *VertexBatchPredictionJob
	createErr     error
	getErr        error
	cancelErr     error
	createdReq    VertexCreateBatchPredictionJobRequest
	cancelledName string
REDACTED

func (f *fakeVertexBatchClient) CreateBatchPredictionJob(_ context.Context, accessToken string, req VertexCreateBatchPredictionJobRequest) (*VertexBatchPredictionJob, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("missing token")
REDACTED
	f.createdReq = req
	if f.createErr != nil {
		return nil, f.createErr
REDACTED
	if f.created != nil {
		return f.created, nil
REDACTED
	return &VertexBatchPredictionJob{Name: "projects/proj/locations/global/batchPredictionJobs/job-1", State: "JOB_STATE_PENDING"REDACTED, nil
REDACTED

func (f *fakeVertexBatchClient) GetBatchPredictionJob(_ context.Context, _ string, _ string) (*VertexBatchPredictionJob, error) {
	if f.getErr != nil {
		return nil, f.getErr
REDACTED
	return f.got, nil
REDACTED

func (f *fakeVertexBatchClient) CancelBatchPredictionJob(_ context.Context, _ string, name string) error {
	f.cancelledName = name
	return f.cancelErr
REDACTED

func (f *fakeVertexBatchClient) createdPayloadForAssert(t *testing.T) []byte {
REDACTED
	b, err := json.Marshal(f.createdReq)
REDACTED
	return b
REDACTED

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
REDACTED

func (f *fakeVertexObjectStore) UploadJSONL(_ context.Context, _ string, uri string, r io.Reader) error {
	f.uploadURI = uri
	f.uploadedJSONL, _ = io.ReadAll(r)
	return f.uploadErr
REDACTED

func (f *fakeVertexObjectStore) ListJSONLObjects(_ context.Context, _ string, _ string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
REDACTED
	out := make([]string, 0, len(f.listed))
	for _, item := range f.listed {
		if strings.HasSuffix(item, ".jsonl") {
			out = append(out, item)
	REDACTED
REDACTED
	return out, nil
REDACTED

func (f *fakeVertexObjectStore) OpenObject(_ context.Context, _ string, uri string) (io.ReadCloser, string, error) {
	if f.openErr != nil {
		return nil, "", f.openErr
REDACTED
	return io.NopCloser(bytes.NewBufferString(f.objects[uri])), "application/jsonl", nil
REDACTED

func (f *fakeVertexObjectStore) DeleteObject(_ context.Context, _ string, uri string) error {
	f.deletedObjects = append(f.deletedObjects, uri)
	return f.deleteErr
REDACTED

func (f *fakeVertexObjectStore) DeletePrefix(_ context.Context, _ string, uri string) error {
	f.deletedPrefixes = append(f.deletedPrefixes, uri)
	return f.deleteErr
REDACTED

type fakeGeminiTokenCache struct {
	token string
REDACTED

func (f *fakeGeminiTokenCache) GetAccessToken(context.Context, string) (string, error) {
	if strings.TrimSpace(f.token) == "" {
		return "", errors.New("missing token")
REDACTED
	return f.token, nil
REDACTED

func (f *fakeGeminiTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
REDACTED

func (f *fakeGeminiTokenCache) DeleteAccessToken(context.Context, string) error {
	return nil
REDACTED

func (f *fakeGeminiTokenCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return false, nil
REDACTED

func (f *fakeGeminiTokenCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
REDACTED
