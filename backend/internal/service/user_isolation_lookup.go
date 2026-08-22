package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const userIsolationLookupPageSize = 1000
const managedUserIsolationIDLength = len("u1_") + 43

var (
	ErrUserIsolationLookupInvalidID = infraerrors.BadRequest(
		"INVALID_USER_ISOLATION_ID",
		"user isolation ID must be a valid u1 identifier",
	)
	ErrUserIsolationLookupDisabled = infraerrors.BadRequest(
		"USER_ISOLATION_NOT_ENABLED",
		"user isolation is not enabled for this account",
	)
	ErrUserIsolationLookupNotFound = infraerrors.NotFound(
		"USER_ISOLATION_USER_NOT_FOUND",
		"no user matches this isolation ID for the selected account",
	)
)

type UserIsolationLookupService struct {
	userRepo    UserRepository
	accountRepo AccountRepository
	cfg         *config.Config
}

type UserIsolationLookupAccount struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	Type     string `json:"type"`
}

type UserIsolationLookupUser struct {
	ID           int64      `json:"id"`
	Email        string     `json:"email"`
	Username     string     `json:"username"`
	Status       string     `json:"status"`
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

type UserIsolationLookupResult struct {
	Account UserIsolationLookupAccount `json:"account"`
	User    UserIsolationLookupUser    `json:"user"`
}

func NewUserIsolationLookupService(
	userRepo UserRepository,
	accountRepo AccountRepository,
	cfg *config.Config,
) *UserIsolationLookupService {
	return &UserIsolationLookupService{
		userRepo:    userRepo,
		accountRepo: accountRepo,
		cfg:         cfg,
	}
}

func (s *UserIsolationLookupService) Lookup(
	ctx context.Context,
	accountID int64,
	isolationID string,
) (*UserIsolationLookupResult, error) {
	if accountID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_ACCOUNT_ID", "account ID must be positive")
	}
	normalizedID, err := normalizeManagedUserIsolationID(isolationID)
	if err != nil {
		return nil, err
	}
	if s == nil || s.accountRepo == nil || s.userRepo == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "USER_ISOLATION_LOOKUP_UNAVAILABLE", "user isolation lookup is unavailable")
	}

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, ErrAccountNotFound
	}
	if !account.IsUserIsolationEnabled() || !supportsManagedUserIsolationAccount(account) {
		return nil, ErrUserIsolationLookupDisabled
	}
	if s.cfg == nil || strings.TrimSpace(s.cfg.Security.UserIsolationSecret) == "" {
		return nil, infraerrors.New(http.StatusInternalServerError, "USER_ISOLATION_SECRET_UNAVAILABLE", "user isolation secret is unavailable")
	}

	includeSubscriptions := false
	for page := 1; ; page++ {
		users, pageResult, err := s.userRepo.ListWithFilters(
			ctx,
			pagination.PaginationParams{
				Page:      page,
				PageSize:  userIsolationLookupPageSize,
				SortBy:    "id",
				SortOrder: pagination.SortOrderAsc,
			},
			UserListFilters{IncludeSubscriptions: &includeSubscriptions},
		)
		if err != nil {
			return nil, err
		}
		for i := range users {
			candidate := deriveManagedUserIsolationID(s.cfg.Security.UserIsolationSecret, account, users[i].ID)
			if hmac.Equal([]byte(candidate), []byte(normalizedID)) {
				return newUserIsolationLookupResult(account, &users[i]), nil
			}
		}
		if len(users) < userIsolationLookupPageSize || pageResult == nil || page >= pageResult.Pages {
			break
		}
	}

	return nil, ErrUserIsolationLookupNotFound
}

func normalizeManagedUserIsolationID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) != managedUserIsolationIDLength || !strings.HasPrefix(value, "u1_") {
		return "", ErrUserIsolationLookupInvalidID
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, "u1_"))
	if err != nil || len(decoded) != sha256.Size {
		return "", ErrUserIsolationLookupInvalidID
	}
	return value, nil
}

func newUserIsolationLookupResult(account *Account, user *User) *UserIsolationLookupResult {
	return &UserIsolationLookupResult{
		Account: UserIsolationLookupAccount{
			ID:       account.ID,
			Name:     account.Name,
			Platform: account.Platform,
			Type:     account.Type,
		},
		User: UserIsolationLookupUser{
			ID:           user.ID,
			Email:        user.Email,
			Username:     user.Username,
			Status:       user.Status,
			LastActiveAt: user.LastActiveAt,
			LastUsedAt:   user.LastUsedAt,
		},
	}
}
