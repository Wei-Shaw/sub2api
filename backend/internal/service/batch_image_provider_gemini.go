package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/geminicli"
)

const defaultGeminiBatchRequeueAfter = 30 * time.Second

type GeminiBatchClient interface {
	UploadJSONL(ctx context.Context, apiKey string, displayName string, r io.Reader) (*GeminiUploadedFile, error)
	CreateBatch(ctx context.Context, apiKey string, model string, fileName string, displayName string) (*GeminiBatchJob, error)
	GetBatch(ctx context.Context, apiKey string, batchName string) (*GeminiBatchJob, error)
	CancelBatch(ctx context.Context, apiKey string, batchName string) error
	DownloadFile(ctx context.Context, apiKey string, fileName string) (io.ReadCloser, string, error)
	DeleteFile(ctx context.Context, apiKey string, fileName string) error
REDACTED

type GeminiUploadedFile struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	URI         string `json:"uri"`
	MimeType    string `json:"mimeType"`
REDACTED

type GeminiBatchJob struct {
	Name     string               `json:"name"`
	State    string               `json:"state"`
	Dest     *GeminiBatchDest     `json:"dest"`
	Response *GeminiBatchResponse `json:"response"`
	Error    *GeminiBatchError    `json:"error"`
	Raw      map[string]any       `json:"-"`
REDACTED

type GeminiBatchDest struct {
	FileName      string `json:"fileName"`
	FileNameSnake string `json:"file_name"`
REDACTED

type GeminiBatchResponse struct {
	ResponsesFile       string `json:"responsesFile"`
	ResponsesFileSnake  string `json:"responses_file"`
	InlinedResponses    []any  `json:"inlinedResponses"`
	InlinedResponsesAlt []any  `json:"inlined_responses"`
REDACTED

type GeminiBatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
REDACTED

type GeminiAPIBatchImageProvider struct {
	client GeminiBatchClient
REDACTED

func NewGeminiAPIBatchImageProvider(client GeminiBatchClient) *GeminiAPIBatchImageProvider {
	if client == nil {
		client = NewGeminiBatchHTTPClient("", nil)
REDACTED
	return &GeminiAPIBatchImageProvider{client: clientREDACTED
REDACTED

func (p *GeminiAPIBatchImageProvider) Name() string {
	return BatchImageProviderGeminiAPI
REDACTED

func (p *GeminiAPIBatchImageProvider) SupportsAccount(account *Account) bool {
	return account != nil &&
		account.Platform == PlatformGemini &&
		account.Type == AccountTypeAPIKey &&
		batchImageProviderAPIKey(account) != ""
REDACTED

func (p *GeminiAPIBatchImageProvider) Submit(ctx context.Context, job *BatchImageJob, account *Account, input BatchImageInput) (*BatchProviderJob, error) {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return nil, ErrBatchImageProviderUnsupportedAccount
REDACTED
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return nil, ErrBatchImageProviderMissingAPIKey
REDACTED
	if input.BatchID == "" && job != nil {
		input.BatchID = job.BatchID
REDACTED
	if input.Model == "" && job != nil {
		input.Model = job.Model
REDACTED

	jsonl, err := BuildGeminiBatchJSONL(input)
	if err != nil {
		return nil, err
REDACTED

	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(input.BatchID)
REDACTED

	uploaded, err := p.client.UploadJSONL(ctx, apiKey, displayName, bytes.NewReader(jsonl))
	if err != nil {
		return nil, mapGeminiClientError(err)
REDACTED
	if uploaded == nil || strings.TrimSpace(uploaded.Name) == "" {
		return nil, geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini upload response is missing file name", nil)
REDACTED

	batch, err := p.client.CreateBatch(ctx, apiKey, input.Model, uploaded.Name, displayName)
	if err != nil {
		return nil, mapGeminiClientError(err)
REDACTED
	if batch == nil || strings.TrimSpace(batch.Name) == "" {
		return nil, geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini batch response is missing job name", nil)
REDACTED

	return &BatchProviderJob{
		ProviderJobName:  batch.Name,
		ProviderInputRef: uploaded.Name,
		RawState:         batch.State,
REDACTED, nil
REDACTED

func (p *GeminiAPIBatchImageProvider) Get(ctx context.Context, job *BatchImageJob, account *Account) (*BatchProviderStatus, error) {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return nil, ErrBatchImageProviderUnsupportedAccount
REDACTED
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return nil, ErrBatchImageProviderMissingAPIKey
REDACTED
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return nil, ErrBatchImageProviderMissingJobName
REDACTED

	batch, err := p.client.GetBatch(ctx, apiKey, jobName)
	if err != nil {
		return nil, mapGeminiClientError(err)
REDACTED
	if batch == nil {
		return nil, geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini batch response is empty", nil)
REDACTED

	status := mapGeminiBatchState(batch)
	if status.InternalState == BatchProviderStateSucceeded {
		if geminiBatchHasInlineResults(batch) {
			return nil, ErrBatchImageProviderInlineResultUnsupported
	REDACTED
		outputRef := geminiBatchOutputRef(batch)
		if outputRef == "" {
			status.InternalState = BatchProviderStateFailed
			status.Done = true
			status.ErrorCode = "GEMINI_RESULT_FILE_MISSING"
			status.ErrorMessage = "Gemini batch succeeded without a result file reference"
	REDACTED
		status.ProviderOutputRef = outputRef
REDACTED
	return status, nil
REDACTED

func (p *GeminiAPIBatchImageProvider) Cancel(ctx context.Context, job *BatchImageJob, account *Account) error {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return ErrBatchImageProviderUnsupportedAccount
REDACTED
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return ErrBatchImageProviderMissingAPIKey
REDACTED
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return ErrBatchImageProviderMissingJobName
REDACTED
	return mapGeminiClientError(p.client.CancelBatch(ctx, apiKey, jobName))
REDACTED

func (p *GeminiAPIBatchImageProvider) OpenResult(ctx context.Context, job *BatchImageJob, account *Account) (io.ReadCloser, string, error) {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return nil, "", ErrBatchImageProviderUnsupportedAccount
REDACTED
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return nil, "", ErrBatchImageProviderMissingAPIKey
REDACTED
	outputRef := batchImageProviderOutputRef(job)
	if outputRef == "" {
		return nil, "", ErrBatchImageProviderMissingResultRef
REDACTED
	r, contentType, err := p.client.DownloadFile(ctx, apiKey, outputRef)
	return r, contentType, mapGeminiClientError(err)
REDACTED

func (p *GeminiAPIBatchImageProvider) Cleanup(ctx context.Context, job *BatchImageJob, account *Account, target CleanupTarget) error {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return ErrBatchImageProviderUnsupportedAccount
REDACTED
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return ErrBatchImageProviderMissingAPIKey
REDACTED

	switch target {
	case CleanupTargetInput:
		return p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderInputRef(job))
	case CleanupTargetOutput:
		return p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderOutputRef(job))
	case CleanupTargetAll:
		if err := p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderInputRef(job)); err != nil {
			return err
	REDACTED
		return p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderOutputRef(job))
	default:
		return ErrUnsupportedCleanupTarget
REDACTED
REDACTED

func (p *GeminiAPIBatchImageProvider) deleteGeminiFileIfPresent(ctx context.Context, apiKey, fileName string) error {
	if strings.TrimSpace(fileName) == "" {
		return nil
REDACTED
	return mapGeminiClientError(p.client.DeleteFile(ctx, apiKey, fileName))
REDACTED

type geminiJSONLLine struct {
	Key     string                `json:"key"`
	Request geminiGenerateRequest `json:"request"`
REDACTED

type geminiGenerateRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
REDACTED

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
REDACTED

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
	FileData   *geminiFileData   `json:"fileData,omitempty"`
REDACTED

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
REDACTED

type geminiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
REDACTED

type geminiGenerationConfig struct {
	ResponseModalities []string `json:"responseModalities"`
REDACTED

func BuildGeminiBatchJSONL(input BatchImageInput) ([]byte, error) {
	if strings.TrimSpace(input.Model) == "" {
		return nil, batchImageProviderInputError("model is required")
REDACTED
	if len(input.Items) == 0 {
		return nil, batchImageProviderInputError("at least one item is required")
REDACTED

	seen := make(map[string]struct{REDACTED, len(input.Items))
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, item := range input.Items {
		customID := strings.TrimSpace(item.CustomID)
		if customID == "" {
			return nil, batchImageProviderInputError("custom_id is required")
	REDACTED
		if _, ok := seen[customID]; ok {
			return nil, batchImageProviderInputError("duplicate custom_id %q", customID)
	REDACTED
		seen[customID] = struct{REDACTED{REDACTED

		prompt := strings.TrimSpace(item.Prompt)
		if prompt == "" {
			return nil, batchImageProviderInputError("prompt is required for custom_id %q", customID)
	REDACTED
		parts, err := batchImageGeminiParts(prompt, item.ReferenceImages)
		if err != nil {
			return nil, err
	REDACTED

		// TODO(batch-image): add response_mime_type/aspect_ratio/image_size once the
		// Gemini batch image REST shape is stabilized for those options.
		line := geminiJSONLLine{
			Key: customID,
			Request: geminiGenerateRequest{
				Contents: []geminiContent{{
					Parts: parts,
		REDACTED
				GenerationConfig: geminiGenerationConfig{
					ResponseModalities: []string{"TEXT", "IMAGE"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		if err := enc.Encode(line); err != nil {
			return nil, err
	REDACTED
REDACTED
	return buf.Bytes(), nil
REDACTED

func batchImageGeminiParts(prompt string, refs []BatchImageReference) ([]geminiPart, error) {
	parts := []geminiPart{{Text: promptREDACTEDREDACTED
	for _, ref := range refs {
		mimeType := normalizeBatchImageReferenceMimeType(ref.MimeType)
		if mimeType == "" {
			return nil, batchImageProviderInputError("reference image mime_type is required")
	REDACTED
		fileURI := strings.TrimSpace(ref.FileURI)
		switch {
		case len(ref.Data) > 0 && fileURI == "":
			parts = append(parts, geminiPart{InlineData: &geminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(ref.Data),
		REDACTEDREDACTED)
		case len(ref.Data) == 0 && fileURI != "":
			parts = append(parts, geminiPart{FileData: &geminiFileData{
				MimeType: mimeType,
				FileURI:  fileURI,
		REDACTEDREDACTED)
		default:
			return nil, batchImageProviderInputError("reference image must contain exactly one of data or file_uri")
	REDACTED
REDACTED
	return parts, nil
REDACTED

func mapGeminiBatchState(batch *GeminiBatchJob) *BatchProviderStatus {
	state := strings.TrimSpace(batch.State)
	normalized := strings.ToUpper(state)
	status := &BatchProviderStatus{
		RawState:              state,
		InternalState:         BatchProviderStateRunning,
		SuggestedRequeueAfter: defaultGeminiBatchRequeueAfter,
REDACTED

	switch normalized {
	case "JOB_STATE_PENDING", "JOB_STATE_QUEUED":
		status.InternalState = BatchProviderStateQueued
	case "JOB_STATE_RUNNING":
		status.InternalState = BatchProviderStateRunning
	case "JOB_STATE_SUCCEEDED":
		status.InternalState = BatchProviderStateSucceeded
		status.Done = true
	case "JOB_STATE_FAILED":
		status.InternalState = BatchProviderStateFailed
		status.Done = true
		status.ErrorCode = "GEMINI_BATCH_FAILED"
	case "JOB_STATE_CANCELLED":
		status.InternalState = BatchProviderStateCancelled
		status.Done = true
		status.ErrorCode = "GEMINI_BATCH_CANCELLED"
	case "JOB_STATE_EXPIRED":
		status.InternalState = BatchProviderStateExpired
		status.Done = true
		status.ErrorCode = "GEMINI_BATCH_EXPIRED"
	default:
		if batch.Error != nil && (strings.TrimSpace(batch.Error.Message) != "" || strings.TrimSpace(batch.Error.Code) != "") {
			status.InternalState = BatchProviderStateFailed
			status.Done = true
			status.ErrorCode = "GEMINI_BATCH_FAILED"
	REDACTED
REDACTED

	if batch.Error != nil {
		if code := strings.TrimSpace(batch.Error.Code); code != "" {
			status.ErrorCode = code
	REDACTED else if status.ErrorCode == "" && strings.TrimSpace(batch.Error.Status) != "" {
			status.ErrorCode = strings.TrimSpace(batch.Error.Status)
	REDACTED
		status.ErrorMessage = strings.TrimSpace(batch.Error.Message)
REDACTED
	return status
REDACTED

func geminiBatchOutputRef(batch *GeminiBatchJob) string {
	if batch == nil {
		return ""
REDACTED
	if batch.Dest != nil {
		if v := strings.TrimSpace(batch.Dest.FileName); v != "" {
			return v
	REDACTED
		if v := strings.TrimSpace(batch.Dest.FileNameSnake); v != "" {
			return v
	REDACTED
REDACTED
	if batch.Response != nil {
		if v := strings.TrimSpace(batch.Response.ResponsesFile); v != "" {
			return v
	REDACTED
		if v := strings.TrimSpace(batch.Response.ResponsesFileSnake); v != "" {
			return v
	REDACTED
REDACTED
	return ""
REDACTED

func geminiBatchHasInlineResults(batch *GeminiBatchJob) bool {
	return batch != nil &&
		batch.Response != nil &&
		(len(batch.Response.InlinedResponses) > 0 || len(batch.Response.InlinedResponsesAlt) > 0)
REDACTED

func geminiProviderError(reason, message string, cause error) error {
	err := infraerrors.New(http.StatusBadGateway, reason, message)
	if cause != nil {
		return err.WithCause(cause)
REDACTED
	return err
REDACTED

func mapGeminiClientError(err error) error {
	if err == nil {
		return nil
REDACTED
	var apiErr *GeminiAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return geminiProviderError("GEMINI_AUTH_FAILED", "Gemini authentication failed", nil)
		case http.StatusTooManyRequests:
			return geminiProviderError("GEMINI_RATE_LIMITED", "Gemini rate limit exceeded", nil)
		case http.StatusNotFound:
			return geminiProviderError("GEMINI_BATCH_NOT_FOUND", "Gemini batch resource was not found", nil)
		default:
			return geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini API request failed", nil)
	REDACTED
REDACTED
	return geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini API request failed", nil)
REDACTED

type GeminiBatchHTTPClient struct {
	baseURL string
	client  *http.Client
REDACTED

func NewGeminiBatchHTTPClient(baseURL string, client *http.Client) *GeminiBatchHTTPClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = geminicli.AIStudioBaseURL
REDACTED
	if client == nil {
		client = http.DefaultClient
REDACTED
	return &GeminiBatchHTTPClient{baseURL: baseURL, client: clientREDACTED
REDACTED

func (c *GeminiBatchHTTPClient) UploadJSONL(ctx context.Context, apiKey string, displayName string, r io.Reader) (*GeminiUploadedFile, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadataHeader := textproto.MIMEHeader{REDACTED
	metadataHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metadataHeader.Set("Content-Type", "application/json; charset=utf-8")
	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return nil, err
REDACTED
	metadata := map[string]any{"file": map[string]any{"displayName": displayName, "mimeType": "application/jsonl"REDACTEDREDACTED
	if err := json.NewEncoder(metadataPart).Encode(metadata); err != nil {
		return nil, err
REDACTED
	fileHeader := textproto.MIMEHeader{REDACTED
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="batch.jsonl"`)
	fileHeader.Set("Content-Type", "application/jsonl")
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, err
REDACTED
	if _, err := io.Copy(filePart, r); err != nil {
		return nil, err
REDACTED
	if err := writer.Close(); err != nil {
		return nil, err
REDACTED

	req, err := c.newRequest(ctx, http.MethodPost, "/upload/v1beta/files?uploadType=multipart", apiKey, &body)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Content-Type", writer.FormDataContentType())

	var resp struct {
		File *GeminiUploadedFile `json:"file"`
		*GeminiUploadedFile
REDACTED
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
REDACTED
	if resp.File != nil {
		return resp.File, nil
REDACTED
	return resp.GeminiUploadedFile, nil
REDACTED

func (c *GeminiBatchHTTPClient) CreateBatch(ctx context.Context, apiKey string, model string, fileName string, displayName string) (*GeminiBatchJob, error) {
	body := map[string]any{
		"batch": map[string]any{
			"displayName": displayName,
			"inputConfig": map[string]any{
				"fileName": fileName,
		REDACTED,
	REDACTED,
REDACTED
	payload, _ := json.Marshal(body)
	path := fmt.Sprintf("/v1beta/models/%s:batchGenerateContent", url.PathEscape(strings.TrimSpace(model)))
	req, err := c.newRequest(ctx, http.MethodPost, path, apiKey, bytes.NewReader(payload))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	return c.doBatchJob(req)
REDACTED

func (c *GeminiBatchHTTPClient) GetBatch(ctx context.Context, apiKey string, batchName string) (*GeminiBatchJob, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1beta/"+strings.TrimLeft(batchName, "/"), apiKey, nil)
	if err != nil {
		return nil, err
REDACTED
	return c.doBatchJob(req)
REDACTED

func (c *GeminiBatchHTTPClient) CancelBatch(ctx context.Context, apiKey string, batchName string) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/v1beta/"+strings.TrimLeft(batchName, "/")+":cancel", apiKey, nil)
	if err != nil {
		return err
REDACTED
	return c.doNoBody(req)
REDACTED

func (c *GeminiBatchHTTPClient) DownloadFile(ctx context.Context, apiKey string, fileName string) (io.ReadCloser, string, error) {
	metaReq, err := c.newRequest(ctx, http.MethodGet, "/v1beta/"+strings.TrimLeft(fileName, "/"), apiKey, nil)
	if err != nil {
		return nil, "", err
REDACTED
	var metadata struct {
		DownloadURI string `json:"downloadUri"`
		DownloadURL string `json:"download_url"`
		MimeType    string `json:"mimeType"`
REDACTED
	if err := c.doJSON(metaReq, &metadata); err != nil {
		return nil, "", err
REDACTED
	downloadURL := strings.TrimSpace(metadata.DownloadURI)
	if downloadURL == "" {
		downloadURL = strings.TrimSpace(metadata.DownloadURL)
REDACTED
	if downloadURL == "" {
		downloadURL = c.baseURL + "/v1beta/" + strings.TrimLeft(fileName, "/") + ":download"
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", err
REDACTED
	req.Header.Set("x-goog-api-key", apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
REDACTED
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() REDACTED()
		return nil, "", readGeminiAPIError(resp)
REDACTED
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = metadata.MimeType
REDACTED
	if contentType == "" {
		contentType = "application/octet-stream"
REDACTED
	return resp.Body, contentType, nil
REDACTED

func (c *GeminiBatchHTTPClient) DeleteFile(ctx context.Context, apiKey string, fileName string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/v1beta/"+strings.TrimLeft(fileName, "/"), apiKey, nil)
	if err != nil {
		return err
REDACTED
	return c.doNoBody(req)
REDACTED

func (c *GeminiBatchHTTPClient) doBatchJob(req *http.Request) (*GeminiBatchJob, error) {
	var job GeminiBatchJob
	if err := c.doJSON(req, &job); err != nil {
		return nil, err
REDACTED
	job.Raw = map[string]any{REDACTED
	return &job, nil
REDACTED

func (c *GeminiBatchHTTPClient) doNoBody(req *http.Request) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readGeminiAPIError(resp)
REDACTED
	return nil
REDACTED

func (c *GeminiBatchHTTPClient) doJSON(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readGeminiAPIError(resp)
REDACTED
	return json.NewDecoder(resp.Body).Decode(out)
REDACTED

func (c *GeminiBatchHTTPClient) newRequest(ctx context.Context, method, path, apiKey string, body io.Reader) (*http.Request, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrBatchImageProviderMissingAPIKey
REDACTED
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("x-goog-api-key", apiKey)
	return req, nil
REDACTED

type GeminiAPIError struct {
	StatusCode int
	Code       string
	Message    string
REDACTED

func (e *GeminiAPIError) Error() string {
	if e == nil {
		return "<nil>"
REDACTED
	if e.Code != "" {
		return fmt.Sprintf("gemini api error: status=%d code=%s message=%s", e.StatusCode, e.Code, e.Message)
REDACTED
	return fmt.Sprintf("gemini api error: status=%d message=%s", e.StatusCode, e.Message)
REDACTED

func readGeminiAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	message := string(body)
	var parsed struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
	REDACTED `json:"error"`
REDACTED
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
		return &GeminiAPIError{StatusCode: resp.StatusCode, Code: parsed.Error.Status, Message: messageREDACTED
REDACTED
	return &GeminiAPIError{StatusCode: resp.StatusCode, Message: messageREDACTED
REDACTED

var _ BatchImageProvider = (*GeminiAPIBatchImageProvider)(nil)
var _ GeminiBatchClient = (*GeminiBatchHTTPClient)(nil)
