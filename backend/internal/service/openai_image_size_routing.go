package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// AccountExtraOpenAIImageSizeTiers 是账号允许承接的生图尺寸档位列表，档位口径与
// ClassifyImageBillingTier 一致（1K/2K/4K）。缺省、空列表或三档齐全表示不限。
const AccountExtraOpenAIImageSizeTiers = "openai_image_size_tiers"

const openAIImageSizeTierUnsupportedReason = "image_size_tier_unsupported"

var openAIImageSizeTierOrder = []string{ImageBillingSize1K, ImageBillingSize2K, ImageBillingSize4K}

// openAIImageSizeTestSizes 是测试连接按账号最低允许档位发送的尺寸，各档取该档内的常用尺寸。
var openAIImageSizeTestSizes = map[string]string{
	ImageBillingSize1K: "1024x1024",
	ImageBillingSize2K: "1536x1024",
	ImageBillingSize4K: "3840x2160",
}

// OpenAIImageSizeTiers 按 1K、2K、4K 顺序返回账号限定的尺寸档位；不限时返回 nil。
func (a *Account) OpenAIImageSizeTiers() []string {
	if a == nil || a.Extra == nil {
		return nil
	}
	var values []string
	switch raw := a.Extra[AccountExtraOpenAIImageSizeTiers].(type) {
	case []any:
		for _, item := range raw {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	case []string:
		values = raw
	}
	tiers := make([]string, 0, len(openAIImageSizeTierOrder))
	for _, tier := range openAIImageSizeTierOrder {
		for _, value := range values {
			if strings.EqualFold(strings.TrimSpace(value), tier) {
				tiers = append(tiers, tier)
				break
			}
		}
	}
	if len(tiers) == 0 || len(tiers) == len(openAIImageSizeTierOrder) {
		return nil
	}
	return tiers
}

// SupportsOpenAIImageSizeTier 报告账号是否允许承接该尺寸档位。
func (a *Account) SupportsOpenAIImageSizeTier(tier string) bool {
	tiers := a.OpenAIImageSizeTiers()
	return tiers == nil || slices.Contains(tiers, tier)
}

// OpenAIImageTestSize 返回测试连接应发送的尺寸：不限档位时为空（沿用上游默认），
// 否则取账号最低允许档位的代表尺寸。
func (a *Account) OpenAIImageTestSize() string {
	tiers := a.OpenAIImageSizeTiers()
	if len(tiers) == 0 {
		return ""
	}
	return openAIImageSizeTestSizes[tiers[0]]
}

type openAIImageSizeRoutingCtxKey struct{}

type openAIImageSizePassCtxKey struct{}

// WithOpenAIImageSizeRouting 记录生图请求的尺寸参数，OpenAI 选号据此按账号尺寸档位分流。
func WithOpenAIImageSizeRouting(ctx context.Context, size string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIImageSizeRoutingCtxKey{}, strings.TrimSpace(size))
}

func openAIImageSizeRoutingFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	size, ok := ctx.Value(openAIImageSizeRoutingCtxKey{}).(string)
	return size, ok
}

type openAIImageSizeStickyAccountsCtxKey struct{}

// withOpenAIImageSizeStickyAccount 在未指定尺寸的选号轮次中标记请求已粘在 accountID 上
// （会话粘性、previous_response_id），档位偏好不约束这些账号；其他请求原样返回 ctx。
func withOpenAIImageSizeStickyAccount(ctx context.Context, accountID int64) context.Context {
	if ctx == nil || accountID <= 0 {
		return ctx
	}
	if pass, _ := ctx.Value(openAIImageSizePassCtxKey{}).(*openAIImageSizePass); pass == nil || pass.tier != "" {
		return ctx
	}
	prev, _ := ctx.Value(openAIImageSizeStickyAccountsCtxKey{}).([]int64)
	if slices.Contains(prev, accountID) {
		return ctx
	}
	ids := make([]int64, len(prev), len(prev)+1)
	copy(ids, prev)
	return context.WithValue(ctx, openAIImageSizeStickyAccountsCtxKey{}, append(ids, accountID))
}

func isOpenAIImageSizeStickyAccount(ctx context.Context, account *Account) bool {
	if ctx == nil || account == nil {
		return false
	}
	ids, _ := ctx.Value(openAIImageSizeStickyAccountsCtxKey{}).([]int64)
	return slices.Contains(ids, account.ID)
}

// openAIImageSizePass 是一轮选号的尺寸档位约束。
type openAIImageSizePass struct {
	// tier 非空时只放行允许该档位的账号（请求尺寸可识别）。
	tier string
	// rank 在 tier 为空时生效：只放行档位偏好层级等于 rank 的账号，
	// 请求已粘在其上的账号（会话粘性、previous_response_id）不受约束。
	rank int
	// queuedIDs 是较早轮次只能排队的账号，本轮不再选它们。
	queuedIDs map[int64]struct{}
	// nextRank 是本轮排除过的后续层级中最小的一级；为 openAIImageSizePreferenceRanks 时后续各轮没有候选。
	nextRank atomic.Int32
}

func (p *openAIImageSizePass) noteLaterRank(rank int) {
	for {
		current := p.nextRank.Load()
		if int32(rank) >= current || p.nextRank.CompareAndSwap(current, int32(rank)) {
			return
		}
	}
}

// openAIImageSizePreferenceRanks 是未指定尺寸请求的选号层级数，见 openAIImageSizePreferenceRank。
const openAIImageSizePreferenceRanks = 4

// openAIImageSizePreferenceRank 返回账号在未指定尺寸请求中的偏好层级：
// 不限档位为 0，允许 4K 为 1，允许 2K（不含 4K）为 2，仅 1K 为 3。
func openAIImageSizePreferenceRank(tiers []string) int {
	switch {
	case tiers == nil:
		return 0
	case slices.Contains(tiers, ImageBillingSize4K):
		return 1
	case slices.Contains(tiers, ImageBillingSize2K):
		return 2
	default:
		return 3
	}
}

// openAIImageSizeVetoReason 判定账号是否被当前选号轮次的尺寸档位约束排除。
func openAIImageSizeVetoReason(ctx context.Context, account *Account) (bool, string) {
	if ctx == nil || account == nil {
		return false, ""
	}
	pass, _ := ctx.Value(openAIImageSizePassCtxKey{}).(*openAIImageSizePass)
	if pass == nil {
		return false, ""
	}
	tiers := account.OpenAIImageSizeTiers()
	if pass.tier != "" {
		if tiers == nil || slices.Contains(tiers, pass.tier) {
			return false, ""
		}
		return true, openAIImageSizeTierUnsupportedReason
	}
	if _, queued := pass.queuedIDs[account.ID]; queued {
		return true, openAIImageSizeTierUnsupportedReason
	}
	if isOpenAIImageSizeStickyAccount(ctx, account) {
		return false, ""
	}
	rank := openAIImageSizePreferenceRank(tiers)
	if rank == pass.rank {
		return false, ""
	}
	if rank > pass.rank {
		pass.noteLaterRank(rank)
	}
	return true, openAIImageSizeTierUnsupportedReason
}

// openAIImageSizeSpilloverKeepsStickyBinding 报告本轮是否保留会话已有的粘性绑定：较早轮次已只拿到
// 排队结果时，本轮选中的账号只是临时溢出，不改写已有绑定；会话尚无绑定时照常绑定。
func (s *OpenAIGatewayService) openAIImageSizeSpilloverKeepsStickyBinding(ctx context.Context, groupID *int64, sessionHash string) bool {
	if ctx == nil || strings.TrimSpace(sessionHash) == "" {
		return false
	}
	pass, _ := ctx.Value(openAIImageSizePassCtxKey{}).(*openAIImageSizePass)
	if pass == nil || len(pass.queuedIDs) == 0 {
		return false
	}
	accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash)
	return err == nil && accountID > 0
}

type openAIAccountSelectFunc func(ctx context.Context) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error)

// selectOpenAIAccountByImageSizePasses 按尺寸档位约束选号，ctx 未记录生图尺寸时直接选号。
// 可识别档位的请求只选一轮，只放行允许该档位的账号。未传、auto 或无法识别的尺寸按偏好层级逐层选号：
// 先选不限档位的账号，再依次降级到允许 4K、2K、1K 的账号，跳过本轮未见到账号的层级。
// 较早轮次抢到槽位的结果优先；某轮只能排队时把该账号留作后备并继续尝试后续层级，
// 都抢不到槽位时返回最早的排队结果。返回的 LatencyMs 是各轮合计耗时。
func selectOpenAIAccountByImageSizePasses(ctx context.Context, selectFn openAIAccountSelectFunc) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
	size, ok := openAIImageSizeRoutingFromContext(ctx)
	if !ok {
		return selectFn(ctx)
	}
	if tier, explicit := ClassifyImageBillingTier(size); explicit {
		return selectFn(context.WithValue(ctx, openAIImageSizePassCtxKey{}, &openAIImageSizePass{tier: tier}))
	}
	start := time.Now()
	var (
		waiting         *AccountSelectionResult
		waitingDecision OpenAIAccountScheduleDecision
		lastDecision    OpenAIAccountScheduleDecision
		lastErr         error
	)
	queuedIDs := make(map[int64]struct{})
	finish := func(selection *AccountSelectionResult, decision OpenAIAccountScheduleDecision, err error) (*AccountSelectionResult, OpenAIAccountScheduleDecision, error) {
		decision.LatencyMs = time.Since(start).Milliseconds()
		return selection, decision, err
	}
	for rank := 0; rank < openAIImageSizePreferenceRanks; {
		pass := &openAIImageSizePass{rank: rank, queuedIDs: queuedIDs}
		pass.nextRank.Store(openAIImageSizePreferenceRanks)
		selection, decision, err := selectFn(context.WithValue(ctx, openAIImageSizePassCtxKey{}, pass))
		switch {
		case err == nil && selection != nil && selection.Account != nil:
			if selection.Acquired || selection.WaitPlan == nil {
				return finish(selection, decision, nil)
			}
			if waiting == nil {
				waiting, waitingDecision = selection, decision
			}
			queuedIDs[selection.Account.ID] = struct{}{}
		case err != nil && !errors.Is(err, ErrNoAvailableAccounts) && !errors.Is(err, ErrNoAvailableCompactAccounts):
			if waiting != nil {
				return finish(waiting, waitingDecision, nil)
			}
			return finish(selection, decision, err)
		default:
			lastDecision, lastErr = decision, err
		}
		rank = int(pass.nextRank.Load())
	}
	if waiting != nil {
		return finish(waiting, waitingDecision, nil)
	}
	return finish(nil, lastDecision, lastErr)
}

// OpenAIImageSizeTierUnsupported 报告分组内持久可调度、支持该模型且具备所需端点与生图能力的
// OpenAI 账号是否都不允许 size 所属档位，并返回该档位。查询绕过调度快照、忽略限流冷却等临时状态；
// 尺寸无法识别、分组内没有满足模型与能力的账号或查询失败时返回 false。
func (s *OpenAIGatewayService) OpenAIImageSizeTierUnsupported(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	size string,
	requiredCapability OpenAIEndpointCapability,
	requiredImageCapability OpenAIImagesCapability,
) (string, bool) {
	tier, ok := ClassifyImageBillingTier(size)
	if !ok || s == nil || s.accountRepo == nil {
		return "", false
	}
	requestedModel = strings.TrimSpace(requestedModel)
	queryGroupID := groupID
	includeGrouped := false
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		queryGroupID = nil
		includeGrouped = true
	}
	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, queryGroupID, []string{PlatformOpenAI}, includeGrouped)
	if err != nil {
		return "", false
	}
	hasModelSupport := false
	for i := range accounts {
		if requestedModel != "" && !accounts[i].IsModelSupported(requestedModel) {
			continue
		}
		if !accountSupportsOpenAICapabilities(&accounts[i], requiredCapability, requiredImageCapability) {
			continue
		}
		hasModelSupport = true
		if accounts[i].SupportsOpenAIImageSizeTier(tier) {
			return "", false
		}
	}
	return tier, hasModelSupport
}

// OpenAIImageSizeTierUnsupportedMessage 是分组缺少某尺寸档位账号时返回给客户端的错误信息。
func OpenAIImageSizeTierUnsupportedMessage(size string, tier string) string {
	return fmt.Sprintf("Image size %q (%s tier) is not supported by any account in this group", strings.TrimSpace(size), tier)
}

// OpenAIResponsesImageRequestSize 返回 Responses 请求生图工具声明的尺寸，工具未声明时取顶层 size。
func OpenAIResponsesImageRequestSize(body []byte) string {
	cfg, _ := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, "")
	return cfg.InputSize
}
