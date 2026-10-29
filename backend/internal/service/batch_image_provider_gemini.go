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
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const defaultGeminiBatchRequeueAfter = 30 * time.Second

type GeminiBatchClient interface {
	UploadJSONL(ctx context.Context, apiKey string, displayName string, r io.Reader) (*GeminiUploadedFile, error)
	CreateBatch(ctx context.Context, apiKey string, model string, fileName string, displayName string) (*GeminiBatchJob, error)
	GetBatch(ctx context.Context, apiKey string, batchName string) (*GeminiBatchJob, error)
	CancelBatch(ctx context.Context, apiKey string, batchName string) error
	DownloadFile(ctx context.Context, apiKey string, fileName string) (io.ReadCloser, string, error)
	DeleteFile(ctx context.Context, apiKey string, fileName string) error
}

type GeminiUploadedFile struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	URI         string `json:"uri"`
	MimeType    string `json:"mimeType"`
}

type GeminiBatchJob struct {
	Name     string               `json:"name"`
	State    string               `json:"state"`
	Dest     *GeminiBatchDest     `json:"dest"`
	Response *GeminiBatchResponse `json:"response"`
	Error    *GeminiBatchError    `json:"error"`
	Raw      map[string]any       `json:"-"`
}

type GeminiBatchDest struct {
	FileName      string `json:"fileName"`
	FileNameSnake string `json:"file_name"`
}

type GeminiBatchResponse struct {
	ResponsesFile       string `json:"responsesFile"`
	ResponsesFileSnake  string `json:"responses_file"`
	InlinedResponses    []any  `json:"inlinedResponses"`
	InlinedResponsesAlt []any  `json:"inlined_responses"`
}

type GeminiBatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type GeminiAPIBatchImageProvider struct {
	client GeminiBatchClient
}

func NewGeminiAPIBatchImageProvider(client GeminiBatchClient) *GeminiAPIBatchImageProvider {
	if client == nil {
		client = NewGeminiBatchHTTPClient("", nil)
	}
	return &GeminiAPIBatchImageProvider{client: client}
}

func (p *GeminiAPIBatchImageProvider) Name() string {
	return BatchImageProviderGeminiAPI
}

func (p *GeminiAPIBatchImageProvider) SupportsAccount(account *Account) bool {
	return account != nil &&
		account.Platform == PlatformGemini &&
		account.Type == AccountTypeAPIKey &&
		batchImageProviderAPIKey(account) != ""
}

func (p *GeminiAPIBatchImageProvider) Submit(ctx context.Context, job *BatchImageJob, account *Account, input BatchImageInput) (*BatchProviderJob, error) {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return nil, ErrBatchImageProviderUnsupportedAccount
	}
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return nil, ErrBatchImageProviderMissingAPIKey
	}
	if input.BatchID == "" && job != nil {
		input.BatchID = job.BatchID
	}
	if input.Model == "" && job != nil {
		input.Model = job.Model
	}

	jsonl, err := BuildGeminiBatchJSONL(input)
	if err != nil {
		return nil, err
	}

	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(input.BatchID)
	}

	uploaded, err := p.client.UploadJSONL(ctx, apiKey, displayName, bytes.NewReader(jsonl))
	if err != nil {
		return nil, mapGeminiClientError(err)
	}
	if uploaded == nil || strings.TrimSpace(uploaded.Name) == "" {
		return nil, geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini upload response is missing file name", nil)
	}

	batch, err := p.client.CreateBatch(ctx, apiKey, input.Model, uploaded.Name, displayName)
	if err != nil {
		return nil, mapGeminiClientError(err)
	}
	if batch == nil || strings.TrimSpace(batch.Name) == "" {
		return nil, geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini batch response is missing job name", nil)
	}

	return &BatchProviderJob{
		ProviderJobName:  batch.Name,
		ProviderInputRef: uploaded.Name,
		RawState:         batch.State,
	}, nil
}

func (p *GeminiAPIBatchImageProvider) Get(ctx context.Context, job *BatchImageJob, account *Account) (*BatchProviderStatus, error) {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return nil, ErrBatchImageProviderUnsupportedAccount
	}
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return nil, ErrBatchImageProviderMissingAPIKey
	}
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return nil, ErrBatchImageProviderMissingJobName
	}

	batch, err := p.client.GetBatch(ctx, apiKey, jobName)
	if err != nil {
		return nil, mapGeminiClientError(err)
	}
	if batch == nil {
		return nil, geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini batch response is empty", nil)
	}

	status := mapGeminiBatchState(batch)
	if status.InternalState == BatchProviderStateSucceeded {
		if geminiBatchHasInlineResults(batch) {
			return nil, ErrBatchImageProviderInlineResultUnsupported
		}
		outputRef := geminiBatchOutputRef(batch)
		if outputRef == "" {
			status.InternalState = BatchProviderStateFailed
			status.Done = true
			status.ErrorCode = "GEMINI_RESULT_FILE_MISSING"
			status.ErrorMessage = "Gemini batch succeeded without a result file reference"
		}
		status.ProviderOutputRef = outputRef
	}
	return status, nil
}

func (p *GeminiAPIBatchImageProvider) Cancel(ctx context.Context, job *BatchImageJob, account *Account) error {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return ErrBatchImageProviderUnsupportedAccount
	}
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return ErrBatchImageProviderMissingAPIKey
	}
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return ErrBatchImageProviderMissingJobName
	}
	return mapGeminiClientError(p.client.CancelBatch(ctx, apiKey, jobName))
}

func (p *GeminiAPIBatchImageProvider) OpenResult(ctx context.Context, job *BatchImageJob, account *Account) (io.ReadCloser, string, error) {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return nil, "", ErrBatchImageProviderUnsupportedAccount
	}
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return nil, "", ErrBatchImageProviderMissingAPIKey
	}
	outputRef := batchImageProviderOutputRef(job)
	if outputRef == "" {
		return nil, "", ErrBatchImageProviderMissingResultRef
	}
	r, contentType, err := p.client.DownloadFile(ctx, apiKey, outputRef)
	return r, contentType, mapGeminiClientError(err)
}

func (p *GeminiAPIBatchImageProvider) Cleanup(ctx context.Context, job *BatchImageJob, account *Account, target CleanupTarget) error {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeAPIKey {
		return ErrBatchImageProviderUnsupportedAccount
	}
	apiKey := batchImageProviderAPIKey(account)
	if apiKey == "" {
		return ErrBatchImageProviderMissingAPIKey
	}

	switch target {
	case CleanupTargetInput:
		return p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderInputRef(job))
	case CleanupTargetOutput:
		return p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderOutputRef(job))
	case CleanupTargetAll:
		if err := p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderInputRef(job)); err != nil {
			return err
		}
		return p.deleteGeminiFileIfPresent(ctx, apiKey, batchImageProviderOutputRef(job))
	default:
		return ErrUnsupportedCleanupTarget
	}
}

func (p *GeminiAPIBatchImageProvider) deleteGeminiFileIfPresent(ctx context.Context, apiKey, fileName string) error {
	if strings.TrimSpace(fileName) == "" {
		return nil
	}
	return mapGeminiClientError(p.client.DeleteFile(ctx, apiKey, fileName))
}

type geminiJSONLLine struct {
	Key     string                `json:"key"`
	Request geminiGenerateRequest `json:"request"`
}

type geminiGenerateRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
	FileData   *geminiFileData   `json:"fileData,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type geminiGenerationConfig struct {
	ResponseModalities []string `json:"responseModalities"`
}

func BuildGeminiBatchJSONL(input BatchImageInput) ([]byte, error) {
	if strings.TrimSpace(input.Model) == "" {
		return nil, batchImageProviderInputError("model is required")
	}
	if len(input.Items) == 0 {
		return nil, batchImageProviderInputError("at least one item is required")
	}

	seen := make(map[string]struct{}, len(input.Items))
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, item := range input.Items {
		customID := strings.TrimSpace(item.CustomID)
		if customID == "" {
			return nil, batchImageProviderInputError("custom_id is required")
		}
		if _, ok := seen[customID]; ok {
			return nil, batchImageProviderInputError("duplicate custom_id %q", customID)
		}
		seen[customID] = struct{}{}

		prompt := strings.TrimSpace(item.Prompt)
		if prompt == "" {
			return nil, batchImageProviderInputError("prompt is required for custom_id %q", customID)
		}
		parts, err := batchImageGeminiParts(prompt, item.ReferenceImages)
		if err != nil {
			return nil, err
		}

		// TODO(batch-image): add response_mime_type/aspect_ratio/image_size once the
		// Gemini batch image REST shape is stabilized for those options.
		line := geminiJSONLLine{
			Key: customID,
			Request: geminiGenerateRequest{
				Contents: []geminiContent{{
					Parts: parts,
				}},
				GenerationConfig: geminiGenerationConfig{
					ResponseModalities: []string{"TEXT", "IMAGE"},
				},
			},
		}
		if err := enc.Encode(line); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func batchImageGeminiParts(prompt string, refs []BatchImageReference) ([]geminiPart, error) {
	parts := []geminiPart{{Text: prompt}}
	for _, ref := range refs {
		mimeType := normalizeBatchImageReferenceMimeType(ref.MimeType)
		if mimeType == "" {
			return nil, batchImageProviderInputError("reference image mime_type is required")
		}
		fileURI := strings.TrimSpace(ref.FileURI)
		switch {
		case len(ref.Data) > 0 && fileURI == "":
			parts = append(parts, geminiPart{InlineData: &geminiInlineData{
				MimeType: mimeType,
				Data:     base64.StdEncoding.EncodeToString(ref.Data),
			}})
		case len(ref.Data) == 0 && fileURI != "":
			parts = append(parts, geminiPart{FileData: &geminiFileData{
				MimeType: mimeType,
				FileURI:  fileURI,
			}})
		default:
			return nil, batchImageProviderInputError("reference image must contain exactly one of data or file_uri")
		}
	}
	return parts, nil
}

func mapGeminiBatchState(batch *GeminiBatchJob) *BatchProviderStatus {
	state := strings.TrimSpace(batch.State)
	normalized := strings.ToUpper(state)
	status := &BatchProviderStatus{
		RawState:              state,
		InternalState:         BatchProviderStateRunning,
		SuggestedRequeueAfter: defaultGeminiBatchRequeueAfter,
	}

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
		}
	}

	if batch.Error != nil {
		if code := strings.TrimSpace(batch.Error.Code); code != "" {
			status.ErrorCode = code
		} else if status.ErrorCode == "" && strings.TrimSpace(batch.Error.Status) != "" {
			status.ErrorCode = strings.TrimSpace(batch.Error.Status)
		}
		status.ErrorMessage = strings.TrimSpace(batch.Error.Message)
	}
	return status
}

func geminiBatchOutputRef(batch *GeminiBatchJob) string {
	if batch == nil {
		return ""
	}
	if batch.Dest != nil {
		if v := strings.TrimSpace(batch.Dest.FileName); v != "" {
			return v
		}
		if v := strings.TrimSpace(batch.Dest.FileNameSnake); v != "" {
			return v
		}
	}
	if batch.Response != nil {
		if v := strings.TrimSpace(batch.Response.ResponsesFile); v != "" {
			return v
		}
		if v := strings.TrimSpace(batch.Response.ResponsesFileSnake); v != "" {
			return v
		}
	}
	return ""
}

func geminiBatchHasInlineResults(batch *GeminiBatchJob) bool {
	return batch != nil &&
		batch.Response != nil &&
		(len(batch.Response.InlinedResponses) > 0 || len(batch.Response.InlinedResponsesAlt) > 0)
}

func geminiProviderError(reason, message string, cause error) error {
	err := infraerrors.New(http.StatusBadGateway, reason, message)
	if cause != nil {
		return err.WithCause(cause)
	}
	return err
}

func mapGeminiClientError(err error) error {
	if err == nil {
		return nil
	}
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
		}
	}
	return geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini API request failed", nil)
}

type GeminiBatchHTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewGeminiBatchHTTPClient(baseURL string, client *http.Client) *GeminiBatchHTTPClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = geminicli.AIStudioBaseURL
	}
	if client == nil {
		client = batchImageDefaultHTTPClient()
	}
	return &GeminiBatchHTTPClient{baseURL: baseURL, client: client}
}

// batchImageDefaultHTTPClient 返回带连接/握手/响应头超时的共享客户端。
// 不设整体 Timeout：大文件上传与结果流式下载耗时不可预估，
// 但拨号、TLS、等待响应头必须有界，否则挂死的连接会无限占用提交路径。
func batchImageDefaultHTTPClient() *http.Client {
	client, err := httpclient.GetClient(httpclient.Options{
		ResponseHeaderTimeout: 60 * time.Second,
	})
	if err != nil {
		return http.DefaultClient
	}
	return client
}

func (c *GeminiBatchHTTPClient) UploadJSONL(ctx context.Context, apiKey string, displayName string, r io.Reader) (*GeminiUploadedFile, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadataHeader := textproto.MIMEHeader{}
	metadataHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metadataHeader.Set("Content-Type", "application/json; charset=utf-8")
	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return nil, err
	}
	metadata := map[string]any{"file": map[string]any{"displayName": displayName, "mimeType": "application/jsonl"}}
	if err := json.NewEncoder(metadataPart).Encode(metadata); err != nil {
		return nil, err
	}
	fileHeader := textproto.MIMEHeader{}
	fileHeader.Set("Content-Disposition", `form-data; name="file"; filename="batch.jsonl"`)
	fileHeader.Set("Content-Type", "application/jsonl")
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(filePart, r); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/upload/v1beta/files?uploadType=multipart", apiKey, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	var resp struct {
		File *GeminiUploadedFile `json:"file"`
		*GeminiUploadedFile
	}
	if err := c.doJSON(req, &resp); err != nil {
		return nil, err
	}
	if resp.File != nil {
		return resp.File, nil
	}
	return resp.GeminiUploadedFile, nil
}

func (c *GeminiBatchHTTPClient) CreateBatch(ctx context.Context, apiKey string, model string, fileName string, displayName string) (*GeminiBatchJob, error) {
	body := map[string]any{
		"batch": map[string]any{
			"displayName": displayName,
			"inputConfig": map[string]any{
				"fileName": fileName,
			},
		},
	}
	payload, _ := json.Marshal(body)
	path := fmt.Sprintf("/v1beta/models/%s:batchGenerateContent", url.PathEscape(strings.TrimSpace(model)))
	req, err := c.newRequest(ctx, http.MethodPost, path, apiKey, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doBatchJob(req)
}

func (c *GeminiBatchHTTPClient) GetBatch(ctx context.Context, apiKey string, batchName string) (*GeminiBatchJob, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1beta/"+strings.TrimLeft(batchName, "/"), apiKey, nil)
	if err != nil {
		return nil, err
	}
	return c.doBatchJob(req)
}

func (c *GeminiBatchHTTPClient) CancelBatch(ctx context.Context, apiKey string, batchName string) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/v1beta/"+strings.TrimLeft(batchName, "/")+":cancel", apiKey, nil)
	if err != nil {
		return err
	}
	return c.doNoBody(req)
}

func (c *GeminiBatchHTTPClient) DownloadFile(ctx context.Context, apiKey string, fileName string) (io.ReadCloser, string, error) {
	metaReq, err := c.newRequest(ctx, http.MethodGet, "/v1beta/"+strings.TrimLeft(fileName, "/"), apiKey, nil)
	if err != nil {
		return nil, "", err
	}
	var metadata struct {
		DownloadURI string `json:"downloadUri"`
		DownloadURL string `json:"download_url"`
		MimeType    string `json:"mimeType"`
	}
	if err := c.doJSON(metaReq, &metadata); err != nil {
		return nil, "", err
	}
	downloadURL := strings.TrimSpace(metadata.DownloadURI)
	if downloadURL == "" {
		downloadURL = strings.TrimSpace(metadata.DownloadURL)
	}
	if downloadURL == "" {
		downloadURL = c.baseURL + "/v1beta/" + strings.TrimLeft(fileName, "/") + ":download"
	}
	// 纵深加固：downloadUri 来自上游响应，跟随前校验目标 host，
	// 防止异常/被劫持的响应把带 api key 的请求带到任意主机。
	if err := validateGeminiDownloadHost(downloadURL, c.baseURL); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("x-goog-api-key", apiKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		return nil, "", readGeminiAPIError(resp)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = metadata.MimeType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return resp.Body, contentType, nil
}

func (c *GeminiBatchHTTPClient) DeleteFile(ctx context.Context, apiKey string, fileName string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/v1beta/"+strings.TrimLeft(fileName, "/"), apiKey, nil)
	if err != nil {
		return err
	}
	return c.doNoBody(req)
}

func (c *GeminiBatchHTTPClient) doBatchJob(req *http.Request) (*GeminiBatchJob, error) {
	var job GeminiBatchJob
	if err := c.doJSON(req, &job); err != nil {
		return nil, err
	}
	job.Raw = map[string]any{}
	return &job, nil
}

func (c *GeminiBatchHTTPClient) doNoBody(req *http.Request) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readGeminiAPIError(resp)
	}
	return nil
}

func (c *GeminiBatchHTTPClient) doJSON(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readGeminiAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *GeminiBatchHTTPClient) newRequest(ctx context.Context, method, path, apiKey string, body io.Reader) (*http.Request, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, ErrBatchImageProviderMissingAPIKey
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", apiKey)
	return req, nil
}

// validateGeminiDownloadHost 只允许跟随到 googleapis.com（含子域）
// 或与配置的 baseURL 同 host 的下载地址。
func validateGeminiDownloadHost(downloadURL, baseURL string) error {
	parsed, err := url.Parse(downloadURL)
	if err != nil {
		return geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini download uri is invalid", err)
	}
	if parsed.Scheme != "https" {
		return geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini download uri must use https", nil)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "googleapis.com" || strings.HasSuffix(host, ".googleapis.com") {
		return nil
	}
	if base, err := url.Parse(baseURL); err == nil && strings.EqualFold(base.Hostname(), host) {
		return nil
	}
	return geminiProviderError("GEMINI_INVALID_RESPONSE", "Gemini download uri host is not allowed", nil)
}

type GeminiAPIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *GeminiAPIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code != "" {
		return fmt.Sprintf("gemini api error: status=%d code=%s message=%s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("gemini api error: status=%d message=%s", e.StatusCode, e.Message)
}

func readGeminiAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	message := string(body)
	var parsed struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
		return &GeminiAPIError{StatusCode: resp.StatusCode, Code: parsed.Error.Status, Message: message}
	}
	return &GeminiAPIError{StatusCode: resp.StatusCode, Message: message}
}

var _ BatchImageProvider = (*GeminiAPIBatchImageProvider)(nil)
var _ GeminiBatchClient = (*GeminiBatchHTTPClient)(nil)
