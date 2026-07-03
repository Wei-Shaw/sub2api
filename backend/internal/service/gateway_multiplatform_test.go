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
	getByIDCalls     int
REDACTED

func (m *mockAccountRepoForPlatform) GetByID(ctx context.Context, id int64) (*Account, error) {
	m.getByIDCalls++
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

func (m *mockAccountRepoForPlatform) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	return nil, nil
REDACTED

func (m *mockAccountRepoForPlatform) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) Update(ctx context.Context, account *Account) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) Delete(ctx context.Context, id int64) error { return nil REDACTED
func (m *mockAccountRepoForPlatform) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
REDACTED
func (m *mockAccountRepoForPlatform) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
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
func (m *mockAccountRepoForPlatform) ClearError(ctx context.Context, id int64) error {
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
func (m *mockAccountRepoForPlatform) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return m.ListSchedulableByPlatform(ctx, platform)
REDACTED
func (m *mockAccountRepoForPlatform) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return m.ListSchedulableByPlatforms(ctx, platforms)
REDACTED
func (m *mockAccountRepoForPlatform) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time, reason ...string) error {
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
func (m *mockAccountRepoForPlatform) ClearModelRateLimits(ctx context.Context, id int64) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) UpdateSessionWindowEnd(ctx context.Context, id int64, end time.Time) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
REDACTED
func (m *mockAccountRepoForPlatform) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	return 0, nil
REDACTED

func (m *mockAccountRepoForPlatform) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
REDACTED

func (m *mockAccountRepoForPlatform) ResetQuotaUsed(ctx context.Context, id int64) error {
	return nil
REDACTED

func (m *mockAccountRepoForPlatform) RevertProxyFallback(ctx context.Context, accountID int64) error {
	return nil
REDACTED

func (m *mockAccountRepoForPlatform) ListShadowsByParent(ctx context.Context, parentID int64) ([]*Account, error) {
	return nil, nil
REDACTED

// Verify interface implementation
var _ AccountRepository = (*mockAccountRepoForPlatform)(nil)

// mockGatewayCacheForPlatform 单平台测试用的 cache mock
type mockGatewayCacheForPlatform struct {
	sessionBindings map[string]int64
	deletedSessions map[string]int
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

func (m *mockGatewayCacheForPlatform) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	if m.sessionBindings == nil {
		return nil
REDACTED
	if m.deletedSessions == nil {
		m.deletedSessions = make(map[string]int)
REDACTED
	m.deletedSessions[sessionHash]++
	delete(m.sessionBindings, sessionHash)
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
func (m *mockGroupRepoForGateway) GetAccountCount(ctx context.Context, groupID int64) (int64, int64, error) {
	return 0, 0, nil
REDACTED
func (m *mockGroupRepoForGateway) DeleteAccountGroupsByGroupID(ctx context.Context, groupID int64) (int64, error) {
	return 0, nil
REDACTED

func (m *mockGroupRepoForGateway) BindAccountsToGroup(ctx context.Context, groupID int64, accountIDs []int64) error {
	return nil
REDACTED

func (m *mockGroupRepoForGateway) GetAccountIDsByGroupIDs(ctx context.Context, groupIDs []int64) ([]int64, error) {
	return nil, nil
REDACTED

func (m *mockGroupRepoForGateway) UpdateSortOrders(ctx context.Context, updates []GroupSortOrderUpdate) error {
	return nil
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

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-sonnet-4-5", nil, PlatformAntigravity)
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
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
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

func TestGatewayService_SelectAccountForModelWithExclusions_ForcePlatform(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAntigravity)

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
			{ID: 2, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
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

	acc, err := svc.SelectAccountForModelWithExclusions(ctx, nil, "", "claude-sonnet-4-5", nil)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
	require.Equal(t, PlatformAntigravity, acc.Platform)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_RoutedStickySessionClears(t *testing.T) {
	ctx := context.Background()
	groupID := int64(10)
	requestedModel := "claude-3-5-sonnet-20241022"

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 2, Status: StatusDisabled, Schedulable: trueREDACTED,
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

	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{
			groupID: {
				ID:                  groupID,
				Name:                "route-group",
				Platform:            PlatformAnthropic,
				Status:              StatusActive,
				Hydrated:            true,
				ModelRoutingEnabled: true,
				ModelRouting: map[string][]int64{
					requestedModel: {1, 2REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
		groupRepo:   groupRepo,
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, &groupID, "session-123", requestedModel, nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
	require.Equal(t, 1, cache.deletedSessions["session-123"])
	require.Equal(t, int64(2), cache.sessionBindings["session-123"])
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_RoutedStickySessionHit(t *testing.T) {
	ctx := context.Background()
	groupID := int64(11)
	requestedModel := "claude-3-5-sonnet-20241022"

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{
		sessionBindings: map[string]int64{"session-456": 1REDACTED,
REDACTED

	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{
			groupID: {
				ID:                  groupID,
				Name:                "route-group-hit",
				Platform:            PlatformAnthropic,
				Status:              StatusActive,
				Hydrated:            true,
				ModelRoutingEnabled: true,
				ModelRouting: map[string][]int64{
					requestedModel: {1, 2REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
		groupRepo:   groupRepo,
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, &groupID, "session-456", requestedModel, nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_RoutedFallbackToNormal(t *testing.T) {
	ctx := context.Background()
	groupID := int64(12)
	requestedModel := "claude-3-5-sonnet-20241022"

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{REDACTED

	groupRepo := &mockGroupRepoForGateway{
		groups: map[int64]*Group{
			groupID: {
				ID:                  groupID,
				Name:                "route-fallback",
				Platform:            PlatformAnthropic,
				Status:              StatusActive,
				Hydrated:            true,
				ModelRoutingEnabled: true,
				ModelRouting: map[string][]int64{
					requestedModel: {99REDACTED,
			REDACTED,
		REDACTED,
	REDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
		groupRepo:   groupRepo,
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, &groupID, "", requestedModel, nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_NoModelSupport(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformAnthropic,
				Priority:    1,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED"model_mapping": map[string]any{"claude-3-5-haiku-20241022": "claude-3-5-haiku-20241022"REDACTEDREDACTED,
		REDACTED,
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
	require.Nil(t, acc)
	require.Contains(t, err.Error(), "supporting model")
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_GeminiPreferOAuth(t *testing.T) {
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
	require.Equal(t, int64(2), acc.ID)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_GeminiAPIKeyModelMappingFilter(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformGemini,
				Type:        AccountTypeAPIKey,
				Priority:    1,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED"model_mapping": map[string]any{"gemini-2.5-pro": "gemini-2.5-pro"REDACTEDREDACTED,
		REDACTED,
			{
				ID:          2,
				Platform:    PlatformGemini,
				Type:        AccountTypeAPIKey,
				Priority:    2,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED"model_mapping": map[string]any{"gemini-2.5-flash": "gemini-2.5-flash"REDACTEDREDACTED,
		REDACTED,
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

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "gemini-2.5-flash", nil, PlatformGemini)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID, "应过滤不支持请求模型的 APIKey 账号")

	acc, err = svc.selectAccountForModelWithPlatform(ctx, nil, "", "gemini-3-pro-preview", nil, PlatformGemini)
REDACTED
	require.Nil(t, acc)
	require.Contains(t, err.Error(), "supporting model")
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_StickyInGroup(t *testing.T) {
	ctx := context.Background()
	groupID := int64(50)

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{
		sessionBindings: map[string]int64{"session-group": 1REDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, &groupID, "session-group", "", nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(1), acc.ID)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_StickyModelMismatchFallback(t *testing.T) {
	ctx := context.Background()

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{
				ID:          1,
				Platform:    PlatformAnthropic,
				Priority:    1,
				Status:      StatusActive,
				Schedulable: true,
		REDACTED"model_mapping": map[string]any{"claude-3-5-haiku-20241022": "claude-3-5-haiku-20241022"REDACTEDREDACTED,
		REDACTED,
			{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
	REDACTED,
		accountsByID: map[int64]*Account{REDACTED,
REDACTED
	for i := range repo.accounts {
		repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
REDACTED

	cache := &mockGatewayCacheForPlatform{
		sessionBindings: map[string]int64{"session-miss": 1REDACTED,
REDACTED

	svc := &GatewayService{
		accountRepo: repo,
		cache:       cache,
		cfg:         testConfig(),
REDACTED

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "session-miss", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_PreferNeverUsed(t *testing.T) {
	ctx := context.Background()
	lastUsed := time.Now().Add(-1 * time.Hour)

	repo := &mockAccountRepoForPlatform{
		accounts: []Account{
			{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: &lastUsedREDACTED,
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

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
REDACTED
	require.NotNil(t, acc)
	require.Equal(t, int64(2), acc.ID)
REDACTED

func TestGatewayService_SelectAccountForModelWithPlatform_NoAccounts(t *testing.T) {
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

	acc, err := svc.selectAccountForModelWithPlatform(ctx, nil, "", "", nil, PlatformAnthropic)
REDACTED
	require.Nil(t, acc)
	require.ErrorIs(t, err, ErrNoAvailableAccounts)
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
			name:     "Antigravity平台-支持默认映射中的claude模型",
			account:  &Account{Platform: PlatformAntigravityREDACTED,
			model:    "claude-sonnet-4-5",
			expected: true,
	REDACTED,
		{
			name:     "Antigravity平台-不支持非默认映射中的claude模型",
			account:  &Account{Platform: PlatformAntigravityREDACTED,
			model:    "claude-3-5-sonnet-20241022",
			expected: false,
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
		{
			name:     "Gemini平台-无映射配置-支持所有模型",
			account:  &Account{Platform: PlatformGemini, Type: AccountTypeAPIKeyREDACTED,
			model:    "gemini-2.5-flash",
			expected: true,
	REDACTED,
		{
			name: "Gemini平台-有映射配置-只支持配置的模型",
			account: &Account{
		REDACTED
				Type:     AccountTypeAPIKey,
		REDACTED
					"model_mapping": map[string]any{"gemini-2.5-pro": "gemini-2.5-pro"REDACTED,
			REDACTED,
		REDACTED,
			model:    "gemini-2.5-flash",
			expected: false,
	REDACTED,
		{
			name: "Gemini平台-有映射配置-支持配置的模型",
			account: &Account{
		REDACTED
				Type:     AccountTypeAPIKey,
		REDACTED
					"model_mapping": map[string]any{"gemini-2.5-pro": "gemini-2.5-pro"REDACTED,
			REDACTED,
		REDACTED,
			model:    "gemini-2.5-pro",
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

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-sonnet-4-5", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID, "应选择优先级最高的账户（包含启用混合调度的antigravity）")
REDACTED)

	t.Run("混合调度-Gemini家族限流后跳过Antigravity账户", func(t *testing.T) {
		resetAt := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID:          1,
					Platform:    PlatformAntigravity,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
					Extra: map[string]any{
						"mixed_scheduling": true,
						modelRateLimitsKey: map[string]any{
							antigravityGeminiModelRateLimitKey: map[string]any{
								"rate_limit_reset_at": resetAt,
						REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
				{
					ID:          2,
					Platform:    PlatformAntigravity,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
					Extra: map[string]any{
						"mixed_scheduling": true,
						modelRateLimitsKey: map[string]any{
							antigravityGeminiModelRateLimitKey: map[string]any{
								"rate_limit_reset_at": resetAt,
						REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
				{
					ID:          3,
					Platform:    PlatformAntigravity,
					Priority:    2,
					Status:      StatusActive,
					Schedulable: true,
					Extra:       map[string]any{"mixed_scheduling": trueREDACTED,
			REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       &mockGatewayCacheForPlatform{REDACTED,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "gemini-3-pro-preview", nil, PlatformGemini)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(3), acc.ID)
REDACTED)

	t.Run("混合调度-Gemini家族限流不影响Claude调度", func(t *testing.T) {
		resetAt := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID:          1,
					Platform:    PlatformAntigravity,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
					Extra: map[string]any{
						"mixed_scheduling": true,
						modelRateLimitsKey: map[string]any{
							antigravityGeminiModelRateLimitKey: map[string]any{
								"rate_limit_reset_at": resetAt,
						REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       &mockGatewayCacheForPlatform{REDACTED,
			cfg:         testConfig(),
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-sonnet-4-5", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID)
REDACTED)

	t.Run("混合调度-路由优先选择路由账号", func(t *testing.T) {
		groupID := int64(30)
		requestedModel := "claude-sonnet-4-5"
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

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Name:                "route-mixed-select",
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						requestedModel: {2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "", requestedModel, nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID)
REDACTED)

	t.Run("混合调度-路由粘性命中", func(t *testing.T) {
		groupID := int64(31)
		requestedModel := "claude-sonnet-4-5"
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAntigravity, Priority: 2, Status: StatusActive, Schedulable: true, Extra: map[string]any{"mixed_scheduling": trueREDACTED, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-777": 2REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Name:                "route-mixed-sticky",
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						requestedModel: {2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "session-777", requestedModel, nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID)
REDACTED)

	t.Run("混合调度-路由账号缺失回退", func(t *testing.T) {
		groupID := int64(32)
		requestedModel := "claude-3-5-sonnet-20241022"
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

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Name:                "route-mixed-miss",
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						requestedModel: {99REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "", requestedModel, nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID)
REDACTED)

	t.Run("混合调度-路由账号未启用mixed_scheduling回退", func(t *testing.T) {
		groupID := int64(33)
		requestedModel := "claude-3-5-sonnet-20241022"
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

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Name:                "route-mixed-disabled",
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						requestedModel: {2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "", requestedModel, nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID)
REDACTED)

	t.Run("混合调度-路由过滤覆盖", func(t *testing.T) {
		groupID := int64(35)
		requestedModel := "claude-3-5-sonnet-20241022"
		resetAt := time.Now().Add(10 * time.Minute)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: falseREDACTED,
				{ID: 3, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
				{
					ID:          4,
					Platform:    PlatformAnthropic,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
					Extra: map[string]any{
						"model_rate_limits": map[string]any{
							"claude-3-5-sonnet-20241022": map[string]any{
								"rate_limit_reset_at": resetAt.Format(time.RFC3339),
						REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
				{
					ID:          5,
					Platform:    PlatformAnthropic,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
			REDACTED"model_mapping": map[string]any{"claude-3-5-haiku-20241022": "claude-3-5-haiku-20241022"REDACTEDREDACTED,
			REDACTED,
				{ID: 6, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
				{ID: 7, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Name:                "route-mixed-filter",
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						requestedModel: {1, 2, 3, 4, 5, 6, 7REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		excluded := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "", requestedModel, excluded, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(7), acc.ID)
REDACTED)

	t.Run("混合调度-粘性命中分组账号", func(t *testing.T) {
		groupID := int64(34)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-group": 1REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:       groupID,
					Platform: PlatformAnthropic,
					Status:   StatusActive,
					Hydrated: true,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "session-group", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(1), acc.ID)
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

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "session-123", "claude-sonnet-4-5", nil, PlatformAnthropic)
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

	t.Run("混合调度-粘性会话不可调度-清理并回退", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusDisabled, Schedulable: true, Extra: map[string]any{"mixed_scheduling": trueREDACTEDREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
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

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "session-123", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID)
		require.Equal(t, 1, cache.deletedSessions["session-123"])
		require.Equal(t, int64(2), cache.sessionBindings["session-123"])
REDACTED)

	t.Run("混合调度-路由粘性不可调度-清理并回退", func(t *testing.T) {
		groupID := int64(12)
		requestedModel := "claude-3-5-sonnet-20241022"
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAntigravity, Priority: 1, Status: StatusDisabled, Schedulable: true, Extra: map[string]any{"mixed_scheduling": trueREDACTEDREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"session-123": 1REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Name:                "route-mixed",
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						requestedModel: {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo: repo,
			cache:       cache,
			cfg:         testConfig(),
			groupRepo:   groupRepo,
	REDACTED

		acc, err := svc.selectAccountWithMixedScheduling(ctx, &groupID, "session-123", requestedModel, nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID)
		require.Equal(t, 1, cache.deletedSessions["session-123"])
		require.Equal(t, int64(2), cache.sessionBindings["session-123"])
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

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-sonnet-4-5", nil, PlatformAnthropic)
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
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
REDACTED)

	t.Run("混合调度-不支持模型返回错误", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{
					ID:          1,
					Platform:    PlatformAnthropic,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
			REDACTED"model_mapping": map[string]any{"claude-3-5-haiku-20241022": "claude-3-5-haiku-20241022"REDACTEDREDACTED,
			REDACTED,
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
		require.Contains(t, err.Error(), "supporting model")
REDACTED)

	t.Run("混合调度-优先未使用账号", func(t *testing.T) {
		lastUsed := time.Now().Add(-2 * time.Hour)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, LastUsedAt: &lastUsedREDACTED,
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

		acc, err := svc.selectAccountWithMixedScheduling(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, PlatformAnthropic)
	REDACTED
		require.NotNil(t, acc)
		require.Equal(t, int64(2), acc.ID)
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

type mockConcurrencyCache struct {
	acquireAccountCalls int
	loadBatchCalls      int
	acquireResults      map[int64]bool
	loadBatchErr        error
	loadMap             map[int64]*AccountLoadInfo
	waitCounts          map[int64]int
	skipDefaultLoad     bool
REDACTED

func (m *mockConcurrencyCache) AcquireAccountSlot(ctx context.Context, accountID int64, maxConcurrency int, requestID string) (bool, error) {
	m.acquireAccountCalls++
	if m.acquireResults != nil {
		if result, ok := m.acquireResults[accountID]; ok {
			return result, nil
	REDACTED
REDACTED
	return true, nil
REDACTED

func (m *mockConcurrencyCache) ReleaseAccountSlot(ctx context.Context, accountID int64, requestID string) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) GetAccountConcurrency(ctx context.Context, accountID int64) (int, error) {
	return 0, nil
REDACTED

func (m *mockConcurrencyCache) GetAccountConcurrencyBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(accountIDs))
	for _, accountID := range accountIDs {
		result[accountID] = 0
REDACTED
	return result, nil
REDACTED

func (m *mockConcurrencyCache) IncrementAccountWaitCount(ctx context.Context, accountID int64, maxWait int) (bool, error) {
	return true, nil
REDACTED

func (m *mockConcurrencyCache) DecrementAccountWaitCount(ctx context.Context, accountID int64) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) GetAccountWaitingCount(ctx context.Context, accountID int64) (int, error) {
	if m.waitCounts != nil {
		if count, ok := m.waitCounts[accountID]; ok {
			return count, nil
	REDACTED
REDACTED
	return 0, nil
REDACTED

func (m *mockConcurrencyCache) AcquireUserSlot(ctx context.Context, userID int64, maxConcurrency int, requestID string) (bool, error) {
	return true, nil
REDACTED

func (m *mockConcurrencyCache) ReleaseUserSlot(ctx context.Context, userID int64, requestID string) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) GetUserConcurrency(ctx context.Context, userID int64) (int, error) {
	return 0, nil
REDACTED

func (m *mockConcurrencyCache) IncrementWaitCount(ctx context.Context, userID int64, maxWait int) (bool, error) {
	return true, nil
REDACTED

func (m *mockConcurrencyCache) DecrementWaitCount(ctx context.Context, userID int64) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) GetAccountsLoadBatch(ctx context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	m.loadBatchCalls++
	if m.loadBatchErr != nil {
		return nil, m.loadBatchErr
REDACTED
	result := make(map[int64]*AccountLoadInfo, len(accounts))
	if m.skipDefaultLoad && m.loadMap != nil {
		for _, acc := range accounts {
			if load, ok := m.loadMap[acc.ID]; ok {
				result[acc.ID] = load
		REDACTED
	REDACTED
		return result, nil
REDACTED
	for _, acc := range accounts {
		if m.loadMap != nil {
			if load, ok := m.loadMap[acc.ID]; ok {
				result[acc.ID] = load
				continue
		REDACTED
	REDACTED
		result[acc.ID] = &AccountLoadInfo{
			AccountID:          acc.ID,
			CurrentConcurrency: 0,
			WaitingCount:       0,
			LoadRate:           0,
	REDACTED
REDACTED
	return result, nil
REDACTED

func (m *mockConcurrencyCache) CleanupExpiredAccountSlots(ctx context.Context, accountID int64) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) CleanupExpiredAccountSlotKeys(ctx context.Context) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) CleanupStaleProcessSlots(ctx context.Context, activeRequestPrefix string) error {
	return nil
REDACTED

func (m *mockConcurrencyCache) GetUsersLoadBatch(ctx context.Context, users []UserWithConcurrency) (map[int64]*UserLoadInfo, error) {
	result := make(map[int64]*UserLoadInfo, len(users))
	for _, user := range users {
		result[user.ID] = &UserLoadInfo{
			UserID:             user.ID,
			CurrentConcurrency: 0,
			WaitingCount:       0,
			LoadRate:           0,
	REDACTED
REDACTED
	return result, nil
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

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(1), result.Account.ID, "应选择优先级最高的账号")
REDACTED)

	t.Run("模型路由-无ConcurrencyService也生效", func(t *testing.T) {
		groupID := int64(1)
		sessionHash := "sticky"

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, AccountGroups: []AccountGroup{{GroupID: groupIDREDACTEDREDACTEDREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{sessionHash: 1REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-a": {1REDACTED,
						"claude-b": {2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: nil, // legacy path
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, "claude-b", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "切换到 claude-b 时应按模型路由切换账号")
		require.Equal(t, int64(2), cache.sessionBindings[sessionHash], "粘性绑定应更新为路由选择的账号")
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

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
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
		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", excludedIDs, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "不应选择被排除的账号")
REDACTED)

	t.Run("粘性命中-不调用GetByID", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"sticky": 1REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "sticky", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(1), result.Account.ID)
		require.Equal(t, 0, repo.getByIDCalls, "粘性命中不应调用GetByID")
		require.Equal(t, 0, concurrencyCache.loadBatchCalls, "粘性命中应在负载批量查询前返回")
REDACTED)

	t.Run("粘性账号不在候选集-回退负载感知选择", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"sticky": 1REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "sticky", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "粘性账号不在候选集时应回退到可用账号")
		require.Equal(t, 0, repo.getByIDCalls, "粘性账号缺失不应回退到GetByID")
		require.Equal(t, 1, concurrencyCache.loadBatchCalls, "应继续进行负载批量查询")
REDACTED)

	t.Run("粘性账号禁用-清理会话并回退选择", func(t *testing.T) {
		testCtx := context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAnthropic)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: false, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED
		repo.listPlatformFunc = func(ctx context.Context, platform string) ([]Account, error) {
			return repo.accounts, nil
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"sticky": 1REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(testCtx, nil, "sticky", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "粘性账号禁用时应回退到可用账号")
		updatedID, ok := cache.sessionBindings["sticky"]
		require.True(t, ok, "粘性会话应更新绑定")
		require.Equal(t, int64(2), updatedID, "粘性会话应绑定到新账号")
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

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrNoAvailableAccounts)
REDACTED)

	t.Run("过滤不可调度账号-限流账号被跳过", func(t *testing.T) {
		now := time.Now()
		resetAt := now.Add(10 * time.Minute)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, RateLimitResetAt: &resetAtREDACTED,
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

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "应跳过限流账号，选择可用账号")
REDACTED)

	t.Run("过滤不可调度账号-过载账号被跳过", func(t *testing.T) {
		now := time.Now()
		overloadUntil := now.Add(10 * time.Minute)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, OverloadUntil: &overloadUntilREDACTED,
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

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID, "应跳过过载账号，选择可用账号")
REDACTED)

	t.Run("粘性账号槽位满-返回粘性等待计划", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{"sticky": 1REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true
		cfg.Gateway.Scheduling.StickySessionMaxWaiting = 1

		concurrencyCache := &mockConcurrencyCache{
			acquireResults: map[int64]bool{1: falseREDACTED,
			waitCounts:     map[int64]int{1: 0REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "sticky", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.WaitPlan)
		require.Equal(t, int64(1), result.Account.ID)
		require.Equal(t, 0, concurrencyCache.loadBatchCalls)
REDACTED)

	t.Run("负载批量查询失败-降级旧顺序选择", func(t *testing.T) {
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

		concurrencyCache := &mockConcurrencyCache{
			loadBatchErr: errors.New("load batch failed"),
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "legacy", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID)
		require.Equal(t, int64(2), cache.sessionBindings["legacy"])
REDACTED)

	t.Run("模型路由-粘性账号等待计划", func(t *testing.T) {
		groupID := int64(20)
		sessionHash := "route-sticky"

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

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{sessionHash: 1REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true
		cfg.Gateway.Scheduling.StickySessionMaxWaiting = 1

		concurrencyCache := &mockConcurrencyCache{
			acquireResults: map[int64]bool{1: falseREDACTED,
			waitCounts:     map[int64]int{1: 0REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.WaitPlan)
		require.Equal(t, int64(1), result.Account.ID)
REDACTED)

	t.Run("模型路由-粘性账号命中", func(t *testing.T) {
		groupID := int64(20)
		sessionHash := "route-hit"

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

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{sessionHash: 1REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(1), result.Account.ID)
		require.Equal(t, 0, concurrencyCache.loadBatchCalls)
REDACTED)

	t.Run("模型路由-粘性账号缺失-清理并回退", func(t *testing.T) {
		groupID := int64(22)
		sessionHash := "route-missing"

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{
			sessionBindings: map[string]int64{sessionHash: 1REDACTED,
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, sessionHash, "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID)
		require.Equal(t, 1, cache.deletedSessions[sessionHash])
		require.Equal(t, int64(2), cache.sessionBindings[sessionHash])
REDACTED)

	t.Run("模型路由-按负载选择账号", func(t *testing.T) {
		groupID := int64(21)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1, LoadRate: 80REDACTED,
				2: {AccountID: 2, LoadRate: 20REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "route", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID)
		require.Equal(t, int64(2), cache.sessionBindings["route"])
REDACTED)

	t.Run("模型路由-路由账号全满返回等待计划", func(t *testing.T) {
		groupID := int64(23)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{
			acquireResults: map[int64]bool{1: false, 2: falseREDACTED,
			loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1, LoadRate: 10REDACTED,
				2: {AccountID: 2, LoadRate: 20REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "route-full", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.WaitPlan)
		require.Equal(t, int64(1), result.Account.ID)
REDACTED)

	t.Run("模型路由-路由账号全满-回退普通选择", func(t *testing.T) {
		groupID := int64(22)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 2, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 3, Platform: PlatformAnthropic, Priority: 0, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1, LoadRate: 100REDACTED,
				2: {AccountID: 2, LoadRate: 100REDACTED,
				3: {AccountID: 3, LoadRate: 0REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "fallback", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(3), result.Account.ID)
		require.Equal(t, int64(3), cache.sessionBindings["fallback"])
REDACTED)

	t.Run("负载批量失败且无法获取-兜底等待", func(t *testing.T) {
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
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{
			loadBatchErr:   errors.New("load batch failed"),
			acquireResults: map[int64]bool{1: false, 2: falseREDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.WaitPlan)
		require.Equal(t, int64(1), result.Account.ID)
REDACTED)

	t.Run("Gemini负载排序-优先OAuth", func(t *testing.T) {
		groupID := int64(24)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, Type: AccountTypeAPIKeyREDACTED,
				{ID: 2, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5, Type: AccountTypeOAuthREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:       groupID,
			REDACTED
					Status:   StatusActive,
					Hydrated: true,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1, LoadRate: 10REDACTED,
				2: {AccountID: 2, LoadRate: 10REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "gemini", "gemini-2.5-pro", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID)
REDACTED)

	t.Run("模型路由-过滤路径覆盖", func(t *testing.T) {
		groupID := int64(70)
		now := time.Now().Add(10 * time.Minute)
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{ID: 3, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: false, Concurrency: 5REDACTED,
				{ID: 4, Platform: PlatformAntigravity, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
				{
					ID:          5,
					Platform:    PlatformAnthropic,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 5,
					Extra: map[string]any{
						"model_rate_limits": map[string]any{
							"claude-3-5-sonnet-20241022": map[string]any{
								"rate_limit_reset_at": now.Format(time.RFC3339),
						REDACTED,
					REDACTED,
				REDACTED,
			REDACTED,
				{
					ID:          6,
					Platform:    PlatformAnthropic,
					Priority:    1,
					Status:      StatusActive,
					Schedulable: true,
					Concurrency: 5,
			REDACTED"model_mapping": map[string]any{"claude-3-5-haiku-20241022": "claude-3-5-haiku-20241022"REDACTEDREDACTED,
			REDACTED,
				{ID: 7, Platform: PlatformAnthropic, Priority: 2, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		cache := &mockGatewayCacheForPlatform{REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:                  groupID,
					Platform:            PlatformAnthropic,
					Status:              StatusActive,
					Hydrated:            true,
					ModelRoutingEnabled: true,
					ModelRouting: map[string][]int64{
						"claude-3-5-sonnet-20241022": {1, 2, 3, 4, 5, 6REDACTED,
				REDACTED,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		excluded := map[int64]struct{REDACTED{1: {REDACTEDREDACTED
		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "", "claude-3-5-sonnet-20241022", excluded, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(7), result.Account.ID)
REDACTED)

	t.Run("ClaudeCode限制-回退分组", func(t *testing.T) {
		groupID := int64(60)
		fallbackID := int64(61)

		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformGemini, Priority: 1, Status: StatusActive, Schedulable: trueREDACTED,
		REDACTED,
			accountsByID: map[int64]*Account{REDACTED,
	REDACTED
		for i := range repo.accounts {
			repo.accountsByID[repo.accounts[i].ID] = &repo.accounts[i]
	REDACTED

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:             groupID,
					Platform:       PlatformAnthropic,
					Status:         StatusActive,
					Hydrated:       true,
					ClaudeCodeOnly: true,
					FallbackGroupID: func() *int64 {
						v := fallbackID
						return &v
				REDACTED(),
			REDACTED,
				fallbackID: {
					ID:       fallbackID,
			REDACTED
					Status:   StatusActive,
					Hydrated: true,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = false

		svc := &GatewayService{
			accountRepo:        repo,
			groupRepo:          groupRepo,
			cache:              &mockGatewayCacheForPlatform{REDACTED,
			cfg:                cfg,
			concurrencyService: nil,
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "", "gemini-2.5-pro", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(1), result.Account.ID)
REDACTED)

	t.Run("ClaudeCode限制-无降级返回错误", func(t *testing.T) {
		groupID := int64(62)

		groupRepo := &mockGroupRepoForGateway{
			groups: map[int64]*Group{
				groupID: {
					ID:             groupID,
					Platform:       PlatformAnthropic,
					Status:         StatusActive,
					Hydrated:       true,
					ClaudeCodeOnly: true,
			REDACTED,
		REDACTED,
	REDACTED

		cfg := testConfig()
		cfg.Gateway.Scheduling.LoadBatchEnabled = false

		svc := &GatewayService{
			accountRepo:        &mockAccountRepoForPlatform{REDACTED,
			groupRepo:          groupRepo,
			cache:              &mockGatewayCacheForPlatform{REDACTED,
			cfg:                cfg,
			concurrencyService: nil,
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, &groupID, "", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.Nil(t, result)
		require.ErrorIs(t, err, ErrClaudeCodeOnly)
REDACTED)

	t.Run("负载可用但无法获取槽位-兜底等待", func(t *testing.T) {
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
		cfg.Gateway.Scheduling.LoadBatchEnabled = true

		concurrencyCache := &mockConcurrencyCache{
			acquireResults: map[int64]bool{1: false, 2: falseREDACTED,
			loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1, LoadRate: 10REDACTED,
				2: {AccountID: 2, LoadRate: 20REDACTED,
		REDACTED,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "wait", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.WaitPlan)
		require.Equal(t, int64(1), result.Account.ID)
REDACTED)

	t.Run("负载信息缺失-使用默认负载", func(t *testing.T) {
		repo := &mockAccountRepoForPlatform{
			accounts: []Account{
				{ID: 1, Platform: PlatformAnthropic, Priority: 1, Status: StatusActive, Schedulable: true, Concurrency: 5REDACTED,
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

		concurrencyCache := &mockConcurrencyCache{
			loadMap: map[int64]*AccountLoadInfo{
				1: {AccountID: 1, LoadRate: 50REDACTED,
		REDACTED,
			skipDefaultLoad: true,
	REDACTED

		svc := &GatewayService{
			accountRepo:        repo,
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(concurrencyCache),
	REDACTED

		result, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "missing-load", "claude-3-5-sonnet-20241022", nil, "", int64(0))
	REDACTED
		require.NotNil(t, result)
		require.NotNil(t, result.Account)
		require.Equal(t, int64(2), result.Account.ID)
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
	require.Equal(t, 1, groupRepo.getByIDCalls) // +1 for require_privacy_set check
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
		Hydrated: true,
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
	require.Equal(t, 1, groupRepo.getByIDCalls) // +1 for require_privacy_set check
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
	require.Equal(t, 1, groupRepo.getByIDCalls) // +1 for require_privacy_set check
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
