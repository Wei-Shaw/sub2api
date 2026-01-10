//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

// testConfig 返回一个用于测试的默认配置
func testConfig() *config.Config {
	return &config.Config{RunMode: config.RunModeStandardREDACTED
REDACTED

// mockAccountRepoForPlatform 单平台测试用的 mock
type mockAccountRepoForPlatform struct {
	accounts         []Account
	accountsByID     map[int64]*Account
	listPlatformFunc func(ctx context.Context, platform string) ([]Account, error)
REDACTED

func (m *mockAccountRepoForPlatform) GetByID(ctx context.Context, id int64) (*Account, error) {
	if acc, ok := m.accountsByID[id]; ok {
		return acc, nil
REDACTED
	return nil, errors.New("account not found")
REDACTED

func (m *mockAccountRepoForPlatform) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	var result []*Account
	for _, id := range ids {
		if acc, ok := m.accountsByID[id]; ok {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED

func (m *mockAccountRepoForPlatform) ExistsByID(ctx context.Context, id int64) (bool, error) {
	if m.accountsByID == nil {
		return false, nil
REDACTED
	_, ok := m.accountsByID[id]
	return ok, nil
REDACTED

func (m *mockAccountRepoForPlatform) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	if m.listPlatformFunc != nil {
		return m.listPlatformFunc(ctx, platform)
REDACTED
	var result []Account
	for _, acc := range m.accounts {
		if acc.Platform == platform && acc.IsSchedulable() {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED

func (m *mockAccountRepoForPlatform) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	return m.ListSchedulableByPlatform(ctx, platform)
REDACTED

// Stub methods to implement AccountRepository interface
func (m *mockAccountRepoForPlatform) Create(ctx context.Context, account *Account) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) Update(ctx context.Context, account *Account) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) Delete(ctx context.Context, id int64) error { return nil REDACTED
func (m *mockAccountRepoForPlatform) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListActive(ctx context.Context) ([]Account, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) UpdateLastUsed(ctx context.Context, id int64) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) SetError(ctx context.Context, id int64, errorMsg string) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
REDACTED
func (m *mockAccountRepoForPlatform) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) ListSchedulable(ctx context.Context) ([]Account, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	var result []Account
	platformSet := make(map[string]bool)
	for _, p := range platforms {
		platformSet[p] = true
REDACTED
	for _, acc := range m.accounts {
		if platformSet[acc.Platform] && acc.IsSchedulable() {
			result = append(result, acc)
	REDACTED
REDACTED
	return result, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	return m.ListSchedulableByPlatforms(ctx, platforms)
REDACTED
func (m *mockAccountRepoForPlatform) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) SetAntigravityQuotaScopeLimit(ctx context.Context, id int64, scope AntigravityQuotaScope, resetAt time.Time) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) ClearRateLimit(ctx context.Context, id int64) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	return 0, nil
REDACTED

// Verify interface implementation
var _ AccountRepository = (*mockAccountRepoForPlatform)(nil)

// mockGatewayCacheForPlatform 单平台测试用的 cache mock
type mockGatewayCacheForPlatform struct {
	sessionBindings map[string]int64
REDACTED

func (m *mockGatewayCacheForPlatform) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	if id, ok := m.sessionBindings[sessionHash]; ok {
		return id, nil
REDACTED
	return 0, errors.New("not found")
REDACTED

func (m *mockGatewayCacheForPlatform) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	if m.sessionBindings == nil {
		m.sessionBindings = make(map[string]int64)
REDACTED
	m.sessionBindings[sessionHash] = accountID
	return nil
REDACTED

func (m *mockGatewayCacheForPlatform) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	return nil
REDACTED

type mockGroupRepoForGateway struct {
	groups           map[int64]*Group
	getByIDCalls     int
	getByIDLiteCalls int
REDACTED

func (m *mockGroupRepoForGateway) GetByID(ctx context.Context, id int64) (*Group, error) {
	m.getByIDCalls++
	if g, ok := m.groups[id]; ok {
		return g, nil
REDACTED
	return nil, ErrGroupNotFound
REDACTED

func (m *mockGroupRepoForGateway) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	m.getByIDLiteCalls++
	if g, ok := m.groups[id]; ok {
		return g, nil
REDACTED
	return nil, ErrGroupNotFound
REDACTED

func (m *mockGroupRepoForGateway) Create(ctx context.Context, group *Group) error { return nil REDACTED
func (m *mockGroupRepoForGateway) Update(ctx context.Context, group *Group) error { return nil REDACTED
func (m *mockGroupRepoForGateway) Delete(ctx context.Context, id int64) error     { return nil REDACTED
func (m *mockGroupRepoForGateway) DeleteCascade(ctx context.Context, id int64) ([]int64, error) {
	return nil, nil
REDACTED
func (m *mockGroupRepoForGateway) List(ctx context.Context, params pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockGroupRepoForGateway) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, status, search string, isExclusive *bool) ([]Group, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockGroupRepoForGateway) ListActive(ctx context.Context) ([]Group, error) {
	return nil, nil
REDACTED
func (m *mockGroupRepoForGateway) ListActiveByPlatform(ctx context.Context, platform string) ([]Group, error) {
	return nil, nil
REDACTED
func (m *mockGroupRepoForGateway) ExistsByName(ctx context.Context, name string) (bool, error) {
	return false, nil
REDACTED
func (m *mockGroupRepoForGateway) GetAccountCount(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
REDACTED
func (m *mockGroupRepoForGateway) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
REDACTED

func ptr[T any](v T) *T {
	return &v
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_Anthropic 测试 anthropic 单平台选择
func TestGatewayService_SelectAccountForModelWithPlatform_Anthropic(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
			{ID: 3, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED, // 应被隔离
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID, "应选择优先级最高的 anthropic 账户")
	require.Equal(t, PlatformAnthropic, acc.Platform, "应只返回 anthropic 平台账户")
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_Antigravity 测试 antigravity 单平台选择
func TestGatewayService_SelectAccountForModelWithPlatform_Antigravity(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED, // 应被隔离
			{ID: 2, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAntigravity)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
	require.Equal(t, PlatformAntigravity, acc.Platform, "应只返回 antigravity 平台账户")
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_PriorityAndLastUsed 测试优先级和最后使用时间
func TestGatewayService_SelectAccountForModelWithPlatform_PriorityAndLastUsed(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: ptr(now.Add(-1 * time.Hour))REDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: ptr(now.Add(-2 * time.Hour))REDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID, "同优先级应选择最久未用的账户")
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_GeminiOAuthPreference(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKeyREDACTED,
			{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuthREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "gemini-2.5-pro", nil, PlatformGemini)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID, "同优先级且未使用时应优先选择OAuth账户")
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_NoAvailableAccounts 测试无可用账户
func TestGatewayService_SelectAccountForModelWithPlatform_NoAvailableAccounts(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts:     []Account{REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
REDACTED
	require.Nil(t, acc)
	require.Contains(t, err.Error(), "no available accounts")
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_AllExcluded 测试所有账户被排除
func TestGatewayService_SelectAccountForModelWithPlatform_AllExcluded(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	excludedIDs := map[int64]struct{REDACTED{1: {REDACTED, 2: {REDACTEDREDACTED
	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", excludedIDs, PlatformAnthropic)
REDACTED
	require.Nil(t, acc)
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_Schedulability 测试账户可调度性检查
func TestGatewayService_SelectAccountForModelWithPlatform_Schedulability(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name       string
		accounts   []Account
		expectedID int64
REDACTED{
		{
			name: "过载账户被跳过",
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, OverloadUntil: ptr(now.Add(1 * time.Hour))REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			expectedID: 2,
	REDACTED,
		{
			name: "限流账户被跳过",
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, RateLimitResetAt: ptr(now.Add(1 * time.Hour))REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			expectedID: 2,
	REDACTED,
		{
			name: "非active账户被跳过",
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: "error", Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			expectedID: 2,
	REDACTED,
		{
			name: "schedulable=false被跳过",
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: falseREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			expectedID: 2,
	REDACTED,
		{
			name: "过期的过载账户可调度",
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, OverloadUntil: ptr(now.Add(-1 * time.Hour))REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			expectedID: 1,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockAccountRepoForPlatform{
				accounts:     tt.accounts,
				accountsByID: map[int64]*Account{REDACTED,
		REDACTED
			for i := range repo.accounts {
				repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
		REDACTED

			cache := &mockGatewayCacheForPlatform{REDACTED

			svc := &GatewayService{
				accountRepo: repo,
				cache:       cache,
				cfg:         testConfig(),
		REDACTED

			acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
		REDACTED
			require.NotNil(t, acc)
			require.Equal(t, tt.expectedID, acc.ID)
	REDACTED)
REDACTED
REDACTED

// TestGatewayService_SelectAccountForModelWithPlatform_StickySession 测试粘性会话
func TestGatewayService_SelectAccountForModelWithPlatform_StickySession(t *testing.T) {
	ctx := context.Background()

	t.Run("粘性会话命中-同平台", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 1REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "应返回粘性会话绑定的账户")
REDACTED)

	t.Run("粘性会话不匹配平台-降级选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED, // 粘性会话绑定但平台不匹配
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 1REDACTED, // 绑定 antigravity 账户
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		// 请求 anthropic 平台，但粘性会话绑定的是 antigravity 账户
		acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "粘性会话账户平台不匹配，应降级选择同平台账户")
		require.Equal(t, PlatformAnthropic, acc.Platform)
REDACTED)

	t.Run("粘性会话账户被排除-降级选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 1REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		excludedIDs := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
		acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", excludedIDs, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "粘性会话账户被排除，应选择其他账户")
REDACTED)

	t.Run("粘性会话账户不可调度-降级选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: "error", Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 1REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "粘性会话账户不可调度，应选择其他账户")
REDACTED)
REDACTED

func TestGatewayService_isModelSupportedByAccount(t *testing.T) {
	svc := &GatewayService{REDACTED

	tests := []struct {
		name     string
		account  *Account
		model    string
		expected bool
REDACTED{
		{
			name:     "Antigravity平台-支持claude模型",
			account:  &Account{Platform: PlatformAntigravityREDACTED,
			model:    "claude-3-5-sonnet-20241022",
			expected: true,
	REDACTED,
		{
			name:     "Antigravity平台-支持gemini模型",
			account:  &Account{Platform: PlatformAntigravityREDACTED,
			model:    "gemini-2.5-flash",
			expected: true,
	REDACTED,
		{
			name:     "Antigravity平台-不支持gpt模型",
			account:  &Account{Platform: PlatformAntigravityREDACTED,
			model:    "gpt-4",
			expected: false,
	REDACTED,
		{
			name:     "Anthropic平台-无映射配置-支持所有模型",
			account:  &Account{Platform: PlatformAnthropicREDACTED,
			model:    "claude-3-5-sonnet-20241022",
			expected: true,
	REDACTED,
		{
			name: "Anthropic平台-有映射配置-只支持配置的模型",
			account: &Account{
				Platform:    PlatformAnthropic,
		REDACTED"model_mapping": map[string]any{"claude-opus-4": "x"REDACTEDREDACTED,
		REDACTED,
			model:    "claude-3-5-sonnet-20241022",
			expected: false,
	REDACTED,
		{
			name: "Anthropic平台-有映射配置-支持配置的模型",
			account: &Account{
				Platform:    PlatformAnthropic,
		REDACTED"model_mapping": map[string]any{"claude-3-5-sonnet-20241022": "x"REDACTEDREDACTED,
		REDACTED,
			model:    "claude-3-5-sonnet-20241022",
			expected: true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.isModelSupportedByAccount(tt.account, tt.model)
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

// TestGatewayService_selectAccountWithMixedScheduling 测试混合调度
func TestGatewayService_selectAccountWithMixedScheduling(t *testing.T) {
	ctx := context.Background()

	t.Run("混合调度-Gemini优先选择OAuth账户", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeAPIKeyREDACTED,
				{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Type: AccountTypeOAuthREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "gemini-2.5-pro", nil, PlatformGemini)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "同优先级且未使用时应优先选择OAuth账户")
REDACTED)

	t.Run("混合调度-包含启用mixed_scheduling的antigravity账户", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": trueREDACTEDREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "应选择优先级最高的账户（包含启用混合调度的antigravity）")
REDACTED)

	t.Run("混合调度-过滤未启用mixed_scheduling的antigravity账户", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED, // 未启用 mixed_scheduling
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "未启用mixed_scheduling的antigravity账户应被过滤")
		require.Equal(t, PlatformAnthropic, acc.Platform)
REDACTED)

	t.Run("混合调度-粘性会话命中启用mixed_scheduling的antigravity账户", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": trueREDACTEDREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 2REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "应返回粘性会话绑定的启用mixed_scheduling的antigravity账户")
REDACTED)

	t.Run("混合调度-粘性会话命中未启用mixed_scheduling的antigravity账户-降级选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED, // 未启用 mixed_scheduling
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 2REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID, "粘性会话绑定的账户未启用mixed_scheduling，应降级选择anthropic账户")
REDACTED)

	t.Run("混合调度-仅有启用mixed_scheduling的antigravity账户", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": trueREDACTEDREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID)
		require.Equal(t, PlatformAntigravity, acc.Platform)
REDACTED)

	t.Run("混合调度-无可用账户", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED, // 未启用 mixed_scheduling
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.Nil(t, acc)
		require.Contains(t, err.Error(), "no available accounts")
REDACTED)
REDACTED

// TestAccount_IsMixedSchedulingEnabled 测试混合调度开关检查
func TestAccount_IsMixedSchedulingEnabled(t *testing.T) {
	tests := []struct {
		name     string
		account  Account
		expected bool
REDACTED{
		{
			name:     "非antigravity平台-返回false",
			account:  Account{Platform: PlatformAnthropicREDACTED,
			expected: false,
	REDACTED,
		{
			name:     "antigravity平台-无extra-返回false",
			account:  Account{Platform: PlatformAntigravityREDACTED,
			expected: false,
	REDACTED,
		{
			name:     "antigravity平台-extra无mixed_scheduling-返回false",
			account:  Account{Platform: PlatformAntigravity, Extra: map[string]any{REDACTEDREDACTED,
			expected: false,
	REDACTED,
		{
			name:     "antigravity平台-mixed_scheduling=false-返回false",
			account:  Account{Platform: PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": falseREDACTEDREDACTED,
			expected: false,
	REDACTED,
		{
			name:     "antigravity平台-mixed_scheduling=true-返回true",
			account:  Account{Platform: PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": trueREDACTEDREDACTED,
			expected: true,
	REDACTED,
		{
			name:     "antigravity平台-mixed_scheduling非bool类型-返回false",
			account:  Account{Platform: PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": "true"REDACTEDREDACTED,
			expected: false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.account.IsMixedSchedulingEnabled()
			require.Equal(t, tt.expected, got)
	REDACTED)
REDACTED
REDACTED

// mockConcurrencyService for testing
type mockConcurrencyService struct {
	accountLoads      map[int64]*AccountLoadInfo
	accountWaitCounts map[int64]int
	acquireResults    map[int64]bool
REDACTED

func (m *mockConcurrencyService) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	if m.accountLoads == nil {
		return map[int64]*AccountLoadInfo{REDACTED, nil
REDACTED
	result := make(map[int64]*AccountLoadInfo)
	for _, acc := range accounts {
		if load, ok := m.accountLoads[acc.ID]; ok {
			result[acc.ID] = load
	REDACTED else {
			result[acc.ID] = &AccountLoadInfo{
				AccountID:          acc.ID,
				CurrentConcurrency: 0,
				WaitingCount:       0,
				LoadRate:           0,
		REDACTED
	REDACTED
REDACTED
	return result, nil
REDACTED

func (m *mockConcurrencyService) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if m.accountWaitCounts == nil {
		return 0, nil
REDACTED
	return m.accountWaitCounts[accountID], nil
REDACTED

// TestGatewayService_SelectAccountWithLoadAwareness tests load-aware account selection
func TestGatewayService_SelectAccountWithLoadAwareness(t *testing.T) {
	ctx := context.Background()

	t.Run("禁用负载批量查询-降级到传统选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = false

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: nil, // No concurrency service
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil)
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(1), result.Account.ID, "应选择优先级最高的账号")
REDACTED)

	t.Run("无ConcurrencyService-降级到传统选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: nil,
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil)
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "应选择优先级最高的账号")
REDACTED)

	t.Run("排除账号-不选择被排除的账号", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = false

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: nil,
	REDACTED

		excludedIDs := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", excludedIDs)
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "不应选择被排除的账号")
REDACTED)

	t.Run("无可用账号-返回错误", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts:     []Account{REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = false

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: nil,
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil)
	REDACTED
		require.Nil(t, result)
		require.Contains(t, err.Error(), "no available accounts")
REDACTED)
REDACTED

func TestGatewayService_GroupResolution_ReusesContextGroup(t *testing.T) {
	ctx := context.Background()
	groupID := int64(42)
	group := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
		Hydrated: true,
REDACTED
	ctx = context.WithValue(ctx, ctxkey.Group, group)

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{groupID: groupREDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cfg:         testConfig(),
REDACTED

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "claude-3-5-sonnet-20241022", nil)
REDACTED
	require.NotNil(t, account)
	require.Equal(t, 0, groupRepo.getByIDCalls)
	require.Equal(t, 0, groupRepo.getByIDLiteCalls)
REDACTED

func TestGatewayService_GroupResolution_IgnoresInvalidContextGroup(t *testing.T) {
	ctx := context.Background()
	groupID := int64(42)
	ctxGroup := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
REDACTED
	ctx = context.WithValue(ctx, ctxkey.Group, ctxGroup)

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	group := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
REDACTED
	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{groupID: groupREDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cfg:         testConfig(),
REDACTED

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "claude-3-5-sonnet-20241022", nil)
REDACTED
	require.NotNil(t, account)
	require.Equal(t, 0, groupRepo.getByIDCalls)
	require.Equal(t, 1, groupRepo.getByIDLiteCalls)
REDACTED

func TestGatewayService_GroupContext_OverwritesInvalidContextGroup(t *testing.T) {
	groupID := int64(42)
	invalidGroup := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
REDACTED
	hydratedGroup := &Group{
		ID:       groupID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
		Hydrated: true,
REDACTED

	ctx := context.WithValue(context.Background(), ctxkey.Group, invalidGroup)
	svc := &GatewayService{REDACTED
	ctx = svc.withGroupContext(ctx, hydratedGroup)

	got, ok := ctx.Value(ctxkey.Group).(*Group)
	require.True(t, ok)
	require.Same(t, hydratedGroup, got)
REDACTED

func TestGatewayService_GroupResolution_FallbackUsesLiteOnce(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10)
	fallbackID := int64(11)
	group := &Group{
		ID:              groupID,
		Platform:        PlatformAnthropic,
		Status:          StatusActive,
		ClaudeCodeOnly:  true,
		FallbackGroupID: &fallbackID,
		Hydrated:        true,
REDACTED
	fallbackGroup := &Group{
		ID:       fallbackID,
		Platform: PlatformAnthropic,
		Status:   StatusActive,
REDACTED
	ctx = context.WithValue(ctx, ctxkey.Group, group)

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{fallbackID: fallbackGroupREDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		groupRepo:   groupRepo,
		cfg:         testConfig(),
REDACTED

	account, err := svc.SelectAccountForModelWithExclusions(ctx, &groupID, "", "claude-3-5-sonnet-20241022", nil)
REDACTED
	require.NotNil(t, account)
	require.Equal(t, 0, groupRepo.getByIDCalls)
	require.Equal(t, 1, groupRepo.getByIDLiteCalls)
REDACTED

func TestGatewayService_ResolveGatewayGroup_DetectsFallbackCycle(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10)
	fallbackID := int64(11)

	group := &Group{
		ID:              groupID,
		Platform:        PlatformAnthropic,
		Status:          StatusActive,
		ClaudeCodeOnly:  true,
		FallbackGroupID: &fallbackID,
REDACTED
	fallbackGroup := &Group{
		ID:              fallbackID,
		Platform:        PlatformAnthropic,
		Status:          StatusActive,
		ClaudeCodeOnly:  true,
		FallbackGroupID: &groupID,
REDACTED

	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{
			groupID:    group,
			fallbackID: fallbackGroup,
	REDACTED,
REDACTED

	svc := &GatewayService{
		groupRepo: groupRepo,
REDACTED

	gotGroup, gotID, err := svc.resolveGatewayGroup(ctx, &groupID)
REDACTED
	require.Nil(t, gotGroup)
	require.Nil(t, gotID)
	require.Contains(t, err.Error(), "fallback group cycle")
REDACTED
