package service

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const openAIAPIKeyAccessTokenRefreshAhead = 3 * time.Minute

func resolveOpenAIAPIKeyAccessTokenForGateway(account *Account) string {
	if account == nil || !account.IsOpenAIOAuth() || account.IsOpenAIPersonalAccessToken() {
		return ""
	}
	token := account.GetOpenAIAPIKeyAccessToken()
	if !isOpenAIAPIKeyAccessTokenUsable(token, account.GetOpenAIAPIKeyAccessTokenExpiresAt()) {
		return ""
	}
	return strings.TrimSpace(token)
}

func isOpenAIAPIKeyAccessTokenUsable(token string, expiresAt *time.Time) bool {
	if strings.TrimSpace(token) == "" {
		return false
	}
	if expiresAt == nil {
		return true
	}
	return time.Until(*expiresAt) > openAIAPIKeyAccessTokenRefreshAhead
}

func decodeOpenAITokenExpiresAtUnix(token string) int64 {
	claims, err := openai.DecodeIDToken(strings.TrimSpace(token))
	if err != nil || claims == nil || claims.Exp <= 0 {
		return 0
	}
	return claims.Exp
}
