package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type GrokMediaEndpoint string

const (
	GrokMediaEndpointImagesGenerations GrokMediaEndpoint = "images_generations"
	GrokMediaEndpointImagesEdits       GrokMediaEndpoint = "images_edits"
	GrokMediaEndpointVideosGenerations GrokMediaEndpoint = "videos_generations"
	GrokMediaEndpointVideoStatus       GrokMediaEndpoint = "video_status"
)

func (e GrokMediaEndpoint) RequiresRequestBody() bool {
	return e != GrokMediaEndpointVideoStatus
REDACTED

func (e GrokMediaEndpoint) IsGenerationRequest() bool {
	switch e {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits, GrokMediaEndpointVideosGenerations:
		return true
	default:
		return false
REDACTED
REDACTED

func (e GrokMediaEndpoint) httpMethod() string {
	if e == GrokMediaEndpointVideoStatus {
		return http.MethodGet
REDACTED
	return http.MethodPost
REDACTED

func ExtractGrokMediaModel(contentType string, body []byte) string {
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); model != "" {
		return model
REDACTED
	return extractGrokMediaMultipartModel(contentType, body)
REDACTED

func extractGrokMediaMultipartModel(contentType string, body []byte) string {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return ""
REDACTED
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return ""
REDACTED
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return ""
	REDACTED
		if err != nil {
			return ""
	REDACTED
		if part.FormName() != "model" || part.FileName() != "" {
			continue
	REDACTED
		data, err := io.ReadAll(part)
		if err != nil {
			return ""
	REDACTED
		return strings.TrimSpace(string(data))
REDACTED
REDACTED

func (e GrokMediaEndpoint) upstreamURL(baseURL, requestID string) (string, error) {
	switch e {
	case GrokMediaEndpointImagesGenerations:
		return xai.BuildImagesGenerationsURL(baseURL)
	case GrokMediaEndpointImagesEdits:
		return xai.BuildImagesEditsURL(baseURL)
	case GrokMediaEndpointVideosGenerations:
		return xai.BuildVideosGenerationsURL(baseURL)
	case GrokMediaEndpointVideoStatus:
		return xai.BuildVideoURL(baseURL, requestID)
	default:
		return "", fmt.Errorf("unsupported grok media endpoint: %s", e)
REDACTED
REDACTED

func (s *OpenAIGatewayService) ForwardGrokMedia(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil {
		return nil, fmt.Errorf("grok account is required")
REDACTED
	if account.Platform != PlatformGrok {
		return nil, fmt.Errorf("account platform %s is not supported for grok media", account.Platform)
REDACTED

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
REDACTED
	targetURL, err := endpoint.upstreamURL(account.GetGrokBaseURL(), requestID)
	if err != nil {
		return nil, err
REDACTED

	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
REDACTED
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
REDACTED
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	upstreamReq.Header.Set("Accept", "application/json")
	upstreamReq.Header.Set("User-Agent", "sub2api-grok/1.0")
	if endpoint.RequiresRequestBody() {
		contentType = strings.TrimSpace(contentType)
		if contentType == "" {
			contentType = "application/json"
	REDACTED
		upstreamReq.Header.Set("Content-Type", contentType)
REDACTED

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	requestModel := ExtractGrokMediaModel(contentType, body)
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
		upstreamMsg := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = fmt.Sprintf("xAI upstream returned status %d", resp.StatusCode)
	REDACTED
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			Kind:               "failover",
			Message:            upstreamMsg,
	REDACTED)
		s.handleGrokAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
		if s.shouldFailoverUpstreamError(resp.StatusCode) {
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
		REDACTED
	REDACTED
		writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
		return &OpenAIForwardResult{
			RequestID:       requestIDHeader,
			Model:           requestModel,
			UpstreamModel:   requestModel,
			ResponseHeaders: resp.Header.Clone(),
			Duration:        time.Since(startTime),
	REDACTED, nil
REDACTED

	s.updateGrokUsageSnapshot(ctx, account.ID, xai.ParseQuotaHeaders(resp.Header, resp.StatusCode))
	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
REDACTED
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	return &OpenAIForwardResult{
		RequestID:       requestIDHeader,
		Model:           requestModel,
		UpstreamModel:   requestModel,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
REDACTED, nil
REDACTED

func writeGrokMediaResponse(c *gin.Context, resp *http.Response, body []byte, filter *responseheaders.CompiledHeaderFilter) {
	if c == nil || resp == nil {
		return
REDACTED
	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, filter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/json"
REDACTED
	c.Data(resp.StatusCode, contentType, body)
REDACTED
