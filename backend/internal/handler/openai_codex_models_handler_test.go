package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type codexModelsFailoverAccountRepo struct {
	service.AccountRepository
	accounts []service.Account
REDACTED

func (r codexModelsFailoverAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			account := r.accounts[i]
			return &account, nil
	REDACTED
REDACTED
	return nil, service.ErrNoAvailableAccounts
REDACTED

func (r codexModelsFailoverAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	accounts := make([]service.Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			accounts = append(accounts, account)
	REDACTED
REDACTED
	return accounts, nil
REDACTED

type codexModelsFailoverHTTPUpstream struct {
	service.HTTPUpstream
	mu          sync.Mutex
	accountIDs  []int64
	firstErr    error
	firstStatus int
	statuses    map[int64]int
REDACTED

func (u *codexModelsFailoverHTTPUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accountIDs = append(u.accountIDs, accountID)
	u.mu.Unlock()

	status, hasStatus := u.statuses[accountID]
	if accountID == 1 || hasStatus {
		if u.firstErr != nil {
			return nil, u.firstErr
	REDACTED
		if !hasStatus {
			status = u.firstStatus
	REDACTED
		if status == 0 {
			status = http.StatusServiceUnavailable
	REDACTED
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"error":{"message":"No available OpenAI accounts","type":"upstream_error"REDACTEDREDACTED`,
			)),
	REDACTED, nil
REDACTED
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-5.6-sol"REDACTED]REDACTED`)),
REDACTED, nil
REDACTED

func (u *codexModelsFailoverHTTPUpstream) calls() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.accountIDs...)
REDACTED

func TestCodexModelsCanceledRequestDoesNotWriteResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil).WithContext(ctx)

	h := &OpenAIGatewayHandler{REDACTED
	h.CodexModels(c)

	if c.Writer.Written() {
		t.Fatalf("canceled request wrote an HTTP response: status=%d body=%q", recorder.Code, recorder.Body.String())
REDACTED
REDACTED

func TestCodexModelsFailsOverFromRetryableUpstreamStatus(t *testing.T) {
	retryableStatuses := []int{
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
REDACTED
	for _, status := range retryableStatuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			handler, upstream, groupID := newCodexModelsFailoverTestHandler(status)
			recorder := performCodexModelsRequest(t, handler, groupID)

			if got, want := upstream.calls(), []int64{1, 2REDACTED; !equalInt64Slices(got, want) {
				t.Fatalf("upstream account calls: got %v, want %v", got, want)
		REDACTED
			if recorder.Code != http.StatusOK {
				t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
		REDACTED
			if got, want := recorder.Body.String(), `{"models":[{"slug":"gpt-5.6-sol"REDACTED]REDACTED`; got != want {
				t.Fatalf("body: got %q, want %q", got, want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCodexModelsFailsOverFromUpstreamTransportError(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	upstream.firstErr = &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: errors.New("connection reset"),
REDACTED
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2REDACTED; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
REDACTED
	if recorder.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
REDACTED
REDACTED

func TestCodexModelsDoesNotFailOverFromPermanentUpstreamStatus(t *testing.T) {
	statuses := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		600,
REDACTED
	for _, status := range statuses {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			handler, upstream, groupID := newCodexModelsFailoverTestHandler(status)
			recorder := performCodexModelsRequest(t, handler, groupID)

			if got, want := upstream.calls(), []int64{1REDACTED; !equalInt64Slices(got, want) {
				t.Fatalf("upstream account calls: got %v, want %v", got, want)
		REDACTED
			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCodexModelsDoesNotFailOverFromUpstreamConfigurationError(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	upstream.firstErr = errors.New("invalid proxy URL")
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1REDACTED; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
REDACTED
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
REDACTED
REDACTED

func TestCodexModelsReturnsLastUpstreamErrorWhenAccountsAreExhausted(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandler(http.StatusServiceUnavailable)
	upstream.statuses = map[int64]int{
		1: http.StatusServiceUnavailable,
		2: http.StatusGatewayTimeout,
REDACTED
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2REDACTED; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
REDACTED
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
REDACTED
	if body := recorder.Body.String(); !strings.Contains(body, "upstream error 504") {
		t.Fatalf("body does not preserve the last upstream error: %s", body)
REDACTED
REDACTED

func TestCodexModelsHonorsAccountSwitchLimit(t *testing.T) {
	handler, upstream, groupID := newCodexModelsFailoverTestHandlerWithAccountCount(http.StatusServiceUnavailable, 4, 2)
	upstream.statuses = map[int64]int{
		1: http.StatusServiceUnavailable,
		2: http.StatusBadGateway,
		3: http.StatusGatewayTimeout,
		4: http.StatusInternalServerError,
REDACTED
	recorder := performCodexModelsRequest(t, handler, groupID)

	if got, want := upstream.calls(), []int64{1, 2, 3REDACTED; !equalInt64Slices(got, want) {
		t.Fatalf("upstream account calls: got %v, want %v", got, want)
REDACTED
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want %d; body=%s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
REDACTED
	if body := recorder.Body.String(); !strings.Contains(body, "upstream error 504") {
		t.Fatalf("body does not preserve the limit-ending upstream error: %s", body)
REDACTED
REDACTED

func newCodexModelsFailoverTestHandler(firstStatus int) (*OpenAIGatewayHandler, *codexModelsFailoverHTTPUpstream, int64) {
	return newCodexModelsFailoverTestHandlerWithAccountCount(firstStatus, 2, 3)
REDACTED

func newCodexModelsFailoverTestHandlerWithAccountCount(firstStatus, accountCount, maxSwitches int) (*OpenAIGatewayHandler, *codexModelsFailoverHTTPUpstream, int64) {
	gin.SetMode(gin.TestMode)
	groupID := int64(42)
	accounts := make([]service.Account, 0, accountCount)
	for i := 1; i <= accountCount; i++ {
		accounts = append(accounts, service.Account{
			ID:          int64(i),
			Name:        fmt.Sprintf("upstream-%d", i),
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeAPIKey,
			Status:      service.StatusActive,
			Schedulable: true,
			Priority:    i - 1,
			Concurrency: 1,
	REDACTED
				"api_key":  fmt.Sprintf("sk-%d", i),
				"base_url": fmt.Sprintf("https://upstream-%d.example/v1", i),
		REDACTED,
	REDACTED)
REDACTED
	upstream := &codexModelsFailoverHTTPUpstream{firstStatus: firstStatusREDACTED
	cfg := &config.Config{RunMode: config.RunModeSimpleREDACTED
	gatewayService := service.NewOpenAIGatewayService(
		codexModelsFailoverAccountRepo{accounts: accountsREDACTED,
		nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil,
		upstream,
		nil, nil, nil, nil, nil, nil, nil, nil,
	)
	return &OpenAIGatewayHandler{gatewayService: gatewayService, maxAccountSwitches: maxSwitchesREDACTED, upstream, groupID
REDACTED

func performCodexModelsRequest(t *testing.T, handler *OpenAIGatewayHandler, groupID int64) *httptest.ResponseRecorder {
REDACTED
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/models?client_version=0.144.0", nil)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAIREDACTED,
REDACTED)

	handler.CodexModels(c)
	return recorder
REDACTED

func equalInt64Slices(got, want []int64) bool {
	if len(got) != len(want) {
		return false
REDACTED
	for i := range got {
		if got[i] != want[i] {
			return false
	REDACTED
REDACTED
	return true
REDACTED
