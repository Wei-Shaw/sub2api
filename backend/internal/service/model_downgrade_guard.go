package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// 模型降级守卫（model downgrade guard）
//
// OpenAI 上游会按账号把某个模型静默降级：请求发的是 A 模型，成功响应里自报的却是
// 更低一档的 B 模型。usage_logs 已经记录了 upstream_model_mismatch，但调度并不感知，
// 同一个账号会持续接到并"降级服务"这个模型的请求。
//
// 守卫挂在 OpenAI 的 RecordUsage 上（HTTP / Chat Completions / WS 都经过这里），
// 只在「发往上游的模型 → 上游自报的模型」命中管理员显式配置的降级对时，按
// 账号 + 发往上游的模型计数；窗口内达到阈值后按配置执行动作：
//   - model_block（默认）：只在该账号的 extra.model_rate_limits 里屏蔽这个模型；
//   - temp_unsched：整账号临时不可调度；
//   - none：观察模式，只记录不处理。
//
// 安全阀：同时因降级受限的账号不能超过 OpenAI 活跃账号的 MaxBlockedRatio，
// 超过时只记录不处理，避免上游全局降级时把号池清空。统计与写入在同一个事务里完成。

const (
	// ModelDowngradeGuardKeyword 是写进 TempUnschedState.MatchedKeyword 的标记，
	// 安全阀的 SQL 也靠它在 temp_unschedulable_reason / model_rate_limits.reason 里
	// 认出"因降级受限"的账号。
	ModelDowngradeGuardKeyword = "model_downgrade"

	// RecordUsage 是热路径，每次请求都会读一次配置，加一层短 TTL 内存缓存避免打 DB。
	modelDowngradeGuardSettingsCacheTTL = 30 * time.Second
)

// ModelDowngradeCounterCache 模型降级命中计数器。
//
// 计数维度是 account_id + sent_model，与流超时计数器（按账号）刻意分开，避免两种
// 熔断互相污染。
type ModelDowngradeCounterCache interface {
	// IncrementModelDowngradeCount 增加一次命中计数并返回窗口内当前值。
	// windowMinutes 是计数窗口，key 过期即自动清零。
	IncrementModelDowngradeCount(ctx context.Context, accountID int64, sentModel string, windowMinutes int) (int64, error)
	// ResetModelDowngradeCount 清零该账号 + 模型的命中计数。
	ResetModelDowngradeCount(ctx context.Context, accountID int64, sentModel string) error
}

// 观察记录的来源：观察模式（动作 = none）命中阈值，或比例上限把真实处理拦了下来。
const (
	ModelDowngradeObservedCauseDryRun   = "dry_run"
	ModelDowngradeObservedCauseRatioCap = "ratio_cap"
)

// ModelDowngradeObservedEntry 是一条「本应处理但没处理」的观察记录。
//
// 观察模式和比例上限拦下时只有 slog 的话，管理后台完全看不到；这里把同样的信息
// 写进 Redis，让管理页的表格能直接展示。整条记录序列化成 JSON 存一个 key，
// 过期时间由写入方按处理时长给定。
type ModelDowngradeObservedEntry struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name,omitempty"`
	// Cause 取 ModelDowngradeObservedCauseDryRun / ModelDowngradeObservedCauseRatioCap
	Cause                string    `json:"cause"`
	SentModel            string    `json:"sent_model"`
	ResponseModel        string    `json:"response_model"`
	TriggerCount         int64     `json:"trigger_count"`
	TriggerThreshold     int       `json:"trigger_threshold"`
	TriggerWindowMinutes int       `json:"trigger_window_minutes"`
	ObservedAt           time.Time `json:"observed_at"`
	// ExpiresAt 在写入时就算好存进 JSON，列表接口不必再回查 TTL。
	ExpiresAt time.Time `json:"expires_at"`
	// 以下三个字段仅在 Cause == ratio_cap 时有值，用来解释「为什么没处理」。
	Blocked         int64   `json:"blocked,omitempty"`
	Total           int64   `json:"total,omitempty"`
	MaxBlockedRatio float64 `json:"max_blocked_ratio,omitempty"`
}

// ModelDowngradeObservedCache 观察记录存储。
//
// 放 Redis 而不是内存：后端可能多实例部署，命中可能落在任意一个实例上，
// 管理后台的请求也可能打到另一个实例。
type ModelDowngradeObservedCache interface {
	// RecordObserved 写入（或覆盖）一条观察记录，ttl 到期自动清理。
	RecordObserved(ctx context.Context, entry ModelDowngradeObservedEntry, ttl time.Duration) error
	// ListObserved 返回当前所有未过期的观察记录，顺序不保证。
	ListObserved(ctx context.Context) ([]ModelDowngradeObservedEntry, error)
	// DeleteObserved 删除一条观察记录，返回是否真的删掉了（用于区分 404）。
	DeleteObserved(ctx context.Context, accountID int64, sentModel string) (bool, error)
}

// SetModelDowngradeCounterCache 设置模型降级计数器（可选依赖）
func (s *RateLimitService) SetModelDowngradeCounterCache(cache ModelDowngradeCounterCache) {
	s.modelDowngradeCounter = cache
}

// SetModelDowngradeObservedCache 设置观察记录存储（可选依赖）。
// 不注入时观察记录只走日志，主流程行为不变。
func (s *RateLimitService) SetModelDowngradeObservedCache(cache ModelDowngradeObservedCache) {
	s.modelDowngradeObserved = cache
}

// HandleModelDowngrade 处理一次"上游静默降级"命中。
//
// 由 OpenAI 的 RecordUsage 调用：上游成功响应自报的模型命中显式配置的降级对时
// 累计计数，窗口内达到阈值后按配置动作处理该账号。任何失败都只打日志，绝不影响
// 用量落库。
func (s *RateLimitService) HandleModelDowngrade(ctx context.Context, account *Account, sentModel, responseModel string) {
	if s == nil || account == nil || account.ID <= 0 {
		return
	}
	if account.Platform != PlatformOpenAI {
		return
	}
	sentModel = strings.TrimSpace(sentModel)
	responseModel = strings.TrimSpace(responseModel)
	if sentModel == "" || responseModel == "" {
		return
	}

	settings := s.modelDowngradeGuardSettings(ctx)
	if settings == nil || !settings.Enabled {
		return
	}
	if !settings.MatchPair(sentModel, responseModel) {
		return
	}

	// 计数失败时退回 count=1：Redis 不可用时宁可不熔断，也不要凭单次命中处理账号。
	var count int64 = 1
	if s.modelDowngradeCounter != nil {
		value, err := s.modelDowngradeCounter.IncrementModelDowngradeCount(ctx, account.ID, sentModel, settings.ThresholdWindowMinutes)
		if err != nil {
			slog.Warn("model_downgrade_increment_count_failed", "account_id", account.ID, "sent_model", sentModel, "error", err)
		} else {
			count = value
		}
	}

	slog.Info("model_downgrade_count",
		"account_id", account.ID,
		"sent_model", sentModel,
		"response_model", responseModel,
		"count", count,
		"threshold", settings.ThresholdCount,
		"window_minutes", settings.ThresholdWindowMinutes,
	)

	if count < int64(settings.ThresholdCount) {
		return
	}

	// 达到阈值后无论是否真的处理都清零，让同一账号 + 模型在一个窗口内最多触发一次，
	// 观察模式和安全阀命中时的日志频率也因此与真实动作保持一致。
	defer s.resetModelDowngradeCount(ctx, account.ID, sentModel)

	if settings.Action == ModelDowngradeGuardActionNone {
		slog.Info("model_downgrade_guard_dry_run",
			"account_id", account.ID,
			"sent_model", sentModel,
			"response_model", responseModel,
			"count", count,
		)
		// 观察模式只记录不动账号，但「本应处理」的事实要落到管理页上。
		s.recordModelDowngradeObserved(ctx, ModelDowngradeObservedEntry{
			AccountID:            account.ID,
			AccountName:          account.Name,
			Cause:                ModelDowngradeObservedCauseDryRun,
			SentModel:            sentModel,
			ResponseModel:        responseModel,
			TriggerCount:         count,
			TriggerThreshold:     settings.ThresholdCount,
			TriggerWindowMinutes: settings.ThresholdWindowMinutes,
		}, settings.BlockHours)
		return
	}

	if s.accountRepo == nil {
		return
	}

	outcome, err := s.applyModelDowngradeBlock(ctx, modelDowngradeBlockInput{
		account:       account,
		settings:      settings,
		action:        settings.Action,
		sentModel:     sentModel,
		responseModel: responseModel,
		count:         count,
	})
	if err != nil {
		slog.Warn("model_downgrade_apply_block_failed",
			"account_id", account.ID,
			"scope", outcome.Scope,
			"sent_model", sentModel,
			"error", err,
		)
		return
	}
	// 重复触发（例如整账号下线后 WS 长连接还在跑、计数继续涨）什么都不做：
	// 计数已由上面的 defer 清零，日志在 applyModelDowngradeBlock 里打过了。
	if outcome.AlreadyBlocked {
		return
	}
	if !outcome.Applied {
		slog.Warn("model_downgrade_guard_ratio_cap_hit",
			"account_id", account.ID,
			"scope", outcome.Scope,
			"blocked", outcome.Blocked,
			"total", outcome.Total,
			"max_blocked_ratio", settings.MaxBlockedRatio,
		)
		// 被安全阀拦下的账号同样要在管理页上留痕，附上当时的分子分母解释原因。
		s.recordModelDowngradeObserved(ctx, ModelDowngradeObservedEntry{
			AccountID:            account.ID,
			AccountName:          account.Name,
			Cause:                ModelDowngradeObservedCauseRatioCap,
			SentModel:            sentModel,
			ResponseModel:        responseModel,
			TriggerCount:         count,
			TriggerThreshold:     settings.ThresholdCount,
			TriggerWindowMinutes: settings.ThresholdWindowMinutes,
			Blocked:              outcome.Blocked,
			Total:                outcome.Total,
			MaxBlockedRatio:      settings.MaxBlockedRatio,
		}, settings.BlockHours)
		return
	}

	slog.Info("account_model_downgrade_blocked",
		"account_id", account.ID,
		"sent_model", sentModel,
		"response_model", responseModel,
		"count", count,
		"action", settings.Action,
		"until", outcome.Until,
	)
}

// modelDowngradeBlockInput 是执行一次降级处理所需的全部输入。
// manual 为 true 时来自管理员在页面上点「立即处理」，reason 里会注明。
type modelDowngradeBlockInput struct {
	account       *Account
	settings      *ModelDowngradeGuardSettings
	action        string
	sentModel     string
	responseModel string
	count         int64
	manual        bool
}

// modelDowngradeBlockOutcome 是一次处理尝试的结果。
//
// Applied=false 且 AlreadyBlocked=false 表示被比例上限拦下，此时 Blocked/Total 是
// 仓储在同一个事务里读到的分子分母；AlreadyBlocked=true 表示这次是重复处理，
// 守卫已经覆盖了这个账号（或这个模型），调用方什么都不用做。
type modelDowngradeBlockOutcome struct {
	Applied        bool
	AlreadyBlocked bool
	Scope          string
	Model          string
	Until          time.Time
	Blocked        int64
	Total          int64
}

// applyModelDowngradeBlock 把「原子写入 + 写成功后的副作用」收成一处：
// 自动触发（HandleModelDowngrade）和管理员手动触发（ApplyModelDowngradeBlockNow）
// 共用同一段逻辑，避免两条路径的 runtime blocker / 缓存行为漂移。
func (s *RateLimitService) applyModelDowngradeBlock(
	ctx context.Context,
	in modelDowngradeBlockInput,
) (modelDowngradeBlockOutcome, error) {
	// 动作 → 写入范围。范围决定写哪一列，也决定比例上限的原子写入走哪条分支。
	scope, blockedModel := "", ""
	switch in.action {
	case ModelDowngradeGuardActionModelBlock:
		scope, blockedModel = ModelDowngradeBlockedScopeModel, in.sentModel
	case ModelDowngradeGuardActionTempUnsched:
		scope = ModelDowngradeBlockedScopeAccount
	default:
		return modelDowngradeBlockOutcome{}, ErrModelDowngradeBlockScopeInvalid
	}

	now := time.Now()
	until := now.Add(time.Duration(in.settings.BlockHours) * time.Hour)
	errorMessage := buildModelDowngradeErrorMessage(in.sentModel, in.responseModel, in.count, in.settings.ThresholdWindowMinutes)
	if in.manual {
		errorMessage += modelDowngradeManualSuffix
	}
	state := &TempUnschedState{
		UntilUnix:            until.Unix(),
		TriggeredAtUnix:      now.Unix(),
		StatusCode:           0, // 降级是成功响应，没有状态码
		MatchedKeyword:       ModelDowngradeGuardKeyword,
		RuleIndex:            -1, // 系统级规则
		ErrorMessage:         errorMessage,
		TriggerCount:         in.count,
		TriggerThreshold:     in.settings.ThresholdCount,
		TriggerWindowMinutes: in.settings.ThresholdWindowMinutes,
	}
	reason := ""
	if raw, err := json.Marshal(state); err == nil {
		reason = string(raw)
	}
	if reason == "" {
		reason = state.ErrorMessage
	}

	// 统计 + 比例判定 + 写入必须在同一个事务里完成，否则并发触发时大家读到同一个
	// blocked 数，比例上限形同虚设。口径见 AccountRepository.ApplyOpenAIModelDowngradeBlock。
	result, err := s.accountRepo.ApplyOpenAIModelDowngradeBlock(
		ctx, in.account.ID, scope, blockedModel, until, reason, in.settings.MaxBlockedRatio, now,
	)
	outcome := modelDowngradeBlockOutcome{
		Applied:        result.Applied,
		AlreadyBlocked: result.AlreadyBlocked,
		Scope:          scope,
		Model:          blockedModel,
		Until:          until,
		Blocked:        result.Blocked,
		Total:          result.Total,
	}
	if err != nil {
		outcome.Applied = false
		return outcome, err
	}
	if outcome.AlreadyBlocked {
		// 幂等路径：守卫已经覆盖这个账号（或这个模型），库里一行都没改。
		// tempUnschedCache、runtime blocker 一概不碰，只留一条 debug 日志——
		// 重复触发在 WS 长连接场景下是常态，不该刷屏。
		slog.Debug("model_downgrade_guard_already_blocked",
			"account_id", in.account.ID,
			"scope", scope,
			"sent_model", in.sentModel,
		)
		// 已经受限的账号不该还挂着「观察中」的记录，顺手清掉。
		s.deleteModelDowngradeObserved(ctx, in.account.ID, in.sentModel)
		return outcome, nil
	}
	if !outcome.Applied {
		return outcome, nil
	}

	// runtime blocker 和缓存都要等写库真正生效后再动，比例上限拦下时不能留下
	// 与库不一致的「已下线」状态。仅屏蔽模型不改变账号级可调度状态，不碰它们。
	if scope == ModelDowngradeBlockedScopeAccount {
		s.notifyAccountSchedulingBlocked(in.account, until, "model_downgrade_temp_unschedulable")
		if s.tempUnschedCache != nil {
			if err := s.tempUnschedCache.SetTempUnsched(ctx, in.account.ID, state); err != nil {
				slog.Warn("model_downgrade_set_temp_unsched_cache_failed", "account_id", in.account.ID, "error", err)
			}
		}
	}

	// 真的处理之后就不该再以「观察中」的身份出现在表格里。
	s.deleteModelDowngradeObserved(ctx, in.account.ID, in.sentModel)
	return outcome, nil
}

// recordModelDowngradeObserved 写一条观察记录，失败只打 warn：这只是给管理页看的，
// 绝不能影响用量落库或熔断主流程。
func (s *RateLimitService) recordModelDowngradeObserved(
	ctx context.Context,
	entry ModelDowngradeObservedEntry,
	blockHours int,
) {
	if s.modelDowngradeObserved == nil {
		return
	}
	ttl := time.Duration(blockHours) * time.Hour
	if ttl <= 0 {
		ttl = time.Duration(modelDowngradeGuardDefaultBlockHours) * time.Hour
	}
	now := time.Now()
	entry.ObservedAt = now
	entry.ExpiresAt = now.Add(ttl)
	if err := s.modelDowngradeObserved.RecordObserved(ctx, entry, ttl); err != nil {
		slog.Warn("model_downgrade_record_observed_failed",
			"account_id", entry.AccountID,
			"sent_model", entry.SentModel,
			"cause", entry.Cause,
			"error", err,
		)
	}
}

func (s *RateLimitService) deleteModelDowngradeObserved(ctx context.Context, accountID int64, sentModel string) {
	if s.modelDowngradeObserved == nil {
		return
	}
	if _, err := s.modelDowngradeObserved.DeleteObserved(ctx, accountID, sentModel); err != nil {
		slog.Warn("model_downgrade_delete_observed_failed", "account_id", accountID, "sent_model", sentModel, "error", err)
	}
}

// modelDowngradeGuardSettings 读配置，带 30s 内存缓存（RecordUsage 是热路径）。
func (s *RateLimitService) modelDowngradeGuardSettings(ctx context.Context) *ModelDowngradeGuardSettings {
	if s.settingService == nil {
		return nil
	}

	now := time.Now()
	s.modelDowngradeSettingsMu.RLock()
	cached, cachedAt := s.modelDowngradeSettings, s.modelDowngradeSettingsAt
	s.modelDowngradeSettingsMu.RUnlock()
	if cached != nil && now.Sub(cachedAt) < modelDowngradeGuardSettingsCacheTTL {
		return cached
	}

	settings, err := s.settingService.GetModelDowngradeGuardSettings(ctx)
	if err != nil {
		slog.Warn("model_downgrade_get_settings_failed", "error", err, "path", "auto")
		// 拿不到最新值时沿用上一次的缓存，避免 DB 抖动期间守卫忽开忽关。
		return cached
	}

	s.cacheModelDowngradeGuardSettings(settings, now)
	return settings
}

// refreshModelDowngradeGuardSettings 绕开 30s 缓存直读配置，供管理员手动触发的路径使用。
//
// 手动路径的配置必须是最新的：管理员往往是刚改完处理方式或比例上限就来点「立即处理」，
// 缓存里的旧值会让这一下按上一份配置执行。读失败时返回错误而不是回落缓存/默认值——
// 按过期配置处理比失败更糟，让管理员重试。读到的最新值顺手写回缓存。
func (s *RateLimitService) refreshModelDowngradeGuardSettings(ctx context.Context) (*ModelDowngradeGuardSettings, error) {
	if s.settingService == nil {
		return nil, ErrModelDowngradeGuardSettingsUnavailable
	}

	now := time.Now()
	settings, err := s.settingService.GetModelDowngradeGuardSettings(ctx)
	if err != nil {
		slog.Warn("model_downgrade_get_settings_failed", "error", err, "path", "manual")
		return nil, ErrModelDowngradeGuardSettingsUnavailable.WithCause(err)
	}
	if settings == nil {
		return nil, ErrModelDowngradeGuardSettingsUnavailable
	}

	s.cacheModelDowngradeGuardSettings(settings, now)
	return settings, nil
}

func (s *RateLimitService) cacheModelDowngradeGuardSettings(settings *ModelDowngradeGuardSettings, at time.Time) {
	s.modelDowngradeSettingsMu.Lock()
	s.modelDowngradeSettings, s.modelDowngradeSettingsAt = settings, at
	s.modelDowngradeSettingsMu.Unlock()
}

func (s *RateLimitService) resetModelDowngradeCount(ctx context.Context, accountID int64, sentModel string) {
	if s.modelDowngradeCounter == nil {
		return
	}
	if err := s.modelDowngradeCounter.ResetModelDowngradeCount(ctx, accountID, sentModel); err != nil {
		slog.Warn("model_downgrade_reset_count_failed", "account_id", accountID, "sent_model", sentModel, "error", err)
	}
}

// buildModelDowngradeErrorMessage 生成写入 TempUnschedState.ErrorMessage 的说明，
// 形如 "gpt-6-astra → gpt-5.6-luna (5 hits in 30 min)"。
// 格式由 SplitModelDowngradeModels 负责解析，两边要一起改。
func buildModelDowngradeErrorMessage(sentModel, responseModel string, count int64, windowMinutes int) string {
	return fmt.Sprintf("%s %s %s (%d hits in %d min)", sentModel, modelDowngradeArrow, responseModel, count, windowMinutes)
}
