package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"golang.org/x/sync/singleflight"
)

const (
	grokQuotaUpstreamTimeout = 20 * time.Second
	grokQuotaProbeInput      = "."
	grokQuotaDefaultModel    = grokDefaultResponsesModel
	grokBillingExtraKey      = "grok_billing_snapshot"
)

type GrokQuotaProbeResult struct {
	Source            string              `json:"source"`
	Model             string              `json:"model,omitempty"`
	Billing           *xai.BillingSummary `json:"billing,omitempty"`
	Snapshot          *xai.QuotaSnapshot  `json:"snapshot,omitempty"`
	LocalUsage24h     *WindowStats        `json:"local_usage_24h,omitempty"`
	LocalUsage7d      *WindowStats        `json:"local_usage_7d,omitempty"`
	LocalUsageMonthly *WindowStats        `json:"local_usage_monthly,omitempty"`
	StatusCode        int                 `json:"status_code,omitempty"`
	HeadersObserved   bool                `json:"headers_observed"`
	ResetSupported    bool                `json:"reset_supported"`
	FetchedAt         int64               `json:"fetched_at"`
	Persisted         bool                `json:"persisted"`
	ProbeError        string              `json:"probe_error,omitempty"`
REDACTED

type GrokQuotaResetResult struct {
	Supported bool   `json:"supported"`
	Code      string `json:"code"`
	Message   string `json:"message"`
REDACTED

type GrokQuotaService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *GrokTokenProvider
	httpUpstream  HTTPUpstream
	usageLogRepo  UsageLogRepository
	probeFlight   singleflight.Group
REDACTED

func NewGrokQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *GrokTokenProvider,
	httpUpstream HTTPUpstream,
	usageLogRepos ...UsageLogRepository,
) *GrokQuotaService {
	var usageLogRepo UsageLogRepository
	if len(usageLogRepos) > 0 {
		usageLogRepo = usageLogRepos[0]
REDACTED
	return &GrokQuotaService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
		usageLogRepo:  usageLogRepo,
REDACTED
REDACTED

// QueryQuota combines xAI billing data with an active quota-header probe for
// Free accounts, whose billing response does not include usage_percent.
func (s *GrokQuotaService) QueryQuota(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	billingResult, billingErr := s.ProbeBilling(ctx, accountID)
	if billingErr == nil && billingResult != nil && grokBillingHasAuthoritativeQuota(billingResult.Billing) {
		return billingResult, nil
REDACTED

	probeResult, probeErr := s.ProbeUsage(ctx, accountID)
	if probeErr != nil {
		if billingResult != nil && billingResult.Billing != nil {
			billingResult.ProbeError = probeErr.Error()
			return billingResult, nil
	REDACTED
		return nil, probeErr
REDACTED
	if probeResult == nil {
		if billingErr != nil {
			return nil, billingErr
	REDACTED
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_PROBE_EMPTY", "Grok quota probe returned no result")
REDACTED
	if billingResult != nil {
		probeResult.Source = "hybrid_probe"
		probeResult.Billing = billingResult.Billing
		probeResult.LocalUsage24h = billingResult.LocalUsage24h
		probeResult.LocalUsage7d = billingResult.LocalUsage7d
		probeResult.LocalUsageMonthly = billingResult.LocalUsageMonthly
		probeResult.Persisted = probeResult.Persisted || billingResult.Persisted
REDACTED
	return probeResult, nil
REDACTED

func grokBillingHasAuthoritativeQuota(billing *xai.BillingSummary) bool {
	if billing == nil {
		return false
REDACTED
	return billing.UsagePercent != nil ||
		billing.UsedPercent != nil ||
		(billing.MonthlyLimitCents != nil && *billing.MonthlyLimitCents > 0) ||
		strings.TrimSpace(billing.Plan) != ""
REDACTED

func (s *GrokQuotaService) ProbeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	return s.runProbeFlight(ctx, "active:"+strconv.FormatInt(accountID, 10), func(sharedCtx context.Context) (*GrokQuotaProbeResult, error) {
		return s.probeUsage(sharedCtx, accountID)
REDACTED)
REDACTED

func (s *GrokQuotaService) probeUsage(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	account, token, proxyURL, err := s.prepareProbe(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED

	probeModel := grokQuotaProbeModel()
	body, err := buildGrokQuotaProbeBody(probeModel)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "GROK_QUOTA_PROBE_BODY_ERROR", "failed to build probe body: %v", err)
REDACTED
	targetURL, err := xai.BuildResponsesURL(account.GetGrokBaseURL())
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadRequest, "GROK_QUOTA_BASE_URL_INVALID", "invalid Grok base_url: %v", err)
REDACTED

	callCtx, cancel := context.WithTimeout(ctx, grokQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "GROK_QUOTA_PROBE_REQUEST_BUILD_FAILED", "failed to build upstream request: %v", err)
REDACTED
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	applyGrokCLIHeaders(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_PROBE_REQUEST_FAILED", "upstream probe failed: %v", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	snapshot := xai.ObserveQuotaHeaders(resp.Header, resp.StatusCode, "active_probe")
	resetAt, limited := grokRateLimitResetAt(snapshot, time.Now())
	if limited {
		normalizeGrokExhaustedWindowResets(snapshot, resetAt, time.Now())
REDACTED
	persistErr := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		grokQuotaSnapshotExtraKey: snapshot,
REDACTED)
	if limited {
		persistGrokRateLimit(ctx, s.accountRepo, account, resetAt)
REDACTED

	result := &GrokQuotaProbeResult{
		Source:          "active_probe",
		Model:           probeModel,
		Snapshot:        snapshot,
		StatusCode:      resp.StatusCode,
		HeadersObserved: snapshot.HeadersObserved,
		ResetSupported:  false,
		FetchedAt:       time.Now().Unix(),
		Persisted:       persistErr == nil,
REDACTED
	if resp.StatusCode == http.StatusTooManyRequests {
		return result, nil
REDACTED
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 240))
		bodyText := truncate(strings.TrimSpace(string(bodyBytes)), 240)
		slog.Warn("grok_quota_probe_failed", "account_id", account.ID, "model", probeModel, "status", resp.StatusCode, "body", bodyText)
		return nil, infraerrors.Newf(mapUpstreamStatus(resp.StatusCode), "GROK_QUOTA_PROBE_UPSTREAM_ERROR", "upstream returned %d for probe model %q: %s", resp.StatusCode, probeModel, bodyText)
REDACTED
	return result, nil
REDACTED

// ProbeBilling only calls the xAI billing endpoints. Account usage refreshes
// use this method so opening the account list never consumes model quota.
func (s *GrokQuotaService) ProbeBilling(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	return s.runProbeFlight(ctx, "billing:"+strconv.FormatInt(accountID, 10), func(sharedCtx context.Context) (*GrokQuotaProbeResult, error) {
		return s.probeBilling(sharedCtx, accountID)
REDACTED)
REDACTED

func (s *GrokQuotaService) probeBilling(ctx context.Context, accountID int64) (*GrokQuotaProbeResult, error) {
	account, token, proxyURL, err := s.prepareProbe(ctx, accountID)
	if err != nil {
		return nil, err
REDACTED

	probeCtx, cancel := context.WithTimeout(ctx, grokQuotaUpstreamTimeout)
	defer cancel()
	type billingResult struct {
		summary *xai.BillingSummary
		status  int
		err     error
REDACTED
	var weekly, monthly billingResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		weekly.summary, weekly.status, weekly.err = s.fetchBilling(probeCtx, account, token, proxyURL, true)
REDACTED()
	go func() {
		defer wg.Done()
		monthly.summary, monthly.status, monthly.err = s.fetchBilling(probeCtx, account, token, proxyURL, false)
REDACTED()
	wg.Wait()

	weeklyOK := weekly.summary != nil
	monthlyOK := monthly.summary != nil
	if !weeklyOK && !monthlyOK {
		return nil, mergeGrokBillingProbeErrors(weekly.status, monthly.status, weekly.err, monthly.err)
REDACTED
	statusCode := preferSuccessfulBillingStatus(weekly.status, monthly.status, weeklyOK, monthlyOK)
	previous, _ := grokBillingSnapshotFromExtra(account.Extra)
	billing := xai.MergeBillingProbeResult(previous, weekly.summary, monthly.summary, weeklyOK, monthlyOK)
	billing = xai.StampBillingSummary(billing, statusCode, "billing_probe")
	persistErr := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		grokBillingExtraKey: billing,
REDACTED)
	if persistErr != nil {
		slog.Warn("grok_billing_persist_failed", "account_id", account.ID, "error", persistErr)
REDACTED
	now := time.Now().UTC()
	localUsage24h, localUsage7d, localUsageMonthly := grokLocalUsageForQuota(ctx, s.usageLogRepo, account.ID, billing, now)
	return &GrokQuotaProbeResult{
		Source:            "billing_probe",
		Billing:           billing,
		LocalUsage24h:     localUsage24h,
		LocalUsage7d:      localUsage7d,
		LocalUsageMonthly: localUsageMonthly,
		StatusCode:        statusCode,
		FetchedAt:         now.Unix(),
		Persisted:         persistErr == nil,
REDACTED, nil
REDACTED

func (s *GrokQuotaService) runProbeFlight(
	ctx context.Context,
	key string,
	probe func(context.Context) (*GrokQuotaProbeResult, error),
) (*GrokQuotaProbeResult, error) {
	if s == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
REDACTED
	resultCh := s.probeFlight.DoChan(key, func() (any, error) {
		sharedCtx, cancel := context.WithTimeout(context.Background(), grokQuotaUpstreamTimeout+5*time.Second)
		defer cancel()
		return probe(sharedCtx)
REDACTED)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
	REDACTED
		result, ok := flightResult.Val.(*GrokQuotaProbeResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_PROBE_RESULT_INVALID", "invalid Grok quota probe result")
	REDACTED
		cloned := *result
		return &cloned, nil
REDACTED
REDACTED

func (s *GrokQuotaService) fetchBilling(
	ctx context.Context,
	account *Account,
	token string,
	proxyURL string,
	weekly bool,
) (*xai.BillingSummary, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, xai.BuildBillingURL(weekly), nil)
	if err != nil {
		return nil, 0, infraerrors.Newf(http.StatusInternalServerError, "GROK_QUOTA_PROBE_REQUEST_BUILD_FAILED", "failed to build billing request: %v", err)
REDACTED
	xai.ApplyCLIBillingHeaders(req, token)
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 2))
	if err != nil {
		return nil, 0, infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_PROBE_REQUEST_FAILED", "billing request failed: %v", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, resp.StatusCode, nil
REDACTED
	if resp.StatusCode >= 400 {
		bodyText := truncate(strings.TrimSpace(string(bodyBytes)), 240)
		slog.Warn("grok_quota_billing_failed", "account_id", account.ID, "weekly", weekly, "status", resp.StatusCode, "body", bodyText)
		return nil, resp.StatusCode, infraerrors.Newf(mapUpstreamStatus(resp.StatusCode), "GROK_QUOTA_PROBE_UPSTREAM_ERROR", "billing returned %d: %s", resp.StatusCode, bodyText)
REDACTED
	payload, err := xai.ParseBillingPayload(bodyBytes)
	if err != nil {
		return nil, resp.StatusCode, infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_BILLING_PARSE_ERROR", "failed to parse billing body: %v", err)
REDACTED
	return xai.BuildBillingSummary(payload.Config), resp.StatusCode, nil
REDACTED

func mergeGrokBillingProbeErrors(weeklyStatus, monthlyStatus int, weeklyErr, monthlyErr error) error {
	weeklyKey := grokBillingProbeErrorKey(weeklyStatus, weeklyErr)
	monthlyKey := grokBillingProbeErrorKey(monthlyStatus, monthlyErr)
	if weeklyKey == monthlyKey {
		switch {
		case weeklyErr != nil:
			return weeklyErr
		case monthlyErr != nil:
			return monthlyErr
		case weeklyStatus == http.StatusTooManyRequests:
			return infraerrors.New(http.StatusTooManyRequests, "GROK_QUOTA_PROBE_UPSTREAM_ERROR", "billing rate limited")
		case weeklyStatus != 0 && weeklyStatus != http.StatusOK:
			return infraerrors.New(mapUpstreamStatus(weeklyStatus), "GROK_QUOTA_PROBE_UPSTREAM_ERROR", "xAI billing endpoints returned the same upstream error")
		default:
			return infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_BILLING_EMPTY", "xAI billing endpoints returned no quota data")
	REDACTED
REDACTED
	slog.Warn("grok_quota_probe_parts_failed", "weekly_status", weeklyStatus, "weekly_error", weeklyErr, "monthly_status", monthlyStatus, "monthly_error", monthlyErr)
	return infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_PROBE_PARTS_FAILED", "weekly and monthly billing probes failed differently").WithMetadata(map[string]string{
		"weekly_status": strconv.Itoa(weeklyStatus), "monthly_status": strconv.Itoa(monthlyStatus),
REDACTED)
REDACTED

func grokBillingProbeErrorKey(status int, err error) string {
	if err != nil {
		return strconv.Itoa(status) + ":" + strconv.Itoa(infraerrors.Code(err)) + ":" + infraerrors.Reason(err)
REDACTED
	return strconv.Itoa(status) + ":empty"
REDACTED

func preferSuccessfulBillingStatus(weeklyStatus, monthlyStatus int, weeklyOK, monthlyOK bool) int {
	if weeklyOK && weeklyStatus >= 200 && weeklyStatus < 300 {
		return weeklyStatus
REDACTED
	if monthlyOK && monthlyStatus >= 200 && monthlyStatus < 300 {
		return monthlyStatus
REDACTED
	if weeklyStatus != 0 {
		return weeklyStatus
REDACTED
	return monthlyStatus
REDACTED

func (s *GrokQuotaService) ResetQuota(ctx context.Context, accountID int64) (*GrokQuotaResetResult, error) {
	if _, err := s.loadGrokOAuthAccount(ctx, accountID); err != nil {
		return nil, err
REDACTED
	return nil, infraerrors.New(http.StatusNotImplemented, "GROK_QUOTA_RESET_UNSUPPORTED", "xAI does not expose a Grok subscription quota reset endpoint for OAuth accounts")
REDACTED

func (s *GrokQuotaService) prepareProbe(ctx context.Context, accountID int64) (*Account, string, string, error) {
	if s == nil || s.tokenProvider == nil || s.httpUpstream == nil {
		return nil, "", "", infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
REDACTED
	account, err := s.loadGrokOAuthAccount(ctx, accountID)
	if err != nil {
		return nil, "", "", err
REDACTED

	token, err := s.tokenProvider.GetAccessToken(ctx, account)
	if err != nil {
		return nil, "", "", infraerrors.Newf(http.StatusBadGateway, "GROK_QUOTA_TOKEN_UNAVAILABLE", "failed to acquire access token: %v", err)
REDACTED
	if strings.TrimSpace(token) == "" {
		return nil, "", "", infraerrors.New(http.StatusBadGateway, "GROK_QUOTA_TOKEN_UNAVAILABLE", "access token is empty")
REDACTED

	return account, token, s.resolveProxyURL(ctx, account), nil
REDACTED

func (s *GrokQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
REDACTED
	switch {
	case account.Proxy != nil:
		return account.Proxy.URL()
	case s != nil && s.proxyRepo != nil:
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			return proxy.URL()
	REDACTED
REDACTED
	return ""
REDACTED

func (s *GrokQuotaService) loadGrokOAuthAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "GROK_QUOTA_NOT_CONFIGURED", "grok quota service is not configured")
REDACTED
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "GROK_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
REDACTED
	if account == nil {
		return nil, infraerrors.New(http.StatusNotFound, "GROK_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
REDACTED
	if account.Platform != PlatformGrok {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_QUOTA_INVALID_PLATFORM", "account is not a Grok account")
REDACTED
	if account.Type != AccountTypeOAuth {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_QUOTA_INVALID_TYPE", "account is not an OAuth account")
REDACTED
	return account, nil
REDACTED

func grokQuotaProbeModel() string {
	return grokQuotaDefaultModel
REDACTED

func buildGrokQuotaProbeBody(model string) ([]byte, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = grokQuotaDefaultModel
REDACTED
	return json.Marshal(map[string]any{
		"model":             model,
		"input":             grokQuotaProbeInput,
		"max_output_tokens": 1,
		"store":             false,
REDACTED)
REDACTED

func maxInt(a, b int) int {
	if a > b {
		return a
REDACTED
	return b
REDACTED
