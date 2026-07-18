package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type upstreamBillingProbeAccountRepo struct {
	AccountRepository
	mu          sync.Mutex
	accounts    map[int64]*Account
	updates     map[int64][]map[string]any
	bulkUpdates []AccountBulkUpdate
REDACTED

type staleDueUpstreamBillingProbeAccountRepo struct {
	*upstreamBillingProbeAccountRepo
	due []Account
REDACTED

func (r *staleDueUpstreamBillingProbeAccountRepo) ListDueUpstreamBillingProbeAccounts(_ context.Context, _ time.Time, limit int) ([]Account, error) {
	if limit < len(r.due) {
		return append([]Account(nil), r.due[:limit]...), nil
REDACTED
	return append([]Account(nil), r.due...), nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) Create(_ context.Context, account *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.accounts == nil {
		r.accounts = make(map[int64]*Account)
REDACTED
	if account.ID == 0 {
		account.ID = int64(len(r.accounts) + 1)
REDACTED
	r.accounts[account.ID] = account
	return nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) Update(_ context.Context, account *Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[account.ID] = account
	return nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bulkUpdates = append(r.bulkUpdates, updates)
	return int64(len(ids)), nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return nil, ErrAccountNotFound
REDACTED
	clone := *account
	clone.Credentials = mergeMap(nil, account.Credentials)
	clone.Extra = mergeMap(nil, account.Extra)
	return &clone, nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*Account, 0, len(ids))
	for _, id := range ids {
		if account := r.accounts[id]; account != nil {
			result = append(result, account)
	REDACTED
REDACTED
	return result, nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[id]
	if account == nil {
		return ErrAccountNotFound
REDACTED
	if account.Extra == nil {
		account.Extra = make(map[string]any)
REDACTED
	for key, value := range updates {
		account.Extra[key] = value
REDACTED
	if r.updates == nil {
		r.updates = make(map[int64][]map[string]any)
REDACTED
	r.updates[id] = append(r.updates[id], updates)
	return nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) UpdateUpstreamBillingProbeSnapshot(_ context.Context, expected *Account, snapshot *UpstreamBillingProbeSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	account := r.accounts[expected.ID]
	if account == nil || account.Platform != expected.Platform || account.Type != expected.Type || !reflect.DeepEqual(account.Credentials, expected.Credentials) {
		return ErrUpstreamBillingProbeIdentityChanged
REDACTED
	if account.Extra == nil {
		account.Extra = make(map[string]any)
REDACTED
	account.Extra[UpstreamBillingProbeExtraKey] = snapshot
	return nil
REDACTED

func (r *upstreamBillingProbeAccountRepo) FindByExtraField(_ context.Context, key string, value any) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]Account, 0)
	for _, account := range r.accounts {
		if account.Extra != nil && account.Extra[key] == value {
			result = append(result, *account)
	REDACTED
REDACTED
	return result, nil
REDACTED

type upstreamBillingProbeSettingRepo struct {
	SettingRepository
	mu     sync.Mutex
	values map[string]string
REDACTED

type upstreamBillingProbeHTTPStub struct {
	calls          atomic.Int64
	active         atomic.Int64
	maxActive      atomic.Int64
	beforeResponse func()
REDACTED

func (u *upstreamBillingProbeHTTPStub) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.calls.Add(1)
	active := u.active.Add(1)
	defer u.active.Add(-1)
	for {
		peak := u.maxActive.Load()
		if active <= peak || u.maxActive.CompareAndSwap(peak, active) {
			break
	REDACTED
REDACTED
	if u.beforeResponse != nil {
		u.beforeResponse()
REDACTED
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(`{
			"object":"sub2api.key_billing",
			"schema_version":1,
			"billing_scope":"token",
			"group_rate_multiplier":0.8,
			"resolved_rate_multiplier":0.8,
			"peak_rate_enabled":false,
			"effective_rate_multiplier":0.8,
			"observed_at":"2026-07-13T01:00:00Z"
	REDACTED`)),
REDACTED, nil
REDACTED

func (u *upstreamBillingProbeHTTPStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
REDACTED

func (r *upstreamBillingProbeSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
REDACTED
	return value, nil
REDACTED

func (r *upstreamBillingProbeSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
REDACTED
	r.values[key] = value
	return nil
REDACTED

func newUpstreamBillingProbeTestService(
	repo AccountRepository,
	upstream HTTPUpstream,
	settingRepo SettingRepository,
) *UpstreamBillingProbeService {
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
		Enabled:           false,
		AllowInsecureHTTP: true,
REDACTEDREDACTEDREDACTED
	accountTestService := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: cfgREDACTED
	return NewUpstreamBillingProbeService(repo, accountTestService, NewSettingService(settingRepo, cfg))
REDACTED

func TestUpstreamBillingProbeSettingsDefaultsAndValidation(t *testing.T) {
	repo := &upstreamBillingProbeSettingRepo{REDACTED
	settingsService := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := settingsService.GetUpstreamBillingProbeSettings(context.Background())
REDACTED
	require.True(t, settings.Enabled)
	require.Equal(t, 30, settings.IntervalMinutes)

	err = settingsService.SetUpstreamBillingProbeSettings(context.Background(), &UpstreamBillingProbeSettings{
		Enabled:         false,
		IntervalMinutes: 4,
REDACTED)
REDACTED
	require.Contains(t, err.Error(), "interval_minutes must be between 5 and 1440")

	err = settingsService.SetUpstreamBillingProbeSettings(context.Background(), &UpstreamBillingProbeSettings{
		Enabled:         false,
		IntervalMinutes: 60,
REDACTED)
REDACTED
	settings, err = settingsService.GetUpstreamBillingProbeSettings(context.Background())
REDACTED
	require.False(t, settings.Enabled)
	require.Equal(t, 60, settings.IntervalMinutes)

	repo.values[SettingKeyUpstreamBillingProbeSettings] = `{"interval_minutes":45REDACTED`
	settings, err = settingsService.GetUpstreamBillingProbeSettings(context.Background())
REDACTED
	require.True(t, settings.Enabled)
	require.Equal(t, 45, settings.IntervalMinutes)
	repo.values[SettingKeyUpstreamBillingProbeSettings] = `{"enabled":falseREDACTED`
	settings, err = settingsService.GetUpstreamBillingProbeSettings(context.Background())
REDACTED
	require.False(t, settings.Enabled)
	require.Equal(t, 30, settings.IntervalMinutes)

	repo.values[SettingKeyUpstreamBillingProbeSettings] = `{"enabled":`
	settings, err = settingsService.GetUpstreamBillingProbeSettings(context.Background())
	require.ErrorContains(t, err, "parse upstream billing probe settings")
	require.Nil(t, settings)
REDACTED

func TestUpstreamBillingProbeSuccessPersistsSanitizedSnapshot(t *testing.T) {
	account := &Account{
		ID:          17,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 2,
REDACTED
			"api_key":  "sk-sensitive",
			"base_url": "https://upstream.example/v1",
	REDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
		Body: io.NopCloser(strings.NewReader(`{
			"object":"sub2api.key_billing",
			"schema_version":1,
			"billing_scope":"token",
			"group_rate_multiplier":0.8,
			"user_rate_multiplier":0.6,
			"resolved_rate_multiplier":0.6,
			"peak_rate_enabled":true,
			"peak_start":"09:00",
			"peak_end":"18:00",
			"peak_rate_multiplier":1.5,
			"applied_peak_multiplier":1.5,
			"effective_rate_multiplier":0.9,
			"timezone":"Asia/Shanghai",
			"observed_at":"2026-07-13T01:00:00Z",
			"unexpected_secret":"must-not-persist"
	REDACTED`)),
REDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)
	fixedNow := time.Date(2026, time.July, 13, 2, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow REDACTED

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
REDACTED
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.Equal(t, 0.9, snapshot.Data["effective_rate_multiplier"])
	require.NotContains(t, snapshot.Data, "unexpected_secret")
	require.NotNil(t, snapshot.ReceivedAt)
	require.Equal(t, fixedNow, *snapshot.ReceivedAt)
	require.NotNil(t, snapshot.FreshUntil)
	require.Equal(t, fixedNow.Add(time.Hour), *snapshot.FreshUntil)
	require.False(t, snapshot.NextProbeAt.Before(fixedNow.Add(24*time.Minute)))
	require.False(t, snapshot.NextProbeAt.After(fixedNow.Add(36*time.Minute)))
	require.Equal(t, "https://upstream.example/v1/sub2api/billing", upstream.lastReq.URL.String())
	require.Equal(t, http.MethodGet, upstream.lastReq.Method)
	require.Equal(t, "Bearer sk-sensitive", upstream.lastReq.Header.Get("Authorization"))
	require.True(t, HTTPUpstreamRedirectsDisabled(upstream.lastReq.Context()))

	persisted := decodeUpstreamBillingProbeSnapshot(account.Extra)
	require.NotNil(t, persisted)
	require.Equal(t, snapshot.Status, persisted.Status)
REDACTED

func TestUpstreamBillingProbeRejectsMissingRequiredMultiplier(t *testing.T) {
	_, err := parseUpstreamBillingProbeResponse([]byte(`{
		"object":"sub2api.key_billing",
		"schema_version":1,
		"billing_scope":"token",
		"group_rate_multiplier":0.8,
		"peak_rate_enabled":false,
		"effective_rate_multiplier":0.8,
		"observed_at":"2026-07-13T01:00:00Z"
REDACTED`))

	require.ErrorContains(t, err, "incomplete billing response")
REDACTED

func TestUpstreamBillingProbeDiscardsResultWhenIdentityChangesInFlight(t *testing.T) {
	account := &Account{
		ID:          19,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-old", "base_url": "https://upstream.example"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstream := &upstreamBillingProbeHTTPStub{beforeResponse: func() {
		repo.mu.Lock()
		defer repo.mu.Unlock()
		repo.accounts[account.ID].Credentials = map[string]any{"api_key": "sk-new", "base_url": "https://new.example"REDACTED
REDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)

	require.Nil(t, snapshot)
	require.ErrorIs(t, err, ErrUpstreamBillingProbeIdentityChanged)
	require.NotContains(t, repo.accounts[account.ID].Extra, UpstreamBillingProbeExtraKey)
REDACTED

func TestUpstreamBillingProbeRejectsInvalidPeakConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		end      string
		timezone string
REDACTED{
		{name: "invalid start", start: "25:00", end: "18:00", timezone: "UTC"REDACTED,
		{name: "cross midnight", start: "22:00", end: "02:00", timezone: "UTC"REDACTED,
		{name: "invalid timezone", start: "09:00", end: "18:00", timezone: "Mars/Olympus"REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{
				"object":"sub2api.key_billing",
				"schema_version":1,
				"billing_scope":"token",
				"group_rate_multiplier":0.8,
				"resolved_rate_multiplier":0.8,
				"peak_rate_enabled":true,
				"peak_start":%q,
				"peak_end":%q,
				"peak_rate_multiplier":1.5,
				"applied_peak_multiplier":1,
				"effective_rate_multiplier":0.8,
				"timezone":%q,
				"observed_at":"2026-07-13T01:00:00Z"
		REDACTED`, tt.start, tt.end, tt.timezone)

			_, err := parseUpstreamBillingProbeResponse([]byte(body))
			require.ErrorContains(t, err, "invalid peak billing response")
	REDACTED)
REDACTED
REDACTED

func TestUpstreamBillingProbeRejectsInconsistentMultipliers(t *testing.T) {
	tests := []struct {
		name string
		body string
REDACTED{
		{
			name: "resolved does not use user override",
			body: `{
				"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token",
				"group_rate_multiplier":0.8,"user_rate_multiplier":0.5,"resolved_rate_multiplier":0.8,
				"peak_rate_enabled":false,"effective_rate_multiplier":0.8,"observed_at":"2026-07-13T01:00:00Z"
		REDACTED`,
	REDACTED,
		{
			name: "effective rate does not match resolved rate",
			body: `{
				"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token",
				"group_rate_multiplier":0.8,"resolved_rate_multiplier":0.8,
				"peak_rate_enabled":false,"effective_rate_multiplier":1.2,"observed_at":"2026-07-13T01:00:00Z"
		REDACTED`,
	REDACTED,
		{
			name: "applied peak does not match observed window",
			body: `{
				"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token",
				"group_rate_multiplier":0.8,"resolved_rate_multiplier":0.8,
				"peak_rate_enabled":true,"peak_start":"09:00","peak_end":"18:00",
				"peak_rate_multiplier":1.5,"applied_peak_multiplier":1,
				"effective_rate_multiplier":0.8,"timezone":"Asia/Shanghai","observed_at":"2026-07-13T01:00:00Z"
		REDACTED`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseUpstreamBillingProbeResponse([]byte(tt.body))
			require.ErrorContains(t, err, "inconsistent")
	REDACTED)
REDACTED
REDACTED

func TestUpstreamBillingRateAtHandlesDST(t *testing.T) {
	data := map[string]any{
		"billing_scope":            "token",
		"resolved_rate_multiplier": 1.0,
		"peak_rate_enabled":        true,
		"peak_start":               "02:00",
		"peak_end":                 "04:00",
		"peak_rate_multiplier":     2.0,
		"timezone":                 "America/New_York",
REDACTED
	beforeJump := time.Date(2026, time.March, 8, 6, 30, 0, 0, time.UTC)
	afterJump := time.Date(2026, time.March, 8, 7, 30, 0, 0, time.UTC)

	rate, ok := upstreamBillingRateAt(data, beforeJump)
	require.True(t, ok)
	require.Equal(t, 1.0, rate)
	rate, ok = upstreamBillingRateAt(data, afterJump)
	require.True(t, ok)
	require.Equal(t, 2.0, rate)
REDACTED

func TestUpstreamBillingProbeFailurePreservesLastSuccessAndRetryAfter(t *testing.T) {
	receivedAt := time.Date(2026, time.July, 12, 12, 0, 0, 0, time.UTC)
	previous := &UpstreamBillingProbeSnapshot{
		Status:       UpstreamBillingProbeStatusOK,
		Data:         map[string]any{"effective_rate_multiplier": 0.5REDACTED,
		ReceivedAt:   &receivedAt,
		FailureCount: 1,
REDACTED
	account := &Account{
		ID:          18,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeExtraKey: previousREDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"14400"REDACTEDREDACTED,
		Body:       io.NopCloser(strings.NewReader(`{"error":"do not persist this"REDACTED`)),
REDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)
	fixedNow := time.Date(2026, time.July, 13, 2, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow REDACTED

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
REDACTED
	require.Equal(t, UpstreamBillingProbeStatusFailed, snapshot.Status)
	require.Equal(t, previous.Data, snapshot.Data)
	require.Equal(t, previous.ReceivedAt, snapshot.ReceivedAt)
	require.NotNil(t, snapshot.FreshUntil)
	require.Equal(t, receivedAt.Add(time.Hour), *snapshot.FreshUntil)
	require.Equal(t, 2, snapshot.FailureCount)
	require.Equal(t, "http_error", snapshot.LastError)
	require.False(t, snapshot.NextProbeAt.Before(fixedNow.Add(4*time.Hour)))
	require.NotContains(t, snapshot.LastError, "do not persist")
REDACTED

func TestUpstreamBillingProbeRetryAfterIsNotShortened(t *testing.T) {
	delay := nextProbeDelay(30, 48*time.Hour)
	require.Equal(t, 48*time.Hour, delay)
REDACTED

func TestUpstreamBillingProbeEmptyResponseIsPersistedAsFailure(t *testing.T) {
	account := &Account{
		ID:          21,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, &httpUpstreamRecorder{REDACTED, &upstreamBillingProbeSettingRepo{REDACTED)
	fixedNow := time.Date(2026, time.July, 13, 2, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow REDACTED

	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
REDACTED
	require.Equal(t, UpstreamBillingProbeStatusFailed, snapshot.Status)
	require.Equal(t, "empty_response", snapshot.LastError)
	require.False(t, snapshot.NextProbeAt.Before(fixedNow.Add(24*time.Minute)))
	require.False(t, snapshot.NextProbeAt.After(fixedNow.Add(36*time.Minute)))

	snapshot, err = svc.ProbeAccount(context.Background(), account.ID)
REDACTED
	require.Equal(t, 2, snapshot.FailureCount)
	require.False(t, snapshot.NextProbeAt.Before(fixedNow.Add(24*time.Minute)))
	require.False(t, snapshot.NextProbeAt.After(fixedNow.Add(36*time.Minute)))
REDACTED

func TestUpstreamBillingProbeUnsupportedAndAccountToggle(t *testing.T) {
	account := &Account{
		ID:          19,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{REDACTED,
		Body:       io.NopCloser(strings.NewReader("not found")),
REDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)
	fixedNow := time.Date(2026, time.July, 13, 2, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow REDACTED

	require.NoError(t, svc.SetAccountEnabled(context.Background(), account.ID, true))
	require.Equal(t, true, account.Extra[UpstreamBillingProbeEnabledExtraKey])
	snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
REDACTED
	require.Equal(t, UpstreamBillingProbeStatusUnsupported, snapshot.Status)
	require.Equal(t, "unsupported", snapshot.LastError)
	require.False(t, snapshot.NextProbeAt.Before(fixedNow.Add(24*time.Minute)))
	require.False(t, snapshot.NextProbeAt.After(fixedNow.Add(36*time.Minute)))

	snapshot, err = svc.ProbeAccount(context.Background(), account.ID)
REDACTED
	require.Equal(t, 2, snapshot.FailureCount)
	require.False(t, snapshot.NextProbeAt.Before(fixedNow.Add(24*time.Minute)))
	require.False(t, snapshot.NextProbeAt.After(fixedNow.Add(36*time.Minute)))

	invalid := &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED
	repo.accounts[invalid.ID] = invalid
	err = svc.SetAccountEnabled(context.Background(), invalid.ID, true)
	require.True(t, errors.Is(err, ErrUpstreamBillingProbeAccountInvalid))
REDACTED

func TestUpstreamBillingProbeRunnerIsBoundedAndManualProbeIgnoresSwitches(t *testing.T) {
	accounts := make(map[int64]*Account, 25)
	for id := int64(1); id <= 25; id++ {
		accounts[id] = &Account{
			ID:          id,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Concurrency: 1,
	REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
			Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
	REDACTED
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: accountsREDACTED
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyUpstreamBillingProbeSettings: `{"enabled":true,"interval_minutes":30REDACTED`,
REDACTEDREDACTED
	upstream := &upstreamBillingProbeHTTPStub{REDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, settingsRepo)
	svc.now = func() time.Time { return time.Date(2026, time.July, 13, 2, 0, 0, 0, time.UTC) REDACTED

	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(20), upstream.calls.Load())

	settingsRepo.mu.Lock()
	settingsRepo.values[SettingKeyUpstreamBillingProbeSettings] = `{"enabled":false,"interval_minutes":30REDACTED`
	settingsRepo.mu.Unlock()
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(20), upstream.calls.Load())

	accounts[25].Extra[UpstreamBillingProbeEnabledExtraKey] = false
	snapshot, err := svc.ProbeAccount(context.Background(), 25)
REDACTED
	require.Equal(t, UpstreamBillingProbeStatusOK, snapshot.Status)
	require.Equal(t, int64(21), upstream.calls.Load())
REDACTED

func TestUpstreamBillingProbeRunnerRechecksEnabledAfterDueSelection(t *testing.T) {
	account := &Account{
		ID:          26,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: falseREDACTED,
REDACTED
	staleDue := *account
	staleDue.Extra = map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED
	baseRepo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	repo := &staleDueUpstreamBillingProbeAccountRepo{upstreamBillingProbeAccountRepo: baseRepo, due: []Account{staleDueREDACTEDREDACTED
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyUpstreamBillingProbeSettings: `{"enabled":true,"interval_minutes":30REDACTED`,
REDACTEDREDACTED
	upstream := &upstreamBillingProbeHTTPStub{REDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, settingsRepo)

	require.NoError(t, svc.RunDue(context.Background()))
	require.Zero(t, upstream.calls.Load())
	require.NotContains(t, account.Extra, UpstreamBillingProbeExtraKey)
REDACTED

func TestUpstreamBillingProbeNeverDowngradesMissingConfiguredProxyToDirect(t *testing.T) {
	proxyID := int64(7)
	for _, tc := range []struct {
		name       string
		proxy      *Proxy
		wantReason string
		wantErr    error
REDACTED{
		{name: "missing hydrated proxy", wantReason: "proxy_unavailable"REDACTED,
		{name: "mismatched hydrated proxy", proxy: &Proxy{ID: 8, Protocol: "http", Host: "127.0.0.1", Port: 8080REDACTED, wantErr: ErrUpstreamBillingProbeIdentityChangedREDACTED,
REDACTED {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{
				ID:          27,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Status:      StatusActive,
				Concurrency: 1,
		REDACTED"api_key": "sk-sensitive", "base_url": "https://upstream.example"REDACTED,
				ProxyID:     &proxyID,
				Proxy:       tc.proxy,
		REDACTED
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
			upstream := &upstreamBillingProbeHTTPStub{REDACTED
			svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)

			snapshot, err := svc.ProbeAccount(context.Background(), account.ID)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, snapshot)
		REDACTED else {
			REDACTED
				require.Equal(t, UpstreamBillingProbeStatusFailed, snapshot.Status)
				require.Equal(t, tc.wantReason, snapshot.LastError)
		REDACTED
			require.Zero(t, upstream.calls.Load())
			if tc.wantErr != nil {
				require.NotContains(t, account.Extra, UpstreamBillingProbeExtraKey)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestUpstreamBillingProbeRunnerOnlyScansOnLeader(t *testing.T) {
	account := &Account{
		ID:          31,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstream := &upstreamBillingProbeHTTPStub{REDACTED
	cache := &fakeLeaderLockCache{REDACTED
	lockKey := upstreamBillingProbeLeaderLockKeyAt(time.Now())
	peer := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)
	peer.instanceID = "peer"
	peer.SetLeaderLock(cache, nil)
	_, acquired, err := peer.tryAcquireLeaderLock(context.Background(), lockKey)
REDACTED
	require.True(t, acquired)
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)
	svc.SetLeaderLock(cache, nil)

	require.NoError(t, svc.RunDue(context.Background()))
	require.Zero(t, upstream.calls.Load())

	require.NoError(t, cache.ReleaseLeaderLock(context.Background(), lockKey, "peer"))
	require.NoError(t, svc.RunDue(context.Background()))
	require.Equal(t, int64(1), upstream.calls.Load())
REDACTED

func TestUpstreamBillingProbeLeaderLockFailsClosedOnCacheError(t *testing.T) {
	svc := newUpstreamBillingProbeTestService(&upstreamBillingProbeAccountRepo{REDACTED, &upstreamBillingProbeHTTPStub{REDACTED, &upstreamBillingProbeSettingRepo{REDACTED)
	svc.SetLeaderLock(&fakeLeaderLockCache{acquireErr: context.DeadlineExceededREDACTED, nil)

	release, acquired, err := svc.tryAcquireLeaderLock(context.Background(), upstreamBillingProbeLeaderLockKeyAt(time.Now()))

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, acquired)
	require.Nil(t, release)
REDACTED

func TestUpstreamBillingProbeLeaderLockUsesCadenceBuckets(t *testing.T) {
	cache := &fakeLeaderLockCache{REDACTED
	first := newUpstreamBillingProbeTestService(&upstreamBillingProbeAccountRepo{REDACTED, &upstreamBillingProbeHTTPStub{REDACTED, &upstreamBillingProbeSettingRepo{REDACTED)
	second := newUpstreamBillingProbeTestService(&upstreamBillingProbeAccountRepo{REDACTED, &upstreamBillingProbeHTTPStub{REDACTED, &upstreamBillingProbeSettingRepo{REDACTED)
	first.SetLeaderLock(cache, nil)
	second.SetLeaderLock(cache, nil)
	beforeBoundary := time.Unix(59, 0)
	afterBoundary := beforeBoundary.Add(time.Second)

	releaseFirst, acquired, err := first.tryAcquireLeaderLock(context.Background(), upstreamBillingProbeLeaderLockKeyAt(beforeBoundary))
REDACTED
	require.True(t, acquired)
	releaseSecond, acquired, err := second.tryAcquireLeaderLock(context.Background(), upstreamBillingProbeLeaderLockKeyAt(afterBoundary))
REDACTED
	require.True(t, acquired, "the prior cadence lock must not suppress the next cadence")
	releaseFirst()
	releaseSecond()
REDACTED

func TestUpstreamBillingProbeFiveInstancesRunOneConcurrentBatch(t *testing.T) {
	account := &Account{
		ID:          32,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "http://127.0.0.1:8080"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyUpstreamBillingProbeSettings: `{"enabled":true,"interval_minutes":30REDACTED`,
REDACTEDREDACTED
	cache := &fakeLeaderLockCache{REDACTED
	entered := make(chan struct{REDACTED)
	unblock := make(chan struct{REDACTED)
	var enteredOnce sync.Once
	upstream := &upstreamBillingProbeHTTPStub{beforeResponse: func() {
		enteredOnce.Do(func() { close(entered) REDACTED)
		<-unblock
REDACTEDREDACTED

	start := make(chan struct{REDACTED)
	results := make(chan error, 5)
	for range 5 {
		svc := newUpstreamBillingProbeTestService(repo, upstream, settingsRepo)
		svc.SetLeaderLock(cache, nil)
		go func() {
			<-start
			results <- svc.RunDue(context.Background())
	REDACTED()
REDACTED
	close(start)

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("leader did not start the probe batch")
REDACTED
	for range 4 {
		select {
		case err := <-results:
		REDACTED
		case <-time.After(time.Second):
			t.Fatal("non-leader instance did not skip the active batch")
	REDACTED
REDACTED
	require.Equal(t, int64(1), upstream.calls.Load())
	close(unblock)
	require.NoError(t, <-results)
	require.Equal(t, int64(1), upstream.calls.Load())
REDACTED

func TestUpstreamBillingProbeManualBatchesShareConcurrencyLimit(t *testing.T) {
	accounts := make(map[int64]*Account, 12)
	for id := int64(1); id <= 12; id++ {
		accounts[id] = &Account{
			ID:          id,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Concurrency: 1,
	REDACTED"api_key": "sk-test", "base_url": "http://127.0.0.1:8080"REDACTED,
	REDACTED
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: accountsREDACTED
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyUpstreamBillingProbeSettings: `{"enabled":true,"interval_minutes":30REDACTED`,
REDACTEDREDACTED
	entered := make(chan struct{REDACTED, len(accounts))
	unblock := make(chan struct{REDACTED)
	var unblockOnce sync.Once
	release := func() { unblockOnce.Do(func() { close(unblock) REDACTED) REDACTED
	t.Cleanup(release)
	upstream := &upstreamBillingProbeHTTPStub{beforeResponse: func() {
		entered <- struct{REDACTED{REDACTED
		<-unblock
REDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, settingsRepo)

	results := make(chan []UpstreamBillingProbeResult, 3)
	for batch := 0; batch < 3; batch++ {
		firstID := int64(batch*4 + 1)
		ids := []int64{firstID, firstID + 1, firstID + 2, firstID + 3REDACTED
		go func() { results <- svc.ProbeAccounts(context.Background(), ids) REDACTED()
REDACTED
	for range upstreamBillingProbeConcurrency {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("shared probe slots did not fill")
	REDACTED
REDACTED
	select {
	case <-entered:
		release()
		t.Fatal("parallel manual batches exceeded the service-wide concurrency limit")
	case <-time.After(100 * time.Millisecond):
REDACTED
	release()

	for range 3 {
		select {
		case batchResults := <-results:
			for _, result := range batchResults {
				require.Empty(t, result.Error)
				require.NotNil(t, result.Snapshot)
		REDACTED
		case <-time.After(time.Second):
			t.Fatal("manual probe batch did not finish")
	REDACTED
REDACTED
	require.Equal(t, int64(upstreamBillingProbeConcurrency), upstream.maxActive.Load())
REDACTED

func TestUpstreamBillingProbeManualAndScheduledRequestsShareOneNetworkProbe(t *testing.T) {
	account := &Account{
		ID:          46,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	started := make(chan struct{REDACTED)
	unblock := make(chan struct{REDACTED)
	var startedOnce sync.Once
	upstream := &upstreamBillingProbeHTTPStub{beforeResponse: func() {
		startedOnce.Do(func() { close(started) REDACTED)
		<-unblock
REDACTEDREDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)

	errs := make(chan error, 2)
	go func() {
		_, err := svc.probeScheduledAccount(context.Background(), account.ID, 30)
		errs <- err
REDACTED()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scheduled probe did not reach the upstream")
REDACTED
	manualStarted := make(chan struct{REDACTED)
	go func() {
		close(manualStarted)
		_, err := svc.ProbeAccount(context.Background(), account.ID)
		errs <- err
REDACTED()
	<-manualStarted
	time.Sleep(20 * time.Millisecond)
	close(unblock)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Equal(t, int64(1), upstream.calls.Load())
REDACTED

func TestUpstreamBillingProbeScheduledRechecksAfterWaitingForSlot(t *testing.T) {
	account := &Account{
		ID:          47,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Concurrency: 1,
REDACTED"api_key": "sk-test", "base_url": "https://upstream.example"REDACTED,
		Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: accountREDACTEDREDACTED
	upstream := &upstreamBillingProbeHTTPStub{REDACTED
	svc := newUpstreamBillingProbeTestService(repo, upstream, &upstreamBillingProbeSettingRepo{REDACTED)
	for range upstreamBillingProbeConcurrency {
		svc.probeSlots <- struct{REDACTED{REDACTED
REDACTED
	result := make(chan error, 1)
	go func() {
		_, err := svc.probeScheduledAccount(context.Background(), account.ID, 30)
		result <- err
REDACTED()
	time.Sleep(20 * time.Millisecond)
	repo.mu.Lock()
	account.Extra[UpstreamBillingProbeEnabledExtraKey] = false
	repo.mu.Unlock()
	<-svc.probeSlots

	require.NoError(t, <-result)
	require.Zero(t, upstream.calls.Load())
REDACTED

func TestUpstreamBillingProbeLeaderLockCoversStaggeredInstancesInCadenceWindow(t *testing.T) {
	account := func(id int64) *Account {
	REDACTED
			ID:          id,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Concurrency: 1,
	REDACTED"api_key": "sk-test", "base_url": "http://127.0.0.1:8080"REDACTED,
			Extra:       map[string]any{UpstreamBillingProbeEnabledExtraKey: trueREDACTED,
	REDACTED
REDACTED
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{41: account(41)REDACTEDREDACTED
	settingsRepo := &upstreamBillingProbeSettingRepo{values: map[string]string{
		SettingKeyUpstreamBillingProbeSettings: `{"enabled":true,"interval_minutes":30REDACTED`,
REDACTEDREDACTED
	cache := &fakeLeaderLockCache{REDACTED
	upstream := &upstreamBillingProbeHTTPStub{REDACTED
	first := newUpstreamBillingProbeTestService(repo, upstream, settingsRepo)
	first.SetLeaderLock(cache, nil)

	require.NoError(t, first.RunDue(context.Background()))
	require.Equal(t, int64(1), upstream.calls.Load())
	require.Equal(t, first.instanceID, cache.heldBy(upstreamBillingProbeLeaderLockKeyAt(time.Now())))

	repo.mu.Lock()
	repo.accounts[42] = account(42)
	repo.mu.Unlock()
	staggered := newUpstreamBillingProbeTestService(repo, upstream, settingsRepo)
	staggered.SetLeaderLock(cache, nil)
	require.NoError(t, staggered.RunDue(context.Background()))
	require.Equal(t, int64(1), upstream.calls.Load(), "a staggered instance must not start a second batch inside the cadence window")
REDACTED
