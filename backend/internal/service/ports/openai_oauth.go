package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// OpenAIOAuthClient interface for OpenAI OAuth operations
type OpenAIOAuthClient interface {
	ExchangeCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL string) (*openai.TokenResponse, error)
	RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*openai.TokenResponse, error)
}
