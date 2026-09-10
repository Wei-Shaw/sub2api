package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var ErrAccountVisibilityInvalidUsers = infraerrors.BadRequest(
	"INVALID_VISIBLE_USERS",
	"one or more visible users do not exist",
)

type AccountVisibilityFilters struct {
	Platform string
	Type     string
	Status   string
	Search   string
	GroupID  int64
}

type AccountVisibilityGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type AccountVisibleUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type AccountVisibilityRepository interface {
	ListVisibleAccountIDs(ctx context.Context, userID int64, params pagination.PaginationParams, filters AccountVisibilityFilters) ([]int64, *pagination.PaginationResult, error)
	CanViewAccount(ctx context.Context, userID, accountID int64) (bool, error)
	ListVisibleGroups(ctx context.Context, userID int64) ([]AccountVisibilityGroup, error)
	ListAccountVisibleUsers(ctx context.Context, accountID int64) ([]AccountVisibleUser, error)
	ReplaceAccountVisibleUsers(ctx context.Context, accountID int64, userIDs []int64) error
}

type AccountVisibilityService struct {
	repo        AccountVisibilityRepository
	accountRepo AccountRepository
}

func NewAccountVisibilityService(repo AccountVisibilityRepository, accountRepo AccountRepository) *AccountVisibilityService {
	return &AccountVisibilityService{repo: repo, accountRepo: accountRepo}
}

func (s *AccountVisibilityService) ListVisibleAccounts(ctx context.Context, userID int64, params pagination.PaginationParams, filters AccountVisibilityFilters) ([]*Account, *pagination.PaginationResult, error) {
	ids, result, err := s.repo.ListVisibleAccountIDs(ctx, userID, params, filters)
	if err != nil {
		return nil, nil, err
	}
	accounts, err := s.accountRepo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	return accounts, result, nil
}

func (s *AccountVisibilityService) CanViewAccount(ctx context.Context, userID, accountID int64) (bool, error) {
	return s.repo.CanViewAccount(ctx, userID, accountID)
}

func (s *AccountVisibilityService) ListVisibleGroups(ctx context.Context, userID int64) ([]AccountVisibilityGroup, error) {
	return s.repo.ListVisibleGroups(ctx, userID)
}

func (s *AccountVisibilityService) ListAccountVisibleUsers(ctx context.Context, accountID int64) ([]AccountVisibleUser, error) {
	exists, err := s.accountRepo.ExistsByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrAccountNotFound
	}
	return s.repo.ListAccountVisibleUsers(ctx, accountID)
}

func (s *AccountVisibilityService) ReplaceAccountVisibleUsers(ctx context.Context, accountID int64, userIDs []int64) error {
	exists, err := s.accountRepo.ExistsByID(ctx, accountID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrAccountNotFound
	}
	return s.repo.ReplaceAccountVisibleUsers(ctx, accountID, userIDs)
}
