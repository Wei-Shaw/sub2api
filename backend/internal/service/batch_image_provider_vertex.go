package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	defaultVertexBatchRequeueAfter = 30 * time.Second
	defaultVertexBatchLocation     = "global"
	defaultVertexManagedGCSPrefix  = "batch-image/{env}/{batch_id}"
)

type VertexBatchImageProviderOptions struct {
	Enabled                bool
	ProjectID              string
	Location               string
	ManagedGCSBucket       string
	ManagedGCSPrefix       string
	Environment            string
	InputRetentionHours    int
	OutputRetentionHours   int
	BatchPredictionBaseURL string
	GCSBaseURL             string
}

func NewVertexBatchImageProviderOptionsFromConfig(cfg *config.Config) VertexBatchImageProviderOptions {
	if cfg == nil {
		return VertexBatchImageProviderOptions{}
	}
	return VertexBatchImageProviderOptions{
		Enabled:                cfg.BatchImage.VertexEnabled,
		ProjectID:              cfg.BatchImage.VertexProjectID,
		Location:               cfg.BatchImage.VertexLocation,
		ManagedGCSBucket:       cfg.BatchImage.VertexManagedGCSBucket,
		ManagedGCSPrefix:       cfg.BatchImage.VertexManagedGCSPrefix,
		Environment:            cfg.Log.Environment,
		InputRetentionHours:    cfg.BatchImage.VertexInputRetentionHours,
		OutputRetentionHours:   cfg.BatchImage.VertexOutputRetentionHours,
		BatchPredictionBaseURL: cfg.BatchImage.VertexBatchPredictionBaseURL,
		GCSBaseURL:             cfg.BatchImage.VertexGCSBaseURL,
	}
}

type VertexBatchClient interface {
	CreateBatchPredictionJob(ctx context.Context, accessToken string, req VertexCreateBatchPredictionJobRequest) (*VertexBatchPredictionJob, error)
	GetBatchPredictionJob(ctx context.Context, accessToken string, name string) (*VertexBatchPredictionJob, error)
	CancelBatchPredictionJob(ctx context.Context, accessToken string, name string) error
}

type VertexBatchObjectStore interface {
	UploadJSONL(ctx context.Context, accessToken string, uri string, r io.Reader) error
	ListJSONLObjects(ctx context.Context, accessToken string, prefixURI string) ([]string, error)
	OpenObject(ctx context.Context, accessToken string, uri string) (io.ReadCloser, string, error)
	DeleteObject(ctx context.Context, accessToken string, uri string) error
	DeletePrefix(ctx context.Context, accessToken string, prefixURI string) error
}

type VertexCreateBatchPredictionJobRequest struct {
	ProjectID      string                     `json:"-"`
	Location       string                     `json:"-"`
	DisplayName    string                     `json:"displayName"`
	Model          string                     `json:"model"`
	InputConfig    VertexBatchInputConfig     `json:"inputConfig"`
	OutputConfig   VertexBatchOutputConfig    `json:"outputConfig"`
	InstanceConfig *VertexBatchInstanceConfig `json:"instanceConfig,omitempty"`
}

type VertexBatchInputConfig struct {
	InstancesFormat string               `json:"instancesFormat"`
	GCSSource       VertexBatchGCSSource `json:"gcsSource"`
}

type VertexBatchGCSSource struct {
	URIs []string `json:"uris"`
}

type VertexBatchOutputConfig struct {
	PredictionsFormat string                    `json:"predictionsFormat"`
	GCSDestination    VertexBatchGCSDestination `json:"gcsDestination"`
}

type VertexBatchGCSDestination struct {
	OutputURIPrefix string `json:"outputUriPrefix"`
}

type VertexBatchInstanceConfig struct {
	KeyField string `json:"keyField"`
}

type VertexBatchPredictionJob struct {
	Name         string                  `json:"name"`
	DisplayName  string                  `json:"displayName"`
	State        string                  `json:"state"`
	OutputConfig VertexBatchOutputConfig `json:"outputConfig"`
	Error        *VertexBatchJobError    `json:"error"`
}

type VertexBatchJobError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type VertexBatchImageProvider struct {
	opts        VertexBatchImageProviderOptions
	client      VertexBatchClient
	objectStore VertexBatchObjectStore
	tokenCache  GeminiTokenCache
}

func NewVertexBatchImageProvider(opts VertexBatchImageProviderOptions, client VertexBatchClient, objectStore VertexBatchObjectStore, tokenCache GeminiTokenCache) *VertexBatchImageProvider {
	opts = normalizeVertexBatchImageProviderOptions(opts)
	if client == nil {
		client = NewVertexBatchHTTPClient(opts.BatchPredictionBaseURL, nil)
	}
	if objectStore == nil {
		objectStore = NewVertexGCSObjectStore(opts.GCSBaseURL, nil)
	}
	return &VertexBatchImageProvider{
		opts:        opts,
		client:      client,
		objectStore: objectStore,
		tokenCache:  tokenCache,
	}
}

func NewVertexBatchImageProviderFromConfig(cfg *config.Config, client VertexBatchClient, objectStore VertexBatchObjectStore, tokenCache GeminiTokenCache) *VertexBatchImageProvider {
	return NewVertexBatchImageProvider(NewVertexBatchImageProviderOptionsFromConfig(cfg), client, objectStore, tokenCache)
}

func normalizeVertexBatchImageProviderOptions(opts VertexBatchImageProviderOptions) VertexBatchImageProviderOptions {
	opts.ProjectID = strings.TrimSpace(opts.ProjectID)
	opts.Location = strings.TrimSpace(opts.Location)
	if opts.Location == "" {
		opts.Location = defaultVertexBatchLocation
	}
	opts.ManagedGCSBucket = strings.Trim(strings.TrimSpace(opts.ManagedGCSBucket), "/")
	opts.ManagedGCSPrefix = strings.Trim(strings.TrimSpace(opts.ManagedGCSPrefix), "/")
	if opts.ManagedGCSPrefix == "" {
		opts.ManagedGCSPrefix = defaultVertexManagedGCSPrefix
	}
	opts.Environment = strings.TrimSpace(opts.Environment)
	if opts.Environment == "" {
		opts.Environment = "default"
	}
	opts.BatchPredictionBaseURL = strings.TrimRight(strings.TrimSpace(opts.BatchPredictionBaseURL), "/")
	opts.GCSBaseURL = strings.TrimRight(strings.TrimSpace(opts.GCSBaseURL), "/")
	return opts
}

func (p *VertexBatchImageProvider) Name() string {
	return BatchImageProviderVertex
}

func (p *VertexBatchImageProvider) SupportsAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeServiceAccount {
		return false
	}
	_, err := parseVertexServiceAccountKey(account)
	return err == nil
}

func (p *VertexBatchImageProvider) Submit(ctx context.Context, job *BatchImageJob, account *Account, input BatchImageInput) (*BatchProviderJob, error) {
	if err := p.validateAccount(account); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.opts.ManagedGCSBucket) == "" {
		return nil, vertexProviderError("VERTEX_MANAGED_GCS_BUCKET_MISSING", "Vertex managed GCS bucket is not configured", nil)
	}
	if input.BatchID == "" && job != nil {
		input.BatchID = job.BatchID
	}
	if input.Model == "" && job != nil {
		input.Model = job.Model
	}

	jsonl, err := BuildVertexBatchJSONL(input)
	if err != nil {
		return nil, err
	}
	refs, err := p.managedRefs(input.BatchID)
	if err != nil {
		return nil, err
	}

	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return nil, mapVertexClientError(err)
	}
	if err := p.objectStore.UploadJSONL(ctx, accessToken, refs.InputURI, bytes.NewReader(jsonl)); err != nil {
		return nil, vertexProviderError("VERTEX_GCS_UPLOAD_FAILED", "Vertex managed GCS upload failed", nil)
	}

	projectID := strings.TrimSpace(p.opts.ProjectID)
	if projectID == "" {
		projectID = account.VertexProjectID()
	}
	if projectID == "" {
		return nil, vertexProviderError("VERTEX_PROJECT_ID_MISSING", "Vertex project id is not configured", nil)
	}
	location := strings.TrimSpace(p.opts.Location)
	if location == "" {
		location = account.VertexLocation(input.Model)
	}

	req := VertexCreateBatchPredictionJobRequest{
		ProjectID:      projectID,
		Location:       location,
		DisplayName:    vertexBatchDisplayName(input),
		Model:          NormalizeVertexBatchModelPath(input.Model),
		InputConfig:    VertexBatchInputConfig{InstancesFormat: "jsonl", GCSSource: VertexBatchGCSSource{URIs: []string{refs.InputURI}}},
		OutputConfig:   VertexBatchOutputConfig{PredictionsFormat: "jsonl", GCSDestination: VertexBatchGCSDestination{OutputURIPrefix: refs.OutputPrefixURI}},
		InstanceConfig: &VertexBatchInstanceConfig{KeyField: "key"},
	}
	created, err := p.client.CreateBatchPredictionJob(ctx, accessToken, req)
	if err != nil {
		return nil, mapVertexClientError(err)
	}
	if created == nil || strings.TrimSpace(created.Name) == "" {
		return nil, vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex batch response is missing job name", nil)
	}
	return &BatchProviderJob{
		ProviderJobName:   created.Name,
		ProviderInputRef:  refs.InputURI,
		ProviderOutputRef: refs.OutputPrefixURI,
		RawState:          created.State,
	}, nil
}

func (p *VertexBatchImageProvider) Get(ctx context.Context, job *BatchImageJob, account *Account) (*BatchProviderStatus, error) {
	if err := p.validateAccount(account); err != nil {
		return nil, err
	}
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return nil, ErrBatchImageProviderMissingJobName
	}
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return nil, mapVertexClientError(err)
	}
	vertexJob, err := p.client.GetBatchPredictionJob(ctx, accessToken, jobName)
	if err != nil {
		return nil, mapVertexClientError(err)
	}
	if vertexJob == nil {
		return nil, vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex batch response is empty", nil)
	}
	status := mapVertexBatchState(vertexJob)
	outputRef := strings.TrimSpace(vertexJob.OutputConfig.GCSDestination.OutputURIPrefix)
	if outputRef == "" {
		outputRef = batchImageProviderOutputRef(job)
	}
	if outputRef == "" && job != nil && job.GCSOutputURI != nil {
		outputRef = strings.TrimSpace(*job.GCSOutputURI)
	}
	status.ProviderOutputRef = outputRef
	return status, nil
}

func (p *VertexBatchImageProvider) Cancel(ctx context.Context, job *BatchImageJob, account *Account) error {
	if err := p.validateAccount(account); err != nil {
		return err
	}
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return ErrBatchImageProviderMissingJobName
	}
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return mapVertexClientError(err)
	}
	return mapVertexClientError(p.client.CancelBatchPredictionJob(ctx, accessToken, jobName))
}

func (p *VertexBatchImageProvider) OpenResult(ctx context.Context, job *BatchImageJob, account *Account) (io.ReadCloser, string, error) {
	if err := p.validateAccount(account); err != nil {
		return nil, "", err
	}
	outputRef := batchImageProviderOutputRef(job)
	if outputRef == "" && job != nil && job.GCSOutputURI != nil {
		outputRef = strings.TrimSpace(*job.GCSOutputURI)
	}
	if outputRef == "" {
		return nil, "", ErrBatchImageProviderMissingResultRef
	}
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return nil, "", mapVertexClientError(err)
	}
	objects, err := p.objectStore.ListJSONLObjects(ctx, accessToken, outputRef)
	if err != nil {
		return nil, "", vertexProviderError("VERTEX_GCS_LIST_FAILED", "Vertex managed GCS list failed", nil)
	}
	sort.Strings(objects)
	if len(objects) == 0 {
		return nil, "", vertexProviderError("VERTEX_RESULT_OBJECTS_MISSING", "Vertex result objects are missing", nil)
	}
	return &vertexCombinedJSONLReadCloser{
		ctx:         ctx,
		accessToken: accessToken,
		objects:     objects,
		store:       p.objectStore,
	}, "application/jsonl", nil
}

func (p *VertexBatchImageProvider) Cleanup(ctx context.Context, job *BatchImageJob, account *Account, target CleanupTarget) error {
	if err := p.validateAccount(account); err != nil {
		return err
	}
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return mapVertexClientError(err)
	}
	inputRef := batchImageProviderInputRef(job)
	outputRef := batchImageProviderOutputRef(job)
	if job != nil {
		if inputRef == "" && job.GCSInputURI != nil {
			inputRef = strings.TrimSpace(*job.GCSInputURI)
		}
		if outputRef == "" && job.GCSOutputURI != nil {
			outputRef = strings.TrimSpace(*job.GCSOutputURI)
		}
	}

	switch target {
	case CleanupTargetInput:
		return p.deleteManagedInput(ctx, accessToken, job, inputRef)
	case CleanupTargetOutput:
		return p.deleteManagedOutput(ctx, accessToken, job, outputRef)
	case CleanupTargetAll:
		if err := p.deleteManagedInput(ctx, accessToken, job, inputRef); err != nil {
			return err
		}
		return p.deleteManagedOutput(ctx, accessToken, job, outputRef)
	default:
		return ErrUnsupportedCleanupTarget
	}
}

func (p *VertexBatchImageProvider) validateAccount(account *Account) error {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeServiceAccount {
		return ErrBatchImageProviderUnsupportedAccount
	}
	if _, err := parseVertexServiceAccountKey(account); err != nil {
		return ErrBatchImageProviderMissingServiceAccount
	}
	return nil
}

func (p *VertexBatchImageProvider) accessToken(ctx context.Context, account *Account) (string, error) {
	return getVertexServiceAccountAccessToken(ctx, p.tokenCache, account)
}

func (p *VertexBatchImageProvider) deleteManagedInput(ctx context.Context, accessToken string, job *BatchImageJob, uri string) error {
	if strings.TrimSpace(uri) == "" {
		return nil
	}
	if !p.isSafeManagedInput(job, uri) {
		return ErrBatchImageProviderUnsafeCleanupPath
	}
	return mapVertexClientError(p.objectStore.DeleteObject(ctx, accessToken, uri))
}

func (p *VertexBatchImageProvider) deleteManagedOutput(ctx context.Context, accessToken string, job *BatchImageJob, uri string) error {
	if strings.TrimSpace(uri) == "" {
		return nil
	}
	if !p.isSafeManagedOutput(job, uri) {
		return ErrBatchImageProviderUnsafeCleanupPath
	}
	return mapVertexClientError(p.objectStore.DeletePrefix(ctx, accessToken, uri))
}

func (p *VertexBatchImageProvider) isSafeManagedInput(job *BatchImageJob, uri string) bool {
	if job == nil || strings.TrimSpace(job.BatchID) == "" {
		return false
	}
	refs, err := p.managedRefs(job.BatchID)
	return err == nil && strings.TrimSpace(uri) == refs.InputURI
}

func (p *VertexBatchImageProvider) isSafeManagedOutput(job *BatchImageJob, uri string) bool {
	if job == nil || strings.TrimSpace(job.BatchID) == "" {
		return false
	}
	refs, err := p.managedRefs(job.BatchID)
	return err == nil && strings.HasPrefix(strings.TrimSpace(uri), refs.OutputPrefixURI)
}

type vertexManagedRefs struct {
	Prefix          string
	InputURI        string
	OutputPrefixURI string
}

func (p *VertexBatchImageProvider) managedRefs(batchID string) (vertexManagedRefs, error) {
	batchID = strings.TrimSpace(batchID)
	if !IsValidBatchImageID(batchID) {
		return vertexManagedRefs{}, batchImageProviderInputError("valid batch_id is required")
	}
	bucket := strings.Trim(strings.TrimSpace(p.opts.ManagedGCSBucket), "/")
	if bucket == "" || strings.Contains(bucket, "://") {
		return vertexManagedRefs{}, vertexProviderError("VERTEX_MANAGED_GCS_BUCKET_MISSING", "Vertex managed GCS bucket is not configured", nil)
	}
	prefix := buildVertexManagedGCSPrefix(p.opts.ManagedGCSPrefix, p.opts.Environment, batchID)
	if !strings.Contains(prefix, batchID) {
		return vertexManagedRefs{}, batchImageProviderInputError("managed GCS prefix must contain batch_id")
	}
	base := "gs://" + bucket + "/" + strings.Trim(prefix, "/")
	return vertexManagedRefs{
		Prefix:          strings.Trim(prefix, "/"),
		InputURI:        base + "/input/requests.jsonl",
		OutputPrefixURI: base + "/output/",
	}, nil
}

func buildVertexManagedGCSPrefix(template, env, batchID string) string {
	template = strings.Trim(strings.TrimSpace(template), "/")
	if template == "" {
		template = defaultVertexManagedGCSPrefix
	}
	env = sanitizeVertexGCSPathSegment(env)
	batchID = sanitizeVertexGCSPathSegment(batchID)
	prefix := strings.ReplaceAll(template, "{env}", env)
	prefix = strings.ReplaceAll(prefix, "{batch_id}", batchID)
	return strings.Trim(prefix, "/")
}

func sanitizeVertexGCSPathSegment(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			_, _ = b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			_, _ = b.WriteRune(r)
		default:
			_ = b.WriteByte('-')
		}
	}
	return b.String()
}

func vertexBatchDisplayName(input BatchImageInput) string {
	if v := strings.TrimSpace(input.DisplayName); v != "" {
		return v
	}
	if v := strings.TrimSpace(input.BatchID); v != "" {
		return "sub2api-" + v
	}
	return "sub2api-image-batch"
}

func BuildVertexBatchJSONL(input BatchImageInput) ([]byte, error) {
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
		parts, err := vertexBatchImageParts(prompt, item.ReferenceImages)
		if err != nil {
			return nil, err
		}
		line := map[string]any{
			"key": customID,
			"request": map[string]any{
				"contents": []any{map[string]any{
					"role":  "user",
					"parts": parts,
				}},
				"generationConfig": map[string]any{
					"responseModalities": []string{"TEXT", "IMAGE"},
				},
			},
		}
		if err := enc.Encode(line); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func vertexBatchImageParts(prompt string, refs []BatchImageReference) ([]any, error) {
	parts := []any{map[string]any{"text": prompt}}
	for _, ref := range refs {
		mimeType := normalizeBatchImageReferenceMimeType(ref.MimeType)
		if mimeType == "" {
			return nil, batchImageProviderInputError("reference image mime_type is required")
		}
		fileURI := strings.TrimSpace(ref.FileURI)
		switch {
		case len(ref.Data) > 0 && fileURI == "":
			parts = append(parts, map[string]any{
				"inlineData": map[string]any{
					"mimeType": mimeType,
					"data":     base64.StdEncoding.EncodeToString(ref.Data),
				},
			})
		case len(ref.Data) == 0 && fileURI != "":
			parts = append(parts, map[string]any{
				"fileData": map[string]any{
					"mimeType": mimeType,
					"fileUri":  fileURI,
				},
			})
		default:
			return nil, batchImageProviderInputError("reference image must contain exactly one of data or file_uri")
		}
	}
	return parts, nil
}

func NormalizeVertexBatchModelPath(model string) string {
	model = strings.Trim(strings.TrimSpace(model), "/")
	if strings.HasPrefix(model, "publishers/") || strings.HasPrefix(model, "projects/") {
		return model
	}
	return "publishers/google/models/" + model
}

func BuildVertexBatchPredictionJobsEndpoint(baseURL, projectID, location string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	location = strings.TrimSpace(location)
	if projectID == "" {
		return "", errors.New("vertex project_id is required")
	}
	if location == "" {
		location = defaultVertexBatchLocation
	}
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
	}
	if strings.TrimSpace(baseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/projects/" + url.PathEscape(projectID) + "/locations/" + url.PathEscape(location) + "/batchPredictionJobs", nil
	}
	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
	}
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/batchPredictionJobs", host, url.PathEscape(projectID), url.PathEscape(location)), nil
}

func mapVertexBatchState(job *VertexBatchPredictionJob) *BatchProviderStatus {
	state := strings.TrimSpace(job.State)
	status := &BatchProviderStatus{
		RawState:              state,
		InternalState:         BatchProviderStateRunning,
		SuggestedRequeueAfter: defaultVertexBatchRequeueAfter,
	}
	switch strings.ToUpper(state) {
	case "JOB_STATE_PENDING", "JOB_STATE_QUEUED":
		status.InternalState = BatchProviderStateQueued
	case "JOB_STATE_RUNNING", "JOB_STATE_PAUSED":
		status.InternalState = BatchProviderStateRunning
	case "JOB_STATE_SUCCEEDED":
		status.InternalState = BatchProviderStateSucceeded
		status.Done = true
		status.SuggestedRequeueAfter = 0
	case "JOB_STATE_FAILED":
		status.InternalState = BatchProviderStateFailed
		status.Done = true
		status.ErrorCode = "VERTEX_BATCH_FAILED"
		status.SuggestedRequeueAfter = 0
	case "JOB_STATE_CANCELLED":
		status.InternalState = BatchProviderStateCancelled
		status.Done = true
		status.ErrorCode = "VERTEX_BATCH_CANCELLED"
		status.SuggestedRequeueAfter = 0
	case "JOB_STATE_EXPIRED":
		status.InternalState = BatchProviderStateExpired
		status.Done = true
		status.ErrorCode = "VERTEX_BATCH_EXPIRED"
		status.SuggestedRequeueAfter = 0
	default:
		if job.Error != nil && strings.TrimSpace(job.Error.Message) != "" {
			status.InternalState = BatchProviderStateFailed
			status.Done = true
			status.ErrorCode = "VERTEX_BATCH_FAILED"
			status.SuggestedRequeueAfter = 0
		}
	}
	if job.Error != nil {
		if code := strings.TrimSpace(job.Error.Status); code != "" {
			status.ErrorCode = code
		}
		status.ErrorMessage = strings.TrimSpace(job.Error.Message)
	}
	return status
}

func vertexProviderError(reason, message string, cause error) error {
	err := infraerrors.New(http.StatusBadGateway, reason, message)
	if cause != nil {
		return err.WithCause(cause)
	}
	return err
}

func mapVertexClientError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrBatchImageProviderMissingServiceAccount) ||
		errors.Is(err, ErrBatchImageProviderMissingJobName) ||
		errors.Is(err, ErrBatchImageProviderMissingResultRef) ||
		errors.Is(err, ErrBatchImageProviderUnsafeCleanupPath) ||
		errors.Is(err, ErrUnsupportedCleanupTarget) {
		return err
	}
	var apiErr *VertexAPIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized:
			return vertexProviderError("VERTEX_AUTH_FAILED", "Vertex authentication failed", nil)
		case http.StatusForbidden:
			return vertexProviderError("VERTEX_PERMISSION_DENIED", "Vertex permission denied", nil)
		case http.StatusTooManyRequests:
			return vertexProviderError("VERTEX_RATE_LIMITED", "Vertex rate limit exceeded", nil)
		case http.StatusNotFound:
			return vertexProviderError("VERTEX_BATCH_NOT_FOUND", "Vertex batch resource was not found", nil)
		default:
			return vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex API request failed", nil)
		}
	}
	return vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex API request failed", err)
}

type vertexCombinedJSONLReadCloser struct {
	ctx          context.Context
	accessToken  string
	objects      []string
	store        VertexBatchObjectStore
	index        int
	current      io.ReadCloser
	needBoundary bool
	closed       bool
}

func (r *vertexCombinedJSONLReadCloser) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if r.needBoundary {
		if len(p) == 0 {
			return 0, nil
		}
		p[0] = '\n'
		r.needBoundary = false
		return 1, nil
	}
	for {
		if r.current == nil {
			if r.index >= len(r.objects) {
				return 0, io.EOF
			}
			obj := r.objects[r.index]
			r.index++
			rc, _, err := r.store.OpenObject(r.ctx, r.accessToken, obj)
			if err != nil {
				return 0, err
			}
			r.current = rc
		}
		n, err := r.current.Read(p)
		if err == io.EOF {
			_ = r.current.Close()
			r.current = nil
			if r.index < len(r.objects) {
				if n > 0 {
					r.needBoundary = true
					return n, nil
				}
				if len(p) == 0 {
					return 0, nil
				}
				p[0] = '\n'
				return 1, nil
			}
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (r *vertexCombinedJSONLReadCloser) Close() error {
	r.closed = true
	if r.current != nil {
		return r.current.Close()
	}
	return nil
}

type VertexBatchHTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewVertexBatchHTTPClient(baseURL string, client *http.Client) *VertexBatchHTTPClient {
	if client == nil {
		client = batchImageDefaultHTTPClient()
	}
	return &VertexBatchHTTPClient{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), client: client}
}

func (c *VertexBatchHTTPClient) CreateBatchPredictionJob(ctx context.Context, accessToken string, req VertexCreateBatchPredictionJobRequest) (*VertexBatchPredictionJob, error) {
	endpoint, err := BuildVertexBatchPredictionJobsEndpoint(c.baseURL, req.ProjectID, req.Location)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexJSON[VertexBatchPredictionJob](c.client, httpReq)
}

func (c *VertexBatchHTTPClient) GetBatchPredictionJob(ctx context.Context, accessToken string, name string) (*VertexBatchPredictionJob, error) {
	endpoint := c.vertexResourceURL(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexJSON[VertexBatchPredictionJob](c.client, req)
}

func (c *VertexBatchHTTPClient) CancelBatchPredictionJob(ctx context.Context, accessToken string, name string) error {
	endpoint := c.vertexResourceURL(name) + ":cancel"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexNoBody(c.client, req)
}

func (c *VertexBatchHTTPClient) vertexResourceURL(name string) string {
	name = strings.TrimLeft(strings.TrimSpace(name), "/")
	if c.baseURL != "" {
		return c.baseURL + "/v1/" + name
	}
	return "https://aiplatform.googleapis.com/v1/" + name
}

type VertexGCSObjectStore struct {
	baseURL string
	client  *http.Client
}

func NewVertexGCSObjectStore(baseURL string, client *http.Client) *VertexGCSObjectStore {
	if client == nil {
		client = batchImageDefaultHTTPClient()
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://storage.googleapis.com"
	}
	return &VertexGCSObjectStore{baseURL: baseURL, client: client}
}

func (s *VertexGCSObjectStore) UploadJSONL(ctx context.Context, accessToken string, uri string, r io.Reader) error {
	bucket, object, err := parseGCSURI(uri)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/upload/storage/v1/b/%s/o?uploadType=media&name=%s", s.baseURL, url.PathEscape(bucket), url.QueryEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/jsonl")
	return doVertexNoBody(s.client, req)
}

func (s *VertexGCSObjectStore) ListJSONLObjects(ctx context.Context, accessToken string, prefixURI string) ([]string, error) {
	return s.listObjects(ctx, accessToken, prefixURI, true)
}

func (s *VertexGCSObjectStore) listObjects(ctx context.Context, accessToken string, prefixURI string, jsonlOnly bool) ([]string, error) {
	bucket, prefix, err := parseGCSURI(prefixURI)
	if err != nil {
		return nil, err
	}
	var objects []string
	pageToken := ""
	for {
		endpoint := fmt.Sprintf("%s/storage/v1/b/%s/o?prefix=%s", s.baseURL, url.PathEscape(bucket), url.QueryEscape(prefix))
		if pageToken != "" {
			endpoint += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		var page struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := doVertexDecodeJSON(s.client, req, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if !jsonlOnly || strings.HasSuffix(item.Name, ".jsonl") {
				objects = append(objects, "gs://"+bucket+"/"+item.Name)
			}
		}
		if page.NextPageToken == "" {
			return objects, nil
		}
		pageToken = page.NextPageToken
	}
}

func (s *VertexGCSObjectStore) OpenObject(ctx context.Context, accessToken string, uri string) (io.ReadCloser, string, error) {
	bucket, object, err := parseGCSURI(uri)
	if err != nil {
		return nil, "", err
	}
	endpoint := fmt.Sprintf("%s/storage/v1/b/%s/o/%s?alt=media", s.baseURL, url.PathEscape(bucket), url.PathEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		return nil, "", readVertexAPIError(resp)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/jsonl"
	}
	return resp.Body, contentType, nil
}

func (s *VertexGCSObjectStore) DeleteObject(ctx context.Context, accessToken string, uri string) error {
	bucket, object, err := parseGCSURI(uri)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/storage/v1/b/%s/o/%s", s.baseURL, url.PathEscape(bucket), url.PathEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexNoBody(s.client, req)
}

func (s *VertexGCSObjectStore) DeletePrefix(ctx context.Context, accessToken string, prefixURI string) error {
	objects, err := s.listObjects(ctx, accessToken, prefixURI, false)
	if err != nil {
		return err
	}
	for _, object := range objects {
		if err := s.DeleteObject(ctx, accessToken, object); err != nil {
			return err
		}
	}
	return nil
}

func parseGCSURI(uri string) (bucket, object string, err error) {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, "gs://") {
		return "", "", fmt.Errorf("invalid gcs uri")
	}
	rest := strings.TrimPrefix(uri, "gs://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid gcs uri")
	}
	return parts[0], parts[1], nil
}

type VertexAPIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *VertexAPIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code != "" {
		return fmt.Sprintf("vertex api error: status=%d code=%s message=%s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("vertex api error: status=%d message=%s", e.StatusCode, e.Message)
}

func doVertexJSON[T any](client *http.Client, req *http.Request) (*T, error) {
	var out T
	if err := doVertexDecodeJSON(client, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func doVertexDecodeJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readVertexAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func doVertexNoBody(client *http.Client, req *http.Request) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readVertexAPIError(resp)
	}
	return nil
}

func readVertexAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	message := string(body)
	code := ""
	var parsed struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
		code = parsed.Error.Status
	}
	return &VertexAPIError{StatusCode: resp.StatusCode, Code: code, Message: message}
}

var _ BatchImageProvider = (*VertexBatchImageProvider)(nil)
var _ VertexBatchClient = (*VertexBatchHTTPClient)(nil)
var _ VertexBatchObjectStore = (*VertexGCSObjectStore)(nil)
