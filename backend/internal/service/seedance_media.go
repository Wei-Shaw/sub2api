package service

// Seedance video support is intentionally kept separate from the xAI/Grok
// media implementation.  Seedance is a Volcano Engine Ark asynchronous task
// API, not an OpenAI-compatible chat or xAI media API.  The account scheduler
// and the existing deferred-video billing cache are shared, but the upstream
// request/response contract is provider-specific here.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	seedanceVideoTasksPath         = "/contents/generations/tasks"
	seedanceDefaultVideoResolution = VideoBillingResolution720P
)

// SeedanceVideoRequestInfo contains the client-facing fields needed for
// routing and the deferred billing snapshot.  Ark's status response does not
// repeat all create-time fields, so these values are captured before submit.
type SeedanceVideoRequestInfo struct {
	Model           string
	Resolution      string
	DurationSeconds int
}

// SeedanceVideoRequest parses both the native Ark content format and the
// OpenAI-style prompt/image convenience format accepted by the gateway.
func ParseSeedanceVideoRequest(body []byte) (SeedanceVideoRequestInfo, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) || !gjson.GetBytes(body, "").IsObject() {
		return SeedanceVideoRequestInfo{}, errors.New("request body must be a JSON object")
	}
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if model == "" {
		return SeedanceVideoRequestInfo{}, errors.New("model is required")
	}
	resolution := strings.TrimSpace(gjson.GetBytes(body, "resolution").String())
	if resolution == "" {
		resolution = strings.TrimSpace(gjson.GetBytes(body, "video_resolution").String())
	}
	if resolution == "" {
		resolution = seedanceDefaultVideoResolution
	}
	duration := 0
	if raw := gjson.GetBytes(body, "duration"); raw.Exists() && raw.Type == gjson.Number {
		duration = int(raw.Int())
	}
	return SeedanceVideoRequestInfo{
		Model:           model,
		Resolution:      NormalizeVideoBillingResolutionOrDefault(resolution),
		DurationSeconds: NormalizeVideoBillingDurationSecondsOrDefault(duration),
	}, nil
}

// PrepareSeedanceVideoRequest converts the small OpenAI-style convenience
// shape into Ark's content array while leaving native Ark requests intact.
// The gateway accepts both resolution and video_resolution as billing aliases.
// Ark versions that support tiered output accept the canonical resolution
// field, so keep it in the upstream payload and only remove the alias.
func PrepareSeedanceVideoRequest(body []byte, account *Account) ([]byte, SeedanceVideoRequestInfo, error) {
	info, err := ParseSeedanceVideoRequest(body)
	if err != nil {
		return nil, SeedanceVideoRequestInfo{}, err
	}
	prepared := append([]byte(nil), body...)
	if mapped := strings.TrimSpace(account.GetMappedModel(info.Model)); mapped != "" && mapped != info.Model {
		prepared, err = sjson.SetBytes(prepared, "model", mapped)
		if err != nil {
			return nil, SeedanceVideoRequestInfo{}, fmt.Errorf("rewrite Seedance model: %w", err)
		}
	}

	// Native Ark requests already contain content.  For prompt/image clients,
	// build the documented content array from their familiar fields.
	if !gjson.GetBytes(prepared, "content").Exists() {
		content := make([]map[string]any, 0, 3)
		if prompt := strings.TrimSpace(gjson.GetBytes(prepared, "prompt").String()); prompt != "" {
			content = append(content, map[string]any{"type": "text", "text": prompt})
		}
		appendImage := func(value gjson.Result) {
			if !value.Exists() {
				return
			}
			if value.IsArray() {
				for _, item := range value.Array() {
					if url := seedanceImageURL(item); url != "" {
						content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
					}
				}
				return
			}
			if url := seedanceImageURL(value); url != "" {
				content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
			}
		}
		appendImage(gjson.GetBytes(prepared, "image"))
		appendImage(gjson.GetBytes(prepared, "images"))
		appendImage(gjson.GetBytes(prepared, "reference_images"))
		if len(content) == 0 {
			return nil, SeedanceVideoRequestInfo{}, errors.New("prompt or content is required")
		}
		prepared, err = sjson.SetBytes(prepared, "content", content)
		if err != nil {
			return nil, SeedanceVideoRequestInfo{}, fmt.Errorf("build Seedance content: %w", err)
		}
	}
	for _, key := range []string{"prompt", "image", "images", "reference_images", "video_resolution", "size"} {
		if gjson.GetBytes(prepared, key).Exists() {
			prepared, _ = sjson.DeleteBytes(prepared, key)
		}
	}
	if !gjson.GetBytes(prepared, "resolution").Exists() {
		prepared, _ = sjson.SetBytes(prepared, "resolution", info.Resolution)
	}
	// aspect_ratio is a common OpenAI-style alias for Ark's ratio field.
	if ratio := strings.TrimSpace(gjson.GetBytes(body, "aspect_ratio").String()); ratio != "" && !gjson.GetBytes(prepared, "ratio").Exists() {
		prepared, _ = sjson.SetBytes(prepared, "ratio", ratio)
	}
	if gjson.GetBytes(prepared, "aspect_ratio").Exists() {
		prepared, _ = sjson.DeleteBytes(prepared, "aspect_ratio")
	}
	return prepared, info, nil
}

func seedanceImageURL(value gjson.Result) string {
	if !value.Exists() {
		return ""
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String())
	}
	for _, path := range []string{"url", "image_url.url", "image_url"} {
		if url := strings.TrimSpace(value.Get(path).String()); url != "" {
			return url
		}
	}
	return ""
}

// IsSeedanceVideoStatusBillable recognizes Ark's successful task result.  Ark
// has used both succeeded and completed in compatible deployments; a URL is
// required in either case so failed/queued tasks never create a bill.
func IsSeedanceVideoStatusBillable(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status != "succeeded" && status != "completed" && status != "success" && status != "done" {
		return false
	}
	for _, path := range []string{"content.video_url", "content.video.uri", "video_url", "video.url"} {
		if strings.TrimSpace(gjson.GetBytes(body, path).String()) != "" {
			return true
		}
	}
	return false
}

func seedanceVideoTaskID(body []byte) string {
	for _, path := range []string{"id", "task_id", "task.id", "data.id", "data.task_id"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func seedanceVideoURL(body []byte) string {
	for _, path := range []string{"content.video_url", "content.video.uri", "video_url", "video.url"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

// ForwardSeedanceVideo creates or polls an Ark video task and writes the
// upstream JSON response to the client.  The handler uses the returned result
// for task ownership and deferred billing.
func (s *OpenAIGatewayService) ForwardSeedanceVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	if account == nil || !account.IsSeedance() {
		return nil, fmt.Errorf("seedance account is required")
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	requestInfo := SeedanceVideoRequestInfo{}
	prepared := body
	method := http.MethodGet
	statusRequest := endpoint == GrokMediaEndpointVideoStatus
	if endpoint != GrokMediaEndpointVideosGenerations && endpoint != GrokMediaEndpointVideoStatus && endpoint != GrokMediaEndpointVideoContent {
		return nil, fmt.Errorf("Seedance supports video generation, status, and content only")
	}
	if !statusRequest && endpoint != GrokMediaEndpointVideoContent {
		prepared, requestInfo, err = PrepareSeedanceVideoRequest(body, account)
		if err != nil {
			return nil, err
		}
		method = http.MethodPost
	}
	if endpoint == GrokMediaEndpointVideoContent {
		return s.forwardSeedanceVideoContent(ctx, c, account, token, requestID)
	}
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIBaseURL()), "/")
	if base == "" {
		base = DefaultSeedanceBaseURL
	}
	target := base + seedanceVideoTasksPath
	if statusRequest {
		requestID = strings.TrimSpace(requestID)
		if requestID == "" {
			return nil, errors.New("request_id is required")
		}
		target += "/" + requestID
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	var reader *bytes.Reader
	if !statusRequest {
		reader = bytes.NewReader(prepared)
	}
	var bodyReader io.Reader
	if reader != nil {
		bodyReader = reader
	}
	// http.NewRequest accepts an io.Reader; keeping the reader nil for GET is
	// important because some Ark proxies reject a GET with a zero content body.
	request, err := http.NewRequestWithContext(upstreamCtx, method, target, bodyReader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if !statusRequest {
		request.Header.Set("Content-Type", "application/json")
	}
	account.ApplyHeaderOverrides(request.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	started := time.Now()
	resp, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer resp.Body.Close()
	responseBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(extractUpstreamErrorMessage(responseBody))
		if message == "" {
			message = fmt.Sprintf("Seedance upstream returned status %d", resp.StatusCode)
		}
		setOpsUpstreamError(c, resp.StatusCode, message, "")
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return nil, &UpstreamFailoverError{StatusCode: resp.StatusCode, ResponseBody: responseBody, ResponseHeaders: resp.Header.Clone(), RetryableOnSameAccount: true}
		}
		if c != nil && !c.Writer.Written() {
			c.JSON(resp.StatusCode, gin.H{"error": gin.H{"type": "upstream_error", "message": message}})
		}
		return nil, fmt.Errorf("Seedance upstream returned status %d: %s", resp.StatusCode, message)
	}
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, (*responseheaders.CompiledHeaderFilter)(nil))
	if c != nil {
		c.Data(resp.StatusCode, "application/json", responseBody)
	}
	result := &OpenAIForwardResult{
		RequestID:       strings.TrimSpace(resp.Header.Get("x-request-id")),
		ResponseID:      seedanceVideoTaskID(responseBody),
		Model:           requestInfo.Model,
		BillingModel:    requestInfo.Model,
		UpstreamModel:   requestInfo.Model,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(started),
	}
	if statusRequest {
		result.ResponseID = firstNonEmpty(result.ResponseID, requestID)
		if IsSeedanceVideoStatusBillable(responseBody) {
			result.VideoCount = 1
			result.Model = strings.TrimSpace(gjson.GetBytes(responseBody, "model").String())
			result.BillingModel = result.Model
			result.UpstreamModel = result.Model
			if result.Model == "" {
				result.Model = requestInfo.Model
				result.BillingModel = result.Model
				result.UpstreamModel = result.Model
			}
		}
	} else {
		result.VideoResolution = requestInfo.Resolution
		result.VideoDurationSeconds = requestInfo.DurationSeconds
	}
	return result, nil
}

// forwardSeedanceVideoContent resolves the completed task and streams the
// signed Ark video URL. Ark returns the playable URL from the task status;
// there is no Grok-style /videos/{id}/content endpoint to proxy directly.
func (s *OpenAIGatewayService) forwardSeedanceVideoContent(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	requestID string,
) (*OpenAIForwardResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, errors.New("request_id is required")
	}
	base := strings.TrimRight(strings.TrimSpace(account.GetOpenAIBaseURL()), "/")
	if base == "" {
		base = DefaultSeedanceBaseURL
	}
	statusURL := base + seedanceVideoTasksPath + "/" + url.PathEscape(requestID)
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	statusReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, err
	}
	statusReq.Header.Set("Authorization", "Bearer "+token)
	statusReq.Header.Set("Accept", "application/json")
	account.ApplyHeaderOverrides(statusReq.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	started := time.Now()
	statusResp, err := s.httpUpstream.Do(statusReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	statusBody, readErr := ReadUpstreamResponseBody(statusResp.Body, s.cfg, c, openAITooLargeError)
	_ = statusResp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if statusResp.StatusCode >= 400 {
		return nil, fmt.Errorf("Seedance upstream returned status %d: %s", statusResp.StatusCode, extractUpstreamErrorMessage(statusBody))
	}
	contentURL := seedanceVideoURL(statusBody)
	if contentURL == "" {
		return nil, errors.New("Seedance video is not ready or has no content URL")
	}
	parsed, err := url.Parse(contentURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Seedance video content URL: %w", err)
	}
	if !parsed.IsAbs() {
		baseURL, baseErr := url.Parse(base)
		if baseErr != nil {
			return nil, baseErr
		}
		parsed = baseURL.ResolveReference(parsed)
	}
	contentURL, err = urlvalidator.ValidateHTTPSURL(parsed.String(), urlvalidator.ValidationOptions{AllowPrivate: false})
	if err != nil {
		return nil, fmt.Errorf("invalid Seedance video content URL: %w", err)
	}
	contentCtx := WithHTTPUpstreamPublicHostsOnly(WithHTTPUpstreamRedirectsDisabled(upstreamCtx))
	contentReq, err := http.NewRequestWithContext(contentCtx, http.MethodGet, contentURL, nil)
	if err != nil {
		return nil, err
	}
	contentReq.Header.Set("Accept", "*/*")
	if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
		contentReq.Header.Set("Range", rangeHeader)
	}
	// Ark content URLs are commonly signed URLs. Do not leak the account token
	// or private header overrides to the storage host.
	contentResp, err := s.httpUpstream.Do(contentReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer contentResp.Body.Close()
	if contentResp.StatusCode >= 400 && contentResp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		return nil, fmt.Errorf("Seedance video content returned status %d", contentResp.StatusCode)
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition"} {
		if value := strings.TrimSpace(contentResp.Header.Get(name)); value != "" {
			c.Header(name, value)
		}
	}
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Header("Content-Type", "application/octet-stream")
	}
	c.Status(contentResp.StatusCode)
	_, err = io.Copy(c.Writer, contentResp.Body)
	result := &OpenAIForwardResult{RequestID: contentResp.Header.Get("x-request-id"), ResponseID: requestID, Duration: time.Since(started)}
	if IsSeedanceVideoStatusBillable(statusBody) {
		result.VideoCount = 1
		result.Model = strings.TrimSpace(gjson.GetBytes(statusBody, "model").String())
		result.BillingModel = result.Model
		result.UpstreamModel = result.Model
	}
	return result, err
}

// SelectSeedanceVideoRequestAccount only admits the account bound at task
// creation time.  A status poll must never escape to a different account.
func (s *OpenAIGatewayService) SelectSeedanceVideoRequestAccount(
	ctx context.Context, groupID *int64, sessionHash string, accountID int64, requestedModel string,
) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	decision := OpenAIAccountScheduleDecision{Layer: openAIAccountScheduleLayerSessionSticky}
	if accountID <= 0 || strings.TrimSpace(sessionHash) == "" {
		return nil, decision, ErrNoAvailableAccounts
	}
	ctx = s.withOpenAIGroupPrivacyRequirement(WithOpenAIProfitControlSuppressed(ctx), groupID)
	scheduler := &defaultOpenAIAccountScheduler{service: s}
	selection, _, err := scheduler.selectBySessionHash(ctx, OpenAIAccountScheduleRequest{
		GroupID: groupID, Platform: PlatformSeedance, SessionHash: sessionHash,
		StickyAccountID: accountID, PreserveStickyBinding: true, DisableStickyEscape: true,
		RequestedModel: requestedModel, RequiredTransport: OpenAIUpstreamTransportHTTPSSE,
		RequiredCapability: OpenAIEndpointCapabilityGrokMediaGeneration,
		RequirePrivacySet:  s.openAIGroupRequiresPrivacySet(ctx, groupID),
	})
	if err != nil || selection == nil || selection.Account == nil {
		if err == nil {
			err = ErrNoAvailableAccounts
		}
		return nil, decision, err
	}
	decision.StickySessionHit = true
	decision.SelectedAccountID = selection.Account.ID
	decision.SelectedAccountType = selection.Account.Type
	return selection, decision, nil
}

func SeedanceVideoRequestSessionHash(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return "seedance-video:" + DeriveSessionHashFromSeed(fmt.Sprintf("%d:%d:%s", userID, apiKeyID, requestID))
}

func (s *OpenAIGatewayService) BindSeedanceVideoRequestAccount(ctx context.Context, groupID *int64, requestID string, userID, apiKeyID, accountID int64) error {
	if s == nil || s.cache == nil {
		return errors.New("seedance video request binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(SeedanceVideoRequestSessionHash(requestID, userID, apiKeyID))
	if cacheKey == "" || accountID <= 0 {
		return errors.New("seedance video request binding is invalid")
	}
	return s.cache.SetSessionAccountID(ctx, derefGroupID(groupID), cacheKey, accountID, grokVideoPendingBillingTTL(s.cfg))
}

func (s *OpenAIGatewayService) ResolveSeedanceVideoRequestAccount(ctx context.Context, groupID *int64, requestID string, userID, apiKeyID int64) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, errors.New("seedance video request binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(SeedanceVideoRequestSessionHash(requestID, userID, apiKeyID))
	if cacheKey == "" {
		return 0, errors.New("seedance video request binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

func seedanceVideoPendingBillingKey(requestID string, userID, apiKeyID int64) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || userID <= 0 || apiKeyID <= 0 {
		return ""
	}
	return fmt.Sprintf("seedance:%d:%d:%s", userID, apiKeyID, requestID)
}

func (s *OpenAIGatewayService) StoreSeedanceVideoPendingBilling(ctx context.Context, requestID string, userID, apiKeyID int64, pending GrokVideoPendingBilling) error {
	if s == nil || s.cache == nil {
		return errors.New("seedance video billing cache is unavailable")
	}
	key := seedanceVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return errors.New("seedance video billing key is invalid")
	}
	pending.Model = strings.TrimSpace(pending.Model)
	pending.BillingModel = strings.TrimSpace(pending.BillingModel)
	pending.UpstreamModel = strings.TrimSpace(pending.UpstreamModel)
	pending.OriginalModel = strings.TrimSpace(pending.OriginalModel)
	pending.VideoResolution = NormalizeVideoBillingResolutionOrDefault(pending.VideoResolution)
	pending.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(pending.VideoDurationSeconds)
	if strings.TrimSpace(pending.CreatedAt) == "" {
		pending.CreatedAt = GrokVideoPendingCreatedAtNow()
	}
	payload, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return s.cache.SetGrokVideoPendingBilling(ctx, key, payload, grokVideoPendingBillingTTL(s.cfg))
}

func (s *OpenAIGatewayService) LoadSeedanceVideoPendingBilling(ctx context.Context, requestID string, userID, apiKeyID int64) (*GrokVideoPendingBilling, error) {
	if s == nil || s.cache == nil {
		return nil, errors.New("seedance video billing cache is unavailable")
	}
	key := seedanceVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return nil, errors.New("seedance video billing key is invalid")
	}
	payload, err := s.cache.GetGrokVideoPendingBilling(ctx, key)
	if err != nil || len(payload) == 0 {
		return nil, err
	}
	var pending GrokVideoPendingBilling
	if err := json.Unmarshal(payload, &pending); err != nil {
		return nil, err
	}
	return &pending, nil
}

func (s *OpenAIGatewayService) ClaimSeedanceVideoBilling(ctx context.Context, requestID string, userID, apiKeyID int64) (bool, error) {
	if s == nil || s.cache == nil {
		return false, errors.New("seedance video billing cache is unavailable")
	}
	key := seedanceVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return false, errors.New("seedance video billing key is invalid")
	}
	return s.cache.ClaimGrokVideoBilled(ctx, key, grokVideoBilledClaimTTL(s.cfg))
}

func (s *OpenAIGatewayService) ReleaseSeedanceVideoBilling(ctx context.Context, requestID string, userID, apiKeyID int64) error {
	if s == nil || s.cache == nil {
		return errors.New("seedance video billing cache is unavailable")
	}
	key := seedanceVideoPendingBillingKey(requestID, userID, apiKeyID)
	if key == "" {
		return errors.New("seedance video billing key is invalid")
	}
	return s.cache.ReleaseGrokVideoBilled(ctx, key)
}

func StableSeedanceVideoBillingRequestID(taskRequestID string) string {
	taskRequestID = strings.TrimSpace(taskRequestID)
	if taskRequestID == "" {
		return ""
	}
	if strings.HasPrefix(taskRequestID, "seedance-video:") {
		return taskRequestID
	}
	return "seedance-video:" + taskRequestID
}

// SeedanceVideoURL is exposed for callers that need to inspect the completed
// response without depending on Ark's exact nesting in their own code.
func SeedanceVideoURL(body []byte) string { return seedanceVideoURL(body) }
