package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIStream403AccountRepo struct {
	AccountRepository
	setErrorCalls int
REDACTED

func (r *openAIStream403AccountRepo) SetError(context.Context, int64, string) error {
	r.setErrorCalls++
	return nil
REDACTED

type openAIAuthPolicyAccountRepo struct {
	AccountRepository
	tempCalls     int
	setErrorCalls int
REDACTED

func (r *openAIAuthPolicyAccountRepo) SetTempUnschedulable(context.Context, int64, time.Time, string) error {
	r.tempCalls++
	return nil
REDACTED

func (r *openAIAuthPolicyAccountRepo) SetError(context.Context, int64, string) error {
	r.setErrorCalls++
	return nil
REDACTED

type openAIAuthPolicy403Counter struct {
	counts []int64
REDACTED

func (s *openAIAuthPolicy403Counter) IncrementOpenAI403Count(context.Context, int64, int) (int64, error) {
	if len(s.counts) == 0 {
		return 1, nil
REDACTED
	count := s.counts[0]
	s.counts = s.counts[1:]
	return count, nil
REDACTED

func (*openAIAuthPolicy403Counter) ResetOpenAI403Count(context.Context, int64) error {
	return nil
REDACTED

func TestOpenAIUpstreamAccessStateClassification(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
REDACTED{
		{"workspace_code", `{"detail":{"code":"deactivated_workspace"REDACTEDREDACTED`, trueREDACTED,
		{"disabled_account_message", `{"error":{"message":"Your account is disabled"REDACTEDREDACTED`, falseREDACTED,
		{"suspended_workspace_message", `{"response":{"error":{"message":"This workspace has been suspended"REDACTEDREDACTEDREDACTED`, falseREDACTED,
		{"deactivated_organization_message", `{"detail":{"message":"The organization is deactivated"REDACTEDREDACTED`, falseREDACTED,
		{"scalar_detail", `{"detail":"This workspace has been disabled"REDACTED`, falseREDACTED,
		{"suspended_org_code", `{"error":{"code":"org_suspended"REDACTEDREDACTED`, trueREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			require.Equal(t, tt.want, isOpenAIUpstreamAccessStateError("", body))
			if !tt.want {
				return
		REDACTED
			require.True(t, (&OpenAIGatewayService{REDACTED).shouldFailoverOpenAIUpstreamResponse(http.StatusForbidden, "", body))
			require.True(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeOAuthREDACTED, http.StatusForbidden, body))

			err := newOpenAIUpstreamFailoverError(http.StatusForbidden, nil, body, "", true)
			require.True(t, err.IsCredentialFailure())
			require.Equal(t, GatewayFailureScopeAccount, err.Scope)
			require.Equal(t, OpenAIUpstreamAccessStateReason, err.Reason)
			require.Equal(t, NextAccountRetry, err.NextAccountAction)
			require.False(t, err.RetryableOnSameAccount)
			require.False(t, err.RequestScopedTransient)
			require.Equal(t, http.StatusBadGateway, err.ClientStatusCode)
			require.Equal(t, openAIUpstreamAccessUnavailableClientMessage, err.ClientMessage)
	REDACTED)
REDACTED
REDACTED

func TestOpenAIUpstreamAccessStateDoesNotScanEchoedJSON(t *testing.T) {
	body := []byte(`{"error":{"code":"invalid_request_error","message":"Invalid input"REDACTED,"echo":{"prompt":"my account is disabled"REDACTEDREDACTED`)
	require.False(t, isOpenAIUpstreamAccessStateError("", body))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeOAuthREDACTED, http.StatusBadRequest, body))
REDACTED

func TestOpenAIHTTPAccessStateDoesNotTrustBadRequestMessage(t *testing.T) {
	body := []byte(`{"error":{"type":"invalid_request_error","code":"unknown_parameter","message":"Unknown parameter: account disabled"REDACTEDREDACTED`)
	svc := &OpenAIGatewayService{REDACTED

	require.False(t, isOpenAIUpstreamAccessStateError("", body), "free-form stream messages are not durable account evidence")
	require.False(t, isOpenAIHTTPUpstreamAccessStateError(http.StatusBadRequest, "", body))
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(http.StatusBadRequest, "", body))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeOAuthREDACTED, http.StatusBadRequest, body))

	err := newOpenAIUpstreamFailoverError(http.StatusBadRequest, nil, body, "", false)
	require.False(t, err.IsCredentialFailure())
REDACTED

func TestOpenAIHTTPAccessStateBadRequestDoesNotDisableAccount(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repoREDACTEDREDACTED
	account := &Account{ID: 925, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: trueREDACTED
	body := []byte(`{"error":{"code":"unknown_parameter","message":"Unknown parameter: account disabled"REDACTEDREDACTED`)

	disabled := svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadRequest, nil, body)

	require.False(t, disabled)
	require.Zero(t, repo.setErrorCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIStreamEchoedAccessStateMessageDoesNotDisableOrFailover(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repoREDACTEDREDACTED
	account := &Account{ID: 926, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: trueREDACTED
	payload := []byte(`{"type":"response.failed","response":{"error":{"type":"invalid_request_error","code":"unknown_parameter","message":"Unknown parameter: account disabled"REDACTEDREDACTEDREDACTED`)
	message := extractOpenAISSEErrorMessage(payload)

	require.False(t, isOpenAIUpstreamAccessStateError(message, payload))
	require.False(t, openAIStreamFailedEventShouldFailover(payload, message))
	status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, message, nil)
	require.Equal(t, http.StatusBadGateway, status)
	require.False(t, disabled)
	require.Zero(t, repo.setErrorCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIHTTPAccessStateTrustsStructuredCode(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repoREDACTEDREDACTED
	account := &Account{ID: 930, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: trueREDACTED
	body := []byte(`{"error":{"code":"organization_deactivated","message":"request rejected"REDACTEDREDACTED`)

	require.True(t, isOpenAIHTTPUpstreamAccessStateError(http.StatusBadRequest, "", body))
	require.True(t, svc.shouldFailoverOpenAIUpstreamResponse(http.StatusBadRequest, "", body))
	require.True(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusBadRequest, nil, body))
	require.Equal(t, 1, repo.setErrorCalls)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIHTTPAuthMessagesUseExistingStatusPolicies(t *testing.T) {
	t.Run("oauth 401 remains recoverable", func(t *testing.T) {
		repo := &openAIAuthPolicyAccountRepo{REDACTED
		rateLimits := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		svc := &OpenAIGatewayService{rateLimitService: rateLimitsREDACTED
		account := &Account{ID: 931, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
	REDACTED"refresh_token": "refreshable"REDACTEDREDACTED
		body := []byte(`{"error":{"message":"account is disabled"REDACTEDREDACTED`)

		require.False(t, isOpenAIHTTPUpstreamAccessStateError(http.StatusUnauthorized, "", body))
		require.True(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusUnauthorized, nil, body))
		require.Zero(t, repo.setErrorCalls)
		require.Equal(t, 1, repo.tempCalls)
REDACTED)

	t.Run("403 uses counter cooldown", func(t *testing.T) {
		repo := &openAIAuthPolicyAccountRepo{REDACTED
		counter := &openAIAuthPolicy403Counter{counts: []int64{1REDACTEDREDACTED
		rateLimits := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
		rateLimits.openAI403CounterCache = counter
		svc := &OpenAIGatewayService{rateLimitService: rateLimitsREDACTED
		rateLimits.SetAccountRuntimeBlocker(svc)
		account := &Account{ID: 932, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: trueREDACTED
		body := []byte(`{"error":{"message":"workspace has been suspended"REDACTEDREDACTED`)

		require.False(t, isOpenAIHTTPUpstreamAccessStateError(http.StatusForbidden, "", body))
		require.True(t, svc.handleOpenAIAccountUpstreamError(context.Background(), account, http.StatusForbidden, nil, body))
		require.Zero(t, repo.setErrorCalls)
		require.Equal(t, 1, repo.tempCalls)
REDACTED)
REDACTED

func TestOpenAICyberPolicyWrapped5xxNeverFailsOver(t *testing.T) {
	body := []byte(`{"error":{"code":"cyber_policy","message":"blocked"REDACTEDREDACTED`)
	svc := &OpenAIGatewayService{REDACTED
	require.False(t, svc.shouldFailoverOpenAIUpstreamResponse(http.StatusBadGateway, "wrapped upstream failure", body))
	require.False(t, shouldFailoverOpenAIPassthroughResponse(&Account{Type: AccountTypeOAuthREDACTED, http.StatusBadGateway, body))
REDACTED

func TestOpenAICapacityFailoverCarriesSafeTerminalResponse(t *testing.T) {
	message := "Our servers are currently overloaded. Please try again later."
	body := []byte(`{"error":{"code":"server_is_overloaded","message":"` + message + `"REDACTEDREDACTED`)
	err := newOpenAIUpstreamFailoverError(http.StatusBadRequest, nil, body, message, false)

	require.True(t, err.IsOpenAICapacityShed())
	require.Equal(t, http.StatusServiceUnavailable, err.ClientStatusCode)
	require.Equal(t, message, err.ClientMessage)
	require.NotContains(t, err.ClientMessage, "server_is_overloaded")
REDACTED

func TestOpenAIStreamSemanticStatusesPreservedAcrossTerminalShapes(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		status       int
		wantFailover bool
REDACTED{
		{"unauthorized", `{"type":"error","error":{"type":"authentication_error","code":"invalid_api_key","message":"unauthorized"REDACTEDREDACTED`, http.StatusUnauthorized, trueREDACTED,
		{"forbidden", `{"type":"response.failed","response":{"error":{"type":"permission_error","message":"forbidden"REDACTEDREDACTEDREDACTED`, http.StatusForbidden, falseREDACTED,
		{"rate_limit", `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"REDACTEDREDACTED`, http.StatusTooManyRequests, trueREDACTED,
		{"overload_529", `{"type":"error","error":{"status_code":529,"code":"overloaded","message":"overloaded"REDACTEDREDACTED`, 529, trueREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte(tt.body)
			message := extractOpenAISSEErrorMessage(payload)
			require.Equal(t, tt.status, openAIStreamFailureStatus(payload, message))
			require.Equal(t, tt.wantFailover, openAIStreamErrorEventShouldFailover(payload, message))
	REDACTED)
REDACTED
REDACTED

func TestOpenAIStreamBareErrorUsesSemanticFailover(t *testing.T) {
	payload := []byte(`{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"REDACTEDREDACTED`)
	require.True(t, openAIStreamErrorEventShouldFailover(payload, "slow down"))
REDACTED

func TestOpenAIStream403FailoverRequiresStructuredAccountCredentialSignal(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    bool
REDACTED{
		{
			name:    "ordinary permission error",
			payload: `{"type":"response.failed","response":{"error":{"type":"permission_error","code":"forbidden","message":"access denied for this request"REDACTEDREDACTEDREDACTED`,
	REDACTED,
		{
			name:    "explicit 403 request status",
			payload: `{"type":"error","error":{"type":"permission_error","code":"forbidden","status_code":403,"message":"forbidden content"REDACTEDREDACTED`,
	REDACTED,
		{
			name:    "structured access state",
			payload: `{"type":"response.failed","response":{"error":{"code":"workspace_suspended","message":"workspace is suspended"REDACTEDREDACTEDREDACTED`,
			want:    true,
	REDACTED,
		{
			name:    "explicit credential auth code",
			payload: `{"type":"error","error":{"type":"permission_error","code":"invalid_api_key","status_code":403,"message":"credential rejected"REDACTEDREDACTED`,
			want:    true,
	REDACTED,
		{
			name:    "explicit authentication type",
			payload: `{"type":"error","error":{"type":"authentication_error","status_code":403,"message":"credential rejected"REDACTEDREDACTED`,
			want:    true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte(tt.payload)
			message := extractOpenAISSEErrorMessage(payload)
			require.Equal(t, http.StatusForbidden, openAIStreamFailureStatus(payload, message))
			require.Equal(t, tt.want, openAIStreamFailedEventShouldFailover(payload, message))
			require.Equal(t, tt.want, openAIStreamErrorEventShouldFailover(payload, message))
	REDACTED)
REDACTED
REDACTED

func TestOpenAIStream403PostOutputAccountSideEffectsIgnoreRequestPermissionErrors(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	rateLimits := &RateLimitService{accountRepo: repoREDACTED
	svc := &OpenAIGatewayService{rateLimitService: rateLimitsREDACTED
	account := &Account{ID: 918, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	payload := []byte(`{"type":"error","error":{"type":"permission_error","code":"forbidden","status_code":403,"message":"access denied for this request"REDACTEDREDACTED`)

	status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, "access denied for this request", nil)

	require.Equal(t, http.StatusForbidden, status)
	require.False(t, disabled)
	require.Zero(t, repo.setErrorCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIStream403ExplicitCredentialAuthAppliesAccountSideEffects(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	rateLimits := &RateLimitService{accountRepo: repoREDACTED
	svc := &OpenAIGatewayService{rateLimitService: rateLimitsREDACTED
	account := &Account{ID: 917, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	payload := []byte(`{"type":"error","error":{"type":"permission_error","code":"invalid_api_key","status_code":403,"message":"credential rejected"REDACTEDREDACTED`)

	status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, "credential rejected", nil)

	require.Equal(t, http.StatusForbidden, status)
	require.True(t, disabled)
	require.Equal(t, 1, repo.setErrorCalls)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIWSStandaloneFailedStructured403AppliesAccountSideEffectsOnce(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repoREDACTEDREDACTED
	account := &Account{ID: 923, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	failed := []byte(`{"type":"response.failed","response":{"error":{"type":"permission_error","code":"invalid_api_key","status_code":403,"message":"credential rejected"REDACTEDREDACTEDREDACTED`)

	require.True(t, svc.handleOpenAIWSFailureAccountSideEffects(context.Background(), account, "gpt-5", nil, failed))
	require.Equal(t, 1, repo.setErrorCalls)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIWSPairedStructured403SideEffectsCanBeDeduplicated(t *testing.T) {
	repo := &openAIStream403AccountRepo{REDACTED
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repoREDACTEDREDACTED
	account := &Account{ID: 924, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	errorEvent := []byte(`{"type":"error","error":{"code":"workspace_suspended","status_code":403,"message":"workspace is suspended"REDACTEDREDACTED`)
	failedEvent := []byte(`{"type":"response.failed","response":{"error":{"code":"workspace_suspended","status_code":403,"message":"workspace is suspended"REDACTEDREDACTEDREDACTED`)

	applied := svc.handleOpenAIWSFailureAccountSideEffects(context.Background(), account, "gpt-5", nil, errorEvent)
	if !applied {
		applied = svc.handleOpenAIWSFailureAccountSideEffects(context.Background(), account, "gpt-5", nil, failedEvent)
REDACTED

	require.True(t, applied)
	require.Equal(t, 1, repo.setErrorCalls)
REDACTED

func TestOpenAIStreamAccessStateAppliesAccountHealthBeforeFailover(t *testing.T) {
	svc := &OpenAIGatewayService{REDACTED
	account := &Account{ID: 919, Platform: PlatformOpenAI, Type: AccountTypeSetupTokenREDACTED
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"workspace_suspended","message":"workspace is suspended"REDACTEDREDACTEDREDACTED`)

	status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, "workspace is suspended", nil)

	require.Equal(t, http.StatusForbidden, status)
	require.True(t, disabled)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
REDACTED

func TestOpenAIStreamPairedFailureAppliesAccountSideEffectsOnce(t *testing.T) {
	const upstream = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"REDACTED\n\n" +
		"data: {\"type\":\"error\",\"error\":{\"status_code\":403,\"code\":\"workspace_suspended\",\"message\":\"workspace is suspended\"REDACTEDREDACTED\n\n" +
		"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"error\":{\"status_code\":403,\"code\":\"workspace_suspended\",\"message\":\"workspace is suspended\"REDACTEDREDACTEDREDACTED\n\n"

	t.Run("native", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		repo := &openAIStream403AccountRepo{REDACTED
		svc := &OpenAIGatewayService{
			cfg:              &config.Config{REDACTED,
			toolCorrector:    NewCodexToolCorrector(),
			rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
	REDACTED
		account := &Account{ID: 921, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
		recorder := newOpenAIResponseFlushRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(upstream)),
	REDACTED

		result, err := svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5")

	REDACTED
		require.NotNil(t, result)
		require.Equal(t, 1, repo.setErrorCalls)
REDACTED)

	t.Run("passthrough", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		repo := &openAIStream403AccountRepo{REDACTED
		svc := &OpenAIGatewayService{
			cfg:              &config.Config{REDACTED,
			rateLimitService: &RateLimitService{accountRepo: repoREDACTED,
	REDACTED
		account := &Account{ID: 922, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		writer := &passthroughFlushTestWriter{
			ResponseWriter:  c.Writer,
			recorder:        recorder,
			failAfterWrites: -1,
	REDACTED
		c.Writer = writer
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(upstream)),
	REDACTED

		result, err := svc.handleStreamingResponsePassthrough(
			context.Background(), resp, c, account, time.Now(), "gpt-5", "gpt-5",
		)

	REDACTED
		require.NotNil(t, result)
		require.Equal(t, 1, repo.setErrorCalls)
REDACTED)
REDACTED

func TestOpenAIStreamOAuthLike429GetsDeadlineWithoutImmediateRuntimeBlock(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupTokenREDACTED {
		t.Run(accountType, func(t *testing.T) {
			svc := &OpenAIGatewayService{REDACTED
			account := &Account{ID: 920, Platform: PlatformOpenAI, Type: accountTypeREDACTED
			payload := []byte(`{"type":"error","error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"slow down"REDACTEDREDACTED`)
			status, disabled := svc.handleOpenAIStreamTerminalAccountSideEffects(nil, account, payload, "slow down", nil)
			err := svc.newOpenAIAccountFailoverError(account, status, nil, payload, "slow down", disabled, false)

			require.Equal(t, http.StatusTooManyRequests, status)
			require.False(t, disabled)
			require.True(t, err.RetryableOnSameAccount)
			require.False(t, err.SameAccountRetryDeadline.IsZero())
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	REDACTED)
REDACTED
REDACTED
