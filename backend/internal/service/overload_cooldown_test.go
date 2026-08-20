//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// errSettingRepo: a SettingRepository that always returns errors on read
// ---------------------------------------------------------------------------

type errSettingRepo struct {
	mockSettingRepo // embed the existing mock from backup_service_test.go
	readErr         error
REDACTED

func (r *errSettingRepo) GetValue(_ context.Context, _ string) (string, error) {
	return "", r.readErr
REDACTED

func (r *errSettingRepo) Get(_ context.Context, _ string) (*Setting, error) {
	return nil, r.readErr
REDACTED

// ---------------------------------------------------------------------------
// overloadAccountRepoStub: records SetOverloaded calls
// ---------------------------------------------------------------------------

type overloadAccountRepoStub struct {
	mockAccountRepoForGemini
	overloadCalls   int
	lastOverloadID  int64
	lastOverloadEnd time.Time
REDACTED

func (r *overloadAccountRepoStub) SetOverloaded(_ context.Context, id int64, until time.Time) error {
	r.overloadCalls++
	r.lastOverloadID = id
	r.lastOverloadEnd = until
	return nil
REDACTED

// ===========================================================================
// SettingService: GetOverloadCooldownSettings
// ===========================================================================

func TestGetOverloadCooldownSettings_DefaultsWhenNotSet(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
REDACTED

func TestGetOverloadCooldownSettings_ReadsFromDB(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 30REDACTED)
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.False(t, settings.Enabled)
	require.Equal(t, 30, settings.CooldownMinutes)
REDACTED

func TestGetOverloadCooldownSettings_ClampsMinValue(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 0REDACTED)
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.Equal(t, 1, settings.CooldownMinutes)
REDACTED

func TestGetOverloadCooldownSettings_ClampsMaxValue(t *testing.T) {
	repo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 999REDACTED)
	repo.data[SettingKeyOverloadCooldownSettings] = string(data)
	svc := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.Equal(t, 120, settings.CooldownMinutes)
REDACTED

func TestGetOverloadCooldownSettings_InvalidJSON_ReturnsDefaults(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOverloadCooldownSettings] = "not-json"
	svc := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
REDACTED

func TestGetOverloadCooldownSettings_EmptyValue_ReturnsDefaults(t *testing.T) {
	repo := newMockSettingRepo()
	repo.data[SettingKeyOverloadCooldownSettings] = ""
	svc := NewSettingService(repo, &config.Config{REDACTED)

	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.True(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes)
REDACTED

// ===========================================================================
// SettingService: SetOverloadCooldownSettings
// ===========================================================================

func TestSetOverloadCooldownSettings_Success(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{REDACTED)

	err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
		Enabled:         false,
		CooldownMinutes: 25,
REDACTED)
REDACTED

	// Verify round-trip
	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.False(t, settings.Enabled)
	require.Equal(t, 25, settings.CooldownMinutes)
REDACTED

func TestSetOverloadCooldownSettings_RejectsNil(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{REDACTED)
	err := svc.SetOverloadCooldownSettings(context.Background(), nil)
REDACTED
REDACTED

func TestSetOverloadCooldownSettings_EnabledRejectsOutOfRange(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{REDACTED)

	for _, minutes := range []int{0, -1, 121, 999REDACTED {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: minutes,
	REDACTED)
		require.Error(t, err, "should reject enabled=true + cooldown_minutes=%d", minutes)
		require.Contains(t, err.Error(), "cooldown_minutes must be between 1-120")
REDACTED
REDACTED

func TestSetOverloadCooldownSettings_DisabledNormalizesOutOfRange(t *testing.T) {
	repo := newMockSettingRepo()
	svc := NewSettingService(repo, &config.Config{REDACTED)

	// enabled=false + cooldown_minutes=0 应该保存成功，值被归一化为10
	err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
		Enabled: false, CooldownMinutes: 0,
REDACTED)
	require.NoError(t, err, "disabled with invalid minutes should NOT be rejected")

	// 验证持久化后读回来的值
	settings, err := svc.GetOverloadCooldownSettings(context.Background())
REDACTED
	require.False(t, settings.Enabled)
	require.Equal(t, 10, settings.CooldownMinutes, "should be normalized to default")
REDACTED

func TestSetOverloadCooldownSettings_AcceptsBoundaries(t *testing.T) {
	svc := NewSettingService(newMockSettingRepo(), &config.Config{REDACTED)

	for _, minutes := range []int{1, 60, 120REDACTED {
		err := svc.SetOverloadCooldownSettings(context.Background(), &OverloadCooldownSettings{
			Enabled: true, CooldownMinutes: minutes,
	REDACTED)
		require.NoError(t, err, "should accept cooldown_minutes=%d", minutes)
REDACTED
REDACTED

// ===========================================================================
// RateLimitService: handle529 behaviour
// ===========================================================================

func TestHandle529_EnabledFromDB_PausesAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{REDACTED
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: true, CooldownMinutes: 15REDACTED)
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{REDACTED)
	svc := NewRateLimitService(accountRepo, nil, &config.Config{REDACTED, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.Equal(t, int64(42), accountRepo.lastOverloadID)
	require.WithinDuration(t, before.Add(15*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
REDACTED

func TestHandle529_DisabledFromDB_SkipsAccount(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{REDACTED
	settingRepo := newMockSettingRepo()
	data, _ := json.Marshal(OverloadCooldownSettings{Enabled: false, CooldownMinutes: 15REDACTED)
	settingRepo.data[SettingKeyOverloadCooldownSettings] = string(data)

	settingSvc := NewSettingService(settingRepo, &config.Config{REDACTED)
	svc := NewRateLimitService(accountRepo, nil, &config.Config{REDACTED, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 42, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED
	svc.handle529(context.Background(), account)

	require.Equal(t, 0, accountRepo.overloadCalls, "should NOT pause when disabled")
REDACTED

func TestHandle529_NilSettingService_FallsBackToConfig(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{REDACTED
	cfg := &config.Config{REDACTED
	cfg.RateLimit.OverloadCooldownMinutes = 20
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	// NOT calling SetSettingService — remains nil

	account := &Account{ID: 77, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(20*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
REDACTED

func TestHandle529_NilSettingService_ZeroConfig_DefaultsTen(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{REDACTED
	svc := NewRateLimitService(accountRepo, nil, &config.Config{REDACTED, nil, nil)

	account := &Account{ID: 88, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(10*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
REDACTED

func TestHandle529_DBReadError_FallsBackToConfig(t *testing.T) {
	accountRepo := &overloadAccountRepoStub{REDACTED
	errRepo := &errSettingRepo{readErr: context.DeadlineExceededREDACTED
	errRepo.data = make(map[string]string)

	cfg := &config.Config{REDACTED
	cfg.RateLimit.OverloadCooldownMinutes = 7
	settingSvc := NewSettingService(errRepo, cfg)
	svc := NewRateLimitService(accountRepo, nil, cfg, nil, nil)
	svc.SetSettingService(settingSvc)

	account := &Account{ID: 99, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED
	before := time.Now()
	svc.handle529(context.Background(), account)

	require.Equal(t, 1, accountRepo.overloadCalls)
	require.WithinDuration(t, before.Add(7*time.Minute), accountRepo.lastOverloadEnd, 2*time.Second)
REDACTED

func TestHandleUpstreamError_529BypassesPoolAndCustomCodeGates(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
REDACTED{
		{
			name:        "pool mode",
			credentials: map[string]any{"pool_mode": trueREDACTED,
	REDACTED,
		{
			name: "custom code filter excludes 529",
			credentials: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(429)REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &overloadAccountRepoStub{REDACTED
			svc := NewRateLimitService(repo, nil, &config.Config{REDACTED, nil, nil)
			account := &Account{
				ID:          101,
				Platform:    PlatformOpenAI,
				Type:        AccountTypeAPIKey,
				Credentials: tt.credentials,
		REDACTED

			shouldDisable := svc.HandleUpstreamError(context.Background(), account, 529, nil, []byte(`{"error":{"message":"overloaded"REDACTEDREDACTED`))

			require.False(t, shouldDisable)
			require.Equal(t, 1, repo.overloadCalls)
			require.Equal(t, account.ID, repo.lastOverloadID)
	REDACTED)
REDACTED
REDACTED

// ===========================================================================
// Model: defaults & JSON round-trip
// ===========================================================================

func TestDefaultOverloadCooldownSettings(t *testing.T) {
	d := DefaultOverloadCooldownSettings()
	require.True(t, d.Enabled)
	require.Equal(t, 10, d.CooldownMinutes)
REDACTED

func TestOverloadCooldownSettings_JSONRoundTrip(t *testing.T) {
	original := OverloadCooldownSettings{Enabled: false, CooldownMinutes: 42REDACTED
	data, err := json.Marshal(original)
REDACTED

	var decoded OverloadCooldownSettings
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original, decoded)

	// Verify JSON uses snake_case field names
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	_, hasEnabled := raw["enabled"]
	_, hasCooldown := raw["cooldown_minutes"]
	require.True(t, hasEnabled, "JSON must use 'enabled'")
	require.True(t, hasCooldown, "JSON must use 'cooldown_minutes'")
REDACTED
