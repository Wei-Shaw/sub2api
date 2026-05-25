package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageConcurrencyLimiter_DefaultDisabledAllowsRequests(t *testing.T) {
	limiter := &imageConcurrencyLimiter{REDACTED

	release, acquired := limiter.TryAcquire(false, 1)

	require.True(t, acquired)
	require.Nil(t, release)
REDACTED

func TestImageConcurrencyLimiter_RejectsWhenLimitReachedAndAllowsAfterRelease(t *testing.T) {
	limiter := &imageConcurrencyLimiter{REDACTED

	release, acquired := limiter.TryAcquire(true, 1)
	require.True(t, acquired)
	require.NotNil(t, release)

	secondRelease, secondAcquired := limiter.TryAcquire(true, 1)
	require.False(t, secondAcquired)
	require.Nil(t, secondRelease)

	release()
	thirdRelease, thirdAcquired := limiter.TryAcquire(true, 1)
	require.True(t, thirdAcquired)
	require.NotNil(t, thirdRelease)
	thirdRelease()
REDACTED

func TestImageConcurrencyLimiter_WaitsUntilSlotReleased(t *testing.T) {
	limiter := &imageConcurrencyLimiter{REDACTED
	release, acquired := limiter.Acquire(context.Background(), true, 1, true, time.Second, 1)
	require.True(t, acquired)
	require.NotNil(t, release)

	acquiredCh := make(chan func(), 1)
	go func() {
		waitRelease, waitAcquired := limiter.Acquire(context.Background(), true, 1, true, time.Second, 1)
		require.True(t, waitAcquired)
		acquiredCh <- waitRelease
REDACTED()

	time.Sleep(20 * time.Millisecond)
	release()

	select {
	case waitRelease := <-acquiredCh:
		require.NotNil(t, waitRelease)
		waitRelease()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for image concurrency slot")
REDACTED
REDACTED

func TestImageConcurrencyLimiter_WaitTimesOut(t *testing.T) {
	limiter := &imageConcurrencyLimiter{REDACTED
	release, acquired := limiter.Acquire(context.Background(), true, 1, true, time.Second, 1)
	require.True(t, acquired)
	require.NotNil(t, release)
	defer release()

	waitRelease, waitAcquired := limiter.Acquire(context.Background(), true, 1, true, 10*time.Millisecond, 1)

	require.False(t, waitAcquired)
	require.Nil(t, waitRelease)
REDACTED

func TestImageConcurrencyLimiter_MaxWaitingRequestsRejectsOverflow(t *testing.T) {
	limiter := &imageConcurrencyLimiter{REDACTED
	release, acquired := limiter.Acquire(context.Background(), true, 1, true, time.Second, 1)
	require.True(t, acquired)
	require.NotNil(t, release)
	defer release()

	waitingStarted := make(chan struct{REDACTED)
	waitingDone := make(chan struct{REDACTED)
	go func() {
		close(waitingStarted)
		waitRelease, waitAcquired := limiter.Acquire(context.Background(), true, 1, true, time.Second, 1)
		if waitAcquired && waitRelease != nil {
			waitRelease()
	REDACTED
		close(waitingDone)
REDACTED()
	<-waitingStarted
	time.Sleep(20 * time.Millisecond)

	overflowRelease, overflowAcquired := limiter.Acquire(context.Background(), true, 1, true, time.Second, 1)

	require.False(t, overflowAcquired)
	require.Nil(t, overflowRelease)
	release()
	<-waitingDone
REDACTED

func TestOpenAIGatewayHandlerAcquireImageGenerationSlot_Returns429WhenFull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	h := &OpenAIGatewayHandler{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				ImageConcurrency: config.ImageConcurrencyConfig{
					Enabled:               true,
					MaxConcurrentRequests: 1,
					OverflowMode:          config.ImageConcurrencyOverflowModeReject,
			REDACTED,
		REDACTED,
	REDACTED,
		imageLimiter: &imageConcurrencyLimiter{REDACTED,
REDACTED
	release, acquired := h.acquireImageGenerationSlot(c, false)
	require.True(t, acquired)
	require.NotNil(t, release)
	defer release()

	blockedRelease, blocked := h.acquireImageGenerationSlot(c, false)

	require.False(t, blocked)
	require.Nil(t, blockedRelease)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, rec.Body.String(), "Image generation concurrency limit exceeded")
REDACTED

func TestOpenAIGatewayHandlerResponses_ImageIntentRejectedByImageConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation"REDACTED]REDACTED`
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	groupID := int64(1)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                   groupID,
			AllowImageGeneration: true,
	REDACTED,
		User: &service.User{ID: 20REDACTED,
REDACTED)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20, Concurrency: 1REDACTED)

	h := &OpenAIGatewayHandler{
		gatewayService:          &service.OpenAIGatewayService{REDACTED,
		billingCacheService:     &service.BillingCacheService{REDACTED,
		apiKeyService:           &service.APIKeyService{REDACTED,
		concurrencyHelper:       &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(&helperConcurrencyCacheStub{userSeq: []bool{trueREDACTEDREDACTED)REDACTED,
		errorPassthroughService: nil,
		cfg: &config.Config{Gateway: config.GatewayConfig{ImageConcurrency: config.ImageConcurrencyConfig{
			Enabled:               true,
			MaxConcurrentRequests: 1,
			OverflowMode:          config.ImageConcurrencyOverflowModeReject,
	REDACTEDREDACTEDREDACTED,
		imageLimiter: &imageConcurrencyLimiter{REDACTED,
REDACTED
	release, acquired := h.acquireImageGenerationSlot(c, false)
	require.True(t, acquired)
	require.NotNil(t, release)
	defer release()
	rec.Body.Reset()
	rec.Code = 0

	h.Responses(c)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, rec.Body.String(), "Image generation concurrency limit exceeded")
REDACTED

func TestOpenAIGatewayHandlerResponses_TextOnlyNotRejectedByImageConcurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-5.4","input":"write code"REDACTED`
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	groupID := int64(1)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                   groupID,
			AllowImageGeneration: true,
	REDACTED,
		User: &service.User{ID: 20REDACTED,
REDACTED)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 20, Concurrency: 1REDACTED)

	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{REDACTED,
		billingCacheService: service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, &config.Config{RunMode: config.RunModeSimpleREDACTED, nil),
		apiKeyService:       &service.APIKeyService{REDACTED,
		concurrencyHelper:   &ConcurrencyHelper{concurrencyService: service.NewConcurrencyService(&helperConcurrencyCacheStub{userSeq: []bool{trueREDACTEDREDACTED)REDACTED,
		cfg: &config.Config{Gateway: config.GatewayConfig{ImageConcurrency: config.ImageConcurrencyConfig{
			Enabled:               true,
			MaxConcurrentRequests: 1,
			OverflowMode:          config.ImageConcurrencyOverflowModeReject,
	REDACTEDREDACTEDREDACTED,
		imageLimiter: &imageConcurrencyLimiter{REDACTED,
REDACTED
	release, acquired := h.acquireImageGenerationSlot(c, false)
	require.True(t, acquired)
	require.NotNil(t, release)
	defer release()
	rec.Body.Reset()
	rec.Code = 0

	h.Responses(c)

	require.NotEqual(t, http.StatusTooManyRequests, rec.Code)
	require.NotContains(t, rec.Body.String(), "Image generation concurrency limit exceeded")
REDACTED
