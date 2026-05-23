package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	BillingSourceTypeSubscription = "subscription"
	BillingSourceTypeBalance      = "balance"

	APIKeyBillingModeStrict                     = "strict"
	APIKeyBillingModePrimaryThenBalance         = "primary_then_balance"
	APIKeyBillingModePrimaryThenPoolThenBalance = "primary_then_pool_then_balance"
)

var (
	ErrBillingPoolNotFound           = infraerrors.NotFound("BILLING_POOL_NOT_FOUND", "billing pool not found")
	ErrBillingPoolExists             = infraerrors.Conflict("BILLING_POOL_EXISTS", "billing pool code already exists")
	ErrBillingPoolDisabled           = infraerrors.Forbidden("BILLING_POOL_DISABLED", "billing pool is disabled")
	ErrBillingPoolGroupNotAllowed    = infraerrors.BadRequest("BILLING_POOL_GROUP_NOT_ALLOWED", "group is not allowed in billing pool")
	ErrBillingPoolInvalidOrder       = infraerrors.BadRequest("BILLING_CHAIN_INVALID_ORDER", "custom fallback order is invalid")
	ErrBillingModeNotAllowed         = infraerrors.BadRequest("BILLING_MODE_NOT_ALLOWED", "billing mode is not allowed for this group")
	ErrPrimarySubscriptionRequired   = infraerrors.Forbidden("PRIMARY_SUBSCRIPTION_REQUIRED", "active primary subscription is required")
	ErrFallbackGroupNotAllowed       = infraerrors.BadRequest("FALLBACK_GROUP_NOT_ALLOWED", "fallback group is not allowed for this key")
	ErrBillingPoolPlatformViolation  = infraerrors.BadRequest("BILLING_POOL_PLATFORM_SCOPE_VIOLATION", "billing pool platform scope violation")
	ErrBillingPoolSubscriptionOnly   = infraerrors.BadRequest("BILLING_POOL_SUBSCRIPTION_ONLY", "only subscription groups can join billing pool")
	ErrBillingPoolPrimaryGroupAbsent = infraerrors.BadRequest("BILLING_POOL_PRIMARY_GROUP_ABSENT", "billing pool must include the route group")
	ErrBillingPoolServiceUnavailable = infraerrors.ServiceUnavailable("BILLING_POOL_SERVICE_UNAVAILABLE", "billing pool runtime is not configured")
)

type BillingPool struct {
	ID                         int64
	Name                       string
	Code                       string
	Description                string
	Status                     string
	PlatformScope              string
	AllowUserReorder           bool
	RequirePrimarySubscription bool
	AllowBalanceFallback       bool
	Members                    []BillingPoolMember
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type BillingPoolMember struct {
	ID            int64
	BillingPoolID int64
	GroupID       int64
	ChainOrder    int
	CanBePrimary  bool
	CanBeFallback bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Group         *Group
}

type BillingPoolLookup struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	PlatformScope string `json:"platform_scope"`
}

type BillingPoolListFilters struct {
	Status        string
	Search        string
	PlatformScope string
}

type CreateBillingPoolInput struct {
	Name                       string
	Code                       string
	Description                string
	Status                     string
	PlatformScope              string
	AllowUserReorder           bool
	RequirePrimarySubscription bool
	AllowBalanceFallback       bool
	Members                    []BillingPoolMemberInput
}

type UpdateBillingPoolInput struct {
	Name                       *string
	Code                       *string
	Description                *string
	Status                     *string
	PlatformScope              *string
	AllowUserReorder           *bool
	RequirePrimarySubscription *bool
	AllowBalanceFallback       *bool
}

type BillingPoolMemberInput struct {
	GroupID       int64
	ChainOrder    int
	CanBePrimary  bool
	CanBeFallback bool
}

type BillingPoolRepository interface {
	Create(ctx context.Context, pool *BillingPool) error
	GetByID(ctx context.Context, id int64) (*BillingPool, error)
	GetByCode(ctx context.Context, code string) (*BillingPool, error)
	Update(ctx context.Context, pool *BillingPool) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, filters BillingPoolListFilters) ([]BillingPool, *pagination.PaginationResult, error)
	ListLookup(ctx context.Context) ([]BillingPoolLookup, error)
	GetByGroupID(ctx context.Context, groupID int64) (*BillingPool, error)
	ReplaceMembers(ctx context.Context, poolID int64, members []BillingPoolMember) error
}

type BillingPoolManager interface {
	Create(ctx context.Context, input *CreateBillingPoolInput) (*BillingPool, error)
	GetByID(ctx context.Context, id int64) (*BillingPool, error)
	List(ctx context.Context, page, pageSize int, filters BillingPoolListFilters, sortBy, sortOrder string) ([]BillingPool, *pagination.PaginationResult, error)
	ListLookup(ctx context.Context) ([]BillingPoolLookup, error)
	Update(ctx context.Context, id int64, input *UpdateBillingPoolInput) (*BillingPool, error)
	Delete(ctx context.Context, id int64) error
	ReplaceMembers(ctx context.Context, poolID int64, inputs []BillingPoolMemberInput) (*BillingPool, error)
	GetByGroupID(ctx context.Context, groupID int64) (*BillingPool, error)
}

type BillingPoolService struct {
	repo      BillingPoolRepository
	groupRepo GroupRepository
}

func NewBillingPoolService(repo BillingPoolRepository, groupRepo GroupRepository) *BillingPoolService {
	return &BillingPoolService{repo: repo, groupRepo: groupRepo}
}

func (s *BillingPoolService) ensureRepoConfigured() error {
	if s == nil || s.repo == nil {
		return ErrBillingPoolServiceUnavailable
	}
	return nil
}

func NormalizeAPIKeyBillingMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case APIKeyBillingModeStrict:
		return APIKeyBillingModeStrict
	case APIKeyBillingModePrimaryThenPoolThenBalance:
		return APIKeyBillingModePrimaryThenPoolThenBalance
	case APIKeyBillingModePrimaryThenBalance:
		fallthrough
	default:
		return APIKeyBillingModePrimaryThenBalance
	}
}

func NormalizeBillingPoolPlatformScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case string(BillingPoolPlatformScopeMixedPlatform):
		return string(BillingPoolPlatformScopeMixedPlatform)
	case string(BillingPoolPlatformScopeSamePlatform):
		fallthrough
	default:
		return string(BillingPoolPlatformScopeSamePlatform)
	}
}

func (p *BillingPool) IsActive() bool {
	return p != nil && strings.EqualFold(p.Status, StatusActive)
}

func (p *BillingPool) SortedFallbackMembers() []BillingPoolMember {
	if p == nil || len(p.Members) == 0 {
		return nil
	}
	out := make([]BillingPoolMember, 0, len(p.Members))
	for _, member := range p.Members {
		if member.CanBeFallback {
			out = append(out, member)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ChainOrder == out[j].ChainOrder {
			return out[i].GroupID < out[j].GroupID
		}
		return out[i].ChainOrder < out[j].ChainOrder
	})
	return out
}

func (s *BillingPoolService) Create(ctx context.Context, input *CreateBillingPoolInput) (*BillingPool, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "billing pool input is required")
	}
	pool := &BillingPool{
		Name:                       strings.TrimSpace(input.Name),
		Code:                       strings.TrimSpace(input.Code),
		Description:                strings.TrimSpace(input.Description),
		Status:                     StatusActive,
		PlatformScope:              NormalizeBillingPoolPlatformScope(input.PlatformScope),
		AllowUserReorder:           input.AllowUserReorder,
		RequirePrimarySubscription: true,
		AllowBalanceFallback:       true,
	}
	if pool.Name == "" || pool.Code == "" {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "name and code are required")
	}
	if strings.TrimSpace(input.Status) != "" {
		pool.Status = strings.TrimSpace(input.Status)
	}
	pool.RequirePrimarySubscription = input.RequirePrimarySubscription
	pool.AllowBalanceFallback = input.AllowBalanceFallback
	if err := s.populateMembers(ctx, pool, input.Members); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, pool); err != nil {
		return nil, fmt.Errorf("create billing pool: %w", err)
	}
	return s.repo.GetByID(ctx, pool.ID)
}

func (s *BillingPoolService) GetByID(ctx context.Context, id int64) (*BillingPool, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, err
	}
	pool, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get billing pool: %w", err)
	}
	return pool, nil
}

func (s *BillingPoolService) List(ctx context.Context, page, pageSize int, filters BillingPoolListFilters, sortBy, sortOrder string) ([]BillingPool, *pagination.PaginationResult, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, nil, err
	}
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder}
	return s.repo.List(ctx, params, filters)
}

func (s *BillingPoolService) ListLookup(ctx context.Context) ([]BillingPoolLookup, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, err
	}
	return s.repo.ListLookup(ctx)
}

func (s *BillingPoolService) Update(ctx context.Context, id int64, input *UpdateBillingPoolInput) (*BillingPool, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "billing pool input is required")
	}
	pool, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get billing pool: %w", err)
	}
	if input.Name != nil {
		pool.Name = strings.TrimSpace(*input.Name)
	}
	if input.Code != nil {
		pool.Code = strings.TrimSpace(*input.Code)
	}
	if input.Description != nil {
		pool.Description = strings.TrimSpace(*input.Description)
	}
	if input.Status != nil {
		pool.Status = strings.TrimSpace(*input.Status)
	}
	if input.PlatformScope != nil {
		pool.PlatformScope = NormalizeBillingPoolPlatformScope(*input.PlatformScope)
	}
	if input.AllowUserReorder != nil {
		pool.AllowUserReorder = *input.AllowUserReorder
	}
	if input.RequirePrimarySubscription != nil {
		pool.RequirePrimarySubscription = *input.RequirePrimarySubscription
	}
	if input.AllowBalanceFallback != nil {
		pool.AllowBalanceFallback = *input.AllowBalanceFallback
	}
	if pool.Name == "" || pool.Code == "" {
		return nil, infraerrors.BadRequest("INVALID_INPUT", "name and code are required")
	}
	if err := s.repo.Update(ctx, pool); err != nil {
		return nil, fmt.Errorf("update billing pool: %w", err)
	}
	return s.repo.GetByID(ctx, id)
}

func (s *BillingPoolService) Delete(ctx context.Context, id int64) error {
	if err := s.ensureRepoConfigured(); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete billing pool: %w", err)
	}
	return nil
}

func (s *BillingPoolService) ReplaceMembers(ctx context.Context, poolID int64, inputs []BillingPoolMemberInput) (*BillingPool, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, err
	}
	pool, err := s.repo.GetByID(ctx, poolID)
	if err != nil {
		return nil, fmt.Errorf("get billing pool: %w", err)
	}
	if err := s.populateMembers(ctx, pool, inputs); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceMembers(ctx, poolID, pool.Members); err != nil {
		return nil, fmt.Errorf("replace billing pool members: %w", err)
	}
	return s.repo.GetByID(ctx, poolID)
}

func (s *BillingPoolService) GetByGroupID(ctx context.Context, groupID int64) (*BillingPool, error) {
	if err := s.ensureRepoConfigured(); err != nil {
		return nil, err
	}
	pool, err := s.repo.GetByGroupID(ctx, groupID)
	if err != nil {
		return nil, fmt.Errorf("get billing pool by group: %w", err)
	}
	return pool, nil
}

func (s *BillingPoolService) populateMembers(ctx context.Context, pool *BillingPool, inputs []BillingPoolMemberInput) error {
	if pool == nil {
		return infraerrors.BadRequest("INVALID_INPUT", "billing pool is required")
	}
	members := make([]BillingPoolMember, 0, len(inputs))
	seen := make(map[int64]struct{}, len(inputs))
	for i, input := range inputs {
		if input.GroupID <= 0 {
			return infraerrors.BadRequest("INVALID_INPUT", "group_id must be positive")
		}
		if _, ok := seen[input.GroupID]; ok {
			return ErrBillingPoolInvalidOrder.WithMetadata(map[string]string{"group_id": fmt.Sprintf("%d", input.GroupID)})
		}
		seen[input.GroupID] = struct{}{}
		group, err := s.groupRepo.GetByIDLite(ctx, input.GroupID)
		if err != nil {
			return err
		}
		if group == nil || !group.IsSubscriptionType() {
			return ErrBillingPoolSubscriptionOnly.WithMetadata(map[string]string{"group_id": fmt.Sprintf("%d", input.GroupID)})
		}
		members = append(members, BillingPoolMember{
			BillingPoolID: pool.ID,
			GroupID:       input.GroupID,
			ChainOrder:    input.ChainOrder,
			CanBePrimary:  input.CanBePrimary,
			CanBeFallback: input.CanBeFallback,
			Group:         group,
			CreatedAt:     time.Now().Add(time.Duration(i) * time.Nanosecond),
		})
	}
	pool.Members = members
	return nil
}
