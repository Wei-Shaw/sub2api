//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// --- repo / fetcher 装配 ---

// quotaModeRepoStub 记录 RunCheck 落库行为（历史行 + MarkChecked）。
type quotaModeRepoStub struct {
	ChannelMonitorRepository
	monitor   *ChannelMonitor
	history   []*ChannelMonitorHistoryRow
	markedIDs []int64
	updated   []*ChannelMonitor
REDACTED

func (r *quotaModeRepoStub) GetByID(_ context.Context, id int64) (*ChannelMonitor, error) {
	if r.monitor == nil || r.monitor.ID != id {
		return nil, ErrChannelMonitorNotFound
REDACTED
	clone := *r.monitor
	return &clone, nil
REDACTED

func (r *quotaModeRepoStub) InsertHistoryBatch(_ context.Context, rows []*ChannelMonitorHistoryRow) error {
	r.history = append(r.history, rows...)
	return nil
REDACTED

func (r *quotaModeRepoStub) MarkChecked(_ context.Context, id int64, _ time.Time) error {
	r.markedIDs = append(r.markedIDs, id)
	return nil
REDACTED

func (r *quotaModeRepoStub) Update(_ context.Context, m *ChannelMonitor) error {
	clone := *m
	r.updated = append(r.updated, &clone)
	return nil
REDACTED

// newQuotaModeService 构造启用 V1 探活的 service（复用 retirement/duplicate 测试的 stub）。
func newQuotaModeService(repo *quotaModeRepoStub) *ChannelMonitorService {
	svc := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{REDACTED)
	svc.SetRuntimeReader(channelMonitorRuntimeStub{rt: ChannelMonitorRuntime{
		Enabled: true,
		Mode:    ChannelMonitorModeV1,
REDACTEDREDACTED)
	return svc
REDACTED

func newQuotaModeFetcher(accounts map[int64]*Account, usage *stubMonitorUsageSource) *ChannelMonitorQuotaFetcher {
	if accounts == nil {
		accounts = make(map[int64]*Account)
REDACTED
	if usage == nil {
		usage = &stubMonitorUsageSource{REDACTED
REDACTED
	return &ChannelMonitorQuotaFetcher{
		usage:    usage,
		accounts: &stubMonitorAccountSource{accounts: accountsREDACTED,
		cache:    make(map[int64]monitorQuotaCacheEntry),
REDACTED
REDACTED

// --- RunCheck 分派 ---

func TestRunCheck_QuotaModeProducesSingleQuotaResult(t *testing.T) {
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              1,
		Name:            "kimi-quota",
		Provider:        MonitorProviderKimi,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       int64Ptr(9),
REDACTEDREDACTED
	svc := newQuotaModeService(repo)
	fetcher := newQuotaModeFetcher(map[int64]*Account{
		9: {ID: 9, Platform: domain.PlatformKimi, Credentials: map[string]any{"account_mode": AccountModeCodingREDACTEDREDACTED,
REDACTED, nil)
	fetcher.cnQuota = &stubMonitorCNQuotaSource{result: &CNProviderQuotaProbeResult{
		Success:         true,
		CredentialValid: true,
		Tiers:           []CNQuotaTier{{Window: "5h", UsedPercent: 30REDACTEDREDACTED,
REDACTEDREDACTED
	svc.SetQuotaFetcher(fetcher)

	results, err := svc.RunCheck(context.Background(), 1)
REDACTED
	require.Len(t, results, 1)

	res := results[0]
	require.Equal(t, "quota", res.Model)
	require.Equal(t, MonitorStatusOperational, res.Status)
	require.Nil(t, res.LatencyMs)
	require.Nil(t, res.PingLatencyMs)
	require.NotNil(t, res.Quota)
	require.True(t, res.Quota.Success)
	require.Equal(t, "cn_quota", res.Quota.Source)

	// 历史行携带配额快照，并推进 last_checked_at。
	require.Len(t, repo.history, 1)
	require.Equal(t, "quota", repo.history[0].Model)
	require.NotNil(t, repo.history[0].Quota)
	require.Equal(t, []int64{1REDACTED, repo.markedIDs)
REDACTED

func TestRunCheck_QuotaModeUnlinkedAccountDegrades(t *testing.T) {
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              2,
		Provider:        MonitorProviderDeepseek,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        "",
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       nil, // FK ON DELETE SET NULL 后的形态
REDACTEDREDACTED
	svc := newQuotaModeService(repo)
	svc.SetQuotaFetcher(newQuotaModeFetcher(nil, nil))

	results, err := svc.RunCheck(context.Background(), 2)
REDACTED
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusDegraded, results[0].Status)
	require.Contains(t, results[0].Message, "linked account not found")
	require.False(t, results[0].Quota.Success)
REDACTED

func TestRunCheck_QuotaModeNilFetcherFailsClosed(t *testing.T) {
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              3,
		Provider:        MonitorProviderZhipu,
		APIMode:         MonitorAPIModeChatCompletions,
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       int64Ptr(5),
REDACTEDREDACTED
	svc := newQuotaModeService(repo) // 不注入 fetcher

	results, err := svc.RunCheck(context.Background(), 3)
REDACTED
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusError, results[0].Status)
	require.Contains(t, results[0].Message, "not configured")
REDACTED

func TestRunCheck_QuotaProbeAttachesSnapshotToPrimaryRowOnly(t *testing.T) {
	h := &openAICaptureHandler{REDACTED
	endpoint := setupFakeOpenAI(t, h)
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              4,
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        endpoint,
		APIKey:          "OLD:sk-openai",
		PrimaryModel:    "gpt-test",
		ExtraModels:     []string{"gpt-extra"REDACTED,
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuotaProbe,
		AccountID:       int64Ptr(12),
REDACTEDREDACTED
	svc := newQuotaModeService(repo)
	usage := &stubMonitorUsageSource{usage: &UsageInfo{
		FiveHour: &UsageProgress{Utilization: 20REDACTED,
REDACTEDREDACTED
	svc.SetQuotaFetcher(newQuotaModeFetcher(map[int64]*Account{
		12: {ID: 12, Platform: domain.PlatformOpenAIREDACTED,
REDACTED, usage))

	results, err := svc.RunCheck(context.Background(), 4)
REDACTED
	require.Len(t, results, 2)

	// 探活状态为准，配额只挂主模型行。
	require.Equal(t, MonitorStatusOperational, results[0].Status)
	require.NotNil(t, results[0].Quota)
	require.True(t, results[0].Quota.Success)
	require.Equal(t, "usage", results[0].Quota.Source)
	require.Nil(t, results[1].Quota, "extra model rows must not carry quota")

	// 历史落库时同样只有主模型行带快照。
	require.Len(t, repo.history, 2)
	require.NotNil(t, repo.history[0].Quota)
	require.Equal(t, "gpt-test", repo.history[0].Model)
	require.Nil(t, repo.history[1].Quota)
REDACTED

func TestRunCheck_QuotaProbeQuotaFailureKeepsProbeStatus(t *testing.T) {
	h := &openAICaptureHandler{REDACTED
	endpoint := setupFakeOpenAI(t, h)
	repo := &quotaModeRepoStub{monitor: &ChannelMonitor{
		ID:              5,
		Provider:        MonitorProviderOpenAI,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        endpoint,
		APIKey:          "OLD:sk-openai",
		PrimaryModel:    "gpt-test",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuotaProbe,
		AccountID:       nil, // 配额侧失效
REDACTEDREDACTED
	svc := newQuotaModeService(repo)
	svc.SetQuotaFetcher(newQuotaModeFetcher(nil, nil))

	results, err := svc.RunCheck(context.Background(), 5)
REDACTED
	require.Len(t, results, 1)
	require.Equal(t, MonitorStatusOperational, results[0].Status, "quota failure must not flip probe status")
	require.False(t, results[0].Quota.Success)
REDACTED

// --- attachQuotaSnapshot 细节 ---

func TestAttachQuotaSnapshot_NoteOnlyWhenProbeMessageEmpty(t *testing.T) {
	results := []*CheckResult{
		{Model: "primary", Status: MonitorStatusOperational, Message: "challenge passed"REDACTED,
		{Model: "extra"REDACTED,
REDACTED
	failed := &domain.MonitorQuotaSnapshot{Success: false, Error: "boom"REDACTED

	attachQuotaSnapshot(results, failed)

	require.Equal(t, "challenge passed", results[0].Message, "existing probe message wins")
	require.Equal(t, failed, results[0].Quota)
	require.Nil(t, results[1].Quota)

	quiet := []*CheckResult{{Model: "primary", Status: MonitorStatusOperationalREDACTEDREDACTED
	attachQuotaSnapshot(quiet, failed)
	require.Contains(t, quiet[0].Message, "quota fetch failed: boom")

	attachQuotaSnapshot(nil, failed)  // 空结果不 panic
	attachQuotaSnapshot(results, nil) // 空快照不动结果
REDACTED

// --- 校验矩阵 ---

func TestValidateCreateParams_CheckModeMatrix(t *testing.T) {
	accountID := int64(9)

	cases := []struct {
		name    string
		params  ChannelMonitorCreateParams
		wantErr error
REDACTED{
		{
			name: "probe requires endpoint",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeProbe,
				APIKey: "sk", IntervalSeconds: 60, PrimaryModel: "gpt-5",
		REDACTED,
			wantErr: ErrChannelMonitorInvalidEndpoint,
	REDACTED,
		{
			name: "probe requires api key",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderOpenAI, CheckMode: MonitorCheckModeProbe,
				Endpoint: "https://api.openai.com", IntervalSeconds: 60, PrimaryModel: "gpt-5",
		REDACTED,
			wantErr: ErrChannelMonitorMissingAPIKey,
	REDACTED,
		{
			name: "quota drops endpoint and api key requirements",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderAntigravity, CheckMode: MonitorCheckModeQuota,
				IntervalSeconds: 60, AccountID: &accountID,
		REDACTED,
			wantErr: nil, // primary_model 默认 "quota"
	REDACTED,
		{
			name: "quota requires account",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota,
				IntervalSeconds: 60, PrimaryModel: "quota",
		REDACTED,
			wantErr: ErrChannelMonitorAccountRequired,
	REDACTED,
		{
			name: "quota_probe requires endpoint and api key too",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuotaProbe,
				IntervalSeconds: 60, AccountID: &accountID, PrimaryModel: "kimi-k2",
		REDACTED,
			wantErr: ErrChannelMonitorInvalidEndpoint,
	REDACTED,
		{
			name: "antigravity probe unsupported",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderAntigravity, CheckMode: MonitorCheckModeProbe,
				Endpoint: "https://example.com", APIKey: "k",
				IntervalSeconds: 60, AccountID: &accountID, PrimaryModel: "gemini-3-pro",
		REDACTED,
			wantErr: ErrChannelMonitorInvalidCheckMode,
	REDACTED,
		{
			name: "antigravity quota_probe unsupported",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderAntigravity, CheckMode: MonitorCheckModeQuotaProbe,
				Endpoint: "https://example.com", APIKey: "k",
				IntervalSeconds: 60, AccountID: &accountID, PrimaryModel: "gemini-3-pro",
		REDACTED,
			wantErr: ErrChannelMonitorInvalidCheckMode,
	REDACTED,
		{
			name: "unknown mode rejected",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderOpenAI, CheckMode: "auto",
				Endpoint: "https://api.openai.com", APIKey: "sk",
				IntervalSeconds: 60, PrimaryModel: "gpt-5",
		REDACTED,
			wantErr: ErrChannelMonitorInvalidCheckMode,
	REDACTED,
		{
			// quota_probe 仍要打真实探活请求：空模型必须报错，不再用 "quota" 占位。
			name: "quota_probe requires primary model",
			params: ChannelMonitorCreateParams{
				Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuotaProbe,
				Endpoint: "https://api.kimi.com", APIKey: "sk",
				IntervalSeconds: 60, AccountID: &accountID,
		REDACTED,
			wantErr: ErrChannelMonitorMissingPrimaryModel,
	REDACTED,
REDACTED

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreateParams(tc.params)
			if tc.wantErr == nil {
			REDACTED
		REDACTED else {
				require.ErrorIs(t, err, tc.wantErr)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNormalizeMonitorPrimaryModel_QuotaDefault(t *testing.T) {
	require.Equal(t, "quota", normalizeMonitorPrimaryModel(MonitorProviderKimi, MonitorCheckModeQuota, ""))
	require.Equal(t, "quota", normalizeMonitorPrimaryModel(MonitorProviderAntigravity, MonitorCheckModeQuota, "  "))
	// quota_probe 仍要打真实探活请求：空模型返回 ""（由上层报 MissingPrimaryModel），
	// 不再用 "quota" 占位打 model="quota" 的请求。
	require.Equal(t, "", normalizeMonitorPrimaryModel(MonitorProviderKimi, MonitorCheckModeQuotaProbe, ""))
	// grok 分支在纯 quota 占位之后：grok+quota 占位 "quota"，
	// grok 探活（probe/quota_probe）默认轻量测活模型。
	require.Equal(t, "quota", normalizeMonitorPrimaryModel(MonitorProviderGrok, MonitorCheckModeQuota, ""))
	require.Equal(t, MonitorDefaultGrokModel, normalizeMonitorPrimaryModel(MonitorProviderGrok, MonitorCheckModeProbe, ""))
	require.Equal(t, MonitorDefaultGrokModel, normalizeMonitorPrimaryModel(MonitorProviderGrok, MonitorCheckModeQuotaProbe, ""))
	// 探活模式沿用原语义：其余必填（空串报错在 validateCreateParams）。
	require.Equal(t, "kimi-k2", normalizeMonitorPrimaryModel(MonitorProviderKimi, MonitorCheckModeQuotaProbe, "kimi-k2"))
REDACTED

func TestProviderProbeCapabilityMatrix(t *testing.T) {
	require.False(t, providerSupportsProbe(MonitorProviderAntigravity))
	for _, p := range []string{
		MonitorProviderOpenAI, MonitorProviderAnthropic, MonitorProviderGemini,
		MonitorProviderGrok, MonitorProviderKimi, MonitorProviderZhipu, MonitorProviderDeepseek,
REDACTED {
		require.True(t, providerSupportsProbe(p), p)
REDACTED
	for _, p := range []string{
		MonitorProviderOpenAI, MonitorProviderAnthropic, MonitorProviderGemini,
		MonitorProviderGrok, MonitorProviderAntigravity,
		MonitorProviderKimi, MonitorProviderZhipu, MonitorProviderDeepseek,
REDACTED {
		require.NoError(t, validateProvider(p), p)
REDACTED
REDACTED

// --- 关联账号校验 ---

func TestValidateLinkedAccount_Matrix(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	fetcher := newQuotaModeFetcher(map[int64]*Account{
		1: {ID: 1, Platform: domain.PlatformKimiREDACTED,
REDACTED, nil)
	svc.SetQuotaFetcher(fetcher)

	require.NoError(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, nil))
	require.NoError(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(0)))
	require.NoError(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(1)))
	require.ErrorIs(t, svc.validateLinkedAccount(context.Background(), MonitorProviderZhipu, int64Ptr(1)), ErrChannelMonitorProviderIncompatible)
	require.ErrorIs(t, svc.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(404)), ErrChannelMonitorAccountRequired)

	noFetcher := NewChannelMonitorService(nil, nil)
	require.ErrorIs(t, noFetcher.validateLinkedAccount(context.Background(), MonitorProviderKimi, int64Ptr(1)), ErrChannelMonitorAccountRequired)
REDACTED

func TestRevalidateLinkedAccount_QuotaErrorsProbeUnbinds(t *testing.T) {
	fetcher := newQuotaModeFetcher(nil, nil) // 账号一律加载失败
	svc := NewChannelMonitorService(nil, nil)
	svc.SetQuotaFetcher(fetcher)

	quota := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota, AccountID: int64Ptr(9)REDACTED
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quota), ErrChannelMonitorAccountRequired)
	require.NotNil(t, quota.AccountID)

	probe := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeProbe, AccountID: int64Ptr(9)REDACTED
	require.NoError(t, svc.revalidateLinkedAccount(context.Background(), probe))
	require.Nil(t, probe.AccountID, "probe mode should silently unbind stale account")

	quotaNoAccount := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuotaREDACTED
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quotaNoAccount), ErrChannelMonitorAccountRequired)
REDACTED

func TestRevalidateLinkedAccount_PlatformMismatch(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetQuotaFetcher(newQuotaModeFetcher(map[int64]*Account{
		2: {ID: 2, Platform: domain.PlatformDeepseekREDACTED,
REDACTED, nil))

	quota := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota, AccountID: int64Ptr(2)REDACTED
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quota), ErrChannelMonitorProviderIncompatible)

	probe := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeProbe, AccountID: int64Ptr(2)REDACTED
	require.NoError(t, svc.revalidateLinkedAccount(context.Background(), probe))
	require.Nil(t, probe.AccountID)
REDACTED

// 能力矩阵：与 fetchUncached 路由一一对应，创建期拦截注定运行期永久 error 的组合。
func TestMonitorAccountQuotaCapability_Matrix(t *testing.T) {
	cases := []struct {
		name    string
		account *Account
		wantErr error
REDACTED{
		{
			name:    "deepseek coding has no quota endpoint",
			account: &Account{ID: 1, Platform: domain.PlatformDeepseek, Credentials: map[string]any{"account_mode": AccountModeCodingREDACTEDREDACTED,
			wantErr: ErrChannelMonitorAccountNotSupportable,
	REDACTED,
		{
			// 自定义域名 kimi coding：GetCodingPlanProvider 识别不到 → 无额度端点。
			name: "custom-domain kimi coding unsupported",
			account: &Account{ID: 2, Platform: domain.PlatformKimi, Type: AccountTypeAPIKey,
		REDACTED"account_mode": AccountModeCoding, "base_url": "https://cw.example.com"REDACTEDREDACTED,
			wantErr: ErrChannelMonitorAccountNotSupportable,
	REDACTED,
		{
			name:    "kimi coding default endpoint ok",
			account: &Account{ID: 3, Platform: domain.PlatformKimi, Credentials: map[string]any{"account_mode": AccountModeCodingREDACTEDREDACTED,
	REDACTED,
		{
			name:    "zhipu coding default endpoint ok",
			account: &Account{ID: 4, Platform: domain.PlatformZhipu, Credentials: map[string]any{"account_mode": AccountModeCodingREDACTEDREDACTED,
	REDACTED,
		{
			name:    "zhipu payg has no balance endpoint",
			account: &Account{ID: 5, Platform: domain.PlatformZhipuREDACTED,
			wantErr: ErrChannelMonitorAccountNotSupportable,
	REDACTED,
		{
			name:    "kimi payg ok",
			account: &Account{ID: 6, Platform: domain.PlatformKimiREDACTED,
	REDACTED,
		{
			name:    "deepseek payg ok",
			account: &Account{ID: 7, Platform: domain.PlatformDeepseekREDACTED,
	REDACTED,
		{
			name:    "anthropic api key cannot query usage",
			account: &Account{ID: 8, Platform: domain.PlatformAnthropic, Type: AccountTypeAPIKeyREDACTED,
			wantErr: ErrChannelMonitorAccountNotSupportable,
	REDACTED,
		{
			name:    "anthropic oauth ok",
			account: &Account{ID: 9, Platform: domain.PlatformAnthropic, Type: AccountTypeOAuthREDACTED,
	REDACTED,
		{
			name:    "anthropic setup token ok (local estimation)",
			account: &Account{ID: 10, Platform: domain.PlatformAnthropic, Type: AccountTypeSetupTokenREDACTED,
	REDACTED,
		{
			name:    "openai api key cannot query usage",
			account: &Account{ID: 11, Platform: domain.PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED,
			wantErr: ErrChannelMonitorAccountNotSupportable,
	REDACTED,
		{
			name:    "openai oauth ok",
			account: &Account{ID: 12, Platform: domain.PlatformOpenAI, Type: AccountTypeOAuthREDACTED,
	REDACTED,
		{
			// 防过度拦截：gemini/grok/antigravity 走本地统计/值通道降级，不会永久 error。
			name:    "gemini api key ok",
			account: &Account{ID: 13, Platform: domain.PlatformGemini, Type: AccountTypeAPIKeyREDACTED,
	REDACTED,
		{
			name:    "grok ok",
			account: &Account{ID: 14, Platform: domain.PlatformGrokREDACTED,
	REDACTED,
		{
			name:    "antigravity ok",
			account: &Account{ID: 15, Platform: domain.PlatformAntigravityREDACTED,
	REDACTED,
REDACTED
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := monitorAccountQuotaCapability(tc.account)
			if tc.wantErr == nil {
			REDACTED
		REDACTED else {
				require.ErrorIs(t, err, tc.wantErr)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestValidateLinkedAccount_CapabilityRejected(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetQuotaFetcher(newQuotaModeFetcher(map[int64]*Account{
		1: {ID: 1, Platform: domain.PlatformDeepseek, Credentials: map[string]any{"account_mode": AccountModeCodingREDACTEDREDACTED,
REDACTED, nil))

	err := svc.validateLinkedAccount(context.Background(), MonitorProviderDeepseek, int64Ptr(1))
	require.ErrorIs(t, err, ErrChannelMonitorAccountNotSupportable)
REDACTED

func TestRevalidateLinkedAccount_Capability(t *testing.T) {
	svc := NewChannelMonitorService(nil, nil)
	svc.SetQuotaFetcher(newQuotaModeFetcher(map[int64]*Account{
		2: {ID: 2, Platform: domain.PlatformDeepseek, Credentials: map[string]any{"account_mode": AccountModeCodingREDACTEDREDACTED,
REDACTED, nil))

	quota := &ChannelMonitor{Provider: MonitorProviderDeepseek, CheckMode: MonitorCheckModeQuota, AccountID: int64Ptr(2)REDACTED
	require.ErrorIs(t, svc.revalidateLinkedAccount(context.Background(), quota), ErrChannelMonitorAccountNotSupportable)
	require.NotNil(t, quota.AccountID, "quota mode keeps the binding for the admin to fix")

	probe := &ChannelMonitor{Provider: MonitorProviderDeepseek, CheckMode: MonitorCheckModeProbe, AccountID: int64Ptr(2)REDACTED
	require.NoError(t, svc.revalidateLinkedAccount(context.Background(), probe))
	require.Nil(t, probe.AccountID, "probe mode should silently unbind unusable account")
REDACTED

// provider-only 更新不得绕过 provider × check_mode 组合校验。
func TestApplyMonitorUpdate_ProviderOnlyRevalidatesCheckMode(t *testing.T) {
	probeKimi := func() *ChannelMonitor {
		return &ChannelMonitor{
			Provider: MonitorProviderKimi, APIMode: MonitorAPIModeChatCompletions,
			Endpoint: "https://api.kimi.com", PrimaryModel: "kimi-k2",
			CheckMode: MonitorCheckModeProbe,
	REDACTED
REDACTED

	provider := MonitorProviderAntigravity
	err := applyMonitorUpdate(probeKimi(), ChannelMonitorUpdateParams{Provider: &providerREDACTED)
	require.ErrorIs(t, err, ErrChannelMonitorInvalidCheckMode)

	// 带上 check_mode/account_id 的完整切换合法。
	accountID := int64(3)
	err = applyMonitorUpdate(probeKimi(), ChannelMonitorUpdateParams{
		Provider: &provider, CheckMode: strPtr(MonitorCheckModeQuota), AccountID: &accountID,
REDACTED)
REDACTED

	// 存量非法行（antigravity+probe）仅改名/停用不被砖化。
	legacy := &ChannelMonitor{
		Provider: MonitorProviderAntigravity, APIMode: MonitorAPIModeChatCompletions,
		Endpoint: "https://example.com", PrimaryModel: "gemini-3-pro",
		CheckMode: MonitorCheckModeProbe,
REDACTED
	newName := "renamed"
	require.NoError(t, applyMonitorUpdate(legacy, ChannelMonitorUpdateParams{Name: &newNameREDACTED))
REDACTED

// --- quota → probe 切换的 key 管控（validateProbeAPIKey） ---

func TestValidateProbeAPIKey_QuotaToProbeRequiresFreshKey(t *testing.T) {
	svc := NewChannelMonitorService(nil, &duplicateChannelMonitorEncryptor{REDACTED)

	quota := &ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeQuota, APIKey: "NEW:"REDACTED
	require.NoError(t, svc.validateProbeAPIKey(quota, "")) // quota 模式不管 key

	quota.CheckMode = MonitorCheckModeProbe
	// 存量密文解出空明文（quota 监控存的加密空串）→ 必须重填 key。
	require.ErrorIs(t, svc.validateProbeAPIKey(quota, ""), ErrChannelMonitorMissingAPIKey)
	// 提供新明文 key → 放行。
	require.NoError(t, svc.validateProbeAPIKey(quota, "sk-fresh"))
	// 密文解出非空明文 → 放行。
	require.NoError(t, svc.validateProbeAPIKey(
		&ChannelMonitor{Provider: MonitorProviderKimi, CheckMode: MonitorCheckModeProbe, APIKey: "OLD:sk-live"REDACTED, ""))
REDACTED

// --- Duplicate：quota 模式空明文重加密 ---

func TestDuplicateChannelMonitorQuotaModeReencryptsEmptyKey(t *testing.T) {
	accountID := int64(9)
	source := &ChannelMonitor{
		ID:              42,
		Name:            "kimi-quota",
		Provider:        MonitorProviderKimi,
		APIMode:         MonitorAPIModeChatCompletions,
		Endpoint:        "",
		APIKey:          "OLD:", // 解密为空串（quota 监控的加密空 key）
		PrimaryModel:    "quota",
		Enabled:         true,
		IntervalSeconds: 60,
		CheckMode:       MonitorCheckModeQuota,
		AccountID:       &accountID,
REDACTED
	repo := &duplicateChannelMonitorRepoStub{source: sourceREDACTED
	service := NewChannelMonitorService(repo, &duplicateChannelMonitorEncryptor{REDACTED)

	dup, err := service.Duplicate(context.Background(), 42, 7, "admin:7", "op-1")
REDACTED
	require.Equal(t, MonitorCheckModeQuota, dup.CheckMode)
	require.NotNil(t, dup.AccountID)
	require.Equal(t, accountID, *dup.AccountID)
	require.Empty(t, dup.APIKey, "plaintext stays empty for quota monitors")
	require.Len(t, repo.created, 1)
	require.Equal(t, "NEW:", repo.created[0].APIKey, "empty key must be re-encrypted, not dropped")
REDACTED
