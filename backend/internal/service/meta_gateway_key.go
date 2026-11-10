package service

import (
	"context"
	"errors"
	"strings"
)

func (s *OpenAIGatewayService) MetaAPIKey(ctx context.Context, account *Account) (string, error) {
	if account == nil || !account.IsMeta() {
		return "", errors.New("meta: account is required")
	}
	if apiKey := strings.TrimSpace(account.GetMetaAPIKey()); apiKey != "" {
		return apiKey, nil
	}
	if s == nil || s.metaTokenProvider == nil || account.GetMetaDCAToken() == "" {
		return "", errors.New("meta: api_key not found in credentials")
	}
	return s.metaTokenProvider.GetAPIKey(ctx, account)
}
