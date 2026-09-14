package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adobe"
)

// resolveAdobeAccessToken returns an IMS access token for an Adobe OAuth account.
//
// The background AdobeTokenRefresher keeps tokens warm, but a request can still
// land on an empty or soon-to-expire token (new account, missed cycle). Refresh
// on the hot path and persist the result so the next call can skip IMS.
//
// If the cookie is missing and the stored token is already expired, the old
// token is returned so the upstream 401 is the source of truth — matching the
// account-test path.
func resolveAdobeAccessToken(ctx context.Context, repo AccountRepository, refresher *AdobeTokenRefresher, account *Account) (string, error) {
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if refresher == nil {
		if token == "" {
			return "", errors.New("access_token not found in credentials")
		}
		return token, nil
	}

	if token != "" && !adobe.IsTokenExpired(token, adobeRefreshWindow) {
		return token, nil
	}

	if !refresher.CanRefresh(account) {
		if token == "" {
			return "", errors.New("access_token not found in credentials")
		}
		return token, nil
	}

	credentials, err := refresher.Refresh(ctx, account)
	if err != nil {
		return "", err
	}
	refreshed := strings.TrimSpace(credentialString(credentials, "access_token"))
	if refreshed == "" {
		return "", errors.New("adobe refresh returned an empty access_token")
	}
	if persistErr := persistAccountCredentials(ctx, repo, account, credentials); persistErr != nil {
		slog.Warn("failed to persist refreshed adobe access_token",
			"account_id", account.ID, "error", persistErr)
	}
	return refreshed, nil
}

func (s *GatewayService) getAdobeOAuthToken(ctx context.Context, account *Account) (string, error) {
	return resolveAdobeAccessToken(ctx, s.accountRepo, s.adobeTokenRefresher, account)
}
