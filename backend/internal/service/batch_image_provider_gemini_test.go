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
REDACTED
	require.Same(t, provider, must)

	_, err = registry.MustGet("unknown_provider")
	require.ErrorIs(t, err, ErrBatchImageInvalidProvider)
REDACTED

func TestGeminiProvider_SupportsOnlyGeminiAPIKeyWithSecret(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{REDACTED)

	require.True(t, provider.SupportsAccount(geminiAPIKeyAccount("sk-gemini")))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{REDACTEDREDACTED))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"api_key": "sk"REDACTEDREDACTED))
	require.False(t, provider.SupportsAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"REDACTEDREDACTED))
	require.False(t, provider.SupportsAccount(nil))
REDACTED

func TestGeminiProvider_MissingAPIKeyRejected(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{REDACTED)
	_, err := provider.Submit(context.Background(), nil, &Account{Platform: PlatformGemini, Type: AccountTypeAPIKeyREDACTED, validGeminiBatchInput())
	require.ErrorIs(t, err, ErrBatchImageProviderMissingAPIKey)
REDACTED

func TestBuildGeminiBatchJSONL_WritesValidLinesAndPreservesCustomID(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_002", Prompt: "Second prompt"REDACTED)

	jsonl, err := BuildGeminiBatchJSONL(input)
REDACTED

	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	require.Len(t, lines, 2)
	requireJSONLLine(t, lines[0], "cover_001", "A clean product hero image")
	requireJSONLLine(t, lines[1], "cover_002", "Second prompt")
REDACTED

func TestBuildGeminiBatchJSONL_RejectsDuplicateCustomIDs(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items = append(input.Items, BatchImageInputItem{CustomID: "cover_001", Prompt: "Duplicate"REDACTED)

	_, err := BuildGeminiBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
REDACTED

func TestBuildGeminiBatchJSONL_RejectsEmptyPrompt(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items[0].Prompt = " "

	_, err := BuildGeminiBatchJSONL(input)
	require.ErrorIs(t, err, ErrBatchImageProviderInvalidInput)
REDACTED

func TestBuildGeminiBatchJSONL_WritesReferenceImages(t *testing.T) {
	input := validGeminiBatchInput()
	input.Items[0].ReferenceImages = []BatchImageReference{
		{MimeType: "image/webp", Data: []byte("webp-bytes")REDACTED,
		{MimeType: "image/jpeg", FileURI: "gs://bucket/refs/style.jpg"REDACTED,
REDACTED

	jsonl, err := BuildGeminiBatchJSONL(input)
REDACTED
	lines := strings.Split(strings.TrimSpace(string(jsonl)), "\n")
	require.Len(t, lines, 1)

REDACTED
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &got))
REDACTED
REDACTED
REDACTED
	require.Len(t, parts, 3)
	require.Equal(t, "A clean product hero image", parts[0].(map[string]any)["text"])
	inlineData := parts[1].(map[string]any)["inlineData"].(map[string]any)
	require.Equal(t, "image/webp", inlineData["mimeType"])
	require.Equal(t, "d2VicC1ieXRlcw==", inlineData["data"])
	fileData := parts[2].(map[string]any)["fileData"].(map[string]any)
	require.Equal(t, "image/jpeg", fileData["mimeType"])
	require.Equal(t, "gs://bucket/refs/style.jpg", fileData["fileUri"])
REDACTED

func TestGeminiProvider_SubmitUploadsJSONLThenCreatesBatch(t *testing.T) {
	client := &fakeGeminiBatchClient{
		uploaded: &GeminiUploadedFile{Name: "files/input-jsonl"REDACTED,
		created:  &GeminiBatchJob{Name: "batches/job-123", State: "JOB_STATE_PENDING"REDACTED,
REDACTED
	provider := NewGeminiAPIBatchImageProvider(client)

	got, err := provider.Submit(context.Background(), &BatchImageJob{BatchID: "imgbatch_123", Model: "gemini-3.1-flash-image"REDACTED, geminiAPIKeyAccount("sk-secret"), validGeminiBatchInput())
REDACTED
	require.Equal(t, []string{"upload", "create"REDACTED, client.calls)
	require.Equal(t, "files/input-jsonl", got.ProviderInputRef)
	require.Equal(t, "batches/job-123", got.ProviderJobName)
	require.Empty(t, got.ProviderOutputRef)
	require.NotContains(t, got.ProviderInputRef, "A clean product hero image")
	require.NotContains(t, string(client.uploadedJSONL), "sk-secret")
REDACTED

func TestGeminiProvider_GetMapsStates(t *testing.T) {
	tests := []struct {
		name      string
		job       *GeminiBatchJob
		wantState BatchProviderInternalState
		wantDone  bool
		wantRef   string
		wantCode  string
REDACTED{
		{name: "running", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_RUNNING"REDACTED, wantState: BatchProviderStateRunningREDACTED,
		{name: "succeeded_dest_fileName", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_SUCCEEDED", Dest: &GeminiBatchDest{FileName: "files/out"REDACTEDREDACTED, wantState: BatchProviderStateSucceeded, wantDone: true, wantRef: "files/out"REDACTED,
		{name: "failed", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_FAILED", Error: &GeminiBatchError{Code: "BAD_PROMPT", Message: "bad prompt"REDACTEDREDACTED, wantState: BatchProviderStateFailed, wantDone: true, wantCode: "BAD_PROMPT"REDACTED,
		{name: "cancelled", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_CANCELLED"REDACTED, wantState: BatchProviderStateCancelled, wantDone: true, wantCode: "GEMINI_BATCH_CANCELLED"REDACTED,
		{name: "expired", job: &GeminiBatchJob{Name: "batches/1", State: "JOB_STATE_EXPIRED"REDACTED, wantState: BatchProviderStateExpired, wantDone: true, wantCode: "GEMINI_BATCH_EXPIRED"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{got: tt.jobREDACTED)
			got, err := provider.Get(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret"))
		REDACTED
			require.Equal(t, tt.wantState, got.InternalState)
			require.Equal(t, tt.wantDone, got.Done)
			require.Equal(t, tt.wantRef, got.ProviderOutputRef)
			require.Equal(t, tt.wantCode, got.ErrorCode)
			require.NotContains(t, got.ErrorMessage, "sk-secret")
	REDACTED)
REDACTED
REDACTED

func TestGeminiProvider_GetExtractsResponsesFileReference(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{
		got: &GeminiBatchJob{
			Name:     "batches/1",
			State:    "JOB_STATE_SUCCEEDED",
			Response: &GeminiBatchResponse{ResponsesFile: "files/responses-jsonl"REDACTED,
	REDACTED,
REDACTED)

	got, err := provider.Get(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret"))
REDACTED
	require.Equal(t, BatchProviderStateSucceeded, got.InternalState)
	require.Equal(t, "files/responses-jsonl", got.ProviderOutputRef)
REDACTED

func TestGeminiProvider_GetRejectsInlineResultShape(t *testing.T) {
	provider := NewGeminiAPIBatchImageProvider(&fakeGeminiBatchClient{
		got: &GeminiBatchJob{
			Name:     "batches/1",
			State:    "JOB_STATE_SUCCEEDED",
			Response: &GeminiBatchResponse{InlinedResponses: []any{map[string]any{"response": "large"REDACTEDREDACTEDREDACTED,
	REDACTED,
REDACTED)

	_, err := provider.Get(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret"))
	require.ErrorIs(t, err, ErrBatchImageProviderInlineResultUnsupported)
REDACTED

func TestGeminiProvider_OpenResultStreamsResultFile(t *testing.T) {
	client := &fakeGeminiBatchClient{downloadBody: "line1\n", downloadContentType: "application/jsonl"REDACTED
	provider := NewGeminiAPIBatchImageProvider(client)

	outputRef := "files/output-jsonl"
	r, contentType, err := provider.OpenResult(context.Background(), &BatchImageJob{ProviderOutputRef: &outputRefREDACTED, geminiAPIKeyAccount("sk-secret"))
REDACTED
	defer r.Close()

	body, err := io.ReadAll(r)
REDACTED
	require.Equal(t, "line1\n", string(body))
	require.Equal(t, "application/jsonl", contentType)
	require.Equal(t, "files/output-jsonl", client.downloadedFile)
REDACTED

func TestGeminiProvider_CancelCallsClient(t *testing.T) {
	client := &fakeGeminiBatchClient{REDACTED
	provider := NewGeminiAPIBatchImageProvider(client)

	require.NoError(t, provider.Cancel(context.Background(), jobWithProviderName("batches/1"), geminiAPIKeyAccount("sk-secret")))
	require.Equal(t, "batches/1", client.cancelledBatch)
REDACTED

func TestGeminiProvider_CleanupDeletesRefsOnlyWhenPresent(t *testing.T) {
	inputRef := "files/input"
	outputRef := "files/output"
	client := &fakeGeminiBatchClient{REDACTED
	provider := NewGeminiAPIBatchImageProvider(client)

	err := provider.Cleanup(context.Background(), &BatchImageJob{ProviderInputRef: &inputRef, ProviderOutputRef: &outputRefREDACTED, geminiAPIKeyAccount("sk-secret"), CleanupTargetAll)
REDACTED
	require.Equal(t, []string{"files/input", "files/output"REDACTED, client.deletedFiles)

	err = provider.Cleanup(context.Background(), &BatchImageJob{REDACTED, geminiAPIKeyAccount("sk-secret"), CleanupTargetAll)
REDACTED
	require.Equal(t, []string{"files/input", "files/output"REDACTED, client.deletedFiles)
REDACTED

func TestGeminiProvider_ErrorsDoNotExposeAPIKey(t *testing.T) {
	apiKey := "sk-top-secret"
	client := &fakeGeminiBatchClient{uploadErr: &GeminiAPIError{StatusCode: 401, Message: "upstream body should be hidden " + apiKeyREDACTEDREDACTED
	provider := NewGeminiAPIBatchImageProvider(client)

	_, err := provider.Submit(context.Background(), nil, geminiAPIKeyAccount(apiKey), validGeminiBatchInput())
REDACTED
	require.Equal(t, "GEMINI_AUTH_FAILED", infraerrors.Reason(err))
	require.NotContains(t, err.Error(), apiKey)
REDACTED

func TestGeminiProvider_MetadataDoesNotStoreImageBytesOrBase64(t *testing.T) {
	client := &fakeGeminiBatchClient{
		uploaded: &GeminiUploadedFile{Name: "files/input-jsonl"REDACTED,
		created:  &GeminiBatchJob{Name: "batches/job-123", State: "JOB_STATE_PENDING"REDACTED,
REDACTED
	provider := NewGeminiAPIBatchImageProvider(client)

	got, err := provider.Submit(context.Background(), nil, geminiAPIKeyAccount("sk-secret"), validGeminiBatchInput())
REDACTED
	require.NotContains(t, got.ProviderJobName, "base64")
	require.NotContains(t, got.ProviderInputRef, "base64")
	require.NotContains(t, got.ProviderOutputRef, "base64")
	require.NotContains(t, got.ProviderJobName+got.ProviderInputRef+got.ProviderOutputRef, "iVBOR")
	require.NotContains(t, got.ProviderJobName+got.ProviderInputRef+got.ProviderOutputRef, "A clean product hero image")
REDACTED

func requireJSONLLine(t *testing.T, line, wantKey, wantPrompt string) {
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

func validGeminiBatchInput() BatchImageInput {
REDACTED
		BatchID:     "imgbatch_123",
REDACTED
		DisplayName: "test batch",
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED
REDACTED

func geminiAPIKeyAccount(apiKey string) *Account {
REDACTED
		Platform:    PlatformGemini,
		Type:        AccountTypeAPIKey,
REDACTED"api_key": apiKeyREDACTED,
REDACTED
REDACTED

func jobWithProviderName(name string) *BatchImageJob {
	return &BatchImageJob{ProviderJobName: &nameREDACTED
REDACTED

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
REDACTED

func (f *fakeGeminiBatchClient) UploadJSONL(_ context.Context, apiKey string, _ string, r io.Reader) (*GeminiUploadedFile, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("missing api key")
REDACTED
	f.calls = append(f.calls, "upload")
	f.uploadedJSONL, _ = io.ReadAll(r)
	if f.uploadErr != nil {
		return nil, f.uploadErr
REDACTED
	if f.uploaded != nil {
		return f.uploaded, nil
REDACTED
	return &GeminiUploadedFile{Name: "files/input-jsonl"REDACTED, nil
REDACTED

func (f *fakeGeminiBatchClient) CreateBatch(_ context.Context, _ string, _ string, fileName string, _ string) (*GeminiBatchJob, error) {
	f.calls = append(f.calls, "create")
	f.createdFile = fileName
	if f.createErr != nil {
		return nil, f.createErr
REDACTED
	if f.created != nil {
		return f.created, nil
REDACTED
	return &GeminiBatchJob{Name: "batches/job-123", State: "JOB_STATE_PENDING"REDACTED, nil
REDACTED

func (f *fakeGeminiBatchClient) GetBatch(_ context.Context, _ string, _ string) (*GeminiBatchJob, error) {
	f.calls = append(f.calls, "get")
	if f.getErr != nil {
		return nil, f.getErr
REDACTED
	return f.got, nil
REDACTED

func (f *fakeGeminiBatchClient) CancelBatch(_ context.Context, _ string, batchName string) error {
	f.calls = append(f.calls, "cancel")
	f.cancelledBatch = batchName
	return f.cancelErr
REDACTED

func (f *fakeGeminiBatchClient) DownloadFile(_ context.Context, _ string, fileName string) (io.ReadCloser, string, error) {
	f.calls = append(f.calls, "download")
	f.downloadedFile = fileName
	if f.downloadErr != nil {
		return nil, "", f.downloadErr
REDACTED
	contentType := f.downloadContentType
	if contentType == "" {
		contentType = "application/octet-stream"
REDACTED
	return io.NopCloser(bytes.NewBufferString(f.downloadBody)), contentType, nil
REDACTED

func (f *fakeGeminiBatchClient) DeleteFile(_ context.Context, _ string, fileName string) error {
	f.calls = append(f.calls, "delete")
	f.deletedFiles = append(f.deletedFiles, fileName)
	return f.deleteErr
REDACTED
