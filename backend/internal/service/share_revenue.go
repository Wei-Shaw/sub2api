package service

import (
	"context"
	"log/slog"
	"math"
	"strings"
)

// 收益分配模式（design: share-revenue-split.md）
const (
	RevenueModeLegacy          = "legacy"
	RevenueModeShareSplit      = "share_split"
	RevenueModeSelfPrivateEnv  = "self_private_env"
)

// ShareRevenuePlan 一笔 usage 的分账计划（在扣费前/后使用）。
type ShareRevenuePlan struct {
	Mode           string
	TotalCost      float64 // 正常计费 C
	BilledAmount   float64 // 实际扣 B 的金额
	InviteAmount   float64
	UserAmount     float64
	PlatformAmount float64
	OwnerUserID    *int64
	InviterUserID  *int64
}

// ShareRevenueSettings 全局分账配置快照。
type ShareRevenueSettings struct {
	Enabled            bool
	InvitePct          float64
	UserPct            float64
	PlatformPct        float64
	PrivateSelfEnvPct  float64
	AffiliateEnabled   bool
}

// ResolveShareRevenueMode 判定计费模式。
// private 自用优先于 share_split。
func ResolveShareRevenueMode(enabled bool, group *Group, account *Account, callerUserID int64) string {
	if !enabled || account == nil || callerUserID <= 0 {
		return RevenueModeLegacy
	}
	// self private: private-{caller}-* 且账号 owner=caller
	if group != nil && IsPrivateGroupNameForUser(group.Name, callerUserID) {
		if account.OwnerUserID != nil && *account.OwnerUserID == callerUserID {
			return RevenueModeSelfPrivateEnv
		}
	}
	// share split: 共享池 + 有 owner；owner==caller 时仍 legacy（避免自己赚自己）
	if group != nil && group.IsSharePool && account.OwnerUserID != nil && *account.OwnerUserID > 0 {
		if *account.OwnerUserID == callerUserID {
			return RevenueModeLegacy
		}
		return RevenueModeShareSplit
	}
	return RevenueModeLegacy
}

// ComputeShareRevenuePlan 根据模式与配置计算分账金额。
// inviterUserID 在无邀请人或 affiliate 关闭时传 nil。
func ComputeShareRevenuePlan(mode string, totalCost float64, cfg ShareRevenueSettings, ownerUserID *int64, inviterUserID *int64) ShareRevenuePlan {
	plan := ShareRevenuePlan{
		Mode:          mode,
		TotalCost:     totalCost,
		BilledAmount:  totalCost,
		OwnerUserID:   ownerUserID,
		InviterUserID: inviterUserID,
	}
	if totalCost <= 0 || mode == "" || mode == RevenueModeLegacy {
		plan.Mode = RevenueModeLegacy
		plan.PlatformAmount = totalCost
		return plan
	}

	switch mode {
	case RevenueModeSelfPrivateEnv:
		r := cfg.PrivateSelfEnvPct
		if r < 0 {
			r = 0
		}
		if r > 100 {
			r = 100
		}
		billed := floorMoney(totalCost * r / 100)
		plan.BilledAmount = billed
		plan.PlatformAmount = billed
		plan.InviteAmount = 0
		plan.UserAmount = 0
		return plan

	case RevenueModeShareSplit:
		invitePct, userPct, platformPct := normalizeShareSplitPercents(cfg.InvitePct, cfg.UserPct, cfg.PlatformPct)
		userAmt := floorMoney(totalCost * userPct / 100)
		inviteAmt := floorMoney(totalCost * invitePct / 100)

		// 无邀请人或邀请功能关：invite 并入平台
		useInvite := inviterUserID != nil && *inviterUserID > 0 && cfg.AffiliateEnabled
		if !useInvite {
			inviteAmt = 0
			plan.InviterUserID = nil
		}

		// owner 校验
		if ownerUserID == nil || *ownerUserID <= 0 {
			// 不应进入 share_split；兜底全平台
			plan.Mode = RevenueModeLegacy
			plan.PlatformAmount = totalCost
			plan.UserAmount = 0
			plan.InviteAmount = 0
			return plan
		}

		plan.UserAmount = userAmt
		plan.InviteAmount = inviteAmt
		plan.PlatformAmount = roundMoney(totalCost - userAmt - inviteAmt)
		if plan.PlatformAmount < 0 {
			plan.PlatformAmount = 0
		}
		_ = platformPct
		return plan

	default:
		plan.Mode = RevenueModeLegacy
		plan.PlatformAmount = totalCost
		return plan
	}
}

func normalizeShareSplitPercents(invite, user, platform float64) (float64, float64, float64) {
	if invite < 0 {
		invite = 0
	}
	if user < 0 {
		user = 0
	}
	if platform < 0 {
		platform = 0
	}
	sum := invite + user + platform
	if sum <= 0 {
		return 0, 0, 100
	}
	// 若和不为 100，按比例缩放；platform 用余量兜底
	if math.Abs(sum-100) > 0.01 {
		invite = invite * 100 / sum
		user = user * 100 / sum
		platform = 100 - invite - user
	}
	return invite, user, platform
}

func floorMoney(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	// 与常见余额 6 位小数对齐
	return math.Floor(v*1e6) / 1e6
}

func roundMoney(v float64) float64 {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*1e6) / 1e6
}

// LoadShareRevenueSettings 从 SettingService 读取配置。
func LoadShareRevenueSettings(ctx context.Context, s *SettingService) ShareRevenueSettings {
	cfg := ShareRevenueSettings{
		Enabled:           false,
		InvitePct:         10,
		UserPct:           40,
		PlatformPct:       50,
		PrivateSelfEnvPct: 1,
		AffiliateEnabled:  false,
	}
	if s == nil {
		return cfg
	}
	cfg.Enabled = s.IsShareRevenueSplitEnabled(ctx)
	cfg.InvitePct = s.GetShareSplitInvitePct(ctx)
	cfg.UserPct = s.GetShareSplitUserPct(ctx)
	cfg.PlatformPct = s.GetShareSplitPlatformPct(ctx)
	cfg.PrivateSelfEnvPct = s.GetPrivateSelfEnvFeePct(ctx)
	cfg.AffiliateEnabled = s.IsAffiliateEnabled(ctx)
	return cfg
}

// ApplyShareRevenueCredits 在 B 扣费成功后：给 A / 邀请人加余额，并写 ledger。
// billedAlready 表示 B 已按 plan.BilledAmount 扣费完成。
func ApplyShareRevenueCredits(
	ctx context.Context,
	userRepo UserRepository,
	billingCache *BillingCacheService,
	ledger ShareRevenueLedgerWriter,
	requestID string,
	callerUserID int64,
	accountID int64,
	groupID *int64,
	plan ShareRevenuePlan,
) {
	if plan.Mode == RevenueModeLegacy || plan.Mode == "" {
		return
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return
	}

	// 贡献者入账
	if plan.UserAmount > 0 && plan.OwnerUserID != nil && *plan.OwnerUserID > 0 && *plan.OwnerUserID != callerUserID {
		if err := userRepo.UpdateBalance(ctx, *plan.OwnerUserID, plan.UserAmount); err != nil {
			slog.Error("share_revenue_credit_owner_failed",
				"request_id", requestID,
				"owner_user_id", *plan.OwnerUserID,
				"amount", plan.UserAmount,
				"error", err.Error(),
			)
		} else if billingCache != nil {
			_ = billingCache.InvalidateUserBalance(ctx, *plan.OwnerUserID)
		}
	}

	// 邀请人入账
	if plan.InviteAmount > 0 && plan.InviterUserID != nil && *plan.InviterUserID > 0 {
		if err := userRepo.UpdateBalance(ctx, *plan.InviterUserID, plan.InviteAmount); err != nil {
			slog.Error("share_revenue_credit_inviter_failed",
				"request_id", requestID,
				"inviter_user_id", *plan.InviterUserID,
				"amount", plan.InviteAmount,
				"error", err.Error(),
			)
		} else if billingCache != nil {
			_ = billingCache.InvalidateUserBalance(ctx, *plan.InviterUserID)
		}
	}

	if ledger != nil {
		var gid int64
		if groupID != nil {
			gid = *groupID
		}
		if err := ledger.InsertShareRevenueLedger(ctx, &ShareRevenueLedgerRow{
			RequestID:     requestID,
			UsageUserID:   callerUserID,
			AccountID:     accountID,
			GroupID:       gid,
			RevenueMode:   plan.Mode,
			TotalCost:     plan.TotalCost,
			BilledAmount:  plan.BilledAmount,
			InviteAmount:  plan.InviteAmount,
			UserAmount:    plan.UserAmount,
			PlatformAmount: plan.PlatformAmount,
			OwnerUserID:   plan.OwnerUserID,
			InviterUserID: plan.InviterUserID,
		}); err != nil {
			slog.Error("share_revenue_ledger_insert_failed",
				"request_id", requestID,
				"error", err.Error(),
			)
		}
	}

	slog.Info("share_revenue_applied",
		"request_id", requestID,
		"mode", plan.Mode,
		"total", plan.TotalCost,
		"billed", plan.BilledAmount,
		"invite", plan.InviteAmount,
		"user", plan.UserAmount,
		"platform", plan.PlatformAmount,
	)
}

// ShareRevenueLedgerRow 流水写入行。
type ShareRevenueLedgerRow struct {
	RequestID      string
	UsageUserID    int64
	AccountID      int64
	GroupID        int64
	RevenueMode    string
	TotalCost      float64
	BilledAmount   float64
	InviteAmount   float64
	UserAmount     float64
	PlatformAmount float64
	OwnerUserID    *int64
	InviterUserID  *int64
}

// ShareRevenueLedgerWriter 流水写入。
type ShareRevenueLedgerWriter interface {
	InsertShareRevenueLedger(ctx context.Context, row *ShareRevenueLedgerRow) error
}

// AffiliateInviterLookup 查询用户的邀请人。
type AffiliateInviterLookup interface {
	GetAffiliateInviterUserID(ctx context.Context, userID int64) (*int64, error)
}

// ShareRevenueQuery 用户侧贡献收益查询。
type ShareRevenueQuery interface {
	// SummarizeContributor 汇总作为贡献者(owner)的 user_amount。
	SummarizeContributor(ctx context.Context, ownerUserID int64) (*ShareRevenueSummary, error)
	// ListContributorLedgers 分页列出作为贡献者的流水。
	ListContributorLedgers(ctx context.Context, ownerUserID int64, page, pageSize int) ([]ShareRevenueLedgerItem, int64, error)
}

// ShareRevenueSummary 贡献收益汇总。
type ShareRevenueSummary struct {
	TotalEarned   float64 `json:"total_earned"`
	TotalRecords  int64   `json:"total_records"`
	Enabled       bool    `json:"enabled"`
	UserPct       float64 `json:"user_pct"`
	InvitePct     float64 `json:"invite_pct"`
	PlatformPct   float64 `json:"platform_pct"`
}

// ShareRevenueLedgerItem 用户可见的流水项。
type ShareRevenueLedgerItem struct {
	ID           int64   `json:"id"`
	RequestID    string  `json:"request_id"`
	AccountID    int64   `json:"account_id"`
	GroupID      int64   `json:"group_id"`
	RevenueMode  string  `json:"revenue_mode"`
	TotalCost    float64 `json:"total_cost"`
	UserAmount   float64 `json:"user_amount"`
	CreatedAt    string  `json:"created_at"`
}

// ShareRevenueService 用户贡献收益查询服务。
type ShareRevenueService struct {
	query    ShareRevenueQuery
	settings *SettingService
}

// NewShareRevenueService 构造。
func NewShareRevenueService(query ShareRevenueQuery, settings *SettingService) *ShareRevenueService {
	return &ShareRevenueService{query: query, settings: settings}
}

// GetMySummary 当前用户作为贡献者的汇总。
func (s *ShareRevenueService) GetMySummary(ctx context.Context, userID int64) (*ShareRevenueSummary, error) {
	cfg := LoadShareRevenueSettings(ctx, s.settings)
	out := &ShareRevenueSummary{
		Enabled:     cfg.Enabled,
		UserPct:     cfg.UserPct,
		InvitePct:   cfg.InvitePct,
		PlatformPct: cfg.PlatformPct,
	}
	if s.query == nil || userID <= 0 {
		return out, nil
	}
	sum, err := s.query.SummarizeContributor(ctx, userID)
	if err != nil {
		return nil, err
	}
	if sum != nil {
		out.TotalEarned = sum.TotalEarned
		out.TotalRecords = sum.TotalRecords
	}
	return out, nil
}

// ListMyLedgers 当前用户作为贡献者的流水。
func (s *ShareRevenueService) ListMyLedgers(ctx context.Context, userID int64, page, pageSize int) ([]ShareRevenueLedgerItem, int64, error) {
	if s.query == nil || userID <= 0 {
		return []ShareRevenueLedgerItem{}, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return s.query.ListContributorLedgers(ctx, userID, page, pageSize)
}
