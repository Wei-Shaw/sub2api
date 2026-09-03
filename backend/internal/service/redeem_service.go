package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var (
	ErrRedeemCodeNotFound  = infraerrors.NotFound("REDEEM_CODE_NOT_FOUND", "redeem code not found")
	ErrRedeemCodeUsed      = infraerrors.Conflict("REDEEM_CODE_USED", "redeem code already used")
	ErrRedeemCodeExpired   = infraerrors.Conflict("REDEEM_CODE_EXPIRED", "redeem code expired")
	ErrInsufficientBalance = infraerrors.BadRequest("INSUFFICIENT_BALANCE", "insufficient balance")
	ErrRedeemRateLimited   = infraerrors.TooManyRequests("REDEEM_RATE_LIMITED", "too many failed attempts, please try again later")
	ErrRedeemCodeLocked    = infraerrors.Conflict("REDEEM_CODE_LOCKED", "redeem code is being processed, please try again")
)

const (
	redeemMaxErrorsPerHour  = 20
	redeemRateLimitDuration = time.Hour
	redeemLockDuration      = 10 * time.Second // 锁超时时间，防止死锁
)

type ctxKeySkipRedeemAffiliate struct{}

// ContextSkipRedeemAffiliate returns a context that suppresses the redeem-level
// affiliate rebate. Used by payment fulfillment which handles rebate separately
// via applyAffiliateRebateForOrder (with audit-log deduplication).
func ContextSkipRedeemAffiliate(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipRedeemAffiliate{}, true)
}

type ctxKeyRedeemPostCommit struct{}

// redeemPostCommitCollector 收集「必须等事务提交后才能执行」的收尾动作。
type redeemPostCommitCollector struct {
	mu    sync.Mutex
	hooks []func(context.Context)
}

func (c *redeemPostCommitCollector) add(fn func(context.Context)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hooks = append(c.hooks, fn)
}

func (c *redeemPostCommitCollector) run(ctx context.Context) {
	c.mu.Lock()
	hooks := c.hooks
	c.hooks = nil
	c.mu.Unlock()
	for _, fn := range hooks {
		fn(ctx)
	}
}

// ContextWithRedeemPostCommit 返回带「提交后回调收集器」的 context，以及必须在
// 调用方 Commit 成功之后调用的执行函数。
//
// Redeem 复用 context 中已有事务时，缓存失效与邀请返利不能在 Redeem 内部直接执行：
// 那一刻外层事务还没提交，失效 L1 只会把提交前的旧行重新灌回缓存（与 0a2191222
// 修掉的问题同源），返利也可能为一笔最终回滚的兑换入账。这些动作因此登记到收集器，
// 由持有事务的调用方在提交之后统一执行。
func ContextWithRedeemPostCommit(ctx context.Context) (context.Context, func(context.Context)) {
	c := &redeemPostCommitCollector{}
	return context.WithValue(ctx, ctxKeyRedeemPostCommit{}, c), c.run
}

func redeemPostCommitFromContext(ctx context.Context) *redeemPostCommitCollector {
	c, _ := ctx.Value(ctxKeyRedeemPostCommit{}).(*redeemPostCommitCollector)
	return c
}

// RedeemCache defines cache operations for redeem service
type RedeemCache interface {
	GetRedeemAttemptCount(ctx context.Context, userID int64) (int, error)
	IncrementRedeemAttemptCount(ctx context.Context, userID int64) error

	// AcquireRedeemLock 以 token 作为持有者标识抢锁；ReleaseRedeemLock 必须比较
	// token 后再删除，防止锁 TTL 到期后误删下一个持有者的锁。
	AcquireRedeemLock(ctx context.Context, code, token string, ttl time.Duration) (bool, error)
	ReleaseRedeemLock(ctx context.Context, code, token string) error
}

type RedeemCodeRepository interface {
	Create(ctx context.Context, code *RedeemCode) error
	CreateBatch(ctx context.Context, codes []RedeemCode) error
	GetByID(ctx context.Context, id int64) (*RedeemCode, error)
	GetByCode(ctx context.Context, code string) (*RedeemCode, error)
	Update(ctx context.Context, code *RedeemCode) error
	BatchUpdate(ctx context.Context, ids []int64, fields RedeemCodeBatchUpdateFields) (int64, error)
	Delete(ctx context.Context, id int64) error
	Use(ctx context.Context, id, userID int64) error

	List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error)
	ListWithFilters(ctx context.Context, params pagination.PaginationParams, codeType, status, search string) ([]RedeemCode, *pagination.PaginationResult, error)
	ListByUser(ctx context.Context, userID int64, limit int) ([]RedeemCode, error)
	// ListByUserPaginated returns paginated balance/concurrency history for a specific user.
	// codeType filter is optional - pass empty string to return all types.
	ListByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCode, *pagination.PaginationResult, error)
	// SumPositiveBalanceByUser returns the total recharged amount (sum of positive balance values) for a user.
	SumPositiveBalanceByUser(ctx context.Context, userID int64) (float64, error)
}

// GenerateCodesRequest 生成兑换码请求
type GenerateCodesRequest struct {
	Count int     `json:"count"`
	Value float64 `json:"value"`
	Type  string  `json:"type"`
}

// RedeemCodeResponse 兑换码响应
type RedeemCodeResponse struct {
	Code      string    `json:"code"`
	Value     float64   `json:"value"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type NullableTimeUpdate struct {
	Set   bool
	Value *time.Time
}

type NullableInt64Update struct {
	Set   bool
	Value *int64
}

type RedeemCodeBatchUpdateFields struct {
	Status    *string
	ExpiresAt NullableTimeUpdate
	Notes     *string
	GroupID   NullableInt64Update

	// Core fields are intentionally modeled only so service validation can
	// reject payloads that try to mutate redemption value semantics in bulk.
	Type  *string
	Value *float64
}

func (f RedeemCodeBatchUpdateFields) HasChanges() bool {
	return f.Status != nil ||
		f.ExpiresAt.Set ||
		f.Notes != nil ||
		f.GroupID.Set ||
		f.Type != nil ||
		f.Value != nil
}

func (f RedeemCodeBatchUpdateFields) HasCoreFieldChanges() bool {
	return f.Type != nil || f.Value != nil
}

func (f RedeemCodeBatchUpdateFields) TouchesUsedSensitiveFields() bool {
	return f.Status != nil || f.ExpiresAt.Set || f.GroupID.Set
}

type RedeemCodeBatchUpdateInput struct {
	IDs    []int64
	Fields RedeemCodeBatchUpdateFields
}

type RedeemCodeBatchUpdateResult struct {
	Updated int64 `json:"updated"`
}

// RedeemService 兑换码服务
type RedeemService struct {
	redeemRepo           RedeemCodeRepository
	userRepo             UserRepository
	redeemUserRepo       RedeemUserAdjustmentRepository
	subscriptionService  *SubscriptionService
	cache                RedeemCache
	billingCacheService  *BillingCacheService
	entClient            *dbent.Client
	authCacheInvalidator APIKeyAuthCacheInvalidator
	affiliateService     *AffiliateService
}

// NewRedeemService 创建兑换码服务实例
func NewRedeemService(
	redeemRepo RedeemCodeRepository,
	userRepo UserRepository,
	subscriptionService *SubscriptionService,
	cache RedeemCache,
	billingCacheService *BillingCacheService,
	entClient *dbent.Client,
	authCacheInvalidator APIKeyAuthCacheInvalidator,
	affiliateService *AffiliateService,
) *RedeemService {
	redeemUserRepo, _ := userRepo.(RedeemUserAdjustmentRepository)
	return &RedeemService{
		redeemRepo:           redeemRepo,
		userRepo:             userRepo,
		redeemUserRepo:       redeemUserRepo,
		subscriptionService:  subscriptionService,
		cache:                cache,
		billingCacheService:  billingCacheService,
		entClient:            entClient,
		authCacheInvalidator: authCacheInvalidator,
		affiliateService:     affiliateService,
	}
}

// GenerateRandomCode 生成随机兑换码
func (s *RedeemService) GenerateRandomCode() (string, error) {
	// 生成16字节随机数据
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}

	// 转换为十六进制字符串
	code := hex.EncodeToString(bytes)

	// 格式化为 XXXX-XXXX-XXXX-XXXX 格式
	parts := []string{
		strings.ToUpper(code[0:8]),
		strings.ToUpper(code[8:16]),
		strings.ToUpper(code[16:24]),
		strings.ToUpper(code[24:32]),
	}

	return strings.Join(parts, "-"), nil
}

// GenerateCodes 批量生成兑换码
func (s *RedeemService) GenerateCodes(ctx context.Context, req GenerateCodesRequest) ([]RedeemCode, error) {
	if req.Count <= 0 {
		return nil, errors.New("count must be greater than 0")
	}

	// 邀请码类型不需要数值，其他类型需要非零值（支持负数用于退款）
	if req.Type != RedeemTypeInvitation && req.Value == 0 {
		return nil, errors.New("value must not be zero")
	}

	if req.Count > 1000 {
		return nil, errors.New("cannot generate more than 1000 codes at once")
	}

	codeType := req.Type
	if codeType == "" {
		codeType = RedeemTypeBalance
	}

	// 邀请码类型的 value 设为 0
	value := req.Value
	if codeType == RedeemTypeInvitation {
		value = 0
	}

	codes := make([]RedeemCode, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code, err := s.GenerateRandomCode()
		if err != nil {
			return nil, fmt.Errorf("generate code: %w", err)
		}

		codes = append(codes, RedeemCode{
			Code:   code,
			Type:   codeType,
			Value:  value,
			Status: StatusUnused,
		})
	}

	// 批量插入
	if err := s.redeemRepo.CreateBatch(ctx, codes); err != nil {
		return nil, fmt.Errorf("create batch codes: %w", err)
	}

	return codes, nil
}

// CreateCode creates a redeem code with caller-provided code value.
// It is primarily used by admin integrations that require an external order ID
// to be mapped to a deterministic redeem code.
func (s *RedeemService) CreateCode(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return errors.New("redeem code is required")
	}
	code.Code = strings.TrimSpace(code.Code)
	if code.Code == "" {
		return errors.New("code is required")
	}
	if code.Type == "" {
		code.Type = RedeemTypeBalance
	}
	if code.Type != RedeemTypeInvitation && code.Value == 0 {
		return errors.New("value must not be zero")
	}
	if code.Status == "" {
		code.Status = StatusUnused
	}
	if code.IsExpired() {
		return ErrRedeemCodeExpired
	}

	if err := s.redeemRepo.Create(ctx, code); err != nil {
		return fmt.Errorf("create redeem code: %w", err)
	}
	return nil
}

func (s *RedeemService) BatchUpdate(ctx context.Context, input *RedeemCodeBatchUpdateInput) (*RedeemCodeBatchUpdateResult, error) {
	if input == nil {
		return nil, infraerrors.BadRequest("REDEEM_CODE_BATCH_UPDATE_INVALID", "batch update input is required")
	}
	if len(input.IDs) == 0 {
		return nil, infraerrors.BadRequest("REDEEM_CODE_BATCH_UPDATE_IDS_REQUIRED", "ids are required")
	}
	if !input.Fields.HasChanges() {
		return nil, infraerrors.BadRequest("REDEEM_CODE_BATCH_UPDATE_EMPTY", "at least one field must be selected")
	}
	if input.Fields.HasCoreFieldChanges() {
		return nil, infraerrors.BadRequest("REDEEM_CODE_CORE_FIELDS_IMMUTABLE", "type and value cannot be batch updated")
	}

	ids := make([]int64, 0, len(input.IDs))
	seen := make(map[int64]struct{}, len(input.IDs))
	for _, id := range input.IDs {
		if id <= 0 {
			return nil, infraerrors.BadRequest("REDEEM_CODE_BATCH_UPDATE_INVALID_ID", "ids must be positive")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("REDEEM_CODE_BATCH_UPDATE_IDS_REQUIRED", "ids are required")
	}

	if input.Fields.Status != nil {
		switch *input.Fields.Status {
		case StatusUnused, StatusDisabled:
		default:
			return nil, infraerrors.BadRequest("REDEEM_CODE_STATUS_INVALID", "status must be unused or disabled")
		}
	}
	if input.Fields.ExpiresAt.Set && input.Fields.ExpiresAt.Value != nil {
		expiresAt := input.Fields.ExpiresAt.Value.UTC()
		if !expiresAt.After(time.Now().UTC()) {
			return nil, infraerrors.BadRequest("REDEEM_CODE_EXPIRES_AT_INVALID", "expires_at must be in the future")
		}
		input.Fields.ExpiresAt.Value = &expiresAt
	}
	if input.Fields.GroupID.Set && input.Fields.GroupID.Value != nil && *input.Fields.GroupID.Value <= 0 {
		return nil, infraerrors.BadRequest("REDEEM_CODE_GROUP_ID_INVALID", "group_id must be positive")
	}

	updated, err := s.redeemRepo.BatchUpdate(ctx, ids, input.Fields)
	if err != nil {
		return nil, err
	}
	return &RedeemCodeBatchUpdateResult{Updated: updated}, nil
}

// checkRedeemRateLimit 检查用户兑换错误次数是否超限
func (s *RedeemService) checkRedeemRateLimit(ctx context.Context, userID int64) error {
	if s.cache == nil {
		return nil
	}

	count, err := s.cache.GetRedeemAttemptCount(ctx, userID)
	if err != nil {
		// Redis 出错时不阻止用户操作
		return nil
	}

	if count >= redeemMaxErrorsPerHour {
		return ErrRedeemRateLimited
	}

	return nil
}

// incrementRedeemErrorCount 增加用户兑换错误计数
func (s *RedeemService) incrementRedeemErrorCount(ctx context.Context, userID int64) {
	if s.cache == nil {
		return
	}

	_ = s.cache.IncrementRedeemAttemptCount(ctx, userID)
}

// newRedeemLockToken 生成锁持有者令牌，语义与 repository 层
// newBatchImageLockToken 一致（16 字节随机数的十六进制表示）。
func newRedeemLockToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// acquireRedeemLock 尝试获取兑换码的分布式锁。
// 返回 (是否可继续, 持有者令牌)。令牌为空表示未真正持锁（降级路径），
// 此时 releaseRedeemLock 不会删除任何键，避免误删他人的锁。
func (s *RedeemService) acquireRedeemLock(ctx context.Context, code string) (bool, string) {
	if s.cache == nil {
		return true, "" // 无 Redis 时降级为不加锁
	}

	token, err := newRedeemLockToken()
	if err != nil {
		// 令牌生成失败时降级为不加锁，依赖数据库层面的状态检查
		return true, ""
	}

	ok, err := s.cache.AcquireRedeemLock(ctx, code, token, redeemLockDuration)
	if err != nil {
		// Redis 出错时不阻止操作，依赖数据库层面的状态检查
		return true, ""
	}
	if !ok {
		return false, ""
	}
	return true, token
}

// releaseRedeemLock 释放兑换码的分布式锁（仅当锁仍由本次持有者持有）
func (s *RedeemService) releaseRedeemLock(ctx context.Context, code, token string) {
	if s.cache == nil || token == "" {
		return
	}

	_ = s.cache.ReleaseRedeemLock(ctx, code, token)
}

func unsupportedRedeemTypeError(codeType string) error {
	if codeType == RedeemTypeInvitation {
		return infraerrors.BadRequest("REDEEM_CODE_UNSUPPORTED_TYPE", "invitation codes can only be used during registration")
	}
	return infraerrors.BadRequest("REDEEM_CODE_UNSUPPORTED_TYPE", fmt.Sprintf("unsupported redeem type: %s", codeType))
}

// Redeem 使用兑换码
func (s *RedeemService) Redeem(ctx context.Context, userID int64, code string) (*RedeemCode, error) {
	// 检查限流
	if err := s.checkRedeemRateLimit(ctx, userID); err != nil {
		return nil, err
	}

	// 获取分布式锁，防止同一兑换码并发使用
	acquired, lockToken := s.acquireRedeemLock(ctx, code)
	if !acquired {
		return nil, ErrRedeemCodeLocked
	}
	defer s.releaseRedeemLock(ctx, code, lockToken)

	// 查找兑换码
	redeemCode, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) {
			s.incrementRedeemErrorCount(ctx, userID)
			return nil, ErrRedeemCodeNotFound
		}
		return nil, fmt.Errorf("get redeem code: %w", err)
	}

	// 检查兑换码状态和码本身的过期时间
	if redeemCode.IsExpired() {
		s.incrementRedeemErrorCount(ctx, userID)
		return nil, ErrRedeemCodeExpired
	}
	if !redeemCode.CanUse() {
		s.incrementRedeemErrorCount(ctx, userID)
		return nil, ErrRedeemCodeUsed
	}

	// 验证兑换码类型的前置条件。邀请码属于注册流程，不能通过普通兑换接口使用。
	switch redeemCode.Type {
	case RedeemTypeBalance, RedeemTypeConcurrency:
	case RedeemTypeSubscription:
		if redeemCode.GroupID == nil {
			return nil, infraerrors.BadRequest("REDEEM_CODE_INVALID", "invalid subscription redeem code: missing group_id")
		}
	default:
		return nil, unsupportedRedeemTypeError(redeemCode.Type)
	}

	// 获取用户信息
	_, err = s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	// 使用数据库事务保证兑换码标记与权益发放的原子性。
	//
	// context 里已经有事务时直接复用（不再自己开一个），这样调用方可以把
	// 「创建兑换码」与「兑换」放进同一个事务：支付履约的 doBalance 原先是
	// CreateCode 一个事务、Redeem 另一个事务，两步之间崩溃会留下一条 unused
	// 的孤儿充值码。复用事务时提交由调用方负责，本函数的 commit 是空操作。
	ownsTx := dbent.TxFromContext(ctx) == nil
	txCtx := ctx
	commit := func() error { return nil }
	if ownsTx {
		if s.entClient == nil {
			return nil, errors.New("redeem service is not configured with a database client")
		}
		tx, err := s.entClient.Tx(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin transaction: %w", err)
		}
		defer func() { _ = tx.Rollback() }()

		// 将事务放入 context，使 repository 方法能够使用同一事务
		txCtx = dbent.NewTxContext(ctx, tx)
		commit = tx.Commit
	}

	// 【关键】先标记兑换码为已使用，确保并发安全
	// 利用数据库乐观锁（WHERE status = 'unused'）保证原子性
	if err := s.redeemRepo.Use(txCtx, redeemCode.ID, userID); err != nil {
		if errors.Is(err, ErrRedeemCodeNotFound) || errors.Is(err, ErrRedeemCodeUsed) {
			return nil, ErrRedeemCodeUsed
		}
		return nil, fmt.Errorf("mark code as used: %w", err)
	}

	// 执行兑换逻辑（兑换码已被锁定，此时可安全操作）
	switch redeemCode.Type {
	case RedeemTypeBalance:
		amount := redeemCode.Value
		if amount < 0 {
			if s.redeemUserRepo == nil {
				return nil, errors.New("user repository does not support atomic redeem balance adjustments")
			}
			if err := s.redeemUserRepo.ApplyRedeemBalanceAdjustment(txCtx, userID, amount); err != nil {
				return nil, fmt.Errorf("update user balance: %w", err)
			}
		} else if err := s.userRepo.UpdateBalance(txCtx, userID, amount); err != nil {
			return nil, fmt.Errorf("update user balance: %w", err)
		}

	case RedeemTypeConcurrency:
		delta := int(redeemCode.Value)
		if delta < 0 {
			if s.redeemUserRepo == nil {
				return nil, errors.New("user repository does not support atomic redeem concurrency adjustments")
			}
			if err := s.redeemUserRepo.ApplyRedeemConcurrencyAdjustment(txCtx, userID, delta); err != nil {
				return nil, fmt.Errorf("update user concurrency: %w", err)
			}
		} else if err := s.userRepo.UpdateConcurrency(txCtx, userID, delta); err != nil {
			return nil, fmt.Errorf("update user concurrency: %w", err)
		}

	case RedeemTypeSubscription:
		validityDays := redeemCode.ValidityDays
		if validityDays < 0 {
			// 负数天数：缩短订阅，减到 0 则取消订阅
			if err := s.reduceOrCancelSubscription(txCtx, userID, *redeemCode.GroupID, -validityDays, redeemCode.Code); err != nil {
				return nil, fmt.Errorf("reduce or cancel subscription: %w", err)
			}
		} else {
			if validityDays == 0 {
				validityDays = 30
			}
			// deferCacheInvalidation=true：兑换事务尚未提交，此处失效 L1 会让并发
			// 请求把提交前的旧行重新灌回缓存。缓存统一在提交后由
			// invalidateRedeemCaches 同步失效（与支付履约路径共用同一套处理）。
			_, _, err := s.subscriptionService.assignOrExtendSubscription(txCtx, &AssignSubscriptionInput{
				UserID:       userID,
				GroupID:      *redeemCode.GroupID,
				ValidityDays: validityDays,
				AssignedBy:   0, // 系统分配
				Notes:        fmt.Sprintf("通过兑换码 %s 兑换", redeemCode.Code),
			}, true)
			if err != nil {
				return nil, fmt.Errorf("assign or extend subscription: %w", err)
			}
		}

	default:
		return nil, unsupportedRedeemTypeError(redeemCode.Type)
	}

	// 提交事务（复用外部事务时是空操作，由调用方提交）
	if err := commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	// 提交后的收尾：失效缓存 + 余额类正数兑换码触发邀请返利
	//（best-effort，失败不影响兑换结果）。
	//
	// 是否返利必须在这里、用兑换时的 ctx 判定：回调可能由调用方用另一个 ctx
	// 触发，而 ContextSkipRedeemAffiliate 标记只挂在兑换时的 ctx 上，
	// 否则支付履约路径会在 applyAffiliateRebateForOrder 之外再返一次。
	shouldAccrue := redeemCode.Type == RedeemTypeBalance &&
		redeemCode.Value > 0 &&
		ctx.Value(ctxKeySkipRedeemAffiliate{}) == nil
	postCommit := func(hookCtx context.Context) {
		s.invalidateRedeemCaches(hookCtx, userID, redeemCode)
		if shouldAccrue {
			s.tryAccrueAffiliateRebateForRedeem(hookCtx, userID, redeemCode.Value)
		}
	}
	if collector := redeemPostCommitFromContext(ctx); !ownsTx && collector != nil {
		collector.add(postCommit)
	} else {
		// 自己持有事务，或调用方没有登记收集器（此时只能尽力而为）。
		postCommit(ctx)
	}

	// 重新获取更新后的兑换码
	redeemCode, err = s.redeemRepo.GetByID(txCtx, redeemCode.ID)
	if err != nil {
		return nil, fmt.Errorf("get updated redeem code: %w", err)
	}

	return redeemCode, nil
}

// invalidateRedeemCaches 失效兑换相关的缓存
func (s *RedeemService) invalidateRedeemCaches(ctx context.Context, userID int64, redeemCode *RedeemCode) {
	switch redeemCode.Type {
	case RedeemTypeBalance:
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		if s.billingCacheService == nil {
			return
		}
		go func() {
			cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.billingCacheService.InvalidateUserBalance(cacheCtx, userID)
		}()
	case RedeemTypeConcurrency:
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		if s.billingCacheService == nil {
			return
		}
	case RedeemTypeSubscription:
		if s.authCacheInvalidator != nil {
			s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
		}
		if redeemCode.GroupID == nil || s.subscriptionService == nil {
			return
		}
		// 复用支付履约路径的同一套失效逻辑：同步失效 L1 + Redis，并广播跨实例
		// 失效消息，否则其他实例的 L1 会保留提交前的旧订阅直到 TTL 过期。
		if err := s.subscriptionService.invalidateSubscriptionCaches(userID, *redeemCode.GroupID); err != nil {
			logger.LegacyPrintf("service.redeem", "[Redeem] invalidate subscription cache failed for user %d group %d: %v", userID, *redeemCode.GroupID, err)
		}
	}
}

func (s *RedeemService) tryAccrueAffiliateRebateForRedeem(ctx context.Context, userID int64, amount float64) {
	if ctx.Value(ctxKeySkipRedeemAffiliate{}) != nil {
		return
	}
	if s.affiliateService == nil {
		return
	}
	if !s.affiliateService.IsEnabled(ctx) {
		return
	}
	rebate, err := s.affiliateService.AccrueInviteRebate(ctx, userID, amount)
	if err != nil {
		logger.LegacyPrintf("service.redeem", "[Redeem] affiliate rebate failed for user %d amount %.2f: %v", userID, amount, err)
		return
	}
	if rebate > 0 {
		logger.LegacyPrintf("service.redeem", "[Redeem] affiliate rebate accrued %.8f for inviter of user %d", rebate, userID)
	}
}

// GetByID 根据ID获取兑换码
func (s *RedeemService) GetByID(ctx context.Context, id int64) (*RedeemCode, error) {
	code, err := s.redeemRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get redeem code: %w", err)
	}
	return code, nil
}

// GetByCode 根据Code获取兑换码
func (s *RedeemService) GetByCode(ctx context.Context, code string) (*RedeemCode, error) {
	redeemCode, err := s.redeemRepo.GetByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("get redeem code: %w", err)
	}
	return redeemCode, nil
}

// List 获取兑换码列表（管理员功能）
func (s *RedeemService) List(ctx context.Context, params pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	codes, pagination, err := s.redeemRepo.List(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list redeem codes: %w", err)
	}
	return codes, pagination, nil
}

// Delete 删除兑换码（管理员功能）
func (s *RedeemService) Delete(ctx context.Context, id int64) error {
	// 检查兑换码是否存在
	code, err := s.redeemRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get redeem code: %w", err)
	}

	// 不允许删除已使用的兑换码
	if code.IsUsed() {
		return infraerrors.Conflict("REDEEM_CODE_DELETE_USED", "cannot delete used redeem code")
	}

	if err := s.redeemRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete redeem code: %w", err)
	}

	return nil
}

// GetStats 获取兑换码统计信息
func (s *RedeemService) GetStats(ctx context.Context) (map[string]any, error) {
	// TODO: 实现统计逻辑
	// 统计未使用、已使用的兑换码数量
	// 统计总面值等

	stats := map[string]any{
		"total_codes":  0,
		"unused_codes": 0,
		"used_codes":   0,
		"total_value":  0.0,
	}

	return stats, nil
}

// GetUserHistory 获取用户的兑换历史
func (s *RedeemService) GetUserHistory(ctx context.Context, userID int64, limit int) ([]RedeemCode, error) {
	codes, err := s.redeemRepo.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("get user redeem history: %w", err)
	}
	return codes, nil
}

// reduceOrCancelSubscription 缩短订阅天数，剩余天数 <= 0 时取消订阅。
//
// expires_at / notes 都是读改写字段，必须在事务内用 GetByIDForUpdate 加行锁后
// 重新读取，否则会与并发续期（updateExistingSubscriptionTerm、支付履约）互相
// 覆盖：先读到旧过期时间的一方会把对方刚写入的天数与备注一起抹掉。
func (s *RedeemService) reduceOrCancelSubscription(ctx context.Context, userID, groupID int64, reduceDays int, code string) error {
	// 仅用于解析订阅 ID；参与计算的字段一律以事务内加锁读取的行为准。
	located, err := s.subscriptionService.userSubRepo.GetByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return ErrSubscriptionNotFound
	}
	subscriptionID := located.ID

	notes := fmt.Sprintf("通过兑换码 %s 退款扣减 %d 天", code, reduceDays)

	// 缓存失效交由事务提交后的 invalidateRedeemCaches 处理；
	// 在事务内失效会把提交前的旧行重新灌回 L1。
	return s.subscriptionService.withSubscriptionUpdateTx(ctx, func(txCtx context.Context) error {
		sub, err := s.subscriptionService.userSubRepo.GetByIDForUpdate(txCtx, subscriptionID)
		if err != nil {
			return fmt.Errorf("lock subscription for reduction: %w", err)
		}

		now := time.Now()
		remaining := int(sub.ExpiresAt.Sub(now).Hours() / 24)
		if remaining < 0 {
			remaining = 0
		}

		if remaining <= reduceDays {
			// 剩余天数不足，直接取消订阅
			if err := s.subscriptionService.userSubRepo.UpdateStatus(txCtx, sub.ID, SubscriptionStatusExpired); err != nil {
				return fmt.Errorf("cancel subscription: %w", err)
			}
			// 设置过期时间为当前时间
			if err := s.subscriptionService.userSubRepo.ExtendExpiry(txCtx, sub.ID, now); err != nil {
				return fmt.Errorf("set subscription expiry: %w", err)
			}
		} else {
			// 缩短天数
			newExpiresAt := sub.ExpiresAt.AddDate(0, 0, -reduceDays)
			if err := s.subscriptionService.userSubRepo.ExtendExpiry(txCtx, sub.ID, newExpiresAt); err != nil {
				return fmt.Errorf("reduce subscription: %w", err)
			}
		}

		// 追加备注（基于加锁读取到的 notes，避免覆盖并发写入的备注）
		if err := s.subscriptionService.userSubRepo.UpdateNotes(txCtx, sub.ID, appendSubscriptionNotes(sub.Notes, notes)); err != nil {
			return fmt.Errorf("update subscription notes: %w", err)
		}

		return nil
	})
}
