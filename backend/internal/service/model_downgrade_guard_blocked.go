package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 因降级受限的记录范围：整账号不可调度，或只屏蔽命中的那个模型。
// observed 不是真实的限制范围，只是「本应处理」的观察记录在列表和释放接口里的伪范围。
const (
	ModelDowngradeBlockedScopeAccount  = "account"
	ModelDowngradeBlockedScopeModel    = "model"
	ModelDowngradeBlockedScopeObserved = "observed"
)

// 表格里每一行的状态：真实受限、观察模式命中、被比例上限拦下。
const (
	ModelDowngradeBlockedStatusBlocked     = "blocked"
	ModelDowngradeBlockedStatusObserved    = "observed"
	ModelDowngradeBlockedStatusRatioCapped = "ratio_capped"
)

// modelDowngradeArrow 是 buildModelDowngradeErrorMessage 写进 error_message 的分隔符。
const modelDowngradeArrow = "→"

// modelDowngradeManualSuffix 标记这条限制是管理员在页面上手动处理的。
// 用方括号而不是圆括号，不会干扰 SplitModelDowngradeModels 对 "(" 的解析。
const modelDowngradeManualSuffix = " [manual]"

// modelDowngradeManualUnknownResponseModel 是手动处理时没有观察记录可查的占位响应模型，
// 保持 error_message 仍然能被 SplitModelDowngradeModels 拆开。
const modelDowngradeManualUnknownResponseModel = "unknown"

var (
	// ErrModelDowngradeBlockScopeInvalid 表示 scope 不是 account / model / observed，
	// 或 model / observed 范围没有带上模型名。
	ErrModelDowngradeBlockScopeInvalid = infraerrors.BadRequest(
		"MODEL_DOWNGRADE_BLOCK_SCOPE_INVALID",
		"model downgrade block scope must be account, model or observed, and model/observed scopes require a model name",
	)
	// ErrModelDowngradeBlockNotFound 表示该账号（或该账号的该模型）当前并没有被
	// 降级守卫限制，提前恢复没有可解除的对象。别的来源写的下线/限流不在此接口的范围内。
	ErrModelDowngradeBlockNotFound = infraerrors.NotFound(
		"MODEL_DOWNGRADE_BLOCK_NOT_FOUND",
		"no model downgrade block found for this account",
	)
	// ErrModelDowngradeBlockRatioCapped 表示手动处理被比例上限拦下：不是参数错误，
	// 也不是不存在，而是安全阀在保护号池，handler 映射成 409 并带上当时的分子分母。
	ErrModelDowngradeBlockRatioCapped = infraerrors.Conflict(
		"MODEL_DOWNGRADE_BLOCK_RATIO_CAPPED",
		"model downgrade block rejected by the blocked-ratio cap",
	)
	// ErrModelDowngradeBlockAlreadyActive 表示该账号（或该账号的该模型）已经被降级守卫
	// 限制了，这次是重复动作。守卫必须幂等：整账号下线期间 WS 长连接还在跑，命中
	// 计数会继续涨并再次触发，此时若配置已切成「仅屏蔽该模型」，同一个账号就会同时出现
	// 「整账号」和「仅模型」两条限制。handler 映射成 409。
	ErrModelDowngradeBlockAlreadyActive = infraerrors.Conflict(
		"MODEL_DOWNGRADE_BLOCK_ALREADY_ACTIVE",
		"account is already blocked by the model downgrade guard",
	)
	// ErrModelDowngradeGuardUnavailable 表示观察记录存储没有装配，
	// 观察记录相关的读写都无从谈起。
	ErrModelDowngradeGuardUnavailable = infraerrors.ServiceUnavailable(
		"MODEL_DOWNGRADE_OBSERVED_CACHE_UNAVAILABLE",
		"model downgrade observed cache is not configured",
	)
	// ErrModelDowngradeGuardSettingsUnavailable 表示手动处理时读不到最新配置。
	// 手动路径刻意不回落缓存或默认值：按过期配置处理（比如比例上限刚改成 0 却仍放行）
	// 比直接失败更糟，handler 映射成 503，管理员重试一次即可。
	ErrModelDowngradeGuardSettingsUnavailable = infraerrors.ServiceUnavailable(
		"MODEL_DOWNGRADE_GUARD_SETTINGS_UNAVAILABLE",
		"failed to load the latest model downgrade guard settings, please retry",
	)
)

// ModelDowngradeBlockApplyResult 是 AccountRepository.ApplyOpenAIModelDowngradeBlock
// 的结果。三种互斥的结局：
//   - Applied=true：真的写进去了；
//   - AlreadyBlocked=true：该账号已经被守卫覆盖，这次是重复处理，直接跳过（幂等）；
//   - 两个都是 false：被比例上限（或分母为 0）拦下。
//
// AlreadyBlocked 必须和比例上限区分开：前者什么都不用做，后者要留观察记录。
type ModelDowngradeBlockApplyResult struct {
	Applied        bool
	AlreadyBlocked bool
	// Blocked / Total 是仓储在同一个事务里读到的安全阀分子分母，供日志和页面解释原因。
	// AlreadyBlocked 时不做统计，两个值都是 0。
	Blocked int64
	Total   int64
}

// ModelDowngradeBlockedAccount 是「当前因降级受限的账号」表格里的一行。
//
// 一个账号可能同时出现在两种范围里（整账号下线 + 某个模型被屏蔽），此时会返回两行。
type ModelDowngradeBlockedAccount struct {
	AccountID   int64
	AccountName string
	// Scope 取 ModelDowngradeBlockedScopeAccount / ModelDowngradeBlockedScopeModel，
	// 观察记录用 ModelDowngradeBlockedScopeObserved
	Scope string
	// Status 取 ModelDowngradeBlockedStatus*，区分真实受限行和观察行。
	// 仓储返回的行不带这个字段，由 ListModelDowngradeBlocked 统一补成 blocked。
	Status string
	// Cause 仅观察行有值，取 ModelDowngradeObservedCause*
	Cause string
	// Model 仅在 Scope == model / observed 时有值，是被屏蔽（或观察）的模型（发往上游的模型名）
	Model                string
	SentModel            string
	ResponseModel        string
	TriggerCount         int64
	TriggerThreshold     int
	TriggerWindowMinutes int
	TriggeredAt          time.Time
	// Until 对真实受限行是预计恢复时间，对观察行是这条记录的过期时间。
	Until time.Time
	// 以下三个字段仅在 Status == ratio_capped 时有值，解释当时为什么没处理。
	Blocked         int64
	Total           int64
	MaxBlockedRatio float64
}

// ModelDowngradeBlockedSummary 是表格上方的汇总：分子、分母和当前配置的比例上限。
// Observed 是观察记录条数（观察模式 + 比例上限拦下），不计入 Blocked，
// 页面上的占比仍然只按真实受限算。
type ModelDowngradeBlockedSummary struct {
	Blocked         int64
	Observed        int64
	TotalActive     int64
	MaxBlockedRatio float64
}

// ModelDowngradeBlockedList 是列表接口的返回体。
type ModelDowngradeBlockedList struct {
	Items   []ModelDowngradeBlockedAccount
	Summary ModelDowngradeBlockedSummary
}

// ModelDowngradeStateFromReason 把 temp_unschedulable_reason / model_rate_limits.reason
// 解析成 TempUnschedState，并确认它确实是模型降级守卫写的。
//
// 守卫以外的写入方（关键词规则、流超时等）也会往同一列写 JSON，因此必须校验
// MatchedKeyword，不能只看字符串里是否出现过 "model_downgrade"。
func ModelDowngradeStateFromReason(reason string) (*TempUnschedState, bool) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" || !strings.Contains(trimmed, ModelDowngradeGuardKeyword) {
		return nil, false
	}
	var state TempUnschedState
	if err := json.Unmarshal([]byte(trimmed), &state); err != nil {
		return nil, false
	}
	if state.MatchedKeyword != ModelDowngradeGuardKeyword {
		return nil, false
	}
	return &state, true
}

// SplitModelDowngradeModels 从 error_message 里拆出「发往上游模型」和「上游响应模型」。
//
// 写入格式由 buildModelDowngradeErrorMessage 决定，形如
// "gpt-6-astra → gpt-5.6-luna (5 hits in 30 min)"，手动处理时末尾再带 " [manual]"。
// 拆不出来时返回两个空串，调用方按缺省展示即可，绝不 panic。
func SplitModelDowngradeModels(errorMessage string) (string, string) {
	msg := strings.TrimSpace(errorMessage)
	if msg == "" {
		return "", ""
	}
	idx := strings.Index(msg, modelDowngradeArrow)
	if idx < 0 {
		return "", ""
	}
	sent := strings.TrimSpace(msg[:idx])
	rest := strings.TrimSpace(msg[idx+len(modelDowngradeArrow):])
	rest = strings.TrimSpace(strings.TrimSuffix(rest, strings.TrimSpace(modelDowngradeManualSuffix)))
	if open := strings.LastIndex(rest, "("); open >= 0 {
		rest = strings.TrimSpace(rest[:open])
	}
	if sent == "" || rest == "" {
		return "", ""
	}
	return sent, rest
}

// ListModelDowngradeBlocked 返回当前仍处于降级受限窗口内的 OpenAI 账号以及观察记录，
// 按预计恢复时间倒序（最晚恢复的排最前）。
//
// 汇总口径复用安全阀的 CountOpenAIModelDowngradeBlocked，保证页面上看到的比例和
// 守卫真正用来判断是否继续处理的比例是同一个数。
func (s *RateLimitService) ListModelDowngradeBlocked(ctx context.Context) (*ModelDowngradeBlockedList, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account repository not configured")
	}

	now := time.Now()
	items, err := s.accountRepo.ListOpenAIModelDowngradeBlocked(ctx, now)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []ModelDowngradeBlockedAccount{}
	}
	// 仓储只认真实受限，状态在这里统一补齐，后面的观察行才能和它们同表排序。
	blockedKeys := make(map[string]struct{}, len(items))
	for i := range items {
		items[i].Status = ModelDowngradeBlockedStatusBlocked
		blockedKeys[modelDowngradeObservedDedupeKey(items[i].AccountID, items[i].SentModel)] = struct{}{}
	}

	observed := s.listModelDowngradeObserved(ctx, blockedKeys)
	items = append(items, observed...)

	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].Until.Equal(items[j].Until) {
			return items[i].Until.After(items[j].Until)
		}
		return items[i].AccountID < items[j].AccountID
	})

	blocked, total, err := s.accountRepo.CountOpenAIModelDowngradeBlocked(ctx, now)
	if err != nil {
		return nil, err
	}

	summary := ModelDowngradeBlockedSummary{
		Blocked:     blocked,
		Observed:    int64(len(observed)),
		TotalActive: total,
	}
	if settings := s.modelDowngradeGuardSettings(ctx); settings != nil {
		summary.MaxBlockedRatio = settings.MaxBlockedRatio
	}

	return &ModelDowngradeBlockedList{Items: items, Summary: summary}, nil
}

// modelDowngradeObservedDedupeKey 是「同一账号 + 同一发往上游模型」的去重键。
// 模型名统一小写，和 Redis 里的 key 口径保持一致。
func modelDowngradeObservedDedupeKey(accountID int64, sentModel string) string {
	return strconv.FormatInt(accountID, 10) + ":" + strings.ToLower(strings.TrimSpace(sentModel))
}

// listModelDowngradeObserved 读观察记录并转成表格行。
//
// 读失败只打日志、返回空：观察记录是辅助信息，不能让 Redis 抖动把「真实受限」
// 这张更重要的表一起打挂。
//
// blockedKeys 里已经有真实受限行的账号 + 模型不再输出观察行：写入时本应已经删掉，
// 这里只是容错。
func (s *RateLimitService) listModelDowngradeObserved(ctx context.Context, blockedKeys map[string]struct{}) []ModelDowngradeBlockedAccount {
	if s.modelDowngradeObserved == nil {
		return nil
	}
	entries, err := s.modelDowngradeObserved.ListObserved(ctx)
	if err != nil {
		slog.Warn("model_downgrade_list_observed_failed", "error", err)
		return nil
	}

	rows := make([]ModelDowngradeBlockedAccount, 0, len(entries))
	for _, entry := range entries {
		if _, ok := blockedKeys[modelDowngradeObservedDedupeKey(entry.AccountID, entry.SentModel)]; ok {
			slog.Debug("model_downgrade_observed_shadowed_by_block",
				"account_id", entry.AccountID,
				"sent_model", entry.SentModel,
				"cause", entry.Cause,
			)
			continue
		}
		status := ModelDowngradeBlockedStatusObserved
		if entry.Cause == ModelDowngradeObservedCauseRatioCap {
			status = ModelDowngradeBlockedStatusRatioCapped
		}
		rows = append(rows, ModelDowngradeBlockedAccount{
			AccountID:            entry.AccountID,
			AccountName:          entry.AccountName,
			Scope:                ModelDowngradeBlockedScopeObserved,
			Status:               status,
			Cause:                entry.Cause,
			Model:                entry.SentModel,
			SentModel:            entry.SentModel,
			ResponseModel:        entry.ResponseModel,
			TriggerCount:         entry.TriggerCount,
			TriggerThreshold:     entry.TriggerThreshold,
			TriggerWindowMinutes: entry.TriggerWindowMinutes,
			TriggeredAt:          entry.ObservedAt,
			Until:                entry.ExpiresAt,
			Blocked:              entry.Blocked,
			Total:                entry.Total,
			MaxBlockedRatio:      entry.MaxBlockedRatio,
		})
	}
	return rows
}

// ReleaseModelDowngradeBlock 提前解除一条降级守卫写的限制，或清除一条观察记录。
//
// 刻意不复用 ClearTempUnschedulable：后者会顺手 ClearModelRateLimits 把整个
// extra.model_rate_limits 清空，会误伤图片模型等无关来源的 429 冷却。这里只解除
// 降级来源的那一条，scope=model 时只删对应的那个模型。
//
// 找不到降级来源的记录时返回 ErrModelDowngradeBlockNotFound，handler 映射成 404。
func (s *RateLimitService) ReleaseModelDowngradeBlock(ctx context.Context, accountID int64, scope, model string) error {
	if s == nil || s.accountRepo == nil {
		return errors.New("account repository not configured")
	}
	if accountID <= 0 {
		return ErrAccountNotFound
	}

	model = strings.TrimSpace(model)
	switch scope {
	case ModelDowngradeBlockedScopeAccount:
	case ModelDowngradeBlockedScopeModel:
		if model == "" {
			return ErrModelDowngradeBlockScopeInvalid
		}
	case ModelDowngradeBlockedScopeObserved:
		// 观察记录只存在 Redis 里：清除它既不碰库，也不动调度状态，
		// 因为这个账号本来就没有被限制过。
		if model == "" {
			return ErrModelDowngradeBlockScopeInvalid
		}
		if s.modelDowngradeObserved == nil {
			return ErrModelDowngradeGuardUnavailable
		}
		deleted, err := s.modelDowngradeObserved.DeleteObserved(ctx, accountID, model)
		if err != nil {
			return err
		}
		if !deleted {
			return ErrModelDowngradeBlockNotFound
		}
		return nil
	default:
		return ErrModelDowngradeBlockScopeInvalid
	}

	released, err := s.accountRepo.ReleaseOpenAIModelDowngradeBlock(ctx, accountID, scope, model)
	if err != nil {
		return err
	}
	if !released {
		return ErrModelDowngradeBlockNotFound
	}

	// 只有整账号下线才改变账号的可调度状态；仅屏蔽模型不碰 temp-unsched 缓存和
	// runtime blocker，否则会把别的原因写的整账号下线一起放行。
	if scope == ModelDowngradeBlockedScopeAccount {
		if s.tempUnschedCache != nil {
			if err := s.tempUnschedCache.DeleteTempUnsched(ctx, accountID); err != nil {
				slog.Warn("temp_unsched_cache_delete_failed", "account_id", accountID, "error", err)
			}
		}
		s.notifyAccountSchedulingBlockCleared(accountID)
	}
	return nil
}

// ModelDowngradeManualBlockResult 是管理员手动处理成功后的回显，
// 让前端知道实际落到了哪个范围、什么时候恢复。
type ModelDowngradeManualBlockResult struct {
	AccountID int64
	Scope     string
	Model     string
	Until     time.Time
}

// ApplyModelDowngradeBlockNow 把一条观察记录「转正」成真实处理。
//
// 切换处理方式时刻意不自动处理历史观察记录（记录可能是很久之前写的，批量处理会
// 误伤早就恢复正常的账号），转正只有两条路：账号再次降级时自然命中，或管理员在页面上
// 点「立即处理」走到这里。
//
// 动作取当前配置；观察模式（none）下按默认动作（仅屏蔽该模型）执行——管理员既然手动
// 点了，就是要处理，观察模式不该拦着。比例上限仍然生效，被拦下时返回 409。
//
// 配置绕开 30s 缓存直读：管理员往往是刚改完处理方式或比例上限就点「立即处理」，
// 缓存里的旧值会让这一下按上一份配置执行。
func (s *RateLimitService) ApplyModelDowngradeBlockNow(
	ctx context.Context,
	accountID int64,
	sentModel string,
) (*ModelDowngradeManualBlockResult, error) {
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("account repository not configured")
	}
	if accountID <= 0 {
		return nil, ErrAccountNotFound
	}
	sentModel = strings.TrimSpace(sentModel)
	if sentModel == "" {
		return nil, ErrModelDowngradeBlockScopeInvalid
	}

	settings, err := s.refreshModelDowngradeGuardSettings(ctx)
	if err != nil {
		return nil, err
	}
	action := settings.Action
	if action == ModelDowngradeGuardActionNone {
		action = modelDowngradeGuardDefaultAction
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}

	// 观察记录里存着当时的上游响应模型和命中次数，拿来填 reason；
	// 记录已经过期或从没写过也允许执行，此时 count 记 0、响应模型记 "unknown"。
	responseModel, count := modelDowngradeManualUnknownResponseModel, int64(0)
	if entry, ok := s.findModelDowngradeObserved(ctx, accountID, sentModel); ok {
		if trimmed := strings.TrimSpace(entry.ResponseModel); trimmed != "" {
			responseModel = trimmed
		}
		count = entry.TriggerCount
	}

	outcome, err := s.applyModelDowngradeBlock(ctx, modelDowngradeBlockInput{
		account:       account,
		settings:      settings,
		action:        action,
		sentModel:     sentModel,
		responseModel: responseModel,
		count:         count,
		manual:        true,
	})
	if err != nil {
		return nil, err
	}
	// 已经在受限中：管理员多半是对着一份没刷新的列表点的，409 直说比默默再写一条好。
	if outcome.AlreadyBlocked {
		slog.Info("model_downgrade_manual_block_already_active",
			"account_id", accountID,
			"scope", outcome.Scope,
			"sent_model", sentModel,
		)
		return nil, ErrModelDowngradeBlockAlreadyActive.WithMetadata(map[string]string{
			"scope": outcome.Scope,
		})
	}
	if !outcome.Applied {
		slog.Warn("model_downgrade_manual_block_ratio_cap_hit",
			"account_id", accountID,
			"scope", outcome.Scope,
			"blocked", outcome.Blocked,
			"total", outcome.Total,
			"max_blocked_ratio", settings.MaxBlockedRatio,
		)
		return nil, ErrModelDowngradeBlockRatioCapped.WithMetadata(map[string]string{
			"blocked":           strconv.FormatInt(outcome.Blocked, 10),
			"total":             strconv.FormatInt(outcome.Total, 10),
			"max_blocked_ratio": strconv.FormatFloat(settings.MaxBlockedRatio, 'f', -1, 64),
		})
	}

	slog.Info("account_model_downgrade_blocked_manually",
		"account_id", accountID,
		"sent_model", sentModel,
		"scope", outcome.Scope,
		"action", action,
		"until", outcome.Until,
	)

	return &ModelDowngradeManualBlockResult{
		AccountID: accountID,
		Scope:     outcome.Scope,
		Model:     outcome.Model,
		Until:     outcome.Until,
	}, nil
}

// findModelDowngradeObserved 在观察记录里找这条账号 + 模型的记录。
// 记录条数很少，直接扫一遍列表，不为此在缓存接口上多开一个 Get。
func (s *RateLimitService) findModelDowngradeObserved(ctx context.Context, accountID int64, sentModel string) (ModelDowngradeObservedEntry, bool) {
	if s.modelDowngradeObserved == nil {
		return ModelDowngradeObservedEntry{}, false
	}
	entries, err := s.modelDowngradeObserved.ListObserved(ctx)
	if err != nil {
		slog.Warn("model_downgrade_list_observed_failed", "account_id", accountID, "error", err)
		return ModelDowngradeObservedEntry{}, false
	}
	want := modelDowngradeObservedDedupeKey(accountID, sentModel)
	for _, entry := range entries {
		if modelDowngradeObservedDedupeKey(entry.AccountID, entry.SentModel) == want {
			return entry, true
		}
	}
	return ModelDowngradeObservedEntry{}, false
}
