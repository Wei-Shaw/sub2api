package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type codexModelsHTTPUpstreamStub struct {
	do func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error)
REDACTED

type codexModelsBlockingBody struct {
	ctx         context.Context
	readStarted chan struct{REDACTED
	startedOnce *sync.Once
	release     <-chan struct{REDACTED
	body        *strings.Reader
REDACTED

func (b *codexModelsBlockingBody) Read(p []byte) (int, error) {
	b.startedOnce.Do(func() { close(b.readStarted) REDACTED)
	select {
	case <-b.release:
		return b.body.Read(p)
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
REDACTED
REDACTED

func (b *codexModelsBlockingBody) Close() error { return nil REDACTED

func (s *codexModelsHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return s.do(req, proxyURL, accountID, accountConcurrency)
REDACTED

func (s *codexModelsHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

func newCodexModelsAPIKeyTestService(upstream HTTPUpstream) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
			Enabled: false,
	REDACTEDREDACTEDREDACTED,
		httpUpstream: upstream,
REDACTED
REDACTED

func newCodexModelsAPIKeyTestAccount(baseURL string) *Account {
	credentials := map[string]any{"api_key": "sk-upstream"REDACTED
	if baseURL != "" {
		credentials["base_url"] = baseURL
REDACTED
REDACTED
		ID:          2,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Credentials: credentials,
		Concurrency: 3,
REDACTED
REDACTED

func newCodexModelsTestAccount() *Account {
REDACTED
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acc-123",
	REDACTED,
REDACTED
REDACTED

func TestFetchCodexModelsManifestPassthrough(t *testing.T) {
	manifestBody := `{"models":[{"slug":"gpt-5.5","display_name":"GPT-5.5"REDACTED]REDACTED`

	var gotAuth, gotAccountID, gotOriginator, gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccountID = r.Header.Get("chatgpt-account-id")
		gotOriginator = r.Header.Get("Originator")
		gotClientVersion = r.URL.Query().Get("client_version")
		w.Header().Set("ETag", `W/"abc123"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(manifestBody))
REDACTED))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original REDACTED()

	s := &OpenAIGatewayService{REDACTED
	manifest, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", "")
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
REDACTED

	if string(manifest.Body) != manifestBody {
		t.Errorf("body not passed through verbatim: got %q", manifest.Body)
REDACTED
	if manifest.ETag != `W/"abc123"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
REDACTED
	if gotAuth != "Bearer test-access-token" {
		t.Errorf("authorization header: got %q", gotAuth)
REDACTED
	if gotAccountID != "acc-123" {
		t.Errorf("chatgpt-account-id header: got %q", gotAccountID)
REDACTED
	if gotOriginator != "codex_cli_rs" {
		t.Errorf("originator header: got %q", gotOriginator)
REDACTED
	if gotClientVersion != "0.137.0" {
		t.Errorf("client_version query: got %q", gotClientVersion)
REDACTED
REDACTED

func TestFetchCodexModelsManifestDefaultClientVersion(t *testing.T) {
	var gotClientVersion string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotClientVersion = r.URL.Query().Get("client_version")
		_, _ = w.Write([]byte(`{"models":[]REDACTED`))
REDACTED))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original REDACTED()

	s := &OpenAIGatewayService{REDACTED
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "", ""); err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
REDACTED
	if gotClientVersion != openAICodexProbeVersion {
		t.Errorf("default client_version: got %q, want %q", gotClientVersion, openAICodexProbeVersion)
REDACTED
REDACTED

func TestFetchCodexModelsManifestNotModified(t *testing.T) {
	var gotIfNoneMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"abc123"`)
		w.WriteHeader(http.StatusNotModified)
REDACTED))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original REDACTED()

	s := &OpenAIGatewayService{REDACTED
	manifest, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", `W/"abc123"`)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
REDACTED
	if !manifest.NotModified {
		t.Error("expected NotModified to be true")
REDACTED
	if gotIfNoneMatch != `W/"abc123"` {
		t.Errorf("if-none-match header: got %q", gotIfNoneMatch)
REDACTED
REDACTED

func TestFetchCodexModelsManifestUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"boom"REDACTED`, http.StatusInternalServerError)
REDACTED))
	defer server.Close()

	original := chatgptCodexModelsURL
	chatgptCodexModelsURL = server.URL
	defer func() { chatgptCodexModelsURL = original REDACTED()

	s := &OpenAIGatewayService{REDACTED
	if _, err := s.FetchCodexModelsManifest(context.Background(), newCodexModelsTestAccount(), "0.137.0", ""); err == nil {
		t.Fatal("expected error for upstream 500, got nil")
REDACTED
REDACTED

func TestFetchCodexModelsManifestMissingToken(t *testing.T) {
	account := newCodexModelsTestAccount()
	delete(account.Credentials, "access_token")

	s := &OpenAIGatewayService{REDACTED
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.137.0", ""); err == nil {
		t.Fatal("expected error for missing access token, got nil")
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyCustomUpstream(t *testing.T) {
	manifestBody := `{"models":[{"slug":"gpt-5.6"REDACTED]REDACTED`
	var gotRequest *http.Request
	var gotProxyURL string
	var gotAccountID int64
	var gotConcurrency int
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
		gotRequest = req
		gotProxyURL = proxyURL
		gotAccountID = accountID
		gotConcurrency = accountConcurrency
		header := make(http.Header)
		header.Set("ETag", `W/"api-key-manifest"`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(manifestBody)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	manifest, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1"),
		"0.144.0",
		"",
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
REDACTED

	if gotRequest == nil {
		t.Fatal("expected request to custom API key upstream")
REDACTED
	if gotRequest.Method != http.MethodGet {
		t.Errorf("method: got %q", gotRequest.Method)
REDACTED
	if gotRequest.URL.String() != "https://upstream.example/v1/models?client_version=0.144.0" {
		t.Errorf("request URL: got %q", gotRequest.URL.String())
REDACTED
	if gotRequest.Header.Get("Authorization") != "Bearer sk-upstream" {
		t.Errorf("authorization header: got %q", gotRequest.Header.Get("Authorization"))
REDACTED
	if gotRequest.Header.Get("Originator") != "codex_cli_rs" {
		t.Errorf("originator header: got %q", gotRequest.Header.Get("Originator"))
REDACTED
	if gotRequest.Header.Get("Version") != "0.144.0" {
		t.Errorf("version header: got %q", gotRequest.Header.Get("Version"))
REDACTED
	if gotRequest.Header.Get("User-Agent") != codexCLIUserAgent {
		t.Errorf("user-agent header: got %q", gotRequest.Header.Get("User-Agent"))
REDACTED
	if gotRequest.Header.Get("chatgpt-account-id") != "" {
		t.Errorf("chatgpt-account-id must not be sent to API key upstream: got %q", gotRequest.Header.Get("chatgpt-account-id"))
REDACTED
	if gotProxyURL != "" || gotAccountID != 2 || gotConcurrency != 3 {
		t.Errorf("upstream routing metadata: proxy=%q account_id=%d concurrency=%d", gotProxyURL, gotAccountID, gotConcurrency)
REDACTED
	if string(manifest.Body) != manifestBody {
		t.Errorf("body not passed through verbatim: got %q", manifest.Body)
REDACTED
	if manifest.ETag != `W/"api-key-manifest"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeySharedRefreshSurvivesCallerCancellation(t *testing.T) {
	const manifestBody = `{"models":[{"slug":"gpt-5.6"REDACTED]REDACTED`
	var calls atomic.Int32
	var readStartedOnce sync.Once
	readStarted := make(chan struct{REDACTED)
	deadlineRemaining := make(chan time.Duration, 1)
	release := make(chan struct{REDACTED)
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		deadline, ok := req.Context().Deadline()
		if !ok {
			deadlineRemaining <- 0
	REDACTED else {
			deadlineRemaining <- time.Until(deadline)
	REDACTED
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Etag": []string{`W/"shared"`REDACTEDREDACTED,
			Body: &codexModelsBlockingBody{
				ctx:         req.Context(),
				readStarted: readStarted,
				startedOnce: &readStartedOnce,
				release:     release,
				body:        strings.NewReader(manifestBody),
		REDACTED,
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstErr := make(chan error, 1)
	go func() {
		_, err := s.FetchCodexModelsManifest(firstCtx, account, "0.144.0", "")
		firstErr <- err
REDACTED()

	select {
	case <-readStarted:
	case <-time.After(time.Second):
		t.Fatal("upstream body read did not start")
REDACTED
	remaining := <-deadlineRemaining
	if remaining < 14*time.Second || remaining > codexModelsManifestRequestTimeout {
		t.Errorf("detached refresh deadline: got %s, want approximately %s", remaining, codexModelsManifestRequestTimeout)
REDACTED
	cancelFirst()
	select {
	case err := <-firstErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("first caller error: got %v, want context.Canceled", err)
	REDACTED
	case <-time.After(time.Second):
		t.Fatal("canceled caller did not return promptly")
REDACTED

	secondResult := make(chan struct {
		manifest *CodexModelsManifest
		err      error
REDACTED, 1)
	go func() {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		secondResult <- struct {
			manifest *CodexModelsManifest
			err      error
	REDACTED{manifest: manifest, err: errREDACTED
REDACTED()

	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream calls before shared refresh completed: got %d, want 1", got)
REDACTED
	close(release)
	select {
	case result := <-secondResult:
		if result.err != nil {
			t.Fatalf("second caller returned error: %v", result.err)
	REDACTED
		if string(result.manifest.Body) != manifestBody {
			t.Errorf("second caller body: got %q", result.manifest.Body)
	REDACTED
	case <-time.After(time.Second):
		t.Fatal("second caller did not receive shared refresh result")
REDACTED
	if got := calls.Load(); got != 1 {
		t.Errorf("total upstream calls: got %d, want 1", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyConcurrentRequestsShareRefresh(t *testing.T) {
	const callers = 8
	var calls atomic.Int32
	started := make(chan struct{REDACTED)
	var startedOnce sync.Once
	release := make(chan struct{REDACTED)
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		startedOnce.Do(func() { close(started) REDACTED)
		<-release
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	begin := make(chan struct{REDACTED)
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			<-begin
			_, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
			errs <- err
	REDACTED()
REDACTED
	close(begin)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
REDACTED
	time.Sleep(50 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("concurrent upstream calls: got %d, want 1", got)
REDACTED
	close(release)
	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Errorf("caller %d returned error: %v", i, err)
	REDACTED
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyFreshCacheHandlesETagLocally(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		if got := req.Header.Get("If-None-Match"); got != "" {
			t.Errorf("cache refresh must not inherit a caller's If-None-Match: got %q", got)
	REDACTED
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Etag": []string{`W/"cached"`REDACTEDREDACTED,
			Body:       io.NopCloser(strings.NewReader(`{"models":[]REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
		t.Fatalf("initial fetch returned error: %v", err)
REDACTED
	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", `W/"cached"`)
	if err != nil {
		t.Fatalf("cached fetch returned error: %v", err)
REDACTED
	if !manifest.NotModified {
		t.Fatal("matching cached ETag must return NotModified")
REDACTED
	if got := calls.Load(); got != 1 {
		t.Errorf("upstream calls: got %d, want 1", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyCacheKeyIsolatesRequestIdentity(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED
	s := newCodexModelsAPIKeyTestService(upstream)

	base := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	fetch := func(account *Account, version string) {
	REDACTED
		if _, err := s.FetchCodexModelsManifest(context.Background(), account, version, ""); err != nil {
			t.Fatalf("fetch returned error: %v", err)
	REDACTED
REDACTED
	fetch(base, "0.144.0")
	fetch(base, "0.144.0")

	differentAccount := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentAccount.ID = 3
	fetch(differentAccount, "0.144.0")

	differentToken := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentToken.Credentials["api_key"] = "sk-other"
	fetch(differentToken, "0.144.0")

	differentUpstream := newCodexModelsAPIKeyTestAccount("https://other-upstream.example")
	fetch(differentUpstream, "0.144.0")
	fetch(base, "0.145.0")

	differentHeaders := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentHeaders.Credentials[credKeyHeaderOverrideEnabled] = true
	differentHeaders.Credentials[credKeyHeaderOverrides] = map[string]any{"x-tenant": "other"REDACTED
	fetch(differentHeaders, "0.144.0")

	proxyID := int64(9)
	differentProxy := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	differentProxy.ProxyID = &proxyID
	differentProxy.Proxy = &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8080REDACTED
	fetch(differentProxy, "0.144.0")
	fetch(differentProxy, "0.144.0")

	if got := calls.Load(); got != 7 {
		t.Errorf("isolated upstream calls: got %d, want 7", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyCacheBoundsEntriesAndBodySize(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		body := `{"models":[]REDACTED`
		if strings.Contains(req.URL.Host, "large") {
			body = strings.Repeat("x", (1<<20)+1)
	REDACTED
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
	REDACTED, nil
REDACTEDREDACTED
	s := newCodexModelsAPIKeyTestService(upstream)
	fetch := func(account *Account) {
	REDACTED
		if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
			t.Fatalf("fetch returned error: %v", err)
	REDACTED
REDACTED

	small := newCodexModelsAPIKeyTestAccount("https://small.example")
	fetch(small)
	fetch(small)
	large := newCodexModelsAPIKeyTestAccount("https://large.example")
	large.ID = 3
	fetch(large)
	fetch(large)
	if got := calls.Load(); got != 3 {
		t.Fatalf("body-size bounded cache calls: got %d, want 3", got)
REDACTED

	for i := int64(10); i < 75; i++ {
		account := newCodexModelsAPIKeyTestAccount("https://bounded.example")
		account.ID = i
		fetch(account)
REDACTED
	last := newCodexModelsAPIKeyTestAccount("https://bounded.example")
	last.ID = 74
	fetch(last)
	if got := calls.Load(); got != 68 {
		t.Fatalf("most recent cache entry was not retained: calls=%d, want 68", got)
REDACTED
	first := newCodexModelsAPIKeyTestAccount("https://bounded.example")
	first.ID = 10
	fetch(first)
	if got := calls.Load(); got != 69 {
		t.Errorf("oldest cache entry was not evicted: calls=%d, want 69", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyServesStaleWhileRefreshing(t *testing.T) {
	var calls atomic.Int32
	refreshStarted := make(chan struct{REDACTED)
	releaseRefresh := make(chan struct{REDACTED)
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		call := calls.Add(1)
		body := `{"models":[{"slug":"old"REDACTED]REDACTED`
		if call > 1 {
			if call == 2 {
				close(refreshStarted)
		REDACTED
			<-releaseRefresh
			body = `{"models":[{"slug":"new"REDACTED]REDACTED`
	REDACTED
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
	REDACTED, nil
REDACTEDREDACTED
	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
		t.Fatalf("initial fetch returned error: %v", err)
REDACTED

	s.codexModelsManifestCache.mu.Lock()
	for key, entry := range s.codexModelsManifestCache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		s.codexModelsManifestCache.entries[key] = entry
REDACTED
	s.codexModelsManifestCache.mu.Unlock()

	resultCh := make(chan struct {
		manifest *CodexModelsManifest
		err      error
REDACTED, 1)
	go func() {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		resultCh <- struct {
			manifest *CodexModelsManifest
			err      error
	REDACTED{manifest: manifest, err: errREDACTED
REDACTED()
	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
REDACTED

	var staleResult struct {
		manifest *CodexModelsManifest
		err      error
REDACTED
	select {
	case staleResult = <-resultCh:
	case <-time.After(100 * time.Millisecond):
		t.Error("stale manifest was not returned while refresh was blocked")
		close(releaseRefresh)
		staleResult = <-resultCh
REDACTED
	if staleResult.err != nil {
		t.Fatalf("stale fetch returned error: %v", staleResult.err)
REDACTED
	if got := string(staleResult.manifest.Body); got != `{"models":[{"slug":"old"REDACTED]REDACTED` {
		t.Errorf("stale body: got %q", got)
REDACTED
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls during stale refresh: got %d, want 2", got)
REDACTED

	select {
	case <-releaseRefresh:
	default:
		close(releaseRefresh)
REDACTED
	deadline := time.Now().Add(time.Second)
	for {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		if err == nil && string(manifest.Body) == `{"models":[{"slug":"new"REDACTED]REDACTED` {
			break
	REDACTED
		if time.Now().After(deadline) {
			t.Fatalf("refreshed manifest was not cached: manifest=%v err=%v", manifest, err)
	REDACTED
		time.Sleep(10 * time.Millisecond)
REDACTED
	if got := calls.Load(); got != 2 {
		t.Errorf("stale refresh was not deduplicated: calls=%d, want 2", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyRevalidatesStaleETag(t *testing.T) {
	var calls atomic.Int32
	refreshDone := make(chan struct{REDACTED)
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		call := calls.Add(1)
		if call == 1 {
			header := make(http.Header)
			header.Set("ETag", `W/"cached"`)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"cached"REDACTED]REDACTED`)),
		REDACTED, nil
	REDACTED
		if got := req.Header.Get("If-None-Match"); got != `W/"cached"` {
			t.Errorf("background revalidation If-None-Match: got %q", got)
	REDACTED
		close(refreshDone)
		header := make(http.Header)
		header.Set("ETag", `W/"cached"`)
		return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBodyREDACTED, nil
REDACTEDREDACTED
	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	if _, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", ""); err != nil {
		t.Fatalf("initial fetch returned error: %v", err)
REDACTED
	s.codexModelsManifestCache.mu.Lock()
	for key, entry := range s.codexModelsManifestCache.entries {
		entry.expiresAt = time.Now().Add(-time.Second)
		s.codexModelsManifestCache.entries[key] = entry
REDACTED
	s.codexModelsManifestCache.mu.Unlock()

	manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
	if err != nil {
		t.Fatalf("stale fetch returned error: %v", err)
REDACTED
	if got := string(manifest.Body); got != `{"models":[{"slug":"cached"REDACTED]REDACTED` {
		t.Fatalf("stale body: got %q", got)
REDACTED
	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("ETag revalidation did not complete")
REDACTED

	deadline := time.Now().Add(time.Second)
	for {
		s.codexModelsManifestCache.mu.Lock()
		fresh := false
		for _, entry := range s.codexModelsManifestCache.entries {
			fresh = time.Now().Before(entry.expiresAt)
	REDACTED
		s.codexModelsManifestCache.mu.Unlock()
		if fresh {
			break
	REDACTED
		if time.Now().After(deadline) {
			t.Fatal("304 revalidation did not renew the cached manifest")
	REDACTED
		time.Sleep(10 * time.Millisecond)
REDACTED
	manifest, err = s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
	if err != nil || string(manifest.Body) != `{"models":[{"slug":"cached"REDACTED]REDACTED` {
		t.Fatalf("renewed cached manifest: body=%q err=%v", manifest.Body, err)
REDACTED
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls: got %d, want 2", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyColdCacheHandlesNotModifiedLocally(t *testing.T) {
	var gotIfNoneMatch string
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		gotIfNoneMatch = req.Header.Get("If-None-Match")
		header := make(http.Header)
		header.Set("ETag", `W/"api-key-manifest"`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       io.NopCloser(strings.NewReader(`{"models":[]REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	manifest, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example"),
		"0.144.0",
		`W/"api-key-manifest"`,
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
REDACTED
	if !manifest.NotModified {
		t.Error("expected NotModified to be true")
REDACTED
	if manifest.ETag != `W/"api-key-manifest"` {
		t.Errorf("etag not passed through: got %q", manifest.ETag)
REDACTED
	if gotIfNoneMatch != "" {
		t.Errorf("cold shared refresh must not inherit caller if-none-match: got %q", gotIfNoneMatch)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyDoesNotCacheUnexpectedColdNotModified(t *testing.T) {
	var calls atomic.Int32
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		calls.Add(1)
		if got := req.Header.Get("If-None-Match"); got != "" {
			t.Errorf("cold shared refresh If-None-Match: got %q", got)
	REDACTED
		header := make(http.Header)
		header.Set("ETag", `W/"unexpected"`)
		return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBodyREDACTED, nil
REDACTEDREDACTED
	s := newCodexModelsAPIKeyTestService(upstream)
	account := newCodexModelsAPIKeyTestAccount("https://upstream.example")
	for i := 0; i < 2; i++ {
		manifest, err := s.FetchCodexModelsManifest(context.Background(), account, "0.144.0", "")
		if err != nil {
			t.Fatalf("fetch %d returned error: %v", i, err)
	REDACTED
		if !manifest.NotModified {
			t.Fatalf("fetch %d: expected upstream NotModified response", i)
	REDACTED
REDACTED
	if got := calls.Load(); got != 2 {
		t.Errorf("unexpected cold 304 was cached: upstream calls=%d, want 2", got)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyPreservesBaseURLQuery(t *testing.T) {
	var gotURL string
	upstream := &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		gotURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1?tenant=acme"),
		"0.144.0",
		"",
	)
	if err != nil {
		t.Fatalf("FetchCodexModelsManifest returned error: %v", err)
REDACTED
	if gotURL != "https://upstream.example/v1/models?client_version=0.144.0&tenant=acme" {
		t.Errorf("request URL: got %q", gotURL)
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyRejectsBaseURLFragment(t *testing.T) {
	called := false
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		called = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"models":[]REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example/v1#models"),
		"0.144.0",
		"",
	)
	if err == nil {
		t.Fatal("expected invalid upstream base URL error, got nil")
REDACTED
	if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_INVALID" {
		t.Errorf("error reason: got %q", infraerrors.Reason(err))
REDACTED
	if called {
		t.Fatal("fragment-bearing base URL must be rejected before the upstream request")
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyUpstreamError(t *testing.T) {
	upstream := &codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"rate limited"REDACTED`)),
	REDACTED, nil
REDACTEDREDACTED

	s := newCodexModelsAPIKeyTestService(upstream)
	_, err := s.FetchCodexModelsManifest(
		context.Background(),
		newCodexModelsAPIKeyTestAccount("https://upstream.example"),
		"0.144.0",
		"",
	)
	if err == nil {
		t.Fatal("expected error for upstream 429, got nil")
REDACTED
	if infraerrors.Code(err) != http.StatusBadGateway {
		t.Errorf("error status: got %d, want %d", infraerrors.Code(err), http.StatusBadGateway)
REDACTED
	if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_UPSTREAM_FAILED" {
		t.Errorf("error reason: got %q", infraerrors.Reason(err))
REDACTED
REDACTED

func TestFetchCodexModelsManifestAPIKeyRejectsOfficialOpenAIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
REDACTED{
		{name: "missing base URL"REDACTED,
		{name: "official host", baseURL: "https://api.openai.com"REDACTED,
		{name: "official versioned URL", baseURL: "https://API.OPENAI.COM:443/v1/"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newCodexModelsAPIKeyTestService(&codexModelsHTTPUpstreamStub{do: func(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
				t.Fatal("official OpenAI API key must not be used as a Codex manifest upstream")
				return nil, nil
		REDACTEDREDACTED)

			_, err := s.FetchCodexModelsManifest(
				context.Background(),
				newCodexModelsAPIKeyTestAccount(tt.baseURL),
				"0.144.0",
				"",
			)
			if err == nil {
				t.Fatal("expected unsupported API key upstream error, got nil")
		REDACTED
			if infraerrors.Reason(err) != "OPENAI_CODEX_MODELS_API_KEY_UPSTREAM_UNSUPPORTED" {
				t.Errorf("error reason: got %q", infraerrors.Reason(err))
		REDACTED
	REDACTED)
REDACTED
REDACTED
