package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type openAIPreferredUserContextKey struct{}
type openAIPreferredOnlyContextKey struct{}

type openAIPreferredUser struct {
	groupID int64
	userID  int64
}

// WithOpenAIPreferredAccountUser uses the authenticated API key, never a
// client-supplied user or account identifier.
func WithOpenAIPreferredAccountUser(ctx context.Context, key *APIKey) context.Context {
	if key == nil || key.UserID <= 0 || key.Group == nil || key.Group.Platform != PlatformOpenAI || !key.Group.PreferUserAccounts {
		return ctx
	}
	return context.WithValue(ctx, openAIPreferredUserContextKey{}, openAIPreferredUser{
		groupID: key.Group.ID,
		userID:  key.UserID,
	})
}

func (s *OpenAIGatewayService) preferredOpenAIUserForNewSession(ctx context.Context, groupID *int64, platform, previousResponseID, sessionHash string) int64 {
	preference, ok := ctx.Value(openAIPreferredUserContextKey{}).(openAIPreferredUser)
	if !ok || groupID == nil || *groupID != preference.groupID || platform != PlatformOpenAI ||
		(s.cfg != nil && s.cfg.RunMode == config.RunModeSimple) || strings.TrimSpace(previousResponseID) != "" {
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
	return preference.userID
}

func (a *Account) prefersOpenAIUser(userID int64) bool {
	return a != nil && a.PreferredUserID != nil && *a.PreferredUserID == userID
}

func filterPreferredOpenAIAccounts(ctx context.Context, accounts []Account) []Account {
	userID, _ := ctx.Value(openAIPreferredOnlyContextKey{}).(int64)
	if userID <= 0 {
		return accounts
	}
	var preferred []Account
	for i := range accounts {
		if accounts[i].prefersOpenAIUser(userID) {
			preferred = append(preferred, accounts[i])
		}
	}
	return preferred
}
