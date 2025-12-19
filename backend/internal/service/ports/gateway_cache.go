package ports

import (
	"context"
	"time"
)

// GatewayCache defines cache operations for gateway service
type GatewayCache interface {
	GetSessionAccountID(ctx context.Context, sessionHash string) (int64, error)
	SetSessionAccountID(ctx context.Context, sessionHash string, accountID int64, ttl time.Duration) error
	RefreshSessionTTL(ctx context.Context, sessionHash string, ttl time.Duration) error
REDACTED
