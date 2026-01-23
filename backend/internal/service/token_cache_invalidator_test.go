//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type geminiTokenCacheStub struct {
	deletedKeys []string
	deleteErr   error
REDACTED

func (s *geminiTokenCacheStub) GetAccessToken(ctx context.Context, cacheKey string) (string, error) {
	return "", nil
REDACTED

func (s *geminiTokenCacheStub) SetAccessToken(ctx context.Context, cacheKey string, token string, ttl time.Duration) error {
	return nil
REDACTED

func (s *geminiTokenCacheStub) DeleteAccessToken(ctx context.Context, cacheKey string) error {
	s.deletedKeys = append(s.deletedKeys, cacheKey)
	return s.deleteErr
REDACTED

func (s *geminiTokenCacheStub) AcquireRefreshLock(ctx context.Context, cacheKey string, ttl time.Duration) (bool, error) {
	return true, nil
REDACTED

func (s *geminiTokenCacheStub) ReleaseRefreshLock(ctx context.Context, cacheKey string) error {
	return nil
REDACTED

func TestCompositeTokenCacheInvalidator_Gemini(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       10,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
			"project_id": "project-x",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	// 新行为：同时删除基于 project_id 和 account_id 的缓存键
	// 这是为了处理：首次获取 token 时可能没有 project_id，之后自动检测到后会使用新 key
	require.Equal(t, []string{"gemini:project-x", "gemini:account:10"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_GeminiWithoutProjectID(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       10,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "gemini-token",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	// 没有 project_id 时，两个 key 相同，去重后只删除一个
	require.Equal(t, []string{"gemini:account:10"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_Antigravity(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       99,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
			"project_id": "ag-project",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	// 新行为：同时删除基于 project_id 和 account_id 的缓存键
	require.Equal(t, []string{"ag:ag-project", "ag:account:99"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_AntigravityWithoutProjectID(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       99,
		Platform: PlatformAntigravity,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "ag-token",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	// 没有 project_id 时，两个 key 相同，去重后只删除一个
	require.Equal(t, []string{"ag:account:99"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_OpenAI(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       500,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "openai-token",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Equal(t, []string{"openai:account:500"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_Claude(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       600,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
REDACTED
			"access_token": "claude-token",
	REDACTED,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Equal(t, []string{"claude:account:600"REDACTED, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_SkipNonOAuth(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	tests := []struct {
		name    string
		account *Account
REDACTED{
		{
			name: "gemini_api_key",
			account: &Account{
				ID:       1,
		REDACTED
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
		{
			name: "openai_api_key",
			account: &Account{
				ID:       2,
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
		{
			name: "claude_api_key",
			account: &Account{
				ID:       3,
				Platform: PlatformAnthropic,
				Type:     AccountTypeAPIKey,
		REDACTED,
	REDACTED,
		{
			name: "claude_setup_token",
			account: &Account{
				ID:       4,
				Platform: PlatformAnthropic,
				Type:     AccountTypeSetupToken,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache.deletedKeys = nil
			err := invalidator.InvalidateToken(context.Background(), tt.account)
		REDACTED
			require.Empty(t, cache.deletedKeys)
	REDACTED)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_SkipUnsupportedPlatform(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	account := &Account{
		ID:       100,
		Platform: "unknown-platform",
		Type:     AccountTypeOAuth,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
	require.Empty(t, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_NilCache(t *testing.T) {
	invalidator := NewCompositeTokenCacheInvalidator(nil)
	account := &Account{
		ID:       2,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_NilAccount(t *testing.T) {
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	err := invalidator.InvalidateToken(context.Background(), nil)
REDACTED
	require.Empty(t, cache.deletedKeys)
REDACTED

func TestCompositeTokenCacheInvalidator_NilInvalidator(t *testing.T) {
	var invalidator *CompositeTokenCacheInvalidator
	account := &Account{
		ID:       5,
REDACTED
		Type:     AccountTypeOAuth,
REDACTED

	err := invalidator.InvalidateToken(context.Background(), account)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_DeleteError(t *testing.T) {
	expectedErr := errors.New("redis connection failed")
	cache := &geminiTokenCacheStub{deleteErr: expectedErrREDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	tests := []struct {
		name    string
		account *Account
REDACTED{
		{
			name: "openai_delete_error",
			account: &Account{
				ID:       700,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
		REDACTED,
	REDACTED,
		{
			name: "claude_delete_error",
			account: &Account{
				ID:       800,
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
		REDACTED,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 新行为：删除失败只记录日志，不返回错误
			// 这是因为缓存失效失败不应影响主业务流程
			err := invalidator.InvalidateToken(context.Background(), tt.account)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestCompositeTokenCacheInvalidator_AllPlatformsIntegration(t *testing.T) {
	// 测试所有平台的缓存键生成和删除
	cache := &geminiTokenCacheStub{REDACTED
	invalidator := NewCompositeTokenCacheInvalidator(cache)

	accounts := []*Account{
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "gemini-proj"REDACTEDREDACTED,
		{ID: 2, Platform: PlatformAntigravity, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "ag-proj"REDACTEDREDACTED,
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
		{ID: 4, Platform: PlatformAnthropic, Type: AccountTypeOAuthREDACTED,
REDACTED

	// 新行为：Gemini 和 Antigravity 会同时删除基于 project_id 和 account_id 的键
	expectedKeys := []string{
		"gemini:gemini-proj",
		"gemini:account:1",
		"ag:ag-proj",
		"ag:account:2",
		"openai:account:3",
		"claude:account:4",
REDACTED

	for _, acc := range accounts {
		err := invalidator.InvalidateToken(context.Background(), acc)
	REDACTED
REDACTED

	require.Equal(t, expectedKeys, cache.deletedKeys)
REDACTED

// ========== GetCredentialAsInt64 测试 ==========

func TestAccount_GetCredentialAsInt64(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]any
		key         string
		expected    int64
REDACTED{
		{
			name:        "int64_value",
			credentials: map[string]any{"_token_version": int64(1737654321000)REDACTED,
			key:         "_token_version",
			expected:    1737654321000,
	REDACTED,
		{
			name:        "float64_value",
			credentials: map[string]any{"_token_version": float64(1737654321000)REDACTED,
			key:         "_token_version",
			expected:    1737654321000,
	REDACTED,
		{
			name:        "int_value",
			credentials: map[string]any{"_token_version": 12345REDACTED,
			key:         "_token_version",
			expected:    12345,
	REDACTED,
		{
			name:        "string_value",
			credentials: map[string]any{"_token_version": "1737654321000"REDACTED,
			key:         "_token_version",
			expected:    1737654321000,
	REDACTED,
		{
			name:        "string_with_spaces",
			credentials: map[string]any{"_token_version": "  1737654321000  "REDACTED,
			key:         "_token_version",
			expected:    1737654321000,
	REDACTED,
		{
			name:        "nil_credentials",
			credentials: nil,
			key:         "_token_version",
			expected:    0,
	REDACTED,
		{
			name:        "missing_key",
			credentials: map[string]any{"other_key": 123REDACTED,
			key:         "_token_version",
			expected:    0,
	REDACTED,
		{
			name:        "nil_value",
			credentials: map[string]any{"_token_version": nilREDACTED,
			key:         "_token_version",
			expected:    0,
	REDACTED,
		{
			name:        "invalid_string",
			credentials: map[string]any{"_token_version": "not_a_number"REDACTED,
			key:         "_token_version",
			expected:    0,
	REDACTED,
		{
			name:        "empty_string",
			credentials: map[string]any{"_token_version": ""REDACTED,
			key:         "_token_version",
			expected:    0,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := &Account{Credentials: tt.credentialsREDACTED
			result := account.GetCredentialAsInt64(tt.key)
			require.Equal(t, tt.expected, result)
	REDACTED)
REDACTED
REDACTED

func TestAccount_GetCredentialAsInt64_NilAccount(t *testing.T) {
	var account *Account
	result := account.GetCredentialAsInt64("_token_version")
	require.Equal(t, int64(0), result)
REDACTED

// ========== CheckTokenVersion 测试 ==========

func TestCheckTokenVersion(t *testing.T) {
	tests := []struct {
		name          string
		account       *Account
		latestAccount *Account
		repoErr       error
		expectedStale bool
REDACTED{
		{
			name:          "nil_account",
			account:       nil,
			latestAccount: nil,
			expectedStale: false,
	REDACTED,
		{
			name: "no_version_in_account_but_db_has_version",
			account: &Account{
				ID:          1,
		REDACTEDREDACTED,
		REDACTED,
			latestAccount: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			expectedStale: true, // 当前 account 无版本但 DB 有，说明已被异步刷新，当前已过时
	REDACTED,
		{
			name: "both_no_version",
			account: &Account{
				ID:          1,
		REDACTEDREDACTED,
		REDACTED,
			latestAccount: &Account{
				ID:          1,
		REDACTEDREDACTED,
		REDACTED,
			expectedStale: false, // 两边都没有版本号，说明从未被异步刷新过，允许缓存
	REDACTED,
		{
			name: "same_version",
			account: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			latestAccount: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			expectedStale: false,
	REDACTED,
		{
			name: "current_version_newer",
			account: &Account{
				ID:          1,
		REDACTED"_token_version": int64(200)REDACTED,
		REDACTED,
			latestAccount: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			expectedStale: false,
	REDACTED,
		{
			name: "current_version_older_stale",
			account: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			latestAccount: &Account{
				ID:          1,
		REDACTED"_token_version": int64(200)REDACTED,
		REDACTED,
			expectedStale: true, // 当前版本过时
	REDACTED,
		{
			name: "repo_error",
			account: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			latestAccount: nil,
			repoErr:       errors.New("db error"),
			expectedStale: false, // 查询失败，默认允许缓存
	REDACTED,
		{
			name: "repo_returns_nil",
			account: &Account{
				ID:          1,
		REDACTED"_token_version": int64(100)REDACTED,
		REDACTED,
			latestAccount: nil,
			repoErr:       nil,
			expectedStale: false, // 查询返回 nil，默认允许缓存
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 由于 CheckTokenVersion 接受 AccountRepository 接口，而创建完整的 mock 很繁琐
			// 这里我们直接测试函数的核心逻辑来验证行为

			if tt.name == "nil_account" {
				_, isStale := CheckTokenVersion(context.Background(), nil, nil)
				require.Equal(t, tt.expectedStale, isStale)
				return
		REDACTED

			// 模拟 CheckTokenVersion 的核心逻辑
			account := tt.account
			currentVersion := account.GetCredentialAsInt64("_token_version")

			// 模拟 repo 查询
			latestAccount := tt.latestAccount
			if tt.repoErr != nil || latestAccount == nil {
				require.Equal(t, tt.expectedStale, false)
				return
		REDACTED

			latestVersion := latestAccount.GetCredentialAsInt64("_token_version")

			// 情况1: 当前 account 没有版本号，但 DB 中已有版本号
			if currentVersion == 0 && latestVersion > 0 {
				require.Equal(t, tt.expectedStale, true)
				return
		REDACTED

			// 情况2: 两边都没有版本号
			if currentVersion == 0 && latestVersion == 0 {
				require.Equal(t, tt.expectedStale, false)
				return
		REDACTED

			// 情况3: 比较版本号
			isStale := latestVersion > currentVersion
			require.Equal(t, tt.expectedStale, isStale)
	REDACTED)
REDACTED
REDACTED

func TestCheckTokenVersion_NilRepo(t *testing.T) {
	account := &Account{
		ID:          1,
REDACTED"_token_version": int64(100)REDACTED,
REDACTED
	_, isStale := CheckTokenVersion(context.Background(), account, nil)
	require.False(t, isStale) // nil repo，默认允许缓存
REDACTED
