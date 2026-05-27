package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"

	"golang.org/x/crypto/bcrypt"
)

// OpenAPIService 编排 /api/v1/openapi 命名空间下的业务动作。
// 自身不直接持有 ent client，全部通过既有 service / repo 完成。
type OpenAPIService struct {
	userRepo      UserRepository
	balanceOpRepo BalanceOperationRepository
	userService   *UserService
	apiKeyService *APIKeyService
	usageService  *UsageService
}

// NewOpenAPIService 构造函数。
func NewOpenAPIService(
	userRepo UserRepository,
	balanceOpRepo BalanceOperationRepository,
	userService *UserService,
	apiKeyService *APIKeyService,
	usageService *UsageService,
) *OpenAPIService {
	return &OpenAPIService{
		userRepo:      userRepo,
		balanceOpRepo: balanceOpRepo,
		userService:   userService,
		apiKeyService: apiKeyService,
		usageService:  usageService,
	}
}

var (
	ErrOpenAPIEmailRequired    = errors.New("email is required")
	ErrOpenAPIUserNotFound     = errors.New("user not found")
	ErrOpenAPIDuplicateOpID    = errors.New("operation already pending")
	ErrOpenAPIOpFailed         = errors.New("operation previously failed, use a new external_op_id")
	ErrOpenAPIInvalidOpType    = errors.New("op_type must be 'set' or 'add'")
	ErrOpenAPINegativeAmount   = errors.New("amount must be >= 0")
	ErrOpenAPIBalanceUnderflow = errors.New("operation would make balance negative")
)

// OpenAPICreateUserRequest M2M 创建用户入参。
type OpenAPICreateUserRequest struct {
	Email          string
	ExternalUserID string
	InitialBalance float64
	KeyName        string
	GroupID        *int64
}

// OpenAPICreateUserResult 创建用户结果。仅 FirstTime=true 时包含 APIKey 明文。
type OpenAPICreateUserResult struct {
	UserID    int64
	Email     string
	Status    string
	Balance   float64
	APIKey    string
	APIKeyID  int64
	FirstTime bool
}

// OpenAPIAdjustBalanceRequest 调整余额入参。
type OpenAPIAdjustBalanceRequest struct {
	ExternalOpID string
	OpType       string
	Amount       float64
	Note         string
	Payload      map[string]any
}

// OpenAPIAdjustBalanceResult 调整余额结果。
type OpenAPIAdjustBalanceResult struct {
	OperationID      int64
	UserID           int64
	Email            string
	OpType           string
	Amount           float64
	BalanceBefore    float64
	BalanceAfter     float64
	IdempotentReplay bool
}

// OpenAPIUsageListParams 用量明细分页过滤。
type OpenAPIUsageListParams struct {
	APIKeyID *int64
	Model    string
	Page     int
	PageSize int
}

// OpenAPIUsageStatsParams 用量聚合查询参数。
type OpenAPIUsageStatsParams struct {
	StartAt time.Time
	EndAt   time.Time
}

// CreateUser 按 email 创建或返回既有用户。email 已存在则返回基础信息且 FirstTime=false。
func (s *OpenAPIService) CreateUser(ctx context.Context, req OpenAPICreateUserRequest) (*OpenAPICreateUserResult, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return nil, ErrOpenAPIEmailRequired
	}

	existing, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if existing != nil {
		return &OpenAPICreateUserResult{
			UserID:    existing.ID,
			Email:     existing.Email,
			Status:    existing.Status,
			Balance:   existing.Balance,
			FirstTime: false,
		}, nil
	}

	// 生成随机密码，bcrypt 入库；密码永不外露
	rawPassword, err := generateRandomPassword(32)
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	newUser := &User{
		Email:        email,
		PasswordHash: string(hashed),
		Role:         RoleUser,
		Status:       StatusActive,
		Balance:      req.InitialBalance,
		Concurrency:  5,
	}
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	keyName := req.KeyName
	if keyName == "" {
		keyName = "default"
	}
	keyReq := CreateAPIKeyRequest{
		Name:    keyName,
		GroupID: req.GroupID,
	}
	apiKey, err := s.apiKeyService.Create(ctx, newUser.ID, keyReq)
	if err != nil {
		return nil, fmt.Errorf("create initial api key: %w", err)
	}

	return &OpenAPICreateUserResult{
		UserID:    newUser.ID,
		Email:     newUser.Email,
		Status:    newUser.Status,
		Balance:   newUser.Balance,
		APIKey:    apiKey.Key,
		APIKeyID:  apiKey.ID,
		FirstTime: true,
	}, nil
}

// GetUserByEmail 按 email 查询用户。找不到返回 ErrOpenAPIUserNotFound。
func (s *OpenAPIService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrOpenAPIEmailRequired
	}
	u, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrOpenAPIUserNotFound
		}
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	if u == nil {
		return nil, ErrOpenAPIUserNotFound
	}
	return u, nil
}

// AdjustBalance 调整指定 email 用户的余额。external_op_id 作为幂等键。
func (s *OpenAPIService) AdjustBalance(ctx context.Context, email string, req OpenAPIAdjustBalanceRequest) (*OpenAPIAdjustBalanceResult, error) {
	if req.OpType != "set" && req.OpType != "add" {
		return nil, ErrOpenAPIInvalidOpType
	}
	if req.Amount < 0 {
		return nil, ErrOpenAPINegativeAmount
	}
	if strings.TrimSpace(req.ExternalOpID) == "" {
		return nil, errors.New("external_op_id is required")
	}

	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// 幂等检查
	existing, err := s.balanceOpRepo.FindByExternalOpID(ctx, req.ExternalOpID)
	if err != nil {
		return nil, fmt.Errorf("idempotency lookup: %w", err)
	}
	if existing != nil {
		switch existing.Status {
		case "succeeded":
			return &OpenAPIAdjustBalanceResult{
				OperationID:      existing.ID,
				UserID:           existing.UserID,
				Email:            user.Email,
				OpType:           existing.OpType,
				Amount:           existing.Amount,
				BalanceBefore:    existing.BalanceBefore,
				BalanceAfter:     existing.BalanceAfter,
				IdempotentReplay: true,
			}, nil
		case "pending":
			return nil, ErrOpenAPIDuplicateOpID
		case "failed":
			return nil, ErrOpenAPIOpFailed
		}
	}

	noteCopy := req.Note
	pending, err := s.balanceOpRepo.CreatePending(ctx, BalanceOperation{
		ExternalOpID:   req.ExternalOpID,
		UserID:         user.ID,
		OpType:         req.OpType,
		Amount:         req.Amount,
		Note:           &noteCopy,
		RequestPayload: req.Payload,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateExternalOpID) {
			// 极小概率：刚才 Find 没查到但 Insert 被并发抢先。重新读一次决定。
			latest, _ := s.balanceOpRepo.FindByExternalOpID(ctx, req.ExternalOpID)
			if latest != nil && latest.Status == "succeeded" {
				return &OpenAPIAdjustBalanceResult{
					OperationID:      latest.ID,
					UserID:           latest.UserID,
					Email:            user.Email,
					OpType:           latest.OpType,
					Amount:           latest.Amount,
					BalanceBefore:    latest.BalanceBefore,
					BalanceAfter:     latest.BalanceAfter,
					IdempotentReplay: true,
				}, nil
			}
			return nil, ErrOpenAPIDuplicateOpID
		}
		return nil, fmt.Errorf("create pending op: %w", err)
	}

	// 重新读最新余额（在调整前快照）
	fresh, err := s.userRepo.GetByID(ctx, user.ID)
	if err != nil {
		_ = s.balanceOpRepo.MarkFailed(ctx, pending.ID, "fetch user balance: "+err.Error())
		return nil, fmt.Errorf("fetch user: %w", err)
	}
	before := fresh.Balance

	var after float64
	switch req.OpType {
	case "set":
		after = req.Amount
	case "add":
		after = before + req.Amount
	}
	if after < 0 {
		_ = s.balanceOpRepo.MarkFailed(ctx, pending.ID, "balance would underflow")
		return nil, ErrOpenAPIBalanceUnderflow
	}

	delta := after - before
	if err := s.userService.UpdateBalance(ctx, user.ID, delta); err != nil {
		_ = s.balanceOpRepo.MarkFailed(ctx, pending.ID, err.Error())
		return nil, fmt.Errorf("update balance: %w", err)
	}

	if err := s.balanceOpRepo.MarkSucceeded(ctx, pending.ID, before, after); err != nil {
		return nil, fmt.Errorf("mark succeeded: %w", err)
	}

	return &OpenAPIAdjustBalanceResult{
		OperationID:      pending.ID,
		UserID:           user.ID,
		Email:            user.Email,
		OpType:           req.OpType,
		Amount:           req.Amount,
		BalanceBefore:    before,
		BalanceAfter:     after,
		IdempotentReplay: false,
	}, nil
}

// CreateKeyForEmail 代用户生成一个新 LLM API Key。
func (s *OpenAPIService) CreateKeyForEmail(ctx context.Context, email string, req CreateAPIKeyRequest) (*APIKey, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return s.apiKeyService.Create(ctx, user.ID, req)
}

// ListKeysForEmail 列该用户所有 keys。
func (s *OpenAPIService) ListKeysForEmail(ctx context.Context, email string, filters APIKeyListFilters) ([]APIKey, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	params := pagination.PaginationParams{
		Page:      1,
		PageSize:  200,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
	keys, _, err := s.apiKeyService.List(ctx, user.ID, params, filters)
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// UpdateKeyForEmail 校验归属后改 key。
func (s *OpenAPIService) UpdateKeyForEmail(ctx context.Context, email string, keyID int64, req UpdateAPIKeyRequest) (*APIKey, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return s.apiKeyService.Update(ctx, keyID, user.ID, req)
}

// DeleteKeyForEmail 校验归属后删 key。
func (s *OpenAPIService) DeleteKeyForEmail(ctx context.Context, email string, keyID int64) error {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return err
	}
	return s.apiKeyService.Delete(ctx, keyID, user.ID)
}

// ListUsageForEmail 取该用户的消费明细分页。
func (s *OpenAPIService) ListUsageForEmail(ctx context.Context, email string, params OpenAPIUsageListParams) ([]UsageLog, *pagination.PaginationResult, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, nil, err
	}
	page := params.Page
	if page <= 0 {
		page = 1
	}
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	pp := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
	return s.usageService.ListByUser(ctx, user.ID, pp)
}

// UsageStatsForEmail 取该用户的消费聚合（默认最近 7 天）。
func (s *OpenAPIService) UsageStatsForEmail(ctx context.Context, email string, params OpenAPIUsageStatsParams) (*UsageStats, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	end := params.EndAt
	if end.IsZero() {
		end = time.Now()
	}
	start := params.StartAt
	if start.IsZero() {
		start = end.Add(-7 * 24 * time.Hour)
	}
	if end.Sub(start) > 90*24*time.Hour {
		return nil, errors.New("time window exceeds 90 days")
	}
	return s.usageService.GetStatsByUser(ctx, user.ID, start, end)
}

// generateRandomPassword 生成 hex 形式的强随机密码。仅用于 M2M 建号场景。
func generateRandomPassword(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
