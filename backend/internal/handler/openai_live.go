package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) Live(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED
	if apiKey.Group == nil || apiKey.Group.Platform != service.PlatformOpenAI {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Live is not supported for this platform")
		return
REDACTED
	if !liveEnabledForAPIKey(apiKey) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "Live is not enabled for this group")
		return
REDACTED
	request, err := parseLiveCallRequest(c)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
REDACTED
	model := strings.TrimSpace(gjson.GetBytes(request.Session, "model").String())
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.live",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if decision := h.checkSecurityAudit(
		c,
		reqLog,
		apiKey,
		subject,
		service.ContentModerationProtocolOpenAIResponses,
		model,
		request.Session,
	); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
REDACTED

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if h.billingCacheService == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Billing service unavailable")
		return
REDACTED
	if err := h.billingCacheService.CheckBillingEligibility(
		c.Request.Context(),
		apiKey.User,
		apiKey,
		apiKey.Group,
		subscription,
		service.QuotaPlatform(c.Request.Context(), apiKey),
	); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
	REDACTED
		h.errorResponse(c, status, code, message)
		return
REDACTED

	userRelease, acquired, err := h.concurrencyHelper.TryAcquireUserSlot(
		c.Request.Context(),
		subject.UserID,
		subject.Concurrency,
	)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Live concurrency unavailable")
		return
REDACTED
	if !acquired {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Live concurrency limit reached")
		return
REDACTED
	defer userRelease()

	identity := liveCallIdentity(c, apiKey, subject.UserID, subscription)
	created, err := h.gatewayService.CreateLiveCall(c.Request.Context(), request, identity, subject.Concurrency)
	if err != nil {
		h.writeLiveCreateError(c, err)
		return
REDACTED
	c.Header("Location", liveSidebandLocation(c.FullPath(), created.CallID))
	c.Data(http.StatusOK, "application/sdp", created.SDP)
REDACTED

func parseLiveCallRequest(c *gin.Context) (*service.LiveCallRequest, error) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		sdp := c.PostForm("sdp")
		session := json.RawMessage(c.PostForm("session"))
		request := &service.LiveCallRequest{SDP: sdp, Session: sessionREDACTED
		if err := service.ValidateLiveCallRequest(request); err != nil {
			return nil, err
	REDACTED
		return request, nil
REDACTED
	var request service.LiveCallRequest
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(&request); err != nil {
		return nil, errors.New("request body must be valid JSON")
REDACTED
	if err := decoder.Decode(&struct{REDACTED{REDACTED); !errors.Is(err, io.EOF) {
		return nil, errors.New("request body must contain one JSON object")
REDACTED
	if err := service.ValidateLiveCallRequest(&request); err != nil {
		return nil, err
REDACTED
	return &request, nil
REDACTED

func liveSidebandLocation(fullPath, callID string) string {
	prefix := "/v1/live/"
	if strings.HasPrefix(fullPath, "/backend-api/codex/") {
		prefix = "/backend-api/codex/"
REDACTED
	return prefix + url.PathEscape(callID)
REDACTED

func liveCallIdentity(
	c *gin.Context,
	apiKey *service.APIKey,
	userID int64,
	subscription *service.UserSubscription,
) service.LiveCallIdentity {
	var subscriptionID *int64
	if subscription != nil {
		value := subscription.ID
		subscriptionID = &value
REDACTED
	return service.LiveCallIdentity{
		APIKeyID:        apiKey.ID,
		UserID:          userID,
		GroupID:         apiKey.GroupID,
		SubscriptionID:  subscriptionID,
		UserAgent:       c.GetHeader("User-Agent"),
		IPAddress:       ip.GetClientIP(c),
		InboundEndpoint: GetInboundEndpoint(c),
REDACTED
REDACTED

func (h *OpenAIGatewayHandler) writeLiveCreateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrLiveConcurrencyFull):
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Live concurrency limit reached")
	case errors.Is(err, service.ErrLiveUnavailable):
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Live is unavailable")
	default:
		var attestationErr *service.LiveAttestationUnavailableError
		if errors.As(err, &attestationErr) {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", attestationErr.Error())
			return
	REDACTED
		var upstreamErr *service.UpstreamFailoverError
		if errors.As(err, &upstreamErr) && upstreamErr.StatusCode >= 400 && upstreamErr.StatusCode < 500 {
			h.errorResponse(c, upstreamErr.StatusCode, "invalid_request_error", "Live upstream rejected the request")
			return
	REDACTED
		h.errorResponse(c, http.StatusBadGateway, "api_error", "Live upstream request failed")
REDACTED
REDACTED

func (h *OpenAIGatewayHandler) LiveSideband(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
REDACTED
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
REDACTED
	if !liveEnabledForAPIKey(apiKey) {
		h.errorResponse(c, http.StatusForbidden, "permission_error", "Live is not enabled for this group")
		return
REDACTED
	identity := service.LiveCallIdentity{
		APIKeyID: apiKey.ID,
		UserID:   subject.UserID,
		GroupID:  apiKey.GroupID,
REDACTED
	record, err := h.gatewayService.GetLiveCallForIdentity(c.Request.Context(), c.Param("call_id"), identity)
	if err != nil {
		if errors.Is(err, service.ErrLiveIdentityMismatch) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", "Live call belongs to another identity")
			return
	REDACTED
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Live call not found")
		return
REDACTED
	downstream, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{
		InsecureSkipVerify: true,
REDACTED)
	if err != nil {
		return
REDACTED
	defer downstream.CloseNow()
	if err := h.gatewayService.ProxyLiveSideband(c.Request.Context(), record, downstream); err != nil {
		_ = downstream.Close(coderws.StatusInternalError, "live sideband closed")
		return
REDACTED
	_ = downstream.Close(coderws.StatusNormalClosure, "")
REDACTED

func liveEnabledForAPIKey(apiKey *service.APIKey) bool {
	return apiKey != nil &&
		apiKey.Group != nil &&
		apiKey.Group.Platform == service.PlatformOpenAI &&
		apiKey.Group.AllowLive
REDACTED
