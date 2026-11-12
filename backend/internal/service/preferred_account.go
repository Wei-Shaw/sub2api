package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type preferredAccountUserContextKey struct{}
type preferredAccountsOnlyContextKey struct{}

type preferredAccountUser struct {
	groupID int64
	userID  int64
}

// WithPreferredAccountUser uses the authenticated API key, never a
// client-supplied user or account identifier.
func WithPreferredAccountUser(ctx context.Context, key *APIKey) context.Context {
	if key == nil || key.UserID <= 0 || key.Group == nil || !key.Group.PreferUserAccounts {
		return ctx
	}
	return context.WithValue(ctx, preferredAccountUserContextKey{}, preferredAccountUser{
		groupID: key.Group.ID,
		userID:  key.UserID,
	})
}

func (s *OpenAIGatewayService) preferredOpenAIUserForNewSession(ctx context.Context, groupID *int64, previousResponseID, sessionHash string) int64 {
	userID := preferredUserInGroup(ctx, groupID, s.cfg)
	if userID == 0 || strings.TrimSpace(previousResponseID) != "" {
		return 0
	}
	if _, hasParent := openAIGuardianParentAffinityFromContext(ctx); hasParent {
		return 0
	}
	if sessionHash != "" {
		accountID, err := s.getStickySessionAccountID(ctx, groupID, sessionHash)
		if accountID > 0 || (err != nil && !errors.Is(err, ErrStickySessionNotFound)) {
			return 0
		}
	}
	return userID
}

func preferredUserInGroup(ctx context.Context, groupID *int64, cfg *config.Config) int64 {
	preference, ok := ctx.Value(preferredAccountUserContextKey{}).(preferredAccountUser)
	if !ok || groupID == nil || *groupID != preference.groupID || (cfg != nil && cfg.RunMode == config.RunModeSimple) {
		return 0
	}
	return preference.userID
}

func preferredAccountsOnly(ctx context.Context) int64 {
	userID, _ := ctx.Value(preferredAccountsOnlyContextKey{}).(int64)
	return userID
}

func (a *Account) prefersUser(userID int64) bool {
	return a != nil && a.PreferredUserID != nil && *a.PreferredUserID == userID
}

func filterPreferredAccounts(ctx context.Context, accounts []Account) []Account {
	userID := preferredAccountsOnly(ctx)
	if userID <= 0 {
		return accounts
	}
	var preferred []Account
	for i := range accounts {
		if accounts[i].prefersUser(userID) {
			preferred = append(preferred, accounts[i])
		}
	}
	return preferred
}
