package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"
)

// 国产供应商 Coding Plan 滚动窗口额度探测服务（Kimi For Coding / 智谱 GLM Coding Plan）。
//
// 与 grok_quota_service 不同：CN 供应商走数据面 API Key（无 OAuth token provider），
// 额度端点为只读 GET，解析 5h + weekly 两档滚动窗口并落 account.Extra 快照，
// 供账号调度阈值评估（account_scheduling_threshold_eval.go）做主动停调。
//
// 解析逻辑对齐 cc-switch（farion1231/cc-switch）services/coding_plan.rs 的
// query_kimi / query_zhipu，包括智谱 unit 字段优先分类与 reset 兜底启发式。
const (
	cnQuotaUpstreamTimeout = 15 * time.Second
	cnQuotaMaxBodyBytes    = 256 * 1024

	// Extra 快照键后缀（加 provider 前缀，如 kimi_5h_used_percent）。
	cnExtraSuffix5hUsed       = "5h_used_percent"
	cnExtraSuffix5hReset      = "5h_reset_at"
	cnExtraSuffixWeeklyUsed   = "weekly_used_percent"
	cnExtraSuffixWeeklyReset  = "weekly_reset_at"
	cnExtraSuffixMonthlyUsed  = "monthly_used_percent"
	cnExtraSuffixMonthlyReset = "monthly_reset_at"
	cnExtraSuffixUsageUpdated = "usage_updated_at"
)

var ErrCNQuotaIdentityChanged = infraerrors.Conflict(
	"CN_QUOTA_IDENTITY_CHANGED",
	"Step Plan account identity changed or a newer usage attempt was already persisted; reload and retry",
)

// stepPlanUsageCASRepository is implemented by the production account
// repository. Keeping it as a narrow capability avoids widening the shared
// AccountRepository interface and makes every StepPlan runtime write go through
// one identity-checked SQL statement.
type stepPlanUsageCASRepository interface {
	NextStepPlanUsageAttemptSeq(ctx context.Context) (int64, error)
	UpdateStepPlanExtraIfIdentityUnchanged(
		ctx context.Context,
		accountID int64,
		expectedAPIKey string,
		expectedBaseURL string,
		updates map[string]any,
		attemptSeq int64,
	) error
}

// cnExtraKey 拼接 provider 维度的 extra 键。
func cnExtraKey(provider, suffix string) string { return provider + "_" + suffix }

// CNQuotaTier 表示一个滚动用量窗口档位（5h / weekly）。
type CNQuotaTier struct {
	Window      string  `json:"window"`             // "5h" | "weekly" | "monthly"
	UsedPercent float64 `json:"used_percent"`       // 已用百分比（0-100+，不做裁剪）
	ResetAt     string  `json:"reset_at,omitempty"` // RFC3339，空表示无重置时间
}

// CNProviderQuotaProbeResult 是 Coding Plan 额度探测的返回结构（管理端 + UI 消费）。
type CNProviderQuotaProbeResult struct {
	Provider        string        `json:"provider"`
	Source          string        `json:"source"`
	Success         bool          `json:"success"`
	CredentialValid bool          `json:"credential_valid"` // false = 401/403 鉴权失败
	Tiers           []CNQuotaTier `json:"tiers,omitempty"`
	PlanLevel       string        `json:"plan_level,omitempty"` // 智谱套餐等级
	StatusCode      int           `json:"status_code,omitempty"`
	FetchedAt       int64         `json:"fetched_at"`
	Persisted       bool          `json:"persisted"`
	Error           string        `json:"error,omitempty"`

	// Balance 为同一次探测得到的余额（Command Code 积分与窗口同源），其余供应商为空。
	Balance *CNProviderBalanceResult `json:"balance,omitempty"`
}

// CNProviderQuotaService 探测 Kimi / Zhipu / MiniMax / StepFun Coding Plan、
// OpenCode Go、Command Code 与 Cline（ClinePass）的滚动窗口用量。
type CNProviderQuotaService struct {
	accountRepo  AccountRepository
	proxyRepo    ProxyRepository
	httpUpstream HTTPUpstream
	cfg          *config.Config
	flight       singleflight.Group
}

// NewCNProviderQuotaService 构造 Coding Plan 额度探测服务。
func NewCNProviderQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *CNProviderQuotaService {
	return &CNProviderQuotaService{
		accountRepo:  accountRepo,
		proxyRepo:    proxyRepo,
		httpUpstream: httpUpstream,
		cfg:          cfg,
	}
}

// QueryUsage 探测指定账号的 Coding Plan 滚动窗口用量并落 Extra 快照。
// 同一账号的并发探测会被 singleflight 合并。
func (s *CNProviderQuotaService) QueryUsage(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error) {
	account, err := s.loadCodingPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return s.QueryUsageForAccount(ctx, account)
}

// QueryUsageForAccount 探测已加载账号（配额监控 fetcher 复用，避免二次 GetByID）。
// singleflight key 与 QueryUsage 相同，按账号 ID 与 admin 侧并发探测合并。
func (s *CNProviderQuotaService) QueryUsageForAccount(ctx context.Context, account *Account) (*CNProviderQuotaProbeResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	if err := validateCodingPlanAccount(account); err != nil {
		return nil, err
	}
	key := "cn_quota:" + strconv.FormatInt(account.ID, 10)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		probeCtx, cancel := context.WithTimeout(context.Background(), cnQuotaUpstreamTimeout+5*time.Second)
		defer cancel()
		return s.queryUsageForAccount(probeCtx, account)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		result, ok := flightResult.Val.(*CNProviderQuotaProbeResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_PROBE_RESULT_INVALID", "invalid cn provider quota probe result")
		}
		cloned := *result
		return &cloned, nil
	}
}

func (s *CNProviderQuotaService) queryUsageForAccount(ctx context.Context, account *Account) (*CNProviderQuotaProbeResult, error) {
	provider := account.GetCodingPlanProvider()
	if provider == PlatformCommandCode {
		return s.queryCommandCodeUsage(ctx, account)
	}
	if provider == PlatformCline {
		return s.queryClineUsage(ctx, account)
	}
	if provider != PlatformKimi && provider != PlatformZhipu && provider != PlatformMiniMax && provider != PlatformStepfun && provider != PlatformOpenCodeGo {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "account is not a kimi/zhipu/minimax/stepfun coding plan or opencode go account")
	}

	apiKey := strings.TrimSpace(account.GetCNAPIKey())
	if apiKey == "" {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NO_APIKEY", "account api_key is empty")
	}

	baseURL := account.GetOpenAIBaseURL()
	var (
		targetURL  string
		authHeader string
		zhipuOrg   string
	)
	switch provider {
	case PlatformKimi:
		targetURL = kimiQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	case PlatformOpenCodeGo:
		targetURL = openCodeGoQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	case PlatformZhipu:
		targetURL = zhipuQuotaURL(baseURL)
		authHeader = apiKey // 智谱额度端点鉴权不加 Bearer 前缀
		// 团队版 GLM Coding Plan：额度端点需 ?type=2 + 组织/项目请求头，
		// 否则官方 API 回「当前用户不存在coding plan」。组织 ID 存在即视为
		// 团队版（个人版凭据不含该字段，走原个人版查询路径）。
		zhipuOrg = strings.TrimSpace(account.GetCredential("zhipu_organization"))
		if zhipuOrg != "" {
			targetURL += "?type=2"
		}
	case PlatformMiniMax:
		targetURL = minimaxQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	case PlatformStepfun:
		targetURL = stepfunQuotaURL(baseURL)
		authHeader = "Bearer " + apiKey
	}
	attemptSeq := int64(0)
	if provider == PlatformStepfun {
		repo, ok := s.accountRepo.(stepPlanUsageCASRepository)
		if !ok {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_CAS_UNAVAILABLE", "account repository does not support atomic Step Plan updates")
		}
		seq, seqErr := repo.NextStepPlanUsageAttemptSeq(ctx)
		if seqErr != nil {
			return nil, infraerrors.Newf(http.StatusInternalServerError, "CN_QUOTA_ATTEMPT_SEQ_FAILED", "allocate Step Plan usage attempt sequence: %v", seqErr)
		}
		attemptSeq = seq
	}
	now := time.Now().UTC()
	result := &CNProviderQuotaProbeResult{
		Provider:        provider,
		Source:          "coding_plan",
		CredentialValid: true,
		FetchedAt:       now.Unix(),
	}

	// 探测发起前过出站 URL 安全策略（与网关转发/Grok 探测同一套校验）：
	// 端点多由账号 base_url 衍生，不得把 API key 发往策略外主机。
	validatedURL, err := cnValidateProbeURL(s.cfg, targetURL)
	if err != nil {
		result.Error = err.Error()
		if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
			return nil, persistErr
		}
		return nil, infraerrors.New(http.StatusForbidden, "CN_QUOTA_URL_REJECTED", err.Error())
	}
	targetURL = validatedURL

	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("build request: %v", err)
		if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
			return nil, persistErr
		}
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CN_QUOTA_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Accept", "application/json")
	if provider == PlatformZhipu || provider == PlatformMiniMax {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Language", "en-US,en")
		if zhipuOrg != "" {
			req.Header.Set("bigmodel-organization", zhipuOrg)
			if project := strings.TrimSpace(account.GetCredential("zhipu_project")); project != "" {
				req.Header.Set("bigmodel-project", project)
			}
		}
	}
	// 探测与真实转发保持同一套账号级请求头覆写，避免探测通过但转发失败。
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		result.Error = fmt.Sprintf("upstream request failed: %v", err)
		if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
			return nil, persistErr
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "CN_QUOTA_REQUEST_FAILED", "upstream request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes))

	result.StatusCode = resp.StatusCode
	result.CredentialValid = resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// 鉴权失败：保留之前的有效窗口快照，但持久化失败状态供编辑面板展示。
		result.CredentialValid = false
		result.Error = fmt.Sprintf("Authentication failed (HTTP %d)", resp.StatusCode)
		if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
			return nil, persistErr
		}
		return result, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Error = fmt.Sprintf("API error (HTTP %d): %s", resp.StatusCode, truncate(strings.TrimSpace(string(bodyBytes)), 240))
		if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
			return nil, persistErr
		}
		return result, nil
	}

	// 智谱业务级错误（HTTP 2xx 但 success=false）。
	if provider == PlatformZhipu {
		if success := gjson.GetBytes(bodyBytes, "success"); success.Exists() && !success.Bool() {
			msg := strings.TrimSpace(gjson.GetBytes(bodyBytes, "msg").String())
			if msg == "" {
				msg = "unknown zhipu quota error"
			}
			result.Error = "API error: " + msg
			return result, nil
		}
	}

	// Step Plan 的「未开通 / 不可用」由 quota.state / quota.allowed 表达，
	// 判定与 cc-switch 的 usage_script 一致；HTTP 200 说明 Key 本身有效。
	if provider == PlatformStepfun {
		if reason := stepFunQuotaUnavailable(bodyBytes); reason != "" {
			result.Error = reason
			result.CredentialValid = true
			if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
				return nil, persistErr
			}
			return result, nil
		}
	}

	var tiers []CNQuotaTier
	switch provider {
	case PlatformKimi:
		tiers = parseKimiUsageTiers(bodyBytes)
	case PlatformOpenCodeGo:
		tiers = parseOpenCodeGoUsageTiers(bodyBytes)
		result.PlanLevel = "OpenCode Go"
	case PlatformZhipu:
		tiers = parseZhipuTokenTiers(gjson.GetBytes(bodyBytes, "data"))
		result.PlanLevel = strings.TrimSpace(gjson.GetBytes(bodyBytes, "data.level").String())
	case PlatformMiniMax:
		if status := gjson.GetBytes(bodyBytes, "base_resp.status_code"); status.Exists() && status.Int() != 0 {
			msg := strings.TrimSpace(gjson.GetBytes(bodyBytes, "base_resp.status_msg").String())
			if msg == "" {
				msg = "unknown minimax quota error"
			}
			result.Error = fmt.Sprintf("API error (%d): %s", status.Int(), msg)
			return result, nil
		}
		tiers = parseMiniMaxUsageTiers(bodyBytes)
		result.PlanLevel = strings.TrimSpace(gjson.GetBytes(bodyBytes, "current_subscribe_title").String())
	case PlatformStepfun:
		tiers, result.PlanLevel = parseStepFunUsageTiers(bodyBytes)
		if len(tiers) == 0 {
			result.Error = "Step Plan quota response has no usable usage windows"
			result.CredentialValid = true
			if persistErr := s.persistStepFunUsageSnapshot(ctx, account, result, now, attemptSeq); persistErr != nil {
				return nil, persistErr
			}
			return result, nil
		}
	}
	result.Tiers = tiers
	result.Success = true
	result.CredentialValid = true

	if provider == PlatformStepfun {
		updates := stepFunQuotaExtraUpdates(tiers, now)
		updates[StepFunUsageSnapshotExtraKey] = newStepFunUsageSnapshot(result, now)
		updates[StepFunUsageAttemptSeqExtraKey] = attemptSeq
		if err := s.updateStepPlanUsageExtra(ctx, account, updates, attemptSeq); err != nil {
			slog.Warn("cn_quota_persist_failed", "account_id", account.ID, "provider", provider, "error", err)
			if errors.Is(err, ErrCNQuotaIdentityChanged) {
				return nil, err
			}
		} else {
			result.Persisted = true
		}
		return result, nil
	}
	updates := cnQuotaExtraUpdates(provider, tiers, now)
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, updates); err != nil {
		slog.Warn("cn_quota_persist_failed", "account_id", account.ID, "provider", provider, "error", err)
	} else {
		result.Persisted = true
	}
	return result, nil
}

// persistStepFunUsageSnapshot 只为 Step Plan 写失败/不可用状态；成功结果与
// 平铺窗口键合并后一次写入，避免两次 UpdateExtra 产生中间态。
func (s *CNProviderQuotaService) persistStepFunUsageSnapshot(
	ctx context.Context,
	account *Account,
	result *CNProviderQuotaProbeResult,
	now time.Time,
	attemptSeq int64,
) error {
	if account == nil || account.Platform != PlatformStepfun || result == nil {
		return nil
	}
	snapshot := newStepFunUsageSnapshot(result, now)
	if snapshot.Status != StepFunUsageStatusOK && account.Extra != nil {
		previous := decodeStepFunUsageSnapshot(account.Extra)
		if previous == nil {
			previous = stepFunUsageSnapshotFromFlatExtra(account.Extra)
		}
		if previous != nil {
			if len(snapshot.Tiers) == 0 {
				snapshot.Tiers = append([]CNQuotaTier(nil), previous.Tiers...)
			}
			if snapshot.PlanLevel == "" {
				snapshot.PlanLevel = previous.PlanLevel
			}
			if snapshot.FetchedAt == nil && previous.FetchedAt != nil {
				fetchedAt := *previous.FetchedAt
				snapshot.FetchedAt = &fetchedAt
			}
		}
	}
	updates := map[string]any{
		StepFunUsageSnapshotExtraKey:   snapshot,
		StepFunUsageAttemptSeqExtraKey: attemptSeq,
	}
	if err := s.updateStepPlanUsageExtra(ctx, account, updates, attemptSeq); err != nil {
		slog.Warn("cn_quota_persist_failed", "account_id", account.ID, "provider", PlatformStepfun, "error", err)
		if errors.Is(err, ErrCNQuotaIdentityChanged) {
			return err
		}
		return nil
	}
	result.Persisted = true
	return nil
}

func (s *CNProviderQuotaService) updateStepPlanUsageExtra(
	ctx context.Context,
	account *Account,
	updates map[string]any,
	attemptSeq int64,
) error {
	if s == nil || s.accountRepo == nil || account == nil {
		return infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	repo, ok := s.accountRepo.(stepPlanUsageCASRepository)
	if !ok {
		return infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_CAS_UNAVAILABLE", "account repository does not support atomic Step Plan updates")
	}
	return repo.UpdateStepPlanExtraIfIdentityUnchanged(
		ctx,
		account.ID,
		account.GetCNAPIKey(),
		account.GetOpenAIBaseURL(),
		updates,
		attemptSeq,
	)
}

// GetStepPlanUsageState 返回账号最近一次 Step Plan 探测快照，不触发上游请求。
func (s *CNProviderQuotaService) GetStepPlanUsageState(ctx context.Context, accountID int64) (*StepPlanUsageState, error) {
	account, err := s.loadStepPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return stepPlanUsageStateFromAccount(account), nil
}

// SetStepPlanUsageAutoRefresh 持久化账号级自动刷新开关并返回最新状态。
func (s *CNProviderQuotaService) SetStepPlanUsageAutoRefresh(
	ctx context.Context,
	accountID int64,
	enabled bool,
) (*StepPlanUsageState, error) {
	account, err := s.loadStepPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := s.updateStepPlanUsageExtra(ctx, account, map[string]any{
		StepFunUsageAutoRefreshExtraKey: enabled,
	}, 0); err != nil {
		return nil, err
	}
	// Re-read after the conditional write so the response cannot resurrect a
	// snapshot that a periodic probe replaced while this request was in flight.
	latest, err := s.loadStepPlanAccount(ctx, accountID)
	if err != nil {
		// The write already succeeded. Return a flag-only state rather than
		// reporting a false save failure; clients merge the toggle and keep the
		// snapshot they already have.
		return &StepPlanUsageState{
			AccountID:          account.ID,
			Eligible:           true,
			AutoRefreshEnabled: enabled,
		}, nil
	}
	return stepPlanUsageStateFromAccount(latest), nil
}

func (s *CNProviderQuotaService) loadStepPlanAccount(ctx context.Context, accountID int64) (*Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "CN_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if err := validateCodingPlanAccount(account); err != nil {
		return nil, err
	}
	if account.GetCodingPlanProvider() != PlatformStepfun {
		return nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "account is not an official Step Plan account")
	}
	return account, nil
}

func stepPlanUsageStateFromAccount(account *Account) *StepPlanUsageState {
	state := &StepPlanUsageState{
		Eligible:           account != nil && account.GetCodingPlanProvider() == PlatformStepfun,
		AutoRefreshEnabled: stepFunUsageAutoRefreshEnabled(account),
	}
	if account == nil {
		return state
	}
	state.AccountID = account.ID
	state.Snapshot = decodeStepFunUsageSnapshot(account.Extra)
	if state.Snapshot == nil {
		state.Snapshot = stepFunUsageSnapshotFromFlatExtra(account.Extra)
	}
	return state
}

func decodeStepFunUsageSnapshot(extra map[string]any) *StepFunUsageSnapshot {
	if extra == nil {
		return nil
	}
	value, ok := extra[StepFunUsageSnapshotExtraKey]
	if !ok || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var snapshot StepFunUsageSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil
	}
	switch snapshot.Status {
	case StepFunUsageStatusOK, StepFunUsageStatusFailed, StepFunUsageStatusUnavailable:
		return &snapshot
	default:
		return nil
	}
}

// stepFunUsageSnapshotFromFlatExtra 兼容接入结构化快照前仅写入窗口键的账号。
func stepFunUsageSnapshotFromFlatExtra(extra map[string]any) *StepFunUsageSnapshot {
	if extra == nil {
		return nil
	}
	readNumber := func(key string) (float64, bool) {
		return cnParseF64(extra[key])
	}
	readString := func(key string) string {
		value, _ := extra[key].(string)
		return value
	}
	var tiers []CNQuotaTier
	for _, item := range []struct {
		window string
		used   string
		reset  string
	}{
		{window: "5h", used: cnExtraKey(PlatformStepfun, cnExtraSuffix5hUsed), reset: cnExtraKey(PlatformStepfun, cnExtraSuffix5hReset)},
		{window: "weekly", used: cnExtraKey(PlatformStepfun, cnExtraSuffixWeeklyUsed), reset: cnExtraKey(PlatformStepfun, cnExtraSuffixWeeklyReset)},
		{window: "monthly", used: cnExtraKey(PlatformStepfun, cnExtraSuffixMonthlyUsed), reset: cnExtraKey(PlatformStepfun, cnExtraSuffixMonthlyReset)},
	} {
		used, ok := readNumber(item.used)
		if !ok {
			continue
		}
		tiers = append(tiers, CNQuotaTier{Window: item.window, UsedPercent: used, ResetAt: readString(item.reset)})
	}
	if len(tiers) == 0 {
		return nil
	}
	snapshot := &StepFunUsageSnapshot{
		Status:          StepFunUsageStatusOK,
		CredentialValid: true,
		Tiers:           tiers,
	}
	updatedAt := readString(cnExtraKey(PlatformStepfun, cnExtraSuffixUsageUpdated))
	if parsed, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		utc := parsed.UTC()
		snapshot.FetchedAt = &utc
		snapshot.LastAttemptAt = utc
	}
	return snapshot
}

func (s *CNProviderQuotaService) loadCodingPlanAccount(ctx context.Context, accountID int64) (*Account, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusNotFound, "CN_QUOTA_ACCOUNT_NOT_FOUND", "account not found: %v", err)
	}
	if err := validateCodingPlanAccount(account); err != nil {
		return nil, err
	}
	return account, nil
}

// validateCodingPlanAccount 加载后的非 DB 校验（ForAccount 入口同样复用，
// 保证直传 account 也不绕过平台/模式检查）。
func validateCodingPlanAccount(account *Account) error {
	if account == nil {
		return infraerrors.New(http.StatusNotFound, "CN_QUOTA_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.IsOpenCodeGoPlan() {
		return nil
	}
	if account.IsCommandCode() {
		if !account.commandCodeUsageSupported() {
			return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "command code usage is only available for api key accounts on the official host")
		}
		return nil
	}
	if account.IsCline() {
		if !account.clineAccountAPISupported() {
			return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "cline usage is only available for api key accounts on the official host")
		}
		return nil
	}
	if account.IsOpenCodeGo() {
		return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "opencode zen accounts have no subscription quota window")
	}
	if !account.IsCNProvider() {
		return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_INVALID_PLATFORM", "account is not a CN provider account")
	}
	if !account.IsCodingPlan() {
		return infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NOT_CODING_PLAN", "account is not a coding plan account")
	}
	return nil
}

func (s *CNProviderQuotaService) resolveProxyURL(ctx context.Context, account *Account) string {
	if account == nil || account.ProxyID == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if s != nil && s.proxyRepo != nil {
		if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
			account.Proxy = proxy
			return proxy.URL()
		}
	}
	return ""
}

// zhipuQuotaURL 根据 base_url 解析智谱额度端点（与数据面推理域名同主机）。
func zhipuQuotaURL(baseURL string) string {
	return zhipuQuotaHost(baseURL) + "/api/monitor/usage/quota/limit"
}

// kimiQuotaURL 根据 base_url 解析 Kimi For Coding 额度端点。
// cc-switch query_kimi 固定探测 https://api.kimi.com/coding/v1/usages
// （实测 /coding/usages 无 /v1 → 404）。coding/v1（CC 协议默认）与
// coding（Anthropic 协议默认）两种 base 统一剥掉尾部后拼回 /v1/usages，
// 协议切换不影响额度探测端点。
func kimiQuotaURL(baseURL string) string {
	base := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	return base + "/v1/usages"
}

// minimaxQuotaURL 根据推理域名选择 Token Plan / Coding Plan 额度主机。
// 官方 FAQ 写 www.minimax.io / www.minimaxi.com，实际以 Bearer Key 打 api.*。
// 国际站 api.minimax.io；国内站 api.minimaxi.com（含 api.minimax.com 与自定义回落）。
func minimaxQuotaURL(baseURL string) string {
	if strings.Contains(strings.ToLower(baseURL), "minimax.io") {
		return "https://api.minimax.io/v1/api/openplatform/coding_plan/remains"
	}
	return "https://api.minimaxi.com/v1/api/openplatform/coding_plan/remains"
}

func zhipuQuotaHost(baseURL string) string {
	switch u := strings.ToLower(baseURL); {
	case strings.Contains(u, "bigmodel.cn"):
		return "https://open.bigmodel.cn"
	case strings.Contains(u, "z.ai"):
		return "https://api.z.ai"
	default:
		// 国产优先：未知域名回落国内站（与前端 zhipu 预设一致）。
		return "https://open.bigmodel.cn"
	}
}

// parseKimiUsageTiers 解析 Kimi For Coding 的 /usages 响应。
//
//   - limits[].detail.{limit,remaining,resetTime} → 5h 窗口（取首个 detail）
//   - usage.{limit,remaining,resetTime} → 周窗口
//
// utilization = (limit-remaining)/limit*100。
func parseKimiUsageTiers(body []byte) []CNQuotaTier {
	var tiers []CNQuotaTier

	if limits := gjson.GetBytes(body, "limits"); limits.IsArray() {
		limits.ForEach(func(_, item gjson.Result) bool {
			detail := item.Get("detail")
			if !detail.Exists() {
				return true
			}
			limit, _ := cnParseF64(detail.Get("limit").Value())
			remaining, _ := cnParseF64(detail.Get("remaining").Value())
			used := limit - remaining
			if used < 0 {
				used = 0
			}
			var util float64
			if limit > 0 {
				util = used / limit * 100
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      "5h",
				UsedPercent: util,
				ResetAt:     cnNormalizeResetTime(detail.Get("resetTime").Value()),
			})
			return false // 取首个 detail 作为 5h 窗口
		})
	}

	if usage := gjson.GetBytes(body, "usage"); usage.Exists() {
		limit, _ := cnParseF64(usage.Get("limit").Value())
		remaining, _ := cnParseF64(usage.Get("remaining").Value())
		used := limit - remaining
		if used < 0 {
			used = 0
		}
		var util float64
		if limit > 0 {
			util = used / limit * 100
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      "weekly",
			UsedPercent: util,
			ResetAt:     cnNormalizeResetTime(usage.Get("resetTime").Value()),
		})
	}

	return tiers
}

// parseMiniMaxUsageTiers 解析 MiniMax Token Plan / Coding Plan remains 响应。
//
// 只取 model_name == "general"（编程套餐），跳过 video。字段是剩余百分比，
// 展示已用 = 100 - remaining：
//   - 5h：current_interval_remaining_percent + end_time
//   - 周限额：仅 current_weekly_status == 1 时用 current_weekly_remaining_percent + weekly_end_time
func parseMiniMaxUsageTiers(body []byte) []CNQuotaTier {
	remains := gjson.GetBytes(body, "model_remains")
	if !remains.IsArray() {
		return nil
	}
	var general gjson.Result
	remains.ForEach(func(_, item gjson.Result) bool {
		if strings.EqualFold(strings.TrimSpace(item.Get("model_name").String()), "general") {
			general = item
			return false
		}
		return true
	})
	if !general.Exists() {
		return nil
	}

	var tiers []CNQuotaTier
	if remaining, ok := cnParseF64(general.Get("current_interval_remaining_percent").Value()); ok {
		used := 100 - remaining
		if used < 0 {
			used = 0
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      "5h",
			UsedPercent: used,
			ResetAt:     minimaxResetTime(general.Get("end_time")),
		})
	}
	if general.Get("current_weekly_status").Int() == 1 {
		if remaining, ok := cnParseF64(general.Get("current_weekly_remaining_percent").Value()); ok {
			used := 100 - remaining
			if used < 0 {
				used = 0
			}
			tiers = append(tiers, CNQuotaTier{
				Window:      "weekly",
				UsedPercent: used,
				ResetAt:     minimaxResetTime(general.Get("weekly_end_time")),
			})
		}
	}
	return tiers
}

func minimaxResetTime(v gjson.Result) string {
	if !v.Exists() {
		return ""
	}
	ms := v.Int()
	if ms <= 0 {
		return cnNormalizeResetTime(v.Value())
	}
	if ms < 1_000_000_000_000 {
		return time.Unix(ms, 0).UTC().Format(time.RFC3339)
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

// cnZhipuWindow 标识智谱额度条目（TOKENS_LIMIT / CREDIT_LIMIT）所属窗口。
type cnZhipuWindow int

const (
	cnZhipuWindowUnknown cnZhipuWindow = iota
	cnZhipuWindow5h
	cnZhipuWindowWeekly
)

// classifyZhipuWindowUnit 按 unit 字段判定窗口类型（3=5h，6=weekly）。
// unit 缺失或未识别时返回 Unknown，由调用方走 reset 时间启发式兜底。
func classifyZhipuWindowUnit(unit int64) cnZhipuWindow {
	switch unit {
	case 3:
		return cnZhipuWindow5h
	case 6:
		return cnZhipuWindowWeekly
	default:
		return cnZhipuWindowUnknown
	}
}

// parseZhipuTokenTiers 解析智谱额度响应 data.limits 为 5h + weekly 两档。
//
// 分类优先级（对齐 cc-switch parse_zhipu_token_tiers，issue #3036）：
//  1. 显式 unit 字段（3=5h / 6=weekly）——不能用 reset 排序代替，周期末尾
//     周窗口会比 5h 更早重置，时间排序必然标反。
//  2. unit 缺失/未识别：无 nextResetTime 的条目优先归 5h（0% 状态下 5h 桶可能
//     没有 reset），其余按 reset 升序依次填入仍空缺的槽位。
//
// CREDIT_LIMIT（信用额度）与 TOKENS_LIMIT（token 窗口）度量不同：两者同时返回时
// 只让 TOKENS_LIMIT 参与 5h/weekly 槽位竞争，避免信用额度百分比污染阈值停调
// 快照；仅当无任何 TOKENS_LIMIT 条目时才降级用 CREDIT_LIMIT 展示 —— 此时
// CREDIT_LIMIT 同样按 unit 分类（它也携带 unit），不依赖 reset 时间排序。
// 老套餐只回 1 条 TOKENS_LIMIT，自然降级为仅 5h；新套餐回 2 条。
func parseZhipuTokenTiers(data gjson.Result) []CNQuotaTier {
	type entry struct {
		resetMs    int64
		hasReset   bool
		percentage float64
		resetISO   string
		unit       int64
	}
	var (
		fiveHour     entry
		fiveHourSet  bool
		weekly       entry
		weeklySet    bool
		unclassified []entry
	)

	classify := func(unit int64, e entry) {
		switch classifyZhipuWindowUnit(unit) {
		case cnZhipuWindow5h:
			if !fiveHourSet {
				fiveHour, fiveHourSet = e, true
			} else {
				unclassified = append(unclassified, e)
			}
		case cnZhipuWindowWeekly:
			if !weeklySet {
				weekly, weeklySet = e, true
			} else {
				unclassified = append(unclassified, e)
			}
		default:
			unclassified = append(unclassified, e)
		}
	}
	var creditFallback []entry
	hasTokensLimit := false

	data.Get("limits").ForEach(func(_, item gjson.Result) bool {
		limitType := strings.ToUpper(strings.TrimSpace(item.Get("type").String()))
		if limitType != "TOKENS_LIMIT" && limitType != "CREDIT_LIMIT" {
			return true
		}
		percentage := 0.0
		if p, ok := cnParseF64(item.Get("percentage").Value()); ok {
			percentage = p
		}
		var (
			resetMs  int64
			hasReset bool
			resetISO string
		)
		if nr := item.Get("nextResetTime"); nr.Exists() {
			switch nr.Type {
			case gjson.Number:
				resetMs = nr.Int()
				hasReset = resetMs > 0
				resetISO = cnMillisToRFC3339(resetMs)
			case gjson.String:
				resetISO = cnNormalizeResetTime(nr.String())
				hasReset = resetISO != ""
			}
		}
		e := entry{resetMs: resetMs, hasReset: hasReset, percentage: percentage, resetISO: resetISO, unit: item.Get("unit").Int()}
		if limitType == "TOKENS_LIMIT" {
			hasTokensLimit = true
			classify(e.unit, e)
		} else {
			creditFallback = append(creditFallback, e)
		}
		return true
	})

	// 无任何 TOKENS_LIMIT 条目（部分套餐只报信用额度）：降级用 CREDIT_LIMIT 展示。
	// CREDIT_LIMIT 条目同样带 unit（3=5h / 6=weekly；实测 GLM Coding Plan 两条窗口
	// 都是 CREDIT_LIMIT），必须和 TOKENS_LIMIT 一样先按 unit 显式分类 —— 否则会退化
	// 成 reset 时间启发式，而周期末尾周窗口比 5h 更早重置，时间排序必然把两档标反。
	if !hasTokensLimit {
		for _, e := range creditFallback {
			if classifyZhipuWindowUnit(e.unit) != cnZhipuWindowUnknown {
				classify(e.unit, e)
				continue
			}
			unclassified = append(unclassified, e)
		}
	}

	// 无 reset 的条目排前，再按 reset 升序，依次填入仍空缺的槽位。
	sort.SliceStable(unclassified, func(i, j int) bool {
		if unclassified[i].hasReset != unclassified[j].hasReset {
			return !unclassified[i].hasReset
		}
		return unclassified[i].resetMs < unclassified[j].resetMs
	})
	for _, e := range unclassified {
		switch {
		case !fiveHourSet:
			fiveHour, fiveHourSet = e, true
		case !weeklySet:
			weekly, weeklySet = e, true
		}
	}

	var tiers []CNQuotaTier
	if fiveHourSet {
		tiers = append(tiers, CNQuotaTier{Window: "5h", UsedPercent: fiveHour.percentage, ResetAt: fiveHour.resetISO})
	}
	if weeklySet {
		tiers = append(tiers, CNQuotaTier{Window: "weekly", UsedPercent: weekly.percentage, ResetAt: weekly.resetISO})
	}
	return tiers
}

// cnQuotaExtraUpdates 根据 tier 列表构造 provider 维度的 Extra 快照更新。
func cnQuotaExtraUpdates(provider string, tiers []CNQuotaTier, now time.Time) map[string]any {
	updates := map[string]any{
		cnExtraKey(provider, cnExtraSuffixUsageUpdated): now.Format(time.RFC3339),
	}
	for _, t := range tiers {
		switch t.Window {
		case "5h":
			updates[cnExtraKey(provider, cnExtraSuffix5hUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffix5hReset)] = t.ResetAt
			}
		case "weekly":
			updates[cnExtraKey(provider, cnExtraSuffixWeeklyUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffixWeeklyReset)] = t.ResetAt
			}
		case "monthly":
			updates[cnExtraKey(provider, cnExtraSuffixMonthlyUsed)] = t.UsedPercent
			if t.ResetAt != "" {
				updates[cnExtraKey(provider, cnExtraSuffixMonthlyReset)] = t.ResetAt
			}
		}
	}
	return updates
}

// parseOpenCodeGoUsageTiers 解析 OpenCode Go GET /usage 响应。
//
// 结构对齐 cc-switch extractor：
//
//	{ "usage": { "rolling": {percent, resetsAt}, "weekly": {...}, "monthly": {...} } }
//
// percent 为已用百分比（0-100）；rolling 映射为 5h 窗口。
func parseOpenCodeGoUsageTiers(body []byte) []CNQuotaTier {
	usage := gjson.GetBytes(body, "usage")
	if !usage.Exists() {
		return nil
	}
	var tiers []CNQuotaTier
	for _, item := range []struct {
		key    string
		window string
	}{
		{key: "rolling", window: "5h"},
		{key: "weekly", window: "weekly"},
		{key: "monthly", window: "monthly"},
	} {
		node := usage.Get(item.key)
		if !node.Exists() {
			continue
		}
		percentNode := node.Get("percent")
		if !percentNode.Exists() {
			continue
		}
		used, ok := cnParseF64(percentNode.Value())
		if !ok {
			continue
		}
		if used < 0 {
			used = 0
		}
		tiers = append(tiers, CNQuotaTier{
			Window:      item.window,
			UsedPercent: used,
			ResetAt:     cnNormalizeResetTime(node.Get("resetsAt").Value()),
		})
	}
	return tiers
}

// cnParseF64 把 JSON 数值或字符串解析为 float64（兼容 "100" 与 100）。
func cnParseF64(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// cnNormalizeResetTime 把上游重置时间（ISO8601 字符串 / 秒级 / 毫秒级数字）归一化为
// RFC3339 字符串；无法识别或非正时间戳返回空串。
func cnNormalizeResetTime(raw any) string {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return ""
		}
		if ts, err := parseSchedulingTime(s); err == nil {
			return ts.UTC().Format(time.RFC3339)
		}
		return ""
	case float64:
		return cnMillisToRFC3339(int64(v))
	case int:
		return cnMillisToRFC3339(int64(v))
	case int64:
		return cnMillisToRFC3339(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return cnMillisToRFC3339(n)
		}
		return ""
	default:
		return ""
	}
}

// cnMillisToRFC3339 把秒级（<1e12）或毫秒级时间戳转为 RFC3339 字符串；非正返回空串。
func cnMillisToRFC3339(n int64) string {
	if n <= 0 {
		return ""
	}
	var ms int64
	if n < 1_000_000_000_000 {
		ms = n * 1000
	} else {
		ms = n
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
