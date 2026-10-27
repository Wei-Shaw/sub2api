package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// 智谱 GLM Coding Plan「用量重置额度」（重置卡）的查询与手动使用。
//
// 与额度探测共用 CNProviderQuotaService 的出站能力（URL 策略校验、代理、账号级
// 请求头覆写、httpUpstream），但端点、快照键和 singleflight key 都与探测分开：
//
//   - list：GET  /api/biz/customer-package-reset/list?targetType=PERSONAL
//   - use ：POST /api/biz/customer-package-reset/use
//
// 鉴权头与额度探测一致：Authorization 直接放 API Key，不加 Bearer 前缀。
// 每张卡独立有效期；用周卡会同时重置 5h 窗口。默认与官网一致，选最早到期的卡。
//
// 支持范围：只支持国内站（open.bigmodel.cn）个人版。国际站（z.ai）是否提供同一套
// 接口未经验证，团队版需要额外的组织/项目上下文，两者都直接拒绝、不发上游请求。
//
// 用卡是「消耗型」操作：卡一旦用掉，后续的快照刷新与账号状态恢复必须跑完，
// 不能因为发起请求的客户端断开而中止。所以 use → 刷新卡列表 → 强制重探额度
// → 恢复账号状态整条链路都在 singleflight 函数里用后台 ctx 顺序执行，
// handler 只负责等待并返回整体结果（客户端断开时流程照常跑完）。

const (
	ZhipuResetTypeWeek     = "WEEK"
	ZhipuResetTypeFiveHour = "FIVE_HOUR"

	zhipuResetTargetPersonal = "PERSONAL"
	zhipuResetListPath       = "/api/biz/customer-package-reset/list?targetType=" + zhipuResetTargetPersonal
	zhipuResetUsePath        = "/api/biz/customer-package-reset/use"
	// zhipuResetSupportedHost 是唯一支持重置卡的额度主机（国内站）。
	zhipuResetSupportedHost = "https://open.bigmodel.cn"

	// list + use + 用后刷新 list 三次上游请求的总预算（每次各自受 cnQuotaUpstreamTimeout 约束）。
	zhipuResetUseFlightTimeout = 3*cnQuotaUpstreamTimeout + 5*time.Second
	// 用卡成功后的后处理（强制重探 + 恢复账号状态）预算，与用卡阶段分开计时：
	// 用卡阶段耗尽预算也不能让已消耗的卡跳过后处理。
	zhipuResetPostProcessTimeout = cnQuotaUpstreamTimeout + 15*time.Second

	// Extra 快照键后缀（加 provider 前缀：zhipu_week_reset_available 等）。
	zhipuExtraSuffixWeekResetAvailable = "week_reset_available"
	zhipuExtraSuffixWeekResetExpireAt  = "week_reset_expire_at"
	zhipuExtraSuffixWeekResetCount     = "week_reset_count"
	zhipuExtraSuffix5hResetAvailable   = "5h_reset_available"
	zhipuExtraSuffix5hResetExpireAt    = "5h_reset_expire_at"
	zhipuExtraSuffix5hResetCount       = "5h_reset_count"
	zhipuExtraSuffixResetCardsUpdated  = "reset_cards_updated_at"
)

// 重置卡相关的机器可读错误码。
const (
	ZhipuResetErrUnsupported       = "CN_QUOTA_RESET_UNSUPPORTED"
	ZhipuResetErrRegionUnsupported = "CN_QUOTA_RESET_REGION_UNSUPPORTED"
	ZhipuResetErrTeamUnsupported   = "CN_QUOTA_RESET_TEAM_UNSUPPORTED"
	ZhipuResetErrInvalidType       = "CN_QUOTA_RESET_INVALID_TYPE"
	ZhipuResetErrInvalidRecord     = "CN_QUOTA_RESET_INVALID_RECORD"
	ZhipuResetErrNoCard            = "CN_QUOTA_RESET_NO_CARD"
	ZhipuResetErrCardNotAvailable  = "CN_QUOTA_RESET_CARD_NOT_AVAILABLE"
	ZhipuResetErrRequestFailed     = "CN_QUOTA_RESET_REQUEST_FAILED"
	ZhipuResetErrAuthFailed        = "CN_QUOTA_RESET_AUTH_FAILED"
	ZhipuResetErrUpstream          = "CN_QUOTA_RESET_UPSTREAM_ERROR"
	ZhipuResetErrRejected          = "CN_QUOTA_RESET_REJECTED"
	ZhipuResetErrInvalidResponse   = "CN_QUOTA_RESET_INVALID_RESPONSE"
)

// 用卡后处理的告警码（上游已消耗成功，但本地刷新/恢复部分失败）。
const (
	ZhipuResetWarningAccountRecoveryFailed = "account_state_recovery_failed"
	ZhipuResetWarningQuotaRefreshFailed    = "quota_refresh_failed"
)

// ZhipuResetCard 是一张可用的重置卡。
// ExpireTime 原样透传上游字符串（官网北京时间格式，未做时区换算）；
// 上游若给的是数字时间戳，则归一化为 RFC3339。
type ZhipuResetCard struct {
	RecordID   int64  `json:"record_id"`
	GrantType  string `json:"grant_type"`
	ExpireTime string `json:"expire_time,omitempty"`
	Available  bool   `json:"available"`
}

// ZhipuResetCards 是一次 list 的解析结果。
// WeekCards / FiveHourCards 只含 available=true 的卡（已用、已过期的记录被过滤），
// 按到期时间升序排列，首张即默认使用的卡。
type ZhipuResetCards struct {
	WeekCards             []ZhipuResetCard `json:"week_cards"`
	FiveHourCards         []ZhipuResetCard `json:"five_hour_cards"`
	LastWeekResetTime     string           `json:"last_week_reset_time,omitempty"`
	LastFiveHourResetTime string           `json:"last_five_hour_reset_time,omitempty"`
	FetchedAt             int64            `json:"fetched_at"`
	// Persisted 表示本次结果是否已落 extra 快照（写库失败不影响返回）。
	Persisted bool `json:"persisted"`
}

// ZhipuResetCardUseRequest 是手动用卡的入参。ResetType 缺省为 WEEK；
// RecordID 为 0 表示由服务端选最早到期的可用卡。
type ZhipuResetCardUseRequest struct {
	ResetType string `json:"reset_type"`
	RecordID  int64  `json:"record_id"`
}

// ZhipuResetCardUseResult 是一次用卡的结果。上游拒绝、鉴权失败、没有可用卡等
// 业务失败都表现为 Success=false + ErrorCode/Error，而不是 Go error。
type ZhipuResetCardUseResult struct {
	Provider  string `json:"provider"`
	Success   bool   `json:"success"`
	ResetType string `json:"reset_type"`
	// Window 是被重置的窗口："weekly"（周卡，同步重置 5h）或 "5h"。
	Window     string `json:"window"`
	RecordID   int64  `json:"record_id,omitempty"`
	ExpireTime string `json:"expire_time,omitempty"`
	// 剩余可用卡数：优先取用后刷新的列表；刷新失败时按用前列表扣掉本次那张推算。
	WeekResetsLeft     int              `json:"week_resets_left"`
	FiveHourResetsLeft int              `json:"five_hour_resets_left"`
	StatusCode         int              `json:"status_code,omitempty"`
	ErrorCode          string           `json:"error_code,omitempty"`
	Error              string           `json:"error,omitempty"`
	FetchedAt          int64            `json:"fetched_at"`
	Cards              *ZhipuResetCards `json:"cards,omitempty"`

	// 以下为用卡成功后的后处理结果（用卡失败时不执行，保持零值）。
	AccountStateRecovered bool                        `json:"account_state_recovered"`
	Probe                 *CNProviderQuotaProbeResult `json:"probe,omitempty"`
	WarningCode           string                      `json:"warning_code,omitempty"`
}

// ZhipuResetAccountRecoverer 是用卡后处理对 RateLimitService 的最小依赖面。
type ZhipuResetAccountRecoverer interface {
	RecoverAccountState(ctx context.Context, accountID int64, options AccountRecoveryOptions) (*SuccessfulTestRecoveryResult, error)
}

// zhipuResetUsageRefresher 是用卡后处理对额度探测的最小依赖面。
type zhipuResetUsageRefresher interface {
	RefreshUsageAfterReset(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error)
}

// SetZhipuResetRecoverer 注入用卡后恢复账号运行时状态的依赖（生产环境是 RateLimitService，
// 由 ProvideCNProviderQuotaService 注入）。未注入时用卡仍会成功，但响应带恢复失败告警。
func (s *CNProviderQuotaService) SetZhipuResetRecoverer(recoverer ZhipuResetAccountRecoverer) {
	if s == nil {
		return
	}
	s.zhipuResetRecoverer = recoverer
}

// zhipuResetFailure 是上游层面的失败（网络、鉴权、非 2xx、响应不可信、业务拒绝）。
type zhipuResetFailure struct {
	Code       string
	Message    string
	StatusCode int
}

// ListZhipuResetCards 拉取智谱账号的重置卡列表并落 extra 快照。
// 同一账号的并发 list 被 singleflight 合并（key 与额度探测区分开）。
func (s *CNProviderQuotaService) ListZhipuResetCards(ctx context.Context, accountID int64) (*ZhipuResetCards, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	account, err := s.loadCodingPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := validateZhipuResetAccount(account); err != nil {
		return nil, err
	}
	key := "zhipu_reset_cards:" + strconv.FormatInt(account.ID, 10)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		listCtx, cancel := context.WithTimeout(context.Background(), cnQuotaUpstreamTimeout+5*time.Second)
		defer cancel()
		cards, failure, err := s.listAndPersistZhipuResetCards(listCtx, account)
		if err != nil {
			return nil, err
		}
		if failure != nil {
			return nil, infraerrors.New(http.StatusBadGateway, failure.Code, failure.Message)
		}
		return cards, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		cards, ok := flightResult.Val.(*ZhipuResetCards)
		if !ok || cards == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_RESET_RESULT_INVALID", "invalid zhipu reset card list result")
		}
		return cloneZhipuResetCards(cards), nil
	}
}

// UseZhipuResetCard 消耗一张重置卡。流程：校验 → list → 选卡 → POST use → 刷新 list
// → （成功时）强制重探额度 → 恢复账号状态。
// 同一账号、同一卡种、同一 recordId 的并发请求被 singleflight 合并，双击只会消耗一张卡。
// 整条链路在后台 ctx 里执行：调用方 ctx 取消只会让本次调用提前返回 ctx.Err()，
// 已经开始的用卡与后处理照常跑完。
func (s *CNProviderQuotaService) UseZhipuResetCard(ctx context.Context, accountID int64, req ZhipuResetCardUseRequest) (*ZhipuResetCardUseResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	resetType, err := normalizeZhipuResetType(req.ResetType)
	if err != nil {
		return nil, err
	}
	if req.RecordID < 0 {
		return nil, infraerrors.New(http.StatusBadRequest, ZhipuResetErrInvalidRecord, "record_id must be a positive integer")
	}
	account, err := s.loadCodingPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := validateZhipuResetAccount(account); err != nil {
		return nil, err
	}

	key := fmt.Sprintf("zhipu_reset_use:%d:%s:%d", account.ID, resetType, req.RecordID)
	resultCh := s.flight.DoChan(key, func() (any, error) {
		useCtx, cancelUse := context.WithTimeout(context.Background(), zhipuResetUseFlightTimeout)
		result, err := s.useZhipuResetCard(useCtx, account, resetType, req.RecordID)
		cancelUse()
		if err != nil || result == nil || !result.Success {
			return result, err
		}
		postCtx, cancelPost := context.WithTimeout(context.Background(), zhipuResetPostProcessTimeout)
		defer cancelPost()
		post := runZhipuResetCardPostProcess(postCtx, account.ID, s, s.zhipuResetRecoverer)
		result.AccountStateRecovered = post.AccountStateRecovered
		result.Probe = post.Probe
		result.WarningCode = post.WarningCode
		return result, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case flightResult := <-resultCh:
		if flightResult.Err != nil {
			return nil, flightResult.Err
		}
		result, ok := flightResult.Val.(*ZhipuResetCardUseResult)
		if !ok || result == nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_RESET_RESULT_INVALID", "invalid zhipu reset card use result")
		}
		cloned := *result
		cloned.Cards = cloneZhipuResetCards(result.Cards)
		if result.Probe != nil {
			probe := *result.Probe
			cloned.Probe = &probe
		}
		return &cloned, nil
	}
}

func (s *CNProviderQuotaService) useZhipuResetCard(ctx context.Context, account *Account, resetType string, recordID int64) (*ZhipuResetCardUseResult, error) {
	result := &ZhipuResetCardUseResult{
		Provider:  PlatformZhipu,
		ResetType: resetType,
		Window:    zhipuResetWindow(resetType),
		FetchedAt: time.Now().UTC().Unix(),
	}
	fail := func(f *zhipuResetFailure) (*ZhipuResetCardUseResult, error) {
		result.ErrorCode = f.Code
		result.Error = f.Message
		if f.StatusCode != 0 {
			result.StatusCode = f.StatusCode
		}
		return result, nil
	}

	before, failure, err := s.listAndPersistZhipuResetCards(ctx, account)
	if err != nil {
		return nil, err
	}
	if failure != nil {
		return fail(failure)
	}
	result.Cards = before
	result.WeekResetsLeft = len(before.WeekCards)
	result.FiveHourResetsLeft = len(before.FiveHourCards)

	card, failure := selectZhipuResetCard(before, resetType, recordID)
	if failure != nil {
		return fail(failure)
	}
	result.RecordID = card.RecordID
	result.ExpireTime = card.ExpireTime

	payload, err := json.Marshal(map[string]any{
		"targetType": zhipuResetTargetPersonal,
		"resetType":  resetType,
		"recordId":   card.RecordID,
		"grantType":  card.GrantType,
		"requestId":  uuid.NewString(),
	})
	if err != nil {
		return nil, infraerrors.Newf(http.StatusInternalServerError, "CN_QUOTA_REQUEST_BUILD_FAILED", "build request body: %v", err)
	}
	status, body, failure, err := s.doZhipuResetRequest(ctx, account, http.MethodPost, zhipuResetUsePath, payload)
	if err != nil {
		return nil, err
	}
	result.StatusCode = status
	if failure == nil {
		failure = zhipuResetUseResponseFailure(body)
	}

	// 请求已发出：无论成败都刷新一次卡列表。失败时（尤其是网络错误这种结果不确定
	// 的情况）也要让快照跟上游对齐，避免前端继续展示一张其实已经用掉的卡。
	after, refreshFailure, refreshErr := s.listAndPersistZhipuResetCards(ctx, account)
	if refreshErr == nil && refreshFailure == nil {
		result.Cards = after
		result.WeekResetsLeft = len(after.WeekCards)
		result.FiveHourResetsLeft = len(after.FiveHourCards)
	} else {
		slog.Warn("zhipu_reset_cards_refresh_after_use_failed",
			"account_id", account.ID, "reset_type", resetType, "use_succeeded", failure == nil,
			"error", zhipuResetRefreshErrorText(refreshFailure, refreshErr))
	}

	if failure != nil {
		slog.Warn("zhipu_reset_card_use_failed",
			"account_id", account.ID, "reset_type", resetType, "record_id", card.RecordID,
			"status", status, "error_code", failure.Code)
		return fail(failure)
	}

	result.Success = true
	result.StatusCode = status
	if refreshErr != nil || refreshFailure != nil {
		// 刷新失败：按用前列表扣掉本次那张，推算剩余并落快照，别让 UI 继续显示用掉的卡。
		derived := zhipuResetCardsWithout(before, resetType, card.RecordID, time.Now().UTC())
		if err := s.accountRepo.UpdateExtra(ctx, account.ID, zhipuResetCardsExtraUpdates(derived)); err != nil {
			slog.Warn("zhipu_reset_cards_persist_failed", "account_id", account.ID, "error", err)
		} else {
			derived.Persisted = true
		}
		result.Cards = derived
		result.WeekResetsLeft = len(derived.WeekCards)
		result.FiveHourResetsLeft = len(derived.FiveHourCards)
	}
	slog.Info("zhipu_reset_card_used",
		"account_id", account.ID, "reset_type", resetType, "record_id", card.RecordID,
		"week_left", result.WeekResetsLeft, "five_hour_left", result.FiveHourResetsLeft)
	return result, nil
}

// listAndPersistZhipuResetCards 请求 list 并在成功时落快照。
// 返回值：err 为配置/校验级错误（URL 被策略拒绝、缺 API Key 等），failure 为上游失败。
func (s *CNProviderQuotaService) listAndPersistZhipuResetCards(ctx context.Context, account *Account) (*ZhipuResetCards, *zhipuResetFailure, error) {
	status, body, failure, err := s.doZhipuResetRequest(ctx, account, http.MethodGet, zhipuResetListPath, nil)
	if err != nil || failure != nil {
		return nil, failure, err
	}
	cards, failure := parseZhipuResetCards(body, time.Now().UTC())
	if failure != nil {
		failure.StatusCode = status
		return nil, failure, nil
	}
	if err := s.accountRepo.UpdateExtra(ctx, account.ID, zhipuResetCardsExtraUpdates(cards)); err != nil {
		slog.Warn("zhipu_reset_cards_persist_failed", "account_id", account.ID, "error", err)
	} else {
		cards.Persisted = true
	}
	return cards, nil, nil
}

// doZhipuResetRequest 发送一次重置卡相关的上游请求，并完成 HTTP 层面的判定
// （网络错误、401/403、非 2xx、非 JSON）。业务层 success/code 由调用方判断。
func (s *CNProviderQuotaService) doZhipuResetRequest(ctx context.Context, account *Account, method, path string, payload []byte) (int, []byte, *zhipuResetFailure, error) {
	apiKey := strings.TrimSpace(account.GetCNAPIKey())
	if apiKey == "" {
		return 0, nil, nil, infraerrors.New(http.StatusBadRequest, "CN_QUOTA_NO_APIKEY", "account api_key is empty")
	}
	// 与额度探测同一套出站 URL 安全策略：不得把 API key 发往策略外主机。
	targetURL, err := cnValidateProbeURL(s.cfg, zhipuResetSupportedHost+path)
	if err != nil {
		return 0, nil, nil, infraerrors.New(http.StatusForbidden, "CN_QUOTA_URL_REJECTED", err.Error())
	}

	proxyURL := s.resolveProxyURL(ctx, account)
	callCtx, cancel := context.WithTimeout(ctx, cnQuotaUpstreamTimeout)
	defer cancel()
	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(callCtx, method, targetURL, reqBody)
	if err != nil {
		return 0, nil, nil, infraerrors.Newf(http.StatusInternalServerError, "CN_QUOTA_REQUEST_BUILD_FAILED", "build request: %v", err)
	}
	req.Header.Set("Authorization", apiKey) // 智谱数据面鉴权不加 Bearer 前缀
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "en-US,en")
	account.ApplyHeaderOverrides(req.Header)

	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, maxInt(account.Concurrency, 1))
	if err != nil {
		return 0, nil, &zhipuResetFailure{
			Code:    ZhipuResetErrRequestFailed,
			Message: fmt.Sprintf("upstream request failed: %v", err),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, cnQuotaMaxBodyBytes))

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return resp.StatusCode, body, &zhipuResetFailure{
			Code:       ZhipuResetErrAuthFailed,
			Message:    fmt.Sprintf("Authentication failed (HTTP %d)", resp.StatusCode),
			StatusCode: resp.StatusCode,
		}, nil
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return resp.StatusCode, body, &zhipuResetFailure{
			Code:       ZhipuResetErrUpstream,
			Message:    fmt.Sprintf("API error (HTTP %d): %s", resp.StatusCode, truncate(strings.TrimSpace(string(body)), 240)),
			StatusCode: resp.StatusCode,
		}, nil
	case !gjson.ValidBytes(body):
		return resp.StatusCode, body, &zhipuResetFailure{
			Code:       ZhipuResetErrInvalidResponse,
			Message:    zhipuResetInvalidResponseError("response body is not valid JSON"),
			StatusCode: resp.StatusCode,
		}, nil
	}
	return resp.StatusCode, body, nil, nil
}

// parseZhipuResetCards 解析 list 响应。业务失败（success=false）与结构不可信都返回
// failure，不产出结果——调用方据此不落快照，避免一个异常响应把有效的卡快照清空。
func parseZhipuResetCards(body []byte, now time.Time) (*ZhipuResetCards, *zhipuResetFailure) {
	if failure := zhipuResetBusinessFailure(body); failure != nil {
		return nil, failure
	}
	data := gjson.GetBytes(body, "data")
	if !data.IsObject() {
		return nil, &zhipuResetFailure{Code: ZhipuResetErrInvalidResponse, Message: zhipuResetInvalidResponseError(`missing "data" object`)}
	}
	// 两个列表的形状约定：
	//   - 字段缺失或为 JSON null：视为该卡种没有记录（空列表）；
	//   - 字段存在且非 null：必须是数组，否则整份响应判为不可信（{}、字符串、数字等）；
	//   - 两个字段都缺失/为 null：同样判为不可信——这更像上游改了字段名，
	//     当成「没有卡」会把有效快照清成 0 并隐藏用卡按钮。
	weekNode, weekIssue := zhipuResetListNode(data, "weekResets")
	fiveHourNode, fiveHourIssue := zhipuResetListNode(data, "fiveHourResets")
	for _, issue := range []string{weekIssue, fiveHourIssue} {
		if issue != "" {
			return nil, &zhipuResetFailure{Code: ZhipuResetErrInvalidResponse, Message: zhipuResetInvalidResponseError(issue)}
		}
	}
	if !weekNode.IsArray() && !fiveHourNode.IsArray() {
		return nil, &zhipuResetFailure{Code: ZhipuResetErrInvalidResponse, Message: zhipuResetInvalidResponseError(`missing "weekResets" and "fiveHourResets" arrays`)}
	}
	return &ZhipuResetCards{
		WeekCards:             parseZhipuResetCardList(weekNode),
		FiveHourCards:         parseZhipuResetCardList(fiveHourNode),
		LastWeekResetTime:     zhipuResetTimeString(data.Get("lastWeekResetTime")),
		LastFiveHourResetTime: zhipuResetTimeString(data.Get("lastFiveHourResetTime")),
		FetchedAt:             now.Unix(),
	}, nil
}

// zhipuResetListNode 取出卡列表节点并校验形状：缺失或 null 返回空节点（无问题）；
// 存在但不是数组返回问题描述。
func zhipuResetListNode(data gjson.Result, field string) (gjson.Result, string) {
	node := data.Get(field)
	if !node.Exists() || node.Type == gjson.Null {
		return gjson.Result{}, ""
	}
	if !node.IsArray() {
		return gjson.Result{}, fmt.Sprintf("%q is not an array", field)
	}
	return node, ""
}

// parseZhipuResetCardList 只保留 available=true 且带合法 recordId 的记录，按到期时间升序。
// 调用方已保证 node 是数组或空节点（缺失/null）。
func parseZhipuResetCardList(node gjson.Result) []ZhipuResetCard {
	cards := make([]ZhipuResetCard, 0)
	if !node.IsArray() {
		return cards
	}
	node.ForEach(func(_, item gjson.Result) bool {
		if !item.Get("available").Bool() {
			return true
		}
		recordID := item.Get("recordId").Int()
		if recordID <= 0 {
			slog.Warn("zhipu_reset_card_missing_record_id", "raw", truncate(item.Raw, 200))
			return true
		}
		cards = append(cards, ZhipuResetCard{
			RecordID:   recordID,
			GrantType:  strings.TrimSpace(item.Get("grantType").String()),
			ExpireTime: zhipuResetTimeString(item.Get("expireTime")),
			Available:  true,
		})
		return true
	})
	sortZhipuResetCards(cards)
	return cards
}

// zhipuResetBusinessFailure 识别智谱业务级失败（HTTP 2xx 但 success=false 或 code 非 200）。
func zhipuResetBusinessFailure(body []byte) *zhipuResetFailure {
	success := gjson.GetBytes(body, "success")
	code := gjson.GetBytes(body, "code")
	if (success.Exists() && !success.Bool()) || (code.Exists() && code.Int() != http.StatusOK) {
		msg := strings.TrimSpace(gjson.GetBytes(body, "msg").String())
		if msg == "" {
			msg = "unknown zhipu reset error"
		}
		return &zhipuResetFailure{Code: ZhipuResetErrRejected, Message: "API error: " + msg}
	}
	return nil
}

// zhipuResetUseResponseFailure 判定 use 响应：只有 code==200 && success==true 才算成功。
func zhipuResetUseResponseFailure(body []byte) *zhipuResetFailure {
	if failure := zhipuResetBusinessFailure(body); failure != nil {
		return failure
	}
	if gjson.GetBytes(body, "code").Int() != http.StatusOK || !gjson.GetBytes(body, "success").Bool() {
		return &zhipuResetFailure{Code: ZhipuResetErrInvalidResponse, Message: zhipuResetInvalidResponseError(`missing "code":200 / "success":true`)}
	}
	return nil
}

func zhipuResetInvalidResponseError(detail string) string {
	return ZhipuResetErrInvalidResponse + ": unexpected upstream payload (" + detail + ")"
}

// zhipuResetTimeString 字符串原样透传；数字时间戳（秒/毫秒）归一化为 RFC3339。
func zhipuResetTimeString(node gjson.Result) string {
	switch node.Type {
	case gjson.String:
		return strings.TrimSpace(node.String())
	case gjson.Number:
		return cnMillisToRFC3339(node.Int())
	default:
		return ""
	}
}

// zhipuResetExpireLocation 是官网展示到期时间所用的时区（北京时间，无夏令时）。
var zhipuResetExpireLocation = time.FixedZone("CST", 8*3600)

// parseZhipuResetExpireTime 尽力解析到期时间：带时区的 ISO 串按其时区，
// 不带时区的「2006-01-02 15:04:05」类格式按北京时间。
func parseZhipuResetExpireTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if ts, err := parseSchedulingTime(raw); err == nil {
		return ts, true
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		"2006/01/02 15:04:05",
	} {
		if ts, err := time.ParseInLocation(layout, raw, zhipuResetExpireLocation); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

// sortZhipuResetCards 按到期时间升序：可解析的在前按时间排；解析失败的排后面按字符串序；
// 同时间再按字符串、recordId 兜底，保证顺序稳定。
func sortZhipuResetCards(cards []ZhipuResetCard) {
	sort.SliceStable(cards, func(i, j int) bool {
		ti, okI := parseZhipuResetExpireTime(cards[i].ExpireTime)
		tj, okJ := parseZhipuResetExpireTime(cards[j].ExpireTime)
		if okI != okJ {
			return okI
		}
		if okI && !ti.Equal(tj) {
			return ti.Before(tj)
		}
		if cards[i].ExpireTime != cards[j].ExpireTime {
			return cards[i].ExpireTime < cards[j].ExpireTime
		}
		return cards[i].RecordID < cards[j].RecordID
	})
}

// selectZhipuResetCard 选卡：指定 recordId 时必须在该卡种的可用列表里；否则取最早到期的。
func selectZhipuResetCard(cards *ZhipuResetCards, resetType string, recordID int64) (ZhipuResetCard, *zhipuResetFailure) {
	var pool []ZhipuResetCard
	if cards != nil {
		if resetType == ZhipuResetTypeFiveHour {
			pool = cards.FiveHourCards
		} else {
			pool = cards.WeekCards
		}
	}
	if recordID > 0 {
		for _, card := range pool {
			if card.RecordID == recordID {
				return card, nil
			}
		}
		return ZhipuResetCard{}, &zhipuResetFailure{
			Code:    ZhipuResetErrCardNotAvailable,
			Message: fmt.Sprintf("reset card %d is not an available %s card; refresh the card list and retry", recordID, resetType),
		}
	}
	if len(pool) == 0 {
		return ZhipuResetCard{}, &zhipuResetFailure{
			Code:    ZhipuResetErrNoCard,
			Message: fmt.Sprintf("no available %s reset card", resetType),
		}
	}
	sorted := append([]ZhipuResetCard(nil), pool...)
	sortZhipuResetCards(sorted)
	return sorted[0], nil
}

// zhipuResetCardsExtraUpdates 构造重置卡的 extra 快照。
// 没有可用卡时 available=false、count=0、expire_at 置 nil：UpdateExtra 走 JSONB `||`
// 合并删不掉键，读侧把 null 当缺失。
func zhipuResetCardsExtraUpdates(cards *ZhipuResetCards) map[string]any {
	updates := map[string]any{
		cnExtraKey(PlatformZhipu, zhipuExtraSuffixResetCardsUpdated): time.Unix(cards.FetchedAt, 0).UTC().Format(time.RFC3339),
	}
	put := func(list []ZhipuResetCard, availableSuffix, expireSuffix, countSuffix string) {
		updates[cnExtraKey(PlatformZhipu, availableSuffix)] = len(list) > 0
		updates[cnExtraKey(PlatformZhipu, countSuffix)] = len(list)
		if len(list) > 0 && list[0].ExpireTime != "" {
			updates[cnExtraKey(PlatformZhipu, expireSuffix)] = list[0].ExpireTime
		} else {
			updates[cnExtraKey(PlatformZhipu, expireSuffix)] = nil
		}
	}
	put(cards.WeekCards, zhipuExtraSuffixWeekResetAvailable, zhipuExtraSuffixWeekResetExpireAt, zhipuExtraSuffixWeekResetCount)
	put(cards.FiveHourCards, zhipuExtraSuffix5hResetAvailable, zhipuExtraSuffix5hResetExpireAt, zhipuExtraSuffix5hResetCount)
	return updates
}

// zhipuResetCardsWithout 返回去掉某张卡后的副本（用后刷新失败时推算剩余用）。
func zhipuResetCardsWithout(cards *ZhipuResetCards, resetType string, recordID int64, now time.Time) *ZhipuResetCards {
	out := cloneZhipuResetCards(cards)
	if out == nil {
		out = &ZhipuResetCards{WeekCards: []ZhipuResetCard{}, FiveHourCards: []ZhipuResetCard{}}
	}
	remove := func(list []ZhipuResetCard) []ZhipuResetCard {
		kept := make([]ZhipuResetCard, 0, len(list))
		for _, card := range list {
			if card.RecordID != recordID {
				kept = append(kept, card)
			}
		}
		return kept
	}
	if resetType == ZhipuResetTypeFiveHour {
		out.FiveHourCards = remove(out.FiveHourCards)
	} else {
		out.WeekCards = remove(out.WeekCards)
	}
	out.FetchedAt = now.Unix()
	out.Persisted = false
	return out
}

func cloneZhipuResetCards(cards *ZhipuResetCards) *ZhipuResetCards {
	if cards == nil {
		return nil
	}
	cloned := *cards
	cloned.WeekCards = append(make([]ZhipuResetCard, 0, len(cards.WeekCards)), cards.WeekCards...)
	cloned.FiveHourCards = append(make([]ZhipuResetCard, 0, len(cards.FiveHourCards)), cards.FiveHourCards...)
	return &cloned
}

func zhipuResetRefreshErrorText(failure *zhipuResetFailure, err error) string {
	if err != nil {
		return err.Error()
	}
	if failure != nil {
		return failure.Code + ": " + failure.Message
	}
	return ""
}

// normalizeZhipuResetType 归一化卡种：缺省 WEEK，大小写不敏感。
func normalizeZhipuResetType(raw string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", ZhipuResetTypeWeek:
		return ZhipuResetTypeWeek, nil
	case ZhipuResetTypeFiveHour:
		return ZhipuResetTypeFiveHour, nil
	default:
		return "", infraerrors.New(http.StatusBadRequest, ZhipuResetErrInvalidType, `reset_type must be "WEEK" or "FIVE_HOUR"`)
	}
}

func zhipuResetWindow(resetType string) string {
	if resetType == ZhipuResetTypeFiveHour {
		return "5h"
	}
	return "weekly"
}

// validateZhipuResetAccount 在 Coding Plan 校验之上要求：智谱供应商、国内站、个人版。
func validateZhipuResetAccount(account *Account) error {
	if err := validateCodingPlanAccount(account); err != nil {
		return err
	}
	if account.GetCodingPlanProvider() != PlatformZhipu {
		return infraerrors.New(http.StatusBadRequest, ZhipuResetErrUnsupported, "reset cards are only supported for zhipu GLM coding plan accounts")
	}
	if zhipuQuotaHost(account.GetOpenAIBaseURL()) != zhipuResetSupportedHost {
		return infraerrors.New(http.StatusBadRequest, ZhipuResetErrRegionUnsupported, "reset cards are only supported for zhipu accounts on open.bigmodel.cn")
	}
	if strings.TrimSpace(account.GetCredential("zhipu_organization")) != "" {
		return infraerrors.New(http.StatusBadRequest, ZhipuResetErrTeamUnsupported, "reset cards are not supported for zhipu team coding plan accounts yet")
	}
	return nil
}

// RefreshUsageAfterReset 用卡后强制重新探测额度并落快照。
// 不复用 QueryUsage 的 singleflight key：用卡前已在途的探测可能读到用卡前的用量，
// 合并进去会把旧数据当成用卡后的结果。
func (s *CNProviderQuotaService) RefreshUsageAfterReset(ctx context.Context, accountID int64) (*CNProviderQuotaProbeResult, error) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "CN_QUOTA_NOT_CONFIGURED", "cn provider quota service is not configured")
	}
	account, err := s.loadCodingPlanAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	key := "cn_quota_post_reset:" + strconv.FormatInt(account.ID, 10)
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

// zhipuResetPostProcessResult 汇总用卡后的本地后处理结果。
type zhipuResetPostProcessResult struct {
	AccountStateRecovered bool
	Probe                 *CNProviderQuotaProbeResult
	WarningCode           string
}

// runZhipuResetCardPostProcess 用卡成功后的本地后处理，顺序固定为：
//
//  1. 强制重新探测额度并落快照。必须在解封之前：阈值停调按 extra 快照判定，
//     如果先解封、再刷新，中间进来的请求会按旧的高用量快照把号重新停掉。
//  2. 恢复账号运行时状态（清额度耗尽冷却 / 临时停调 / 限流）。
//
// 刷新失败不阻断恢复：卡已经用掉，账号状态必须恢复。
func runZhipuResetCardPostProcess(
	ctx context.Context,
	accountID int64,
	refresher zhipuResetUsageRefresher,
	recoverer ZhipuResetAccountRecoverer,
) zhipuResetPostProcessResult {
	result := zhipuResetPostProcessResult{}

	quotaWarning := ""
	probe, err := refresher.RefreshUsageAfterReset(ctx, accountID)
	switch {
	case err != nil || probe == nil:
		slog.Warn("zhipu_reset_quota_refresh_failed", "account_id", accountID, "error_code", infraerrors.Reason(err), "error", err)
		quotaWarning = ZhipuResetWarningQuotaRefreshFailed
	default:
		result.Probe = probe
		if !probe.Success || !probe.Persisted {
			quotaWarning = ZhipuResetWarningQuotaRefreshFailed
		}
	}

	if recoverer == nil {
		result.WarningCode = ZhipuResetWarningAccountRecoveryFailed
		return result
	}
	if _, err := recoverer.RecoverAccountState(ctx, accountID, AccountRecoveryOptions{}); err != nil {
		slog.Warn("zhipu_reset_account_recovery_failed", "account_id", accountID, "error_code", infraerrors.Reason(err), "error", err)
		result.WarningCode = ZhipuResetWarningAccountRecoveryFailed
		return result
	}
	result.AccountStateRecovered = true
	result.WarningCode = quotaWarning
	return result
}
