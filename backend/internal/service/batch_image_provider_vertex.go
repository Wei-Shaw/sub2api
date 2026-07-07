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
	defaultVertexManagedGCSPrefix  = "batch-image/{envREDACTED/{batch_idREDACTED"
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
REDACTED

func NewVertexBatchImageProviderOptionsFromConfig(cfg *config.Config) VertexBatchImageProviderOptions {
	if cfg == nil {
		return VertexBatchImageProviderOptions{REDACTED
REDACTED
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
REDACTED
REDACTED

type VertexBatchClient interface {
	CreateBatchPredictionJob(ctx context.Context, accessToken string, req VertexCreateBatchPredictionJobRequest) (*VertexBatchPredictionJob, error)
	GetBatchPredictionJob(ctx context.Context, accessToken string, name string) (*VertexBatchPredictionJob, error)
	CancelBatchPredictionJob(ctx context.Context, accessToken string, name string) error
REDACTED

type VertexBatchObjectStore interface {
	UploadJSONL(ctx context.Context, accessToken string, uri string, r io.Reader) error
	ListJSONLObjects(ctx context.Context, accessToken string, prefixURI string) ([]string, error)
	OpenObject(ctx context.Context, accessToken string, uri string) (io.ReadCloser, string, error)
	DeleteObject(ctx context.Context, accessToken string, uri string) error
	DeletePrefix(ctx context.Context, accessToken string, prefixURI string) error
REDACTED

type VertexCreateBatchPredictionJobRequest struct {
	ProjectID      string                     `json:"-"`
	Location       string                     `json:"-"`
	DisplayName    string                     `json:"displayName"`
	Model          string                     `json:"model"`
	InputConfig    VertexBatchInputConfig     `json:"inputConfig"`
	OutputConfig   VertexBatchOutputConfig    `json:"outputConfig"`
	InstanceConfig *VertexBatchInstanceConfig `json:"instanceConfig,omitempty"`
REDACTED

type VertexBatchInputConfig struct {
	InstancesFormat string               `json:"instancesFormat"`
	GCSSource       VertexBatchGCSSource `json:"gcsSource"`
REDACTED

type VertexBatchGCSSource struct {
	URIs []string `json:"uris"`
REDACTED

type VertexBatchOutputConfig struct {
	PredictionsFormat string                    `json:"predictionsFormat"`
	GCSDestination    VertexBatchGCSDestination `json:"gcsDestination"`
REDACTED

type VertexBatchGCSDestination struct {
	OutputURIPrefix string `json:"outputUriPrefix"`
REDACTED

type VertexBatchInstanceConfig struct {
	KeyField string `json:"keyField"`
REDACTED

type VertexBatchPredictionJob struct {
	Name         string                  `json:"name"`
	DisplayName  string                  `json:"displayName"`
	State        string                  `json:"state"`
	OutputConfig VertexBatchOutputConfig `json:"outputConfig"`
	Error        *VertexBatchJobError    `json:"error"`
REDACTED

type VertexBatchJobError struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
REDACTED

type VertexBatchImageProvider struct {
	opts        VertexBatchImageProviderOptions
	client      VertexBatchClient
	objectStore VertexBatchObjectStore
	tokenCache  GeminiTokenCache
REDACTED

func NewVertexBatchImageProvider(opts VertexBatchImageProviderOptions, client VertexBatchClient, objectStore VertexBatchObjectStore, tokenCache GeminiTokenCache) *VertexBatchImageProvider {
	opts = normalizeVertexBatchImageProviderOptions(opts)
	if client == nil {
		client = NewVertexBatchHTTPClient(opts.BatchPredictionBaseURL, nil)
REDACTED
	if objectStore == nil {
		objectStore = NewVertexGCSObjectStore(opts.GCSBaseURL, nil)
REDACTED
	return &VertexBatchImageProvider{
		opts:        opts,
		client:      client,
		objectStore: objectStore,
		tokenCache:  tokenCache,
REDACTED
REDACTED

func NewVertexBatchImageProviderFromConfig(cfg *config.Config, client VertexBatchClient, objectStore VertexBatchObjectStore, tokenCache GeminiTokenCache) *VertexBatchImageProvider {
	return NewVertexBatchImageProvider(NewVertexBatchImageProviderOptionsFromConfig(cfg), client, objectStore, tokenCache)
REDACTED

func normalizeVertexBatchImageProviderOptions(opts VertexBatchImageProviderOptions) VertexBatchImageProviderOptions {
	opts.ProjectID = strings.TrimSpace(opts.ProjectID)
	opts.Location = strings.TrimSpace(opts.Location)
	if opts.Location == "" {
		opts.Location = defaultVertexBatchLocation
REDACTED
	opts.ManagedGCSBucket = strings.Trim(strings.TrimSpace(opts.ManagedGCSBucket), "/")
	opts.ManagedGCSPrefix = strings.Trim(strings.TrimSpace(opts.ManagedGCSPrefix), "/")
	if opts.ManagedGCSPrefix == "" {
		opts.ManagedGCSPrefix = defaultVertexManagedGCSPrefix
REDACTED
	opts.Environment = strings.TrimSpace(opts.Environment)
	if opts.Environment == "" {
		opts.Environment = "default"
REDACTED
	opts.BatchPredictionBaseURL = strings.TrimRight(strings.TrimSpace(opts.BatchPredictionBaseURL), "/")
	opts.GCSBaseURL = strings.TrimRight(strings.TrimSpace(opts.GCSBaseURL), "/")
	return opts
REDACTED

func (p *VertexBatchImageProvider) Name() string {
	return BatchImageProviderVertex
REDACTED

func (p *VertexBatchImageProvider) SupportsAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeServiceAccount {
		return false
REDACTED
	_, err := parseVertexServiceAccountKey(account)
	return err == nil
REDACTED

func (p *VertexBatchImageProvider) Submit(ctx context.Context, job *BatchImageJob, account *Account, input BatchImageInput) (*BatchProviderJob, error) {
	if err := p.validateAccount(account); err != nil {
		return nil, err
REDACTED
	if strings.TrimSpace(p.opts.ManagedGCSBucket) == "" {
		return nil, vertexProviderError("VERTEX_MANAGED_GCS_BUCKET_MISSING", "Vertex managed GCS bucket is not configured", nil)
REDACTED
	if input.BatchID == "" && job != nil {
		input.BatchID = job.BatchID
REDACTED
	if input.Model == "" && job != nil {
		input.Model = job.Model
REDACTED

	jsonl, err := BuildVertexBatchJSONL(input)
	if err != nil {
		return nil, err
REDACTED
	refs, err := p.managedRefs(input.BatchID)
	if err != nil {
		return nil, err
REDACTED

	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return nil, mapVertexClientError(err)
REDACTED
	if err := p.objectStore.UploadJSONL(ctx, accessToken, refs.InputURI, bytes.NewReader(jsonl)); err != nil {
		return nil, vertexProviderError("VERTEX_GCS_UPLOAD_FAILED", "Vertex managed GCS upload failed", nil)
REDACTED

	projectID := strings.TrimSpace(p.opts.ProjectID)
	if projectID == "" {
		projectID = account.VertexProjectID()
REDACTED
	if projectID == "" {
		return nil, vertexProviderError("VERTEX_PROJECT_ID_MISSING", "Vertex project id is not configured", nil)
REDACTED
	location := strings.TrimSpace(p.opts.Location)
	if location == "" {
		location = account.VertexLocation(input.Model)
REDACTED

	req := VertexCreateBatchPredictionJobRequest{
		ProjectID:      projectID,
		Location:       location,
		DisplayName:    vertexBatchDisplayName(input),
		Model:          NormalizeVertexBatchModelPath(input.Model),
		InputConfig:    VertexBatchInputConfig{InstancesFormat: "jsonl", GCSSource: VertexBatchGCSSource{URIs: []string{refs.InputURIREDACTEDREDACTEDREDACTED,
		OutputConfig:   VertexBatchOutputConfig{PredictionsFormat: "jsonl", GCSDestination: VertexBatchGCSDestination{OutputURIPrefix: refs.OutputPrefixURIREDACTEDREDACTED,
		InstanceConfig: &VertexBatchInstanceConfig{KeyField: "key"REDACTED,
REDACTED
	created, err := p.client.CreateBatchPredictionJob(ctx, accessToken, req)
	if err != nil {
		return nil, mapVertexClientError(err)
REDACTED
	if created == nil || strings.TrimSpace(created.Name) == "" {
		return nil, vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex batch response is missing job name", nil)
REDACTED
	return &BatchProviderJob{
		ProviderJobName:   created.Name,
		ProviderInputRef:  refs.InputURI,
		ProviderOutputRef: refs.OutputPrefixURI,
		RawState:          created.State,
REDACTED, nil
REDACTED

func (p *VertexBatchImageProvider) Get(ctx context.Context, job *BatchImageJob, account *Account) (*BatchProviderStatus, error) {
	if err := p.validateAccount(account); err != nil {
		return nil, err
REDACTED
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return nil, ErrBatchImageProviderMissingJobName
REDACTED
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return nil, mapVertexClientError(err)
REDACTED
	vertexJob, err := p.client.GetBatchPredictionJob(ctx, accessToken, jobName)
	if err != nil {
		return nil, mapVertexClientError(err)
REDACTED
	if vertexJob == nil {
		return nil, vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex batch response is empty", nil)
REDACTED
	status := mapVertexBatchState(vertexJob)
	outputRef := strings.TrimSpace(vertexJob.OutputConfig.GCSDestination.OutputURIPrefix)
	if outputRef == "" {
		outputRef = batchImageProviderOutputRef(job)
REDACTED
	if outputRef == "" && job != nil && job.GCSOutputURI != nil {
		outputRef = strings.TrimSpace(*job.GCSOutputURI)
REDACTED
	status.ProviderOutputRef = outputRef
	return status, nil
REDACTED

func (p *VertexBatchImageProvider) Cancel(ctx context.Context, job *BatchImageJob, account *Account) error {
	if err := p.validateAccount(account); err != nil {
		return err
REDACTED
	jobName := batchImageProviderJobName(job)
	if jobName == "" {
		return ErrBatchImageProviderMissingJobName
REDACTED
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return mapVertexClientError(err)
REDACTED
	return mapVertexClientError(p.client.CancelBatchPredictionJob(ctx, accessToken, jobName))
REDACTED

func (p *VertexBatchImageProvider) OpenResult(ctx context.Context, job *BatchImageJob, account *Account) (io.ReadCloser, string, error) {
	if err := p.validateAccount(account); err != nil {
		return nil, "", err
REDACTED
	outputRef := batchImageProviderOutputRef(job)
	if outputRef == "" && job != nil && job.GCSOutputURI != nil {
		outputRef = strings.TrimSpace(*job.GCSOutputURI)
REDACTED
	if outputRef == "" {
		return nil, "", ErrBatchImageProviderMissingResultRef
REDACTED
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return nil, "", mapVertexClientError(err)
REDACTED
	objects, err := p.objectStore.ListJSONLObjects(ctx, accessToken, outputRef)
	if err != nil {
		return nil, "", vertexProviderError("VERTEX_GCS_LIST_FAILED", "Vertex managed GCS list failed", nil)
REDACTED
	sort.Strings(objects)
	if len(objects) == 0 {
		return nil, "", vertexProviderError("VERTEX_RESULT_OBJECTS_MISSING", "Vertex result objects are missing", nil)
REDACTED
	return &vertexCombinedJSONLReadCloser{
		ctx:         ctx,
		accessToken: accessToken,
		objects:     objects,
		store:       p.objectStore,
REDACTED, "application/jsonl", nil
REDACTED

func (p *VertexBatchImageProvider) Cleanup(ctx context.Context, job *BatchImageJob, account *Account, target CleanupTarget) error {
	if err := p.validateAccount(account); err != nil {
		return err
REDACTED
	accessToken, err := p.accessToken(ctx, account)
	if err != nil {
		return mapVertexClientError(err)
REDACTED
	inputRef := batchImageProviderInputRef(job)
	outputRef := batchImageProviderOutputRef(job)
	if job != nil {
		if inputRef == "" && job.GCSInputURI != nil {
			inputRef = strings.TrimSpace(*job.GCSInputURI)
	REDACTED
		if outputRef == "" && job.GCSOutputURI != nil {
			outputRef = strings.TrimSpace(*job.GCSOutputURI)
	REDACTED
REDACTED

	switch target {
	case CleanupTargetInput:
		return p.deleteManagedInput(ctx, accessToken, job, inputRef)
	case CleanupTargetOutput:
		return p.deleteManagedOutput(ctx, accessToken, job, outputRef)
	case CleanupTargetAll:
		if err := p.deleteManagedInput(ctx, accessToken, job, inputRef); err != nil {
			return err
	REDACTED
		return p.deleteManagedOutput(ctx, accessToken, job, outputRef)
	default:
		return ErrUnsupportedCleanupTarget
REDACTED
REDACTED

func (p *VertexBatchImageProvider) validateAccount(account *Account) error {
	if account == nil || account.Platform != PlatformGemini || account.Type != AccountTypeServiceAccount {
		return ErrBatchImageProviderUnsupportedAccount
REDACTED
	if _, err := parseVertexServiceAccountKey(account); err != nil {
		return ErrBatchImageProviderMissingServiceAccount
REDACTED
	return nil
REDACTED

func (p *VertexBatchImageProvider) accessToken(ctx context.Context, account *Account) (string, error) {
	return getVertexServiceAccountAccessToken(ctx, p.tokenCache, account)
REDACTED

func (p *VertexBatchImageProvider) deleteManagedInput(ctx context.Context, accessToken string, job *BatchImageJob, uri string) error {
	if strings.TrimSpace(uri) == "" {
		return nil
REDACTED
	if !p.isSafeManagedInput(job, uri) {
		return ErrBatchImageProviderUnsafeCleanupPath
REDACTED
	return mapVertexClientError(p.objectStore.DeleteObject(ctx, accessToken, uri))
REDACTED

func (p *VertexBatchImageProvider) deleteManagedOutput(ctx context.Context, accessToken string, job *BatchImageJob, uri string) error {
	if strings.TrimSpace(uri) == "" {
		return nil
REDACTED
	if !p.isSafeManagedOutput(job, uri) {
		return ErrBatchImageProviderUnsafeCleanupPath
REDACTED
	return mapVertexClientError(p.objectStore.DeletePrefix(ctx, accessToken, uri))
REDACTED

func (p *VertexBatchImageProvider) isSafeManagedInput(job *BatchImageJob, uri string) bool {
	if job == nil || strings.TrimSpace(job.BatchID) == "" {
		return false
REDACTED
	refs, err := p.managedRefs(job.BatchID)
	return err == nil && strings.TrimSpace(uri) == refs.InputURI
REDACTED

func (p *VertexBatchImageProvider) isSafeManagedOutput(job *BatchImageJob, uri string) bool {
	if job == nil || strings.TrimSpace(job.BatchID) == "" {
		return false
REDACTED
	refs, err := p.managedRefs(job.BatchID)
	return err == nil && strings.HasPrefix(strings.TrimSpace(uri), refs.OutputPrefixURI)
REDACTED

type vertexManagedRefs struct {
	Prefix          string
	InputURI        string
	OutputPrefixURI string
REDACTED

func (p *VertexBatchImageProvider) managedRefs(batchID string) (vertexManagedRefs, error) {
	batchID = strings.TrimSpace(batchID)
	if !IsValidBatchImageID(batchID) {
		return vertexManagedRefs{REDACTED, batchImageProviderInputError("valid batch_id is required")
REDACTED
	bucket := strings.Trim(strings.TrimSpace(p.opts.ManagedGCSBucket), "/")
	if bucket == "" || strings.Contains(bucket, "://") {
		return vertexManagedRefs{REDACTED, vertexProviderError("VERTEX_MANAGED_GCS_BUCKET_MISSING", "Vertex managed GCS bucket is not configured", nil)
REDACTED
	prefix := buildVertexManagedGCSPrefix(p.opts.ManagedGCSPrefix, p.opts.Environment, batchID)
	if !strings.Contains(prefix, batchID) {
		return vertexManagedRefs{REDACTED, batchImageProviderInputError("managed GCS prefix must contain batch_id")
REDACTED
	base := "gs://" + bucket + "/" + strings.Trim(prefix, "/")
	return vertexManagedRefs{
		Prefix:          strings.Trim(prefix, "/"),
		InputURI:        base + "/input/requests.jsonl",
		OutputPrefixURI: base + "/output/",
REDACTED, nil
REDACTED

func buildVertexManagedGCSPrefix(template, env, batchID string) string {
	template = strings.Trim(strings.TrimSpace(template), "/")
	if template == "" {
		template = defaultVertexManagedGCSPrefix
REDACTED
	env = sanitizeVertexGCSPathSegment(env)
	batchID = sanitizeVertexGCSPathSegment(batchID)
	prefix := strings.ReplaceAll(template, "{envREDACTED", env)
	prefix = strings.ReplaceAll(prefix, "{batch_idREDACTED", batchID)
	return strings.Trim(prefix, "/")
REDACTED

func sanitizeVertexGCSPathSegment(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "default"
REDACTED
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			_, _ = b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			_, _ = b.WriteRune(r)
		default:
			_ = b.WriteByte('-')
	REDACTED
REDACTED
	return b.String()
REDACTED

func vertexBatchDisplayName(input BatchImageInput) string {
	if v := strings.TrimSpace(input.DisplayName); v != "" {
		return v
REDACTED
	if v := strings.TrimSpace(input.BatchID); v != "" {
		return "sub2api-" + v
REDACTED
	return "sub2api-image-batch"
REDACTED

func BuildVertexBatchJSONL(input BatchImageInput) ([]byte, error) {
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
		parts, err := vertexBatchImageParts(prompt, item.ReferenceImages)
		if err != nil {
			return nil, err
	REDACTED
		line := map[string]any{
			"key": customID,
			"request": map[string]any{
				"contents": []any{map[string]any{
					"role":  "user",
					"parts": parts,
		REDACTED
				"generationConfig": map[string]any{
					"responseModalities": []string{"TEXT", "IMAGE"REDACTED,
			REDACTED,
		REDACTED,
	REDACTED
		if err := enc.Encode(line); err != nil {
			return nil, err
	REDACTED
REDACTED
	return buf.Bytes(), nil
REDACTED

func vertexBatchImageParts(prompt string, refs []BatchImageReference) ([]any, error) {
	parts := []any{map[string]any{"text": promptREDACTEDREDACTED
	for _, ref := range refs {
		mimeType := normalizeBatchImageReferenceMimeType(ref.MimeType)
		if mimeType == "" {
			return nil, batchImageProviderInputError("reference image mime_type is required")
	REDACTED
		fileURI := strings.TrimSpace(ref.FileURI)
		switch {
		case len(ref.Data) > 0 && fileURI == "":
			parts = append(parts, map[string]any{
				"inlineData": map[string]any{
					"mimeType": mimeType,
					"data":     base64.StdEncoding.EncodeToString(ref.Data),
			REDACTED,
		REDACTED)
		case len(ref.Data) == 0 && fileURI != "":
			parts = append(parts, map[string]any{
				"fileData": map[string]any{
					"mimeType": mimeType,
					"fileUri":  fileURI,
			REDACTED,
		REDACTED)
		default:
			return nil, batchImageProviderInputError("reference image must contain exactly one of data or file_uri")
	REDACTED
REDACTED
	return parts, nil
REDACTED

func NormalizeVertexBatchModelPath(model string) string {
	model = strings.Trim(strings.TrimSpace(model), "/")
	if strings.HasPrefix(model, "publishers/") || strings.HasPrefix(model, "projects/") {
		return model
REDACTED
	return "publishers/google/models/" + model
REDACTED

func BuildVertexBatchPredictionJobsEndpoint(baseURL, projectID, location string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	location = strings.TrimSpace(location)
	if projectID == "" {
		return "", errors.New("vertex project_id is required")
REDACTED
	if location == "" {
		location = defaultVertexBatchLocation
REDACTED
	if !vertexLocationPattern.MatchString(location) {
		return "", fmt.Errorf("invalid vertex location: %s", location)
REDACTED
	if strings.TrimSpace(baseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/projects/" + url.PathEscape(projectID) + "/locations/" + url.PathEscape(location) + "/batchPredictionJobs", nil
REDACTED
	host := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
	if location == "global" {
		host = "aiplatform.googleapis.com"
REDACTED
	return fmt.Sprintf("https://%s/v1/projects/%s/locations/%s/batchPredictionJobs", host, url.PathEscape(projectID), url.PathEscape(location)), nil
REDACTED

func mapVertexBatchState(job *VertexBatchPredictionJob) *BatchProviderStatus {
	state := strings.TrimSpace(job.State)
	status := &BatchProviderStatus{
		RawState:              state,
		InternalState:         BatchProviderStateRunning,
		SuggestedRequeueAfter: defaultVertexBatchRequeueAfter,
REDACTED
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
	REDACTED
REDACTED
	if job.Error != nil {
		if code := strings.TrimSpace(job.Error.Status); code != "" {
			status.ErrorCode = code
	REDACTED
		status.ErrorMessage = strings.TrimSpace(job.Error.Message)
REDACTED
	return status
REDACTED

func vertexProviderError(reason, message string, cause error) error {
	err := infraerrors.New(http.StatusBadGateway, reason, message)
	if cause != nil {
		return err.WithCause(cause)
REDACTED
	return err
REDACTED

func mapVertexClientError(err error) error {
	if err == nil {
		return nil
REDACTED
	if errors.Is(err, ErrBatchImageProviderMissingServiceAccount) ||
		errors.Is(err, ErrBatchImageProviderMissingJobName) ||
		errors.Is(err, ErrBatchImageProviderMissingResultRef) ||
		errors.Is(err, ErrBatchImageProviderUnsafeCleanupPath) ||
		errors.Is(err, ErrUnsupportedCleanupTarget) {
		return err
REDACTED
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
	REDACTED
REDACTED
	return vertexProviderError("VERTEX_INVALID_RESPONSE", "Vertex API request failed", err)
REDACTED

type vertexCombinedJSONLReadCloser struct {
	ctx          context.Context
	accessToken  string
	objects      []string
	store        VertexBatchObjectStore
	index        int
	current      io.ReadCloser
	needBoundary bool
	closed       bool
REDACTED

func (r *vertexCombinedJSONLReadCloser) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.ErrClosedPipe
REDACTED
	if r.needBoundary {
		if len(p) == 0 {
			return 0, nil
	REDACTED
		p[0] = '\n'
		r.needBoundary = false
		return 1, nil
REDACTED
	for {
		if r.current == nil {
			if r.index >= len(r.objects) {
				return 0, io.EOF
		REDACTED
			obj := r.objects[r.index]
			r.index++
			rc, _, err := r.store.OpenObject(r.ctx, r.accessToken, obj)
			if err != nil {
				return 0, err
		REDACTED
			r.current = rc
	REDACTED
		n, err := r.current.Read(p)
		if err == io.EOF {
			_ = r.current.Close()
			r.current = nil
			if r.index < len(r.objects) {
				if n > 0 {
					r.needBoundary = true
					return n, nil
			REDACTED
				if len(p) == 0 {
					return 0, nil
			REDACTED
				p[0] = '\n'
				return 1, nil
		REDACTED
			if n > 0 {
				return n, nil
		REDACTED
			continue
	REDACTED
		return n, err
REDACTED
REDACTED

func (r *vertexCombinedJSONLReadCloser) Close() error {
	r.closed = true
	if r.current != nil {
		return r.current.Close()
REDACTED
	return nil
REDACTED

type VertexBatchHTTPClient struct {
	baseURL string
	client  *http.Client
REDACTED

func NewVertexBatchHTTPClient(baseURL string, client *http.Client) *VertexBatchHTTPClient {
	if client == nil {
		client = http.DefaultClient
REDACTED
	return &VertexBatchHTTPClient{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), client: clientREDACTED
REDACTED

func (c *VertexBatchHTTPClient) CreateBatchPredictionJob(ctx context.Context, accessToken string, req VertexCreateBatchPredictionJobRequest) (*VertexBatchPredictionJob, error) {
	endpoint, err := BuildVertexBatchPredictionJobsEndpoint(c.baseURL, req.ProjectID, req.Location)
	if err != nil {
		return nil, err
REDACTED
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
REDACTED
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
REDACTED
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexJSON[VertexBatchPredictionJob](c.client, httpReq)
REDACTED

func (c *VertexBatchHTTPClient) GetBatchPredictionJob(ctx context.Context, accessToken string, name string) (*VertexBatchPredictionJob, error) {
	endpoint := c.vertexResourceURL(name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexJSON[VertexBatchPredictionJob](c.client, req)
REDACTED

func (c *VertexBatchHTTPClient) CancelBatchPredictionJob(ctx context.Context, accessToken string, name string) error {
	endpoint := c.vertexResourceURL(name) + ":cancel"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexNoBody(c.client, req)
REDACTED

func (c *VertexBatchHTTPClient) vertexResourceURL(name string) string {
	name = strings.TrimLeft(strings.TrimSpace(name), "/")
	if c.baseURL != "" {
		return c.baseURL + "/v1/" + name
REDACTED
	return "https://aiplatform.googleapis.com/v1/" + name
REDACTED

type VertexGCSObjectStore struct {
	baseURL string
	client  *http.Client
REDACTED

func NewVertexGCSObjectStore(baseURL string, client *http.Client) *VertexGCSObjectStore {
	if client == nil {
		client = http.DefaultClient
REDACTED
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://storage.googleapis.com"
REDACTED
	return &VertexGCSObjectStore{baseURL: baseURL, client: clientREDACTED
REDACTED

func (s *VertexGCSObjectStore) UploadJSONL(ctx context.Context, accessToken string, uri string, r io.Reader) error {
	bucket, object, err := parseGCSURI(uri)
	if err != nil {
		return err
REDACTED
	endpoint := fmt.Sprintf("%s/upload/storage/v1/b/%s/o?uploadType=media&name=%s", s.baseURL, url.PathEscape(bucket), url.QueryEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, r)
	if err != nil {
		return err
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/jsonl")
	return doVertexNoBody(s.client, req)
REDACTED

func (s *VertexGCSObjectStore) ListJSONLObjects(ctx context.Context, accessToken string, prefixURI string) ([]string, error) {
	return s.listObjects(ctx, accessToken, prefixURI, true)
REDACTED

func (s *VertexGCSObjectStore) listObjects(ctx context.Context, accessToken string, prefixURI string, jsonlOnly bool) ([]string, error) {
	bucket, prefix, err := parseGCSURI(prefixURI)
	if err != nil {
		return nil, err
REDACTED
	var objects []string
	pageToken := ""
	for {
		endpoint := fmt.Sprintf("%s/storage/v1/b/%s/o?prefix=%s", s.baseURL, url.PathEscape(bucket), url.QueryEscape(prefix))
		if pageToken != "" {
			endpoint += "&pageToken=" + url.QueryEscape(pageToken)
	REDACTED
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
	REDACTED
		req.Header.Set("Authorization", "Bearer "+accessToken)
		var page struct {
			Items []struct {
				Name string `json:"name"`
		REDACTED `json:"items"`
			NextPageToken string `json:"nextPageToken"`
	REDACTED
		if err := doVertexDecodeJSON(s.client, req, &page); err != nil {
			return nil, err
	REDACTED
		for _, item := range page.Items {
			if !jsonlOnly || strings.HasSuffix(item.Name, ".jsonl") {
				objects = append(objects, "gs://"+bucket+"/"+item.Name)
		REDACTED
	REDACTED
		if page.NextPageToken == "" {
			return objects, nil
	REDACTED
		pageToken = page.NextPageToken
REDACTED
REDACTED

func (s *VertexGCSObjectStore) OpenObject(ctx context.Context, accessToken string, uri string) (io.ReadCloser, string, error) {
	bucket, object, err := parseGCSURI(uri)
	if err != nil {
		return nil, "", err
REDACTED
	endpoint := fmt.Sprintf("%s/storage/v1/b/%s/o/%s?alt=media", s.baseURL, url.PathEscape(bucket), url.PathEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
REDACTED
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() REDACTED()
		return nil, "", readVertexAPIError(resp)
REDACTED
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/jsonl"
REDACTED
	return resp.Body, contentType, nil
REDACTED

func (s *VertexGCSObjectStore) DeleteObject(ctx context.Context, accessToken string, uri string) error {
	bucket, object, err := parseGCSURI(uri)
	if err != nil {
		return err
REDACTED
	endpoint := fmt.Sprintf("%s/storage/v1/b/%s/o/%s", s.baseURL, url.PathEscape(bucket), url.PathEscape(object))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
REDACTED
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return doVertexNoBody(s.client, req)
REDACTED

func (s *VertexGCSObjectStore) DeletePrefix(ctx context.Context, accessToken string, prefixURI string) error {
	objects, err := s.listObjects(ctx, accessToken, prefixURI, false)
	if err != nil {
		return err
REDACTED
	for _, object := range objects {
		if err := s.DeleteObject(ctx, accessToken, object); err != nil {
			return err
	REDACTED
REDACTED
	return nil
REDACTED

func parseGCSURI(uri string) (bucket, object string, err error) {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, "gs://") {
		return "", "", fmt.Errorf("invalid gcs uri")
REDACTED
	rest := strings.TrimPrefix(uri, "gs://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid gcs uri")
REDACTED
	return parts[0], parts[1], nil
REDACTED

type VertexAPIError struct {
	StatusCode int
	Code       string
	Message    string
REDACTED

func (e *VertexAPIError) Error() string {
	if e == nil {
		return "<nil>"
REDACTED
	if e.Code != "" {
		return fmt.Sprintf("vertex api error: status=%d code=%s message=%s", e.StatusCode, e.Code, e.Message)
REDACTED
	return fmt.Sprintf("vertex api error: status=%d message=%s", e.StatusCode, e.Message)
REDACTED

func doVertexJSON[T any](client *http.Client, req *http.Request) (*T, error) {
	var out T
	if err := doVertexDecodeJSON(client, req, &out); err != nil {
		return nil, err
REDACTED
	return &out, nil
REDACTED

func doVertexDecodeJSON(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readVertexAPIError(resp)
REDACTED
	return json.NewDecoder(resp.Body).Decode(out)
REDACTED

func doVertexNoBody(client *http.Client, req *http.Request) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return readVertexAPIError(resp)
REDACTED
	return nil
REDACTED

func readVertexAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	message := string(body)
	code := ""
	var parsed struct {
		Error struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
	REDACTED `json:"error"`
REDACTED
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
		code = parsed.Error.Status
REDACTED
	return &VertexAPIError{StatusCode: resp.StatusCode, Code: code, Message: messageREDACTED
REDACTED

var _ BatchImageProvider = (*VertexBatchImageProvider)(nil)
var _ VertexBatchClient = (*VertexBatchHTTPClient)(nil)
var _ VertexBatchObjectStore = (*VertexGCSObjectStore)(nil)
