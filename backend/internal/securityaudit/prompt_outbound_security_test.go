package securityaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type staticResolver struct{ addresses []netip.Addr REDACTED

func (r staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addresses, nil
REDACTED

func TestNormalizeBaseURLSecurity(t *testing.T) {
	allowed := []string{"https://guard.example.com", "https://guard.example.com/v1", "http://127.0.0.1:8080", "http://10.0.0.8:8080"REDACTED
	for _, raw := range allowed {
		_, err := NormalizeBaseURL(raw)
		require.NoError(t, err, raw)
REDACTED
	blocked := []string{
		"ftp://guard.example.com", "http://guard.example.com", "https://user:pass@guard.example.com",
		"https://guard.example.com?q=secret", "https://guard.example.com/#fragment", "http://169.254.169.254",
		"https://metadata.google.internal", "https://0.0.0.0", "https://224.0.0.1", "https://192.0.2.1",
		"https://[::]", "https://[fe80::1]", "https://[ff02::1]", "https://[2001:db8::1]",
REDACTED
	for _, raw := range blocked {
		_, err := NormalizeBaseURL(raw)
		require.Error(t, err, raw)
REDACTED
	url, err := ChatCompletionsURL("https://guard.example.com/v1")
REDACTED
	require.Equal(t, "https://guard.example.com/v1/chat/completions", url)
REDACTED

func TestSecureDialRejectsDNSRebindingToPrivateAddress(t *testing.T) {
	dial := secureDialContext(nil, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("127.0.0.1")REDACTEDREDACTED, false)
	_, err := dial(context.Background(), "tcp", "guard.example.com:443")
REDACTED
REDACTED

func TestSecureHTTPClientDoesNotBypassDestinationValidationThroughEnvironmentProxy(t *testing.T) {
	client, err := NewSecureHTTPClient(ActiveEndpoint{BaseURL: "https://guard.example.com", TimeoutMS: 1000REDACTED)
REDACTED
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy)
REDACTED

func TestOpenAICompatibleScannerRequestContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, DefaultGuardModel, payload["model"])
		require.Equal(t, float64(0), payload["temperature"])
		require.Equal(t, float64(64), payload["max_tokens"])
		require.Equal(t, float64(42), payload["seed"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"REDACTEDREDACTED]REDACTED`))
REDACTED))
	defer server.Close()
	scanner := NewOpenAICompatibleScanner()
	result, err := scanner.Scan(context.Background(), ActiveEndpoint{ID: "one", BaseURL: server.URL, Model: DefaultGuardModel, Token: "token", TimeoutMS: 1000REDACTED, "hello", AllScannerIDs)
REDACTED
	require.Equal(t, EventPass, result.Decision)
REDACTED

func TestOpenAICompatibleScannerRejectsRedirectAndOversize(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/other", http.StatusFound)
REDACTED))
	defer redirect.Close()
	_, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "redirect", BaseURL: redirect.URL, Model: DefaultGuardModel, TimeoutMS: 1000REDACTED, "hello", AllScannerIDs)
REDACTED
	oversize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxGuardResponseBytes)+1)))
REDACTED))
	defer oversize.Close()
	_, err = NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "large", BaseURL: oversize.URL, Model: DefaultGuardModel, TimeoutMS: 1000REDACTED, "hello", AllScannerIDs)
REDACTED
REDACTED

func TestOpenAICompatibleScannerClassifiesHTTPConnectionAndTimeoutFailures(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		retryable bool
REDACTED{
		{name: "authentication", status: http.StatusUnauthorized, retryable: falseREDACTED,
		{name: "forbidden", status: http.StatusForbidden, retryable: falseREDACTED,
		{name: "rate limited", status: http.StatusTooManyRequests, retryable: trueREDACTED,
		{name: "server failure", status: http.StatusBadGateway, retryable: trueREDACTED,
		{name: "other client error", status: http.StatusBadRequest, retryable: falseREDACTED,
REDACTED
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
		REDACTED))
			defer server.Close()
			_, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "status", BaseURL: server.URL, Model: DefaultGuardModel, TimeoutMS: 1000REDACTED, "hello", AllScannerIDs)
			var guardErr *GuardError
			require.ErrorAs(t, err, &guardErr)
			require.Equal(t, ErrorCodeUnavailable, guardErr.Code)
			require.Equal(t, tt.status, guardErr.HTTPStatus)
			require.Equal(t, tt.retryable, guardErr.Retryable)
			require.NotContains(t, err.Error(), server.URL)
	REDACTED)
REDACTED

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {REDACTED))
	closedURL := closed.URL
	closed.Close()
	_, err := NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "closed", BaseURL: closedURL, Model: DefaultGuardModel, TimeoutMS: 100REDACTED, "hello", AllScannerIDs)
	var connectionErr *GuardError
	require.ErrorAs(t, err, &connectionErr)
	require.True(t, connectionErr.Retryable)

	timeout := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
REDACTED))
	defer timeout.Close()
	_, err = NewOpenAICompatibleScanner().Scan(context.Background(), ActiveEndpoint{ID: "timeout", BaseURL: timeout.URL, Model: DefaultGuardModel, TimeoutMS: 20REDACTED, "hello", AllScannerIDs)
	var timeoutErr *GuardError
	require.ErrorAs(t, err, &timeoutErr)
	require.True(t, timeoutErr.Retryable)
	require.True(t, timeoutErr.Timeout)
REDACTED

func TestPromptAuditProbeModelsFallbackAndResponseSafety(t *testing.T) {
	t.Run("models contains configured model", func(t *testing.T) {
		var chatCalls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "Bearer temporary-token", r.Header.Get("Authorization"))
			if r.URL.Path == "/v1/models" {
				_, _ = w.Write([]byte(`{"data":[{"id":"` + DefaultGuardModel + `"REDACTED]REDACTED`))
				return
		REDACTED
			chatCalls.Add(1)
	REDACTED))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")REDACTED)
		require.True(t, result.OK)
		require.True(t, result.TokenApplied)
		require.Equal(t, http.StatusOK, result.HTTPStatus)
		require.Zero(t, chatCalls.Load())
REDACTED)

	t.Run("invalid models response performs real guard fallback", func(t *testing.T) {
		var chatCalls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				_, _ = w.Write([]byte(`{"unexpected":trueREDACTED`))
				return
		REDACTED
			chatCalls.Add(1)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Safety: Safe\nCategories: None"REDACTEDREDACTED]REDACTED`))
	REDACTED))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")REDACTED)
		require.True(t, result.OK)
		require.Equal(t, int64(1), chatCalls.Load())
REDACTED)

	t.Run("fallback authentication failure is stable", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/models" {
				w.WriteHeader(http.StatusNotFound)
				return
		REDACTED
			w.WriteHeader(http.StatusUnauthorized)
	REDACTED))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")REDACTED)
		require.False(t, result.OK)
		require.Equal(t, ErrorCodeUnavailable, result.ErrorCode)
		require.Equal(t, http.StatusUnauthorized, result.HTTPStatus)
		require.False(t, result.Retryable)
REDACTED)

	t.Run("oversized models response is rejected without fallback", func(t *testing.T) {
		var chatCalls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/models" {
				chatCalls.Add(1)
		REDACTED
			_, _ = w.Write([]byte(strings.Repeat("x", int(maxGuardResponseBytes)+1)))
	REDACTED))
		defer server.Close()
		result := newProbeTestService().Probe(context.Background(), ProbeRequest{Endpoint: probeEndpoint(server.URL, "temporary-token")REDACTED)
		require.False(t, result.OK)
		require.Equal(t, "response_too_large", result.ErrorCode)
		require.Zero(t, chatCalls.Load())
REDACTED)
REDACTED

func newProbeTestService() *PromptService {
	return &PromptService{
		config: &ConfigManager{REDACTED, scanner: NewOpenAICompatibleScanner(), clock: realClock{REDACTED,
		probes: map[string]ProbeResult{REDACTED,
REDACTED
REDACTED

func probeEndpoint(baseURL, token string) UpdateEndpoint {
	return UpdateEndpoint{
		ID: "probe-one", Name: "Probe One", Protocol: "openai_compatible", BaseURL: baseURL,
		Model: DefaultGuardModel, Token: token, TimeoutMS: 1000, InputLimit: 1024, Enabled: true,
REDACTED
REDACTED
